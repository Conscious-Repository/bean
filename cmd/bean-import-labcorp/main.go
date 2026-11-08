// Command bean-import-labcorp turns a Labcorp patient report ("Enterprise
// Report Version 2.00", the PDF that Labcorp, Superpower and other
// services hand to patients) into JSONL partial records (SPEC.md 9). It
// never signs and never holds the dedupe key.
//
// Input is the PDF, which it reads with `pdftotext -layout` (poppler), or
// a text file that pdftotext already made. One record per result row:
//
//   - type: "lab." plus a slug of the Labcorp test name ("LDL Chol Calc
//     (NIH)" is lab.ldl-chol-calc-nih). No LOINC mapping yet; the report
//     does not print codes and this adapter does not guess them.
//   - value: {"n", "unit"} for a number, {"s", "unit"} for anything else
//     ("<0.5", "Negative"), {"n": null} when the lab did not report one.
//   - native id: the specimen id and the test name. The specimen id never
//     leaves the adapter except inside the HMAC input.
//   - context: test, panel, flag, reference_interval, performing_lab,
//     lab_comment, report_note, reported, fasting, and with -blob the
//     report_sha256 of the kept file. A reading does not list the file in
//     blobs, so purging the lab.report erases the file (C25).
//   - times.observed: "Date Collected" from the specimen details. The report
//     says "Local" without a zone, so the day is all that is known unless
//     -tz names the zone; then the minute is used.
//
// Recognized patient header fields are excluded. Comments and other free text
// may contain names or identifiers; medical text is preserved without redaction.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Conscious-Repository/bean/core"
)

const (
	version = "bean-import-labcorp@0.2.0"
	system  = "labcorp"
)

func main() {
	fs := flag.NewFlagSet("bean-import-labcorp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	blob := fs.String("blob", "", "blob id of the PDF, from bean blob put")
	tz := fs.String("tz", "", "IANA zone of the collection site, for example America/Chicago")
	if err := fs.Parse(os.Args[1:]); err != nil || fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: bean-import-labcorp [-blob <blob id>] [-tz <zone>] <report.pdf | report.txt>")
		os.Exit(2)
	}
	if *blob == "" {
		fmt.Fprintln(os.Stderr, "bean-import-labcorp: warning: no -blob: the report is not kept; run bean blob put <report.pdf> first and pass -blob <id>")
	}
	text, raw, err := readSource(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "bean-import-labcorp: input or report validation failed")
		os.Exit(1)
	}
	var loc *time.Location
	if *tz != "" {
		if loc, err = time.LoadLocation(*tz); err != nil {
			fmt.Fprintf(os.Stderr, "bean-import-labcorp: bad -tz: %s\n", err)
			os.Exit(2)
		}
	}
	rep, err := parseReport(text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bean-import-labcorp: %s\n", err)
		os.Exit(1)
	}
	digest := ""
	if *blob != "" {
		sum := sha256.Sum256(raw)
		digest = hex.EncodeToString(sum[:])
	}
	records := rep.records(digest, loc)
	missing := 0
	for _, rec := range records {
		if v := rec["value"].(map[string]any); v["n"] == nil && v["s"] == nil {
			missing++
		}
	}
	if *blob != "" {
		if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(*blob) {
			fmt.Fprintln(os.Stderr, "bean-import-labcorp: invalid blob reference")
			os.Exit(2)
		}
		records = append([]map[string]any{archiveRecord(raw, *blob)}, records...)
	}
	if err := writeRecords(os.Stdout, records); err != nil {
		fmt.Fprintln(os.Stderr, "bean-import-labcorp: output failed; partial output possible")
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "bean-import-labcorp: %d results, %d without a value; existing specimen/test identities dedupe, amendments require explicit correction\n", len(rep.Results), missing)
}

const maxReportBytes = 32 << 20
const maxReportText = 32 << 20

// Adapter resource limits, deliberately above ordinary lab reports. Bound line
// splitting and result maps independently of source bytes and serialized output.
const maxReportLines = 100000
const maxReportResults = 10000

