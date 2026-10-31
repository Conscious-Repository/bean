package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Text from a record (a note, a value, a label, a file name) must not
// control the terminal or fake output lines.
func TestCLIOutputEscapesControlCharacters(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "", "init")
	hostile := "hi\x1b[2J\nverify: OK all fine\u202e\u2028"
	id, _, code := run(t, dir, "", "add", "weight", "80", "-unit", "kg", "-note", hostile)
	if code != 0 {
		t.Fatal("add failed")
	}
	id = strings.TrimSpace(id)
	run(t, dir, "", "add", "note", hostile, "-about", id)
	if err := os.WriteFile(filepath.Join(dir, "objects", "00", "x\x1b[2J\n.json"), []byte("{}"), 0o600); err != nil {
		if err := os.MkdirAll(filepath.Join(dir, "objects", "00"), 0o700); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, "objects", "00", "x\x1b[2J\n.json"), []byte("{}"), 0o600)
	}
	for _, args := range [][]string{{"ls"}, {"ls", "-all"}, {"show", id}, {"verify"}} {
		out, errs, _ := run(t, dir, "", args...)
		all := out + errs
		if strings.ContainsAny(all, "\x1b\u202e\u2028") {
			t.Errorf("%v: raw control character in output:\n%q", args, all)
		}
		for _, line := range strings.Split(all, "\n") {
			if strings.HasPrefix(line, "verify: OK") {
				t.Errorf("%v: a note made its own line: %q", args, all)
			}
		}
	}
	out, _, _ := run(t, dir, "", "ls")
	if !strings.Contains(out, `hi\x1b[2J\x0averify: OK all fine\u202e\u2028`) {
		t.Errorf("ls does not show the escaped note: %q", out)
	}
}
