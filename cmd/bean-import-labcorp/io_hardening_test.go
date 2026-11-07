package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReadTextRejectsOversizedInput(t *testing.T) {
	p := filepath.Join(t.TempDir(), "large.txt")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate((32 << 20) + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err = readText(p); err == nil {
		t.Fatal("oversized report accepted")
	}
}

func TestPDFUsesStagedBytesAndHidesConverterErrors(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "source.pdf")
	os.WriteFile(input, []byte("%PDF-FAKE"), 0600)
	script := `#!/bin/sh
# Original path must not be reopened: only a private snapshot is permitted.
if [ "$4" = "$ORIGINAL_PDF" ]; then echo 'PRIVATE-CONVERTER-DETAIL' >&2; exit 1; fi
cat "$SYNTHETIC_TEXT"
`
	tool := filepath.Join(dir, "pdftotext")
	os.WriteFile(tool, []byte(script), 0700)
	text := filepath.Join(dir, "report.txt")
	os.WriteFile(text, []byte(fixture()), 0600)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ORIGINAL_PDF", input)
	t.Setenv("SYNTHETIC_TEXT", text)
	got, err := readText(input)
	if err != nil || got != fixture() {
		t.Fatalf("snapshot not used: %v", err)
	}
	os.WriteFile(tool, []byte("#!/bin/sh\necho 'PRIVATE-CONVERTER-DETAIL' >&2\nexit 1\n"), 0700)
	_, err = readText(input)
	if err == nil || strings.Contains(err.Error(), "PRIVATE-CONVERTER-DETAIL") {
		t.Fatal("converter details leaked or error ignored")
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestReportOutputFailure(t *testing.T) {
	rep, err := parseReport(fixture())
	if err != nil {
		t.Fatal(err)
	}
	if err = writeRecords(brokenWriter{}, rep.records("", nil)); err == nil {
		t.Fatal("output failure ignored")
	}
	var out bytes.Buffer
	if err = writeRecords(&out, rep.records("", nil)); err != nil || out.Len() == 0 {
		t.Fatalf("write: %v", err)
	}
}

func TestExtractedTextBound(t *testing.T) {
	b := &boundedText{limit: 4}
	if _, err := b.Write([]byte("1234")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte("5")); err == nil || b.Len() != 4 {
		t.Fatal("output bound not enforced")
	}
}

func TestInvalidResultNameEmitsNoArchivePrefix(t *testing.T) {
	var out bytes.Buffer
	records := []map[string]any{archiveRecord([]byte("synthetic"), "sha256:"+strings.Repeat("a", 64)), {"type": "lab.", "value": map[string]any{"n": 1}}}
	if err := writeRecords(&out, records); err == nil || out.Len() != 0 {
		t.Fatal("invalid result left importable prefix")
	}
}

func TestReportNotesDoNotMultiplyAllocation(t *testing.T) {
	rep := &Report{Collected: "01/02/2025", Specimen: "synthetic", Notes: []string{strings.Repeat("x", 64<<10), strings.Repeat("y", 64<<10)}}
	for i := 0; i < 200; i++ {
		rep.Results = append(rep.Results, &Result{Name: "Synthetic", Value: "1"})
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	rows := rep.records("", nil)
	runtime.ReadMemStats(&after)
	if len(rows) != 200 || rows[199]["context"].(map[string]any)["report_note"] != strings.Join(rep.Notes, " ") {
		t.Fatal("report notes lost")
	}
	runtime.KeepAlive(rows)
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 8<<20 {
		t.Fatalf("report notes amplified allocations: %d bytes", allocated)
	}
}
