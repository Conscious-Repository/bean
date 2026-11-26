package main

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/pack"
)

// comparePack is bean compare -pack: it places readings in the cells of a
// reference pack (docs/REFERENCE.md). The pack is a file on this machine.
// bean reads it, checks every rule of its format and goes nowhere. With no
// reading it shows the pack's menu. With readings and no cohort, each row
// gets its own cohort from two records of the person (person.go).
func comparePack(s *core.Store, path, cohort, typ string, pos []string, save, unknown bool) {
	// No more is read than a pack can be: a pipe states no size.
	f, err := os.Open(path)
	if err != nil {
		fail(err)
	}
	data, err := io.ReadAll(io.LimitReader(f, pack.MaxBytes+1))
	f.Close()
	if err != nil {
		fail(err)
	}
	p, err := pack.Read(data)
	if err != nil {
		fail(fmt.Errorf("%s: %w", path, err))
	}
	menu := slices.Sorted(maps.Keys(p.Cohorts))
	if cohort == "" && len(pos) == 0 && typ == "" && !save {
		printPack(p)
		return
	}
	// A comparison is saved only with a cohort that the person names: the
	// signed claim says that they chose it (DECISIONS 345).
	if cohort == "" && save {
		badArg(fmt.Errorf("name a cohort of the pack with -cohort: %s. -save keeps a comparison only with a cohort that you name: run once for each cohort, with the ids of its readings", strings.Join(menu, ", ")))
	}
	co, ok := p.Cohorts[cohort]
	if cohort != "" && !ok {
		badArg(fmt.Errorf("the pack has no cohort %q. It has: %s", cohort, strings.Join(menu, ", ")))
	}
	// With no cohort, each row gets its own from two records of the person.
	// bean knows whom a cohort holds only for a pack on record. For any
	// other pack this is the first error, as it was before.
	onRec := pack.IsKnown(p.SHA256)
	fromRecords := cohort == ""
	if fromRecords && !onRec {
		badArg(errNoCohortForUnknownPack(strings.Join(menu, ", ")))
	}
	if (len(pos) == 0) == (typ == "") {
		badArg(errors.New("say which readings to compare: record ids, or -type, and not both"))
	}
	// A pack that bean has no record of is used only when told to, as a
	// file that is not the one on record is. Whoever built it chose its
	// tables, its caveats and what each reading is set against.
	if !onRec && !unknown {
		fail(fmt.Errorf("bean has no record of this pack (SHA-256 %s). Whoever built it chose its tables, its caveats and what each reading is set against. Compare its SHA-256 with the one that its publisher states, or build the pack again from the files that it names. To use it as it is, pass -unknown-pack: each result then says so", p.SHA256))
	}
	readings, l := compareReadings(s, typ, pos)
	var who person
	if fromRecords {
		who = readPerson(s, l, strings.Join(menu, ", "))
	}
	labels := l.PairedLabels()
	var rows []comparison
	var claims []map[string]any
	for _, r := range readings {
		c := comparison{reading: r, by: byText(r, l.ID, labels)}
		v, isNumber := r.Value["n"].(float64)
		unit, _ := r.Value["unit"].(string)
		id, units, panelMismatch := p.MatchRecord(r)
		// The row's own cohort, or why it has none. It is worked out only
		// for a reading that the pack has a measure for: any other row
		// was set against no cohort, and shows none.
		rowCohort, noCohort := cohort, ""
		if fromRecords && id != "" && isNumber {
			rowCohort, noCohort = who.cohortOf(s, pack.Known[p.SHA256], r)
			// The row names it, also when the pack has no table for it.
			c.cohort = rowCohort
		}
		cell := p.Cell(id, rowCohort)
		switch {
		case len(units) == 0:
			c.refused, c.unmapped = "no reference data for this type", true
		case panelMismatch:
			// The pack names what the reading must hold. One measure, at
			// most, takes this type and unit.
			c.refused = "the source, printed test or panel does not match the pack"
			same := pack.Same{Type: r.Type, Unit: unit}
			for _, m := range p.Measures {
				if want, ok := m.When[same]; ok {
					c.refused = fmt.Sprintf("required signed fields: source.system=%q and context.test=%q and context.panel=%q", want.System, want.Test, want.Panel)
				} else if rules := m.AnyOf[same]; len(rules) > 0 {
					c.refused = "required signed evidence: " + evidenceText(rules)
				}
			}
		case !isNumber:
			c.refused = "the reading is not a number"
		case id == "" && unit == "":
			c.refused = fmt.Sprintf("the reading has no unit, and the pack takes this type only in %s", strings.Join(units, " or "))
		case id == "":
			c.refused = fmt.Sprintf("the reading is in %q, and the pack takes this type only in %s", unit, strings.Join(units, " or "))
		case noCohort != "":
			c.refused = noCohort
		case cell == nil:
			c.refused = "the pack has no table of this measure for this cohort"
		default:
			res, err := cell.Place(v)
			if err != nil {
				c.refused = err.Error()
				break
			}
			c.measure = id
			if unit == "" {
				c.against = fmt.Sprintf("%s (dimensionless) against %s", r.Type, p.Measures[id].Label)
			} else {
				c.against = fmt.Sprintf("%s in %s against %s", r.Type, unit, p.Measures[id].Label)
			}
			c.result = res
			c.caveats = p.PlacementCaveats(id, rowCohort, res.N, c.by != "")
			c.stated = len(p.StatedCaveats(id, rowCohort))
			if fromRecords {
				// On the screen only: nothing of this run is saved.
				c.caveats[c.stated] = caveatFromRecords
				c.cohortCaveats = len(p.Cohorts[rowCohort].Caveats)
				break
			}
			claims = append(claims, packRecord(c, p, cohort, co, cell.MethodVersion()))
		}
		rows = append(rows, c)
	}
	var known []string
	for _, id := range slices.Sorted(maps.Keys(p.Measures)) {
		for _, same := range p.Measures[id].Same {
			known = append(known, matchText(p.Measures[id], same))
		}
	}
	slices.Sort(known)
	where := fmt.Sprintf("The pack is %s, SHA-256 %s.\n%s", path, p.SHA256, onRecord(p))
	if fromRecords {
		// No line says that the person stated or chose the cohort: a key
		// proves neither. The ids let them see which records it follows.
		head := fmt.Sprintf("Compared with the pack %q.\nCohort: each row's own, from the records person.sex (%s) and person.birth (%s), signed under your identity. To choose one, pass -cohort.\n%s", p.Title, who.sexIDs, who.birthIDs, where)
		compared := printComparisons(os.Stdout, rows, head, strings.Join(known, ", "), typ == "", true)
		endComparisons(l, compared)
		fmt.Println(notSavedFromRecords)
		return
	}
	head := fmt.Sprintf("Compared with the pack %q. Cohort: %q.\n%s", p.Title, co.Text, where)
	printComparisons(os.Stdout, rows, head, strings.Join(known, ", "), typ == "", false)
	saveComparisons(s, l, claims, save, "the pack")
}

