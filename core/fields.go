package core

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// Record is a parsed and field-checked record (SPEC.md 3.2 to 3.7).
type Record struct {
	Path    string
	Raw     map[string]any
	Bytes   []byte
	V       int
	Type    string
	Subject string // "" for genesis
	Author  string
	Kid     string
	ID      string
	Sig     string
	Value   map[string]any // nil when absent
	Refs    []Ref
	Blobs   []Blob
	Source  Source
	Context map[string]any
	// Observed is nil when times.observed is null.
	Observed any
	Imported string
}

type Ref struct{ Rel, ID string }
type Blob struct{ Hash, Role, MediaType string }
type Source struct {
	System, Adapter string
	Ref             string // "" when null
}

// IsIdentity reports whether the record is a genesis, a succession or a
// seal continuation (C28). Identity records are signed by a genesis key or a
// recovery key, never by a chain signing key.
func (r *Record) IsIdentity() bool {
	return r.Type == "bean.genesis" || r.Type == "bean.succession" || r.Type == "bean.seal"
}

// IsDestructive reports whether the record supersedes or is a tombstone.
func (r *Record) IsDestructive() bool {
	if r.Type == "bean.tombstone" {
		return true
	}
	for _, ref := range r.Refs {
		if ref.Rel == "supersedes" {
			return true
		}
	}
	return false
}

// Targets returns the ids named by refs with the given rel.
func (r *Record) Targets(rel string) []string {
	var out []string
	for _, ref := range r.Refs {
		if ref.Rel == rel {
			out = append(out, ref.ID)
		}
	}
	return out
}

// ValueKey returns value.key for identity records.
func (r *Record) ValueKey() string { s, _ := r.Value["key"].(string); return s }

// Recovery returns value.recovery for identity records.
func (r *Record) Recovery() string { s, _ := r.Value["recovery"].(string); return s }

// Parts returns value.parts of a succession (C28), 0 when absent.
func (r *Record) Parts() int { f, _ := r.Value["parts"].(float64); return int(f) }

// Part returns value.part of a seal continuation (C28).
func (r *Record) Part() int { f, _ := r.Value["part"].(float64); return int(f) }

// Sealed returns value.sealed for successions and seal continuations.
func (r *Record) Sealed() []string {
	a, _ := r.Value["sealed"].([]any)
	out := make([]string, 0, len(a))
	for _, x := range a {
		out = append(out, x.(string))
	}
	return out
}

var typePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*(\.[a-z0-9_-]+)*$`)
var adapterPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+$`)

var precisions = map[string]bool{"year": true, "month": true, "day": true, "hour": true, "minute": true, "second": true, "millisecond": true}
var knownRels = map[string]bool{"succeeds": true, "supersedes": true, "purges": true, "about": true, "seals": true}

// MaxSealParts bounds value.parts and value.part (C28): 1,000 parts of
// about 230,000 ids each.
const MaxSealParts = 1000

var (
	ErrUnknownCoreType = errors.New("unknown core type")
	ErrUnknownRel      = errors.New("unknown rel")
	ErrBlobNotListed   = errors.New("blob not listed")
	ErrOldFormat       = errors.New("old format")
)

func badField(name string) error { return errors.New("bad field: " + name) }

// IsTaggedKey reports whether s has the shape of a tagged key for a
// supported algorithm (it does not check the point).
func IsTaggedKey(s string) bool {
	_, err := ParseKey(s)
	return err == nil
}

