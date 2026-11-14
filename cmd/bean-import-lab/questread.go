package main

// A Quest report is read by word position (DECISIONS 235). Each line of each
// page is one known kind, or the report is refused and the error names the
// page and how far down it is:
//
//   - page boilerplate: "Report Status: Final" and the patient's name under
//     it, the "CLIENT SERVICES: ... SPECIMEN: ... PAGE n OF m" footer and the
//     trademark line
//   - the block under "Patient Information / Specimen Information / Client
//     Information": the specimen column is fields only, and is read for the
//     specimen and the times; the patient column for Fasting
//   - a column header, which sets the columns of the rows under it, and the
//     line directly before one, a section title ("Endocrinology")
//   - at the margin: "COMMENTS:", "Endnote n" and its text, "PERFORMING
//     SITE:" and its lab lines, "Physician Comments:" and its text
//   - in a table, by where the line starts: at a row indent, a row; from the
//     comment column on, a comment of the test above it
//
// Rows form a tree by indent: a row with a value is a test, a row without
// one is a group heading, or the first line of a wrapped name.

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Conscious-Repository/bean/core"
)

const (
	qResults = iota
	qEndnote
	qPhysician
	qSites
)

// qline is a line of the results, in reading order.
type qline struct {
	item    *qitem // a row, or nil
	comment string // else a comment
	brk     bool   // else a new table or section: the rows above are no group of the rows below
	section string // the section title a break starts, if any
	page    int
	y       float64
}

// qitem is a row: a test, a group heading, or part of a wrapped name.
type qitem struct {
	page        int
	y           float64
	depth       int
	name        string
	code        string
	value, flag string
	seeNote     bool
	ref, unit   string
	calc        bool
	cells       bool // a value, range, unit or "(calc)"
	phrase      bool // the whole line is one phrase: no column, no code
	comments    []string
}

type questReader struct {
	rep      *Report
	pages    []bpage
	seq      []qline
	table    *questTable
	cols     []float64 // where each column after the name starts
	region   int
	endnote  string
	endnotes map[string][]string
	footers  []string // the specimen of each footer
	pageNos  map[int]bool
}

func parseQuest(pages []bpage) (*Report, error) {
	r := &questReader{
		rep:      &Report{System: questSystem, Labs: map[string]string{}},
		pages:    pages,
		endnotes: map[string][]string{},
		pageNos:  map[int]bool{},
	}
	for i := range pages {
		if err := r.page(i + 1); err != nil {
			return nil, err
		}
	}
	return r.finish()
}

func (r *questReader) where(page int, y float64) string {
	return fmt.Sprintf("page %d, %s", page, atY(y))
}

// headerOf is the table whose column header l is, or nil.
func headerOf(l bline) *questTable {
	for _, t := range questTables {
		if wordsAre(l.words, t.header...) {
			return t
		}
	}
	return nil
}

