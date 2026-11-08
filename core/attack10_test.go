package core

import (
	"os"
	"strings"
	"testing"
)

// Attack round 10, on fix-audit7 (63103b0). Synthetic data only.

// stranger returns a second identity whose genesis is planted in s, and its key.
func strangerIn(t *testing.T, s *Store) (string, Signer) {
	t.Helper()
	m, mid := initStore(t)
	g, err := os.ReadFile(m.path(PathOf(mid)))
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, s, PathOf(mid), g)
	return mid, m.mustLocal(t).Signer
}

// B1: gc erases the blobs that a bad-path "copy" names, at once, when a valid
// tombstone of the copy's subject names the copy's content id and no record
// file has that id. A stranger can satisfy that alone: a tombstone of their
// own for an id that never existed, and a JSON file under a bad path whose
// bytes give that id and name any blob. No hold, no record, no 7 days: gc
// erases the person's file that no record names yet (blob put, or a file
// bean import stored before its record).
func TestAttack10GCCopyErasesPersonsUnreferencedBlob(t *testing.T) {
	for _, withCopy := range []bool{false, true} {
		s, _ := initStore(t)
		h, err := s.PutBlob([]byte("PERSON'S SCAN, NOT YET NAMED BY A RECORD"))
		if err != nil {
			t.Fatal(err)
		}
		mid, mk := strangerIn(t, s)
		// Any parseable object: it need not be a record at all.
		junk := map[string]any{"subject": mid, "blobs": []any{map[string]any{"hash": h}}}
		target := RecordID(junk)
		rePlant(t, s, NewTombstone(mid, mk.Public(), target, "", "x@1", "2026-11-06T00:00:00Z"), mk)
		if withCopy {
			p := strings.TrimSuffix(s.path(PathOf(target)), ".json") + ".sync-conflict-20261107-101010-ABCDEFG.json"
			if err := writeFileAtomic(p, Canonical(junk), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.Load(); err != nil {
			t.Fatal(err)
		}
		r, b, err := s.GC()
		_, gerr := s.GetBlob(h)
		t.Logf("copy=%v: gc records=%d blobs=%d err=%v; person's file reads: %v", withCopy, r, b, err, gerr == nil)
		if gerr != nil {
			t.Errorf("copy=%v: a stranger's tombstone and a bad-path file made gc erase the person's file at once", withCopy)
		}
	}
}
