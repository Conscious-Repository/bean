package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// testdata/quest-layout.html is the pdftotext -bbox-layout output of a real
// Quest Diagnostics report: every word sits where it did. Names, ids,
// dates, addresses, values and comment prose are synthetic, the values
// digit-mapped and the flags moved to other rows (DECISIONS 235). There is
// no synthetic PDF: these tests read the word positions directly, as the
// importer does with pdftotext -bbox-layout output given as input.

func questPagesFixture(t *testing.T) []bpage {
	t.Helper()
	b, err := os.ReadFile("testdata/quest-layout.html")
	if err != nil {
		t.Fatal(err)
	}
	pages, err := parseBBox(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return pages
}

func questFixture(t *testing.T) *Report {
	t.Helper()
	rep, err := parseQuest(questPagesFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestQuestLayoutReads(t *testing.T) {
	rep := questFixture(t)
	recs := rep.records("", nil)
	if len(recs) != 77 {
		t.Fatalf("%d readings, want 77", len(recs))
	}
	by := map[string][]map[string]any{}
	for _, r := range recs {
		by[r["type"].(string)] = append(by[r["type"].(string)], r)
		if r["source"].(map[string]any)["system"] != "quest" {
			t.Fatalf("source: %v", r["source"])
		}
	}
	if len(by) != 76 {
		t.Fatalf("%d types, want 76", len(by))
	}
	one := func(typ string) map[string]any {
		t.Helper()
		if len(by[typ]) != 1 {
			t.Fatalf("%s: %d readings", typ, len(by[typ]))
		}
		return by[typ][0]
	}
	type want struct {
		value                       any
		unit, ref, flag, panel, lab string
		calc                        bool
	}
	const site, other = "QUEST DIAGNOSTICS SITE B", "EXAMPLE LAB"
	for typ, w := range map[string]want{
		// A group under a group; the inner heading's name wraps onto the next
		// line, and its lab code goes to its tests.
		"lab.iron-total":            {271.0, "mcg/dL", "63-228", "H", "IRON, TIBC AND FERRITIN PANEL / IRON AND TOTAL IRON BINDING CAPACITY", site, false},
		"lab.iron-binding-capacity": {323.0, "mcg/dL", "383-558", "", "IRON, TIBC AND FERRITIN PANEL / IRON AND TOTAL IRON BINDING CAPACITY", site, true},
		"lab.ferritin":              {269.0, "ng/mL", "41-413", "L", "IRON, TIBC AND FERRITIN PANEL", site, false},
		"lab.t3-uptake":             {46.0, "%", "35-48", "", "THYROID PANEL WITH TSH / THYROID PANEL", site, false},
		"lab.tsh":                   {2.71, "mIU/L", "1.73-5.83", "L", "THYROID PANEL WITH TSH", site, false},
		// One test whose name wraps onto a second line, with its value.
		"lab.sex-hormone-binding-globulin": {61.0, "nmol/L", "23-63", "", "TESTOSTERONE, FREE, BIOAVAILABLE AND TOTAL, MS", other, false},
		"lab.testosterone-free":            {91.7, "pg/mL", "59.3-357.3", "", "TESTOSTERONE, FREE, BIOAVAILABLE AND TOTAL, MS / TESTOSTERONE, FREE AND BIOAVAILABLE", other, false},
		"lab.lipoprotein-a":                {"<23", "nmol/L", "<88", "", "", site, false},
		"lab.hdl-cholesterol":              {79.0, "mg/dL", "> OR = 53", "L", "LIPID PANEL WITH RATIOS", site, false},
		"lab.triglycerides":                {50.0, "mg/dL", "<283", "HH", "LIPID PANEL WITH RATIOS", site, false},
		"lab.ldl-cholesterol":              {250.0, "mg/dL", "<233", "", "LIPID PANEL WITH RATIOS", site, true},
		"lab.ldl-hdl-ratio":                {2.2, "", "", "", "LIPID PANEL WITH RATIOS", site, true},
		"lab.hs-crp":                       {1.0, "mg/L", "", "", "", site, false},
		"lab.homocysteine":                 {23.0, "umol/L", "< or = 25.2", "", "", site, false},
		"lab.apolipoprotein-b":             {14.0, "mg/dL", "<13", "", "", site, false},
		"lab.glucose":                      {234.0, "mg/dL", "78-12", "", "COMPREHENSIVE METABOLIC PANEL", site, false},
		"lab.egfr":                         {245.0, "mL/min/1.73m2", "> OR = 73", "", "COMPREHENSIVE METABOLIC PANEL", site, false},
		"lab.bun-creatinine-ratio":         {nil, "", "7-35", "", "COMPREHENSIVE METABOLIC PANEL", site, true},
		"lab.sodium":                       {262.0, "mmol/L", "268-279", "LL", "COMPREHENSIVE METABOLIC PANEL", site, false},
		"lab.bilirubin-total":              {3.7, "mg/dL", "1.5-2.5", "", "COMPREHENSIVE METABOLIC PANEL", site, false},
		"lab.hemoglobin-a1c":               {6.8, "% of total Hgb", "<6.0", "", "HEMOGLOBIN A1c WITH eAG", "QUEST DIAGNOSTICS-SITE C", false},
		"lab.eag-mg-dl":                    {244.0, "mg/dL", "", "", "HEMOGLOBIN A1c WITH eAG", "QUEST DIAGNOSTICS-SITE C", false},
		"lab.bilirubin-direct":             {1.7, "mg/dL", "< OR = 1.5", "", "BILIRUBIN, FRACTIONATED", site, false},
		"lab.white-blood-cell-count":       {7.9, "Thousand/uL", "4.1-23.1", "", "CBC (INCLUDES DIFF/PLT)", site, false},
		"lab.basophils":                    {2.5, "%", "", "", "CBC (INCLUDES DIFF/PLT)", site, false},
		"lab.vitamin-b12":                  {656.0, "pg/mL", "333-2433", "H", "", site, false},
		"lab.folate-rbc":                   {592.0, "ng/mL RBC", ">313", "", "", site, false},
		"lab.cortisol-total":               {29.6, "mcg/dL", "", "", "", site, false},
		"lab.estradiol":                    {"<43", "pg/mL", "< OR = 42", "", "", site, false},
		"lab.methylmalonic-acid":           {251.0, "nmol/L", "68-468", "", "", "QUEST DIAGNOSTICS SITE A", false},
		// The alternate table, under its section title.
		"lab.vitamin-d-25-oh-total-ia": {41.0, "ng/mL", "43-233", "", "Endocrinology", site, false},
	} {
		r := one(typ)
		v, ctx := r["value"].(map[string]any), r["context"].(map[string]any)
		got := v["n"]
		if s, ok := v["s"]; ok {
			got = s
		}
		calc, _ := ctx["calculated"].(bool)
		if got != w.value || str(v["unit"]) != w.unit || str(ctx["reference_interval"]) != w.ref || str(ctx["flag"]) != w.flag ||
			str(ctx["panel"]) != w.panel || str(ctx["performing_lab"]) != w.lab || calc != w.calc {
			t.Errorf("%s: value %v, context %v", typ, v, ctx)
		}
	}
	// Albumin in two panels with two readings: two readings of one type.
	albumin := by["lab.albumin"]
	if len(albumin) != 2 || albumin[0]["value"].(map[string]any)["n"] != 5.2 || albumin[1]["value"].(map[string]any)["n"] != 6.3 ||
		str(albumin[0]["context"].(map[string]any)["panel"]) == str(albumin[1]["context"].(map[string]any)["panel"]) {
		t.Fatalf("albumin: %v", albumin)
	}
	if a, b := albumin[0]["source"].(map[string]any)["ref_input"], albumin[1]["source"].(map[string]any)["ref_input"]; str(a.([]any)[3]) == str(b.([]any)[3]) {
		t.Fatalf("one native id for two readings: %v", a)
	}
	// SEE NOTE: no value, and the note under it.
	if c := str(one("lab.bun-creatinine-ratio")["context"].(map[string]any)["lab_comment"]); !strings.HasPrefix(c, "Not Reported:") {
		t.Fatalf("SEE NOTE comment: %q", c)
	}
	// The printed digits, and the collection time in the printed zone.
	r := one("lab.free-t4-index-t7")
	if r["context"].(map[string]any)["raw_value"] != "3.0" {
		t.Fatalf("raw value: %v", r["context"])
	}
	obs := r["times"].(map[string]any)["observed"].(map[string]any)
	if obs["start"] != "2025-06-10T07:41:00-05:00" || obs["precision"] != "minute" {
		t.Fatalf("observed: %v", obs)
	}
	ctx := r["context"].(map[string]any)
	if ctx["reported"] != "06/12/2025" || ctx["fasting"] != "Yes" || ctx["report_note"] != "FASTING:YES" || ctx["raw_time"] != nil {
		t.Fatalf("context: %v", ctx)
	}
	// A note that starts at a row indent is a comment, and an endnote goes to
	// the test that cites it.
	res := map[string]*Result{}
	for _, x := range rep.Results {
		res[slug(x.Name)] = x
	}
	if n := len(res["vitamin-d-25-oh-total-ia"].Comment); n != 8 {
		t.Fatalf("vitamin D: %d comment lines, want 8", n)
	}
	mma := res["methylmalonic-acid"].Comment
	if len(mma) != 23 || mma[0] != "See Endnote 1" {
		t.Fatalf("methylmalonic acid: %d comment lines, want 23 with the endnote", len(mma))
	}
	if len(rep.Notes) != 1 {
		t.Fatalf("report notes: %q", rep.Notes)
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// findLine is the page and line whose text starts with prefix.
func findLine(t *testing.T, pages []bpage, prefix string) *bline {
	t.Helper()
	for p := range pages {
		for i := range pages[p].lines {
			if strings.HasPrefix(pages[p].lines[i].text(), prefix) {
				return &pages[p].lines[i]
			}
		}
	}
	t.Fatalf("no line %q", prefix)
	return nil
}

// insertAfter puts a line of words after l, 5 points below it.
func insertAfter(pages []bpage, l *bline, ws ...word) {
	for p := range pages {
		for i := range pages[p].lines {
			if &pages[p].lines[i] == l {
				nl := bline{page: l.page, y: l.y + 5, words: ws}
				pages[p].lines = append(pages[p].lines[:i+1], append([]bline{nl}, pages[p].lines[i+1:]...)...)
				return
			}
		}
	}
}

// w is a word at x of a monospace font, 6 points a character.
func w(x float64, text string) word { return word{x, 0, x + 6*float64(len(text)), 8, text} }

func TestQuestRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		edit       func(t *testing.T, pages []bpage)
	}{
		{"a row with a range and no value", "a row with a range or unit and no value", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "GGT ")
			l.words = append(l.words[:1], l.words[2:]...)
		}},
		{"a test with no value and nothing under it", "a row with no value and no rows under it", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "GGT ")
			l.words = append(l.words[:1], l.words[len(l.words)-1])
		}},
		{"SEE NOTE with no note", "no note follows it", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "Not Reported:")
			for p := range pages {
				for i := range pages[p].lines {
					if &pages[p].lines[i] == l {
						pages[p].lines = append(pages[p].lines[:i], pages[p].lines[i+2:]...) // the note is two lines
						return
					}
				}
			}
		}},
		{"a value in Out Of Range with no flag", "Out Of Range column with no flag", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "TRIGLYCERIDES")
			l.words = append(l.words[:2], l.words[3:]...)
		}},
		{"a flag in In Range", "a flag in the In Range column", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "POTASSIUM")
			l.words = append(l.words[:2], append([]word{w(l.words[1].x1+6, "H")}, l.words[2:]...)...)
		}},
		{"values in two columns", "a value in two columns", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "POTASSIUM")
			l.words = append(l.words[:2], append([]word{w(315.8, "5"), w(333.8, "H")}, l.words[2:]...)...)
		}},
		{"a value across two columns", "runs from one column into the next", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "POTASSIUM")
			l.words[1] = w(380, "5.6000")
		}},
		{"a word after a gap in the name column", "test name column after a gap", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "POTASSIUM")
			l.words = append(l.words[:1], append([]word{w(150, "5.6")}, l.words[1:]...)...)
		}},
		{"a test name run into the values", "runs into the value columns", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "POTASSIUM")
			l.words = append(l.words[:1], append([]word{w(l.words[0].x1+6, strings.Repeat("X", 25))}, l.words[1:]...)...)
		}},
		{"a value the grammar does not know", "not a number or a result", func(t *testing.T, pages []bpage) {
			findLine(t, pages, "POTASSIUM").words[1].text = "1,234"
		}},
		{"a reference column it cannot read", "Reference Range column", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "POTASSIUM")
			l.words[3].text = "mmol"
		}},
		{"a lab code not in the list", "does not name it", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "GGT ")
			l.words[len(l.words)-1].text = "ZZ"
		}},
		{"two names for one type", "map to one type", func(t *testing.T, pages []bpage) {
			findLine(t, pages, "eAG (mmol/L)").words[1].text = "(mg/dL)."
		}},
		{"one test twice in a group", "printed twice in one group", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "MONOCYTES ")
			ws := append([]word{}, l.words...)
			ws[1].text = "9.9"
			insertAfter(pages, l, ws...)
		}},
		{"a test with a row under it", "a test with rows indented under it", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "GGT ")
			insertAfter(pages, l, w(39.7, "EXTRA"), w(239.7, "1"))
		}},
		{"a row indented with no heading", "no group heading above it", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "IRON, TIBC")
			for i := range l.words {
				l.words[i].x0 += 10.8
				l.words[i].x1 += 10.8
			}
		}},
		{"a line between the indents and the comments", "between the row indents and the comment column", func(t *testing.T, pages []bpage) {
			insertAfter(pages, findLine(t, pages, "GGT "), w(45, "stray"))
		}},
		{"a comment that reads as a row", "a comment reads as a result row", func(t *testing.T, pages []bpage) {
			insertAfter(pages, findLine(t, pages, "GGT "), w(61.3, "DEEPER"), w(239.7, "12"))
		}},
		{"a line at the margin", "a line at the margin", func(t *testing.T, pages []bpage) {
			insertAfter(pages, findLine(t, pages, "GGT "), w(18, "NOTE"))
		}},
		{"a footer naming another specimen", "a page footer names another specimen", func(t *testing.T, pages []bpage) {
			findLine(t, pages, "CLIENT SERVICES:").words[4].text = "OTHER"
		}},
		{"a missing page", "a page may be missing", func(t *testing.T, pages []bpage) {
			findLine(t, pages, "CLIENT SERVICES:").words[8].text = "8"
		}},
		{"no page number", "has no PAGE n OF m footer", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "CLIENT SERVICES:")
			l.words = l.words[:5]
		}},
		{"a field the specimen column does not know", "the specimen column holds a line", func(t *testing.T, pages []bpage) {
			findLine(t, pages, "Requisition:").words[0].text = "Accession:"
		}},
		{"two collection times", "conflicting collection dates or times", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "Collected:")
			l.words[3].text = "07:42"
		}},
		{"a preliminary report", "the report status is not Final", func(t *testing.T, pages []bpage) {
			findLine(t, pages, "Report Status:").words[2].text = "Preliminary"
		}},
		{"a cited endnote that is not printed", "which the report does not print", func(t *testing.T, pages []bpage) {
			findLine(t, pages, "Endnote 1").words[1].text = "2"
		}},
		{"a column header after the performing sites", "a column header after the results", func(t *testing.T, pages []bpage) {
			l := findLine(t, pages, "Test Name Result")
			insertAfter(pages, findLine(t, pages, "PERFORMING SITE:"), l.words...)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pages := questPagesFixture(t)
			tc.edit(t, pages)
			if _, err := parseQuest(pages); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error with %q", err, tc.want)
			}
		})
	}
}

