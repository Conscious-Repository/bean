// Command bean-import-csv turns a CSV file into JSONL partial records
// (SPEC.md 9 and 12.3). It never signs and never holds the dedupe key.
//
// The CSV has a header row. Column "value" is required. Optional columns:
// "time" (RFC 3339, the observed start), "id" (a native id). Any other
// column is copied into context as a string.
//
// With -blob it first writes a csv.file record that keeps the file, as
// lab.report keeps a lab report, so bean import csv works. Readings name the
// file by its SHA-256 in context.file_sha256, not in blobs: then purging the
// csv.file erases the file.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Conscious-Repository/bean/core"
)

const version = "bean-import-csv@0.6.0"

// maxFile bounds a file kept with -blob, as bean import does.
const maxFile = 256 << 20

var blobPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type options struct {
	typ, system, blob, unit, precision string
	digest                             string // plaintext SHA-256 of the kept file
}

func main() {
	fs := flag.NewFlagSet("bean-import-csv", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var o options
	fs.StringVar(&o.typ, "type", "", "record type, for example hrv.rmssd")
	fs.StringVar(&o.system, "system", "csv", "source.system")
	fs.StringVar(&o.blob, "blob", "", "blob id of the raw file, from bean blob put")
	fs.StringVar(&o.unit, "unit", "", "unit of value")
	fs.StringVar(&o.precision, "precision", "second", "precision of the time column")
	if err := fs.Parse(os.Args[1:]); err != nil || o.typ == "" || fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: bean-import-csv -type <type> [-system <system>] [-blob <blob id>] [-unit <unit>] [-precision <p>] <file.csv>")
		os.Exit(2)
	}
	if o.blob == "" {
		fmt.Fprintln(os.Stderr, noBlobWarning)
	}
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "bean-import-csv: %s\n", err)
		os.Exit(1)
	}
	defer f.Close()
	var in io.Reader = f
	if o.blob != "" {
		raw, err := keep(f, os.Stdout, &o)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bean-import-csv: %s\n", err)
			os.Exit(1)
		}
		in = bytes.NewReader(raw)
	}
	if err := convert(in, os.Stdout, o); err != nil {
		fmt.Fprintf(os.Stderr, "bean-import-csv: %s\n", err)
		os.Exit(1)
	}
}

// keep reads the whole file, checks that every row converts, and writes the
// csv.file record. A file that does not convert writes nothing.
func keep(f io.Reader, out io.Writer, o *options) ([]byte, error) {
	if !blobPattern.MatchString(o.blob) {
		return nil, errors.New("invalid blob reference")
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxFile {
		return nil, fmt.Errorf("the file is larger than %d MiB", maxFile>>20)
	}
	sum := sha256.Sum256(raw)
	o.digest = hex.EncodeToString(sum[:])
	if err := convert(bytes.NewReader(raw), io.Discard, *o); err != nil {
		return nil, err
	}
	_, err = out.Write(append(core.Canonical(archive(raw, *o)), '\n'))
	return raw, err
}

// archive is the record that keeps the file.
func archive(raw []byte, o options) map[string]any {
	value := map[string]any{"blob": o.blob}
	return map[string]any{
		"type":  "csv.file",
		"value": value,
		"times": map[string]any{"observed": nil},
		"source": map[string]any{
			"system": o.system, "adapter": version,
			"ref_input": core.RefInputTuple(o.system, "csv.file", nil, value, 0),
		},
		"context": map[string]any{"sha256": o.digest, "bytes": len(raw), "assurance": "unsigned-file"},
		"blobs":   []any{map[string]any{"hash": o.blob, "role": "raw-export", "media_type": "text/csv"}},
	}
}

// noBlobWarning reminds the person to keep the raw file first (SPEC.md 9.3
// step 1): records without it cannot be re-parsed when the adapter improves.
const noBlobWarning = "bean-import-csv: warning: no -blob: the raw file is not kept; run bean blob put <file.csv> first and pass -blob <id>"

// convert writes one partial record per CSV row.
func convert(in io.Reader, out io.Writer, o options) error {
	br := bufio.NewReader(in)
	// Spreadsheet exports often start with a UTF-8 byte order mark; it is
	// not part of the first column name.
	if b, err := br.Peek(3); err == nil && string(b) == "\ufeff" {
		br.Discard(3)
	}
	r := csv.NewReader(br)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return err
	}
	for _, h := range header {
		if !utf8.ValidString(h) {
			return errors.New("line 1: bad string: not UTF-8")
		}
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	vi, ok := col["value"]
	if !ok {
		return errors.New("no value column")
	}
	ti, hasTime := col["time"]
	ii, hasID := col["id"]
	if _, has := col["file_sha256"]; has && o.digest != "" {
		return errors.New("a column is named file_sha256, which names the kept file")
	}
	occurrence := map[string]int{} // for identical readings without a native id
	for line := 2; ; line++ {
		row, err := r.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("line %d: %s", line, err)
		}
		for _, f := range row {
			if !utf8.ValidString(f) {
				return fmt.Errorf("line %d: bad string: not UTF-8", line)
			}
		}
		get := func(i int) string {
			if i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		// SPEC.md 3.4 shapes; 9.6: null when the source has none, never invent.
		value := map[string]any{"n": nil}
		if v := get(vi); v != "" {
			if n, ok := number(v); ok {
				value = map[string]any{"n": n}
				if o.unit != "" {
					value["unit"] = o.unit
				}
			} else {
				value = map[string]any{"s": v}
			}
		}
		context := map[string]any{}
		var observed any
		if raw := get(ti); hasTime && raw != "" {
			// Keep the time as written, with its own offset.
			if _, ok := core.ParseTime(raw); ok {
				observed = map[string]any{"start": raw, "precision": o.precision}
			} else {
				context["raw_time"] = raw
			}
		}
		for name, i := range col {
			if name == "value" || name == "time" || name == "id" {
				continue
			}
			if v := get(i); v != "" {
				context[name] = v
			}
		}
		src := map[string]any{"system": o.system, "adapter": version}
		if hasID && get(ii) != "" {
			src["ref_input"] = core.RefInputNative(o.system, get(ii), 0)
		} else {
			key := string(core.Canonical([]any{observed, value}))
			src["ref_input"] = core.RefInputTuple(o.system, o.typ, observed, value, occurrence[key])
			occurrence[key]++
		}
		rec := map[string]any{
			"type":   o.typ,
			"value":  value,
			"times":  map[string]any{"observed": observed},
			"source": src,
		}
		if o.digest != "" {
			context["file_sha256"] = o.digest
		}
		if len(context) > 0 {
			rec["context"] = context
		}
		if _, err := out.Write(append(core.Canonical(rec), '\n')); err != nil {
			return err
		}
	}
}

// decimal is a plain decimal number as a person or a spreadsheet writes it.
// strconv.ParseFloat also takes "NaN", "Inf", hex floats and underscores;
// those are kept as text.
var decimal = regexp.MustCompile(`^[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

// number parses v as a finite decimal number. An integer outside the
// exact range of a float64 (from 2^53 up) is kept as text so no digit is
// lost, and so is any value that SPEC 4.1 cannot carry (core.ExactNumber).
func number(v string) (float64, bool) {
	if !decimal.MatchString(v) {
		return 0, false
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || !core.ExactNumber(n) {
		return 0, false
	}
	if !strings.ContainsAny(v, ".eE") && math.Abs(n) >= 1<<53 {
		return 0, false
	}
	return n, true
}
