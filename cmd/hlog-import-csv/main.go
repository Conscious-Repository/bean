// Command hlog-import-csv turns a CSV file into JSONL partial records
// (SPEC.md 9 and 12.3). It never signs and never holds the dedupe key.
//
// The CSV has a header row. Column "value" is required. Optional columns:
// "time" (RFC 3339, the observed start), "id" (a native id). Any other
// column is copied into context as a string.
package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Conscious-Repository/bean/core"
)

const version = "hlog-import-csv@0.5.0"

type options struct {
	typ, system, blob, unit, precision string
}

func main() {
	fs := flag.NewFlagSet("hlog-import-csv", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var o options
	fs.StringVar(&o.typ, "type", "", "record type, for example hrv.rmssd")
	fs.StringVar(&o.system, "system", "csv", "source.system")
	fs.StringVar(&o.blob, "blob", "", "blob id of the raw file, from hlog blob put")
	fs.StringVar(&o.unit, "unit", "", "unit of value")
	fs.StringVar(&o.precision, "precision", "second", "precision of the time column")
	if err := fs.Parse(os.Args[1:]); err != nil || o.typ == "" || fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: hlog-import-csv -type <type> [-system <system>] [-blob <blob id>] [-unit <unit>] [-precision <p>] <file.csv>")
		os.Exit(2)
	}
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "hlog-import-csv: %s\n", err)
		os.Exit(1)
	}
	defer f.Close()
	if err := convert(f, os.Stdout, o); err != nil {
		fmt.Fprintf(os.Stderr, "hlog-import-csv: %s\n", err)
		os.Exit(1)
	}
}

// convert writes one partial record per CSV row.
func convert(in io.Reader, out io.Writer, o options) error {
	r := csv.NewReader(in)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return err
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
		get := func(i int) string {
			if i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		// SPEC.md 3.4 shapes; 9.6: null when the source has none, never invent.
		value := map[string]any{"n": nil}
		if v := get(vi); v != "" {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
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
