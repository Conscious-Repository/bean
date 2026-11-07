package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

// Check the same envelope fields Bean supplies, with only public synthetic keys.
func checkSignedLabPartial(row map[string]any, signer core.Signer) error {
	parsed, err := core.Parse(core.Canonical(row))
	if err != nil {
		return err
	}
	r := parsed.(map[string]any)
	source := r["source"].(map[string]any)
	source["ref"] = core.DedupeRef(bytes.Repeat([]byte{0x24}, 32), source["ref_input"].([]any))
	delete(source, "ref_input")
	r["v"] = float64(0)
	r["nonce"] = strings.Repeat("0", 32)
	r["subject"] = "sha256:" + strings.Repeat("0", 64)
	r["author"], r["kid"] = signer.Public(), signer.Public()
	r["times"].(map[string]any)["imported"] = "2026-11-02T00:00:00Z"
	id, signed, err := core.SignRecord(r, signer)
	if err != nil {
		return err
	}
	return core.CheckFile(core.PathOf(id), signed).Err
}

func TestAcceptedOutputFitsSignedRecordLimit(t *testing.T) {
	rep, err := parseReport(fixture())
	if err != nil {
		t.Fatal(err)
	}
	row := rep.records("", nil)[0]
	context := row["context"].(map[string]any)
	context["lab_comment"] = ""
	context["lab_comment"] = strings.Repeat("x", (1<<20)-100-len(core.Canonical(row)))
	var out bytes.Buffer
	if err := writeRecords(&out, []map[string]any{row}); err == nil {
		if err := checkSignedLabPartial(row, core.NewEd25519Signer(bytes.Repeat([]byte{0x42}, 32))); err != nil {
			t.Fatalf("accepted output fails signed verification: %v", err)
		}
	} else if out.Len() != 0 {
		t.Fatal("size rejection left output prefix")
	}
}

func TestReorderedTableColumnsAreRejected(t *testing.T) {
	heading := pad("      Test", 56) + "Current Result and Flag"
	heading = pad(heading, 92) + "Previous Result and Date"
	heading = pad(heading, 139) + "Reference Interval"
	heading = pad(heading, 158) + "Units"
	text := strings.Join([]string{"Specimen ID: SYNTHETIC-A", "Date Collected: 01/02/2025", "Basic Panel", heading, row("Glucose", "", "100", "", "mg/dL", "70-99")}, "\n")
	if _, err := parseReport(text); err == nil {
		t.Fatal("unsupported column order silently lost result")
	}
	// A repeated malformed heading must not fall through to the previous page's
	// column layout and become a fabricated "Test" reading.
	text = strings.Join([]string{"Specimen ID: SYNTHETIC-A", "Date Collected: 01/02/2025", "Basic Panel", header(), row("First", "", "1", "", "mg/dL", "0-2"), heading, row("Second", "", "9", "", "mg/dL", "2-20")}, "\n")
	if _, err := parseReport(text); err == nil {
		t.Fatal("malformed repeated heading became a reading")
	}
}

func TestCollectionDateCannotMatchYearPrefix(t *testing.T) {
	text := strings.Join([]string{"Specimen ID: SYNTHETIC-A", "Date Collected: 01/02/20250", "Basic Panel", header(), row("Glucose", "", "100", "", "mg/dL", "70-99")}, "\n")
	if _, err := parseReport(text); err == nil {
		t.Fatal("five-digit year silently truncated")
	}
	if _, err := parseReport(fixture() + "\nDate Collected: 01/02/20250"); err == nil {
		t.Fatal("malformed later collection label ignored")
	}
}

func TestSpecimenSeparatorCannotCollideWithTestName(t *testing.T) {
	makeReport := func(specimen, name string) string {
		return strings.Join([]string{"Specimen ID: " + specimen, "Date Collected: 01/02/2025", "Basic Panel", header(), row(name, "", "1", "", "", "")}, "\n")
	}
	first, err := parseReport(makeReport("A/B", "C"))
	second, secondErr := parseReport(makeReport("A", "B/C"))
	if secondErr != nil {
		t.Fatalf("slash in real test name must remain allowed: %v", secondErr)
	}
	if err == nil {
		a := first.records("", nil)[0]["source"].(map[string]any)["ref_input"]
		b := second.records("", nil)[0]["source"].(map[string]any)["ref_input"]
		if bytes.Equal(core.Canonical(a), core.Canonical(b)) {
			t.Fatal("two distinct specimen/test pairs have identical native identity")
		}
		t.Fatal("ambiguous specimen separator accepted")
	}
}

func TestRoundedDecimalRemainsText(t *testing.T) {
	for _, value := range []string{"9007199254740990.5", "0.10000000000000001"} {
		rep := Report{Collected: "01/02/2025", Specimen: "SYNTHETIC-A", Results: []*Result{{Name: "Synthetic", Value: value}}}
		got := rep.records("", nil)[0]["value"].(map[string]any)
		if got["s"] != value {
			t.Fatalf("decimal changed into different numeric value: %v", got)
		}
	}
}

func TestEverySpecimenLabelIsChecked(t *testing.T) {
	for _, tail := range []string{
		"Specimen ID: 000-000-0000-0  Specimen ID: SYNTHETIC-DIFFERENT",
		"Specimen ID: 000-000-0000-0  Specimen ID:",
	} {
		if _, err := parseReport(fixture() + "\n" + tail); err == nil {
			t.Fatal("second specimen label ignored")
		}
	}
}
