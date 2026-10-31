package core

import (
	"bytes"
	"testing"
)

// TestN4WholePath pins DECISIONS 172: a succession may not name a current or
// earlier signing key, or a recovery key already committed on its path, as its
// next recovery key.
func TestN4WholePath(t *testing.T) {
	succ := func(l *testLog, i *testIdentity, signer Signer, parent, key, rec string) string {
		s := NewSuccession(i.id, signer.Public(), parent, key, rec, nil, "test@0", "2026-10-27T00:00:04Z")
		s["nonce"] = l.nonce()
		return l.put(s, signer)
	}
	other := func(b byte) string { return NewEd25519Signer(bytes.Repeat([]byte{b}, 32)).Public() }
	r := func(i *testIdentity, g int) Signer { return RecoverySigner(i.seed, g) }

	// The genesis signing key as the next recovery key.
	l := newTestLog()
	i := l.newIdentity(0x31)
	s1 := succ(l, i, r(i, 0), i.id, other(0xd1), Commitment(i.signer.Public()))
	wantError(t, l.verify(), s1, "key reuse")

	// Recovery key 0 committed again after S1 revealed it.
	l = newTestLog()
	i = l.newIdentity(0x32)
	s1 = succ(l, i, r(i, 0), i.id, other(0xd2), Commitment(r(i, 1).Public()))
	s2 := succ(l, i, r(i, 1), s1, other(0xd3), Commitment(r(i, 0).Public()))
	wantError(t, l.verify(), s2, "recovery not advanced")

	// The current signing key (of S1) as the next recovery key.
	l = newTestLog()
	i = l.newIdentity(0x33)
	s1 = succ(l, i, r(i, 0), i.id, r(i, 2).Public(), Commitment(r(i, 1).Public()))
	s2 = succ(l, i, r(i, 1), s1, other(0xd4), Commitment(r(i, 2).Public()))
	rp := l.verify()
	if !rp.Valid[s1] {
		t.Fatalf("s1 should stay valid")
	}
	wantError(t, rp, s2, "key reuse")
}
