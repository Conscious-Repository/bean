package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Conscious-Repository/bean/core"
)

// The tools of bean mcp: what each one takes, and what it returns. The
// server that offers them is mcp.go.

var errUnknownTool = errors.New("unknown tool")

type toolArgs struct {
	Type        string `json:"type"`
	Since       string `json:"since"`
	Until       string `json:"until"`
	Limit       int    `json:"limit"`
	NewestFirst bool   `json:"newest_first"`
	Bucket      string `json:"bucket"`
	ID          string `json:"id"`
	MaxBytes    int    `json:"max_bytes"`
	// IncludeOthers widens the read tools past the person's own records.
	IncludeOthers bool `json:"include_others"`
	// log_entry
	Value     any            `json:"value"`
	Unit      string         `json:"unit"`
	At        string         `json:"at"`
	Note      string         `json:"note"`
	About     string         `json:"about"`
	Corrects  string         `json:"corrects"`
	Details   map[string]any `json:"details"`
	PhotoPath string         `json:"photo_path"`
}

func callTool(s *core.Store, name string, raw json.RawMessage, opts mcpOpts) (any, error) {
	var a toolArgs
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return nil, fmt.Errorf("bad arguments: %v", err)
	}
	// One struct holds the arguments of every tool, so the decoder takes an
	// argument of another tool. Each tool takes only what its schema lists.
	var given map[string]json.RawMessage
	if err := json.Unmarshal(raw, &given); err != nil {
		return nil, fmt.Errorf("bad arguments: %v", err)
	}
	for _, tool := range slices.Concat(mcpTools, blobTools, writeTools) {
		if tool.Name != name {
			continue
		}
		props, _ := tool.InputSchema["properties"].(map[string]any)
		for _, arg := range slices.Sorted(maps.Keys(given)) {
			if _, ok := props[arg]; !ok {
				return nil, fmt.Errorf("bad arguments: %s takes no %q", name, arg)
			}
		}
	}
	self, err := s.Identity()
	if err != nil {
		return nil, err
	}
	f := core.Filter{Type: a.Type}
	if !a.IncludeOthers {
		f.Subject = self
	}
	if f.Since, err = parseBound(a.Since); err != nil {
		return nil, fmt.Errorf("bad since: %q", a.Since)
	}
	if f.Until, err = parseBound(a.Until); err != nil {
		return nil, fmt.Errorf("bad until: %q", a.Until)
	}
	switch name {
	case "list_types":
		return toolListTypes(s, f)
	case "query_records":
		return toolQuery(s, f, a, self)
	case "stats":
		if a.Type == "" {
			return nil, errors.New("type is required")
		}
		return toolStats(s, f, a.Bucket, self)
	case "get_record":
		return toolGetRecord(s, f, a.ID, self)
	case "read_blob":
		if opts.blobs {
			return toolReadBlob(s, f, a.ID, a.MaxBytes)
		}
	case "log_entry":
		if opts.write {
			return toolLogEntry(s, a)
		}
	}
	return nil, errUnknownTool
}

// namer names an author or subject as ls does: self for the local person,
// agent:<label> for one of their paired agents, peer:<name> for a person
// whose notes they accepted, the npub of another key,
// else the id.
type namer func(id string) string

// self is the local identity: an agent-only replica has no l.Local.
func names(l *core.Loaded, self string) namer {
	labels, agents, peers := l.PairedLabels(), l.FileLabels(), l.PeerShown()
	return func(id string) string {
		if id == self {
			return "self"
		}
		if label, ok := labels[id]; ok {
			// A peer is another person whose key the person paired. The
			// pairing says which, not the text of the label.
			if peers[id] != "" && agents[id] == "" {
				return "peer:" + peers[id]
			}
			return "agent:" + label
		}
		if npub := core.Npub(id); npub != "" {
			return npub
		}
		return id
	}
}

// row is the compact form of a record for a client.
func row(r *core.Record, name namer) map[string]any {
	m := map[string]any{"id": r.ID, "type": r.Type, "author": name(r.Author), "subject": name(r.Subject),
		"observed": r.Observed, "imported": r.Imported, "source": r.Source.System}
	if r.Value != nil {
		m["value"] = r.Value
	}
	if len(r.Context) > 0 {
		m["context"] = r.Context
	}
	if len(r.Refs) > 0 {
		var refs []any
		for _, x := range r.Refs {
			refs = append(refs, map[string]any{"rel": x.Rel, "id": x.ID})
		}
		m["refs"] = refs
	}
	return m
}

