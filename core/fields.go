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

// IsIdentity reports whether the record is a genesis or a succession.
func (r *Record) IsIdentity() bool { return r.Type == "bean.genesis" || r.Type == "bean.succession" }

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

// Sealed returns value.sealed for successions.
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
var knownRels = map[string]bool{"succeeds": true, "supersedes": true, "purges": true, "about": true}

var (
	ErrUnknownCoreType = errors.New("unknown core type")
	ErrUnknownRel      = errors.New("unknown rel")
	ErrBlobNotListed   = errors.New("blob not listed")
	ErrOldFormat       = errors.New("old format: core record from a build before the rename to bean; this log cannot be used, run bean init in a new folder")
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
	nonce, ok := m["nonce"].(string)
	if !ok || !isLowerHex(nonce, 32) {
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
	subj, hasSubj := m["subject"]
	if r.Type == "bean.genesis" {
		if hasSubj {
			return badField("subject")
		}
	} else {
		s, ok := subj.(string)
		if !hasSubj || !ok || !IsID(s) {
			return badField("subject")
		}
		r.Subject = s
	}
	// author
	if !IsID(r.Author) && !IsTaggedKey(r.Author) {
		return badField("author")
	}
	// value
	if v, has := m["value"]; has {
		obj, ok := v.(map[string]any)
		if !ok {
			return badField("value")
		}
		r.Value = obj
	}
	// times
	times, ok := m["times"].(map[string]any)
	if !ok {
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
	src, ok := m["source"].(map[string]any)
	if !ok {
		return badField("source")
	}
	sys, ok := src["system"].(string)
	if !ok || len(sys) < 1 || len(sys) > 64 {
		return badField("source.system")
	}
	ad, ok := src["adapter"].(string)
	if !ok || !adapterPattern.MatchString(ad) {
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
			obj, ok := it.(map[string]any)
			if !ok {
				return badField("refs")
			}
			rel, ok1 := obj["rel"].(string)
			id, ok2 := obj["id"].(string)
			if !ok1 || !ok2 || !IsID(id) {
				return badField("refs")
			}
			if !knownRels[rel] {
				return ErrUnknownRel
			}
			if (rel == "succeeds" && r.Type != "bean.succession") || (rel == "purges" && r.Type != "bean.tombstone") {
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
			obj, ok := it.(map[string]any)
			if !ok {
				return badField("blobs")
			}
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
		obj, ok := raw.(map[string]any)
		if !ok {
			return badField("context")
		}
		r.Context = obj
	}
	// sig shape (verified earlier)
	if _, ok := m["sig"].(string); !ok {
		return badField("sig")
	}
	return r.checkReservedType()
}

func (r *Record) checkReservedType() error {
	switch r.Type {
	case "bean.genesis":
		key := r.ValueKey()
		if r.Value == nil || len(r.Value) != 2 || !IsTaggedKey(key) || !IsID(r.Recovery()) {
			return badField("value")
		}
		if r.Author != key || r.Kid != key {
			return badField("author")
		}
		if len(r.Refs) != 0 {
			return badField("refs")
		}
		if r.Observed != nil {
			return badField("times.observed")
		}
		if r.Source.System != "self" || r.Source.Ref != "" {
			return badField("source")
		}
	case "bean.succession":
		if r.Subject != r.Author || !IsID(r.Author) {
			return badField("author")
		}
		if len(r.Refs) != 1 || r.Refs[0].Rel != "succeeds" {
			return badField("refs")
		}
		if r.Value == nil || len(r.Value) != 3 || !IsTaggedKey(r.ValueKey()) || !IsID(r.Recovery()) {
			return badField("value")
		}
		sealed, ok := r.Value["sealed"].([]any)
		if !ok {
			return badField("value.sealed")
		}
		prev := ""
		for _, x := range sealed {
			s, ok := x.(string)
			if !ok || !IsID(s) || s <= prev {
				return badField("value.sealed")
			}
			prev = s
		}
	case "bean.tombstone":
		if r.Subject != r.Author {
			return badField("author")
		}
		if len(r.Refs) != 1 || r.Refs[0].Rel != "purges" {
			return badField("refs")
		}
		if r.Value != nil || r.Blobs != nil {
			return badField("value")
		}
		if reason, has := r.Context["reason"]; has {
			s, ok := reason.(string)
			if !ok || len([]rune(s)) > 200 {
				return badField("context.reason")
			}
		}
	default:
		if strings.HasPrefix(r.Type, "bean.") {
			return ErrUnknownCoreType
		}
		if b, has := r.Value["blob"]; has {
			id, ok := b.(string)
			if !ok || !IsID(id) {
				return ErrBlobNotListed
			}
			found := false
			for _, bl := range r.Blobs {
				if bl.Hash == id {
					found = true
				}
			}
			if !found {
				return ErrBlobNotListed
			}
		}
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
