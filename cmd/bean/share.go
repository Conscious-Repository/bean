package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Conscious-Repository/bean/core"
)

// bean share writes chosen records, and with -files the files they keep,
// into a folder that anyone can check without bean's log, an account or the
// person's keys: a bundle. bean check-bundle checks one.
//
// A bundle is a small log: objects/ holds the chosen record files as they
// are stored, the identity records that show which keys speak for the
// person, and one share record, signed by the person, that lists exactly
// what the bundle holds. files/ holds the plaintext of each shared file,
// named by its SHA-256. So the checks of a log apply to it unchanged, and
// the second verifier (tools/second) can check it too.
//
// A bundle shows that what it holds is authentic. It does not show that it
// is complete: it is the records the person chose. And it is a disclosure:
// once sent, it cannot be taken back.

const (
	shareType   = "share"
	shareSystem = "bean-share"
	// shareStatement goes into every share record.
	shareStatement = "Selected records, not a complete history."
)

// sharedFile is one stored file that a shared record names.
type sharedFile struct {
	blob, record, mediaType string
	digest                  string // SHA-256 of the plaintext, "" when not known
	size                    int
	data                    []byte // the plaintext, with -files
}

func cmdShare(s *core.Store, args []string) {
	fs := flag.NewFlagSet("share", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	out := fs.String("out", "", "the folder to write, new or empty")
	typ := fs.String("type", "", "share every current record of this type, or prefix ending in *")
	since := fs.String("since", "", "with -type: from this time or date")
	until := fs.String("until", "", "with -type: before this time or date")
	files := fs.Bool("files", false, "also share the files that the records keep, in plaintext")
	note := fs.String("note", "", "a line for the reader, stored in the share record")
	yes := fs.Bool("yes", false, "write without the review")
	pos, err := parseInterspersed(fs, args)
	if err != nil || *out == "" || (len(pos) == 0 && *typ == "") || (*typ == "" && (*since != "" || *until != "")) {
		usage()
	}
	f := core.Filter{Type: *typ}
	if f.Since, err = parseBound(*since); err != nil {
		badArg(err)
	}
	if f.Until, err = parseBound(*until); err != nil {
		badArg(err)
	}
	recs, l, err := s.List(f)
	if err != nil {
		fail(err)
	}
	if l.ID == "" || l.Local == nil {
		fail(errors.New("only the person shares: this copy has no personal key"))
	}
	if l.IsFrozen() {
		warnFrozen(l)
		os.Exit(4)
	}
	// What the person chose.
	chosen := map[string]*core.Record{}
	shareable := func(r *core.Record) bool {
		return r.Subject == l.ID && l.Current(r.ID) && r.Type != shareType && !r.IsIdentity() && r.Type != "bean.tombstone" && r.Type != "agent.pair" && r.Type != "device.pair"
	}
	if *typ != "" {
		for _, r := range recs {
			if shareable(r) {
				chosen[r.ID] = r
			}
		}
		if len(chosen) == 0 {
			fail(fmt.Errorf("no current record of type %s in that time", *typ))
		}
	}
	for _, id := range pos {
		r, err := l.Resolve(id)
		if err != nil {
			fail(err)
		}
		if !shareable(r) {
			fail(fmt.Errorf("%s cannot be shared: it is %s, or not a record about you", r.ID, l.Status(r.ID)))
		}
		chosen[r.ID] = r
	}
	// A record is read with what it is about: a comparison with its
	// reading, an estimate with its meal. Those come along.
	for grew := true; grew; {
		grew = false
		for _, id := range slices.Sorted(maps.Keys(chosen)) {
			for _, target := range chosen[id].Targets("about") {
				if t := l.Report.Records[target]; t != nil && chosen[target] == nil && shareable(t) {
					chosen[target] = t
					grew = true
				}
			}
		}
	}
	shared := slices.Collect(maps.Values(chosen))
	sort.Slice(shared, func(i, j int) bool {
		wi, wj := core.When(shared[i]), core.When(shared[j])
		if !wi.Equal(wj) {
			return wi.Before(wj)
		}
		return shared[i].ID < shared[j].ID
	})
	// The files those records keep.
	var kept []sharedFile
	for _, r := range shared {
		for _, b := range r.Blobs {
			sf := sharedFile{blob: b.Hash, record: r.ID, mediaType: b.MediaType}
			stated, _ := r.Context["sha256"].(string)
			if r.Value["blob"] == b.Hash && len(r.Blobs) == 1 {
				sf.digest = stated
			}
			if *files {
				data, err := s.GetBlob(b.Hash)
				if err != nil {
					fail(fmt.Errorf("the file of %s cannot be read: %w", r.ID[7:19], err))
				}
				sum := sha256.Sum256(data)
				if got := hex.EncodeToString(sum[:]); sf.digest != "" && sf.digest != got {
					fail(fmt.Errorf("the file of %s does not match the SHA-256 in its record", r.ID[7:19]))
				} else {
					sf.digest = got
				}
				sf.data, sf.size = data, len(data)
			}
			kept = append(kept, sf)
		}
	}
	// The person sees what leaves before anything is written. Without a
	// terminal and without -yes nothing is written: an assistant that runs
	// this cannot disclose for the person.
	labels := l.AgentLabels()
	if !*yes {
		if !isTerminal(os.Stdin) {
			fail(errors.New("share needs a terminal: run bean share yourself to see what leaves, or pass -yes to write without review"))
		}
		fmt.Printf("To share, into %s:\n", *out)
		for _, r := range shared {
			fmt.Printf("  %s  %s  %s  %s\n", whenText(r), r.Type, safe(strings.TrimSpace(valueText(r)+"  "+byText(r, l.ID, labels))), r.ID[7:19])
		}
		for _, sf := range kept {
			if *files {
				fmt.Printf("  file of %s: %s, %d bytes, in plaintext\n", sf.record[7:19], safe(sf.mediaType), sf.size)
			} else {
				fmt.Printf("  file of %s: named, not shared (pass -files to share it)\n", sf.record[7:19])
			}
		}
		fmt.Println("A bundle is a disclosure: once sent, it cannot be taken back. Each record carries your identity id, so two bundles of yours can be linked.")
		fmt.Fprintf(os.Stderr, "Share %d records? [y/N] ", len(shared))
		if !confirmed(bufio.NewReader(os.Stdin)) {
			fail(errors.New("nothing was written: not shared"))
		}
	}
	if err := emptyFolder(*out); err != nil {
		fail(err)
	}
	// The share record: the person's signed list of what the bundle holds.
	// It stays in the log too, as the record of what was disclosed.
	var ids, fileList []any
	for _, r := range shared {
		ids = append(ids, r.ID)
	}
	for _, sf := range kept {
		e := map[string]any{"blob": sf.blob, "record": sf.record, "media_type": sf.mediaType, "included": *files}
		if sf.digest != "" {
			e["sha256"] = sf.digest
		}
		if *files {
			e["bytes"] = float64(sf.size)
		}
		fileList = append(fileList, e)
	}
	ctx := map[string]any{"records": ids, "statement": shareStatement}
	if fileList != nil {
		ctx["files"] = fileList
	}
	if *note != "" {
		ctx["note"] = *note
	}
	rec := map[string]any{
		"type":    shareType,
		"value":   map[string]any{"n": float64(len(shared)), "unit": "{record}"},
		"times":   map[string]any{"observed": map[string]any{"start": s.Now().Truncate(time.Minute).Format(time.RFC3339), "precision": "minute"}},
		"source":  map[string]any{"system": shareSystem, "adapter": core.Adapter, "ref": nil},
		"context": ctx,
	}
	res, err := s.Append(strings.NewReader(string(core.Canonical(rec)) + "\n"))
	if err != nil {
		fail(err)
	}
	if len(res.IDs) != 1 {
		fail(errors.New("the share record was not added"))
	}
	shareID := res.IDs[0]
	gone := func(err error) {
		fail(fmt.Errorf("the bundle was not written whole (%w). The share record %s is in your log: bean purge it if you send nothing", err, shareID[7:19]))
	}
	l, err = s.Load()
	if err != nil {
		gone(err)
	}
	// The records that show which keys speak for the person, and which
	// agents the person paired, for the readings an agent signed.
	write := map[string]*core.Record{shareID: l.Report.Records[shareID]}
	authors := map[string]bool{}
	for _, r := range shared {
		write[r.ID] = r
		authors[r.Author] = true
	}
	for id, r := range l.Report.Records {
		switch {
		case !l.Report.Valid[id]:
		case r.IsIdentity() && (id == l.ID || r.Subject == l.ID):
			write[id] = r
		case (r.Type == "agent.pair" || r.Type == "device.pair") && r.Author == l.ID && r.Subject == l.ID && authors[r.ValueKey()] && l.Current(id):
			write[id] = r
		}
	}
	for id, r := range write {
		if r == nil {
			gone(errors.New("a record is no longer in the log"))
		}
		p := filepath.Join(*out, filepath.FromSlash(core.PathOf(id)))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			gone(err)
		}
		if err := os.WriteFile(p, r.Bytes, 0o600); err != nil {
			gone(err)
		}
	}
	if *files {
		if err := os.MkdirAll(filepath.Join(*out, "files"), 0o700); err != nil {
			gone(err)
		}
		for _, sf := range kept {
			if err := os.WriteFile(filepath.Join(*out, "files", sf.digest), sf.data, 0o600); err != nil {
				gone(err)
			}
		}
	}
	// What was written must pass the check its reader will run.
	lines, ok := checkBundle(*out)
	if !ok {
		for _, line := range lines {
			fmt.Fprintln(os.Stderr, safe(line))
		}
		gone(errors.New("it does not pass bean check-bundle"))
	}
	n := 0
	if *files {
		n = len(kept)
	}
	fmt.Printf("shared %d records and %d files in %s\n", len(shared), n, *out)
	fmt.Printf("share record %s\n", shareID)
	fmt.Println("The bundle is not encrypted. Anyone can check it with bean check-bundle, or without bean: tools/second/checkbundle.py.")
}