func toolListTypes(s *core.Store, f core.Filter) (any, error) {
	recs, l, err := s.List(core.Filter{Subject: f.Subject})
	if err != nil {
		return nil, err
	}
	type agg struct {
		count       int
		units       map[string]bool
		first, last time.Time
	}
	byType := map[string]*agg{}
	for _, r := range recs {
		g := byType[r.Type]
		if g == nil {
			g = &agg{units: map[string]bool{}}
			byType[r.Type] = g
		}
		g.count++
		if u, ok := r.Value["unit"].(string); ok && u != "" {
			g.units[u] = true
		}
		w := core.When(r)
		if g.first.IsZero() || w.Before(g.first) {
			g.first = w
		}
		if w.After(g.last) {
			g.last = w
		}
	}
	var out []any
	for _, t := range sortedStrings(byType) {
		g := byType[t]
		out = append(out, map[string]any{
			"type": t, "count": g.count, "units": sortedStrings(g.units),
			"first": g.first.Local().Format(time.RFC3339), "last": g.last.Local().Format(time.RFC3339),
		})
	}
	return withFrozen(map[string]any{"types": out, "records": len(recs)}, l), nil
}

func toolQuery(s *core.Store, f core.Filter, a toolArgs, self string) (any, error) {
	limit := a.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	recs, l, err := s.List(f)
	if err != nil {
		return nil, err
	}
	name := names(l, self)
	if a.NewestFirst {
		for i, j := 0, len(recs)-1; i < j; i, j = i+1, j-1 {
			recs[i], recs[j] = recs[j], recs[i]
		}
	}
	total := len(recs)
	if total > limit {
		recs = recs[:limit]
	}
	rows := []any{}
	for _, r := range recs {
		rows = append(rows, row(r, name))
	}
	return withFrozen(map[string]any{"records": rows, "total": total, "truncated": total > limit}, l), nil
}

type reading struct {
	when time.Time
	n    float64
	id   string
}

func summarize(rs []reading) map[string]any {
	vals := make([]float64, len(rs))
	sum := 0.0
	for i, r := range rs {
		vals[i] = r.n
		sum += r.n
	}
	sort.Float64s(vals)
	mean := sum / float64(len(vals))
	ss := 0.0
	for _, v := range vals {
		ss += (v - mean) * (v - mean)
	}
	median := vals[len(vals)/2]
	if len(vals)%2 == 0 {
		median = (vals[len(vals)/2-1] + vals[len(vals)/2]) / 2
	}
	m := map[string]any{"count": len(vals), "min": vals[0], "max": vals[len(vals)-1], "mean": round(mean), "median": median}
	if len(vals) > 1 {
		m["stdev"] = round(math.Sqrt(ss / float64(len(vals)-1)))
	}
	return m
}

func round(x float64) float64 { return math.Round(x*1e4) / 1e4 }

func bucketStart(t time.Time, bucket string) time.Time {
	t = t.Local()
	y, mo, d := t.Date()
	switch bucket {
	case "week":
		wd := (int(t.Weekday()) + 6) % 7 // Monday is 0
		return time.Date(y, mo, d-wd, 0, 0, 0, 0, time.Local)
	case "month":
		return time.Date(y, mo, 1, 0, 0, 0, 0, time.Local)
	}
	return time.Date(y, mo, d, 0, 0, 0, 0, time.Local)
}

func toolStats(s *core.Store, f core.Filter, bucket, self string) (any, error) {
	if bucket != "" && bucket != "day" && bucket != "week" && bucket != "month" {
		return nil, fmt.Errorf("bad bucket: %q", bucket)
	}
	recs, l, err := s.List(f)
	if err != nil {
		return nil, err
	}
	// One group per author, subject and unit: numbers from different
	// writers, people or units are never averaged together.
	name := names(l, self)
	type key struct{ authorID, subjectID, unit string }
	groups := map[key][]reading{}
	var keys []key
	units := map[string]bool{}
	skipped := 0
	for _, r := range recs {
		n, ok := r.Value["n"].(float64)
		if !ok {
			skipped++
			continue
		}
		u, _ := r.Value["unit"].(string)
		if u != "" {
			units[u] = true
		}
		k := key{r.Author, r.Subject, u}
		if groups[k] == nil {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], reading{core.When(r), n, r.ID})
	}
	out := withFrozen(map[string]any{"type": f.Type, "units": sortedStrings(units), "non_numeric": skipped}, l)
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		return a.authorID+"\x00"+a.subjectID+"\x00"+a.unit < b.authorID+"\x00"+b.subjectID+"\x00"+b.unit
	})
	var list []any
	for _, k := range keys {
		g := summarizeGroup(groups[k], bucket)
		g["author"], g["subject"], g["unit"] = name(k.authorID), name(k.subjectID), k.unit
		g["author_id"], g["subject_id"] = k.authorID, k.subjectID
		list = append(list, g)
	}
	switch len(list) {
	case 0:
		out["count"] = 0
	case 1:
		for k, v := range list[0].(map[string]any) {
			out[k] = v
		}
	default:
		out["groups"] = list
	}
	return out, nil
}

