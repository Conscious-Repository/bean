package core

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// A stranger's own too-new genesis is a line, not a halt: the owner's
// verify exits 0 and gc runs (DECISIONS 155). gc reads only the targets of the local
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
// erases the target after the hold (SPEC 7.6 step 5).
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
	if h, err := c.readHolds(); err != nil || len(h) != 0 {
		t.Errorf("holds left: %v %v", h, err)
	}
}

// A purge tombstone carries nothing that names the erased content: no
// dedupe ref and no source, so a holder of the identity secret cannot test
// guesses of the erased reading against the synced tombstone (DECISIONS 168).
// The ref stays in local/purged-refs.jsonl of the copy that purged.
func TestPurgeTombstoneHasNoRef(t *testing.T) {
	a, _ := initStore(t)
	loc, _ := a.ReadLocal()
	obs := map[string]any{"start": "2026-10-01T09:00:00Z", "precision": "day"}
	in := `{"type":"lab.hiv","value":{"s":"positive"},"times":{"observed":{"start":"2026-10-01T09:00:00Z","precision":"day"}},"source":{"system":"labcorp","adapter":"x@1","ref_input":["bean/v0/ref","tuple","labcorp","lab.hiv",{"start":"2026-10-01T09:00:00Z","precision":"day"},{"s":"positive"},0]}}` + "\n"
	res, err := a.Append(strings.NewReader(in))
	if err != nil || len(res.IDs) != 1 {
		t.Fatal(err, res)
	}
	tid, err := a.Purge(res.IDs[0], "")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(a.path(PathOf(tid)))
	ref := DedupeRefTuple(DedupeKey(loc.Secret), "labcorp", "lab.hiv", obs, map[string]any{"s": "positive"}, 0)
	if strings.Contains(string(b), ref) || strings.Contains(string(b), "labcorp") {
		t.Errorf("tombstone names the erased reading: %s", b)
	}
	if r2, err := a.Append(strings.NewReader(in)); err != nil || r2.Added != 0 {
		t.Errorf("re-import on the purging copy added %d, %v", r2.Added, err)
	}
}

// The hold's own tombstone becomes invalid while another valid tombstone
// still purges the target: gc erases the target after the hold
// (DECISIONS 169).
func TestGCHoldTombstoneDiesOtherLives(t *testing.T) {
	s := newStore(t)
	me, words, err := s.Init(NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	xid := writeRaw(t, s, `{"type":"weight","value":{"n":1,"unit":"kg"},"times":{"observed":null},"source":{"system":"self","adapter":"x@1"}}`)
	loc, _ := s.ReadLocal()
	for i := 0; i < 2; i++ {
		id, b, err := SignRecord(NewTombstone(me, loc.Signer.Public(), xid, "", Adapter, "2026-10-28T12:00:00Z"), loc.Signer)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeFileAtomic(s.path(PathOf(id)), b, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Load(); err != nil {
			t.Fatal(err)
		}
	}
	h, _ := s.readHolds()
	var holder string
	for tid := range h {
		holder = tid
	}
	if _, err := s.Rotate(words, Seals{All: true, NoSeal: []string{holder}}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	l, _ := s.Load()
	if _, purged := l.Report.Purged[xid]; !purged || l.Report.Valid[holder] {
		t.Fatalf("setup: purged %v, holder valid %v", purged, l.Report.Valid[holder])
	}
	s.Now = func() time.Time { return testClock.Add(30 * 24 * time.Hour) }
	if n, _, err := s.GC(); err != nil || n != 1 {
		t.Errorf("gc erased %d, %v", n, err)
	}
}

// On an agent-only replica, local/identity names the person. When that
// identity is too new, verify exits 3 and gc refuses, as on the person's
// own copy (SPEC.md 12.1).
func TestAgentReplicaTooNewPerson(t *testing.T) {
	s := newStore(t)
	sk := NewEd25519Signer(make([]byte, 32))
	g := NewGenesis(sk.Public(), Commitment(sk.Public()), "x@1", "2026-10-28T12:00:00Z")
	g["v"] = 99.0
	id, b, err := SignRecord(g, sk)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(s.path(PathOf(id)), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AgentInit(id, "t"); err != nil {
		t.Fatal(err)
	}
	if _, code, err := s.VerifyLines(); err != nil || code != 3 {
		t.Errorf("exit %d, %v", code, err)
	}
	if _, _, err := s.GC(); err == nil || !strings.Contains(err.Error(), "too new") {
		t.Errorf("gc: %v", err)
	}
}

// A target hidden under T1 stays hidden when one sync brings a rotation
// that drops T1 and a new tombstone T2. T2 gets its own hold, so gc waits
// 7 days from T2's arrival, and the erase drops every hold of the target
// (SPEC 7.6 steps 3 and 5, DECISIONS 179).
func TestGCNewTombstoneGetsOwnHold(t *testing.T) {
	a := newStore(t)
	_, words, err := a.Init(NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	xid := writeRaw(t, a, `{"type":"weight","value":{"n":1,"unit":"kg"},"times":{"observed":null},"source":{"system":"self","adapter":"x@1"}}`)
	bdir := t.TempDir()
	if err := os.CopyFS(bdir, os.DirFS(a.Dir)); err != nil {
		t.Fatal(err)
	}
	b := Open(bdir)
	b.Now = a.Now
	// T1 on B, hidden under a hold at day 0.
	t1, err := b.Purge(xid, "")
	if err != nil {
		t.Fatal(err)
	}
	_ = t1
	// B's own purge erases at once; to get a hold, copy T1 into A and let
	// A hide it, then use A as "B" from here on. Simpler: swap roles.
	if _, err := a.Sync(b); err != nil {
		t.Fatal(err)
	}
	la, _ := a.Load()
	if !la.Hidden[xid] {
		t.Fatalf("setup: X not hidden on A")
	}
	h, _ := a.readHolds()
	t.Logf("A holds after T1: %v", h)
	// Day 30 on B: rotate with -no-seal T1, then purge X again with the new
	// key (B no longer holds X, so write T2 by hand).
	day30 := func() time.Time { return testClock.Add(30 * 24 * time.Hour) }
	b.Now = day30
	// B erased X; give it back so Purge finds it.
	ax, _ := os.ReadFile(hiddenPath(a.Dir, xid))
	if err := writeFileAtomic(b.path(PathOf(xid)), ax, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Rotate(words, Seals{All: true, NoSeal: []string{t1}}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	t2, err := b.Purge(xid, "")
	if err != nil {
		t.Fatal(err)
	}
	// One sync brings the rotation and T2 to A together.
	a.Now = day30
	if _, err := a.Sync(b); err != nil {
		t.Fatal(err)
	}
	la, _ = a.Load()
	h, _ = a.readHolds()
	_, hasT2Hold := h[t2]
	t.Logf("A after sync: T1 valid=%v T2 valid=%v X hidden=%v T2 hold=%v holds=%v", la.Report.Valid[t1], la.Report.Valid[t2], la.Hidden[xid], hasT2Hold, h)
	n, _, err := a.GC()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || !hasT2Hold {
		t.Errorf("gc erased %d at once; T2 hold %v: T2 arrived now and gets its own 7 days", n, hasT2Hold)
	}
	a.Now = func() time.Time { return day30().Add(8 * 24 * time.Hour) }
	if n, _, err := a.GC(); err != nil || n != 1 {
		t.Errorf("gc after T2's hold erased %d, %v", n, err)
	}
	if h, _ := a.readHolds(); len(h) != 0 {
		t.Errorf("holds left after the erase: %v", h)
	}
}
