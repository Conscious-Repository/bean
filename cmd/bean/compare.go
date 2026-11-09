package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/nhanes"
	"github.com/Conscious-Repository/bean/internal/xpt"
)

// bean compare sets a reading against public reference data: where the
// value lies among the people of an NHANES cohort that the person states,
// by sex and age and by any further rule over the survey's variables
// (ethnicity, body measures, a day's food). It is not part of the core: it
// reads records, and with -save it appends ordinary signed claims.
// docs/COMPARE.md has the method.
//
// It describes a position in a sample. It does not diagnose, flag or advise.

// compareType is the type of a saved comparison.
const compareType = "comparison.percentile"

// compareSystem is source.system of a saved comparison.
const compareSystem = "bean-compare"

// Reasons why a comparison with NHANES is qualified. Every comparison of a
// lab reading has the first three. The texts are part of the saved claim.
const (
	reasonMethod     = "the lab that measured the reading and NHANES used their own instruments and methods"
	reasonPopulation = "NHANES is a sample of people living in the United States in the survey years, not a reference range, and like you only in what the cohort states"
	reasonCohort     = "you chose the cohort: bean does not know whether you belong to it"
	reasonSmall      = "the cohort is small: fewer than %d people"
	reasonAuthor     = "the reading was signed by an agent or another key, not by you"
	reasonTopAge     = "NHANES records every age of 80 and over as 80"
	reasonUnknown    = "this copy of %s is not the file on record: its SHA-256 differs"
)

// noReference is the refusal for a type that no analyte row names.
const noReference = "bean has no reference data for this type"

// comparison is one row of the output.
type comparison struct {
	reading *core.Record
	analyte nhanes.Analyte
	// refused says why there is no result; then nothing else is set.
	refused string
	by      string // who signed the reading, when not the person
	result  nhanes.Result
	reasons []string
	files   []*refFile // the analyte's file, the demographics file, then each rule's file
}

// refFile is a kept NHANES file.
type refFile struct {
	record  *core.Record
	dataset string
	digest  string
	size    int
	file    *xpt.File
}