// checkFields performs SPEC.md 3.8 step 15 (fields 3.2 to 3.7) on a parsed
// object whose envelope has already been checked. It fills the typed fields.
func (r *Record) checkFields() error {
	m := r.Raw
	// nonce
	if nonce, _ := m["nonce"].(string); !isLowerHex(nonce, 32) {
		return badField("nonce")
	}
	// type
	if len(r.Type) < 1 || len(r.Type) > 128 || !typePattern.MatchString(r.Type) {
		return badField("type")
	}
	// A core record from a build before the rename (C24) cannot be valid:
	// say so instead of failing on a later field (DECISIONS 111).
	if r.Type == "hlog.genesis" || r.Type == "hlog.succession" || r.Type == "hlog.tombstone" {
		return ErrOldFormat
	}
	// subject
	// subject: on every record but a genesis
	subj, hasSubj := m["subject"]
	r.Subject, _ = subj.(string)
	if hasSubj == (r.Type == "bean.genesis") || (hasSubj && !IsID(r.Subject)) {
		return badField("subject")
	}
	// author
	if !IsID(r.Author) && !IsTaggedKey(r.Author) {
		return badField("author")
	}
	// value
	if v, has := m["value"]; has {
		if r.Value, _ = v.(map[string]any); r.Value == nil {
			return badField("value")
		}
	}
	// times
	times, _ := m["times"].(map[string]any)
	if times == nil {
		return badField("times")
	}
	imp, ok := times["imported"].(string)
	if !ok || !isImportedTime(imp) {
		return badField("times.imported")
	}
	r.Imported = imp
	obs, has := times["observed"]
	if !has {
		return badField("times.observed")
	}
	if obs != nil {
		if err := checkInterval(obs); err != nil {
			return err
		}
		r.Observed = obs
	}
	// source
	src, _ := m["source"].(map[string]any)
	if src == nil {
		return badField("source")
	}
	sys, _ := src["system"].(string)
	if len(sys) < 1 || len(sys) > 64 {
		return badField("source.system")
	}
	ad, _ := src["adapter"].(string)
	if !adapterPattern.MatchString(ad) {
		return badField("source.adapter")
	}
	ref, has := src["ref"]
	if !has {
		return badField("source.ref")
	}
	if ref != nil {
		s, ok := ref.(string)
		if !ok || !IsRef(s) {
			return badField("source.ref")
		}
		r.Source.Ref = s
	}
	r.Source.System, r.Source.Adapter = sys, ad
	// refs
	if raw, has := m["refs"]; has {
		arr, ok := raw.([]any)
		if !ok {
			return badField("refs")
		}
		seen := map[Ref]bool{}
		for _, it := range arr {
			obj, _ := it.(map[string]any) // a nil map fails the next check
			rel, ok1 := obj["rel"].(string)
			id, ok2 := obj["id"].(string)
			if !ok1 || !ok2 || !IsID(id) {
				return badField("refs")
			}
			if !knownRels[rel] {
				return ErrUnknownRel
			}
			if (rel == "succeeds" && r.Type != "bean.succession") || (rel == "purges" && r.Type != "bean.tombstone") || (rel == "seals" && r.Type != "bean.seal") {
				return badField("refs")
			}
			rf := Ref{rel, id}
			if seen[rf] {
				return badField("refs")
			}
			seen[rf] = true
			r.Refs = append(r.Refs, rf)
		}
	}
	// blobs
	if raw, has := m["blobs"]; has {
		arr, ok := raw.([]any)
		if !ok {
			return badField("blobs")
		}
		for _, it := range arr {
			obj, _ := it.(map[string]any) // a nil map fails the next check
			hash, ok1 := obj["hash"].(string)
			role, ok2 := obj["role"].(string)
			mt, ok3 := obj["media_type"].(string)
			if !ok1 || !ok2 || !ok3 || !IsID(hash) {
				return badField("blobs")
			}
			r.Blobs = append(r.Blobs, Blob{hash, role, mt})
		}
	}
	// context
	if raw, has := m["context"]; has {
		if r.Context, _ = raw.(map[string]any); r.Context == nil {
			return badField("context")
		}
	}
	return r.checkReservedType()
}

// fieldRule is one check of 3.7: when bad, the record fails with field.
type fieldRule struct {
	bad   bool
	field string
}

// firstBad returns the error of the first rule that fails, in order.
func firstBad(rules ...fieldRule) error {
	for _, x := range rules {
		if x.bad {
			return badField(x.field)
		}
	}
	return nil
}

