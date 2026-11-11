// Package pack reads a reference pack and places a reading in one of its
// cells. A pack is one file of public reference data: for each measure and
// each cohort on its menu, the table that a method needs. The person's
// value stays on their machine: the table comes to the value.
//
// docs/REFERENCE.md is the format and the method in words;
// tools/pack/check.py is a second implementation.
package pack

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/view"
)

// Format is the value of a pack's "bean" member.
const Format = "pack/0"

// Kind is the one kind of cell that version 0 has: the weight that each
// distinct value of the cohort carries.
const Kind = "weights"

// The method that places a reading in a cell of that kind. A change to
// what Place computes is a new version.
const (
	MethodID      = "weighted-percentile"
	MethodVersion = "1"
)

// MinN is the smallest cohort a cell may hold, and SmallN the size below
// which a result is qualified as coming from a small cohort.
const (
	MinN   = 30
	SmallN = 100
)

// MaxN is the largest n that a cell may state.
const MaxN = 1<<31 - 1

// MaxBytes is the largest pack that is read.
const MaxBytes = 64 << 20

// File is one file of the source that the pack was built from.
type File struct {
	Name, SHA256, URL string
	Bytes             int64
}

// Same says that a reading of this type, in this unit, is the measure's
// quantity in the measure's unit. It is the publisher's claim.
type Same struct{ Type, Unit string }

// Measure is one quantity that the source measured.
type Measure struct {
	Label, Unit string
	Caveats     []string
	Same        []Same
}

// Cohort is one group of people on the pack's menu.
type Cohort struct {
	Text    string
	Caveats []string
}

// Cell is the table of one measure in one cohort: the distinct values in
// rising order, and the weight that each carries. N is the number of
// people.
type Cell struct {
	Measure, Cohort string
	N               int
	Values, Weights []float64
}

// Pack is a reference pack.
type Pack struct {
	Title                       string
	Source, Publisher, Home     string
	Files                       []File
	BuilderName, BuilderVersion string
	Caveats                     []string
	Measures                    map[string]Measure
	Cohorts                     map[string]Cohort
	Cells                       []Cell
	// SHA256 is the digest of the file's bytes: the pack's name.
	SHA256 string
}

