package main

import (
	"strings"
	"testing"
)

// Regression tests from attack 10, which the line grammar answers
// (DECISIONS 222). Synthetic data only. Each test fails when the attack
// works: a report is accepted and a value is lost, moved or misnamed.

func showResults(t *testing.T, rep *Report) {
	for _, r := range rep.Results {
		t.Logf("  result %q value=%q flag=%q units=%q ref=%q prev=%q comment=%q", r.Name, r.Value, r.Flag, r.Units, r.Ref, r.Previous, r.Comment)
	}
}

// 1. A header line holding "DOB:" is boilerplate, so it skips the ASCII
// check, but it is still read as the column header. Five two-byte letters
// before "Current Result" push every column five bytes right: the previous
// result is read as the current value.
func TestGrammarBoilerHeaderShiftsColumns(t *testing.T) {
	// The same characters as the plain header, so pdftotext lays the rows
	// out under it unchanged; five of them are two bytes long.
	h := strings.Replace(header(), "Test"+strings.Repeat(" ", 11), "Test ééééé DOB:", 1)
	// Glucose: no current value, previous 95 and its date in the previous column.
	g := pad("", 6) + "Glucose"
	g = pad(g, 35) + "01"
	g = pad(g, 92) + "95  01/02/2024"
	g = pad(g, 140) + "mg/dL"
	g = pad(g, 160) + "65-99"
	text := strings.Join([]string{
		"Specimen ID: 123-456-7890-0  Date Collected: 10/19/2026 0800 Local",
		h,
		"Metabolic Panel",
		row("Sodium", "01", "140", "", "mmol/L", "134-144"),
		g,
		"",
		"Performing Labs",
		"01: BN - Labcorp Burlington, 1447 York Court, Burlington, NC 27215",
	}, "\n") + "\n"
	rep, err := parseReport(text)
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	showResults(t, rep)
	for _, r := range rep.Results {
		if r.Name == "Glucose" && r.Value == "95" {
			t.Errorf("previous result 95 stored as the current Glucose value (flag %q)", r.Flag)
		}
	}
}

// 2. A result row holding "DOB:" anywhere is boilerplate and is skipped
// without a word: the reading is lost.
func TestGrammarBoilerRowLost(t *testing.T) {
	for _, bad := range []string{
		row("Glucose", "01", "101", "", "mg/dL", "DOB: adjusted"),
		row("Glucose", "01", "101", "", "mg/dL", "65-99") + "  Date Created and Stored",
	} {
		rep, err := parseReport(ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), bad))
		if err != nil {
			continue
		}
		showResults(t, rep)
		if len(rep.Results) != 2 {
			t.Errorf("%q: accepted with %d results; Glucose 101 lost", bad, len(rep.Results))
		}
	}
}

// 3. Inside the results, a line at indent 0 is a panel heading, whatever it
// holds. A row at indent 0 with its value in the result column is lost.
func TestGrammarIndentZeroRowIsPanel(t *testing.T) {
	g := strings.TrimLeft(row("Glucose", "", "101", "H", "mg/dL", "65-99"), " ")
	g = "Glucose" + strings.Repeat(" ", 50) + strings.TrimLeft(g[len("Glucose"):], " ")
	rep, err := parseReport(ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), g))
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	showResults(t, rep)
	t.Errorf("accepted with %d results; %q became a panel heading", len(rep.Results), g)
}

// 4. Rows are told from comments by a fixed indent of 20, not by the
// header. A page laid out 20 columns to the right (its header too) turns
// every row into a comment of the last result before it.
func TestGrammarShiftedPageRowsBecomeComments(t *testing.T) {
	sh := func(s string) string { return strings.Repeat(" ", 20) + s }
	text := strings.Join([]string{
		"Specimen ID: 123-456-7890-0  Date Collected: 10/19/2026 0800 Local",
		header(),
		"Metabolic Panel",
		row("Sodium", "01", "140", "", "mmol/L", "134-144"),
		sh(header()),
		sh(row("Glucose", "01", "101", "High", "mg/dL", "65-99")),
		sh(row("Potassium", "01", "6.1", "High", "mmol/L", "3.5-5.2")),
		"",
		"Performing Labs",
		"01: BN - Labcorp Burlington, 1447 York Court, Burlington, NC 27215",
	}, "\n") + "\n"
	rep, err := parseReport(text)
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	showResults(t, rep)
	if len(rep.Results) != 3 {
		t.Errorf("accepted with %d results; Glucose and Potassium lost", len(rep.Results))
	}
}

