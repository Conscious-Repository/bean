package core

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// A stranger's own too-new genesis is a line, not a halt: the owner's
// verify exits 0 and gc runs (L1). gc reads only the targets of the local
// identity's tombstones, which a too-new stranger cannot change.
func TestStrangerTooNewDoesNotHaltOwner(t *testing.T) {
	a, _ := initStore(t)
	sk := NewEd25519Signer(make([]byte, 32))
	g := NewGenesis(sk.Public(), Commitment(sk.Public()), "x@1", "2026-10-28T12:00:00Z")
	g["v"] = 99.0
	id, b, err := SignRecord(g, sk)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(a.path(PathOf(id)), b, 0o600); err != nil {
		t.Fatal(err)
	}
	lines, code, err := a.VerifyLines()
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !slices.Contains(lines, "too new: "+id) {
		t.Errorf("exit %d, lines %v", code, lines)
	}
	if _, _, err := a.GC(); err != nil {
		t.Errorf("gc: %v", err)
	}
}

// Two valid tombstones for one target, the higher id seen first: gc still
// erases the target after the hold (L2).
func TestGCTwoTombstonesOneTarget(t *testing.T) {
	a, me := initStore(t)
	c := cloneLocal(t, a)
	xid := writeRaw(t, a, `{"type":"weight","value":{"n":1,"unit":"kg"},"times":{"observed":null},"source":{"system":"self","adapter":"x@1"}}`)
	if _, err := c.Sync(a); err != nil {
		t.Fatal(err)
	}
	loc, _ := a.ReadLocal()
	type file struct {
		id string
		b  []byte
	}
	var ts []file
	for i := 0; i < 2; i++ {
		id, b, err := SignRecord(NewTombstone(me, loc.Signer.Public(), xid, "", Adapter, "2026-10-28T12:00:00Z"), loc.Signer)
		if err != nil {
			t.Fatal(err)
		}
		ts = append(ts, file{id, b})
	}
	if ts[0].id < ts[1].id {
		ts[0], ts[1] = ts[1], ts[0]
	}
	for _, x := range ts {
		if err := writeFileAtomic(c.path(PathOf(x.id)), x.b, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Load(); err != nil {
			t.Fatal(err)
		}
	}
	c.Now = func() time.Time { return testClock.Add(30 * 24 * time.Hour) }
	n, _, err := c.GC()
	if err != nil || n != 1 {
		t.Errorf("gc erased %d, %v", n, err)
	}
	if _, err := os.Stat(hiddenPath(c.Dir, xid)); err == nil {
		t.Error("target still on disk")
	}
}

// A purge writes the target's dedupe ref into the tombstone, so a copy
// without local/purged-refs.jsonl still skips a re-import (Buildtall M2).
func TestPurgedRefInTombstone(t *testing.T) {
	a, _ := initStore(t)
	line := `{"type":"lab.glucose","value":{"n":90,"unit":"mg/dL"},"times":{"observed":null},"source":{"system":"labcorp","adapter":"x@1","ref_input":["bean/v0/ref","id","labcorp","s1/glucose",0]}}` + "\n"
	r, err := a.Append(strings.NewReader(line))
	if err != nil || r.Added != 1 {
		t.Fatal(r, err)
	}
	tid, err := a.Purge(r.IDs[0], "")
	if err != nil {
		t.Fatal(err)
	}
	b := cloneLocal(t, a)
	if _, err := b.Sync(a); err != nil {
		t.Fatal(err)
	}
	os.Remove(a.path(filePurgedRef))
	for _, s := range []*Store{a, b} {
		if _, err := os.Stat(s.path(filePurgedRef)); err == nil {
			os.Remove(s.path(filePurgedRef))
		}
		r2, err := s.Append(strings.NewReader(line))
		if err != nil || r2.Added != 0 {
			t.Errorf("%s: re-import added %d, %v (tombstone %s)", filepath.Base(s.Dir), r2.Added, err, tid)
		}
	}
}

// Sync refuses a source with more record bytes than the limit, before it
// reads any of them (Satoshi m4).
func TestSyncLimit(t *testing.T) {
	a, _ := initStore(t)
	writeRaw(t, a, `{"type":"weight","value":{"n":1,"unit":"kg"},"times":{"observed":null},"source":{"system":"self","adapter":"x@1"}}`)
	b := cloneLocal(t, a)
	b.SyncLimit = 100
	if _, err := b.Sync(a); err == nil || !strings.HasPrefix(err.Error(), "too large: ") {
		t.Errorf("sync above the limit: %v", err)
	}
	b.SyncLimit = 0
	if _, err := b.Sync(a); err != nil {
		t.Errorf("sync: %v", err)
	}
}
