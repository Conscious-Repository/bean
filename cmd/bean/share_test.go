package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/bundle"
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
	// A note about the report brings the report's record along, never its
	// file: only a record named by its id takes its file with it.
	note, _, _ := run(t, dir, "", "add", "note", "ask about this report", "-about", reportID(t, dir))
	about := filepath.Join(t.TempDir(), "about")
	stdout, errs, code = run(t, dir, "", "share", "-yes", "-files", "-out", about, strings.TrimSpace(note))
	if code != 0 || !strings.Contains(stdout, "shared 2 records and 0 files") || !strings.Contains(errs, "not shared: the file that") {
		t.Fatalf("a note about the report: %d\n%s\n%s", code, stdout, errs)
	}
	if _, err := os.Stat(filepath.Join(about, "files")); err == nil {
		t.Error("the lab report left with a note about it")
	}
	// Every bundle has a note for a person who has only the folder, and the
	// check does not read it.
	if readme, err := os.ReadFile(filepath.Join(about, "README.txt")); err != nil || !strings.Contains(string(readme), "bean check-bundle") {
		t.Errorf("no read-me in the bundle: %v", err)
	}
	// Named by its id, with -files, the report goes.
	named := filepath.Join(t.TempDir(), "named")
	if stdout, errs, code := run(t, dir, "", "share", "-yes", "-files", "-out", named, reportID(t, dir)); code != 0 || !strings.Contains(stdout, "shared 1 records and 1 files") {
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
	// A bundle with an empty file where the reader's identity record would
	// be is refused too: the record must be there whole.
	me, _, _ := run(t, sam, "", "whoami")
	samID := strings.Fields(me)[1]
	planted := filepath.Join(t.TempDir(), "planted")
	os.CopyFS(planted, os.DirFS(out))
	os.MkdirAll(filepath.Dir(objectPath(planted, samID)), 0o700)
	os.WriteFile(objectPath(planted, samID), nil, 0o600)
	if _, errs, code := run(t, sam, "", "sync", planted); code != 1 || !strings.Contains(errs, "is not a copy of your log") {
		t.Errorf("sync of a bundle with a planted file: %d\n%s", code, errs)
	}
	// A device's folder holds only what the device signed, and says whose it
	// is: that syncs.
	device := t.TempDir()
	if _, errs, code := run(t, device, "", "agent", "init", "-label", "ring", "-identity", samID); code != 0 {
		t.Fatalf("agent init -identity: %s", errs)
	}
	if _, errs, code := run(t, device, "", "add", "-agent", "heart.resting", "61", "-unit", "/min"); code != 0 {
		t.Fatalf("a device reading: %s", errs)
	}
	if stdout, errs, code := run(t, sam, "", "sync", device); code != 0 || !strings.Contains(stdout, "copied 1 records") {
		t.Errorf("sync of a device folder: %d\n%s\n%s", code, stdout, errs)
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

// reportID is the id of the log's one lab.report record.
func reportID(t *testing.T, dir string) string {
	t.Helper()
	ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "lab.report")
	var rep struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(ls)), &rep); err != nil || rep.ID == "" {
		t.Fatalf("no lab.report: %v\n%s", err, ls)
	}
	return rep.ID
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
	// Without -files the files are named, not shared. The files of this
	// test are synthetic: bean has no record of them, and says so.
	named := filepath.Join(t.TempDir(), "named")
	if out, errs, code := run(t, dir, "", "share", "-yes", "-out", named, cmp.ID); code != 0 || !strings.Contains(out, "shared 4 records and 0 files") || strings.Count(errs, "it is not a file that bean has on record") != 2 {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(named, "files")); err == nil {
		t.Error("files were shared without -files")
	}
	if out, _, code := run(t, t.TempDir(), "", "check-bundle", named); code != 0 || !strings.Contains(out, "files: 0 included, 2 named and not included") {
		t.Errorf("%d\n%s", code, out)
	}
	// With -files alone they stay home too: a file that is not on record
	// goes only when the person names its record.
	typed := filepath.Join(t.TempDir(), "typed")
	if out, errs, code := run(t, dir, "", "share", "-yes", "-files", "-out", typed, cmp.ID); code != 0 || !strings.Contains(out, "shared 4 records and 0 files") || strings.Count(errs, "it is not a file that bean has on record") != 2 {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(typed, "files")); err == nil {
		t.Error("a file that is not on record was shared, and nobody named its record")
	}
	// With -files and its record named, each file is there in plaintext,
	// named by its SHA-256.
	full := filepath.Join(t.TempDir(), "full")
	if out, errs, code := run(t, dir, "", append([]string{"share", "-yes", "-files", "-out", full, cmp.ID}, fileRecordIDs(t, dir)...)...); code != 0 || !strings.Contains(out, "shared 4 records and 2 files") {
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

// fileDigest is the SHA-256 of a file, or "" when it cannot be read.
func fileDigest(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestBundleFromMemory checks a bundle that is not on a disk, as the page
// that checks one in a browser gets it: files in memory, or a zip file. It
// must read as its folder reads.
func TestBundleFromMemory(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "DEMO_J")
	out := filepath.Join(t.TempDir(), "for-ana")
	if stdout, errs, code := run(t, dir, "", "compare", "-unknown-files", "-sex", "male", "-age", "30-39", "-save", ids[0]); code != 0 {
		t.Fatalf("%s\n%s", stdout, errs)
	}
	// A comparison with its reading, the records of its two files, and the
	// files themselves.
	if stdout, errs, code := run(t, dir, "", append([]string{"share", "-yes", "-files", "-note", "for Ana", "-out", out, "-type", "comparison.*"}, fileRecordIDs(t, dir)...)...); code != 0 || !strings.Contains(stdout, "shared 4 records and 2 files") {
		t.Fatalf("%d\n%s\n%s", code, stdout, errs)
	}
	want, ok := checkBundle(out)
	if !ok {
		t.Fatalf("the folder fails: %v", want)
	}
	// The folder's files, under the folder's own name.
	files := map[string][]byte{}
	filepath.WalkDir(out, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			rel, _ := filepath.Rel(filepath.Dir(out), path)
			files[filepath.ToSlash(rel)], _ = os.ReadFile(path)
		}
		return nil
	})
	zipOf := func(files map[string][]byte, more func(*zip.Writer)) []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for _, name := range slices.Sorted(maps.Keys(files)) {
			w, _ := zw.Create(name)
			w.Write(files[name])
		}
		if more != nil {
			more(zw)
		}
		zw.Close()
		return buf.Bytes()
	}
	same := func(name string, res bundle.Result) {
		t.Helper()
		if got := res.Lines(); !res.OK || !slices.Equal(got, want) {
			t.Errorf("%s does not read as its folder:\n%s", name, strings.Join(got, "\n"))
		}
	}
	same("files in memory", bundle.Check(bundle.FromFiles("for-ana", files)))
	same("a zip file", bundle.Check(bundle.FromZip("for-ana.zip", zipOf(files, nil))))
	if res := bundle.Check(bundle.FromFiles("for-ana", files)); res.Note != "for Ana" || len(res.Rows) != 4 {
		t.Errorf("note %q, %d rows", res.Note, len(res.Rows))
	}
	// A zip made on a Mac carries a tree of its own and files that start
	// with a dot. Neither is part of the bundle.
	same("a zip file from a Mac", bundle.Check(bundle.FromZip("for-ana.zip", zipOf(files, func(zw *zip.Writer) {
		for name := range files {
			w, _ := zw.Create("__MACOSX/" + filepath.Dir(name) + "/._" + filepath.Base(name))
			w.Write([]byte("x"))
		}
		w, _ := zw.Create("for-ana/.DS_Store")
		w.Write([]byte("x"))
	}))))
	// The bundle's own files, with no folder around them.
	bare := map[string][]byte{}
	for name, data := range files {
		bare[strings.TrimPrefix(name, "for-ana/")] = data
	}
	same("files with no folder around them", bundle.Check(bundle.FromFiles("for-ana", bare)))

	fails := func(name string, res bundle.Result, word string) {
		t.Helper()
		if text := strings.Join(res.Lines(), "\n"); res.OK || res.Rows != nil || res.Note != "" || !strings.Contains(text, word) {
			t.Errorf("%s: ok %v, want %q in:\n%s", name, res.OK, word, text)
		}
	}
	change := func(edit func(m map[string][]byte)) map[string][]byte {
		m := maps.Clone(files)
		edit(m)
		return m
	}
	var record, file string
	for name := range files {
		if strings.HasPrefix(name, "for-ana/objects/") && strings.Contains(string(files[name]), `"type":"lab.cholesterol-total"`) {
			record = name
		}
		if strings.HasPrefix(name, "for-ana/files/") {
			file = name
		}
	}
	if record == "" || file == "" {
		t.Fatalf("no record or no file in the bundle: %q %q", record, file)
	}
	fails("a record removed", bundle.Check(bundle.FromFiles("x", change(func(m map[string][]byte) { delete(m, record) }))), "is listed, and is not in the bundle")
	fails("a record changed", bundle.Check(bundle.FromFiles("x", change(func(m map[string][]byte) {
		m[record] = bytes.Replace(m[record], []byte(`"n":193`), []byte(`"n":139`), 1)
	}))), "fails a check of a log")
	fails("a file changed", bundle.Check(bundle.FromZip("x", zipOf(change(func(m map[string][]byte) { m[file] = append(slices.Clone(m[file]), 'x') }), nil))), "is not the file the share record lists")
	fails("a file added", bundle.Check(bundle.FromFiles("x", change(func(m map[string][]byte) { m["for-ana/files/extra"] = []byte("x") }))), "does not list it")
	fails("a folder where a file belongs", bundle.Check(bundle.FromFiles("x", change(func(m map[string][]byte) { m[file+"/inside"] = m[file]; delete(m, file) }))), "is not a plain file")
	fails("a file where a record's folder belongs", bundle.Check(bundle.FromFiles("x", change(func(m map[string][]byte) { m["for-ana/objects/stray"] = []byte("x") }))), "objects/stray is not a record file")
	fails("a record too large to be one", bundle.Check(bundle.FromFiles("x", change(func(m map[string][]byte) { m["for-ana/objects/ab/big.json"] = make([]byte, core.MaxSuccessionSize+1) }))), "objects/ab/big.json is not a record file")
	fails("a file named files", bundle.Check(bundle.FromFiles("x", change(func(m map[string][]byte) {
		for name := range m {
			if strings.HasPrefix(name, "for-ana/files/") {
				delete(m, name)
			}
		}
		m["for-ana/files"] = []byte("x")
	}))), "files is not a folder")
	// The files are the bundle's own folder, or one folder that is the
	// bundle. Anything beside it is refused, not passed over: a reader
	// could take it for part of what was checked.
	for _, beside := range []string{"other/objects/ab/cd.json", "REPORT.pdf", "elsewhere/files/x", "For-Ana/files/x"} {
		fails("beside the bundle: "+beside, bundle.Check(bundle.FromFiles("x", change(func(m map[string][]byte) { m[beside] = []byte("{}") }))), "choose the bundle's own folder")
	}
	fails("no bundle", bundle.Check(bundle.FromFiles("x", map[string][]byte{"notes.txt": []byte("x")})), "is not a bundle")
	fails("a folder with no objects", bundle.Check(bundle.FromFiles("x", map[string][]byte{"for-ana/notes.txt": []byte("x")})), "has no objects folder")
	// A name that no folder can hold reads one way here and another way
	// once it is unpacked, so it is refused.
	for _, odd := range []string{"for-ana/files/./x", "for-ana/./files/x", "for-ana/../for-ana/files/x", "for-ana/objects/../files/x", "/for-ana/files/x", "for-ana//files/x", "for-ana/objects/zz/./evil.json", "./for-ana/files/x", "../x"} {
		fails("an odd name: "+odd, bundle.Check(bundle.FromZip("x.zip", zipOf(files, func(zw *zip.Writer) {
			w, _ := zw.Create(odd)
			w.Write([]byte("{}"))
		}))), "no folder can hold")
	}
	// A zip entry can state a size that is no size.
	fails("a size that is no size", bundle.Check(bundle.FromZip("x.zip", zipOf(files, func(zw *zip.Writer) {
		zw.CreateRaw(&zip.FileHeader{Name: "for-ana/objects/ab/huge.json", UncompressedSize64: 1 << 63})
	}))), "no folder can hold")
	// A second copy of the bundle inside its folder is not part of it. It
	// is named, so that a reader does not take it for checked.
	inside := bundle.Check(bundle.FromFiles("x", change(func(m map[string][]byte) {
		for name, data := range files {
			m["for-ana/copy/"+strings.TrimPrefix(name, "for-ana/")] = bytes.ToUpper(data)
		}
	})))
	if !inside.OK || !slices.Contains(inside.Ignored, "copy") {
		t.Errorf("a copy inside the bundle's folder: ok %v, not part of it %v", inside.OK, inside.Ignored)
	}
	fails("not a zip file", bundle.Check(bundle.FromZip("x.zip", []byte("not a zip"))), "cannot be read as a zip file")
	// Two entries with one name: which one a reader sees depends on the
	// tool that reads the zip, so neither is read.
	fails("a zip file with one name twice", bundle.Check(bundle.FromZip("x.zip", zipOf(files, func(zw *zip.Writer) {
		w, _ := zw.Create(record)
		w.Write([]byte("{}"))
	}))), "two files with one name")
	// What lies in the bundle's folder under a name the bundle does not use
	// is named and not read, and changes nothing.
	extra := bundle.Check(bundle.FromFiles("x", change(func(m map[string][]byte) { m["for-ana/local/identity"] = []byte("x") })))
	if text := strings.Join(extra.Lines(), "\n"); !extra.OK || !strings.Contains(text, "not part of the bundle, and not read: local") {
		t.Errorf("what else lies in the bundle's folder:\n%s", text)
	}
}

