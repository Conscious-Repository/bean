package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

// FuzzAppleExport exercises only synthetic, bounded in-memory XML. Successful
// reading output must be canonical and pass Bean's ordinary signed-file checks.
func FuzzAppleExport(f *testing.F) {
	step := sampleRecord("HKQuantityTypeIdentifierStepCount", "12", "count")
	f.Add(healthXML(step, step))
	f.Add(healthXML(sampleRecord("HKCategoryTypeIdentifierSleepAnalysis", "HKCategoryValueSleepAnalysisAsleepCore", "")))
	f.Add(healthXML(sampleRecord("HKQuantityTypeIdentifierBodyMass", "1e-400", "kg")))
	f.Add(`<HealthData><Nested><Record/></Nested></HealthData>`)
	f.Add(healthXML() + `<broken`)
	f.Add(`<HealthData/>`)
	// A public synthetic signer, never an identity or key from a real log.
	signer := core.NewEd25519Signer(bytes.Repeat([]byte{0x42}, 32))
	dedupeKey := bytes.Repeat([]byte{0x24}, 32)
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 64<<10 {
			return
		}
		var output bytes.Buffer
		_, err := parseExport(strings.NewReader(text), &limitedOutput{writer: &output, remaining: 2 << 20}, options{blob: testBlob, records: true}, nil)
		if err != nil {
			return
		}
		occurrences := map[string]int{}
		for _, line := range bytes.Split(bytes.TrimSuffix(output.Bytes(), []byte{'\n'}), []byte{'\n'}) {
			if len(line) == 0 {
				continue
			}
			parsed, err := core.Parse(line)
			if err != nil || !bytes.Equal(core.Canonical(parsed), line) {
				t.Fatal("successful parse emitted noncanonical JSON")
			}
			record := parsed.(map[string]any)
			source := record["source"].(map[string]any)
			times := record["times"].(map[string]any)
			key := string(core.Canonical([]any{record["type"], times["observed"], record["value"]}))
			want := core.RefInputTuple(system, record["type"].(string), times["observed"], record["value"], occurrences[key])
			if !bytes.Equal(core.Canonical(source["ref_input"]), core.Canonical(want)) {
				t.Fatal("reading tuple identity or occurrence is inconsistent")
			}
			occurrences[key]++
			source["ref"] = core.DedupeRef(dedupeKey, source["ref_input"].([]any))
			delete(source, "ref_input")
			record["v"] = float64(0)
			record["nonce"] = strings.Repeat("0", 32)
			record["subject"] = "sha256:" + strings.Repeat("0", 64)
			record["author"], record["kid"] = signer.Public(), signer.Public()
			times["imported"] = "2026-11-02T00:00:00Z"
			id, signed, err := core.SignRecord(record, signer)
			if err != nil {
				t.Fatalf("reading cannot be signed: %v", err)
			}
			if checked := core.CheckFile(core.PathOf(id), signed); checked.Err != nil {
				t.Fatalf("reading fails Bean field verification: %v", checked.Err)
			}
		}
	})
}
