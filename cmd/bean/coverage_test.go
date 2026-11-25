package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/internal/pack"
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

// The plain replay is read by a person. Each percentile names its caveats,
// the list says what a percentile means, and a guide's words are a quote
// with the name of the guide's publisher.
func TestCoveragePlainNamesCaveatsAndSources(t *testing.T) {
	dir, _, _ := compareLog(t, "TCHOL_J", "HDL_J", "DEMO_J")
	reading := `{"type":"lab.cortisol-total","times":{"observed":null},"value":{"n":12,"unit":"ug/dL"},"context":{"test":"CORTISOL, TOTAL"},"source":{"system":"quest","adapter":"synthetic@1","ref":null}}`
	if _, errs, code := run(t, dir, reading+"\n", "append"); code != 0 {
		t.Fatal(errs)
	}
	source, digest := testPack(t)
	cache := t.TempDir()
	guide := []byte(`{"format":"stele-assay-sources/1","publisher":"A made-up publisher","entries":[{"provider":"quest","test_labels":["CORTISOL, TOTAL"],"panels":[],"title":"Cortisol, total","url":"https://example.org/cortisol","interpretation_limit":"Use the time of collection."}]}`)
	sum := sha256.Sum256(guide)
	guideDigest := hex.EncodeToString(sum[:])
	for name, data := range map[string][]byte{digest: mustRead(t, source), guideDigest: guide,
		"sources.json": []byte(fmt.Sprintf(`{"format":"bean-coverage-sources/1","packs":[{"sha256":%q}],"guides":[{"sha256":%q}]}`, digest, guideDigest))} {
		if err := os.WriteFile(filepath.Join(cache, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	out, errs, code := run(t, dir, "", "coverage", "-cohort", "male-30-39", "-sources", filepath.Join(cache, "sources.json"))
	if code != 0 {
		t.Fatal(errs)
	}
	for _, want := range []string{
		"  Caveats [1] ",
		"not whether it is healthy",
		"] " + pack.CaveatNotOnRecord,
		"] " + pack.CaveatCohort,
		"Lab documentation that A made-up publisher lists (guide SHA-256 " + guideDigest + "): Cortisol, total https://example.org/cortisol",
		`A made-up publisher says: "Use the time of collection."`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plain replay lacks %q:\n%s", want, out)
		}
	}
	// With no placement there is no percentile, and no line about one.
	empty := filepath.Join(cache, "none.json")
	if err := os.WriteFile(empty, []byte(fmt.Sprintf(`{"format":"bean-coverage-sources/1","packs":[],"guides":[{"sha256":%q}]}`, guideDigest)), 0600); err != nil {
		t.Fatal(err)
	}
	out, errs, code = run(t, dir, "", "coverage", "-sources", empty)
	if code != 0 || strings.Contains(out, "PERCENTILE") || strings.Contains(out, "Caveats") || !strings.Contains(out, "A made-up publisher says") {
		t.Fatalf("replay with a guide only: %d\n%s\n%s", code, out, errs)
	}
}
