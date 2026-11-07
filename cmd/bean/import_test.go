package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// These replace the Python helper's tests: bean import keeps the file
// first, writes nothing when the importer refuses, and stops on a changed
// reading until the person says -corrections.

var (
	labcorpOnce sync.Once
	labcorpDir  string
	labcorpErr  error
)

// importerOnPath builds the real importers once and returns a PATH
// that finds it.
func importerOnPath(t *testing.T) string {
	t.Helper()
	labcorpOnce.Do(func() {
		labcorpDir, labcorpErr = os.MkdirTemp("", "bean-import-bin-*")
		if labcorpErr == nil {
			for _, name := range []string{"bean-import-labcorp", "bean-import-apple-health"} {
				out, err := exec.Command("go", "build", "-o", filepath.Join(labcorpDir, name), "../"+name).CombinedOutput()
				if err != nil {
					labcorpErr = &buildError{string(out)}
				}
			}
		}
	})
	if labcorpErr != nil {
		t.Fatal(labcorpErr)
	}
	return labcorpDir + string(os.PathListSeparator) + os.Getenv("PATH")
}

type buildError struct{ out string }

func (e *buildError) Error() string { return "build an importer: " + e.out }

func lcPad(s string, col int) string {
	if len(s) >= col {
		return s + "  "
	}
	return s + strings.Repeat(" ", col-len(s))
}

func lcRow(name, value, units, ref string) string {
	l := lcPad(lcPad("", 6)+name, 35) + "01"
	l = lcPad(l, 57) + value
	l = lcPad(l, 140) + units
	return lcPad(l, 160) + ref
}

// labReport is a synthetic report in pdftotext -layout form.
func labReport(glucose string) string {
	h := lcPad("", 6) + "Test"
	h = lcPad(h, 56) + "Current Result and Flag"
	h = lcPad(h, 92) + "Previous Result and Date"
	h = lcPad(h, 139) + "Units"
	h = lcPad(h, 158) + "Reference Interval"
	return strings.Join([]string{
		"Specimen ID: 000-000-0000-0  Date Collected: 01/02/2025 0815 Local",
		"Date Reported: 01/03/2025",
		h,
		"Basic Panel",
		lcRow("Glucose", glucose, "mg/dL", "70-99"),
		lcRow("Sodium", "140", "mmol/L", "134-144"),
		"",
		"Performing Labs",
		"01: BN - Labcorp Burlington, 1447 York Court, Burlington, NC 27215",
	}, "\n") + "\n"
}

func runPath(t *testing.T, dir, path string, args ...string) (string, string, int) {
	t.Helper()
	old := os.Getenv("PATH")
	os.Setenv("PATH", path)
	defer os.Setenv("PATH", old)
	return run(t, dir, "", args...)
}

func TestCLIImportKeepsFileFirst(t *testing.T) {
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	report := filepath.Join(t.TempDir(), "report.txt")
	os.WriteFile(report, []byte(labReport("101")), 0o600)

	out, errs, code := runPath(t, dir, path, "import", "labcorp", report)
	if code != 0 || !strings.Contains(out, "kept lab.report sha256:") || !strings.Contains(out, "added 2, skipped 0") || !strings.Contains(out, "2 results") {
		t.Fatalf("import: %d\n%s\n%s", code, out, errs)
	}
	ls, _, _ := run(t, dir, "", "ls")
	if !strings.Contains(ls, "lab.glucose") || !strings.Contains(ls, "lab.report") {
		t.Errorf("ls:\n%s", ls)
	}
	// The stored copy is the file, byte for byte.
	blob := strings.Fields(out[strings.Index(out, "kept lab.report ")+len("kept lab.report "):])[0]
	if got, _, code := run(t, dir, "", "blob", "get", blob); code != 0 || got != labReport("101") {
		t.Errorf("blob get: %d %q", code, got)
	}

	// Again: nothing new.
	out, errs, code = runPath(t, dir, path, "import", "labcorp", report)
	if code != 0 || !strings.Contains(out, "already kept") || !strings.Contains(out, "added 0, skipped 2") {
		t.Errorf("reimport: %d\n%s\n%s", code, out, errs)
	}

	// An amended report: the new file is kept, the changed reading is not
	// written until the person says so.
	os.WriteFile(report, []byte(labReport("99")), 0o600)
	out, errs, code = runPath(t, dir, path, "import", "labcorp", report)
	if code != 1 || !strings.Contains(out, "kept lab.report") || !strings.Contains(errs, "changed:") || !strings.Contains(errs, "-corrections") {
		t.Fatalf("amended: %d\n%s\n%s", code, out, errs)
	}
	if ls, _, _ := run(t, dir, "", "ls", "-type", "lab.glucose"); !strings.Contains(ls, "101") || strings.Contains(ls, " 99 ") {
		t.Errorf("changed reading was written:\n%s", ls)
	}
	out, errs, code = runPath(t, dir, path, "import", "labcorp", "-corrections", report)
	if code != 0 || !strings.Contains(out, "1 of them new values") {
		t.Fatalf("-corrections: %d\n%s\n%s", code, out, errs)
	}
	if ls, _, _ := run(t, dir, "", "ls", "-type", "lab.glucose"); strings.Contains(ls, "101") {
		t.Errorf("correction not shown:\n%s", ls)
	}
	if out, _, code := run(t, dir, "", "verify"); code != 0 {
		t.Errorf("verify: %d\n%s", code, out)
	}

	// After the person purges the kept report, a reimport does not quietly
	// add readings without it.
	js, _, _ := run(t, dir, "", "ls", "-type", "lab.report", "-json")
	for _, id := range recordIDs(js) {
		if _, errs, code := run(t, dir, "", "purge", id); code != 0 {
			t.Fatalf("purge: %d %s", code, errs)
		}
	}
	other := filepath.Join(t.TempDir(), "other.txt")
	os.WriteFile(other, []byte(labReport("99")), 0o600)
	_, errs, code = runPath(t, dir, path, "import", "labcorp", other)
	if code != 1 || !strings.Contains(errs, "does not keep the file") {
		t.Errorf("after purge: %d %s", code, errs)
	}
}

