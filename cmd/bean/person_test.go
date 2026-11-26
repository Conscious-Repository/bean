package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

// personLog is compareLog with the synthetic pack on record for the runs
// of this test, as a pack of the survey is. It returns the folder, the
// pack and the ids of compareLog: three readings of 2026-11-01.
func personLog(t *testing.T) (dir, path string, ids [3]string) {
	t.Helper()
	dir, _, ids = compareLog(t, "TCHOL_J", "HDL_J", "DEMO_J")
	path, digest := testPack(t)
	t.Setenv("BEAN_TEST_PACK_ON_RECORD", digest)
	return dir, path, ids
}

// add runs bean add and returns the id.
func add(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, errs, code := run(t, dir, "", append([]string{"add"}, args...)...)
	if code != 0 {
		t.Fatalf("add %v: %d\n%s", args, code, errs)
	}
	return strings.TrimSpace(out)
}

// cholesterol adds a reading of total cholesterol on a day.
func cholesterol(t *testing.T, dir, value, at string) string {
	t.Helper()
	return add(t, dir, "lab.cholesterol-total", value, "-unit", "mg/dL", "-at", at)
}

// runAt is run with the clock set, as $BEAN_NOW sets it.
func runAt(t *testing.T, dir, now string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "BEAN_TEST_MAIN=1", "BEAN_DIR="+dir, "BEAN_NOW="+now, "GORACE=atexit_sleep_ms=0")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), errb.String(), code
}

// cohortOfRow returns the COHORT column of a reading's row, and the row.
func cohortOfRow(t *testing.T, out, id string) (string, string) {
	t.Helper()
	head := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "DATE ") {
			head = line
		}
		if strings.Contains(line, id[7:19]) && head != "" {
			// The cell starts under the head of its column. An empty cell
			// is not the dash of the column after it.
			at := strings.Index(head, "COHORT")
			if at < 0 || at >= len(line) || line[at] == ' ' {
				return "", line
			}
			return strings.Fields(line[at:])[0], line
		}
	}
	t.Fatalf("no row of %s in\n%s", id[7:19], out)
	return "", ""
}

// The helper tells a cohort, a dash and an empty cell apart: a test that
// reads the column with it holds what the column shows.
func TestCohortOfRowReadsItsOwnColumn(t *testing.T) {
	id := "sha256:" + strings.Repeat("ab", 32)
	table := "DATE  VALUE  COHORT      PERCENTILE  ID\n" +
		"2014  160    %s  -           " + id[7:19] + "\n"
	for cell, want := range map[string]string{"male-20-29": "male-20-29", "-         ": "-", "          ": ""} {
		if got, _ := cohortOfRow(t, strings.Replace(table, "%s", cell, 1), id); got != want {
			t.Errorf("the cell %q reads as %q, want %q", cell, got, want)
		}
	}
}

