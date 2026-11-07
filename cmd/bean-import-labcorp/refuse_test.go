package main

import (
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

// Regression tests from the audit of 028a3a5. Synthetic data only. Each one
// failed before its fix.

func mustRefuse(t *testing.T, text, want string) {
	t.Helper()
	rep, err := parseReport(text)
	if err == nil {
		for _, r := range rep.records("", nil) {
			t.Logf("accepted %s %s", r["type"], core.Canonical(r["value"]))
		}
		t.Fatalf("accepted, want refusal %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("err %v, want %q", err, want)
	}
}

// A long name that runs into the value column is refused by where it ends,
// not by what it ends with: a flag stored as the value, a comment under the
// row, and a categorical value are all caught.
func TestNameIntoResultColumnRefused(t *testing.T) {
	name := pad("", 6) + strings.Repeat("Long Test Name ", 3)
	flagged := pad(pad(name+"Overrun 5.9", 68)+"High", 140) + "%"
	flagged = pad(flagged, 160) + "4.8-5.6"
	for label, rows := range map[string][]string{
		"flag as value": {flagged},
		"with comment":  {name + "Overrun 90", comment("Prediabetes: 5.7 - 6.4")},
		"categorical":   {name + "HCV Ab Non Reactive"},
	} {
		t.Run(label, func(t *testing.T) {
			mustRefuse(t, ambiguityReport(rows...), "runs into the result column")
		})
	}
	// A name that ends just short of the column is a name.
	short := pad("", 6) + strings.Repeat("x", 45)
	if _, err := parseReport(ambiguityReport(short, vals("1", "", "", ""))); err != nil {
		t.Errorf("short name: %v", err)
	}
}

// Without a Performing Labs list that reads whole, two digits at the end of
// a name stay in the name. A code in the code column is still a code.
func TestNoLabListKeepsTestNumber(t *testing.T) {
	start := []string{
		"Specimen ID: 123-456-7890-0  Date Collected: 10/19/2026 0800 Local",
		header(),
		"Cytokines",
		row("Interleukin 10", "", "42", "High", "pg/mL", "0-9"),
		row("Glucose", "01", "90", "", "mg/dL", "70-99"),
	}
	for label, tail := range map[string][]string{
		"no list":   nil,
		"torn list": {"", "Performing Labs", "01: BN - Labcorp Burlington", "    1447 York Court, Burlington, NC 27215"},
	} {
		t.Run(label, func(t *testing.T) {
			rep, err := parseReport(strings.Join(append(append([]string{}, start...), tail...), "\n") + "\n")
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			for _, r := range rep.Results {
				got[slug(r.Name)] = r.Value + "/" + r.LabCode
			}
			if got["interleukin-10"] != "42/" || got["glucose"] != "90/01" {
				t.Errorf("results: %v", got)
			}
		})
	}
}

// The same test twice collapses only when everything it keeps is the same.
func TestDuplicateWithOtherCommentOrLabRefused(t *testing.T) {
	for label, second := range map[string][]string{
		"comment": {row("Glucose", "01", "90", "", "mg/dL", "70-99"), comment("Specimen hemolyzed; interpret with caution.")},
		"lab":     {row("Glucose", "02", "90", "", "mg/dL", "70-99")},
	} {
		t.Run(label, func(t *testing.T) {
			text := strings.Join(append(append([]string{
				"Specimen ID: 123-456-7890-0  Date Collected: 10/19/2026 0800 Local",
				header(),
				"Panel A",
				row("Glucose", "01", "90", "", "mg/dL", "70-99"),
				"Panel B",
			}, second...),
				"",
				"Performing Labs",
				"01: BN - Labcorp Burlington, 1447 York Court, Burlington, NC 27215",
				"02: SO - Labcorp Other, 2 Main St, Other, CA 90000",
			), "\n") + "\n"
			mustRefuse(t, text, "map to one type")
		})
	}
}

// A row before the first column header cannot be read, so the report is
// refused instead of losing it.
func TestRowBeforeHeaderRefused(t *testing.T) {
	text := strings.Join([]string{
		"Specimen ID: 123-456-7890-0  Date Collected: 10/19/2026 0800 Local",
		"Basic Panel",
		row("Potassium", "01", "6.8", "High", "mmol/L", "3.5-5.2"),
		"",
		"Lipids",
		header(),
		row("Glucose", "01", "90", "", "mg/dL", "70-99"),
		"",
		"Performing Labs",
		"01: BN - Labcorp Burlington, 1447 York Court, Burlington, NC 27215",
	}, "\n") + "\n"
	mustRefuse(t, text, "before the column header")
}

// A reading names the kept report by digest, never in blobs, so purging the
// lab.report can erase the file.
func TestReadingsNameReportByDigest(t *testing.T) {
	rep, err := parseReport(fixture())
	if err != nil {
		t.Fatal(err)
	}
	sum := strings.Repeat("c", 64)
	for _, r := range rep.records(sum, nil) {
		if _, has := r["blobs"]; has || r["context"].(map[string]any)["report_sha256"] != sum {
			t.Fatalf("%s: blobs %v, context %v", r["type"], r["blobs"], r["context"])
		}
	}
	for _, r := range rep.records("", nil) {
		if _, has := r["context"].(map[string]any)["report_sha256"]; has {
			t.Fatalf("%s names a report that is not kept", r["type"])
		}
	}
}
