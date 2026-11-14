// Command bean-import-lab turns a lab report PDF in a layout it knows into
// JSONL partial records (SPEC.md 9). It never signs and never holds the
// dedupe key. The layouts it knows are in files of their own: labcorp.go,
// the Labcorp patient report, and quest.go, the Quest Diagnostics report.
// Any other report is refused.
//
// Input is the PDF, which it reads with `pdftotext -layout` (poppler), or
// a text file that pdftotext already made. A Quest report is read by word
// position, with `pdftotext -bbox-layout`, from the PDF or from that
// output. One record per result row:
//
//   - type: "lab." plus a slug of the test name as printed ("LDL Chol Calc
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

const version = "bean-import-lab@0.5.0"

func main() {
	fs := flag.NewFlagSet("bean-import-lab", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	blob := fs.String("blob", "", "blob id of the PDF, from bean blob put")
	tz := fs.String("tz", "", "IANA zone of the collection site, for example America/Chicago")
	count := fs.Bool("count", false, "write no record: say how far each reader gets, in counts, with no value")
	if err := fs.Parse(os.Args[1:]); err != nil || fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: bean-import-lab [-blob <blob id>] [-tz <zone>] [-count] <report.pdf | report.txt | report.html>")
		os.Exit(2)
	}
	if *blob == "" && !*count {
		fmt.Fprintln(os.Stderr, "bean-import-lab: warning: no -blob: the report is not kept; run bean blob put <report.pdf> first and pass -blob <id>")
	}
	text, raw, err := readSource(fs.Arg(0))
	if err != nil {
		// The importer's own messages hold no data and say what to do (install
		// poppler, the size limit). An error from the system can name the
		// file, and a file name can be private, so it is not printed.
		var pe *os.PathError
		if errors.As(err, &pe) {
			fmt.Fprintln(os.Stderr, "bean-import-lab: the input could not be read")
		} else {
			fmt.Fprintf(os.Stderr, "bean-import-lab: %s\n", err)
		}
		os.Exit(1)
	}
	if *count {
		for _, line := range measure(text, raw) {
			fmt.Println(line)
		}
		return
	}
	var loc *time.Location
	if *tz != "" {
		if loc, err = time.LoadLocation(*tz); err != nil {
			fmt.Fprintf(os.Stderr, "bean-import-lab: bad -tz: %s\n", err)
			os.Exit(2)
		}
	}
	if *blob != "" && !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(*blob) {
		fmt.Fprintln(os.Stderr, "bean-import-lab: invalid blob reference")
		os.Exit(2)
	}
	rep, err := readReport(text, raw)
	if err == nil {
		err = rep.checkZone(loc)
	}
	if err != nil && *blob != "" && !looksLikeReport(text) {
		// Not a lab report at all, as far as this importer can tell: a
		// file of another kind is not kept as one.
		fmt.Fprintln(os.Stderr, "bean-import-lab: this does not look like a lab report to this importer: it found no line of column names, such as Test, Result and Reference Range, set apart by two spaces or more. A report in another language, or with its column names on two lines, is not known yet. To keep the file all the same: bean blob put <file>")
		os.Exit(1)
	}
	if err != nil && *blob != "" {
		// A report that cannot be read for sure is still the person's
		// report. Its file can be kept, and no result is written: a reading
		// is refused, never the file (docs/IMPORTERS.md, rule 1). The lab
		// is not named: a report that was not read does not say which.
		if werr := writeRecords(os.Stdout, []map[string]any{archiveFor(unreadSystem, raw, *blob)}); werr != nil {
			fmt.Fprintln(os.Stderr, "bean-import-lab: output failed")
			os.Exit(1)
		}
		// The reason can quote a line of the report, and bean prints it
		// where a script or an assistant may read it. What it quotes is
		// masked: the line number says where to look.
		fmt.Fprintf(os.Stderr, "bean-import-lab: no result was read: %s\n", masked(err.Error()))
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "bean-import-lab: %s\n", err)
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
		records = append([]map[string]any{archiveFor(rep.system(), raw, *blob)}, records...)
	}
	if err := writeRecords(os.Stdout, records); err != nil {
		fmt.Fprintln(os.Stderr, "bean-import-lab: output failed; partial output possible")
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "bean-import-lab: %d results, %d without a value. Importing the same report again adds nothing; a result whose value changed needs -corrections\n", len(rep.Results), missing)
}

// unreadSystem is the source.system of a report that was kept and not
// read: which lab printed it is not known.
const unreadSystem = "unknown"

// columnWords are the kinds of word that the line of column names of a
// lab report holds: what was tested, what was found, and what it is set
// against.
var columnWords = [][]string{
	{"test", "tests", "analyte", "component"},
	{"result", "results", "value", "in range", "out of range"},
	{"reference", "range", "interval", "units", "flag"},
}

// looksLikeReport reports whether a file that could not be read is a lab
// report all the same, as far as its text tells: it has a line of column
// names, or the marks of a Quest report. A line of column names has cells
// of a few words each, set apart by two spaces or more, and two of them
// are of different kinds. A sentence is one cell. It decides only whether
// the file is kept as a lab report, never a value.
func looksLikeReport(text string) bool {
	if isBBoxDoc(text) {
		pages, err := parseBBox(text)
		return err == nil && questPages(pages)
	}
	for _, l := range splitLines(text) {
		if strings.Join(strings.Fields(l), " ") == questTrademark {
			return true
		}
		found := [3]bool{}
		for _, cell := range columnGap.Split(strings.TrimSpace(l), -1) {
			words := strings.Fields(strings.ToLower(cell))
			if len(words) == 0 || len(words) > 4 {
				continue
			}
			name := " " + strings.Join(words, " ") + " "
		kind:
			for k, list := range columnWords {
				for _, w := range list {
					if strings.Contains(name, " "+w+" ") {
						found[k] = true
						break kind // a cell is of one kind
					}
				}
			}
		}
		if n := countTrue(found[:]); n >= 2 {
			return true
		}
	}
	return false
}