func (r *questReader) page(page int) error {
	pg := r.pages[page-1]
	if r.region == qEndnote {
		r.region = qResults // an endnote's text ends with its page
	}
	var block []float64 // the starts of the patient, specimen and client columns, while in the block
	named := false      // the block's patient column has had its name line
	status := false     // the line above is "Report Status: Final"
	clientServices := false
	for i, l := range pg.lines {
		ws := l.words
		where := r.where(page, l.y)
		afterStatus := status
		status = false
		afterClient := clientServices
		clientServices = false
		switch {
		case wordsAre(ws, "Report", "Status:", "Final"):
			status = true
			continue
		case len(ws) >= 2 && ws[0].text == "Report" && ws[1].text == "Status:":
			return fmt.Errorf("%s: the report status is not Final", where)
		case afterStatus && ws[0].x0 >= pg.width/2:
			continue // the patient's name, at the top right; not read
		case l.text() == questTrademark:
			continue
		case len(ws) >= 5 && wordsAre(ws[:2], "CLIENT", "SERVICES:"):
			ok, err := r.footer(page, where, ws)
			if err != nil {
				return err
			}
			clientServices = !ok
			continue
		case afterClient && len(ws) == 4 && ws[0].text == "PAGE":
			if err := r.pageNo(page, where, ws); err != nil {
				return err
			}
			continue
		case wordsAre(ws, questBlockHeading...):
			block, named = []float64{ws[0].x0, ws[2].x0, ws[4].x0}, false
			continue
		}
		var next *questTable
		if i+1 < len(pg.lines) {
			next = headerOf(pg.lines[i+1])
		}
		t := headerOf(l)
		if block != nil && t == nil {
			// A section title ends the block: one phrase in the patient
			// column, not a field, directly before a column header. It names
			// the group of the rows under it.
			if next != nil && phrase(ws) == len(ws) && ws[len(ws)-1].x1 < block[1] && !patientLabel(ws) {
				if r.region != qResults {
					return fmt.Errorf("%s: a section title after the results, so a result there would be lost", where)
				}
				r.seq = append(r.seq, qline{brk: true, section: l.text(), page: page, y: l.y})
				block = nil
				continue
			}
			if ws[0].x0 >= block[0]-indentTolerance {
				if err := r.blockLine(where, block, &named, ws); err != nil {
					return err
				}
				continue
			}
		}
		block = nil
		if t != nil {
			if err := r.header(where, t, ws); err != nil {
				return err
			}
			continue
		}
		if err := r.line(page, where, l); err != nil {
			return err
		}
	}
	if !r.pageNos[page] {
		return fmt.Errorf("page %d has no PAGE n OF m footer, so a page may be missing", page)
	}
	return nil
}

// footer reads "CLIENT SERVICES: <phone> SPECIMEN: <id>", with "PAGE n OF m"
// on the same line or the next. It reports whether the page number was on it.
func (r *questReader) footer(page int, where string, ws []word) (bool, error) {
	if !reQuestPhone.MatchString(ws[2].text) || ws[3].text != "SPECIMEN:" {
		return false, fmt.Errorf("%s: a footer line this importer does not know", where)
	}
	r.footers = append(r.footers, ws[4].text)
	switch len(ws) {
	case 5:
		return false, nil
	case 9:
		return true, r.pageNo(page, where, ws[5:])
	}
	return false, fmt.Errorf("%s: a footer line this importer does not know", where)
}

func (r *questReader) pageNo(page int, where string, ws []word) error {
	if ws[0].text != "PAGE" || ws[2].text != "OF" || !reQuestPage.MatchString(ws[1].text) || !reQuestPage.MatchString(ws[3].text) {
		return fmt.Errorf("%s: a footer line this importer does not know", where)
	}
	if ws[1].text != fmt.Sprint(page) || ws[3].text != fmt.Sprint(len(r.pages)) || r.pageNos[page] {
		return fmt.Errorf("%s: the footer says PAGE %q OF %q, but this is page %d of %d: a page may be missing", where, ws[1].text, ws[3].text, page, len(r.pages))
	}
	r.pageNos[page] = true
	return nil
}

// blockLine reads a line of the block at the top of a page, word by word
// into its three columns. The client column (its number, the physician and
// the address) is not read.
func (r *questReader) blockLine(where string, block []float64, named *bool, ws []word) error {
	var pat, spec []word
	for _, w := range ws {
		switch {
		case w.x0 < block[1]-indentTolerance:
			if w.x1 > block[1] {
				return fmt.Errorf("%s: a word runs from the patient column into the specimen column", where)
			}
			pat = append(pat, w)
		case w.x0 < block[2]-indentTolerance:
			if w.x1 > block[2] {
				return fmt.Errorf("%s: a word runs from the specimen column into the client column", where)
			}
			spec = append(spec, w)
		}
	}
	if len(spec) > 0 {
		if err := r.specimenField(where, spec); err != nil {
			return err
		}
	}
	if len(pat) > 0 {
		return r.patientFields(where, named, pat)
	}
	return nil
}