// fileDigest is the SHA-256 of a file, or "" when it cannot be read.
func fileDigest(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// emptyFolder makes dir if it is not there, and refuses one that holds
// anything.
func emptyFolder(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(dir, 0o700)
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("%s is not empty: a bundle goes into a new or empty folder", dir)
	}
	return nil
}

func cmdCheckBundle(args []string) {
	if len(args) != 1 {
		usage()
	}
	lines, ok := checkBundle(args[0])
	for _, line := range lines {
		fmt.Println(safe(line))
	}
	if !ok {
		os.Exit(1)
	}
}

// checkBundle checks a bundle and changes nothing in it. It returns the
// lines to print and whether the bundle passed:
//
//  1. Every record file passes the checks of a log (bean verify): its id is
//     its hash, its signature is good, its key spoke for its author when it
//     signed.
//  2. There is exactly one share record, signed by a person about
//     themselves.
//  3. The records in the bundle are exactly the ones the share record
//     lists, each valid and about that person, plus that person's identity
//     and pairing records. Nothing was added and nothing removed. A record
//     that an agent or another key signed is valid, and counted apart: the
//     person chose to share it, and did not write it.
//  4. Every stored file that a listed record names is accounted for in the
//     share record, and every file in files/ is one the share record says
//     it includes, with the SHA-256 and size it gives, and with the SHA-256
//     its own record states, where the record states one.
func checkBundle(dir string) ([]string, bool) {
	var problems []string
	bad := func(format string, a ...any) { problems = append(problems, "problem: "+fmt.Sprintf(format, a...)) }
	done := func(lines []string) ([]string, bool) {
		sort.Strings(problems)
		lines = append(lines, problems...)
		if len(problems) > 0 {
			return append(lines, fmt.Sprintf("bundle FAILED: %d problems", len(problems))), false
		}
		return append(lines, "bundle ok"), true
	}
	if info, err := os.Stat(filepath.Join(dir, "objects")); err != nil || !info.IsDir() {
		bad("%s is not a bundle: it has no objects folder", dir)
		return done(nil)
	}
	l, err := core.Open(dir).LoadReadOnly()
	if err != nil {
		bad("the records cannot be read: %s", err)
		return done(nil)
	}
	rp := l.Report
	lines := rp.Lines()
	lines = lines[:len(lines)-1] // the summary counts a log, not a bundle
	for range rp.Errors {
		bad("a record file fails its checks (the error lines say which)")
		break
	}
	if len(rp.TooNew) > 0 || len(rp.Compromised) > 0 {
		bad("an identity in the bundle is too new for this bean, or frozen")
	}
	var share *core.Record
	for _, id := range slices.Sorted(maps.Keys(rp.Records)) {
		if r := rp.Records[id]; r.Type == shareType && rp.Valid[id] {
			if share != nil {
				bad("more than one share record: a bundle holds one")
				return done(lines)
			}
			share = r
		}
	}
	if share == nil {
		bad("no valid share record: nothing says what this bundle holds")
		return done(lines)
	}
	person := share.Author
	if !core.IsID(person) || share.Subject != person || share.Source.System != shareSystem {
		bad("the share record is not signed by a person about themselves")
		return done(lines)
	}
	listed := map[string]bool{}
	list, _ := share.Context["records"].([]any)
	for _, x := range list {
		id, _ := x.(string)
		if !core.IsID(id) || listed[id] {
			bad("the share record's list is damaged")
			return done(lines)
		}
		listed[id] = true
	}
	// Who signed the listed records: the person, an agent the person
	// paired (the pairing is in the bundle), or another key.
	paired := map[string]bool{}
	for id, r := range rp.Records {
		if (r.Type == "agent.pair" || r.Type == "device.pair") && r.Author == person && r.Subject == person && rp.Valid[id] {
			paired[r.ValueKey()] = true
		}
	}
	byPerson, byAgent, byOther := 0, 0, 0
	for _, id := range slices.Sorted(maps.Keys(rp.Records)) {
		r := rp.Records[id]
		if listed[id] {
			switch {
			case r.Author == person:
				byPerson++
			case paired[r.Author]:
				byAgent++
			default:
				byOther++
			}
		}
		switch {
		case id == share.ID:
		case r.IsIdentity() && (id == person || r.Subject == person):
		case (r.Type == "agent.pair" || r.Type == "device.pair") && r.Author == person && r.Subject == person:
		case !listed[id]:
			bad("%s is in the bundle, and the share record does not list it", id)
		case !rp.Valid[id]:
			bad("%s is listed, and is not valid", id)
		case r.Subject != person:
			bad("%s is listed, and is about someone else", id)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(listed)) {
		if rp.Records[id] == nil {
			bad("%s is listed, and is not in the bundle", id)
		}
	}
	// The files.
	type entry struct {
		digest   string
		size     float64
		included bool
	}
	account := map[string]entry{} // blob id and record id -> the share's word
	want := map[string]float64{}  // SHA-256 -> size, of the files it includes
	entries, _ := share.Context["files"].([]any)
	for _, x := range entries {
		m, _ := x.(map[string]any)
		blob, _ := m["blob"].(string)
		record, _ := m["record"].(string)
		e := entry{}
		e.digest, _ = m["sha256"].(string)
		e.size, _ = m["bytes"].(float64)
		e.included, _ = m["included"].(bool)
		if !core.IsID(blob) || !listed[record] || (e.included && len(e.digest) != 64) {
			bad("the share record's list of files is damaged")
			return done(lines)
		}
		account[blob+" "+record] = e
		if e.included {
			want[e.digest] = e.size
		}
	}
	included, named := 0, 0
	for id := range listed {
		r := rp.Records[id]
		if r == nil {
			continue
		}
		for _, b := range r.Blobs {
			e, ok := account[b.Hash+" "+id]
			if !ok {
				bad("%s names a stored file that the share record does not account for", id)
				continue
			}
			if stated, _ := r.Context["sha256"].(string); stated != "" && r.Value["blob"] == b.Hash && len(r.Blobs) == 1 && e.digest != "" && e.digest != stated {
				bad("the file of %s is not the one its record states", id)
			}
			if e.included {
				included++
			} else {
				named++
			}
		}
	}
	found, err := os.ReadDir(filepath.Join(dir, "files"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		bad("the files cannot be read: %s", err)
	}
	for _, f := range found {
		size, ok := want[f.Name()]
		if !ok {
			bad("files/%s is in the bundle, and the share record does not list it", f.Name())
			continue
		}
		delete(want, f.Name())
		if !f.Type().IsRegular() {
			bad("files/%s is not a plain file", f.Name())
			continue
		}
		// The size first: a file of another size is not read at all.
		if info, err := f.Info(); err != nil || float64(info.Size()) != size || fileDigest(filepath.Join(dir, "files", f.Name())) != f.Name() {
			bad("files/%s is not the file the share record lists", f.Name())
		}
	}
	for digest := range want {
		bad("files/%s is listed, and is not in the bundle", digest)
	}
	when := "-"
	if obs, ok := share.Observed.(map[string]any); ok {
		when, _ = obs["start"].(string)
	}
	lines = append(lines,
		fmt.Sprintf("share %s", share.ID),
		fmt.Sprintf("signed by identity %s, with key %s, at %s", person, share.Kid, when),
		fmt.Sprintf("records: %d listed; files: %d included, %d named and not included", len(listed), included, named),
		fmt.Sprintf("signed: %d by the person, %d by an agent the person paired, %d by another key", byPerson, byAgent, byOther),
	)
	if n, _ := share.Context["note"].(string); n != "" {
		lines = append(lines, "note: "+n)
	}
	lines = append(lines,
		"These are selected records, not a complete history: a correction or a deletion that the share leaves out does not show.",
		"A signature shows which identity signed, not that a value is true, nor who that identity is.",
	)
	return done(lines)
}
