package core

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// Read side of the CLI and the MCP server (docs/ROADMAP.md M6, M9): list,
// resolve, replies and status. None of this is in the spec. It reads the
// loaded folder, never local/cache/index.json.

// Filter selects records for List. Zero values select everything.
type Filter struct {
	// Type is an exact type, or a prefix when it ends in "*" ("hrv.*").
	Type   string
	Since  time.Time // When(r) at or after Since
	Until  time.Time // When(r) before Until
	Author string
	// Subject is the identity the record is about.
	Subject string
	// All includes superseded records, duplicates, tombstones, identity
	// records and pairing claims. Purged, hidden and invalid records are
	// never listed.
	All bool
}

// When is the time a record is about: the observed start, or the import
// time when the observed time is null.
func When(r *Record) time.Time {
	if obs, ok := r.Observed.(map[string]any); ok {
		if s, ok := obs["start"].(string); ok {
			if t, ok := parseRFC3339(s); ok {
				return t
			}
		}
	}
	t, _ := time.Parse("2006-01-02T15:04:05Z", r.Imported)
	return t
}

func (f Filter) match(r *Record) bool {
	if p, ok := strings.CutSuffix(f.Type, "*"); ok {
		if !strings.HasPrefix(r.Type, p) {
			return false
		}
	} else if f.Type != "" && r.Type != f.Type {
		return false
	}
	if f.Author != "" && r.Author != f.Author {
		return false
	}
	if f.Subject != "" && r.Subject != f.Subject {
		return false
	}
	w := When(r)
	if !f.Since.IsZero() && w.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && !w.Before(f.Until) {
		return false
	}
	return true
}

// Current reports whether a reader should show the record: visible, not
// superseded, not a duplicate, and not a core record.
func (l *Loaded) Current(id string) bool {
	r := l.Report.Records[id]
	if r == nil || !l.Visible(id) || IsCoreType(r.Type) {
		return false
	}
	if len(l.Report.Superseded[id]) > 0 {
		return false
	}
	_, dup := l.Report.Duplicates[id]
	return !dup
}

// isSetup reports pairing claims: configuration, not things that happened.
func isSetup(typ string) bool { return typ == "agent.pair" || typ == "device.pair" }

// List returns the matching records in time order, then by id.
func (s *Store) List(f Filter) ([]*Record, *Loaded, error) {
	l, err := s.Load()
	if err != nil {
		return nil, nil, err
	}
	var out []*Record
	for id, r := range l.Report.Records {
		if !l.Visible(id) || (!f.All && (!l.Current(id) || isSetup(r.Type))) || !f.match(r) {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		wi, wj := When(out[i]), When(out[j])
		if !wi.Equal(wj) {
			return wi.Before(wj)
		}
		return out[i].ID < out[j].ID
	})
	return out, l, nil
}

// Resolve finds the record whose id is id, or the one record whose hex
// starts with a prefix of at least 6 digits. Hidden records resolve too.
func (l *Loaded) Resolve(id string) (*Record, error) {
	hexPart := strings.TrimPrefix(id, "sha256:")
	if r, ok := l.Report.Records["sha256:"+hexPart]; ok {
		return r, nil
	}
	if len(hexPart) < 6 || !isLowerHex(hexPart, len(hexPart)) {
		return nil, errors.New("input missing: " + id)
	}
	var found *Record
	for rid, r := range l.Report.Records {
		if strings.HasPrefix(rid[7:], hexPart) {
			if found != nil {
				return nil, errors.New("ambiguous id: " + id)
			}
			found = r
		}
	}
	if found == nil {
		return nil, errors.New("input missing: " + id)
	}
	return found, nil
}

// Replies returns the valid records that name id with any ref, sorted by
// id: comments and attestations (about), corrections (supersedes) and
// tombstones (purges).
func (l *Loaded) Replies(id string) []*Record {
	var out []*Record
	for _, rid := range sortedKeys(l.Report.Records) {
		r := l.Report.Records[rid]
		if !l.Report.Valid[rid] {
			continue
		}
		for _, ref := range r.Refs {
			if ref.ID == id {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// Status is a short reader-facing state of one record.
func (l *Loaded) Status(id string) string {
	rp := l.Report
	switch {
	case l.Hidden[id]:
		return "hidden, purged by " + rp.Purged[id]
	case rp.Errors[id] != "":
		return "invalid: " + rp.Errors[id]
	case rp.Purged[id] != "":
		return "purged by " + rp.Purged[id]
	case !rp.Valid[id]:
		return "not valid"
	case len(rp.Superseded[id]) > 0:
		return "superseded by " + rp.Superseded[id][0]
	case rp.Duplicates[id] != "":
		return "duplicate of " + rp.Duplicates[id]
	}
	return "valid"
}