// 5. A lab comment that names the four columns in order is read as a new
// column header. Columns move, and the previous result becomes the unit.
func TestGrammarCommentRemapsColumns(t *testing.T) {
	c := comment("Note: Current Result is compared to Previous Result. Units and Reference Interval changed 01/2025.")
	g := pad("", 6) + "Glucose"
	g = pad(g, 35) + "01"
	g = pad(g, 57) + "101"
	g = pad(g, 92) + "95 01/02/2024"
	g = pad(g, 140) + "mg/dL"
	g = pad(g, 160) + "65-99"
	rep, err := parseReport(ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), c, g))
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	showResults(t, rep)
	for _, r := range rep.Results {
		if r.Name == "Glucose" && r.Units != "mg/dL" {
			t.Errorf("Glucose unit %q, ref %q: a comment moved the columns", r.Units, r.Ref)
		}
	}
}

// 6. hidesValue runs only on a row with no value. A name that holds the
// value one space after it, with only the flag in the result column, gives
// type lab.glucose-101 and value "H".
func TestGrammarNameHidesValueFlagIsValue(t *testing.T) {
	g := pad(pad("", 6)+"Glucose 101", 57) + "H"
	g = pad(g, 140) + "mg/dL"
	g = pad(g, 160) + "65-99"
	rep, err := parseReport(ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), g))
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	showResults(t, rep)
	for _, r := range rep.Results {
		if r.Name == "Glucose 101" {
			t.Errorf("type lab.%s value %q: the flag is stored as the value", slug(r.Name), r.Value)
		}
	}
}

// 7. hidesValue gaps: value words it does not know, with a not-performed
// comment, become part of the type and the reading has no value. A name
// word after the first that is not letters and hyphens is refused. A word
// of letters is a word of the name: the grammar does not guess that "Neg"
// is a result. Such a row is read as the lab printed it, a test it did not
// perform, and only with the lab's own "Test not performed" under it.
func TestGrammarHidesValueGaps(t *testing.T) {
	report := func(name string) string {
		return ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), pad("", 6)+name, comment("Test not performed."))
	}
	for _, name := range []string{
		"Glucose (101)", "Glucose .5", "Glucose ~101", "Glucose *101", "Glucose (H)", "Glucose H*",
	} {
		if rep, err := parseReport(report(name)); err == nil {
			t.Errorf("%q accepted: type lab.%s, no value", name, slug(rep.Results[1].Name))
		}
	}
	for _, name := range []string{
		"Nitrite Neg", "HBsAg Pos", "HIV Screen NR", "RPR R", "Glucose WNL",
		"Urine Color Yellow", "Bacteria Few", "Glucose Hi",
	} {
		rep, err := parseReport(report(name))
		if err != nil {
			continue
		}
		if r := rep.Results[1]; r.Name != name || r.Value != "" || strings.Join(r.Comment, " ") != "Test not performed." {
			t.Errorf("%q read as %+v", name, *r)
		}
		// Without the lab's word, the same row is refused.
		if _, err := parseReport(ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), pad("", 6)+name)); err == nil {
			t.Errorf("%q with no value and no comment accepted", name)
		}
	}
}

