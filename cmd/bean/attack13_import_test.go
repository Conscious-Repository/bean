package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Two CSV exports under the default -system csv that both number their rows
// 1..n: a reading of the second file whose value equals the first file's
// reading at the same id was dropped as "skipped", exit 0, though its time
// differs. Now each is a changed: line, and the import exits 1.
func TestA13CrossFileSameIDSameValueDropped(t *testing.T) {
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	if _, _, code := r12import(t, dir, path, "w2025.csv", "id,time,value\n1,2025-01-05T08:00:00Z,70.4\n2,2025-01-06T08:00:00Z,70.1\n", "-type", "body.weight", "-unit", "kg"); code != 0 {
		t.Fatalf("first import exit %d", code)
	}
	out, errs, code := r12import(t, dir, path, "w2026.csv", "id,time,value\n1,2026-03-09T08:00:00Z,70.4\n2,2026-03-10T08:00:00Z,70.1\n", "-type", "body.weight", "-unit", "kg")
	ls := r12ls(t, dir, "body.weight")
	t.Logf("exit %d stdout %q stderr %q\nls:\n%s", code, out, errs, ls)
	// Either written, or refused loudly: never "skipped" with exit 0.
	if !strings.Contains(ls, "2026-03-09") && (code != 1 || strings.Count(errs, "its time differs") != 2) {
		t.Errorf("the 2026 readings were dropped silently (exit %d)", code)
	}
}

// SPEC 9.5 now says a record of another type is never skipped in silence.
// Two lines of one input with one ref and two types: the second is skipped,
// exit 0, no changed: line.
func TestA13SameInputOtherTypeSilent(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "", "init")
	line := func(typ string, n string) string {
		return `{"type":"` + typ + `","value":{"n":` + n + `,"unit":"count"},"times":{"observed":null},"source":{"system":"s","adapter":"t@1","ref_input":["bean/v0/ref","id","s","1",0]}}` + "\n"
	}
	o, e, c := run(t, dir, line("hrv.rmssd", "41")+line("heart.rate.resting", "52"), "append")
	t.Logf("exit %d stdout %q stderr %q", c, o, e)
	ls, _, _ := run(t, dir, "", "ls", "-type", "heart.rate.resting")
	if c == 0 && ls == "" {
		t.Errorf("a reading of another type was skipped in silence")
	}
}

// Two current records of one ref with different types (two devices with the
// same identity imported before a sync). Re-importing either one exactly was
// skipped before; now one of the two is a changed: line, exit 1, and
// -corrections cannot write it either, for a reading that is in the log.
func TestA13TwoCurrentTypesReimport(t *testing.T) {
	d1 := t.TempDir()
	run(t, d1, "", "init")
	d2 := filepath.Join(t.TempDir(), "copy")
	if out, err := exec.Command("cp", "-R", d1, d2).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v %s", err, out)
	}
	line := func(typ string) string {
		return `{"type":"` + typ + `","value":{"n":12,"unit":"count"},"times":{"observed":null},"source":{"system":"s","adapter":"t@1","ref_input":["bean/v0/ref","id","s","1",0]}}` + "\n"
	}
	if _, e, c := run(t, d1, line("steps"), "append"); c != 0 {
		t.Fatal(e)
	}
	if _, e, c := run(t, d2, line("flights.climbed"), "append"); c != 0 {
		t.Fatal(e)
	}
	// sync: copy d2's objects into d1
	filepath.Walk(filepath.Join(d2, "objects"), func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(d2, p)
		dst := filepath.Join(d1, rel)
		if _, err := os.Stat(dst); err == nil {
			return nil
		}
		os.MkdirAll(filepath.Dir(dst), 0o700)
		b, _ := os.ReadFile(p)
		return os.WriteFile(dst, b, 0o600)
	})
	out, _, _ := run(t, d1, "", "ls", "-json")
	t.Logf("ls after sync:\n%s", out)
	for _, typ := range []string{"steps", "flights.climbed"} {
		for _, corr := range []bool{false, true} {
			args := []string{"append"}
			if corr {
				args = append(args, "-corrections")
			}
			o, e, c := run(t, d1, line(typ), args...)
			t.Logf("re-append %s corrections=%v: exit %d %q %q", typ, corr, c, o, e)
			if c != 0 {
				t.Errorf("re-append of a reading that is current in the log: exit %d %s", c, e)
			}
		}
	}
}
