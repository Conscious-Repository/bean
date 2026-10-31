package core

import (
	"bytes"
	"os"
	"sort"
	"strings"
	"testing"
)

// C29. A thief with the current
// signing key halts the identity with one record of an unknown version.
// The 24 words go past that halt: the succession retires the key, so the
// record is ungated everywhere (8.2) and the halt ends.

// thiefHalt writes a v=1 core record of id signed by sg and returns its id.
func thiefHalt(t *testing.T, s *Store, id string, sg Signer) string {
	t.Helper()
	return futureRecord(t, s, id, sg)
}

func mustTooNew(t *testing.T, s *Store, id string, want bool) *Loaded {
	t.Helper()
	l, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if contains(l.Report.TooNew, id) != want {
		t.Fatalf("too new %v, want %v for %s", l.Report.TooNew, want, id)
	}
	return l
}

func TestThiefHaltOwnerRotatesPast(t *testing.T) {
	s, id, words := initWithWords(t)
	claims := appendLines(t, s, partial("hr", 1, nil), partial("hr", 2, nil)).IDs
	x := thiefHalt(t, s, id, s.mustLocal(t).Signer)
	l := mustTooNew(t, s, id, true)
	if got := l.Report.HeadHalt[id]; len(got) != 1 || got[0] != x {
		t.Fatalf("head halt %v, want [%s]", got, x)
	}
	if _, err := s.Append(strings.NewReader(partial("hr", 3, nil) + "\n")); err == nil || !strings.HasPrefix(err.Error(), "too new") {
		t.Fatalf("append while halted: %v", err)
	}
	res, err := s.Rotate(words, nil, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Past) != 1 || res.Past[0] != x || !contains(res.Sealed, x) {
		t.Fatalf("past %v, sealed %v", res.Past, res.Sealed)
	}
	l = verifyOK(t, s)
	if !contains(l.Report.Warnings[x], "ungated: "+x) {
		t.Fatalf("halting record not ungated: %v", l.Report.Warnings[x])
	}
	for _, c := range claims {
		if !l.Report.Valid[c] {
			t.Fatalf("claim %s lost: %s", c, l.Report.Errors[c])
		}
	}
	appendLines(t, s, partial("hr", 3, nil))
	if _, _, err := s.GC(); err != nil {
		t.Fatal(err)
	}
}

// The thief keeps the retired key and tries again, also on the recovery path.
func TestThiefRetriesAfterRecovery(t *testing.T) {
	s, id, words := initWithWords(t)
	appendLines(t, s, partial("hr", 1, nil))
	stolen := s.mustLocal(t).Signer
	thiefHalt(t, s, id, stolen)
	if _, err := s.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	// A second halt by the stolen key: ungated.
	again := thiefHalt(t, s, id, stolen)
	// A v=1 succession signed by the stolen key, posing as a rotation.
	m := NewSuccession(id, stolen.Public(), id, NewEd25519Signer(bytes.Repeat([]byte{0x31}, 32)).Public(), Commitment(stolen.Public()), nil, "x@1", "2026-10-30T00:00:00Z")
	m["v"] = 1.0
	fid, fb, err := SignRecord(m, stolen)
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, s, PathOf(fid), fb)
	// A known-version succession signed by the stolen key: bad succession.
	m2 := NewSuccession(id, stolen.Public(), id, NewEd25519Signer(bytes.Repeat([]byte{0x32}, 32)).Public(), Commitment(stolen.Public()), nil, "x@1", "2026-10-30T00:00:00Z")
	bid, bb, err := SignRecord(m2, stolen)
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, s, PathOf(bid), bb)
	l := mustTooNew(t, s, id, false)
	for _, r := range []string{again, fid} {
		if !contains(l.Report.Warnings[r], "ungated: "+r) {
			t.Fatalf("%s not ungated: %v", r, l.Report.Warnings[r])
		}
	}
	if l.Report.Errors[bid] != "bad succession" {
		t.Fatalf("forged succession: %q", l.Report.Errors[bid])
	}
	if _, err := s.Append(strings.NewReader(partial("hr", 2, nil) + "\n")); err != nil {
		t.Fatal(err)
	}
}