// TestComparePackFromRecords: with no -cohort, each row gets the cohort of
// the age on its own date, from person.sex and person.birth. The numbers
// are those of a run that names that cohort. Nothing is saved.
func TestComparePackFromRecords(t *testing.T) {
	dir, path, ids := personLog(t)
	sex := add(t, dir, "person.sex", "male")
	birth := add(t, dir, "person.birth", "1986-05")
	// The two readings of DECISIONS 345 (b): the same value, ten years apart.
	old, recent := cholesterol(t, dir, "160", "2014-06-01"), cholesterol(t, dir, "160", "2024-06-01")
	out, errs, code := run(t, dir, "", "compare", "-pack", path, "-type", "lab.*")
	if code != 0 {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	lines := strings.Split(out, "\n")
	// The line above the table names the two records by id, and says who
	// signed them. It does not say that the person stated or chose anything.
	want := "Cohort: each row's own, from the records person.sex (" + sex[7:19] + ") and person.birth (" + birth[7:19] + "), signed under your identity. To choose one, pass -cohort."
	if lines[1] != want {
		t.Errorf("the line above the table:\n%s\nwant:\n%s", lines[1], want)
	}
	// The ids and the path of the pack are random: they are no word of bean.
	said := strings.ReplaceAll(out, lines[2], "")
	for _, id := range []string{sex, birth, old, recent, ids[0], ids[1]} {
		said = strings.ReplaceAll(said, id[7:19], "")
	}
	for _, never := range []string{"you chose", "you stated", "You chose", "You stated", "1986", "male, age"} {
		if strings.Contains(said, never) {
			t.Errorf("the output says %q:\n%s", never, out)
		}
	}
	for id, cohort := range map[string]string{old: "male-20-29", recent: "male-30-39", ids[0]: "male-40-49", ids[1]: "male-40-49"} {
		got, row := cohortOfRow(t, out, id)
		if got != cohort {
			t.Errorf("the row of %s has the cohort %q, want %q:\n%s", id[7:19], got, cohort, row)
		}
		// The numbers are the ones of a run that names the cohort.
		named, errs, code := run(t, dir, "", "compare", "-pack", path, "-cohort", cohort, id)
		if code != 0 {
			t.Fatalf("-cohort %s: %d\n%s\n%s", cohort, code, named, errs)
		}
		if a, b := strings.Join(strings.Fields(row), " "), packRow(named, id); strings.Replace(a, " "+cohort+" ", " ", 1) != b {
			t.Errorf("by the records: %s\nby -cohort:     %s", a, b)
		}
	}
	// With -type, a reading of a type that the pack has no measure for is
	// named below the table, and has no row.
	if !strings.Contains(out, "No reference data for 1 other type: lab.tsh.\nThese types have reference data, each in this unit: lab.cholesterol-total (mg/dL), lab.hdl-cholesterol (mg/dL).\n") || strings.Contains(out, ids[2][7:19]) {
		t.Errorf("a type with no reference data, with -type:\n%s", out)
	}
	// The caveat on the cohort is the one of this mode, and the last line
	// says how to save.
	for _, want := range []string{"  - " + caveatFromRecords, notSavedFromRecords, "Compared 4 of 5 selected readings.", percentileMeans} {
		if !slices0(lines, want) {
			t.Errorf("no line %q in\n%s", want, out)
		}
	}
	// A reading that the pack has no measure for was set against no
	// cohort, and its row shows none: another unit, and another type.
	otherUnit := add(t, dir, "lab.cholesterol-total", "4.9", "-unit", "mmol/L", "-at", "2024-06-01")
	words := `{"type":"lab.cholesterol-total","times":{"observed":{"start":"2024-06-01T00:00:00Z","precision":"day"}},"value":{"s":"see the report","unit":"mg/dL"},"source":{"system":"self","adapter":"synthetic@1","ref":null}}`
	if _, errs, code := run(t, dir, words+"\n", "append"); code != 0 {
		t.Fatal(errs)
	}
	ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "lab.cholesterol-total")
	notNumber := ""
	for _, line := range strings.Split(ls, "\n") {
		if strings.Contains(line, "see the report") {
			notNumber = line[strings.Index(line, `"id":"`)+6:][:71]
		}
	}
	shown, _, _ := run(t, dir, "", "compare", "-pack", path, otherUnit, ids[2], recent, notNumber)
	for id, want := range map[string]string{otherUnit: "-", ids[2]: "-", notNumber: "-", recent: "male-30-39"} {
		if got, row := cohortOfRow(t, shown, id); got != want {
			t.Errorf("the cohort of a row is %q, want %q:\n%s", got, want, row)
		}
	}
	if lines[len(lines)-2] != notSavedFromRecords {
		t.Errorf("the last line is %q", lines[len(lines)-2])
	}
	if ls, _, _ := run(t, dir, "", "ls", "-all", "-type", "comparison.*"); ls != "" {
		t.Errorf("a run with no -cohort saved a comparison:\n%s", ls)
	}
	// Record ids beside -type: with a pack on record, bean says that.
	if _, errs, code := run(t, dir, "", "compare", "-pack", path, "-type", "lab.*", old); code != 2 || !strings.Contains(errs, "say which readings to compare: record ids, or -type, and not both") {
		t.Errorf("ids beside -type: %d\n%s", code, errs)
	}
	// -unknown-pack changes nothing here: the pack is one on record.
	plain, _, _ := run(t, dir, "", "compare", "-pack", path, "-type", "lab.*")
	if same, _, code := run(t, dir, "", "compare", "-pack", path, "-unknown-pack", "-type", "lab.*"); code != 0 || same != plain || !strings.Contains(same, "male-20-29") {
		t.Errorf("-unknown-pack with a pack on record: %d\n%s", code, same)
	}
	// -save needs -cohort, as before, and writes nothing without it.
	out, errs, code = run(t, dir, "", "compare", "-pack", path, "-type", "lab.*", "-save")
	if code != 2 || !strings.Contains(errs, "name a cohort of the pack with -cohort: female-20-29") || !strings.Contains(errs, "-save keeps a comparison only with a cohort that you name: run once for each cohort, with the ids of its readings") || out != "" {
		t.Errorf("-save with no -cohort: %d\n%s\n%s", code, out, errs)
	}
	if ls, _, _ := run(t, dir, "", "ls", "-all", "-type", "comparison.*"); ls != "" {
		t.Errorf("-save with no -cohort saved a comparison:\n%s", ls)
	}
	// With -cohort the table has no COHORT column, and the caveat and the
	// claim are the ones of a cohort that the person chose.
	out, _, code = run(t, dir, "", "compare", "-pack", path, "-cohort", "male-30-39", "-save", old)
	if code != 0 || strings.Contains(out, "COHORT") || strings.Contains(out, "person.") || !strings.Contains(out, "  - you chose the cohort: bean does not know whether you belong to it") || !strings.Contains(out, `Cohort: "male, age 30-39".`) {
		t.Errorf("-cohort: %d\n%s", code, out)
	}
	if ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "comparison.*"); !strings.Contains(ls, `"cohort":"male-30-39"`) || !strings.Contains(ls, "you chose the cohort") || strings.Contains(ls, "follows from") {
		t.Errorf("the saved claim:\n%s", ls)
	}
	// The way with the files keeps -sex and -age: it reads neither record.
	if _, errs, code := run(t, dir, "", "compare", "-unknown-files", old); code != 2 || !strings.Contains(errs, "usage") && !strings.Contains(errs, "give -sex") {
		t.Errorf("the files with no -sex: %d\n%s", code, errs)
	}
}

