package core

import (
	"strings"
	"testing"
)

// Nothing of a too-new identity is evaluated, also not step 3: no held
// or bad succession lines for it (DECISIONS 159, SPEC 8.2 item 2).
func TestTooNewIdentityHasNoLines(t *testing.T) {
	l := newTestLog()
	i := l.newIdentity(1)
	hid, _ := l.rotate(i, "sha256:"+strings.Repeat("ab", 32), 0, nil) // parent missing: held
	bid, _ := l.rotate(i, i.id, 5, nil)                               // wrong recovery key: bad
	halt := map[string]any{"v": 99.0, "nonce": l.nonce(), "type": "bean.succession", "subject": i.id, "author": i.id, "kid": i.signer.Public()}
	l.put(halt, i.signer)
	rp := l.verify()
	if len(rp.TooNew) != 1 {
		t.Fatalf("too new %v", rp.TooNew)
	}
	for _, id := range []string{hid, bid} {
		if rp.Errors[id] != "" || len(rp.Warnings[id]) > 0 || rp.Held[id] {
			t.Errorf("%s judged: %q %v %v", id[:14], rp.Errors[id], rp.Warnings[id], rp.Held[id])
		}
	}
}

// Steps 6 to 8 in order: a tombstone of a retired trunk key that no
// succession sealed is "not sealed" (step 7) also when the identity is
// frozen (step 8). DECISIONS 159, conformance case 14.
func TestNotSealedBeforeFrozen(t *testing.T) {
	l := newTestLog()
	i := l.newIdentity(1)
	target := l.claim(i, "weight", 80, "")
	d := l.tombstone(i, target)
	s1, _ := l.rotate(i, i.id, 0, []string{target})
	l.rotate(i, s1, 1, nil)
	l.rotate(i, s1, 1, nil) // fork below s1
	rp := l.verify()
	if len(rp.Compromised) != 1 {
		t.Fatalf("not frozen: %v", rp.Compromised)
	}
	wantError(t, rp, d, "not sealed")
}

// A supersede record with missing targets has one held line, and none
// when another target makes it invalid. DECISIONS 159, conformance cases 13 and 13b.
func TestHeldOncePerRecord(t *testing.T) {
	l := newTestLog()
	i, j := l.newIdentity(1), l.newIdentity(2)
	m1, m2 := "sha256:"+strings.Repeat("a1", 32), "sha256:"+strings.Repeat("a2", 32)
	sup := func(targets ...string) string {
		var refs []any
		for _, x := range targets {
			refs = append(refs, map[string]any{"rel": "supersedes", "id": x})
		}
		c := map[string]any{"v": 0.0, "nonce": l.nonce(), "type": "weight", "subject": i.id, "author": i.id, "kid": i.signer.Public(),
			"value": map[string]any{"n": 1.0, "unit": "kg"}, "refs": refs,
			"times": map[string]any{"observed": nil, "imported": "2026-10-28T12:00:00Z"}, "source": map[string]any{"system": "self", "adapter": "x@1", "ref": nil}}
		return l.put(c, i.signer)
	}
	two := sup(m1, m2)
	other := l.claim(j, "weight", 70, "")
	mixed := sup(m1, other)
	rp := l.verify()
	if len(rp.Warnings[two]) != 1 || !rp.Held[two] {
		t.Errorf("two missing: %v", rp.Warnings[two])
	}
	wantError(t, rp, mixed, "not author")
	if len(rp.Warnings[mixed]) != 0 || rp.Held[mixed] {
		t.Errorf("invalid record held: %v", rp.Warnings[mixed])
	}
}