func recordIDs(jsonl string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(jsonl), "\n") {
		var r struct {
			ID string `json:"id"`
		}
		if json.Unmarshal([]byte(l), &r) == nil && r.ID != "" {
			out = append(out, r.ID)
		}
	}
	return out
}

// A report the importer refuses writes nothing: no record and no blob.
func TestCLIImportRefusedWritesNothing(t *testing.T) {
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	before := countFiles(t, dir)
	bad := filepath.Join(t.TempDir(), "bad.txt")
	os.WriteFile(bad, []byte(strings.Replace(labReport("101"), "Sodium", "Glucose", 1)), 0o600)
	_, errs, code := runPath(t, dir, path, "import", "labcorp", bad)
	if code != 1 || !strings.Contains(errs, "nothing was written") || !strings.Contains(errs, "map to one type") {
		t.Errorf("refused: %d %s", code, errs)
	}
	if after := countFiles(t, dir); after != before {
		t.Errorf("files %d -> %d", before, after)
	}
	if _, errs, code := runPath(t, dir, path, "import", "nosuch", bad); code != 1 || !strings.Contains(errs, "no importer bean-import-nosuch") {
		t.Errorf("missing importer: %d %s", code, errs)
	}
	if _, errs, code := runPath(t, dir, path, "import", "labcorp", dir); code != 1 || !strings.Contains(errs, "not a regular file") {
		t.Errorf("directory: %d %s", code, errs)
	}
}

// An importer that does not start with a record of the file gets no
// readings written.
func TestCLIImportNeedsArchiveFirst(t *testing.T) {
	bin := t.TempDir()
	fake := "#!/bin/sh\n" + `echo '{"type":"lab.x","value":{"n":1},"times":{"observed":null},"source":{"system":"fake","adapter":"f@1","ref_input":["bean/v0/ref","id","fake","x",0]}}'` + "\n"
	os.WriteFile(filepath.Join(bin, "bean-import-fake"), []byte(fake), 0o755)
	dir := t.TempDir()
	run(t, dir, "", "init")
	f := filepath.Join(t.TempDir(), "f.txt")
	os.WriteFile(f, []byte("x"), 0o600)
	_, errs, code := runPath(t, dir, bin+string(os.PathListSeparator)+os.Getenv("PATH"), "import", "fake", f)
	if code != 1 || !strings.Contains(errs, "did not start with a record of the file") || !strings.Contains(errs, "no reading was written") {
		t.Errorf("fake: %d %s", code, errs)
	}
	if ls, _, _ := run(t, dir, "", "ls"); strings.Contains(ls, "lab.x") {
		t.Errorf("reading written:\n%s", ls)
	}
}

