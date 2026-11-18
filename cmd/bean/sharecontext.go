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

// fieldLines lists, for the review before a share, every field of the
// given records as their JSON holds it. A source can write any field, and
// a lab's comment can hold a name. So the list is not of known names: it
// is the whole record, less the fields that bean itself makes and checks
// (ownFields), and a number that the row already shows in full.
//
// Each field says in how many records it is, then each different value,
// and the records that carry a long value when they are few. Only a field
// whose values are all numbers or times, with more than maxValues of them,
// shows a count in place of its values: an export gives each reading its
// own index and time, and neither can hold a name.
func fieldLines(records []*core.Record) []string {
	type field struct {
		records int
		values  []string
		ids     map[string][]string
		plain   bool // every value is a number or a time
	}
	fields := map[string]*field{}
	var walk func(path string, v any, id string)
	walk = func(path string, v any, id string) {
		_, number := v.(float64)
		if ownFields[path] || path == "value.n" && number {
			return
		}
		if obj, ok := v.(map[string]any); ok && len(obj) > 0 {
			for _, k := range slices.Sorted(maps.Keys(obj)) {
				sub := segment(k)
				if path != "" {
					sub = path + "." + sub
				}
				walk(sub, obj[k], id)
			}
			return
		}
		text := shown(v)
		f := fields[path]
		if f == nil {
			f = &field{ids: map[string][]string{}, plain: true}
			fields[path] = f
		}
		s, isText := v.(string)
		if _, isTime := core.ParseTime(s); !number && !(isText && isTime) {
			f.plain = false
		}
		f.records++
		if f.ids[text] == nil {
			f.values = append(f.values, text)
		}
		f.ids[text] = append(f.ids[text], id)
	}
	for _, r := range records {
		walk("", r.Raw, r.ID)
	}
	var out []string
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		f := fields[name]
		in := fmt.Sprintf("in %d records", f.records)
		if f.records == 1 {
			in = "in 1 record"
		}
		if f.plain && len(f.values) > maxValues {
			out = append(out, fmt.Sprintf("  %s, %s: %d different numbers or times, not shown here", name, in, len(f.values)))
			continue
		}
		short := true
		for _, v := range f.values {
			if len(v) > shortField {
				short = false
			}
		}
		if short {
			out = append(out, fmt.Sprintf("  %s, %s: %s", name, in, strings.Join(f.values, ", ")))
			continue
		}
		out = append(out, fmt.Sprintf("  %s, %s:", name, in))
		for _, v := range f.values {
			line := "    " + v
			switch ids := f.ids[v]; {
			case len(ids) == f.records:
			case len(ids) <= maxIDs:
				names := make([]string, len(ids))
				for i, id := range ids {
					names[i] = shortID(id)
				}
				line += "  (" + strings.Join(names, " ") + ")"
			default:
				line += fmt.Sprintf("  (in %d records)", len(ids))
			}
			out = append(out, line)
		}
	}
	return out
}

const (
	shortField = 42 // a value of 40 characters and its quotation marks
	maxValues  = 50
	maxIDs     = 5
)

// ownFields are the fields that bean makes and checks to a format itself:
// the format version, ids, keys, the signature, the type that the row
// shows, the time of entry and the ref that bean derives from the source's.
var ownFields = map[string]bool{
	"v": true, "id": true, "sig": true, "kid": true, "author": true, "subject": true,
	"nonce": true, "type": true, "times.imported": true, "source.ref": true,
}

// plainSegment is a field name that is shown as it is. Any other is
// quoted, so that no name can pass for a header or for two fields.
var plainSegment = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

func segment(k string) string {
	if plainSegment.MatchString(k) {
		return k
	}
	return quote(k)
}

// quote shows a text between quotation marks that it cannot close.
func quote(s string) string {
	return `"` + safe(strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)) + `"`
}

// shown is a value as the review shows it: a text quoted, any other value
// as its JSON. A record's JSON was read from a file, so it encodes again.
func shown(v any) string {
	if s, ok := v.(string); ok {
		return quote(s)
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return safe(fmt.Sprint(v))
	}
	return safe(strings.TrimSpace(b.String()))
}

// shortID is the start of a record id, as a review's rows show it.
func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	return id[:min(12, len(id))]
}
