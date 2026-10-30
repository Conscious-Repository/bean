package core

import (
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