func hasLabel(ws []word, label []string) bool {
	return len(ws) >= len(label) && wordsAre(ws[:len(label)], label...)
}

// patientLabel reports a line that starts with a field of the patient column.
func patientLabel(ws []word) bool {
	for _, l := range questPatientLabels {
		if hasLabel(ws, l) {
			return true
		}
	}
	return false
}

// specimenField reads a line of the specimen column: one field, its value
// in its shape. The specimen, the times and the report day must agree on
// every page.
func (r *questReader) specimenField(where string, ws []word) error {
	rep := r.rep
	for _, f := range questSpecimenFields {
		if !hasLabel(ws, f.label) {
			continue
		}
		v := joinWords(ws[len(f.label):])
		m := f.value.FindStringSubmatch(v)
		if m == nil {
			return fmt.Errorf("%s: malformed %q field", where, strings.Join(f.label, " "))
		}
		switch f.label[0] {
		case "Specimen:":
			if strings.Contains(v, "/") {
				return errors.New("ambiguous specimen identifier")
			}
			if rep.Specimen != "" && rep.Specimen != v {
				return errors.New("conflicting specimen identifiers")
			}
			rep.Specimen = v
		case "Collected:":
			if r.rep.printedTime != "" && r.rep.printedTime != v {
				return errors.New("conflicting collection dates or times")
			}
			rep.printedTime, rep.Collected, rep.zone = v, m[1], m[4]
			if m[2] != "" {
				rep.Collected += " " + m[2] + m[3]
			}
		case "Received:":
			rep.Received = m[1]
		case "Reported:":
			if rep.Reported != "" && rep.Reported != m[1] {
				return errors.New("conflicting report dates")
			}
			rep.Reported = m[1]
		}
		return nil
	}
	return fmt.Errorf("%s: the specimen column holds a line this importer does not know", where)
}

// patientFields reads a line of the patient column: fields, each a label
// and the words up to the next label, or the patient's name, once a page.
// Only Fasting is read.
func (r *questReader) patientFields(where string, named *bool, ws []word) error {
	label := func(i int) []string {
		for _, l := range questPatientLabels {
			if hasLabel(ws[i:], l) {
				return l
			}
		}
		return nil
	}
	if label(0) == nil {
		if *named {
			return fmt.Errorf("%s: the patient column holds a line this importer does not know", where)
		}
		*named = true // the patient's name; not read
		return nil
	}
	for i := 0; i < len(ws); {
		l := label(i)
		j := i + len(l)
		for j < len(ws) && label(j) == nil {
			j++
		}
		if l[0] == "Fasting:" {
			v := questFasting[joinWords(ws[i+len(l):j])]
			if v == "" {
				return fmt.Errorf("%s: malformed \"Fasting\" field", where)
			}
			if r.rep.Fasting != "" && r.rep.Fasting != v {
				return errors.New("conflicting fasting fields")
			}
			r.rep.Fasting = v
		}
		i = j
	}
	return nil
}

// header sets the columns of the rows under a column header.
func (r *questReader) header(where string, t *questTable, ws []word) error {
	if r.region == qSites || r.region == qPhysician {
		return fmt.Errorf("%s: a column header after the results, so results there would be lost", where)
	}
	cols := make([]float64, len(t.columns))
	for i, k := range t.columns {
		cols[i] = ws[k].x0
		if i > 0 && cols[i] <= cols[i-1] {
			return fmt.Errorf("%s: the column header is out of order", where)
		}
	}
	if n := len(r.seq); t != r.table && (n == 0 || !r.seq[n-1].brk) {
		r.seq = append(r.seq, qline{brk: true})
	}
	r.table, r.cols, r.region = t, cols, qResults
	return nil
}

// margin is where rows start: a line left of it is at the margin.
func (r *questReader) margin() float64 {
	t := r.table
	if t == nil {
		t = questMain
	}
	return t.indents[0] - indentTolerance
}

