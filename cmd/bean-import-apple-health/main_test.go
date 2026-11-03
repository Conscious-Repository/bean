package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Conscious-Repository/bean/core"
)

const testBlob = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func sampleRecord(kind, value, unit string) string {
	return `<Record type="` + kind + `" sourceName="PRIVATE-SOURCE" sourceVersion="1" device="PRIVATE-DEVICE" unit="` + unit + `" creationDate="2026-10-01 10:01:00 -0500" startDate="2026-10-01 10:00:00 -0500" endDate="2026-10-01 10:01:00 -0500" value="` + value + `"><MetadataEntry key="private-key" value="PRIVATE-METADATA"/></Record>`
}

func healthXML(records ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE HealthData [<!ELEMENT HealthData ANY>]><HealthData locale="en_US"><Me HKCharacteristicTypeIdentifierDateOfBirth="PRIVATE-DOB"/>` + strings.Join(records, "") + `<ActivitySummary dateComponents="2026-10-01" activeEnergyBurned="30"/></HealthData>`
}

func writeXML(t *testing.T, text string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "export.xml")
	if err := os.WriteFile(p, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func decodeRows(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	var rows []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal(line, &row); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	return rows
}

func TestArchiveDefaultAndPrivacy(t *testing.T) {
	text := healthXML(sampleRecord("HKQuantityTypeIdentifierStepCount", "12", "count"), sampleRecord("FutureUnknownType", "opaque", "u"))
	var out bytes.Buffer
	if err := convertFile(writeXML(t, text), &out, options{blob: testBlob}); err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out.Bytes())
	if len(rows) != 1 {
		t.Fatalf("got %d records", len(rows))
	}
	r := rows[0]
	if r["type"] != "apple-health.export" || r["value"].(map[string]any)["blob"] != testBlob {
		t.Fatalf("bad archive: %v", r)
	}
	c := r["context"].(map[string]any)
	sum := sha256.Sum256([]byte(text))
	if c["sha256"] != hex.EncodeToString(sum[:]) || c["bytes"] != float64(len(text)) || c["record_count"] != float64(2) || c["assurance"] != "unsigned-export" || c["unmapped_record_count"] != float64(1) {
		t.Fatalf("bad context: %v", c)
	}
	counts := c["record_types"].(map[string]any)
	if c["activity_summary_count"] != float64(1) {
		t.Fatalf("activity summary count: %v", c)
	}
	if counts["HKQuantityTypeIdentifierStepCount"] != float64(1) || counts["FutureUnknownType"] != float64(1) {
		t.Fatalf("bad counts: %v", counts)
	}
	if strings.Contains(out.String(), "PRIVATE-") || strings.Contains(out.String(), "private-key") {
		t.Fatal("private source metadata copied")
	}
}

func TestRecordsPreserveUnitsIntervalsAndTupleIdentity(t *testing.T) {
	step := sampleRecord("HKQuantityTypeIdentifierStepCount", "12", "count")
	text := healthXML(step, step, sampleRecord("HKQuantityTypeIdentifierDistanceWalkingRunning", "0.25", "mi"), sampleRecord("HKQuantityTypeIdentifierBodyMass", "70", "kg"), sampleRecord("FutureUnknownType", "opaque", "u"))
	var out bytes.Buffer
	if err := convertFile(writeXML(t, text), &out, options{blob: testBlob, records: true}); err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out.Bytes())
	if len(rows) != 5 {
		t.Fatalf("got %d records", len(rows))
	}
	expected := []string{"apple-health.export", "apple-health.steps", "apple-health.steps", "apple-health.distance-walking-running", "apple-health.body-mass"}
	for i, r := range rows {
		if r["type"] != expected[i] {
			t.Fatalf("row %d: %v", i, r)
		}
	}
	if rows[3]["value"].(map[string]any)["unit"] != "mi" {
		t.Fatal("original unit changed")
	}
	for i, r := range rows[1:] {
		obs := r["times"].(map[string]any)["observed"].(map[string]any)
		if obs["start"] != "2026-10-01T10:00:00-05:00" || obs["end"] != "2026-10-01T10:01:00-05:00" {
			t.Fatalf("interval lost: %v", obs)
		}
		c := r["context"].(map[string]any)
		if c["source_record_index"] != float64(i+1) || c["apple_type"] == nil {
			t.Fatalf("source locator: %v", c)
		}
		src := r["source"].(map[string]any)
		occurrence := 0
		if i == 1 {
			occurrence = 1
		}
		want := core.RefInputTuple(src["system"].(string), r["type"].(string), obs, r["value"], occurrence)
		if !bytes.Equal(core.Canonical(src["ref_input"]), core.Canonical(want)) {
			t.Fatalf("not tuple identity: %v", src)
		}
	}
	if strings.Contains(out.String(), "PRIVATE-") {
		t.Fatal("private metadata copied")
	}
	// Same source rows in a changed surrounding export retain their tuple refs.
	var again bytes.Buffer
	if err := convertFile(writeXML(t, healthXML(step, step)), &again, options{blob: testBlob, records: true}); err != nil {
		t.Fatal(err)
	}
	repeated := decodeRows(t, again.Bytes())
	for i := 1; i <= 2; i++ {
		if !reflect.DeepEqual(rows[i]["source"].(map[string]any)["ref_input"], repeated[i]["source"].(map[string]any)["ref_input"]) {
			t.Fatal("overlap identity changed")
		}
	}
}

func TestTypesFilter(t *testing.T) {
	text := healthXML(sampleRecord("HKQuantityTypeIdentifierStepCount", "12", "count"), sampleRecord("HKQuantityTypeIdentifierBodyMass", "70", "kg"))
	var out bytes.Buffer
	if err := convertFile(writeXML(t, text), &out, options{blob: testBlob, records: true, types: "apple-health.steps"}); err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out.Bytes())
	if len(rows) != 2 || rows[1]["type"] != "apple-health.steps" {
		t.Fatalf("filter: %v", rows)
	}
}

func TestInvalidInputHasZeroOutput(t *testing.T) {
	valid := sampleRecord("HKQuantityTypeIdentifierStepCount", "12", "count")
	cases := map[string]string{
		"malformed":           healthXML(valid) + `<broken`,
		"second-root":         healthXML(valid) + `<HealthData/>`,
		"trailing-garbage":    healthXML(valid) + `garbage`,
		"duplicate-attribute": healthXML(strings.Replace(valid, `value="12"`, `value="12" value="13"`, 1)),
		"bad-start":           healthXML(valid, strings.Replace(valid, `startDate="2026-10-01 10:00:00 -0500"`, `startDate="bad"`, 1)),
		"bad-end":             healthXML(valid, strings.Replace(valid, `endDate="2026-10-01 10:01:00 -0500"`, `endDate="bad"`, 1)),
		"bad-number":          healthXML(valid, sampleRecord("HKQuantityTypeIdentifierStepCount", "NaN", "count")),
		"reversed-interval":   healthXML(valid, strings.Replace(valid, `endDate="2026-10-01 10:01:00 -0500"`, `endDate="2026-10-01 09:59:00 -0500"`, 1)),
		"wrong-root":          `<Other/>`,
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := convertFile(writeXML(t, text), &out, options{blob: testBlob, records: true}); err == nil {
				t.Fatal("accepted invalid input")
			}
			if out.Len() != 0 {
				t.Fatal("partial output on failure")
			}
		})
	}
}

func TestRequiredBlobAndOptions(t *testing.T) {
	p := writeXML(t, healthXML())
	for _, o := range []options{{}, {blob: "sha256:bad"}, {blob: testBlob, types: "apple-health.steps"}, {blob: testBlob, records: true, types: "not-supported"}} {
		var out bytes.Buffer
		if err := convertFile(p, &out, o); err == nil {
			t.Fatalf("accepted options: %+v", o)
		}
		if out.Len() != 0 {
			t.Fatal("partial output")
		}
	}
}

func TestNonzeroUnderflowCannotBecomeZeroReading(t *testing.T) {
	for _, value := range []string{"1e-400", "-1e-400", "0." + strings.Repeat("0", 400) + "1"} {
		t.Run(value[:min(len(value), 20)], func(t *testing.T) {
			p := writeXML(t, healthXML(sampleRecord("HKQuantityTypeIdentifierBodyMass", value, "kg")))
			var out bytes.Buffer
			if err := convertFile(p, &out, options{blob: testBlob, records: true}); err == nil {
				t.Fatal("nonzero source was silently converted into a zero measurement")
			}
			if out.Len() != 0 {
				t.Fatal("invalid measurement must not leave a partial import")
			}
			if err := convertFile(p, &out, options{blob: testBlob}); err != nil {
				t.Fatal("raw archive must remain preservable:", err)
			}
		})
	}
	for _, value := range []string{"0e-400", "-0.000", "5e-324", "0.0001"} {
		var out bytes.Buffer
		if err := convertFile(writeXML(t, healthXML(sampleRecord("HKQuantityTypeIdentifierBodyMass", value, "kg"))), &out, options{blob: testBlob, records: true}); err != nil {
			t.Fatalf("valid zero/representable number %s: %v", value, err)
		}
	}
}

func TestDecimalPrecisionCannotSilentlyChangeReading(t *testing.T) {
	for _, raw := range []string{"9007199254740990.5", "0.10000000000000001", "1.0000000000000001", "9007199254740991.1", "0e999999999999", "0e-1001", "0." + strings.Repeat("0", 255)} {
		t.Run(raw, func(t *testing.T) {
			p := writeXML(t, healthXML(sampleRecord("HKQuantityTypeIdentifierBodyMass", raw, "kg")))
			var out bytes.Buffer
			if err := convertFile(p, &out, options{blob: testBlob, records: true}); err == nil {
				t.Fatal("source decimal silently changed in the normalized reading")
			}
			if out.Len() != 0 {
				t.Fatal("partial output on precision failure")
			}
			if err := convertFile(p, &out, options{blob: testBlob}); err != nil {
				t.Fatal("source must still be archivable:", err)
			}
		})
	}
	for _, raw := range []string{"0.1", "1.000", "+12.0", "1.2e1", "-0e1000", "9007199254740991"} {
		var out bytes.Buffer
		if err := convertFile(writeXML(t, healthXML(sampleRecord("HKQuantityTypeIdentifierBodyMass", raw, "kg"))), &out, options{blob: testBlob, records: true}); err != nil {
			t.Fatalf("equivalent decimal %s rejected: %v", raw, err)
		}
	}
}

func TestXMLDeclarationMustBeAtDocumentStart(t *testing.T) {
	valid := sampleRecord("HKQuantityTypeIdentifierStepCount", "12", "count")
	declaration := `<?xml version="1.0" encoding="UTF-8"?>`
	for _, text := range []string{
		declaration + healthXML(valid),
		`<HealthData>` + valid + declaration + `</HealthData>`,
		healthXML(valid) + declaration,
		" \n" + declaration + `<HealthData/>`,
		`<?XML version="1.0"?><HealthData/>`,
	} {
		var out bytes.Buffer
		if err := convertFile(writeXML(t, text), &out, options{blob: testBlob, records: true}); err == nil {
			t.Fatal("accepted misplaced or reserved XML declaration")
		}
		if out.Len() != 0 {
			t.Fatal("malformed document emitted claims")
		}
	}
}

func TestDuplicateDOCTYPEHasZeroOutput(t *testing.T) {
	text := `<!DOCTYPE HealthData [<!ELEMENT HealthData ANY>]>` +
		`<!DOCTYPE HealthData [<!ELEMENT HealthData ANY>]><HealthData/>`
	var out bytes.Buffer
	if err := convertFile(writeXML(t, text), &out, options{blob: testBlob, records: true}); err == nil {
		t.Fatal("accepted multiple document type declarations")
	}
	if out.Len() != 0 {
		t.Fatal("malformed document emitted an archive claim")
	}
}

func TestArchiveRetainsSourceAnomaliesWithoutMeasurementClaims(t *testing.T) {
	text := healthXML(sampleRecord("HKQuantityTypeIdentifierStepCount", "not-a-number", "count"))
	var out bytes.Buffer
	if err := convertFile(writeXML(t, text), &out, options{blob: testBlob}); err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out.Bytes())
	if len(rows) != 1 || rows[0]["type"] != "apple-health.export" {
		t.Fatal("archive mode emitted measurement")
	}
}

type zipMember struct {
	name, text string
	mode       os.FileMode
}

func writeZIP(t *testing.T, members ...zipMember) string {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, m := range members {
		h := &zip.FileHeader{Name: m.name, Method: zip.Deflate}
		if m.mode != 0 {
			h.SetMode(m.mode)
		}
		f, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte(m.text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "export.zip")
	if err := os.WriteFile(p, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestZIPSelectionAndUnsafeMembers(t *testing.T) {
	text := healthXML(sampleRecord("HKQuantityTypeIdentifierStepCount", "12", "count"))
	p := writeZIP(t, zipMember{name: "apple_health_export/export.xml", text: text}, zipMember{name: "apple_health_export/export_cda.xml", text: "ignored"})
	var out bytes.Buffer
	if err := convertFile(p, &out, options{blob: testBlob}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	c := decodeRows(t, out.Bytes())[0]["context"].(map[string]any)
	if c["sha256"] != hex.EncodeToString(sum[:]) || c["bytes"] != float64(len(data)) {
		t.Fatal("digest must identify original ZIP")
	}
	for name, members := range map[string][]zipMember{
		"traversal": {{name: "../export.xml", text: text}},
		"absolute":  {{name: "/export.xml", text: text}},
		"symlink":   {{name: "export.xml", text: text, mode: os.ModeSymlink | 0777}},
		"duplicate": {{name: "export.xml", text: text}, {name: "other/export.xml", text: text}},
		"missing":   {{name: "unrelated.xml", text: text}},
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := convertFile(writeZIP(t, members...), &out, options{blob: testBlob}); err == nil {
				t.Fatal("accepted unsafe ZIP")
			}
			if out.Len() != 0 {
				t.Fatal("partial output")
			}
		})
	}
}

func TestZIPChecksumFailureHasZeroOutput(t *testing.T) {
	p := writeZIP(t, zipMember{name: "export.xml", text: healthXML(sampleRecord("HKQuantityTypeIdentifierStepCount", "12", "count"))})
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	index := bytes.Index(data, []byte{'P', 'K', 1, 2})
	if index < 0 {
		t.Fatal("missing ZIP central directory")
	}
	data[index+16] ^= 1 // Alter the central-directory CRC, leaving XML intact.
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := convertFile(p, &out, options{blob: testBlob, records: true}); err == nil {
		t.Fatal("accepted corrupt ZIP checksum")
	}
	if out.Len() != 0 {
		t.Fatal("partial output before ZIP checksum validation")
	}
}

func TestBoundedInputs(t *testing.T) {
	t.Run("large-source", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "export.xml")
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(256<<20 + 1); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := convertFile(p, &out, options{blob: testBlob}); err == nil {
			t.Fatal("accepted oversized source")
		}
		if out.Len() != 0 {
			t.Fatal("partial output")
		}
	})
	t.Run("large-zip-declaration", func(t *testing.T) {
		p := writeZIP(t, zipMember{name: "export.xml", text: healthXML()})
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		index := bytes.Index(data, []byte{'P', 'K', 1, 2})
		if index < 0 {
			t.Fatal("missing central directory")
		}
		binary.LittleEndian.PutUint32(data[index+24:index+28], 256<<20+1)
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := convertFile(p, &out, options{blob: testBlob}); err == nil {
			t.Fatal("accepted oversized ZIP member")
		}
		if out.Len() != 0 {
			t.Fatal("partial output")
		}
	})
	for name, text := range map[string]string{
		"attributes": `<HealthData oversized="` + strings.Repeat("x", 65537) + `"/>`,
		"nesting":    `<HealthData>` + strings.Repeat("<Nested>", 33) + strings.Repeat("</Nested>", 33) + `</HealthData>`,
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := convertFile(writeXML(t, text), &out, options{blob: testBlob}); err == nil {
				t.Fatal("accepted unbounded structure")
			}
			if out.Len() != 0 {
				t.Fatal("partial output")
			}
		})
	}
}

// An archive describes source bytes, independent of which optional readings a
// particular invocation emitted. The archive dedupes across these invocations.
func TestArchiveIndependentOfReadingSelection(t *testing.T) {
	p := writeXML(t, healthXML(sampleRecord("HKQuantityTypeIdentifierStepCount", "12", "count")))
	var archive, readings bytes.Buffer
	if err := convertFile(p, &archive, options{blob: testBlob}); err != nil {
		t.Fatal(err)
	}
	if err := convertFile(p, &readings, options{blob: testBlob, records: true}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decodeRows(t, archive.Bytes())[0], decodeRows(t, readings.Bytes())[0]) {
		t.Fatal("archive metadata depends on reading selection")
	}
}

func TestReadingSpoolLimitHasZeroOutput(t *testing.T) {
	// A compact synthetic source expands substantially when each reading carries
	// provenance, a blob reference and a tuple dedupe input.
	record := `<Record type="HKQuantityTypeIdentifierStepCount" startDate="2026-10-01 10:00:00 -0500" endDate="2026-10-01 10:01:00 -0500" value="12" unit="count"/>`
	text := healthXML(strings.Repeat(record, 1000))
	p := writeXML(t, text)
	var full bytes.Buffer
	if err := convertFile(p, &full, options{blob: testBlob, records: true}); err != nil {
		t.Fatal(err)
	}
	t.Logf("synthetic XML %d bytes, JSONL %d bytes (%.1fx)", len(text), full.Len(), float64(full.Len())/float64(len(text)))
	if full.Len() < 3*len(text) {
		t.Fatal("fixture must exercise material output amplification")
	}
	var limited bytes.Buffer
	if err := convertFileBounded(p, &limited, options{blob: testBlob, records: true}, 4096); err == nil {
		t.Fatal("accepted reading spool above its limit")
	}
	if limited.Len() != 0 {
		t.Fatal("spool overflow emitted a partial import")
	}
	if err := convertFileBounded(p, &limited, options{blob: testBlob}, 4096); err != nil {
		t.Fatal("raw archive must remain available:", err)
	}
}

func TestRejectSpecialInputPromptly(t *testing.T) {
	if p := os.Getenv("BEAN_APPLE_TEST_FIFO"); p != "" {
		if err := convertFile(p, &bytes.Buffer{}, options{blob: testBlob}); err == nil {
			t.Fatal("accepted FIFO")
		}
		return
	}
	p := filepath.Join(t.TempDir(), "source.fifo")
	if err := syscall.Mkfifo(p, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRejectSpecialInputPromptly$")
	cmd.Env = append(os.Environ(), "BEAN_APPLE_TEST_FIFO="+p)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatal("opening a FIFO blocked instead of rejecting it")
	}
	if err != nil {
		t.Fatalf("child failed: %v: %s", err, output)
	}
}