// Bean adds identity, signature, imported time and dedupe fields after import.
// Leave a conservative allowance within its 1 MiB ordinary-record limit.
const maxPartialRecordBytes = (1 << 20) - (4 << 10)

func writeRecords(out io.Writer, records []map[string]any) error {
	// Stage before output so local validation never creates an importable prefix.
	var staged bytes.Buffer
	for _, r := range records {
		typ, _ := r["type"].(string)
		if len(typ) > 128 || typ == "lab." {
			return errors.New("invalid lab result name")
		}
		b := append(core.Canonical(r), '\n')
		if len(b) > maxPartialRecordBytes {
			return errors.New("lab result exceeds record limit")
		}
		if staged.Len()+len(b) > maxReportText*4 {
			return errors.New("report output exceeds limit")
		}
		staged.Write(b)
	}
	_, err := io.Copy(out, &staged)
	return err
}

func readText(path string) (string, error) {
	text, _, err := readSource(path)
	return text, err
}

// readSource binds extraction to the same bytes used for the source digest.
// Temporary PDF and extracted text are bounded; diagnostics contain no source text.
func readSource(name string) (string, []byte, error) {
	info, err := os.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxReportBytes {
		return "", nil, errors.New("input must be a regular report no larger than 32 MiB")
	}
	f, err := os.Open(name)
	if err != nil {
		return "", nil, errors.New("cannot open report")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return "", nil, errors.New("report changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxReportBytes+1))
	if err != nil || len(data) > maxReportBytes {
		return "", nil, errors.New("report read failed or exceeds limit")
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		if !utf8.Valid(data) {
			return "", nil, errors.New("bad string: report is not UTF-8")
		}
		return string(data), data, nil
	}
	bin, err := exec.LookPath("pdftotext")
	if err != nil {
		return "", nil, errors.New("pdftotext not found; install poppler or supply extracted layout text")
	}
	dir, err := os.MkdirTemp("", "bean-lab-report-*")
	if err != nil {
		return "", nil, errors.New("cannot stage report")
	}
	defer os.RemoveAll(dir)
	snapshot := filepath.Join(dir, "source.pdf")
	if err = os.WriteFile(snapshot, data, 0600); err != nil {
		return "", nil, errors.New("cannot stage report")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-layout", "-enc", "UTF-8", snapshot, "-")
	// A converter descendant may inherit the pipes after the direct child exits.
	cmd.WaitDelay = time.Second
	out := &boundedText{limit: maxReportText}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	if err = cmd.Run(); err != nil {
		return "", nil, errors.New("PDF extraction failed, timed out or exceeded output limit")
	}
	if !utf8.Valid(out.Bytes()) {
		return "", nil, errors.New("bad string: pdftotext output is not UTF-8")
	}
	return out.String(), data, nil
}

type boundedText struct {
	bytes.Buffer
	limit int
}

func (b *boundedText) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, errors.New("extracted text exceeds limit")
	}
	return b.Buffer.Write(p)
}

// Result is one row of the report.
type Result struct {
	Name, Panel, LabCode string
	Value, Flag, Units   string
	Ref, Previous        string
	Comment              []string
}

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

var (
	reSpecimen    = regexp.MustCompile(`Specimen ID:\s*(\S+)`)
	reCollected   = regexp.MustCompile(`Date Collected:\s*(\d\d/\d\d/\d{4})(?:\s+(\d{4})\s+Local)?(?:\s|$)`)
	reReported    = regexp.MustCompile(`Date Reported:\s*(\d\d/\d\d/\d{4})`)
	reFasting     = regexp.MustCompile(`Fasting:\s*(\S.*?)\s*$`)
	reLab         = regexp.MustCompile(`^(\d\d):\s*\S+\s*-\s*([^,]+),`)
	reLabLike     = regexp.MustCompile(`^\d\d:\s`)
	reCode        = regexp.MustCompile(`\s+(\d\d)$`)
	reNumber      = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
	reBoilerplate = []*regexp.Regexp{
		regexp.MustCompile(`^\s*©`),
		regexp.MustCompile(`^All Rights Reserved`),
		regexp.MustCompile(`DOB:`),
		regexp.MustCompile(`^\s*Patient ID:.*Account Number:`),
		regexp.MustCompile(`^\s*Specimen ID:.*Ordering Physician`),
		regexp.MustCompile(`Date Created and Stored`),
		regexp.MustCompile(`^\s+Date Collected:\s*\S+\s*$`),
	}
)

type token struct {
	start int
	text  string
}

// tokens splits a layout line at runs of two or more spaces.
func tokens(line string) []token {
	var out []token
	i := 0
	for i < len(line) {
		for i < len(line) && line[i] == ' ' {
			i++
		}
		if i >= len(line) {
			break
		}
		j := i
		for j < len(line) && !(line[j] == ' ' && (j+1 >= len(line) || line[j+1] == ' ')) {
			j++
		}
		out = append(out, token{i, line[i:j]})
		i = j
	}
	return out
}

func indent(line string) int { return len(line) - len(strings.TrimLeft(line, " ")) }

// columns are the start positions from the "Test  Current Result..." header.
type columns struct{ current, previous, units, ref int }

func headerColumns(line string) (columns, bool) {
	c := strings.Index(line, "Current Result")
	p := strings.Index(line, "Previous Result")
	u := strings.Index(line, "Units")
	r := strings.Index(line, "Reference Interval")
	if c < 0 || p <= c || u <= p || r <= u {
		return columns{}, false
	}
	return columns{current: c, previous: p, units: u, ref: r}, true
}

// fill assigns value tokens to the result. It reports false when the
// tokens do not look like a result row (a comment line).
func (r *Result) fill(toks []token, c columns) bool {
	next := *r
	if !next.fillTokens(toks, c) {
		return false
	}
	*r = next
	return true
}

func (r *Result) fillTokens(toks []token, c columns) bool {
	for _, t := range toks {
		// Comments start left of the result column. Text length cannot distinguish
		// a comment from a legitimate long categorical result or reference range.
		if t.start < c.current-4 {
			return false
		}
	}
	var right []token
	for _, t := range toks {
		switch {
		case t.start < c.previous-4:
			if r.Value == "" {
				r.Value = t.text
			} else if r.Flag == "" {
				r.Flag = t.text
			} else {
				return false
			}
		case t.start < c.units-10:
			r.Previous = strings.TrimSpace(r.Previous + " " + t.text)
		default:
			right = append(right, t)
		}
	}
	// Use the observed header spacing: pdftotext can compress the layout.
	var units, ref []string
	for _, t := range right {
		if t.start >= (c.units+c.ref)/2 {
			ref = append(ref, t.text)
		} else {
			units = append(units, t.text)
		}
	}
	r.Units, r.Ref = strings.Join(units, " "), strings.Join(ref, " ")
	return true
}

func parseReport(text string) (*Report, error) {
	if strings.Count(text, "\n") >= maxReportLines {
		return nil, errors.New("report exceeds line limit")
	}
	rep := &Report{Labs: map[string]string{}}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	// pdftotext puts a form feed at the start of the first line of each page:
	// that one has no width, and goes. Every other character outside
	// printable ASCII is refused below (see printable).
	for i, l := range lines {
		l = strings.TrimPrefix(l, "\f")
		if i == len(lines)-1 && l == "\f" {
			l = ""
		}
		lines[i] = l
	}
	for _, l := range lines {
		specimenMatches := reSpecimen.FindAllStringSubmatch(l, -1)
		if len(specimenMatches) != strings.Count(l, "Specimen ID:") {
			return nil, errors.New("malformed specimen identifier label")
		}
		for _, m := range specimenMatches {
			// The established native ID joins specimen and test with '/'. Test
			// names may contain that character; specimen IDs must not.
			// A line the character check lets through (a page header, the
			// patient details) may hold any character. The id may not.
			if strings.Contains(m[1], "/") || !printable(m[1]) {
				return nil, errors.New("ambiguous specimen identifier")
			}
			if rep.Specimen != "" && rep.Specimen != m[1] {
				return nil, errors.New("conflicting specimen identifiers")
			}
			rep.Specimen = m[1]
		}
		collectedMatches := reCollected.FindAllStringSubmatch(l, -1)
		if len(collectedMatches) != strings.Count(l, "Date Collected:") {
			return nil, errors.New("malformed collection date label")
		}
		for _, m := range collectedMatches {
			collected := strings.TrimSpace(m[1] + " " + m[2])
			if rep.Collected != "" && (rep.Collected[:10] != m[1] ||
				(len(rep.Collected) > 10 && m[2] != "" && rep.Collected != collected)) {
				return nil, errors.New("conflicting collection dates or times")
			}
			if rep.Collected == "" || m[2] != "" {
				rep.Collected = collected // the specimen details line has the time
			}
		}
		if m := reReported.FindStringSubmatch(l); m != nil {
			rep.Reported = m[1]
		}
		if m := reFasting.FindStringSubmatch(l); m != nil && rep.Fasting == "" {
			if !printable(m[1]) {
				return nil, errors.New("a character outside printable ASCII in the fasting field")
			}
			rep.Fasting = m[1]
		}
		if m := reLab.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			if !printable(m[2]) {
				return nil, errors.New("a character outside printable ASCII in a performing lab")
			}
			rep.Labs[m[1]] = strings.TrimSpace(m[2])
		} else if reLabLike.MatchString(strings.TrimSpace(l)) {
			rep.LabsTorn = true
		}
	}
	if rep.Specimen == "" || rep.Collected == "" {
		return nil, fmt.Errorf("not a Labcorp patient report: no specimen id or collection date")
	}

	var (
		cols      columns
		inResults bool
		panel     string
		cur       *Result // the row whose value line or comment comes next
		note      *Result // a "Please note" pseudo-row collecting text
		wantValue bool
		ended     string // the heading that ends the results, if one came
	)
	addComment := func(s string) {
		s = strings.Join(strings.Fields(s), " ")
		if s == "" {
			return
		}
		switch {
		case note != nil && (cur == nil || cur.Panel != note.Panel):
			rep.Notes = append(rep.Notes, s)
		case cur != nil:
			cur.Comment = append(cur.Comment, s)
		}
	}
	var (
		details bool // in the patient details, after the end of the results
		rowLike bool // after the end, the last line with text was indented like a row
	)
	for i, l := range lines {
		l = strings.TrimRight(l, " ")
		boiler := false
		for _, re := range reBoilerplate {
			boiler = boiler || re.MatchString(l)
		}
		labLine := reLabLike.MatchString(strings.TrimSpace(l))
		if ended != "" && indent(l) == 0 && strings.HasPrefix(l, "Patient Details") {
			details = true
		}
		// Columns are read from spaces, one byte each. Any other character
		// can hide where a column starts or shift it: a tab, a Unicode space,
		// an invisible mark, a letter of two bytes. So a line that is read
		// holds printable ASCII only. Two kinds of lines are not read for
		// results and may hold more: page header lines (the © and ® of the
		// copyright line, the patient's name) and, after the end of the
		// results, the patient details. Their header fields are checked
		// above.
		if !boiler && (!details || labLine) && !printable(l) {
			return nil, fmt.Errorf("line %d holds a character outside printable ASCII, so its columns cannot be read for sure", i+1)
		}
		if ended != "" {
			// After the end of the results, nothing is read. A column header,
			// or a line indented like a row, is a result that would be lost:
			// refuse the report. Lab list and page header lines may be
			// indented. A line of one token, a disclaimer sentence, passes if
			// no word in it looks like a value, and if no line under it does.
			if strings.Contains(l, "Current Result") && strings.Contains(l, "Units") {
				return nil, fmt.Errorf("a result column header comes after %q, so results there would be lost", ended)
			}
			if strings.TrimSpace(l) == "" || boiler || labLine {
				continue
			}
			n := indent(l)
			if n > 0 && n < 20 {
				if toks := tokens(l); len(toks) > 1 || hidesValue(toks[0].text) {
					return nil, fmt.Errorf("a line like a result row comes after %q, so it would be lost", ended)
				}
			} else if n >= 20 && rowLike {
				return nil, fmt.Errorf("a line like a result value comes after %q, so it would be lost", ended)
			}
			rowLike = n > 0 && n < 20
			continue
		}
		if c, ok := headerColumns(l); ok {
			cols, inResults = c, true
			continue
		}
		if strings.Contains(l, "Current Result") && strings.Contains(l, "Previous Result") &&
			strings.Contains(l, "Units") && strings.Contains(l, "Reference Interval") {
			return nil, errors.New("unsupported result column order")
		}
		if boiler {
			continue
		}
		if !inResults {
			if strings.TrimSpace(l) != "" && indent(l) == 0 {
				panel = l // the first panel heading comes before the first header
			}
			// A row needs the header's columns. Without them it would be lost.
			if n := indent(l); strings.TrimSpace(l) != "" && n > 0 && n < 20 {
				return nil, errors.New("a result row comes before the column header")
			}
			continue
		}
		if strings.TrimSpace(l) == "" {
			continue
		}
		if indent(l) == 0 {
			if strings.HasPrefix(l, "Disclaimer") || strings.HasPrefix(l, "Icon Legend") || strings.HasPrefix(l, "Performing Labs") {
				ended = l
				continue
			}
			p := strings.TrimSuffix(l, " (Cont.)")
			if p != panel {
				note = nil
				wantValue = false
			}
			panel = p
			continue
		}
		toks := tokens(l)
		if wantValue && indent(l) >= 20 {
			if cur.fill(toks, cols) {
				wantValue = false
				continue
			}
			if len(toks) > 0 && toks[0].start >= cols.current-4 {
				return nil, errors.New("ambiguous wrapped result columns")
			}
		}
		if indent(l) < 20 {
			// A row: name, optional lab code, then the value or nothing.
			// A name that runs into the result column may hold the value
			// ("... Overrun 90", "... HCV Ab Non Reactive"): refuse it, with
			// or without a comment under it.
			if toks[0].start+len(toks[0].text) > cols.current-4 {
				return nil, errors.New("a test name runs into the result column, so its value cannot be read for sure")
			}
			name, code := toks[0].text, ""
			if m := reCode.FindStringSubmatch(name); m != nil && rep.labCode(m[1], false) {
				name, code = strings.TrimSpace(name[:len(name)-len(m[0])]), m[1]
			}
			rest := toks[1:]
			// A lab code sits left of the result column; a two-digit value
			// ("eGFR  90") sits under it.
			if len(rest) > 0 && code == "" && len(rest[0].text) == 2 && rest[0].start < cols.current-6 && strings.Trim(rest[0].text, "0123456789") == "" && rep.labCode(rest[0].text, true) {
				code, rest = rest[0].text, rest[1:]
				// Without a list of performing labs, two digits in the code
				// column with nothing after them may be the value of a
				// compressed row ("eGFR  90"). Refuse rather than guess.
				if len(rest) == 0 && (len(rep.Labs) == 0 || rep.LabsTorn) {
					return nil, fmt.Errorf("result %q has %q in the lab code column and no value; without a list of performing labs it may be the value", name, code)
				}
			}
			if strings.EqualFold(strings.TrimSuffix(name, ":"), "please note") {
				note = &Result{Panel: panel}
				for _, t := range rest {
					addComment(t.text)
				}
				continue
			}
			note = nil
			if len(rep.Results) >= maxReportResults {
				return nil, errors.New("report exceeds result limit")
			}
			cur = &Result{Name: name, Panel: panel, LabCode: code}
			rep.Results = append(rep.Results, cur)
			wantValue = len(rest) == 0
			if !wantValue && !cur.fill(rest, cols) {
				return nil, errors.New("ambiguous result columns")
			}
			continue
		}
		addComment(l)
	}
	if len(rep.Results) == 0 {
		return nil, fmt.Errorf("no results found")
	}
	// Refuse what cannot be read for sure. A wrong value in a health record is
	// worse than no import: the person can fix a report, not a silent error.
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
		// A row with no value may hide its value: one space after the name, or
		// under a header whose columns sit further right, it stays in the
		// name ("Glucose 101", "Urine Protein Trace"). So a row with no value
		// is taken only when the lab says why, in the first line under it,
		// and no word of its name after the first may be a value. A lab code
		// the list names was taken off the name already.
		if r.Value == "" {
			if len(r.Comment) == 0 || !notPerformed(r.Comment[0]) {
				return nil, fmt.Errorf("result %q has no value, and no line under it says the test was not performed", r.Name)
			}
			if hidesValue(r.Name) {
				return nil, fmt.Errorf("result %q has no value, and a word of its name may be the value", r.Name)
			}
		}
	}
	rep.Results = kept
	return rep, nil
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