func (r *questReader) line(page int, where string, l bline) error {
	ws := l.words
	x := ws[0].x0
	switch {
	case wordsAre(ws, "Physician", "Comments:") && x < r.margin()+2*indentTolerance:
		if r.region == qSites {
			return fmt.Errorf("%s: Physician Comments after the performing sites", where)
		}
		r.region = qPhysician
		return nil
	case x < r.margin():
		return r.marginLine(page, where, ws)
	case r.region == qEndnote:
		if r.rowLike(ws) {
			return fmt.Errorf("%s: a line of an endnote reads as a result row, so its reading would be lost", where)
		}
		r.endnotes[r.endnote] = append(r.endnotes[r.endnote], l.text())
		return nil
	case r.region == qPhysician:
		if r.rowLike(ws) {
			return fmt.Errorf("%s: a line of the physician comments reads as a result row, so its reading would be lost", where)
		}
		r.rep.Notes = append(r.rep.Notes, l.text())
		return nil
	case r.region == qSites:
		return fmt.Errorf("%s: after PERFORMING SITE: only its lab lines may follow", where)
	case r.table == nil:
		return fmt.Errorf("%s: a line before the column header that this importer does not know", where)
	}
	t := r.table
	for d, ind := range t.indents {
		if math.Abs(x-ind) <= indentTolerance {
			return r.row(page, where, l, d)
		}
	}
	if x < t.commentFrom {
		return fmt.Errorf("%s: a line starts between the row indents and the comment column, so it cannot be read for sure", where)
	}
	if r.rowLike(ws) {
		return fmt.Errorf("%s: a comment reads as a result row: it may be a row indented further than this importer knows, and its reading would be lost", where)
	}
	r.seq = append(r.seq, qline{comment: l.text(), page: page, y: l.y})
	return nil
}

// marginLine reads a line at the margin: "COMMENTS:" and the report's
// comment, "Endnote n", "PERFORMING SITE:", or a performing site.
func (r *questReader) marginLine(page int, where string, ws []word) error {
	switch {
	case ws[0].text == "COMMENTS:" && r.region == qResults:
		if r.rowLike(ws) {
			return fmt.Errorf("%s: the report comment reads as a result row", where)
		}
		if len(ws) > 1 {
			r.rep.Notes = append(r.rep.Notes, joinWords(ws[1:]))
		}
		return nil
	case len(ws) == 2 && ws[0].text == "Endnote" && reEndnote.MatchString(ws[1].text) && r.region != qSites:
		if r.endnotes[ws[1].text] != nil {
			return fmt.Errorf("%s: endnote %q is printed twice", where, ws[1].text)
		}
		r.region, r.endnote = qEndnote, ws[1].text
		r.endnotes[r.endnote] = []string{}
		return nil
	case wordsAre(ws, "PERFORMING", "SITE:"):
		r.region = qSites
		return nil
	case r.region == qSites:
		return r.site(where, ws)
	}
	return fmt.Errorf("%s: a line at the margin that this importer does not know", where)
}

// site reads a performing site: "<code>  <name>, <address> Laboratory
// Director: <name>, CLIA: <number>".
func (r *questReader) site(where string, ws []word) error {
	rest := joinWords(ws[1:])
	name, _, ok := strings.Cut(rest, ",")
	if len(ws) < 3 || !reQuestCode.MatchString(ws[0].text) || !spaced(ws[0], ws[1]) || !ok || strings.TrimSpace(name) == "" ||
		!strings.Contains(rest, " Laboratory Director: ") || !strings.Contains(rest, ", CLIA: ") {
		return fmt.Errorf("%s: a performing site line this importer does not know", where)
	}
	code := ws[0].text
	if old := r.rep.Labs[code]; old != "" && old != name {
		return fmt.Errorf("%s: two performing sites with code %q", where, code)
	}
	r.rep.Labs[code] = name
	return nil
}

