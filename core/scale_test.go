package core

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// Verify is linear in the number of identities and in the
// length of a succession chain. Signatures are marked proven so the test
// times the rules, not the arithmetic.
func TestVerifyScales(t *testing.T) {
	const n = 4000
	seed := bytes.Repeat([]byte{9}, 32)
	key := func(i int) Signer {
		b := make([]byte, 32)
		binary.BigEndian.PutUint64(b, uint64(i)+1)
		return NewEd25519Signer(b)
	}
	add := func(files map[string][]byte, obj map[string]any, s Signer) string {
		id, b, err := SignRecord(obj, s)
		if err != nil {
			t.Fatal(err)
		}
		files[PathOf(id)] = b
		return id
	}
	timed := func(name string, files map[string][]byte, check func(*Report)) {
		proven := map[string]bool{}
		for p := range files {
			proven[p] = true
		}
		start := time.Now()
		rp := verifyFiles(files, proven)
		d := time.Since(start)
		t.Logf("%s: %d files in %v", name, len(files), d)
		if d > time.Second {
			t.Errorf("%s: %d files took %v", name, len(files), d)
		}
		check(rp)
	}
	const at = "2026-10-30T12:00:00Z"
	rec0, rec1 := RecoverySigner(seed, 0), RecoverySigner(seed, 1)

	// Many identities, one succession each.
	files := map[string][]byte{}
	for i := 0; i < n; i++ {
		gid := add(files, NewGenesis(key(i).Public(), Commitment(rec0.Public()), Adapter, at), key(i))
		add(files, NewSuccession(gid, rec0.Public(), gid, key(n+i).Public(), Commitment(rec1.Public()), nil, Adapter, at), rec0)
	}
	timed("identities", files, func(rp *Report) {
		if len(rp.Valid) != 2*n || len(rp.Chains) != n || len(rp.Errors) != 0 {
			t.Errorf("identities: %d valid, %d chains, %d errors", len(rp.Valid), len(rp.Chains), len(rp.Errors))
		}
	})

	// One long chain.
	files = map[string][]byte{}
	gid := add(files, NewGenesis(key(0).Public(), Commitment(rec0.Public()), Adapter, at), key(0))
	parent, rec := gid, rec0
	for g := 0; g < n; g++ {
		next := RecoverySigner(seed, g+1)
		parent = add(files, NewSuccession(gid, rec.Public(), parent, key(g+1).Public(), Commitment(next.Public()), nil, Adapter, at), rec)
		rec = next
	}
	timed("chain", files, func(rp *Report) {
		if c := rp.Chains[gid]; len(c) != n+1 || c[n].ID != parent || len(rp.Errors) != 0 {
			t.Errorf("chain: %d nodes, %d errors", len(c), len(rp.Errors))
		}
	})
}
