package core

import (
	"strings"
	"testing"
)

// C26 with corrections, pairings and two rotations.

// A correction excluded with -no-seal dies; its target is shown again and
// stays valid.
func TestNoSealSupersedeShowsTarget(t *testing.T) {
	a, _, words := initWithWords(t)
	tgt := appendLines(t, a, partial("hr", 1, nil)).IDs[0]
	fix := strings.Replace(partial("hr", 2, nil), `"type":"hr"`, `"refs":[{"id":"`+tgt+`","rel":"supersedes"}],"type":"hr"`, 1)
	d := appendLines(t, a, fix).IDs[0]
	res, err := a.Rotate(words, Seals{All: true, NoSeal: []string{d}}, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.Sealed, tgt) || !contains(res.NotSealed, d) {
		t.Fatalf("sealed %v not sealed %v", res.Sealed, res.NotSealed)
	}
	l, _ := a.Load()
	if !l.Current(tgt) || l.Report.Errors[d] != "not sealed" {
		t.Errorf("target current=%v, correction %q", l.Current(tgt), l.Report.Errors[d])
	}
}

// An agent pairing made with the first key keeps its label after two
// rotations, and the chain of seals holds for records of every key.
func TestTwoRotationsKeepEverything(t *testing.T) {
	a, id, words := initWithWords(t)
	r1 := appendLines(t, a, partial("hr", 1, nil)).IDs[0]
	if _, _, err := a.AgentInit("", "assistant"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	r2 := appendLines(t, a, partial("hr", 2, nil)).IDs[0]
	if _, err := a.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	r3 := appendLines(t, a, partial("hr", 3, nil)).IDs[0]
	l := verifyOK(t, a)
	for _, r := range []string{r1, r2, r3} {
		if !l.Current(r) {
			t.Errorf("%s: %q", r[:16], l.Report.Errors[r])
		}
	}
	if len(l.AgentLabels()) != 1 || len(l.Report.Chains[id]) != 3 {
		t.Errorf("labels %v chain %d", l.AgentLabels(), len(l.Report.Chains[id]))
	}
	// Restore on a fresh folder from the synced files keeps all of it.
	n := newStore(t)
	n.Sync(a)
	if _, _, err := n.Restore(words, Seals{All: true}, NewKey{}, ""); err != nil {
		t.Fatal(err)
	}
	ln := verifyOK(t, n)
	for _, r := range []string{r1, r2, r3} {
		if !ln.Current(r) {
			t.Errorf("after restore %s: %q", r[:16], ln.Report.Errors[r])
		}
	}
}

// Export and import-export keep records of retired keys valid: the
// successions that seal them travel with them.
func TestExportKeepsSealedRecords(t *testing.T) {
	a, _, words := initWithWords(t)
	r1 := appendLines(t, a, partial("hr", 1, nil)).IDs[0]
	if _, err := a.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	x := t.TempDir() + "/x"
	if _, _, err := a.Export(x); err != nil {
		t.Fatal(err)
	}
	b := cloneLocal(t, a)
	if _, _, err := b.ImportExport(x); err != nil {
		t.Fatal(err)
	}
	if l := verifyOK(t, b); !l.Current(r1) {
		t.Errorf("r1 after import-export: %q", l.Report.Errors[r1])
	}
}
