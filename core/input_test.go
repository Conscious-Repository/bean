package core

import (
	"strings"
	"testing"
	"time"
)

// Audit 4, m2 to m4: input that would be stored wrong is refused, and the
// error says what to type.
func TestAddInputRefused(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Skip("no zone data")
	}
	// m3: 02:30 on 2026-03-08 does not exist in Chicago.
	for _, s := range []string{"2026-03-08 02:30", "2026-03-08T02:59"} {
		if _, _, err := ParseWhen(s, chicago); err == nil || !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("ParseWhen(%q) = %v", s, err)
		}
	}
	if start, _, err := ParseWhen("2026-03-08 03:30", chicago); err != nil || start != "2026-03-08T03:30:00-05:00" {
		t.Errorf("03:30: %s %v", start, err)
	}
	// Go accepts one hour digit; that is not a clock change (re-audit 1).
	if start, _, err := ParseWhen("2026-03-09 9:30", chicago); err != nil || start != "2026-03-09T09:30:00-05:00" {
		t.Errorf("9:30: %s %v", start, err)
	}
	// m4: a bad time says which forms work.
	if _, _, err := ParseWhen("yesterday", chicago); err == nil || !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Errorf("yesterday: %v", err)
	}
	s := newStore(t)
	if _, _, err := s.Init(NewKey{}); err != nil {
		t.Fatal(err)
	}
	// m2: a unit needs a number.
	for _, v := range []string{"12,5", " 80", "1e400", "heavy"} {
		if _, err := s.Add("weight", v, AddOptions{Unit: "kg"}); err == nil {
			t.Errorf("Add(weight %q kg) accepted", v)
		}
	}
	if _, err := s.Add("weight", "80.5", AddOptions{Unit: "kg"}); err != nil {
		t.Error(err)
	}
	// m4: a bad type shows the pattern.
	if _, err := s.Add("Weight", "3", AddOptions{}); err == nil || !strings.Contains(err.Error(), "drink.water") {
		t.Errorf("bad type: %v", err)
	}
}
