package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Ported from PR 17's tools/apple-health/test_ingest.py.

const appleXML = `<?xml version="1.0"?><HealthData locale="en_US"><Me HKCharacteristicTypeIdentifierDateOfBirth="1900-01-01"/><Record type="HKQuantityTypeIdentifierStepCount" sourceName="Private Name" device="Private Device" unit="count" creationDate="2026-01-01 10:00:00 -0600" startDate="2026-01-01 09:00:00 -0600" endDate="2026-01-01 09:05:00 -0600" value="25"/></HealthData>`

func appleZip(t *testing.T, path, xml string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, _ := z.Create("apple_health_export/export.xml")
	w.Write([]byte(xml))
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, b.Bytes(), 0o600)
	return b.Bytes()
}

// The export is kept, a retry adds nothing, and bean export gives back the
// exact file without the importer.
func TestCLIImportAppleKeepsExport(t *testing.T) {
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	src := filepath.Join(t.TempDir(), "export.zip")
	original := appleZip(t, src, appleXML)
	out, errs, code := runPath(t, dir, path, "import", "apple-health", src)
	if code != 0 || !strings.Contains(out, "kept apple-health.export") || !strings.Contains(out, "added 0, skipped 0") {
		t.Fatalf("import: %d\n%s\n%s", code, out, errs)
	}
	if out, _, code := runPath(t, dir, path, "import", "apple-health", src); code != 0 || !strings.Contains(out, "already kept") {
		t.Errorf("retry: %d %s", code, out)
	}
	js, _, _ := run(t, dir, "", "ls", "-type", "apple-health.export", "-json")
	if strings.Contains(js, "Private Name") || strings.Count(strings.TrimSpace(js), "\n") != 0 {
		t.Errorf("archive: %s", js)
	}
	exp := filepath.Join(t.TempDir(), "out")
	if _, errs, code := run(t, dir, "", "export", exp); code != 0 {
		t.Fatalf("export: %d %s", code, errs)
	}
	os.Remove(src)
	found := 0
	filepath.Walk(filepath.Join(exp, "blobs"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			found++
			if b, _ := os.ReadFile(p); !bytes.Equal(b, original) {
				t.Errorf("exported blob differs")
			}
		}
		return nil
	})
	if found != 1 {
		t.Errorf("exported blobs: %d", found)
	}
}

// Readings retry without duplicates; a changed export is kept as its own
// file while its unchanged reading dedupes.
func TestCLIImportAppleRecords(t *testing.T) {
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	src := filepath.Join(t.TempDir(), "export.zip")
	appleZip(t, src, appleXML)
	if out, errs, code := runPath(t, dir, path, "import", "apple-health", "-records", src); code != 0 || !strings.Contains(out, "added 1, skipped 0") {
		t.Fatalf("records: %d %s %s", code, out, errs)
	}
	if out, _, _ := runPath(t, dir, path, "import", "apple-health", "-records", src); !strings.Contains(out, "added 0, skipped 1") {
		t.Errorf("retry: %s", out)
	}
	appleZip(t, src, strings.Replace(appleXML, "en_US", "en_GB", 1))
	out, errs, code := runPath(t, dir, path, "import", "apple-health", "-records", src)
	if code != 0 || !strings.Contains(out, "kept apple-health.export") || strings.Contains(out, "already kept") || !strings.Contains(out, "added 0, skipped 1") {
		t.Errorf("changed export: %d %s %s", code, out, errs)
	}
	if ls, _, _ := run(t, dir, "", "ls", "-type", "apple-health.steps"); strings.Count(ls, "apple-health.steps") != 1 {
		t.Errorf("steps:\n%s", ls)
	}
	if out, _, code := run(t, dir, "", "verify"); code != 0 {
		t.Errorf("verify: %d %s", code, out)
	}
}

// A malformed export writes no record and no blob.
func TestCLIImportAppleMalformed(t *testing.T) {
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	before := countFiles(t, dir)
	src := filepath.Join(t.TempDir(), "export.xml")
	os.WriteFile(src, []byte("<HealthData><Record"), 0o600)
	if _, errs, code := runPath(t, dir, path, "import", "apple-health", src); code != 1 || !strings.Contains(errs, "nothing was written") || strings.Contains(errs, "<HealthData") {
		t.Errorf("malformed: %d %s", code, errs)
	}
	if after := countFiles(t, dir); after != before {
		t.Errorf("files %d -> %d", before, after)
	}
}
