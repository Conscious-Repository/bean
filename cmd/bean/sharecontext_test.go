package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

func id(i int) string { return fmt.Sprintf("sha256:%012x%052x", i+1, 0) }

// The review before a share lists every field that travels beside the row,
// with each different value in full. A lab's comment can hold a name.
func TestShareReviewListsContext(t *testing.T) {
	comment := "Called to Dr. Example Person on 01/02/2026, read back."
	long := strings.Repeat("a long note that the row cuts at 80 characters ", 3)
	reading := func(i int, test, c string) *core.Record {
		ctx := map[string]any{"test": test, "performing_lab": "Example Lab", "note": long, "flag": "High",
			"fasting": map[string]any{"range": "<5"}}
		if c != "" {
			ctx["lab_comment"] = c
		}
		return &core.Record{ID: id(i), Type: "lab.x", Context: ctx,
			Value:  map[string]any{"n": 5.0, "unit": "mg", "hidden": "Jane Roe 1970"},
			Source: core.Source{System: "Example Clinic of Jane Roe", Adapter: "x@1", Ref: "hmac-sha256:00"}}
	}
	// A record that calls itself a comparison is listed like any other.
	fake := &core.Record{ID: id(9), Type: "comparison.fake", Context: map[string]any{"secret": "John Doe, 12 Main St"}}
	got := strings.Join(contextLines([]*core.Record{reading(0, "WBC", comment), reading(1, "RBC", ""), reading(2, "HGB", "a\x1b[2Jb\nfake row\u200b"), fake}), "\n")

	for _, want := range []string{
		`  context.lab_comment, in 2 records:`,
		`    "` + comment + `"  (` + shortID(id(0)) + `)`,
		`    "a\x1b[2Jb\x0afake row\u200b"  (` + shortID(id(2)) + `)`,
		`  context.performing_lab, in 3 records: "Example Lab"`,
		`  context.test, in 3 records: "WBC", "RBC", "HGB"`,
		`  context.fasting, in 3 records: {"range":"<5"}`,
		`  context.flag, in 3 records: "High"`,
		`    "` + long + `"`,
		`  context.secret, in 1 record: "John Doe, 12 Main St"`,
		`  value.hidden, in 3 records: "Jane Roe 1970"`,
		`  source.system, in 3 records: "Example Clinic of Jane Roe"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("review lacks %q:\n%s", want, got)
		}
	}
	// The number and unit are in the row; ref is an id that bean derives.
	for _, not := range []string{"value.n", "value.unit", "hmac-sha256", "\x1b", "\nfake", "\u200b"} {
		if strings.Contains(got, not) {
			t.Errorf("review shows %q:\n%s", not, got)
		}
	}
}

// A value cannot close its own quotes, and a field name cannot pass for a
// header with a value.
func TestShareReviewQuotes(t *testing.T) {
	r := &core.Record{ID: id(0), Type: "x", Context: map[string]any{
		"lab_comment":             `x", "fake", "y\`,
		`ctl, in 1 record: "ok"`: "q",
	}}
	got := strings.Join(contextLines([]*core.Record{r}), "\n")
	for _, want := range []string{
		`  context.lab_comment, in 1 record: "x\", \"fake\", \"y\\"`,
		`  "context.ctl, in 1 record: \"ok\"", in 1 record: "q"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("review lacks %q:\n%s", want, got)
		}
	}
}

// Past maxValues different short values a field shows its count and the
// longest few, and says that the rest are not shown. A field with a long
// value is never cut.
func TestShareReviewCapsShortFields(t *testing.T) {
	var shared []*core.Record
	for i := range maxValues + 10 {
		ctx := map[string]any{"source_record_index": fmt.Sprint(i), "comment": fmt.Sprint("c", i)}
		if i == 40 {
			ctx["source_record_index"] = "Jane Roe 1/1/70"
			ctx["comment"] = strings.Repeat("a name hides in a long comment ", 2)
		}
		shared = append(shared, &core.Record{ID: id(i), Type: "apple-health.x", Context: ctx})
	}
	got := strings.Join(contextLines(shared), "\n")
	n := maxValues + 10
	for _, want := range []string{
		fmt.Sprintf(`  context.source_record_index, in %d records, %d different values. The longest: "Jane Roe 1/1/70"`, n, n),
		fmt.Sprintf("    %d more values are not shown here. bean show <id> shows a record in full.", n-examples),
		`    "c0"`, `    "c59"`, `    "a name hides in a long comment a name hides in a long comment "  (` + shortID(id(40)) + `)`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("review lacks %q:\n%s", want, got)
		}
	}
	if one := contextLines(shared[:1]); !strings.Contains(one[0], "in 1 record: ") {
		t.Errorf("one record: %q", one)
	}
}
