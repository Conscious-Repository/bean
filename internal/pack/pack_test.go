package pack

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

// small is a pack of two measures and two cohorts, with weights that are
// counts.
func small() *Pack {
	values := make([]float64, 40)
	weights := make([]float64, 40)
	for i := range values {
		values[i], weights[i] = float64(i+1), 1
	}
	return &Pack{
		Title: "A test pack", Source: "A survey", Publisher: "A publisher", Home: "https://example.org/",
		Files:       []File{{Name: "a.csv", SHA256: strings.Repeat("a", 64), URL: "https://example.org/a.csv", Bytes: 10}},
		BuilderName: "test", BuilderVersion: "1",
		Caveats: []string{"a sample, not a reference range"},
		Measures: map[string]Measure{
			"M1": {Label: "First (mg/dL)", Unit: "mg/dL", Caveats: []string{"measured another way"}, Same: []Same{{"lab.first", "mg/dL"}, {"lab.first", "mg/dl"}}},
			"M2": {Label: "Second (kg)", Unit: "kg", Same: []Same{{"weight", "kg"}}},
		},
		Cohorts: map[string]Cohort{"all": {Text: "everyone"}, "old": {Text: "age 80 and over", Caveats: []string{"80 stands for more"}}},
		Cells: []Cell{
			{Measure: "M1", Cohort: "all", N: 40, Values: values, Weights: weights},
			{Measure: "M2", Cohort: "old", N: 31, Values: []float64{50.5, 60, 70}, Weights: []float64{10, 11, 10}},
		},
	}
}

// TestRoundTrip: what a builder writes reads back as the same pack, under
// the SHA-256 of its bytes, and writes the same bytes again.
// reading is a signed reading of a type and a unit, with the three fields
// that a panel names.
func reading(typ, unit string, panel Panel) *core.Record {
	return &core.Record{Type: typ, Value: map[string]any{"n": 1.0, "unit": unit}, Source: core.Source{System: panel.System},
		Context: map[string]any{"test": panel.Test, "panel": panel.Panel}}
}

func TestRoundTrip(t *testing.T) {
	p := small()
	data := p.Bytes()
	got, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if got.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("sha256 %s", got.SHA256)
	}
	if string(got.Bytes()) != string(data) {
		t.Errorf("a pack that was read writes other bytes")
	}
	got.SHA256 = ""
	if !reflect.DeepEqual(got.Cells, p.Cells) || !reflect.DeepEqual(got.Measures["M1"], p.Measures["M1"]) || got.Cohorts["old"].Caveats[0] != "80 stands for more" {
		t.Errorf("read back: %+v", got)
	}
	if id, units, _ := got.MatchRecord(reading("lab.first", "mg/dl", Panel{})); id != "M1" || len(units) != 2 {
		t.Errorf("the measure of a reading: %q %v", id, units)
	}
	if id, units, _ := got.MatchRecord(reading("lab.first", "mmol/L", Panel{})); id != "" || len(units) != 2 {
		t.Errorf("another unit: %q %v", id, units)
	}
	if id, units, _ := got.MatchRecord(reading("lab.third", "mg/dL", Panel{})); id != "" || units != nil {
		t.Errorf("another type: %q %v", id, units)
	}
	if got.Cell("M1", "old") != nil || got.Cell("M2", "old") == nil {
		t.Errorf("Cell finds what is not there, or not what is")
	}
}