// checkReservedType applies the rules of the bean.* types (3.7, C28).
func (r *Record) checkReservedType() error {
	selfSource := r.Source.System == "self" && r.Source.Ref == ""
	oneRef := func(rel string) bool { return len(r.Refs) == 1 && r.Refs[0].Rel == rel }
	switch r.Type {
	case "bean.genesis":
		key := r.ValueKey()
		return firstBad(
			fieldRule{r.Value == nil || len(r.Value) != 2 || !IsTaggedKey(key) || !IsID(r.Recovery()), "value"},
			fieldRule{r.Author != key || r.Kid != key, "author"},
			fieldRule{len(r.Refs) != 0, "refs"},
			fieldRule{r.Observed != nil, "times.observed"},
			fieldRule{!selfSource, "source"})
	case "bean.succession":
		// value.parts is present only when seal continuations follow (C28).
		parts, hasParts := r.Value["parts"]
		n := 3
		if hasParts {
			n = 4
		}
		return firstBad(
			fieldRule{r.Subject != r.Author || !IsID(r.Author), "author"},
			fieldRule{!oneRef("succeeds"), "refs"},
			fieldRule{hasParts && !isCount(parts), "value.parts"},
			fieldRule{r.Value == nil || len(r.Value) != n || !IsTaggedKey(r.ValueKey()) || !IsID(r.Recovery()), "value"},
			fieldRule{!sealedList(r.Value["sealed"], false), "value.sealed"})
	case "bean.seal":
		// A seal continuation (C28): more ids for the seal set of the
		// succession that its one "seals" ref names.
		return firstBad(
			fieldRule{r.Subject != r.Author || !IsID(r.Author), "author"},
			fieldRule{!oneRef("seals"), "refs"},
			fieldRule{r.Value == nil || len(r.Value) != 2 || !isCount(r.Value["part"]), "value"},
			fieldRule{!sealedList(r.Value["sealed"], true), "value.sealed"},
			fieldRule{r.Observed != nil, "times.observed"},
			fieldRule{r.Blobs != nil, "blobs"},
			fieldRule{!selfSource, "source"})
	case "bean.tombstone":
		reason, hasReason := r.Context["reason"]
		s, isText := reason.(string)
		return firstBad(
			fieldRule{r.Subject != r.Author, "author"},
			fieldRule{!oneRef("purges"), "refs"},
			fieldRule{r.Value != nil, "value"},
			fieldRule{r.Blobs != nil, "blobs"},
			fieldRule{hasReason && (!isText || len([]rune(s)) > 200), "context.reason"})
	}
	if strings.HasPrefix(r.Type, "bean.") {
		return ErrUnknownCoreType
	}
	if b, has := r.Value["blob"]; has {
		id, _ := b.(string)
		for _, bl := range r.Blobs {
			if bl.Hash == id && IsID(id) {
				return nil
			}
		}
		return ErrBlobNotListed
	}
	return nil
}

func isImportedTime(s string) bool {
	if len(s) != 20 {
		return false
	}
	_, err := time.Parse("2006-01-02T15:04:05Z", s)
	return err == nil
}

func parseRFC3339(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil
}

func checkInterval(v any) error {
	obj, ok := v.(map[string]any)
	if !ok {
		return badField("times.observed")
	}
	start, ok := obj["start"].(string)
	if !ok {
		return badField("times.observed.start")
	}
	st, ok := parseRFC3339(start)
	if !ok {
		return badField("times.observed.start")
	}
	if e, has := obj["end"]; has {
		end, ok := e.(string)
		if !ok {
			return badField("times.observed.end")
		}
		et, ok := parseRFC3339(end)
		if !ok || et.Before(st) {
			return badField("times.observed.end")
		}
	}
	p, ok := obj["precision"].(string)
	if !ok || !precisions[p] {
		return badField("times.observed.precision")
	}
	return nil
}

// isCount reports an integer from 1 to MaxSealParts.
func isCount(v any) bool {
	f, ok := v.(float64)
	return ok && f == float64(int(f)) && f >= 1 && f <= MaxSealParts
}

// sealedList reports whether value.sealed is a list of unique ids in
// ascending ASCII order. A continuation must seal at least one id.
func sealedList(v any, nonEmpty bool) bool {
	sealed, ok := v.([]any)
	if !ok || (nonEmpty && len(sealed) == 0) {
		return false
	}
	prev := ""
	for _, x := range sealed {
		s, ok := x.(string)
		if !ok || !IsID(s) || s <= prev {
			return false
		}
		prev = s
	}
	return true
}
