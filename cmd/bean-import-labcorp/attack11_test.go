package main

import (
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

// Attack 11. Synthetic data only. Each test logs what was accepted and fails
// when a wrong reading is accepted.

func a11Show(t *testing.T, rep *Report) {
	t.Helper()
	for _, r := range rep.Results {
		t.Logf("  lab.%s name=%q value=%q flag=%q units=%q ref=%q code=%q comment=%q",
			slug(r.Name), r.Name, r.Value, r.Flag, r.Units, r.Ref, r.LabCode, r.Comment)
	}
	for _, rec := range rep.records("", nil) {
		t.Logf("  record %s value=%s", rec["type"], core.Canonical(rec["value"]))
	}
}

// F1. The value joins the name one space after it, and the flag stays at the
// flag column. "Abnormal" is a flag and a value, so the flag is stored as
// the value. nameHidesValue only knows bare numbers and flag words: 3+,
// 1:160, -2.0 and result words pass.
func TestA11NameHidesValueAbnormalFlag(t *testing.T) {
	for _, name := range []string{"Urine Glucose 3+", "ANA Titer 1:160", "Base Excess -2.0", "HCV Ab Reactive", "Urine Protein Trace", "Factor 101"} {
		g := pad(pad("", 6)+name, 68) + "Abnormal"
		rep, err := parseReport(ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), g))
		if err != nil {
			t.Logf("%q refused: %v", name, err)
			continue
		}
		a11Show(t, rep)
		t.Errorf("%q accepted: lab.%s value %q flag %q (the flag became the value)", name, slug(rep.Results[1].Name), rep.Results[1].Value, rep.Results[1].Flag)
	}
}

// F2. Without a list of performing labs that reads whole (no list, or a lab
// line wrapped before its first comma), a two-digit token in the code
// column is a code whenever the result column holds anything. A compressed
// row "Bacteria  10  Abnormal" stores the flag as the value and 10 as the
// lab code.
func TestA11CodeColumnValueWithAbnormalFlag(t *testing.T) {
	b := pad(pad(pad("", 6)+"Bacteria", 35)+"10", 68) + "Abnormal"
	b = pad(b, 160) + "None seen"
	text := strings.Join([]string{
		"Specimen ID: 123-456-7890-0  Date Collected: 10/19/2026 0800 Local",
		header(),
		"Urinalysis",
		row("Sodium", "", "140", "", "mmol/L", "134-144"),
		b,
		"",
		"Performing Labs",
		"01: BN Labcorp Burlington",
		"    1447 York Court, Burlington, NC 27215",
	}, "\n") + "\n"
	text = strings.Replace(text, "    1447 York Court", "1447 York Court", 1)
	rep, err := parseReport(text)
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	a11Show(t, rep)
	for _, r := range rep.Results {
		if r.Name == "Bacteria" {
			t.Errorf("Bacteria accepted: value %q flag %q lab code %q (the value 10 is read as a lab code)", r.Value, r.Flag, r.LabCode)
		}
	}
}

// F3. With a list that names code 10, "Interleukin 10" with no code in the
// code column loses its number: it is read as test "Interleukin" from lab 10.
func TestA11NumberedTestLosesNumberToLabCode(t *testing.T) {
	text := strings.Join([]string{
		"Specimen ID: 123-456-7890-0  Date Collected: 10/19/2026 0800 Local",
		header(),
		"Cytokines",
		row("Sodium", "01", "140", "", "mmol/L", "134-144"),
		row("Interleukin 10", "", "42", "High", "pg/mL", "0-9"),
		"",
		"Performing Labs",
		"01: BN - Labcorp Burlington, 1447 York Court, Burlington, NC 27215",
		"10: RN - Labcorp Raritan, 69 First Ave, Raritan, NJ 08869",
	}, "\n") + "\n"
	rep, err := parseReport(text)
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	a11Show(t, rep)
	for _, r := range rep.Results {
		if r.Value == "42" && r.Name != "Interleukin 10" {
			t.Errorf("Interleukin 10 = 42 stored as lab.%s from lab %q", slug(r.Name), rep.Labs[r.LabCode])
		}
	}
}

// F4. A reference interval that starts one column left of its band, on a
// row with no unit, is read as the unit: value 12 with unit "9-20".
func TestA11RefReadAsUnit(t *testing.T) {
	r := pad(pad("", 6)+"BUN/Creatinine Ratio", 57) + "12"
	r = pad(r, 155) + "9-20"
	rep, err := parseReport(ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), r))
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	a11Show(t, rep)
	if g := rep.Results[1]; g.Units == "9-20" {
		t.Errorf("BUN/Creatinine Ratio: unit %q, ref %q (the interval became the unit)", g.Units, g.Ref)
	}
}

// F4b. A previous result whose value starts two columns left of the previous
// band is read as the current flag: HIV Screen Negative, flag "Abnormal".
func TestA11PreviousValueReadAsFlag(t *testing.T) {
	h := pad(pad(pad(pad(pad("", 6)+"HIV Screen", 35)+"01", 57)+"Negative", 88)+"Abnormal", 92) + "Abnormal  01/02/2024"
	rep, err := parseReport(ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), h))
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	a11Show(t, rep)
	if r := rep.Results[1]; r.Flag == "Abnormal" {
		t.Errorf("HIV Screen %q flagged %q; previous %q", r.Value, r.Flag, r.Previous)
	}
}

// F5. A comment line that starts at the name column is a row: "Note:" with a
// result word in the result column becomes a reading lab.note.
func TestA11NoteAtNameColumnIsReading(t *testing.T) {
	rep, err := parseReport(ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), pad(pad("", 6)+"Note:", 57)+"Positive"))
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	a11Show(t, rep)
	for _, r := range rep.Results {
		if r.Name == "Note:" {
			t.Errorf("comment read as reading lab.%s = %q", slug(r.Name), r.Value)
		}
	}
}

// F6 (nit). A test named "Report" gets the archive's type, lab.report.
func TestA11ReadingTypeIsArchiveType(t *testing.T) {
	rep, err := parseReport(ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), row("Report", "01", "5", "", "", "")))
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	for _, rec := range rep.records("", nil) {
		if rec["type"] == "lab.report" {
			t.Errorf("a reading has the archive's type lab.report")
		}
	}
}
