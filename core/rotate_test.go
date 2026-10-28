package core

import (
	"os"
	"strings"
	"testing"
)

func initWithWords(t *testing.T) (*Store, string, []string) {
	t.Helper()
	s := newStore(t)
	id, words, err := s.Init()
	if err != nil {
		t.Fatal(err)
	}
	return s, id, words
}

// Conformance test 17 (rewritten): rotate twice; three kids, same identity.
func TestRotateTwice(t *testing.T) {
	s, id, words := initWithWords(t)
	c1 := appendLines(t, s, partial("hrv.rmssd", 1, nil)).IDs[0]
	r1, err := s.Rotate(words, nil)
	if err != nil {
		t.Fatal(err)
	}
	c2 := appendLines(t, s, partial("hrv.rmssd", 2, nil)).IDs[0]
	r2, err := s.Rotate(words, nil)
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
	if _, err := s.Rotate(bad, nil); err != ErrSeedMismatch {
		t.Errorf("wrong words: %v", err)
	}
	other, _, otherWords := initWithWords(t)
	_ = other
	if _, err := s.Rotate(otherWords, nil); err != ErrSeedMismatch {
		t.Errorf("other seed: %v", err)
	}
	e := newStore(t)
	if _, err := e.Rotate(words, nil); err != ErrNotInitialized {
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
	res, err := a.Rotate(words, []string{d3})
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
	if _, err := a.Rotate(words, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Rotate(words, nil); err != nil {
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
	if _, err := a.Rotate(words, nil); err == nil || !strings.HasPrefix(err.Error(), "compromised: ") {
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
