package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

// The review before a share lists every context field that travels, with
// each different value in full. A lab's comment can hold a name.
func TestShareReviewListsContext(t *testing.T) {
	comment := "Called to Dr. Example Person on 01/02/2026, read back."
	reading := func(test, c string) *core.Record {
		ctx := map[string]any{"test": test, "performing_lab": "Example Lab", "note": "my own note", "flag": "High",
			"report_sha256": strings.Repeat("ab", 32), "fasting": map[string]any{"hours": 12}}
		if c != "" {
			ctx["lab_comment"] = c
		}
		return &core.Record{Type: "lab.x", Context: ctx}
	}
	comparison := &core.Record{Type: "comparison.percentile", Context: map[string]any{"cohort": map[string]any{"text": "male"}, "secret_field": "left out"}}
	got := strings.Join(contextLines([]*core.Record{reading("WBC", comment), reading("RBC", ""), reading("HGB", "a\x1b[2Jb\nfake row"), comparison}), "\n")

	for _, want := range []string{
		`  lab_comment, in 2 records:`,
		`    "` + comment + `"`,
		`    "a\x1b[2Jb\x0afake row"`,
		`  performing_lab, in 3 records: "Example Lab"`,
		`  test, in 3 records: "WBC", "RBC", "HGB"`,
		`  fasting, in 3 records: {"hours":12}`,
		`  report_sha256, in 3 records:`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("review lacks %q:\n%s", want, got)
		}
	}
	// The row shows note and flag already; a comparison is bean's own.
	for _, not := range []string{"my own note", "High", "secret_field", "left out", "\x1b", "\nfake"} {
		if strings.Contains(got, not) {
			t.Errorf("review shows %q:\n%s", not, got)
		}
	}
}

// Past maxValues different values a field shows its count and a few, and
// says how to see each record's own.
func TestShareReviewCapsLongFields(t *testing.T) {
	var shared []*core.Record
	for i := range maxValues + 1 {
		shared = append(shared, &core.Record{Type: "apple-health.x", Context: map[string]any{"source_record_index": fmt.Sprint(i), "assurance": "unsigned-export"}})
	}
	lines := contextLines(shared)
	got := strings.Join(lines, "\n")
	want := fmt.Sprintf(`  source_record_index, in %d records, %d different values, such as:`, maxValues+1, maxValues+1)
	if !strings.Contains(got, want) || !strings.Contains(got, fmt.Sprintf("and %d other values: bean show <id>", maxValues+1-examples)) {
		t.Errorf("long field not capped:\n%s", got)
	}
	if len(lines) != 1+1+examples+1 {
		t.Errorf("%d lines, want %d:\n%s", len(lines), 2+examples+1, got)
	}
	if one := contextLines(shared[:1]); len(one) != 2 || !strings.Contains(one[0], "in 1 record:") {
		t.Errorf("one record: %q", one)
	}
}
