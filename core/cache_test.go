package core

import (
	"os"
	"strings"
	"testing"
	"time"
)

// age backdates every record file past the racy window, as if it was
// written an hour ago.
func age(t *testing.T, s *Store) {
	t.Helper()
	disk, _, _, err := s.listObjects()
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	for _, d := range disk {
		if err := os.Chtimes(s.path(d.rel), old, old); err != nil {
			t.Fatal(err)
		}
	}
}

func proven(t *testing.T, s *Store) int {
	t.Helper()
	o, err := s.readObjects(false)
	if err != nil {
		t.Fatal(err)
	}
	return len(o.proven)
}

func TestObjectCache(t *testing.T) {
	s, _ := initStore(t)
	ids := appendLines(t, s, partial("hrv.rmssd", 1, nil), partial("hrv.rmssd", 2, nil), partial("hrv.rmssd", 3, nil)).IDs

	// Fresh files are racy: not cached yet.
	verifyOK(t, s)
	if n := proven(t, s); n != 0 {
		t.Errorf("racy files cached: %d", n)
	}
	age(t, s)
	if _, err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if n := proven(t, s); n != 4 {
		t.Fatalf("cached %d, want 4 (genesis and 3 claims)", n)
	}

	// An ordinary edit changes the stamp: Load sees it at once.
	p := s.path(PathOf(ids[0]))
	b, _ := os.ReadFile(p)
	os.WriteFile(p, []byte(strings.Replace(string(b), `"n":1`, `"n":7`, 1)), 0o600)
	l, _ := s.Load()
	if l.Report.Valid[ids[0]] || l.Report.Errors[PathOf(ids[0])] == "" {
		t.Error("edited file still valid")
	}
	os.WriteFile(p, b, 0o600)
	age(t, s)
	s.Load()

	// An edit that keeps size and time is trusted by ordinary commands,
	// as git trusts its index, and caught by verify, which rebuilds the
	// cache without it.
	p2 := s.path(PathOf(ids[1]))
	b2, _ := os.ReadFile(p2)
	fi, _ := os.Stat(p2)
	os.WriteFile(p2, []byte(strings.Replace(string(b2), `"n":2`, `"n":8`, 1)), 0o600)
	os.Chtimes(p2, fi.ModTime(), fi.ModTime())
	if l, _ := s.Load(); !l.Report.Valid[ids[1]] {
		t.Error("cache not used for a matching stamp")
	}
	lines, code, err := s.VerifyLines()
	if err != nil || code != 1 || !strings.Contains(strings.Join(lines, "\n"), PathOf(ids[1])) {
		t.Errorf("verify missed the edit: %d\n%s", code, strings.Join(lines, "\n"))
	}
	if l, _ := s.Load(); l.Report.Valid[ids[1]] {
		t.Error("verify left the bad file in the cache")
	}
	os.WriteFile(p2, b2, 0o600)
	age(t, s)

	// A garbled or missing cache changes nothing but time.
	want, _, _ := s.VerifyLines()
	for _, junk := range []string{"garbage\n\tno\ttabs\n", "objects/x\t1\t2\tab\n", ""} {
		os.WriteFile(s.path(fileObjCache), []byte(junk), 0o600)
		got, _, err := s.VerifyLines()
		if err != nil || strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("cache %q changed the result", junk)
		}
		l, err := s.Load()
		if err != nil || len(l.Report.Valid) != 4 {
			t.Errorf("load with cache %q: %v", junk, err)
		}
	}
	os.Remove(s.path(fileObjCache))
	if l, err := s.Load(); err != nil || len(l.Report.Valid) != 4 {
		t.Errorf("load without cache: %v", err)
	}
}

// The parallel verifier gives the same report as file-by-file checks.
func TestParallelMatchesSerial(t *testing.T) {
	s, _ := initStore(t)
	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, partial("hrv.rmssd", float64(i), nil))
	}
	appendLines(t, s, lines...)
	o, err := s.readObjects(true)
	if err != nil {
		t.Fatal(err)
	}
	files := o.files
	files["objects/00/"+strings.Repeat("0", 62)+".json"] = []byte("{")
	par := VerifyFiles(files)
	for p, data := range files {
		fr := CheckFile(p, data)
		switch {
		case fr.Err != nil && par.Errors[p] != fr.Err.Error():
			t.Errorf("%s: parallel %q, serial %q", p, par.Errors[p], fr.Err)
		case fr.Err == nil && par.Records[fr.Record.ID] == nil:
			t.Errorf("%s: parallel lost a good record", p)
		}
	}
}

// Two bean processes (a CLI and an MCP server) load at once after a purge
// arrives by sync. Both must succeed and agree.
func TestConcurrentLoads(t *testing.T) {
	a, _ := initStore(t)
	ids := appendLines(t, a, partial("hrv.rmssd", 1, nil), partial("hrv.rmssd", 2, nil)).IDs
	b := cloneLocal(t, a)
	if _, err := a.Purge(ids[0], ""); err != nil {
		t.Fatal(err)
	}
	// Copy the tombstone into B by hand, so no load has hidden the target yet.
	l, _ := a.Load()
	for p, data := range l.Files {
		if _, err := os.Stat(b.path(p)); os.IsNotExist(err) {
			writeFileAtomic(b.path(p), data, 0o600)
		}
	}
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			l, err := b.Load()
			if err == nil && (l.Visible(ids[0]) || !l.Visible(ids[1])) {
				err = os.ErrInvalid
			}
			errs <- err
		}()
	}
	for i := 0; i < 8; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent load: %v", err)
		}
	}
}

// A cache from a build with other signature rules is not used (N2): its
// entries skipped the small-order R check.
func TestObjectCacheOldRules(t *testing.T) {
	s, _ := initStore(t)
	appendLines(t, s, partial("hrv.rmssd", 1, nil))
	age(t, s)
	if _, err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if n := proven(t, s); n == 0 {
		t.Fatal("nothing cached")
	}
	b, _ := os.ReadFile(s.path(fileObjCache))
	os.WriteFile(s.path(fileObjCache), []byte(strings.TrimPrefix(string(b), objCacheRules)), 0o600)
	if n := proven(t, s); n != 0 {
		t.Errorf("old cache used: %d", n)
	}
}
