package pack

import (
	"fmt"
	"slices"
)

// OnRecordPack is what bean holds of a pack that it has on record: what it
// is, the label of each measure and the words of each cohort. With these a
// check that does not have the pack's file still shows the pack's own
// label and cohort beside a comparison, not the words of whoever signed
// the claim, and tells a reading that the pack does not take.
type OnRecordPack struct {
	What     string
	Measures map[string]string // measure id -> label
	Cohorts  map[string]string // cohort id -> text
	Same     map[string][]Same // measure id -> the types and units that it takes
	// The caveats that the pack gives: for every comparison, for each
	// measure, and for each cohort that has one.
	Caveats        []string
	MeasureCaveats map[string][]string
	CohortCaveats  map[string][]string
	// groups says whom each cohort holds: its sex and its ages. No check
	// of a saved comparison reads it. bean compare -pack reads it when the
	// person names no cohort (DECISIONS 345).
	groups map[string]group
}

// The caveats that a claim adds to its pack's, in this order
// (docs/REFERENCE.md, "The claim"). CaveatSmall takes SmallN.
const (
	CaveatCohort = "you chose the cohort: bean does not know whether you belong to it"
	CaveatSmall  = "the cohort is small: fewer than %d people"
	CaveatAuthor = "the reading was signed by an agent or another key, not by you"
)

// ClaimCaveats reports whether caveats are ones that a claim with this
// pack, measure and cohort can have: the pack's, the measure's, the
// cohort's and that the person chose the cohort, in that order, then the
// small cohort, that the pack was not on record, and the reading's signer,
// where each applies. small and otherSigner say whether those two apply;
// nil means that the caller cannot tell, and then either is allowed.
//
// That the pack was not on record is what the signer's bean knew on the
// day of the claim. A pack can be put on record later, so a claim may
// have that caveat or not, whatever this bean has on record: a signed
// record means the same for ever (DECISIONS 319).
func (k OnRecordPack) ClaimCaveats(measure, cohort string, caveats []string, small, otherSigner *bool) bool {
	want := append(append(append(append([]string{}, k.Caveats...), k.MeasureCaveats[measure]...), k.CohortCaveats[cohort]...), CaveatCohort)
	if len(caveats) < len(want) {
		return false
	}
	for i, text := range want {
		if caveats[i] != text {
			return false
		}
	}
	rest := caveats[len(want):]
	for _, optional := range []struct {
		text    string
		applies *bool
	}{{fmt.Sprintf(CaveatSmall, SmallN), small}, {CaveatNotOnRecord, nil}, {CaveatAuthor, otherSigner}} {
		has := len(rest) > 0 && rest[0] == optional.text
		if has {
			rest = rest[1:]
		}
		if optional.applies != nil && *optional.applies != has {
			return false
		}
	}
	return len(rest) == 0
}

// Takes reports whether the pack sets a reading of this type and unit
// against the measure.
func (k OnRecordPack) Takes(measure, typ, unit string) bool {
	for _, s := range k.Same[measure] {
		if s.Type == typ && s.Unit == unit {
			return true
		}
	}
	return false
}

// IsDigest reports a SHA-256 as a pack is named by: 64 lower-case hex
// digits.
func IsDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// OwnType reports a type of bean's own records. Such a record is not a
// reading, so no measure may claim it and no comparison is about one.
func OwnType(typ string) bool { return ownType(typ) }

// Known are the packs that bean has on record, by SHA-256. Both are built
// by bean-pack-nhanes version 1 from NHANES files on record
// (internal/nhanes). The first is the pack of six files: twelve blood
// tests and weight. The second is the pack of eight files: it adds six
// tests of the standard biochemistry profile, and ferritin. The
// first stays on record: a comparison that a person saved with it still
// names it. A test with CDC's files keeps each digest, the labels and the
// cohorts true, and docs/REFERENCE.md states the digests too.
var Known = map[string]OnRecordPack{
	"10555d6769421d4305cbcd4ff14960d8b966ddd829a50581a9584a60a84d29e2": nhanesPack("the NHANES 2017-2018 pack", nhanesSix),
	"5cae07db64295d565ffae7c251c17fdb9f043dc0638868557275f8b0b0311efa": nhanesPack("the NHANES 2017-2018 pack of 20 measures", nhanesSix, nhanesAdded),
}

// nhanesMeasure is what bean holds of one measure of an NHANES pack: its
// label, and the types and units that it takes.
type nhanesMeasure struct {
	id, label string
	same      []Same
}

// nhanesSix are the measures of the pack of six files.
var nhanesSix = []nhanesMeasure{
	{"BMXWT", "Weight (kg)", []Same{{"weight", "kg"}, {"apple-health.body-mass", "kg"}}},
	{"LBDHDD", "Direct HDL-Cholesterol (mg/dL)", []Same{{"lab.hdl-cholesterol", "mg/dL"}}},
	{"LBXGH", "Glycohemoglobin (%)", []Same{{"lab.hemoglobin-a1c", "%"}, {"lab.hemoglobin-a1c", "% of total Hgb"}}},
	{"LBXHCT", "Hematocrit (%)", []Same{{"lab.hematocrit", "%"}}},
	{"LBXHGB", "Hemoglobin (g/dL)", []Same{{"lab.hemoglobin", "g/dL"}}},
	{"LBXMC", "Mean Cell Hgb Conc. (g/dL)", []Same{{"lab.mchc", "g/dL"}}},
	{"LBXMCHSI", "Mean cell hemoglobin (pg)", []Same{{"lab.mch", "pg"}}},
	{"LBXMCVSI", "Mean cell volume (fL)", []Same{{"lab.mcv", "fL"}}},
	{"LBXPLTSI", "Platelet count (1000 cells/uL)", []Same{{"lab.platelets", "x10E3/uL"}, {"lab.platelet-count", "Thousand/uL"}}},
	{"LBXRBCSI", "Red blood cell count (million cells/uL)", []Same{{"lab.rbc", "x10E6/uL"}, {"lab.red-blood-cell-count", "Million/uL"}}},
	{"LBXRDW", "Red cell distribution width (%)", []Same{{"lab.rdw", "%"}}},
	{"LBXTC", "Total Cholesterol (mg/dL)", []Same{{"lab.cholesterol-total", "mg/dL"}}},
	{"LBXWBCSI", "White blood cell count (1000 cells/uL)", []Same{{"lab.wbc", "x10E3/uL"}, {"lab.white-blood-cell-count", "Thousand/uL"}}},
}

