package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Conscious-Repository/bean/internal/nhanes"
	"github.com/Conscious-Repository/bean/internal/pack"
	"github.com/Conscious-Repository/bean/internal/xpt"
)

// testPack builds a pack from the synthetic files and returns its path and
// its SHA-256.
func testPack(t *testing.T) (string, string) {
	t.Helper()
	files := map[string]nhanes.SourceFile{}
	for _, name := range []string{"TCHOL_J", "HDL_J", "DEMO_J"} {
		data, err := os.ReadFile(filepath.Join("../../internal/nhanes/testdata", name+".XPT"))
		if err != nil {
			t.Fatal(err)
		}
		f, err := xpt.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		files[name] = nhanes.SourceFile{File: f, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(data))}
	}
	p, err := nhanes.BuildPack(files)
	if err != nil {
		t.Fatal(err)
	}
	data := p.Bytes()
	path := filepath.Join(t.TempDir(), "syn.pack")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return path, hex.EncodeToString(sum[:])
}

// packRow returns the line of the output that holds a reading's id.
func packRow(out, id string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, id[7:19]) {
			return strings.Join(strings.Fields(line), " ")
		}
	}
	return ""
}

// TestComparePack: a comparison with a pack gives the numbers that the
// files give, names the pack by its SHA-256, and saves a claim that holds
// what a reader needs.
func TestComparePack(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "HDL_J", "DEMO_J")
	path, digest := testPack(t)
	// Without a cohort: what the pack holds.
	out, errs, code := run(t, dir, "", "compare", "-pack", path)
	if code != 0 {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	for _, want := range []string{"NHANES 2017-2018: adults by sex and ten years of age", "sha256 " + digest, "TCHOL_J.xpt", "lab.cholesterol-total (mg/dL)", "male-30-39: male, age 30-39", "female-80-over: female, age 80 and over", "bean has NO record of this pack"} {
		if !strings.Contains(out, want) {
			t.Errorf("the menu has no %q:\n%s", want, out)
		}
	}
	// The same numbers as with the files.
	out, errs, code = run(t, dir, "", "compare", "-pack", path, "-unknown-pack", "-cohort", "male-30-39", ids[0])
	if code != 0 {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	byFiles, _, _ := run(t, dir, "", "compare", "-unknown-files", "-sex", "male", "-age", "30-39", ids[0])
	got, want := packRow(out, ids[0]), strings.Replace(packRow(byFiles, ids[0]), "small cohort, file not on record ", "small cohort, pack NOT on record ", 1)
	if got == "" || got != want {
		t.Errorf("with the pack:  %s\nwith the files: %s", got, want)
	}
	for _, want := range []string{"SHA-256 " + digest, "male, age 30-39", "WARNING: bean has NO record of this pack", "pack NOT on record", "nobody has checked its tables", "lab.cholesterol-total in mg/dL against Total Cholesterol (mg/dL)", "this copy of TCHOL_J is not the file on record", "you chose the cohort", "Not saved"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in\n%s", want, out)
		}
	}
	// -save signs a claim, once.
	out, errs, code = run(t, dir, "", "compare", "-pack", path, "-unknown-pack", "-cohort", "male-30-39", "-type", "lab.*", "-save")
	if code != 0 || !strings.Contains(out, "added 2, already there 0") || !strings.Contains(out, "No reference data for 1 other type: lab.tsh.") || !strings.Contains(out, "with the pack, gives the reading's value away") {
		t.Fatalf("-save: %d\n%s\n%s", code, out, errs)
	}
	if out, _, _ := run(t, dir, "", "compare", "-pack", path, "-unknown-pack", "-cohort", "male-30-39", "-type", "lab.*", "-save"); !strings.Contains(out, "added 0, already there 2") {
		t.Errorf("a second -save:\n%s", out)
	}
	ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "comparison.percentile")
	for _, want := range []string{`"method":{"id":"weighted-percentile","version":"1"}`, `"pack":"` + digest + `"`, `"cohort":"male-30-39"`, `"measure":"LBXTC"`, `"cohort":{"text":"male, age 30-39"}`, `"reading":"` + ids[0] + `"`} {
		if !strings.Contains(ls, want) {
			t.Errorf("the claim has no %s:\n%s", want, ls)
		}
	}
	// A listing shows it in words, like a comparison with the files.
	if list, _, _ := run(t, dir, "", "ls", "-type", "comparison.percentile"); !strings.Contains(list, "among male, age 30-39 (n ") {
		t.Errorf("ls:\n%s", list)
	}
	// Shared, it tells its reader which pack it was made with, and that
	// the pack is needed to compute it again.
	bundle := filepath.Join(t.TempDir(), "b")
	if out, errs, code := run(t, dir, "", "share", "-yes", "-out", bundle, "-type", "comparison.*"); code != 0 {
		t.Fatalf("share: %d\n%s\n%s", code, out, errs)
	}
	checked, _, code := run(t, t.TempDir(), "", "check-bundle", bundle)
	for _, want := range []string{"bundle ok", "among male, age 30-39 (n ", ` against "Total Cholesterol (mg/dL)", among male`, ", by pack " + digest[:12] + " (NOT on record)", "get the pack with that SHA-256", digest + `: bean has NO record of this pack. The sender calls it "NHANES 2017-2018: adults by sex and ten years of age".`, "nobody has checked its tables"} {
		if code != 0 || !strings.Contains(checked, want) {
			t.Errorf("check-bundle: %d, no %q in\n%s", code, want, checked)
		}
	}
	// A comparison does not outlive its reading, whatever it was made with.
	if _, errs, code := run(t, dir, "", "purge", ids[0]); code != 0 {
		t.Fatalf("purge: %s", errs)
	}
	run(t, dir, "", "verify")
	if ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "comparison.percentile"); strings.Contains(ls, ids[0]) || !strings.Contains(ls, ids[1]) {
		t.Errorf("after the reading is deleted:\n%s", ls)
	}
}

