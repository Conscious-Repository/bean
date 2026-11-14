package main

import (
	"os"
	"path/filepath"
	"regexp"
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
	if !strings.Contains(lines, "looks like a lab report: yes") {
		t.Errorf("the count does not say that the file would be kept:\n%s", lines)
	}
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

// TestRefusalsQuoteTheReport: -count and an unread import show a refusal
// with what it quotes masked. So every refusal puts what it takes from the
// report between quotation marks (%q). A format with %s or %v is on this
// list only when what it prints is the importer's own: a place, a fixed
// word, an inner refusal, or a type that masked hides.
func TestRefusalsQuoteTheReport(t *testing.T) {
	own := map[string]bool{
		"line %d: unsupported result column header: it must name %s, in that order":              true, // the importer's column names
		"line %d at the margin may be a row with its value in the name, not a panel heading: %v": true, // an inner refusal
		"line %d: %v": true, // an inner refusal
		"%s: %v":      true, // a place, and an inner refusal
		"%s: a row with no value and no rows under it: a test with no value is refused unless it says %s":   true, // the importer's own words
		"%s: result %q says %s, and no note follows it":                                                     true, // the importer's own words
		"two results, %q and %q, map to one type lab.%s":                                                    true, // masked hides lab.<name>
		"result %q would have the type of the kept report, %s":                                              true, // the fixed type
		"-tz %s is %s at the collection time, but the report prints %q (%s); leave -tz out for this report": true, // the person's flag, and offsets
		"page %d, %s: two words are printed over each other, so the line cannot be read for sure":           true, // a place
	}
	format := regexp.MustCompile(`Errorf\("((?:[^"\\]|\\.)*)"`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range format.FindAllStringSubmatch(string(src), -1) {
			f := m[1]
			rest := strings.TrimPrefix(f, "%s: ") // where: a page and a place on it
			if (strings.Contains(rest, "%s") || strings.Contains(rest, "%v")) && !own[f] {
				t.Errorf("%s: a refusal prints with %%s or %%v what may be the report's text: %q", name, f)
			}
		}
	}
	// A long quote is cut, and what is left is masked.
	if got := masked(`a word "` + strings.Repeat("Glucose9", 40) + `" too long`); got != `a word "xxxxxxx9xxxxxxx9xxxx..." too long` {
		t.Errorf("a long quote: %s", got)
	}
}

// TestLooksLikeReport: a file that cannot be read is kept as a lab report
// only when it has a line of column names, or the marks of a Quest report.
func TestLooksLikeReport(t *testing.T) {
	raw, err := os.ReadFile("testdata/real-layout.txt")
	if err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]bool{
		string(raw): true,
		"   Test Name                In Range    Out Of Range    Reference Range    Lab\n":    true,
		"TESTS                RESULT      FLAG      UNITS      REFERENCE INTERVAL      LAB\n": true,
		questTrademark + "\n":                      true,
		"hello, this is my grocery list\nmilk 2\n": false,
		"": false,
		"the test went well and the weather was in range": false,
		// One kind of column name is not a line of column names.
		"Test          Name          Date          Place\n": false,
		"Test          Result\n":                            true,
	} {
		if got := looksLikeReport(text); got != want {
			t.Errorf("looksLikeReport = %v, want %v, for %.60q", got, want, text)
		}
	}
}
