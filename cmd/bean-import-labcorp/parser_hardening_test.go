package main

import (
	"reflect"
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

func TestFailedFillDoesNotMutateResult(t *testing.T) {
	cols, ok := headerColumns(header())
	if !ok {
		t.Fatal("fixture header")
	}
	before := Result{Name: "Glucose", Ref: "original reference"}
	got := before
	// Three tokens in the current-result region overflow value/flag slots.
	if got.fill([]token{{57, "100"}, {68, "High"}, {75, "unexpected"}}, cols) {
		t.Fatal("accepted ambiguous result columns")
	}
	if !reflect.DeepEqual(got, before) {
		t.Fatalf("failed parse mutated row: %+v", got)
	}
}
