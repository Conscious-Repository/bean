// Package nhanes knows the public NHANES files that bean compares a reading
// with, and the one method it uses: a survey-weighted percentile of a
// reading in a cohort that the person states as rules over NHANES
// variables (sex, age, and any of ethnicity, body measures and a day's
// food). docs/COMPARE.md is the method in words; tools/compare/reproduce.py
// is a second implementation of it.
package nhanes

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Conscious-Repository/bean/internal/xpt"
)

// System is source.system of a kept NHANES file.
const System = "nhanes"

// FileType is the type of the record that keeps one NHANES file.
const FileType = "reference.file"

// Cycle is one survey cycle.
type Cycle struct {
	Suffix string // the end of each data set name
	Years  string
	URL    string // the folder CDC serves the files from
	Demo   string // the demographics data set
	Weight string // the examination weight
}

// Cycles are the cycles bean reads. A file of another cycle is refused: its
// weights and variables are not the same.
var Cycles = []Cycle{
	{Suffix: "_J", Years: "2017-2018", URL: "https://wwwn.cdc.gov/Nchs/Data/Nhanes/Public/2017/DataFiles/", Demo: "DEMO_J", Weight: "WTMEC2YR"},
}

// CycleOf returns the cycle of a data set name such as TCHOL_J.
func CycleOf(dataset string) (Cycle, bool) {
	for _, c := range Cycles {
		if strings.HasSuffix(dataset, c.Suffix) && len(dataset) > len(c.Suffix) {
			return c, true
		}
	}
	return Cycle{}, false
}

// Known holds the SHA-256 of each file as wwwn.cdc.gov served it on
// 2026-11-09. A file with another digest still imports and compares, and
// the comparison says that bean does not know it: CDC may have published it
// again, or it is not CDC's file.
var Known = map[string]string{
	"DEMO_J":   "c0b46e0345ea19404928656277c8b0d10b0cca348a9b2fe4fc3c67e8b7ee73ec",
	"TCHOL_J":  "0291a6a4f6d82dac8392c8c992b519946824dfeda01647935520cf506ed9f4ab",
	"HDL_J":    "9f4eb41b89f0c9f3d6262f873e671eeb2f2eb40581a8426b2a3c8fd4fc19439e",
	"GHB_J":    "35f07094573a0061a03ed609a5a363b34eb1b1c7065d1623b43d72e132a8a654",
	"CBC_J":    "00f964098dc91272e415344554cbf1b627ecaeb2eda201518527446a7d81e742",
	"BMX_J":    "8d675e42d8826ac98714b2c3dd4c5138a5e353fb4424f7eff5e6db4a01ce838a",
	"DR1TOT_J": "390ae5205acfbcb87e0ca3d9aabd1c801b60657d3f2802ac08d5b3ed93f0af6d",
}

// CohortFile is a data set whose variables a cohort rule may name.
type CohortFile struct {
	// Weight is the sample weight that an analysis with this file must
	// use, when it is not the examination weight. CDC's notes on the
	// dietary files say to use the day one weight whenever day one
	// nutrient data are used, alone or with examination data.
	Weight string
	// Reason qualifies every comparison whose cohort uses the file.
	Reason string
}

// CohortFiles are the data sets a rule may name. Another data set is
// refused until it has a row here with the weight CDC gives for it: a
// wrong weight gives a wrong number that looks right.
var CohortFiles = map[string]CohortFile{
	"DEMO_J": {},
	"BMX_J":  {},
	"DR1TOT_J": {Weight: "WTDRD1",
		Reason: "the cohort is chosen by what people said they ate on one day, which is not what a person usually eats"},
}

// Analyte maps the record types of one lab test to its NHANES variable.
// Every row is a claim that the two measure the same quantity in the same
// unit. Add a row only from a real report and the file's own label.
type Analyte struct {
	// Types are the bean record types, as an importer writes them.
	Types []string
	// Units are the units of a reading that equal the variable's unit.
	Units []string
	// Dataset and Variable name the NHANES column; Unit is the unit in
	// its label, which Compare checks.
	Dataset, Variable, Unit string
}