var (
	idPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Bytes writes the pack in the one form that a builder writes: bean's
// canonical JSON, and one line feed. The same pack gives the same bytes.
func (p *Pack) Bytes() []byte {
	list := func(texts []string) []any {
		out := []any{}
		for _, t := range texts {
			out = append(out, t)
		}
		return out
	}
	nums := func(xs []float64) []any {
		out := make([]any, len(xs))
		for i, x := range xs {
			out[i] = x
		}
		return out
	}
	files := []any{}
	for _, f := range p.Files {
		files = append(files, map[string]any{"name": f.Name, "sha256": f.SHA256, "bytes": float64(f.Bytes), "url": f.URL})
	}
	measures := map[string]any{}
	for id, m := range p.Measures {
		same := []any{}
		for _, s := range m.Same {
			same = append(same, map[string]any{"type": s.Type, "unit": s.Unit})
		}
		measures[id] = map[string]any{"label": m.Label, "unit": m.Unit, "caveats": list(m.Caveats), "same": same}
	}
	cohorts := map[string]any{}
	for id, c := range p.Cohorts {
		cohorts[id] = map[string]any{"text": c.Text, "caveats": list(c.Caveats)}
	}
	cells := []any{}
	for _, c := range p.Cells {
		cells = append(cells, map[string]any{"measure": c.Measure, "cohort": c.Cohort, "kind": Kind, "n": float64(c.N), "values": nums(c.Values), "weights": nums(c.Weights)})
	}
	return append(core.Canonical(map[string]any{
		"bean":     Format,
		"title":    p.Title,
		"source":   map[string]any{"name": p.Source, "publisher": p.Publisher, "home": p.Home, "files": files},
		"builder":  map[string]any{"name": p.BuilderName, "version": p.BuilderVersion},
		"caveats":  list(p.Caveats),
		"measures": measures,
		"cohorts":  cohorts,
		"cells":    cells,
	}), '\n')
}

// reader takes the members of one object, and reports the first fault.
type reader struct {
	err error
}

func (r *reader) fail(format string, a ...any) {
	if r.err == nil {
		r.err = fmt.Errorf(format, a...)
	}
}

// object returns v as an object with exactly the members named.
func (r *reader) object(v any, where string, members ...string) map[string]any {
	obj, ok := v.(map[string]any)
	if !ok {
		r.fail("%s is not an object", where)
		return map[string]any{}
	}
	for _, name := range slices.Sorted(maps.Keys(obj)) {
		if !slices.Contains(members, name) {
			r.fail("%s has a member that the format does not have: %s", where, view.Safe(name))
		}
	}
	for _, name := range members {
		if _, has := obj[name]; !has {
			r.fail("%s has no %s", where, name)
		}
	}
	return obj
}

// text returns v as text that can be shown as it is: not empty, not long,
// and with no character that hides or reorders text.
func (r *reader) text(v any, where string) string {
	s, ok := v.(string)
	if !ok || s == "" || len(s) > 400 || view.Safe(s) != s || strings.TrimSpace(s) != s {
		r.fail("%s is not plain text of 1 to 400 bytes", where)
		return ""
	}
	return s
}

func (r *reader) texts(v any, where string) []string {
	list, ok := v.([]any)
	if !ok {
		r.fail("%s is not a list", where)
	}
	var out []string
	for _, x := range list {
		out = append(out, r.text(x, where))
	}
	return out
}

// Read reads a pack and checks every rule of the format. It refuses a pack
// that breaks one, and a cell of a kind it does not know: it never reads a
// table by guessing what it means.
func Read(data []byte) (*Pack, error) {
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("a pack is at most %d MiB", MaxBytes>>20)
	}
	v, err := core.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("it is not a pack: %w", err)
	}
	r := &reader{}
	top, _ := v.(map[string]any)
	if format, _ := top["bean"].(string); format != Format {
		return nil, fmt.Errorf("it is not a pack of version 0: its bean member is not %q", Format)
	}
	top = r.object(v, "the pack", "bean", "title", "source", "builder", "caveats", "measures", "cohorts", "cells")
	sum := sha256.Sum256(data)
	p := &Pack{SHA256: hex.EncodeToString(sum[:]), Measures: map[string]Measure{}, Cohorts: map[string]Cohort{}}
	p.Title = r.text(top["title"], "title")
	src := r.object(top["source"], "source", "name", "publisher", "home", "files")
	p.Source, p.Publisher, p.Home = r.text(src["name"], "source.name"), r.text(src["publisher"], "source.publisher"), r.text(src["home"], "source.home")
	files, ok := src["files"].([]any)
	if !ok || len(files) == 0 {
		r.fail("source.files is not a list of one file or more")
	}
	named := map[string]bool{}
	for _, x := range files {
		f := r.object(x, "a source file", "name", "sha256", "bytes", "url")
		file := File{Name: r.text(f["name"], "a source file's name"), URL: r.text(f["url"], "a source file's url")}
		if named[file.Name] {
			r.fail("a source file is named twice")
		}
		named[file.Name] = true
		file.SHA256, _ = f["sha256"].(string)
		if !digestPattern.MatchString(file.SHA256) {
			r.fail("a source file's sha256 is not 64 lower-case hex digits")
		}
		n, isNumber := f["bytes"].(float64)
		if !isNumber || n < 1 || n != math.Trunc(n) || n > 1<<53 {
			r.fail("a source file's bytes is not a whole number above zero")
		}
		file.Bytes = int64(n)
		p.Files = append(p.Files, file)
	}
	b := r.object(top["builder"], "builder", "name", "version")
	p.BuilderName, p.BuilderVersion = r.text(b["name"], "builder.name"), r.text(b["version"], "builder.version")
	p.Caveats = r.texts(top["caveats"], "caveats")
	// Each measure, and no type and unit claimed by two of them: a reading
	// must have one place.
	claimed := map[Same]bool{}
	measures, _ := top["measures"].(map[string]any)
	for _, id := range slices.Sorted(maps.Keys(measures)) {
		if !idPattern.MatchString(id) {
			r.fail("a measure's id is not a short name of letters, digits, dots, - and _")
		}
		m := r.object(measures[id], "measure "+view.Safe(id), "label", "unit", "caveats", "same")
		measure := Measure{Label: r.text(m["label"], "a measure's label"), Unit: r.text(m["unit"], "a measure's unit"), Caveats: r.texts(m["caveats"], "a measure's caveats")}
		same, ok := m["same"].([]any)
		if !ok || len(same) == 0 {
			r.fail("a measure's same is not a list of one entry or more")
		}
		for _, x := range same {
			s := r.object(x, "a same row", "type", "unit")
			row := Same{r.text(s["type"], "a same row's type"), r.text(s["unit"], "a same row's unit")}
			if claimed[row] {
				r.fail("two measures claim %s in %s", row.Type, row.Unit)
			}
			claimed[row] = true
			measure.Same = append(measure.Same, row)
		}
		p.Measures[id] = measure
	}
	cohorts, _ := top["cohorts"].(map[string]any)
	for _, id := range slices.Sorted(maps.Keys(cohorts)) {
		if !idPattern.MatchString(id) {
			r.fail("a cohort's id is not a short name of letters, digits, dots, - and _")
		}
		c := r.object(cohorts[id], "cohort "+view.Safe(id), "text", "caveats")
		p.Cohorts[id] = Cohort{Text: r.text(c["text"], "a cohort's text"), Caveats: r.texts(c["caveats"], "a cohort's caveats")}
	}
	if len(p.Measures) == 0 || len(p.Cohorts) == 0 {
		r.fail("a pack has one measure and one cohort or more")
	}
	cells, ok := top["cells"].([]any)
	if !ok || len(cells) == 0 {
		r.fail("cells is not a list of one cell or more")
	}
	seen := map[[2]string]bool{}
	for _, x := range cells {
		c := r.object(x, "a cell", "measure", "cohort", "kind", "n", "values", "weights")
		cell := Cell{}
		cell.Measure, _ = c["measure"].(string)
		cell.Cohort, _ = c["cohort"].(string)
		where := fmt.Sprintf("the cell of %s in %s", view.Safe(cell.Measure), view.Safe(cell.Cohort))
		if _, has := p.Measures[cell.Measure]; !has {
			r.fail("%s names a measure that the pack does not have", where)
		}
		if _, has := p.Cohorts[cell.Cohort]; !has {
			r.fail("%s names a cohort that the pack does not have", where)
		}
		if seen[[2]string{cell.Measure, cell.Cohort}] {
			r.fail("%s is there twice", where)
		}
		seen[[2]string{cell.Measure, cell.Cohort}] = true
		if kind, _ := c["kind"].(string); kind != Kind {
			r.fail("%s is of kind %q, and this version reads only %q: a table is not read by a guess", where, view.Safe(kind), Kind)
		}
		cell.Values, cell.Weights = r.numbers(c["values"], where), r.numbers(c["weights"], where)
		n, isNumber := c["n"].(float64)
		switch {
		case len(cell.Values) == 0 || len(cell.Values) != len(cell.Weights):
			r.fail("%s does not have one weight for each value", where)
		case !isNumber || n != math.Trunc(n) || n < MinN || n > MaxN:
			r.fail("%s has an n that is not a whole number from %d to %d", where, MinN, MaxN)
		case int(n) < len(cell.Values):
			r.fail("%s has more distinct values than people", where)
		}
		for i := 1; i < len(cell.Values); i++ {
			if !(cell.Values[i] > cell.Values[i-1]) {
				r.fail("%s does not have its values in rising order, each once", where)
				break
			}
		}
		// Weights that are each a number can add up to none.
		var total float64
		for _, w := range cell.Weights {
			total += w
		}
		if math.IsInf(total, 0) {
			r.fail("%s has weights whose sum is too large for a number", where)
		}
		cell.N = int(n)
		p.Cells = append(p.Cells, cell)
	}
	if r.err != nil {
		return nil, fmt.Errorf("it is not a pack that this version reads: %w", r.err)
	}
	return p, nil
}

