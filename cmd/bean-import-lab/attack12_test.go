package main

import (
	"strings"
	"testing"
)

// Attack 12. Synthetic data only. Each test logs what was accepted and fails
// when a wrong reading is accepted.

func a12Report(labs []string, rows ...string) string {
	lines := append([]string{
		"Specimen ID: 123-456-7890-0  Date Collected: 10/19/2026 0800 Local",
		"Date Reported: 10/20/2026",
		"Cytokines",
		header(),
	}, rows...)
	lines = append(lines, "", "Performing Labs")
	lines = append(lines, labs...)
	return strings.Join(lines, "\n") + "\n"
}

var a12Labs = []string{
	"01: BN - Labcorp Burlington, 1447 York Court, Burlington, NC 27215",
	"10: RN - Labcorp Raritan, 69 First Ave, Raritan, NJ 08869",
}

// P1. A numbered test whose number pdftotext sets two spaces from its family
// word: the number is a token in the Test band, and with a list that names
// it, it is taken as the lab code. Interleukin 10 = 42 is stored as
// lab.interleukin from lab 10. The F3 guard (number glued to the name) does
// not see it, because the code was taken from the code column.
func TestA12NumberedTestNumberInCodeColumn(t *testing.T) {
	for _, il := range []string{
		pad("", 6) + "Interleukin  10",           // two spaces, number right after the name
		pad(pad("", 6)+"Interleukin", 35) + "10", // number at the code column
	} {
		g := pad(il, 57) + vals("42", "High", "pg/mL", "0.0-9.7")[57:]
		rep, err := parseReport(a12Report(a12Labs,
			row("Sodium", "01", "140", "", "mmol/L", "134-144"), g))
		if err != nil {
			t.Logf("refused: %v", err)
			continue
		}
		a11Show(t, rep)
		for _, r := range rep.Results {
			if r.Value == "42" && r.Name != "Interleukin 10" {
				t.Errorf("Interleukin 10 = 42 stored as lab.%s from lab %q", slug(r.Name), rep.Labs[r.LabCode])
			}
		}
	}
}

// probe: a row at the margin with its value one space after the name is a
// panel heading: the reading is dropped and later rows get its panel.
func TestA12ProbeMarginGluedRow(t *testing.T) {
	rep, err := parseReport(a12Report(a12Labs,
		row("Sodium", "01", "140", "", "mmol/L", "134-144"),
		"Glucose 95",
		row("Potassium", "01", "4.1", "", "mmol/L", "3.5-5.2")))
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	a11Show(t, rep)
	for _, r := range rep.Results {
		t.Logf("  %s panel %q", r.Name, r.Panel)
		if r.Panel == "Glucose 95" {
			t.Errorf("Glucose 95 dropped as a panel heading; %s filed under panel %q", r.Name, r.Panel)
		}
	}
}

// probe: a reference word in the units band on a row with no unit.
func TestA12ProbeRefWordInUnits(t *testing.T) {
	g := pad(pad(pad("", 6)+"Antibody Screen", 35)+"01", 57) + "Negative"
	g = pad(g, 145) + "Negative"
	rep, err := parseReport(a12Report(a12Labs, g))
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	a11Show(t, rep)
	t.Error("accepted: the reading is stored with a wrong unit or under a wrong test")
}

// probe: a wrapped name whose second line is indented into the comment band.
func TestA12ProbeWrappedName(t *testing.T) {
	rep, err := parseReport(a12Report(a12Labs,
		row("Hemoglobin", "01", "5.9", "High", "%", "4.8-5.6"),
		pad("", 8)+"A1c"))
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	a11Show(t, rep)
	t.Error("accepted: the reading is stored with a wrong unit or under a wrong test")
}