// TestPlace: the method, on a cell where every number can be done by hand.
func TestPlace(t *testing.T) {
	c := &Cell{N: 40, Values: []float64{1, 2, 3, 4}, Weights: []float64{1, 1, 1, 1}}
	for _, tc := range []struct {
		v    float64
		want Result
	}{
		{2, Result{40, 37.5, 25, 25, 1, 2, 3}},
		{2.5, Result{40, 50, 50, 0, 1, 2, 3}},
		{0.5, Result{40, 0, 0, 0, 1, 2, 3}},
		{9, Result{40, 100, 100, 0, 1, 2, 3}},
	} {
		got, err := c.Place(tc.v)
		if err != nil || got != tc.want {
			t.Errorf("Place(%v) = %+v, %v; want %+v", tc.v, got, err, tc.want)
		}
	}
	for _, v := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := c.Place(v); err == nil {
			t.Errorf("Place(%v) gives a result", v)
		}
	}
	// The vectors of docs/REFERENCE.md. In the last one the weights add up
	// to 0.4, and three quarters of that is a hair above 0.3 in doubles:
	// the first value does not reach it. tools/pack/run.sh asks the Python
	// check for the same four.
	for _, tc := range []struct {
		values, weights []float64
		v               float64
		want            Result
	}{
		{[]float64{1, 2, 3, 4}, []float64{1, 1, 1, 1}, 2, Result{30, 37.5, 25, 25, 1, 2, 3}},
		{[]float64{1, 2, 3, 4}, []float64{1, 1, 1, 1}, 2.5, Result{30, 50, 50, 0, 1, 2, 3}},
		{[]float64{10, 20, 30}, []float64{1, 1, 8}, 30, Result{30, 60, 20, 80, 30, 30, 30}},
		{[]float64{10, 20}, []float64{0.3, 0.1}, 15, Result{30, 75, 75, 0, 10, 10, 20}},
	} {
		got, err := (&Cell{N: 30, Values: tc.values, Weights: tc.weights}).Place(tc.v)
		if err != nil || got != tc.want {
			t.Errorf("vector %v %v at %v: %+v, %v; want %+v", tc.values, tc.weights, tc.v, got, err, tc.want)
		}
	}
	// Weights that are not equal move the quartiles.
	w := &Cell{N: 30, Values: []float64{10, 20, 30}, Weights: []float64{1, 1, 8}}
	if got, _ := w.Place(30); got != (Result{30, 60, 20, 80, 30, 30, 30}) {
		t.Errorf("weighted: %+v", got)
	}
}

