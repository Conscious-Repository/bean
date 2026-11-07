package main

import (
	"bytes"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

// Regression test from audit 8. Apple writes walking steadiness, like the *Percentage types, as a
// fraction with unit "%" (HKUnit.percent). The rule keys on the type name,
// so this one is stored as {"n":0.85,"unit":"%"}: 0.85 percent, not 85.
func TestWalkingSteadinessPercentConverted(t *testing.T) {
	text := healthXML(
		sampleRecord("HKQuantityTypeIdentifierAppleWalkingSteadiness", "0.85", "%"),
		sampleRecord("HKQuantityTypeIdentifierWalkingAsymmetryPercentage", "0.03", "%"),
	)
	var out bytes.Buffer
	if err := convertFile(writeXML(t, text), &out, options{blob: testBlob, records: true}); err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeRows(t, out.Bytes())[1:] {
		v := string(core.Canonical(r["value"]))
		t.Logf("%s %s", r["type"], v)
		if r["type"] == "apple-health.walking-steadiness" && (v != `{"n":0.85,"unit":"1"}` || r["context"].(map[string]any)["source_unit"] != "%") {
			t.Errorf("walking steadiness not stored as a fraction with unit 1 and source_unit %%: %s", v)
		}
	}
	// A unit "%" value that is not a fraction is refused, whatever the type.
	bad := healthXML(sampleRecord("HKQuantityTypeIdentifierAppleWalkingSteadiness", "85", "%"))
	if err := convertFile(writeXML(t, bad), &bytes.Buffer{}, options{blob: testBlob, records: true}); err == nil {
		t.Errorf("walking steadiness 85 %% accepted")
	}
}
