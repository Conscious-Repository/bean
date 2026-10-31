package core

import (
	"strings"
	"testing"
)

func TestAgentOnPersonsMachine(t *testing.T) {
	s, ident := initStore(t)
	pub, pairID, err := s.AgentInit("", "claude")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pub, "secp256k1:") || pairID == "" {
		t.Fatalf("agent init: %s %q", pub, pairID)
	}
	// Idempotent: same key, no second pair claim.
	pub2, pairID2, err := s.AgentInit("", "claude")
	if err != nil || pub2 != pub || pairID2 != pairID {
		t.Errorf("second init: %s %s %v", pub2, pairID2, err)
	}

	meal, err := s.Add("food", "ramen, extra broth", AddOptions{Agent: true, Details: map[string]any{"meal": "lunch"}})
	if err != nil {
		t.Fatal(err)
	}
	est, err := s.Add("food.estimate", "650", AddOptions{Agent: true, Unit: "kcal", About: meal, Details: map[string]any{"sodium_mg": 2400.0, "method": "test"}})
	if err != nil {
		t.Fatal(err)
	}
	mine, err := s.Add("symptom.headache", "7", AddOptions{Unit: "{score}"})
	if err != nil {
		t.Fatal(err)
	}
	l := verifyOK(t, s)
	for _, id := range []string{meal, est} {
		r := l.Report.Records[id]
		if r.Author != pub || r.Kid != pub || r.Subject != ident || !l.Current(id) {
			t.Errorf("%s: author %s subject %s", r.Type, r.Author, r.Subject)
		}
	}
	if l.Report.Records[mine].Author != ident {
		t.Error("person's entry signed by the agent")
	}
	if l.AgentLabels()[pub] != "claude" {
		t.Errorf("labels %v", l.AgentLabels())
	}
	if r := l.Replies(meal); len(r) != 1 || r[0].ID != est {
		t.Errorf("estimate not a reply of the meal")
	}
	if ctx := l.Report.Records[est].Context; ctx["sodium_mg"] != 2400.0 {
		t.Errorf("details %v", ctx)
	}

	// The agent cannot correct the person's entry.
	sup := `{"type":"symptom.headache","value":{"n":2},"times":{"observed":null},"source":{"system":"agent","adapter":"t@1","ref":null},"refs":[{"rel":"supersedes","id":"` + mine + `"}]}`
	if _, err := s.AppendAsAgent(strings.NewReader(sup + "\n")); err != ErrNotAuthor {
		t.Errorf("agent supersede of the person's entry: %v", err)
	}
	// Nor delete it, even when written straight to disk.
	a, _ := s.ReadAgent()
	tomb := NewTombstone(ident, a.Signer.Public(), mine, "", "t@1", "2026-10-28T12:00:00Z")
	tomb["author"] = a.Signer.Public()
	tid, data, err := SignRecord(tomb, a.Signer)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(s.path(PathOf(tid)), data, 0o600); err != nil {
		t.Fatal(err)
	}
	l2, _ := s.Load()
	if !l2.Visible(mine) || l2.Report.Valid[tid] {
		t.Errorf("agent tombstone valid %v, target visible %v, errors %v", l2.Report.Valid[tid], l2.Visible(mine), l2.Report.Errors)
	}

	photo := []byte("\x89PNG\r\n\x1a\nfake image bytes")
	pid, err := s.Add("food", "toast", AddOptions{Agent: true, Photo: photo, PhotoType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}
	l3, _ := s.Load()
	bl := l3.Report.Records[pid].Blobs
	if len(bl) != 1 || bl[0].Role != "photo" || bl[0].MediaType != "image/png" {
		t.Fatalf("blobs %+v", bl)
	}
	if got, err := s.GetBlob(bl[0].Hash); err != nil || string(got) != string(photo) {
		t.Errorf("photo round trip: %v", err)
	}
}

// An agent on a cloud machine: a synced copy with an agent key and no
// personal key. It writes; the person pairs it from their own machine.
func TestAgentOnlyReplica(t *testing.T) {
	a, ident := initStore(t)
	appendLines(t, a, partial("hrv.rmssd", 40, nil))
	cloud := newStore(t)
	if _, err := cloud.Sync(a); err != nil {
		t.Fatal(err)
	}
	pub, pairID, err := cloud.AgentInit("", "hermes")
	if err != nil {
		t.Fatal(err)
	}
	if pairID != "" {
		t.Error("an agent-only replica paired itself")
	}
	if got, _ := cloud.Identity(); got != ident {
		t.Errorf("identity %s", got)
	}
	if _, err := cloud.ReadLocal(); err == nil {
		t.Error("cloud replica has a personal key")
	}
	id, err := cloud.Add("drink.water", "0.5", AddOptions{Agent: true, Unit: "L"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cloud.Add("food", "toast", AddOptions{Agent: true, Photo: []byte("\x89PNG\r\n\x1a\n"), PhotoType: "image/png"}); err == nil || !strings.Contains(err.Error(), "no identity secret") {
		t.Errorf("photo without the identity secret: %v", err)
	}
	if _, err := cloud.AppendAsAgent(strings.NewReader(partial("hrv.rmssd", 41, []any{"bean/v0/ref", "id", "x", "1", 0.0}) + "\n")); err == nil || !strings.Contains(err.Error(), "no identity secret") {
		t.Errorf("dedupe ref without the identity secret: %v", err)
	}
	if _, err := cloud.Add("symptom.headache", "3", AddOptions{}); err == nil {
		t.Error("cloud replica wrote as the person")
	}
	verifyOK(t, cloud)

	if _, err := a.Sync(cloud); err != nil {
		t.Fatal(err)
	}
	l := verifyOK(t, a)
	if !l.Current(id) || l.Report.Records[id].Author != pub {
		t.Fatal("hermes entry missing on the person's machine")
	}
	if _, ok := l.AgentLabels()[pub]; ok {
		t.Error("labelled before pairing")
	}
	if _, err := a.Pair(pub, "hermes"); err != nil {
		t.Fatal(err)
	}
	l = verifyOK(t, a)
	if l.AgentLabels()[pub] != "hermes" {
		t.Errorf("labels %v", l.AgentLabels())
	}
	// Agent init on a folder with no log yet tells you to sync first.
	if _, _, err := newStore(t).AgentInit("", ""); err == nil || !strings.Contains(err.Error(), "sync the log first") {
		t.Errorf("empty folder: %v", err)
	}
}