// TestRefuses: each change breaks one rule of docs/REFERENCE.md, and Read
// refuses the pack.
func TestRefuses(t *testing.T) {
	change := func(edit func(top map[string]any)) []byte {
		v, err := core.Parse(small().Bytes())
		if err != nil {
			t.Fatal(err)
		}
		top := v.(map[string]any)
		edit(top)
		return core.Canonical(top)
	}
	cell := func(top map[string]any, i int) map[string]any { return top["cells"].([]any)[i].(map[string]any) }
	measure := func(top map[string]any, id string) map[string]any {
		return top["measures"].(map[string]any)[id].(map[string]any)
	}
	for name, edit := range map[string]func(top map[string]any){
		"another format":             func(top map[string]any) { top["bean"] = "pack/1" },
		"a member too many":          func(top map[string]any) { top["signature"] = "x" },
		"a member missing":           func(top map[string]any) { delete(top, "caveats") },
		"no title":                   func(top map[string]any) { top["title"] = "" },
		"a title with a line end":    func(top map[string]any) { top["title"] = "a\nb" },
		"a title that reorders text": func(top map[string]any) { top["title"] = "a\u202eb" },
		"a title with a space after": func(top map[string]any) { top["title"] = "a " },
		"no source file":             func(top map[string]any) { top["source"].(map[string]any)["files"] = []any{} },
		"a digest in upper case": func(top map[string]any) {
			top["source"].(map[string]any)["files"].([]any)[0].(map[string]any)["sha256"] = strings.Repeat("A", 64)
		},
		"a file of no bytes": func(top map[string]any) {
			top["source"].(map[string]any)["files"].([]any)[0].(map[string]any)["bytes"] = float64(0)
		},
		"a source file twice": func(top map[string]any) {
			files := top["source"].(map[string]any)["files"].([]any)
			top["source"].(map[string]any)["files"] = append(files, files[0])
		},
		"a measure that takes no reading":            func(top map[string]any) { measure(top, "M2")["same"] = []any{} },
		"more people than a cell may state":          func(top map[string]any) { cell(top, 0)["n"] = float64(MaxN + 1) },
		"a caveat that is a number":                  func(top map[string]any) { top["caveats"] = []any{float64(1)} },
		"another kind of cell":                       func(top map[string]any) { cell(top, 0)["kind"] = "quantiles" },
		"a cell member too many":                     func(top map[string]any) { cell(top, 0)["note"] = "x" },
		"values not rising":                          func(top map[string]any) { cell(top, 1)["values"] = []any{float64(60), 50.5, float64(70)} },
		"a value twice":                              func(top map[string]any) { cell(top, 1)["values"] = []any{50.5, 50.5, float64(70)} },
		"a value of zero":                            func(top map[string]any) { cell(top, 1)["values"] = []any{float64(0), float64(60), float64(70)} },
		"a negative weight":                          func(top map[string]any) { cell(top, 1)["weights"] = []any{float64(10), float64(-11), float64(10)} },
		"a weight that is text":                      func(top map[string]any) { cell(top, 1)["weights"] = []any{float64(10), "11", float64(10)} },
		"a weight too large":                         func(top map[string]any) { cell(top, 1)["weights"] = []any{1e16, float64(11), float64(10)} },
		"a weight of a whole 1e21":                   func(top map[string]any) { cell(top, 1)["weights"] = []any{1e21, float64(11), float64(10)} },
		"a value too small":                          func(top map[string]any) { cell(top, 1)["values"] = []any{1e-13, float64(60), float64(70)} },
		"a title with a space no wider than nothing": func(top map[string]any) { top["title"] = "a\u200bb" },
		"a title with a blank filler":                func(top map[string]any) { top["title"] = "\u3164" },
		"a title with a no-break space":              func(top map[string]any) { top["title"] = "a\u00a0b" },
		"a title with a tag character":               func(top map[string]any) { top["title"] = "a\U000e0041b" },
		"a title with a noncharacter":                func(top map[string]any) { top["title"] = "a\ufffeb" },
		"a measure that claims a share": func(top map[string]any) {
			measure(top, "M2")["same"] = []any{map[string]any{"type": "share", "unit": "{record}"}}
		},
		"a measure that claims a comparison": func(top map[string]any) {
			measure(top, "M2")["same"] = []any{map[string]any{"type": "comparison.percentile", "unit": "{percentile}"}}
		},
		"a weight missing":            func(top map[string]any) { cell(top, 1)["weights"] = []any{float64(10), float64(11)} },
		"no values":                   func(top map[string]any) { cell(top, 1)["values"], cell(top, 1)["weights"] = []any{}, []any{} },
		"no people":                   func(top map[string]any) { cell(top, 1)["n"] = float64(0) },
		"fewer people than none":      func(top map[string]any) { cell(top, 1)["n"] = float64(-3) },
		"an n with a fraction":        func(top map[string]any) { cell(top, 1)["n"] = 31.5 },
		"fewer people than values":    func(top map[string]any) { cell(top, 0)["n"] = float64(39) },
		"a measure that is not there": func(top map[string]any) { cell(top, 0)["measure"] = "M3" },
		"a cohort that is not there":  func(top map[string]any) { cell(top, 0)["cohort"] = "young" },
		"a cell twice":                func(top map[string]any) { top["cells"] = append(top["cells"].([]any), cell(top, 0)) },
		"no cell":                     func(top map[string]any) { top["cells"] = []any{} },
		"no measure":                  func(top map[string]any) { top["measures"] = map[string]any{} },
		"an id with a space": func(top map[string]any) {
			top["cohorts"].(map[string]any)["very old"] = top["cohorts"].(map[string]any)["old"]
		},
		"a type and unit claimed twice": func(top map[string]any) {
			measure(top, "M2")["same"] = []any{map[string]any{"type": "lab.first", "unit": "mg/dL"}}
		},
		"a same row with no unit": func(top map[string]any) {
			measure(top, "M2")["same"] = []any{map[string]any{"type": "weight"}}
		},
		"a measure member too many": func(top map[string]any) { measure(top, "M1")["loinc"] = "2093-3" },
	} {
		if p, err := Read(change(edit)); err == nil {
			t.Errorf("%s: read as a pack: %+v", name, p.Title)
		}
	}
	// What is not JSON, a member twice, and what is too large.
	for name, data := range map[string][]byte{
		"not JSON":       []byte("pack"),
		"a list":         []byte("[]"),
		"a member twice": []byte(strings.Replace(string(small().Bytes()), `{"bean":"pack/0",`, `{"bean":"pack/0","bean":"pack/0",`, 1)),
		"too large":      make([]byte, MaxBytes+1),
	} {
		if _, err := Read(data); err == nil {
			t.Errorf("%s: read as a pack", name)
		}
	}
	// A cell of one person with one value is read: a cell has no least size.
	one := func(top map[string]any) {
		c := cell(top, 1)
		c["n"], c["values"], c["weights"] = float64(1), []any{float64(52)}, []any{float64(1)}
	}
	if p, err := Read(change(one)); err != nil {
		t.Errorf("a cell of one person: %v", err)
	} else if res, err := p.Cells[1].Place(52); err != nil || res.N != 1 || res.Percentile != 50 {
		t.Errorf("a value in a cell of one person with that value: %+v, %v", res, err)
	}
	// The largest n that a cell may state is read.
	if _, err := Read(change(func(top map[string]any) { cell(top, 0)["n"] = float64(MaxN) })); err != nil {
		t.Errorf("an n of %d: %v", MaxN, err)
	}
	// The unchanged pack passes: the changes above are what is refused.
	if _, err := Read(change(func(map[string]any) {})); err != nil {
		t.Errorf("the unchanged pack: %v", err)
	}
}

