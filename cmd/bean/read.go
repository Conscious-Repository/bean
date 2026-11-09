package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Conscious-Repository/bean/core"
)

// parseInterspersed lets flags come before or after the positional
// arguments, so "bean add symptom.headache 8 -unit {score}" works.
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

// badArg reports an argument that the command cannot read, such as a bad
// time, and exits 2 like any other usage error.
func badArg(err error) {
	fmt.Fprintf(os.Stderr, "bean: %s\n", safe(err.Error()))
	usage()
}

// parseBound reads a -since or -until value as bean add -at does.
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
	}
	f := core.Filter{Type: *typ, Author: *author, All: *all}
	if f.Since, err = parseBound(*since); err != nil {
		badArg(err)
	}
	if f.Until, err = parseBound(*until); err != nil {
		badArg(err)
	}
	switch {
	case f.Author == "me":
		if f.Author, err = s.Identity(); err != nil {
			fail(err)
		}
	case f.Author == "agent":
		a, err := s.ReadAgent()
		if err != nil {
			fail(err)
		}
		f.Author = a.Signer.Public()
	default:
		if f.Author, err = core.KeyFromNpub(f.Author); err != nil {
			fail(err)
		}
	}
	recs, l, err := s.List(f)
	if err != nil {
		fail(err)
	}
	if *asJSON {
		for _, r := range recs {
			os.Stdout.Write(append(r.Bytes, '\n'))
		}
		if warnFrozen(l) {
			os.Exit(4)
		}
		return
	}
	me, _ := s.Identity()
	labels := l.AgentLabels()
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, r := range recs {
		by := byText(r, me, labels)
		if l.Report.InDoubt(r.ID) {
			by = strings.TrimSpace(by + " [in doubt]")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", whenText(r), r.Type, safe(recordText(r, l.Report.Records)), safe(by), r.ID[7:19])
	}
	tw.Flush()
	if warnFrozen(l) {
		os.Exit(4)
	}
}

// warnFrozen tells the person that the log they just read is not in its usual
// state. It reports whether the identity is frozen.
func warnFrozen(l *core.Loaded) bool {
	if !l.IsFrozen() {
		return false
	}
	fmt.Fprintf(os.Stderr, "bean: compromised: %s\nbean: two rotations split this identity, and it is frozen for good. Where the two sides differ, this list can leave a record out or show an old value: bean ls -all marks what is in doubt, and bean verify has the details. To go on writing, run bean migrate <empty folder>.\n", l.ID)
	return true
}

