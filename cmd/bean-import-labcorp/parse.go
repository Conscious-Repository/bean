package main

// The report is read by a grammar (DECISIONS 222). Every line of the
// extracted text is one known kind, or the report is refused with the line
// number. Results are read by column: each token belongs to the column whose
// band holds its start, and every character of a result line is accounted
// for. A wrong lab value in a health record is worse than no import, and the
// person keeps the file, so anything the grammar does not know is refused.
//
// Line kinds, in the order they are tried:
//
//   - blank
//   - page boilerplate, whole line: the patient's name line (name, "DOB:",
//     "Patient Report"), the copyright line, the "All Rights Reserved" line,
//     and the "Date Created and Stored ... Page n of m" line
//   - a field line: every token a known "Label: value" field, such as
//     "Specimen ID:", "Date Collected:", "Date Reported:", "Fasting:". Only
//     fields are read for the specimen, dates and fasting, and every one of
//     them must agree
//   - before the first column header: a panel heading (one token at the
//     margin), or the column header
//   - in the results: the column header again (the same columns), an end
//     heading, a panel heading (one token at the margin that ends left of the
//     result column), a result row (a name at the Test column), a "Please
//     note" row, a value line (only under a row with nothing after its name),
//     a comment line (one token, or "Note:" and one token, in the comment
//     band under a result)
//   - after "Disclaimer", "Icon Legend" or "Performing Labs": a lab list
//     line ("01: BN - Labcorp ..., ..."), one token of text at the margin,
//     an end heading, or "Patient Details" and its column-aligned lines
//
// Anything else is refused.

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Result is one row of the report.
type Result struct {
	Name, Panel, LabCode string
	Value, Flag, Units   string
	Ref, Previous        string
	Comment              []string
	// at is where Value, Flag, Units and Ref were read, by atValue...atRef.
	at [4]cell
}

// cell is a token's place in the text: line from 1, column from 0. Line 0
// means the field is empty.
type cell struct{ line, col int }

const (
	atValue = iota
	atFlag
	atUnits
	atRef
)

// Report is the parsed report.
type Report struct {
	Specimen  string
	Collected string // "MM/DD/YYYY" or "MM/DD/YYYY HHMM"
	Reported  string // "MM/DD/YYYY"
	Fasting   string
	Labs      map[string]string // "01" -> "Labcorp San Diego"
	LabsTorn  bool              // a Performing Labs line that reLab cannot read
	Notes     []string          // report-level notes ("Please note" sections)
	Results   []*Result
}

// A line longer than this is not a layout line.
const maxLineBytes = 16 << 10

// splitLines splits text as the parser reads it. pdftotext puts a form feed
// at the start of the first line of each page: that one has no width, and
// goes. Trailing spaces go too.
func splitLines(text string) []string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, l := range lines {
		l = strings.TrimPrefix(l, "\f")
		if i == len(lines)-1 && l == "\f" {
			l = ""
		}
		lines[i] = strings.TrimRight(l, " ")
	}
	return lines
}

type token struct {
	start int // column, in characters
	text  string
}

// tokens splits a layout line at runs of two or more spaces.
func tokens(line string) []token {
	var out []token
	col := 0
	for i := 0; i < len(line); {
		if line[i] == ' ' {
			i++
			col++
			continue
		}
		start, startCol := i, col
		for i < len(line) && !(line[i] == ' ' && (i+1 >= len(line) || line[i+1] == ' ')) {
			_, size := utf8.DecodeRuneInString(line[i:])
			i += size
			col++
		}
		out = append(out, token{startCol, line[start:i]})
	}
	return out
}

// rebuild lays tokens out at their columns with spaces between. A line is
// read whole when rebuilding the tokens read from it gives the line back.
func rebuild(toks []token) string {
	var b strings.Builder
	col := 0
	for _, t := range toks {
		if t.start < col {
			return "\x00overlap"
		}
		b.WriteString(strings.Repeat(" ", t.start-col))
		b.WriteString(t.text)
		col = t.start + utf8.RuneCountInString(t.text)
	}
	return b.String()
}

