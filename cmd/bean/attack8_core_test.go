package main

import (
	"strings"
	"testing"
)

// Audit 8, item P. Synthetic data only. The test fails when the attack works.

// Purge keeps a stored file that another record still names. It must say so:
// "purged" alone while the file is still readable is a lie by omission.
func TestAttack8PurgeSaysNothingWhenFileStays(t *testing.T) {
	dir, _, _, rec := importedReport(t)
	bid := rec["value"].(map[string]any)["blob"].(string)
	old := `{"type":"lab.glucose","value":{"n":101,"unit":"mg/dL"},"times":{"observed":null},"source":{"system":"labcorp","adapter":"bean-import-labcorp@0.1.4","ref_input":["bean/v0/ref","id","labcorp","old",0]},"blobs":[{"hash":"` + bid + `","role":"raw-export","media_type":"application/pdf"}]}`
	if _, errs, code := run(t, dir, old+"\n", "append"); code != 0 {
		t.Fatalf("append: %s", errs)
	}
	out, errs, code := run(t, dir, "", "purge", rec["id"].(string))
	t.Logf("purge: exit %d %q %q", code, out, errs)
	// The file stays, because the second record names it: the test is about
	// what purge says then, so it must not pass when the file is gone.
	got, gerrs, gcode := run(t, dir, "", "blob", "get", bid)
	if gcode != 0 {
		t.Fatalf("setup: the file should still be readable: %s", gerrs)
	}
	if !strings.Contains(out+errs, "kept") && !strings.Contains(out+errs, "still") {
		t.Errorf("purge said only %q, and the file is still readable (%d bytes)", strings.TrimSpace(out), len(got))
	}
}