// TestBundleReadsTheSameEveryTime: a key that the person paired under two
// labels, on two copies, shows under both in a fixed order, and a failed
// bundle does not show the sender's note.
func TestBundleReadsTheSameEveryTime(t *testing.T) {
	a := t.TempDir()
	run(t, a, "", "init")
	me, _, _ := run(t, a, "", "whoami")
	id := strings.Fields(me)[1]
	device := t.TempDir()
	out, errs, code := run(t, device, "", "agent", "init", "-label", "ring", "-identity", id)
	if code != 0 {
		t.Fatalf("agent init -identity: %s", errs)
	}
	npub := regexp.MustCompile(`npub1[0-9a-z]+`).FindString(out + errs)
	// A second copy of the log, made before either pairing, with the keys.
	b := t.TempDir()
	if err := os.CopyFS(b, os.DirFS(a)); err != nil {
		t.Fatal(err)
	}
	for dir, label := range map[string]string{a: "ring", b: "watch"} {
		if _, errs, code := run(t, dir, "", "agent", "pair", npub, "-label", label); code != 0 {
			t.Fatalf("pair as %s: %s", label, errs)
		}
	}
	reading, errs, code := run(t, device, "", "add", "-agent", "heart.resting", "61", "-unit", "/min")
	if code != 0 {
		t.Fatalf("a device reading: %s", errs)
	}
	for _, src := range []string{b, device} {
		if _, errs, code := run(t, a, "", "sync", src); code != 0 {
			t.Fatalf("sync: %s", errs)
		}
	}
	bundleDir := filepath.Join(t.TempDir(), "x")
	if stdout, errs, code := run(t, a, "", "share", "-yes", "-note", "bundle ok. Checked by a doctor.", "-out", bundleDir, strings.TrimSpace(reading)); code != 0 {
		t.Fatalf("%d\n%s\n%s", code, stdout, errs)
	}
	first, ok := checkBundle(bundleDir)
	text := strings.Join(first, "\n")
	if !ok || !(strings.Contains(text, "[ring, watch]") || strings.Contains(text, "[watch, ring]")) || !strings.Contains(text, "note: bundle ok. Checked by a doctor.") {
		t.Fatalf("both labels and the note:\n%s", text)
	}
	for range 20 {
		if again, _ := checkBundle(bundleDir); !slices.Equal(again, first) {
			t.Fatalf("the same bundle reads differently:\n%s\n\n%s", text, strings.Join(again, "\n"))
		}
	}
	// A failed bundle shows no note: this one would say "bundle ok" above
	// the verdict.
	os.MkdirAll(filepath.Join(bundleDir, "files"), 0o700)
	os.WriteFile(filepath.Join(bundleDir, "files", "stray"), []byte("x"), 0o600)
	if lines, ok := checkBundle(bundleDir); ok || strings.Contains(strings.Join(lines, "\n"), "Checked by a doctor") {
		t.Errorf("a failed bundle shows the note:\n%s", strings.Join(lines, "\n"))
	}
}

