package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

// Attack round 9. Synthetic data only. Fails when the attack works.

// A sync tool's conflict copy of a purged record: purge tells the person to
// purge the other record, which no command can find or purge; the photo stays
// readable, and export writes the deleted record's text to invalid/.
func TestAttack9CLIConflictCopy(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "", "init")
	s := core.Open(dir)
	x, err := s.Add("photo", "rash on my genitals", core.AddOptions{Photo: []byte(tinyPNG), PhotoType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}
	l, _ := s.Load()
	hash := l.Report.Records[x].Blobs[0].Hash
	p := filepath.Join(dir, filepath.FromSlash(core.PathOf(x)))
	b, _ := os.ReadFile(p)
	os.WriteFile(strings.TrimSuffix(p, ".json")+".sync-conflict-20261107-101010-ABCDEFG.json", b, 0o600)
	out, errs, code := run(t, dir, "", "purge", x)
	t.Logf("purge: exit %d\n%s%s", code, out, errs)
	ls, _, _ := run(t, dir, "", "ls")
	t.Logf("ls after purge: %q", ls)
	got, _, gcode := run(t, dir, "", "blob", "get", hash)
	t.Logf("blob get: exit %d, %d bytes", gcode, len(got))
	exp := filepath.Join(t.TempDir(), "out")
	eo, ee, ecode := run(t, dir, "", "export", exp)
	t.Logf("export: exit %d\n%s%s", ecode, eo, ee)
	man, _ := os.ReadFile(filepath.Join(exp, "manifest.json"))
	t.Logf("manifest: %s", man)
	if gcode == 0 {
		t.Errorf("the purged photo is still readable after purge (%d bytes)", len(got))
	}
	if strings.Contains(string(man), "sync-conflict") {
		t.Errorf("the export carries the purged record under invalid/")
	}
}