// printPack shows what a pack holds: where it comes from, and its menu.
func printPack(p *pack.Pack) {
	// The first line is the pack's own name for itself, and says so.
	fmt.Println("The pack's title: " + quoted(p.Title))
	fmt.Println("sha256 " + p.SHA256)
	fmt.Printf("source: %s, by %s (%s)\n", safe(p.Source), safe(p.Publisher), safe(p.Home))
	fmt.Printf("built by %s, version %s, from %d files:\n", safe(p.BuilderName), safe(p.BuilderVersion), len(p.Files))
	for _, f := range p.Files {
		fmt.Printf("  %s  %s  %d bytes\n", safe(f.Name), f.SHA256, f.Bytes)
	}
	fmt.Println("It has a table for a reading of these types, each in this unit:")
	for _, id := range slices.Sorted(maps.Keys(p.Measures)) {
		m := p.Measures[id]
		var same []string
		for _, row := range m.Same {
			same = append(same, matchText(m, row))
		}
		fmt.Printf("  %s: %s\n", safe(m.Label), safe(strings.Join(same, ", ")))
	}
	fmt.Println("Its cohorts, for -cohort:")
	for _, id := range slices.Sorted(maps.Keys(p.Cohorts)) {
		fmt.Printf("  %s: %s\n", id, safe(p.Cohorts[id].Text))
	}
	// The words are the pack's, also here: bean wrote none of them.
	fmt.Println("The pack states these caveats for every result:")
	for _, text := range p.Caveats {
		fmt.Println("  - the pack says: " + quoted(text))
	}
	fmt.Println(onRecord(p))
}

// onRecord says whether bean knows the pack. A pack is what its SHA-256
// says, and bean has one on record.
func onRecord(p *pack.Pack) string {
	if k, ok := pack.Known[p.SHA256]; ok {
		return "This is " + k.What + " that bean has on record."
	}
	return "WARNING: bean has NO record of this pack. Whoever built it chose its tables, its caveats and what each reading is set against. Compare its SHA-256 with the one that its publisher states, or build the pack again from the files that it names."
}

// packRecord is the claim that -save signs for a comparison with a pack.
// It names the reading by id and the pack by its SHA-256, so anyone with
// the pack computes the result again.
func packRecord(c comparison, p *pack.Pack, cohort string, co pack.Cohort, version string) map[string]any {
	var caveats []any
	for _, text := range c.caveats {
		caveats = append(caveats, text)
	}
	m := p.Measures[c.measure]
	res := c.result
	return map[string]any{
		"type":   compareType,
		"value":  map[string]any{"n": res.Percentile, "unit": "{percentile}"},
		"times":  map[string]any{"observed": c.reading.Raw["times"].(map[string]any)["observed"]},
		"refs":   []any{map[string]any{"rel": "about", "id": c.reading.ID}},
		"source": map[string]any{"system": compareSystem, "adapter": core.Adapter, "ref": nil},
		"context": map[string]any{
			"caveats": caveats,
			"reading": c.reading.ID,
			"reference": map[string]any{
				"pack": p.SHA256, "title": p.Title, "source": p.Source,
				"measure": c.measure, "label": m.Label, "unit": m.Unit, "cohort": cohort,
			},
			"cohort": map[string]any{"text": co.Text},
			"method": map[string]any{"id": pack.MethodID, "version": version},
			"result": map[string]any{
				"n": float64(res.N), "percentile": res.Percentile, "below": res.Below, "equal": res.Equal,
				"p25": res.P25, "p50": res.P50, "p75": res.P75,
			},
		},
	}
}

// matchText states all the evidence the menu asks for.
func matchText(m pack.Measure, row pack.Same) string {
	unit := row.Unit
	if unit == "" {
		unit = "dimensionless; printed unit omitted"
	}
	text := row.Type + " (" + unit + ")"
	if w, ok := m.When[row]; ok {
		text += fmt.Sprintf(" for %s test %q in panel %q", w.System, w.Test, w.Panel)
	}
	if rules := m.AnyOf[row]; len(rules) > 0 {
		text += " when " + evidenceText(rules)
	}
	return text
}

func evidenceText(rules []pack.Evidence) string {
	var choices []string
	for _, rule := range rules {
		var fields []string
		for _, key := range slices.Sorted(maps.Keys(rule)) {
			fields = append(fields, fmt.Sprintf("%s=%q", key, rule[key]))
		}
		choices = append(choices, strings.Join(fields, " and "))
	}
	return strings.Join(choices, " or ")
}