// resultWords are categorical results that can sit in a name with no value.
var resultWords = map[string]bool{
	"negative": true, "positive": true, "reactive": true, "nonreactive": true, "non-reactive": true,
	"detected": true, "normal": true, "abnormal": true, "borderline": true, "equivocal": true,
	"indeterminate": true, "inconclusive": true, "pending": true, "immune": true,
	"trace": true, "clear": true, "cloudy": true, "hazy": true, "turbid": true,
	"present": true, "absent": true, "none": true, "tnp": true, "qns": true,
}

// flags are what the flag column holds. A one-letter flag counts in
// capitals only.
var flags = map[string]bool{"H": true, "L": true, "A": true, "high": true, "low": true, "abnormal": true, "critical": true, "panic": true}

var reDigitColon = regexp.MustCompile(`[0-9]:[0-9]`)

// hidesValue reports whether a word of a name, after the first, may be a
// value, a flag or a unit: it starts with a digit, '<', '>', '=' or '+', or
// with '-' and a digit; it holds '/', or ':' between digits; it is a flag or
// a result word.
func hidesValue(name string) bool {
	f := strings.Fields(name)
	for _, w := range f[min(1, len(f)):] {
		lw := strings.ToLower(strings.Trim(w, ",;."))
		switch {
		case strings.ContainsAny(w[:1], "0123456789<>=+"),
			len(w) > 1 && w[0] == '-' && w[1] >= '0' && w[1] <= '9',
			strings.Contains(w, "/"),
			reDigitColon.MatchString(w),
			flags[w] || len(lw) > 1 && flags[lw],
			resultWords[lw]:
			return true
		}
	}
	return false
}