// Analytes are the tests bean compares. All of them were measured in the
// whole examined sample, so they use the examination weight.
var Analytes = []Analyte{
	{[]string{"lab.cholesterol-total"}, []string{"mg/dL"}, "TCHOL_J", "LBXTC", "mg/dL"},
	{[]string{"lab.hdl-cholesterol"}, []string{"mg/dL"}, "HDL_J", "LBDHDD", "mg/dL"},
	{[]string{"lab.hemoglobin-a1c"}, []string{"%"}, "GHB_J", "LBXGH", "%"},
	{[]string{"lab.wbc"}, []string{"x10E3/uL"}, "CBC_J", "LBXWBCSI", "1000 cells/uL"},
	{[]string{"lab.rbc"}, []string{"x10E6/uL"}, "CBC_J", "LBXRBCSI", "million cells/uL"},
	{[]string{"lab.hemoglobin"}, []string{"g/dL"}, "CBC_J", "LBXHGB", "g/dL"},
	{[]string{"lab.hematocrit"}, []string{"%"}, "CBC_J", "LBXHCT", "%"},
	{[]string{"lab.mcv"}, []string{"fL"}, "CBC_J", "LBXMCVSI", "fL"},
	{[]string{"lab.mch"}, []string{"pg"}, "CBC_J", "LBXMCHSI", "pg"},
	{[]string{"lab.mchc"}, []string{"g/dL"}, "CBC_J", "LBXMC", "g/dL"},
	{[]string{"lab.rdw"}, []string{"%"}, "CBC_J", "LBXRDW", "%"},
	{[]string{"lab.platelets"}, []string{"x10E3/uL"}, "CBC_J", "LBXPLTSI", "1000 cells/uL"},
}

// AnalyteFor returns the analyte that a record type names.
func AnalyteFor(typ string) (Analyte, bool) {
	for _, a := range Analytes {
		for _, t := range a.Types {
			if t == typ {
				return a, true
			}
		}
	}
	return Analyte{}, false
}

// HasUnit reports whether a reading in this unit can be compared as it is.
func (a Analyte) HasUnit(unit string) bool {
	for _, u := range a.Units {
		if u == unit {
			return true
		}
	}
	return false
}

// Method names the computation. A change to what Compare computes is a new
// version.
const (
	MethodID      = "nhanes-weighted-percentile"
	MethodVersion = "1"
)

// MinN is the smallest cohort Compare gives a result for, and SmallN the
// size below which a result is qualified as coming from a small cohort.
const (
	MinN   = 30
	SmallN = 100
)

// MaxAge is the oldest age NHANES records: 80 stands for 80 and over.
const MaxAge = 80

// Rule keeps the people whose value of one variable lies from Min to Max,
// both included. A person with no row in the data set, or no value, is
// left out.
type Rule struct {
	Dataset, Variable string
	Min, Max          float64
}

func (r Rule) String() string {
	if r.Min == r.Max {
		return fmt.Sprintf("%s.%s=%s", r.Dataset, r.Variable, num(r.Min))
	}
	return fmt.Sprintf("%s.%s=%s-%s", r.Dataset, r.Variable, num(r.Min), num(r.Max))
}

func num(x float64) string { return strconv.FormatFloat(x, 'g', -1, 64) }

// Cohort selects the people a reading is compared with: those who pass
// every rule. Sex and age come first; bean gives no result without them.
type Cohort struct{ Rules []Rule }

// NewCohort makes the cohort of one sex and an age range in years, both
// ends included, in the demographics file of the cycle.
func NewCohort(cycle Cycle, sex string, ageMin, ageMax int) (Cohort, error) {
	code := map[string]float64{"male": 1, "female": 2}[sex]
	if code == 0 {
		return Cohort{}, errors.New("sex is male or female, as NHANES records it")
	}
	if ageMin < 0 || ageMin > ageMax || ageMax > MaxAge {
		return Cohort{}, fmt.Errorf("age is a range of years from 0 to %d, for example 30-39; %d means %d and over", MaxAge, MaxAge, MaxAge)
	}
	return Cohort{[]Rule{
		{cycle.Demo, "RIAGENDR", code, code},
		{cycle.Demo, "RIDAGEYR", float64(ageMin), float64(ageMax)},
	}}, nil
}

