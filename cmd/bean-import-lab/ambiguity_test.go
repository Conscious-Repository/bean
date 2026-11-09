package main

import (
	"strings"
	"testing"
)

func ambiguityReport(rows ...string) string {
	return strings.Join(append(append([]string{
		"Specimen ID: 123-456-7890-0  Date Collected: 10/19/2026 0800 Local",
		"Date Reported: 10/20/2026",
		header(),
		"Metabolic Panel",
	}, rows...), "", "Performing Labs", "01: BN - Labcorp Burlington, 1447 York Court, Burlington, NC 27215"), "\n") + "\n"
}

// A number at the end of a test name stays in the name unless it is a lab code
// that the report lists. Interleukin 10 and 12 used to become one type.
func TestTestNumberIsNotALabCode(t *testing.T) {
	rep, err := parseReport(ambiguityReport(
		row("Interleukin 10", "", "42", "High", "pg/mL", "0-9"),
		row("Interleukin 12", "", "3", "", "pg/mL", "0-9"),
		row("Chloride 01", "", "95", "Low", "mmol/L", "96-106"),
	))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range rep.Results {
		got[slug(r.Name)] = r.Value + "/" + r.LabCode
	}
	if got["interleukin-10"] != "42/" || got["interleukin-12"] != "3/" || got["chloride"] != "95/01" {
		t.Errorf("results: %v", got)
	}
}

// letters names the i-th test without digits, which could read as a lab code.
func letters(i int) string {
	s := ""
	for {
		s = string(rune('a'+i%26)) + s
		if i /= 26; i == 0 {
			return s
		}
	}
}

// The same reading listed in two panels is one result.
func TestSameReadingTwicePanelsIsOne(t *testing.T) {
	rep, err := parseReport(ambiguityReport(
		row("Glucose", "01", "90", "", "mg/dL", "70-99"),
		"Hemoglobin A1c",
		row("Glucose", "01", "90", "", "mg/dL", "70-99"),
	))
	if err != nil || len(rep.Results) != 1 {
		t.Fatalf("err %v, results %+v", err, rep)
	}
}

// Two results that map to one type are refused, not mixed.
func TestTwoResultsOneTypeRefused(t *testing.T) {
	_, err := parseReport(ambiguityReport(
		row("HDL-C", "01", "50", "", "mg/dL", ">39"),
		row("HDL C", "01", "51", "", "mg/dL", ">39"),
	))
	if err == nil || !strings.Contains(err.Error(), "map to one type") {
		t.Errorf("err %v", err)
	}
}

// A name that runs into its value loses the value: refuse instead of storing
// "not performed".
func TestValueSwallowedByNameRefused(t *testing.T) {
	long := pad("", 6) + strings.Repeat("Long Test Name ", 3) + "Overrun 90"
	_, err := parseReport(ambiguityReport(long))
	if err == nil || !strings.Contains(err.Error(), "runs into the result column") {
		t.Errorf("err %v", err)
	}
	// A test the lab did not perform, with its comment, still parses.
	if _, err := parseReport(ambiguityReport(row("Glucose", "01", "-", "", "", ""), vals("", "", "", "70-99"), comment("Test not performed."))); err != nil {
		t.Errorf("not performed: %v", err)
	}
}
