package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

const keepBlob = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// With -blob the first record keeps the file, so bean import csv works, and
// the readings name the file by digest, never in blobs, so purging the
// csv.file erases it.
func TestKeepWritesFileRecordFirst(t *testing.T) {
	raw := "time,value,id\n2026-10-20T03:00:00Z,42.5,r1\n"
	o := options{typ: "hrv.rmssd", system: "csv", unit: "ms", precision: "second", blob: keepBlob}
	var out bytes.Buffer
	data, err := keep(strings.NewReader(raw), &out, &o)
	if err != nil {
		t.Fatal(err)
	}
	if err := convert(bytes.NewReader(data), &out, o); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, m)
	}
	sum := sha256.Sum256([]byte(raw))
	digest := hex.EncodeToString(sum[:])
	if len(rows) != 2 || rows[0]["type"] != "csv.file" || rows[0]["value"].(map[string]any)["blob"] != keepBlob ||
		rows[0]["context"].(map[string]any)["sha256"] != digest || len(rows[0]["blobs"].([]any)) != 1 {
		t.Fatalf("archive: %v", rows)
	}
	if _, has := rows[1]["blobs"]; has || rows[1]["context"].(map[string]any)["file_sha256"] != digest {
		t.Errorf("reading: %v", rows[1])
	}
}

// A file that does not convert writes no record of it.
func TestKeepRefusedWritesNothing(t *testing.T) {
	for _, in := range []string{"time,reading\n1,2\n", "value,file_sha256\n1,x\n", "value\n\"bad\n"} {
		o := options{typ: "x", system: "csv", blob: keepBlob}
		var out bytes.Buffer
		if _, err := keep(strings.NewReader(in), &out, &o); err == nil || out.Len() != 0 {
			t.Errorf("%q: err %v, output %q", in, err, out.String())
		}
	}
	o := options{typ: "x", system: "csv", blob: "sha256:bad"}
	if _, err := keep(strings.NewReader("value\n1\n"), &bytes.Buffer{}, &o); err == nil {
		t.Error("bad blob accepted")
	}
}
