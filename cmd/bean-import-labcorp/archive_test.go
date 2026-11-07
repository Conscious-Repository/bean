package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

func TestArchivePreservesOriginalAndAmendedReport(t *testing.T) {
	original := fixture()
	amended := strings.Replace(original, vals("6.0", "", "x10E3/uL", "3.4-10.8"), vals("7.0", "", "x10E3/uL", "3.4-10.8"), 1)
	if amended == original {
		t.Fatal("synthetic amendment did not change the report")
	}
	a, err := parseReport(original)
	if err != nil {
		t.Fatal(err)
	}
	b, err := parseReport(amended)
	if err != nil {
		t.Fatal(err)
	}
	firstBlob := "sha256:" + strings.Repeat("a", 64)
	secondBlob := "sha256:" + strings.Repeat("b", 64)
	firstReading, secondReading := a.records(firstBlob, nil)[0], b.records(secondBlob, nil)[0]
	if string(core.Canonical(firstReading["value"])) == string(core.Canonical(secondReading["value"])) {
		t.Fatal("amendment must change a reading")
	}
	if string(core.Canonical(firstReading["source"])) != string(core.Canonical(secondReading["source"])) {
		t.Fatal("regression requires the same specimen/test source identity")
	}
	first, second := archiveRecord([]byte(original), firstBlob), archiveRecord([]byte(amended), secondBlob)
	if string(core.Canonical(first["source"])) == string(core.Canonical(second["source"])) {
		t.Fatal("amended raw source must survive independently of reading dedupe")
	}
	if string(core.Canonical(first)) != string(core.Canonical(archiveRecord([]byte(original), firstBlob))) {
		t.Fatal("retry must reproduce the same archive claim")
	}
	for i, rec := range []map[string]any{first, second} {
		raw, blob := []byte(original), firstBlob
		if i == 1 {
			raw, blob = []byte(amended), secondBlob
		}
		if rec["type"] != "lab.report" || rec["value"].(map[string]any)["blob"] != blob {
			t.Fatal("archive must reference the original encrypted blob")
		}
		if rec["times"].(map[string]any)["observed"] != nil {
			t.Fatal("archive must not invent an observation time")
		}
		ctx := rec["context"].(map[string]any)
		h := sha256.Sum256(raw)
		if ctx["sha256"] != hex.EncodeToString(h[:]) || ctx["bytes"] != len(raw) || ctx["assurance"] != "unsigned-report" {
			t.Fatal("archive must retain exact plaintext digest, length and limited assurance")
		}
		blobs := rec["blobs"].([]any)
		if len(blobs) != 1 || blobs[0].(map[string]any)["hash"] != blob || blobs[0].(map[string]any)["role"] != "raw-export" {
			t.Fatal("original source must remain eligible for export")
		}
		if strings.Contains(string(core.Canonical(rec)), "Doe, Jane") || strings.Contains(string(core.Canonical(rec)), "000-000-0000-0") {
			t.Fatal("archive metadata must not copy patient/specimen text")
		}
	}
}

func TestArchiveRequiresBlobAndDescribesOriginalMedia(t *testing.T) {
	if archiveRecord([]byte("text"), "") != nil {
		t.Fatal("extraction without a preserved blob must not claim archival")
	}
	for _, tc := range []struct{ raw, media string }{{"%PDF-1.7\nsynthetic", "application/pdf"}, {"synthetic extracted text", "text/plain"}} {
		rec := archiveRecord([]byte(tc.raw), "sha256:"+strings.Repeat("a", 64))
		if rec["blobs"].([]any)[0].(map[string]any)["media_type"] != tc.media {
			t.Fatal("media type must describe the exact retained source")
		}
	}
}