// printable reports whether s holds only printable ASCII, 0x20 to 0x7E.
func printable(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

// safeText reports whether s, which is not read for results, holds no
// control character, no space but ' ' and no invisible format character:
// any of those can hide where a column starts.
func safeText(s string) bool {
	for _, r := range s {
		if r == utf8.RuneError || r < 0x20 || r == 0x7f || r >= 0x80 && r <= 0x9f ||
			r != ' ' && unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return s != ""
}

// Column header.

var headerNames = [5]string{"Test", "Current Result and Flag", "Previous Result and Date", "Units", "Reference Interval"}

// columns are the start columns of the header's five names.
type columns [5]int

// slack is how far left of its header name a column's band starts.
const slack = 2

// parseHeader reads the five column names, each one token, in order.
func parseHeader(toks []token) (columns, bool) {
	var c columns
	if len(toks) != len(headerNames) {
		return c, false
	}
	for i, t := range toks {
		if t.text != headerNames[i] {
			return c, false
		}
		c[i] = t.start
	}
	// A row's name starts at Test; panel headings start at the margin.
	return c, c[0] >= 2
}

// band says which column a token starting at col is in: 0 Test, 1 Current
// Result and Flag, 2 Previous Result and Date, 3 Units, 4 Reference
// Interval. Each band starts slack columns left of its header name.
func (c columns) band(col int) int {
	for b := 4; b >= 1; b-- {
		if col >= c[b]-slack {
			return b
		}
	}
	return 0
}

// nameAt reports whether col is where a test name starts.
func (c columns) nameAt(col int) bool { return col >= c[0]-1 && col <= c[0]+1 }

// commentAt reports whether col is in the comment band: right of where a
// name starts, left of the result column.
func (c columns) commentAt(col int) bool { return col >= c[0]+2 && col < c[1]-slack }

// Values.

var (
	reNumberValue  = regexp.MustCompile(`^(?:<=|>=|<|>)?-?\d+(?:\.\d+)?$`)
	reValuePattern = regexp.MustCompile(`^(?:[1-4]\+|\d+-\d+|[<>]?1:\d+)$`)
	reDate         = regexp.MustCompile(`^\d\d/\d\d/\d{4}$`)
	reUnits        = regexp.MustCompile(`^[!-~]+$`)
)

// resultTexts are the categorical results the result column may hold, in
// any letter case. The list is short on purpose: any other text there is
// refused, and the list can grow from real reports.
var resultTexts = map[string]bool{
	"negative": true, "positive": true, "reactive": true, "non reactive": true, "nonreactive": true, "non-reactive": true,
	"detected": true, "not detected": true, "normal": true, "abnormal": true, "borderline": true, "equivocal": true,
	"indeterminate": true, "inconclusive": true, "pending": true, "immune": true,
	"trace": true, "clear": true, "cloudy": true, "hazy": true, "turbid": true,
	"present": true, "absent": true, "none": true, "tnp": true, "qns": true,
}

// isValue reports whether s is a current result: a number, with an optional
// <, >, <= or >=; a dipstick grade 1+ to 4+; a count range such as 0-5; a
// titer such as 1:40; or one of resultTexts.
func isValue(s string) bool {
	return reNumberValue.MatchString(s) || reValuePattern.MatchString(s) || resultTexts[strings.ToLower(s)]
}

var flagWords = map[string]bool{"high": true, "low": true, "abnormal": true, "critical": true, "panic": true, "alert": true}

// isFlag reports whether s is a flag. A one-letter flag counts in capitals
// only.
func isFlag(s string) bool {
	return s == "H" || s == "L" || s == "A" || len(s) > 1 && flagWords[strings.ToLower(s)]
}

// isPrevious reports whether s is a previous result: a value, an optional
// flag, and the date.
func isPrevious(s string) bool {
	i := strings.LastIndexByte(s, ' ')
	if i < 0 || !reDate.MatchString(s[i+1:]) {
		return false
	}
	v := s[:i]
	if isValue(v) {
		return true
	}
	j := strings.LastIndexByte(v, ' ')
	return j >= 0 && isFlag(v[j+1:]) && isValue(v[:j])
}

// cells are the result columns of a row or a value line.
type cells struct {
	value, flag, units, ref string
	prev                    []string
	at                      [4]int // columns by atValue...atRef; -1 when empty
}

// cells assigns each token to the band its start is in. The result column
// holds a value and an optional flag, the units and reference columns one
// token each, the previous column a value, an optional flag and a date.
func (c columns) cells(toks []token) (cells, error) {
	out := cells{at: [4]int{-1, -1, -1, -1}}
	var cur []token
	for _, t := range toks {
		switch c.band(t.start) {
		case 0:
			return out, fmt.Errorf("%q sits in the test column, where only a name and a lab code go", t.text)
		case 1:
			cur = append(cur, t)
		case 2:
			out.prev = append(out.prev, t.text)
		case 3:
			if out.at[atUnits] >= 0 {
				return out, errors.New("two tokens in the units column")
			}
			out.units, out.at[atUnits] = t.text, t.start
		case 4:
			if out.at[atRef] >= 0 {
				return out, errors.New("two tokens in the reference interval column")
			}
			out.ref, out.at[atRef] = t.text, t.start
		}
	}
	switch {
	case len(cur) > 2:
		return out, errors.New("more than a value and a flag in the result column")
	case len(cur) > 0 && !isValue(cur[0].text):
		return out, fmt.Errorf("%q in the result column is not a number or a result this importer knows", cur[0].text)
	case len(cur) > 1 && !isFlag(cur[1].text):
		return out, fmt.Errorf("%q after the result is not a flag this importer knows", cur[1].text)
	case len(out.prev) > 0 && !isPrevious(strings.Join(out.prev, " ")):
		return out, errors.New("the previous result column does not hold a result and its date")
	case out.units != "" && !reUnits.MatchString(out.units):
		return out, fmt.Errorf("%q in the units column is not one word", out.units)
	}
	if len(cur) > 0 {
		out.value, out.at[atValue] = cur[0].text, cur[0].start
	}
	if len(cur) > 1 {
		out.flag, out.at[atFlag] = cur[1].text, cur[1].start
	}
	return out, nil
}

// fill puts the cells into r, read from line n.
func (r *Result) fill(c cells, n int) {
	r.Value, r.Flag, r.Units, r.Ref = c.value, c.flag, c.units, c.ref
	r.Previous = strings.Join(c.prev, " ")
	for i, col := range c.at {
		if col >= 0 {
			r.at[i] = cell{n, col}
		}
	}
}

// Fields: "Label: value", one token each.

type fieldSpec struct {
	label string
	value *regexp.Regexp // nil: any text a name may hold (safeText)
}

var fieldSpecs = []fieldSpec{
	{"Patient ID", regexp.MustCompile(`^[!-~]+$`)},
	{"Age", regexp.MustCompile(`^\d{1,3}$`)},
	{"Account Number", regexp.MustCompile(`^[!-~]+$`)},
	{"Specimen ID", regexp.MustCompile(`^[!-~]+$`)},
	{"Sex", regexp.MustCompile(`^[A-Za-z]+$`)},
	{"Ordering Physician", nil},
	{"Date Collected", regexp.MustCompile(`^(\d\d/\d\d/\d{4})(?: (\d{4}) Local)?$`)},
	{"Date Received", regexp.MustCompile(`^\d\d/\d\d/\d{4}(?: \d{4} Local)?$`)},
	{"Date Reported", regexp.MustCompile(`^(\d\d/\d\d/\d{4})(?: \d{4} [A-Z]{2,4})?$`)},
	{"Fasting", regexp.MustCompile(`^(?:Yes|No)$`)},
	{"Ordered Items", regexp.MustCompile(`^[ -~]+$`)},
}

// fieldLabel returns the field whose label and colon start s, or nil.
func fieldLabel(s string) *fieldSpec {
	for i := range fieldSpecs {
		if strings.HasPrefix(s, fieldSpecs[i].label+":") {
			return &fieldSpecs[i]
		}
	}
	return nil
}

// parseField reads s as a field. It returns the field and the value's
// submatches, or nil when s is not a well-formed field.
func parseField(s string) (*fieldSpec, []string) {
	f := fieldLabel(s)
	if f == nil {
		return nil, nil
	}
	v, ok := strings.CutPrefix(s[len(f.label)+1:], " ")
	if !ok {
		return nil, nil
	}
	if f.value == nil {
		if !safeText(v) {
			return nil, nil
		}
		return f, []string{v}
	}
	m := f.value.FindStringSubmatch(v)
	if m == nil {
		return nil, nil
	}
	return f, m
}

// Page boilerplate.

var (
	reDOB        = regexp.MustCompile(`^DOB: \d\d/\d\d/\d{4}$`)
	reCopyright  = regexp.MustCompile(`^©\d{4} Laboratory Corporation of America® Holdings$`)
	reRightsCall = regexp.MustCompile(`^If you have received this document in error please call \d{3}-\d{3}-\d{4}$`)
	reCreated    = regexp.MustCompile(`^Date Created and Stored \d\d/\d\d/\d\d \d{4} [A-Z]{2,4} Final Report Page \d{1,4} of \d{1,4}$`)
)

const (
	confidential = "This document contains private and confidential health information."
	rightsLine   = "All Rights Reserved - Enterprise Report Version 2.00"
)

// boilerplate reports whether a line is one of the page header and footer
// lines, whole.
func boilerplate(toks []token) bool {
	first := toks[0]
	switch {
	case len(toks) == 3 && first.start == 0 && safeText(first.text) && reDOB.MatchString(toks[1].text) && toks[2].text == "Patient Report":
		return true // the patient's name; it is not read
	case first.start == 0 && reCopyright.MatchString(first.text):
		return len(toks) == 1 || len(toks) == 2 && toks[1].text == confidential
	case first.start == 0 && first.text == rightsLine:
		return len(toks) == 1 || len(toks) == 2 && reRightsCall.MatchString(toks[1].text)
	case len(toks) == 1 && first.start > 0 && reCreated.MatchString(first.text):
		return true
	}
	return false
}

// End of the results.

var endHeadings = map[string]bool{"Disclaimer": true, "Icon Legend": true, "Performing Labs": true}

var (
	reLab     = regexp.MustCompile(`^(\d\d): [A-Z0-9]{2,3} - ([^,]+), [ -~]+$`)
	reLabLike = regexp.MustCompile(`^\d\d: `)
)

var detailsNames = []string{"Patient Details", "Physician Details", "Specimen Details"}

// detailsHeading reads the "Patient Details" heading: some of the three
// names, in order, the first at the margin. It returns their columns.
func detailsHeading(toks []token) ([]token, bool) {
	if toks[0].start != 0 || toks[0].text != detailsNames[0] {
		return nil, false
	}
	next := 1
	for _, t := range toks[1:] {
		for next < len(detailsNames) && detailsNames[next] != t.text {
			next++
		}
		if next == len(detailsNames) {
			return nil, false
		}
		next++
	}
	return toks, true
}

// Parser.

const (
	beforeHeader = iota
	inResults
	afterEnd
	inDetails
)

type parser struct {
	rep     *Report
	region  int
	cols    columns
	colsAt  int    // line of the first column header
	ended   string // the end heading of the current section
	details []token
	panel   string
	cur     *Result  // the result a comment belongs to
	note    *string  // the panel of a "Please note" row whose text comes next
	pending *rowRead // a row with nothing after its name, waiting for its value line
	rows    []*rowRead
}

// rowRead is a result row as read, before lab codes are known.
type rowRead struct {
	r     *Result
	code  string // a two-digit token in the code column
	cells bool   // the row line held result columns after the name
	line  int
}

func parseReport(text string) (*Report, error) {
	if strings.Count(text, "\n") >= maxReportLines {
		return nil, errors.New("report exceeds line limit")
	}
	p := &parser{rep: &Report{Labs: map[string]string{}}}
	for i, l := range splitLines(text) {
		if err := p.line(i+1, l); err != nil {
			return nil, err
		}
	}
	return p.finish()
}

func (p *parser) line(n int, l string) error {
	if l == "" {
		return nil
	}
	if len(l) > maxLineBytes {
		return fmt.Errorf("line %d is longer than %d bytes", n, maxLineBytes)
	}
	toks := tokens(l)
	if boilerplate(toks) {
		return nil
	}
	if ok, err := p.fieldLine(toks); ok || err != nil {
		return err
	}
	// Columns are read from spaces, one character each. A tab, a Unicode
	// space, an invisible mark or a letter of two bytes can hide where a
	// column starts or shift it, so a line that is read is printable ASCII.
	// The patient details hold names, and are checked by column.
	if p.region != inDetails && !printable(l) {
		return fmt.Errorf("line %d holds a character outside printable ASCII, so its columns cannot be read for sure", n)
	}
	switch p.region {
	case beforeHeader:
		return p.beforeHeader(n, l, toks)
	case inResults:
		return p.results(n, l, toks)
	}
	return p.afterEnd(n, l, toks)
}

// fieldLine reads a line whose every token is a field. Every specimen id,
// collection date, report date and fasting field must agree.
func (p *parser) fieldLine(toks []token) (bool, error) {
	type read struct {
		f *fieldSpec
		m []string
	}
	var fields []read
	for _, t := range toks {
		f, m := parseField(t.text)
		if f == nil {
			return false, nil
		}
		fields = append(fields, read{f, m})
	}
	for _, f := range fields {
		if err := p.readField(f.f.label, f.m); err != nil {
			return true, err
		}
	}
	return true, nil
}

func (p *parser) readField(label string, m []string) error {
	rep := p.rep
	switch label {
	case "Specimen ID":
		// The native ID joins specimen and test with '/'. Test names may
		// contain that character; specimen IDs must not.
		if strings.Contains(m[0], "/") {
			return errors.New("ambiguous specimen identifier")
		}
		if rep.Specimen != "" && rep.Specimen != m[0] {
			return errors.New("conflicting specimen identifiers")
		}
		rep.Specimen = m[0]
	case "Date Collected":
		collected := strings.TrimSpace(m[1] + " " + m[2])
		if rep.Collected != "" && (rep.Collected[:10] != m[1] ||
			(len(rep.Collected) > 10 && m[2] != "" && rep.Collected != collected)) {
			return errors.New("conflicting collection dates or times")
		}
		if rep.Collected == "" || m[2] != "" {
			rep.Collected = collected // the specimen details line has the time
		}
	case "Date Reported":
		if rep.Reported != "" && rep.Reported != m[1] {
			return errors.New("conflicting report dates")
		}
		rep.Reported = m[1]
	case "Fasting":
		if rep.Fasting != "" && rep.Fasting != m[0] {
			return errors.New("conflicting fasting fields")
		}
		rep.Fasting = m[0]
	}
	return nil
}

// margin reads one token at the margin that is not a field or a header:
// it reports whether the line is one, and refuses a malformed field.
func margin(n int, toks []token) (bool, error) {
	if len(toks) != 1 || toks[0].start != 0 {
		return false, nil
	}
	if f := fieldLabel(toks[0].text); f != nil {
		return false, fmt.Errorf("line %d: malformed %q field", n, f.label)
	}
	return true, nil
}

func (p *parser) beforeHeader(n int, l string, toks []token) error {
	if toks[0].text == "Test" && len(toks) > 1 {
		return p.header(n, l, toks)
	}
	ok, err := margin(n, toks)
	switch {
	case err != nil:
		return err
	case ok && endHeadings[toks[0].text]:
		p.region, p.ended = afterEnd, toks[0].text
	case ok:
		p.panel = toks[0].text // the first panel heading comes before the first header
	case toks[0].start > 0:
		return fmt.Errorf("line %d is indented before the column header: a result row there comes before the column header, with no columns to be read by", n)
	default:
		return fmt.Errorf("line %d before the column header is not a page header line, a field line or a panel heading", n)
	}
	return nil
}

func (p *parser) header(n int, l string, toks []token) error {
	c, ok := parseHeader(toks)
	if !ok {
		return fmt.Errorf("line %d: unsupported result column header: it must name %s, in that order", n, strings.Join(headerNames[:], ", "))
	}
	if p.region == beforeHeader {
		p.cols, p.colsAt, p.region = c, n, inResults
	} else if c != p.cols {
		return fmt.Errorf("line %d: a column header with other columns than the one at line %d; every header must give the same columns", n, p.colsAt)
	}
	return p.whole(n, l, toks)
}

// whole checks that the tokens read from line n give the line back.
func (p *parser) whole(n int, l string, used []token) error {
	if rebuild(used) != l {
		return fmt.Errorf("line %d was not read whole", n)
	}
	return nil
}

func (p *parser) results(n int, l string, toks []token) error {
	c := p.cols
	first := toks[0]
	switch {
	case first.text == "Test" && len(toks) > 1:
		return p.header(n, l, toks)
	case first.start == 0:
		ok, err := margin(n, toks)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("line %d starts at the margin but is not a panel heading: a panel heading is one column of text", n)
		}
		if endHeadings[first.text] {
			p.region, p.ended, p.cur, p.note, p.pending = afterEnd, first.text, nil, nil, nil
			return nil
		}
		if utf8.RuneCountInString(first.text) > c[1]-slack {
			return fmt.Errorf("line %d: a panel heading runs into the result column", n)
		}
		if panel := strings.TrimSuffix(first.text, " (Cont.)"); panel != p.panel {
			p.panel, p.cur, p.note, p.pending = panel, nil, nil, nil
		}
		return p.whole(n, l, toks)
	case c.nameAt(first.start):
		return p.row(n, l, toks)
	case c.commentAt(first.start):
		return p.comment(n, l, toks)
	case c.band(first.start) > 0:
		if p.pending == nil {
			return fmt.Errorf("line %d holds results, but the line above it is not a row waiting for them", n)
		}
		v, err := c.cells(toks)
		if err != nil {
			return fmt.Errorf("line %d: %v", n, err)
		}
		p.pending.r.fill(v, n)
		p.pending = nil
		return p.whole(n, l, toks)
	}
	return fmt.Errorf("line %d starts left of the test column", n)
}

func (p *parser) row(n int, l string, toks []token) error {
	c := p.cols
	name := toks[0]
	// A name that runs into the result column may hold the value
	// ("... Overrun 90", "... HCV Ab Non Reactive").
	if name.start+utf8.RuneCountInString(name.text) > c[1]-4 {
		return fmt.Errorf("line %d: a test name runs into the result column, so its value cannot be read for sure", n)
	}
	rest, code := toks[1:], ""
	if len(rest) > 0 && c.band(rest[0].start) == 0 {
		if !twoDigits(rest[0].text) {
			return fmt.Errorf("line %d: %q in the test column is not a lab code", n, rest[0].text)
		}
		code, rest = rest[0].text, rest[1:]
	}
	p.pending = nil
	if strings.EqualFold(strings.TrimSuffix(name.text, ":"), "please note") {
		if len(rest) > 0 {
			return fmt.Errorf("line %d: a Please note line with text in the result columns", n)
		}
		panel := p.panel
		p.note = &panel
		return p.whole(n, l, toks)
	}
	p.note = nil
	v, err := c.cells(rest)
	if err != nil {
		return fmt.Errorf("line %d: %v", n, err)
	}
	if len(p.rep.Results) >= maxReportResults {
		return errors.New("report exceeds result limit")
	}
	r := &Result{Name: name.text, Panel: p.panel}
	r.fill(v, n)
	p.rep.Results = append(p.rep.Results, r)
	rw := &rowRead{r: r, code: code, cells: len(rest) > 0, line: n}
	p.rows = append(p.rows, rw)
	p.cur = r
	if len(rest) == 0 {
		p.pending = rw
	}
	return p.whole(n, l, toks)
}

func twoDigits(s string) bool {
	return len(s) == 2 && s[0] >= '0' && s[0] <= '9' && s[1] >= '0' && s[1] <= '9'
}

// comment reads a comment line: one token of text, or "Note:" and one more
// (the fixture's "Note:     Values greater than ..."). More columns may be
// a row or a value, and are refused.
func (p *parser) comment(n int, l string, toks []token) error {
	if len(toks) > 2 || len(toks) == 2 && toks[0].text != "Note:" {
		return fmt.Errorf("line %d in the comment column is not one column of text: a row or a value there would be lost", n)
	}
	p.pending = nil
	parts := make([]string, len(toks))
	for i, t := range toks {
		parts[i] = t.text
	}
	s := strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
	switch {
	case p.note != nil && (p.cur == nil || p.cur.Panel != *p.note):
		p.rep.Notes = append(p.rep.Notes, s)
	case p.cur != nil:
		p.cur.Comment = append(p.cur.Comment, s)
	default:
		return fmt.Errorf("line %d is a comment with no result above it", n)
	}
	return p.whole(n, l, toks)
}

// afterEnd reads the lines after the results: nothing there is a result, so
// only what is known to be something else passes.
func (p *parser) afterEnd(n int, l string, toks []token) error {
	first := toks[0]
	switch {
	case first.text == "Test" && len(toks) > 1:
		return fmt.Errorf("line %d: a result column header comes after %q, so results there would be lost", n, p.ended)
	case len(toks) == 1 && first.start == 0 && endHeadings[first.text]:
		p.region, p.ended = afterEnd, first.text
		return nil
	case first.start == 0 && reLabLike.MatchString(l):
		if !printable(l) {
			return fmt.Errorf("line %d holds a character outside printable ASCII, so its columns cannot be read for sure", n)
		}
		m := reLab.FindStringSubmatch(l)
		if m == nil {
			p.rep.LabsTorn = true // a lab line it cannot read: lab codes come from the code column only
			return nil
		}
		name := strings.TrimSpace(m[2])
		if old := p.rep.Labs[m[1]]; old != "" && old != name {
			return fmt.Errorf("line %d: two performing labs with code %s", n, m[1])
		}
		p.rep.Labs[m[1]] = name
		return nil
	}
	if cols, ok := detailsHeading(toks); ok {
		p.region, p.ended, p.details = inDetails, first.text, cols
		return nil
	}
	if p.region == inDetails {
		return p.detailsLine(n, toks)
	}
	ok, err := margin(n, toks)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("line %d after %q is not a lab list, disclaimer or legend line (one column of text at the margin), so a result there would be lost", n, p.ended)
	}
	return nil
}

