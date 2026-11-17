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

	"github.com/Conscious-Repository/bean/internal/pack"
	"github.com/Conscious-Repository/bean/internal/xpt"
)

// System is source.system of a kept NHANES file.
const System = "nhanes"

// FileType is the type of the record that keeps one NHANES file.
const FileType = "reference.file"

// The one survey cycle bean reads: 2017-2018. A file of another cycle is
// refused: its weights and variables are not the same. A second cycle is a
// second set of these, and then a type for them.
const (
	Years  = "2017-2018"
	Suffix = "_J" // the end of each data set name
	// URL is the folder CDC serves the files from.
	URL    = "https://wwwn.cdc.gov/Nchs/Data/Nhanes/Public/2017/DataFiles/"
	Demo   = "DEMO_J"   // the demographics data set
	Weight = "WTMEC2YR" // the examination weight, in Demo
)

// InCycle reports whether a data set name such as TCHOL_J is of the cycle.
func InCycle(dataset string) bool {
	return strings.HasSuffix(dataset, Suffix) && len(dataset) > len(Suffix)
}

// Known holds the SHA-256 of each file as one download from wwwn.cdc.gov
// gave it on 2026-11-09: the files on record. BIOPRO_J and FERTIN_J gave
// the same digest again on 2026-11-14. A file with another digest
// still imports, but bean compare uses it only when told to: it may be cut
// short, changed, or a later release. tools/compare/reproduce.py holds the
// same table, and a test keeps the two equal.
var Known = map[string]string{
	"DEMO_J":   "c0b46e0345ea19404928656277c8b0d10b0cca348a9b2fe4fc3c67e8b7ee73ec",
	"TCHOL_J":  "0291a6a4f6d82dac8392c8c992b519946824dfeda01647935520cf506ed9f4ab",
	"HDL_J":    "9f4eb41b89f0c9f3d6262f873e671eeb2f2eb40581a8426b2a3c8fd4fc19439e",
	"GHB_J":    "35f07094573a0061a03ed609a5a363b34eb1b1c7065d1623b43d72e132a8a654",
	"CBC_J":    "00f964098dc91272e415344554cbf1b627ecaeb2eda201518527446a7d81e742",
	"BMX_J":    "8d675e42d8826ac98714b2c3dd4c5138a5e353fb4424f7eff5e6db4a01ce838a",
	"DR1TOT_J": "390ae5205acfbcb87e0ca3d9aabd1c801b60657d3f2802ac08d5b3ed93f0af6d",
	"BIOPRO_J": "5bcd5722c1892883b96a9d7fed0befadabacae313f800fadc0133cd5dd00c4c6",
	"FERTIN_J": "6bc5775797aafbd627316b6208418275fb8951cd571276e1a64436769afa16d5",
}

