package main

import (
	"strings"
	"testing"
)

func nameReport(name, value string) string {
	return strings.Join([]string{
		"Specimen ID: 123-456-7890-0  Date Collected: 10/19/2026 0800 Local",
		header(), "Metabolic Panel",
		row(name, "01", value, "", "mg/dL", "65-99"),
		"", "Performing Labs", "01: BN - Labcorp Burlington, 1447 York Court, Burlington, NC 27215",
	}, "\n") + "\n"
}

// A value glued to the name by one space is refused even when the result
// column holds a word the grammar takes.
func TestNameHidesValueOnRowWithValue(t *testing.T) {
	for _, c := range []struct{ name, value string }{
		{"Glucose 101", "Abnormal"}, {"Glucose 101", "Negative"}, {"Glucose 101", "101"},
		{"Glucose 5.9", "Negative"}, {"Glucose <5", "Negative"}, {"Glucose High", "101"},
		{"Interleukin 6 101", "Abnormal"},
		// A number inside a name may be a value with its flag after it
		// ("Glucose 101 H"), so it is refused too.
		{"Beta 2 Microglobulin", "4.2"}, {"Glucose 101 H", "Negative"},
	} {
		if _, err := parseReport(nameReport(c.name, c.value)); err == nil {
			t.Errorf("%q with %q accepted", c.name, c.value)
		}
	}
	for _, name := range []string{"Interleukin 6", "CA 125", "CA 19-9", "Cyfra 21-1", "Vitamin A", "Hepatitis A Ab", "Beta-2 Microglobulin", "Hemoglobin A1c", "Vitamin D, 25-Hydroxy"} {
		rep, err := parseReport(nameReport(name, "4.2"))
		if err != nil || len(rep.Results) != 1 || rep.Results[0].Value != "4.2" {
			t.Errorf("%q refused or misread: %v", name, err)
		}
	}
}
