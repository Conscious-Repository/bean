package core

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
)

// Writing a claim by hand or through an agent (hlog add, MCP log_entry).

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
	About            string // id or 6+ hex prefix of the record this claim comments on
	// Details are extra context keys, for example the protein_g of a
	// food.estimate. They never replace note.
	Details map[string]any
	// Photo is stored as an encrypted blob with role "photo".
	Photo     []byte
	PhotoType string // media type, for example image/jpeg
	// Agent signs with the local agent key instead of the person's.
	Agent bool
}

// Add writes one claim that the person types in (hlog add), or that an
// agent logs (o.Agent). It loads the log once and goes through the same
// path as Append, so every check and the dedupe rules apply. It returns
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
	l, err := s.Load()
	if err != nil {
		return "", err
	}
	w, err := s.writerFor(l, o.Agent)
	if err != nil {
		return "", err
	}
	if o.About != "" {
		r, err := l.Resolve(o.About)
		if err != nil {
			return "", err
		}
		obj["refs"] = []any{map[string]any{"rel": "about", "id": r.ID}}
	}
	if o.Photo != nil {
		blob, err := s.PutBlob(o.Photo)
		if err != nil {
			return "", err
		}
		obj["blobs"] = []any{map[string]any{"hash": blob, "role": "photo", "media_type": o.PhotoType}}
	}
	res, err := s.appendWith(l, w, strings.NewReader(string(Canonical(obj))+"\n"))
	if err != nil {
		return "", err
	}
	if len(res.IDs) != 1 {
		return "", errors.New("not added")
	}
	return res.IDs[0], nil
}
