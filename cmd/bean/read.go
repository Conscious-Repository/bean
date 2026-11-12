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
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/view"
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
		text := recordText(r, currentRecord(l))
		if by != "" {
			text = view.Said(r, text)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", whenText(r), r.Type, safe(text), safe(by), r.ID[7:19])
	}
	tw.Flush()
	if warnFrozen(l) {
		os.Exit(4)
	}
}

// currentRecord looks a record up in the log, and returns it only when a
// reader should see it.
func currentRecord(l *core.Loaded) func(id string) *core.Record {
	return func(id string) *core.Record {
		if r := l.Report.Records[id]; r != nil && l.Current(id) {
			return r
		}
		return nil
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
	labels := l.AgentLabels()
	for _, x := range l.Replies(r.ID) {
		rel := ""
		for _, ref := range x.Refs {
			if ref.ID == r.ID {
				rel = ref.Rel
				break
			}
		}
		// Who wrote each one: a note of a peer is not the person's own.
		text, by := valueText(x), byText(x, l.ID, labels)
		if by != "" {
			text = view.Said(x, text) + "  " + by
		}
		fmt.Printf("reply %s %s %s %s\n", x.ID, rel, x.Type, safe(text))
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
	if o.Corrects != "" {
		// Two corrections of one record both stay current. And a correction
		// with no -at is dated now, which moves a reading in time.
		if l, err := s.Load(); err == nil {
			if t, err := l.Resolve(o.Corrects); err == nil {
				if by := l.Report.Superseded[t.ID]; len(by) > 0 {
					fail(fmt.Errorf("%s was corrected already, by %s: correct that record instead", t.ID[7:19], by[len(by)-1][7:19]))
				}
				if _, timed := t.Observed.(map[string]any); timed && *at == "" {
					fmt.Fprintf(os.Stderr, "bean: note: this correction is dated now. The record it replaces was at %s: pass -at to keep that time\n", whenText(t))
				}
			}
		}
	}
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
