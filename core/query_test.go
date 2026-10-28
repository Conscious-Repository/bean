package core

import (
	"strings"
	"testing"
	"time"
)

func TestManualValue(t *testing.T) {
	cases := []struct {
		text, unit, want string
	}{
		{"8", "{score}", `{"n":8,"unit":"{score}"}`},
		{"81.2", "", `{"n":81.2}`},
		{"yes", "", `{"b":true}`},
		{"false", "", `{"b":false}`},
		{"NaN", "", `{"s":"NaN"}`},
		{"1e400", "", `{"s":"1e400"}`},
		{"salt headache", "", `{"s":"salt headache"}`},
	}
	for _, c := range cases {
		if got := string(Canonical(ManualValue(c.text, c.unit))); got != c.want {
			t.Errorf("ManualValue(%q, %q) = %s, want %s", c.text, c.unit, got, c.want)
		}
	}
}

func TestParseWhen(t *testing.T) {
	loc := time.FixedZone("CDT", -5*3600)
	cases := []struct{ in, start, precision string }{
		{"2026-10-28T09:00:00Z", "2026-10-28T09:00:00Z", "second"},
		{"2026-10-28 09:30", "2026-10-28T09:30:00-05:00", "minute"},
		{"2026-10-28T09:30", "2026-10-28T09:30:00-05:00", "minute"},
		{"2026-10-28", "2026-10-28T00:00:00-05:00", "day"},
	}
	for _, c := range cases {
		start, p, err := ParseWhen(c.in, loc)
		if err != nil || start != c.start || p != c.precision {
			t.Errorf("ParseWhen(%q) = %q %q %v, want %q %q", c.in, start, p, err, c.start, c.precision)
		}
	}
	if _, _, err := ParseWhen("yesterday", loc); err == nil {
		t.Error("ParseWhen accepted yesterday")
	}
}