func countFiles(t *testing.T, dir string) int {
	n := 0
	filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// importedReport sets up a log with one imported report and returns the
// log, the report file, the PATH and the lab.report record.
func importedReport(t *testing.T) (dir, report, path string, rec map[string]any) {
	t.Helper()
	path = importerOnPath(t)
	dir = t.TempDir()
	run(t, dir, "", "init")
	report = filepath.Join(t.TempDir(), "report.txt")
	os.WriteFile(report, []byte(labReport("101")), 0o600)
	if _, errs, code := runPath(t, dir, path, "import", "labcorp", report); code != 0 {
		t.Fatalf("import: %d %s", code, errs)
	}
	js, _, _ := run(t, dir, "", "ls", "-type", "lab.report", "-json")
	if err := json.Unmarshal([]byte(strings.TrimSpace(js)), &rec); err != nil {
		t.Fatalf("%v: %s", err, js)
	}
	return dir, report, path, rec
}

func importerRecord(t *testing.T, blob, report string) map[string]any {
	t.Helper()
	out, err := exec.Command(filepath.Join(labcorpDir, "bean-import-labcorp"), "-blob", blob, report).Output()
	if err != nil {
		t.Fatal(err)
	}
	var rec map[string]any
	first, _, _ := strings.Cut(string(out), "\n")
	json.Unmarshal([]byte(first), &rec)
	return rec
}

// A record about someone else that names the same file does not stand in
// for the person's own record after they purge it.
func TestCLIImportOtherSubjectDoesNotCount(t *testing.T) {
	dir, report, path, rec := importedReport(t)
	other := importerRecord(t, rec["value"].(map[string]any)["blob"].(string), report)
	other["subject"] = "sha256:" + strings.Repeat("7", 64)
	line, _ := json.Marshal(other)
	if _, errs, code := run(t, dir, string(line)+"\n", "append"); code != 0 {
		t.Fatalf("append: %d %s", code, errs)
	}
	run(t, dir, "", "purge", rec["id"].(string))
	if _, errs, code := runPath(t, dir, path, "import", "labcorp", report); code != 1 || !strings.Contains(errs, "does not keep the file") {
		t.Errorf("reimport: %d %s", code, errs)
	}
}

// A record that claims the blob with a wrong digest takes the dedupe ref of
// the real one. Import says so instead of reporting the file kept.
func TestCLIImportWrongDigestDoesNotCount(t *testing.T) {
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	report := filepath.Join(t.TempDir(), "report.txt")
	os.WriteFile(report, []byte(labReport("101")), 0o600)
	bid, _, _ := run(t, dir, "", "blob", "put", report)
	rec := importerRecord(t, strings.TrimSpace(bid), report)
	rec["context"].(map[string]any)["sha256"] = strings.Repeat("0", 64)
	line, _ := json.Marshal(rec)
	run(t, dir, string(line)+"\n", "append")
	if _, errs, code := runPath(t, dir, path, "import", "labcorp", report); code != 1 || !strings.Contains(errs, "does not keep the file") {
		t.Errorf("import: %d %s", code, errs)
	}
	if ls, _, _ := run(t, dir, "", "ls", "-type", "lab.glucose"); strings.Contains(ls, "glucose") {
		t.Errorf("readings written:\n%s", ls)
	}
}

// The object cache vouches for a file by size and time. A record edited in
// place with both kept does not count as kept.
func TestCLIImportProvesRecordFiles(t *testing.T) {
	dir, report, path, rec := importedReport(t)
	id := rec["id"].(string)
	obj := filepath.Join(dir, "objects", id[7:9], id[9:]+".json")
	old := time.Now().Add(-time.Hour)
	os.Chtimes(obj, old, old)
	run(t, dir, "", "ls") // caches the file
	data, _ := os.ReadFile(obj)
	sig := rec["sig"].(string)
	repl := sig[:len(sig)-1] + map[bool]string{true: "1", false: "0"}[sig[len(sig)-1] == '0']
	os.WriteFile(obj, []byte(strings.Replace(string(data), sig, repl, 1)), 0o600)
	os.Chtimes(obj, old, old)
	if _, errs, code := runPath(t, dir, path, "import", "labcorp", report); code != 1 || !strings.Contains(errs, "does not keep the file") {
		t.Errorf("reimport: %d %s", code, errs)
	}
}

// A corrupt file already at the blob's path is not adopted.
func TestCLIImportCorruptBlobRefused(t *testing.T) {
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	report := filepath.Join(t.TempDir(), "report.txt")
	os.WriteFile(report, []byte(labReport("101")), 0o600)
	bid, _, _ := run(t, dir, "", "blob", "put", report)
	h := strings.TrimPrefix(strings.TrimSpace(bid), "sha256:")
	os.WriteFile(filepath.Join(dir, "blobs", h[:2], h[2:]), []byte("corrupt"), 0o600)
	if _, errs, code := runPath(t, dir, path, "import", "labcorp", report); code != 1 || !strings.Contains(errs, "no record was written") {
		t.Errorf("import: %d %s", code, errs)
	}
	if js, _, _ := run(t, dir, "", "ls", "-type", "lab.report", "-json"); strings.TrimSpace(js) != "" {
		t.Errorf("archive written: %s", js)
	}
}
