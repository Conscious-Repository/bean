package main

import (
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

// Regression tests from audit 8. Synthetic data only. Each fails when the importer accepts
// a report and loses or misplaces a value: it logs what was written.

func logAccepted(t *testing.T, text string) []map[string]any {
	t.Helper()
	rep, err := parseReport(text)
	if err != nil {
		t.Logf("refused: %v", err)
		return nil
	}
	recs := rep.records("", nil)
	for _, r := range recs {
		t.Logf("accepted %q %s ctx=%s", r["type"], core.Canonical(r["value"]), core.Canonical(r["context"]))
	}
	return recs
}

// A tab between the name and the value: tokens and indent count only spaces,
// so the name check never sees the value. The old trailing-number check
// (removed in this diff) caught "\t101".
func TestTabBetweenNameAndValueRefused(t *testing.T) {
	recs := logAccepted(t, ambiguityReport(
		pad("", 6)+"Glucose\t\t\t\t\t101",
		row("Sodium", "01", "140", "", "mmol/L", "134-144"),
	))
	for _, r := range recs {
		if strings.HasPrefix(r["type"].(string), "lab.glucose") && string(core.Canonical(r["value"])) == `{"n":null}` {
			t.Errorf("value 101 lost into the name: %s", r["type"])
		}
	}
}

// A tab-indented row has indent 0, so it is read as a panel heading: the
// row and its value vanish, in silence, and before the header the new
// "row before the column header" refusal does not fire either.
func TestTabIndentedRowRefused(t *testing.T) {
	text := ambiguityReport(
		"\tGlucose"+strings.Repeat(" ", 44)+"101"+strings.Repeat(" ", 80)+"mg/dL",
		row("Sodium", "01", "140", "", "mmol/L", "134-144"),
	)
	recs := logAccepted(t, text)
	if len(recs) == 1 {
		t.Errorf("glucose row dropped, %d reading(s) written, panel now %q", len(recs), recs[0]["context"].(map[string]any)["panel"])
	}
	before := strings.Replace(text, header(), "\tGlucose      101    mg/dL\n"+header(), 1)
	if logAccepted(t, before) != nil {
		t.Errorf("a tab-indented row before the header is accepted, and dropped")
	}
}

// Results after a "Disclaimer" heading are dropped: the loop breaks there.
func TestResultsAfterDisclaimerNotDropped(t *testing.T) {
	recs := logAccepted(t, ambiguityReport(
		row("Sodium", "01", "140", "", "mmol/L", "134-144"),
		"",
		"Disclaimer",
		"Some text.",
		"Lipids",
		header(),
		row("LDL Cholesterol", "01", "190", "High", "mg/dL", "0-99"),
	))
	for _, r := range recs {
		if r["type"] == "lab.ldl-cholesterol" {
			return
		}
	}
	if recs != nil {
		t.Errorf("LDL 190 after a Disclaimer heading is dropped; %d reading(s) written", len(recs))
	}
}

// Without a Performing Labs list, a two-digit value printed in the code
// column (a compressed layout) is taken for a lab code: the reading is
// stored with no value and lab code "90".
func TestCodeColumnValueWithoutLabListRefused(t *testing.T) {
	text := strings.Join([]string{
		"Specimen ID: 123-456-7890-0  Date Collected: 10/19/2026 0800 Local",
		header(),
		"Metabolic Panel",
		pad(pad("", 6)+"eGFR", 35) + "90",
		row("Sodium", "", "140", "", "mmol/L", "134-144"),
	}, "\n") + "\n"
	rep, err := parseReport(text)
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	for _, r := range rep.Results {
		t.Logf("%+v", *r)
		if r.Name == "eGFR" && r.Value == "" && r.LabCode == "90" {
			t.Errorf("value 90 taken for a lab code; eGFR stored without a value")
		}
	}
}

// A header whose columns sit right of where this page prints values (a
// compressed page under an earlier header): a value one space after the
// name passes the column-4 check, and the reading is stored without it.
func TestValueOneSpaceAfterNameRefused(t *testing.T) {
	recs := logAccepted(t, ambiguityReport(
		pad("", 6)+"Glucose 101",
		row("Sodium", "01", "140", "", "mmol/L", "134-144"),
	))
	for _, r := range recs {
		if r["type"] == "lab.glucose-101" {
			t.Errorf("value 101 became part of the type %s with value %s", r["type"], core.Canonical(r["value"]))
		}
	}
}

// A form feed that starts a page, as pdftotext writes it, has no width and is
// read. Any other tab, form feed, vertical tab or no-break space is refused.
func TestHiddenWhitespaceRefused(t *testing.T) {
	ok := ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), "\f"+row("Potassium", "01", "4.1", "", "mmol/L", "3.5-5.2")) + "\f"
	rep, err := parseReport(ok)
	if err != nil || len(rep.Results) != 2 {
		t.Fatalf("page-break form feed: %v", err)
	}
	for _, bad := range []string{"\f", "\v", "\t", " "} {
		text := ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), pad("", 6)+"Glucose"+bad+"101")
		if _, err := parseReport(text); err == nil || !strings.Contains(err.Error(), "columns cannot be read") {
			t.Errorf("%q accepted: %v", bad, err)
		}
	}
}

// A row with its value in the result column after the end of the results,
// without a header, is refused, not dropped.
func TestRowAfterDisclaimerRefused(t *testing.T) {
	_, err := parseReport(ambiguityReport(
		row("Sodium", "01", "140", "", "mmol/L", "134-144"),
		"",
		"Disclaimer",
		row("LDL Cholesterol", "01", "190", "High", "mg/dL", "0-99"),
	))
	if err == nil || !strings.Contains(err.Error(), "would be lost") {
		t.Errorf("row after Disclaimer: %v", err)
	}
}

// A categorical result glued to the name, or a number with a comment under
// it, leaves a row with no value whose name ends in the value: refused.
func TestGluedValueWithoutValueRefused(t *testing.T) {
	for _, rows := range [][]string{
		{pad("", 6) + "HCV Ab Non Reactive"},
		{pad("", 6) + "Glucose 101", comment("Specimen hemolyzed.")},
		{pad("", 6) + "Vitamin D <5.0"},
	} {
		if _, err := parseReport(ambiguityReport(rows...)); err == nil || !strings.Contains(err.Error(), "may be the value") {
			t.Errorf("%q accepted: %v", rows[0], err)
		}
	}
}
