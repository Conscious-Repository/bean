package nhanes

import (
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/internal/xpt"
	"github.com/Conscious-Repository/bean/internal/xpt/xpttest"
)

func load(t *testing.T, dir string, names ...string) map[string]*xpt.File {
	t.Helper()
	out := map[string]*xpt.File{}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name+".XPT"))
		if err != nil {
			t.Fatal(err)
		}
		f, err := xpt.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		out[name] = f
	}
	return out
}

func cohort(t *testing.T, sex string, lo, hi int, rules ...Rule) Cohort {
	t.Helper()
	c, err := NewCohort(sex, lo, hi)
	if err != nil {
		t.Fatal(err)
	}
	c.Rules = append(c.Rules, rules...)
	return c
}

var synthetic = []string{"TCHOL_J", "HDL_J", "DEMO_J", "BMX_J", "DR1TOT_J"}

// person is one row of the cohort, as a slower and plainer reading of the
// method collects it.
type person struct{ x, w float64 }

// plain selects the cohort by joining whole rows first, then computes the
// shares with exact sums of the sorted weights. It shares no step with
// Compare but the rounding.
func plain(t *testing.T, files map[string]*xpt.File, a Analyte, c Cohort) []person {
	t.Helper()
	cache := map[string]map[float64]float64{}
	get := func(dataset, variable string) map[float64]float64 {
		if c := cache[dataset+"."+variable]; c != nil {
			return c
		}
		seqn, err := files[dataset].Column("SEQN")
		if err != nil {
			t.Fatal(err)
		}
		v, err := files[dataset].Column(variable)
		if err != nil {
			t.Fatal(err)
		}
		out := map[float64]float64{}
		for i := range seqn {
			out[seqn[i]] = v[i]
		}
		cache[dataset+"."+variable] = out
		return out
	}
	wd, wv := c.Weight()
	weights := get(wd, wv)
	var out []person
next:
	for s, x := range get(a.Dataset, a.Variable) {
		w, ok := weights[s]
		if math.IsNaN(x) || !ok || math.IsNaN(w) || w <= 0 {
			continue
		}
		for _, r := range c.Rules {
			y, ok := get(r.Dataset, r.Variable)[s]
			if !ok || math.IsNaN(y) || y < r.Min || y > r.Max {
				continue next
			}
		}
		out = append(out, person{x, w})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].x < out[j].x })
	return out
}

func TestCompareAgainstPlain(t *testing.T) {
	files := load(t, "testdata", synthetic...)
	tc, _ := AnalyteFor("lab.cholesterol-total")
	hdl, _ := AnalyteFor("lab.hdl-cholesterol")
	for _, c := range []Cohort{
		cohort(t, "male", 30, 39),
		cohort(t, "female", 6, 80),
		cohort(t, "female", 20, 59, Rule{"DEMO_J", "RIDRETH3", 3, 4}),
		cohort(t, "male", 20, 80, Rule{"BMX_J", "BMXBMI", 22, 30}),
		cohort(t, "female", 18, 80, Rule{"DR1TOT_J", "DR1TPROT", 60, 140}, Rule{"DR1TOT_J", "DR1TFIBE", 10, 40}),
		cohort(t, "male", 18, 80, Rule{"DEMO_J", "RIDRETH3", 3, 3}, Rule{"BMX_J", "BMXBMI", 18, 35}, Rule{"DR1TOT_J", "DR1TKCAL", 1500, 3000}),
	} {
		for _, a := range []Analyte{tc, hdl} {
			people := plain(t, files, a, c)
			var total float64
			for _, p := range people {
				total += p.w
			}
			for _, v := range []float64{0.5, 127, 150.5, 193, 200, 250, 1000, 40, 52} {
				got, err := Compare(files, a, c, v)
				if err != nil {
					t.Fatalf("%s %s %v: %v", a.Variable, c, v, err)
				}
				var below, equal float64
				for _, p := range people {
					if p.x < v {
						below += p.w
					} else if p.x == v {
						equal += p.w
					}
				}
				want := Result{N: len(people), Below: 100 * below / total, Equal: 100 * equal / total, Percentile: 100 * (below + equal/2) / total}
				// The plain sums add in another order: allow the last digit
				// of the rounding to differ only at a rounding boundary.
				near := func(a, b float64) bool { return math.Abs(a-b) <= 0.05+1e-9 }
				if got.N != want.N || !near(got.Below, want.Below) || !near(got.Equal, want.Equal) || !near(got.Percentile, want.Percentile) {
					t.Errorf("%s %s %v: got %+v, want %+v", a.Variable, c, v, got, want)
				}
				// A quantile is a value of the sample that splits the weight.
				for q, x := range map[float64]float64{0.05: got.P5, 0.25: got.P25, 0.5: got.P50, 0.75: got.P75, 0.95: got.P95} {
					var upTo, under float64
					for _, p := range people {
						if p.x <= x {
							upTo += p.w
						}
						if p.x < x {
							under += p.w
						}
					}
					if upTo < q*total*(1-1e-12) || under > q*total*(1+1e-12) {
						t.Errorf("%s %s: quantile %v is %v, which does not split the weight", a.Variable, c, q, x)
					}
				}
			}
		}
	}
}

