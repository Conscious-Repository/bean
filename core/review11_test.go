package core

import (
	"testing"
)

// Review 11. Synthetic data only.

// R1: a key paired under two names (two of the person's machines, DECISIONS
// 209) refuses its own name, and the error tells the person to use that
// same name.
func TestReview11TwoLabelsRefuseTheirOwnName(t *testing.T) {
	s, _ := initStore(t)
	pub, _, err := s.AgentInit("", "laptop")
	if err != nil {
		t.Fatal(err)
	}
	// The second machine paired the same key as "desktop" before the copies met.
	appendLines(t, s, `{"type":"agent.pair","value":{"key":"`+pub+`","label":"desktop"},"times":{"observed":null},"source":{"system":"self","adapter":"t@1","ref":null}}`)
	l := verifyOK(t, s)
	t.Logf("labels: %q", l.AgentLabels()[pub])
	_, _, err = s.AgentInit("", "laptop")
	t.Logf("agent init -label laptop: %v", err)
	if err != nil {
		t.Errorf("the key's own label is refused")
	}
	// The error's advice: -label "desktop".
	if _, _, err := s.AgentInit("", "desktop"); err != nil {
		t.Errorf("the label the error advises is refused too: %v", err)
	}
	_, err = s.Pair(pub, "laptop")
	t.Logf("agent pair -label laptop: %v", err)
	if err != nil {
		t.Errorf("pair with the key's own label is refused")
	}
}
