package nhanes

import (
	"fmt"
	"slices"
	"sort"

	"github.com/Conscious-Repository/bean/internal/pack"
	"github.com/Conscious-Repository/bean/internal/xpt"
)

// The caveats of a comparison with NHANES. The texts are part of a saved
// claim and of a pack, and the Python tools hold the same texts: a test
// keeps them equal.
const (
	CaveatLab        = "the lab that measured the reading and NHANES used their own instruments and methods"
	CaveatBody       = "the reading and NHANES were measured with their own instruments and methods"
	CaveatPopulation = "NHANES is a sample of people living in the United States in the survey years, not a reference range"
	CaveatTopAge     = "NHANES records every age of 80 and over as 80"
	CaveatUnknown    = "this copy of %s is not the file on record: its SHA-256 differs"
)

// The builder of a pack from NHANES files. A change to what BuildPack
// writes for the same files is a new version.
const (
	PackBuilder        = "bean-pack-nhanes"
	PackBuilderVersion = "1"
)

// KnownPack is the SHA-256 of the pack that BuildPack gives from the six
// files on record: DEMO_J, TCHOL_J, HDL_J, GHB_J, CBC_J and BMX_J. It is
// the pack on record. docs/REFERENCE.md states it too, and a test with
// CDC's files keeps it true.
const KnownPack = "efbb252a361b1994c67207618a088d492d1b0dafb8ea5ff88f857f97abc69277"

// PackMeasure is a variable that a pack holds, beside the lab tests of
// Analytes: a body measure. Each Same row is a type that bean or one of
// its importers writes, in the variable's own unit.
type PackMeasure struct {
	Dataset, Variable, Label, Caveat string
	Same                             []pack.Same
}

// BodyMeasures are measured in everyone examined, as the lab tests are, so
// they use the examination weight.
var BodyMeasures = []PackMeasure{
	{"BMX_J", "BMXWT", "kg", CaveatBody, []pack.Same{{Type: "weight", Unit: "kg"}, {Type: "apple-health.body-mass", Unit: "kg"}}},
	{"BMX_J", "BMXHT", "cm", CaveatBody, []pack.Same{{Type: "apple-health.height", Unit: "cm"}}},
}

// PackAges are the age ranges of a pack's menu: ten years each from 20,
// and 80, which stands for 80 and over. The menu is short on purpose. For
// another cohort, compare with the files.
var PackAges = [][2]int{{20, 29}, {30, 39}, {40, 49}, {50, 59}, {60, 69}, {70, 79}, {80, 80}}

// PackCohortID names a cohort of the menu: male-30-39, female-80-over.
func PackCohortID(sex string, ages [2]int) string {
	if ages[1] == MaxAge {
		return fmt.Sprintf("%s-%d-over", sex, ages[0])
	}
	return fmt.Sprintf("%s-%d-%d", sex, ages[0], ages[1])
}

// SourceFile is one NHANES file that a pack is built from.
type SourceFile struct {
	File   *xpt.File
	SHA256 string
	Bytes  int64
}

// BuildPack builds the pack of the files given, by data set name: for each
// measure whose file is there, and each cohort of the menu, the table of
// steps 1 to 5 of the method (docs/COMPARE.md). A cell of fewer than MinN
// people is left out. The same files give the same pack, byte for byte. A
// file that is not the one on record is named in the pack's caveats.
func BuildPack(files map[string]SourceFile) (*pack.Pack, error) {
	if _, ok := files[Demo]; !ok {
		return nil, fmt.Errorf("the file of %s is needed: it has the sex, the age and the weight of each person", Demo)
	}
	p := &pack.Pack{
		Title:       "NHANES " + Years + ": adults by sex and ten years of age",
		Source:      "NHANES " + Years,
		Publisher:   "US National Center for Health Statistics (CDC)",
		Home:        "https://wwwn.cdc.gov/nchs/nhanes/",
		BuilderName: PackBuilder, BuilderVersion: PackBuilderVersion,
		Caveats:  []string{CaveatPopulation},
		Measures: map[string]pack.Measure{},
		Cohorts:  map[string]pack.Cohort{},
	}
	parsed := map[string]*xpt.File{}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		f := files[name]
		if f.File == nil || f.File.Dataset != name || !InCycle(name) {
			return nil, fmt.Errorf("%s is not a file of NHANES %s", name, Years)
		}
		parsed[name] = f.File
		p.Files = append(p.Files, pack.File{Name: name + ".xpt", SHA256: f.SHA256, Bytes: f.Bytes, URL: URL + name + ".xpt"})
		if f.SHA256 != Known[name] {
			p.Caveats = append(p.Caveats, fmt.Sprintf(CaveatUnknown, name))
		}
	}
	// The measures: each lab test once, with a Same row for each name and
	// unit that a report prints, then the body measures.
	var measures []PackMeasure
	for _, a := range Analytes {
		i := slices.IndexFunc(measures, func(m PackMeasure) bool { return m.Variable == a.Variable })
		if i < 0 {
			measures = append(measures, PackMeasure{Dataset: a.Dataset, Variable: a.Variable, Label: a.Label, Caveat: CaveatLab})
			i = len(measures) - 1
		}
		measures[i].Same = append(measures[i].Same, pack.Same{Type: a.Type, Unit: a.Unit})
	}
	measures = append(measures, BodyMeasures...)
	type menu struct {
		id string
		c  Cohort
	}
	var cohorts []menu
	for _, sex := range []string{"female", "male"} {
		for _, ages := range PackAges {
			c, err := NewCohort(sex, ages[0], ages[1])
			if err != nil {
				return nil, err
			}
			id := PackCohortID(sex, ages)
			cohorts = append(cohorts, menu{id, c})
			entry := pack.Cohort{Text: c.String()}
			if c.TopAge() {
				entry.Caveats = []string{CaveatTopAge}
			}
			p.Cohorts[id] = entry
		}
	}
	for _, m := range measures {
		f := parsed[m.Dataset]
		if f == nil {
			continue
		}
		// A file that does not have the variable gives no measure: the
		// pack holds what the files hold.
		lv, has := f.Variable(m.Variable)
		if !has {
			continue
		}
		p.Measures[m.Variable] = pack.Measure{Label: lv.Label, Unit: m.Label, Caveats: []string{m.Caveat}, Same: m.Same}
		for _, co := range cohorts {
			byValue, n, err := Table(parsed, m.Dataset, m.Variable, m.Label, co.c)
			if err == ErrSmall {
				continue
			}
			if err != nil {
				return nil, err
			}
			cell := pack.Cell{Measure: m.Variable, Cohort: co.id, N: n}
			for value := range byValue {
				cell.Values = append(cell.Values, value)
			}
			sort.Float64s(cell.Values)
			for _, value := range cell.Values {
				cell.Weights = append(cell.Weights, byValue[value])
			}
			p.Cells = append(p.Cells, cell)
		}
	}
	if len(p.Cells) == 0 {
		return nil, fmt.Errorf("these files give no cell: give %s and the file of a test, such as TCHOL_J", Demo)
	}
	sort.SliceStable(p.Cells, func(i, j int) bool {
		if p.Cells[i].Measure != p.Cells[j].Measure {
			return p.Cells[i].Measure < p.Cells[j].Measure
		}
		return p.Cells[i].Cohort < p.Cells[j].Cohort
	})
	return p, nil
}
