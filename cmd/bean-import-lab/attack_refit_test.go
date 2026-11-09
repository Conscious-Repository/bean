package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// Attack on the refit of DECISIONS 230. Synthetic data only: each test
// mutates testdata/real-layout.txt and fails when a wrong reading is accepted.

func realLines(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile("testdata/real-layout.txt")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(string(b), "\n")
}

// set replaces line n (from 1) with the given lines.
func set(ls []string, n int, with ...string) []string {
	out := append([]string{}, ls[:n-1]...)
	out = append(out, with...)
	return append(out, ls[n:]...)
}

func at(cols ...any) string {
	l := ""
	for i := 0; i < len(cols); i += 2 {
		l = pad(l, cols[i].(int)) + cols[i+1].(string)
	}
	return strings.TrimRight(l, " ")
}

func find(rep *Report, name string) *Result {
	for _, r := range rep.Results {
		if r.Name == name {
			return r
		}
	}
	return nil
}

// R1. The rest of a row that has a value, on the line under it, starts at
// the flag column, so it is a comment: the flag, unit and range are lost.
// Line 22: "Hematocrit 01  00.4  High  %  37.5-51.0".
func TestRefitRestOfRowBecomesComment(t *testing.T) {
	cases := map[string][]string{
		"flag alone under": {
			at(6, "Hematocrit 01", 57, "00.4", 141, "%", 163, "37.5-51.0"),
			at(76, "High"),
		},
		"flag, unit, range under": {
			at(6, "Hematocrit 01", 57, "00.4"),
			at(76, "High", 141, "%", 163, "37.5-51.0"),
		},
		"previous, unit, range under": {
			at(6, "Hematocrit 01", 57, "00.4", 76, "High"),
			at(97, "41.2", 104, "01/02/2024", 141, "%", 163, "37.5-51.0"),
		},
	}
	for name, lines := range cases {
		rep, err := parseReport(strings.Join(set(realLines(t), 22, lines...), "\n"))
		if err != nil {
			t.Logf("%s: refused: %v", name, err)
			continue
		}
		r := find(rep, "Hematocrit")
		t.Logf("%s: accepted value=%q flag=%q units=%q ref=%q prev=%q comment=%q", name, r.Value, r.Flag, r.Units, r.Ref, r.Previous, r.Comment)
		if r.Flag != "High" || r.Units != "%" || r.Ref != "37.5-51.0" {
			t.Errorf("%s: Hematocrit stored without its flag, unit or range", name)
		}
	}
}

// R2. A unit that holds a digit, in the reference band of a row with no
// range, is stored as the range; the number has no unit.
func TestRefitUnitInRefBand(t *testing.T) {
	// Line 41: Eos (Absolute), units x10E3/uL at 140; drop its range and
	// move the unit right of where units and ranges meet (152).
	ls := set(realLines(t), 41, at(6, "Eos (Absolute) 01", 56, "3.6", 154, "x10E3/uL"))
	rep, err := parseReport(strings.Join(ls, "\n"))
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	r := find(rep, "Eos (Absolute)")
	rec := rep.records("", nil)
	for _, x := range rec {
		if x["context"].(map[string]any)["test"] == "Eos (Absolute)" {
			t.Logf("accepted value=%v ctx=%v", x["value"], x["context"])
		}
	}
	if r.Units != "x10E3/uL" {
		t.Errorf("unit %q stored as the reference interval; units %q", r.Ref, r.Units)
	}
}

// R3. The note that the collection time was not on the requisition, when
// the phrase wraps across two lines, is not seen: the printed time is
// stored to the minute.
func TestRefitCollectionNoteWrapped(t *testing.T) {
	ls := realLines(t)
	// Lines 322-325: the four lines of the report's "Please note".
	ls[321] = comment("Specimen received without a collection time. The date and/or time of collection")
	ls[322] = comment("was not indicated on the requisition as required by state and federal law.")
	rep, err := parseReport(strings.Join(ls, "\n"))
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	loc, _ := time.LoadLocation("America/Chicago")
	obs, raw := rep.observed(loc)
	t.Logf("notes=%q observed=%v raw=%q", rep.Notes, obs, raw)
	if obs["precision"] != "day" {
		t.Errorf("collection time stored to the minute although the report says it was not indicated")
	}
}

// R3b. The same note as a "Please Note:" row in a result's panel is that
// result's comment, not a report note: the rule never sees it.
func TestRefitCollectionNoteInPanel(t *testing.T) {
	ls := realLines(t)
	// Ferritin panel, line 316: add a Please Note row with the note.
	ls = set(ls, 316, ls[315],
		at(6, "Please Note:", 15, "01"),
		comment("The date and/or time of collection was not indicated on the"),
		comment("requisition as required by state and federal law."),
	)
	rep, err := parseReport(strings.Join(ls, "\n"))
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	loc, _ := time.LoadLocation("America/Chicago")
	obs, _ := rep.observed(loc)
	t.Logf("ferritin comment=%q observed=%v", find(rep, "Ferritin").Comment, obs)
	if obs["precision"] != "day" {
		t.Errorf("collection time stored to the minute although the report says it was not indicated")
	}
}

// R4. The Ordered Items list runs on through a field line until a blank
// line, so a panel heading right after it is read as an ordered item.
func TestRefitOrderedItemsSwallowsPanel(t *testing.T) {
	ls := realLines(t)
	ls = append(ls[:12], ls[13:]...) // drop blank line 13
	ls = append(ls[:13], ls[14:]...) // drop blank line after Date Collected
	rep, err := parseReport(strings.Join(ls, "\n"))
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	r := find(rep, "WBC")
	t.Logf("WBC panel=%q", r.Panel)
	if r.Panel != "CBC With Differential/Platelet" {
		t.Errorf("WBC stored with panel %q", r.Panel)
	}
}

// R1b. The split the real layout already has (name and code on one line,
// values on the next) with the flag one line lower again: the value line
// fills the pending row, and the flag line is a comment.
func TestRefitPendingRowFlagOnThirdLine(t *testing.T) {
	cases := map[string][]string{
		"flag under the values":   {at(58, "78", 141, "fL", 166, "79-97"), at(76, "Low")},
		"value alone, rest under": {at(58, "78"), at(76, "Low", 141, "fL", 166, "79-97")},
	}
	for name, lines := range cases {
		rep, err := parseReport(strings.Join(set(realLines(t), 24, lines...), "\n"))
		if err != nil {
			t.Logf("%s: refused: %v", name, err)
			continue
		}
		r := find(rep, "MCV")
		t.Logf("%s: accepted value=%q flag=%q units=%q ref=%q comment=%q", name, r.Value, r.Flag, r.Units, r.Ref, r.Comment)
		if r.Flag != "Low" || r.Units != "fL" || r.Ref != "79-97" {
			t.Errorf("%s: MCV stored without its flag, unit or range", name)
		}
	}
}

// R5. A row whose name starts at or right of Test+40 is a comment of the
// row above it: the reading is dropped and the others import.
func TestRefitRowInCommentZoneDropped(t *testing.T) {
	ls := set(realLines(t), 57, at(46, "Sodium 01", 57, "801", 137, "mmol/L", 164, "134-144"))
	rep, err := parseReport(strings.Join(ls, "\n"))
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	t.Logf("accepted %d results; BUN/Creatinine Ratio comment=%q", len(rep.Results), find(rep, "BUN/Creatinine Ratio").Comment)
	if find(rep, "Sodium") == nil {
		t.Errorf("Sodium 801 mmol/L dropped into a comment")
	}
}
