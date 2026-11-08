package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

// Regression tests from the audit of 028a3a5. Synthetic data only.

func bloodPressure() string {
	return `<Correlation type="HKCorrelationTypeIdentifierBloodPressure" sourceName="PRIVATE-SOURCE" creationDate="2026-10-01 10:01:00 -0500" startDate="2026-10-01 10:00:00 -0500" endDate="2026-10-01 10:00:00 -0500">` +
		sampleRecord("HKQuantityTypeIdentifierBloodPressureSystolic", "120", "mmHg") +
		sampleRecord("HKQuantityTypeIdentifierBloodPressureDiastolic", "80", "mmHg") + `</Correlation>`
}

// Apple writes blood pressure as a Correlation with its Records inside. The
// export is kept and the Records are counted; no reading is made of them.
func TestCorrelationKeptAndCounted(t *testing.T) {
	text := healthXML(sampleRecord("HKQuantityTypeIdentifierStepCount", "12", "count"), bloodPressure())
	for _, o := range []options{{blob: testBlob}, {blob: testBlob, records: true}} {
		var out bytes.Buffer
		if err := convertFile(writeXML(t, text), &out, o); err != nil {
			t.Fatalf("records %v: %v", o.records, err)
		}
		rows := decodeRows(t, out.Bytes())
		ctx := rows[0]["context"].(map[string]any)
		if ctx["correlation_count"] != float64(1) || ctx["correlation_record_count"] != float64(2) || ctx["record_count"] != float64(3) {
			t.Errorf("counts: %v", ctx)
		}
		if want := map[bool]int{false: 1, true: 2}[o.records]; len(rows) != want {
			t.Errorf("records %v: %d rows", o.records, len(rows))
		}
		if strings.Contains(out.String(), "PRIVATE-") {
			t.Errorf("leaked: %s", out.String())
		}
	}
	// A Record two levels down that is not in a Correlation is still refused.
	bad := healthXML(`<Workout>` + sampleRecord("HKQuantityTypeIdentifierStepCount", "12", "count") + `</Workout>`)
	if err := convertFile(writeXML(t, bad), &bytes.Buffer{}, options{blob: testBlob}); err == nil {
		t.Error("record inside a Workout accepted")
	}
}

// A selected reading inside a Correlation is refused, not dropped.
func TestSelectedReadingInCorrelationRefused(t *testing.T) {
	inside := `<Correlation type="HKCorrelationTypeIdentifierFood" startDate="2026-10-01 10:00:00 -0500" endDate="2026-10-01 10:00:00 -0500">` +
		sampleRecord("HKQuantityTypeIdentifierBodyMass", "70", "kg") + `</Correlation>`
	p := writeXML(t, healthXML(inside))
	var out bytes.Buffer
	if err := convertFile(p, &out, options{blob: testBlob, records: true}); err == nil || !strings.Contains(err.Error(), "Correlation") || out.Len() != 0 {
		t.Errorf("err %v, %d bytes", err, out.Len())
	}
	if err := convertFile(p, &out, options{blob: testBlob, records: true, types: "apple-health.steps"}); err != nil {
		t.Errorf("not selected: %v", err)
	}
}

// Apple's percentage types carry a fraction with unit "%". The reading says
// unit "1"; its ref keeps Apple's value, so an older reading with "%" is
// reported as changed, not added again.
func TestPercentageIsAFraction(t *testing.T) {
	kind := "HKQuantityTypeIdentifierWalkingDoubleSupportPercentage"
	var out bytes.Buffer
	if err := convertFile(writeXML(t, healthXML(sampleRecord(kind, "0.272", "%"))), &out, options{blob: testBlob, records: true}); err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out.Bytes())
	if v := string(core.Canonical(rows[1]["value"])); v != `{"n":0.272,"unit":"1"}` {
		t.Errorf("value %s", v)
	}
	if rows[1]["context"].(map[string]any)["source_unit"] != "%" {
		t.Errorf("context %v", rows[1]["context"])
	}
	obs := rows[1]["times"].(map[string]any)["observed"]
	want := core.RefInputTuple(system, "apple-health.walking-double-support", obs, map[string]any{"n": 0.272, "unit": "%"}, 0)
	if got := rows[1]["source"].(map[string]any)["ref_input"]; !bytes.Equal(core.Canonical(got), core.Canonical(want)) {
		t.Errorf("ref_input %v", got)
	}
	for _, bad := range [][2]string{{"27.2", "%"}, {"0.272", "1"}, {"-0.1", "%"}} {
		if err := convertFile(writeXML(t, healthXML(sampleRecord(kind, bad[0], bad[1]))), &bytes.Buffer{}, options{blob: testBlob, records: true}); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

// A reading names the export by digest, never in blobs, so purging the
// apple-health.export can erase the file.
func TestReadingsNameExportByDigest(t *testing.T) {
	var out bytes.Buffer
	if err := convertFile(writeXML(t, healthXML(sampleRecord("HKQuantityTypeIdentifierStepCount", "12", "count"))), &out, options{blob: testBlob, records: true}); err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out.Bytes())
	sum := rows[0]["context"].(map[string]any)["sha256"]
	if _, has := rows[1]["blobs"]; has || sum == nil || rows[1]["context"].(map[string]any)["export_sha256"] != sum {
		t.Errorf("reading %v", rows[1])
	}
}
