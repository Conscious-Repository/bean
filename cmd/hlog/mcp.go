package main

// hlog mcp (docs/ROADMAP.md M9): a read-only Model Context Protocol server on
// standard input and output, so an AI client can ask questions of the log.
// JSON-RPC 2.0, one message per line. Standard library only. Every tool
// call loads and verifies the folder, as hlog ls does, so a client never
// sees a record that fails verification or one that was purged.

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Conscious-Repository/bean/core"
)

//go:embed vocabulary.md
var vocabulary string

const writeInstructions = `
Writing is on: log_entry adds a claim signed by this machine's agent key, not the person's key, so readers can tell what came through an AI. It cannot change or delete anything.
When the person says what they ate, drank, took, did or felt, log it with log_entry: their words in value or note, the time it happened in at.
Put your own inferences (calories, macros, sodium) in a separate food.estimate entry with about set to the food entry's id.
Follow the type vocabulary below.`

const mcpVersion = "2025-06-18"

// mcpVersions are the protocol versions this server can speak. It answers
// with the client's version when it is in this list.
var mcpVersions = map[string]bool{"2024-11-05": true, "2025-03-26": true, "2025-06-18": true, "2025-11-25": true}

const mcpInstructions = `This is a personal health log: signed claims about one person, from devices, imports and the person.
Every record is a claim, not a fact. value is {"n": number, "unit": string}, {"s": text}, {"b": bool} or {"blob": id} for a series.
times.observed is when the claim is about (null when unknown); times.imported is when it entered the log.
Start with list_types. Use stats for trends, query_records for rows, get_record for one record and its replies, read_blob for raw source files.
The tools are read-only. Times without an offset and plain dates use the person's local zone: %s.`

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations"`
}

func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

var (
	strProp   = func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
	sinceProp = strProp("Start time, inclusive: RFC 3339, YYYY-MM-DD or YYYY-MM-DD HH:MM (local zone).")
	untilProp = strProp("End time, exclusive, same formats as since.")
	typeProp  = strProp("Record type, or a prefix ending in * (for example hrv.*).")
)

var mcpTools = []mcpTool{
	{
		Name:        "list_types",
		Description: "List every record type in the log with its count, units and first and last observed time. Call this first.",
		InputSchema: schema(map[string]any{}),
	},
	{
		Name:        "query_records",
		Description: "Return current records (valid, not superseded, not duplicate, not deleted) in time order. Each row has id, type, observed, imported, value, context and refs.",
		InputSchema: schema(map[string]any{
			"type":         typeProp,
			"since":        sinceProp,
			"until":        untilProp,
			"limit":        map[string]any{"type": "integer", "description": "Maximum rows, 1 to 1000. Default 100.", "minimum": 1, "maximum": 1000},
			"newest_first": map[string]any{"type": "boolean", "description": "Return the latest records first. Default false."},
		}),
	},
	{
		Name:        "stats",
		Description: "Summarize the numeric values (value.n) of one type over a time range: count, min, max, mean, median, standard deviation, first and last reading, and optional per-day, per-week or per-month buckets in the local zone. Use this for trend questions.",
		InputSchema: schema(map[string]any{
			"type":   typeProp,
			"since":  sinceProp,
			"until":  untilProp,
			"bucket": map[string]any{"type": "string", "enum": []string{"day", "week", "month"}, "description": "Group readings by local day, ISO week (Monday) or month."},
		}, "type"),
	},
	{
		Name:        "get_record",
		Description: "Return one record in full with its status, the blobs it names and the records that reply to it (notes, corrections, attestations). Accepts a full id or a unique prefix of 6 or more hex digits.",
		InputSchema: schema(map[string]any{"id": strProp("Record id (sha256:...) or hex prefix.")}, "id"),
	},
	{
		Name:        "read_blob",
		Description: "Return the decrypted text of a raw source file (blob) that a current record names, for example the CSV a reading came from. Binary blobs return only their size.",
		InputSchema: schema(map[string]any{
			"id":        strProp("Blob id (sha256:...)."),
			"max_bytes": map[string]any{"type": "integer", "description": "Maximum bytes of text, 1 to 1000000. Default 100000.", "minimum": 1, "maximum": 1000000},
		}, "id"),
	},
}