// nhanesAdded are the measures that the pack of eight files adds to
// those of the pack of six.
var nhanesAdded = []nhanesMeasure{
	{"LBXFER", "Ferritin (ng/mL)", []Same{{"lab.ferritin", "ng/mL"}}},
	{"LBXSBU", "Blood Urea Nitrogen (mg/dL)", []Same{{"lab.bun", "mg/dL"}, {"lab.urea-nitrogen-bun", "mg/dL"}}},
	{"LBXSCK", "Creatine Phosphokinase (CPK) (IU/L)", []Same{{"lab.creatine-kinase-total", "U/L"}}},
	{"LBXSCR", "Creatinine, refrigerated serum (mg/dL)", []Same{{"lab.creatinine", "mg/dL"}}},
	{"LBXSKSI", "Potassium (mmol/L)", []Same{{"lab.potassium", "mmol/L"}}},
	{"LBXSTP", "Total Protein (g/dL)", []Same{{"lab.protein-total", "g/dL"}}},
	{"LBXSUA", "Uric acid (mg/dL)", []Same{{"lab.uric-acid", "mg/dL"}}},
}

// nhanesPack is what bean holds of an NHANES pack with these measures.
// Weight is a body measure, and every other measure is a lab test.
func nhanesPack(what string, lists ...[]nhanesMeasure) OnRecordPack {
	cohorts, groups := sexByAge()
	k := OnRecordPack{
		What:           what,
		Measures:       map[string]string{},
		Cohorts:        cohorts,
		groups:         groups,
		Same:           map[string][]Same{},
		Caveats:        []string{"NHANES is a sample of people living in the United States in the survey years, not a reference range"},
		MeasureCaveats: map[string][]string{},
		CohortCaveats: map[string][]string{
			"female-80-over": {"NHANES records every age of 80 and over as 80"},
			"male-80-over":   {"NHANES records every age of 80 and over as 80"},
		},
	}
	for _, list := range lists {
		for _, m := range list {
			k.Measures[m.id], k.Same[m.id] = m.label, m.same
			k.MeasureCaveats[m.id] = []string{nhanesLab}
			if m.id == "BMXWT" {
				k.MeasureCaveats[m.id] = []string{"the reading and NHANES were measured with their own instruments and methods"}
			}
		}
	}
	return k
}

const nhanesLab = "the lab that measured the reading and NHANES used their own instruments and methods"

// sexByAge is the menu of the NHANES pack: each sex, by ten years of age
// from 20 to 79, and 80 and over. It gives each cohort in words, and the
// sex and the ages that it holds.
func sexByAge() (map[string]string, map[string]group) {
	texts, groups := map[string]string{}, map[string]group{}
	for _, sex := range []string{"female", "male"} {
		for from := 20; from < 80; from += 10 {
			id := fmt.Sprintf("%s-%d-%d", sex, from, from+9)
			texts[id], groups[id] = fmt.Sprintf("%s, age %d-%d", sex, from, from+9), group{sex, from, from + 9}
		}
		texts[sex+"-80-over"], groups[sex+"-80-over"] = sex+", age 80 and over", group{sex, 80, -1}
	}
	return texts, groups
}

// OnRecord says in a few words whether bean knows a pack by its SHA-256.
func OnRecord(sha256 string) string {
	if k, ok := Known[sha256]; ok {
		return "this is " + k.What + " that bean has on record"
	}
	return "bean has NO record of this pack"
}

// IsKnown reports a pack that bean has on record.
func IsKnown(sha256 string) bool { _, ok := Known[sha256]; return ok }

// CaveatNotOnRecord is the caveat of a comparison with a pack that bean has
// no record of. Whoever built such a pack chose its tables, its caveats
// and what each reading is set against, and nothing here has checked them.
const CaveatNotOnRecord = "bean has no record of this pack: nobody has checked its tables, its caveats, or what it sets a reading against"

// StatedCaveats are the texts that the pack itself states for one
// comparison: the pack's, the measure's and the cohort's. PlacementCaveats
// starts with them. Whoever built the pack wrote them, and bean wrote the rest.
func (p *Pack) StatedCaveats(measure, cohort string) []string {
	return slices.Concat(p.Caveats, p.Measures[measure].Caveats, p.Cohorts[cohort].Caveats)
}

// PlacementCaveats lists the caveats of one comparison with a pack, in the order
// that every implementation gives them: the pack's, the measure's, the
// cohort's, that the person chose the cohort, the small cohort, a pack
// that bean has no record of, and the reading's signer.
func (p *Pack) PlacementCaveats(measure, cohort string, n int, otherSigner bool) []string {
	out := append(p.StatedCaveats(measure, cohort), CaveatCohort)
	if n < SmallN {
		out = append(out, fmt.Sprintf(CaveatSmall, SmallN))
	}
	if !IsKnown(p.SHA256) {
		out = append(out, CaveatNotOnRecord)
	}
	if otherSigner {
		out = append(out, CaveatAuthor)
	}
	return out
}
