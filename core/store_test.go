package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testClock = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func newStore(t *testing.T) *Store {
	t.Helper()
	s := Open(t.TempDir())
	s.Now = func() time.Time { return testClock }
	return s
}

func initStore(t *testing.T) (*Store, string) {
	t.Helper()
	s := newStore(t)
	id, words, err := s.Init(NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 24 || !IsID(id) {
		t.Fatal("init output")
	}
	return s, id
}

// cloneLocal makes replica B of A: same local/, empty objects.
func cloneLocal(t *testing.T, a *Store) *Store {
	t.Helper()
	b := newStore(t)
	for _, f := range []string{fileIdentity, fileSecret, fileSignKey} {
		data, err := os.ReadFile(a.path(f))
		if err != nil {
			t.Fatal(err)
		}
		if err := writeFileAtomic(b.path(f), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// B needs the genesis to be a replica of the same identity.
	if _, err := b.Sync(a); err != nil {
		t.Fatal(err)
	}
	return b
}

func partial(typ string, n float64, refInput []any) string {
	m := map[string]any{
		"type":   typ,
		"value":  map[string]any{"n": n, "unit": "ms"},
		"times":  map[string]any{"observed": map[string]any{"start": "2026-10-20T00:00:00Z", "precision": "day"}},
		"source": map[string]any{"system": "test", "adapter": "test@1", "ref_input": refInput},
	}
	if refInput == nil {
		m["source"].(map[string]any)["ref"] = nil
		delete(m["source"].(map[string]any), "ref_input")
	}
	return string(Canonical(m))
}

func appendLines(t *testing.T, s *Store, lines ...string) AppendResult {
	t.Helper()
	res, err := s.Append(strings.NewReader(strings.Join(lines, "\n") + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func verifyOK(t *testing.T, s *Store) *Loaded {
	t.Helper()
	lines, code, err := s.VerifyLines()
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("verify exit %d:\n%s", code, strings.Join(lines, "\n"))
	}
	l, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func fileSet(t *testing.T, s *Store) string {
	t.Helper()
	var out []string
	for _, d := range []string{dirObjects, dirBlobs} {
		fs, bad, err := listShards(s.Dir, d)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range fs {
			out = append(out, f.rel)
		}
		out = append(out, bad...)
	}
	return strings.Join(out, "\n")
}

func TestInitAndAppend(t *testing.T) {
	s, id := initStore(t)
	if _, _, err := s.Init(NewKey{}); err != ErrAlreadyInitialized {
		t.Errorf("second init: %v", err)
	}
	res := appendLines(t, s, partial("hrv.rmssd", 40, []any{"bean/v0/ref", "id", "test", "1", 0.0}))
	if res.Added != 1 {
		t.Fatalf("added %d", res.Added)
	}
	l := verifyOK(t, s)
	r := l.Report.Records[res.IDs[0]]
	if r.Subject != id || r.Author != id || r.Imported != "2026-10-28T12:00:00Z" || !IsRef(r.Source.Ref) {
		t.Errorf("filled fields wrong: %+v", r)
	}
	if _, has := r.Raw["source"].(map[string]any)["ref_input"]; has {
		t.Error("ref_input reached the log")
	}
	// Uninitialized store.
	e := newStore(t)
	if _, err := e.Append(strings.NewReader(partial("x", 1, nil))); err != ErrNotInitialized {
		t.Errorf("uninitialized append: %v", err)
	}
}

// Conformance test 2: blob byte flip. Also GetBlob round trip.
func TestBlobFlip(t *testing.T) {
	s, _ := initStore(t)
	bid, err := s.PutBlob([]byte("raw export bytes"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := s.GetBlob(bid)
	if err != nil || string(pt) != "raw export bytes" {
		t.Fatalf("get blob: %s %v", pt, err)
	}
	rec := map[string]any{
		"type": "hrv.rmssd", "value": map[string]any{"n": 1.0},
		"times":  map[string]any{"observed": nil},
		"source": map[string]any{"system": "test", "adapter": "t@1", "ref": nil},
		"blobs":  []any{map[string]any{"hash": bid, "role": "raw-export", "media_type": "text/plain"}},
	}
	appendLines(t, s, string(Canonical(rec)))
	verifyOK(t, s)
	p := s.path(BlobPathOf(bid))
	data, _ := os.ReadFile(p)
	data[20] ^= 1
	os.WriteFile(p, data, 0o600)
	lines, code, _ := s.VerifyLines()
	if code != 1 || !strings.Contains(strings.Join(lines, "\n"), "blob corrupt: "+bid) {
		t.Errorf("exit %d\n%s", code, strings.Join(lines, "\n"))
	}
	os.Remove(p)
	lines, code, _ = s.VerifyLines()
	if code != 1 || !strings.Contains(strings.Join(lines, "\n"), "error: ") || !strings.Contains(strings.Join(lines, "\n"), "blob missing: "+bid) {
		t.Errorf("missing: exit %d\n%s", code, strings.Join(lines, "\n"))
	}
}

// Conformance test 4: union of two folders.
func TestSyncUnion(t *testing.T) {
	a, _ := initStore(t)
	b := cloneLocal(t, a)
	appendLines(t, a, partial("hrv.rmssd", 1, nil), partial("hrv.rmssd", 2, nil))
	appendLines(t, b, partial("hrv.rmssd", 3, nil))
	bid, _ := a.PutBlob([]byte("blob"))
	rec := map[string]any{"type": "x.y", "value": map[string]any{"blob": bid}, "times": map[string]any{"observed": nil},
		"source": map[string]any{"system": "test", "adapter": "t@1", "ref": nil},
		"blobs":  []any{map[string]any{"hash": bid, "role": "raw", "media_type": "text/plain"}}}
	appendLines(t, a, string(Canonical(rec)))
	r1, err := b.Sync(a)
	if err != nil || r1.Records != 3 || r1.Blobs != 1 {
		t.Fatalf("sync a->b %+v %v", r1, err)
	}
	r2, err := a.Sync(b)
	if err != nil || r2.Records != 1 {
		t.Fatalf("sync b->a %+v %v", r2, err)
	}
	if fileSet(t, a) != fileSet(t, b) {
		t.Error("file sets differ")
	}
	verifyOK(t, a)
	verifyOK(t, b)
	// Idempotent.
	r3, _ := b.Sync(a)
	if r3.Records != 0 || r3.Blobs != 0 {
		t.Errorf("second sync copied %+v", r3)
	}
}

// Conformance tests 5 and 6: same export twice, overlapping exports, with
// and without native ids.
func TestDedupeImports(t *testing.T) {
	for _, native := range []bool{true, false} {
		a, _ := initStore(t)
		b := cloneLocal(t, a)
		mk := func(i int) string {
			if native {
				return partial("hr.bpm", float64(60+i), []any{"bean/v0/ref", "id", "test", fmt.Sprint(i), 0.0})
			}
			obs := map[string]any{"start": fmt.Sprintf("2026-10-%02dT00:00:00Z", i), "precision": "day"}
			m := map[string]any{"type": "hr.bpm", "value": map[string]any{"n": float64(60 + i), "unit": "bpm"},
				"times":  map[string]any{"observed": obs},
				"source": map[string]any{"system": "test", "adapter": "t@1", "ref_input": []any{"bean/v0/ref", "tuple", "test", "hr.bpm", obs, map[string]any{"n": float64(60 + i), "unit": "bpm"}, 0.0}}}
			return string(Canonical(m))
		}
		var e1, e2 []string
		for i := 1; i <= 10; i++ {
			e1 = append(e1, mk(i))
		}
		for i := 6; i <= 15; i++ {
			e2 = append(e2, mk(i))
		}
		// Two exactly identical readings in the overlap: occurrence 1 of reading 8.
		dup := strings.Replace(mk(8), `,0]`, `,1]`, 1)
		e1 = append(e1, dup)
		e2 = append(e2, dup)
		r1 := appendLines(t, a, e1...)
		if r1.Added != 11 {
			t.Fatalf("native=%v E1 added %d", native, r1.Added)
		}
		r2 := appendLines(t, a, e2...)
		if r2.Added != 5 || r2.Skipped != 6 {
			t.Fatalf("native=%v E2 added %d skipped %d", native, r2.Added, r2.Skipped)
		}
		// Test 5: same export on a second device after sync.
		if _, err := b.Sync(a); err != nil {
			t.Fatal(err)
		}
		r3 := appendLines(t, b, e1...)
		if r3.Added != 0 {
			t.Errorf("native=%v second device added %d", native, r3.Added)
		}
		verifyOK(t, a)
	}
}

// Conformance tests 11, 12 and 15: propagate, stale replica, re-import
// after purge. With C14, B hides and only gc erases.
func TestPurgePropagation(t *testing.T) {
	a, _ := initStore(t)
	res := appendLines(t, a, partial("hrv.rmssd", 1, []any{"bean/v0/ref", "id", "test", "T", 0.0}))
	tgt := res.IDs[0]
	b := cloneLocal(t, a)
	c := cloneLocal(t, a) // stale copy from before the purge
	tid, err := a.Purge(tgt, "mistake")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.path(PathOf(tgt))); err == nil {
		t.Error("A did not erase at once")
	}
	if _, err := b.Sync(a); err != nil {
		t.Fatal(err)
	}
	lb, _ := b.Load()
	if !lb.Hidden[tgt] || lb.Visible(tgt) {
		t.Error("B did not hide T")
	}
	if _, err := os.Stat(b.path(PathOf(tgt))); err == nil {
		t.Error("hidden T still in objects/")
	}
	if _, ok := lb.Holds[tid]; !ok {
		t.Error("no hold entry")
	}
	// Day 8: nothing erased without gc; gc erases.
	b.Now = func() time.Time { return testClock.Add(8 * 24 * time.Hour) }
	verifyOK(t, b)
	if _, err := os.Stat(hiddenPath(b.Dir, tgt)); err != nil {
		t.Error("B erased on a timer")
	}
	n, _, err := b.GC()
	if err != nil || n != 1 {
		t.Fatalf("gc %d %v", n, err)
	}
	if _, err := os.Stat(hiddenPath(b.Dir, tgt)); err == nil {
		t.Error("gc did not erase")
	}
	verifyOK(t, b)
	// Test 12: stale replica C syncs into A; A must not get T back.
	r, err := a.Sync(c)
	if err != nil {
		t.Fatal(err)
	}
	if r.Refused != 1 || r.Records != 0 {
		t.Errorf("stale sync %+v", r)
	}
	if _, err := os.Stat(a.path(PathOf(tgt))); err == nil {
		t.Error("A got T back")
	}
	if _, err := c.Sync(a); err != nil {
		t.Fatal(err)
	}
	lc, _ := c.Load()
	if !lc.Hidden[tgt] {
		t.Error("C did not hide T")
	}
	// Test 15: re-import after purge adds nothing on A and on B.
	for _, s := range []*Store{a, b} {
		rr := appendLines(t, s, partial("hrv.rmssd", 1, []any{"bean/v0/ref", "id", "test", "T", 0.0}))
		if rr.Added != 0 || rr.Skipped != 1 {
			t.Errorf("re-import added %d", rr.Added)
		}
	}
	// Purge errors.
	if _, err := a.Purge(tid, ""); err != ErrBadTarget {
		t.Errorf("tombstone of tombstone: %v", err)
	}
	if _, err := a.Purge("sha256:"+strings.Repeat("00", 32), ""); err == nil || !strings.HasPrefix(err.Error(), "input missing") {
		t.Errorf("missing: %v", err)
	}
}

// Conformance test 16: stranger tombstone cannot block a record.
func TestStrangerTombstone(t *testing.T) {
	a, _ := initStore(t)
	res := appendLines(t, a, partial("hrv.rmssd", 1, nil))
	tgt := res.IDs[0]
	// J is another identity with a copy of A's genesis in its log (replica B).
	j, _ := initStore(t)
	if _, err := j.Sync(a); err != nil {
		t.Fatal(err)
	}
	// J writes a tombstone naming T by direct file write (purge would refuse).
	lj, _ := j.Load()
	tb := NewTombstone(lj.Local.Identity, lj.Local.Signer.Public(), tgt, "", Adapter, "2026-10-28T12:00:00Z")
	tid, b, _ := SignRecord(tb, lj.Local.Signer)
	os.Remove(j.path(PathOf(tgt)))
	writeFileAtomic(j.path(PathOf(tid)), b, 0o600)
	r, err := j.Sync(a)
	if err != nil {
		t.Fatal(err)
	}
	if r.Refused != 0 {
		t.Errorf("stranger tombstone refused T: %+v", r)
	}
	lines, code, _ := j.VerifyLines()
	if code != 1 || !strings.Contains(strings.Join(lines, "\n"), tid+": not subject") {
		t.Errorf("exit %d\n%s", code, strings.Join(lines, "\n"))
	}
	lj, _ = j.Load()
	if !lj.Visible(tgt) {
		t.Error("T not visible on J")
	}
}

// Conformance test 25 (rewritten for seed-only rotation): a thief's
// tombstone is hidden on B, then a rotation that does not seal it makes it
// invalid, and B restores T.
func TestHoldRestore(t *testing.T) {
	a, id := initStore(t)
	res := appendLines(t, a, partial("hrv.rmssd", 1, nil))
	tgt := res.IDs[0]
	b := cloneLocal(t, a)
	// Thief: tombstone by direct file write on A (A's key, so it is valid).
	la, _ := a.Load()
	tb := NewTombstone(id, la.Local.Signer.Public(), tgt, "thief", Adapter, "2026-10-28T12:00:00Z")
	tid, tbytes, _ := SignRecord(tb, la.Local.Signer)
	writeFileAtomic(a.path(PathOf(tid)), tbytes, 0o600)
	if _, err := b.Sync(a); err != nil {
		t.Fatal(err)
	}
	lb, _ := b.Load()
	if !lb.Hidden[tgt] {
		t.Fatal("B did not hide T")
	}
	// Day 3: the person rotates without sealing D. Build the succession by
	// hand (M5 adds the command); we need the paper seed, so make a fresh
	// identity whose seed we hold instead.
	seed := bytes.Repeat([]byte{5}, 32)
	a2 := newStore(t)
	sk := bytes.Repeat([]byte{6}, 32)
	signer := NewEd25519Signer(sk)
	g := NewGenesis(signer.Public(), Commitment(RecoverySigner(seed, 0).Public()), Adapter, "2026-10-28T12:00:00Z")
	gid, gb, _ := SignRecord(g, signer)
	writeFileAtomic(a2.path(PathOf(gid)), gb, 0o600)
	writeFileAtomic(a2.path(fileIdentity), []byte(gid+"\n"), 0o600)
	writeFileAtomic(a2.path(fileSecret), []byte(strings.Repeat("00", 32)+"\n"), 0o600)
	writeFileAtomic(a2.path(fileSignKey), []byte("ed25519:"+fmt.Sprintf("%x", sk)+"\n"), 0o600)
	res = appendLines(t, a2, partial("hrv.rmssd", 1, nil))
	tgt = res.IDs[0]
	b2 := cloneLocal(t, a2)
	tb = NewTombstone(gid, signer.Public(), tgt, "thief", Adapter, "2026-10-28T12:00:00Z")
	tid, tbytes, _ = SignRecord(tb, signer)
	writeFileAtomic(a2.path(PathOf(tid)), tbytes, 0o600)
	b2.Sync(a2)
	lb, _ = b2.Load()
	if !lb.Hidden[tgt] {
		t.Fatal("B2 did not hide T")
	}
	nk := NewEd25519Signer(bytes.Repeat([]byte{7}, 32))
	rec := RecoverySigner(seed, 0)
	// The claim T is sealed (C26) so that only D's missing seal matters.
	succ := NewSuccession(gid, rec.Public(), gid, nk.Public(), Commitment(RecoverySigner(seed, 1).Public()), []string{tgt}, Adapter, "2026-10-31T12:00:00Z")
	sid, sb, _ := SignRecord(succ, rec)
	writeFileAtomic(a2.path(PathOf(sid)), sb, 0o600)
	b2.Now = func() time.Time { return testClock.Add(3 * 24 * time.Hour) }
	if _, err := b2.Sync(a2); err != nil {
		t.Fatal(err)
	}
	lines, _, _ := b2.VerifyLines()
	out := strings.Join(lines, "\n")
	if !strings.Contains(out, tid+": not sealed") {
		t.Errorf("D not 'not sealed':\n%s", out)
	}
	lb, _ = b2.Load()
	if lb.Hidden[tgt] || !lb.Visible(tgt) {
		t.Error("B2 did not restore T")
	}
	if len(lb.Holds) != 0 {
		t.Error("hold entry not removed")
	}
	b2.Now = func() time.Time { return testClock.Add(30 * 24 * time.Hour) }
	n, _, _ := b2.GC()
	if n != 0 {
		t.Error("gc erased a restored record")
	}
	if _, err := os.Stat(b2.path(PathOf(tgt))); err != nil {
		t.Error("T gone")
	}
}

// Conformance test 26: export round trip.
func TestExportRoundTrip(t *testing.T) {
	a, _ := initStore(t)
	bid, _ := a.PutBlob([]byte("csv,data\n1,2\n"))
	rec := map[string]any{"type": "hrv.rmssd", "value": map[string]any{"n": 1.0}, "times": map[string]any{"observed": nil},
		"source": map[string]any{"system": "test", "adapter": "t@1", "ref": nil},
		"blobs":  []any{map[string]any{"hash": bid, "role": "raw-export", "media_type": "text/csv"}}}
	res := appendLines(t, a, string(Canonical(rec)), partial("hrv.rmssd", 2, nil), partial("hrv.rmssd", 3, nil))
	if _, err := a.Purge(res.IDs[2], ""); err != nil {
		t.Fatal(err)
	}
	x := filepath.Join(t.TempDir(), "x")
	r, bl, err := a.Export(x)
	if err != nil || r != 4 || bl != 1 { // genesis, 2 claims, tombstone
		t.Fatalf("export %d %d %v", r, bl, err)
	}
	man, _ := os.ReadFile(filepath.Join(x, "manifest.json"))
	if !strings.Contains(string(man), `"plaintext_sha256"`) || !strings.Contains(string(man), bid) {
		t.Errorf("manifest %s", man)
	}
	if _, _, err := a.Export(x); err == nil {
		t.Error("export into non-empty dir")
	}
	b := newStore(t)
	for _, f := range []string{fileIdentity, fileSecret, fileSignKey} {
		d, _ := os.ReadFile(a.path(f))
		writeFileAtomic(b.path(f), d, 0o600)
	}
	r, bl, err = b.ImportExport(x)
	if err != nil || r != 4 || bl != 1 {
		t.Fatalf("import %d %d %v", r, bl, err)
	}
	if fileSet(t, a) != fileSet(t, b) {
		t.Errorf("sets differ:\n%s\n---\n%s", fileSet(t, a), fileSet(t, b))
	}
	verifyOK(t, b)
}

// C13: a replica replaces a stored file with a bad signature when a good
// copy arrives.
func TestSyncReplacesBadSignature(t *testing.T) {
	a, _ := initStore(t)
	res := appendLines(t, a, partial("hrv.rmssd", 1, nil))
	id := res.IDs[0]
	b := cloneLocal(t, a)
	good, _ := os.ReadFile(a.path(PathOf(id)))
	v, _ := Parse(good)
	v.(map[string]any)["sig"] = "ed25519:" + strings.Repeat("00", 64)
	bad := Canonical(v)
	// Ensure bad still parses and has the right id, but a wrong signature.
	if fr := CheckFile(PathOf(id), bad); fr.Err == nil || fr.Err.Error() != "bad signature" {
		t.Fatalf("test setup: %v", fr.Err)
	}
	writeFileAtomic(b.path(PathOf(id)), bad, 0o600)
	r, err := b.Sync(a)
	if err != nil || r.Replaced != 1 {
		t.Fatalf("sync %+v %v", r, err)
	}
	now, _ := os.ReadFile(b.path(PathOf(id)))
	if !bytes.Equal(now, good) {
		t.Error("bad copy kept")
	}
	// Sync never copies a file that fails its signature.
	c := cloneLocal(t, a)
	writeFileAtomic(a.path(PathOf(id)), bad, 0o600)
	r, _ = c.Sync(a)
	if r.Records != 0 {
		t.Error("copied a bad signature")
	}
}