// numbers returns a list of numbers above zero.
func (r *reader) numbers(v any, where string) []float64 {
	list, ok := v.([]any)
	if !ok {
		r.fail("%s has values or weights that are not a list", where)
	}
	out := make([]float64, 0, len(list))
	for _, x := range list {
		n, isNumber := x.(float64)
		if !isNumber || !(n > 0) || math.IsInf(n, 0) {
			r.fail("%s has a value or a weight that is not a number above zero", where)
			return nil
		}
		out = append(out, n)
	}
	return out
}

// MeasureFor returns the id of the measure that a reading of this type and
// unit is, by the pack's own claim. units lists the units that the pack
// takes for the type: it is empty when the pack has no measure for it.
func (p *Pack) MeasureFor(typ, unit string) (id string, units []string) {
	for _, mid := range slices.Sorted(maps.Keys(p.Measures)) {
		for _, s := range p.Measures[mid].Same {
			if s.Type != typ {
				continue
			}
			units = append(units, s.Unit)
			if s.Unit == unit {
				id = mid
			}
		}
	}
	return id, units
}

// Cell returns the cell of a measure in a cohort, or nil: the pack's
// builder leaves out a cell of fewer than MinN people.
func (p *Pack) Cell(measure, cohort string) *Cell {
	for i := range p.Cells {
		if p.Cells[i].Measure == measure && p.Cells[i].Cohort == cohort {
			return &p.Cells[i]
		}
	}
	return nil
}