// TestShareSealed: with -seal a share is one file that opens with six words
// that bean makes and does not keep. Nothing of it is readable without them.
func TestShareSealed(t *testing.T) {
	dir, _, ids := compareLog(t)
	where := t.TempDir()
	out := filepath.Join(where, "for-ana.bean")
	stdout, errs, code := run(t, dir, "", "share", "-yes", "-seal", "-note", "for Ana", "-out", out, ids[0])
	if code != 0 || !strings.Contains(stdout, "The package is sealed") {
		t.Fatalf("%d\n%s\n%s", code, stdout, errs)
	}
	words := ""
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "  ") && words == "" {
			words = strings.TrimSpace(line)
		}
	}
	if len(strings.Fields(words)) != 6 {
		t.Fatalf("no six words in:\n%s", stdout)
	}
	data, err := os.ReadFile(out)
	if err != nil || !bundle.IsSealed(data) {
		t.Fatalf("no sealed file: %v", err)
	}
	// The file shows no id, no type and no value, and the working folder is gone.
	for _, clear := range []string{ids[0][7:], "lab.cholesterol-total", "for Ana", "objects/"} {
		if bytes.Contains(data, []byte(clear)) {
			t.Errorf("the sealed file shows %q in the clear", clear)
		}
	}
	if entries, _ := os.ReadDir(where); len(entries) != 1 {
		t.Errorf("the share left %d entries beside the package", len(entries)-1)
	}
	// The words open it, for bean check-bundle too.
	// A person may write the words in capitals, or with hyphens or commas.
	if res := bundle.Check(bundle.FromPackage("p", data, strings.ReplaceAll(words, " ", "-"))); !res.OK {
		t.Errorf("the words with hyphens do not open it: %v", res.Lines())
	}
	if res := bundle.Check(bundle.FromPackage("p", data, strings.ToUpper(words)+"  ")); !res.OK || len(res.Rows) != 1 || res.Note != "for Ana" {
		t.Errorf("the words do not open it: %v", res.Lines())
	}
	// A full stop after the last word is how a sentence ends. An opened
	// package says that words opened it, and names the signer in words.
	res := bundle.Check(bundle.FromPackage("p", data, strings.ReplaceAll(words, " ", ", ")+"."))
	if !res.OK || !res.Opened || res.Locked || len(strings.Fields(res.Words)) != 8 || res.Words == words {
		t.Errorf("the words with commas and a full stop: %v", res.Lines())
	}
	if res := bundle.Check(bundle.FromPackage("p", data, "abandon ability able about above absent")); res.OK || !res.Locked || !res.Unread || res.Words != "" {
		t.Errorf("other words: locked %v, unread %v", res.Locked, res.Unread)
	}
	if stdout, _, code := run(t, t.TempDir(), words+"\n", "check-bundle", out); code != 0 || !strings.Contains(stdout, "bundle ok") || !strings.Contains(stdout, "identity words: "+res.Words) {
		t.Errorf("check-bundle with the words: %d\n%s", code, stdout)
	}
	// Nothing else does: other words, a misspelt word, no words, a changed byte.
	for name, try := range map[string]string{
		"other words":    "abandon ability able about above absent",
		"a misspelt one": "abandn ability able about above absent",
		"five words":     "abandon ability able about above",
		"no words":       "",
	} {
		if res := bundle.Check(bundle.FromPackage("p", data, try)); res.OK || !res.Unread || res.Rows != nil {
			t.Errorf("%s opened the package", name)
		}
	}
	changed := slices.Clone(data)
	changed[len(changed)-20] ^= 1
	if res := bundle.Check(bundle.FromPackage("p", changed, words)); res.OK || res.Rows != nil {
		t.Error("a changed package opened")
	}
	if stdout, _, code := run(t, t.TempDir(), "abandon ability able about above absent\n", "check-bundle", out); code != 1 || strings.Contains(stdout, "It holds") {
		t.Errorf("check-bundle with other words: %d\n%s", code, stdout)
	}
	// The log says that the share left sealed, and holds no word of the six.
	ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "share")
	if !strings.Contains(ls, `"seal":"passphrase"`) || strings.Contains(ls, strings.Fields(words)[0]+" "+strings.Fields(words)[1]) {
		t.Errorf("the share record: %s", ls)
	}
	// A package is a new file: it replaces nothing.
	if _, errs, code := run(t, dir, "", "share", "-yes", "-seal", "-out", out, ids[0]); code == 0 || !strings.Contains(errs, "is there already") {
		t.Errorf("a second share onto the package: %d\n%s", code, errs)
	}
	// Two packages of the same records have different words.
	other := filepath.Join(where, "again.bean")
	stdout2, _, _ := run(t, dir, "", "share", "-yes", "-seal", "-out", other, ids[0])
	if strings.Contains(stdout2, words) {
		t.Error("two packages got the same words")
	}
	// A zip file of a bundle's folder is read as its folder is.
	folder := filepath.Join(where, "plain")
	run(t, dir, "", "share", "-yes", "-out", folder, ids[0])
	packed, err := bundle.Pack(folder)
	if err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(where, "plain.zip")
	os.WriteFile(zipPath, packed, 0o600)
	want, _ := checkBundle(folder)
	if got, ok, _ := checkPackage(zipPath); !ok || len(got) != len(want) {
		t.Errorf("a zip file of the folder:\n%s", strings.Join(got, "\n"))
	}
	if again, _ := bundle.Pack(folder); !bytes.Equal(again, packed) {
		t.Error("the same folder packs to different bytes")
	}
}