func slices0(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

// TestComparePackRowsWithNoCohort: a reading that gets no cohort has no
// number, and its row says what the person can do. No row names an age.
func TestComparePackRowsWithNoCohort(t *testing.T) {
	dir, path, _ := personLog(t)
	add(t, dir, "person.sex", "female")
	year := add(t, dir, "person.birth", "1986")
	const (
		noDate   = "not comparable: the reading has no date: name a cohort with -cohort"
		after    = "not comparable: the reading is dated after now: correct its date, or name a cohort with -cohort"
		before   = "not comparable: the reading is dated before the birth in person.birth: correct one. -cohort sets it against a cohort of another age"
		maybe    = "not comparable: the reading may be dated before the birth in person.birth: correct one. -cohort sets it against a cohort of another age"
		none     = "not comparable: the pack has no cohort for the age on that date: -cohort sets it against a cohort of another age"
		edge     = "not comparable: the date lies on the edge of two cohorts: name one with -cohort"
		menuEdge = "not comparable: the date lies on the edge of the pack's cohorts: name one with -cohort if it holds the age on that date"
		month    = ", or correct person.birth to a year and a month"
	)
	undated := `{"type":"lab.cholesterol-total","times":{"observed":null},"value":{"n":160,"unit":"mg/dL"},"source":{"system":"self","adapter":"synthetic@1","ref":null}}`
	wholeYear := `{"type":"lab.cholesterol-total","times":{"observed":{"start":"2015-01-01T00:00:00Z","precision":"year"}},"value":{"n":161,"unit":"mg/dL"},"source":{"system":"self","adapter":"synthetic@1","ref":null}}`
	edgeYear := `{"type":"lab.cholesterol-total","times":{"observed":{"start":"2016-01-01T00:00:00Z","precision":"year"}},"value":{"n":162,"unit":"mg/dL"},"source":{"system":"self","adapter":"synthetic@1","ref":null}}`
	appended, errs, code := run(t, dir, undated+"\n"+wholeYear+"\n"+edgeYear+"\n", "append")
	if code != 0 {
		t.Fatalf("append: %s", errs)
	}
	var appendedIDs []string
	for _, line := range strings.Split(appended, "\n") {
		for _, word := range strings.Fields(line) {
			if strings.HasPrefix(word, "sha256:") {
				appendedIDs = append(appendedIDs, word)
			}
		}
	}
	if len(appendedIDs) != 3 {
		// append prints a count: find the three by value.
		appendedIDs = nil
		ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "lab.cholesterol-total")
		for _, n := range []string{`"n":160,`, `"n":161,`, `"n":162,`} {
			for _, line := range strings.Split(ls, "\n") {
				if strings.Contains(line, n) {
					appendedIDs = append(appendedIDs, line[strings.Index(line, `"id":"`)+6:][:71])
				}
			}
		}
	}
	if len(appendedIDs) != 3 {
		t.Fatalf("the appended readings: %v", appendedIDs)
	}
	// With a birth of a year alone.
	rows := map[string]string{
		appendedIDs[0]:                           noDate,
		appendedIDs[1]:                           "",   // all of 2015: 28 or 29
		appendedIDs[2]:                           edge, // all of 2016: 29 or 30, whatever the month of birth: no advice of a month
		cholesterol(t, dir, "160", "2999-01-01"): after,
		cholesterol(t, dir, "160", "1985-12-31"): before,
		cholesterol(t, dir, "160", "1986-06-01"): maybe,            // the year of the birth: no age is sure
		cholesterol(t, dir, "160", "1987-06-01"): none,             // 0 or 1
		cholesterol(t, dir, "160", "2004-06-01"): none,             // 17 or 18
		cholesterol(t, dir, "160", "2006-06-01"): menuEdge + month, // 19 or 20
		cholesterol(t, dir, "160", "2007-01-01"): "",               // 20 or 21
		cholesterol(t, dir, "160", "2015-12-31"): "",               // 28 or 29
		cholesterol(t, dir, "160", "2016-01-01"): edge + month,
		cholesterol(t, dir, "160", "2017-01-01"): "", // 30 or 31
	}
	check := func(rows map[string]string, sex string) {
		t.Helper()
		out, errs, code := run(t, dir, "", "compare", "-pack", path, "-type", "lab.cholesterol-total")
		if code != 0 {
			t.Fatalf("%d\n%s\n%s", code, out, errs)
		}
		for id, why := range rows {
			cohort, row := cohortOfRow(t, out, id)
			switch {
			case why == "" && !strings.HasPrefix(cohort, sex+"-"):
				t.Errorf("no cohort in the row:\n%s", row)
			case why != "" && (cohort != "-" || !strings.Contains(row, "  "+why+"  ")):
				t.Errorf("want %q in the row:\n%s", why, row)
			}
		}
		// No line names an age, and none names the value of a record.
		for _, line := range strings.Split(out, "\n") {
			// The last field of a row is its id, which is random.
			fields := strings.Fields(line)
			if strings.Contains(line, "not comparable") && strings.ContainsAny(strings.SplitN(strings.Join(fields[:len(fields)-1], " "), "not comparable", 2)[1], "0123456789") {
				t.Errorf("a refusal with a number in it:\n%s", line)
			}
		}
	}
	check(rows, "female")
	// With the month of birth, a date on the edge is the month of the
	// birthday alone, and the advice of a month is gone.
	byMonth := add(t, dir, "person.birth", "1986-05", "-corrects", year)
	check(map[string]string{
		appendedIDs[1]:                           "",
		appendedIDs[2]:                           edge, // all of 2016: 29 or 30, and the month does not help
		cholesterol(t, dir, "160", "2016-04-30"): "",
		cholesterol(t, dir, "160", "2016-05-01"): edge,
		cholesterol(t, dir, "160", "2016-05-31"): edge,
		// The month is the one of the date as written, whatever the zone
		// of the machine that shows it.
		cholesterol(t, dir, "160", "2016-05-31T23:30:00-05:00"): edge,
		cholesterol(t, dir, "160", "2016-06-01T00:30:00+02:00"): "",
		cholesterol(t, dir, "160", "2016-06-01"):                "",
		cholesterol(t, dir, "160", "2006-05-15"):                menuEdge,
		cholesterol(t, dir, "160", "1986-04-30"):                before,
		cholesterol(t, dir, "160", "1986-05-01"):                maybe,
		cholesterol(t, dir, "160", "1986-06-01"):                none,
	}, "female")
	// A cohort that the pack has no table for: the row names the cohort
	// that it means, and has no number.
	early := add(t, dir, "person.birth", "1930", "-corrects", byMonth)
	old := cholesterol(t, dir, "160", "2020-06-15")
	out, _, _ := run(t, dir, "", "compare", "-pack", path, old)
	if cohort, row := cohortOfRow(t, out, old); cohort != "female-80-over" || !strings.Contains(row, "not comparable: the pack has no table of this measure for this cohort") {
		t.Errorf("a cohort with no table:\n%s", out)
	}
	add(t, dir, "person.birth", "1986-05", "-corrects", early)
	// A run in which no reading gets a cohort compares nothing.
	lone := cholesterol(t, dir, "160", "2016-05-15")
	if out, errs, code := run(t, dir, "", "compare", "-pack", path, lone); code != 1 || !strings.Contains(errs, "nothing could be compared") || strings.Contains(out, notSavedFromRecords) {
		t.Errorf("only a reading on an edge: %d\n%s\n%s", code, out, errs)
	}
	// A reading that is dated after now, by the clock of the run.
	// Both are instants, so that the zone of the machine does not matter.
	may := cholesterol(t, dir, "160", "2020-06-15T00:00:00Z")
	out, _, _ = runAt(t, dir, "2020-06-14T23:00:00Z", "compare", "-pack", path, may)
	if _, row := cohortOfRow(t, out, may); !strings.Contains(row, after) {
		t.Errorf("a reading after the clock of the run:\n%s", out)
	}
}