// OnRecord reports a SHA-256 that is the one on record for a file of the
// survey. Bytes with such a digest are a file that its publisher serves to
// anyone, whatever a record says about them.
func OnRecord(sha256 string) bool {
	for _, digest := range Known {
		if digest == sha256 {
			return true
		}
	}
	return false
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

// Analyte maps the record type of one lab test to its NHANES variable.
// Every row is a claim that the two measure the same quantity in the same
// unit. Add a row only from a real report and the file's own label. A
// second lab's name or unit for a test is a second row.
type Analyte struct {
	// Type is the bean record type, as an importer writes it, and Unit the
	// unit of a reading that equals the variable's unit.
	Type, Unit string
	// Dataset and Variable name the NHANES column; Label is the unit in
	// the variable's label, which Compare checks.
	Dataset, Variable, Label string
}

// Analytes are the tests bean compares, under the names and units that the
// Labcorp and the Quest report print. All of them were measured in the
// whole examined sample, so they use the examination weight. Mean platelet
// volume is left out on purpose: it differs by a fifth and more between
// makes of analyzer, so a percentile of it would mislead.
var Analytes = []Analyte{
	{"lab.cholesterol-total", "mg/dL", "TCHOL_J", "LBXTC", "mg/dL"},
	{"lab.hdl-cholesterol", "mg/dL", "HDL_J", "LBDHDD", "mg/dL"},
	{"lab.hemoglobin-a1c", "%", "GHB_J", "LBXGH", "%"},
	{"lab.hemoglobin-a1c", "% of total Hgb", "GHB_J", "LBXGH", "%"},
	{"lab.wbc", "x10E3/uL", "CBC_J", "LBXWBCSI", "1000 cells/uL"},
	{"lab.white-blood-cell-count", "Thousand/uL", "CBC_J", "LBXWBCSI", "1000 cells/uL"},
	{"lab.rbc", "x10E6/uL", "CBC_J", "LBXRBCSI", "million cells/uL"},
	{"lab.red-blood-cell-count", "Million/uL", "CBC_J", "LBXRBCSI", "million cells/uL"},
	{"lab.hemoglobin", "g/dL", "CBC_J", "LBXHGB", "g/dL"},
	{"lab.hematocrit", "%", "CBC_J", "LBXHCT", "%"},
	{"lab.mcv", "fL", "CBC_J", "LBXMCVSI", "fL"},
	{"lab.mch", "pg", "CBC_J", "LBXMCHSI", "pg"},
	{"lab.mchc", "g/dL", "CBC_J", "LBXMC", "g/dL"},
	{"lab.rdw", "%", "CBC_J", "LBXRDW", "%"},
	{"lab.platelets", "x10E3/uL", "CBC_J", "LBXPLTSI", "1000 cells/uL"},
	{"lab.platelet-count", "Thousand/uL", "CBC_J", "LBXPLTSI", "1000 cells/uL"},
	// Six tests of the standard biochemistry profile, and ferritin. A
	// second lab's name for a test is a second row. DECISIONS 309 says why
	// each other test of the profile is left out: CDC names another file
	// for it, its file marks values below a detection limit, or CDC's own
	// comparison of two analyzers moves a middle value by a fifth or more
	// of the middle half of a cohort. Tests with CDC's files hold the last
	// two rules.
	{"lab.bun", "mg/dL", "BIOPRO_J", "LBXSBU", "mg/dL"},
	{"lab.urea-nitrogen-bun", "mg/dL", "BIOPRO_J", "LBXSBU", "mg/dL"},
	{"lab.creatinine", "mg/dL", "BIOPRO_J", "LBXSCR", "mg/dL"},
	{"lab.potassium", "mmol/L", "BIOPRO_J", "LBXSKSI", "mmol/L"},
	{"lab.protein-total", "g/dL", "BIOPRO_J", "LBXSTP", "g/dL"},
	{"lab.uric-acid", "mg/dL", "BIOPRO_J", "LBXSUA", "mg/dL"},
	{"lab.creatine-kinase-total", "U/L", "BIOPRO_J", "LBXSCK", "IU/L"},
	{"lab.ferritin", "ng/mL", "FERTIN_J", "LBXFER", "ng/mL"},
}

// AnalyteFor returns the row for a record type and the unit of a reading.
// units lists the units the type has rows for: it is empty when bean has
// no reference data for the type.
func AnalyteFor(typ, unit string) (a Analyte, ok bool, units []string) {
	for _, row := range Analytes {
		if row.Type != typ {
			continue
		}
		units = append(units, row.Unit)
		if row.Unit == unit {
			a, ok = row, true
		}
	}
	return a, ok, units
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
		return fmt.Sprintf("%s.%s=%s", r.Dataset, r.Variable, Num(r.Min))
	}
	return fmt.Sprintf("%s.%s=%s-%s", r.Dataset, r.Variable, Num(r.Min), Num(r.Max))
}

// Num writes a number the short way, in digits and never with an exponent:
// 20, 18.5, 1000000. A cohort's text is part of a signed claim, and a second
// implementation must write the same text.
func Num(x float64) string { return strconv.FormatFloat(x, 'f', -1, 64) }

// Cohort selects the people a reading is compared with: those who pass
// every rule. Sex and age come first; bean gives no result without them.
type Cohort struct{ Rules []Rule }

