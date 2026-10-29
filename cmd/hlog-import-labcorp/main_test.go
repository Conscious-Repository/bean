package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Conscious-Repository/bean/core"
)

// The fixture is synthetic: a made-up person and values laid out as
// pdftotext -layout prints a Labcorp patient report. Columns follow the
// real report: name at 6, lab code near 35, result near 57, flag near 68,
// units near 140, reference interval near 160.

func pad(s string, col int) string {
	if len(s) >= col {
		return s + "  "
	}
	return s + strings.Repeat(" ", col-len(s))
}

func header() string {
	l := pad("", 6) + "Test"
	l = pad(l, 56) + "Current Result and Flag"
	l = pad(l, 92) + "Previous Result and Date"
	l = pad(l, 139) + "Units"
	return pad(l, 158) + "Reference Interval"
}

// row is a name line; when value is "-" the values go on the next line.
func row(name, code, value, flag, units, ref string) string {
	l := pad("", 6) + name
	if code != "" {
		l = pad(l, 35) + code
	}
	if value == "-" {
		return l + "\n" + vals("", "", units, ref)
	}
	return pad(l, 57) + vals(value, flag, units, ref)[57:]
}

func vals(value, flag, units, ref string) string {
	l := pad("", 57) + value
	if flag != "" {
		l = pad(l, 68) + flag
	}
	if units != "" {
		l = pad(l, 140) + units
	}
	if ref != "" {
		l = pad(l, 160) + ref
	}
	return strings.TrimRight(l, " ")
}

func comment(s string) string { return pad("", 49) + s }

func pageBreak(n int) string {
	return strings.Join([]string{
		pad("", 100) + "Date Created and Stored 01/09/25 1710 ET Final Report Page " + string(rune('0'+n)) + " of 3",
		"",
		"©2025 Laboratory Corporation of America® Holdings      This document contains private and confidential health information.",
		"All Rights Reserved - Enterprise Report Version 2.00   If you have received this document in error please call 800-000-0000",
		"Doe, Jane                                              DOB: 01/01/1990                  Patient Report",
		"Patient ID: 9999                                       Age: 35                          Account Number: 1111",
		"Specimen ID: 000-000-0000-0                            Sex: Female                      Ordering Physician: X DOE",
		"",
		pad("", 150) + "Date Collected: 01/02/2025",
		"",
	}, "\n")
}

func fixture() string {
	return strings.Join([]string{
		"Doe, Jane                                              DOB: 01/01/1990                  Patient Report",
		"Patient ID: 9999                                       Age: 35                          Account Number: 1111",
		"Specimen ID: 000-000-0000-0                            Sex: Female                      Ordering Physician: X DOE",
		"",
		"Date Collected: 01/02/2025                       Date Received: 01/03/2025          Date Reported: 01/09/2025     Fasting: Yes",
		"",
		"Ordered Items: Basic Panel; Lipids; Please note",
		"",
		pad("", 150) + "Date Collected: 01/02/2025",
		"",
		"Basic Panel",
		header(),
		row("WBC", "01", "-", "", "", ""),
		vals("6.0", "", "x10E3/uL", "3.4-10.8"),
		row("Glucose", "01", "-", "", "", ""),
		vals("", "", "", "70-99"),
		comment("Test not performed. Specimen was hemolyzed."),
		row("eGFR", "", "90", "", "mL/min/1.73", ">59"),
		row("BUN/Creatinine Ratio", "", "12", "", "", "9-20"),
		row("Chloride 01", "", "95", "Low", "mmol/L", "96-106"),
		row("TSH", "01", "-", "", "", ""),
		vals("1.190", "", "uIU/mL", "0.450-4.500"),
		row("Antibody Screen", "02", "Negative", "", "", "Negative"),
		"",
		pageBreak(1),
		"Basic Panel (Cont.)",
		row("Hemoglobin A1c", "01", "5.9", "High", "%", "4.8-5.6"),
		row("Please Note:", "01", "-", "", "", ""),
		comment("Prediabetes: 5.7 - 6.4"),
		"",
		"Lipids",
		header(),
		row("Lipoprotein (a)", "02", "-", "", "", ""),
		vals("80.1", "High", "nmol/L", "<75.0"),
		comment("Note:     Values greater than or equal to 75.0 nmol/L may"),
		"",
		pageBreak(2),
		"Lipids (Cont.)",
		comment("indicate an independent risk factor."),
		"",
		"Please note",
		header(),
		row("Please note", "01", "-", "", "", ""),
		comment("The date of receipt was used as the collection date."),
		"",
		"Disclaimer",
		"The Previous Result is listed for the most recent test.",
		"",
		"Performing Labs",
		"01: SO - Labcorp Example City, 1 Main St, Example, CA 90000 Dir: A Person, MD",
		"02: BN - Labcorp Other Town, 2 Main St, Other, NC 20000 Dir: B Person, MD",
		"Patient Details                     Physician Details                Specimen Details",
		"Doe, Jane                           X DOE                            Specimen ID: 000-000-0000-0",
		"                                                                     Date Collected: 01/02/2025 0815 Local",
		"Date of Birth: 01/01/1990                                            Date Reported: 01/09/2025 1708 ET",
	}, "\n")
}

