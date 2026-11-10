package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/internal/nhanes"
)

const refData = "../../internal/nhanes/testdata/"

// compareLog is a log with the synthetic NHANES files and three readings.
// It returns the folder, the PATH with the importers, and the ids of a
// total cholesterol, an HDL and a TSH reading.
func compareLog(t *testing.T, files ...string) (dir, path string, ids [3]string) {
	t.Helper()
	path = importerOnPath(t)
	dir = t.TempDir()
	run(t, dir, "", "init")
	for _, f := range files {
		if out, errs, code := runPath(t, dir, path, "import", "-yes", "nhanes", refData+f+".XPT"); code != 0 || !strings.Contains(out, "kept reference.file") {
			t.Fatalf("import %s: %d\n%s\n%s", f, code, out, errs)
		}
	}
	for i, a := range [][]string{
		{"add", "lab.cholesterol-total", "193", "-unit", "mg/dL", "-at", "2026-11-01"},
		{"add", "lab.hdl-cholesterol", "52", "-unit", "mg/dL", "-at", "2026-11-01"},
		{"add", "lab.tsh", "2.1", "-unit", "uIU/mL", "-at", "2026-11-01"},
	} {
		out, errs, code := run(t, dir, "", a...)
		if code != 0 {
			t.Fatalf("%v: %s", a, errs)
		}
		ids[i] = strings.TrimSpace(out)
	}
	return dir, path, ids
}

func TestCompare(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "HDL_J", "DEMO_J")
	out, errs, code := run(t, dir, "", "compare", "-unknown-files", "-sex", "male", "-age", "30-39", ids[0])
	if code != 0 {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	for _, want := range []string{
		"Compared with NHANES 2017-2018: male, age 30-39.", "lab.cholesterol-total", "193 mg/dL", "Read each number with these caveats:",
		"not whether it is healthy", "their own instruments and methods", "you chose the cohort",
		"this copy of TCHOL_J is not the file on record", "this copy of DEMO_J is not the file on record", "small cohort, file not on record",
		"Not saved. Pass -save",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in\n%s", want, out)
		}
	}
	// Nothing was written without -save.
	if ls, _, _ := run(t, dir, "", "ls", "-type", "comparison.percentile"); ls != "" {
		t.Errorf("a comparison was saved:\n%s", ls)
	}
	// The same command prints the same bytes.
	if again, _, _ := run(t, dir, "", "compare", "-unknown-files", "-sex", "male", "-age", "30-39", ids[0]); again != out {
		t.Errorf("two runs differ:\n%s\n%s", out, again)
	}
	// -type compares what it can and counts the rest.
	out, _, code = run(t, dir, "", "compare", "-unknown-files", "-sex", "female", "-age", "20-59", "-type", "lab.*")
	if code != 0 || !strings.Contains(out, "lab.hdl-cholesterol") || !strings.Contains(out, "No reference data for 1 other types: lab.tsh.") {
		t.Errorf("-type: %d\n%s", code, out)
	}
	// An id that cannot be compared is a row that says why, and exit 1 when
	// nothing could be.
	out, errs, code = run(t, dir, "", "compare", "-unknown-files", "-sex", "female", "-age", "20-59", ids[2])
	if code != 1 || !strings.Contains(out, "not comparable: no reference data for this type") || !strings.Contains(errs, "nothing could be compared") {
		t.Errorf("tsh: %d\n%s\n%s", code, out, errs)
	}
}

