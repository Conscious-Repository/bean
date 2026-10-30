package core

import (
	"bytes"
	"strings"
	"testing"
)

// futureRecord writes a well-signed v=1 core record of identity id, signed by
// signer, as a newer bean that this verifier cannot read would.
func futureRecord(t *testing.T, s *Store, id string, signer Signer) string {
	t.Helper()
	m := NewTombstone(id, signer.Public(), id, "x", "test@1", "2026-10-30T00:00:00Z")
	m["v"] = 1.0
	rid, b, err := SignRecord(m, signer)
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, s, PathOf(rid), b)
	return rid
}

// Audit 2: rotate and restore on a too-new identity said "input missing".
// They must say "too new" so the person knows to update bean.
func TestRotateRestoreTooNewError(t *testing.T) {
	a, id, words := initWithWords(t)
	appendLines(t, a, partial("hr", 1, nil))
	futureRecord(t, a, id, a.mustLocal(t).Signer)
	if _, err := a.Rotate(words, nil, NewKey{}); err == nil || !strings.HasPrefix(err.Error(), "too new: "+id) {
		t.Fatalf("rotate: %v, want too new", err)
	}
	b := newStore(t)
	if _, err := b.Sync(a); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Restore(words, nil, NewKey{}); err == nil || !strings.HasPrefix(err.Error(), "too new: "+id) {
		t.Fatalf("restore: %v, want too new", err)
	}
	c := newStore(t)
	if _, err := c.Sync(a); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.RestoreIdentity(words, nil, NewKey{}, id); err == nil || !strings.HasPrefix(err.Error(), "too new: "+id) {
		t.Fatalf("restore -identity: %v, want too new", err)
	}
}

// K1 (prototype): a rotated-out key or a stranger cannot halt the identity.
// Before K1 one v=1 core record of a stolen retired key stopped verify,
// append, gc and rotate on every replica, for good.
func TestTooNewGateRetiredKeyAndStranger(t *testing.T) {
	s, id, words := initWithWords(t)
	appendLines(t, s, partial("hr", 1, nil))
	old := s.mustLocal(t).Signer
	if _, err := s.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	retired := futureRecord(t, s, id, old)
	stranger := futureRecord(t, s, id, NewEd25519Signer(bytes.Repeat([]byte{0x77}, 32)))
	l, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Report.TooNew) != 0 {
		t.Fatalf("too new %v, want none", l.Report.TooNew)
	}
	for _, r := range []string{retired, stranger} {
		if !contains(l.Report.Warnings[r], "ungated: "+r) {
			t.Fatalf("no ungated warning for %s: %v", r, l.Report.Warnings[r])
		}
	}
	if _, err := s.Append(strings.NewReader(partial("hr", 2, nil) + "\n")); err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := s.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if _, _, err := s.GC(); err != nil {
		t.Fatalf("gc: %v", err)
	}
}

// K1 (prototype): a newer bean rotates with the next recovery key. Before
// K1 that key was not "on the tree", so this verifier skipped the record in
// silence, and rotate here wrote a second succession from the same
// recovery key: a fork that froze the identity.
func TestTooNewGateNextRecoveryKey(t *testing.T) {
	s, id, words := initWithWords(t)
	appendLines(t, s, partial("hr", 1, nil))
	seed, err := WordsToSeed(words)
	if err != nil {
		t.Fatal(err)
	}
	futureRecord(t, s, id, RecoverySigner(seed, 0))
	l, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !contains(l.Report.TooNew, id) {
		t.Fatalf("too new %v, want %s", l.Report.TooNew, id)
	}
	if _, err := s.Rotate(words, nil, NewKey{}); err == nil || !strings.HasPrefix(err.Error(), "too new: "+id) {
		t.Fatalf("rotate: %v, want too new", err)
	}
	if _, err := s.Append(strings.NewReader(partial("hr", 2, nil) + "\n")); err == nil || !strings.HasPrefix(err.Error(), "too new: "+id) {
		t.Fatalf("append: %v, want too new", err)
	}
}

// An ungated record of a too-new identity gets no warning: nothing of a
// too-new identity is evaluated (SPEC.md 8.2 item 2).
func TestUngatedSilentOnTooNewIdentity(t *testing.T) {
	s, id, _ := initWithWords(t)
	futureRecord(t, s, id, s.mustLocal(t).Signer)
	stranger := futureRecord(t, s, id, NewEd25519Signer(bytes.Repeat([]byte{0x78}, 32)))
	l, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !contains(l.Report.TooNew, id) || len(l.Report.Warnings[stranger]) != 0 {
		t.Fatalf("too new %v, warnings %v", l.Report.TooNew, l.Report.Warnings[stranger])
	}
}
