package core

import (
	"strings"
	"testing"
)

// One machine has one agent key. A second label for it would show the
// first assistant's records under the second name, so it is refused, by
// agent init and by pair (DECISIONS 209).
func TestAgentLabelIsRefusedNotRenamed(t *testing.T) {
	s, _ := initStore(t)
	pub, pairID, err := s.AgentInit("", "claude")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AgentInit("", "chatgpt"); err == nil || !strings.Contains(err.Error(), `paired as "claude"`) {
		t.Errorf("second label at init: %v", err)
	}
	if _, err := s.Pair(pub, "renamed"); err == nil || !strings.Contains(err.Error(), `paired as "claude"`) {
		t.Errorf("second label at pair: %v", err)
	}
	// The same label, or none, is the existing pairing.
	for _, label := range []string{"claude", ""} {
		if _, id, err := s.AgentInit("", label); err != nil || id != pairID {
			t.Errorf("init %q: %s %v", label, id, err)
		}
		if id, err := s.Pair(pub, label); err != nil || id != pairID {
			t.Errorf("pair %q: %s %v", label, id, err)
		}
	}
	l := verifyOK(t, s)
	if got := l.AgentLabels()[pub]; got != "claude" {
		t.Errorf("label %q", got)
	}
	n := 0
	for _, r := range l.Report.Records {
		if r.Type == "agent.pair" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d pair claims, want 1", n)
	}
}

// An agent the person unpaired is not trusted again by agent init or by
// mcp -write. agent pair trusts it again with a new claim, and says so
// truthfully: before, it returned the unpairing as "paired".
func TestAgentUnpairedIsNotRepairedByInit(t *testing.T) {
	s, ident := initStore(t)
	pub, pairID, err := s.AgentInit("", "claude")
	if err != nil {
		t.Fatal(err)
	}
	unpair := `{"type":"agent.pair","value":{"key":"` + pub + `","status":"unpaired"},"times":{"observed":null},"source":{"system":"self","adapter":"t@1","ref":null},"refs":[{"rel":"supersedes","id":"` + pairID + `"}]}`
	res := appendLines(t, s, unpair)
	if len(res.IDs) != 1 {
		t.Fatalf("unpair: %+v", res)
	}
	if _, ok := verifyOK(t, s).AgentLabels()[pub]; ok {
		t.Fatal("unpaired key still labelled")
	}
	if _, _, err := s.AgentInit("", "claude"); err == nil || !strings.Contains(err.Error(), "unpaired") {
		t.Errorf("init re-paired an unpaired agent: %v", err)
	}
	id, err := s.Pair(pub, "claude")
	if err != nil || id == pairID || id == res.IDs[0] {
		t.Fatalf("pair after unpair: %s %v", id, err)
	}
	l := verifyOK(t, s)
	if r := l.Report.Records[id]; r == nil || r.Type != "agent.pair" || r.Author != ident || r.Value["status"] != nil {
		t.Errorf("new pair claim %+v", r)
	}
	if l.AgentLabels()[pub] != "claude" {
		t.Errorf("labels %v", l.AgentLabels())
	}
}

// On an agent-only replica the person's pairing arrives by sync. A second
// label is refused there too, and agent init reports the pairing.
func TestAgentOnlyReplicaKeepsSyncedLabel(t *testing.T) {
	a, _ := initStore(t)
	cloud := newStore(t)
	if _, err := cloud.Sync(a); err != nil {
		t.Fatal(err)
	}
	pub, _, err := cloud.AgentInit("", "server")
	if err != nil {
		t.Fatal(err)
	}
	pairID, err := a.Pair(pub, "server")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cloud.Sync(a); err != nil {
		t.Fatal(err)
	}
	if _, id, err := cloud.AgentInit("", "server"); err != nil || id != pairID {
		t.Errorf("init after pairing: %s %v", id, err)
	}
	if _, _, err := cloud.AgentInit("", "other"); err == nil || !strings.Contains(err.Error(), `paired as "server"`) {
		t.Errorf("second label on the replica: %v", err)
	}
}

// Two of the person's machines can pair one key under two names, and sync
// both claims. Readers show both names, in id order, never one at random.
func TestAgentLabelsTwoNamesAreBothShown(t *testing.T) {
	s, _ := initStore(t)
	pub, first, err := s.AgentInit("", "laptop")
	if err != nil {
		t.Fatal(err)
	}
	second := `{"type":"agent.pair","value":{"key":"` + pub + `","label":"desktop"},"times":{"observed":null},"source":{"system":"self","adapter":"t@2","ref":null}}`
	res := appendLines(t, s, second)
	if len(res.IDs) != 1 {
		t.Fatalf("second claim: %+v", res)
	}
	want := "laptop, desktop"
	if res.IDs[0] < first {
		want = "desktop, laptop"
	}
	for range 20 {
		if got := verifyOK(t, s).AgentLabels()[pub]; got != want {
			t.Fatalf("labels %q, want %q", got, want)
		}
	}
	// Each name the key has is its own; a third is refused.
	for _, name := range []string{"laptop", "desktop"} {
		if _, err := s.Pair(pub, name); err != nil {
			t.Errorf("pair with its name %q: %v", name, err)
		}
	}
	if got := verifyOK(t, s).AgentLabels()[pub]; got != want {
		t.Errorf("pairing again changed the labels: %q", got)
	}
	if _, err := s.Pair(pub, "phone"); err == nil || !strings.Contains(err.Error(), `"laptop" and "desktop"`) && !strings.Contains(err.Error(), `"desktop" and "laptop"`) {
		t.Errorf("pair with a third name: %v", err)
	}
}