// TestComparePackRefuses: what is not a pack, a cohort that is not on the
// menu, a unit that the pack does not take, and the flags of the files.
func TestComparePackRefuses(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "DEMO_J")
	path, _ := testPack(t)
	data, _ := os.ReadFile(path)
	bad := filepath.Join(t.TempDir(), "bad.pack")
	os.WriteFile(bad, []byte(strings.Replace(string(data), `"kind":"weights"`, `"kind":"quantiles"`, 1)), 0o644)
	cut := filepath.Join(t.TempDir(), "cut.pack")
	os.WriteFile(cut, data[:len(data)/2], 0o644)
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"another kind of cell":            {[]string{"compare", "-pack", bad, "-unknown-pack", "-cohort", "male-30-39", ids[0]}, "this version reads only"},
		"a pack cut short":                {[]string{"compare", "-pack", cut}, "it is not a pack"},
		"no such file":                    {[]string{"compare", "-pack", filepath.Join(dir, "none.pack")}, "no such file"},
		"a cohort not there":              {[]string{"compare", "-pack", path, "-unknown-pack", "-cohort", "male-35-44", ids[0]}, "the pack has no cohort"},
		"no cohort":                       {[]string{"compare", "-pack", path, ids[0]}, "name a cohort of the pack with -cohort"},
		"a pack not on record":            {[]string{"compare", "-pack", path, "-cohort", "male-30-39", ids[0]}, "pass -unknown-pack"},
		"-unknown-pack with no pack":      {[]string{"compare", "-unknown-pack", "-sex", "male", "-age", "30-39", ids[0]}, "usage"},
		"-sex with a pack":                {[]string{"compare", "-pack", path, "-sex", "male", "-age", "30-39", ids[0]}, "with -pack, name a cohort"},
		"-cohort with no pack":            {[]string{"compare", "-cohort", "male-30-39", ids[0]}, "usage"},
		"no reading":                      {[]string{"compare", "-pack", path, "-cohort", "male-30-39"}, "say which readings to compare"},
		"-save with no cohort":            {[]string{"compare", "-pack", path, "-save"}, "name a cohort of the pack with -cohort"},
		"only what has no reference data": {[]string{"compare", "-pack", path, "-unknown-pack", "-cohort", "male-30-39", ids[2]}, "nothing could be compared"},
	} {
		out, errs, code := run(t, dir, "", tc.args...)
		if code == 0 || !strings.Contains(out+errs, tc.want) {
			t.Errorf("%s: exit %d, no %q in\n%s%s", name, code, tc.want, out, errs)
		}
	}
	// A reading in another unit gets a row that says so, and no number.
	id, _, _ := run(t, dir, "", "add", "lab.cholesterol-total", "4.9", "-unit", "mmol/L", "-at", "2026-11-02")
	out, _, code := run(t, dir, "", "compare", "-pack", path, "-unknown-pack", "-cohort", "male-30-39", strings.TrimSpace(id))
	if code == 0 || !strings.Contains(out, `the reading is in "mmol/L", and the pack takes this type only in mg/dL`) {
		t.Errorf("another unit: %d\n%s", code, out)
	}
	if ls, _, _ := run(t, dir, "", "ls", "-type", "comparison.percentile"); ls != "" {
		t.Errorf("a refusal saved a comparison:\n%s", ls)
	}
}

