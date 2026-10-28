package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"bean/core"
)

func mcpStore(t *testing.T) (*core.Store, map[string]string) {
	t.Helper()
	time.Local = time.UTC
	s := core.Open(t.TempDir())
	s.Now = func() time.Time { return time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC) }
	if _, _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	add := func(name, typ, v, unit, at string) {
		id, err := s.Add(typ, v, core.AddOptions{Unit: unit, Start: at, Precision: "second"})
		if err != nil {
			t.Fatal(err)
		}
		ids[name] = id
	}
	add("rhr1", "heart.resting", "50", "/min", "2026-10-20T07:00:00Z")
	add("rhr2", "heart.resting", "54", "/min", "2026-10-21T07:00:00Z")
	add("rhr3", "heart.resting", "58", "/min", "2026-10-27T07:00:00Z")
	add("ache", "symptom.headache", "8", "{score}", "2026-10-27T09:00:00Z")
	id, err := s.Add("note", "salt?", core.AddOptions{About: ids["ache"]})
	if err != nil {
		t.Fatal(err)
	}
	ids["note"] = id
	return s, ids
}

// rpc sends lines to the server and returns one decoded response per
// request that has an id.
func rpc(t *testing.T, s *core.Store, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := serveMCP(s, strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var resps []map[string]any
	dec := json.NewDecoder(&out)
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatal(err)
		}
		resps = append(resps, m)
	}
	return resps
}

// call runs one tool and returns its decoded JSON text and isError.
func call(t *testing.T, s *core.Store, name string, args any) (map[string]any, bool) {
	t.Helper()
	a, _ := json.Marshal(args)
	resps := rpc(t, s, fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, name, a))
	if len(resps) != 1 {
		t.Fatalf("%s: %d responses", name, len(resps))
	}
	res, ok := resps[0]["result"].(map[string]any)
	if !ok {
		t.Fatalf("%s: no result: %v", name, resps[0])
	}
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	isErr := res["isError"].(bool)
	var m map[string]any
	if !isErr {
		if err := json.Unmarshal([]byte(text), &m); err != nil {
			t.Fatalf("%s: %v: %s", name, err, text)
		}
	} else {
		m = map[string]any{"error": text}
	}
	return m, isErr
}

func TestMCPHandshake(t *testing.T) {
	s, _ := mcpStore(t)
	resps := rpc(t, s,
		`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":"a","method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":3,"method":"resources/list"}`,
		`not json`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"write_record","arguments":{}}}`,
	)
	if len(resps) != 6 {
		t.Fatalf("got %d responses, want 6 (the notification has none)", len(resps))
	}
	init := resps[0]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-03-26" {
		t.Errorf("protocolVersion = %v", init["protocolVersion"])
	}
	if _, ok := init["capabilities"].(map[string]any)["tools"]; !ok {
		t.Error("no tools capability")
	}
	if resps[1]["id"] != "a" {
		t.Errorf("string id not echoed: %v", resps[1]["id"])
	}
	tools := resps[1]["result"].(map[string]any)["tools"].([]any)
	var names []string
	for _, x := range tools {
		tl := x.(map[string]any)
		names = append(names, tl["name"].(string))
		if tl["annotations"].(map[string]any)["readOnlyHint"] != true {
			t.Errorf("%s is not marked read-only", tl["name"])
		}
	}
	if strings.Join(names, ",") != "list_types,query_records,stats,get_record,read_blob" {
		t.Errorf("tools = %v", names)
	}
	for i, code := range map[int]float64{3: -32601, 4: -32700, 5: -32602} {
		e, _ := resps[i]["error"].(map[string]any)
		if e == nil || e["code"] != code {
			t.Errorf("response %d: error = %v, want code %v", i, resps[i]["error"], code)
		}
	}
	// An unknown protocol version gets ours.
	r := rpc(t, s, `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	if v := r[0]["result"].(map[string]any)["protocolVersion"]; v != mcpVersion {
		t.Errorf("fallback version = %v", v)
	}
}

