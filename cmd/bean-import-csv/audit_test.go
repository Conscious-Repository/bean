package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

func convertValues(t *testing.T, in string) []string {
	t.Helper()
	var out bytes.Buffer
	if err := convert(strings.NewReader(in), &out, options{typ: "x", system: "csv", precision: "second"}); err != nil {
		t.Fatal(err)
	}
	var vals []string
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(m["value"])
		vals = append(vals, string(b))
	}
	return vals
}

// NaN, Inf, hex floats, underscores and huge integers stay text (audit MINOR).
func TestConvertStrictNumbers(t *testing.T) {
	in := "value\nNaN\nInf\n-infinity\n0x1p4\n1_0\n9007199254740993\n1e400\n42\n-1.5e3\n.5\n+7\n"
	got := convertValues(t, in)
	want := []string{`{"s":"NaN"}`, `{"s":"Inf"}`, `{"s":"-infinity"}`, `{"s":"0x1p4"}`, `{"s":"1_0"}`, `{"s":"9007199254740993"}`, `{"s":"1e400"}`, `{"n":42}`, `{"n":-1500}`, `{"n":0.5}`, `{"n":7}`}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

// A UTF-8 byte order mark does not hide the first column.
func TestConvertBOM(t *testing.T) {
	if got := convertValues(t, "\ufeffvalue\n3\n"); len(got) != 1 || got[0] != `{"n":3}` {
		t.Errorf("got %v", got)
	}
}

// Invalid UTF-8 is refused, not rewritten to U+FFFD.
func TestConvertInvalidUTF8(t *testing.T) {
	var out bytes.Buffer
	if err := convert(strings.NewReader("value,note\n1,a\xffb\n"), &out, options{typ: "x", system: "csv"}); err == nil || !strings.Contains(err.Error(), "line 2: bad string") {
		t.Errorf("err %v", err)
	}
}

// Every line convert writes is a partial record that Parse accepts and
// that canonical form does not change.
func FuzzConvert(f *testing.F) {
	f.Add("time,value,id\n2026-10-20T03:00:00Z,42.5,r1\n")
	f.Add("\ufeffvalue,x\nNaN,\"q\"\"\"\n")
	f.Add("value\n1e999\n0x10\n")
	f.Fuzz(func(t *testing.T, in string) {
		var out bytes.Buffer
		if err := convert(strings.NewReader(in), &out, options{typ: "x", system: "csv", precision: "second"}); err != nil {
			return
		}
		for _, l := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
			if l == "" {
				continue
			}
			v, err := core.Parse([]byte(l))
			if err != nil {
				t.Fatalf("output not parseable: %q: %v", l, err)
			}
			if string(core.Canonical(v)) != l {
				t.Fatalf("output not canonical: %q", l)
			}
		}
	})
}

// Round 2 fuzz finding: 1e17 parses, but SPEC 4.1 cannot carry an integer
// value above 2^53, so append failed on the whole file. It stays text.
func TestConvertOutOfRangeNumber(t *testing.T) {
	got := convertValues(t, "value\n1e17\n1.5e17\n9007199254740991\n")
	want := []string{`{"s":"1e17"}`, `{"s":"1.5e17"}`, `{"n":9007199254740991}`}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %v want %v", got, want)
	}
}
