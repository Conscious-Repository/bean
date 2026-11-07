package core

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Each test here is an attack from the 2026-11-06 audit, or a user error with
// the same result. The test passes when the attack fails.

const labLine = `{"type":"lab.glucose","value":{"n":90,"unit":"mg/dL"},"times":{"observed":null},"source":{"system":"labcorp","adapter":"x@1","ref_input":["bean/v0/ref","id","labcorp","S1/glucose",0]}}`

func randomSigner(t *testing.T) Signer {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	k, err := NewSecp256k1Signer(b)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func plant(t *testing.T, s *Store, obj map[string]any, k Signer) string {
	t.Helper()
	id, b, err := SignRecord(obj, k)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(s.path(PathOf(id)), b, 0o600); err != nil {
		t.Fatal(err)
	}
	return id
}

func claimBy(k Signer, author, subject, typ string, value map[string]any) map[string]any {
	return map[string]any{"v": 0.0, "nonce": NewNonce(), "type": typ, "subject": subject, "author": author, "kid": k.Public(),
		"value": value, "times": map[string]any{"observed": nil, "imported": "2026-11-06T00:00:00Z"},
		"source": map[string]any{"system": "labcorp", "adapter": "x@1", "ref": nil}}
}

func listTypes(t *testing.T, s *Store, typ string) []*Record {
	t.Helper()
	recs, _, err := s.List(Filter{Type: typ})
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

func copyDir(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.Walk(from, func(p string, i os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		rel, _ := filepath.Rel(from, p)
		if i.IsDir() {
			return os.MkdirAll(filepath.Join(to, rel), 0o700)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(to, rel), b, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Blocker 1: one signature from a stolen or retired signing key must not hide
// the identity, stop its rotation or empty its export.
func TestAttackTooNewFromSigningKey(t *testing.T) {
	s, id, words := initWithWords(t)
	if _, err := s.Add("weight", "81", AddOptions{Unit: "kg"}); err != nil {
		t.Fatal(err)
	}
	old := s.mustLocal(t).Signer
	if _, err := s.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []Signer{old, s.mustLocal(t).Signer} {
		bad := map[string]any{"v": 1.0, "nonce": NewNonce(), "type": "bean.anything", "author": id, "kid": k.Public(), "subject": id}
		plant(t, s, bad, k)
	}
	if lines, code, _ := s.VerifyLines(); code != 0 {
		t.Errorf("verify exit %d: %v", code, lines)
	}
	if got := listTypes(t, s, "weight"); len(got) != 1 {
		t.Errorf("ls shows %d records", len(got))
	}
	if n, _, err := s.Export(filepath.Join(t.TempDir(), "x")); err != nil || n == 0 {
		t.Errorf("export %d records, %v", n, err)
	}
	if _, err := s.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Errorf("rotate after the attack: %v", err)
	}
}

// A real too-new identity (the recovery key signed) stops writes, and reads
// say so instead of showing an empty log. SPEC.md 8.2.
func TestTooNewStopsWritesAndSaysSo(t *testing.T) {
	seed := make([]byte, 32)
	s := newStore(t)
	g := NewGenesis(NewEd25519Signer(seed).Public(), Commitment(RecoverySigner(seed, 0).Public()), "x@1", "2026-11-06T00:00:00Z")
	id := plant(t, s, g, NewEd25519Signer(seed))
	rec := RecoverySigner(seed, 0)
	plant(t, s, map[string]any{"v": 1.0, "nonce": NewNonce(), "type": "bean.anything", "author": id, "kid": rec.Public(), "subject": id}, rec)
	if err := writeFileAtomic(s.path(fileIdentity), []byte(id+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(s.path(fileSecret), []byte(strings.Repeat("00", 32)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(s.path(fileSignKey), []byte("ed25519:"+strings.Repeat("00", 32)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := "too new: " + id
	if _, err := s.Add("weight", "81", AddOptions{}); err == nil || err.Error() != want {
		t.Errorf("add: %v", err)
	}
	if _, _, err := s.List(Filter{}); err == nil || err.Error() != want {
		t.Errorf("list: %v", err)
	}
	if _, _, err := s.Export(filepath.Join(t.TempDir(), "x")); err == nil || err.Error() != want {
		t.Errorf("export: %v", err)
	}
	if _, err := s.Purge(id, ""); err == nil || err.Error() != want {
		t.Errorf("purge: %v", err)
	}
}

// Blocker 2: a key that copies a record's dedupe ref and grinds a lower id must
// not hide the record, and must not block a re-import.
func TestAttackDedupeSubstitution(t *testing.T) {
	s, id, _ := initWithWords(t)
	res, err := s.Append(strings.NewReader(labLine + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	orig := res.IDs[0]
	l, _ := s.Load()
	ref := l.Report.Records[orig].Source.Ref
	k := randomSigner(t)
	var forged string
	for {
		o := claimBy(k, k.Public(), id, "lab.glucose", map[string]any{"n": 400.0, "unit": "mg/dL"})
		o["source"].(map[string]any)["ref"] = ref
		if RecordID(o) < orig {
			forged = plant(t, s, o, k)
			break
		}
	}
	l, _ = s.Load()
	if _, dup := l.Report.Duplicates[orig]; dup {
		t.Error("the original is a duplicate of a stranger's record")
	}
	if got := listTypes(t, s, "lab.glucose"); len(got) != 1 || got[0].ID != orig {
		t.Errorf("ls shows %d records", len(got))
	}
	if !l.Report.Valid[forged] {
		t.Error("the stranger's claim should stay valid and visible with -all")
	}
	// Re-import still dedupes against the person's own record, not the forgery.
	if r, err := s.Append(strings.NewReader(labLine + "\n")); err != nil || r.Added != 0 || r.Skipped != 1 {
		t.Errorf("re-import: %+v %v", r, err)
	}
	// The same forgery cannot stop a first import either.
	s2, id2, _ := initWithWords(t)
	sec, _ := s2.identitySecret()
	squat := claimBy(k, k.Public(), id2, "lab.glucose", map[string]any{"n": 400.0})
	var arr []any
	for _, x := range []string{"bean/v0/ref", "id", "labcorp", "S1/glucose"} {
		arr = append(arr, x)
	}
	squat["source"].(map[string]any)["ref"] = DedupeRef(DedupeKey(sec), append(arr, 0.0))
	plant(t, s2, squat, k)
	if r, err := s2.Append(strings.NewReader(labLine + "\n")); err != nil || r.Added != 1 {
		t.Errorf("import after a squat: %+v %v", r, err)
	}
}

// Blocker 3: following the docs (sync, then rotate) must not make a thief's
// deletion permanent without the person choosing that.
func TestAttackThiefDeletionNotSealedByDefault(t *testing.T) {
	s, _, words := initWithWords(t)
	x, _ := s.Add("lab.hba1c", "9.1", AddOptions{Unit: "%"})
	d := t.TempDir()
	copyDir(t, s.Dir, d)
	thief := Open(d)
	tomb, err := thief.Purge(x, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sync(thief); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Load()
	head := before.Report.Chains[before.Me]
	_, err = s.Rotate(words, Seals{}, NewKey{})
	ue, ok := err.(*UndecidedError)
	if !ok || len(ue.Records) == 0 {
		t.Fatalf("rotate with no decision: %v", err)
	}
	after, _ := s.Load()
	if len(after.Report.Chains[after.Me]) != len(head) {
		t.Error("rotate wrote a succession with records undecided")
	}
	// A mistyped id must not fall through to -seal-all.
	if _, err := s.Rotate(words, Seals{All: true, NoSeal: []string{"abcdef"}}, NewKey{}); err == nil {
		t.Error("rotate accepted a -no-seal id that names nothing")
	}
	if _, err := s.Rotate(words, Seals{All: true, NoSeal: []string{tomb}}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	if got := listTypes(t, s, "lab.hba1c"); len(got) != 1 || got[0].ID != x {
		t.Errorf("deleted record not back: %d", len(got))
	}
}

// Blocker 4: a removed record shows in verify once a checkpoint covers it.
func TestAttackRemovedRecordDetected(t *testing.T) {
	s, id, _ := initWithWords(t)
	a, _ := s.Add("lab.hba1c", "9.1", AddOptions{Unit: "%"})
	b, _ := s.Add("weight", "81", AddOptions{Unit: "kg"})
	first, err := s.Checkpoint(nil)
	if err != nil || len(first) != 1 {
		t.Fatalf("checkpoint: %v %v", first, err)
	}
	if again, err := s.Checkpoint(nil); err != nil || len(again) != 0 {
		t.Errorf("a checkpoint with nothing new wrote %v, %v", again, err)
	}
	if lines, code, _ := s.VerifyLines(); code != 0 {
		t.Fatalf("clean log: exit %d %v", code, lines)
	}
	if err := os.Remove(s.path(PathOf(a))); err != nil {
		t.Fatal(err)
	}
	lines, code, _ := s.VerifyLines()
	if code != 1 || !contains(lines, "missing: "+a+" of "+id) {
		t.Errorf("removed record: exit %d %v", code, lines)
	}
	// A purge explains the absence. So does the tombstone being there.
	if _, err := s.Purge(b, ""); err != nil {
		t.Fatal(err)
	}
	lines, _, _ = s.VerifyLines()
	if strings.Contains(strings.Join(lines, "\n"), "missing: "+b) {
		t.Errorf("purged record reported missing: %v", lines)
	}
	// Removing an old checkpoint is seen through the next one.
	s2, _, _ := initWithWords(t)
	s2.Add("weight", "81", AddOptions{})
	c1, _ := s2.Checkpoint(nil)
	s2.Add("weight", "82", AddOptions{})
	if c2, err := s2.Checkpoint(nil); err != nil || len(c2) != 1 {
		t.Fatal(err)
	}
	os.Remove(s2.path(PathOf(c1[0])))
	if lines, code, _ := s2.VerifyLines(); code != 1 {
		t.Errorf("removed checkpoint: exit %d %v", code, lines)
	}
}

// A stranger's tombstone must not excuse a removed record, and a stranger's
// checkpoint must not fail the person's verify or vouch for anything.
func TestAttackStrangerCheckpointAndTombstone(t *testing.T) {
	s, id, _ := initWithWords(t)
	a, _ := s.Add("lab.hba1c", "9.1", AddOptions{})
	if _, err := s.Checkpoint(nil); err != nil {
		t.Fatal(err)
	}
	k := randomSigner(t)
	gid := plant(t, s, NewGenesis(k.Public(), "sha256:"+strings.Repeat("0", 64), "x@1", "2026-11-06T00:00:00Z"), k)
	bogus := "sha256:" + strings.Repeat("a", 64)
	cp := claimBy(k, gid, gid, "bean.checkpoint", map[string]any{"covers": []any{bogus}})
	delete(cp, "times")
	cp["times"] = map[string]any{"observed": nil, "imported": "2026-11-06T00:00:00Z"}
	cp["source"] = map[string]any{"system": "self", "adapter": "x@1", "ref": nil}
	plant(t, s, cp, k)
	if lines, code, _ := s.VerifyLines(); code != 0 {
		t.Errorf("a stranger's checkpoint failed verify: exit %d %v", code, lines)
	}
	os.Remove(s.path(PathOf(a)))
	plant(t, s, NewTombstone(gid, k.Public(), a, "", "x@1", "2026-11-06T00:00:00Z"), k)
	lines, code, _ := s.VerifyLines()
	if code != 1 || !contains(lines, "missing: "+a+" of "+id) {
		t.Errorf("a stranger's tombstone excused a removal: exit %d %v", code, lines)
	}
	// A stranger's checkpoint at a high position must not attest my retired claim.
	s3, id3, w3 := initWithWords(t)
	old := s3.mustLocal(t).Signer
	if _, err := s3.Rotate(w3, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	f := plant(t, s3, claimBy(old, id3, id3, "lab.hiv", map[string]any{"s": "reactive"}), old)
	gid3 := plant(t, s3, NewGenesis(k.Public(), "sha256:"+strings.Repeat("0", 64), "x@1", "2026-11-06T00:00:00Z"), k)
	cp3 := claimBy(k, gid3, gid3, "bean.checkpoint", map[string]any{"covers": []any{f}})
	cp3["source"] = map[string]any{"system": "self", "adapter": "x@1", "ref": nil}
	plant(t, s3, cp3, k)
	if l, _ := s3.Load(); !l.Report.Unattested[f] {
		t.Error("a stranger's checkpoint attested a retired key's claim")
	}
}

// Blocker 4: a retired key's new claim is not shown as the person's.
func TestAttackRetiredKeyClaim(t *testing.T) {
	s, id, words := initWithWords(t)
	mine, _ := s.Add("weight", "81", AddOptions{})
	old := s.mustLocal(t).Signer
	if _, err := s.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	if got := listTypes(t, s, "weight"); len(got) != 1 || got[0].ID != mine {
		t.Error("a sealed claim of the old key must stay current")
	}
	o := claimBy(old, id, id, "lab.hiv", map[string]any{"s": "reactive"})
	o["times"] = map[string]any{"observed": map[string]any{"start": "2020-01-01T00:00:00Z", "precision": "day"}, "imported": "2020-01-02T00:00:00Z"}
	f := plant(t, s, o, old)
	l, _ := s.Load()
	if !l.Report.Valid[f] || !l.Report.Unattested[f] || l.Current(f) {
		t.Errorf("valid %v unattested %v current %v", l.Report.Valid[f], l.Report.Unattested[f], l.Current(f))
	}
	if got := listTypes(t, s, "lab.hiv"); len(got) != 0 {
		t.Errorf("ls shows a retired key's claim: %d", len(got))
	}
	// The person can vouch for a record that really is theirs.
	if _, err := s.Checkpoint([]string{"nothing"}); err == nil {
		t.Error("-accept of a bad id")
	}
	if _, err := s.Checkpoint([]string{f}); err != nil {
		t.Fatal(err)
	}
	if got := listTypes(t, s, "lab.hiv"); len(got) != 1 {
		t.Errorf("an accepted claim should show: %d", len(got))
	}
	// A checkpoint must not sweep up an unattested claim by itself.
	g := plant(t, s, claimBy(old, id, id, "lab.other", map[string]any{"s": "x"}), old)
	if _, err := s.Add("weight", "82", AddOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Checkpoint(nil); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.Load(); !l.Report.Unattested[g] {
		t.Error("a checkpoint swept up an unattested claim")
	}
	// A retired key's own checkpoint counts only if a succession sealed it.
	cp := claimBy(old, id, id, "bean.checkpoint", map[string]any{"covers": []any{g}})
	cp["source"] = map[string]any{"system": "self", "adapter": "x@1", "ref": nil}
	c := plant(t, s, cp, old)
	if l, _ := s.Load(); l.Report.Errors[c] != "not sealed" || !l.Report.Unattested[g] {
		t.Errorf("retired checkpoint: %q, attested %v", l.Report.Errors[c], !l.Report.Unattested[g])
	}
}

// Blocker 5: reads show the person and their paired agents only.
func TestAttackStrangerInReads(t *testing.T) {
	s, id, _ := initWithWords(t)
	mine, _ := s.Add("weight", "81", AddOptions{Unit: "kg"})
	k := randomSigner(t)
	about := plant(t, s, claimBy(k, k.Public(), id, "weight", map[string]any{"n": 300.0}), k)
	other := "sha256:" + strings.Repeat("a", 64)
	plant(t, s, claimBy(k, k.Public(), other, "weight", map[string]any{"n": 300.0}), k)
	if got := listTypes(t, s, "weight"); len(got) != 1 || got[0].ID != mine {
		t.Errorf("ls shows %d records", len(got))
	}
	recs, _, _ := s.List(Filter{Type: "weight", Author: k.Public()})
	if len(recs) != 1 || recs[0].ID != about {
		t.Errorf("asking for the author by name: %d", len(recs))
	}
	if recs, _, _ := s.List(Filter{Type: "weight", All: true}); len(recs) != 3 {
		t.Errorf("-all shows %d", len(recs))
	}
	// A stranger's identity pairing their own key gets no label and no trust.
	g := plant(t, s, NewGenesis(k.Public(), "sha256:"+strings.Repeat("0", 64), "x@1", "2026-11-06T00:00:00Z"), k)
	bare := randomSigner(t)
	pair := claimBy(k, g, g, "agent.pair", map[string]any{"key": bare.Public(), "label": "assistant"})
	plant(t, s, pair, k)
	plant(t, s, claimBy(bare, bare.Public(), id, "weight", map[string]any{"n": 1.0}), bare)
	l, _ := s.Load()
	if l.AgentLabels()[bare.Public()] != "" {
		t.Error("a stranger labelled their own key")
	}
	if got := listTypes(t, s, "weight"); len(got) != 1 {
		t.Errorf("a self-paired key got into ls: %d", len(got))
	}
	// A real pairing does let the agent in.
	if _, err := s.Pair(bare.Public(), "assistant"); err != nil {
		t.Fatal(err)
	}
	if got := listTypes(t, s, "weight"); len(got) != 2 {
		t.Errorf("a paired agent should show: %d", len(got))
	}
}

// Blocker 6: rotations at the same moment queue up; they never fork.
func TestAttackConcurrentRotateDoesNotFreeze(t *testing.T) {
	for i := 0; i < 8; i++ {
		s, _, words := initWithWords(t)
		var wg sync.WaitGroup
		for j := 0; j < 2; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := Open(s.Dir).Rotate(words, Seals{All: true}, NewKey{}); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if lines, code, _ := s.VerifyLines(); code != 0 {
			t.Fatalf("exit %d %v", code, lines)
		}
	}
}

// Blocker 6: a freeze from a stale restore keeps what both branches sealed,
// and every command says so. A write stops, and migrate moves on.
func TestFreezeKeepsWhatBothSidesSealed(t *testing.T) {
	s, id, words := initWithWords(t)
	bad, _ := s.Add("lab.hba1c", "91", AddOptions{Unit: "%", Note: "typo"})
	fixed, _ := s.Add("lab.hba1c", "9.1", AddOptions{Unit: "%", Corrects: bad})
	blob, _ := s.PutBlob([]byte("raw"))
	withBlob := `{"type":"lab.report","value":{"blob":"` + blob + `"},"blobs":[{"hash":"` + blob + `","role":"raw-export","media_type":"text/plain"}],"times":{"observed":null},"source":{"system":"x","adapter":"x@1","ref":null}}`
	rep := appendLines(t, s, withBlob).IDs[0]
	note, _ := s.Add("note", "see report", AddOptions{About: rep})
	d := t.TempDir()
	copyDir(t, s.path("objects"), filepath.Join(d, "objects"))
	copyDir(t, s.path("blobs"), filepath.Join(d, "blobs"))
	stale := Open(d)
	if _, err := s.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := stale.Restore(words, Seals{All: true}, NewKey{}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sync(stale); err != nil {
		t.Fatal(err)
	}
	if lines, code, _ := s.VerifyLines(); code != 4 {
		t.Fatalf("exit %d %v", code, lines)
	}
	l, _ := s.Load()
	if !l.IsFrozen() || !l.Current(fixed) || l.Current(bad) {
		t.Errorf("frozen %v, corrected value current %v, typo current %v", l.IsFrozen(), l.Current(fixed), l.Current(bad))
	}
	want := "compromised: " + id
	if _, err := s.Add("weight", "81", AddOptions{}); err == nil || err.Error() != want {
		t.Errorf("add while frozen: %v", err)
	}
	if _, err := s.Purge(fixed, ""); err == nil || err.Error() != want {
		t.Errorf("purge while frozen: %v", err)
	}
	if _, err := s.Checkpoint(nil); err == nil || err.Error() != want {
		t.Errorf("checkpoint while frozen: %v", err)
	}
	// One side not sealing is enough to undo a correction: neither side decides alone.
	a, _, wa := initWithWords(t)
	b1, _ := a.Add("x", "1", AddOptions{})
	f1, _ := a.Add("x", "2", AddOptions{Corrects: b1})
	d2 := t.TempDir()
	copyDir(t, a.path("objects"), filepath.Join(d2, "objects"))
	st2 := Open(d2)
	if _, err := a.Rotate(wa, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st2.Restore(wa, Seals{All: true, NoSeal: []string{f1}}, NewKey{}, ""); err != nil {
		t.Fatal(err)
	}
	a.Sync(st2)
	if l, _ := a.Load(); l.Report.Errors[f1] != "frozen" {
		t.Errorf("one-sided seal: %q", l.Report.Errors[f1])
	}

	// Migrate: a new identity with the current records, blobs and links.
	dst := Open(t.TempDir())
	res, err := s.Migrate(dst, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Identity == id || len(res.Words) != 24 || res.MissingBlobs != 0 || res.Blobs != 1 {
		t.Errorf("migrate: %+v", res)
	}
	if lines, code, _ := dst.VerifyLines(); code != 0 {
		t.Errorf("migrated log: exit %d %v", code, lines)
	}
	got := listTypes(t, dst, "lab.hba1c")
	if len(got) != 1 || got[0].Value["n"] != 9.1 || got[0].Context["migrated_from"] != fixed {
		t.Errorf("migrated labs: %+v", got)
	}
	reports := listTypes(t, dst, "lab.report")
	if len(reports) != 1 {
		t.Fatalf("migrated reports: %d", len(reports))
	}
	if pt, err := dst.GetBlob(reports[0].Blobs[0].Hash); err != nil || string(pt) != "raw" || reports[0].Value["blob"] != reports[0].Blobs[0].Hash {
		t.Errorf("migrated blob: %q %v", pt, err)
	}
	notes := listTypes(t, dst, "note")
	if len(notes) != 1 || notes[0].Context["migrated_from"] != note || notes[0].Targets("about")[0] != reports[0].ID {
		t.Errorf("migrated note link: %+v", notes)
	}
	if _, err := dst.Add("weight", "81", AddOptions{}); err != nil {
		t.Errorf("the new identity must write: %v", err)
	}
	if _, err := s.Migrate(dst, NewKey{}); err == nil {
		t.Error("migrate into a used folder")
	}
}

// A writer on a copy whose key was retired is told, and rotate gets it back.
func TestRetiredKeyWriterIsRefusedThenRotates(t *testing.T) {
	a, _, words := initWithWords(t)
	b := cloneLocal(t, a)
	if _, err := a.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Sync(a); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Add("weight", "81", AddOptions{}); err == nil || !strings.HasPrefix(err.Error(), "retired key") {
		t.Errorf("add: %v", err)
	}
	if _, err := b.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Add("weight", "81", AddOptions{}); err != nil {
		t.Errorf("add after rotate: %v", err)
	}
}

// A person's writes add a checkpoint when one is due, without being asked.
func TestAutoCheckpoint(t *testing.T) {
	s, id, _ := initWithWords(t)
	countCheckpoints := func() int {
		l, _ := s.Load()
		n := 0
		for _, r := range l.Report.Records {
			if r.IsCheckpoint() && r.Author == id {
				n++
			}
		}
		return n
	}
	if _, err := s.Add("weight", "81", AddOptions{}); err != nil {
		t.Fatal(err)
	}
	if n := countCheckpoints(); n != 0 {
		t.Errorf("a fresh log wrote %d checkpoints", n)
	}
	day := time.Now().Add(25 * time.Hour)
	s.Now = func() time.Time { return day }
	if _, err := s.Add("weight", "82", AddOptions{}); err != nil {
		t.Fatal(err)
	}
	if n := countCheckpoints(); n != 1 {
		t.Errorf("after a day: %d checkpoints", n)
	}
	var lines []string
	for i := 0; i < autoCheckpointCount; i++ {
		lines = append(lines, partial("hrv.rmssd", float64(i), nil))
	}
	appendLines(t, s, lines...)
	if n := countCheckpoints(); n != 2 {
		t.Errorf("after a batch: %d checkpoints", n)
	}
	if lines, code, _ := s.VerifyLines(); code != 0 {
		t.Errorf("exit %d %v", code, lines)
	}
}

// A checkpoint over more ids than one record holds splits, and chains.
func TestLargeCheckpointSplits(t *testing.T) {
	s, _, _ := initWithWords(t)
	l, _ := s.Load()
	k := s.mustLocal(t).Signer
	files := map[string][]byte{}
	for i := 0; i < maxCovers+5; i++ {
		id, b, err := SignRecord(claimBy(k, l.Me, l.Me, "x", map[string]any{"n": float64(i)}), k)
		if err != nil {
			t.Fatal(err)
		}
		files[PathOf(id)] = b
	}
	for p, b := range files {
		if err := writeFileAtomic(s.path(p), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := s.Checkpoint(nil)
	if err != nil || len(ids) != 2 {
		t.Fatalf("%d checkpoints, %v", len(ids), err)
	}
	if lines, code, _ := s.VerifyLines(); code != 0 {
		t.Errorf("exit %d %v", code, lines)
	}
	os.Remove(s.path(PathOf(ids[0])))
	if lines, code, _ := s.VerifyLines(); code != 1 {
		t.Errorf("first part removed: exit %d %v", code, lines)
	}
}

// A tombstone must not name a checkpoint.
func TestCheckpointCannotBePurged(t *testing.T) {
	s, _, _ := initWithWords(t)
	s.Add("weight", "81", AddOptions{})
	ids, err := s.Checkpoint(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Purge(ids[0], ""); err != ErrBadTarget {
		t.Errorf("purge of a checkpoint: %v", err)
	}
	k := s.mustLocal(t).Signer
	l, _ := s.Load()
	tid := plant(t, s, NewTombstone(l.Me, k.Public(), ids[0], "", "x@1", "2026-11-06T00:00:00Z"), k)
	if l, _ := s.Load(); l.Report.Errors[tid] != "bad target" {
		t.Errorf("hand-written tombstone of a checkpoint: %q", l.Report.Errors[tid])
	}
}

// gc must not leave erased plaintext in the object cache.
func TestGCLeavesNoPlaintextInCache(t *testing.T) {
	a, _, _ := initWithWords(t)
	y, _ := a.Add("lab.secret", "SENSITIVE", AddOptions{})
	d := t.TempDir()
	copyDir(t, a.Dir, d)
	b := Open(d)
	old := time.Now().Add(-time.Hour)
	os.Chtimes(b.path(PathOf(y)), old, old)
	b.Load()
	if _, err := a.Purge(y, ""); err != nil {
		t.Fatal(err)
	}
	b.Sync(a)
	os.Chtimes(hiddenPath(b.Dir, y), old, old)
	b.Load()
	b.Now = func() time.Time { return time.Now().Add(8 * 24 * time.Hour) }
	if r, _, err := b.GC(); err != nil || r != 1 {
		t.Fatalf("gc %d %v", r, err)
	}
	c, _ := os.ReadFile(b.path(fileObjCache))
	if strings.Contains(string(c), "SENSITIVE") {
		t.Error("erased plaintext in the cache after gc")
	}
}
