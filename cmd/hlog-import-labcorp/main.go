// Command hlog-import-labcorp turns a Labcorp patient report ("Enterprise
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
//     lab_comment, report_note, reported, fasting.
//   - times.observed: "Date Collected" from the specimen details. The report
//     says "Local" without a zone, so the day is all that is known unless
//     -tz names the zone; then the minute is used.
//
// Patient name, birth date and patient ids are never copied.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Conscious-Repository/bean/core"
)

const (
	version = "hlog-import-labcorp@0.1.0"
	system  = "labcorp"
)

func main() {
	fs := flag.NewFlagSet("hlog-import-labcorp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	blob := fs.String("blob", "", "blob id of the PDF, from hlog blob put")
	tz := fs.String("tz", "", "IANA zone of the collection site, for example America/Chicago")
	if err := fs.Parse(os.Args[1:]); err != nil || fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: hlog-import-labcorp [-blob <blob id>] [-tz <zone>] <report.pdf | report.txt>")
		os.Exit(2)
	}
	text, err := readText(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "hlog-import-labcorp: %s\n", err)
		os.Exit(1)
	}
	var loc *time.Location
	if *tz != "" {
		if loc, err = time.LoadLocation(*tz); err != nil {
			fmt.Fprintf(os.Stderr, "hlog-import-labcorp: bad -tz: %s\n", err)
			os.Exit(2)
		}
	}
	rep, err := parseReport(text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hlog-import-labcorp: %s\n", err)
		os.Exit(1)
	}
	missing := 0
	for _, rec := range rep.records(*blob, loc) {
		if v := rec["value"].(map[string]any); v["n"] == nil && v["s"] == nil {
			missing++
		}
		os.Stdout.Write(append(core.Canonical(rec), '\n'))
	}
	fmt.Fprintf(os.Stderr, "hlog-import-labcorp: %d results, %d without a value, collected %s\n", len(rep.Results), missing, rep.Collected)
}

func readText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		return string(data), nil
	}
	bin, err := exec.LookPath("pdftotext")
	if err != nil {
		return "", fmt.Errorf("pdftotext not found; install poppler (brew install poppler) or pass the text from pdftotext -layout")
	}
	var out, errb bytes.Buffer
	cmd := exec.Command(bin, "-layout", "-enc", "UTF-8", path, "-")
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftotext: %v: %s", err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
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
	Notes     []string          // report-level notes ("Please note" sections)
	Results   []*Result
}

var (
	reSpecimen    = regexp.MustCompile(`Specimen ID:\s*(\S+)`)
	reCollected   = regexp.MustCompile(`Date Collected:\s*(\d\d/\d\d/\d{4})(?:\s+(\d{4})\s+Local)?`)
	reReported    = regexp.MustCompile(`Date Reported:\s*(\d\d/\d\d/\d{4})`)
	reFasting     = regexp.MustCompile(`Fasting:\s*(\S.*?)\s*$`)
	reLab         = regexp.MustCompile(`^(\d\d):\s*\S+\s*-\s*([^,]+),`)
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
	if c < 0 || p < 0 || u < 0 || r < 0 {
		return columns{}, false
	}
	return columns{current: c, previous: p, units: u, ref: r}, true
}

