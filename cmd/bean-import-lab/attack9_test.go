package main

import (
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

// Attack 9. Synthetic data only. Each test fails when the attack works:
// a report is accepted and a value is lost, moved or misnamed.

func noValue(r map[string]any) bool {
	return string(core.Canonical(r["value"])) == `{"n":null}`
}

// valueLike sees only ASCII \s before the number and "$" at the very end.
// A Unicode space between name and value, or an invisible character after
// it, passes the check, and the value becomes part of the type.
func TestAttack9InvisibleCharsHideValue(t *testing.T) {
	c := func(x rune) string { return string(x) }
	for _, name := range []string{
		"Glucose" + c(0x2003) + "101", // em space
		"Glucose" + c(0x2007) + "101", // figure space
		"Glucose" + c(0x202f) + "101", // narrow no-break space
		"Glucose" + c(0x3000) + "101", // ideographic space
		"Glucose 101" + c(0x200b),     // zero-width space after the value
		"Glucose 101" + c(0xfeff),     // BOM after the value
		"Glucose 101" + c(0x0301),     // combining mark
		"Glucose 101" + c(0x0d) + " ", // lone CR (only CRLF is normalized)
	} {
		recs := logAccepted(t, ambiguityReport(pad("", 6)+name, row("Sodium", "01", "140", "", "mmol/L", "134-144")))
		for _, r := range recs {
			if strings.HasPrefix(r["type"].(string), "lab.glucose") && noValue(r) {
				t.Errorf("%q: value 101 lost into type %s", name, r["type"])
			}
		}
	}
}

// valueLike checks only the last word. A compressed row whose value is
// followed by its flag or unit, one space apart, keeps the value in the
// name. So do common categorical results missing from resultWords.
func TestAttack9LastWordNotAValue(t *testing.T) {
	for _, name := range []string{
		"Glucose 101 High",
		"Glucose 101 H",
		"Glucose 101 mg/dL",
		"Glucose 101 mg/dL 65-99",
		"Urine Protein Trace",
		"Urine Glucose 1+",
		"WBC Urine 0-5",
		"ANA Titer 1:40",
		"Appearance Clear",
		"Hep B Surface Ab Immune",
		"Covid PCR Inconclusive",
		"Culture TNP",
		"Vitamin D 1,234",
	} {
		recs := logAccepted(t, ambiguityReport(pad("", 6)+name, row("Sodium", "01", "140", "", "mmol/L", "134-144")))
		for _, r := range recs {
			if r["type"] != "lab.sodium" && noValue(r) {
				t.Errorf("%q: stored as %s with no value", name, r["type"])
			}
		}
	}
}

// Columns are byte offsets, but pdftotext -layout pads by characters. A
// name with non-ASCII characters shifts every later token right by the
// extra bytes: the unit lands in the reference column and is lost from
// the value.
func TestAttack9MultibyteNameShiftsColumns(t *testing.T) {
	name := "Glucose ①②③④⑤" // 5 three-byte characters: 10 extra bytes
	// Pad by characters, as pdftotext does.
	padc := func(s string, col int) string { return s + strings.Repeat(" ", col-len([]rune(s))) }
	l := padc(padc(padc(padc(pad("", 6)+name, 35)+"01", 57)+"101", 140)+"mg/dL", 160) + "65-99"
	t.Logf("row (chars): %q", l)
	recs := logAccepted(t, ambiguityReport(l, row("Sodium", "01", "140", "", "mmol/L", "134-144")))
	for _, r := range recs {
		if strings.HasPrefix(r["type"].(string), "lab.glucose") {
			v := r["value"].(map[string]any)
			if v["unit"] != "mg/dL" {
				t.Errorf("unit lost: value %s, reference_interval %q", core.Canonical(v), r["context"].(map[string]any)["reference_interval"])
			}
		}
	}
}

// After the end heading, a row whose value sits left of the result column
// is dropped in silence. The same row before the heading is refused.
func TestAttack9RowAfterEndLeftOfColumnDropped(t *testing.T) {
	compressed := pad(pad("", 6)+"LDL Cholesterol", 30) + "190  High  mg/dL  0-99"
	if _, err := parseReport(ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), compressed)); err == nil {
		t.Fatal("control: before the end the row is accepted")
	} else {
		t.Logf("before the end: refused: %v", err)
	}
	for _, r := range []string{compressed, pad("", 6) + "LDL Cholesterol 190 High mg/dL 0-99"} {
		recs := logAccepted(t, ambiguityReport(row("Sodium", "01", "140", "", "mmol/L", "134-144"), "", "Disclaimer", r))
		if recs != nil {
			t.Errorf("after Disclaimer %q dropped: %d reading(s) written, no LDL", r, len(recs))
		}
	}
}
