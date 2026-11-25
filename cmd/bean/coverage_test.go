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
	got := replay()
	if got.Total != 3 || got.SourceCovered != 2 || got.Counts["placed"] != 2 {
		t.Fatalf("unexpected counts: %v", got.Counts)
	}
	// A program that reads the replay learns what a percentile means, and
	// which caveats the pack states: bean wrote the last three of these.
	if got.Means != percentileMeans {
		t.Errorf("the replay does not say what a percentile means: %q", got.Means)
	}
	for _, row := range got.Rows {
		for _, m := range row.Matches {
			if m.FromPack < 1 || m.FromPack != len(m.Caveats)-3 || m.Caveats[m.FromPack] != pack.CaveatCohort {
				t.Errorf("%d of %d caveats are the pack's: %q", m.FromPack, len(m.Caveats), m.Caveats)
			}
		}
	}
	// A key in another case is refused: Go would read it as the member.
	for _, bad := range []string{
		fmt.Sprintf(`{"format":"bean-coverage-sources/1","packs":[{"SHA256":%q}],"guides":[]}`, digest),
		fmt.Sprintf(`{"format":"bean-coverage-sources/1","Packs":[{"sha256":%q}],"guides":[]}`, digest),
	} {
		write(selection, []byte(bad))
		if out, errs, code := run(t, dir, "", args...); code == 0 || out != "" || !strings.Contains(errs, "a key is in lower case") {
			t.Fatalf("a key in another case: %d %s %s", code, out, errs)
		}
	}
	write(selection, []byte(selectPack))
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
	// The pack states one of bean's own caveats, which is false of these
	// readings. Its source name tries to end bean's sentence and add to it.
	_, digest, packBytes := editedPack(t, func(body map[string]any) {
		body["caveats"] = append(body["caveats"].([]any), pack.CaveatAuthor)
		body["source"].(map[string]any)["name"] = `a survey" checked by bean (`
	})
	cache := t.TempDir()
	// One text is cut inside a character of two bytes. A title of 400
	// bytes is whole, and an address of 401 bytes is cut.
	long := "a" + strings.Repeat("é", 300)
	title := strings.Repeat("t", 400)
	address := "https://example.org/" + strings.Repeat("u", 381)
	guide := []byte(`{"format":"stele-assay-sources/1","publisher":"bean\" says: \"all is well","entries":[` +
		`{"provider":"quest","test_labels":["CORTISOL, TOTAL"],"panels":[],"title":"Cortisol, total\\","url":"https://example.org/cortisol","interpretation_limit":"Use the time of collection.\\\" bean says: all is well. \\\"\u001b[31m"},` +
		`{"provider":"quest","test_labels":["CORTISOL, TOTAL"],"panels":[],"title":"` + title + `","url":"` + address + `","interpretation_limit":"` + long + `"}]}`)
	sum := sha256.Sum256(guide)
	guideDigest := hex.EncodeToString(sum[:])
	for name, data := range map[string][]byte{digest: packBytes, guideDigest: guide,
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
		"Read each percentile with the caveats that its Caveats line names. A guide's words are no caveat of a percentile.",
		`  Review that your selection names: "https://example.org/review"`,
		"Lab documentation in the guide " + guideDigest + `, which names its publisher as "bean\" says: \"all is well": "Cortisol, total\\" "https://example.org/cortisol"`,
		", by the pack " + digest + `, which names its source as "a survey\" checked by bean ("` + "\n",
		`: "` + title + `" "` + address[:400] + `" (cut at 400 bytes)` + "\n",
		"\nThe lines above name these by number:\n",
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
	// The guide's text holds a backslash and a quote mark. Each is escaped,
	// so the words after them stay inside the guide's quote.
	// An escape code of a terminal is written out.
	said := "the guide " + guideDigest + ` says: "Use the time of collection.\\\" bean says: all is well. \\\"\x1b[31m"`
	for _, marks := range caveats {
		got := texts(marks)
		if !slices.Contains(got, pack.CaveatCohort) || !slices.Contains(got, pack.CaveatNotOnRecord) {
			t.Errorf("a percentile does not name bean's caveats: %q", got)
		}
		// What is not a pack's quote is one of bean's three caveats here.
		small := fmt.Sprintf(pack.CaveatSmall, pack.SmallN)
		for _, text := range got {
			if !strings.HasPrefix(text, "the pack "+digest+` says: "`) && text != pack.CaveatCohort && text != pack.CaveatNotOnRecord && text != small {
				t.Errorf("a text that bean did not write is in bean's words: %q", text)
			}
		}
		// The person signed these readings. Only the pack says otherwise.
		if slices.Contains(got, pack.CaveatAuthor) || !slices.Contains(got, "the pack "+digest+` says: "`+pack.CaveatAuthor+`"`) {
			t.Errorf("a pack's text in bean's words, or bean's words lost: %q", got)
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
		want := said
		if i%2 == 1 {
			want = "the guide " + guideDigest + ` says: "a` + strings.Repeat("é", 199) + `" (cut at 400 bytes)`
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
	if code != 0 || strings.Contains(out, "PERCENTILE") || strings.Contains(out, "Caveats") || !strings.Contains(out, said) {
		t.Fatalf("replay with a guide only: %d\n%s\n%s", code, out, errs)
	}
	// A guide with a key in another case is refused, as a selection is.
	odd := []byte(strings.Replace(string(guide), `"url"`, `"URL"`, 1))
	oddSum := sha256.Sum256(odd)
	oddDigest := hex.EncodeToString(oddSum[:])
	for name, data := range map[string][]byte{oddDigest: odd, "none.json": []byte(fmt.Sprintf(`{"format":"bean-coverage-sources/1","packs":[],"guides":[{"sha256":%q}]}`, oddDigest))} {
		if err := os.WriteFile(filepath.Join(cache, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if out, errs, code := run(t, dir, "", "coverage", "-sources", empty); code == 0 || out != "" || !strings.Contains(errs, `the key "URL" is not "url"`) {
		t.Fatalf("a guide with a key in another case: %d %s %s", code, out, errs)
	}
	// With no source at all there is no list, and no line that leads to one.
	if err := os.WriteFile(empty, []byte(`{"format":"bean-coverage-sources/1","packs":[],"guides":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	out, errs, code = run(t, dir, "", "coverage", "-sources", empty)
	if code != 0 || strings.Contains(out, "by number") || strings.HasSuffix(out, "\n\n") {
		t.Fatalf("replay with no source: %d\n%q\n%s", code, out, errs)
	}
}

// A replay shows the fields that a pack requires of a reading: what was
// matched, and what differs when there is no match. A reading that an agent
// signed has the caveat that says so, after the texts that the pack states.
func TestCoverageReplaysEvidence(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "HDL_J", "DEMO_J")
	signed := `{"type":"lab.cholesterol-total","times":{"observed":null},"value":{"n":190,"unit":"mg/dL"},"context":{"test":"CHOLESTEROL, TOTAL","panel":"LIPID PANEL"},"source":{"system":"quest","adapter":"synthetic@1","ref":null}}`
	report := `{"type":"lab.report","times":{"observed":null},"value":{"s":"a report"},"source":{"system":"quest","adapter":"synthetic@1","ref":null}}`
	other := `{"type":"lab.other","times":{"observed":null},"value":{"n":5,"unit":"mg/dL"},"context":{"test":"ANOTHER TEST","panel":"LIPID PANEL"},"source":{"system":"quest","adapter":"synthetic@1","ref":null}}`
	if _, errs, code := run(t, dir, signed+"\n"+report+"\n"+other+"\n", "append"); code != 0 {
		t.Fatal(errs)
	}
	if _, errs, code := run(t, dir, "", "agent", "init", "-label", "assistant"); code != 0 {
		t.Fatal(errs)
	}
	if _, errs, code := run(t, dir, strings.Replace(signed, "190", "191", 1)+"\n", "append", "-agent"); code != 0 {
		t.Fatal(errs)
	}
	row := func(body map[string]any) map[string]any {
		return body["measures"].(map[string]any)["LBXTC"].(map[string]any)["same"].([]any)[0].(map[string]any)
	}
	_, panelPack, panelBytes := editedPack(t, func(body map[string]any) {
		body["bean"] = "pack/1"
		row(body)["when"] = map[string]any{"system": "quest", "test": "CHOLESTEROL, TOTAL", "panel": "LIPID PANEL"}
	})
	_, evidencePack, evidenceBytes := editedPack(t, func(body map[string]any) {
		body["bean"] = "pack/2"
		row(body)["any_of"] = []any{map[string]any{"source.system": "quest", "context.panel": "LIPID PANEL"}, map[string]any{"source.system": "labcorp"}}
	})
	// A guide's entry is for one provider and one test, and for the panels
	// that it names, or for any panel when it names none.
	entry := func(provider, panels, title string) string {
		return fmt.Sprintf(`{"provider":%q,"test_labels":["CHOLESTEROL, TOTAL"],"panels":[%s],"title":%q,"url":"https://example.org/a","interpretation_limit":"A limit."}`, provider, panels, title)
	}
	guide := []byte(`{"format":"stele-assay-sources/1","publisher":"P","entries":[` + entry("quest", `"LIPID PANEL"`, "this panel") + "," +
		entry("quest", "", "any panel") + "," + entry("quest", `"ANOTHER PANEL"`, "another panel") + "," + entry("labcorp", "", "another provider") + `]}`)
	guideSum := sha256.Sum256(guide)
	guideDigest := hex.EncodeToString(guideSum[:])
	cache := t.TempDir()
	for name, data := range map[string][]byte{panelPack: panelBytes, evidencePack: evidenceBytes, guideDigest: guide,
		"sources.json": []byte(fmt.Sprintf(`{"format":"bean-coverage-sources/1","packs":[{"sha256":%q},{"sha256":%q}],"guides":[{"sha256":%q}]}`, panelPack, evidencePack, guideDigest))} {
		if err := os.WriteFile(filepath.Join(cache, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	out, errs, code := run(t, dir, "", "coverage", "-cohort", "male-30-39", "-sources", filepath.Join(cache, "sources.json"), "-json")
	if code != 0 {
		t.Fatal(errs)
	}
	var got coverageReport
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	// Three readings of total cholesterol, HDL, TSH and one other test: the
	// report is no reading.
	if got.Total != 6 || got.Counts["placed"] != 3 || got.Counts["unmatched_numeric"] != 3 {
		t.Fatalf("counts: %d %v", got.Total, got.Counts)
	}
	seen := 0
	for _, r := range got.Rows {
		n, _ := r.Value["n"].(float64)
		switch {
		case r.Type == "lab.other":
			// The same provider and panel, and another test: no entry of the guide.
			if len(r.References) != 0 || r.Status != "unmatched_numeric" {
				t.Errorf("a reading of another test: %+v", r)
			}
		case r.ID == ids[0]:
			// Typed by hand: each pack declines, and names what differs.
			seen++
			if r.Status != "unmatched_numeric" || len(r.Matches) != 0 || len(r.Declines) != 2 || len(r.References) != 0 {
				t.Fatalf("a reading typed by hand: %+v", r)
			}
			for _, d := range r.Declines {
				want := []string{"context.panel", "context.test", "source.system"}
				if d.SHA256 == evidencePack {
					want = []string{"context.panel", "source.system"}
				}
				if d.Reason != "signed_fields" || len(d.Rules) == 0 || !slices.Equal(d.Rules[0].Different, want) {
					t.Errorf("the decline of %s: %+v, want the fields %v", d.SHA256[:8], d, want)
				}
			}
		case r.Type == "lab.cholesterol-total" && (n == 190 || n == 191):
			// With the fields: a place in each pack, and the rule that matched.
			seen++
			if r.Status != "placed" || len(r.Matches) != 2 {
				t.Fatalf("a reading with the fields: %+v", r)
			}
			if len(r.References) != 2 || r.References[0].Title != "this panel" || r.References[1].Title != "any panel" || r.References[0].SHA256 != guideDigest {
				t.Errorf("the entries of the guide for this reading: %+v", r.References)
			}
			for _, m := range r.Matches {
				if len(m.Evidence.Different) != 0 || m.Evidence.Required["source.system"] != "quest" || m.Evidence.Required["context.panel"] != "LIPID PANEL" {
					t.Errorf("the rule that matched: %+v", m.Evidence)
				}
				if m.SHA256 == panelPack && m.Evidence.Required["context.test"] != "CHOLESTEROL, TOTAL" {
					t.Errorf("the rule of a panel names no test: %+v", m.Evidence)
				}
				byAgent := m.Caveats[len(m.Caveats)-1] == pack.CaveatAuthor
				if byAgent != (n == 191) || slices.Contains(m.Caveats[:m.FromPack], pack.CaveatAuthor) {
					t.Errorf("the caveat of another signer, for the reading of %v: %q", n, m.Caveats)
				}
			}
		}
	}
	if seen != 3 {
		t.Errorf("%d of the 3 readings of total cholesterol are in the replay", seen)
	}
}

// A selection or a guide that is not what the format names is refused,
// before any reading is read.
func TestCoverageRefusesSources(t *testing.T) {
	dir, _, _ := compareLog(t, "TCHOL_J", "HDL_J", "DEMO_J")
	source, digest := testPack(t)
	cache := t.TempDir()
	put := func(name string, data []byte) string {
		t.Helper()
		if err := os.WriteFile(filepath.Join(cache, name), data, 0600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	}
	put(digest, mustRead(t, source))
	guide := func(text string) string {
		d := put("guide", []byte(text))
		put(d, []byte(text))
		return d
	}
	entry := `{"provider":"quest","test_labels":["A TEST"],"panels":[],"title":"A title","url":"https://example.org/a","interpretation_limit":"A limit."}`
	selection := func(packs, guides string) string {
		return `{"format":"bean-coverage-sources/1","packs":[` + packs + `],"guides":[` + guides + `]}`
	}
	pin := fmt.Sprintf(`{"sha256":%q}`, digest)
	withGuide := func(text string) string { return selection("", fmt.Sprintf(`{"sha256":%q}`, guide(text))) }
	for name, c := range map[string]struct{ selection, cohort, want string }{
		"a pack twice":                  {selection(pin+","+pin, ""), "male-30-39", "invalid or duplicate source digest"},
		"a digest in upper case":        {selection(fmt.Sprintf(`{"sha256":%q}`, strings.ToUpper(digest)), ""), "male-30-39", "invalid or duplicate source digest"},
		"a review address with no TLS":  {selection(fmt.Sprintf(`{"sha256":%q,"review":"http://example.org/r"}`, digest), ""), "male-30-39", "invalid review URL"},
		"a review address with a user":  {selection(fmt.Sprintf(`{"sha256":%q,"review":"https://u@example.org/r"}`, digest), ""), "male-30-39", "invalid review URL"},
		"no list of guides":             {`{"format":"bean-coverage-sources/1","packs":[` + pin + `]}`, "male-30-39", "requires packs and guides arrays"},
		"a member more":                 {`{"format":"bean-coverage-sources/1","packs":[` + pin + `],"guides":[],"note":"x"}`, "male-30-39", "source selection"},
		"another format":                {`{"format":"bean-coverage-sources/2","packs":[` + pin + `],"guides":[]}`, "male-30-39", "unsupported source selection"},
		"a pack that is not there":      {selection(fmt.Sprintf(`{"sha256":%q}`, strings.Repeat("0", 64)), ""), "male-30-39", "no such file"},
		"no cohort with a pack":         {selection(pin, ""), "", "choose a cohort with -cohort"},
		"a cohort that no pack has":     {selection(pin, ""), "male-200-209", "no selected pack defines cohort"},
		"a guide with no publisher":     {withGuide(`{"format":"stele-assay-sources/1","publisher":"","entries":[` + entry + `]}`), "", "unsupported or empty assay guide"},
		"a guide with no entry":         {withGuide(`{"format":"stele-assay-sources/1","publisher":"P","entries":[]}`), "", "unsupported or empty assay guide"},
		"a guide of another format":     {withGuide(`{"format":"assay-sources/9","publisher":"P","entries":[` + entry + `]}`), "", "unsupported or empty assay guide"},
		"an entry with no panels":       {withGuide(`{"format":"stele-assay-sources/1","publisher":"P","entries":[` + strings.Replace(entry, `"panels":[],`, ``, 1) + `]}`), "", "invalid assay entry"},
		"an entry with no limit":        {withGuide(`{"format":"stele-assay-sources/1","publisher":"P","entries":[` + strings.Replace(entry, `"A limit."`, `""`, 1) + `]}`), "", "invalid assay entry"},
		"an entry with no TLS address":  {withGuide(`{"format":"stele-assay-sources/1","publisher":"P","entries":[` + strings.Replace(entry, `https://`, `http://`, 1) + `]}`), "", "invalid assay entry"},
		"an entry with an empty test":   {withGuide(`{"format":"stele-assay-sources/1","publisher":"P","entries":[` + strings.Replace(entry, `["A TEST"]`, `[""]`, 1) + `]}`), "", "empty assay test"},
		"an entry with no provider":     {withGuide(`{"format":"stele-assay-sources/1","publisher":"P","entries":[` + strings.Replace(entry, `"quest"`, `""`, 1) + `]}`), "", "invalid assay entry"},
		"a selection over the size cap": {selection(pin, "") + strings.Repeat(" ", 1<<20), "male-30-39", "exceeds"},
	} {
		path := filepath.Join(cache, "sources.json")
		if err := os.WriteFile(path, []byte(c.selection), 0600); err != nil {
			t.Fatal(err)
		}
		args := []string{"coverage", "-sources", path, "-json"}
		if c.cohort != "" {
			args = append(args, "-cohort", c.cohort)
		}
		if out, errs, code := run(t, dir, "", args...); code == 0 || out != "" || !strings.Contains(errs, c.want) {
			t.Errorf("%s: exit %d, want %q in: %s%s", name, code, c.want, out, errs)
		}
	}
	// The good forms of the same files are read.
	path := filepath.Join(cache, "sources.json")
	good := selection(fmt.Sprintf(`{"sha256":%q,"review":"https://example.org/r"}`, digest), fmt.Sprintf(`{"sha256":%q}`, guide(`{"format":"stele-assay-sources/1","publisher":"P","entries":[`+entry+`]}`)))
	if err := os.WriteFile(path, []byte(good), 0600); err != nil {
		t.Fatal(err)
	}
	if out, errs, code := run(t, dir, "", "coverage", "-sources", path, "-cohort", "male-30-39", "-json"); code != 0 || !strings.Contains(out, `"format":"bean-coverage/1"`) {
		t.Fatalf("a good selection: %d %s %s", code, out, errs)
	}
}