func cmdCompare(s *core.Store, args []string) {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var sex, age onceFlag
	fs.Var(&sex, "sex", "male or female")
	fs.Var(&age, "age", "age range in years, for example 30-39")
	unknown := fs.Bool("unknown-files", false, "also use a kept file whose SHA-256 is not the one on record")
	var where multiFlag
	fs.Var(&where, "where", "a further rule, <DATASET>.<VARIABLE>=<value> or =<from>-<to>; may repeat")
	typ := fs.String("type", "", "compare every current reading of this type, or prefix ending in *")
	save := fs.Bool("save", false, "keep each comparison as a signed record")
	pos, err := parseInterspersed(fs, args)
	if err != nil || (len(pos) == 0) == (*typ == "") {
		usage()
	}
	if sex.twice || age.twice {
		badArg(errors.New("-sex and -age take one value each: one cohort at a time"))
	}
	cohort, err := parseCohort(sex.value, age.value, where)
	if err != nil {
		badArg(err)
	}
	// A comparison whose reading was corrected or deleted meanwhile, on
	// this machine or another, goes before anything is shown.
	sweepComparisons(s)
	recs, l, err := s.List(core.Filter{Type: *typ})
	if err != nil {
		fail(err)
	}
	if l.ID == "" {
		fail(core.ErrNotInitialized)
	}
	var readings []*core.Record
	if *typ != "" {
		for _, r := range recs {
			if r.Subject == l.ID && r.Type != compareType && r.Type != nhanes.FileType && r.Value["blob"] == nil {
				readings = append(readings, r)
			}
		}
		if len(readings) == 0 {
			fail(fmt.Errorf("no current reading of type %s", *typ))
		}
	}
	for _, id := range pos {
		r, err := l.Resolve(id)
		if err != nil {
			fail(err)
		}
		if !l.Current(r.ID) || r.Subject != l.ID {
			fail(fmt.Errorf("%s is not a current record about you: it is %s", r.ID, l.Status(r.ID)))
		}
		readings = append(readings, r)
	}
	files := map[string]*refFile{}
	if *unknown {
		files[""] = nil // the mark that unknown copies are allowed
	}
	labels := l.AgentLabels()
	var rows []comparison
	for _, r := range readings {
		c := compareOne(s, l, files, r, cohort)
		// A reading that someone else signed is marked as in bean ls, and
		// the comparison says so: the person did not write its value.
		if c.by = byText(r, l.ID, labels); c.by != "" && c.refused == "" {
			c.reasons = append(c.reasons, reasonAuthor)
		}
		rows = append(rows, c)
	}
	printComparisons(os.Stdout, rows, cohort, *typ == "")
	if warnFrozen(l) {
		os.Exit(4)
	}
	done := 0
	for _, c := range rows {
		if c.refused == "" {
			done++
		}
	}
	if done == 0 {
		fail(errors.New("nothing could be compared"))
	}
	if !*save {
		fmt.Println("Not saved. Pass -save to keep these comparisons as signed records.")
		return
	}
	var in bytes.Buffer
	deleted := 0
	for _, c := range rows {
		if c.refused != "" {
			continue
		}
		line := core.Canonical(comparisonRecord(c, cohort))
		// A comparison the person deleted stays deleted: say so, where
		// Append would only skip it.
		if purged, err := s.InputPurged(line); err != nil {
			fail(err)
		} else if purged {
			fmt.Printf("not saved: you deleted this comparison of %s before (bean purge)\n", c.reading.ID[7:19])
			deleted++
			continue
		}
		in.Write(line)
		in.WriteByte('\n')
	}
	res, err := s.Append(&in)
	if err != nil {
		fail(err)
	}
	for _, id := range res.IDs {
		fmt.Println("saved", id)
	}
	if res.Added > 0 {
		fmt.Println("Each saved comparison states the cohort you chose and, with the public files, gives the reading's value away, in a record anyone with the log or a share of it can read.")
	}
	fmt.Printf("added %d, already there %d\n", res.Added, res.Skipped-len(res.Changed))
	if deleted > 0 {
		os.Exit(1)
	}
	// The same inputs give the same result, so this is a fault to report,
	// not a value to replace.
	for _, c := range res.Changed {
		fmt.Fprintf(os.Stderr, "bean: the log already has a comparison of these inputs with another result (%s): nothing was written for it\n", c.ID[7:19])
	}
	if len(res.Changed) > 0 {
		os.Exit(1)
	}
}

// parseCohort reads -sex, -age and each -where. Sex and age are required:
// bean does not know them, and never guesses a cohort.
func parseCohort(sex, age string, where []string) (nhanes.Cohort, error) {
	var c nhanes.Cohort
	if sex == "" || age == "" {
		return c, errors.New("not comparable without a cohort: give -sex male|female and -age <from>-<to>, for example -sex female -age 30-39")
	}
	lo, hi, ok := strings.Cut(age, "-")
	ageMin, err1 := strconv.Atoi(lo)
	ageMax, err2 := strconv.Atoi(hi)
	if !ok || err1 != nil || err2 != nil || lo != strconv.Itoa(ageMin) || hi != strconv.Itoa(ageMax) {
		return c, fmt.Errorf("bad -age %q: use <from>-<to> in years, for example 30-39", age)
	}
	c, err := nhanes.NewCohort(sex, ageMin, ageMax)
	if err != nil {
		return c, err
	}
	for _, w := range where {
		m := wherePattern.FindStringSubmatch(w)
		if m == nil {
			return c, fmt.Errorf("bad -where %q: use <DATASET>.<VARIABLE>=<value> or =<from>-<to>, for example BMX_J.BMXBMI=22-27", w)
		}
		r := nhanes.Rule{Dataset: m[1], Variable: m[2]}
		r.Min, _ = strconv.ParseFloat(m[3], 64)
		r.Max = r.Min
		if m[4] != "" {
			r.Max, _ = strconv.ParseFloat(m[4], 64)
		}
		c.Rules = append(c.Rules, r)
	}
	return c, c.Check()
}

// wherePattern is a rule: a data set, a variable, and a number or a range
// of two numbers. NHANES has no negative values a rule would name.
var wherePattern = regexp.MustCompile(`^([A-Z][A-Z0-9_]{0,7})\.([A-Z][A-Z0-9_]{0,7})=([0-9]{1,9}(?:\.[0-9]{1,6})?)(?:-([0-9]{1,9}(?:\.[0-9]{1,6})?))?$`)

