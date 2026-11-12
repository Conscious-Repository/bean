package pack

import (
	"crypto/sha256"
	"encoding/hex"
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
	if id, units := got.MeasureFor("lab.first", "mg/dl"); id != "M1" || len(units) != 2 {
		t.Errorf("MeasureFor: %q %v", id, units)
	}
	if id, units := got.MeasureFor("lab.first", "mmol/L"); id != "" || len(units) != 2 {
		t.Errorf("another unit: %q %v", id, units)
	}
	if id, units := got.MeasureFor("lab.third", "mg/dL"); id != "" || units != nil {
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
		"a title that reorders text": func(top map[string]any) { top["title"] = "a‮b" },
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
		"too few people":              func(top map[string]any) { cell(top, 1)["n"] = float64(29) },
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
	// The largest n that a cell may state is read.
	if _, err := Read(change(func(top map[string]any) { cell(top, 0)["n"] = float64(MaxN) })); err != nil {
		t.Errorf("an n of %d: %v", MaxN, err)
	}
	// The unchanged pack passes: the changes above are what is refused.
	if _, err := Read(change(func(map[string]any) {})); err != nil {
		t.Errorf("the unchanged pack: %v", err)
	}
}
