package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"bean/core"
)

// parseInterspersed lets flags come before or after the positional
// arguments, so "hlog add symptom.headache 8 -unit {score}" works.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

// parseBound reads a -since or -until value as hlog add -at does.
func parseBound(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	start, _, err := core.ParseWhen(s, time.Local)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339Nano, start)
}

func cmdList(s *core.Store, args []string) {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	typ := fs.String("type", "", "type, or prefix ending in *")
	since := fs.String("since", "", "start time or date")
	until := fs.String("until", "", "end time or date (exclusive)")
	author := fs.String("author", "", "author id or tagged key; me for the local identity")
	all := fs.Bool("all", false, "include superseded, duplicate and core records")
	asJSON := fs.Bool("json", false, "print the stored record bytes, one per line")
	pos, err := parseInterspersed(fs, args)
	if err != nil || len(pos) != 0 {
		usage()
		os.Exit(2)
	}
	f := core.Filter{Type: *typ, Author: *author, All: *all}
	if f.Since, err = parseBound(*since); err != nil {
		fail(err)
	}
	if f.Until, err = parseBound(*until); err != nil {
		fail(err)
	}
	if f.Author == "me" {
		local, err := s.ReadLocal()
		if err != nil {
			fail(err)
		}
		f.Author = local.Identity
	}
	recs, _, err := s.List(f)
	if err != nil {
		fail(err)
	}
	if *asJSON {
		for _, r := range recs {
			os.Stdout.Write(append(r.Bytes, '\n'))
		}
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, r := range recs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", whenText(r), r.Type, valueText(r), r.ID[7:19])
	}
	tw.Flush()
}

func cmdShow(s *core.Store, args []string) {
	if len(args) != 1 {
		usage()
		os.Exit(2)
	}
	l, err := s.Load()
	if err != nil {
		fail(err)
	}
	r, err := l.Resolve(args[0])
	if err != nil {
		fail(err)
	}
	fmt.Println("id", r.ID)
	fmt.Println("status", l.Status(r.ID))
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, r.Bytes, "", "  "); err != nil {
		fail(err)
	}
	fmt.Println(pretty.String())
	for _, b := range r.Blobs {
		fmt.Printf("blob %s %s %s\n", b.Hash, b.Role, b.MediaType)
	}
	for _, x := range l.Replies(r.ID) {
		rel := ""
		for _, ref := range x.Refs {
			if ref.ID == r.ID {
				rel = ref.Rel
				break
			}
		}
		fmt.Printf("reply %s %s %s %s\n", x.ID, rel, x.Type, valueText(x))
	}
}

func cmdAdd(s *core.Store, args []string) {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	unit := fs.String("unit", "", "unit of a number, UCUM where possible")
	at := fs.String("at", "", "observed time: RFC 3339, YYYY-MM-DD HH:MM or YYYY-MM-DD (default now)")
	note := fs.String("note", "", "free text, stored in context.note")
	about := fs.String("about", "", "id of a record this claim comments on")
	pos, err := parseInterspersed(fs, args)
	if err != nil || len(pos) != 2 {
		usage()
		os.Exit(2)
	}
	o := core.AddOptions{Unit: *unit, Note: *note}
	if *at != "" {
		if o.Start, o.Precision, err = core.ParseWhen(*at, time.Local); err != nil {
			fail(err)
		}
	}
	if *about != "" {
		l, err := s.Load()
		if err != nil {
			fail(err)
		}
		r, err := l.Resolve(*about)
		if err != nil {
			fail(err)
		}
		o.About = r.ID
	}
	id, err := s.Add(pos[0], pos[1], o)
	if err != nil {
		fail(err)
	}
	fmt.Println(id)
}

// whenText is the observed start cut to its precision, or "-".
func whenText(r *core.Record) string {
	obs, ok := r.Observed.(map[string]any)
	if !ok {
		return "-"
	}
	start, _ := obs["start"].(string)
	switch obs["precision"] {
	case "year":
		return start[:min(4, len(start))]
	case "month":
		return start[:min(7, len(start))]
	case "day":
		return start[:min(10, len(start))]
	}
	if t, err := time.Parse(time.RFC3339Nano, start); err == nil {
		return t.Local().Format("2006-01-02 15:04") // same zone as -since and -until
	}
	return start
}

// valueText is a one-line view of value and context.note.
func valueText(r *core.Record) string {
	v := r.Value
	var out string
	switch {
	case v == nil:
		out = ""
	case v["n"] != nil:
		n, _ := v["n"].(float64)
		out = strconv.FormatFloat(n, 'g', -1, 64)
		if u, ok := v["unit"].(string); ok && u != "" {
			out += " " + u
		}
	case v["s"] != nil:
		out, _ = v["s"].(string)
	case v["b"] != nil:
		out = fmt.Sprint(v["b"])
	case v["blob"] != nil:
		b, _ := v["blob"].(string)
		b = strings.TrimPrefix(b, "sha256:")
		out = "series " + b[:min(12, len(b))]
	case r.Type == "hlog.genesis" || r.Type == "hlog.succession":
		out = r.ValueKey()
	default:
		out = string(core.Canonical(v))
	}
	if note, ok := r.Context["note"].(string); ok && note != "" {
		if out != "" {
			out += "  "
		}
		out += "# " + note
	}
	if rs := []rune(out); len(rs) > 80 {
		out = string(rs[:77]) + "..."
	}
	return out
}
