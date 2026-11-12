package pack

import "fmt"

// OnRecordPack is what bean holds of a pack that it has on record: what it
// is, the label of each measure and the words of each cohort. With these a
// check that does not have the pack's file still shows the pack's own
// label and cohort beside a comparison, not the words of whoever signed
// the claim.
type OnRecordPack struct {
	What     string
	Measures map[string]string // measure id -> label
	Cohorts  map[string]string // cohort id -> text
}

// Known are the packs that bean has on record, by SHA-256. Today that is
// one: the pack that bean-pack-nhanes version 1 builds from the six NHANES
// files on record (internal/nhanes). A test with CDC's files keeps the
// digest, the labels and the cohorts true, and docs/REFERENCE.md states
// the digest too.
var Known = map[string]OnRecordPack{
	"10555d6769421d4305cbcd4ff14960d8b966ddd829a50581a9584a60a84d29e2": {
		What: "the NHANES 2017-2018 pack",
		Measures: map[string]string{
			"BMXWT":    "Weight (kg)",
			"LBDHDD":   "Direct HDL-Cholesterol (mg/dL)",
			"LBXGH":    "Glycohemoglobin (%)",
			"LBXHCT":   "Hematocrit (%)",
			"LBXHGB":   "Hemoglobin (g/dL)",
			"LBXMC":    "Mean Cell Hgb Conc. (g/dL)",
			"LBXMCHSI": "Mean cell hemoglobin (pg)",
			"LBXMCVSI": "Mean cell volume (fL)",
			"LBXPLTSI": "Platelet count (1000 cells/uL)",
			"LBXRBCSI": "Red blood cell count (million cells/uL)",
			"LBXRDW":   "Red cell distribution width (%)",
			"LBXTC":    "Total Cholesterol (mg/dL)",
			"LBXWBCSI": "White blood cell count (1000 cells/uL)",
		},
		Cohorts: sexByAge(),
	},
}

// sexByAge is the menu of the NHANES pack: each sex, by ten years of age
// from 20 to 79, and 80 and over.
func sexByAge() map[string]string {
	out := map[string]string{}
	for _, sex := range []string{"female", "male"} {
		for from := 20; from < 80; from += 10 {
			out[fmt.Sprintf("%s-%d-%d", sex, from, from+9)] = fmt.Sprintf("%s, age %d-%d", sex, from, from+9)
		}
		out[sex+"-80-over"] = sex + ", age 80 and over"
	}
	return out
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