func TestAddAndList(t *testing.T) {
	s, ident := initStore(t)
	h, err := s.Add("symptom.headache", "8", AddOptions{Unit: "{score}", Note: "salt?"})
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Add("weight", "81.2", AddOptions{Unit: "kg", Start: "2026-10-27T00:00:00Z", Precision: "day"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Add("note", "took ibuprofen", AddOptions{About: h})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add("hlog.genesis", "1", AddOptions{}); err == nil {
		t.Error("Add wrote a core type")
	}
	if _, err := s.Add("Bad Type", "1", AddOptions{}); err == nil {
		t.Error("Add accepted a bad type")
	}
	verifyOK(t, s)

	recs, l, err := s.List(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	// Time order: weight (Oct 27), then headache and note at the same
	// minute ordered by id.
	if len(recs) != 3 || recs[0].ID != w {
		t.Fatalf("list = %v", ids(recs))
	}
	r := l.Report.Records[h]
	if obs := r.Observed.(map[string]any); obs["start"] != "2026-10-28T12:00:00Z" || obs["precision"] != "minute" {
		t.Errorf("default observed = %v", obs)
	}
	if r.Context["note"] != "salt?" || r.Author != ident {
		t.Errorf("headache record = %s", r.Bytes)
	}
	if got := l.Replies(h); len(got) != 1 || got[0].ID != c {
		t.Errorf("replies = %v", ids(got))
	}

	for _, tc := range []struct {
		f    Filter
		want int
	}{
		{Filter{Type: "weight"}, 1},
		{Filter{Type: "symptom.*"}, 1},
		{Filter{Since: testClock.Add(-time.Hour)}, 2},
		{Filter{Until: testClock.Add(-time.Hour)}, 1},
		{Filter{Author: ident}, 3},
		{Filter{Author: "ed25519:00"}, 0},
		{Filter{All: true}, 4}, // plus genesis
	} {
		got, _, err := s.List(tc.f)
		if err != nil || len(got) != tc.want {
			t.Errorf("List(%+v) = %d records, want %d", tc.f, len(got), tc.want)
		}
	}
}

func TestListHidesSupersededAndPurged(t *testing.T) {
	s, _ := initStore(t)
	res := appendLines(t, s, partial("hrv.rmssd", 40, []any{"hlog/v0/ref", "id", "test", "a", 0.0}), partial("hrv.rmssd", 50, []any{"hlog/v0/ref", "id", "test", "b", 0.0}))
	old, gone := res.IDs[0], res.IDs[1]
	fix := strings.Replace(partial("hrv.rmssd", 41, []any{"hlog/v0/ref", "id", "test", "a", 0.0}), `"source":`, `"refs":[{"id":"`+old+`","rel":"supersedes"}],"source":`, 1)
	fixID := appendLines(t, s, fix).IDs[0]
	if _, err := s.Purge(gone, ""); err != nil {
		t.Fatal(err)
	}
	recs, l, err := s.List(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].ID != fixID {
		t.Fatalf("list = %v, want only %s", ids(recs), fixID)
	}
	if st := l.Status(old); st != "superseded by "+fixID {
		t.Errorf("status = %q", st)
	}
	all, _, _ := s.List(Filter{All: true})
	for _, r := range all {
		if r.ID == gone {
			t.Error("-all listed a purged record")
		}
	}
}

func TestResolvePrefix(t *testing.T) {
	s, ident := initStore(t)
	id, err := s.Add("weight", "80", AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []string{id, id[7:], id[7:19], id[7:13]} {
		r, err := l.Resolve(in)
		if err != nil || r.ID != id {
			t.Errorf("Resolve(%q) = %v", in, err)
		}
	}
	if r, err := l.Resolve(ident); err != nil || r.Type != "hlog.genesis" {
		t.Errorf("Resolve(identity) = %v", err)
	}
	for _, in := range []string{id[7:12], "zzzzzz", "ffffffffffff0000"} {
		if _, err := l.Resolve(in); err == nil {
			t.Errorf("Resolve(%q) succeeded", in)
		}
	}
}

func ids(rs []*Record) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

// A parser fix (SPEC.md 9.7) keeps the dedupe ref of the record it
// corrects. Ids are random, so run it enough times to see both id orders:
// the fix must never be marked a duplicate of the record it supersedes.
func TestParserFixIsNotADuplicate(t *testing.T) {
	ref := []any{"hlog/v0/ref", "id", "test", "r1", 0.0}
	orders := map[bool]bool{}
	for i := 0; i < 24; i++ {
		s, _ := initStore(t)
		old := appendLines(t, s, partial("hrv.rmssd", 40, ref)).IDs[0]
		fix := strings.Replace(partial("hrv.rmssd", 41, nil), `"source":{"adapter":"test@1","ref":null,"system":"test"}`,
			`"source":{"adapter":"test@1","ref_input":["hlog/v0/ref","id","test","r1",0],"supersedes_ref":["hlog/v0/ref","id","test","r1",0],"system":"test"}`, 1)
		res := appendLines(t, s, fix)
		if res.Added != 1 {
			t.Fatalf("fix not added: %+v", res)
		}
		fixID := res.IDs[0]
		orders[fixID > old] = true
		recs, l, err := s.List(Filter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(recs) != 1 || recs[0].ID != fixID || l.Report.Duplicates[fixID] != "" {
			t.Fatalf("fix %s old %s: list %v, duplicates %v", fixID, old, ids(recs), l.Report.Duplicates)
		}
	}
	if len(orders) != 2 {
		t.Log("only one id order seen; rerun to cover both")
	}
}

// hlog add -corrects: a correction replaces the person's own entry; ls
// shows only the new one; an agent cannot correct the person's entry.
func TestAddCorrects(t *testing.T) {
	s, _ := initStore(t)
	wrong, err := s.Add("weight", "18.2", AddOptions{Unit: "kg"})
	if err != nil {
		t.Fatal(err)
	}
	right, err := s.Add("weight", "81.2", AddOptions{Unit: "kg", Corrects: wrong[7:15]})
	if err != nil {
		t.Fatal(err)
	}
	recs, l, _ := s.List(Filter{Type: "weight"})
	if len(recs) != 1 || recs[0].ID != right || l.Status(wrong) != "superseded by "+right {
		t.Errorf("list %v, status %q", ids(recs), l.Status(wrong))
	}
	if _, _, err := s.AgentInit("", "claude"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add("weight", "70", AddOptions{Unit: "kg", Corrects: right, Agent: true}); err != ErrNotAuthor {
		t.Errorf("agent corrected the person's entry: %v", err)
	}
	if _, err := s.Add("weight", "70", AddOptions{Corrects: "ffffff"}); err == nil {
		t.Error("correction of a missing record accepted")
	}
}

// Purge and rotate -no-seal take a shortened id, like every other command.
func TestShortIDsEverywhere(t *testing.T) {
	s, _, words := initWithWords(t)
	a, _ := s.Add("weight", "80", AddOptions{Unit: "kg"})
	b, _ := s.Add("weight", "81", AddOptions{Unit: "kg"})
	tid, err := s.Purge(a[7:13], "")
	if err != nil {
		t.Fatal(err)
	}
	l, _ := s.Load()
	if l.Visible(a) || l.Report.Records[tid].Targets("purges")[0] != a {
		t.Error("purge by prefix did not purge the full id")
	}
	c, _ := s.Add("weight", "82", AddOptions{Unit: "kg", Corrects: b})
	res, err := s.Rotate(words, []string{c[7:13]}, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.NotSealed) != 1 || res.NotSealed[0] != c {
		t.Errorf("no-seal by prefix: sealed %v not sealed %v", res.Sealed, res.NotSealed)
	}
}