// TestComparePackStopsOnTheRecords: when the two records do not give one
// sex and one birth, bean stops. The stop names the type, the short ids
// and the rule, and no value. It names -cohort, and a correction only where
// one is the way out.
func TestComparePackStopsOnTheRecords(t *testing.T) {
	dir, path, ids := personLog(t)
	compare := func(d string) (string, string, int) {
		return run(t, d, "", "compare", "-pack", path, ids[0])
	}
	const cohort = "name a cohort of the pack with -cohort: female-20-29, "
	// No record: the words of before come first. Then it names the two
	// records and where their rules are. It has no command to copy: whoever
	// runs this may be the person's assistant, and the two are optional.
	out, errs, code := compare(dir)
	if code != 2 || out != "" || !strings.HasPrefix(errs, "bean: "+cohort) || !strings.Contains(errs, "The log has no current person.sex and no person.birth record that counts. Both are optional, and -cohort needs neither. docs/REFERENCE.md, \"The cohort from two records\", has the rules and says who can read them") || strings.Contains(errs, "bean add") || strings.Contains(errs, "does not count") {
		t.Errorf("no record: %d\n%s\n%s", code, out, errs)
	}
	// A record that was deleted is not a record that bean does not read.
	gone := t.TempDir()
	if err := os.CopyFS(gone, os.DirFS(dir)); err != nil {
		t.Fatal(err)
	}
	run(t, gone, "", "purge", add(t, gone, "person.sex", "male"))
	// Nor is one that a record of another type replaced: it is not current.
	add(t, gone, "note", "not for the log", "-corrects", add(t, gone, "person.birth", "1986"))
	if _, errs, code := compare(gone); code != 2 || !strings.Contains(errs, "no current person.sex and no person.birth") || strings.Contains(errs, "does not count") {
		t.Errorf("a deleted record: %d\n%s", code, errs)
	}
	type stop struct {
		// what the log holds of each type, as arguments of bean add
		sex, birth [][]string
		code       int
		want       []string
		never      []string
	}
	twoValues := "do not state one value. Correct one with bean add %s <value> -corrects <id>, or delete one with bean purge <id>. Or " + cohort
	wrongForm := "is not a year of four digits with no unit, or a year and a month as 1986-05. Correct it with bean add person.birth <value> -corrects <id>. Or " + cohort
	for name, tc := range map[string]stop{
		"no birth": {sex: [][]string{{"male"}}, code: 2, want: []string{cohort, "The log has no current person.birth record that counts."}, never: []string{"no current person.sex"}},
		"no sex":   {birth: [][]string{{"1986"}}, code: 2, want: []string{cohort, "The log has no current person.sex record that counts."}, never: []string{"no person.birth"}},
		// Two values of sex can both be true, each for its own dates: -cohort
		// comes before the correction.
		"two sexes":                  {sex: [][]string{{"male"}, {"female"}}, birth: [][]string{{"1986"}}, code: 1, want: []string{"the records person.sex (", "do not state one value, and bean takes one value for every date. If each holds for its own dates, " + cohort, ". If one is wrong, correct it with bean add person.sex <value> -corrects <id>, or delete it with bean purge <id>"}},
		"two births":                 {sex: [][]string{{"male"}}, birth: [][]string{{"1986"}, {"1987"}}, code: 1, want: []string{"the records person.birth (", strings.Replace(twoValues, "%s", "person.birth", 1)}},
		"a year and its month":       {sex: [][]string{{"male"}}, birth: [][]string{{"1986"}, {"1986-05"}}, code: 1, want: []string{"the records person.birth ("}},
		"a birth and one in no form": {sex: [][]string{{"male"}}, birth: [][]string{{"1986"}, {"spring"}}, code: 1, want: []string{"the records person.birth (", "do not state one value"}},
		"two births in no form":      {sex: [][]string{{"male"}}, birth: [][]string{{"spring"}, {"autumn"}}, code: 1, want: []string{"the records person.birth (", "do not state one value"}},
		"a birth in words":           {sex: [][]string{{"male"}}, birth: [][]string{{"May 1986"}}, code: 1, want: []string{"the record person.birth (", wrongForm}},
		"a birth of two digits":      {sex: [][]string{{"male"}}, birth: [][]string{{"86"}}, code: 1, want: []string{wrongForm}},
		"a birth of five digits":     {sex: [][]string{{"male"}}, birth: [][]string{{"19860"}}, code: 1, want: []string{wrongForm}},
		"a birth with a day":         {sex: [][]string{{"male"}}, birth: [][]string{{"1986-05-17"}}, code: 1, want: []string{wrongForm}},
		"a birth with a month 13":    {sex: [][]string{{"male"}}, birth: [][]string{{"1986-13"}}, code: 1, want: []string{wrongForm}},
		"a birth with a month 0":     {sex: [][]string{{"male"}}, birth: [][]string{{"1986-00"}}, code: 1, want: []string{wrongForm}},
		"a birth with a short month": {sex: [][]string{{"male"}}, birth: [][]string{{"1986-5"}}, code: 1, want: []string{wrongForm}},
		"a birth with a fraction":    {sex: [][]string{{"male"}}, birth: [][]string{{"1986.5"}}, code: 1, want: []string{wrongForm}},
		"a birth with a unit":        {sex: [][]string{{"male"}}, birth: [][]string{{"1986", "-unit", "a"}}, code: 1, want: []string{wrongForm}},
		"a birth after now":          {sex: [][]string{{"male"}}, birth: [][]string{{"2999"}}, code: 1, want: []string{"the record person.birth (", "is after now. Correct it with bean add person.birth <value> -corrects <id>. Or " + cohort}},
		// For a word that the pack does not have, -cohort alone: nobody is
		// asked to write what is not true.
		"another word of sex":                      {sex: [][]string{{"intersex"}}, birth: [][]string{{"1986"}}, code: 1, want: []string{"the record person.sex (", "is not exactly male or female, the two groups that this pack has. Name a cohort of the pack with -cohort: female-20-29, "}, never: []string{"orrect", "purge", "bean add"}},
		"a sex in capitals":                        {sex: [][]string{{"Male"}}, birth: [][]string{{"1986"}}, code: 1, want: []string{"is not exactly male or female"}, never: []string{"orrect"}},
		"a sex that is a number":                   {sex: [][]string{{"1"}}, birth: [][]string{{"1986"}}, code: 1, want: []string{"is not exactly male or female"}},
		"a sex that is yes":                        {sex: [][]string{{"yes"}}, birth: [][]string{{"1986"}}, code: 1, want: []string{"is not exactly male or female"}},
		"the same word twice, and not of the pack": {sex: [][]string{{"intersex"}, {"intersex"}}, birth: [][]string{{"1986"}}, code: 1, want: []string{"is not exactly male or female"}, never: []string{"orrect"}},
	} {
		d := t.TempDir()
		if err := os.CopyFS(d, os.DirFS(dir)); err != nil {
			t.Fatal(err)
		}
		var recordIDs, values []string
		for typ, list := range map[string][][]string{"person.sex": tc.sex, "person.birth": tc.birth} {
			for _, args := range list {
				recordIDs = append(recordIDs, add(t, d, append([]string{typ}, args...)...)[7:19])
				values = append(values, args[0])
			}
		}
		out, errs, code := compare(d)
		if code != tc.code || out != "" {
			t.Errorf("%s: exit %d, want %d\n%s\n%s", name, code, tc.code, out, errs)
		}
		for _, want := range tc.want {
			if !strings.Contains(errs, want) {
				t.Errorf("%s: no %q in\n%s", name, want, errs)
			}
		}
		for _, never := range tc.never {
			if strings.Contains(errs, never) {
				t.Errorf("%s: %q in\n%s", name, never, errs)
			}
		}
		// A stop on what a record holds names each record of the type by
		// its short id, and no value.
		if tc.code == 1 {
			// The first sentence is the stop. The menu of the pack follows it.
			first := strings.SplitN(errs, ". ", 2)[0]
			named := 0
			for _, id := range recordIDs {
				if strings.Contains(first, id) {
					named++
				}
			}
			// An id is random, and can hold the digits of a value by chance.
			said := first
			for _, id := range recordIDs {
				said = strings.ReplaceAll(said, id, "")
			}
			if strings.Contains(first, "person.sex (") && named != len(tc.sex) || strings.Contains(first, "person.birth (") && named != len(tc.birth) {
				t.Errorf("%s: the stop does not name each record by id:\n%s", name, first)
			}
			for _, v := range values {
				// "as 1986-05" is bean's own example of the form.
				if v != "1" && v != "yes" && strings.Contains(strings.ReplaceAll(said, "as 1986-05", ""), v) {
					t.Errorf("%s: the stop holds the value %q:\n%s", name, v, first)
				}
			}
		}
		// -cohort always wins: bean then reads neither record.
		if out, errs, code := run(t, d, "", "compare", "-pack", path, "-cohort", "male-30-39", ids[0]); code != 0 || strings.Contains(out+errs, "person.") {
			t.Errorf("%s: -cohort: %d\n%s\n%s", name, code, out, errs)
		}
	}
}

