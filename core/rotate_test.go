package core

import (
	"os"
	"strings"
	"testing"
)

func initWithWords(t *testing.T) (*Store, string, []string) {
	t.Helper()
	s := newStore(t)
	id, words, err := s.Init(NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	return s, id, words
}

// Conformance test 17 (rewritten): rotate twice; three kids, same identity.
func TestRotateTwice(t *testing.T) {
	s, id, words := initWithWords(t)
	c1 := appendLines(t, s, partial("hrv.rmssd", 1, nil)).IDs[0]
	r1, err := s.Rotate(words, nil, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	c2 := appendLines(t, s, partial("hrv.rmssd", 2, nil)).IDs[0]
	r2, err := s.Rotate(words, nil, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	c3 := appendLines(t, s, partial("hrv.rmssd", 3, nil)).IDs[0]
	l := verifyOK(t, s)
	k := map[string]bool{l.Report.Records[c1].Kid: true, l.Report.Records[c2].Kid: true, l.Report.Records[c3].Kid: true}
	if len(k) != 3 {
		t.Error("kids not distinct")
	}
	if l.Local.Identity != id || len(l.Report.Chains[id]) != 3 || r1.NewKey == r2.NewKey {
		t.Error("chain or identity wrong")
	}
	// Pasted init output parses.
	text := ""
	for i, w := range words {
		text += strings.ToUpper(w[:1]) + w[1:]
		if i%3 == 0 {
			text += "\n" + string(rune('1'+i%9)) + ". "
		} else {
			text += " "
		}
	}
	if got := ParseWords(text); strings.Join(got, " ") != strings.Join(words, " ") {
		t.Errorf("ParseWords: %v", got)
	}
}

func TestRotateErrors(t *testing.T) {
	s, _, words := initWithWords(t)
	bad := append([]string{}, words...)
	bad[0] = "zoo"
	if _, err := s.Rotate(bad, nil, NewKey{}); err != ErrSeedMismatch {
		t.Errorf("wrong words: %v", err)
	}
	other, _, otherWords := initWithWords(t)
	_ = other
	if _, err := s.Rotate(otherWords, nil, NewKey{}); err != ErrSeedMismatch {
		t.Errorf("other seed: %v", err)
	}
	e := newStore(t)
	if _, err := e.Rotate(words, nil, NewKey{}); err != ErrNotInitialized {
		t.Errorf("uninitialized: %v", err)
	}
}

// Conformance tests 9, 10, 13 and 20: seals.
func TestRotateSeals(t *testing.T) {
	a, _, words := initWithWords(t)
	ids := appendLines(t, a, partial("hrv.rmssd", 1, nil), partial("hrv.rmssd", 2, nil), partial("hrv.rmssd", 3, nil)).IDs
	b := cloneLocal(t, a)
	// D1: tombstone of ids[0] on A before rotation (test 10, 20: sealed).
	d1, _ := a.Purge(ids[0], "")
	// D2: tombstone of ids[1] written by a copy of K1 on replica B, not in A
	// at rotation time (test 9, 13: not sealed).
	d2, _ := b.Purge(ids[1], "")
	// D3: in A but excluded with no-seal.
	sup := map[string]any{"type": "hrv.rmssd", "value": map[string]any{"n": 9.0}, "times": map[string]any{"observed": nil},
		"source": map[string]any{"system": "test", "adapter": "t@1", "ref": nil},
		"refs":   []any{map[string]any{"rel": "supersedes", "id": ids[2]}}}
	d3 := appendLines(t, a, string(Canonical(sup))).IDs[0]
	res, err := a.Rotate(words, []string{d3}, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sealed) != 1 || res.Sealed[0] != d1 || len(res.NotSealed) != 1 || res.NotSealed[0] != d3 {
		t.Fatalf("sealed %v not sealed %v", res.Sealed, res.NotSealed)
	}
	// Two passes: the first delivers the succession that invalidates D2, the
	// second copies D2's target back to B (7.4 step 5).
	for i := 0; i < 2; i++ {
		if _, err := a.Sync(b); err != nil {
			t.Fatal(err)
		}
		if _, err := b.Sync(a); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []*Store{a, b} {
		lines, code, _ := s.VerifyLines()
		out := strings.Join(lines, "\n")
		if code != 1 || !strings.Contains(out, d2+": not sealed") || !strings.Contains(out, d3+": not sealed") {
			t.Errorf("exit %d\n%s", code, out)
		}
		l, _ := s.Load()
		if l.Visible(ids[0]) || !l.Visible(ids[1]) || len(l.Report.Superseded[ids[2]]) != 0 {
			t.Error("seal effects wrong")
		}
	}
	// B's signing key is now the outgoing key: its new claims are still
	// valid (6.5) but it cannot purge any more.
	if r := appendLines(t, b, partial("hrv.rmssd", 4, nil)); r.Added != 1 {
		t.Error("old key claim")
	}
	lb, _ := b.Load()
	if !lb.Visible(ids[1]) {
		t.Error("target lost on B")
	}
}

// Conformance test 21: double rotation off one parent freezes.
func TestDoubleRotationFreeze(t *testing.T) {
	a, id, words := initWithWords(t)
	appendLines(t, a, partial("hrv.rmssd", 1, nil))
	b := cloneLocal(t, a)
	if _, err := a.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	a.Sync(b)
	b.Sync(a)
	for _, s := range []*Store{a, b} {
		lines, code, _ := s.VerifyLines()
		if code != 4 || !contains(lines, "compromised: "+id) {
			t.Errorf("exit %d\n%s", code, strings.Join(lines, "\n"))
		}
	}
	if _, err := a.Rotate(words, nil, NewKey{}); err == nil || !strings.HasPrefix(err.Error(), "compromised: ") {
		t.Errorf("rotate on frozen: %v", err)
	}
	// Trunk claim is still valid; new claims by either branch key are frozen.
	appendLines(t, a, partial("hrv.rmssd", 2, nil))
	la, _ := a.Load()
	n := 0
	for rid, e := range la.Report.Errors {
		if e == "frozen" && la.Report.Records[rid] != nil && !la.Report.Records[rid].IsIdentity() {
			n++
		}
	}
	if n != 1 {
		t.Errorf("frozen claims %d", n)
	}
	if _, err := os.Stat(a.path(fileSignKey)); err != nil {
		t.Error(err)
	}
}

// A lost laptop: only the 24 words and a synced copy of objects/ and
// blobs/ remain. Restore rebuilds local/, keeps the identity id, decrypts
// the blobs, seals the old key's destructive records, and moves to a new
// key so the lost one can no longer delete or correct anything.
func TestRestoreLostLaptop(t *testing.T) {
	a, id, words := initWithWords(t)
	blob, err := a.PutBlob([]byte("raw export"))
	if err != nil {
		t.Fatal(err)
	}
	withBlob := strings.Replace(partial("hrv.rmssd", 1, nil), `"source":`, `"blobs":[{"hash":"`+blob+`","media_type":"text/plain","role":"raw-export"}],"source":`, 1)
	ids := appendLines(t, a, withBlob, partial("hrv.rmssd", 2, nil), partial("hrv.rmssd", 3, nil)).IDs
	if _, err := a.Rotate(words, nil, NewKey{}); err != nil { // one earlier rotation: restore must use generation 1
		t.Fatal(err)
	}
	purge, err := a.Purge(ids[2], "")
	if err != nil {
		t.Fatal(err)
	}
	lostKey := a.mustLocal(t).Signer.Public()

	// The backup: a new folder with only the synced files, no local/.
	b := newStore(t)
	if _, err := b.Sync(a); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ReadLocal(); err != ErrNotInitialized {
		t.Fatalf("backup has local/: %v", err)
	}

	res, got, err := b.Restore(words, nil, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if got != id {
		t.Fatalf("restored identity %s, want %s", got, id)
	}
	if len(res.Sealed) != 1 || res.Sealed[0] != purge || res.NewKey == lostKey {
		t.Errorf("restore result %+v", res)
	}
	l := verifyOK(t, b)
	if len(l.Report.Chains[id]) != 3 || l.Local.Identity != id {
		t.Errorf("chain length %d", len(l.Report.Chains[id]))
	}
	if pt, err := b.GetBlob(blob); err != nil || string(pt) != "raw export" {
		t.Errorf("blob after restore: %q %v", pt, err)
	}
	if l.Visible(ids[2]) {
		t.Error("sealed purge undone")
	}
	// The new key writes; dedupe still works because the dedupe key comes
	// from the same identity secret.
	if r := appendLines(t, b, partial("hrv.rmssd", 4, nil)); r.Added != 1 {
		t.Error("append after restore")
	}
	if _, err := b.Purge(ids[1], ""); err != nil {
		t.Errorf("purge with the new key: %v", err)
	}

	// The lost laptop's key can still add claims (6.5) but its deletions
	// after the restore are not sealed.
	thief, err := a.Purge(ids[0], "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Sync(a); err != nil {
		t.Fatal(err)
	}
	lines, _, _ := b.VerifyLines()
	if !contains(lines, "error: "+thief+": not sealed") {
		t.Errorf("thief tombstone:\n%s", strings.Join(lines, "\n"))
	}
	if lb, _ := b.Load(); !lb.Visible(ids[0]) {
		t.Error("thief's purge took effect")
	}
}

func TestRestoreErrors(t *testing.T) {
	a, _, words := initWithWords(t)
	appendLines(t, a, partial("hrv.rmssd", 1, nil))
	if _, _, err := a.Restore(words, nil, NewKey{}); err != ErrAlreadyInitialized {
		t.Errorf("restore over local/: %v", err)
	}
	b := newStore(t)
	if _, err := b.Sync(a); err != nil {
		t.Fatal(err)
	}
	_, _, otherWords := initWithWords(t)
	if _, _, err := b.Restore(otherWords, nil, NewKey{}); err == nil || err.Error() != "input missing: genesis for these words" {
		t.Errorf("other words: %v", err)
	}
	bad := append([]string{}, words...)
	bad[0] = "notaword"
	if _, _, err := b.Restore(bad, nil, NewKey{}); err != ErrSeedMismatch {
		t.Errorf("bad word: %v", err)
	}
	if _, err := b.ReadLocal(); err != ErrNotInitialized {
		t.Errorf("failed restore left local/ initialized: %v", err)
	}
	if _, _, err := newStore(t).Restore(words, nil, NewKey{}); err == nil {
		t.Error("restore into an empty folder")
	}
}

func (s *Store) mustLocal(t *testing.T) *Local {
	t.Helper()
	l, err := s.ReadLocal()
	if err != nil {
		t.Fatal(err)
	}
	return l
}