// 8. A "Please note" row does not end the wait for a value. A row with no
// value, then a note whose text wraps into the result column: the note
// text becomes the row's value, and no not-performed check runs.
func TestGrammarPleaseNoteFillsValue(t *testing.T) {
	rep, err := parseReport(ambiguityReport(
		row("Sodium", "01", "140", "", "mmol/L", "134-144"),
		pad("", 6)+"Glucose",
		pad("", 6)+"Please note:",
		pad("", 57)+"Repeat advised",
	))
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	showResults(t, rep)
	for _, r := range rep.Results {
		if r.Name == "Glucose" && r.Value != "" {
			t.Errorf("Glucose took value %q from a Please note row", r.Value)
		}
	}
}

// 9. After the end of the results, a value line that comes before its name
// line (pdftotext orders by baseline) is not under an indented line, and a
// one-token name passes: the reading is lost without a word.
func TestGrammarAfterEndValueBeforeName(t *testing.T) {
	text := strings.Join([]string{
		"Specimen ID: 123-456-7890-0  Date Collected: 10/19/2026 0800 Local",
		header(),
		"Metabolic Panel",
		row("Sodium", "01", "140", "", "mmol/L", "134-144"),
		"",
		"Disclaimer",
		vals("101", "High", "mg/dL", "65-99"),
		pad("", 6) + "Glucose",
		"",
		"Performing Labs",
		"01: BN - Labcorp Burlington, 1447 York Court, Burlington, NC 27215",
	}, "\n") + "\n"
	if rep, err := parseReport(text); err == nil {
		t.Errorf("accepted with %d results; Glucose 101 after Disclaimer lost", len(rep.Results))
	}
}

// 10. After "Patient Details", a line may hold any character, and indent
// counts spaces only: a row indented with a no-break space or a tab has
// indent 0 and passes the row check. The reading is lost without a word.
func TestGrammarPatientDetailsIndentBypass(t *testing.T) {
	for _, lead := range []string{"      ", "\t"} {
		text := strings.Join([]string{
			"Specimen ID: 123-456-7890-0  Date Collected: 10/19/2026 0800 Local",
			header(),
			"Metabolic Panel",
			row("Sodium", "01", "140", "", "mmol/L", "134-144"),
			"",
			"Performing Labs",
			"01: BN - Labcorp Burlington, 1447 York Court, Burlington, NC 27215",
			"Patient Details",
			lead + strings.TrimLeft(row("Glucose", "01", "101", "High", "mg/dL", "65-99"), " "),
		}, "\n") + "\n"
		if rep, err := parseReport(text); err == nil {
			t.Errorf("%q: accepted with %d results; a row after Patient Details lost", lead, len(rep.Results))
		}
	}
}

// 11. The unit and reference interval on the line under a row with a value
// (pdftotext splits a row by baseline) are a comment: the reading has no unit.
func TestGrammarUnitsOnNextLineLost(t *testing.T) {
	rep, err := parseReport(ambiguityReport(
		row("Sodium", "01", "140", "", "mmol/L", "134-144"),
		row("Glucose", "01", "101", "High", "", ""),
		vals("", "", "mg/dL", "65-99"),
	))
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	showResults(t, rep)
	if g := rep.Results[1]; g.Units == "" {
		t.Errorf("Glucose 101 stored with no unit; units went to lab_comment %q", g.Comment)
	}
}

// 12. Date Reported: the last line that names one wins, without a conflict
// check; Fasting: the first line anywhere, a comment too, wins. Both go
// into every reading's context.
func TestGrammarContextFieldsFromComments(t *testing.T) {
	text := strings.Replace(ambiguityReport(
		row("Glucose", "01", "101", "High", "mg/dL", "65-99"),
		comment("Fasting: not required. Date Reported: 01/01/2020 for the prior draw."),
	), "Date Reported: 10/20/2026\n", "", 1)
	text = "Date Reported: 10/20/2026\n" + text
	rep, err := parseReport(text)
	if err != nil {
		t.Logf("refused: %v", err)
		return
	}
	if rep.Reported != "10/20/2026" || rep.Fasting != "" {
		t.Errorf("reported %q, fasting %q: taken from a lab comment", rep.Reported, rep.Fasting)
	}
}