// The diet rules change the weight, and with it the result.
func TestWeight(t *testing.T) {
	plainCohort := cohort(t, "male", 30, 39)
	diet := cohort(t, "male", 30, 39, Rule{"BMX_J", "BMXBMI", 10, 90}, Rule{"DR1TOT_J", "DR1TKCAL", 0, 99999})
	if d, v := plainCohort.Weight(); d != "DEMO_J" || v != "WTMEC2YR" {
		t.Errorf("%s %s", d, v)
	}
	if d, v := diet.Weight(); d != "DR1TOT_J" || v != "WTDRD1" {
		t.Errorf("%s %s", d, v)
	}
}

func TestRefusals(t *testing.T) {
	files := load(t, "testdata", synthetic...)
	tc, _ := AnalyteFor("lab.cholesterol-total")
	good := cohort(t, "male", 30, 39)
	if _, err := Compare(files, tc, good, 190); err != nil {
		t.Fatal(err)
	}
	// A small cohort gives its size and no number.
	res, err := Compare(files, tc, cohort(t, "male", 30, 30, Rule{"DEMO_J", "RIDRETH3", 6, 6}), 190)
	if err == nil || !strings.Contains(err.Error(), "no result below 30") || res.N >= MinN || res != (Result{N: res.N}) {
		t.Errorf("small cohort: %+v %v", res, err)
	}
	for name, c := range map[string]Cohort{
		"no sex and age":      {},
		"age first":           {[]Rule{good.Rules[1], good.Rules[0]}},
		"unknown data set":    cohort(t, "male", 30, 39, Rule{"SMQ_J", "SMQ020", 1, 1}),
		"a weight as a rule":  cohort(t, "male", 30, 39, Rule{"DEMO_J", "WTMEC2YR", 0, 1}),
		"a design variable":   cohort(t, "male", 30, 39, Rule{"DEMO_J", "SDMVPSU", 1, 1}),
		"the join key":        cohort(t, "male", 30, 39, Rule{"DEMO_J", "SEQN", 1, 2}),
		"a range backwards":   cohort(t, "male", 30, 39, Rule{"BMX_J", "BMXBMI", 30, 20}),
		"not a number":        cohort(t, "male", 30, 39, Rule{"BMX_J", "BMXBMI", math.NaN(), 20}),
		"one variable twice":  cohort(t, "male", 30, 39, Rule{"BMX_J", "BMXBMI", 20, 25}, Rule{"BMX_J", "BMXBMI", 22, 30}),
		"an unknown variable": cohort(t, "male", 30, 39, Rule{"BMX_J", "BMXWAIST", 20, 25}),
	} {
		if _, err := Compare(files, tc, c, 190); err == nil {
			t.Errorf("%s: compared", name)
		}
	}
	for _, bad := range [][3]any{{"other", 30, 39}, {"male", -1, 39}, {"male", 40, 39}, {"male", 30, 81}} {
		if _, err := NewCohort(bad[0].(string), bad[1].(int), bad[2].(int)); err == nil {
			t.Errorf("%v: a cohort", bad)
		}
	}
	for _, v := range []float64{math.NaN(), math.Inf(1), 0, -5} {
		if _, err := Compare(files, tc, good, v); err == nil {
			t.Errorf("%v: compared", v)
		}
	}
	// A file missing, or under another name.
	without := load(t, "testdata", "TCHOL_J", "DEMO_J")
	if _, err := Compare(without, tc, cohort(t, "male", 30, 39, Rule{"BMX_J", "BMXBMI", 20, 25}), 190); err == nil {
		t.Error("compared without the rule's file")
	}
	swapped := map[string]*xpt.File{"TCHOL_J": files["HDL_J"], "DEMO_J": files["DEMO_J"]}
	if _, err := Compare(swapped, tc, good, 190); err == nil {
		t.Error("compared with another file under the analyte's name")
	}
	// The label of the variable must give the unit of the table.
	wrongUnit := tc
	wrongUnit.Variable = "LBDTCSI" // mmol/L
	if _, err := Compare(files, wrongUnit, good, 190); err == nil || !strings.Contains(err.Error(), "labelled") {
		t.Errorf("compared in the wrong unit: %v", err)
	}
	// Lab and demographics files of two releases.
	fewer, err := xpttest.Write("DEMO_J", "09NOV26:00:00:00", []string{"SEQN", "RIAGENDR", "RIDAGEYR", "WTMEC2YR"}, []string{"", "", "", ""}, [][]float64{{1, 1, 30, 1}})
	if err != nil {
		t.Fatal(err)
	}
	other, _ := xpt.Parse(fewer)
	if _, err := Compare(map[string]*xpt.File{"TCHOL_J": files["TCHOL_J"], "DEMO_J": other}, tc, good, 190); err == nil || !strings.Contains(err.Error(), "one release") {
		t.Errorf("compared files of two releases: %v", err)
	}
}

