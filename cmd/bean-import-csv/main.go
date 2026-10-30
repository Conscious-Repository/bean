// Command bean-import-csv turns a CSV file into JSONL partial records
// (SPEC.md 9 and 12.3). It never signs and never holds the dedupe key.
//
// The CSV has a header row. Column "value" is required. Optional columns:
// "time" (RFC 3339, the observed start), "id" (a native id). Any other
// column is copied into context as a string.
package main

import (
	"bufio"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Conscious-Repository/bean/core"
)

const version = "bean-import-csv@0.5.1"

type options struct {
	typ, system, blob, unit, precision string
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
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "bean-import-csv: %s\n", err)
		os.Exit(1)
	}
	defer f.Close()
	if err := convert(f, os.Stdout, o); err != nil {
		fmt.Fprintf(os.Stderr, "bean-import-csv: %s\n", err)
		os.Exit(1)
	}
}

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
			if _, err := time.Parse(time.RFC3339Nano, raw); err == nil {
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
		if len(context) > 0 {
			rec["context"] = context
		}
		if o.blob != "" {
			rec["blobs"] = []any{map[string]any{"hash": o.blob, "role": "raw-export", "media_type": "text/csv"}}
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