// The edits above refuse because of what they change: the fixture itself
// reads, and a harmless edit still does.
func TestQuestEditsAreTargeted(t *testing.T) {
	pages := questPagesFixture(t)
	findLine(t, pages, "POTASSIUM").words[1].text = "5.7"
	rep, err := parseQuest(pages)
	if err != nil || len(rep.Results) != 77 {
		t.Fatalf("%v", err)
	}
}

func TestQuestTimeZone(t *testing.T) {
	rep := questFixture(t)
	chicago, _ := time.LoadLocation("America/Chicago")
	newYork, _ := time.LoadLocation("America/New_York")
	if err := rep.checkZone(chicago); err != nil {
		t.Fatalf("-tz agrees with CDT in June: %v", err)
	}
	if err := rep.checkZone(newYork); err == nil || !strings.Contains(err.Error(), "leave -tz out") {
		t.Fatalf("-tz disagrees: %v", err)
	}
	// A zone it does not know leaves the day, and the printed time.
	pages := questPagesFixture(t)
	for p := range pages {
		for i := range pages[p].lines {
			for j := range pages[p].lines[i].words {
				if pages[p].lines[i].words[j].text == "CDT" {
					pages[p].lines[i].words[j].text = "XST"
				}
			}
		}
	}
	rep, err := parseQuest(pages)
	if err != nil {
		t.Fatal(err)
	}
	if err := rep.checkZone(newYork); err != nil {
		t.Fatal(err)
	}
	r := rep.records("", newYork)[0]
	obs := r["times"].(map[string]any)["observed"].(map[string]any)
	if obs["start"] != "2025-06-10T00:00:00Z" || obs["precision"] != "day" || r["context"].(map[string]any)["raw_time"] != "06/10/2025 / 07:41 XST" {
		t.Fatalf("unknown zone: %v %v", obs, r["context"])
	}
}

