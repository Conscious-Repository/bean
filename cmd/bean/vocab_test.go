package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The agent allowlist is app level (SPEC 9.9): the built-in vocabulary
// plus an optional vocabulary.md in the data folder that adds terms.
func TestVocabularyPersonal(t *testing.T) {
	s, _ := mcpStore(t)
	mcpWrite = true
	t.Cleanup(func() { mcpWrite = false })
	if _, _, err := s.AgentInit("", "claude"); err != nil {
		t.Fatal(err)
	}
	write := func(typ string) (map[string]any, bool) {
		return call(t, s, "log_entry", map[string]any{"type": typ, "value": "1", "at": "2026-10-24"})
	}
	// Built in: resting heart rate and lab results.
	for _, typ := range []string{"heart.resting", "lab.glucose", "lab.bun-creatinine-ratio"} {
		if m, isErr := write(typ); isErr {
			t.Errorf("agent cannot write %s: %v", typ, m)
		}
	}
	for _, typ := range []string{"lab.glucose.fasting", "kombucha", "cgm.libre"} {
		if _, isErr := write(typ); !isErr {
			t.Errorf("agent wrote %s with no personal vocabulary", typ)
		}
	}

	// A personal file adds terms, exact and prefix, and the AI sees them.
	personal := "# Mine\n\n| Type | Value | Useful details |\n|---|---|---|\n| `kombucha` | number, `mL` | `brand` |\n| `cgm.<sensor>` (`libre`) | number, `mg/dL` | |\n"
	if err := os.WriteFile(filepath.Join(s.Dir, "vocabulary.md"), []byte(personal), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{"kombucha", "cgm.libre", "food"} {
		if m, isErr := write(typ); isErr {
			t.Errorf("agent cannot write %s: %v", typ, m)
		}
	}
	if _, isErr := write("cgm.libre.raw"); !isErr {
		t.Error("prefix term allowed two levels")
	}
	init := rpc(t, s, `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{}}`)
	if text := init[0]["result"].(map[string]any)["instructions"].(string); !strings.Contains(text, "`kombucha`") || !strings.Contains(text, "# Type vocabulary") || strings.Contains(text, s.Dir) {
		t.Error("instructions lack the personal or the built-in vocabulary")
	}

	// A personal term never redefines a built-in one or opens a setup or
	// core type. The file is refused as a whole, loudly.
	for _, row := range []string{"`food`", "`heart.resting`", "`lab.<test>`", "`exercise.swim`", "`drink.<kind>`", "`bean.note`", "`agent.pair`", "`device.<x>`", "`exercise.r<x>`", "`symptom.head<x>`"} {
		bad := personal + "| " + row + " | text | |\n"
		if err := os.WriteFile(filepath.Join(s.Dir, "vocabulary.md"), []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		m, isErr := write("kombucha")
		if !isErr || !strings.Contains(m["error"].(string), "vocabulary.md") || strings.Contains(m["error"].(string), s.Dir) {
			t.Errorf("personal %s: %v", row, m)
		}
		if r := rpc(t, s, `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{}}`); r[0]["error"] == nil {
			t.Errorf("personal %s: initialize passed", row)
		}
	}
}
