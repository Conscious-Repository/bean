package core

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Attacks from the re-audit of ff4ef77. Each test passes when the attack
// fails. DECISIONS 185 to 195.

const reLab = `{"type":"lab.potassium","value":VALUE,"times":{"observed":null},"source":{"system":"labcorp","adapter":"x@1","ref_input":["bean/v0/ref","id","labcorp","SPEC1/Potassium",0]}}`

func labLine(value string) string { return strings.Replace(reLab, "VALUE", value, 1) + "\n" }

func reKey(t *testing.T) Signer {
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

func rePlant(t *testing.T, s *Store, o map[string]any, k Signer) string {
	t.Helper()
	id, b, err := SignRecord(o, k)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(s.path(PathOf(id)), b, 0o600); err != nil {
		t.Fatal(err)
	}
	return id
}

func reClaim(k Signer, author, subject, typ string, v map[string]any) map[string]any {
	return map[string]any{"v": 0.0, "nonce": NewNonce(), "type": typ, "subject": subject, "author": author, "kid": k.Public(), "value": v,
		"times":  map[string]any{"observed": nil, "imported": "2026-11-06T00:00:00Z"},
		"source": map[string]any{"system": "labcorp", "adapter": "x@1", "ref": nil}}
}

func reCopy(t *testing.T, from, to string) {
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

func reList(t *testing.T, s *Store, typ string) []*Record {
	t.Helper()
	recs, _, err := s.List(Filter{Type: typ})
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

// Blocker 1: a stranger's record that names absent ids must not get them
// sealed, or a thief can deliver pre-signed records after the rotation.
func TestReauditAbsentIDsNotSealedForStranger(t *testing.T) {
	s, id, words := initWithWords(t)
	real, _ := s.Add("lab.hba1c", "9.1", AddOptions{})
	thief := s.mustLocal(t).Signer
	forged := reClaim(thief, id, id, "lab.hiv", map[string]any{"s": "reactive"})
	fid, fb, err := SignRecord(forged, thief)
	if err != nil {
		t.Fatal(err)
	}
	tomb := NewTombstone(id, thief.Public(), real, "", "x@1", "2026-11-06T00:00:00Z")
	tid, tb, err := SignRecord(tomb, thief)
	if err != nil {
		t.Fatal(err)
	}
	k := reKey(t)
	z := reClaim(k, k.Public(), id, "note", map[string]any{"s": "x"})
	z["refs"] = []any{map[string]any{"rel": "supersedes", "id": fid}, map[string]any{"rel": "supersedes", "id": tid}}
	rePlant(t, s, z, k)
	res, err := s.Rotate(words, Seals{All: true}, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if contains(res.Sealed, fid) || contains(res.Sealed, tid) {
		t.Fatalf("a stranger's record got absent ids sealed: forged %v tombstone %v", contains(res.Sealed, fid), contains(res.Sealed, tid))
	}
	writeFileAtomic(s.path(PathOf(fid)), fb, 0o600)
	writeFileAtomic(s.path(PathOf(tid)), tb, 0o600)
	l, _ := s.Load()
	if l.Current(fid) || !l.Current(real) {
		t.Errorf("forged current %v, real current %v", l.Current(fid), l.Current(real))
	}
	// A kept record's own absent target is still sealed (DECISIONS 122).
	s2, _, w2 := initWithWords(t)
	x, _ := s2.Add("weight", "81", AddOptions{})
	if _, err := s2.Purge(x, ""); err != nil {
		t.Fatal(err)
	}
	res2, err := s2.Rotate(w2, Seals{All: true}, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res2.Sealed, x) {
		t.Error("a kept tombstone's erased target should stay sealed")
	}
}

// A hold on a record of an older key seals nothing: only the outgoing key's
// records need this succession's seal.
func TestReauditHoldOfOlderKeyNotSealed(t *testing.T) {
	a, _, words := initWithWords(t)
	x, _ := a.Add("weight", "81", AddOptions{})
	if _, err := a.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	d := t.TempDir()
	reCopy(t, a.Dir, d)
	b := Open(d)
	if _, err := b.Purge(x, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Sync(b); err != nil {
		t.Fatal(err)
	}
	if l, _ := a.Load(); len(l.Holds) != 1 {
		t.Fatalf("holds: %v", l.Holds)
	}
	res, err := a.Rotate(words, Seals{All: true}, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if contains(res.Sealed, x) {
		t.Errorf("sealed a held record of an older key: %v", res.Sealed)
	}
}

// Blocker 2: a rotation keeps nothing without a decision, and says what it
// would keep. A bad -no-seal is an error, never a silent seal.
func TestReauditRotationNeedsDecision(t *testing.T) {
	s, id, words := initWithWords(t)
	x, _ := s.Add("lab.hba1c", "9.1", AddOptions{})
	d := t.TempDir()
	reCopy(t, s.Dir, d)
	thief := Open(d)
	tomb, err := thief.Purge(x, "")
	if err != nil {
		t.Fatal(err)
	}
	k := thief.mustLocal(t).Signer
	forged := rePlant(t, thief, reClaim(k, id, id, "lab.hiv", map[string]any{"s": "reactive"}), k)
	if _, err := s.Sync(thief); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Load()
	_, err = s.Rotate(words, Seals{}, NewKey{})
	ue, ok := err.(*UndecidedError)
	if !ok {
		t.Fatalf("rotate with no decision: %v", err)
	}
	have := map[string]bool{}
	for _, r := range ue.Records {
		have[r.ID] = true
	}
	if !have[tomb] || !have[forged] || !have[x] {
		t.Errorf("the list misses a record: tombstone %v forged %v target %v", have[tomb], have[forged], have[x])
	}
	after, _ := s.Load()
	if len(after.Report.Chains[id]) != len(before.Report.Chains[id]) {
		t.Error("a rotation with no decision wrote a succession")
	}
	typo := "1"
	if tomb[13] == '1' {
		typo = "2"
	}
	for _, bad := range []string{tomb[7:13] + typo, "zzzz", "abcdef"} {
		if _, err := s.Rotate(words, Seals{All: true, NoSeal: []string{bad}}, NewKey{}); err == nil {
			t.Errorf("-no-seal %q was ignored", bad)
		}
	}
	if l, _ := s.Load(); len(l.Report.Chains[id]) != len(before.Report.Chains[id]) {
		t.Fatal("a bad -no-seal wrote a succession")
	}
	res, err := s.Rotate(words, Seals{All: true, NoSeal: []string{tomb, forged}}, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if contains(res.Sealed, tomb) || contains(res.Sealed, forged) {
		t.Errorf("sealed what the person refused: %v", res.Sealed)
	}
	if got := reList(t, s, "lab.hba1c"); len(got) != 1 {
		t.Errorf("the thief's deletion was not undone: %d", len(got))
	}
	if got := reList(t, s, "lab.hiv"); len(got) != 0 {
		t.Errorf("the thief's claim still shows: %d", len(got))
	}
	// A log with nothing to decide on needs no flag, as on a new machine.
	s3, _, w3 := initWithWords(t)
	if _, err := s3.Rotate(w3, Seals{}, NewKey{}); err != nil {
		t.Errorf("rotate with nothing to decide: %v", err)
	}
	// Restore asks in the same way, and writes nothing first.
	s4, _, w4 := initWithWords(t)
	s4.Add("weight", "81", AddOptions{})
	b := newStore(t)
	if _, err := b.Sync(s4); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Restore(w4, Seals{}, NewKey{}, ""); err == nil {
		t.Fatal("restore with no decision")
	} else if _, ok := err.(*UndecidedError); !ok {
		t.Fatalf("restore: %v", err)
	}
	if _, err := os.Stat(b.path(fileSecret)); err == nil {
		t.Error("restore with no decision wrote the identity secret")
	}
}

// Blocker 3: a record that no key vouches for does not reach a reader as the
// person's. verify still counts it, so no validity rule changes.
func TestReauditForgedSuccessionNotShown(t *testing.T) {
	s, id, _ := initWithWords(t)
	mine, _ := s.Add("weight", "81", AddOptions{})
	k := reKey(t)
	f := NewSuccession(id, k.Public(), mine, k.Public(), "sha256:"+strings.Repeat("c", 64), nil, "x@1", "2026-11-06T00:00:00Z")
	f["context"] = map[string]any{"note": "stop all medication"}
	fid := rePlant(t, s, f, k)
	l, _ := s.Load()
	if l.Visible(fid) {
		t.Error("a forged succession is visible")
	}
	for _, r := range l.Replies(mine) {
		if r.ID == fid {
			t.Error("a forged succession is a reply to the person's record")
		}
	}
	if recs, _, _ := s.List(Filter{All: true, Type: "bean.succession"}); len(recs) != 0 {
		t.Errorf("ls -all shows it: %d", len(recs))
	}
	if st := l.Status(fid); !strings.HasPrefix(st, "held") {
		t.Errorf("status %q", st)
	}
	if !l.Report.Valid[fid] {
		t.Error("the verifier's result must not change")
	}
	// The same for a succession whose author has no genesis here, and a seal.
	o := NewSuccession("sha256:"+strings.Repeat("d", 64), k.Public(), mine, k.Public(), "sha256:"+strings.Repeat("c", 64), nil, "x@1", "2026-11-06T00:00:00Z")
	u := rePlant(t, s, o, k)
	if l, _ := s.Load(); l.Visible(u) {
		t.Error("a succession of an unknown identity is visible")
	}
	// A real rotation stays visible with -all.
	s2, _, w2 := initWithWords(t)
	r, err := s2.Rotate(w2, Seals{All: true}, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := s2.Load(); !l.Visible(r.Succession) {
		t.Error("a real succession is not visible")
	}
}

// Blocker 4: a new value is never dropped in silence and never written by
// itself. A newer report, an older one, a null and a colliding id look the
// same to bean, so the person decides.
func TestReauditChangedReadingIsReported(t *testing.T) {
	s, _, _ := initWithWords(t)
	if r, err := s.Append(strings.NewReader(labLine(`{"s":"Pending"}`))); err != nil || r.Added != 1 {
		t.Fatalf("first: %+v %v", r, err)
	}
	r, err := s.Append(strings.NewReader(labLine(`{"n":5.9,"unit":"mmol/L"}`)))
	if err != nil || r.Added != 0 || len(r.Changed) != 1 || r.Changed[0].Type != "lab.potassium" {
		t.Fatalf("a changed value: %+v %v", r, err)
	}
	if got := reList(t, s, "lab.potassium"); len(got) != 1 || got[0].Value["s"] != "Pending" {
		t.Fatalf("append replaced a value by itself: %+v", got)
	}
	// With the person's word it corrects, and says so.
	r, err = s.AppendCorrections(strings.NewReader(labLine(`{"n":5.9,"unit":"mmol/L"}`)))
	if err != nil || r.Added != 1 || r.Corrected != 1 {
		t.Fatalf("-corrections: %+v %v", r, err)
	}
	if got := reList(t, s, "lab.potassium"); len(got) != 1 || got[0].Value["n"] != 5.9 {
		t.Fatalf("ls: %+v", got)
	}
	if lines, code, _ := s.VerifyLines(); code != 0 {
		t.Errorf("verify: %d %v", code, lines)
	}
	// A value the ref has had adds nothing, in either mode, so an old report
	// cannot bring its value back.
	for _, v := range []string{`{"n":5.9,"unit":"mmol/L"}`, `{"s":"Pending"}`} {
		for _, f := range []func(*strings.Reader) (AppendResult, error){
			func(x *strings.Reader) (AppendResult, error) { return s.Append(x) },
			func(x *strings.Reader) (AppendResult, error) { return s.AppendCorrections(x) },
		} {
			r, err := f(strings.NewReader(labLine(v)))
			if err != nil || r.Added != 0 || r.Skipped != 1 || len(r.Changed) != 0 {
				t.Errorf("re-import of %s: %+v %v", v, r, err)
			}
		}
	}
	// A null never replaces a result, even with -corrections.
	r, _ = s.AppendCorrections(strings.NewReader(labLine(`{"n":null}`)))
	if r.Added != 0 || len(r.Changed) != 1 {
		t.Errorf("a null: %+v", r)
	}
	// Nor does a different type that collides on one ref.
	line2 := strings.Replace(labLine(`{"n":7}`), "lab.potassium", "lab.sodium", 1)
	r, _ = s.AppendCorrections(strings.NewReader(line2))
	if r.Added != 0 || len(r.Changed) != 1 {
		t.Errorf("another type: %+v", r)
	}
	if got := reList(t, s, "lab.potassium"); len(got) != 1 || got[0].Value["n"] != 5.9 {
		t.Errorf("after the refusals: %+v", got)
	}
	// In one input, the first line decides.
	s2, _, _ := initWithWords(t)
	s2.Append(strings.NewReader(labLine(`{"n":1}`)))
	r, _ = s2.AppendCorrections(strings.NewReader(labLine(`{"n":2}`) + labLine(`{"n":3}`)))
	if r.Added != 1 || r.Skipped != 1 {
		t.Errorf("one input: %+v", r)
	}
	// A correction made by hand stays in effect when the file comes again, and
	// a parser fix by ref still finds its target.
	s3, _, _ := initWithWords(t)
	a, _ := s3.Append(strings.NewReader(labLine(`{"n":59}`)))
	if _, err := s3.Add("lab.potassium", "5.9", AddOptions{Corrects: a.IDs[0]}); err != nil {
		t.Fatal(err)
	}
	if r, _ := s3.Append(strings.NewReader(labLine(`{"n":59}`))); r.Added != 0 || len(r.Changed) != 0 {
		t.Errorf("re-import after a hand correction: %+v", r)
	}
	if got := reList(t, s3, "lab.potassium"); len(got) != 1 || got[0].Value["n"] != 5.9 {
		t.Errorf("the hand correction lost: %+v", got)
	}
	fix := strings.Replace(labLine(`{"n":6}`), `"ref_input"`, `"supersedes_ref":["bean/v0/ref","id","labcorp","SPEC1/Potassium",0],"ref_input"`, 1)
	if r, err := s3.Append(strings.NewReader(fix)); err != nil || r.Added != 1 {
		t.Errorf("a parser fix after a hand correction: %+v %v", r, err)
	}
	// A correction covers duplicates, so a copy cannot hide it.
	s4, _, _ := initWithWords(t)
	s4.Append(strings.NewReader(labLine(`{"n":1}`)))
	o := newStore(t)
	reCopy(t, s4.Dir, o.Dir)
	l, _ := s4.Load()
	var rec *Record
	for _, x := range l.Report.Records {
		if x.Type == "lab.potassium" {
			rec = x
		}
	}
	k := s4.mustLocal(t).Signer
	dup := reClaim(k, l.ID, l.ID, "lab.potassium", map[string]any{"n": 1.0})
	dup["source"].(map[string]any)["ref"] = rec.Source.Ref
	rePlant(t, s4, dup, k)
	if r, err := s4.AppendCorrections(strings.NewReader(labLine(`{"n":2}`))); err != nil || r.Corrected != 1 {
		t.Fatalf("correct duplicates: %+v %v", r, err)
	}
	if got := reList(t, s4, "lab.potassium"); len(got) != 1 || got[0].Value["n"] != 2.0 {
		t.Errorf("a duplicate hid the correction: %+v", got)
	}
	// A purge still blocks, also with a new value.
	s5, _, _ := initWithWords(t)
	x, _ := s5.Append(strings.NewReader(labLine(`{"n":1}`)))
	s5.Purge(x.IDs[0], "")
	if r, err := s5.AppendCorrections(strings.NewReader(labLine(`{"n":2}`))); err != nil || r.Added != 0 {
		t.Errorf("after a purge: %+v %v", r, err)
	}
	// An undone deletion unblocks the reading.
	s6, _, w6 := initWithWords(t)
	y, _ := s6.Append(strings.NewReader(labLine(`{"n":1}`)))
	d := t.TempDir()
	reCopy(t, s6.Dir, d)
	thief := Open(d)
	tomb, err := thief.Purge(y.IDs[0], "")
	if err != nil {
		t.Fatal(err)
	}
	s6.Sync(thief)
	if _, err := s6.Rotate(w6, Seals{All: true, NoSeal: []string{tomb}}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s6.Load(); err != nil {
		t.Fatal(err)
	}
	if refs, _ := s6.readPurgedRefs(); len(refs) != 0 {
		t.Errorf("a purged ref stayed after the deletion was undone: %v", refs)
	}
}

// Blocker 5: a halted log says so. An empty list looks like lost data.
func TestReauditHaltIsLoud(t *testing.T) {
	s, id, _ := initWithWords(t)
	s.Add("weight", "81", AddOptions{})
	k := s.mustLocal(t).Signer
	rePlant(t, s, map[string]any{"v": 1.0, "nonce": NewNonce(), "type": "bean.anything", "author": id, "kid": k.Public(), "subject": id}, k)
	if _, _, err := s.List(Filter{}); err == nil || !strings.HasPrefix(err.Error(), "too new: ") {
		t.Errorf("list: %v", err)
	}
	// The exit still works: every file goes out, and the command says why it failed.
	dir := filepath.Join(t.TempDir(), "x")
	n, _, err := s.Export(dir)
	if err == nil || !strings.HasPrefix(err.Error(), "too new: ") || n != 3 {
		t.Errorf("export: %d records, %v", n, err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "records"))
	if len(entries) != 3 {
		t.Errorf("exported files: %d", len(entries))
	}
	// A record of a newer format about nothing in particular also goes out.
	s2, id2, _ := initWithWords(t)
	k2 := s2.mustLocal(t).Signer
	u := rePlant(t, s2, map[string]any{"v": 1.0, "nonce": NewNonce(), "type": "hrv.rmssd", "author": id2, "kid": k2.Public(), "subject": id2}, k2)
	dir2 := filepath.Join(t.TempDir(), "x")
	if _, _, err := s2.Export(dir2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir2, "records", u[7:]+".json")); err != nil {
		t.Errorf("export dropped a record of a newer format: %v", err)
	}
}

// Blocker 6: one file must not stop the exit.
func TestReauditExportPastMissingBlob(t *testing.T) {
	s, id, _ := initWithWords(t)
	s.Add("weight", "81", AddOptions{})
	k := reKey(t)
	o := reClaim(k, k.Public(), id, "x", map[string]any{"s": "x"})
	o["blobs"] = []any{map[string]any{"hash": "sha256:" + strings.Repeat("e", 64), "role": "x", "media_type": "x"}}
	rePlant(t, s, o, k)
	dir := filepath.Join(t.TempDir(), "x")
	n, _, err := s.Export(dir)
	if err != nil || n != 3 {
		t.Errorf("a stranger's absent blob: %d records, %v", n, err)
	}
	m, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if !strings.Contains(string(m), "missing_blobs") {
		t.Errorf("manifest: %s", m)
	}
	// One of the person's own blobs: all else goes out, then it fails.
	s2, _, _ := initWithWords(t)
	s2.Add("weight", "81", AddOptions{})
	s2.Add("food", "toast", AddOptions{Photo: []byte("\x89PNG\r\n\x1a\nxx"), PhotoType: "image/png"})
	s2.Add("food", "eggs", AddOptions{Photo: []byte("\x89PNG\r\n\x1a\nyy"), PhotoType: "image/png"})
	fs, _, _ := listShards(s2.Dir, dirBlobs)
	os.Remove(s2.path(fs[0].rel))
	dir2 := filepath.Join(t.TempDir(), "x")
	n, b, err := s2.Export(dir2)
	if err == nil || !strings.HasPrefix(err.Error(), "blob missing") || n != 4 || b != 1 {
		t.Errorf("own blob gone: %d records %d blobs, %v", n, b, err)
	}
	// A blob path that is not a file must not hang the export.
	s3, _, _ := initWithWords(t)
	s3.Add("food", "toast", AddOptions{Photo: []byte("\x89PNG\r\n\x1a\nxx"), PhotoType: "image/png"})
	fs3, _, _ := listShards(s3.Dir, dirBlobs)
	p := s3.path(fs3[0].rel)
	os.Remove(p)
	if err := mkfifo(p); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, _, err := s3.Export(filepath.Join(t.TempDir(), "x"))
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("export past a pipe where a blob should be")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a pipe hung the export")
	}
}

// Blocker 7: a freeze from a stale restore: what the person kept stays in
// effect for readers, a thief's new claim does not show, and a deleted record
// does not come back. verify's lines do not change.
func TestReauditFreezeReads(t *testing.T) {
	s, id, words := initWithWords(t)
	bad, _ := s.Add("lab.hba1c", "91", AddOptions{Unit: "%"})
	fixed, _ := s.Add("lab.hba1c", "9.1", AddOptions{Unit: "%", Corrects: bad})
	secret, _ := s.Add("lab.secret", "HIDEME", AddOptions{})
	old := s.mustLocal(t).Signer
	d := t.TempDir()
	reCopy(t, s.path("objects"), filepath.Join(d, "objects"))
	stale := Open(d)
	if _, err := s.Purge(secret, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := stale.Restore(words, Seals{All: true}, NewKey{}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sync(stale); err != nil {
		t.Fatal(err)
	}
	if _, code, _ := s.VerifyLines(); code != 4 {
		t.Fatalf("verify exit %d", code)
	}
	l, _ := s.Load()
	if l.Report.Valid[fixed] || l.Report.Errors[fixed] != "frozen" {
		t.Errorf("the verifier's result must not change: %q", l.Report.Errors[fixed])
	}
	got := reList(t, s, "lab.hba1c")
	if len(got) != 1 || got[0].ID != fixed {
		t.Errorf("ls after the freeze should show the correction both sides kept: %d rows", len(got))
	}
	f := rePlant(t, s, reClaim(old, id, id, "lab.hiv", map[string]any{"s": "reactive"}), old)
	if got := reList(t, s, "lab.hiv"); len(got) != 0 {
		t.Errorf("a thief's claim after the fork shows: %s", f)
	}
	// The stale copy hears of the deletion only with the fork.
	if _, err := stale.Sync(s); err != nil {
		t.Fatal(err)
	}
	if got := reList(t, stale, "lab.secret"); len(got) != 0 {
		t.Error("a record the person deleted came back on the stale copy")
	}
	// One side not sealing a correction leaves the old value, and verify
	// and migrate both say what happened.
	a, _, wa := initWithWords(t)
	b1, _ := a.Add("x", "1", AddOptions{})
	f1, _ := a.Add("x", "2", AddOptions{Corrects: b1})
	d2 := t.TempDir()
	reCopy(t, a.path("objects"), filepath.Join(d2, "objects"))
	st2 := Open(d2)
	a.Rotate(wa, Seals{All: true}, NewKey{})
	if _, _, err := st2.Restore(wa, Seals{All: true, NoSeal: []string{f1}}, NewKey{}, ""); err != nil {
		t.Fatal(err)
	}
	a.Sync(st2)
	if got := reList(t, a, "x"); len(got) != 1 || got[0].ID != b1 {
		t.Errorf("a correction one side refused: %d rows", len(got))
	}
}

// One signature with the key at the fork, which a thief may hold, must not hide
// the log. -all shows what is in doubt, and an export carries it.
func TestReauditThiefTombstoneAtFork(t *testing.T) {
	s, id, words := initWithWords(t)
	real, _ := s.Add("lab.hba1c", "9.1", AddOptions{})
	old := s.mustLocal(t).Signer
	d := t.TempDir()
	reCopy(t, s.path("objects"), filepath.Join(d, "objects"))
	stale := Open(d)
	s.Rotate(words, Seals{All: true}, NewKey{})
	stale.Restore(words, Seals{All: true}, NewKey{}, "")
	s.Sync(stale)
	tomb := rePlant(t, s, NewTombstone(id, old.Public(), real, "", "x@1", "2026-11-06T00:00:00Z"), old)
	if got := reList(t, s, "lab.hba1c"); len(got) != 1 {
		t.Errorf("a tombstone no branch sealed hid a record: %s", tomb)
	}
	// A correction that one branch kept: both are visible with -all, and the
	// old value stays current.
	s2, _, w2 := initWithWords(t)
	bad, _ := s2.Add("x", "91", AddOptions{})
	d2 := t.TempDir()
	reCopy(t, s2.path("objects"), filepath.Join(d2, "objects"))
	st2 := Open(d2)
	fix, _ := s2.Add("x", "9.1", AddOptions{Corrects: bad})
	s2.Rotate(w2, Seals{All: true}, NewKey{})
	st2.Restore(w2, Seals{All: true}, NewKey{}, "")
	s2.Sync(st2)
	recs, l, err := s2.List(Filter{All: true, Type: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || !l.Report.InDoubt(fix) {
		t.Errorf("-all: %d rows, fix in doubt %v", len(recs), l.Report.InDoubt(fix))
	}
	if st := l.Status(bad); !strings.HasPrefix(st, "in doubt") {
		t.Errorf("status of the disputed value: %q", st)
	}
	dst := Open(t.TempDir())
	res, err := s2.Migrate(dst, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Disputed != 1 || res.LeftOut < 1 {
		t.Errorf("migrate must say what is disputed: %+v", res)
	}
	got := reList(t, dst, "x")
	if len(got) != 1 || got[0].Context["migrated_disputed_by"] == nil {
		t.Errorf("the moved value does not name the dispute: %+v", got)
	}
	// An export carries a record that a deletion in doubt names.
	s3, id3, w3 := initWithWords(t)
	sec, _ := s3.Add("lab.secret", "1", AddOptions{})
	d3 := t.TempDir()
	reCopy(t, s3.path("objects"), filepath.Join(d3, "objects"))
	st3 := Open(d3)
	s3.Purge(sec, "")
	s3.Rotate(w3, Seals{All: true}, NewKey{})
	st3.Restore(w3, Seals{All: true}, NewKey{}, "")
	st3.Sync(s3)
	dir := filepath.Join(t.TempDir(), "x")
	st3.Export(dir)
	if _, err := os.Stat(filepath.Join(dir, "records", sec[7:]+".json")); err != nil {
		t.Errorf("export left out a record in doubt: %v", err)
	}
	if got := reList(t, st3, "lab.secret"); len(got) != 0 {
		t.Error("a deletion the person kept came undone on the stale copy")
	}
	_ = id3
}

// A person's report of a frozen log's Kept status reads right.
func TestReauditKeptStatus(t *testing.T) {
	s, _, words := initWithWords(t)
	bad, _ := s.Add("x", "91", AddOptions{})
	fix, _ := s.Add("x", "9.1", AddOptions{Corrects: bad})
	d := t.TempDir()
	reCopy(t, s.path("objects"), filepath.Join(d, "objects"))
	stale := Open(d)
	s.Rotate(words, Seals{All: true}, NewKey{})
	stale.Restore(words, Seals{All: true}, NewKey{}, "")
	s.Sync(stale)
	l, _ := s.Load()
	if st := l.Status(bad); !strings.HasPrefix(st, "superseded") {
		t.Errorf("the corrected value says %q", st)
	}
	if st := l.Status(fix); !strings.HasPrefix(st, "frozen, but every branch") {
		t.Errorf("the correction says %q", st)
	}
}

// Blocker 7: migrate is the way forward from a freeze.
func TestReauditMigrate(t *testing.T) {
	s, id, words := initWithWords(t)
	bad, _ := s.Add("lab.hba1c", "91", AddOptions{Unit: "%"})
	fixed, _ := s.Add("lab.hba1c", "9.1", AddOptions{Unit: "%", Corrects: bad})
	blob, _ := s.PutBlob([]byte("raw"))
	withBlob := `{"type":"lab.report","value":{"blob":"` + blob + `"},"blobs":[{"hash":"` + blob + `","role":"raw-export","media_type":"text/plain"}],"times":{"observed":null},"source":{"system":"x","adapter":"x@1","ref":null}}`
	rep := appendLines(t, s, withBlob).IDs[0]
	note, _ := s.Add("note", "see report", AddOptions{About: rep})
	if _, _, err := s.AgentInit("", "assistant"); err != nil {
		t.Fatal(err)
	}
	ag, _ := s.Add("food", "toast", AddOptions{Agent: true})
	old := s.mustLocal(t).Signer
	d := t.TempDir()
	reCopy(t, s.path("objects"), filepath.Join(d, "objects"))
	reCopy(t, s.path("blobs"), filepath.Join(d, "blobs"))
	stale := Open(d)
	s.Rotate(words, Seals{All: true}, NewKey{})
	stale.Restore(words, Seals{All: true}, NewKey{}, "")
	s.Sync(stale)
	thief := rePlant(t, s, reClaim(old, id, id, "lab.hiv", map[string]any{"s": "reactive"}), old)
	k := reKey(t)
	rePlant(t, s, reClaim(k, k.Public(), id, "weight", map[string]any{"n": 300.0}), k)

	dst := Open(t.TempDir())
	res, err := s.Migrate(dst, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Identity == id || len(res.Words) != 24 || res.Blobs != 1 || res.MissingBlobs != 0 {
		t.Errorf("migrate: %+v", res)
	}
	if res.LeftOut < 3 { // the agent's, the thief's and the stranger's
		t.Errorf("migrate must count what it left out: %+v", res)
	}
	if lines, code, _ := dst.VerifyLines(); code != 0 {
		t.Errorf("migrated log: exit %d %v", code, lines)
	}
	got := reList(t, dst, "lab.hba1c")
	if len(got) != 1 || got[0].Value["n"] != 9.1 || got[0].Context["migrated_from"] != fixed {
		t.Errorf("labs: %+v", got)
	}
	if got := reList(t, dst, "lab.hiv"); len(got) != 0 {
		t.Errorf("migrate re-signed a thief's claim: %s", thief)
	}
	if got := reList(t, dst, "weight"); len(got) != 0 {
		t.Error("migrate re-signed a stranger's claim")
	}
	reports := reList(t, dst, "lab.report")
	if len(reports) != 1 {
		t.Fatalf("reports: %d", len(reports))
	}
	if pt, err := dst.GetBlob(reports[0].Blobs[0].Hash); err != nil || string(pt) != "raw" || reports[0].Value["blob"] != reports[0].Blobs[0].Hash {
		t.Errorf("blob: %q %v", pt, err)
	}
	notes := reList(t, dst, "note")
	if len(notes) != 1 || notes[0].Context["migrated_from"] != note || notes[0].Targets("about")[0] != reports[0].ID {
		t.Errorf("note: %+v", notes)
	}
	if got := reList(t, dst, "food"); len(got) != 0 {
		t.Errorf("migrate signed an agent's record as the person: %s", ag)
	}
	dl, _ := dst.Load()
	for _, r := range dl.Report.Records {
		if r.Type == "agent.pair" {
			t.Error("a pairing moved")
		}
	}
	if _, err := dst.Add("weight", "81", AddOptions{}); err != nil {
		t.Errorf("the new identity must write: %v", err)
	}
	if _, err := s.Migrate(dst, NewKey{}); err == nil {
		t.Error("migrate into a used folder")
	}
}

// Migrate refuses a log that can go on: a thief's current-key claims would
// move with no decision. A record too close to the size limit is counted.
func TestReauditMigrateEdges(t *testing.T) {
	s, id, words := initWithWords(t)
	s.Add("weight", "81", AddOptions{})
	if _, err := s.Migrate(Open(t.TempDir()), NewKey{}); err == nil || !strings.HasPrefix(err.Error(), "not frozen") {
		t.Fatalf("migrate of a log that is not frozen: %v", err)
	}
	k := s.mustLocal(t).Signer
	_, probe, err := SignRecord(reClaim(k, id, id, "note", map[string]any{"s": ""}), k)
	if err != nil {
		t.Fatal(err)
	}
	rePlant(t, s, reClaim(k, id, id, "note", map[string]any{"s": strings.Repeat("x", MaxRecordSize-len(probe)-10)}), k)
	d := t.TempDir()
	reCopy(t, s.path("objects"), filepath.Join(d, "objects"))
	stale := Open(d)
	s.Rotate(words, Seals{All: true}, NewKey{})
	stale.Restore(words, Seals{All: true}, NewKey{}, "")
	s.Sync(stale)
	dst := Open(t.TempDir())
	res, err := s.Migrate(dst, NewKey{})
	if err != nil {
		t.Fatalf("migrate: %+v %v", res, err)
	}
	if res.TooLarge != 1 || res.Records != 1 {
		t.Errorf("too large: %+v", res)
	}
}

// Two machines rotate from one point, keep logging, then sync. What they logged
// after the fork is visible with -all, and migrate moves this machine's own.
func TestReauditPostForkClaims(t *testing.T) {
	a, _, words := initWithWords(t)
	a.Add("weight", "81", AddOptions{})
	d := t.TempDir()
	reCopy(t, a.path("objects"), filepath.Join(d, "objects"))
	b := Open(d)
	a.Rotate(words, Seals{All: true}, NewKey{})
	if _, _, err := b.Restore(words, Seals{All: true}, NewKey{}, ""); err != nil {
		t.Fatal(err)
	}
	mine, _ := a.Add("lab.a", "1", AddOptions{})
	theirs, _ := b.Add("lab.b", "2", AddOptions{})
	if _, err := a.Sync(b); err != nil {
		t.Fatal(err)
	}
	recs, l, err := a.List(Filter{All: true, Type: "lab.*"})
	if err != nil || len(recs) != 2 || !l.Report.InDoubt(mine) {
		t.Errorf("-all: %d rows, %v", len(recs), err)
	}
	if st := l.Status(mine); !strings.HasPrefix(st, "in doubt: a key below") {
		t.Errorf("status %q", st)
	}
	dst := Open(t.TempDir())
	res, err := a.Migrate(dst, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if got := reList(t, dst, "lab.a"); len(got) != 0 {
		t.Errorf("a claim in doubt moved with no decision: %+v", got)
	}
	if res.LeftOut != 2 {
		t.Errorf("left out: %+v", res)
	}
	// With the person's word, it moves. A wrong id is an error.
	if _, err := a.Migrate(Open(t.TempDir()), NewKey{}, "abcdef"); err == nil {
		t.Error("-also with a wrong id")
	}
	dst2 := Open(t.TempDir())
	if _, err := a.Migrate(dst2, NewKey{}, mine); err != nil {
		t.Fatal(err)
	}
	if got := reList(t, dst2, "lab.a"); len(got) != 1 || got[0].Context["migrated_from"] != mine {
		t.Errorf("-also: %+v", got)
	}
	if got := reList(t, dst2, "lab.b"); len(got) != 0 {
		t.Errorf("another record moved: %s", theirs)
	}
	recs2, _, err := a.MigrateList()
	if err != nil || len(recs2) != 2 {
		t.Errorf("list: %d %v", len(recs2), err)
	}
}

// A claim the person names cannot take a real reading's place through a ref.
func TestReauditMigrateRefCollision(t *testing.T) {
	s, id, words := initWithWords(t)
	r, _ := s.Append(strings.NewReader(labLine(`{"n":4.1}`)))
	l, _ := s.Load()
	ref := l.Report.Records[r.IDs[0]].Source.Ref
	d := t.TempDir()
	reCopy(t, s.path("objects"), filepath.Join(d, "objects"))
	stale := Open(d)
	s.Rotate(words, Seals{All: true}, NewKey{})
	stale.Restore(words, Seals{All: true}, NewKey{}, "")
	s.Sync(stale)
	k := s.mustLocal(t).Signer
	var forged string
	for {
		o := reClaim(k, id, id, "lab.potassium", map[string]any{"n": 7.9})
		o["source"].(map[string]any)["ref"] = ref
		if RecordID(o) < r.IDs[0] {
			forged = rePlant(t, s, o, k)
			break
		}
	}
	dst := Open(t.TempDir())
	res, err := s.Migrate(dst, NewKey{}, forged)
	if err != nil {
		t.Fatal(err)
	}
	if got := reList(t, dst, "lab.potassium"); len(got) != 1 || got[0].Value["n"] != 4.1 {
		t.Errorf("the real reading lost: %+v", got)
	}
	if res.LeftOut < 1 {
		t.Errorf("collision not counted: %+v", res)
	}
}

// A reading must not vanish behind a record that a freeze put in doubt.
func TestReauditNoVanishingReadings(t *testing.T) {
	// A: a thief's copy of a ref with the key at the fork and a lower id.
	s, id, words := initWithWords(t)
	r, _ := s.Append(strings.NewReader(labLine(`{"n":4.1}`)))
	l, _ := s.Load()
	ref := l.Report.Records[r.IDs[0]].Source.Ref
	old := s.mustLocal(t).Signer
	d := t.TempDir()
	reCopy(t, s.path("objects"), filepath.Join(d, "objects"))
	stale := Open(d)
	s.Rotate(words, Seals{All: true}, NewKey{})
	stale.Restore(words, Seals{All: true}, NewKey{}, "")
	s.Sync(stale)
	for {
		o := reClaim(old, id, id, "lab.potassium", map[string]any{"n": 7.9})
		o["source"].(map[string]any)["ref"] = ref
		if RecordID(o) < r.IDs[0] {
			rePlant(t, s, o, old)
			break
		}
	}
	got := reList(t, s, "lab.potassium")
	if len(got) != 1 || got[0].Value["n"] != 4.1 {
		t.Errorf("A: %d rows %+v", len(got), got)
	}
	dst := Open(t.TempDir())
	res, err := s.Migrate(dst, NewKey{})
	if err != nil || res.Records != 1 {
		t.Errorf("A: migrate %+v %v", res, err)
	}
	// C: a correction both branches kept, and a deletion of it that one kept.
	a, _, wa := initWithWords(t)
	base, _ := a.Add("x", "5.5", AddOptions{})
	fix, _ := a.Add("x", "6.6", AddOptions{Corrects: base})
	d2 := t.TempDir()
	reCopy(t, a.path("objects"), filepath.Join(d2, "objects"))
	st := Open(d2)
	if _, err := a.Purge(fix, ""); err != nil {
		t.Fatal(err)
	}
	a.Rotate(wa, Seals{All: true}, NewKey{})
	st.Restore(wa, Seals{All: true}, NewKey{}, "")
	st.Sync(a)
	got = reList(t, st, "x")
	if len(got) != 1 || got[0].ID != base {
		t.Errorf("C: %d rows", len(got))
	}
	sl, _ := st.Load()
	if s := sl.Status(fix); !strings.HasPrefix(s, "hidden") {
		t.Errorf("C: status %q", s)
	}
}

// One record of a newer format must not close the way forward from a freeze.
func TestReauditHaltOnFrozenLog(t *testing.T) {
	s, id, words := initWithWords(t)
	s.Add("weight", "81", AddOptions{})
	old := s.mustLocal(t).Signer
	d := t.TempDir()
	reCopy(t, s.path("objects"), filepath.Join(d, "objects"))
	stale := Open(d)
	s.Rotate(words, Seals{All: true}, NewKey{})
	stale.Restore(words, Seals{All: true}, NewKey{}, "")
	s.Sync(stale)
	rePlant(t, s, map[string]any{"v": 1.0, "nonce": NewNonce(), "type": "bean.anything", "author": id, "kid": old.Public(), "subject": id}, old)
	_, _, err := s.List(Filter{})
	if err == nil || !strings.Contains(err.Error(), "bean migrate") {
		t.Errorf("list: %v", err)
	}
	dst := Open(t.TempDir())
	res, err := s.Migrate(dst, NewKey{})
	if err != nil || res.Records != 1 {
		t.Errorf("migrate: %+v %v", res, err)
	}
}

// A thief's tombstone with a key below the fork must not hide the log.
func TestReauditBranchKeyTombstone(t *testing.T) {
	s, id, words := initWithWords(t)
	real, _ := s.Add("lab.a", "1", AddOptions{})
	d := t.TempDir()
	reCopy(t, s.path("objects"), filepath.Join(d, "objects"))
	stale := Open(d)
	s.Rotate(words, Seals{All: true}, NewKey{})
	stale.Restore(words, Seals{All: true}, NewKey{}, "")
	branch := stale.mustLocal(t).Signer
	s.Sync(stale)
	rePlant(t, s, NewTombstone(id, branch.Public(), real, "", "x@1", "2026-11-06T00:00:00Z"), branch)
	if got := reList(t, s, "lab.a"); len(got) != 1 {
		t.Errorf("a branch key's tombstone hid a record: %d", len(got))
	}
}

// Frozen log, laptop lost: the words still open the files.
func TestReauditRestoreOfFrozenLog(t *testing.T) {
	s, _, words := initWithWords(t)
	blob, _ := s.PutBlob([]byte("raw"))
	withBlob := `{"type":"lab.report","value":{"blob":"` + blob + `"},"blobs":[{"hash":"` + blob + `","role":"raw-export","media_type":"text/plain"}],"times":{"observed":null},"source":{"system":"x","adapter":"x@1","ref":null}}`
	appendLines(t, s, withBlob)
	d := t.TempDir()
	reCopy(t, s.path("objects"), filepath.Join(d, "objects"))
	reCopy(t, s.path("blobs"), filepath.Join(d, "blobs"))
	stale := Open(d)
	s.Rotate(words, Seals{All: true}, NewKey{})
	stale.Restore(words, Seals{All: true}, NewKey{}, "")
	s.Sync(stale)
	lost := newStore(t)
	reCopy(t, s.path("objects"), lost.path("objects"))
	reCopy(t, s.path("blobs"), lost.path("blobs"))
	if _, _, err := lost.Restore(words, Seals{}, NewKey{}, ""); err == nil || !strings.HasPrefix(err.Error(), "compromised") || !strings.Contains(err.Error(), "bean migrate") {
		t.Fatalf("restore: %v", err)
	}
	if n, b, err := lost.Export(filepath.Join(t.TempDir(), "x")); err != nil || n == 0 || b != 1 {
		t.Errorf("export: %d %d %v", n, b, err)
	}
	dst := Open(t.TempDir())
	res, err := lost.Migrate(dst, NewKey{})
	if err != nil || res.Blobs != 1 {
		t.Errorf("migrate: %+v %v", res, err)
	}
}

// Migrate keeps a reading's date, ignores input keys in a stored source, and
// counts the person's records about others.
func TestReauditMigrateSmall(t *testing.T) {
	s, id, words := initWithWords(t)
	k := s.mustLocal(t).Signer
	dated := reClaim(k, id, id, "weight", map[string]any{"n": 81.0})
	dated["times"] = map[string]any{"observed": nil, "imported": "2019-03-01T00:00:00Z"}
	rePlant(t, s, dated, k)
	bad := reClaim(k, id, id, "note", map[string]any{"s": "x"})
	bad["source"].(map[string]any)["supersedes_ref"] = []any{"bean/v0/ref", "id", "x", "y", 0.0}
	rePlant(t, s, bad, k)
	rePlant(t, s, reClaim(k, id, "sha256:"+strings.Repeat("a", 64), "note", map[string]any{"s": "about someone"}), k)
	d := t.TempDir()
	reCopy(t, s.path("objects"), filepath.Join(d, "objects"))
	stale := Open(d)
	s.Rotate(words, Seals{All: true}, NewKey{})
	stale.Restore(words, Seals{All: true}, NewKey{}, "")
	s.Sync(stale)
	dst := Open(t.TempDir())
	res, err := s.Migrate(dst, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Records != 2 || res.LeftOut != 1 {
		t.Errorf("migrate: %+v", res)
	}
	got := reList(t, dst, "weight")
	if len(got) != 1 || When(got[0]).Year() != 2019 {
		t.Errorf("the date moved: %+v", got)
	}
}