// column is the column a word starting at x is in: 0 the name, then each
// column whose start it is at or right of.
func (r *questReader) column(x float64) int {
	for c := len(r.cols); c >= 1; c-- {
		if x >= r.cols[c-1]-indentTolerance {
			return c
		}
	}
	return 0
}

// rowLike reports a line of free text that reads as a row: it starts left
// of the value columns, and after a column gap a word that is a value
// starts in a value column. As text, its reading would be lost.
func (r *questReader) rowLike(ws []word) bool {
	if r.table == nil || ws[0].x0 >= r.cols[0]-indentTolerance {
		return false
	}
	for i := 1; i < len(ws); i++ {
		if !spaced(ws[i-1], ws[i]) || !slices.Contains(r.table.value, r.column(ws[i].x0)) {
			continue
		}
		if isValue(ws[i].text) || strings.HasPrefix(joinWords(ws[i:]), seeNote) {
			return true
		}
	}
	return false
}

// row reads a line that starts at a row indent: the name, a phrase from the
// indent, then each word in the column whose start it is at or right of.
func (r *questReader) row(page int, where string, l bline, depth int) error {
	t, ws := r.table, l.words
	n := phrase(ws)
	it := &qitem{page: page, y: l.y, depth: depth, name: joinWords(ws[:n]), phrase: n == len(ws)}
	cells := make([][]word, len(r.cols)+1)
	for _, w := range ws[n:] {
		k := r.column(w.x0)
		if k == 0 {
			return fmt.Errorf("%s: a word in the test name column after a gap, so the row cannot be read for sure", where)
		}
		if k < len(r.cols) && w.x1 > r.cols[k]+indentTolerance {
			return fmt.Errorf("%s: %q runs from one column into the next, so its column cannot be read for sure", where, w.text)
		}
		cells[k] = append(cells[k], w)
	}
	if lab := cells[t.lab]; len(lab) > 0 {
		if len(lab) != 1 || !reQuestCode.MatchString(lab[0].text) {
			return fmt.Errorf("%s: %q in the Lab column is not a lab code", where, joinWords(lab))
		}
		it.code = lab[0].text
	}
	var filled []int
	for _, k := range t.value {
		if len(cells[k]) > 0 {
			filled = append(filled, k)
		}
	}
	if len(filled) > 1 {
		return fmt.Errorf("%s: a row with a value in two columns", where)
	}
	it.cells = len(filled) > 0 || len(cells[t.ref]) > 0
	if it.cells && ws[n-1].x1 > r.cols[0]+indentTolerance {
		return fmt.Errorf("%s: a test name runs into the value columns, so its value cannot be read for sure", where)
	}
	if len(filled) == 1 {
		if err := it.readValue(t, filled[0], cells[filled[0]]); err != nil {
			return fmt.Errorf("%s: %v", where, err)
		}
	}
	if err := it.readRef(cells[t.ref]); err != nil {
		return fmt.Errorf("%s: %v", where, err)
	}
	r.seq = append(r.seq, qline{item: it, page: page, y: l.y})
	return nil
}

// readValue reads a value cell. In Range holds a value, or "SEE NOTE:"; Out
// Of Range a value and its flag; Result a value and an optional flag.
func (it *qitem) readValue(t *questTable, k int, ws []word) error {
	v, flag := joinWords(ws), ""
	if last := ws[len(ws)-1].text; len(ws) > 1 && questFlags[last] {
		v, flag = joinWords(ws[:len(ws)-1]), last
	}
	switch {
	case t == questMain && k == 1 && v == seeNote && flag == "":
		it.seeNote = true
		return nil
	case !isValue(v):
		return fmt.Errorf("%q is not a number or a result this importer knows", v)
	case t == questMain && k == 1 && flag != "":
		return fmt.Errorf("a flag in the In Range column")
	case t == questMain && k == 2 && flag == "":
		return fmt.Errorf("a value in the Out Of Range column with no flag")
	}
	it.value, it.flag = v, flag
	return nil
}

