package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/Conscious-Repository/bean/core"
)

// The review before a share lists what the shared records carry beside
// the row that it shows for each: every field of context, every field of
// value but the number and its unit, and the system and adapter of its
// source. The source's ref is an id that bean derives. An importer copies a report's own words into
// these fields, and a lab's comment can hold a name. No record and no
// field is left out by its name or its type: a name is chosen by whoever
// wrote the record.
//
// A field whose values are all short, with more than maxValues different
// values, shows its count and its longest few: an export gives each
// reading its own index and time. Any other field shows each different
// value in full, and a long value names the records that carry it when
// they are few.
const (
	shortField = 40
	maxValues  = 50
	examples   = 3
	maxIDs     = 5
)

// plainName is a field name that is shown as it is. Any other is quoted.
var plainName = regexp.MustCompile(`^[a-z0-9_.-]{1,64}$`)

// quote shows a text between quotation marks that it cannot close.
func quote(s string) string {
	return `"` + safe(strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)) + `"`
}

func shown(v any) (string, bool) {
	if s, ok := v.(string); ok {
		return quote(s), true
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", false
	}
	return safe(strings.TrimSpace(b.String())), true
}

func contextLines(shared []*core.Record) []string {
	type field struct {
		records int
		values  []string
		ids     map[string][]string
	}
	fields := map[string]*field{}
	add := func(name string, v any, id string) {
		text, ok := shown(v)
		if !ok {
			return
		}
		f := fields[name]
		if f == nil {
			f = &field{ids: map[string][]string{}}
			fields[name] = f
		}
		f.records++
		if f.ids[text] == nil {
			f.values = append(f.values, text)
		}
		f.ids[text] = append(f.ids[text], id)
	}
	for _, r := range shared {
		for k, v := range r.Context {
			add("context."+k, v, r.ID)
		}
		for k, v := range r.Value {
			if k != "n" && k != "unit" {
				add("value."+k, v, r.ID)
			}
		}
		if r.Source.System != "" {
			add("source.system", r.Source.System, r.ID)
		}
		if r.Source.Adapter != "" {
			add("source.adapter", r.Source.Adapter, r.ID)
		}
	}
	var out []string
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		f := fields[name]
		label := name
		if !plainName.MatchString(name) {
			label = quote(name)
		}
		in := fmt.Sprintf("in %d records", f.records)
		if f.records == 1 {
			in = "in 1 record"
		}
		short := true
		for _, v := range f.values {
			if len(v) > shortField+2 {
				short = false
			}
		}
		if short && len(f.values) > maxValues {
			// The longest first: a text that holds a name is longer
			// than an index or a time.
			longest := slices.Clone(f.values)
			slices.SortStableFunc(longest, func(a, b string) int { return len(b) - len(a) })
			out = append(out, fmt.Sprintf("  %s, %s, %d different values. The longest: %s", label, in, len(f.values), strings.Join(longest[:examples], ", ")))
			out = append(out, fmt.Sprintf("    %d more values are not shown here. bean show <id> shows a record in full.", len(f.values)-examples))
			continue
		}
		if short {
			out = append(out, fmt.Sprintf("  %s, %s: %s", label, in, strings.Join(f.values, ", ")))
			continue
		}
		out = append(out, fmt.Sprintf("  %s, %s:", label, in))
		for _, v := range f.values {
			line := "    " + v
			if ids := f.ids[v]; len(ids) <= maxIDs && len(ids) < f.records {
				names := make([]string, len(ids))
				for i, id := range ids {
					names[i] = shortID(id)
				}
				line += "  (" + strings.Join(names, " ") + ")"
			}
			out = append(out, line)
		}
	}
	return out
}

// shortID is the start of a record id, as a review's rows show it.
func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	return id[:min(12, len(id))]
}
