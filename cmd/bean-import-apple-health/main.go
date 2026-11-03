// Command bean-import-apple-health validates an Apple Health XML/ZIP export and
// emits an archive claim, optionally followed by individual reading claims.
// It never signs or holds identity keys. -blob must refer to the exact input.
// The inspected export has no sample IDs: readings use the standard tuple plus
// occurrence convention, NOT a fabricated native ID. Cross-export identity,
// corrections, source deletions and XML/HealthKit reconciliation are not inferred.
package main

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Conscious-Repository/bean/core"
)

const version = "bean-import-apple-health@0.1.1"
const system = "apple-health-xml"
const maxInput int64 = 256 << 20
const maxXML int64 = 256 << 20
const maxReadingOutput int64 = 1 << 30
const maxRecords = 2000000

type options struct {
	blob    string
	records bool
	types   string
}
type metric struct {
	slug     string
	category bool
}

var metrics = map[string]metric{
	"HKQuantityTypeIdentifierHeight":                         {"height", false},
	"HKQuantityTypeIdentifierBodyMass":                       {"body-mass", false},
	"HKQuantityTypeIdentifierStepCount":                      {"steps", false},
	"HKQuantityTypeIdentifierDistanceWalkingRunning":         {"distance-walking-running", false},
	"HKQuantityTypeIdentifierBasalEnergyBurned":              {"basal-energy", false},
	"HKQuantityTypeIdentifierActiveEnergyBurned":             {"active-energy", false},
	"HKQuantityTypeIdentifierFlightsClimbed":                 {"flights-climbed", false},
	"HKQuantityTypeIdentifierHeadphoneAudioExposure":         {"headphone-audio-exposure", false},
	"HKQuantityTypeIdentifierWalkingDoubleSupportPercentage": {"walking-double-support", false},
	"HKQuantityTypeIdentifierWalkingSpeed":                   {"walking-speed", false},
	"HKQuantityTypeIdentifierWalkingStepLength":              {"walking-step-length", false},
	"HKQuantityTypeIdentifierWalkingAsymmetryPercentage":     {"walking-asymmetry", false},
	"HKDataTypeSleepDurationGoal":                            {"sleep-duration-goal", false},
	"HKQuantityTypeIdentifierAppleWalkingSteadiness":         {"walking-steadiness", false},
	"HKCategoryTypeIdentifierSleepAnalysis":                  {"sleep-analysis", true},
	"HKCategoryTypeIdentifierHeadphoneAudioExposureEvent":    {"headphone-audio-exposure-event", true},
}
var blobPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var decimal = regexp.MustCompile(`^[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

func main() {
	fs := flag.NewFlagSet("bean-import-apple-health", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var o options
	fs.StringVar(&o.blob, "blob", "", "encrypted blob id of this exact input")
	fs.BoolVar(&o.records, "records", false, "also emit individual supported readings")
	fs.StringVar(&o.types, "types", "", "comma-separated apple-health reading types (requires -records)")
	if fs.Parse(os.Args[1:]) != nil || fs.NArg() != 1 || !blobPattern.MatchString(o.blob) {
		fmt.Fprintln(os.Stderr, "usage: bean-import-apple-health -blob sha256:ID [-records [-types apple-health.steps,...]] <export.zip|export.xml>")
		os.Exit(2)
	}
	if err := convertFile(fs.Arg(0), os.Stdout, o); err != nil {
		// Parsing errors intentionally exclude personal source attributes and values.
		fmt.Fprintln(os.Stderr, "bean-import-apple-health:", err)
		os.Exit(1)
	}
}

func convertFile(input string, out io.Writer, o options) error {
	return convertFileBounded(input, out, o, maxReadingOutput)
}

func convertFileBounded(input string, out io.Writer, o options, outputLimit int64) error {
	if !blobPattern.MatchString(o.blob) {
		return errors.New("invalid blob reference")
	}
	selected := map[string]bool{}
	if o.types != "" {
		if !o.records {
			return errors.New("-types requires -records")
		}
		for _, t := range strings.Split(o.types, ",") {
			t = strings.TrimPrefix(strings.TrimSpace(t), "apple-health.")
			known := false
			for _, m := range metrics {
				if m.slug == t {
					known = true
				}
			}
			if !known {
				return errors.New("unsupported reading type filter")
			}
			selected[t] = true
		}
	}
	// Take one private snapshot: parsing and hashing must see the same input bytes.
	f, err := os.OpenFile(input, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return errors.New("cannot open input")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > maxInput {
		return errors.New("input must be a regular file no larger than 256 MiB")
	}
	snapshot, err := os.CreateTemp("", "bean-apple-source-*")
	if err != nil {
		return err
	}
	defer os.Remove(snapshot.Name())
	defer snapshot.Close()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(snapshot, hash), io.LimitReader(f, maxInput+1))
	if err != nil {
		return errors.New("cannot snapshot input")
	}
	if n > maxInput {
		return errors.New("input exceeds 256 MiB")
	}
	if _, err = snapshot.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var head [4]byte
	_, _ = snapshot.ReadAt(head[:], 0)
	var reader io.Reader = snapshot
	media := "application/xml"
	if string(head[:]) == "PK\x03\x04" {
		media = "application/zip"
		zr, e := zip.NewReader(snapshot, n)
		if e != nil {
			return errors.New("invalid ZIP")
		}
		if len(zr.File) > 4096 {
			return errors.New("too many ZIP members")
		}
		var target *zip.File
		var total uint64
		for _, entry := range zr.File {
			name := strings.TrimSuffix(entry.Name, "/")
			if name == "" || path.IsAbs(name) || path.Clean(name) != name || strings.Contains(name, "\\") || name == ".." || strings.HasPrefix(name, "../") || entry.Mode()&os.ModeSymlink != 0 || (!entry.Mode().IsRegular() && !entry.FileInfo().IsDir()) {
				return errors.New("unsafe ZIP member")
			}
			if entry.UncompressedSize64 > uint64(maxXML)*2-total {
				return errors.New("ZIP declared contents exceed limit")
			}
			total += entry.UncompressedSize64
			if path.Base(name) == "export.xml" && !entry.FileInfo().IsDir() {
				if target != nil {
					return errors.New("ambiguous export.xml members")
				}
				target = entry
			}
		}
		if target == nil || target.UncompressedSize64 > uint64(maxXML) {
			return errors.New("missing or oversized export.xml")
		}
		member, e := target.Open()
		if e != nil {
			return errors.New("cannot open export.xml")
		}
		defer member.Close()
		reader = member
	}
	// Spool output until XML EOF and ZIP CRC have been checked. A malformed tail
	// must never leave a successfully importable prefix on stdout.
	rows, err := os.CreateTemp("", "bean-apple-rows-*")
	if err != nil {
		return err
	}
	defer os.Remove(rows.Name())
	defer rows.Close()
	writer := bufio.NewWriter(&limitedOutput{writer: rows, remaining: outputLimit})
	limited := &io.LimitedReader{R: reader, N: maxXML + 1}
	summary, err := parseExport(limited, writer, o, selected)
	if err != nil {
		return err
	}
	if limited.N <= 0 {
		return errors.New("XML exceeds 256 MiB")
	}
	if err = writer.Flush(); err != nil {
		return err
	}
	summary["sha256"] = hex.EncodeToString(hash.Sum(nil))
	summary["bytes"] = int(n)
	summary["assurance"] = "unsigned-export"
	summary["sample_identity"] = "not-provided"
	value := map[string]any{"blob": o.blob}
	archive := map[string]any{"type": "apple-health.export", "value": value, "times": map[string]any{"observed": nil},
		"source": map[string]any{"system": system, "adapter": version, "ref_input": core.RefInputTuple(system, "apple-health.export", nil, value, 0)},
		"blobs":  []any{map[string]any{"hash": o.blob, "role": "raw-export", "media_type": media}}, "context": summary}
	if _, err = out.Write(append(core.Canonical(archive), '\n')); err != nil {
		return err
	}
	if _, err = rows.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err = io.Copy(out, rows)
	return err
}

// Reject before writing beyond the private spool limit. convertFile publishes
// no JSONL until parsing and buffered output have both completed successfully.
type limitedOutput struct {
	writer    io.Writer
	remaining int64
}

func (w *limitedOutput) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, errors.New("reading output exceeds limit; use archive-only or select fewer types")
	}
	n, err := w.writer.Write(p)
	w.remaining -= int64(n)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}

func parseExport(in io.Reader, out io.Writer, o options, selected map[string]bool) (map[string]any, error) {
	d := xml.NewDecoder(in)
	counts := map[string]any{}
	occurrences := map[[32]byte]int{}
	depth := 0
	roots := 0
	records := 0
	activities := 0
	unmapped := 0
	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("invalid or truncated XML/ZIP contents")
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			if depth > 32 {
				return nil, errors.New("XML nesting exceeds limit")
			}
			if t.Name.Space != "" {
				return nil, errors.New("XML namespaces unsupported")
			}
			if depth == 1 {
				roots++
				if roots != 1 || t.Name.Local != "HealthData" {
					return nil, errors.New("expected one HealthData root")
				}
			}
			attrs := map[string]string{}
			size := 0
			for _, a := range t.Attr {
				if _, exists := attrs[a.Name.Local]; exists || a.Name.Space != "" {
					return nil, errors.New("duplicate or namespaced XML attribute")
				}
				size += len(a.Name.Local) + len(a.Value)
				if size > 65536 {
					return nil, errors.New("XML attributes exceed limit")
				}
				attrs[a.Name.Local] = a.Value
			}
			if t.Name.Local == "Record" {
				if depth != 2 {
					return nil, errors.New("record must be a HealthData child")
				}
				records++
				if records > maxRecords {
					return nil, errors.New("too many records")
				}
				typ := attrs["type"]
				if typ == "" || len(typ) > 256 {
					return nil, errors.New("missing or oversized record type")
				}
				count, _ := counts[typ].(int)
				counts[typ] = count + 1
				if len(counts) > 1024 {
					return nil, errors.New("too many record types")
				}
				m, known := metrics[typ]
				if !known {
					unmapped++
					continue
				}
				if o.records && (len(selected) == 0 || selected[m.slug]) {
					rec, e := reading(attrs, m, records, o.blob, occurrences)
					if e != nil {
						return nil, fmt.Errorf("record %d: %w", records, e)
					}
					if _, e = out.Write(append(core.Canonical(rec), '\n')); e != nil {
						return nil, e
					}
				}
			}
			if depth == 2 && t.Name.Local == "ActivitySummary" {
				activities++
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return nil, errors.New("unexpected XML text")
			}
		case xml.Directive:
			// Apple's export embeds a DTD. encoding/xml never fetches external entities;
			// only the five predefined XML entities and character references are accepted.
			if depth != 0 || roots != 0 || len(t) > 65536 || !strings.HasPrefix(strings.TrimSpace(string(t)), "DOCTYPE HealthData") {
				return nil, errors.New("unsupported XML directive")
			}
		}
	}
	if roots != 1 || depth != 0 {
		return nil, errors.New("incomplete HealthData document")
	}
	return map[string]any{"record_count": records, "record_types": counts, "activity_summary_count": activities, "unmapped_record_count": unmapped}, nil
}

func reading(a map[string]string, m metric, index int, blob string, occ map[[32]byte]int) (map[string]any, error) {
	value := map[string]any{}
	raw, exists := a["value"]
	if !exists || raw == "" {
		return nil, errors.New("missing reading value")
	}
	if m.category {
		// Categories are opaque Apple constants, never interpreted as quantities.
		if len(raw) > 256 || !strings.HasPrefix(raw, "HKCategoryValue") {
			return nil, errors.New("unsupported category value")
		}
		value["s"] = raw
	} else {
		n, err := strconv.ParseFloat(raw, 64)
		mantissa := raw
		if exponent := strings.IndexAny(raw, "eE"); exponent >= 0 {
			mantissa = raw[:exponent]
		}
		underflow := n == 0 && strings.Trim(mantissa, "+-.0") != ""
		if !decimal.MatchString(raw) || err != nil || !core.ExactNumber(n) || underflow || (!strings.ContainsAny(raw, ".eE") && math.Abs(n) >= 1<<53) {
			return nil, errors.New("invalid or inexact numeric value")
		}
		value["n"] = n
	}
	if u := a["unit"]; u != "" {
		if len(u) > 64 {
			return nil, errors.New("oversized unit")
		}
		value["unit"] = u
	}
	start, err := appleTime(a["startDate"])
	if err != nil {
		return nil, errors.New("invalid startDate")
	}
	end, err := appleTime(a["endDate"])
	if err != nil {
		return nil, errors.New("invalid endDate")
	}
	if end.Before(start) {
		return nil, errors.New("endDate precedes startDate")
	}
	observed := map[string]any{"start": start.Format(time.RFC3339), "end": end.Format(time.RFC3339), "precision": "second"}
	typ := "apple-health." + m.slug
	key := sha256.Sum256(core.Canonical([]any{typ, observed, value}))
	count := occ[key]
	occ[key] = count + 1
	context := map[string]any{"apple_type": a["type"], "source_record_index": index, "assurance": "unsigned-export", "identity_basis": "tuple-occurrence"}
	if a["creationDate"] != "" {
		created, e := appleTime(a["creationDate"])
		if e != nil {
			return nil, errors.New("invalid creationDate")
		}
		context["source_created"] = created.Format(time.RFC3339)
	}
	return map[string]any{"type": typ, "value": value, "times": map[string]any{"observed": observed}, "context": context,
		"source": map[string]any{"system": system, "adapter": version, "ref_input": core.RefInputTuple(system, typ, observed, value, count)},
		"blobs":  []any{map[string]any{"hash": blob, "role": "raw-export", "media_type": "application/octet-stream"}}}, nil
}

func appleTime(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02 15:04:05 -0700", s)
	if err != nil || t.Format("2006-01-02 15:04:05 -0700") != s {
		return time.Time{}, errors.New("invalid source time")
	}
	if _, ok := core.ParseTime(t.Format(time.RFC3339)); !ok {
		return time.Time{}, errors.New("unsupported source time")
	}
	return t, nil
}
