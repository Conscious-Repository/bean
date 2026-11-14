package main

import (
	"os"
	"strings"
	"testing"
)

// TestMeasure: -count says how far each reader gets on a report, in counts
// and line numbers, and shows no word of the report.
func TestMeasure(t *testing.T) {
	raw, err := os.ReadFile("testdata/real-layout.txt")
	if err != nil {
		t.Fatal(err)
	}
	known := string(raw)
	rep, err := readReport(known, raw)
	if err != nil {
		t.Fatal(err)
	}
	want := len(rep.Results)
	lines := strings.Join(measure(known, raw), "\n")
	if !strings.Contains(lines, "as an import reads it: "+itoa(want)+" results") || !strings.Contains(lines, ", 0 lines passed over, "+itoa(want)+" results, read to the end") {
		t.Errorf("a layout that the importer knows:\n%s", lines)
	}
	// The same report under two lines that the grammar does not know: one
	// with several fields, as a real report of another year has.
	unknown := "Zebedee Q. Xylophone\nDOB: 01/02/1990          Age: 36        Specimen: ZZ123456Q\n" + known
	if _, err := readReport(unknown, []byte(unknown)); err == nil {
		t.Fatal("setup: the changed report was read")
	}
	lines = strings.Join(measure(unknown, []byte(unknown)), "\n")
	if !strings.Contains(lines, "as an import reads it: no result") || !strings.Contains(lines, "column header found, 1 lines passed over, "+itoa(want)+" results, read to the end") {
		t.Errorf("a layout with lines that the importer does not know:\n%s", lines)
	}
	for _, word := range []string{"Zebedee", "Xylophone", "ZZ123456Q", "1990", "Glucose", "Sodium"} {
		if strings.Contains(lines, word) {
			t.Errorf("the count shows %q of the report:\n%s", word, lines)
		}
	}
	// A refusal that quotes the report is masked.
	if got := masked(`line 4 after "Glucose" ends in a reading ("101"), type lab.glucose-101`); got != `line 4 after "xxxxxxx" ends in a reading ("999"), type lab.xxx` {
		t.Errorf("masked: %s", got)
	}
	// An import is not changed by the measuring mode: the same report is
	// refused as before.
	if _, err := parseReport(unknown); err == nil {
		t.Error("an import passes over lines that it does not know")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for ; n > 0; n /= 10 {
		s = string(rune('0'+n%10)) + s
	}
	return s
}