func cmdShow(s *core.Store, args []string) {
	if len(args) != 1 {
		usage()
	}
	l, err := s.Load()
	if err != nil {
		fail(err)
	}
	if l.IsTooNew() {
		fail(l.TooNewError())
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
	for _, line := range strings.Split(pretty.String(), "\n") {
		fmt.Println(safe(line))
	}
	for _, b := range r.Blobs {
		fmt.Printf("blob %s %s %s\n", b.Hash, safe(b.Role), safe(b.MediaType))
	}
	for _, x := range l.Replies(r.ID) {
		rel := ""
		for _, ref := range x.Refs {
			if ref.ID == r.ID {
				rel = ref.Rel
				break
			}
		}
		fmt.Printf("reply %s %s %s %s\n", x.ID, rel, x.Type, safe(valueText(x)))
	}
	if warnFrozen(l) {
		os.Exit(4)
	}
}

func cmdAdd(s *core.Store, args []string) {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	unit := fs.String("unit", "", "unit of a number, UCUM where possible")
	at := fs.String("at", "", "observed time: RFC 3339, YYYY-MM-DD HH:MM or YYYY-MM-DD (default now)")
	note := fs.String("note", "", "free text, stored in context.note")
	about := fs.String("about", "", "id of a record this claim comments on")
	photo := fs.String("photo", "", "image file to attach, stored encrypted")
	corrects := fs.String("corrects", "", "id of your record that this one replaces")
	agent := fs.Bool("agent", false, "sign with this machine's agent key, not yours")
	pos, err := parseInterspersed(fs, args)
	if err != nil || len(pos) != 2 {
		usage()
	}
	o := core.AddOptions{Unit: *unit, Note: *note, Agent: *agent}
	if *photo != "" {
		if o.Photo, o.PhotoType, err = readImage(*photo); err != nil {
			fail(err)
		}
	}
	if *at != "" {
		if o.Start, o.Precision, err = core.ParseWhen(*at, time.Local); err != nil {
			badArg(err)
		}
	}
	o.About, o.Corrects = *about, *corrects
	id, err := s.Add(pos[0], pos[1], o)
	if err != nil {
		fail(err)
	}
	fmt.Println(id)
	// A correction makes the saved comparisons of the old reading stale.
	// Only then: this is the command people run most, and the check reads
	// the whole log.
	if o.Corrects != "" {
		sweepComparisons(s)
	}
}

// byText names the author when it is not the person: a paired agent's
// label, or the start of an unpaired key's npub.
func byText(r *core.Record, me string, labels map[string]string) string {
	switch {
	case r.Author == me:
		return ""
	case labels[r.Author] != "":
		return "[" + labels[r.Author] + "]"
	case core.Npub(r.Author) != "":
		return "[unpaired " + core.Npub(r.Author)[:14] + "]"
	}
	return "[" + r.Author[:min(18, len(r.Author))] + "]"
}

// readImage reads an image file for -photo and returns its media type.
// Anything that does not sniff as an image is refused, so a photo flag
// cannot pull an arbitrary file into the log.
func readImage(path string) ([]byte, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	// Stat the open file, not the path, and read at most maxPhoto+1
	// bytes: a device such as /dev/zero or a file that grows after the
	// check cannot exhaust memory.
	fi, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	if !fi.Mode().IsRegular() {
		return nil, "", errors.New("not a regular file: " + path)
	}
	if fi.Size() > maxPhoto {
		return nil, "", fmt.Errorf("photo larger than %d MiB", maxPhoto>>20)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxPhoto+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxPhoto {
		return nil, "", fmt.Errorf("photo larger than %d MiB", maxPhoto>>20)
	}
	mt := http.DetectContentType(data)
	if !strings.HasPrefix(mt, "image/") {
		return nil, "", fmt.Errorf("not an image: %s", mt)
	}
	return data, mt, nil
}

const maxPhoto = 25 << 20

// maxBlobPut is the largest file blob put stores: the most import-export
// reads back (core ops.go maxImportBlobBytes), so every export of the log
// can be read again. The format has no limit (DECISIONS 183, 210).
const maxBlobPut = 256 << 20

// readBlobFile reads a file for blob put, at most maxBlobPut bytes.
func readBlobFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tooLarge := fmt.Errorf("%s is larger than %d MiB, the most that import-export reads back", path, maxBlobPut>>20)
	if fi, err := f.Stat(); err != nil {
		return nil, err
	} else if fi.Mode().IsRegular() && fi.Size() > maxBlobPut {
		return nil, tooLarge
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBlobPut+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBlobPut {
		return nil, tooLarge
	}
	return data, nil
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

// blobText is a one-line view of a {"blob": id} value. SPEC 3.4 uses that
// shape for a series, with its unit, and for a large payload such as a
// kept lab report, without one. bean knows no series format (SPEC 9.4), so
// only a value with a unit is called a series; anything else is a file,
// with the media type its record states.
func blobText(r *core.Record) string {
	b, _ := r.Value["blob"].(string)
	short := strings.TrimPrefix(b, "sha256:")
	short = short[:min(12, len(short))]
	if u, ok := r.Value["unit"].(string); ok && u != "" {
		return "series " + short + " " + u
	}
	// A kept reference file says which data set it is.
	if d, ok := r.Context["dataset"].(string); ok && d != "" {
		return "file " + d + " " + short
	}
	for _, ref := range r.Blobs {
		if ref.Hash == b && ref.MediaType != "" {
			return "file " + ref.MediaType + " " + short
		}
	}
	return "file " + short
}

// recordText is valueText, and for a comparison what it compares and with
// whom: "percentile 46 of lab.cholesterol-total 187 mg/dL, among male, age
// 30-39 (n 339)". records is where its reading is looked up.
func recordText(r *core.Record, records map[string]*core.Record) string {
	cohort, _ := r.Context["cohort"].(map[string]any)
	among, _ := cohort["text"].(string)
	p, isNumber := r.Value["n"].(float64)
	if !strings.HasPrefix(r.Type, "comparison.") || among == "" || !isNumber {
		return valueText(r)
	}
	out := "percentile " + strconv.FormatFloat(p, 'g', -1, 64)
	reading, _ := r.Context["reading"].(string)
	if of := records[reading]; of != nil {
		out += " of " + of.Type + " " + valueText(of)
	}
	out += ", among " + among
	if result, ok := r.Context["result"].(map[string]any); ok {
		if n, ok := result["n"].(float64); ok {
			out += " (n " + strconv.FormatFloat(n, 'g', -1, 64) + ")"
		}
	}
	return out
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
		out = blobText(r)
	case r.Type == "bean.genesis" || r.Type == "bean.succession":
		out = r.ValueKey()
	case r.Type == "agent.pair" || r.Type == "device.pair":
		label, _ := v["label"].(string)
		out = strings.TrimSpace(label + " " + core.Npub(r.ValueKey()))
	case len(v) == 1 && v["n"] == nil:
		if _, has := v["n"]; has {
			out = "no value"
			// A lab says why it has no value: show that, not a bare blank.
			if c, _ := r.Context["lab_comment"].(string); strings.HasPrefix(strings.ToLower(c), "test not performed") || strings.HasPrefix(strings.ToLower(c), "not performed") {
				out = "not performed"
			}
			break
		}
		out = string(core.Canonical(v))
	default:
		out = string(core.Canonical(v))
	}
	if flag, ok := r.Context["flag"].(string); ok && flag != "" {
		out += "  " + flag // a lab's High or Low
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
