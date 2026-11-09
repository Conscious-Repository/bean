package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// testdata/real-layout.txt is the layout of a real Labcorp report, column for
// column: every line has the length it had, and every token starts where it
// did. Names, ids, dates, values, flags and the lab's comment prose are
// synthetic (DECISIONS 230). The grammar of DECISIONS 222 came from a
// synthetic fixture and refused the real report at line 6; this test keeps
// it reading the real one.
func realLayout(t *testing.T) *Report {
	t.Helper()
	b, err := os.ReadFile("testdata/real-layout.txt")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := parseReport(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestRealLayoutReads(t *testing.T) {
	rep := realLayout(t)
	want := strings.Fields(`wbc rbc hemoglobin hematocrit mcv mch mchc rdw platelets neutrophils lymphs
		monocytes eos basos neutrophils-absolute lymphs-absolute monocytes-absolute eos-absolute
		baso-absolute immature-granulocytes immature-grans-abs glucose bun creatinine egfr
		bun-creatinine-ratio sodium potassium chloride carbon-dioxide-total calcium protein-total
		albumin globulin-total bilirubin-total alkaline-phosphatase ast-sgot alt-sgpt
		cholesterol-total triglycerides hdl-cholesterol vldl-cholesterol-cal ldl-chol-calc-nih
		iron-bind-cap-tibc uibc iron iron-saturation lh fsh testosterone free-testosterone-direct
		lipoprotein-a c-reactive-protein-cardiac homocyst-e-ine estradiol vitamin-d-25-hydroxy
		cortisol prolactin tsh prostate-specific-ag t4-free-direct hemoglobin-a1c uric-acid ggt
		apolipoprotein-b creatine-kinase-total triiodothyronine-t3-free vitamin-b12 progesterone
		insulin ferritin`)
	if len(rep.Results) != len(want) {
		t.Fatalf("%d results, want %d", len(rep.Results), len(want))
	}
	by := map[string]*Result{}
	for i, r := range rep.Results {
		if got := slug(r.Name); got != want[i] {
			t.Errorf("result %d: %s, want %s", i, got, want[i])
		}
		by[slug(r.Name)] = r
	}
	// Two tests the lab did not perform: no value, and its reason.
	for _, name := range []string{"glucose", "potassium"} {
		r := by[name]
		if r.Value != "" || !notPerformed(r.Comment[0]) || r.Ref == "" {
			t.Errorf("%s: %+v", name, *r)
		}
	}
	for name, r := range by {
		if name != "glucose" && name != "potassium" && r.Value == "" {
			t.Errorf("%s has no value", name)
		}
	}
	// Centered units and reference intervals, odd shapes included.
	for name, want := range map[string][2]string{
		"wbc":                  {"x10E3/uL", "3.4-10.8"},
		"mcv":                  {"fL", "79-97"},
		"hematocrit":           {"%", "37.5-51.0"},
		"egfr":                 {"mL/min/1.73", ">59"},
		"bun-creatinine-ratio": {"", "9-20"},
		"neutrophils":          {"%", "Not Estab."},
		"lipoprotein-a":        {"nmol/L", "<75.0"},
		"tsh":                  {"uIU/mL", "0.450-4.500"},
	} {
		if r := by[name]; r.Units != want[0] || r.Ref != want[1] {
			t.Errorf("%s: units %q ref %q, want %q %q", name, r.Units, r.Ref, want[0], want[1])
		}
	}
	// Lab codes from the list: 01 and 02, and none for calculated tests.
	if rep.Labs["01"] != "Labcorp San Diego" || rep.Labs["02"] != "Labcorp Burlington" || rep.LabsTorn {
		t.Errorf("labs %v torn %v", rep.Labs, rep.LabsTorn)
	}
	for name, code := range map[string]string{"wbc": "01", "free-testosterone-direct": "02", "lipoprotein-a": "02", "egfr": "", "globulin-total": "", "ldl-chol-calc-nih": ""} {
		if by[name].LabCode != code {
			t.Errorf("%s: lab code %q, want %q", name, by[name].LabCode, code)
		}
	}
	// Comments under a result, from the comment column, in any shape: a
	// "Note:" block, a risk table, a "Please Note:" row's text.
	for _, name := range []string{"lipoprotein-a", "c-reactive-protein-cardiac", "apolipoprotein-b", "hemoglobin-a1c", "testosterone"} {
		if len(by[name].Comment) == 0 {
			t.Errorf("%s has no comment", name)
		}
	}
	if len(rep.Notes) != 4 { // the four lines of the report's "Please note"
		t.Errorf("report notes: %d", len(rep.Notes))
	}
	if rep.Specimen != "000-000-0000-0" || rep.Collected != "01/02/2025 0815" || rep.Reported != "01/09/2025" || rep.Fasting != "Not Given" {
		t.Errorf("fields: %q %q %q %q", rep.Specimen, rep.Collected, rep.Reported, rep.Fasting)
	}
}

// When the report says the collection time was not on the requisition, the
// time it prints may be the receipt time: only the day is recorded, and the
// printed time is kept.
func TestCollectionTimeUnsureKeepsDay(t *testing.T) {
	rep := realLayout(t)
	loc, _ := time.LoadLocation("America/Chicago")
	rep.Notes = nil
	if obs, _ := rep.observed(loc); obs["precision"] != "minute" {
		t.Errorf("without the note: %v", obs)
	}
	rep.Notes = []string{"The date and/or time of collection was not indicated on the requisition as required by state and federal law."}
	obs, raw := rep.observed(loc)
	if obs["precision"] != "day" || obs["start"] != "2025-01-02T00:00:00Z" || raw != "01/02/2025 0815" {
		t.Errorf("with the note: %v %q", obs, raw)
	}
}

// With the lab's note and the same collection and receipt day, the day
// itself may be the receipt day, and every reading says so.
func TestCollectionDayMayBeReceipt(t *testing.T) {
	rep := realLayout(t)
	rep.Notes = []string{"The date and/or time of collection was not indicated on the requisition."}
	if rep.collectionDayUnsure() {
		t.Fatal("unsure with different days")
	}
	rep.Received = rep.Collected[:10]
	if !rep.collectionDayUnsure() {
		t.Fatal("not unsure with the same day")
	}
	if recs := rep.records("", nil); recs[0]["context"].(map[string]any)["collection_day"] == nil {
		t.Errorf("no collection_day in context: %v", recs[0]["context"])
	}
}