// readRef reads the Reference Range column: a range, then a unit, then
// "(calc)", each optional. A result word alone ("NEGATIVE") is a range too.
func (it *qitem) readRef(ws []word) error {
	s := joinWords(ws)
	if rest, ok := strings.CutSuffix(s, "(calc)"); ok {
		s, it.calc = strings.TrimSpace(rest), true
	}
	if m := reQuestRange.FindString(s); m != "" {
		it.ref, s = strings.TrimSpace(m), s[len(m):]
	} else if s != "" && isValue(s) && !reNumberValue.MatchString(s) {
		it.ref, s = s, ""
	}
	if s == "" {
		return nil
	}
	f := strings.Fields(s)
	if !unitOK(f[0]) || len(f) > 4 {
		return fmt.Errorf("%q in the Reference Range column is not a range and a unit this importer knows", joinWords(ws))
	}
	for _, w := range f[1:] {
		if !reNameWord.MatchString(w) || strings.Contains(w, "-") || isValue(w) || isFlag(w) || questFlags[w] {
			return fmt.Errorf("%q in the Reference Range column is not a range and a unit this importer knows", joinWords(ws))
		}
	}
	it.unit = s
	return nil
}

func (r *questReader) finish() (*Report, error) {
	rep := r.rep
	if rep.Specimen == "" || rep.Collected == "" {
		return nil, errors.New("not a lab report in a layout this importer knows: no specimen id or collection date")
	}
	for _, s := range r.footers {
		if s != rep.Specimen {
			return nil, errors.New("conflicting specimen identifiers: a page footer names another specimen")
		}
	}
	r.resolveProse()
	// Comments go to the row above them; a break starts a new tree.
	type segment struct {
		section string
		items   []*qitem
	}
	segs := []segment{{}}
	var cur *qitem
	for _, q := range r.seq {
		switch {
		case q.brk:
			cur = nil
			segs = append(segs, segment{section: q.section})
		case q.item != nil:
			cur = q.item
			segs[len(segs)-1].items = append(segs[len(segs)-1].items, cur)
		case cur == nil:
			return nil, fmt.Errorf("%s: a comment with no test above it", r.where(q.page, q.y))
		default:
			cur.comments = append(cur.comments, q.comment)
		}
	}
	for _, s := range segs {
		var path []string
		if s.section != "" {
			path = []string{s.section}
		}
		if err := r.tree(s.items, 0, path, ""); err != nil {
			return nil, err
		}
	}
	if len(rep.Results) == 0 {
		return nil, errors.New("no results found")
	}
	if err := r.check(); err != nil {
		return nil, err
	}
	return rep, nil
}

// resolveProse reads a row that is one phrase, with no column, under a
// comment and with no row indented under it, as more of that comment. A
// lab's note may start at a row indent (a long note under one test does on
// the real report), and a phrase holds no value: a row's value is in a
// column of its own.
func (r *questReader) resolveProse() {
	for i := 1; i < len(r.seq); i++ {
		q, prev := &r.seq[i], r.seq[i-1]
		if q.item == nil || !q.item.phrase || prev.item != nil || prev.brk {
			continue
		}
		if i+1 < len(r.seq) && r.seq[i+1].item != nil && r.seq[i+1].item.depth > q.item.depth {
			continue // a group heading
		}
		*q = qline{comment: q.item.name, page: q.page, y: q.y}
	}
}

