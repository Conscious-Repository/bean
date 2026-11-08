package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCollectionDSTDoesNotInventInstant(t *testing.T) {
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	for _, collected := range []string{"03/09/2025 0230", "11/02/2025 0130"} {
		t.Run(collected, func(t *testing.T) {
			rep := Report{Collected: collected}
			observed, raw := rep.observed(loc)
			if observed["precision"] != "day" || raw != collected {
				t.Fatalf("invented precise instant: %v raw=%q", observed, raw)
			}
		})
	}
}

func TestShortPendingCommentIsNotAResult(t *testing.T) {
	text := strings.Join([]string{"Specimen ID: SYNTHETIC-A", "Date Collected: 01/02/2025", "Basic Panel", header(), "      Glucose", comment("Test not performed.")}, "\n")
	rep, err := parseReport(text)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Results[0].Value != "" || strings.Join(rep.Results[0].Comment, " ") != "Test not performed." {
		t.Fatalf("comment became value: %+v", rep.Results[0])
	}
}

// A long reference interval is one token and is kept. A free-text result is
// not a value the grammar knows (DECISIONS 222): it is refused, not lost.
func TestLongAlignedResultsAreNotSilentlyLost(t *testing.T) {
	ref := "Negative: less than 10; Positive: greater than 20"
	report := func(value string) string {
		return strings.Join([]string{"Specimen ID: SYNTHETIC-A", "Date Collected: 01/02/2025", "Basic Panel", header(), row("Antibody Screen", "", value, "", "", ref)}, "\n")
	}
	rep, err := parseReport(report("Not Detected"))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Results[0].Value != "Not Detected" || rep.Results[0].Ref != ref {
		t.Fatalf("aligned text lost: %+v", rep.Results[0])
	}
	if _, err := parseReport(report("No clinically significant antibodies detected")); err == nil || !strings.Contains(err.Error(), "not a number or a result") {
		t.Fatalf("free-text result: %v", err)
	}
}

func TestAmbiguousInlineColumnsFailInsteadOfNull(t *testing.T) {
	line := pad("      Glucose", 57) + "100"
	line = pad(line, 68) + "High"
	line = pad(line, 75) + "unexpected"
	text := strings.Join([]string{"Specimen ID: SYNTHETIC-A", "Date Collected: 01/02/2025", "Basic Panel", header(), line}, "\n")
	if _, err := parseReport(text); err == nil {
		t.Fatal("ambiguous inline columns silently became absent result")
	}
}

func TestUnderflowRemainsText(t *testing.T) {
	value := "0." + strings.Repeat("0", 400) + "1"
	rep := Report{Collected: "01/02/2025", Specimen: "SYNTHETIC-A", Results: []*Result{{Name: "Synthetic", Value: value}}}
	got := rep.records("", nil)[0]["value"].(map[string]any)
	if got["s"] != value {
		t.Fatal("nonzero underflow became numeric zero")
	}
}

func TestPDFDescendantCannotHoldExtractionOpen(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "source.pdf")
	tool := filepath.Join(dir, "pdftotext")
	if err := os.WriteFile(input, []byte("%PDF-SYNTHETIC"), 0600); err != nil {
		t.Fatal(err)
	}
	// The direct child exits, but its descendant retains the extraction pipes.
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nsleep 4 &\nprintf 'synthetic'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	start := time.Now()
	_, err := readText(input)
	if err == nil {
		t.Fatal("accepted extraction with inherited pipe left open")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("descendant defeated bounded extraction wait")
	}
}

func TestCLITextSourceBlobMedia(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(path, []byte(fixture()), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".", "-blob", "sha256:"+strings.Repeat("a", 64), path)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(out), []byte{'\n'}) {
		var row map[string]any
		if err := json.Unmarshal(line, &row); err != nil {
			t.Fatal(err)
		}
		blobs, _ := row["blobs"].([]any)
		// Only the lab.report lists the file, so purging it erases the file.
		if (row["type"] == "lab.report") != (len(blobs) == 1) {
			t.Fatalf("%s lists blobs %v", row["type"], blobs)
		}
		for _, blob := range blobs {
			if blob.(map[string]any)["media_type"] != "text/plain" {
				t.Fatalf("text mislabeled in %s", row["type"])
			}
		}
	}
}

func TestCompressedPDFColumnsPreserveUnits(t *testing.T) {
	text := "Specimen ID: SYNTHETIC-PDF\nDate Collected: 01/02/2025 0815 Local\nBasic Panel\n" +
		"      Test                              Current Result and Flag   Previous Result and Date   Units    Reference Interval\n" +
		"      Glucose                            101                                                  mg/dL     70-99\n"
	rep, err := parseReport(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Results) != 1 || rep.Results[0].Units != "mg/dL" || rep.Results[0].Ref != "70-99" {
		t.Fatalf("compressed columns lost units: %+v", rep.Results)
	}
}

func TestReportResultCountBound(t *testing.T) {
	prefix := "Specimen ID: SYNTHETIC-A\nDate Collected: 01/02/2025\nBasic Panel\n" + header() + "\n"
	for _, count := range []int{10000, 10001} {
		var b strings.Builder
		b.WriteString(prefix)
		for i, n := 0, 0; n < count; i++ {
			if wordFlags[letters(i)] {
				continue // "Synthetic low" is refused: a flag in a name
			}
			b.WriteString(row("Synthetic "+letters(i), "", "1", "", "", "") + "\n")
			n++
		}
		text := b.String()
		rep, err := parseReport(text)
		if count == 10000 {
			if err != nil || len(rep.Results) != count {
				t.Fatalf("valid boundary rejected: %v", err)
			}
		} else if err == nil {
			t.Fatal("unbounded result allocations accepted")
		}
	}
}

func TestReportLineCountBound(t *testing.T) {
	base := "Specimen ID: SYNTHETIC-A\nDate Collected: 01/02/2025\nBasic Panel\n" + header() + "\n" + row("Synthetic", "", "1", "", "", "")
	text := base + strings.Repeat("\n", 100000-(strings.Count(base, "\n")+1))
	if _, err := parseReport(text); err != nil {
		t.Fatalf("valid line boundary rejected: %v", err)
	}
	if _, err := parseReport(text + "\n"); err == nil {
		t.Fatal("unbounded line splitting accepted")
	}
}

func TestUnrepresentableCollectionDateRemainsRaw(t *testing.T) {
	for _, loc := range []*time.Location{nil, time.UTC} {
		rep := Report{Collected: "01/02/0000 0815"}
		observed, raw := rep.observed(loc)
		if observed != nil || raw != rep.Collected {
			t.Fatalf("invalid Bean time emitted: %v", observed)
		}
	}
}