// TestWhichPersonRecordsCount: current records that the person's identity
// signed about the person, with the source self and no ref. The number
// 1986 and the text 1986 are one value. A record that was corrected or
// deleted is not read.
func TestWhichPersonRecordsCount(t *testing.T) {
	dir, path, ids := personLog(t)
	compare := func() (string, string, int) { return run(t, dir, "", "compare", "-pack", path, ids[0]) }
	// With only an agent's records, bean says that the log has a record
	// that it does not read. bean ls shows it, so the person must know why.
	if _, errs, code := run(t, dir, "", "agent", "init", "-label", "assistant"); code != 0 {
		t.Fatal(errs)
	}
	add(t, dir, "-agent", "person.sex", "male")
	// The record that this is said of was written with bean add -agent: the
	// words name no command, and nothing that an assistant can do.
	const notRead = "What it does hold does not count. No record of an agent or of a peer counts, and none about another person, with another source or with a ref."
	if _, errs, code := compare(); code != 2 || !strings.Contains(errs, "The log has no current person.sex and no person.birth record that counts. "+notRead+" Both are optional") || strings.Contains(errs, "bean add") {
		t.Fatalf("only an agent's record: %d\n%s", code, errs)
	}
	// That is said only of a type that is missing. The person's own sex
	// counts now: the agent's record of sex is no reason for the stop.
	sex := add(t, dir, "person.sex", "female")
	if _, errs, code := compare(); code != 2 || !strings.Contains(errs, "The log has no current person.birth record that counts. Both are optional") {
		t.Fatalf("an agent's sex beside the person's, and no birth: %d\n%s", code, errs)
	}
	add(t, dir, "-agent", "person.birth", "1950")
	if _, errs, code := compare(); code != 2 || !strings.Contains(errs, "The log has no current person.birth record that counts. "+notRead) {
		t.Fatalf("an agent's birth beside the person's sex: %d\n%s", code, errs)
	}
	number := add(t, dir, "person.birth", "1990")
	out, errs, code := compare()
	if cohort, _ := cohortOfRow(t, out, ids[0]); code != 0 || cohort != "female-30-39" {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	// The same year as text: one value, and the line names both records.
	text := `{"type":"person.birth","times":{"observed":null},"value":{"s":"1990"},"source":{"system":"self","adapter":"synthetic@1","ref":null}}`
	if _, errs, code := run(t, dir, text+"\n", "append"); code != 0 {
		t.Fatal(errs)
	}
	out, errs, code = compare()
	if code != 0 || !strings.Contains(out, "person.birth ("+number[7:19]+", ") && !strings.Contains(out, ", "+number[7:19]+"), signed") {
		t.Fatalf("a number and a text of one year: %d\n%s\n%s", code, out, errs)
	}
	// A number and a text of one year are one value for a birth only. As
	// two records of sex they are two values, and the stop says so.
	two := t.TempDir()
	if err := os.CopyFS(two, os.DirFS(dir)); err != nil {
		t.Fatal(err)
	}
	run(t, two, "", "purge", sex)
	add(t, two, "person.sex", "1990")
	if _, errs, code := run(t, two, strings.Replace(text, "person.birth", "person.sex", 1)+"\n", "append"); code != 0 {
		t.Fatal(errs)
	}
	if _, errs, code := run(t, two, "", "compare", "-pack", path, ids[0]); code != 1 || !strings.Contains(errs, "the records person.sex (") || !strings.Contains(errs, "do not state one value") {
		t.Fatalf("a number and a text as two records of sex: %d\n%s", code, errs)
	}
	// Records that do not count, each with another value. None of them
	// stops the run, moves the cohort or is named above the table.
	before := strings.Split(out, "\n")[1]
	for _, other := range []string{
		// another source system, as an importer writes
		`{"type":"person.sex","times":{"observed":null},"value":{"s":"male"},"source":{"system":"quest","adapter":"synthetic@1","ref":null}}`,
		`{"type":"person.birth","times":{"observed":null},"value":{"n":1950},"source":{"system":"apple-health","adapter":"synthetic@1","ref":null}}`,
		// about someone else
		`{"type":"person.sex","subject":"sha256:` + strings.Repeat("7", 64) + `","times":{"observed":null},"value":{"s":"male"},"source":{"system":"self","adapter":"synthetic@1","ref":null}}`,
		`{"type":"person.birth","subject":"sha256:` + strings.Repeat("7", 64) + `","times":{"observed":null},"value":{"n":1950},"source":{"system":"self","adapter":"synthetic@1","ref":null}}`,
		// the source self with a ref, as an import of a file writes
		`{"type":"person.sex","times":{"observed":null},"value":{"s":"male"},"source":{"system":"self","adapter":"synthetic@1","ref":"hmac-sha256:` + strings.Repeat("1", 64) + `"}}`,
		`{"type":"person.birth","times":{"observed":null},"value":{"n":1950},"source":{"system":"self","adapter":"synthetic@1","ref":"hmac-sha256:` + strings.Repeat("2", 64) + `"}}`,
	} {
		if _, errs, code := run(t, dir, other+"\n", "append"); code != 0 {
			t.Fatalf("append %s: %s", other, errs)
		}
	}
	out, errs, code = compare()
	if cohort, _ := cohortOfRow(t, out, ids[0]); code != 0 || cohort != "female-30-39" || strings.Split(out, "\n")[1] != before {
		t.Fatalf("records that do not count were read: %d\n%s\n%s", code, out, errs)
	}
	// A correction replaces a record: the old value is not read, and no
	// two values stop the run.
	corrected := add(t, dir, "person.sex", "male", "-corrects", sex)
	out, errs, code = compare()
	if cohort, _ := cohortOfRow(t, out, ids[0]); code != 0 || cohort != "male-30-39" || !strings.Contains(out, "person.sex ("+corrected[7:19]+")") || strings.Contains(out, sex[7:19]) {
		t.Fatalf("after a correction: %d\n%s\n%s", code, out, errs)
	}
	// A deleted correction brings back the record that it replaced, as
	// bean ls shows. bean reads what the listing shows as current.
	if _, errs, code := run(t, dir, "", "purge", corrected); code != 0 {
		t.Fatal(errs)
	}
	out, errs, code = compare()
	if cohort, _ := cohortOfRow(t, out, ids[0]); code != 0 || cohort != "female-30-39" || !strings.Contains(out, "person.sex ("+sex[7:19]+")") {
		t.Fatalf("after the correction is deleted: %d\n%s\n%s", code, out, errs)
	}
	// A deleted record is not read: the person is back at no record.
	if _, errs, code := run(t, dir, "", "purge", sex); code != 0 {
		t.Fatal(errs)
	}
	if _, errs, code := compare(); code != 2 || !strings.Contains(errs, "The log has no current person.sex record that counts. "+notRead) {
		t.Fatalf("after a deletion: %d\n%s", code, errs)
	}
}

// TestComparePackFromRecordsNeedsAPackOnRecord: bean knows the sex and the
// ages of a cohort only for a pack on record. For another pack the words
// of a cohort are its builder's, so the person names the cohort.
func TestComparePackFromRecordsNeedsAPackOnRecord(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "HDL_J", "DEMO_J")
	path, _ := testPack(t)
	add(t, dir, "person.sex", "male")
	add(t, dir, "person.birth", "1986-05")
	// The last has record ids beside -type: for a pack that is not on
	// record, the first error is the one of before.
	for _, args := range [][]string{{"compare", "-pack", path, ids[0]}, {"compare", "-pack", path, "-unknown-pack", ids[0]}, {"compare", "-pack", path, "-unknown-pack", "-type", "lab.*"}, {"compare", "-pack", path, "-type", "lab.*", ids[0]}} {
		out, errs, code := run(t, dir, "", args...)
		if code != 2 || out != "" || !strings.Contains(errs, "name a cohort of the pack with -cohort: female-20-29, ") || !strings.Contains(errs, "bean takes a cohort from your records only with a pack that it has on record") {
			t.Errorf("%v: %d\n%s\n%s", args, code, out, errs)
		}
	}
}