// compareOne compares one reading, or says why not. files caches the
// reference files by data set.
func compareOne(s *core.Store, l *core.Loaded, files map[string]*refFile, r *core.Record, cohort nhanes.Cohort) comparison {
	c := comparison{reading: r}
	v, isNumber := r.Value["n"].(float64)
	unit, _ := r.Value["unit"].(string)
	a, ok, units := nhanes.AnalyteFor(r.Type, unit)
	if len(units) == 0 {
		c.refused = noReference
		return c
	}
	if !isNumber {
		c.refused = "the reading is not a number"
		return c
	}
	if !ok {
		c.refused = fmt.Sprintf("the reading is in %q, and bean compares this type only in %s", unit, strings.Join(units, " or "))
		return c
	}
	c.analyte = a
	if !(v > 0) {
		c.refused = "the reading is not above zero"
		return c
	}
	parsed := map[string]*xpt.File{}
	for _, name := range append([]string{a.Dataset, nhanes.Demo}, cohort.Datasets()...) {
		if parsed[name] != nil {
			continue
		}
		f, err := referenceFile(s, l, files, name)
		if err != nil {
			c.refused = err.Error()
			return c
		}
		parsed[name] = f.file
		c.files = append(c.files, f)
	}
	res, err := nhanes.Compare(parsed, a, cohort, v)
	if err != nil {
		c.refused = err.Error()
		return c
	}
	c.result = res
	c.reasons = []string{reasonMethod, reasonPopulation, reasonCohort}
	if cohort.TopAge() {
		c.reasons = append(c.reasons, reasonTopAge)
	}
	for _, d := range cohort.Datasets() {
		if reason := nhanes.CohortFiles[d].Reason; reason != "" {
			c.reasons = append(c.reasons, reason)
		}
	}
	if res.N < nhanes.SmallN {
		c.reasons = append(c.reasons, fmt.Sprintf(reasonSmall, nhanes.SmallN))
	}
	for _, f := range c.files {
		if nhanes.Known[f.dataset] != f.digest {
			c.reasons = append(c.reasons, fmt.Sprintf(reasonUnknown, f.dataset))
		}
	}
	return c
}

// referenceFile finds the person's kept copy of an NHANES file, reads it
// back, checks it against the SHA-256 in its record, and parses it. It
// takes the copy whose SHA-256 is the one on record. Another copy may be a
// download that was cut short, which reads as a smaller file and gives
// another number: it is used only with -unknown-files, and only when it is
// the one such copy.
func referenceFile(s *core.Store, l *core.Loaded, files map[string]*refFile, dataset string) (*refFile, error) {
	if f := files[dataset]; f != nil {
		return f, nil
	}
	_, allowUnknown := files[""]
	var known, others []*core.Record
	for _, id := range slices.Sorted(maps.Keys(l.Report.Records)) {
		r := l.Report.Records[id]
		if r.Type != nhanes.FileType || r.Source.System != nhanes.System || r.Author != l.ID || r.Subject != l.ID || !l.Current(id) || r.Context["dataset"] != dataset {
			continue
		}
		if r.Context["sha256"] == nhanes.Known[dataset] {
			known = append(known, r)
		} else {
			others = append(others, r)
		}
	}
	var r *core.Record
	switch {
	case len(known) > 0:
		r = known[0]
	case len(others) == 0:
		return nil, fmt.Errorf("the log does not keep %s: download %s%s.xpt and run bean import nhanes on it", dataset, nhanes.URL, dataset)
	case !allowUnknown:
		return nil, fmt.Errorf("the kept copy of %s is not the file on record (its SHA-256 is %v): it may be cut short or changed. Download it again from %s%s.xpt and import it, or pass -unknown-files to use this copy", dataset, others[0].Context["sha256"], nhanes.URL, dataset)
	case len(others) > 1:
		return nil, fmt.Errorf("the log keeps %d copies of %s that are not the file on record, and bean cannot choose", len(others), dataset)
	default:
		r = others[0]
	}
	blob, _ := r.Value["blob"].(string)
	digest, _ := r.Context["sha256"].(string)
	data, err := s.GetBlob(blob)
	if err != nil {
		return nil, fmt.Errorf("the kept copy of %s cannot be read: %w", dataset, err)
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != digest {
		return nil, fmt.Errorf("the kept copy of %s no longer matches its record", dataset)
	}
	x, err := xpt.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dataset, err)
	}
	if x.Dataset != dataset {
		return nil, fmt.Errorf("the record of %s keeps a file of %s", dataset, x.Dataset)
	}
	f := &refFile{record: r, dataset: dataset, digest: digest, size: len(data), file: x}
	files[dataset] = f
	return f, nil
}