// TestCorrectOnce: a record is corrected once. A second correction of the
// same record would leave two current values, so bean names the record to
// correct instead. A correction with no -at says that it is dated now.
func TestCorrectOnce(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "", "init")
	first, _, _ := run(t, dir, "", "add", "weight", "81.2", "-unit", "kg", "-at", "2026-10-27 07:15")
	first = strings.TrimSpace(first)
	second, errs, code := run(t, dir, "", "add", "weight", "80.4", "-unit", "kg", "-corrects", first)
	if code != 0 || !strings.Contains(errs, "this correction is dated now") || !strings.Contains(errs, "2026-10-27") {
		t.Fatalf("a correction with no -at: %d\n%s", code, errs)
	}
	second = strings.TrimSpace(second)
	if _, errs, code := run(t, dir, "", "add", "weight", "80.1", "-unit", "kg", "-corrects", first, "-at", "2026-10-27 07:15"); code == 0 || !strings.Contains(errs, "was corrected already, by "+second[7:19]) {
		t.Errorf("a second correction of one record: %d\n%s", code, errs)
	}
	// The record that replaced it can be corrected, and with -at nothing is noted.
	if _, errs, code := run(t, dir, "", "add", "weight", "80.1", "-unit", "kg", "-corrects", second, "-at", "2026-10-27 07:15"); code != 0 || strings.Contains(errs, "dated now") {
		t.Errorf("a correction of the correction: %d\n%s", code, errs)
	}
	if ls, _, _ := run(t, dir, "", "ls", "-type", "weight"); strings.Count(ls, "weight") != 1 || !strings.Contains(ls, "80.1 kg") {
		t.Errorf("one current weight is wanted:\n%s", ls)
	}
	// An id that names nothing says so.
	if _, errs, code := run(t, dir, "", "show", "d"); code == 0 || !strings.Contains(errs, "first 6 or more hex digits") {
		t.Errorf("a short id: %d\n%s", code, errs)
	}
	if _, errs, code := run(t, dir, "", "show", "000000"); code == 0 || !strings.Contains(errs, "no record has this id") {
		t.Errorf("an id of no record: %d\n%s", code, errs)
	}
}

