package core

import (
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Read side of the CLI (ROADMAP.md M6): list, show and manual entry. None
// of this is in the spec. It reads the verified folder, never the index,
// so it cannot show a stale or forged cache.

// Filter selects records for List. Zero values select everything.
type Filter struct {
	// Type is an exact type, or a prefix when it ends in "*" ("hrv.*").
	Type   string
	Since  time.Time // When(r) at or after Since
	Until  time.Time // When(r) before Until
	Author string
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

// ManualValue turns a typed-in value into a value object (SPEC.md 3.4):
// a number with an optional unit, true or false, or text.
func ManualValue(text, unit string) map[string]any {
	if n, err := strconv.ParseFloat(text, 64); err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) {
		v := map[string]any{"n": n}
		if unit != "" {
			v["unit"] = unit
		}
		return v
	}
	switch text {
	case "true", "yes":
		return map[string]any{"b": true}
	case "false", "no":
		return map[string]any{"b": false}
	}
	return map[string]any{"s": text}
}

// ParseWhen reads the -at flag of hlog add. It accepts RFC 3339 (second
// precision), "YYYY-MM-DD HH:MM" or "YYYY-MM-DDTHH:MM" in loc (minute),
// and "YYYY-MM-DD" (day). It returns the start string and the precision.
func ParseWhen(s string, loc *time.Location) (string, string, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.Format(time.RFC3339Nano), "second", nil
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t.Format(time.RFC3339), "minute", nil
		}
	}
	if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
		return t.Format("2006-01-02") + "T00:00:00" + t.Format("Z07:00"), "day", nil
	}
	return "", "", errors.New("bad field: times.observed.start")
}

// AddOptions are the inputs of Add beyond type and value.
type AddOptions struct {
	Unit string
	// Start and Precision are the observed interval start. An empty Start
	// means now, at minute precision: the person reports the present.
	Start, Precision string
	Note             string
	About            string // a record id this claim comments on
	// Details are extra context keys, for example the protein_g of a
	// food.estimate. They never replace note.
	Details map[string]any
	// Photo is stored as an encrypted blob with role "photo".
	Photo     []byte
	PhotoType string // media type, for example image/jpeg
	// Agent signs with the local agent key instead of the person's.
	Agent bool
}

// Add writes one claim that the person types in (hlog add). It goes
// through Append, so every check and the dedupe rules apply. It returns
// the record id.
func (s *Store) Add(typ, value string, o AddOptions) (string, error) {
	if !typePattern.MatchString(typ) || len(typ) > 128 {
		return "", ErrBadType
	}
	if IsCoreType(typ) {
		return "", errors.New("unknown core type")
	}
	start, precision := o.Start, o.Precision
	if start == "" {
		start, precision = s.Now().Truncate(time.Minute).Format(time.RFC3339), "minute"
	}
	obj := map[string]any{
		"type":   typ,
		"value":  ManualValue(value, o.Unit),
		"times":  map[string]any{"observed": map[string]any{"start": start, "precision": precision}},
		"source": map[string]any{"system": "self", "adapter": Adapter, "ref": nil},
	}
	ctx := map[string]any{}
	for k, v := range o.Details {
		ctx[k] = v
	}
	if o.Note != "" {
		ctx["note"] = o.Note
	}
	if len(ctx) > 0 {
		obj["context"] = ctx
	}
	if o.About != "" {
		obj["refs"] = []any{map[string]any{"rel": "about", "id": o.About}}
	}
	if o.Photo != nil {
		blob, err := s.PutBlob(o.Photo)
		if err != nil {
			return "", err
		}
		obj["blobs"] = []any{map[string]any{"hash": blob, "role": "photo", "media_type": o.PhotoType}}
	}
	line := strings.NewReader(string(Canonical(obj)) + "\n")
	var res AppendResult
	var err error
	if o.Agent {
		res, err = s.AppendAsAgent(line)
	} else {
		res, err = s.Append(line)
	}
	if err != nil {
		return "", err
	}
	if len(res.IDs) != 1 {
		return "", errors.New("not added")
	}
	return res.IDs[0], nil
}
