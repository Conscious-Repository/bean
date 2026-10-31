package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Audit 4, M4: verify fails closed when this copy's own identity has no
// genesis, and a load never moves a deleted record back into objects/
// while the tombstone that hid it is only unresolved.
func TestOwnIdentityMissing(t *testing.T) {
	s, id := initStore(t)
	a, err := s.Add("weight", "80", AddOptions{Unit: "kg"})
	if err != nil {
		t.Fatal(err)
	}
	// The purge comes from another copy, so this one holds the target.
	b := cloneLocal(t, s)
	if _, err := b.Purge(a, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sync(b); err != nil {
		t.Fatal(err)
	}
	if l := verifyOK(t, s); !l.Hidden[a] {
		t.Fatal("the target is not held")
	}
	g, _ := os.ReadFile(s.path(PathOf(id)))
	if err := os.Remove(s.path(PathOf(id))); err != nil {
		t.Fatal(err)
	}
	lines, code, err := s.VerifyLines()
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(strings.Join(lines, "\n"), "error: "+id+": own identity missing") {
		t.Fatalf("verify without the own genesis: exit %d\n%s", code, strings.Join(lines, "\n"))
	}
	if _, err := os.Stat(s.path(PathOf(a))); err == nil {
		t.Fatal("the purged record came back into objects/")
	}
	if _, _, err := s.GC(); err == nil || !strings.Contains(err.Error(), "own identity missing") {
		t.Fatalf("gc without the own genesis: %v", err)
	}
	// The genesis comes back: all is as before.
	if err := os.WriteFile(s.path(PathOf(id)), g, 0o600); err != nil {
		t.Fatal(err)
	}
	l := verifyOK(t, s)
	if !l.Hidden[a] {
		t.Fatal("the purged record is not hidden")
	}
}

// A copy that has local/ but has not synced yet may write (DECISIONS 108
// as amended), but verify says what is missing.
func TestFirstWriteBeforeSyncVerifySays(t *testing.T) {
	a, id := initStore(t)
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
	if _, err := b.Add("weight", "80", AddOptions{Unit: "kg"}); err != nil {
		t.Fatal(err)
	}
	lines, code, _ := b.VerifyLines()
	if code != 1 || !strings.Contains(strings.Join(lines, "\n"), "error: "+id+": own identity missing: sync from another copy") {
		t.Fatalf("exit %d\n%s", code, strings.Join(lines, "\n"))
	}
}

// Dot files that operating systems and sync tools leave are ignored. A
// symlink or any other entry that is not a plain record file is an error,
// never a silent skip and never a hang.
func TestFolderJunk(t *testing.T) {
	s, id := initStore(t)
	if _, err := s.Add("weight", "80", AddOptions{Unit: "kg"}); err != nil {
		t.Fatal(err)
	}
	shard := filepath.Dir(s.path(PathOf(id)))
	for _, p := range []string{filepath.Join(shard, ".DS_Store"), s.path("objects/.DS_Store"), filepath.Join(shard, ".x.json.icloud"), s.path("blobs/.DS_Store")} {
		os.MkdirAll(filepath.Dir(p), 0o700)
		if err := os.WriteFile(p, []byte("junk"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(shard, ".stversions"), 0o700)
	verifyOK(t, s)

	// A symlinked shard: its records must not vanish without a word.
	moved := t.TempDir()
	sh := filepath.Base(shard)
	if err := os.Rename(shard, filepath.Join(moved, sh)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(moved, sh), shard); err != nil {
		t.Fatal(err)
	}
	lines, code, err := s.VerifyLines()
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(strings.Join(lines, "\n"), "error: objects/"+sh+": bad path") {
		t.Fatalf("symlinked shard: exit %d\n%s", code, strings.Join(lines, "\n"))
	}
}