// tree reads the rows of one group, at depth, and the groups under them.
// A row with a value is a test. A row without one is a group heading, and
// the rows indented under it are its group, except:
//
//   - a row with no value, no lab code and nothing under it, one indent
//     deeper and directly after a heading, is the rest of the heading's name
//     ("COMPREHENSIVE METABOLIC" and "PANEL");
//   - a heading with exactly one row under it, a test with nothing under it,
//     is that test, its name wrapped ("SEX HORMONE BINDING" and "GLOBULIN").
//     So a real group of one test is read as one test with the two names
//     joined; the review shows it.
//
// Children take their group's lab code unless they print their own.
func (r *questReader) tree(seg []*qitem, depth int, path []string, code string) error {
	for i := 0; i < len(seg); {
		it := seg[i]
		where := r.where(it.page, it.y)
		if it.depth != depth {
			return fmt.Errorf("%s: a row indented with no group heading above it", where)
		}
		c := code
		if it.code != "" {
			c = it.code
		}
		if it.value != "" || it.seeNote {
			if i+1 < len(seg) && seg[i+1].depth > depth {
				return fmt.Errorf("%s: a test with rows indented under it", where)
			}
			if err := r.emit(it, it.name, path, c); err != nil {
				return err
			}
			i++
			continue
		}
		switch {
		case it.cells:
			return fmt.Errorf("%s: a row with a range or unit and no value", where)
		case len(it.comments) > 0:
			return fmt.Errorf("%s: a row with no value and a comment under it: it is neither a test this importer can read nor a group heading", where)
		}
		name, j := it.name, i+1
		for j < len(seg) && seg[j].depth == depth+1 && seg[j].bare() && (j+1 == len(seg) || seg[j+1].depth <= seg[j].depth) {
			name += " " + seg[j].name
			j++
		}
		k := j
		for k < len(seg) && seg[k].depth > depth {
			k++
		}
		switch {
		case k == j:
			return fmt.Errorf("%s: a row with no value and no rows under it: a test with no value is refused unless it says %s", where, seeNote)
		case k == j+1 && (seg[j].value != "" || seg[j].seeNote):
			child := seg[j]
			if child.code != "" && it.code != "" && child.code != it.code {
				return fmt.Errorf("%s: a wrapped test name with two lab codes", where)
			}
			if child.code != "" {
				c = child.code
			}
			if err := r.emit(child, name+" "+child.name, path, c); err != nil {
				return err
			}
		default:
			if err := r.tree(seg[j:k], depth+1, append(slices.Clone(path), name), c); err != nil {
				return err
			}
		}
		i = k
	}
	return nil
}

// bare reports a row with nothing but a name: no value, range, unit, code or
// comment.
func (it *qitem) bare() bool {
	return it.value == "" && !it.seeNote && !it.cells && it.code == "" && len(it.comments) == 0
}

func (r *questReader) emit(it *qitem, name string, path []string, code string) error {
	if len(r.rep.Results) >= maxReportResults {
		return errors.New("report exceeds result limit")
	}
	r.rep.Results = append(r.rep.Results, &Result{
		Name: name, Panel: strings.Join(path, " / "), LabCode: code,
		Value: it.value, Flag: it.flag, Units: it.unit, Ref: it.ref, Comment: it.comments,
		Calculated: it.calc, seeNote: it.seeNote, group: path, where: r.where(it.page, it.y),
	})
	return nil
}

var reSeeEndnote = regexp.MustCompile(`See Endnote (\d+)\b`)

