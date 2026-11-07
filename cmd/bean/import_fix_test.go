package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Conscious-Repository/bean/core"
)

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

// After -corrections, a reading still reported changed has another type or
// no value; the message does not send the person back to -corrections.
func TestChangedMessageAfterCorrections(t *testing.T) {
	if msg := changedError(2, "csv", false).Error(); !strings.Contains(msg, "bean import csv -corrections") {
		t.Errorf("without -corrections: %s", msg)
	}
	msg := changedError(2, "csv", true).Error()
	if strings.Contains(msg, "-corrections") || !strings.Contains(msg, "different type or no value") || !strings.Contains(msg, "own -system") {
		t.Errorf("with -corrections: %s", msg)
	}
}
