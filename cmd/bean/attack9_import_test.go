package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Attack 9. Synthetic data only. Each test fails when the attack works.

// The archive line passes bean import's own checks (blob, ref_input, purge)
// but Append refuses it (source.system over 64 bytes). By then PutBlob has
// stored the file. No record names it, gc never erases a blob that no record
// named, and export decrypts it as unreferenced. The error says only that
// the record was not added.
func TestAttack9ArchiveRefusedAfterStore(t *testing.T) {
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	src := filepath.Join(t.TempDir(), "hrv.csv")
	os.WriteFile(src, []byte("time,value\n2026-10-20T03:00:00Z,42.5\n"), 0o600)
	before := countFiles(t, filepath.Join(dir, "blobs"))
	out, errs, code := runPath(t, dir, path, "import", "csv", "-type", "hrv.rmssd", "-system", strings.Repeat("s", 65), src)
	t.Logf("exit %d\nstdout: %sstderr: %s", code, out, errs)
	after := countFiles(t, filepath.Join(dir, "blobs"))
	o, e, _ := run(t, dir, "", "gc")
	t.Logf("gc: %s%s", o, e)
	afterGC := countFiles(t, filepath.Join(dir, "blobs"))
	x := filepath.Join(t.TempDir(), "x")
	run(t, dir, "", "export", x)
	man, _ := os.ReadFile(filepath.Join(x, "manifest.json"))
	t.Logf("blobs %d -> %d, after gc %d; manifest unreferenced: %v", before, after, afterGC, strings.Contains(string(man), "unreferenced_blobs\":[\"sha256"))
	if after > before && !strings.Contains(errs, "encrypted copy") {
		t.Errorf("an encrypted copy of the file is stored, no record names it, and the error does not say so")
	}
	if afterGC > before {
		t.Errorf("gc does not erase it either: it stays for good")
	}
}

// Ctrl-C reaches the whole foreground process group: the importer dies of
// SIGINT too. Then the main goroutine and the signal goroutine race: when
// the main one wins, bean exits 1 with a "not accepted" error, not 130.
func TestAttack9CtrlCRaceExitCode(t *testing.T) {
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "bean-import-slow"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o755)
	dir := t.TempDir()
	run(t, dir, "", "init")
	f := filepath.Join(t.TempDir(), "report.txt")
	os.WriteFile(f, []byte("SYNTHETIC glucose 101\n"), 0o600)
	codes := map[int]int{}
	var sample string
	for i := 0; i < 30; i++ {
		tmp := t.TempDir()
		cmd := exec.Command(os.Args[0], "import", "slow", f)
		cmd.Env = append(os.Environ(), "BEAN_TEST_MAIN=1", "BEAN_DIR="+dir, "BEAN_NOW=", "TMPDIR="+tmp,
			"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		var errb strings.Builder
		cmd.Stderr = &errb
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			if m, _ := filepath.Glob(filepath.Join(tmp, "bean-import-*", "out")); len(m) > 0 || time.Now().After(deadline) {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		time.Sleep(20 * time.Millisecond)
		syscall.Kill(-cmd.Process.Pid, syscall.SIGINT) // as a terminal sends Ctrl-C
		cmd.Wait()
		c := cmd.ProcessState.ExitCode()
		codes[c]++
		if c != 130 {
			sample = errb.String()
		}
		if left, _ := filepath.Glob(filepath.Join(tmp, "bean-import-*", "source")); len(left) > 0 {
			t.Errorf("plaintext left: %v", left)
		}
	}
	t.Logf("exit codes over 30 runs: %v", codes)
	if codes[130] != 30 {
		t.Errorf("Ctrl-C did not exit 130 every time: %v; stderr: %q", codes, sample)
	}
}
