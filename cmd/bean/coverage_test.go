package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverageSelectionAndCurrentReadings(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "HDL_J", "DEMO_J")
	source, digest := testPack(t)
	cache := t.TempDir()
	local := filepath.Join(cache, digest)
	selection := filepath.Join(cache, "sources.json")
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(local, mustRead(t, source))
	selectPack := fmt.Sprintf(`{"format":"bean-coverage-sources/1","packs":[{"sha256":%q}],"guides":[]}`, digest)
	write(selection, []byte(selectPack))
	args := []string{"coverage", "-cohort", "male-30-39", "-sources", selection, "-json"}
	replay := func() coverageReport {
		t.Helper()
		out, errs, code := run(t, dir, "", args...)
		var got coverageReport
		if code != 0 {
			t.Fatal(errs)
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := replay(); got.Total != 3 || got.SourceCovered != 2 || got.Counts["placed"] != 2 {
		t.Fatalf("unexpected counts: %v", got.Counts)
	}
	run2(t, dir, "purge", ids[0])
	if got := replay(); got.Total != 2 || got.Counts["placed"] != 1 {
		t.Fatalf("purged row counted: %v", got.Counts)
	}
	write(local, append(mustRead(t, local), ' '))
	out, errs, code := run(t, dir, "", args...)
	if code == 0 || out != "" || !strings.Contains(errs, "checksum mismatch") {
		t.Fatalf("tampered asset: %d %s %s", code, out, errs)
	}
	write(selection, []byte(`{"format":"bean-coverage-sources/1","packs":[],"guides":[]}`))
	if got := replay(); got.Total != 2 || got.SourceCovered != 0 || got.Counts["unmatched_numeric"] != 2 {
		t.Fatalf("unselected pack used: %v", got.Counts)
	}
}
