package pack

import (
	"maps"
	"slices"
	"strings"

	"github.com/Conscious-Repository/bean/core"
)

// Evidence is a conjunction of exact strings on a signed reading.
type Evidence map[string]string

func (r *reader) evidence(value any) []Evidence {
	choices, ok := value.([]any)
	if !ok || len(choices) < 1 || len(choices) > 16 {
		r.fail("any_of needs 1 through 16 alternatives")
		return nil
	}
	var result []Evidence
	for _, choice := range choices {
		fields, ok := choice.(map[string]any)
		if !ok || len(fields) < 1 || len(fields) > 8 {
			r.fail("an evidence alternative needs 1 through 8 fields")
			return nil
		}
		rule := Evidence{}
		for _, key := range slices.Sorted(maps.Keys(fields)) {
			if key != "source.system" && !(strings.HasPrefix(key, "context.") && idPattern.MatchString(strings.TrimPrefix(key, "context.")) && !strings.Contains(strings.TrimPrefix(key, "context."), ".")) {
				r.fail("unsupported evidence field %q", key)
			}
			rule[key] = r.text(fields[key], "evidence value")
		}
		result = append(result, rule)
	}
	return result
}

func (rule Evidence) matches(record *core.Record) bool {
	for key, want := range rule {
		got := record.Source.System
		if key != "source.system" {
			got, _ = record.Context[strings.TrimPrefix(key, "context.")].(string)
		}
		if got != want {
			return false
		}
	}
	return true
}