var writeTools = []mcpTool{
	{
		Name:        "log_entry",
		Description: "Add one entry to the person's log: something they ate, drank, took, did or felt, or an estimate you made about an entry. Signed by this machine's agent key. Returns the new id.",
		InputSchema: schema(map[string]any{
			"type":       strProp("Record type from the vocabulary, for example food, drink.water, symptom.headache, food.estimate."),
			"value":      map[string]any{"type": []string{"string", "number", "boolean"}, "description": "A number with unit, or text in the person's words."},
			"unit":       strProp("Unit of a number value, for example kg, L, mg, kcal, {score}."),
			"at":         strProp("When it happened: RFC 3339, YYYY-MM-DD HH:MM or YYYY-MM-DD in the local zone. Omit for now."),
			"note":       strProp("Free text from the person."),
			"about":      strProp("Id or 6+ hex prefix of the entry this one is about (required for food.estimate)."),
			"corrects":   strProp("Id or 6+ hex prefix of an entry you wrote earlier that this one replaces."),
			"details":    map[string]any{"type": "object", "description": "Extra fields from the vocabulary, for example meal, dose, protein_g, sodium_mg."},
			"photo_path": strProp("Absolute path of an image on this machine to attach, stored encrypted."),
		}, "type", "value"),
	},
}

func init() {
	for i := range mcpTools {
		mcpTools[i].Annotations = map[string]any{"readOnlyHint": true, "openWorldHint": false}
	}
	for i := range writeTools {
		writeTools[i].Annotations = map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": false, "openWorldHint": false}
	}
}

func instructions(zone string, write bool) string {
	text := fmt.Sprintf(mcpInstructions, time.Local.String()+" ("+zone+")")
	if write {
		text += writeInstructions
	}
	return text + "\n\n" + vocabulary
}

// toolLogEntry writes one claim with the agent key (hlog add -agent).
func toolLogEntry(s *core.Store, a toolArgs) (any, error) {
	if a.Type == "" {
		return nil, errors.New("type is required")
	}
	var value string
	switch v := a.Value.(type) {
	case string:
		value = v
	case float64:
		value = strconv.FormatFloat(v, 'g', -1, 64)
	case bool:
		value = strconv.FormatBool(v)
	default:
		return nil, errors.New("value is required: a string, number or boolean")
	}
	o := core.AddOptions{Unit: a.Unit, Note: a.Note, Details: a.Details, Agent: true}
	if a.At != "" {
		var err error
		if o.Start, o.Precision, err = core.ParseWhen(a.At, time.Local); err != nil {
			return nil, fmt.Errorf("bad at: %q", a.At)
		}
	}
	o.About, o.Corrects = a.About, a.Corrects
	if a.About == "" && a.Type == "food.estimate" {
		return nil, errors.New("food.estimate needs about: the id of the food entry")
	}
	if a.PhotoPath != "" {
		var err error
		if o.Photo, o.PhotoType, err = readImage(a.PhotoPath); err != nil {
			return nil, err
		}
	}
	id, err := s.Add(a.Type, value, o)
	if err != nil {
		return nil, err
	}
	ag, err := s.ReadAgent()
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "type": a.Type, "author": core.Npub(ag.Signer.Public())}, nil
}

func cmdMCP(s *core.Store, args []string) {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	write := fs.Bool("write", false, "add log_entry, signed by this machine's agent key")
	host, _ := os.Hostname()
	label := fs.String("label", "agent@"+host, "label for the agent key if one is made now")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		usage()
	}
	if _, err := s.Identity(); err != nil {
		fail(err)
	}
	if *write {
		// Make the agent key on first use. On the person's own machine this
		// also writes the agent.pair claim. Nothing goes to stdout.
		if _, _, err := s.AgentInit("", *label); err != nil {
			fail(err)
		}
	}
	if err := serveMCP(s, os.Stdin, os.Stdout, *write); err != nil {
		fail(err)
	}
}

// serveMCP answers requests from in until it closes. write adds the
// log_entry tool.
func serveMCP(s *core.Store, in io.Reader, out io.Writer, write bool) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			if err := enc.Encode(rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}}); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) == 0 {
			continue // a notification, for example notifications/initialized
		}
		resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
		resp.Result, resp.Error = handleMCP(s, req, write)
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

func handleMCP(s *core.Store, req rpcRequest, write bool) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		v := mcpVersion
		if mcpVersions[p.ProtocolVersion] {
			v = p.ProtocolVersion
		}
		zone, _ := time.Now().Zone()
		return map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "hlog", "version": core.Adapter},
			"instructions":    instructions(zone, write),
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		tools := mcpTools
		if write {
			tools = append(append([]mcpTool{}, mcpTools...), writeTools...)
		}
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{-32602, "invalid params"}
		}
		if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
			p.Arguments = json.RawMessage("{}")
		}
		data, err := callTool(s, p.Name, p.Arguments, write)
		if errors.Is(err, errUnknownTool) {
			return nil, &rpcError{-32602, "unknown tool: " + p.Name}
		}
		if err != nil {
			return toolResult(err.Error(), true), nil
		}
		b, err := json.Marshal(data)
		if err != nil {
			return toolResult(err.Error(), true), nil
		}
		return toolResult(string(b), false), nil
	}
	return nil, &rpcError{-32601, "method not found: " + req.Method}
}

