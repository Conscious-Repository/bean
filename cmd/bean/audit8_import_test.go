package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Regression tests from audit 8. Synthetic data only. Each fails when the attack works.

var blobRe = regexp.MustCompile(`sha256:[0-9a-f]{64}`)

// A purged csv.file comes back when the same file is imported with another
// -system: the archive ref includes the system, so InputPurged says no.
func TestImportRefusesPurgedFileUnderOtherSystem(t *testing.T) {
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	src := filepath.Join(t.TempDir(), "hrv.csv")
	os.WriteFile(src, []byte("time,value\n2026-10-20T03:00:00Z,42.5\n"), 0o600)
	out, errs, code := runPath(t, dir, path, "import", "csv", "-type", "hrv.rmssd", src)
	if code != 0 {
		t.Fatalf("import: %d %s %s", code, out, errs)
	}
	bid := blobRe.FindString(out)
	js, _, _ := run(t, dir, "", "ls", "-type", "csv.file", "-json")
	for _, id := range recordIDs(js) {
		o, _, _ := run(t, dir, "", "purge", id)
		t.Logf("purge: %s", strings.TrimSpace(o))
	}
	if _, _, code := run(t, dir, "", "blob", "get", bid); code == 0 {
		t.Fatal("purge did not erase")
	}
	out, errs, code = runPath(t, dir, path, "import", "csv", "-type", "hrv.rmssd", "-system", "csv-2", src)
	t.Logf("reimport -system csv-2: exit %d\n%s%s", code, out, errs)
	if got, _, code := run(t, dir, "", "blob", "get", bid); code == 0 {
		t.Errorf("the purged file is stored and kept again (%d bytes readable)", len(got))
	}
}

// An importer whose first record carries source.ref (the finished ref) in
// place of ref_input: InputPurged sees no ref, PutBlob stores the purged
// file, Append skips the line as purged, and the error does not say that an
// encrypted copy of the deleted file is now on disk again.
func TestImportRefusesArchiveWithoutRefInput(t *testing.T) {
	dir, report, path, rec := importedReport(t)
	bid := rec["value"].(map[string]any)["blob"].(string)
	src := rec["source"].(map[string]any)
	line := map[string]any{
		"type": rec["type"], "value": rec["value"], "times": map[string]any{"observed": nil},
		"context": rec["context"], "blobs": rec["blobs"],
		"source": map[string]any{"system": src["system"], "adapter": src["adapter"], "ref": src["ref"]},
	}
	b, _ := json.Marshal(line)
	run(t, dir, "", "purge", rec["id"].(string))
	if _, _, code := run(t, dir, "", "blob", "get", bid); code == 0 {
		t.Fatal("purge did not erase")
	}
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "bean-import-fake"), []byte("#!/bin/sh\ncat <<'EOF'\n"+string(b)+"\nEOF\n"), 0o755)
	_, errs, code := runPath(t, dir, bin+string(os.PathListSeparator)+path, "import", "fake", report)
	t.Logf("exit %d: %s", code, strings.TrimSpace(errs))
	if got, _, code := run(t, dir, "", "blob", "get", bid); code == 0 {
		t.Errorf("the purged file is on disk again, %d bytes readable by blob get", len(got))
	}
}

// Every signal that stops bean import removes the plaintext copy, and the
// exit code is 128 plus the signal number: SIGHUP (terminal closed, ssh
// dropped) and SIGQUIT were not trapped, and SIGTERM exited 130.
func TestImportSignalsRemoveCopy(t *testing.T) {
	for _, s := range []syscall.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM} {
		t.Run(s.String(), func(t *testing.T) {
			bin := t.TempDir()
			os.WriteFile(filepath.Join(bin, "bean-import-slow"), []byte("#!/bin/sh\nsleep 30\n"), 0o755)
			dir := t.TempDir()
			run(t, dir, "", "init")
			tmp := t.TempDir()
			f := filepath.Join(t.TempDir(), "report.txt")
			os.WriteFile(f, []byte("SYNTHETIC glucose 101\n"), 0o600)
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
			cmd.Wait()
			left, _ := filepath.Glob(filepath.Join(tmp, "bean-import-*", "source"))
			for _, p := range left {
				data, _ := os.ReadFile(p)
				t.Errorf("plaintext copy left after %s: %s = %q", s, p, data)
			}
			if got := cmd.ProcessState.ExitCode(); got != 128+int(s) {
				t.Errorf("%s exits %d (%s), want %d", s, got, cmd.ProcessState, 128+int(s))
			}
		})
	}
}

// The importer's last stderr line goes to stdout only when the import
// worked, made safe and named after the importer: an escape and a forged
// "kept" line do not reach the terminal as they are.
func TestImporterNoteSanitized(t *testing.T) {
	path := importerOnPath(t)
	bin := t.TempDir()
	forged := "printf 'kept lab.report sha256:0000\\033[2K\\n' >&2\n"
	os.WriteFile(filepath.Join(bin, "bean-import-noisy"), []byte("#!/bin/sh\n"+forged+"echo '{}'\n"), 0o755)
	os.WriteFile(filepath.Join(bin, "bean-import-wrap"), []byte("#!/bin/sh\nbean-import-labcorp \"$@\" || exit 1\n"+forged), 0o755)
	dir := t.TempDir()
	run(t, dir, "", "init")
	f := filepath.Join(t.TempDir(), "f.txt")
	os.WriteFile(f, []byte("x"), 0o600)
	out, errs, code := runPath(t, dir, bin+string(os.PathListSeparator)+path, "import", "noisy", f)
	if code == 0 || strings.Contains(out, "kept") || strings.Contains(out+errs, "\x1b") {
		t.Errorf("failed import: exit %d stdout %q stderr %q", code, out, errs)
	}
	report := filepath.Join(t.TempDir(), "report.txt")
	os.WriteFile(report, []byte(labReport("101")), 0o600)
	out, errs, code = runPath(t, dir, bin+string(os.PathListSeparator)+path, "import", "wrap", report)
	if code != 0 {
		t.Fatalf("import: %d %s %s", code, out, errs)
	}
	if strings.Contains(out, "\x1b") || !strings.Contains(out, "\nbean-import-wrap: kept lab.report sha256:0000\\x1b[2K\n") {
		t.Errorf("note not made safe and named: %q", out)
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "kept lab.report sha256:0000") {
			t.Errorf("forged line on stdout: %q", l)
		}
	}
}
