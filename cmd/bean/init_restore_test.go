package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// init refuses inside a git work tree unless -force.
func TestCLIInitInGitTree(t *testing.T) {
	top := t.TempDir()
	os.Mkdir(filepath.Join(top, ".git"), 0o700)
	dir := filepath.Join(top, "log")
	os.Mkdir(dir, 0o700)
	if _, errs, code := run(t, dir, "", "init"); code != 1 || !strings.Contains(errs, "inside a git work tree") {
		t.Fatalf("init in a git tree: %d %q", code, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "local")); !os.IsNotExist(err) {
		t.Error("refused init wrote local/")
	}
	if out, _, code := run(t, dir, "", "init", "-force"); code != 0 || !strings.HasPrefix(out, "identity ") {
		t.Errorf("init -force: %d", code)
	}
}

// -photo refuses devices and anything that is not a regular file.
func TestReadImageNotRegular(t *testing.T) {
	if _, _, err := readImage("/dev/zero"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("/dev/zero: %v", err)
	}
	if _, _, err := readImage(t.TempDir()); err == nil {
		t.Error("a directory passed")
	}
}

// restore -identity picks one of several matching genesis records.
func TestCLIRestoreIdentityFlag(t *testing.T) {
	dir := t.TempDir()
	out, _, code := run(t, dir, "", "init")
	if code != 0 {
		t.Fatal("init")
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	id := strings.TrimPrefix(lines[0], "identity ")
	var words []string
	for _, l := range lines[1:] {
		f := strings.Fields(l)
		words = append(words, f[len(f)-1])
	}
	os.RemoveAll(filepath.Join(dir, "local"))
	if _, errs, code := run(t, dir, strings.Join(words, " "), "restore", "-identity", "sha256:"+strings.Repeat("0", 64)); code == 0 {
		t.Errorf("restore to a wrong identity passed: %q", errs)
	}
	if out, errs, code := run(t, dir, strings.Join(words, " "), "restore", "-identity", id); code != 0 || !strings.HasPrefix(out, "identity "+id) {
		t.Errorf("restore -identity: %d %q %q", code, out, errs)
	}
}