func TestPanelMatch(t *testing.T) {
	p := small()
	key := Same{"lab.saturation", "%"}
	panel := Panel{"quest", "% SATURATION", "IRON AND TOTAL IRON BINDING CAPACITY"}
	m := p.Measures["M1"]
	m.Same = append(m.Same, key)
	m.When = map[Same]Panel{key: panel}
	p.Measures["M1"] = m
	data := p.Bytes()
	got, err := Read(data)
	if err != nil || !strings.Contains(string(data), `"bean":"pack/1"`) || string(got.Bytes()) != string(data) {
		t.Fatalf("qualified pack round trip: %v", err)
	}
	for _, tc := range []struct {
		unit  string
		panel Panel
		want  string
	}{
		{"%", panel, "M1"},
		{"%", Panel{}, ""},
		{"%", Panel{"quest", "% SATURATION", "OXYGEN SATURATION"}, ""},
		{"%", Panel{"quest", "SATURATION", panel.Panel}, ""},
		{"%", Panel{"other", panel.Test, panel.Panel}, ""},
		{"mg/dL", panel, ""},
	} {
		id, _, mismatch := got.MatchRecord(reading(key.Type, tc.unit, tc.panel))
		if id != tc.want || mismatch != (tc.want == "" && tc.unit == key.Unit) {
			t.Errorf("match(%q, %+v) = %q, %v, want %q", tc.unit, tc.panel, id, mismatch, tc.want)
		}
	}
	for _, mutation := range [][2]string{
		{`"pack/1"`, `"pack/0"`}, {`"system":"quest"`, `"system":""`},
		{`"test":"% SATURATION"`, `"test":null`}, {`"panel":`, `"unknown":`},
	} {
		if _, err := Read([]byte(strings.Replace(string(data), mutation[0], mutation[1], 1))); err == nil {
			t.Fatalf("accepted %v", mutation)
		}
	}
}