func TestQuestInput(t *testing.T) {
	b, err := os.ReadFile("testdata/quest-layout.html")
	if err != nil {
		t.Fatal(err)
	}
	// The word positions given as input read like the PDF.
	rep, err := readReport(string(b), b)
	if err != nil || len(rep.Results) != 77 || rep.System != questSystem {
		t.Fatalf("%v", err)
	}
	if rec := archiveFor(rep.system(), b, "sha256:"+strings.Repeat("a", 64)); rec["source"].(map[string]any)["system"] != "quest" ||
		rec["blobs"].([]any)[0].(map[string]any)["media_type"] != "application/xhtml+xml" {
		t.Fatalf("archive: %v", rec)
	}
	// -layout text of a Quest report is refused: its columns drift.
	text := "   Test Name     In Range     Out Of Range     Reference Range     Lab\n" +
		"   GLUCOSE       90                            65-99 mg/dL         QA\n" + questTrademark + "\n"
	if _, err := readReport(text, []byte(text)); err == nil || !strings.Contains(err.Error(), "give the PDF") {
		t.Fatalf("layout text: %v", err)
	}
	// Word positions of another report are refused.
	other := strings.Replace(string(b), "trademarks", "marks", -1)
	if _, err := readReport(other, []byte(other)); err == nil || !strings.Contains(err.Error(), "not a Quest Diagnostics report") {
		t.Fatalf("other layout: %v", err)
	}
}