func TestCompareSave(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "HDL_J", "DEMO_J", "BMX_J", "DR1TOT_J")
	args := []string{"compare", "-unknown-files", "-sex", "female", "-age", "20-80", "-where", "DEMO_J.RIDRETH3=3-4", "-where", "DR1TOT_J.DR1TFIBE=5-40", "-save", ids[0], ids[1]}
	out, errs, code := run(t, dir, "", args...)
	if code != 0 || !strings.Contains(out, "added 2, already there 0") || !strings.Contains(out, "states the cohort you chose") || !strings.Contains(out, "one day") || !strings.Contains(out, "NHANES records every age of 80 and over as 80") {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	// Again: the same inputs are already there.
	if out, _, code := run(t, dir, "", args...); code != 0 || !strings.Contains(out, "added 0, already there 2") {
		t.Errorf("again: %d\n%s", code, out)
	}
	// Another cohort is another comparison.
	if out, _, code := run(t, dir, "", "compare", "-unknown-files", "-sex", "female", "-age", "20-79", "-save", ids[0]); code != 0 || !strings.Contains(out, "added 1, already there 0") {
		t.Errorf("another cohort: %d\n%s", code, out)
	}
	ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "comparison.percentile")
	lines := strings.Split(strings.TrimSpace(ls), "\n")
	if len(lines) != 3 {
		t.Fatalf("%d comparisons:\n%s", len(lines), ls)
	}
	var found bool
	for _, line := range lines {
		var r struct {
			ID    string         `json:"id"`
			Value map[string]any `json:"value"`
			Times struct {
				Observed map[string]any `json:"observed"`
			} `json:"times"`
			Refs    []map[string]string `json:"refs"`
			Context struct {
				Caveats   []string `json:"caveats"`
				Reading   string   `json:"reading"`
				Reference struct {
					Variable, Label, Dataset string
					Weight                   map[string]string
					Files                    []map[string]any
				} `json:"reference"`
				Cohort struct {
					Text  string
					Rules []map[string]any
				} `json:"cohort"`
				Method map[string]any     `json:"method"`
				Result map[string]float64 `json:"result"`
			} `json:"context"`
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatal(err)
		}
		c := r.Context
		if c.Reading != ids[0] || len(c.Cohort.Rules) != 4 {
			continue
		}
		found = true
		if len(c.Caveats) < 5 || r.Value["unit"] != "{percentile}" || r.Value["n"] != c.Result["percentile"] {
			t.Errorf("caveats or value: %s", line)
		}
		if c.Reference.Variable != "LBXTC" || c.Reference.Label != "Total Cholesterol (mg/dL)" || c.Reference.Dataset != "TCHOL_J" {
			t.Errorf("reference: %+v", c.Reference)
		}
		// A diet rule brings the day one weight.
		if c.Reference.Weight["dataset"] != "DR1TOT_J" || c.Reference.Weight["variable"] != "WTDRD1" {
			t.Errorf("weight: %v", c.Reference.Weight)
		}
		if c.Cohort.Text != "female, age 20 and over, DEMO_J.RIDRETH3=3-4, DR1TOT_J.DR1TFIBE=5-40" || c.Cohort.Rules[3]["label"] != "Dietary fiber (gm)" || c.Cohort.Rules[2]["min"] != 3.0 || c.Cohort.Rules[2]["max"] != 4.0 {
			t.Errorf("cohort: %+v", c.Cohort)
		}
		if c.Method["id"] != "nhanes-weighted-percentile" || c.Method["version"] != "1" {
			t.Errorf("method: %v", c.Method)
		}
		// The claim is about the reading and each file it read, and names
		// each file by its SHA-256. It does not copy the reading's value.
		if len(r.Refs) != 4 || r.Refs[0]["id"] != ids[0] || len(c.Reference.Files) != 3 {
			t.Errorf("refs and files: %v %v", r.Refs, c.Reference.Files)
		}
		for _, f := range c.Reference.Files {
			if fileDigest(filepath.Join(refData, f["dataset"].(string)+".XPT")) != f["sha256"] || len(f) != 2 {
				t.Errorf("file: %v", f)
			}
		}
		if strings.Contains(line, `"n":193`) || strings.Contains(line, "mg/dL\",\"n\"") {
			t.Errorf("the claim copies the reading's value: %s", line)
		}
		if r.Times.Observed["precision"] != "day" || c.Result["n"] < 30 || len(c.Result) != 7 {
			t.Errorf("time or result: %s", line)
		}
	}
	if !found {
		t.Errorf("no comparison of the first reading with four rules:\n%s", ls)
	}
	// The log with its comparisons is valid, and a comparison shows in ls
	// and as a reply of the reading.
	if out, _, code := run(t, dir, "", "verify"); code != 0 || !strings.Contains(out, "0 invalid") {
		t.Errorf("verify: %d\n%s", code, out)
	}
	if show, _, _ := run(t, dir, "", "show", ids[0]); strings.Count(show, "about comparison.percentile") != 2 {
		t.Errorf("show:\n%s", show)
	}
}