var columnGap = regexp.MustCompile(` {2,}|\t+`)

func countTrue(list []bool) int {
	n := 0
	for _, b := range list {
		if b {
			n++
		}
	}
	return n
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
	text, err := pdftotext(data, "-layout")
	if err != nil {
		return "", nil, err
	}
	return text, data, nil
}

// pdftotext runs pdftotext with mode (-layout or -bbox-layout) on a private
// copy of the PDF, bounded in time and output.
func pdftotext(data []byte, mode string) (string, error) {
	bin, err := exec.LookPath("pdftotext")
	if err != nil {
		return "", errors.New("pdftotext not found; install poppler or supply extracted layout text")
	}
	dir, err := os.MkdirTemp("", "bean-lab-report-*")
	if err != nil {
		return "", errors.New("cannot stage report")
	}
	defer os.RemoveAll(dir)
	snapshot := filepath.Join(dir, "source.pdf")
	if err = os.WriteFile(snapshot, data, 0600); err != nil {
		return "", errors.New("cannot stage report")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, mode, "-enc", "UTF-8", snapshot, "-")
	// A converter descendant may inherit the pipes after the direct child exits.
	cmd.WaitDelay = time.Second
	out := &boundedText{limit: maxReportText}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	if err = cmd.Run(); err != nil {
		return "", errors.New("PDF extraction failed, timed out or exceeded output limit")
	}
	if !utf8.Valid(out.Bytes()) {
		return "", errors.New("bad string: pdftotext output is not UTF-8")
	}
	return out.String(), nil
}

// readReport reads a report in a layout this importer knows. A Labcorp
// report is read from its -layout text by a grammar (parse.go). A Quest
// report is read by word position (questread.go), from the PDF or from
// pdftotext -bbox-layout output; its -layout text is refused, as its columns
// drift there. Any other report is refused.
func readReport(text string, raw []byte) (*Report, error) {
	switch {
	case isBBoxDoc(text):
		pages, err := parseBBox(text)
		if err != nil {
			return nil, err
		}
		if !questPages(pages) {
			return nil, errors.New("not a lab report in a layout this importer knows: the word positions are not a Quest Diagnostics report")
		}
		return parseQuest(pages)
	case !questDetect(text):
		return parseReport(text)
	case !bytes.HasPrefix(raw, []byte("%PDF")):
		return nil, errors.New("a Quest Diagnostics report is read by where each word is printed, which -layout text does not keep: give the PDF")
	}
	bbox, err := pdftotext(raw, "-bbox-layout")
	if err != nil {
		return nil, err
	}
	pages, err := parseBBox(bbox)
	if err != nil {
		return nil, err
	}
	return parseQuest(pages)
}

// questPages reports whether word positions are a Quest report: its
// trademark line and a results column header.
func questPages(pages []bpage) bool {
	trademark, header := false, false
	for _, p := range pages {
		for _, l := range p.lines {
			trademark = trademark || l.text() == questTrademark
			header = header || headerOf(l) != nil
		}
	}
	return trademark && header
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

// reNumber is a value stored as a number; any other value stays text.
var reNumber = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

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
// report gives a time and loc is known, else the day in UTC. When the
// report says the collection time was not on the requisition, the time it
// prints may be another one (receipt), so only the day is recorded and the
// printed time is kept as raw_time.
func (rep *Report) observed(loc *time.Location) (map[string]any, string) {
	if rep.System == questSystem {
		return rep.questObserved()
	}
	if loc != nil && !rep.collectionTimeUnsure() {
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

// collectionTimeUnsure reports a note that says the collection date or time
// was not given, as a report prints when the lab used the receipt date. The note
// may be a report note or a result's comment, and may wrap, so all of them
// are read as one text.
func (rep *Report) collectionTimeUnsure() bool {
	all := append([]string{}, rep.Notes...)
	for _, r := range rep.Results {
		all = append(all, r.Comment...)
	}
	text := strings.Join(strings.Fields(strings.ToLower(strings.Join(all, " "))), " ")
	return strings.Contains(text, collectionNote)
}

// collectionDayUnsure: the lab says it used the receipt date when no
// collection date was supplied, and the two days are the same, so the day
// recorded may be the day the lab received the specimen.
func (rep *Report) collectionDayUnsure() bool {
	return rep.collectionTimeUnsure() && rep.Received != "" && len(rep.Collected) >= 10 && rep.Received == rep.Collected[:10]
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
	collectionDay := ""
	if rep.collectionDayUnsure() {
		collectionDay = "may be the receipt day: the report says it used the receipt date when no collection date was supplied"
	}
	reportNote := strings.Join(rep.Notes, " ")
	system := rep.system()
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
			"collection_day": collectionDay,
			"report_note":    reportNote, "report_sha256": reportSHA,
		} {
			if v != "" {
				ctx[k] = v
			}
		}
		if r.Calculated {
			ctx["calculated"] = true
		}
		// A Quest test may be printed in two groups with two readings, such
		// as Albumin in two panels: the groups are part of its identity.
		native := rep.Specimen + "/" + r.Name
		if len(r.group) > 0 {
			native = rep.Specimen + "/" + strings.Join(r.group, "/") + "/" + r.Name
		}
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
