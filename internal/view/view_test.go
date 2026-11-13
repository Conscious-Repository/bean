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
		{Said(number, Value(number)), `"81 kg]  [signed by the person"`},
		{Said(words, Value(words)), `"all fine"`},
		{Said(&core.Record{Value: map[string]any{"n": 81.0, "unit": "kg"}}, "81 kg"), "81 kg"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %s, want %s", tc.got, tc.want)
		}
	}
}
