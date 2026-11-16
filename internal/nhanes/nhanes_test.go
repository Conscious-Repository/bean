package nhanes

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
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

// Fixed results on the synthetic files, one for each kind of rule and each
// weight. tools/compare/run.sh computes the same cohorts again in Python.
func TestCompareSynthetic(t *testing.T) {
	files := load(t, "testdata", synthetic...)
	tc, _, _ := AnalyteFor("lab.cholesterol-total", "mg/dL")
	hdl, _, _ := AnalyteFor("lab.hdl-cholesterol", "mg/dL")
	for _, c := range []struct {
		analyte Analyte
		cohort  Cohort
		value   float64
		want    Result
	}{
		{tc, cohort(t, "male", 30, 39), 187, syntheticWant[0]},
		{hdl, cohort(t, "female", 20, 59, Rule{"DEMO_J", "RIDRETH3", 3, 4}), 52, syntheticWant[1]},
		{tc, cohort(t, "male", 20, 80, Rule{"BMX_J", "BMXBMI", 22, 30}), 200.5, syntheticWant[2]},
		{tc, cohort(t, "female", 18, 80, Rule{"DR1TOT_J", "DR1TPROT", 60, 140}, Rule{"DR1TOT_J", "DR1TFIBE", 10, 40}), 193, syntheticWant[3]},
	} {
		got, err := Compare(files, c.analyte, c.cohort, c.value)
		if err != nil || got != c.want {
			t.Errorf("%s %s %v: got %#v %v, want %#v", c.analyte.Variable, c.cohort, c.value, got, err, c.want)
		}
	}
	// A rule on the dietary file changes the weight.
	if d, v := cohort(t, "male", 30, 39).Weight(); d != "DEMO_J" || v != "WTMEC2YR" {
		t.Errorf("%s %s", d, v)
	}
	if d, v := cohort(t, "male", 30, 39, Rule{"BMX_J", "BMXBMI", 10, 90}, Rule{"DR1TOT_J", "DR1TKCAL", 0, 99999}).Weight(); d != "DR1TOT_J" || v != "WTDRD1" {
		t.Errorf("%s %s", d, v)
	}
	// A cohort's text never has an exponent: it is part of a signed claim.
	if got := cohort(t, "female", 18, 80, Rule{"DR1TOT_J", "DR1TKCAL", 0, 1000000}, Rule{"BMX_J", "BMXBMI", 18.5, 40}).String(); got != "female, age 18 and over, DR1TOT_J.DR1TKCAL=0-1000000, BMX_J.BMXBMI=18.5-40" {
		t.Error(got)
	}
}