// TestComparisonThatNamesNothing: a comparison signed by hand that names
// neither a pack nor files is marked wherever it is shown. Nobody can
// compute it again.
func TestComparisonThatNamesNothing(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "DEMO_J")
	claim := `{"type":"comparison.percentile","source":{"system":"by-hand","adapter":"test@1","ref":null},"times":{"observed":{"start":"2026-11-01T00:00:00Z","precision":"day"}},"value":{"n":95,"unit":"{percentile}"},"refs":[{"rel":"about","id":"` + ids[0] + `"}],"context":{"reading":"` + ids[0] + `","cohort":{"text":"male, age 30-39"},"result":{"n":339}}}`
	if out, errs, code := run(t, dir, claim+"\n", "append"); code != 0 {
		t.Fatalf("append: %d\n%s\n%s", code, out, errs)
	}
	ls, _, _ := run(t, dir, "", "ls", "-type", "comparison.percentile")
	if !strings.Contains(ls, "percentile 95 of lab.cholesterol-total 193 mg/dL, among male, age 30-39 (n 339) (names NO reference data)") {
		t.Errorf("ls:\n%s", ls)
	}
}

// TestPythonHoldsThePackTables: the Python tools hold the packs on record
// and the caveat of a pack that is not, as bean does. The texts and rows of
// the NHANES pack need no such test: tools/pack/rebuild_nhanes.py builds the
// pack from its own and compares the bytes (make pack).
func TestPythonHoldsThePackTables(t *testing.T) {
	py, err := os.ReadFile("../../tools/compare/reproduce.py")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(py), `"`+pack.CaveatNotOnRecord+`"`) {
		t.Errorf("tools/compare/reproduce.py does not have the caveat %q", pack.CaveatNotOnRecord)
	}
	for digest, k := range pack.Known {
		if !strings.Contains(string(py), `"`+digest+`": "`+k.What+`"`) {
			t.Errorf("tools/compare/reproduce.py does not have the pack on record %s", digest)
		}
	}
	if n := len(regexp.MustCompile(`(?m)^    "[0-9a-f]{64}": "`).FindAllString(string(py), -1)); n != len(pack.Known) {
		t.Errorf("tools/compare/reproduce.py has %d packs on record, bean has %d", n, len(pack.Known))
	}
}

// TestThePackOnRecordIsInTheDocument: docs/REFERENCE.md states the SHA-256
// of each pack that bean has on record, and no other.
func TestThePackOnRecordIsInTheDocument(t *testing.T) {
	doc, err := os.ReadFile("../../docs/REFERENCE.md")
	if err != nil {
		t.Fatal(err)
	}
	stated := regexp.MustCompile(`(?m)^[0-9a-f]{64}$`).FindAllString(string(doc), -1)
	if len(stated) != len(pack.Known) {
		t.Errorf("docs/REFERENCE.md states %d digests of packs, bean has %d on record", len(stated), len(pack.Known))
	}
	for _, digest := range stated {
		if !pack.IsKnown(digest) {
			t.Errorf("docs/REFERENCE.md states the pack %s, and bean has no record of it", digest)
		}
	}
}

