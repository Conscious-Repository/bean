package core

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Writing a claim by hand or through an agent (bean add, MCP log_entry).

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

// ParseWhen reads the -at flag of bean add. It accepts RFC 3339 (second
// precision), "YYYY-MM-DD HH:MM" or "YYYY-MM-DDTHH:MM" in loc (minute),
// and "YYYY-MM-DD" (day). It returns the start string and the precision.
func ParseWhen(s string, loc *time.Location) (string, string, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.Format(time.RFC3339Nano), "second", nil
	}
	// A local time in a clock change gap does not exist; Go would move it
	// by an hour. Refuse it (audit 4, m3).
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04", "2006-01-02"} {
		t, err := time.ParseInLocation(layout, s, loc)
		if err != nil {
			continue
		}
		if t.Format(layout) != s || (layout == "2006-01-02" && t.Hour() != 0) {
			return "", "", fmt.Errorf("bad time: %q does not exist in %s (the clock changed)", s, loc)
		}
		if layout == "2006-01-02" {
			return s + "T00:00:00" + t.Format("Z07:00"), "day", nil
		}
		return t.Format(time.RFC3339), "minute", nil
	}
	return "", "", fmt.Errorf("bad time: %q: use YYYY-MM-DD, YYYY-MM-DD HH:MM or RFC 3339", s)
}

// AddOptions are the inputs of Add beyond type and value.
type AddOptions struct {
	Unit string
	// Start and Precision are the observed interval start. An empty Start
	// means now, at minute precision: the person reports the present.
	Start, Precision string
	Note             string
	About            string // id or 6+ hex prefix of the record this claim comments on
	// Corrects is the id or 6+ hex prefix of a record of the same author
	// that this one replaces (a supersede, SPEC.md 6.6).
	Corrects string
	// Details are extra context keys, for example the protein_g of a
	// food.estimate. They never replace note.
	Details map[string]any
	// Photo is stored as an encrypted blob with role "photo".
	Photo     []byte
	PhotoType string // media type, for example image/jpeg
	// Agent signs with the local agent key instead of the person's.
	Agent bool
}

// Add writes one claim that the person types in (bean add), or that an
// agent logs (o.Agent). It loads the log once and goes through the same
// path as Append, so every check and the dedupe rules apply. It returns
// the record id.
func (s *Store) Add(typ, value string, o AddOptions) (string, error) {
	if !typePattern.MatchString(typ) || len(typ) > 128 {
		return "", fmt.Errorf("%w: %q: use lowercase letters, digits, - and _, in parts joined by dots, for example weight or drink.water", ErrBadType, typ)
	}
	if IsCoreType(typ) {
		return "", errors.New("unknown core type")
	}
	v := ManualValue(value, o.Unit)
	if _, isNumber := v["n"]; o.Unit != "" && !isNumber {
		return "", fmt.Errorf("not a number: %q: a value with a unit must be a number like 80.5", value)
	}
	start, precision := o.Start, o.Precision
	if start == "" {
		start, precision = s.Now().Truncate(time.Minute).Format(time.RFC3339), "minute"
	}
	obj := map[string]any{
		"type":   typ,
		"value":  v,
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
	unlock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	l, err := s.load(false)
	if err != nil {
		return "", err
	}
	w, err := s.writerFor(l, o.Agent)
	if err != nil {
		return "", err
	}
	var refs []any
	for _, ref := range []struct{ rel, id string }{{"about", o.About}, {"supersedes", o.Corrects}} {
		if ref.id == "" {
			continue
		}
		r, err := l.Resolve(ref.id)
		if err != nil {
			return "", err
		}
		refs = append(refs, map[string]any{"rel": ref.rel, "id": r.ID})
	}
	if refs != nil {
		obj["refs"] = refs
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