// detailsLine reads a line of the patient details: each token starts at
// one of the heading's columns, give or take two, one token per column.
// A field is read; the patient's and physician's columns may hold any
// text a name can; the specimen column holds fields only.
func (p *parser) detailsLine(n int, toks []token) error {
	used := make([]bool, len(p.details))
	for _, t := range toks {
		k := -1
		for i, h := range p.details {
			if t.start >= h.start-2 && t.start <= h.start+2 {
				k = i
			}
		}
		if k < 0 || used[k] {
			return fmt.Errorf("line %d in the patient details does not line up with its columns, so a result there would be lost", n)
		}
		used[k] = true
		if label := fieldLabel(t.text); label != nil {
			f, m := parseField(t.text)
			if f == nil {
				return fmt.Errorf("line %d: malformed %q field", n, label.label)
			}
			if err := p.readField(f.label, m); err != nil {
				return err
			}
			continue
		}
		if p.details[k].text == "Specimen Details" {
			return fmt.Errorf("line %d: the specimen details hold a field this importer does not know", n)
		}
		if !safeText(t.text) {
			return fmt.Errorf("line %d holds a control, space or invisible character outside printable ASCII, so its columns cannot be read for sure", n)
		}
	}
	return nil
}

var reCode = regexp.MustCompile(` (\d\d)$`)

