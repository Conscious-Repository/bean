package main

import (
	"strings"
	"testing"
)

func TestRejectConflictingReportIdentity(t *testing.T) {
	for name, tail := range map[string]string{
		"specimen":        "Specimen ID: SYNTHETIC-DIFFERENT",
		"collection-day":  "Date Collected: 02/03/2025",
		"collection-time": "Date Collected: 01/02/2025 0930 Local",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseReport(fixture() + "\n" + tail); err == nil {
				t.Fatal("accepted conflicting report identity")
			}
		})
	}
}

func TestWrappedValueSurvivesRepeatedPageHeader(t *testing.T) {
	for _, continuation := range []string{header(), "Basic Panel (Cont.)\n" + header()} {
		text := strings.Join([]string{"Specimen ID: SYNTHETIC-A", "Date Collected: 01/02/2025 0815 Local", "Basic Panel", header(), "      Glucose", continuation, vals("100", "", "mg/dL", "70-99")}, "\n")
		rep, err := parseReport(text)
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.Results) != 1 || rep.Results[0].Value != "100" || rep.Results[0].Units != "mg/dL" || len(rep.Results[0].Comment) != 0 {
			t.Fatalf("lost wrapped value: %+v", rep.Results)
		}
	}
}

// cells leaves out a token it puts in no field: a third token in the result
// column, a second unit, a second reference interval. Only the rebuild check
// (whole) refuses those rows, so this test fails if the check goes.
func TestRebuildRefusesUnreadToken(t *testing.T) {
	for label, l := range map[string]string{
		"third result token": pad(pad(pad(pad("", 6)+"Glucose", 57)+"100", 68)+"High", 75) + "H",
		// Within alignedWithin of the column name, so only whole() can refuse.
		"second unit":     pad(pad(pad(pad(pad("", 6)+"Glucose", 57)+"100", 140)+"g", 143)+"x", 160) + "70-99",
		"second interval": pad(pad(pad(pad(pad("", 6)+"Glucose", 57)+"100", 140)+"mg/dL", 158)+"7", 161) + "5",
		"value line":      row("Glucose", "01", "-", "", "", "") + "\n" + pad(pad(pad(pad("", 57)+"100", 68)+"High", 75)+"L", 140) + "mg/dL",
	} {
		t.Run(label, func(t *testing.T) {
			mustRefuse(t, ambiguityReport(l), "is in no field of the result")
		})
	}
	cols, ok := parseHeader(tokens(header()))
	if !ok {
		t.Fatal("fixture header")
	}
	got, err := cols.cells([]token{{57, "100"}, {68, "High"}, {140, "mg/dL"}, {160, "70-99"}})
	if err != nil || got.value != "100" || got.flag != "High" || got.units != "mg/dL" || got.ref != "70-99" {
		t.Fatalf("cells %+v: %v", got, err)
	}
}