func TestCompareRefuses(t *testing.T) {
	dir, path, ids := compareLog(t, "TCHOL_J", "DEMO_J")
	for name, c := range map[string]struct {
		args []string
		code int
		want string
	}{
		"no cohort":          {[]string{ids[0]}, 2, "not comparable without a cohort"},
		"no age":             {[]string{"-sex", "male", ids[0]}, 2, "not comparable without a cohort"},
		"bad sex":            {[]string{"-sex", "m", "-age", "30-39", ids[0]}, 2, "sex is male or female"},
		"one age":            {[]string{"-sex", "male", "-age", "35", ids[0]}, 2, "For someone of 35, try 30-39"},
		"age over 80":        {[]string{"-sex", "male", "-age", "70-90", ids[0]}, 2, "80 means 80 and over"},
		"age with a sign":    {[]string{"-sex", "male", "-age", "+30-39", ids[0]}, 2, "bad -age"},
		"bad rule":           {[]string{"-sex", "male", "-age", "30-39", "-where", "bmi=20-25", ids[0]}, 2, "bad -where"},
		"rule with a sign":   {[]string{"-sex", "male", "-age", "30-39", "-where", "BMX_J.BMXBMI=-5-25", ids[0]}, 2, "bad -where"},
		"unknown data set":   {[]string{"-sex", "male", "-age", "30-39", "-where", "SMQ_J.SMQ020=1", ids[0]}, 2, "no sample weight on record"},
		"a weight as a rule": {[]string{"-sex", "male", "-age", "30-39", "-where", "DEMO_J.WTMEC2YR=1-2", ids[0]}, 2, "survey's design"},
		"no reading":         {[]string{"-sex", "male", "-age", "30-39"}, 2, "usage"},
		"id and type":        {[]string{"-sex", "male", "-age", "30-39", "-type", "lab.*", ids[0]}, 2, "usage"},
		"unknown id":         {[]string{"-sex", "male", "-age", "30-39", "abcdef123456"}, 1, "bean:"},
		"no such type":       {[]string{"-sex", "male", "-age", "30-39", "-type", "cgm.*"}, 1, "no current reading of type"},
		// The reference file is not in the log: the row says where to get it.
		"file not kept": {[]string{"-sex", "male", "-age", "30-39", ids[1]}, 1, "run bean import nhanes"},
		// A rule's file is not in the log.
		"rule file not kept": {[]string{"-sex", "male", "-age", "30-39", "-where", "BMX_J.BMXBMI=20-25", ids[0]}, 1, "BMX_J.xpt"},
		// Too few people: no number.
		"small cohort": {[]string{"-sex", "male", "-age", "30-30", "-where", "DEMO_J.RIDRETH3=6", ids[0]}, 1, "bean gives no number for a cohort of fewer than 30 people"},
		// A variable the file does not have.
		"unknown variable": {[]string{"-sex", "male", "-age", "30-39", "-where", "DEMO_J.RIDRETH9=1", ids[0]}, 1, "has no variable RIDRETH9"},
	} {
		out, errs, code := run(t, dir, "", append([]string{"compare", "-unknown-files"}, c.args...)...)
		if code != c.code || !strings.Contains(out+errs, c.want) {
			t.Errorf("%s: exit %d, want %d and %q\n%s\n%s", name, code, c.code, c.want, out, errs)
		}
		if strings.Contains(out, "caveats") {
			t.Errorf("%s: a result was printed\n%s", name, out)
		}
	}
	// A reading in another unit, as text, corrected or deleted.
	mmol, _, _ := run(t, dir, "", "add", "lab.cholesterol-total", "4.9", "-unit", "mmol/L")
	text, _, _ := run(t, dir, "", "add", "lab.cholesterol-total", "<100")
	newer, _, _ := run(t, dir, "", "add", "lab.cholesterol-total", "201", "-unit", "mg/dL", "-corrects", ids[0])
	for name, c := range map[string][2]string{
		"another unit":      {strings.TrimSpace(mmol), `the reading is in "mmol/L", and bean compares this type only in mg/dL`},
		"text":              {strings.TrimSpace(text), "the reading is not a number"},
		"a corrected value": {ids[0], "is not a current record about you"},
	} {
		out, errs, code := run(t, dir, "", "compare", "-unknown-files", "-sex", "male", "-age", "30-39", c[0])
		if code != 1 || !strings.Contains(out+errs, c[1]) || strings.Contains(out, "caveats") {
			t.Errorf("%s: %d\n%s\n%s", name, code, out, errs)
		}
	}
	if out, _, code := run(t, dir, "", "compare", "-unknown-files", "-sex", "male", "-age", "30-39", strings.TrimSpace(newer)); code != 0 || !strings.Contains(out, "201 mg/dL") {
		t.Errorf("the correction: %d\n%s", code, out)
	}
	// A value that cannot be a measurement gets no percentile.
	zero, _, _ := run(t, dir, "", "add", "lab.cholesterol-total", "0", "-unit", "mg/dL", "-at", "2026-10-01")
	if out, errs, code := run(t, dir, "", "compare", "-unknown-files", "-sex", "male", "-age", "30-39", strings.TrimSpace(zero)); code != 1 || !strings.Contains(out, "the reading is not a number above zero") {
		t.Errorf("zero: %d\n%s\n%s", code, out, errs)
	}
	// Without -unknown-files a copy that is not the file on record is not
	// used: it may be a download that was cut short.
	if out, errs, code := run(t, dir, "", "compare", "-sex", "male", "-age", "30-39", strings.TrimSpace(newer)); code != 1 || !strings.Contains(out, "is not the file on record") || !strings.Contains(out, "-unknown-files") || strings.Contains(out, "caveats") {
		t.Errorf("unknown copy without the flag: %d\n%s\n%s", code, out, errs)
	}
	// Two kept copies of one data set that bean does not know: it cannot
	// choose, and says so.
	other := filepath.Join(t.TempDir(), "TCHOL_J.XPT")
	raw, _ := os.ReadFile(refData + "TCHOL_J.XPT")
	// One changed value in the last row keeps the file readable.
	raw[len(raw)-80-24+8+7] ^= 1
	os.WriteFile(other, raw, 0o600)
	if out, errs, code := runPath(t, dir, path, "import", "-yes", "nhanes", other); code != 0 {
		t.Fatalf("second copy: %d\n%s\n%s", code, out, errs)
	}
	if out, errs, code := run(t, dir, "", "compare", "-unknown-files", "-sex", "male", "-age", "30-39", strings.TrimSpace(newer)); code != 1 || !strings.Contains(out, "keeps 2 copies of TCHOL_J") {
		t.Errorf("two copies: %d\n%s\n%s", code, out, errs)
	}
	// A purged reference file is gone for compare too.
	ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "reference.file")
	for _, line := range strings.Split(strings.TrimSpace(ls), "\n") {
		var r struct {
			ID      string         `json:"id"`
			Context map[string]any `json:"context"`
		}
		json.Unmarshal([]byte(line), &r)
		if r.Context["dataset"] == "TCHOL_J" {
			run(t, dir, "", "purge", r.ID)
		}
	}
	if out, errs, code := run(t, dir, "", "compare", "-unknown-files", "-sex", "male", "-age", "30-39", strings.TrimSpace(newer)); code != 1 || !strings.Contains(out, "The log does not keep TCHOL_J") {
		t.Errorf("purged file: %d\n%s\n%s", code, out, errs)
	}
}