func TestSignedEvidenceAlternatives(t *testing.T) {
	p := small()
	key := p.Measures["M1"].Same[0]
	m := p.Measures["M1"]
	m.AnyOf = map[Same][]Evidence{key: {
		{"source.system": "lab", "context.fasting": "Yes"},
		{"context.specimen_type": "plasma", "context.fasting": "Yes"},
	}}
	p.Measures["M1"] = m
	data := p.Bytes()
	got, err := Read(data)
	if err != nil || string(data) != string(got.Bytes()) {
		t.Fatalf("round trip: %v", err)
	}
	for _, tc := range []struct {
		system string
		ctx    map[string]any
		want   string
	}{
		{"lab", map[string]any{"fasting": "Yes"}, "M1"},
		{"other", map[string]any{"fasting": "Yes", "specimen_type": "plasma"}, "M1"},
		{"lab", map[string]any{"fasting": "No"}, ""},
		{"lab", map[string]any{"fasting": true}, ""},
		{"other", map[string]any{"fasting": "Yes"}, ""},
		{"lab", nil, ""},
	} {
		rec := core.Record{Type: key.Type, Value: map[string]any{"unit": key.Unit}, Source: core.Source{System: tc.system}, Context: tc.ctx}
		if mid, _, _ := got.MatchRecord(&rec); mid != tc.want {
			t.Errorf("matched %q, want %q", mid, tc.want)
		}
	}
	for _, pair := range [][2]string{
		{`"bean":"pack/2"`, `"bean":"pack/1"`},
		{`"context.fasting":"Yes"`, `"value.n":"Yes"`},
		{`"context.fasting":"Yes"`, `"context.fasting":true`},
		{`{"context.fasting":"Yes","source.system":"lab"}`, `{}`},
	} {
		if _, err := Read([]byte(strings.Replace(string(data), pair[0], pair[1], 1))); err == nil {
			t.Errorf("accepted mutation %v", pair)
		}
	}
}

func TestNonnegativeCells(t *testing.T) {
	p := small()
	c := &p.Cells[0]
	c.AllowZero = true
	c.Values[0] = 0
	raw := p.Bytes()
	got, err := Read(raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Bytes()) != string(raw) {
		t.Fatal("pack/3 round trip differs")
	}
	for _, v := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := got.Cells[0].Place(v); err == nil {
			t.Fatalf("placed %v", v)
		}
	}
	result, err := got.Cells[0].Place(0)
	if err != nil || result.Below != 0 || result.Equal != 2.5 || result.Percentile != 1.3 || got.Cells[0].MethodVersion() != "2" {
		t.Fatalf("zero: %+v %v", result, err)
	}
	if _, err := got.Cells[1].Place(0); err == nil || got.Cells[1].MethodVersion() != "1" {
		t.Fatal("positive-only cell changed")
	}
	for _, version := range []string{"pack/0", "pack/1", "pack/2"} {
		if _, err := Read([]byte(strings.Replace(string(raw), "pack/3", version, 1))); err == nil {
			t.Fatalf("%s accepted nonnegative cells", version)
		}
	}
	c.Weights[0] = 0
	if _, err := Read(p.Bytes()); err == nil {
		t.Fatal("accepted zero weight")
	}
}

func TestRatioOmittedUnit(t *testing.T) {
	p := small()
	key := Same{"lab.bun-creatinine-ratio", ""}
	panel := Panel{"labcorp", "BUN/Creatinine Ratio", "Comp. Metabolic Panel (14)"}
	p.Measures["M1"] = Measure{Label: "BUN/creatinine", Unit: "1", Same: []Same{key}, When: map[Same]Panel{key: panel}}
	raw := p.Bytes()
	got, err := Read(raw)
	if err != nil || string(got.Bytes()) != string(raw) {
		t.Fatalf("round trip: %v", err)
	}
	for _, version := range []string{Format, PanelFormat, EvidenceFormat, NonnegativeFormat} {
		if _, err := Read([]byte(strings.Replace(string(raw), RatioFormat, version, 1))); err == nil {
			t.Fatalf("%s accepted omitted unit", version)
		}
	}
	rec := core.Record{Type: key.Type, Source: core.Source{System: panel.System}, Context: map[string]any{"test": panel.Test, "panel": panel.Panel}, Value: map[string]any{"n": 12}}
	if id, _, _ := got.MatchRecord(&rec); id != "M1" {
		t.Fatal("named ratio declined")
	}
	for _, u := range []any{"", "1", "mg/dL", nil, 42} {
		rec.Value["unit"] = u
		if id, _, _ := got.MatchRecord(&rec); id != "" {
			t.Fatalf("accepted explicit unit %v", u)
		}
	}
	delete(rec.Value, "unit")
	rec.Context["panel"] = "OTHER"
	if id, _, _ := got.MatchRecord(&rec); id != "" {
		t.Fatal("accepted different panel")
	}
	m := p.Measures["M1"]
	m.Unit = "mg/dL"
	p.Measures["M1"] = m
	if _, err := Read(p.Bytes()); err == nil {
		t.Fatal("dimensioned measure accepted")
	}
	m.Unit = "1"
	m.When = nil
	p.Measures["M1"] = m
	if _, err := Read(p.Bytes()); err == nil {
		t.Fatal("unguarded unit omission")
	}
}

