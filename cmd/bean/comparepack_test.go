package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/internal/nhanes"
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
	for _, want := range []string{"NHANES 2017-2018: adults by sex and ten years of age", "sha256 " + digest, "TCHOL_J.xpt", "lab.cholesterol-total (mg/dL)", "male-30-39: male, age 30-39", "female-80-over: female, age 80 and over", "bean has no record of this pack"} {
		if !strings.Contains(out, want) {
			t.Errorf("the menu has no %q:\n%s", want, out)
		}
	}
	// The same numbers as with the files.
	out, errs, code = run(t, dir, "", "compare", "-pack", path, "-cohort", "male-30-39", ids[0])
	if code != 0 {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	byFiles, _, _ := run(t, dir, "", "compare", "-unknown-files", "-sex", "male", "-age", "30-39", ids[0])
	got, want := packRow(out, ids[0]), strings.Replace(packRow(byFiles, ids[0]), "small cohort, file not on record ", "small cohort ", 1)
	if got == "" || got != want {
		t.Errorf("with the pack:  %s\nwith the files: %s", got, want)
	}
	for _, want := range []string{"SHA-256 " + digest, "male, age 30-39", "lab.cholesterol-total in mg/dL against Total Cholesterol (mg/dL)", "this copy of TCHOL_J is not the file on record", "you chose the cohort", "Not saved"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in\n%s", want, out)
		}
	}
	// -save signs a claim, once.
	out, errs, code = run(t, dir, "", "compare", "-pack", path, "-cohort", "male-30-39", "-type", "lab.*", "-save")
	if code != 0 || !strings.Contains(out, "added 2, already there 0") || !strings.Contains(out, "No reference data for 1 other type: lab.tsh.") || !strings.Contains(out, "with the pack, gives the reading's value away") {
		t.Fatalf("-save: %d\n%s\n%s", code, out, errs)
	}
	if out, _, _ := run(t, dir, "", "compare", "-pack", path, "-cohort", "male-30-39", "-type", "lab.*", "-save"); !strings.Contains(out, "added 0, already there 2") {
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
		"another kind of cell":            {[]string{"compare", "-pack", bad, "-cohort", "male-30-39", ids[0]}, "this version reads only"},
		"a pack cut short":                {[]string{"compare", "-pack", cut}, "it is not a pack"},
		"no such file":                    {[]string{"compare", "-pack", filepath.Join(dir, "none.pack")}, "no such file"},
		"a cohort not there":              {[]string{"compare", "-pack", path, "-cohort", "male-35-44", ids[0]}, "the pack has no cohort"},
		"no cohort":                       {[]string{"compare", "-pack", path, ids[0]}, "name a cohort of the pack with -cohort"},
		"-sex with a pack":                {[]string{"compare", "-pack", path, "-sex", "male", "-age", "30-39", ids[0]}, "with -pack, name a cohort"},
		"-cohort with no pack":            {[]string{"compare", "-cohort", "male-30-39", ids[0]}, "usage"},
		"only what has no reference data": {[]string{"compare", "-pack", path, "-cohort", "male-30-39", ids[2]}, "nothing could be compared"},
	} {
		out, errs, code := run(t, dir, "", tc.args...)
		if code == 0 || !strings.Contains(out+errs, tc.want) {
			t.Errorf("%s: exit %d, no %q in\n%s%s", name, code, tc.want, out, errs)
		}
	}
	// A reading in another unit gets a row that says so, and no number.
	id, _, _ := run(t, dir, "", "add", "lab.cholesterol-total", "4.9", "-unit", "mmol/L", "-at", "2026-11-02")
	out, _, code := run(t, dir, "", "compare", "-pack", path, "-cohort", "male-30-39", strings.TrimSpace(id))
	if code == 0 || !strings.Contains(out, `the reading is in "mmol/L", and the pack takes this type only in mg/dL`) {
		t.Errorf("another unit: %d\n%s", code, out)
	}
	if ls, _, _ := run(t, dir, "", "ls", "-type", "comparison.percentile"); ls != "" {
		t.Errorf("a refusal saved a comparison:\n%s", ls)
	}
}