// labCode reports whether a two-digit token can be a lab code. With a list of
// performing labs that reads whole, it is a code the list names. Without one,
// only a token in the code column is a code, and two digits at the end of a
// name stay in the name. So "Interleukin 10" keeps its number unless the list
// names a lab 10.
func (rep *Report) labCode(c string, inColumn bool) bool {
	if len(rep.Labs) == 0 || rep.LabsTorn {
		return inColumn
	}
	return rep.Labs[c] != ""
}

func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// observed is the collection time: minute precision in loc when the
// report gives a time and loc is known, else the day in UTC.
func (rep *Report) observed(loc *time.Location) (map[string]any, string) {
	if loc != nil {
		if t, err := time.ParseInLocation("01/02/2006 1504", rep.Collected, loc); err == nil && uniqueLocalTime(t, rep.Collected) {
			start := t.Format(time.RFC3339)
			if _, ok := core.ParseTime(start); ok {
				return map[string]any{"start": start, "precision": "minute"}, ""
			}
		}
	}
	t, err := time.Parse("01/02/2006", rep.Collected[:10])
	if err != nil {
		return nil, rep.Collected
	}
	start := t.Format("2006-01-02") + "T00:00:00Z"
	if _, ok := core.ParseTime(start); !ok {
		return nil, rep.Collected
	}
	return map[string]any{"start": start, "precision": "day"}, rep.Collected
}