func TestBBoxRefuses(t *testing.T) {
	page := func(body string) string {
		return `<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><head><title></title></head><body><doc>` +
			`<page width="612" height="792">` + body + `</page></doc></body></html>`
	}
	for _, tc := range []struct{ doc, want string }{
		{page(`<word xMin="10" yMin="10" xMax="40" yMax="18">A</word><word xMin="30" yMin="10" xMax="60" yMax="18">B</word>`), "printed over each other"},
		{page(`<word xMin="10" yMin="10" xMax="700" yMax="18">A</word>`), "not inside the page"},
		{page(`<word xMin="10" yMin="10" xMax="40" yMax="18">A&#x200B;</word>`), "invisible"},
		{page(`<script>x</script>`), `"script" element`},
		{page(`text`), "text outside a word"},
		{page(`<word xMin="10" yMin="10" xMax="40" yMax="18">A &nbsp;</word>`), "not well-formed"},
		{`<!DOCTYPE html><html><body><doc></doc></body></html>`, "no page"},
	} {
		if _, err := parseBBox(tc.doc); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: got %v, want %q", tc.doc, err, tc.want)
		}
	}
	// Words on one line within a point of each other's top, left to right.
	pages, err := parseBBox(page(`<word xMin="50" yMin="10.5" xMax="60" yMax="18">B</word><word xMin="10" yMin="10" xMax="20" yMax="18">A</word><word xMin="10" yMin="20" xMax="20" yMax="28">C&amp;</word>`))
	if err != nil || len(pages[0].lines) != 2 || pages[0].lines[0].text() != "A B" || pages[0].lines[1].text() != "C&" {
		t.Fatalf("%v %+v", err, pages)
	}
}