// Result is where a value lies in a cell. The percentages are rounded to
// one decimal; the quartiles are values of the sample.
type Result struct {
	N                        int
	Percentile, Below, Equal float64
	P25, P50, P75            float64
}

// Place is the method, version 1: where v lies among the values of a cell.
// docs/REFERENCE.md has it as numbered steps.
func (c *Cell) Place(v float64) (Result, error) {
	if !(v > 0) || math.IsInf(v, 0) {
		return Result{}, errors.New("the reading is not a number above zero")
	}
	res := Result{N: c.N}
	res.Percentile, res.Below, res.Equal, res.P25, res.P50, res.P75 = Place(c.Values, c.Weights, v)
	return res, nil
}

// Place is steps 2 to 4 of the method: values are distinct and in rising
// order, and weights[i] is the weight of values[i]. Every sum runs in that
// order, in IEEE 754 doubles, so that every implementation gets the same
// bits.
func Place(values, weights []float64, v float64) (percentile, below, equal, p25, p50, p75 float64) {
	var total, b, e float64
	for i, value := range values {
		total += weights[i]
		if value < v {
			b += weights[i]
		} else if value == v {
			e = weights[i]
		}
	}
	quartile := func(q float64) float64 {
		var sum float64
		for i, value := range values {
			sum += weights[i]
			if sum >= q*total {
				return value
			}
		}
		return values[len(values)-1]
	}
	return round1(100 * (b + e/2) / total), round1(100 * b / total), round1(100 * e / total), quartile(0.25), quartile(0.5), quartile(0.75)
}

// round1 rounds to one decimal. The conversions keep a compiler from
// fusing the multiplication and the addition into one operation, as Go
// allows and arm64 does: every implementation must round each step.
func round1(y float64) float64 { return math.Floor(float64(float64(10*y)+0.5)) / 10 }