// TestSealedFilesComeOut: a reader gets a carried file out of a sealed
// package with check-bundle -unpack, and the folder it writes checks as a
// bundle. A sealed share writes the package and nothing else.
func TestSealedFilesComeOut(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "DEMO_J")
	if out, errs, code := run(t, dir, "", "compare", "-unknown-files", "-sex", "male", "-age", "30-39", "-save", ids[0]); code != 0 {
		t.Fatalf("%s\n%s", out, errs)
	}
	where := t.TempDir()
	pkg := filepath.Join(where, "p.bean")
	stdout, errs, code := run(t, dir, "", append([]string{"share", "-yes", "-seal", "-files", "-out", pkg, "-type", "comparison.*"}, fileRecordIDs(t, dir)...)...)
	if code != 0 || !strings.Contains(stdout, "shared 4 records and 2 files") {
		t.Fatalf("%d\n%s\n%s", code, stdout, errs)
	}
	// No folder of plain records was made beside the package.
	if entries, _ := os.ReadDir(where); len(entries) != 1 || entries[0].Name() != "p.bean" {
		t.Errorf("a sealed share wrote more than the package: %v", entries)
	}
	words := ""
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "  ") && words == "" {
			words = strings.TrimSpace(line)
		}
	}
	data, _ := os.ReadFile(pkg)
	res := bundle.Check(bundle.FromPackage("p", data, words))
	if !res.OK || len(res.Carried) != 2 || res.Carried[0].Type != "reference.file" || res.Carried[0].Bytes == 0 {
		t.Fatalf("the files a package carries: %+v", res.Carried)
	}
	// With the words, -unpack writes the bundle, and its files are the files.
	open := filepath.Join(where, "open")
	if stdout, errs, code := run(t, t.TempDir(), words+"\n", "check-bundle", "-unpack", open, pkg); code != 0 || !strings.Contains(stdout, "unpacked into") {
		t.Fatalf("-unpack: %d\n%s\n%s", code, stdout, errs)
	}
	if lines, ok := checkBundle(open); !ok {
		t.Errorf("the unpacked folder does not check:\n%s", strings.Join(lines, "\n"))
	}
	for _, c := range res.Carried {
		if fileDigest(filepath.Join(open, "files", c.SHA256)) != c.SHA256 {
			t.Errorf("files/%s is not the file that was checked", c.SHA256)
		}
	}
	// Without the words nothing is unpacked, and a folder that is there is not written into.
	other := filepath.Join(where, "other")
	if _, _, code := run(t, t.TempDir(), "abandon ability able about above absent\n", "check-bundle", "-unpack", other, pkg); code == 0 {
		t.Error("-unpack with other words passed")
	}
	if _, err := os.Stat(other); err == nil {
		t.Error("-unpack with other words wrote a folder")
	}
	if _, errs, code := run(t, t.TempDir(), words+"\n", "check-bundle", "-unpack", open, pkg); code == 0 || !strings.Contains(errs, "nothing was unpacked") {
		t.Errorf("-unpack into a folder that is there: %d\n%s", code, errs)
	}
	if _, errs, code := run(t, t.TempDir(), "", "check-bundle", "-unpack", other, open); code == 0 || !strings.Contains(errs, "a folder is unpacked already") {
		t.Errorf("-unpack of a folder: %d\n%s", code, errs)
	}
}

