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
	version = "bean-import-labcorp@0.3.0"
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

// collectionTimeUnsure reports a report note that says the collection date
// or time was not given, as Labcorp prints when it used the receipt date.
func (rep *Report) collectionTimeUnsure() bool {
	for _, n := range rep.Notes {
		if strings.Contains(strings.ToLower(n), "time of collection was not indicated") {
			return true
		}
	}
	return false
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
