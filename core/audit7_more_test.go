package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regression tests for the findings next to audit7_test.go (DECISIONS 206 to
// 208). Each failed before its fix.

var testPNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

func readManifest(t *testing.T, dir string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	v, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return v.(map[string]any)
}

// A6: the stranger's blob is listed as not exported, with a reason, and the
// person's own photo still goes out.
func TestAudit7ExportListsStrangerBlob(t *testing.T) {
	s, id, _ := initWithWords(t)
	photo, _ := s.Add("photo", "rash", AddOptions{Photo: []byte("SENSITIVE"), PhotoType: "image/jpeg"})
	keep, _ := s.Add("photo", "mole", AddOptions{Photo: []byte("KEEP"), PhotoType: "image/jpeg"})
	l, _ := s.Load()
	hash := l.Report.Records[photo].Blobs[0].Hash
	k := reKey(t)
	o := reClaim(k, k.Public(), id, "note", map[string]any{"s": "x"})
	o["blobs"] = []any{map[string]any{"hash": hash, "role": "x", "media_type": "image/jpeg"}}
	rePlant(t, s, o, k)
	if _, err := s.Purge(photo, ""); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "out")
	res, err := s.ExportAll(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.NotExported[hash] == "" || res.Blobs != 1 {
		t.Errorf("export: %+v", res)
	}
	man := readManifest(t, dir)
	if ne, _ := man["not_exported_blobs"].([]any); len(ne) != 1 || !strings.Contains(string(Canonical(ne)), "not yours") {
		t.Errorf("manifest: %v", man["not_exported_blobs"])
	}
	kh := l.Report.Records[keep].Blobs[0].Hash
	if pt, err := os.ReadFile(filepath.Join(dir, "blobs", kh[7:])); err != nil || string(pt) != "KEEP" {
		t.Errorf("own photo: %q %v", pt, err)
	}
}

// E1: a source folder that is not a log is an error, not "copied 0".
func TestAudit7SyncNeedsALog(t *testing.T) {
	s, _ := initStore(t)
	for _, dir := range []string{filepath.Join(t.TempDir(), "typo"), t.TempDir()} {
		if _, err := s.Sync(Open(dir)); err == nil || !strings.HasPrefix(err.Error(), "no log in ") {
			t.Errorf("sync from %s: %v", dir, err)
		}
	}
}

// E2: sync counts what it skipped in a damaged source.
func TestAudit7SyncCountsDamage(t *testing.T) {
	backup, _ := initStore(t)
	backup.Add("meal", "lunch", AddOptions{Photo: testPNG, PhotoType: "image/png"})
	w, _ := backup.Add("weight", "80", AddOptions{Unit: "kg"})
	bf, _, _ := listShards(backup.Dir, dirBlobs)
	f, _ := os.OpenFile(backup.path(bf[0].rel), os.O_APPEND|os.O_WRONLY, 0)
	f.Write([]byte("x"))
	f.Close()
	data, _ := os.ReadFile(backup.path(PathOf(w)))
	os.WriteFile(backup.path(PathOf(w)), []byte(strings.Replace(string(data), "80", "90", 1)), 0o600)
	fresh := newStore(t)
	res, err := fresh.Sync(backup)
	if err != nil || res.BadRecords != 1 || res.BadBlobs != 1 {
		t.Errorf("sync: %+v %v", res, err)
	}
}

// E3: a record file that fails its checks goes out as it is.
func TestAudit7ExportKeepsInvalidFiles(t *testing.T) {
	s, _ := initStore(t)
	w, _ := s.Add("bp", "120/80", AddOptions{Note: "after run"})
	p := PathOf(w)
	data, _ := os.ReadFile(s.path(p))
	bad := []byte(strings.Replace(string(data), "after run", "after walk", 1))
	os.WriteFile(s.path(p), bad, 0o600)
	dir := filepath.Join(t.TempDir(), "x")
	res, err := s.ExportAll(dir)
	if err != nil || len(res.Invalid) != 1 || res.Invalid[0] != p {
		t.Fatalf("export: %+v %v", res, err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "invalid", filepath.FromSlash(p))); err != nil || string(got) != string(bad) {
		t.Errorf("the bytes of the bad file: %v", err)
	}
	if inv, _ := readManifest(t, dir)["invalid_records"].([]any); len(inv) != 1 {
		t.Errorf("manifest: %v", inv)
	}
}

// E4: a blob that no record names goes out; the blob of a deleted record that
// waits for gc does not.
func TestAudit7ExportUnreferencedBlob(t *testing.T) {
	a, _ := initStore(t)
	put, _ := a.PutBlob([]byte("kept scan"))
	photo, _ := a.Add("meal", "lunch", AddOptions{Photo: testPNG, PhotoType: "image/png"})
	b := cloneLocal(t, a)
	if _, err := b.Purge(photo, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Sync(b); err != nil {
		t.Fatal(err)
	}
	l, _ := a.Load()
	if !l.Hidden[photo] {
		t.Fatal("setup: the photo is not hidden")
	}
	dir := filepath.Join(t.TempDir(), "x")
	res, err := a.ExportAll(dir)
	if err != nil || len(res.Unreferenced) != 1 || res.Unreferenced[0] != put || res.Blobs != 1 {
		t.Fatalf("export: %+v %v", res, err)
	}
	if pt, err := os.ReadFile(filepath.Join(dir, "blobs", put[7:])); err != nil || string(pt) != "kept scan" {
		t.Errorf("blob put file: %q %v", pt, err)
	}
	if u, _ := readManifest(t, dir)["unreferenced_blobs"].([]any); len(u) != 1 {
		t.Errorf("manifest: %v", u)
	}
}

// E5: an older export does not bring back a record that was purged since,
// nor its photo.
func TestAudit7ImportExportRefusesPurged(t *testing.T) {
	s, _ := initStore(t)
	id, _ := s.Add("diagnosis.private", "secret", AddOptions{Photo: testPNG, PhotoType: "image/png"})
	l, _ := s.Load()
	hash := l.Report.Records[id].Blobs[0].Hash
	x := filepath.Join(t.TempDir(), "x")
	if _, _, err := s.Export(x); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Purge(id, ""); err != nil {
		t.Fatal(err)
	}
	res, err := s.ImportExportAll(x)
	if err != nil || res.Refused != 1 || res.Records != 0 || res.Blobs != 0 {
		t.Errorf("import-export: %+v %v", res, err)
	}
	if _, err := os.Stat(s.path(PathOf(id))); err == nil {
		t.Error("the purged record is on disk again")
	}
	if _, err := os.Stat(s.path(BlobPathOf(hash))); err == nil {
		t.Error("the purged photo is on disk again")
	}
}

// E6: an export of another identity says so.
func TestAudit7ImportExportOtherIdentity(t *testing.T) {
	a, aid := initStore(t)
	a.Add("meal", "lunch", AddOptions{Photo: testPNG, PhotoType: "image/png"})
	x := filepath.Join(t.TempDir(), "x")
	if _, _, err := a.Export(x); err != nil {
		t.Fatal(err)
	}
	o, oid := initStore(t)
	_, _, err := o.ImportExport(x)
	if err == nil || err.Error() != "export of identity "+aid+", this log is "+oid {
		t.Errorf("import-export: %v", err)
	}
}

// E8: a changed reading that -corrections refuses says why.
func TestAudit7CorrectionsSayWhy(t *testing.T) {
	s, _ := initStore(t)
	rec := func(typ, v string) string {
		return `{"type":"` + typ + `","value":` + v + `,"times":{"observed":null},"source":{"system":"s","adapter":"t@1","ref_input":["r1"]}}`
	}
	appendLines(t, s, rec("lab.glucose", `{"n":90,"unit":"mg/dL"}`))
	for _, c := range []struct{ typ, v, want string }{
		{"lab.ldl", `{"n":120,"unit":"mg/dL"}`, "type differs"},
		{"lab.glucose", `{"n":null}`, "no value"},
	} {
		res, err := s.AppendCorrections(strings.NewReader(rec(c.typ, c.v) + "\n"))
		if err != nil || len(res.Changed) != 1 || res.Changed[0].NewType != c.typ || !strings.Contains(res.Changed[0].Reason, c.want) {
			t.Errorf("%s: %+v %v", c.typ, res, err)
		}
	}
	// A change that -corrections would write has no reason.
	res, _ := s.Append(strings.NewReader(rec("lab.glucose", `{"n":95,"unit":"mg/dL"}`) + "\n"))
	if len(res.Changed) != 1 || res.Changed[0].Reason != "" {
		t.Errorf("append: %+v", res)
	}
}