// summarizeGroup summarizes readings of one author, subject and unit.
func summarizeGroup(rs []reading, bucket string) map[string]any {
	out := summarize(rs)
	first, last := rs[0], rs[len(rs)-1]
	out["first"] = map[string]any{"when": first.when.Local().Format(time.RFC3339), "n": first.n, "id": first.id}
	out["last"] = map[string]any{"when": last.when.Local().Format(time.RFC3339), "n": last.n, "id": last.id}
	if bucket == "" {
		return out
	}
	groups := map[time.Time][]reading{}
	for _, r := range rs {
		k := bucketStart(r.when, bucket)
		groups[k] = append(groups[k], r)
	}
	var keys []time.Time
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })
	var buckets []any
	for _, k := range keys {
		b := summarize(groups[k])
		b["start"] = k.Format("2006-01-02")
		delete(b, "stdev")
		buckets = append(buckets, b)
	}
	out["buckets"] = buckets
	return out
}

func toolGetRecord(s *core.Store, f core.Filter, id, self string) (any, error) {
	if id == "" {
		return nil, errors.New("id is required")
	}
	l, err := s.Load()
	if err != nil {
		return nil, err
	}
	if l.IsTooNew() {
		return nil, l.TooNewError()
	}
	r, err := l.Resolve(id)
	if err != nil {
		return nil, err
	}
	if !l.Visible(r.ID) || !inScope(r, f) {
		return nil, errors.New("input missing: " + id) // purged, invalid or about someone else
	}
	name := names(l, self)
	var blobs []any
	for _, b := range r.Blobs {
		blobs = append(blobs, map[string]any{"id": b.Hash, "role": b.Role, "media_type": b.MediaType})
	}
	var replies []any
	for _, x := range l.Replies(r.ID) {
		if l.Visible(x.ID) && inScope(x, f) {
			replies = append(replies, row(x, name))
		}
	}
	return withFrozen(map[string]any{"status": l.Status(r.ID), "author": name(r.Author), "subject": name(r.Subject), "record": json.RawMessage(r.Bytes), "blobs": blobs, "replies": replies}, l), nil
}

// withFrozen adds what an AI must tell the person when the identity is frozen.
func withFrozen(out map[string]any, l *core.Loaded) map[string]any {
	if l.IsFrozen() {
		out["frozen"] = "Two rotations split this identity, and it is frozen for good. Where the two sides differ, these results can leave a record out or show an old value. The person should stop writing here and run bean migrate."
	}
	return out
}

// inScope reports whether r is about the subject the tools may show.
func inScope(r *core.Record, f core.Filter) bool {
	return f.Subject == "" || r.Subject == f.Subject
}

func toolReadBlob(s *core.Store, f core.Filter, id string, most int) (any, error) {
	if most <= 0 {
		most = 100000
	}
	if most > 1000000 {
		most = 1000000
	}
	l, err := s.Load()
	if err != nil {
		return nil, err
	}
	if l.IsTooNew() {
		return nil, l.TooNewError()
	}
	// A short id works as in blob get.
	if full, err := s.ResolveBlob(id); err == nil {
		id = full
	}
	// Only a current record of the person or a paired agent unlocks a blob. A
	// stranger can name any blob, also one the person deleted.
	mediaType, named := "", false
	labels := l.FileLabels()
	for rid, r := range l.Report.Records {
		if !l.Current(rid) || !inScope(r, f) || (r.Author != l.ID && labels[r.Author] == "") {
			continue
		}
		for _, b := range r.Blobs {
			if b.Hash == id {
				named, mediaType = true, b.MediaType
			}
		}
	}
	if !named {
		return nil, errors.New("blob missing: " + id) // no current record names it
	}
	pt, err := s.GetBlob(id)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"id": id, "media_type": mediaType, "size": len(pt)}
	if !utf8.Valid(pt) {
		out["binary"] = true
		return out, nil
	}
	if len(pt) > most {
		cut := most
		for cut > 0 && !utf8.RuneStart(pt[cut]) {
			cut--
		}
		pt = pt[:cut]
		out["truncated"] = true
	}
	out["text"] = string(pt)
	return out, nil
}

func sortedStrings[V any](m map[string]V) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