// A saved comparison does not outlive its reading: a correction or a
// deletion of the reading deletes it, as a record every copy obeys, and a
// comparison the person deleted is not saved again in silence.
func TestCompareDoesNotOutliveItsReading(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "HDL_J", "DEMO_J")
	save := func(id string) {
		t.Helper()
		if out, errs, code := run(t, dir, "", "compare", "-unknown-files", "-sex", "male", "-age", "30-39", "-save", id); code != 0 || !strings.Contains(out, "added 1") {
			t.Fatalf("save: %d\n%s\n%s", code, out, errs)
		}
	}
	count := func() int {
		ls, _, _ := run(t, dir, "", "ls", "-type", "comparison.percentile")
		return strings.Count(ls, "comparison.percentile")
	}
	save(ids[0])
	save(ids[1])
	if count() != 2 {
		t.Fatalf("%d comparisons", count())
	}
	// A correction of the first reading deletes its comparison.
	out, errs, code := run(t, dir, "", "add", "lab.cholesterol-total", "201", "-unit", "mg/dL", "-at", "2026-11-01", "-corrects", ids[0])
	if code != 0 || !strings.Contains(errs, "deleted 1 saved comparisons") || count() != 1 {
		t.Fatalf("after a correction: %d, %d comparisons\n%s\n%s", code, count(), out, errs)
	}
	newer := strings.TrimSpace(out)
	save(newer)
	// A deletion of the second reading deletes its comparison.
	if _, errs, code := run(t, dir, "", "purge", ids[1]); code != 0 || !strings.Contains(errs, "deleted 1 saved comparisons") || count() != 1 {
		t.Fatalf("after a purge: %d, %d comparisons\n%s", code, count(), errs)
	}
	if out, _, code := run(t, dir, "", "verify"); code != 0 || !strings.Contains(out, "0 invalid") {
		t.Errorf("verify: %d\n%s", code, out)
	}
	// A comparison the person deleted can be saved again when they ask: it
	// is a computation, not an import.
	ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "comparison.percentile")
	var r struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(strings.TrimSpace(ls)), &r)
	run(t, dir, "", "purge", r.ID)
	if count() != 0 {
		t.Fatalf("%d comparisons after purging the last", count())
	}
	save(newer)
	// A reading that comes back, because its correction was deleted, can be
	// compared and saved again: the cleanup that deleted its comparison was
	// bean's, not the person's.
	run(t, dir, "", "purge", newer)
	if stdout, errs, code := run(t, dir, "", "compare", "-unknown-files", "-sex", "male", "-age", "30-39", "-save", ids[0]); code != 0 || !strings.Contains(stdout, "added 1, already there 0") {
		t.Errorf("a reading that came back: %d\n%s\n%s", code, stdout, errs)
	}
}

