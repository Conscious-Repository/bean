package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// R12-A2: the 64 KiB attribute bound was checked only after encoding/xml had
// built every attribute of the element in memory: 3 million attributes cost
// many times the XML. Now a run of more than 1 MiB without '>' is refused
// before the decoder sees the rest of it.
func TestR12AppleAttributeFloodMemory(t *testing.T) {
	var x bytes.Buffer
	x.WriteString(`<?xml version="1.0" encoding="UTF-8"?><HealthData><Me`)
	for i := 0; i < 3_000_000; i++ {
		fmt.Fprintf(&x, ` a%x=""`, i)
	}
	x.WriteString(`/></HealthData>`)
	var z bytes.Buffer
	zw := zip.NewWriter(&z)
	w, _ := zw.Create("apple_health_export/export.xml")
	w.Write(x.Bytes())
	zw.Close()
	p := filepath.Join(t.TempDir(), "export.zip")
	if err := os.WriteFile(p, z.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	err := convertFile(p, io.Discard, options{blob: testBlob})
	runtime.ReadMemStats(&after)
	alloc := after.TotalAlloc - before.TotalAlloc
	t.Logf("zip %d B, xml %d B, err=%v, allocated %d MiB before the refusal", z.Len(), x.Len(), err, alloc>>20)
	if !errors.Is(err, errTag) {
		t.Errorf("err %v, want %v", err, errTag)
	}
	if alloc > 64<<20 {
		t.Errorf("allocated %d MiB before the refusal", alloc>>20)
	}
}

// The bound counts bytes since the last '>', so an export of any size made
// of ordinary tags passes, and a long run of text is refused too.
func TestR12AppleTagBound(t *testing.T) {
	big := strings.Repeat(sampleRecord("HKQuantityTypeIdentifierStepCount", "12", "count"), 20000)
	if err := convertFile(writeXML(t, healthXML(big)), io.Discard, options{blob: testBlob, records: true}); err != nil {
		t.Errorf("an export of %d bytes in short tags: %v", len(big), err)
	}
	text := healthXML(strings.Repeat(" ", maxTag+1))
	if err := convertFile(writeXML(t, text), io.Discard, options{blob: testBlob}); !errors.Is(err, errTag) {
		t.Errorf("1 MiB of text without '>': %v", err)
	}
}

// R12-A1: a quantity with no unit was stored with none, and unit "%" on a
// type that is not a percentage was stored as a fraction (steps as 0.5 of 1).
// Both are refused; a category needs no unit.
func TestR12AppleUnitless(t *testing.T) {
	for _, rec := range []string{
		`<Record type="HKQuantityTypeIdentifierBodyMass" startDate="2026-10-01 10:00:00 -0500" endDate="2026-10-01 10:00:00 -0500" value="70"/>`,
		sampleRecord("HKQuantityTypeIdentifierBodyMass", "70", ""),
		sampleRecord("HKDataTypeSleepDurationGoal", "8", ""),
	} {
		var out bytes.Buffer
		err := convertFile(writeXML(t, healthXML(rec)), &out, options{blob: testBlob, records: true})
		if err == nil || !strings.Contains(err.Error(), "quantity without a unit") || out.Len() != 0 {
			t.Errorf("no unit: err %v, output %q", err, out.String())
		}
	}
	for _, kind := range []string{"HKQuantityTypeIdentifierStepCount", "HKQuantityTypeIdentifierBodyMass", "HKQuantityTypeIdentifierWalkingSpeed"} {
		var out bytes.Buffer
		err := convertFile(writeXML(t, healthXML(sampleRecord(kind, "0.5", "%"))), &out, options{blob: testBlob, records: true})
		if err == nil || !strings.Contains(err.Error(), "unit % on a type that is not a percentage") || out.Len() != 0 {
			t.Errorf("%s with unit %%: err %v, output %q", kind, err, out.String())
		}
	}
	// The three percent types still convert; a category without a unit passes.
	var out bytes.Buffer
	err := convertFile(writeXML(t, healthXML(
		sampleRecord("HKQuantityTypeIdentifierAppleWalkingSteadiness", "0.85", "%"),
		sampleRecord("HKQuantityTypeIdentifierWalkingAsymmetryPercentage", "0.03", "%"),
		sampleRecord("HKQuantityTypeIdentifierWalkingDoubleSupportPercentage", "0.272", "%"),
		sampleRecord("HKCategoryTypeIdentifierSleepAnalysis", "HKCategoryValueSleepAnalysisAsleepCore", ""),
	)), &out, options{blob: testBlob, records: true})
	if err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out.Bytes())
	for _, r := range rows[1:4] {
		if v := r["value"].(map[string]any); v["unit"] != "1" {
			t.Errorf("%v: %v", r["type"], v)
		}
	}
	if _, has := rows[4]["value"].(map[string]any)["unit"]; has {
		t.Errorf("category got a unit: %v", rows[4]["value"])
	}
}
