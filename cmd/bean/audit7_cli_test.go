package main

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

// The CLI findings of the seventh audit (DECISIONS 206 to 208). Each test
// failed before its fix.

const tinyPNG = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\nIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01\x0d\n-\xb4\x00\x00\x00\x00IEND\xaeB`\x82"

func writePNG(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "p.png")
	if err := os.WriteFile(p, []byte(tinyPNG), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// E1: sync from a folder that is not a log fails and names it.
func TestCLISyncFromNoLog(t *testing.T) {
	a := t.TempDir()
	run(t, a, "", "init")
	typo := filepath.Join(t.TempDir(), "does-not-exist")
	for _, src := range []string{typo, t.TempDir()} {
		out, errs, code := run(t, a, "", "sync", src)
		if code != 1 || out != "" || !strings.Contains(errs, "no log in "+src) {
			t.Errorf("sync %s: %d %q %q", src, code, out, errs)
		}
	}
}

// E2: sync from a damaged copy says what it skipped and exits 1.
func TestCLISyncFromDamagedCopy(t *testing.T) {
	backup := t.TempDir()
	run(t, backup, "", "init")
	run(t, backup, "", "add", "meal", "lunch", "-photo", writePNG(t))
	w, _, _ := run(t, backup, "", "add", "weight", "80", "-unit", "kg")
	w = strings.TrimSpace(w)
	filepath.WalkDir(filepath.Join(backup, "blobs"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
			f.Write([]byte("x"))
			f.Close()
		}
		return nil
	})
	rf := filepath.Join(backup, filepath.FromSlash(core.PathOf(w)))
	data, _ := os.ReadFile(rf)
	os.WriteFile(rf, []byte(strings.Replace(string(data), `"n":80`, `"n":90`, 1)), 0o600)
	fresh := t.TempDir()
	_, errs, code := run(t, fresh, "", "sync", backup)
	if code != 1 || !strings.Contains(errs, "skipped 1 bad record files, 1 bad blobs in "+backup) {
		t.Errorf("sync: %d %q", code, errs)
	}
}

// E3: export copies a record file that fails its checks and says so.
func TestCLIExportKeepsInvalidFile(t *testing.T) {
	a := t.TempDir()
	run(t, a, "", "init")
	w, _, _ := run(t, a, "", "add", "bp", "120/80", "-note", "after run")
	rel := core.PathOf(strings.TrimSpace(w))
	rf := filepath.Join(a, filepath.FromSlash(rel))
	data, _ := os.ReadFile(rf)
	os.WriteFile(rf, []byte(strings.Replace(string(data), "after run", "after walk", 1)), 0o600)
	x := filepath.Join(t.TempDir(), "x")
	_, errs, code := run(t, a, "", "export", x)
	if code != 0 || !strings.Contains(errs, "1 record files fail their checks") {
		t.Errorf("export: %d %q", code, errs)
	}
	if got, err := os.ReadFile(filepath.Join(x, "invalid", filepath.FromSlash(rel))); err != nil || !strings.Contains(string(got), "after walk") {
		t.Errorf("invalid/ copy: %v", err)
	}
}

// E4: export writes a blob that no record names, from blob put.
func TestCLIExportBlobPut(t *testing.T) {
	a := t.TempDir()
	run(t, a, "", "init")
	scan := filepath.Join(t.TempDir(), "scan.txt")
	os.WriteFile(scan, []byte("my kept scan, never attached\n"), 0o600)
	id, _, _ := run(t, a, "", "blob", "put", scan)
	id = strings.TrimSpace(id)
	x := filepath.Join(t.TempDir(), "x")
	out, errs, code := run(t, a, "", "export", x)
	if code != 0 || !strings.Contains(out, "1 blobs") {
		t.Errorf("export: %d %q %q", code, out, errs)
	}
	if got, err := os.ReadFile(filepath.Join(x, "blobs", id[7:])); err != nil || string(got) != "my kept scan, never attached\n" {
		t.Errorf("blob put file in export: %v", err)
	}
	if man, _ := os.ReadFile(filepath.Join(x, "manifest.json")); !strings.Contains(string(man), `"unreferenced_blobs":["`+id+`"]`) {
		t.Errorf("manifest: %s", man)
	}
}

// E5: import-export of an older export does not bring back a purged record.
func TestCLIImportExportRefusesPurged(t *testing.T) {
	a := t.TempDir()
	run(t, a, "", "init")
	id, _, _ := run(t, a, "", "add", "diagnosis.private", "hiv positive", "-at", "2026-10-01")
	id = strings.TrimSpace(id)
	x := filepath.Join(t.TempDir(), "x1")
	run(t, a, "", "export", x)
	run(t, a, "", "purge", id)
	out, errs, code := run(t, a, "", "import-export", x)
	if code != 0 || !strings.Contains(out, "added 0 records, 0 blobs, refused 1 purged") {
		t.Errorf("import-export: %d %q %q", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(a, filepath.FromSlash(core.PathOf(id)))); err == nil {
		t.Error("the purged record is on disk again")
	}
}

// E6: import-export of another identity's export says whose it is.
func TestCLIImportExportOtherIdentity(t *testing.T) {
	a, other := t.TempDir(), t.TempDir()
	run(t, a, "", "init")
	run(t, a, "", "add", "meal", "lunch", "-photo", writePNG(t))
	x := filepath.Join(t.TempDir(), "x")
	run(t, a, "", "export", x)
	run(t, other, "", "init")
	_, errs, code := run(t, other, "", "import-export", x)
	if code != 1 || !strings.Contains(errs, "export of identity sha256:") || !strings.Contains(errs, ", this log is sha256:") || strings.Contains(errs, "blob corrupt") {
		t.Errorf("import-export: %d %q", code, errs)
	}
}

// E7: export into a folder that is not empty fails before it prints a count.
func TestCLIExportNotEmpty(t *testing.T) {
	a := t.TempDir()
	run(t, a, "", "init")
	x := t.TempDir()
	os.WriteFile(filepath.Join(x, "keep.txt"), []byte("x"), 0o600)
	out, errs, code := run(t, a, "", "export", x)
	if code != 1 || strings.Contains(out, "exported") || !strings.Contains(errs, "not empty") {
		t.Errorf("export: %d %q %q", code, out, errs)
	}
}

// E8: append -corrections names why it cannot write a changed reading.
func TestCLICorrectionsSayWhy(t *testing.T) {
	a := t.TempDir()
	run(t, a, "", "init")
	rec := func(typ, v string) string {
		return `{"type":"` + typ + `","value":` + v + `,"times":{"observed":null},"source":{"system":"s","adapter":"t@1","ref_input":["r1"]}}` + "\n"
	}
	run(t, a, rec("lab.glucose", `{"n":90,"unit":"mg/dL"}`), "append")
	for _, c := range []struct{ typ, v, want string }{
		{"lab.ldl", `{"n":120,"unit":"mg/dL"}`, "(type lab.ldl): not written, because its type differs"},
		{"lab.glucose", `{"n":null}`, "not written, because the input has no value"},
	} {
		_, errs, code := run(t, a, rec(c.typ, c.v), "append", "-corrections")
		if code != 1 || !strings.Contains(errs, c.want) || strings.Contains(errs, "then run append -corrections") {
			t.Errorf("%s: %d %q", c.typ, code, errs)
		}
	}
}

// A6: read_blob does not decrypt a blob that only a stranger's record names,
// such as the photo of a record the person purged.
func TestMCPReadBlobRefusesStranger(t *testing.T) {
	s, _ := mcpStore(t)
	id, _ := s.Identity()
	photo, err := s.Add("photo", "rash", core.AddOptions{Photo: []byte(tinyPNG), PhotoType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}
	l, _ := s.Load()
	hash := l.Report.Records[photo].Blobs[0].Hash
	raw := make([]byte, 32)
	rand.Read(raw)
	k := core.NewEd25519Signer(raw)
	o := map[string]any{"v": 0.0, "nonce": core.NewNonce(), "type": "note", "subject": id, "author": k.Public(), "kid": k.Public(),
		"value": map[string]any{"s": "x"}, "times": map[string]any{"observed": nil, "imported": "2026-11-06T00:00:00Z"},
		"source": map[string]any{"system": "x", "adapter": "x@1", "ref": nil},
		"blobs":  []any{map[string]any{"hash": hash, "role": "x", "media_type": "image/png"}}}
	sid, b, err := core.SignRecord(o, k)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(s.Dir, filepath.FromSlash(core.PathOf(sid)))
	os.MkdirAll(filepath.Dir(p), 0o700)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Purge(photo, ""); err != nil {
		t.Fatal(err)
	}
	if m, isErr := call(t, s, "read_blob", map[string]any{"id": hash}); !isErr {
		t.Errorf("read_blob of a deleted photo that a stranger names: %v", m)
	}
}