// A deletion that arrives with a copy made by another tool takes the saved
// comparison with it at the next bean verify, which is what the docs say to
// run after such a copy. Until then no command shows the deleted value, and
// the comparison cannot be shared.
func TestCompareAfterADeletionArrives(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "DEMO_J")
	// Another copy of the log, made before the comparison is saved, deletes
	// the reading: it has no comparison to clean up itself.
	other := t.TempDir()
	if err := os.CopyFS(other, os.DirFS(dir)); err != nil {
		t.Fatal(err)
	}
	if out, errs, code := run(t, dir, "", "compare", "-unknown-files", "-sex", "male", "-age", "30-39", "-save", ids[0]); code != 0 {
		t.Fatalf("%s\n%s", out, errs)
	}
	// CopyFS does not keep the modes of the key files: the copy's own bean
	// would refuse them. Purge there with the copy's keys made private again.
	filepath.WalkDir(filepath.Join(other, "local"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			os.Chmod(p, 0o600)
		}
		return nil
	})
	if _, errs, code := run(t, other, "", "purge", ids[0]); code != 0 {
		t.Fatalf("purge in the copy: %s", errs)
	}
	// The tombstone comes over as a file, the way rsync brings it.
	filepath.WalkDir(filepath.Join(other, "objects"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(other, p)
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			data, _ := os.ReadFile(p)
			os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o700)
			os.WriteFile(filepath.Join(dir, rel), data, 0o600)
		}
		return nil
	})
	// Before any cleanup: ls still lists the comparison, without the deleted
	// value.
	ls, _, _ := run(t, dir, "", "ls")
	if !strings.Contains(ls, "comparison.percentile") || strings.Contains(ls, "193 mg/dL") {
		t.Errorf("ls before the cleanup:\n%s", ls)
	}
	// verify cleans up.
	if _, errs, code := run(t, dir, "", "verify"); code != 0 || !strings.Contains(errs, "deleted 1 saved comparisons") {
		t.Errorf("verify: %d\n%s", code, errs)
	}
	if ls, _, _ := run(t, dir, "", "ls", "-type", "comparison.percentile"); ls != "" {
		t.Errorf("the comparison outlived its reading:\n%s", ls)
	}
}