// check joins the endnotes to the tests that cite them, reads a range
// printed on the line under a test, and refuses what cannot be read for
// sure. The same test in two groups with two readings is two readings of
// one type; the same reading twice is one.
func (r *questReader) check() error {
	rep := r.rep
	cited := map[string]bool{}
	for _, res := range rep.Results {
		var notes []string
		seen := map[string]bool{}
		for _, c := range res.Comment {
			for _, m := range reSeeEndnote.FindAllStringSubmatch(c, -1) {
				text, ok := r.endnotes[m[1]]
				if !ok {
					return fmt.Errorf("%s: a comment cites endnote %q, which the report does not print", res.where, m[1])
				}
				if !seen[m[1]] {
					notes = append(notes, text...)
				}
				seen[m[1]], cited[m[1]] = true, true
			}
		}
		res.Comment = append(slices.Clone(res.Comment), notes...)
	}
	for n := 1; n <= 99; n++ {
		if text, ok := r.endnotes[fmt.Sprint(n)]; ok && !cited[fmt.Sprint(n)] {
			rep.Notes = append(rep.Notes, text...)
		}
	}
	byType := map[string][]*Result{}
	kept := rep.Results[:0]
next:
	for _, res := range rep.Results {
		if res.Ref == "" && len(res.Comment) > 0 {
			if m := reQuestRefLine.FindStringSubmatch(res.Comment[0]); m != nil && reQuestRangeWhole.MatchString(m[1]) {
				res.Ref = m[1]
			}
		}
		switch {
		case res.seeNote && len(res.Comment) == 0:
			return fmt.Errorf("%s: result %q says %s, and no note follows it", res.where, res.Name, seeNote)
		case res.LabCode != "" && rep.Labs[res.LabCode] == "":
			return fmt.Errorf("%s: result %q has lab code %q, and the list of performing sites does not name it", res.where, res.Name, res.LabCode)
		case "lab."+slug(res.Name) == archiveType:
			return fmt.Errorf("result %q would have the type of the kept report, %s", res.Name, archiveType)
		}
		if err := nameHidesValue(res.Name); err != nil {
			return fmt.Errorf("%s: %v", res.where, err)
		}
		t := slug(res.Name)
		for _, prev := range byType[t] {
			switch {
			case prev.Name != res.Name:
				return fmt.Errorf("two results, %q and %q, map to one type lab.%s", prev.Name, res.Name, t)
			case sameReading(prev, res):
				continue next // the same reading printed twice, as when two panels share a test
			case prev.Panel == res.Panel:
				return fmt.Errorf("%s: result %q is printed twice in one group with two readings", res.where, res.Name)
			}
		}
		byType[t] = append(byType[t], res)
		kept = append(kept, res)
	}
	rep.Results = kept
	return nil
}

func sameReading(a, b *Result) bool {
	return a.Value == b.Value && a.Flag == b.Flag && a.Units == b.Units && a.Ref == b.Ref && a.LabCode == b.LabCode &&
		a.Calculated == b.Calculated && a.seeNote == b.seeNote && strings.Join(a.Comment, "\n") == strings.Join(b.Comment, "\n")
}

// questObserved is the collection time: to the minute in the printed zone
// when the report prints a time and a US zone this importer knows, else the
// day, with the printed time as raw_time.
func (rep *Report) questObserved() (map[string]any, string) {
	if t, ok := rep.questTime(); ok {
		start := t.Format(time.RFC3339)
		if _, ok := core.ParseTime(start); ok {
			return map[string]any{"start": start, "precision": "minute"}, ""
		}
	}
	t, err := time.Parse("01/02/2006", rep.Collected[:min(10, len(rep.Collected))])
	if err != nil {
		return nil, rep.printedTime
	}
	start := t.Format("2006-01-02") + "T00:00:00Z"
	if _, ok := core.ParseTime(start); !ok {
		return nil, rep.printedTime
	}
	return map[string]any{"start": start, "precision": "day"}, rep.printedTime
}

func (rep *Report) questTime() (time.Time, bool) {
	off, known := questZones[rep.zone]
	if !known || len(rep.Collected) <= 10 {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("01/02/2006 1504", rep.Collected, time.FixedZone(rep.zone, off))
	return t, err == nil && t.Format("01/02/2006 1504") == rep.Collected
}

// checkZone refuses a -tz that disagrees with the zone the report prints, at
// the collection time. The report's own zone is used; -tz is not needed.
func (rep *Report) checkZone(loc *time.Location) error {
	if rep.System != questSystem || loc == nil {
		return nil
	}
	t, ok := rep.questTime()
	if !ok {
		return nil
	}
	_, want := t.Zone()
	if _, got := t.In(loc).Zone(); got != want {
		return fmt.Errorf("-tz %s is %s at the collection time, but the report prints %q (%s); leave -tz out for this report", loc, utcOffset(got), rep.zone, utcOffset(want))
	}
	return nil
}

func utcOffset(sec int) string {
	sign := "+"
	if sec < 0 {
		sign, sec = "-", -sec
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, sec/3600, sec%3600/60)
}