// A halt that a recovery key took part in is never passed: only the seed
// holder can sign it, and it may be a real rotation by a newer bean.
func TestHaltByRecoveryKeyIsNotPassed(t *testing.T) {
	s, id, words := initWithWords(t)
	seed, err := WordsToSeed(words)
	if err != nil {
		t.Fatal(err)
	}
	thiefHalt(t, s, id, s.mustLocal(t).Signer)
	thiefHalt(t, s, id, RecoverySigner(seed, 0))
	l := mustTooNew(t, s, id, true)
	if len(l.Report.HeadHalt[id]) != 0 {
		t.Fatalf("head halt %v with a recovery-key record", l.Report.HeadHalt[id])
	}
	before := fileSet(t, s)
	if _, err := s.Rotate(words, nil, NewKey{}); err == nil || !strings.HasPrefix(err.Error(), "too new: "+id) {
		t.Fatalf("rotate: %v", err)
	}
	if fileSet(t, s) != before {
		t.Fatal("refused rotation wrote files")
	}
}

// Wrong words do not go past a halt.
func TestHaltWrongWords(t *testing.T) {
	s, id, _ := initWithWords(t)
	thiefHalt(t, s, id, s.mustLocal(t).Signer)
	_, _, words2 := initWithWords(t)
	if _, err := s.Rotate(words2, nil, NewKey{}); err != ErrSeedMismatch {
		t.Fatalf("rotate with other words: %v", err)
	}
}