func TestMCPListTypesAndQuery(t *testing.T) {
	s, ids := mcpStore(t)
	m, isErr := call(t, s, "list_types", map[string]any{})
	if isErr {
		t.Fatal(m)
	}
	types := m["types"].([]any)
	if len(types) != 3 || m["records"] != 5.0 {
		t.Fatalf("list_types = %v", m)
	}
	rhr := types[0].(map[string]any)
	if rhr["type"] != "heart.resting" || rhr["count"] != 3.0 || rhr["first"] != "2026-10-20T07:00:00Z" || rhr["last"] != "2026-10-27T07:00:00Z" {
		t.Errorf("heart.resting = %v", rhr)
	}

	m, _ = call(t, s, "query_records", map[string]any{"type": "heart.*", "since": "2026-10-21", "newest_first": true, "limit": 1})
	rows := m["records"].([]any)
	if m["total"] != 2.0 || m["truncated"] != true || len(rows) != 1 || rows[0].(map[string]any)["id"] != ids["rhr3"] {
		t.Errorf("query = %v", m)
	}

	if m, isErr := call(t, s, "query_records", map[string]any{"since": "last week"}); !isErr {
		t.Errorf("bad since accepted: %v", m)
	}
	if m, isErr := call(t, s, "query_records", map[string]any{"color": "red"}); !isErr {
		t.Errorf("unknown argument accepted: %v", m)
	}
}

func TestMCPStats(t *testing.T) {
	s, ids := mcpStore(t)
	m, isErr := call(t, s, "stats", map[string]any{"type": "heart.resting", "bucket": "week"})
	if isErr {
		t.Fatal(m)
	}
	if m["count"] != 3.0 || m["min"] != 50.0 || m["max"] != 58.0 || m["mean"] != 54.0 || m["median"] != 54.0 || m["stdev"] != 4.0 {
		t.Errorf("stats = %v", m)
	}
	if m["last"].(map[string]any)["id"] != ids["rhr3"] {
		t.Errorf("last = %v", m["last"])
	}
	// Oct 20 and 21 are in the week of Monday Oct 19; Oct 27 in the week of Oct 26.
	b := m["buckets"].([]any)
	if len(b) != 2 || b[0].(map[string]any)["start"] != "2026-10-19" || b[0].(map[string]any)["mean"] != 52.0 || b[1].(map[string]any)["start"] != "2026-10-26" {
		t.Errorf("buckets = %v", b)
	}
	if m, isErr := call(t, s, "stats", map[string]any{}); !isErr {
		t.Errorf("stats without type = %v", m)
	}
	if m, _ := call(t, s, "stats", map[string]any{"type": "note"}); m["count"] != 0.0 || m["non_numeric"] != 1.0 {
		t.Errorf("stats of text = %v", m)
	}
}

func TestMCPRecordAndBlob(t *testing.T) {
	s, ids := mcpStore(t)
	m, isErr := call(t, s, "get_record", map[string]any{"id": ids["ache"][7:17]})
	if isErr {
		t.Fatal(m)
	}
	if m["status"] != "valid" || m["record"].(map[string]any)["id"] != ids["ache"] {
		t.Errorf("get_record = %v", m)
	}
	if r := m["replies"].([]any); len(r) != 1 || r[0].(map[string]any)["id"] != ids["note"] {
		t.Errorf("replies = %v", m["replies"])
	}

	csv := "time,value\n2026-10-20T07:00:00Z,50\n"
	blob, err := s.PutBlob([]byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	// Not yet named by any record: refused.
	if m, isErr := call(t, s, "read_blob", map[string]any{"id": blob}); !isErr {
		t.Errorf("unnamed blob read: %v", m)
	}
	line := fmt.Sprintf(`{"type":"heart.resting","value":{"n":50,"unit":"/min"},"times":{"observed":null},"source":{"system":"csv","adapter":"test@1","ref":null},"blobs":[{"hash":%q,"role":"raw-export","media_type":"text/csv"}]}`, blob)
	res, err := s.Append(strings.NewReader(line + "\n"))
	if err != nil || res.Added != 1 {
		t.Fatal(err)
	}
	m, isErr = call(t, s, "read_blob", map[string]any{"id": blob, "max_bytes": 10})
	if isErr || m["text"] != csv[:10] || m["truncated"] != true || m["media_type"] != "text/csv" {
		t.Errorf("read_blob = %v", m)
	}
	// Purge the only record that names it: no longer readable.
	if _, err := s.Purge(res.IDs[0], ""); err != nil {
		t.Fatal(err)
	}
	if m, isErr := call(t, s, "read_blob", map[string]any{"id": blob}); !isErr {
		t.Errorf("blob of purged record read: %v", m)
	}
	if m, isErr := call(t, s, "get_record", map[string]any{"id": res.IDs[0]}); !isErr {
		t.Errorf("purged record returned: %v", m)
	}
}