// Check reports whether Compare can select the cohort.
func (c Cohort) Check() error {
	if len(c.Rules) < 2 || c.Rules[0].Variable != "RIAGENDR" || c.Rules[1].Variable != "RIDAGEYR" {
		return errors.New("a cohort starts with a sex and an age range")
	}
	seen := map[string]bool{}
	for _, r := range c.Rules {
		if _, ok := CohortFiles[r.Dataset]; !ok {
			return fmt.Errorf("bean does not choose a cohort by %s: it has no sample weight on record for that data set", r.Dataset)
		}
		if math.IsNaN(r.Min) || math.IsNaN(r.Max) || math.IsInf(r.Min, 0) || math.IsInf(r.Max, 0) || r.Min > r.Max {
			return fmt.Errorf("%s.%s: the range runs from a number to a number that is not smaller", r.Dataset, r.Variable)
		}
		if r.Variable == "SEQN" || strings.HasPrefix(r.Variable, "WT") || strings.HasPrefix(r.Variable, "SDMV") {
			return fmt.Errorf("%s is a number of the survey's design, not something about a person", r.Variable)
		}
		if seen[r.Dataset+"."+r.Variable] {
			return fmt.Errorf("two rules name %s.%s", r.Dataset, r.Variable)
		}
		seen[r.Dataset+"."+r.Variable] = true
	}
	return nil
}

// Datasets returns the data sets the rules name, each once, in rule order.
func (c Cohort) Datasets() []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range c.Rules {
		if !seen[r.Dataset] {
			seen[r.Dataset] = true
			out = append(out, r.Dataset)
		}
	}
	return out
}

// Weight returns the data set and the variable of the sample weight: the
// weight of the first rule's data set that needs its own, else the
// examination weight in the demographics file.
func (c Cohort) Weight(cycle Cycle) (dataset, variable string) {
	for _, d := range c.Datasets() {
		if w := CohortFiles[d].Weight; w != "" {
			return d, w
		}
	}
	return cycle.Demo, cycle.Weight
}

// TopAge reports whether the age range ends at MaxAge, which stands for
// every age from there on.
func (c Cohort) TopAge() bool { return len(c.Rules) > 1 && c.Rules[1].Max == MaxAge }

func (c Cohort) String() string {
	sex := "male"
	if c.Rules[0].Min == 2 {
		sex = "female"
	}
	out := fmt.Sprintf("%s, age %s-%s", sex, num(c.Rules[1].Min), num(c.Rules[1].Max))
	if c.TopAge() {
		out = fmt.Sprintf("%s, age %s and over", sex, num(c.Rules[1].Min))
	}
	for _, r := range c.Rules[2:] {
		out += ", " + r.String()
	}
	return out
}

// Result is where a value lies in a cohort. The percentages are rounded to
// one decimal; the quantiles are values of the sample.
type Result struct {
	N int // people in the cohort with a value and a weight
	// Below and Equal are the weighted shares of the cohort with a lower
	// value and with the same value. Percentile is Below plus half of Equal.
	Percentile, Below, Equal float64
	P5, P25, P50, P75, P95   float64
}

// ErrSmall is returned for a cohort of fewer than MinN people.
type ErrSmall struct{ N int }

func (e ErrSmall) Error() string {
	return fmt.Sprintf("the cohort has %d people with a value; bean gives no result below %d. Widen a range or drop a rule", e.N, MinN)
}

// column is one variable of one file, by SEQN.
type column struct {
	row    map[float64]int
	values []float64
}

func (c column) of(seqn float64) float64 {
	if i, ok := c.row[seqn]; ok {
		return c.values[i]
	}
	return math.NaN()
}

