package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

func id(i int) string { return fmt.Sprintf("sha256:%012x%052x", i+1, 0) }

// rec is a record as bean reads it from a file: its JSON, parsed.
func rec(t *testing.T, i int, js string) *core.Record {
	t.Helper()
	v, err := core.Parse([]byte(js))
	if err != nil {
		t.Fatal(err)
	}
	raw := v.(map[string]any)
	raw["id"] = id(i)
	typ, _ := raw["type"].(string)
	return &core.Record{ID: id(i), Type: typ, Raw: raw}
}

// envelope is what bean makes and checks itself, which the review leaves out.
const envelope = `"v":0,"author":"sha256:aa","subject":"sha256:aa","kid":"ed25519:bb","sig":"ed25519:cc","nonce":"00","times":{"imported":"2026-11-17T00:00:00Z","observed":{"start":"2026-11-02T08:15:00-06:00","precision":"minute"}}`

// The review lists every field that a record's JSON holds, with each
// different value in full, whatever its name or its record's type.
func TestShareReviewListsEveryField(t *testing.T) {
	comment := "Called to Dr. Example Person on 01/02/2026, read back."
	long := strings.Repeat("a long note that the row cuts at 80 characters ", 3)
	reading := func(i int, test, extra string) *core.Record {
		return rec(t, i, `{`+envelope+`,"type":"lab.x","value":{"n":5,"unit":"mg","hidden":"Jane Roe 1970"},`+
			`"source":{"system":"Example Clinic of Jane Roe","adapter":"x@1","ref":"hmac-sha256:00","note":"SRC SECRET"},`+
			`"context":{"test":"`+test+`","performing_lab":"Example Lab","note":"`+long+`","flag":"High","fasting":{"range":"<5"}`+extra+`}}`)
	}
	shared := []*core.Record{
		reading(0, "WBC", `,"lab_comment":"`+comment+`"`),
		reading(1, "RBC", ""),
		reading(2, "HGB", `,"lab_comment":"a\u001b[2Jb\nfake row\u200b"`),
		// A record that calls itself a comparison is listed like any other.
		rec(t, 3, `{`+envelope+`,"type":"comparison.fake","value":{"n":1},"source":{"system":"self","adapter":"x@1","ref":null},"context":{"secret":"John Doe, 12 Main St"}}`),
		// Fields that no rule of bean names: a unit the row cannot show,
		// a file's role, a field at the top, and keys inside times.
		rec(t, 4, `{`+envelope+`,"type":"x","value":{"n":null,"unit":["Jane Roe"],"s":"hello"},"source":{"system":"self","adapter":"x@1","ref":null},`+
			`"blobs":[{"hash":"sha256:dd","role":"Jane Roe's passport","media_type":"secret/type"}],"zzz":"TOPLEVEL SECRET"}`),
	}
	shared[4].Raw["times"].(map[string]any)["observed"].(map[string]any)["who"] = "Jane"
	got := strings.Join(fieldLines(shared), "\n")

	for _, want := range []string{
		`  context.lab_comment, in 2 records:`,
		`    "` + comment + `"  (` + shortID(id(0)) + `)`,
		`    "a\x1b[2Jb\x0afake row\u200b"  (` + shortID(id(2)) + `)`,
		`  context.performing_lab, in 3 records: "Example Lab"`,
		`  context.test, in 3 records: "WBC", "RBC", "HGB"`,
		`  context.fasting.range, in 3 records: "<5"`,
		`  context.flag, in 3 records: "High"`,
		`    "` + long + `"`,
		`  context.secret, in 1 record: "John Doe, 12 Main St"`,
		`  value.hidden, in 3 records: "Jane Roe 1970"`,
		`  value.unit, in 4 records: "mg", ["Jane Roe"]`,
		`  value.n, in 1 record: null`,
		`  source.system, in 5 records: "Example Clinic of Jane Roe", "self"`,
		`  source.note, in 3 records: "SRC SECRET"`,
		`  blobs, in 1 record:`,
		`Jane Roe's passport`,
		`  zzz, in 1 record: "TOPLEVEL SECRET"`,
		`  times.observed.who, in 1 record: "Jane"`,
		`  times.observed.start, in 5 records: "2026-11-02T08:15:00-06:00"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("review lacks %q:\n%s", want, got)
		}
	}
	// A number is in the row; the envelope is bean's own.
	for _, not := range []string{"value.n, in 4", "hmac-sha256", "ed25519", "times.imported", "author", "\x1b", "\nfake", "\u200b"} {
		if strings.Contains(got, not) {
			t.Errorf("review shows %q:\n%s", not, got)
		}
	}
}

// A value cannot close its own quotes, and a field name cannot pass for a
// header with a value, or for two fields.
func TestShareReviewQuotes(t *testing.T) {
	r := rec(t, 0, `{`+envelope+`,"type":"x","context":{"lab_comment":"x\", \"fake\", \"y\\","ctl, in 1 record: \"ok\"":"q","":"empty","a.b":"dot"}}`)
	got := strings.Join(fieldLines([]*core.Record{r}), "\n")
	for _, want := range []string{
		`  context.lab_comment, in 1 record: "x\", \"fake\", \"y\\"`,
		`  context."ctl, in 1 record: \"ok\"", in 1 record: "q"`,
		`  context."", in 1 record: "empty"`,
		`  context."a.b", in 1 record: "dot"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("review lacks %q:\n%s", want, got)
		}
	}
}

// Past maxValues different short values a field shows its count and the
// longest few, and says that the rest are not shown. A field with a long
// value is never cut, and a long value in many records says how many.
func TestShareReviewCapsShortFields(t *testing.T) {
	var shared []*core.Record
	n := maxValues + 10
	for i := range n {
		index, comment := fmt.Sprint(i), fmt.Sprint("c", i)
		if i == 40 {
			index, comment = "Jane Roe 1/1/70", strings.Repeat("a name hides in a long comment ", 2)
		}
		if i >= 50 {
			comment = strings.Repeat("the same long comment in many records ", 2)
		}
		shared = append(shared, rec(t, i, `{`+envelope+`,"type":"apple-health.x","context":{"source_record_index":"`+index+`","comment":"`+comment+`"}}`))
	}
	got := strings.Join(fieldLines(shared), "\n")
	for _, want := range []string{
		fmt.Sprintf(`  context.source_record_index, in %d records, %d different values. The longest: "Jane Roe 1/1/70"`, n, n),
		fmt.Sprintf("    %d more values are not shown here, and one of them could hold a name.", n-examples),
		`    "c0"`, `    "c49"`,
		`    "a name hides in a long comment a name hides in a long comment "  (` + shortID(id(40)) + `)`,
		`    "the same long comment in many records the same long comment in many records "  (in 10 records)`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("review lacks %q:\n%s", want, got)
		}
	}
	if one := fieldLines(shared[:1]); !strings.Contains(strings.Join(one, "\n"), "in 1 record: ") {
		t.Errorf("one record: %q", one)
	}
}
