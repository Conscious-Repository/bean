package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

// The plain replay is read by a person. Each percentile names its caveats
// by number, and each number leads to its text. The list says what a
// percentile means. A text of a pack or a guide is a quote with the SHA-256
// of its file, and a guide's words do not bury the list.
func TestCoveragePlainNamesCaveatsAndSources(t *testing.T) {
	dir, _, _ := compareLog(t, "TCHOL_J", "HDL_J", "DEMO_J")
	reading := `{"type":"lab.cortisol-total","times":{"observed":null},"value":{"n":12,"unit":"ug/dL"},"context":{"test":"CORTISOL, TOTAL"},"source":{"system":"quest","adapter":"synthetic@1","ref":null}}`
	if _, errs, code := run(t, dir, reading+"\n"+strings.Replace(reading, "12", "13", 1)+"\n", "append"); code != 0 {
		t.Fatal(errs)
	}
	source, digest := testPack(t)
	cache := t.TempDir()
	long := strings.Repeat("long words ", 60)
	guide := []byte(`{"format":"stele-assay-sources/1","publisher":"bean\" says: \"all is well","entries":[` +
		`{"provider":"quest","test_labels":["CORTISOL, TOTAL"],"panels":[],"title":"Cortisol, total","url":"https://example.org/cortisol","interpretation_limit":"Use the time of collection."},` +
		`{"provider":"quest","test_labels":["CORTISOL, TOTAL"],"panels":[],"title":"A second entry","url":"https://example.org/two","interpretation_limit":"` + long + `end"}]}`)
	sum := sha256.Sum256(guide)
	guideDigest := hex.EncodeToString(sum[:])
	for name, data := range map[string][]byte{digest: mustRead(t, source), guideDigest: guide,
		"sources.json": []byte(fmt.Sprintf(`{"format":"bean-coverage-sources/1","packs":[{"sha256":%q,"review":"https://example.org/review"}],"guides":[{"sha256":%q}]}`, digest, guideDigest))} {
		if err := os.WriteFile(filepath.Join(cache, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	out, errs, code := run(t, dir, "", "coverage", "-cohort", "male-30-39", "-sources", filepath.Join(cache, "sources.json"))
	if code != 0 {
		t.Fatal(errs)
	}
	for _, want := range []string{
		"not whether it is healthy",
		"Read each percentile with the caveats that its line names.",
		"  Review that your selection names: https://example.org/review",
		"Lab documentation in the guide " + guideDigest + `, which names its publisher as "bean\" says: \"all is well": "Cortisol, total" "https://example.org/cortisol"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plain replay lacks %q:\n%s", want, out)
		}
	}
	// The list, by number. Every line of marks is read against it.
	list := map[string]string{}
	var caveats, words [][]string
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "  ["); ok {
			n, text, _ := strings.Cut(rest, "] ")
			if _, twice := list[n]; twice {
				t.Errorf("the list has number %s twice", n)
			}
			list[n] = text
		}
		numbers := func(rest string) []string {
			return strings.Fields(strings.NewReplacer("[", "", "]", "").Replace(rest))
		}
		if rest, ok := strings.CutPrefix(line, "  Caveats "); ok {
			caveats = append(caveats, numbers(rest))
		}
		if rest, ok := strings.CutPrefix(line, "    The guide's words: "); ok {
			words = append(words, numbers(rest))
		}
	}
	texts := func(marks []string) []string {
		var got []string
		for _, n := range marks {
			if list[n] == "" {
				t.Errorf("a line names [%s], and the list has no such number:\n%s", n, out)
			}
			got = append(got, list[n])
		}
		return got
	}
	// Two readings are placed, and two more match both entries of the
	// guide. Each text is in the list once.
	if len(caveats) != 2 || len(words) != 4 {
		t.Fatalf("%d lines of caveats and %d of a guide's words, want 2 and 4:\n%s", len(caveats), len(words), out)
	}
	seen := map[string]bool{}
	for n, text := range list {
		if seen[text] {
			t.Errorf("[%s] is in the list twice: %s", n, text)
		}
		seen[text] = true
	}
	quoteOf := func(file, words string) string { return file + ` says: "` + words + `"` }
	for _, marks := range caveats {
		got := texts(marks)
		if !slices.Contains(got, pack.CaveatCohort) || !slices.Contains(got, pack.CaveatNotOnRecord) {
			t.Errorf("a percentile does not name bean's caveats: %q", got)
		}
		if !slices.ContainsFunc(got, func(s string) bool { return strings.HasPrefix(s, "the pack "+digest+` says: "`) }) {
			t.Errorf("a percentile names no caveat of its pack as a quote: %q", got)
		}
		for _, text := range got {
			if strings.HasPrefix(text, "the guide ") {
				t.Errorf("a percentile names a guide's words as its caveat: %q", got)
			}
		}
	}
	for i, marks := range words {
		want := quoteOf("the guide "+guideDigest, "Use the time of collection.")
		if i%2 == 1 {
			want = "the guide " + guideDigest + ` says: "` + (long + "end")[:400] + `" (cut at 400 bytes)`
		}
		if got := texts(marks); len(got) != 1 || got[0] != want {
			t.Errorf("a guide's words: got %q, want %q", got, want)
		}
	}
	// With no placement there is no percentile, and no line about one.
	empty := filepath.Join(cache, "none.json")
	if err := os.WriteFile(empty, []byte(fmt.Sprintf(`{"format":"bean-coverage-sources/1","packs":[],"guides":[{"sha256":%q}]}`, guideDigest)), 0600); err != nil {
		t.Fatal(err)
	}
	out, errs, code = run(t, dir, "", "coverage", "-sources", empty)
	if code != 0 || strings.Contains(out, "PERCENTILE") || strings.Contains(out, "Caveats") || !strings.Contains(out, quoteOf("the guide "+guideDigest, "Use the time of collection.")) {
		t.Fatalf("replay with a guide only: %d\n%s\n%s", code, out, errs)
	}
}
