package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func r12import(t *testing.T, dir, path, name, body string, args ...string) (string, string, int) {
	t.Helper()
	src := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(src, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	a := append(append([]string{"import", "csv"}, args...), src)
	out, errs, code := runPath(t, dir, path, a...)
	t.Logf("$ bean %s\n%s%s(exit %d)", strings.Join(a[:len(a)-1], " "), out, errs, code)
	return out, errs, code
}

func r12ls(t *testing.T, dir, typ string) string {
	t.Helper()
	out, _, _ := run(t, dir, "", "ls", "-type", typ, "-json")
	return out
}

// R12-4 end to end: the second row with the same id was skipped with no
// word. Now the file is refused, the error names both lines, and nothing is
// written, not even the file.
func TestR12ImportRepeatedIDRefused(t *testing.T) {
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	_, errs, code := r12import(t, dir, path, "w.csv", "time,value,id\n2026-10-20T08:00:00Z,70.1,7\n2026-10-21T08:00:00Z,70.9,7\n", "-type", "body.weight", "-unit", "kg")
	if code != 1 || !strings.Contains(errs, "line 3: its id is the id of line 2") {
		t.Errorf("exit %d, stderr %q", code, errs)
	}
	if ls := r12ls(t, dir, "body.weight") + r12ls(t, dir, "csv.file"); ls != "" {
		t.Errorf("a refused file wrote: %s", ls)
	}
}

// R12-6: a reading of another type with the same id and the same value was
// skipped with exit 0: the value check ran before the type check. Now it is
// a changed: line that names the type and the reason, and exit 1.
func TestR12ImportCrossTypeSameValueReported(t *testing.T) {
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	if _, _, code := r12import(t, dir, path, "steps.csv", "time,value,id\n2026-10-20T08:00:00Z,12,1\n", "-type", "steps", "-unit", "count"); code != 0 {
		t.Fatalf("first import exit %d", code)
	}
	for _, corrections := range []bool{false, true} {
		args := []string{"-type", "flights.climbed", "-unit", "count"}
		if corrections {
			args = append(args, "-corrections")
		}
		out, errs, code := r12import(t, dir, path, "flights.csv", "time,value,id\n2026-10-21T09:00:00Z,12,1\n", args...)
		if code != 1 || !strings.Contains(errs, "changed: ") || !strings.Contains(errs, "(type flights.climbed): not written, because its type differs") {
			t.Errorf("corrections=%v: exit %d, stdout %q, stderr %q", corrections, code, out, errs)
		}
	}
	if ls := r12ls(t, dir, "flights.climbed"); ls != "" {
		t.Errorf("a reading of another type under the same ref was written: %s", ls)
	}
	// The same type and value is still skipped, with exit 0.
	out, _, code := r12import(t, dir, path, "steps2.csv", "time,value,id\n2026-10-20T08:00:00Z,12,1\n", "-type", "steps", "-unit", "count")
	if code != 0 || !strings.Contains(out, "added 0, skipped 1") {
		t.Errorf("same reading again: exit %d, %q", code, out)
	}
}

// R12-6 through append: the same rule, the same changed: line and exit 1.
func TestR12AppendCrossTypeSameValueReported(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "", "init")
	line := func(typ string) string {
		return `{"type":"` + typ + `","value":{"n":12,"unit":"count"},"times":{"observed":null},"source":{"system":"s","adapter":"t@1","ref_input":["bean/v0/ref","id","s","1",0]}}` + "\n"
	}
	if _, _, code := run(t, dir, line("steps"), "append"); code != 0 {
		t.Fatalf("first append exit %d", code)
	}
	out, errs, code := run(t, dir, line("flights.climbed"), "append")
	if code != 1 || !strings.Contains(errs, "(type flights.climbed): not written, because its type differs") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out, errs)
	}
}

// R12-7: rows whose time does not parse had observed null, and the tuple
// ignores context.raw_time, so a re-export with one more day stored the
// wrong day. Now bean import refuses the file and names the line.
func TestR12ImportUnreadTimeRefused(t *testing.T) {
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	_, errs, code := r12import(t, dir, path, "v1.csv", "time,value\n2026-10-01,70\n2026-10-03,70\n", "-type", "body.weight", "-unit", "kg")
	if code != 1 || !strings.Contains(errs, "line 2: the time is not RFC 3339") {
		t.Errorf("exit %d, stderr %q", code, errs)
	}
	if ls := r12ls(t, dir, "body.weight") + r12ls(t, dir, "csv.file"); ls != "" {
		t.Errorf("a refused file wrote: %s", ls)
	}
}