// A zone alone cannot identify a nonexistent or repeated DST wall-clock minute.
// Retain day precision and the printed local time in those cases.
func uniqueLocalTime(t time.Time, printed string) bool {
	const layout = "01/02/2006 1504"
	if t.Format(layout) != printed {
		return false
	}
	_, offset := t.Zone()
	start, end := t.ZoneBounds()
	var neighbors []time.Time
	if !start.IsZero() {
		neighbors = append(neighbors, start.Add(-time.Second))
	}
	if !end.IsZero() {
		neighbors = append(neighbors, end)
	}
	for _, adjacent := range neighbors {
		_, other := adjacent.In(t.Location()).Zone()
		candidate := t.Add(time.Duration(offset-other) * time.Second)
		if !candidate.Equal(t) && candidate.Format(layout) == printed {
			return false
		}
	}
	return true
}

// records are the readings. reportSHA is the plaintext SHA-256 of the kept
// report, or empty when it is not kept. A reading names the report by that
// digest, not in blobs: then purging the lab.report erases the file.
func (rep *Report) records(reportSHA string, loc *time.Location) []map[string]any {
	observed, rawTime := rep.observed(loc)
	reportNote := strings.Join(rep.Notes, " ")
	occurrence := map[string]int{}
	var out []map[string]any
	for _, r := range rep.Results {
		ctx := map[string]any{"test": r.Name, "panel": r.Panel}
		var value map[string]any
		switch {
		case r.Value == "":
			value = map[string]any{"n": nil}
		case reNumber.MatchString(r.Value) && exact(r.Value):
			n, _ := strconv.ParseFloat(r.Value, 64)
			value = map[string]any{"n": n}
			if string(core.Canonical(n)) != r.Value {
				ctx["raw_value"] = r.Value // keep the printed digits: 1.190, 3.0
			}
		default:
			value = map[string]any{"s": r.Value}
		}
		if r.Units != "" && r.Value != "" {
			value["unit"] = r.Units
		}
		for k, v := range map[string]string{
			"flag": r.Flag, "reference_interval": r.Ref, "previous_result": r.Previous,
			"performing_lab": rep.Labs[r.LabCode], "reported": rep.Reported, "fasting": rep.Fasting,
			"raw_time": rawTime, "lab_comment": strings.Join(r.Comment, " "),
			"report_note": reportNote, "report_sha256": reportSHA,
		} {
			if v != "" {
				ctx[k] = v
			}
		}
		native := rep.Specimen + "/" + r.Name
		rec := map[string]any{
			"type":    "lab." + slug(r.Name),
			"value":   value,
			"times":   map[string]any{"observed": observed},
			"source":  map[string]any{"system": system, "adapter": version, "ref_input": core.RefInputNative(system, native, occurrence[native])},
			"context": ctx,
		}
		occurrence[native]++
		if observed == nil {
			rec["times"] = map[string]any{"observed": nil}
		}
		out = append(out, rec)
	}
	return out
}

// exact reports whether a printed number fits a record number (SPEC 4.1).
// A larger one stays text, so the import never fails on it.
func exact(v string) bool {
	// Keep unusually long numeric tokens as source text without costly big-int
	// parsing. Normal values must survive conversion to canonical decimal intact.
	if len(v) > 256 {
		return false
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || !core.ExactNumber(n) {
		return false
	}
	original, ok := new(big.Rat).SetString(v)
	if !ok {
		return false
	}
	canonical, ok := new(big.Rat).SetString(string(core.Canonical(n)))
	return ok && original.Cmp(canonical) == 0
}