// onceFlag is a flag that may be given once: a second value would
// silently replace the first.
type onceFlag struct {
	value string
	set   bool
	twice bool
}

func (f *onceFlag) String() string { return f.value }
func (f *onceFlag) Set(v string) error {
	f.twice = f.set
	f.value, f.set = v, true
	return nil
}

// sweepComparisons deletes the person's saved comparisons whose reading is
// no longer current: corrected, or deleted. A comparison is derived from one
// reading, and its result gives that reading's value away to anyone with the
// public files, so it must not outlive it. The deletion is a record, so
// every copy and every reader obeys it. It can be computed again from the
// reading that replaced it. A comparison whose reading this copy has never
// seen is left alone: the reading may not have arrived yet.
func sweepComparisons(s *core.Store) {
	l, err := s.Load()
	if err != nil || l.ID == "" || l.Local == nil || l.IsTooNew() || l.IsFrozen() {
		return
	}
	// What the person deleted: the target of each of their tombstones. A
	// target that is already erased is in no other list.
	deleted := map[string]bool{}
	for _, r := range l.Report.Records {
		if r.Type == "bean.tombstone" && r.Author == l.ID && r.Subject == l.ID && (l.Report.Valid[r.ID] || l.Report.Held[r.ID]) {
			for _, target := range r.Targets("purges") {
				deleted[target] = true
			}
		}
	}
	var stale []string
	for _, id := range slices.Sorted(maps.Keys(l.Report.Records)) {
		r := l.Report.Records[id]
		if !strings.HasPrefix(r.Type, "comparison.") || r.Author != l.ID || r.Subject != l.ID || !l.Current(id) {
			continue
		}
		reading, _ := r.Context["reading"].(string)
		if seen := l.Report.Records[reading]; deleted[reading] || l.Hidden[reading] || (seen != nil && !l.Current(reading)) {
			stale = append(stale, id)
		}
	}
	for _, id := range stale {
		if _, err := s.Purge(id, "its reading was corrected or deleted"); err != nil {
			fmt.Fprintf(os.Stderr, "bean: a comparison of a reading that is gone could not be deleted (%s): %s\n", id[7:19], safe(err.Error()))
			return
		}
	}
	if len(stale) > 0 {
		fmt.Fprintf(os.Stderr, "bean: deleted %d saved comparisons: their readings were corrected or deleted. Run bean compare again for the current readings\n", len(stale))
	}
}

// readingText is the reading's value as bean ls shows it, with who signed
// it when that is not the person.
func readingText(c comparison) string {
	return strings.TrimSpace(valueText(c.reading) + "  " + c.by)
}

// num prints a number the short way.
func num(x float64) string { return strconv.FormatFloat(x, 'g', -1, 64) }

