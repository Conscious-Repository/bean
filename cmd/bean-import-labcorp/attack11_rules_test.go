package main

import (
	"strings"
	"testing"
)

// Each rule of DECISIONS 224 alone, so a later change to one rule cannot
// hide behind another. Synthetic data only.

// A flag word is never a value, so a flag alone in the result column leaves
// the row with no value, and a flag with no value is refused, even with the
// lab's "not performed" line under it.
func TestFlagIsNeverValue(t *testing.T) {
	for w := range flagWords {
		for _, s := range []string{w, strings.ToUpper(w[:1]) + w[1:], strings.ToUpper(w)} {
			if isValue(s) {
				t.Errorf("flag %q is a value", s)
			}
		}
	}
	for _, s := range []string{"H", "L", "A"} {
		if isValue(s) {
			t.Errorf("flag %q is a value", s)
		}
	}
	for _, flag := range []string{"Abnormal", "High", "H", "A"} {
		g := pad(pad("", 6)+"Glucose", 68) + flag
		mustRefuse(t, ambiguityReport(g), "has a flag and no value")
		mustRefuse(t, ambiguityReport(g, comment("Test not performed.")), "has a flag and no value")
	}
}

// A name word after the first that may be a reading is refused, also when
// the result column holds a value.
func TestNameWordMayBeReading(t *testing.T) {
	for _, name := range []string{
		"Urine Glucose 3+", "ANA Titer 1:160", "Base Excess -2.0", "HCV Ab Reactive", "Urine Protein Trace",
		"Factor 101", "Interleukin 101", "CA 19", "Cyfra 21", "Glucose 1,234", "Glucose 5.0-6.0", "HCV Non Reactive",
	} {
		t.Run(name, func(t *testing.T) {
			mustRefuse(t, nameReport(name, "4.2"), "in its name")
		})
	}
	for _, name := range []string{"Factor 8", "CA 15-3", "CA 27.29", "Interleukin 1"} {
		if _, err := parseReport(nameReport(name, "4.2")); err != nil {
			t.Errorf("%q refused: %v", name, err)
		}
	}
}

// A token that runs across the place where two columns meet is refused, on
// a row and on a value line (DECISIONS 230). One that lies inside a column
// is read in it.
func TestTokenAcrossColumnsRefused(t *testing.T) {
	cols, _ := parseHeader(tokens(header()))
	b := cols.bounds()
	for i := 2; i < len(b); i++ {
		v := pad(pad("", 57)+"100", b[i]-2) + "mg/dL" // starts 2 left of the boundary, ends right of it
		name := pad("", 6) + "Glucose"
		mustRefuse(t, ambiguityReport(name+v[len(name):]), "runs across two columns")
		// On a value line it is not read as values, so it is a comment, and
		// the row above it has no value: refused.
		mustRefuse(t, ambiguityReport(row("Glucose", "01", "-", "", "", ""), v), "has no value")
	}
	// A value that starts in the test column and ends in the result column.
	mustRefuse(t, ambiguityReport(pad(pad("", 6)+"Glucose", b[1]-2)+"1000"), "runs across two columns")
	// The fixture's columns (result 57, flag 68, units 140, range 160) pass.
	if _, err := parseReport(ambiguityReport(row("Glucose", "01", "100", "High", "mg/dL", "70-99"))); err != nil {
		t.Fatal(err)
	}
}

// A unit is one word with a letter, % or /: a reference interval in the
// units band is refused, not stored as the unit.
func TestUnitsShape(t *testing.T) {
	for _, u := range []string{"9-20", ">59", "<=0.5", "12", "0.450-4.500", "-"} {
		mustRefuse(t, ambiguityReport(row("Glucose", "01", "100", "", u, "")), "units column")
	}
	for _, u := range []string{"%", "mg/dL", "x10E3/uL", "mL/min/1.73", "ratio"} {
		if _, err := parseReport(ambiguityReport(row("Glucose", "01", "100", "", u, ""))); err != nil {
			t.Errorf("unit %q refused: %v", u, err)
		}
	}
}

// A name at the test column that ends in a colon is a label.
func TestColonNameRefused(t *testing.T) {
	for _, name := range []string{"Note:", "Comment:", "Glucose:"} {
		mustRefuse(t, ambiguityReport(row(name, "01", "5", "", "", "")), "ends in a colon")
	}
}

// No reading has the archive's type.
func TestArchiveTypeReserved(t *testing.T) {
	for _, name := range []string{"Report", "REPORT", "Report."} {
		mustRefuse(t, ambiguityReport(row(name, "01", "5", "", "", "")), "type of the kept report")
	}
}
