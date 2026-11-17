package view

import (
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

// TestNoTextPassesForAMark: a row ends in the tool's mark of who signed,
// in square brackets. A text that holds a bracket shows in quotation
// marks, and so do the words of a key that is not the person's.
func TestNoTextPassesForAMark(t *testing.T) {
	number := &core.Record{Value: map[string]any{"n": 81.0, "unit": "kg]  [signed by the person"}}
	words := &core.Record{Value: map[string]any{"s": "all fine"}}
	for _, tc := range []struct{ got, want string }{
		{Unmarked("81 kg"), "81 kg"},
		{Unmarked(Value(number)), `"81 kg]  [signed by the person"`},
		{Said(words, Value(words)), `"all fine"`},
		// The words of another key are quoted once, with a bracket or without.
		{Said(&core.Record{Value: map[string]any{"s": "hi [x]"}}, "hi [x]"), `"hi [x]"`},
		// A listing of the log shows a value of the log itself as it is.
		{Said(&core.Record{Value: map[string]any{"part": 1.0}}, `{"part":1,"sealed":["sha256:aa"]}`), `{"part":1,"sealed":["sha256:aa"]}`},
		{Said(&core.Record{Value: map[string]any{"n": 81.0, "unit": "kg"}}, "81 kg"), "81 kg"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %s, want %s", tc.got, tc.want)
		}
	}
}

// Characters that a terminal draws as nothing are written out, so a text
// cannot hide words or pass for an empty one.
func TestSafeShowsInvisible(t *testing.T) {
	for in, want := range map[string]string{
		"\u200b":                                     `\u200b`,
		"a\u00adb\ufeffc\u2060d\u180e":               `a\u00adb\ufeffc\u2060d\u180e`,
		"\U000e0041\U000e0042 secret":                `\U000e0041\U000e0042 secret`,
		"fine\U000e0100\U000e01ef":                   `fine\U000e0100\U000e01ef`,
		"vs\ufe0f\u034f\u115f\u1160":                 `vs\ufe0f\u034f\u115f\u1160`,
		"\u3164\uffa0\u17b4\u17b5\u2800\ufff0\ufffb": `\u3164\uffa0\u17b4\u17b5\u2800\ufff0\ufffb`,
		"\u2028\u202e\u0085":                         `\u2028\u202e\u0085`,
		"Ünïcödé 日本 ✓ a b":                           "Ünïcödé 日本 ✓ a b",
		"café ü":                                     "café ü",
	} {
		if got := Safe(in); got != want {
			t.Errorf("Safe(%q) = %q, want %q", in, got, want)
		}
	}
}