// printComparisons writes one row per reading. Unless all is set, the
// readings of a type that bean has no reference data for are only counted:
// with -type they are most of a lab report.
func printComparisons(w io.Writer, rows []comparison, cohort nhanes.Cohort, all bool) {
	fmt.Fprintf(w, "Compared with NHANES %s: %s.\n", nhanes.Years, cohort)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "READING\tVALUE\tPERCENTILE\tLOWER\tSAME\tMIDDLE HALF\tMEDIAN\tN\tSTATUS\tID")
	reasons := map[string]bool{}
	var order []string
	unmapped := 0
	for _, c := range rows {
		r := c.reading
		if c.refused == noReference && !all {
			unmapped++
			continue
		}
		if c.refused != "" {
			fmt.Fprintf(tw, "%s\t%s\t-\t-\t-\t-\t-\t-\tnot comparable: %s\t%s\n", r.Type, safe(readingText(c)), safe(c.refused), r.ID[7:19])
			continue
		}
		res := c.result
		status := "qualified"
		for _, f := range c.files {
			if nhanes.Known[f.dataset] != f.digest {
				status = "qualified, file not on record"
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s%%\t%s%%\t%s to %s\t%s\t%d\t%s\t%s\n", r.Type, safe(readingText(c)),
			num(res.Percentile), num(res.Below), num(res.Equal), num(res.P25), num(res.P75), num(res.P50), res.N, status, r.ID[7:19])
		for _, reason := range c.reasons {
			if !reasons[reason] {
				reasons[reason] = true
				order = append(order, reason)
			}
		}
	}
	tw.Flush()
	if unmapped > 0 {
		fmt.Fprintf(w, "%d other readings are not comparable: bean has no reference data for their types.\n", unmapped)
	}
	if len(order) > 0 {
		fmt.Fprintln(w, "PERCENTILE is the weighted share of the cohort with a lower value, plus half of those with the same value. It says where a value lies in this sample, not whether it is healthy.")
		fmt.Fprintln(w, "Qualified, because:")
		for _, reason := range order {
			fmt.Fprintln(w, "  - "+reason)
		}
	}
}

// comparisonRecord is the claim that -save signs: the result, what it was
// computed from, and how. It names the reading by id, a hash that pins its
// value. It does not copy the value, but the result gives it away to anyone
// with the files, so sweepComparisons keeps a comparison from outliving its
// reading.
func comparisonRecord(c comparison, cohort nhanes.Cohort) map[string]any {
	refs := []any{map[string]any{"rel": "about", "id": c.reading.ID}}
	var files, reasons []any
	native := []string{c.reading.ID}
	for _, f := range c.files {
		refs = append(refs, map[string]any{"rel": "about", "id": f.record.ID})
		files = append(files, map[string]any{
			"dataset": f.dataset, "sha256": f.digest, "bytes": float64(f.size), "record": f.record.ID,
			"known": nhanes.Known[f.dataset] == f.digest,
		})
		native = append(native, f.digest)
	}
	for _, reason := range c.reasons {
		reasons = append(reasons, reason)
	}
	// Each rule with the label its file gives the variable, so the claim
	// says in words what was selected.
	var rules []any
	for _, r := range cohort.Rules {
		label := ""
		for _, f := range c.files {
			if v, ok := f.file.Variable(r.Variable); ok && f.dataset == r.Dataset {
				label = v.Label
			}
		}
		rules = append(rules, map[string]any{"dataset": r.Dataset, "variable": r.Variable, "label": label, "min": r.Min, "max": r.Max})
		native = append(native, r.String())
	}
	wd, wv := cohort.Weight()
	native = append(native, c.analyte.Variable, wv, nhanes.MethodID+"@"+nhanes.MethodVersion)
	lv, _ := c.files[0].file.Variable(c.analyte.Variable)
	res := c.result
	return map[string]any{
		"type":  compareType,
		"value": map[string]any{"n": res.Percentile, "unit": "{percentile}"},
		"times": map[string]any{"observed": c.reading.Raw["times"].(map[string]any)["observed"]},
		"refs":  refs,
		"source": map[string]any{
			"system": compareSystem, "adapter": core.Adapter,
			"ref_input": core.RefInputNative(compareSystem, strings.Join(native, " "), 0),
		},
		"context": map[string]any{
			"status":  "qualified",
			"reasons": reasons,
			"reading": c.reading.ID,
			"reference": map[string]any{
				"system": nhanes.System, "cycle": nhanes.Years,
				"variable": c.analyte.Variable, "label": lv.Label, "unit": c.analyte.Label,
				"dataset": c.analyte.Dataset, "demographics": nhanes.Demo,
				"weight": map[string]any{"dataset": wd, "variable": wv},
				"files":  files,
			},
			"cohort": map[string]any{"text": cohort.String(), "rules": rules},
			"method": map[string]any{"id": nhanes.MethodID, "version": nhanes.MethodVersion, "min_n": float64(nhanes.MinN)},
			"result": map[string]any{
				"n": float64(res.N), "percentile": res.Percentile, "below": res.Below, "equal": res.Equal,
				"p5": res.P5, "p25": res.P25, "p50": res.P50, "p75": res.P75, "p95": res.P95,
			},
		},
	}
}
