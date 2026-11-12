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
	"github.com/Conscious-Repository/bean/internal/pack"
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
// draw a second row. A comparison with a reference pack also says what the
// reading was set against, and by which pack.
func Record(r *core.Record, reading func(id string) *core.Record) string {
	cohort, _ := r.Context["cohort"].(map[string]any)
	among, _ := cohort["text"].(string)
	p, isNumber := r.Value["n"].(float64)
	if !strings.HasPrefix(r.Type, "comparison.") || among == "" || !isNumber {
		return Value(r)
	}
	out := "percentile " + strconv.FormatFloat(p, 'g', -1, 64)
	id, _ := r.Context["reading"].(string)
	of := reading(id)
	if of != nil {
		out += " of " + of.Type + " " + Value(of)
	}
	against, among, by, mark := reference(r, among, of)
	out += against + ", among " + among
	if result, ok := r.Context["result"].(map[string]any); ok {
		if n, ok := result["n"].(float64); ok {
			out += " (n " + strconv.FormatFloat(n, 'g', -1, 64) + ")"
		}
	}
	// The marks come last, and from bean alone.
	out += by
	if mark != "" {
		out += " (" + mark + ")"
	}
	return out
}

// Mark is what bean says about a comparison beyond what the claim says. It
// is "" for a comparison with nothing to remark and for any other record.
// The marks, by what comes first:
//
//   - "is NOT about a reading": the record it is about is one of bean's
//     own, such as another comparison;
//   - "names NO reference data": it names no pack and no files, or names
//     them in a form that is neither, so nobody can compute it again;
//   - "names BOTH a pack and files": a comparison is made with one;
//   - "the claim MISSTATES this pack": the pack is on record, and the claim
//     gives another label or cohort, names a measure or a cohort that the
//     pack does not have, or sets a reading against a measure that does
//     not take its type and unit;
//   - "NOT on record": bean has no record of the pack.
func Mark(r *core.Record, reading func(id string) *core.Record) string {
	cohort, _ := r.Context["cohort"].(map[string]any)
	among, _ := cohort["text"].(string)
	if _, isNumber := r.Value["n"].(float64); !strings.HasPrefix(r.Type, "comparison.") || among == "" || !isNumber {
		return ""
	}
	id, _ := r.Context["reading"].(string)
	_, _, _, mark := reference(r, among, reading(id))
	return mark
}

// texts returns a list of texts, or nil when v is not one.
func texts(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, x := range list {
		text, ok := x.(string)
		if !ok {
			return nil
		}
		out = append(out, text)
	}
	return out
}

// small reports whether the claim's own n is below the size under which a
// result is qualified, or nil when the claim states no n.
func small(r *core.Record) *bool {
	result, _ := r.Context["result"].(map[string]any)
	n, ok := result["n"].(float64)
	if !ok {
		return nil
	}
	is := n < pack.SmallN
	return &is
}

// otherSigner reports whether the reading was signed by another key than
// the claim, or nil when the reading is not at hand.
func otherSigner(r, of *core.Record) *bool {
	if of == nil {
		return nil
	}
	is := of.Author != r.Author
	return &is
}

// namesFiles reports a list of files as a comparison with files names
// them: one or more, each a data set and a SHA-256.
func namesFiles(v any) bool {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return false
	}
	for _, x := range list {
		f, _ := x.(map[string]any)
		dataset, _ := f["dataset"].(string)
		digest, _ := f["sha256"].(string)
		if dataset == "" || !pack.IsDigest(digest) {
			return false
		}
	}
	return true
}

// reference reads what a comparison was made with: a pack, or files. What
// a reading was set against, and the cohort in words, are the pack's to
// say. For a pack that bean has on record they are returned in bean's own
// words, and a claim that says otherwise is marked. For any other pack the
// label is the claim's word, so it is returned in quotation marks. of is
// the reading, when the caller may show it.
func reference(r *core.Record, claimed string, of *core.Record) (against, among, by, mark string) {
	among = strings.Join(strings.Fields(claimed), " ")
	if rs := []rune(among); len(rs) > 160 {
		among = string(rs[:157]) + "..."
	}
	ref, _ := r.Context["reference"].(map[string]any)
	digest, _ := ref["pack"].(string)
	_, statesPack := ref["pack"]
	_, statesFiles := ref["files"]
	byPack, byFiles := pack.IsDigest(digest), namesFiles(ref["files"])
	known, onRecord := pack.Known[digest]
	misstates := false
	if byPack {
		by = ", by pack " + digest[:12]
		measure, _ := ref["measure"].(string)
		label, _ := ref["label"].(string)
		cohortID, _ := ref["cohort"].(string)
		if rs := []rune(label); len(rs) > 80 {
			label = string(rs[:77]) + "..."
		}
		switch own, has := known.Measures[measure]; {
		case onRecord && has:
			against = " against " + own
			misstates = own != label
			if of != nil {
				unit, _ := of.Value["unit"].(string)
				misstates = misstates || !known.Takes(measure, of.Type, unit)
			}
		case onRecord:
			misstates = true
		case label != "":
			against = " against " + strconv.Quote(label)
		}
		if own, has := known.Cohorts[cohortID]; onRecord && has {
			misstates = misstates || own != among
			among = own
		} else if onRecord {
			misstates = true
		}
		// The caveats are the pack's too: a claim cannot drop one, or put
		// words of its own among them.
		if onRecord && !misstates {
			misstates = !known.ClaimCaveats(measure, cohortID, texts(r.Context["caveats"]), small(r), otherSigner(r, of))
		}
	}
	switch {
	case of != nil && pack.OwnType(of.Type):
		mark = "is NOT about a reading"
	case statesPack && statesFiles:
		mark = "names BOTH a pack and files"
	case !byPack && !byFiles:
		mark = "names NO reference data"
	case onRecord && misstates:
		mark = "the claim MISSTATES this pack"
	case byPack && !onRecord:
		mark = "NOT on record"
	}
	return against, among, by, mark
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
