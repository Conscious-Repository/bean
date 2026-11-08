package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Conscious-Repository/bean/core"
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
			for _, name := range []string{"bean-import-labcorp", "bean-import-apple-health", "bean-import-csv"} {
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
	// add readings without it, and does not store the file again.
	js, _, _ := run(t, dir, "", "ls", "-type", "lab.report", "-json")
	for _, id := range recordIDs(js) {
		if _, errs, code := run(t, dir, "", "purge", id); code != 0 {
			t.Fatalf("purge: %d %s", code, errs)
		}
	}
	other := filepath.Join(t.TempDir(), "other.txt")
	os.WriteFile(other, []byte(labReport("99")), 0o600)
	_, errs, code = runPath(t, dir, path, "import", "labcorp", other)
	if code != 1 || !strings.Contains(errs, "you deleted this file before") || !strings.Contains(errs, "nothing was written") {
		t.Errorf("after purge: %d %s", code, errs)
	}
	if n := countFiles(t, filepath.Join(dir, "blobs")); n != 0 {
		t.Errorf("%d blobs left after purging every report", n)
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
	if code != 1 || !strings.Contains(errs, "did not start with a record of the file") || !strings.Contains(errs, "nothing was written") {
		t.Errorf("fake: %d %s", code, errs)
	}
	if n := countFiles(t, filepath.Join(dir, "blobs")); n != 0 {
		t.Errorf("%d blobs stored for an importer that keeps no file", n)
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
	if _, errs, code := runPath(t, dir, path, "import", "labcorp", report); code != 1 || !strings.Contains(errs, "you deleted this file before") {
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

// Regression tests from the audit of 028a3a5. Synthetic data only. Each one
// failed before its fix.

// Purging the record that keeps a file erases the file at once, because no
// reading names the blob, and a later import of it stores nothing.
func TestCLIImportPurgeErasesFile(t *testing.T) {
	path := importerOnPath(t)
	for _, tc := range []struct{ source, typ string }{
		{"labcorp", "lab.report"},
		{"apple-health", "apple-health.export"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			dir := t.TempDir()
			run(t, dir, "", "init")
			src := filepath.Join(t.TempDir(), "file")
			args := []string{"import", tc.source}
			if tc.source == "labcorp" {
				os.WriteFile(src, []byte(labReport("101")), 0o600)
				args = append(args, "-tz", "America/Chicago")
			} else {
				appleZip(t, src, appleXML)
				args = append(args, "-records")
			}
			args = append(args, src)
			out, errs, code := runPath(t, dir, path, args...)
			if code != 0 || strings.Contains(out, "added 0") {
				t.Fatalf("import: %d %s %s", code, out, errs)
			}
			bid := regexp.MustCompile(`sha256:[0-9a-f]{64}`).FindString(out)
			js, _, _ := run(t, dir, "", "ls", "-type", tc.typ, "-json")
			for _, id := range recordIDs(js) {
				if _, errs, code := run(t, dir, "", "purge", id); code != 0 {
					t.Fatalf("purge: %s", errs)
				}
			}
			if got, _, code := run(t, dir, "", "blob", "get", bid); code == 0 {
				t.Errorf("purged file still readable: %d bytes", len(got))
			}
			if _, err := toolReadBlob(core.Open(dir), core.Filter{}, bid, 0); err == nil {
				t.Error("MCP read_blob serves the purged file")
			}
			_, errs, code = runPath(t, dir, path, args...)
			if code != 1 || !strings.Contains(errs, "you deleted this file before") {
				t.Errorf("reimport: %d %s", code, errs)
			}
			if n := countFiles(t, filepath.Join(dir, "blobs")); n != 0 {
				t.Errorf("reimport stored the purged file again: %d blobs", n)
			}
			if out, _, code := run(t, dir, "", "verify"); code != 0 {
				t.Errorf("verify: %d %s", code, out)
			}
		})
	}
}

// bean import csv keeps the file as csv.file, then the readings.
func TestCLIImportCSV(t *testing.T) {
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	src := filepath.Join(t.TempDir(), "hrv.csv")
	os.WriteFile(src, []byte("time,value,id\n2026-10-20T03:00:00Z,42.5,r1\n2026-10-21T03:00:00Z,40,r2\n"), 0o600)
	args := []string{"import", "csv", "-type", "hrv.rmssd", "-unit", "ms", src}
	out, errs, code := runPath(t, dir, path, args...)
	if code != 0 || !strings.Contains(out, "kept csv.file sha256:") || !strings.Contains(out, "added 2, skipped 0") {
		t.Fatalf("import: %d\n%s\n%s", code, out, errs)
	}
	if out, _, code := runPath(t, dir, path, args...); code != 0 || !strings.Contains(out, "already kept") || !strings.Contains(out, "added 0, skipped 2") {
		t.Errorf("retry: %d %s", code, out)
	}
	if ls, _, _ := run(t, dir, "", "ls", "-type", "hrv.rmssd"); strings.Count(ls, "hrv.rmssd") != 2 {
		t.Errorf("ls:\n%s", ls)
	}
	// A refused CSV leaves no blob.
	bad := filepath.Join(t.TempDir(), "bad.csv")
	os.WriteFile(bad, []byte("time,reading\n1,2\n"), 0o600)
	before := countFiles(t, filepath.Join(dir, "blobs"))
	if _, errs, code := runPath(t, dir, path, "import", "csv", "-type", "x", bad); code != 1 || !strings.Contains(errs, "nothing was written") {
		t.Errorf("bad csv: %d %s", code, errs)
	}
	if after := countFiles(t, filepath.Join(dir, "blobs")); after != before {
		t.Errorf("blobs %d -> %d", before, after)
	}
}

// An interrupted or terminated import removes its plaintext copy and stops
// the importer.
func TestCLIImportSignalRemovesSnapshot(t *testing.T) {
	for _, s := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(s.String(), func(t *testing.T) {
			bin := t.TempDir()
			os.WriteFile(filepath.Join(bin, "bean-import-slow"), []byte("#!/bin/sh\nsleep 30\n"), 0o755)
			dir := t.TempDir()
			run(t, dir, "", "init")
			tmp := t.TempDir()
			f := filepath.Join(t.TempDir(), "report.txt")
			os.WriteFile(f, []byte("SYNTHETIC PATIENT Doe, Jane DOB 01/01/1990 glucose 101\n"), 0o600)
			cmd := exec.Command(os.Args[0], "import", "slow", f)
			cmd.Env = append(os.Environ(), "BEAN_TEST_MAIN=1", "BEAN_DIR="+dir, "BEAN_NOW=", "TMPDIR="+tmp,
				"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				if m, _ := filepath.Glob(filepath.Join(tmp, "bean-import-*", "out")); len(m) > 0 || time.Now().After(deadline) {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			cmd.Process.Signal(s)
			start := time.Now()
			cmd.Wait()
			if time.Since(start) > 5*time.Second {
				t.Error("bean waited for the importer")
			}
			if left, _ := filepath.Glob(filepath.Join(tmp, "bean-import-*")); len(left) > 0 {
				t.Errorf("left after %s: %v", s, left)
			}
		})
	}
}

// The importer's output goes to a bounded private file, not to memory.
func TestImporterOutputCapped(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bean-import-big")
	os.WriteFile(bin, []byte("#!/bin/sh\nhead -c 3000000 /dev/zero\n"), 0o755)
	dir := t.TempDir()
	old := importerOutputLimit
	importerOutputLimit = 1 << 20
	defer func() { importerOutputLimit = old }()
	if _, _, err := runImporter(bin, "sha256:"+strings.Repeat("0", 64), nil, "x", dir); err == nil || !strings.Contains(err.Error(), "more than") {
		t.Errorf("err %v", err)
	}
	if info, err := os.Stat(filepath.Join(dir, "out")); err != nil || info.Size() > 1<<20 {
		t.Errorf("spool: %v %v", info, err)
	}
}

// A reading that -corrections cannot write either says why, and does not
// send the person back to -corrections.
func TestImportChangedSaysWhy(t *testing.T) {
	c := core.Changed{ID: "sha256:" + strings.Repeat("a", 64), Type: "lab.glucose", Old: "90", New: "120"}
	if msg := reportChanged([]core.Changed{c}, "run bean import csv -corrections").Error(); !strings.Contains(msg, "bean import csv -corrections") {
		t.Errorf("without a reason: %s", msg)
	}
	c.NewType, c.Reason = "lab.ldl", "the type differs"
	if msg := reportChanged([]core.Changed{c}, "run bean import csv -corrections").Error(); strings.Contains(msg, "then run") || !strings.Contains(msg, "cannot write them either") {
		t.Errorf("with a reason: %s", msg)
	}
}

// A signal stops the importer's whole process group: a child it started,
// as the Labcorp importer starts pdftotext, does not run on.
func TestImportSignalKillsImporterGroup(t *testing.T) {
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "bean-import-forks"), []byte("#!/bin/sh\nsleep 30 &\necho $! > \"$CHILD_PID\"\nwait\n"), 0o755)
	dir := t.TempDir()
	run(t, dir, "", "init")
	pidFile := filepath.Join(t.TempDir(), "child")
	f := filepath.Join(t.TempDir(), "report.txt")
	os.WriteFile(f, []byte("SYNTHETIC glucose 101\n"), 0o600)
	cmd := exec.Command(os.Args[0], "import", "forks", f)
	cmd.Env = append(os.Environ(), "BEAN_TEST_MAIN=1", "BEAN_DIR="+dir, "BEAN_NOW=", "TMPDIR="+t.TempDir(), "CHILD_PID="+pidFile,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := 0
	for deadline := time.Now().Add(5 * time.Second); pid == 0 && time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if b, _ := os.ReadFile(pidFile); strings.HasSuffix(string(b), "\n") {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
	}
	if pid <= 0 {
		cmd.Process.Kill()
		t.Fatal("the importer did not start its child")
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	cmd.Process.Signal(syscall.SIGTERM)
	cmd.Wait()
	if c := cmd.ProcessState.ExitCode(); c != 128+int(syscall.SIGTERM) {
		t.Errorf("exit %d", c)
	}
	for deadline := time.Now().Add(3 * time.Second); syscall.Kill(pid, 0) != syscall.ESRCH; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the importer's child runs on after bean was stopped")
		}
	}
}

// The real-layout fixture (a real report's columns, synthetic content)
// imports whole through bean import, and again adds nothing.
func TestCLIImportRealLayout(t *testing.T) {
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	report := "../bean-import-labcorp/testdata/real-layout.txt"
	out, errs, code := runPath(t, dir, path, "import", "labcorp", "-tz", "America/Chicago", report)
	if code != 0 || !strings.Contains(out, "added 71, skipped 0") || !strings.Contains(out, "71 results, 2 without a value") {
		t.Fatalf("import: %d\n%s\n%s", code, out, errs)
	}
	if out, _, code := runPath(t, dir, path, "import", "labcorp", "-tz", "America/Chicago", report); code != 0 || !strings.Contains(out, "added 0, skipped 71") {
		t.Errorf("retry: %d %s", code, out)
	}
	if out, _, code := run(t, dir, "", "verify"); code != 0 || !strings.Contains(out, "73 valid, 0 invalid") {
		t.Errorf("verify: %d %s", code, out)
	}
}