// NewCohort makes the cohort of one sex and an age range in years, both
// ends included.
func NewCohort(sex string, ageMin, ageMax int) (Cohort, error) {
	code := map[string]float64{"male": 1, "female": 2}[sex]
	if code == 0 {
		return Cohort{}, errors.New("sex is male or female, as NHANES records it")
	}
	if ageMin < 0 || ageMin > ageMax || ageMax > MaxAge {
		return Cohort{}, fmt.Errorf("age is a range of years from 0 to %d, for example 30-39; %d means %d and over", MaxAge, MaxAge, MaxAge)
	}
	return Cohort{[]Rule{
		{Demo, "RIAGENDR", code, code},
		{Demo, "RIDAGEYR", float64(ageMin), float64(ageMax)},
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
func (c Cohort) Weight() (dataset, variable string) {
	for _, d := range c.Datasets() {
		if w := CohortFiles[d].Weight; w != "" {
			return d, w
		}
	}
	return Demo, Weight
}

// TopAge reports whether the age range ends at MaxAge, which stands for
// every age from there on.
func (c Cohort) TopAge() bool { return len(c.Rules) > 1 && c.Rules[1].Max == MaxAge }

func (c Cohort) String() string {
	sex := "male"
	if c.Rules[0].Min == 2 {
		sex = "female"
	}
	out := fmt.Sprintf("%s, age %s-%s", sex, Num(c.Rules[1].Min), Num(c.Rules[1].Max))
	if c.TopAge() {
		out = fmt.Sprintf("%s, age %s and over", sex, Num(c.Rules[1].Min))
	}
	for _, r := range c.Rules[2:] {
		out += ", " + r.String()
	}
	return out
}

// Result is where a value lies in a cohort: the result of the method, as a
// reference pack gives it too.
type Result = pack.Result

// ErrSmall is the error for a cohort of fewer than MinN people. The result
// then holds the count and no other number.
var ErrSmall = fmt.Errorf("the cohort has fewer than %d people with a value", MinN)

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
// files holds the analyte's file, the demographics file, and the file of
// every rule, by data set name. The method is written once, as numbered
// steps, in docs/COMPARE.md ("The method, version 1"); the comments below
// name the steps.
//
// Everything after the cohort is chosen depends only on the weight that
// each distinct value carries (steps 5 to 8). So a table of those weights
// for a cohort gives the same result, bit for bit, as the files do.
func Compare(files map[string]*xpt.File, a Analyte, c Cohort, v float64) (Result, error) {
	// Step 0.
	if !(v > 0) || math.IsInf(v, 0) {
		if err := c.Check(); err != nil {
			return Result{}, err
		}
		return Result{}, errors.New("the reading is not a number above zero")
	}
	byValue, n, err := Table(files, a.Dataset, a.Variable, a.Label, c)
	if err != nil {
		return Result{N: n}, err
	}
	res := Result{N: n}
	res.Percentile, res.Below, res.Equal, res.P25, res.P50, res.P75 = Place(byValue, v)
	return res, nil
}

// Table is steps 1 to 5: the weight that each distinct value of a variable
// carries in a cohort, added in file order, and the number of people. A
// reference pack holds this table for each cohort on its menu
// (docs/REFERENCE.md). label is the unit in the variable's label, which
// Table checks.
func Table(files map[string]*xpt.File, dataset, variable, label string, c Cohort) (map[float64]float64, int, error) {
	if err := c.Check(); err != nil {
		return nil, 0, err
	}
	file := func(dataset string) (*xpt.File, error) {
		f := files[dataset]
		if f == nil || f.Dataset != dataset {
			return nil, fmt.Errorf("the file of %s is missing", dataset)
		}
		return f, nil
	}
	lab, err := file(dataset)
	if err != nil {
		return nil, 0, err
	}
	if lv, _ := lab.Variable(variable); !strings.HasSuffix(lv.Label, "("+label+")") {
		return nil, 0, fmt.Errorf("%s in %s is labelled %q, not in %s", variable, lab.Dataset, lv.Label, label)
	}
	// Each file's rows by SEQN, read once.
	rows := map[string]map[float64]int{}
	index := func(dataset string) (map[float64]int, []float64, error) {
		f, err := file(dataset)
		if err != nil {
			return nil, nil, err
		}
		seqn, err := f.Column("SEQN")
		if err != nil {
			return nil, nil, err
		}
		if rows[dataset] == nil {
			m := make(map[float64]int, len(seqn))
			for i, s := range seqn {
				if _, twice := m[s]; twice || math.IsNaN(s) {
					return nil, nil, fmt.Errorf("%s has a missing or repeated SEQN", dataset)
				}
				m[s] = i
			}
			rows[dataset] = m
		}
		return rows[dataset], seqn, nil
	}
	col := func(dataset, variable string) (column, error) {
		m, _, err := index(dataset)
		if err != nil {
			return column{}, err
		}
		values, err := files[dataset].Column(variable)
		return column{m, values}, err
	}
	_, seqn, err := index(dataset)
	if err != nil {
		return nil, 0, err
	}
	x, err := lab.Column(variable)
	if err != nil {
		return nil, 0, err
	}
	people, _, err := index(Demo)
	if err != nil {
		return nil, 0, err
	}
	rules := make([]column, len(c.Rules))
	for i, r := range c.Rules {
		if rules[i], err = col(r.Dataset, r.Variable); err != nil {
			return nil, 0, err
		}
	}
	weight, err := col(c.Weight())
	if err != nil {
		return nil, 0, err
	}
	n := 0
	byValue := map[float64]float64{}
rowLoop:
	for i, s := range seqn {
		if math.IsNaN(x[i]) {
			continue
		}
		if _, ok := people[s]; !ok {
			return nil, 0, fmt.Errorf("%s has a person that %s does not: they are not files of one release", lab.Dataset, Demo)
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
		n++
		byValue[x[i]] += w
	}
	if n < MinN {
		return nil, n, ErrSmall
	}
	return byValue, n, nil
}

// Place is steps 6 to 8: where v lies, given the weight of each distinct
// value of a cohort. The sums run over the values in rising order. It is
// the method of a reference pack, and one implementation serves both.
func Place(byValue map[float64]float64, v float64) (percentile, below, equal, p25, p50, p75 float64) {
	values := make([]float64, 0, len(byValue))
	for value := range byValue {
		values = append(values, value)
	}
	sort.Float64s(values)
	weights := make([]float64, len(values))
	for i, value := range values {
		weights[i] = byValue[value]
	}
	return pack.Place(values, weights, v)
}