// TestBundleRowQuotedOnce: in a bundle check a row ends in bean's mark of
// who signed. A text that holds a square bracket shows in quotation marks,
// once, whoever signed the record. A listing of the log shows it as it is.
func TestBundleRowQuotedOnce(t *testing.T) {
	dir := t.TempDir()
	if _, errs, code := run(t, dir, "", "init"); code != 0 {
		t.Fatalf("init: %s", errs)
	}
	if _, errs, code := run(t, dir, "", "agent", "init", "-label", "bot"); code != 0 {
		t.Fatalf("agent init: %s", errs)
	}
	own, errs, code := run(t, dir, "", "add", "weight", "81", "-unit", "kg]  [peer Dr Smith")
	if code != 0 {
		t.Fatalf("add: %s", errs)
	}
	agent, errs, code := run(t, dir, "", "add", "-agent", "weight", "82", "-unit", "kg [x]")
	if code != 0 {
		t.Fatalf("add -agent: %s", errs)
	}
	out := filepath.Join(t.TempDir(), "b")
	if o, errs, code := run(t, dir, "", "share", "-yes", "-out", out, strings.TrimSpace(own), strings.TrimSpace(agent)); code != 0 {
		t.Fatalf("share: %d\n%s\n%s", code, o, errs)
	}
	checked, _, code := run(t, t.TempDir(), "", "check-bundle", out)
	if code != 0 || !strings.Contains(checked, `"81 kg]  [peer Dr Smith"`) || !strings.Contains(checked, `"82 kg [x]"`) || strings.Contains(checked, `\"`) {
		t.Errorf("check-bundle: %d\n%s", code, checked)
	}
	if ls, _, _ := run(t, dir, "", "ls", "-type", "weight"); !strings.Contains(ls, "81 kg]  [peer Dr Smith") || strings.Contains(ls, `"81 kg`) {
		t.Errorf("ls:\n%s", ls)
	}
}