// TestCohortCaveatNamesItsCohort: the rows of one run can differ in
// cohort, so a caveat that the pack states for one cohort names it.
func TestCohortCaveatNamesItsCohort(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "HDL_J", "DEMO_J")
	path, digest, _ := editedPack(t, func(body map[string]any) {
		c := body["cohorts"].(map[string]any)["male-40-49"].(map[string]any)
		c["caveats"] = []any{"a word of this cohort", "a second word of it"}
	})
	t.Setenv("BEAN_TEST_PACK_ON_RECORD", digest)
	add(t, dir, "person.sex", "male")
	add(t, dir, "person.birth", "1986-05")
	old := cholesterol(t, dir, "160", "2014-06-01")
	out, errs, code := run(t, dir, "", "compare", "-pack", path, ids[0], old)
	if code != 0 {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	lines := strings.Split(out, "\n")
	for _, want := range []string{
		`  - the pack says of male-40-49: "a word of this cohort"`,
		`  - the pack says of male-40-49: "a second word of it"`,
		`  - the pack says: "the lab that measured the reading and NHANES used their own instruments and methods"`,
		"  - " + caveatFromRecords,
	} {
		if !slices0(lines, want) {
			t.Errorf("no line %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "says of male-20-29") || strings.Count(out, "the pack says of ") != 2 {
		t.Errorf("a caveat names a cohort that does not state it:\n%s", out)
	}
	// With -cohort every row has the one cohort of the first line, and the
	// caveat is as before.
	out, _, _ = run(t, dir, "", "compare", "-pack", path, "-cohort", "male-40-49", ids[0])
	if !strings.Contains(out, `  - the pack says: "a word of this cohort"`) || strings.Contains(out, "says of") {
		t.Errorf("-cohort:\n%s", out)
	}
}

// TestPersonRecordsAreOrdinaryRecords: the two are listed and shared as
// any record. A share of lab readings or of a comparison does not bring
// them, and a share of everything does. The MCP tool does not write them:
// they are not in the vocabulary. Other tests correct and delete them.
func TestPersonRecordsAreOrdinaryRecords(t *testing.T) {
	dir, path, ids := personLog(t)
	sex, birth := add(t, dir, "person.sex", "male"), add(t, dir, "person.birth", "1986-05")
	if ls, _, _ := run(t, dir, "", "ls", "-type", "person.*"); !strings.Contains(ls, "person.sex") || !strings.Contains(ls, "person.birth") || !strings.Contains(ls, "1986-05") {
		t.Errorf("ls:\n%s", ls)
	}
	if out, errs, code := run(t, dir, "", "compare", "-pack", path, "-cohort", "male-40-49", "-save", ids[0]); code != 0 {
		t.Fatalf("-save: %d\n%s\n%s", code, out, errs)
	}
	holds := func(args ...string) string {
		t.Helper()
		b := filepath.Join(t.TempDir(), "b")
		if out, errs, code := run(t, dir, "", append([]string{"share", "-yes", "-out", b}, args...)...); code != 0 {
			t.Fatalf("share %v: %d\n%s\n%s", args, code, out, errs)
		}
		checked, errs, code := run(t, t.TempDir(), "", "check-bundle", b)
		if code != 0 || !strings.Contains(checked, "bundle ok") {
			t.Fatalf("check-bundle: %d\n%s\n%s", code, checked, errs)
		}
		return checked
	}
	for _, args := range [][]string{{"-type", "lab.*"}, {"-type", "comparison.*"}, {ids[0]}} {
		if got := holds(args...); strings.Contains(got, sex[7:19]) || strings.Contains(got, birth[7:19]) || strings.Contains(got, "person.") || !strings.Contains(got, ids[0][7:19]) {
			t.Errorf("a share of %v brings a person record, or not the reading:\n%s", args, got)
		}
	}
	if got := holds("-type", "*"); !strings.Contains(got, sex[7:19]) || !strings.Contains(got, birth[7:19]) || !strings.Contains(got, "person.sex") {
		t.Errorf("a share of everything leaves a person record out:\n%s", got)
	}
	// The MCP tool refuses both types.
	run(t, dir, "", "agent", "init", "-label", "assistant")
	for _, typ := range []string{"person.sex", "person.birth"} {
		call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"log_entry","arguments":{"type":"` + typ + `","value":"1986"}}}` + "\n"
		out, _, _ := run(t, dir, call, "mcp", "-write", "-label", "assistant")
		if !strings.Contains(out, `"isError":true`) {
			t.Errorf("%s through the MCP tool:\n%s", typ, out)
		}
	}
	if ls, _, _ := run(t, dir, "", "ls", "-all", "-json", "-type", "person.*"); strings.Count(ls, "\n") != 2 {
		t.Errorf("the MCP tool wrote a person record:\n%s", ls)
	}
}

// TestComparePackFromRecordsOnAFrozenLog: a frozen log can leave a record
// out or show an old value. As every read does, this one prints, says so
// loudly and exits 4. It does not end as a run that went well.
func TestComparePackFromRecordsOnAFrozenLog(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	out, _, _ := run(t, a, "", "init")
	words := wordsFrom(out)
	path, digest := testPack(t)
	t.Setenv("BEAN_TEST_PACK_ON_RECORD", digest)
	add(t, a, "person.sex", "male")
	add(t, a, "person.birth", "1986-05")
	reading := cholesterol(t, a, "160", "2014-06-01")
	if _, errs, code := run(t, b, "", "sync", a); code != 0 {
		t.Fatalf("sync: %d %s", code, errs)
	}
	if _, errs, code := run(t, a, words, "rotate", "-seal-all"); code != 0 {
		t.Fatalf("rotate: %d %s", code, errs)
	}
	if _, errs, code := run(t, b, words, "restore", "-seal-all"); code != 0 {
		t.Fatalf("restore: %d %s", code, errs)
	}
	run(t, a, "", "sync", b)
	out, errs, code := run(t, a, "", "compare", "-pack", path, reading)
	if code != 4 || !strings.Contains(errs, "bean: compromised:") || strings.Contains(out, notSavedFromRecords) {
		t.Errorf("a frozen log: %d\n%s\n%s", code, out, errs)
	}
}

// With BEAN_NHANES_DIR set to a folder of CDC's own files: the pack of the
// six files is one that bean has on record, with no hook of a test. The
// two readings of DECISIONS 345 (b), the same value ten years apart, each
// get the cohort of their own date. The numbers are statistics of the
// public files, and are the ones that 345 (b) states.
func TestRealPackTakesTheCohortFromTheRecords(t *testing.T) {
	data := os.Getenv("BEAN_NHANES_DIR")
	if data == "" {
		t.Skip("BEAN_NHANES_DIR is not set")
	}
	path, digest := packFrom(t, data, "DEMO_J", "TCHOL_J", "HDL_J", "GHB_J", "CBC_J", "BMX_J")
	if digest != "10555d6769421d4305cbcd4ff14960d8b966ddd829a50581a9584a60a84d29e2" {
		t.Fatalf("the pack of CDC's six files has the SHA-256 %s", digest)
	}
	dir := t.TempDir()
	run(t, dir, "", "init")
	add(t, dir, "person.sex", "male")
	add(t, dir, "person.birth", "1986")
	old, recent := cholesterol(t, dir, "160", "2014-06-01"), cholesterol(t, dir, "160", "2024-06-01")
	out, errs, code := run(t, dir, "", "compare", "-pack", path, "-type", "lab.*")
	if code != 0 || !strings.Contains(out, "This is the NHANES 2017-2018 pack that bean has on record.") {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	for id, want := range map[string][2]string{old: {"male-20-29", "46.7"}, recent: {"male-30-39", "18.9"}} {
		cohort, row := cohortOfRow(t, out, id)
		if fields := strings.Fields(row); cohort != want[0] || len(fields) < 6 || fields[5] != want[1] {
			t.Errorf("want the cohort %s and the percentile %s in the row:\n%s", want[0], want[1], row)
		}
	}
	// One run with -cohort sets both against one cohort: what 345 (b) mends.
	out, _, _ = run(t, dir, "", "compare", "-pack", path, "-cohort", "male-30-39", "-type", "lab.*")
	for _, id := range []string{old, recent} {
		if fields := strings.Fields(packRow(out, id)); len(fields) < 5 || fields[4] != "18.9" {
			t.Errorf("with -cohort male-30-39, want 18.9 in the row: %s", packRow(out, id))
		}
	}
}

// TestComparePackFromRecordsDeletesAStaleComparison: as every compare with
// readings, a run with no -cohort first deletes the saved comparisons of a
// reading that was corrected or deleted. It saves no comparison, and it is
// not a run that writes nothing. It does so also when it then stops for
// the records. A correction that another tool copied into the log is the
// case: no command of bean has seen it yet.
func TestComparePackFromRecordsDeletesAStaleComparison(t *testing.T) {
	dir, path, ids := personLog(t)
	if out, errs, code := run(t, dir, "", "compare", "-pack", path, "-cohort", "male-40-49", "-save", ids[0], ids[1]); code != 0 {
		t.Fatalf("-save: %d\n%s\n%s", code, out, errs)
	}
	count := func(d string) int {
		ls, _, _ := run(t, d, "", "ls", "-type", "comparison.percentile")
		return strings.Count(ls, "comparison.percentile")
	}
	// Each reading is corrected in a copy, and only the file of the
	// correction comes back.
	stale := func(reading, value string) {
		t.Helper()
		other := t.TempDir()
		if err := os.CopyFS(other, os.DirFS(dir)); err != nil {
			t.Fatal(err)
		}
		typ := map[string]string{ids[0]: "lab.cholesterol-total", ids[1]: "lab.hdl-cholesterol"}[reading]
		correction := add(t, other, typ, value, "-unit", "mg/dL", "-at", "2026-11-01", "-corrects", reading)
		data, err := os.ReadFile(filepath.Join(other, core.PathOf(correction)))
		if err != nil {
			t.Fatal(err)
		}
		to := filepath.Join(dir, core.PathOf(correction))
		if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(to, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stale(ids[0], "201")
	// No person record: the run stops as before, and has deleted first.
	out, errs, code := run(t, dir, "", "compare", "-pack", path, "-type", "lab.*")
	if code != 2 || out != "" || !strings.Contains(errs, "bean: deleted 1 saved comparisons: their readings were corrected or deleted") || !strings.Contains(errs, "name a cohort of the pack with -cohort") {
		t.Fatalf("a stop after a stale comparison: %d\n%s\n%s", code, out, errs)
	}
	// With both records: it deletes, prints the table and saves none.
	add(t, dir, "person.sex", "male")
	add(t, dir, "person.birth", "1986-05")
	stale(ids[1], "61")
	out, errs, code = run(t, dir, "", "compare", "-pack", path, "-type", "lab.*")
	if code != 0 || !strings.Contains(errs, "bean: deleted 1 saved comparisons") || !strings.Contains(out, notSavedFromRecords) || count(dir) != 0 {
		t.Fatalf("a table after a stale comparison: %d, %d comparisons\n%s\n%s", code, count(dir), out, errs)
	}
}
