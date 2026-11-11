// Package view holds the words a record is shown in: one line for its
// value, and the same line made safe to print. bean's commands and the
// bundle check (internal/bundle) share it, so a record reads the same in a
// terminal and on the page that checks a bundle.
package view

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Conscious-Repository/bean/core"
)

// Blob is a one-line view of a {"blob": id} value. SPEC 3.4 uses that
// shape for a series, with its unit, and for a large payload such as a
// kept lab report, without one. bean knows no series format (SPEC 9.4), so
// only a value with a unit is called a series; anything else is a file,
// with the media type its record states.
func Blob(r *core.Record) string {
	b, _ := r.Value["blob"].(string)
	short := strings.TrimPrefix(b, "sha256:")
	short = short[:min(12, len(short))]
	if u, ok := r.Value["unit"].(string); ok && u != "" {
		return "series " + short + " " + u
	}
	// A kept reference file says which data set it is.
	if d, ok := r.Context["dataset"].(string); ok && d != "" {
		return "file " + d + " " + short
	}
	for _, ref := range r.Blobs {
		if ref.Hash == b && ref.MediaType != "" {
			return "file " + ref.MediaType + " " + short
		}
	}
	return "file " + short
}

// Record is Value, and for a comparison what it compares and with
// whom: "percentile 46 of lab.cholesterol-total 187 mg/dL, among male, age
// 30-39 (n 339)". reading returns the comparison's reading when the caller
// may show it: a current record, never one that was corrected or deleted.
// The cohort is the claim's own text, so it is cut to a length that cannot
// draw a second row.
func Record(r *core.Record, reading func(id string) *core.Record) string {
	cohort, _ := r.Context["cohort"].(map[string]any)
	among, _ := cohort["text"].(string)
	p, isNumber := r.Value["n"].(float64)
	if !strings.HasPrefix(r.Type, "comparison.") || among == "" || !isNumber {
		return Value(r)
	}
	out := "percentile " + strconv.FormatFloat(p, 'g', -1, 64)
	id, _ := r.Context["reading"].(string)
	if of := reading(id); of != nil {
		out += " of " + of.Type + " " + Value(of)
	}
	among = strings.Join(strings.Fields(among), " ")
	if rs := []rune(among); len(rs) > 160 {
		among = string(rs[:157]) + "..."
	}
	out += ", among " + among
	if result, ok := r.Context["result"].(map[string]any); ok {
		if n, ok := result["n"].(float64); ok {
			out += " (n " + strconv.FormatFloat(n, 'g', -1, 64) + ")"
		}
	}
	return out
}

// Value is a one-line view of value and context.note.
func Value(r *core.Record) string {
	v := r.Value
	var out string
	switch {
	case v == nil:
		out = ""
	case v["n"] != nil:
		n, isNumber := v["n"].(float64)
		if !isNumber { // not a number at all: show it as it is written
			out = string(core.Canonical(v))
			break
		}
		out = strconv.FormatFloat(n, 'g', -1, 64)
		if u, ok := v["unit"].(string); ok && u != "" {
			out += " " + u
		}
	case v["s"] != nil:
		out, _ = v["s"].(string)
	case v["b"] != nil:
		out = fmt.Sprint(v["b"])
	case v["blob"] != nil:
		out = Blob(r)
	case r.Type == "bean.genesis" || r.Type == "bean.succession":
		out = r.ValueKey()
	case r.Type == "agent.pair":
		label, _ := v["label"].(string)
		out = strings.TrimSpace(label + " " + core.Npub(r.ValueKey()))
	case len(v) == 1 && v["n"] == nil:
		if _, has := v["n"]; has {
			out = "no value"
			// A lab says why it has no value: show that, not a bare blank.
			if c, _ := r.Context["lab_comment"].(string); strings.HasPrefix(strings.ToLower(c), "test not performed") || strings.HasPrefix(strings.ToLower(c), "not performed") {
				out = "not performed"
			}
			break
		}
		out = string(core.Canonical(v))
	default:
		out = string(core.Canonical(v))
	}
	if flag, ok := r.Context["flag"].(string); ok && flag != "" {
		out += "  " + flag // a lab's High or Low
	}
	if note, ok := r.Context["note"].(string); ok && note != "" {
		if out != "" {
			out += "  "
		}
		out += "# " + note
	}
	if rs := []rune(out); len(rs) > 80 {
		out = string(rs[:77]) + "..."
	}
	return out
}

// Safe returns s with every character that can end a line, move the
// cursor, start a terminal escape or reorder text written as \xNN or
// \uNNNN, so text from a record cannot fake output lines or control the
// terminal. Other characters are unchanged.
func Safe(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r >= 0x80 && r <= 0x9f, r == 0x061c, r == 0x200e, r == 0x200f,
			r >= 0x2028 && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}
