package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// objectPath is where a bundle or a log keeps the record with this id.
func objectPath(dir, id string) string {
	return filepath.Join(dir, "objects", id[7:9], id[9:]+".json")
}

func TestShare(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "DEMO_J")
	note, _, _ := run(t, dir, "", "add", "note", "after a week of oats", "-about", ids[0])
	note = strings.TrimSpace(note)
	out := filepath.Join(t.TempDir(), "bundle")

	// Without a terminal and without -yes nothing is written, and nothing
	// of the records is shown: an assistant cannot disclose for the person.
	stdout, errs, code := run(t, dir, "y\n", "share", "-out", out, note)
	if code != 1 || !strings.Contains(errs, "share needs a terminal") || strings.Contains(stdout+errs, "oats") {
		t.Fatalf("no terminal: %d\n%s\n%s", code, stdout, errs)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("a bundle was written without review")
	}
	if ls, _, _ := run(t, dir, "", "ls", "-type", "share"); ls != "" {
		t.Fatalf("a share record was written without review:\n%s", ls)
	}

	// A note comes with the reading it is about, and nothing else does.
	stdout, errs, code = run(t, dir, "", "share", "-yes", "-note", "for my doctor", "-out", out, note)
	if code != 0 || !strings.Contains(stdout, "shared 2 records and 0 files") || !strings.Contains(stdout, "share record sha256:") {
		t.Fatalf("share: %d\n%s\n%s", code, stdout, errs)
	}
	for id, want := range map[string]bool{note: true, ids[0]: true, ids[1]: false, ids[2]: false} {
		if _, err := os.Stat(objectPath(out, id)); (err == nil) != want {
			t.Errorf("%s in the bundle: %v, want %v", id[7:19], err == nil, want)
		}
	}
	// The bundle holds the record files as the log stores them, and no key.
	have, _ := os.ReadFile(objectPath(out, ids[0]))
	stored, _ := os.ReadFile(objectPath(dir, ids[0]))
	if string(have) != string(stored) || len(have) == 0 {
		t.Error("the bundle's record is not the stored record")
	}
	if _, err := os.Stat(filepath.Join(out, "local")); err == nil {
		t.Error("the bundle holds a local folder")
	}
	// The share record is in the log too: what was disclosed, and when.
	ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "share")
	var share struct {
		ID      string `json:"id"`
		Context struct {
			Records []string `json:"records"`
			Proof   []string `json:"proof"`
			Note    string   `json:"note"`
		} `json:"context"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(ls)), &share); err != nil {
		t.Fatalf("%v\n%s", err, ls)
	}
	if len(share.Context.Records) != 2 || len(share.Context.Proof) != 1 || share.Context.Note != "for my doctor" {
		t.Errorf("share record: %+v", share)
	}
	if _, err := os.Stat(objectPath(out, share.ID)); err != nil {
		t.Error("the bundle does not hold its share record")
	}

	// Anyone can check it, with no log: from an empty folder.
	nowhere := t.TempDir()
	stdout, errs, code = run(t, nowhere, "", "check-bundle", out)
	before := treeDigest(t, out)
	for _, want := range []string{"bundle ok", "share " + share.ID, "It holds:", "after a week of oats", "lab.cholesterol-total  193 mg/dL", "records: 2 listed; files: 0 included, 0 named and not included", "signed: 2 by the person, 0 by an agent", "corrections: 0 of the listed records", "note: for my doctor", "not a complete history", "not that a value is true", "cannot show that it is still the person's key"} {
		if code != 0 || !strings.Contains(stdout, want) {
			t.Errorf("check-bundle: %d, no %q in\n%s\n%s", code, want, stdout, errs)
		}
	}
	// The check changes nothing in the bundle: no entry more or fewer, and
	// no byte of any file.
	if treeDigest(t, out) != before {
		t.Error("check-bundle changed the bundle")
	}
	if _, errs, code := run(t, dir, "", "verify"); code != 0 {
		t.Errorf("the log after a share: %s", errs)
	}
}

// A type picks readings, never the record that keeps a file: 'lab.*' must
// not send the lab report, with the person's name in it. And a bundle is
// read with check-bundle, never synced into the reader's own log.
func TestShareKeepsFilesHomeAndBundlesOut(t *testing.T) {
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	report := filepath.Join(t.TempDir(), "report.txt")
	os.WriteFile(report, []byte(labReport("101")), 0o600)
	if out, errs, code := runPath(t, dir, path, "import", "-yes", "lab", report); code != 0 {
		t.Fatalf("%s\n%s", out, errs)
	}
	out := filepath.Join(t.TempDir(), "bundle")
	stdout, errs, code := run(t, dir, "", "share", "-yes", "-files", "-out", out, "-type", "lab.*")
	if code != 0 || !strings.Contains(stdout, "shared 2 records and 0 files") {
		t.Fatalf("%d\n%s\n%s", code, stdout, errs)
	}
	if _, err := os.Stat(filepath.Join(out, "files")); err == nil {
		t.Error("the lab report was shared by type")
	}
	// Named by its id, with -files, the report goes.
	ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "lab.report")
	var rep struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(strings.TrimSpace(ls)), &rep)
	named := filepath.Join(t.TempDir(), "named")
	if stdout, errs, code := run(t, dir, "", "share", "-yes", "-files", "-out", named, rep.ID); code != 0 || !strings.Contains(stdout, "shared 1 records and 1 files") {
		t.Fatalf("%d\n%s\n%s", code, stdout, errs)
	}
	// Another person's log refuses to sync a bundle, and is unchanged.
	sam := t.TempDir()
	run(t, sam, "", "init")
	before, _, _ := run(t, sam, "", "ls", "-all")
	if stdout, errs, code := run(t, sam, "", "sync", out); code != 1 || !strings.Contains(errs, "is not a copy of your log") || !strings.Contains(errs, "bean check-bundle") {
		t.Errorf("sync of a bundle: %d\n%s\n%s", code, stdout, errs)
	}
	if after, _, _ := run(t, sam, "", "ls", "-all"); after != before {
		t.Errorf("a refused sync changed the log:\n%s", after)
	}
	// The person's own copy still syncs.
	copyDir := t.TempDir()
	if err := os.CopyFS(filepath.Join(copyDir, "objects"), os.DirFS(filepath.Join(dir, "objects"))); err != nil {
		t.Fatal(err)
	}
	if _, errs, code := run(t, dir, "", "sync", copyDir); code != 0 {
		t.Errorf("sync of an own copy: %d\n%s", code, errs)
	}
}

// treeDigest is a digest of every path and every file's bytes under dir.
func treeDigest(t *testing.T, dir string) string {
	t.Helper()
	var all strings.Builder
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		all.WriteString(p + "\x00" + d.Type().String() + "\x00")
		if d.Type().IsRegular() {
			all.WriteString(fileDigest(p))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return all.String()
}

// A record brings the record it is about, and no further: what that one is
// about in turn stays home unless it is chosen too.
func TestShareBringsOneStep(t *testing.T) {
	dir, _, ids := compareLog(t)
	first, _, _ := run(t, dir, "", "add", "note", "first", "-about", ids[2])
	second, _, _ := run(t, dir, "", "add", "note", "second", "-about", strings.TrimSpace(first))
	first, second = strings.TrimSpace(first), strings.TrimSpace(second)
	out := filepath.Join(t.TempDir(), "bundle")
	if stdout, errs, code := run(t, dir, "", "share", "-yes", "-out", out, second); code != 0 || !strings.Contains(stdout, "shared 2 records") {
		t.Fatalf("%d\n%s\n%s", code, stdout, errs)
	}
	for id, want := range map[string]bool{second: true, first: true, ids[2]: false} {
		if _, err := os.Stat(objectPath(out, id)); (err == nil) != want {
			t.Errorf("%s in the bundle: %v, want %v", id[7:19], err == nil, want)
		}
	}
	// What a record is about, when that was corrected, stays home too: a
	// share holds only current records. The bundle still checks.
	run(t, dir, "", "add", "lab.tsh", "2.4", "-unit", "uIU/mL", "-corrects", ids[2])
	alone := filepath.Join(t.TempDir(), "alone")
	if stdout, errs, code := run(t, dir, "", "share", "-yes", "-out", alone, first); code != 0 || !strings.Contains(stdout, "shared 1 records") {
		t.Fatalf("%d\n%s\n%s", code, stdout, errs)
	}
	if stdout, _, code := run(t, t.TempDir(), "", "check-bundle", alone); code != 0 || !strings.Contains(stdout, "bundle ok") {
		t.Errorf("%d\n%s", code, stdout)
	}
}

func TestShareFilesAndComparison(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "DEMO_J")
	if out, errs, code := run(t, dir, "", "compare", "-unknown-files", "-sex", "male", "-age", "30-39", "-save", ids[0]); code != 0 {
		t.Fatalf("%s\n%s", out, errs)
	}
	ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "comparison.percentile")
	var cmp struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(strings.TrimSpace(ls)), &cmp)

	// A comparison comes with its reading and the records of its files.
	// Without -files the files are named, not shared.
	named := filepath.Join(t.TempDir(), "named")
	if out, errs, code := run(t, dir, "", "share", "-yes", "-out", named, cmp.ID); code != 0 || !strings.Contains(out, "shared 4 records and 0 files") {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(named, "files")); err == nil {
		t.Error("files were shared without -files")
	}
	if out, _, code := run(t, t.TempDir(), "", "check-bundle", named); code != 0 || !strings.Contains(out, "files: 0 included, 2 named and not included") {
		t.Errorf("%d\n%s", code, out)
	}
	// With -files each file is there in plaintext, named by its SHA-256.
	full := filepath.Join(t.TempDir(), "full")
	if out, errs, code := run(t, dir, "", "share", "-yes", "-files", "-out", full, cmp.ID); code != 0 || !strings.Contains(out, "shared 4 records and 2 files") {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	entries, _ := os.ReadDir(filepath.Join(full, "files"))
	if len(entries) != 2 {
		t.Fatalf("%d files", len(entries))
	}
	want, _ := os.ReadFile(refData + "TCHOL_J.XPT")
	found := false
	for _, e := range entries {
		got, _ := os.ReadFile(filepath.Join(full, "files", e.Name()))
		found = found || string(got) == string(want)
		if fileDigest(filepath.Join(full, "files", e.Name())) != e.Name() {
			t.Errorf("files/%s is not named by its SHA-256", e.Name())
		}
	}
	if !found {
		t.Error("the shared file is not the plaintext")
	}
	if out, _, code := run(t, t.TempDir(), "", "check-bundle", full); code != 0 || !strings.Contains(out, "files: 2 included, 0 named and not included") {
		t.Errorf("%d\n%s", code, out)
	}

	// Damage, one change at a time: each is refused.
	copyOf := func() string {
		d := filepath.Join(t.TempDir(), "d")
		if err := os.CopyFS(d, os.DirFS(full)); err != nil {
			t.Fatal(err)
		}
		return d
	}
	for name, change := range map[string]func(d string){
		"a record removed": func(d string) { os.Remove(objectPath(d, ids[0])) },
		"a record added": func(d string) {
			data, _ := os.ReadFile(objectPath(dir, ids[1]))
			os.MkdirAll(filepath.Dir(objectPath(d, ids[1])), 0o700)
			os.WriteFile(objectPath(d, ids[1]), data, 0o600)
		},
		"a value changed": func(d string) {
			data, _ := os.ReadFile(objectPath(d, ids[0]))
			os.WriteFile(objectPath(d, ids[0]), []byte(strings.Replace(string(data), `"n":193`, `"n":163`, 1)), 0o600)
		},
		"a file changed": func(d string) {
			p := filepath.Join(d, "files", entries[0].Name())
			data, _ := os.ReadFile(p)
			data[len(data)-200] ^= 1
			os.WriteFile(p, data, 0o600)
		},
		"a file removed": func(d string) { os.Remove(filepath.Join(d, "files", entries[0].Name())) },
		"a file added":   func(d string) { os.WriteFile(filepath.Join(d, "files", strings.Repeat("a", 64)), []byte("x"), 0o600) },
		"a link for a file": func(d string) {
			p := filepath.Join(d, "files", entries[0].Name())
			os.Remove(p)
			os.Symlink(refData+"TCHOL_J.XPT", p)
		},
	} {
		d := copyOf()
		change(d)
		if out, _, code := run(t, t.TempDir(), "", "check-bundle", d); code != 1 || !strings.Contains(out, "bundle FAILED") || strings.Contains(out, "bundle ok") {
			t.Errorf("%s: exit %d\n%s", name, code, out)
		}
	}
}

func TestShareRefuses(t *testing.T) {
	dir, _, ids := compareLog(t)
	out := filepath.Join(t.TempDir(), "bundle")
	newer, _, _ := run(t, dir, "", "add", "lab.cholesterol-total", "201", "-unit", "mg/dL", "-corrects", ids[0])
	run(t, dir, "", "purge", ids[1])
	full := filepath.Join(t.TempDir(), "full")
	os.MkdirAll(full, 0o700)
	os.WriteFile(filepath.Join(full, "x"), []byte("x"), 0o600)
	for name, c := range map[string]struct {
		args []string
		code int
		want string
	}{
		"no folder":          {[]string{"-yes", ids[2]}, 2, "usage"},
		"nothing chosen":     {[]string{"-yes", "-out", out}, 2, "usage"},
		"since without type": {[]string{"-yes", "-out", out, "-since", "2026-01-01", ids[2]}, 2, "usage"},
		"a corrected record": {[]string{"-yes", "-out", out, ids[0]}, 1, "cannot be shared: it is superseded"},
		"a deleted record":   {[]string{"-yes", "-out", out, ids[1]}, 1, "input missing"},
		"an unknown id":      {[]string{"-yes", "-out", out, "abcdef123456"}, 1, "input missing"},
		"no such type":       {[]string{"-yes", "-out", out, "-type", "cgm.*"}, 1, "no current record of type"},
		"a folder in use":    {[]string{"-yes", "-out", full, ids[2]}, 1, "is not empty"},
		"a file in the way":  {[]string{"-yes", "-out", filepath.Join(full, "x"), ids[2]}, 1, "bean:"},
		// A bundle inside the log would put plaintext among the records.
		"inside the log": {[]string{"-yes", "-out", filepath.Join(dir, "objects", "zz"), ids[2]}, 1, "is inside your log"},
		// A place that cannot be written: nothing is left behind, in the
		// log or on disk.
		"no parent folder": {[]string{"-yes", "-out", filepath.Join(full, "x", "y", "z"), ids[2]}, 1, "bean:"},
	} {
		stdout, errs, code := run(t, dir, "", append([]string{"share"}, c.args...)...)
		if code != c.code || !strings.Contains(stdout+errs, c.want) {
			t.Errorf("%s: exit %d, want %d and %q\n%s\n%s", name, code, c.code, c.want, stdout, errs)
		}
		if _, err := os.Stat(out); err == nil {
			t.Errorf("%s: a bundle was written", name)
			os.RemoveAll(out)
		}
	}
	if ls, _, _ := run(t, dir, "", "ls", "-type", "share"); ls != "" {
		t.Errorf("a refused share left a record:\n%s", ls)
	}
	// The correction can be shared, and a share record cannot.
	if stdout, errs, code := run(t, dir, "", "share", "-yes", "-out", out, strings.TrimSpace(newer)); code != 0 {
		t.Fatalf("%d\n%s\n%s", code, stdout, errs)
	}
	ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "share")
	var share struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(strings.TrimSpace(ls)), &share)
	if _, errs, code := run(t, dir, "", "share", "-yes", "-out", filepath.Join(t.TempDir(), "again"), share.ID); code != 1 || !strings.Contains(errs, "it is the record of a share") {
		t.Errorf("a share of a share: %d\n%s", code, errs)
	}
	// check-bundle refuses what is not a bundle.
	for name, d := range map[string]string{"an empty folder": t.TempDir(), "a log": dir, "no folder": filepath.Join(t.TempDir(), "none")} {
		if stdout, _, code := run(t, t.TempDir(), "", "check-bundle", d); code != 1 || !strings.Contains(stdout, "bundle FAILED") {
			t.Errorf("%s: %d\n%s", name, code, stdout)
		}
	}
	if !computedOnly("share") {
		t.Error("the MCP tool takes a share record")
	}
	// The log is as valid as before, and holds nothing in its folder that
	// a refused share left.
	if stdout, _, code := run(t, dir, "", "verify"); code != 0 {
		t.Errorf("verify after refused shares: %d\n%s", code, stdout)
	}
}