// TestComparePackEdges: a cohort that the pack has no table for, a reading
// with no unit, a file that never ends, and a claim signed by hand that has
// the inputs of a saved one and another value.
func TestComparePackEdges(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "DEMO_J")
	path, _ := testPack(t)
	// The builder leaves out a cell of fewer than 30 people. Find one.
	menu, _, _ := run(t, dir, "", "compare", "-pack", path)
	missing := ""
	for _, line := range strings.Split(menu, "\n") {
		id, _, isCohort := strings.Cut(strings.TrimSpace(line), ": ")
		if !isCohort || !strings.Contains(line, ", age ") || missing != "" {
			continue
		}
		if out, _, _ := run(t, dir, "", "compare", "-pack", path, "-unknown-pack", "-cohort", id, ids[0]); strings.Contains(out, "the pack has no table of this measure for this cohort") {
			missing = id
		}
	}
	if missing == "" {
		t.Error("no cohort of the synthetic pack lacks a table: the case is not tried")
	}
	id, _, _ := run(t, dir, "", "add", "lab.cholesterol-total", "190", "-at", "2026-11-02")
	if out, _, code := run(t, dir, "", "compare", "-pack", path, "-unknown-pack", "-cohort", "male-30-39", strings.TrimSpace(id)); code == 0 || !strings.Contains(out, "the reading has no unit, and the pack takes this type only in mg/dL") {
		t.Errorf("a reading with no unit: %d\n%s", code, out)
	}
	if _, err := os.Stat("/dev/zero"); err == nil {
		if out, errs, code := run(t, dir, "", "compare", "-pack", "/dev/zero"); code == 0 || !strings.Contains(out+errs, "a pack is at most 64 MiB") {
			t.Errorf("a file that never ends: %d\n%s%s", code, out, errs)
		}
	}
	// The same inputs with another value are another claim: neither is
	// deleted as one that was saved twice.
	if out, errs, code := run(t, dir, "", "compare", "-pack", path, "-unknown-pack", "-cohort", "male-30-39", "-save", ids[0]); code != 0 {
		t.Fatalf("-save: %d\n%s\n%s", code, out, errs)
	}
	ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "comparison.percentile")
	var claim map[string]any
	if err := json.Unmarshal([]byte(strings.SplitN(ls, "\n", 2)[0]), &claim); err != nil {
		t.Fatal(err)
	}
	forged, _ := json.Marshal(map[string]any{
		"type": claim["type"], "value": map[string]any{"n": 99.9, "unit": "{percentile}"}, "refs": claim["refs"], "context": claim["context"],
		"times":  map[string]any{"observed": claim["times"].(map[string]any)["observed"]},
		"source": map[string]any{"system": "by-hand", "adapter": "test@1", "ref": nil},
	})
	if out, errs, code := run(t, dir, string(forged)+"\n", "append"); code != 0 {
		t.Fatalf("append: %d\n%s\n%s", code, out, errs)
	}
	run(t, dir, "", "verify")
	if ls, _, _ := run(t, dir, "", "ls", "-type", "comparison.percentile"); strings.Count(ls, "comparison.percentile") != 2 || !strings.Contains(ls, "percentile 99.9 of") {
		t.Errorf("a claim with another value was deleted as a duplicate, or the saved one was:\n%s", ls)
	}
}