// fill assigns value tokens to the result. It reports false when the
// tokens do not look like a result row (a comment line).
func (r *Result) fill(toks []token, c columns) bool {
	for _, t := range toks {
		if len(t.text) > 25 {
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
	// In real reports units start 18 to 25 columns left of the "Reference
	// Interval" header and intervals 3 to 8 columns right of it.
	var units, ref []string
	for _, t := range right {
		if t.start >= c.ref-8 {
			ref = append(ref, t.text)
		} else {
			units = append(units, t.text)
		}
	}
	r.Units, r.Ref = strings.Join(units, " "), strings.Join(ref, " ")
	return true
}

func parseReport(text string) (*Report, error) {
	rep := &Report{Labs: map[string]string{}}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for _, l := range lines {
		if m := reSpecimen.FindStringSubmatch(l); m != nil && rep.Specimen == "" {
			rep.Specimen = m[1]
		}
		if m := reCollected.FindStringSubmatch(l); m != nil && (rep.Collected == "" || m[2] != "") {
			rep.Collected = strings.TrimSpace(m[1] + " " + m[2]) // the specimen details line has the time
		}
		if m := reReported.FindStringSubmatch(l); m != nil {
			rep.Reported = m[1]
		}
		if m := reFasting.FindStringSubmatch(l); m != nil && rep.Fasting == "" {
			rep.Fasting = m[1]
		}
		if m := reLab.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			rep.Labs[m[1]] = strings.TrimSpace(m[2])
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
lines:
	for _, l := range lines {
		l = strings.TrimRight(l, " \t\f")
		if c, ok := headerColumns(l); ok {
			cols, inResults = c, true
			wantValue = false
			continue
		}
		if !inResults {
			if strings.TrimSpace(l) != "" && indent(l) == 0 {
				panel = l // the first panel heading comes before the first header
			}
			continue
		}
		for _, re := range reBoilerplate {
			if re.MatchString(l) {
				continue lines
			}
		}
		if strings.TrimSpace(l) == "" {
			continue
		}
		if indent(l) == 0 {
			if strings.HasPrefix(l, "Disclaimer") || strings.HasPrefix(l, "Icon Legend") || strings.HasPrefix(l, "Performing Labs") {
				break
			}
			p := strings.TrimSuffix(l, " (Cont.)")
			if p != panel {
				note = nil
			}
			panel, wantValue = p, false
			continue
		}
		toks := tokens(l)
		if wantValue && indent(l) >= 20 {
			wantValue = false
			if cur.fill(toks, cols) {
				continue
			}
		}
		if indent(l) < 20 {
			// A row: name, optional lab code, then the value or nothing.
			name, code := toks[0].text, ""
			if m := reCode.FindStringSubmatch(name); m != nil {
				name, code = strings.TrimSpace(name[:len(name)-len(m[0])]), m[1]
			}
			rest := toks[1:]
			// A lab code sits left of the result column; a two-digit value
			// ("eGFR  90") sits under it.
			if len(rest) > 0 && code == "" && len(rest[0].text) == 2 && rest[0].start < cols.current-6 && strings.Trim(rest[0].text, "0123456789") == "" {
				code, rest = rest[0].text, rest[1:]
			}
			if strings.EqualFold(strings.TrimSuffix(name, ":"), "please note") {
				note = &Result{Panel: panel}
				for _, t := range rest {
					addComment(t.text)
				}
				continue
			}
			note = nil
			cur = &Result{Name: name, Panel: panel, LabCode: code}
			rep.Results = append(rep.Results, cur)
			wantValue = len(rest) == 0 || !cur.fill(rest, cols)
			continue
		}
		addComment(l)
	}
	if len(rep.Results) == 0 {
		return nil, fmt.Errorf("no results found")
	}
	return rep, nil
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
		if t, err := time.ParseInLocation("01/02/2006 1504", rep.Collected, loc); err == nil {
			return map[string]any{"start": t.Format(time.RFC3339), "precision": "minute"}, ""
		}
	}
	t, err := time.Parse("01/02/2006", rep.Collected[:10])
	if err != nil {
		return nil, rep.Collected
	}
	return map[string]any{"start": t.Format("2006-01-02") + "T00:00:00Z", "precision": "day"}, rep.Collected
}

func (rep *Report) records(blob string, loc *time.Location) []map[string]any {
	observed, rawTime := rep.observed(loc)
	occurrence := map[string]int{}
	var out []map[string]any
	for _, r := range rep.Results {
		ctx := map[string]any{"test": r.Name, "panel": r.Panel}
		var value map[string]any
		switch {
		case r.Value == "":
			value = map[string]any{"n": nil}
		case reNumber.MatchString(r.Value):
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
			"report_note": strings.Join(rep.Notes, " "),
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
		if blob != "" {
			rec["blobs"] = []any{map[string]any{"hash": blob, "role": "raw-export", "media_type": "application/pdf"}}
		}
		out = append(out, rec)
	}
	return out
}