func TestRefusals(t *testing.T) {
	files := load(t, "testdata", synthetic...)
	tc, _, _ := AnalyteFor("lab.cholesterol-total", "mg/dL")
	good := cohort(t, "male", 30, 39)
	if _, err := Compare(files, tc, good, 190); err != nil {
		t.Fatal(err)
	}
	// A small cohort gives its size and no number.
	res, err := Compare(files, tc, cohort(t, "male", 30, 30, Rule{"DEMO_J", "RIDRETH3", 6, 6}), 190)
	if !errors.Is(err, ErrSmall) || res.N >= MinN || res != (Result{N: res.N}) {
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
		if seen[a.Type+" "+a.Unit] || !strings.HasPrefix(a.Type, "lab.") {
			t.Errorf("%s in %s: twice, or not a lab type", a.Type, a.Unit)
		}
		seen[a.Type+" "+a.Unit] = true
		if got, ok, _ := AnalyteFor(a.Type, a.Unit); !ok || got != a {
			t.Errorf("%s: %+v", a.Type, got)
		}
	}
	// One type in two units, as two labs print it; another unit is not a row.
	if a, ok, units := AnalyteFor("lab.hemoglobin-a1c", "% of total Hgb"); !ok || a.Variable != "LBXGH" || len(units) != 2 {
		t.Errorf("%+v %v %v", a, ok, units)
	}
	if _, ok, units := AnalyteFor("lab.hemoglobin-a1c", "mmol/mol"); ok || len(units) != 2 {
		t.Errorf("another unit: %v %v", ok, units)
	}
	if _, ok, units := AnalyteFor("lab.tsh", "mIU/L"); ok || len(units) != 0 {
		t.Errorf("no row: %v %v", ok, units)
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
	if _, _, units := AnalyteFor("lab.glucose", "mg/dL"); len(units) != 0 {
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
	tc, _, _ := AnalyteFor("lab.cholesterol-total", "mg/dL")
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

// With CDC's files: no test that bean compares has a value that NHANES
// filled in. NHANES puts a number in place of a value below a detection
// limit, and marks it in a variable whose label ends in "Comment Code".
// The method has no rule for such a value (DECISIONS 240), so a row for
// such a test is refused here.
func TestRealNoCommentCode(t *testing.T) {
	dir := os.Getenv("BEAN_NHANES_DIR")
	if dir == "" {
		t.Skip("BEAN_NHANES_DIR is not set")
	}
	var names []string
	for _, a := range Analytes {
		if !slices.Contains(names, a.Dataset) {
			names = append(names, a.Dataset)
		}
	}
	files := load(t, dir, names...)
	for _, a := range Analytes {
		if _, ok := files[a.Dataset].Variable(a.Variable); !ok {
			t.Errorf("%s has no variable %s", a.Dataset, a.Variable)
		}
	}
	if marked := commentCoded(files, Analytes); len(marked) > 0 {
		t.Errorf("these tests have a comment code, and the method has no rule for one: %v", marked)
	}
	// The check finds the tests that were left out for this reason.
	left := []Analyte{{Dataset: "BIOPRO_J", Variable: "LBXSATSI"}, {Dataset: "BIOPRO_J", Variable: "LBXSGTSI"}, {Dataset: "BIOPRO_J", Variable: "LBXSTB"}, {Dataset: "BIOPRO_J", Variable: "LBXSCR"}}
	if marked := commentCoded(files, left); !slices.Equal(marked, []string{"LBXSATSI", "LBXSGTSI", "LBXSTB"}) {
		t.Errorf("of ALT, GGT, total bilirubin and creatinine, these have a comment code: %v", marked)
	}
	// CDC's page of FERTIN_J names a comment code, and the file has none.
	// So hold the data itself: no ferritin value is at or below the limit
	// of detection that the page states, 0.5 ng/mL.
	ferritin, err := files["FERTIN_J"].Column("LBXFER")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range ferritin {
		if v <= ferritinLimit {
			t.Fatalf("FERTIN_J has a ferritin value of %v, at or below the limit of detection", v)
		}
	}
}

// ferritinLimit is the lower limit of detection that CDC's page of
// FERTIN_J states, in ng/mL.
const ferritinLimit = 0.5

// commentCoded returns the variables of rows whose file has a comment code
// for them: a variable labelled "<name> Comment Code", where the test's own
// label holds that name. Letter case does not count.
func commentCoded(files map[string]*xpt.File, rows []Analyte) []string {
	var out []string
	for _, a := range rows {
		f := files[a.Dataset]
		v, ok := f.Variable(a.Variable)
		if !ok {
			continue
		}
		for _, other := range f.Variables {
			name, isCode := strings.CutSuffix(strings.ToLower(other.Label), " comment code")
			if isCode && name != "" && strings.Contains(strings.ToLower(v.Label), name) && !slices.Contains(out, a.Variable) {
				out = append(out, a.Variable)
			}
		}
	}
	return out
}

// analyzerDiff is the mean difference between two analyzers on the same
// blood samples, in percent. It is CDC's own table: the page of BIOPRO_J,
// "regression equations to compare 2017-2018 and 2015-2016 data", column
// "% DIFF", read on 2026-11-14. Here are the rows of the tests that a
// report at hand prints and that no other rule leaves out.
var analyzerDiff = map[string]float64{
	"LBXSTP": 0.36, "LBXSUA": 2.37, "LBXSCK": 5.91, "LBXSBU": -3.36, "LBXSKSI": -1.398, "LBXSCR": 3.354,
	"LBXSIR": 10.01, "LBXSAPSI": 9.74, "LBXSCLSI": -1.44, "LBXSNASI": 0.80, "LBXSCA": -2.12, "LBXSASSI": -18.03,
}

// maxShift is the rule of DECISIONS 309: a test of BIOPRO_J is compared
// only when the difference between two analyzers moves a middle value by
// less than a fifth of the middle half of a cohort. Beyond that, a
// percentile says more about the analyzer than about the person.
const maxShift = 0.2

// analyzerShift is that move for one variable: CDC's difference times the
// median, over the width of the middle half, as the median of the cohorts
// of a pack's menu.
func analyzerShift(t *testing.T, files map[string]*xpt.File, variable, unit string) float64 {
	t.Helper()
	var shifts []float64
	for _, sex := range []string{"female", "male"} {
		for _, ages := range PackAges {
			r, err := Compare(files, Analyte{Dataset: "BIOPRO_J", Variable: variable, Label: unit}, cohort(t, sex, ages[0], ages[1]), 1)
			if err != nil {
				t.Fatal(err)
			}
			shifts = append(shifts, math.Abs(analyzerDiff[variable])/100*r.P50/(r.P75-r.P25))
		}
	}
	slices.Sort(shifts)
	return shifts[len(shifts)/2]
}

// With CDC's files: every test of BIOPRO_J that bean compares is under the
// rule, and the tests that were left out for it are over.
func TestRealAnalyzerDifference(t *testing.T) {
	dir := os.Getenv("BEAN_NHANES_DIR")
	if dir == "" {
		t.Skip("BEAN_NHANES_DIR is not set")
	}
	files := load(t, dir, "DEMO_J", "BIOPRO_J")
	seen := map[string]bool{}
	for _, a := range Analytes {
		if a.Dataset != "BIOPRO_J" || seen[a.Variable] {
			continue
		}
		seen[a.Variable] = true
		if _, ok := analyzerDiff[a.Variable]; !ok {
			t.Errorf("%s: CDC's table of the two analyzers has no row for it here", a.Variable)
			continue
		}
		if s := analyzerShift(t, files, a.Variable, a.Label); s >= maxShift {
			t.Errorf("%s: two analyzers move a middle value by %.2f of the middle half, and the rule is under %.2f", a.Variable, s, maxShift)
		}
	}
	for variable, unit := range map[string]string{"LBXSASSI": "U/L", "LBXSAPSI": "IU/L", "LBXSCA": "mg/dL", "LBXSCLSI": "mmol/L", "LBXSNASI": "mmol/L", "LBXSIR": "ug/dL"} {
		if s := analyzerShift(t, files, variable, unit); s < maxShift {
			t.Errorf("%s was left out for the difference between analyzers, and it is %.2f of the middle half", variable, s)
		}
	}
}

// Total cholesterol of 187 mg/dL in three cohorts of NHANES 2017-2018.
var (
	realMale30         = Result{N: 339, Percentile: 46, Below: 44.2, Equal: 3.5, P25: 164, P50: 188, P75: 213}
	realMale30WhiteBMI = Result{N: 39, Percentile: 55, Below: 50.1, Equal: 9.8, P25: 161, P50: 182, P75: 204}
	realMale30Diet     = Result{N: 81, Percentile: 37.8, Below: 37.8, Equal: 0, P25: 171, P50: 191, P75: 217}
)

// syntheticWant are the results of TestCompareSynthetic.
var syntheticWant = []Result{
	{N: 86, Percentile: 44.3, Below: 43.7, Equal: 1.2, P25: 176, P50: 190, P75: 203},
	{N: 195, Percentile: 53.7, Below: 49.9, Equal: 7.6, P25: 47, P50: 52, P75: 57},
	{N: 277, Percentile: 65.4, Below: 65.4, Equal: 0, P25: 176, P50: 191, P75: 205},
	{N: 327, Percentile: 52.1, Below: 51, Equal: 2.1, P25: 178, P50: 192, P75: 205},
}

// Each digest of Known is on record, and nothing else is: not the empty
// text, not a digest in upper case, not another digest.
func TestOnRecord(t *testing.T) {
	for name, digest := range Known {
		if !OnRecord(digest) {
			t.Errorf("the digest of %s is not on record", name)
		}
		if OnRecord(strings.ToUpper(digest)) && strings.ToUpper(digest) != digest {
			t.Errorf("the digest of %s in upper case is on record", name)
		}
	}
	for _, other := range []string{"", "0", strings.Repeat("0", 64), "TCHOL_J"} {
		if OnRecord(other) {
			t.Errorf("%q is on record", other)
		}
	}
}
