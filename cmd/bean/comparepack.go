package main

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/nhanes"
	"github.com/Conscious-Repository/bean/internal/pack"
)

// comparePack is bean compare -pack: it places readings in the cells of a
// reference pack (docs/REFERENCE.md). The pack is a file on this machine.
// bean reads it, checks every rule of its format and goes nowhere. Without
// a cohort it shows the pack's menu.
func comparePack(s *core.Store, path, cohort, typ string, pos []string, save bool) {
	info, err := os.Stat(path)
	if err != nil {
		fail(err)
	}
	if info.Size() > pack.MaxBytes {
		fail(fmt.Errorf("%s is not read: a pack is at most %d MiB", path, pack.MaxBytes>>20))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fail(err)
	}
	p, err := pack.Read(data)
	if err != nil {
		fail(fmt.Errorf("%s: %w", path, err))
	}
	menu := slices.Sorted(maps.Keys(p.Cohorts))
	if cohort == "" {
		if len(pos) > 0 || typ != "" || save {
			badArg(fmt.Errorf("name a cohort of the pack with -cohort: %s", strings.Join(menu, ", ")))
		}
		printPack(p)
		return
	}
	co, ok := p.Cohorts[cohort]
	if !ok {
		badArg(fmt.Errorf("the pack has no cohort %q. It has: %s", cohort, strings.Join(menu, ", ")))
	}
	if (len(pos) == 0) == (typ == "") {
		badArg(errors.New("say which readings to compare: record ids, or -type, and not both"))
	}
	readings, l := compareReadings(s, typ, pos)
	labels := l.AgentLabels()
	var rows []comparison
	var claims []map[string]any
	for _, r := range readings {
		c := comparison{reading: r, by: byText(r, l.ID, labels)}
		v, isNumber := r.Value["n"].(float64)
		unit, _ := r.Value["unit"].(string)
		id, units := p.MeasureFor(r.Type, unit)
		cell := p.Cell(id, cohort)
		switch {
		case len(units) == 0:
			c.refused, c.unmapped = "no reference data for this type", true
		case !isNumber:
			c.refused = "the reading is not a number"
		case id == "":
			c.refused = fmt.Sprintf("the reading is in %q, and the pack takes this type only in %s", unit, strings.Join(units, " or "))
		case cell == nil:
			c.refused = fmt.Sprintf("the pack has no table of this measure for this cohort: a cohort of fewer than %d people is left out", pack.MinN)
		default:
			res, err := cell.Place(v)
			if err != nil {
				c.refused = err.Error()
				break
			}
			c.measure = id
			c.against = fmt.Sprintf("%s in %s against %s", r.Type, unit, p.Measures[id].Label)
			c.result = nhanes.Result{N: res.N, Percentile: res.Percentile, Below: res.Below, Equal: res.Equal, P25: res.P25, P50: res.P50, P75: res.P75}
			c.caveats = packCaveats(p, id, cohort, res.N, c.by != "")
			claims = append(claims, packRecord(c, p, cohort, co))
		}
		rows = append(rows, c)
	}
	var known []string
	for _, id := range slices.Sorted(maps.Keys(p.Measures)) {
		for _, same := range p.Measures[id].Same {
			known = append(known, same.Type+" ("+same.Unit+")")
		}
	}
	slices.Sort(known)
	head := fmt.Sprintf("Compared with the pack %q. Cohort: %s.\nThe pack is %s, SHA-256 %s.\n%s", p.Title, co.Text, path, p.SHA256, onRecord(p))
	printComparisons(os.Stdout, rows, head, strings.Join(known, ", "), typ == "")
	saveComparisons(s, l, claims, save, "the pack")
}

// packCaveats are the caveats of one comparison with a pack, in the order
// that every implementation gives them: the pack's, the measure's, the
// cohort's, that the person chose the cohort, the small cohort, and the
// reading's signer.
func packCaveats(p *pack.Pack, measure, cohort string, n int, otherSigner bool) []string {
	out := slices.Concat(p.Caveats, p.Measures[measure].Caveats, p.Cohorts[cohort].Caveats, []string{caveatCohort})
	if n < pack.SmallN {
		out = append(out, fmt.Sprintf(caveatSmall, pack.SmallN))
	}
	if otherSigner {
		out = append(out, caveatAuthor)
	}
	return out
}

// printPack shows what a pack holds: where it comes from, and its menu.
func printPack(p *pack.Pack) {
	fmt.Println(safe(p.Title))
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
			same = append(same, row.Type+" ("+row.Unit+")")
		}
		fmt.Printf("  %s: %s\n", safe(m.Label), safe(strings.Join(same, ", ")))
	}
	fmt.Println("Its cohorts, for -cohort:")
	for _, id := range slices.Sorted(maps.Keys(p.Cohorts)) {
		fmt.Printf("  %s: %s\n", id, safe(p.Cohorts[id].Text))
	}
	fmt.Println("Read every result with these caveats:")
	for _, text := range p.Caveats {
		fmt.Println("  - " + safe(text))
	}
	fmt.Println(onRecord(p))
}

// onRecord says whether bean knows the pack. A pack is what its SHA-256
// says, and bean has one on record.
func onRecord(p *pack.Pack) string {
	if _, ok := pack.Known[p.SHA256]; ok {
		return "This is the NHANES pack that bean has on record."
	}
	return "bean has no record of this pack. Compare its SHA-256 with the one that its publisher states, or build the pack again from the files that it names."
}

// packRecord is the claim that -save signs for a comparison with a pack.
// It names the reading by id and the pack by its SHA-256, so anyone with
// the pack computes the result again.
func packRecord(c comparison, p *pack.Pack, cohort string, co pack.Cohort) map[string]any {
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
			"method": map[string]any{"id": pack.MethodID, "version": pack.MethodVersion},
			"result": map[string]any{
				"n": float64(res.N), "percentile": res.Percentile, "below": res.Below, "equal": res.Equal,
				"p25": res.P25, "p50": res.P50, "p75": res.P75,
			},
		},
	}
}