// TestRowOfAPackOnRecord: for a pack that bean has on record, a row shows
// the pack's own label and cohort, whatever the claim says, and marks a
// claim that says otherwise. The claims are signed by hand: no pack file
// is needed to show a row.
func TestRowOfAPackOnRecord(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "DEMO_J")
	var digest string
	for d := range pack.Known {
		digest = d
	}
	claim := func(reference, cohortText string) string {
		return `{"type":"comparison.percentile","source":{"system":"by-hand","adapter":"test@1","ref":null},"times":{"observed":{"start":"2026-11-01T00:00:00Z","precision":"day"}},"value":{"n":46,"unit":"{percentile}"},"refs":[{"rel":"about","id":"` + ids[0] + `"}],"context":{"reading":"` + ids[0] + `","cohort":{"text":"` + cohortText + `"},"result":{"n":339},"reference":{` + reference + `}}}`
	}
	onRecord := `"pack":"` + digest + `","measure":"LBXTC","cohort":"male-30-39",`
	const own = "percentile 46 of lab.cholesterol-total 193 mg/dL against Total Cholesterol (mg/dL), among male, age 30-39 (n 339), by pack "
	for name, tc := range map[string]struct{ claim, want string }{
		"what the pack says":                    {claim(onRecord+`"label":"Total Cholesterol (mg/dL)"`, "male, age 30-39"), own + digest[:12] + "    "},
		"another label":                         {claim(onRecord+`"label":"Hemoglobin A1c, fasting (%) (on record)"`, "male, age 30-39"), own + digest[:12] + " (the claim MISSTATES this pack)"},
		"a label that imitates a mark":          {claim(onRecord+`"label":"Total Cholesterol (mg/dL), by pack `+digest[:12]+`"`, "male, age 30-39"), own + digest[:12] + " (the claim MISSTATES this pack)"},
		"another cohort in words":               {claim(onRecord+`"label":"Total Cholesterol (mg/dL)"`, "everyone, by pack "+digest[:12]), own + digest[:12] + " (the claim MISSTATES this pack)"},
		"a measure that the pack does not have": {claim(`"pack":"`+digest+`","measure":"LBXLDL","cohort":"male-30-39","label":"LDL (mg/dL)"`, "male, age 30-39"), "193 mg/dL, among male, age 30-39 (n 339), by pack " + digest[:12] + " (the claim MISSTATES this pack)"},
		"a cohort that the pack does not have":  {claim(`"pack":"`+digest+`","measure":"LBXTC","cohort":"male-35-44","label":"Total Cholesterol (mg/dL)"`, "male, age 35-44"), "(the claim MISSTATES this pack)"},
		"a pack and files":                      {claim(onRecord+`"label":"Total Cholesterol (mg/dL)","files":[]`, "male, age 30-39"), "(names BOTH a pack and files)"},
		"a pack not on record and files":        {claim(`"pack":"`+strings.Repeat("a", 64)+`","measure":"LBXTC","cohort":"male-30-39","label":"x","files":[]`, "male, age 30-39"), "(names BOTH a pack and files)"},
		"files that name nothing":               {claim(`"files":[]`, "male, age 30-39"), "(names NO reference data)"},
		"files that are not files":              {claim(`"files":[1]`, "male, age 30-39"), "(names NO reference data)"},
		"a pack of 64 bytes that are no hex":    {claim(`"pack":"a`+strings.Repeat("é", 31)+`b","measure":"LBXTC","cohort":"male-30-39","label":"x"`, "male, age 30-39"), "193 mg/dL, among male, age 30-39 (n 339) (names NO reference data)"},
		"a pack that is no SHA-256":             {claim(`"pack":"10555d","measure":"LBXTC","cohort":"male-30-39","label":"x"`, "male, age 30-39"), "(names NO reference data)"},
	} {
		d := t.TempDir()
		if err := os.CopyFS(d, os.DirFS(dir)); err != nil {
			t.Fatal(err)
		}
		if out, errs, code := run(t, d, tc.claim+"\n", "append"); code != 0 {
			t.Fatalf("%s: append: %d\n%s\n%s", name, code, out, errs)
		}
		if ls, _, _ := run(t, d, "", "ls", "-type", "comparison.percentile"); !strings.Contains(ls+"    ", tc.want) || strings.Count(ls, "by pack") > 1 || !utf8.ValidString(ls) {
			t.Errorf("%s: no %q in\n%s", name, tc.want, ls)
		}
	}
	// A reading of a type that the measure does not take, and a record that
	// is no reading at all: the pack on record says which types it takes.
	weight, _, _ := run(t, dir, "", "add", "weight", "81.2", "-unit", "kg", "-at", "2026-11-01")
	weight = strings.TrimSpace(weight)
	about := func(reading string) string {
		return strings.ReplaceAll(claim(onRecord+`"label":"Total Cholesterol (mg/dL)"`, "male, age 30-39"), ids[0], reading)
	}
	if out, errs, code := run(t, dir, about(weight)+"\n", "append"); code != 0 {
		t.Fatalf("append: %d\n%s\n%s", code, out, errs)
	}
	ls, _, _ := run(t, dir, "", "ls", "-type", "comparison.percentile")
	if !strings.Contains(ls, "of weight 81.2 kg against Total Cholesterol (mg/dL), among male, age 30-39 (n 339), by pack "+digest[:12]+" (the claim MISSTATES this pack)") {
		t.Errorf("a weight set against cholesterol:\n%s", ls)
	}
	first, _, _ := run(t, dir, "", "ls", "-json", "-type", "comparison.percentile")
	var c map[string]any
	if err := json.Unmarshal([]byte(strings.SplitN(first, "\n", 2)[0]), &c); err != nil {
		t.Fatal(err)
	}
	if out, errs, code := run(t, dir, about(c["id"].(string))+"\n", "append"); code != 0 {
		t.Fatalf("append: %d\n%s\n%s", code, out, errs)
	}
	if ls, _, _ := run(t, dir, "", "ls", "-type", "comparison.percentile"); !strings.Contains(ls, "of comparison.percentile ") || !strings.Contains(ls, "(is NOT about a reading)") {
		t.Errorf("a comparison of a comparison:\n%s", ls)
	}
}