func TestAnalytes(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range Analytes {
		if !InCycle(a.Dataset) || Known[a.Dataset] == "" || a.Unit == "" || a.Label == "" {
			t.Errorf("%+v: no cycle, digest or unit", a)
		}
		if seen[a.Type] || !strings.HasPrefix(a.Type, "lab.") {
			t.Errorf("%s: twice, or not a lab type", a.Type)
		}
		seen[a.Type] = true
		if got, ok := AnalyteFor(a.Type); !ok || got.Variable != a.Variable {
			t.Errorf("%s: %+v", a.Type, got)
		}
	}
	// The checker in Python holds the same digests.
	py, err := os.ReadFile("../../tools/compare/reproduce.py")
	if err != nil {
		t.Fatal(err)
	}
	for name, digest := range Known {
		if !strings.Contains(string(py), `"`+name+`": "`+digest+`"`) {
			t.Errorf("tools/compare/reproduce.py does not have the digest of %s", name)
		}
	}
	if _, ok := AnalyteFor("lab.glucose"); ok {
		t.Error("glucose has no row: fasting is not known")
	}
	for d := range CohortFiles {
		if Known[d] == "" {
			t.Errorf("%s: no digest", d)
		}
	}
}

// With BEAN_NHANES_DIR set to a folder of CDC's files, each file has the
// SHA-256 on record, each analyte's variable is labelled in its unit, and
// these results hold. The numbers are statistics of public data, checked
// with pandas (tools/compare/reproduce.py) on 2026-11-09.
func TestRealFiles(t *testing.T) {
	dir := os.Getenv("BEAN_NHANES_DIR")
	if dir == "" {
		t.Skip("BEAN_NHANES_DIR is not set")
	}
	var names []string
	for name := range Known {
		names = append(names, name)
	}
	files := load(t, dir, names...)
	for _, a := range Analytes {
		if _, err := Compare(files, a, cohort(t, "female", 20, 80), 1); err != nil {
			t.Errorf("%s: %v", a.Variable, err)
		}
	}
	tc, _ := AnalyteFor("lab.cholesterol-total")
	for _, c := range []struct {
		cohort Cohort
		want   Result
	}{
		{cohort(t, "male", 30, 39), realMale30},
		{cohort(t, "male", 30, 39, Rule{"DEMO_J", "RIDRETH3", 3, 3}, Rule{"BMX_J", "BMXBMI", 20, 27}), realMale30WhiteBMI},
		{cohort(t, "male", 30, 39, Rule{"DR1TOT_J", "DR1TPROT", 90, 160}, Rule{"DR1TOT_J", "DR1TFIBE", 15, 60}), realMale30Diet},
	} {
		got, err := Compare(files, tc, c.cohort, 187)
		if err != nil || got != c.want {
			t.Errorf("%s: got %+v %v, want %+v", c.cohort, got, err, c.want)
		}
	}
}

// Total cholesterol of 187 mg/dL in three cohorts of NHANES 2017-2018.
var (
	realMale30         = Result{N: 339, Percentile: 46, Below: 44.2, Equal: 3.5, P5: 135, P25: 164, P50: 188, P75: 213, P95: 256}
	realMale30WhiteBMI = Result{N: 39, Percentile: 55, Below: 50.1, Equal: 9.8, P5: 130, P25: 161, P50: 182, P75: 204, P95: 222}
	realMale30Diet     = Result{N: 81, Percentile: 37.8, Below: 37.8, Equal: 0, P5: 145, P25: 171, P50: 191, P75: 217, P95: 251}
)
