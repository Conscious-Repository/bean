package main

import (
	"strings"
	"testing"
)

// Allow-lists, not deny-lists (DECISIONS 219). Synthetic data only.

func TestOnlyPrintableASCIIIsRead(t *testing.T) {
	sodium := row("Sodium", "01", "140", "", "mmol/L", "134-144")
	// Any byte outside 0x20-0x7E in a line that is read is refused, wherever
	// it is: in a name, a value, a comment, a heading.
	for _, bad := range []string{
		row("Glucose", "01", "101", "", "mg/dL", "65-99") + "\t",
		row("Gluc\u00f6se", "01", "101", "", "mg/dL", "65-99"),
		row("Glucose", "01", "101\u00b5", "", "mg/dL", "65-99"),
		row("Glucose", "01", "101", "", "mg/dL", "65-99") + "\r ", // a lone CR
		comment("Roche Elecsys\u00ae method."),
		"Metabolic Panel \u2013 Basic",
	} {
		if _, err := parseReport(ambiguityReport(sodium, bad)); err == nil || !strings.Contains(err.Error(), "printable ASCII") {
			t.Errorf("%q: %v", bad, err)
		}
	}
	// The page header (the copyright line, the patient's name) and the
	// patient details after the end of the results are not read: they may
	// hold any character.
	text := strings.Join([]string{
		"M\u00fcller, Jos\u00e9                                         DOB: 01/01/1990                  Patient Report",
		"\u00a92025 Laboratory Corporation of America\u00ae Holdings",
		"Specimen ID: 123-456-7890-0  Date Collected: 10/19/2026 0800 Local",
		header(),
		"Metabolic Panel",
		sodium,
		"",
		"Performing Labs",
		"01: BN - Labcorp Burlington, 1447 York Court, Burlington, NC 27215",
		"Patient Details                     Physician Details",
		"M\u00fcller, Jos\u00e9                       Dr. Pe\u00f1a",
	}, "\n") + "\n"
	if _, err := parseReport(text); err != nil {
		t.Errorf("header and patient details: %v", err)
	}
	// A header field read from such a line must still be ASCII, and so must
	// a lab list line among the patient details.
	for _, bad := range []string{
		"Specimen ID: 123-456-7890-0\u200b      Ordering Physician: X DOE\n" + text,
		text + "02: BN - Labcorp Z\u00fcrich, 1 Main St\n",
	} {
		if _, err := parseReport(bad); err == nil {
			t.Errorf("accepted: %q", bad)
		}
	}
	// Before the end, "Patient Details" does not open the exemption.
	early := strings.Replace(text, "Metabolic Panel\n", "Patient Details\n"+comment("caf\u00e9")+"\n", 1)
	if _, err := parseReport(early); err == nil {
		t.Error("Patient Details before the end lets a non-ASCII line through")
	}
}

func TestNoValueNeedsNotPerformed(t *testing.T) {
	sodium := row("Sodium", "01", "140", "", "mmol/L", "134-144")
	for _, c := range []string{"Test not performed.", "TEST CANCELLED", "Test canceled: duplicate order", "Not performed"} {
		rep, err := parseReport(ambiguityReport(sodium, row("Glucose", "01", "-", "", "", ""), vals("", "", "", "70-99"), comment(c)))
		if err != nil {
			t.Errorf("%q: %v", c, err)
			continue
		}
		if g := rep.Results[1]; g.Name != "Glucose" || g.Value != "" {
			t.Errorf("%q: %+v", c, *g)
		}
	}
	// Any other comment, a phrase that is not whole, or none: refused.
	for _, c := range []string{"", "Specimen hemolyzed.", "Test not performedx", "Result: Test not performed", "Cancelledby lab"} {
		rows := []string{sodium, row("Glucose", "01", "-", "", "", "")}
		if c != "" {
			rows = append(rows, comment(c))
		}
		if _, err := parseReport(ambiguityReport(rows...)); err == nil || !strings.Contains(err.Error(), "not performed") {
			t.Errorf("%q: %v", c, err)
		}
	}
	// With the phrase, a name word that may be a value is still refused.
	for _, name := range []string{
		"Glucose 101", "Glucose High", "Glucose H", "Glucose mg/dL", "ANA Titer 1:40",
		"Urine Glucose 1+", "WBC -5", "Appearance Clear", "Vitamin A", "Vitamin D, 25-Hydroxy",
		"Culture TNP", "Hep B Surface Ab Immune",
	} {
		if _, err := parseReport(ambiguityReport(sodium, pad("", 6)+name, comment("Test not performed."))); err == nil || !strings.Contains(err.Error(), "may be the value") {
			t.Errorf("%q: %v", name, err)
		}
	}
	// The first word is the test, whatever it is; "(a)" and "Ab" are words.
	for _, name := range []string{"Lipoprotein (a)", "HCV Ab", "eGFR If NonAfricn Am", "1,25-Dihydroxy"} {
		if _, err := parseReport(ambiguityReport(sodium, pad("", 6)+name, comment("Test not performed."))); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
}

func TestAfterEndOnlyListsAndSentences(t *testing.T) {
	sodium := row("Sodium", "01", "140", "", "mmol/L", "134-144")
	end := func(tail ...string) string {
		return ambiguityReport(append([]string{sodium, "", "Disclaimer"}, tail...)...)
	}
	for _, tail := range [][]string{
		{pad("", 6) + "LDL Cholesterol  01"},                      // two tokens
		{pad("", 6) + "LDL Cholesterol 190"},                      // one token, a value in it
		{pad("", 6) + "LDL Cholesterol", vals("190", "", "", "")}, // a value under it
		{pad("", 6) + "LDL Cholesterol", "", comment("x")},        // a comment under it
		{"    1447 York Court, Burlington, NC 27215"},             // a wrapped address: refused too
	} {
		if _, err := parseReport(end(tail...)); err == nil || !strings.Contains(err.Error(), "would be lost") {
			t.Errorf("%q: %v", tail, err)
		}
	}
	for _, tail := range [][]string{
		{"The Previous Result is listed for the most recent test."},
		{"   See the Icon Legend for flags."},
		{pad("", 6) + "Flags", "Next sentence at the margin."},
		{"01: BN - Labcorp Burlington, 1447 York Court  Dir: A Person, MD"},
		{pad("", 150) + "Date Collected: 10/19/2026"},
	} {
		if _, err := parseReport(end(tail...)); err != nil {
			t.Errorf("%q: %v", tail, err)
		}
	}
}
