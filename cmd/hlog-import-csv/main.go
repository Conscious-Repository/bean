// Command hlog-import-csv turns a CSV file into JSONL partial records
// (SPEC.md 9 and 12.3). It never signs and never holds the dedupe key.
//
// The CSV has a header row. Column "value" is required. Optional columns:
// "time" (RFC 3339, the observed start), "id" (a native id). Any other
// column is copied into context as a string.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"bean/core"
)

const version = "hlog-import-csv@0.4.0"

func main() {
	fs := flag.NewFlagSet("hlog-import-csv", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	typ := fs.String("type", "", "record type, for example hrv.rmssd")
	system := fs.String("system", "csv", "source.system")
	blob := fs.String("blob", "", "blob id of the raw file, from hlog blob put")
	unit := fs.String("unit", "", "unit of value")
	precision := fs.String("precision", "second", "precision of the time column")
	if err := fs.Parse(os.Args[1:]); err != nil || *typ == "" || fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: hlog-import-csv -type <type> [-system <system>] [-blob <blob id>] [-unit <unit>] [-precision <p>] <file.csv>")
		os.Exit(2)
	}
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "hlog-import-csv: %s\n", err)
		os.Exit(1)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hlog-import-csv: %s\n", err)
		os.Exit(1)
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	vi, ok := col["value"]
	if !ok {
		fmt.Fprintln(os.Stderr, "hlog-import-csv: no value column")
		os.Exit(1)
	}
	ti, hasTime := col["time"]
	ii, hasID := col["id"]
	occurrence := map[string]int{} // for identical readings without a native id
	out := os.Stdout
	line := 1
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			fmt.Fprintf(os.Stderr, "hlog-import-csv: line %d: %s\n", line, err)
			os.Exit(1)
		}
		get := func(i int) string {
			if i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		// value (9.6: null when the source has none; never invent)
		var value any
		if v := get(vi); v != "" {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				value = map[string]any{"n": n}
				if *unit != "" {
					value.(map[string]any)["unit"] = *unit
				}
			} else {
				value = map[string]any{"text": v}
			}
		} else {
			value = map[string]any{"n": nil}
		}
		context := map[string]any{}
		var observed any
		if hasTime {
			raw := get(ti)
			if t, err := time.Parse(time.RFC3339Nano, raw); err == nil && raw != "" {
				observed = map[string]any{"start": t.UTC().Format(time.RFC3339Nano), "precision": *precision}
			} else if raw != "" {
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
		src := map[string]any{"system": *system, "adapter": version}
		if hasID && get(ii) != "" {
			src["ref_input"] = core.RefInputNative(*system, get(ii), 0)
		} else {
			key := string(core.Canonical([]any{observed, value}))
			src["ref_input"] = core.RefInputTuple(*system, *typ, observed, value, occurrence[key])
			occurrence[key]++
		}
		rec := map[string]any{
			"type":   *typ,
			"value":  value,
			"times":  map[string]any{"observed": observed},
			"source": src,
		}
		if len(context) > 0 {
			rec["context"] = context
		}
		if *blob != "" {
			rec["blobs"] = []any{map[string]any{"hash": *blob, "role": "raw-export", "media_type": "text/csv"}}
		}
		out.Write(core.Canonical(rec))
		out.Write([]byte{'\n'})
	}
}