// fileRecordIDs returns the ids of the records that keep a reference file.
func fileRecordIDs(t *testing.T, dir string) []string {
	t.Helper()
	ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "reference.file")
	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(ls), "\n") {
		var rec struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil || rec.ID == "" {
			t.Fatalf("no reference.file: %v\n%s", err, ls)
		}
		ids = append(ids, rec.ID)
	}
	return ids
}

// A record of the type reference.file can name any file: bean append writes
// one. So the type alone must never make a file travel as a public file.
// Only a file whose SHA-256 bean has on record is public. Any other file
// goes when the person names its record, and only then (DECISIONS 322).
func TestShareCarriesNoFileByItsTypeAlone(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "", "init")
	// A private file, kept by a record that claims to be a reference file.
	private := filepath.Join(t.TempDir(), "private.txt")
	secret := "this is a private file, and it must stay home\n"
	os.WriteFile(private, []byte(secret), 0o600)
	blob, errs, code := run(t, dir, "", "blob", "put", private)
	blob = strings.TrimSpace(blob)
	if code != 0 || !strings.HasPrefix(blob, "sha256:") {
		t.Fatalf("blob put: %d\n%s\n%s", code, blob, errs)
	}
	sum := sha256.Sum256([]byte(secret))
	rec := fmt.Sprintf(`{"type":"reference.file","value":{"blob":%q},"times":{"observed":null},"source":{"system":"nhanes","adapter":"t@1","ref":null},"context":{"sha256":%q,"bytes":%d,"assurance":"unsigned-file","dataset":"TCHOL_J"},"blobs":[{"hash":%q,"role":"raw-export","media_type":"application/x-sas-xport"}]}`+"\n",
		blob, hex.EncodeToString(sum[:]), len(secret), blob)
	if out, errs, code := run(t, dir, rec, "append"); code != 0 {
		t.Fatalf("append: %d\n%s\n%s", code, out, errs)
	}
	keeper := fileRecordIDs(t, dir)[0]
	note, _, _ := run(t, dir, "", "add", "note", "see this file", "-about", keeper)
	note = strings.TrimSpace(note)

	// A note about that record brings the record along, and never its file.
	about := filepath.Join(t.TempDir(), "about")
	stdout, errs, code := run(t, dir, "", "share", "-yes", "-files", "-out", about, note)
	if code != 0 || !strings.Contains(stdout, "shared 2 records and 0 files") {
		t.Fatalf("a note about a file that is not on record: %d\n%s\n%s", code, stdout, errs)
	}
	if !strings.Contains(errs, "it is not a file that bean has on record") {
		t.Errorf("the share does not say why the file stays home:\n%s", errs)
	}
	if _, err := os.Stat(filepath.Join(about, "files")); err == nil {
		t.Error("a private file left as a public file, and nobody named its record")
	}
	// Named by its id, with -files, it goes: the person chose it.
	named := filepath.Join(t.TempDir(), "named")
	if stdout, errs, code := run(t, dir, "", "share", "-yes", "-files", "-out", named, note, keeper); code != 0 || !strings.Contains(stdout, "shared 2 records and 1 files") {
		t.Fatalf("named by its id: %d\n%s\n%s", code, stdout, errs)
	}
}

