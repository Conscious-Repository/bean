package coverage

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/pack"
)

type Rule struct {
	Measure   string         `json:"measure"`
	Required  map[string]any `json:"required"`
	Different []string       `json:"different,omitempty"`
}

type Match struct {
	Pin
	Measure    string   `json:"measure"`
	Builder    string   `json:"builder"`
	Survey     string   `json:"survey"`
	Source     string   `json:"source"`
	Cohort     string   `json:"cohort"`
	N          int      `json:"n"`
	Percentile float64  `json:"percentile"`
	Below      float64  `json:"percent_below"`
	Equal      float64  `json:"percent_equal"`
	Method     string   `json:"method"`
	Evidence   Rule     `json:"evidence"`
	Caveats    []string `json:"caveats"`
	// FromPack counts the texts at the start of Caveats that the pack
	// states. bean wrote the others. A pack can state bean's own words, so
	// the two are told apart by where they come from, never by their text.
	FromPack int `json:"-"`
}

type Decline struct {
	SHA256 string `json:"sha256"`
	Reason string `json:"reason"`
	Rules  []Rule `json:"alternatives,omitempty"`
}

type Reference struct {
	SHA256    string `json:"sha256"`
	Publisher string `json:"publisher"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Limit     string `json:"limit"`
}

type Row struct {
	Reason           string         `json:"reason,omitempty"`
	ID               string         `json:"id"`
	Type             string         `json:"type"`
	Report           string         `json:"report_sha256,omitempty"`
	Value            map[string]any `json:"value"`
	Facts            map[string]any `json:"observed_fields"`
	Status           string         `json:"status"`
	Matches          []Match        `json:"matches"`
	Declines         []Decline      `json:"declines,omitempty"`
	References       []Reference    `json:"assay_sources,omitempty"`
	PrintedReference string         `json:"printed_reference,omitempty"`
}

var bound = regexp.MustCompile(`^\s*(?:<=|>=|<|>|≤|≥)\s*[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?\s*$`)

func form(value map[string]any) string {
	if _, ok := value["n"].(float64); ok {
		return "unmatched_numeric"
	}
	if s, ok := value["s"].(string); ok {
		if bound.MatchString(s) {
			return "bounded"
		}
		return "text"
	}
	if n, ok := value["n"]; ok && n == nil {
		return "unreported"
	}
	return "other"
}

// Explain the exact alternatives declared by the publisher. MatchRecord remains
// the authority for placement. A null expected unit requires an absent member.
func rulesFor(r *core.Record, p *pack.Pack, facts map[string]any) []Rule {
	var out []Rule
	for _, mid := range slices.Sorted(maps.Keys(p.Measures)) {
		m := p.Measures[mid]
		for _, alias := range m.Same {
			if alias.Type != r.Type {
				continue
			}
			choices := m.AnyOf[alias]
			if panel, ok := m.When[alias]; ok {
				choices = []pack.Evidence{{"source.system": panel.System, "context.test": panel.Test, "context.panel": panel.Panel}}
			}
			if len(choices) == 0 {
				choices = []pack.Evidence{nil}
			}
			for _, choice := range choices {
				required := map[string]any{"type": r.Type, "value.unit": nil}
				if alias.Unit != "" {
					required["value.unit"] = alias.Unit
				}
				rule := Rule{Measure: mid, Required: required}
				v, hasUnit := r.Value["unit"]
				if (alias.Unit == "" && hasUnit) || (alias.Unit != "" && v != alias.Unit) {
					rule.Different = append(rule.Different, "value.unit")
				}
				for _, key := range slices.Sorted(maps.Keys(choice)) {
					want := choice[key]
					required[key] = want
					var got any = r.Source.System
					present := true
					if k, ok := strings.CutPrefix(key, "context."); ok {
						got, present = r.Context[k]
					}
					if present {
						facts[key] = got
					}
					if got != want {
						rule.Different = append(rule.Different, key)
					}
				}
				out = append(out, rule)
			}
		}
	}
	return out
}

func (s *Sources) Replay(r *core.Record, cohort string) (Row, error) {
	report, _ := r.Context["report_sha256"].(string)
	printed, _ := r.Context["reference_interval"].(string)
	row := Row{ID: r.ID, Type: r.Type, Report: report, Value: r.Value, PrintedReference: printed, Status: form(r.Value),
		Matches: []Match{}, Facts: map[string]any{"type": r.Type, "source.system": r.Source.System}}
	for _, k := range []string{"test", "panel", "fasting", "specimen_type", "ldl_equation", "report_note", "raw_time"} {
		if v, ok := r.Context[k]; ok {
			row.Facts["context."+k] = v
		}
	}
	if u, ok := r.Value["unit"]; ok {
		row.Facts["value.unit"] = u
	}
	test, _ := r.Context["test"].(string)
	panel, _ := r.Context["panel"].(string)
	for _, g := range s.guides {
		for _, e := range g.Entries {
			if r.Source.System == e.Provider && slices.Contains(e.Tests, test) && (len(e.Panels) == 0 || slices.Contains(e.Panels, panel)) {
				row.References = append(row.References, Reference{g.SHA256, g.Publisher, e.Title, e.URL, e.Limit})
			}
		}
	}
	number, isNumber := r.Value["n"].(float64)
	for _, source := range s.packs {
		p := source.pack
		rules := rulesFor(r, p, row.Facts)
		if len(rules) == 0 {
			continue
		}
		mid, _, _ := p.MatchRecord(r)
		if mid == "" {
			row.Declines = append(row.Declines, Decline{source.SHA256, "signed_fields", rules})
			continue
		}
		if !isNumber {
			continue
		}
		c := p.Cell(mid, cohort)
		if c == nil {
			row.Declines = append(row.Declines, Decline{SHA256: source.SHA256, Reason: "no_cohort_table"})
			continue
		}
		res, err := c.Place(number)
		if err != nil {
			row.Declines = append(row.Declines, Decline{SHA256: source.SHA256, Reason: err.Error()})
			continue
		}
		i := slices.IndexFunc(rules, func(a Rule) bool { return a.Measure == mid && len(a.Different) == 0 })
		if i < 0 {
			return row, fmt.Errorf("could not reproduce signed matching evidence for %s", mid)
		}
		row.Matches = append(row.Matches, Match{Pin: source.Pin, Measure: mid, Builder: p.BuilderName, Survey: p.Source, Source: p.Home,
			Cohort: cohort, N: c.N, Percentile: res.Percentile, Below: res.Below, Equal: res.Equal, Method: pack.MethodID + "/" + c.MethodVersion(), Evidence: rules[i],
			Caveats: p.PlacementCaveats(mid, cohort, c.N, r.Author != r.Subject), FromPack: len(p.StatedCaveats(mid, cohort))})
		row.Status = "placed"
	}
	switch row.Status {
	case "bounded":
		row.Reason = "The report gives a bound. An exact rank needs an exact number."
	case "unreported":
		row.Reason = "The signed row has no reported result."
	case "other":
		row.Reason = "This value has no supported lab result form."
	case "text":
		row.Reason = "The report gives text without an exact numeric measurement."
	case "unmatched_numeric":
		row.Reason = "No selected survey pack has a rule for this reading type."
		if len(row.Declines) > 0 {
			row.Reason = "The selected survey rules did not place this reading. See the individual reasons."
		}
	}
	return row, nil
}