// finish resolves lab codes, now that the list of performing labs is
// known, and refuses what cannot be read for sure.
func (p *parser) finish() (*Report, error) {
	rep := p.rep
	if rep.Specimen == "" || rep.Collected == "" {
		return nil, fmt.Errorf("not a Labcorp patient report: no specimen id or collection date")
	}
	if len(rep.Results) == 0 {
		return nil, fmt.Errorf("no results found")
	}
	// With a list of performing labs that reads whole, a lab code is one the
	// list names. Without one, only a token in the code column is a code, and
	// only when result columns follow it on the row: with nothing after it,
	// it may be the value of a compressed row ("eGFR  90"). Two digits at the
	// end of a name stay in the name, unless the list names them, so
	// "Interleukin 10" keeps its number.
	whole := len(rep.Labs) > 0 && !rep.LabsTorn
	for _, rw := range p.rows {
		r := rw.r
		switch {
		case rw.code != "" && whole && rep.Labs[rw.code] == "":
			return nil, fmt.Errorf("line %d: result %q has %q in the lab code column, and the list of performing labs does not name it", rw.line, r.Name, rw.code)
		case rw.code != "" && !whole && !rw.cells:
			return nil, fmt.Errorf("result %q has %q in the lab code column and no value; without a list of performing labs it may be the value", r.Name, rw.code)
		case rw.code != "" && whole && reCode.MatchString(r.Name) && rep.Labs[reCode.FindStringSubmatch(r.Name)[1]] != "":
			return nil, fmt.Errorf("line %d: result %q has a lab code at the end of its name and another in the code column", rw.line, r.Name)
		case rw.code != "":
			r.LabCode = rw.code
		case whole:
			if m := reCode.FindStringSubmatch(r.Name); m != nil && rep.Labs[m[1]] != "" {
				r.Name, r.LabCode = strings.TrimSuffix(r.Name, m[0]), m[1]
			}
		}
	}
	// The same test listed twice with the same reading, as when two panels
	// share it, is one reading. Two different readings for one type are not.
	types := map[string]*Result{}
	kept := rep.Results[:0]
	for _, r := range rep.Results {
		t := slug(r.Name)
		if prev, ok := types[t]; ok {
			// Everything the reading keeps must match, or the second is lost.
			if prev.Name == r.Name && prev.Value == r.Value && prev.Flag == r.Flag && prev.Units == r.Units && prev.Ref == r.Ref &&
				prev.Previous == r.Previous && prev.LabCode == r.LabCode && strings.Join(prev.Comment, "\n") == strings.Join(r.Comment, "\n") {
				continue
			}
			return nil, fmt.Errorf("two results, %q and %q, map to one type lab.%s", prev.Name, r.Name, t)
		}
		types[t] = r
		kept = append(kept, r)
		// A row with no value is taken only when the lab says why, in the
		// first line under it, and no word of its name after the first may be
		// a value: the value may sit in the name, one space after it.
		if r.Value == "" {
			if len(r.Comment) == 0 || !notPerformed(r.Comment[0]) {
				return nil, fmt.Errorf("result %q has no value, and no line under it says the test was not performed", r.Name)
			}
			if nameMayHoldValue(r.Name) {
				return nil, fmt.Errorf("result %q has no value, and a word of its name may be the value", r.Name)
			}
		}
	}
	rep.Results = kept
	return rep, nil
}

var reNameWord = regexp.MustCompile(`^[A-Za-z][A-Za-z-]*$`)

// nameMayHoldValue reports whether a word of a name, after the first, is
// anything but letters and hyphens, or is a flag or a result.
func nameMayHoldValue(name string) bool {
	f := strings.Fields(name)
	for _, w := range f[min(1, len(f)):] {
		if !reNameWord.MatchString(w) || isFlag(w) || isValue(w) {
			return true
		}
	}
	return false
}

// notPerformedPhrases are how a lab says it has no value for a test. The
// list is short on purpose: a row with no value and any other comment is
// refused.
var notPerformedPhrases = []string{
	"test not performed", "not performed",
	"test cancelled", "test canceled", "cancelled", "canceled",
}

// notPerformed reports whether a comment line starts with one of
// notPerformedPhrases, as a whole phrase.
func notPerformed(comment string) bool {
	c := strings.ToLower(comment)
	for _, p := range notPerformedPhrases {
		if rest, ok := strings.CutPrefix(c, p); ok && (rest == "" || strings.ContainsRune(" .,;:", rune(rest[0]))) {
			return true
		}
	}
	return false
}
