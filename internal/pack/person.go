package pack

import (
	"math"
	"regexp"
	"strconv"
	"time"
)

// The two records that bean compare -pack reads when the person names no
// cohort (DECISIONS 345). Both are ordinary records, written with bean add,
// and both are optional.
const (
	SexType   = "person.sex"
	BirthType = "person.birth"
)

// Months is a run of calendar months, each counted from year 0: the month
// of January of the year y is 12*y. A birth or a reading that states a year
// alone is the twelve months of that year.
type Months struct{ First, Last int }

var (
	yearPattern  = regexp.MustCompile(`^[0-9]{4}$`)
	monthPattern = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)
)

// calendar reads YYYY or YYYY-MM, which the caller has matched.
func calendar(text string) Months {
	year, _ := strconv.Atoi(text[:4])
	if len(text) == 4 {
		return Months{12 * year, 12*year + 11}
	}
	month, _ := strconv.Atoi(text[5:7])
	return Months{12*year + month - 1, 12*year + month - 1}
}

// BirthMonths reads the value of a person.birth record: a year of four
// digits, as a number with no unit or as text, or the text YYYY-MM. The
// number 1986 and the text 1986 are one value. It has no earliest year and
// no day: a rule of thumb is no rule of the form.
func BirthMonths(value map[string]any) (Months, bool) {
	if len(value) != 1 {
		return Months{}, false
	}
	if n, ok := value["n"].(float64); ok {
		if n != math.Trunc(n) || n < 1000 || n > 9999 {
			return Months{}, false
		}
		return Months{12 * int(n), 12*int(n) + 11}, true
	}
	s, _ := value["s"].(string)
	if !yearPattern.MatchString(s) && !monthPattern.MatchString(s) {
		return Months{}, false
	}
	return calendar(s), true
}

// Sex reads the value of a person.sex record: exactly the text male or
// female, the two groups that a pack on record has. It picks a group to
// compare with, and says nothing more about the person.
func Sex(value map[string]any) (string, bool) {
	s, _ := value["s"].(string)
	return s, len(value) == 1 && (s == "male" || s == "female")
}

// ReadingMonths are the months of a reading's observed start, as written:
// no offset is applied. A reading whose precision is a year is the twelve
// months of that year. start is a time of SPEC.md 3.5.
func ReadingMonths(start, precision string) (Months, bool) {
	if len(start) < 7 || !monthPattern.MatchString(start[:7]) {
		return Months{}, false
	}
	if precision == "year" {
		return calendar(start[:4]), true
	}
	return calendar(start[:7]), true
}

// After reports months that all lie after the month of t.
func (m Months) After(t time.Time) bool {
	return m.First > 12*t.Year()+int(t.Month())-1
}

// Ages returns the lowest and the highest age, in whole years, that a
// person born in one of the months of birth can have in one of the months
// of reading. The lowest is that of a birth on the last day of its last
// month and a reading on the first day of its first month. An age below
// zero says that the reading is before the birth.
func Ages(reading, birth Months) (lowest, highest int) {
	return floorDiv(reading.First-birth.Last-1, 12), floorDiv(reading.Last-birth.First, 12)
}

// floorDiv divides and rounds down, also below zero.
func floorDiv(a, b int) int {
	q := a / b
	if a%b < 0 {
		q--
	}
	return q
}

// group says whom a cohort of a pack on record holds: one sex, and the
// ages From to To in whole years, both ends included. To is below zero for
// a cohort with no highest age. pack/0 has no such field: bean's code keeps
// it beside the cohort's words, for a pack on record only.
type group struct {
	Sex      string
	From, To int
}

// Why a reading gets no cohort from the person's records.
type Why int

const (
	// HasCohort: both ages are in one cohort.
	HasCohort Why = iota
	// BeforeBirth: the reading is before the birth.
	BeforeBirth
	// Edge: the two ages are in two cohorts.
	Edge
	// MenuEdge: one age is in a cohort, and the other is in none.
	MenuEdge
	// NoCohort: no cohort of the pack holds either age.
	NoCohort
)

// cohortOf returns the id of the cohort that holds this sex and age, or "".
func (k OnRecordPack) cohortOf(sex string, age int) string {
	for id, g := range k.groups {
		if g.Sex == sex && age >= g.From && (g.To < 0 || age <= g.To) {
			return id
		}
	}
	return ""
}

// CohortAt returns the cohort of a person of this sex, born in birth, on
// the date of a reading. It has a cohort only when the lowest and the
// highest age on that date are in the same one: bean does not guess
// between two.
func (k OnRecordPack) CohortAt(sex string, birth, reading Months) (string, Why) {
	lowest, highest := Ages(reading, birth)
	if highest < 0 {
		return "", BeforeBirth
	}
	a, b := k.cohortOf(sex, lowest), k.cohortOf(sex, highest)
	switch {
	case a == "" && b == "":
		return "", NoCohort
	case a == "" || b == "":
		return "", MenuEdge
	case a != b:
		return "", Edge
	}
	return a, HasCohort
}
