package main

import (
	"strings"
	"testing"
)

// Attack 13. Synthetic data only. Each test logs what was accepted and fails
// when a wrong reading is accepted.

func a13Show(t *testing.T, text string) []map[string]any {
	t.Helper()
	return logAccepted(t, text)
}

// T1. A wrapped test name whose second line (hanging indent, comment band)
// has a space in it is read as a comment: the value is stored under the
// first line's name. Attack 12 refused only a one-word continuation.
func TestA13WrappedNameTwoWords(t *testing.T) {
	cases := [][]string{
		{row("Testosterone", "01", "12.1", "", "pg/mL", "8.7-25.1"), pad("", 9) + "Free, Direct"},
		{row("Hemoglobin", "01", "5.9", "High", "%", "4.8-5.6"), pad("", 9) + "A1c (Glycated)"},
		// Values on their own line under the first line of the name.
		{row("Testosterone", "01", "-", "", "", ""), vals("12.1", "", "pg/mL", "8.7-25.1"), pad("", 9) + "Free, Direct"},
	}
	for _, c := range cases {
		recs := a13Show(t, ambiguityReport(c...))
		for _, r := range recs {
			if typ := r["type"].(string); typ == "lab.testosterone" || typ == "lab.hemoglobin" {
				t.Errorf("accepted: the second line of the name became a comment; value stored as %s", typ)
			}
		}
	}
}

// T2. A one-word reference interval with a letter or '/' that sits in the
// units band on a row without units is stored as the unit. Attack 12 refused
// only a result or flag word there.
func TestA13RefWordWithSlashInUnits(t *testing.T) {
	for _, ref := range []string{"Negative/Trace", "Yellow", "<5/hpf"} {
		g := pad(pad(pad("", 6)+"Protein, Urine", 35)+"01", 57) + "Negative"
		g = pad(g, 145) + ref
		recs := a13Show(t, ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), g))
		for _, r := range recs {
			if r["type"] == "lab.protein-urine" {
				t.Errorf("accepted: reference %q stored as the unit", ref)
			}
		}
	}
}

// T3. A unit that sits in the reference band on a row with no reference
// interval is stored as the reference interval: the number has no unit.
func TestA13UnitInRefBand(t *testing.T) {
	g := pad(pad(pad("", 6)+"Glucose", 35)+"01", 57) + "95"
	g = pad(g, 160) + "mg/dL"
	recs := a13Show(t, ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), g))
	for _, r := range recs {
		if r["type"] == "lab.glucose" {
			t.Errorf("accepted: lab.glucose stored without its unit")
		}
	}
}

// T4. A row at the margin with its value in the name is a panel heading
// before the first column header, and one token of text after the end of the
// results: the reading is dropped while the others import. Attack 12 checked
// the panel heading only inside the results.
func TestA13MarginRowOutsideResults(t *testing.T) {
	before := strings.Join([]string{
		"Specimen ID: 123-456-7890-0  Date Collected: 10/19/2026 0800 Local",
		"Date Reported: 10/20/2026",
		"Glucose 101",
		"Metabolic Panel",
		header(),
		row("Sodium", "01", "140", "", "mmol/L", "134-144"),
		"", "Performing Labs", "01: BN - Labcorp Burlington, 1447 York Court, Burlington, NC 27215",
	}, "\n") + "\n"
	after := ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), "", "Disclaimer", "HIV-1/2 Ab Reactive", "Glucose 101 High")
	for _, text := range []string{before, after} {
		if recs := a13Show(t, text); recs != nil {
			t.Errorf("accepted: a reading at the margin was dropped, %d other reading(s) written", len(recs))
		}
	}
}

// T5. A lab code that drifts into the result band, on a row the lab did not
// perform, is read as the value.
func TestA13CodeInResultBandIsValue(t *testing.T) {
	g := pad(pad("", 6)+"Glucose", 57) + "01"
	recs := a13Show(t, ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), g, comment("Test not performed. Specimen was hemolyzed.")))
	for _, r := range recs {
		if r["type"] == "lab.glucose" {
			t.Errorf("accepted: a not-performed test stored with value 1")
		}
	}
}

// T6. After the end of the results any margin line that starts "NN: " passes
// as a (torn) lab line, whatever follows.
func TestA13LabLikeLineAfterEnd(t *testing.T) {
	recs := a13Show(t, ambiguityReport(row("Sodium", "", "140", "", "mmol/L", "134-144"), "", "Disclaimer", "01: Glucose          101   High   mg/dL   70-99"))
	if recs != nil {
		t.Errorf("accepted: a row after the end dropped as a torn lab line")
	}
}
