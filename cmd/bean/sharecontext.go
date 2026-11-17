package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Conscious-Repository/bean/core"
)

// shownInRow are the context fields that the review's row of a record
// already shows (view.Value).
var shownInRow = map[string]bool{"note": true, "flag": true}

// shortField is the length in bytes under which every value of a field
// goes on one line. Past maxValues different values, a field shows its
// count and the first few: an export gives each reading its own time.
const (
	shortField = 40
	maxValues  = 50
	examples   = 3
)

// contextLines lists, for the review before a share, every context field
// that the shared records carry beside their values, with each different
// value in full. An importer copies a report's own words into these
// fields, and a lab's comment can hold a name. A comparison is left out:
// its context is bean's own arithmetic, and its row shows the cohort.
func contextLines(shared []*core.Record) []string {
	type field struct {
		records int
		values  []string
		seen    map[string]bool
	}
	fields := map[string]*field{}
	for _, r := range shared {
		if strings.HasPrefix(r.Type, "comparison.") {
			continue
		}
		for k, v := range r.Context {
			if shownInRow[k] {
				continue
			}
			// Text is quoted; any other value is its JSON, unquoted.
			var text string
			if str, ok := v.(string); ok {
				text = `"` + safe(str) + `"`
			} else {
				b, err := json.Marshal(v)
				if err != nil {
					continue
				}
				text = safe(string(b))
			}
			f := fields[k]
			if f == nil {
				f = &field{seen: map[string]bool{}}
				fields[k] = f
			}
			f.records++
			if !f.seen[text] {
				f.seen[text] = true
				f.values = append(f.values, text)
			}
		}
	}
	var out []string
	for _, k := range slices.Sorted(maps.Keys(fields)) {
		f := fields[k]
		head := fmt.Sprintf("  %s, in %d records:", safe(k), f.records)
		if f.records == 1 {
			head = fmt.Sprintf("  %s, in 1 record:", safe(k))
		}
		values := f.values
		more := ""
		if len(values) > maxValues {
			more = fmt.Sprintf("    and %d other values: bean show <id> shows a record's own", len(values)-examples)
			values = values[:examples]
			head = strings.TrimSuffix(head, ":") + fmt.Sprintf(", %d different values, such as:", len(f.values))
		}
		short := more == ""
		for _, v := range values {
			if len(v) > shortField+2 {
				short = false
			}
		}
		quoted := values
		if short {
			out = append(out, head+" "+strings.Join(quoted, ", "))
			continue
		}
		out = append(out, head)
		for _, q := range quoted {
			out = append(out, "    "+q)
		}
		if more != "" {
			out = append(out, more)
		}
	}
	return out
}