// TestRulesOfTheNewerFormats holds the rules of pack/1 to pack/4, and a few
// of every format, that no other test holds: each was taken out of the
// reader in turn, and the other tests passed.
func TestRulesOfTheNewerFormats(t *testing.T) {
	edit := func(data []byte, f func(top map[string]any)) []byte {
		v, err := core.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		f(v.(map[string]any))
		return core.Canonical(v)
	}
	row := func(top map[string]any) map[string]any {
		return top["measures"].(map[string]any)["M1"].(map[string]any)["same"].([]any)[0].(map[string]any)
	}
	refused := func(name string, data []byte) {
		t.Helper()
		if p, err := Read(data); err == nil {
			t.Errorf("%s: read as a pack with the label %s", name, strings.SplitN(string(p.Bytes()), `"`, 5)[3])
		}
	}
	read := func(name string, data []byte) *Pack {
		t.Helper()
		p, err := Read(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return p
	}
	key := small().Measures["M1"].Same[0]
	panel := Panel{"quest", "A TEST", "A PANEL"}
	// with returns the test pack with a change, as the bytes that a builder writes.
	with := func(f func(p *Pack)) []byte {
		p := small()
		f(p)
		return p.Bytes()
	}
	when := func(p *Pack) {
		m := p.Measures["M1"]
		m.When = map[Same]Panel{key: panel}
		p.Measures["M1"] = m
	}
	anyOf := func(p *Pack) {
		m := p.Measures["M1"]
		m.AnyOf = map[Same][]Evidence{key: {{"source.system": "quest"}}}
		p.Measures["M1"] = m
	}
	zero := func(p *Pack) { p.Cells[0].AllowZero, p.Cells[0].Values[0] = true, 0 }
	ratio := func(p *Pack) {
		same := Same{"lab.a-ratio", ""}
		p.Measures["M2"] = Measure{Label: "A ratio", Unit: "1", Same: []Same{same}, When: map[Same]Panel{same: panel}}
	}

	// Every format.
	base := small().Bytes()
	read("a title of 400 bytes", edit(base, func(top map[string]any) { top["title"] = strings.Repeat("a", 400) }))
	refused("a title of 401 bytes", edit(base, func(top map[string]any) { top["title"] = strings.Repeat("a", 401) }))
	for name, c := range map[string]string{"a paragraph end": " ", "a private use character": "", "a Hangul filler": "ᅠ", "a halfwidth filler": "ﾠ", "a noncharacter of the Arabic block": "﷐"} {
		refused("a title with "+name, edit(base, func(top map[string]any) { top["title"] = "a" + c + "b" }))
	}
	refused("a source file with half a byte", edit(base, func(top map[string]any) {
		top["source"].(map[string]any)["files"].([]any)[0].(map[string]any)["bytes"] = 1.5
	}))
	refused("a measure id with a space", edit(base, func(top map[string]any) {
		measures := top["measures"].(map[string]any)
		measures["M 2"] = measures["M2"]
		delete(measures, "M2")
		top["cells"].([]any)[1].(map[string]any)["measure"] = "M 2"
	}))
	for _, typ := range []string{"agent.note", "reference.file", "bean.genesis"} {
		refused("a measure that claims "+typ, edit(base, func(top map[string]any) {
			top["measures"].(map[string]any)["M2"].(map[string]any)["same"] = []any{map[string]any{"type": typ, "unit": "kg"}}
		}))
	}

	// pack/2: the alternatives of an entry.
	evidence := with(anyOf)
	if !strings.Contains(string(evidence), `"bean":"pack/2"`) {
		t.Fatalf("a pack with any_of does not get the label pack/2")
	}
	many := func(n int) []any {
		var out []any
		for i := range n {
			out = append(out, map[string]any{"source.system": fmt.Sprintf("lab-%d", i)})
		}
		return out
	}
	fields := func(n int) []any {
		one := map[string]any{}
		for i := range n {
			one[fmt.Sprintf("context.f%d", i)] = "x"
		}
		return []any{one}
	}
	read("16 alternatives", edit(evidence, func(top map[string]any) { row(top)["any_of"] = many(16) }))
	refused("17 alternatives", edit(evidence, func(top map[string]any) { row(top)["any_of"] = many(17) }))
	refused("no alternative", edit(evidence, func(top map[string]any) { row(top)["any_of"] = []any{} }))
	read("an alternative of 8 fields", edit(evidence, func(top map[string]any) { row(top)["any_of"] = fields(8) }))
	refused("an alternative of 9 fields", edit(evidence, func(top map[string]any) { row(top)["any_of"] = fields(9) }))
	refused("a field under a field of the context", edit(evidence, func(top map[string]any) {
		row(top)["any_of"] = []any{map[string]any{"context.a.b": "x"}}
	}))
	refused("when and any_of on one entry", edit(evidence, func(top map[string]any) {
		row(top)["when"] = map[string]any{"system": "quest", "test": "A TEST", "panel": "A PANEL"}
	}))
	// Also beside an entry that has its alternatives alone: the label is
	// then right, and only the entry with both is wrong.
	refused("when and any_of on one entry, beside a good entry", edit(evidence, func(top map[string]any) {
		second := top["measures"].(map[string]any)["M1"].(map[string]any)["same"].([]any)[1].(map[string]any)
		second["when"] = map[string]any{"system": "quest", "test": "A TEST", "panel": "A PANEL"}
		second["any_of"] = []any{map[string]any{"source.system": "quest"}}
	}))

	// pack/3: a table with a zero. An earlier label does not take one, also
	// with the entry that the label requires.
	nonneg := with(zero)
	if !strings.Contains(string(nonneg), `"bean":"pack/3"`) {
		t.Fatalf("a pack with a zero does not get the label pack/3")
	}
	refused("a negative value in a table with a zero", edit(nonneg, func(top map[string]any) {
		top["cells"].([]any)[0].(map[string]any)["values"].([]any)[0] = float64(-1)
	}))
	refused("pack/3 with no table that takes a zero", []byte(strings.Replace(string(base), `"bean":"pack/0"`, `"bean":"pack/3"`, 1)))
	refused("pack/1 with a table that takes a zero", []byte(strings.Replace(string(with(func(p *Pack) { when(p); zero(p) })), `"bean":"pack/3"`, `"bean":"pack/1"`, 1)))
	refused("pack/2 with a table that takes a zero", []byte(strings.Replace(string(with(func(p *Pack) { anyOf(p); zero(p) })), `"bean":"pack/3"`, `"bean":"pack/2"`, 1)))
	// A later label takes what the labels before it take.
	for name, f := range map[string]func(p *Pack){
		"pack/3 with when and any_of entries": func(p *Pack) {
			zero(p)
			m := p.Measures["M1"]
			m.When = map[Same]Panel{m.Same[1]: panel}
			m.AnyOf = map[Same][]Evidence{key: {{"source.system": "quest"}}}
			p.Measures["M1"] = m
		},
		"pack/4 with an any_of entry and a table that takes a zero": func(p *Pack) { anyOf(p); zero(p); ratio(p) },
	} {
		data := with(f)
		if got := read(name, data); string(got.Bytes()) != string(data) || !strings.Contains(string(data), `"bean":"`+name[:6]+`"`) {
			t.Errorf("%s: read and written again as other bytes, or under another label", name)
		}
	}
}