// A record can state the SHA-256 of a file on record and keep other bytes.
// Such a file is public by what the record says, so the share must hold the
// bytes to that SHA-256, and send nothing when they differ.
func TestShareHoldsAPublicFileToItsDigest(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "", "init")
	private := filepath.Join(t.TempDir(), "private.txt")
	os.WriteFile(private, []byte("not the survey's file\n"), 0o600)
	blob, _, _ := run(t, dir, "", "blob", "put", private)
	blob = strings.TrimSpace(blob)
	onRecord := "0291a6a4f6d82dac8392c8c992b519946824dfeda01647935520cf506ed9f4ab" // TCHOL_J
	rec := fmt.Sprintf(`{"type":"reference.file","value":{"blob":%q},"times":{"observed":null},"source":{"system":"nhanes","adapter":"t@1","ref":null},"context":{"sha256":%q,"bytes":22,"assurance":"unsigned-file","dataset":"TCHOL_J"},"blobs":[{"hash":%q,"role":"raw-export","media_type":"application/x-sas-xport"}]}`+"\n", blob, onRecord, blob)
	if out, errs, code := run(t, dir, rec, "append"); code != 0 {
		t.Fatalf("append: %d\n%s\n%s", code, out, errs)
	}
	note, _, _ := run(t, dir, "", "add", "note", "see this file", "-about", fileRecordIDs(t, dir)[0])
	out := filepath.Join(t.TempDir(), "out")
	stdout, errs, code := run(t, dir, "", "share", "-yes", "-files", "-out", out, strings.TrimSpace(note))
	if code == 0 || !strings.Contains(errs, "does not match the SHA-256 in its record") {
		t.Fatalf("a file that is not what its record states: %d\n%s\n%s", code, stdout, errs)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("something was written")
	}
	// A record that names two files states no SHA-256 for either, whatever
	// its context says. So neither is public.
	two := fmt.Sprintf(`{"type":"reference.file","value":{"blob":%q},"times":{"observed":null},"source":{"system":"nhanes","adapter":"t@2","ref":null},"context":{"sha256":%q,"bytes":22,"assurance":"unsigned-file","dataset":"TCHOL_J"},"blobs":[{"hash":%q,"role":"raw-export","media_type":"application/x-sas-xport"},{"hash":%q,"role":"other","media_type":"text/plain"}]}`+"\n", blob, onRecord, blob, blob)
	if out, errs, code := run(t, dir, two, "append"); code != 0 {
		t.Fatalf("append: %d\n%s\n%s", code, out, errs)
	}
	var twoID string
	for _, id := range fileRecordIDs(t, dir) {
		if shown, _, _ := run(t, dir, "", "show", id); strings.Contains(shown, "t@2") {
			twoID = id
		}
	}
	if twoID == "" {
		t.Fatal("the record with two files is not in the log")
	}
	note2, _, _ := run(t, dir, "", "add", "note", "and this one", "-about", twoID)
	out2 := filepath.Join(t.TempDir(), "out2")
	stdout, errs, code = run(t, dir, "", "share", "-yes", "-files", "-out", out2, strings.TrimSpace(note2))
	if code != 0 || !strings.Contains(stdout, "shared 2 records and 0 files") {
		t.Fatalf("a record that names two files: %d\n%s\n%s", code, stdout, errs)
	}
	if _, err := os.Stat(filepath.Join(out2, "files")); err == nil {
		t.Error("a file of a record that names two files left as a public file")
	}
}

// With BEAN_NHANES_DIR set to a folder of CDC's own files: a kept file that
// bean has on record is public, and goes with -files and the comparison
// that read it, with no record named.
func TestRealFileOnRecordTravels(t *testing.T) {
	data := os.Getenv("BEAN_NHANES_DIR")
	if data == "" {
		t.Skip("BEAN_NHANES_DIR is not set")
	}
	path := importerOnPath(t)
	dir := t.TempDir()
	run(t, dir, "", "init")
	for _, f := range []string{"TCHOL_J", "DEMO_J"} {
		if out, errs, code := runPath(t, dir, path, "import", "-yes", "nhanes", filepath.Join(data, f+".XPT")); code != 0 || !strings.Contains(out, "kept reference.file") {
			t.Fatalf("import %s: %d\n%s\n%s", f, code, out, errs)
		}
	}
	reading, _, _ := run(t, dir, "", "add", "lab.cholesterol-total", "193", "-unit", "mg/dL", "-at", "2026-11-01")
	if out, errs, code := run(t, dir, "", "compare", "-sex", "male", "-age", "30-39", "-save", strings.TrimSpace(reading)); code != 0 {
		t.Fatalf("%s\n%s", out, errs)
	}
	home := filepath.Join(t.TempDir(), "home")
	if out, errs, code := run(t, dir, "", "share", "-yes", "-out", home, "-type", "comparison.*"); code != 0 || !strings.Contains(out, "shared 4 records and 0 files") || !strings.Contains(errs, "not shared: the 2 public files") {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	full := filepath.Join(t.TempDir(), "full")
	if out, errs, code := run(t, dir, "", "share", "-yes", "-files", "-out", full, "-type", "comparison.*"); code != 0 || !strings.Contains(out, "shared 4 records and 2 files") {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	if out, _, code := run(t, t.TempDir(), "", "check-bundle", full); code != 0 || !strings.Contains(out, "files: 2 included, 0 named and not included") {
		t.Errorf("%d\n%s", code, out)
	}
}