// A reading that an agent signed is marked, in the table and in the claim.
func TestCompareMarksAnotherAuthor(t *testing.T) {
	dir, _, _ := compareLog(t, "TCHOL_J", "DEMO_J")
	if _, errs, code := run(t, dir, "", "agent", "init", "-label", "assistant"); code != 0 {
		t.Fatal(errs)
	}
	out, errs, code := run(t, dir, "", "add", "-agent", "lab.cholesterol-total", "180", "-unit", "mg/dL")
	if code != 0 {
		t.Fatalf("an agent's reading: %s", errs)
	}
	id := strings.TrimSpace(out)
	out, _, code = run(t, dir, "", "compare", "-unknown-files", "-sex", "male", "-age", "30-39", "-save", id)
	if code != 0 || !strings.Contains(out, "180 mg/dL  [assistant]") || !strings.Contains(out, "signed by an agent or another key") {
		t.Errorf("%d\n%s", code, out)
	}
	if ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "comparison.percentile"); !strings.Contains(ls, "signed by an agent or another key") {
		t.Errorf("the claim does not say who signed the reading:\n%s", ls)
	}
}

// The MCP tool does not write a comparison, a reference file or the record
// of a share, also when the person's own vocabulary names the type. And the
// caveats in tools/compare/reproduce.py are the ones bean writes.
func TestCompareIsNotForTheMCPTool(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "", "init")
	run(t, dir, "", "agent", "init", "-label", "assistant")
	os.WriteFile(filepath.Join(dir, "vocabulary.md"), []byte("| Type | Value | Useful details |\n|---|---|---|\n| `comparison.percentile` | number | |\n| `share` | number | |\n| `reference.file` | text | |\n"), 0o600)
	for _, typ := range []string{"comparison.percentile", "share", "reference.file"} {
		call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"log_entry","arguments":{"type":"` + typ + `","value":46}}}` + "\n"
		out, _, _ := run(t, dir, call, "mcp", "-write", "-label", "assistant")
		if !strings.Contains(out, "an assistant cannot write them") {
			t.Errorf("%s through the MCP tool:\n%s", typ, out)
		}
		if ls, _, _ := run(t, dir, "", "ls", "-all", "-type", typ); ls != "" {
			t.Errorf("%s was written:\n%s", typ, ls)
		}
	}
	py, err := os.ReadFile("../../tools/compare/reproduce.py")
	if err != nil {
		t.Fatal(err)
	}
	// The table of tests is the same in both, row for row.
	for _, a := range nhanes.Analytes {
		row := fmt.Sprintf(`(%q, %q): (%q, %q),`, a.Type, a.Unit, a.Dataset, a.Variable)
		if !strings.Contains(string(py), row) {
			t.Errorf("tools/compare/reproduce.py does not have the row %s", row)
		}
	}
	if got := strings.Count(string(py), `"): ("`); got != len(nhanes.Analytes) {
		t.Errorf("tools/compare/reproduce.py has %d rows of tests, bean has %d", got, len(nhanes.Analytes))
	}
	// And what no bundle can show is said in the same words by both checkers.
	second, err := os.ReadFile("../../tools/second/checkbundle.py")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range bundleCaveats {
		if !strings.Contains(string(second), `"`+text+`"`) {
			t.Errorf("tools/second/checkbundle.py does not have the caveat %q", text)
		}
	}
	for _, text := range []string{caveatMethod, caveatPopulation, caveatCohort, caveatTopAge, caveatAuthor, fmt.Sprintf(caveatSmall, nhanes.SmallN), strings.Replace(caveatUnknown, "%s", "%s", 1), nhanes.CohortFiles["DR1TOT_J"].Reason} {
		if !strings.Contains(string(py), `"`+text+`"`) {
			t.Errorf("tools/compare/reproduce.py does not have the caveat %q", text)
		}
	}
}