// Compare computes where v lies among the cohort's values of the analyte.
// files holds the analyte's file, the demographics file of its cycle, and
// the file of every rule, by data set name. The steps, in this order, are
// the method:
//
//  1. Take the rows of the analyte's file in file order. Skip a row with
//     no value.
//  2. The demographics file must have a row with the same SEQN; none is an
//     error.
//  3. For each rule, find the row of its data set with the same SEQN. Skip
//     the row if there is none, if it has no value of the variable, or if
//     the value is below Min or above Max.
//  4. The weight is that of Cohort.Weight, from the row with the same
//     SEQN. Skip the row if the weight is missing or not above zero.
//  5. W is the sum of the weights of the rows left, B of those with a
//     value below v, E of those with a value equal to v, each added in
//     file order.
//  6. below = 100 B / W, equal = 100 E / W, percentile = 100 (B + E/2) / W.
//  7. Quantile q is the smallest value x such that the weights of the rows
//     with a value at most x reach q W. The weights of one value are added
//     in file order, then the values in rising order.
//  8. Each percentage is rounded to one decimal: floor(10 y + 0.5) / 10.
//
// Fewer than MinN rows left is ErrSmall, and no result.
func Compare(files map[string]*xpt.File, a Analyte, c Cohort, v float64) (Result, error) {
	var res Result
	if err := c.Check(); err != nil {
		return res, err
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return res, errors.New("the reading is not a number")
	}
	cycle, ok := CycleOf(a.Dataset)
	if !ok {
		return res, fmt.Errorf("%s is not a file of a cycle bean reads", a.Dataset)
	}
	// Every file is the data set its name says, of the analyte's cycle.
	file := func(dataset string) (*xpt.File, error) {
		f := files[dataset]
		if f == nil || f.Dataset != dataset {
			return nil, fmt.Errorf("the file of %s is missing", dataset)
		}
		if fc, ok := CycleOf(dataset); !ok || fc != cycle {
			return nil, fmt.Errorf("%s is not a file of NHANES %s", dataset, cycle.Years)
		}
		return f, nil
	}
	lab, err := file(a.Dataset)
	if err != nil {
		return res, err
	}
	if lv, _ := lab.Variable(a.Variable); !strings.HasSuffix(lv.Label, "("+a.Unit+")") {
		return res, fmt.Errorf("%s in %s is labelled %q, not in %s", a.Variable, lab.Dataset, lv.Label, a.Unit)
	}
	// Each file's rows by SEQN, read once.
	rows := map[string]map[float64]int{}
	col := func(dataset, variable string) (column, error) {
		f, err := file(dataset)
		if err != nil {
			return column{}, err
		}
		if rows[dataset] == nil {
			seqn, err := f.Column("SEQN")
			if err != nil {
				return column{}, err
			}
			index := make(map[float64]int, len(seqn))
			for i, s := range seqn {
				if _, twice := index[s]; twice || math.IsNaN(s) {
					return column{}, fmt.Errorf("%s has a missing or repeated SEQN", dataset)
				}
				index[s] = i
			}
			rows[dataset] = index
		}
		values, err := f.Column(variable)
		return column{rows[dataset], values}, err
	}
	seqn, err := lab.Column("SEQN")
	if err != nil {
		return res, err
	}
	x, err := col(a.Dataset, a.Variable)
	if err != nil {
		return res, err
	}
	people, err := col(cycle.Demo, "SEQN")
	if err != nil {
		return res, err
	}
	rules := make([]column, len(c.Rules))
	for i, r := range c.Rules {
		if rules[i], err = col(r.Dataset, r.Variable); err != nil {
			return res, err
		}
	}
	wd, wv := c.Weight(cycle)
	weight, err := col(wd, wv)
	if err != nil {
		return res, err
	}
	var total, below, equal float64
	byValue := map[float64]float64{}
rowLoop:
	for i, s := range seqn {
		if math.IsNaN(x.values[i]) {
			continue
		}
		if _, ok := people.row[s]; !ok {
			return res, fmt.Errorf("%s has a person that %s does not: they are not files of one release", lab.Dataset, cycle.Demo)
		}
		for k, r := range c.Rules {
			if y := rules[k].of(s); !(y >= r.Min && y <= r.Max) {
				continue rowLoop
			}
		}
		w := weight.of(s)
		if !(w > 0) || math.IsInf(w, 0) {
			continue
		}
		res.N++
		total += w
		if x.values[i] < v {
			below += w
		} else if x.values[i] == v {
			equal += w
		}
		byValue[x.values[i]] += w
	}
	if res.N < MinN {
		return Result{N: res.N}, ErrSmall{res.N}
	}
	res.Below = round1(100 * below / total)
	res.Equal = round1(100 * equal / total)
	res.Percentile = round1(100 * (below + equal/2) / total)
	values := make([]float64, 0, len(byValue))
	for value := range byValue {
		values = append(values, value)
	}
	sort.Float64s(values)
	quantile := func(q float64) float64 {
		var sum float64
		for _, value := range values {
			sum += byValue[value]
			if sum >= q*total {
				return value
			}
		}
		return values[len(values)-1]
	}
	res.P5, res.P25, res.P50, res.P75, res.P95 = quantile(0.05), quantile(0.25), quantile(0.5), quantile(0.75), quantile(0.95)
	return res, nil
}

func round1(y float64) float64 { return math.Floor(10*y+0.5) / 10 }