func toolResult(text string, isErr bool) map[string]any {
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "isError": isErr}
}

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

func callTool(s *core.Store, name string, raw json.RawMessage, write bool) (any, error) {
	var a toolArgs
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return nil, fmt.Errorf("bad arguments: %v", err)
	}
	f := core.Filter{Type: a.Type}
	var err error
	if f.Since, err = parseBound(a.Since); err != nil {
		return nil, fmt.Errorf("bad since: %q", a.Since)
	}
	if f.Until, err = parseBound(a.Until); err != nil {
		return nil, fmt.Errorf("bad until: %q", a.Until)
	}
	switch name {
	case "list_types":
		return toolListTypes(s)
	case "query_records":
		return toolQuery(s, f, a)
	case "stats":
		if a.Type == "" {
			return nil, errors.New("type is required")
		}
		return toolStats(s, f, a.Bucket)
	case "get_record":
		return toolGetRecord(s, a.ID)
	case "read_blob":
		return toolReadBlob(s, a.ID, a.MaxBytes)
	case "log_entry":
		if write {
			return toolLogEntry(s, a)
		}
	}
	return nil, errUnknownTool
}

// row is the compact form of a record for a client.
func row(r *core.Record) map[string]any {
	m := map[string]any{"id": r.ID, "type": r.Type, "observed": r.Observed, "imported": r.Imported, "source": r.Source.System}
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

func toolListTypes(s *core.Store) (any, error) {
	recs, _, err := s.List(core.Filter{})
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
	return map[string]any{"types": out, "records": len(recs)}, nil
}

func toolQuery(s *core.Store, f core.Filter, a toolArgs) (any, error) {
	limit := a.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	recs, _, err := s.List(f)
	if err != nil {
		return nil, err
	}
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
		rows = append(rows, row(r))
	}
	return map[string]any{"records": rows, "total": total, "truncated": total > limit}, nil
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

func toolStats(s *core.Store, f core.Filter, bucket string) (any, error) {
	if bucket != "" && bucket != "day" && bucket != "week" && bucket != "month" {
		return nil, fmt.Errorf("bad bucket: %q", bucket)
	}
	recs, _, err := s.List(f)
	if err != nil {
		return nil, err
	}
	var rs []reading
	units := map[string]bool{}
	skipped := 0
	for _, r := range recs {
		n, ok := r.Value["n"].(float64)
		if !ok {
			skipped++
			continue
		}
		if u, ok := r.Value["unit"].(string); ok && u != "" {
			units[u] = true
		}
		rs = append(rs, reading{core.When(r), n, r.ID})
	}
	out := map[string]any{"type": f.Type, "units": sortedStrings(units), "non_numeric": skipped}
	if len(units) > 1 {
		out["warning"] = "readings use more than one unit; the numbers are not converted"
	}
	if len(rs) == 0 {
		out["count"] = 0
		return out, nil
	}
	for k, v := range summarize(rs) {
		out[k] = v
	}
	first, last := rs[0], rs[len(rs)-1]
	out["first"] = map[string]any{"when": first.when.Local().Format(time.RFC3339), "n": first.n, "id": first.id}
	out["last"] = map[string]any{"when": last.when.Local().Format(time.RFC3339), "n": last.n, "id": last.id}
	if bucket != "" {
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
	}
	return out, nil
}

func toolGetRecord(s *core.Store, id string) (any, error) {
	if id == "" {
		return nil, errors.New("id is required")
	}
	l, err := s.Load()
	if err != nil {
		return nil, err
	}
	r, err := l.Resolve(id)
	if err != nil {
		return nil, err
	}
	if !l.Visible(r.ID) {
		return nil, errors.New("input missing: " + id) // purged or invalid
	}
	var blobs []any
	for _, b := range r.Blobs {
		blobs = append(blobs, map[string]any{"id": b.Hash, "role": b.Role, "media_type": b.MediaType})
	}
	var replies []any
	for _, x := range l.Replies(r.ID) {
		if l.Visible(x.ID) {
			replies = append(replies, row(x))
		}
	}
	return map[string]any{"status": l.Status(r.ID), "record": json.RawMessage(r.Bytes), "blobs": blobs, "replies": replies}, nil
}

func toolReadBlob(s *core.Store, id string, max int) (any, error) {
	if max <= 0 {
		max = 100000
	}
	if max > 1000000 {
		max = 1000000
	}
	l, err := s.Load()
	if err != nil {
		return nil, err
	}
	mediaType, named := "", false
	for rid, r := range l.Report.Records {
		if !l.Visible(rid) {
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
	if len(pt) > max {
		cut := max
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