func TestParseReport(t *testing.T) {
	rep, err := parseReport(fixture())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Specimen != "000-000-0000-0" || rep.Collected != "01/02/2025 0815" || rep.Reported != "01/09/2025" || rep.Fasting != "Yes" {
		t.Errorf("header = %+v", rep)
	}
	if rep.Labs["02"] != "Labcorp Other Town" {
		t.Errorf("labs = %v", rep.Labs)
	}
	type want struct{ name, panel, code, value, flag, units, ref, comment string }
	wants := []want{
		{"WBC", "Basic Panel", "01", "6.0", "", "x10E3/uL", "3.4-10.8", ""},
		{"Glucose", "Basic Panel", "01", "", "", "", "70-99", "Test not performed. Specimen was hemolyzed."},
		{"eGFR", "Basic Panel", "", "90", "", "mL/min/1.73", ">59", ""},
		{"BUN/Creatinine Ratio", "Basic Panel", "", "12", "", "", "9-20", ""},
		{"Chloride", "Basic Panel", "01", "95", "Low", "mmol/L", "96-106", ""},
		{"TSH", "Basic Panel", "01", "1.190", "", "uIU/mL", "0.450-4.500", ""},
		{"Antibody Screen", "Basic Panel", "02", "Negative", "", "", "Negative", ""},
		{"Hemoglobin A1c", "Basic Panel", "01", "5.9", "High", "%", "4.8-5.6", "Prediabetes: 5.7 - 6.4"},
		{"Lipoprotein (a)", "Lipids", "02", "80.1", "High", "nmol/L", "<75.0", "Note: Values greater than or equal to 75.0 nmol/L may indicate an independent risk factor."},
	}
	if len(rep.Results) != len(wants) {
		for _, r := range rep.Results {
			t.Logf("%+v", *r)
		}
		t.Fatalf("got %d results, want %d", len(rep.Results), len(wants))
	}
	for i, w := range wants {
		r := rep.Results[i]
		got := want{r.Name, r.Panel, r.LabCode, r.Value, r.Flag, r.Units, r.Ref, strings.Join(r.Comment, " ")}
		if got != w {
			t.Errorf("result %d:\n got %+v\nwant %+v", i, got, w)
		}
	}
	if len(rep.Notes) != 1 || !strings.HasPrefix(rep.Notes[0], "The date of receipt") {
		t.Errorf("notes = %q", rep.Notes)
	}
}

func TestRecords(t *testing.T) {
	rep, err := parseReport(fixture())
	if err != nil {
		t.Fatal(err)
	}
	chi, _ := time.LoadLocation("America/Chicago")
	recs := rep.records("sha256:"+strings.Repeat("ab", 32), chi)
	byType := map[string]map[string]any{}
	for _, r := range recs {
		byType[r["type"].(string)] = r
	}
	check := func(typ, want string) {
		t.Helper()
		r := byType[typ]
		if r == nil {
			t.Fatalf("no %s", typ)
		}
		if got := string(core.Canonical(r["value"])); got != want {
			t.Errorf("%s value = %s, want %s", typ, got, want)
		}
	}
	check("lab.wbc", `{"n":6,"unit":"x10E3/uL"}`)
	check("lab.glucose", `{"n":null}`)
	check("lab.tsh", `{"n":1.19,"unit":"uIU/mL"}`)
	check("lab.antibody-screen", `{"s":"Negative"}`)
	check("lab.bun-creatinine-ratio", `{"n":12}`)

	tsh := byType["lab.tsh"]
	ctx := tsh["context"].(map[string]any)
	if ctx["raw_value"] != "1.190" || ctx["performing_lab"] != "Labcorp Example City" || ctx["report_note"] == nil || ctx["fasting"] != "Yes" {
		t.Errorf("tsh context = %v", ctx)
	}
	if obs := tsh["times"].(map[string]any)["observed"].(map[string]any); obs["start"] != "2025-01-02T08:15:00-06:00" || obs["precision"] != "minute" {
		t.Errorf("observed = %v", obs)
	}
	for _, r := range recs {
		// The specimen id is the native id. It travels to hlog append in
		// source.ref_input, which append turns into the HMAC dedupe ref
		// and removes before signing (SPEC.md 9.4).
		src := r["source"].(map[string]any)
		if in := src["ref_input"].([]any); in[3] != "000-000-0000-0/"+r["context"].(map[string]any)["test"].(string) {
			t.Errorf("ref_input = %v", in)
		}
		stored := map[string]any{}
		for k, v := range r {
			stored[k] = v
		}
		stored["source"] = map[string]any{"system": src["system"], "adapter": src["adapter"]}
		b := string(core.Canonical(stored))
		for _, secret := range []string{"Doe", "01/01/1990", "9999", "000-000-0000-0"} {
			if strings.Contains(b, secret) {
				t.Errorf("%s leaks %q", r["type"], secret)
			}
		}
	}

	// Without a zone: the day only, raw time kept.
	day := rep.records("", nil)[0]
	if obs := day["times"].(map[string]any)["observed"].(map[string]any); obs["start"] != "2025-01-02T00:00:00Z" || obs["precision"] != "day" {
		t.Errorf("observed without tz = %v", obs)
	}
	if day["context"].(map[string]any)["raw_time"] != "01/02/2025 0815" {
		t.Errorf("raw_time = %v", day["context"])
	}
}

func TestNotAReport(t *testing.T) {
	if _, err := parseReport("hello\nworld\n"); err == nil {
		t.Error("parsed a non-report")
	}
}
