package main

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/pack"
)

// bean compare -pack with no -cohort takes each reading's cohort from two
// records of the person, person.sex and person.birth, and from the
// reading's own date (DECISIONS 345). It shows, and saves nothing. With
// -cohort bean reads neither record.

// caveatFromRecords takes the place of "you chose the cohort" on the
// screen. No claim holds it: a comparison is saved only with -cohort.
const caveatFromRecords = "the cohort follows from those records as they are now: bean does not know whether you belong to it"

// notSavedFromRecords is the last line of such a run.
const notSavedFromRecords = "Not saved. -save needs -cohort: run once for each cohort, with -save and the ids of its readings."

// person is what the two records state: one sex and one birth.
type person struct {
	sex       string
	birth     pack.Months
	birthYear bool   // the birth is a year alone
	sexIDs    string // the short ids of the records, for the line above the table
	birthIDs  string
}

// personRecords returns the records of one type that count, by id: current
// ones that the person's identity signed about the person, with the source
// self and no ref. A record of an agent, of a peer or of an importer is
// not the person's word, and is not read.
func personRecords(l *core.Loaded, typ string) []*core.Record {
	var out []*core.Record
	for _, id := range slices.Sorted(maps.Keys(l.Report.Records)) {
		r := l.Report.Records[id]
		if r.Type == typ && l.Current(id) && r.Author == l.ID && r.Subject == l.ID && r.Source.System == "self" && r.Source.Ref == "" {
			out = append(out, r)
		}
	}
	return out
}

// oneValue reports records that all state the same value. A birth in one
// of its forms is the months that it names, so that a number and a text of
// one year are one value. Any other value is its canonical bytes.
func oneValue(records []*core.Record) bool {
	stated := func(r *core.Record) string {
		if months, ok := pack.BirthMonths(r.Value); ok {
			return fmt.Sprint(months)
		}
		return string(core.Canonical(r.Value))
	}
	for _, r := range records[1:] {
		if stated(r) != stated(records[0]) {
			return false
		}
	}
	return true
}

// otherRecords reports a current record of one of these types that does
// not count: bean lists it, and does not read it.
func otherRecords(l *core.Loaded, types []string) bool {
	for id, r := range l.Report.Records {
		if slices.Contains(types, r.Type) && l.Current(id) {
			return true
		}
	}
	return false
}

func shortIDs(records []*core.Record) string {
	var ids []string
	for _, r := range records {
		ids = append(ids, r.ID[7:19])
	}
	return strings.Join(ids, ", ")
}

// readPerson reads the two records, or stops. A stop names the type, the
// short ids and the rule that failed, and no value. Then it names -cohort,
// which leaves nothing in the log. It names a correction only for two
// values and for a birth in a wrong form. For a word of sex that the pack
// does not have it names -cohort alone: nobody is asked to write what is
// not true.
func readPerson(s *core.Store, l *core.Loaded, menu string) person {
	sexes, births := personRecords(l, pack.SexType), personRecords(l, pack.BirthType)
	if len(sexes) == 0 || len(births) == 0 {
		var missing []string
		if len(sexes) == 0 {
			missing = append(missing, pack.SexType)
		}
		if len(births) == 0 {
			missing = append(missing, pack.BirthType)
		}
		// No command to copy is here: the two are the person's to state,
		// or not, and whoever runs this may be their assistant.
		notRead := ""
		if otherRecords(l, missing) {
			notRead = " It has one that bean does not read: only a record that your identity signed about you with bean add counts, and not one of an agent, a peer or an importer."
		}
		badArg(fmt.Errorf("name a cohort of the pack with -cohort: %s. With no -cohort, bean takes each reading's cohort from its date and from two records, person.sex and person.birth. The log has no current %s that your identity signed.%s Both are optional, and -cohort needs neither: docs/REFERENCE.md, \"The cohort from two records\", has the rules and says who can read them", menu, strings.Join(missing, " and no "), notRead))
	}
	orCohort := "Or name a cohort of the pack with -cohort: " + menu
	p := person{sexIDs: shortIDs(sexes), birthIDs: shortIDs(births)}
	// All of one type must state one value. The number 1986 and the text
	// 1986 are one value.
	if !oneValue(sexes) {
		fail(fmt.Errorf("the records person.sex (%s) do not state one value. Correct one with bean add person.sex <value> -corrects <id>, or delete one with bean purge <id>. %s", p.sexIDs, orCohort))
	}
	if !oneValue(births) {
		fail(fmt.Errorf("the records person.birth (%s) do not state one value. Correct one with bean add person.birth <value> -corrects <id>, or delete one with bean purge <id>. %s", p.birthIDs, orCohort))
	}
	birth, form := pack.BirthMonths(births[0].Value)
	if !form {
		fail(fmt.Errorf("the record person.birth (%s) is not a year of four digits with no unit, or a year and a month as 1986-05. Correct it with bean add person.birth <value> -corrects <id>. %s", p.birthIDs, orCohort))
	}
	if birth.After(s.Now()) {
		fail(fmt.Errorf("the record person.birth (%s) is after now. Correct it with bean add person.birth <value> -corrects <id>. %s", p.birthIDs, orCohort))
	}
	sex, ok := pack.Sex(sexes[0].Value)
	if !ok {
		fail(fmt.Errorf("the record person.sex (%s) is not exactly male or female, the two groups that this pack has. Name a cohort of the pack with -cohort: %s", p.sexIDs, menu))
	}
	p.sex, p.birth, p.birthYear = sex, birth, birth.First != birth.Last
	return p
}

// cohortOf returns the cohort of one reading on its own date, or why it
// has none. The words name no age. Each says what the person can do.
func (p person) cohortOf(s *core.Store, k pack.OnRecordPack, r *core.Record) (cohort, refused string) {
	obs, _ := r.Observed.(map[string]any)
	start, _ := obs["start"].(string)
	precision, _ := obs["precision"].(string)
	// The start of a valid record is a time, or there is none.
	at, _ := core.ParseTime(start)
	months, ok := pack.ReadingMonths(start, precision)
	switch {
	case !ok:
		return "", "the reading has no date: name a cohort with -cohort"
	case at.After(s.Now()):
		return "", "the reading is dated after now: name a cohort with -cohort"
	}
	id, why := k.CohortAt(p.sex, p.birth, months)
	switch why {
	case pack.BeforeBirth:
		return "", "the reading is dated before the birth in person.birth: correct one, or name a cohort with -cohort"
	case pack.NoCohort:
		return "", "the pack has no cohort for the age on that date: to choose one, pass -cohort"
	case pack.Edge, pack.MenuEdge:
		refused = "the date lies on the edge of two cohorts: name one with -cohort"
		if why == pack.MenuEdge {
			refused = "the date lies on the edge of the pack's cohorts: to choose one, pass -cohort"
		}
		if p.birthYear {
			refused += ", or add the month of birth"
		}
		return "", refused
	}
	return id, ""
}

// errNoCohortForUnknownPack: bean knows the sex and the ages of a cohort
// only for a pack on record. A pack's words for a cohort are its builder's.
func errNoCohortForUnknownPack(menu string) error {
	return errors.New("name a cohort of the pack with -cohort: " + menu + ". bean takes a cohort from your records only with a pack that it has on record: it does not know whom a cohort of this pack holds")
}
