package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestConvert(t *testing.T) {
	in := "time,value,id,device\n" +
		"2026-10-20T03:00:00-05:00,42.5,r1,ring\n" + // number, offset kept
		"2026-10-21T03:00:00Z,Negative,r2,\n" + // text
		"2026-10-22T03:00:00Z,,r3,\n" + // no value
		"yesterday,40,,\n" + // unparseable time, no native id
		"yesterday,40,,\n" // identical reading: next occurrence
	var out bytes.Buffer
	if err := convert(strings.NewReader(in), &out, options{typ: "hrv.rmssd", system: "csv", unit: "ms", precision: "second"}); err != nil {
		t.Fatal(err)
	}
	var recs []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(err)
		}
		recs = append(recs, m)
	}
	if len(recs) != 5 {
		t.Fatalf("%d records", len(recs))
	}
	val := func(i int) string { b, _ := json.Marshal(recs[i]["value"]); return string(b) }
	for i, want := range []string{`{"n":42.5,"unit":"ms"}`, `{"s":"Negative"}`, `{"n":null}`, `{"n":40,"unit":"ms"}`} {
		if got := val(i); got != want {
			t.Errorf("row %d value %s, want %s", i+1, got, want)
		}
	}
	if obs := recs[0]["times"].(map[string]any)["observed"].(map[string]any); obs["start"] != "2026-10-20T03:00:00-05:00" {
		t.Errorf("time rewritten: %v", obs)
	}
	if recs[0]["context"].(map[string]any)["device"] != "ring" {
		t.Errorf("extra column lost: %v", recs[0]["context"])
	}
	if recs[3]["times"].(map[string]any)["observed"] != nil || recs[3]["context"].(map[string]any)["raw_time"] != "yesterday" {
		t.Errorf("bad time not kept raw: %v", recs[3])
	}
	occ := func(i int) any {
		in := recs[i]["source"].(map[string]any)["ref_input"].([]any)
		return in[len(in)-1]
	}
	if occ(3) != 0.0 || occ(4) != 1.0 {
		t.Errorf("occurrences %v %v", occ(3), occ(4))
	}
	if err := convert(strings.NewReader("time,reading\n1,2\n"), &bytes.Buffer{}, options{typ: "x"}); err == nil {
		t.Error("no value column accepted")
	}
}