// Restore on a new host from a backup that holds the halt.
func TestThiefHaltRestoreElsewhere(t *testing.T) {
	a, id, words := initWithWords(t)
	claims := appendLines(t, a, partial("hr", 1, nil)).IDs
	x := thiefHalt(t, a, id, a.mustLocal(t).Signer)
	b := newStore(t)
	if _, err := b.Sync(a); err != nil {
		t.Fatal(err)
	}
	res, got, err := b.Restore(words, nil, NewKey{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != id || len(res.Past) != 1 || res.Past[0] != x {
		t.Fatalf("restore %s past %v", got, res.Past)
	}
	l := verifyOK(t, b)
	if !l.Report.Valid[claims[0]] {
		t.Fatal("claim lost")
	}
	// The old copy syncs the succession and is no longer halted; its key
	// is retired.
	if _, err := a.Sync(b); err != nil {
		t.Fatal(err)
	}
	mustTooNew(t, a, id, false)
	if _, err := a.Append(strings.NewReader(partial("hr", 2, nil) + "\n")); err == nil || !strings.Contains(err.Error(), "retired key") {
		t.Fatalf("old copy append: %v", err)
	}
	la, lb := verifyLines(t, a), verifyLines(t, b)
	if la != lb {
		t.Fatalf("copies disagree:\n%s\n--\n%s", la, lb)
	}
}

// Race: the owner rotates on one copy while the thief halts another with
// the same (now retired) key. After sync, both agree: no halt.
func TestThiefHaltRacesRotation(t *testing.T) {
	a, id, words := initWithWords(t)
	appendLines(t, a, partial("hr", 1, nil))
	b := bareLocal(t, a)
	if _, err := b.Sync(a); err != nil {
		t.Fatal(err)
	}
	stolen := a.mustLocal(t).Signer
	if _, err := a.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	x := thiefHalt(t, b, id, stolen)
	mustTooNew(t, b, id, true)
	if _, err := b.Sync(a); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Sync(b); err != nil {
		t.Fatal(err)
	}
	for _, s := range []*Store{a, b} {
		l := mustTooNew(t, s, id, false)
		if !contains(l.Report.Warnings[x], "ungated: "+x) {
			t.Fatalf("not ungated: %v", l.Report.Warnings[x])
		}
	}
	if verifyLines(t, a) != verifyLines(t, b) {
		t.Fatal("copies disagree")
	}
}

// Two copies both go past the same halt from the same head: a fork, so the
// identity freezes (fork = freeze, 5.6). Nothing is erased.
func TestPastHaltTwiceForks(t *testing.T) {
	a, id, words := initWithWords(t)
	appendLines(t, a, partial("hr", 1, nil))
	thiefHalt(t, a, id, a.mustLocal(t).Signer)
	b := bareLocal(t, a)
	if _, err := b.Sync(a); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Sync(b); err != nil {
		t.Fatal(err)
	}
	// The fork point is the head of a frozen identity, and its key signed
	// the halting record, so verify still says too new; going past the
	// halt finds the fork and stops.
	mustTooNew(t, a, id, true)
	before := fileSet(t, a)
	if _, err := a.Rotate(words, nil, NewKey{}); err == nil || !strings.HasPrefix(err.Error(), "compromised") {
		t.Fatalf("rotate after fork: %v", err)
	}
	if fileSet(t, a) != before {
		t.Fatal("refused rotation wrote files")
	}
}

// A frozen identity that is also halted by its fork-point key: rotate says
// compromised, not "past halt".
func TestPastHaltOnFrozenIdentity(t *testing.T) {
	a, id, words := initWithWords(t)
	appendLines(t, a, partial("hr", 1, nil))
	b := bareLocal(t, a)
	if _, err := b.Sync(a); err != nil {
		t.Fatal(err)
	}
	forkKey := a.mustLocal(t).Signer
	if _, err := a.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Sync(b); err != nil {
		t.Fatal(err)
	}
	thiefHalt(t, a, id, forkKey) // the fork point's key is the head of a frozen identity
	before := fileSet(t, a)
	if _, err := a.Rotate(words, nil, NewKey{}); err == nil || !strings.HasPrefix(err.Error(), "compromised") {
		t.Fatalf("rotate: %v", err)
	}
	if fileSet(t, a) != before {
		t.Fatal("refused rotation wrote files")
	}
}

// The thief also deleted a claim with the current key. The owner goes past
// the halt and leaves the tombstone unsealed: the claim comes back. While
// the identity was halted nothing was hidden.
func TestPastHaltWithThiefTombstone(t *testing.T) {
	s, id, words := initWithWords(t)
	claim := appendLines(t, s, partial("hr", 1, nil)).IDs[0]
	stolen := s.mustLocal(t).Signer
	tid, tb, err := SignRecord(NewTombstone(id, stolen.Public(), claim, "", "x@1", "2026-10-30T00:00:00Z"), stolen)
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, s, PathOf(tid), tb)
	thiefHalt(t, s, id, stolen)
	mustTooNew(t, s, id, true)
	if _, err := os.Stat(s.path(PathOf(claim))); err != nil {
		t.Fatalf("claim hidden while halted: %v", err)
	}
	res, err := s.Rotate(words, []string{tid}, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.NotSealed, tid) {
		t.Fatalf("not sealed %v", res.NotSealed)
	}
	l := verifyOK0(t, s)
	if !l.Report.Valid[claim] || l.Report.Errors[tid] != "not sealed" {
		t.Fatalf("claim valid %v, tombstone %q", l.Report.Valid[claim], l.Report.Errors[tid])
	}
}

// An agent's records about the person are not evaluated while the person
// is halted and are valid again after the owner goes past the halt.
func TestPastHaltKeepsAgentRecords(t *testing.T) {
	s, id, words := initWithWords(t)
	if _, _, err := s.AgentInit("", "bot"); err != nil {
		t.Fatal(err)
	}
	ar, err := s.AppendAsAgent(strings.NewReader(partial("note", 1, nil) + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	thiefHalt(t, s, id, s.mustLocal(t).Signer)
	l := mustTooNew(t, s, id, true)
	if l.Report.Valid[ar.IDs[0]] {
		t.Fatal("agent record evaluated while halted")
	}
	if _, err := s.AppendAsAgent(strings.NewReader(partial("note", 2, nil) + "\n")); err == nil || !strings.HasPrefix(err.Error(), "too new: "+id) {
		t.Fatalf("agent write for a halted identity: %v", err)
	}
	if _, err := s.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	l = verifyOK(t, s)
	if !l.Report.Valid[ar.IDs[0]] {
		t.Fatalf("agent record: %s", l.Report.Errors[ar.IDs[0]])
	}
}

// Crash in the middle of a rotation past a halt, between seal parts: the
// next load undoes it, removes the orphan parts (the identity is still too
// new, so they are among the dropped records), and the halt is still there
// to go past again.
func TestPastHaltCrashBetweenParts(t *testing.T) {
	withChunk(t, 2)
	s, id, words := initWithWords(t)
	var lines []string
	for i := 0; i < 7; i++ {
		lines = append(lines, partial("hr", float64(i), nil))
	}
	appendLines(t, s, lines...)
	thiefHalt(t, s, id, s.mustLocal(t).Signer)
	snap := newStore(t)
	n := 0
	hookAfterPart = func() {
		if n++; n == 2 {
			os.CopyFS(snap.Dir, os.DirFS(s.Dir))
		}
	}
	defer func() { hookAfterPart = nil }()
	if _, err := s.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	hookAfterPart = nil
	l := mustTooNew(t, snap, id, true)
	for rid, r := range l.Report.Dropped {
		if r.Type == "bean.seal" {
			t.Fatalf("orphan part %s left", rid)
		}
	}
	for _, f := range []string{fileSignKeyNext, fileRotationPending} {
		if _, err := os.Stat(snap.path(f)); !os.IsNotExist(err) {
			t.Fatalf("%s left", f)
		}
	}
	if _, err := snap.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	verifyOK(t, snap)
	verifyOK(t, s)
}

// Crash after the succession, before the key moved: the next load finishes
// the rotation; the identity is no longer halted.
func TestPastHaltCrashAfterSuccession(t *testing.T) {
	s, id, words := initWithWords(t)
	appendLines(t, s, partial("hr", 1, nil))
	thiefHalt(t, s, id, s.mustLocal(t).Signer)
	snap := newStore(t)
	hookAfterSuccession = func() { os.CopyFS(snap.Dir, os.DirFS(s.Dir)) }
	defer func() { hookAfterSuccession = nil }()
	res, err := s.Rotate(words, nil, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	hookAfterSuccession = nil
	l := mustTooNew(t, snap, id, false)
	if l.Local.Signer.Public() != res.NewKey {
		t.Fatal("rotation not finished")
	}
	verifyOK(t, snap)
}

func verifyLines(t *testing.T, s *Store) string {
	t.Helper()
	lines, _, err := s.VerifyLines()
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// Strangers' halted identities in the backup do not match the words and
// do not stop the restore.
func TestRestorePastHaltAmongStrangers(t *testing.T) {
	a, id, words := initWithWords(t)
	appendLines(t, a, partial("hr", 1, nil))
	thiefHalt(t, a, id, a.mustLocal(t).Signer)
	for i := 0; i < 3; i++ {
		o, oid, _ := initWithWords(t)
		thiefHalt(t, o, oid, o.mustLocal(t).Signer)
		if _, err := a.Sync(o); err != nil {
			t.Fatal(err)
		}
	}
	b := newStore(t)
	if _, err := b.Sync(a); err != nil {
		t.Fatal(err)
	}
	if _, got, err := b.Restore(words, nil, NewKey{}, ""); err != nil || got != id {
		t.Fatalf("restore %s: %v", got, err)
	}
	mustTooNew(t, b, id, false)
}
