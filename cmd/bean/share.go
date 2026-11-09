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
// person's keys: a bundle. bean check-bundle checks one. docs/SHARE.md has
// the format and the rules.
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
)

// isPairing reports a pairing that the person signed about themselves.
func isPairing(r *core.Record, person string) bool {
	return (r.Type == "agent.pair" || r.Type == "device.pair") && r.Author == person && r.Subject == person
}

// statedDigest is the SHA-256 that a record states for the one file it
// keeps (context.sha256, as lab.report does), or "".
func statedDigest(r *core.Record, blob string) string {
	if len(r.Blobs) != 1 || r.Value["blob"] != blob {
		return ""
	}
	s, _ := r.Context["sha256"].(string)
	return s
}

// neverListed reports the kinds of record that a share does not list:
// identity records and pairings travel as proof, and a deletion or the
// record of a share is not shared.
func neverListed(r *core.Record) bool {
	return r.IsIdentity() || r.Type == "bean.tombstone" || r.Type == "agent.pair" || r.Type == "device.pair" || r.Type == shareType
}

// onChain reports an identity record of the person that their chain holds.
func onChain(rp *core.Report, r *core.Record, person string) bool {
	return r.IsIdentity() && (r.ID == person || r.Subject == person) && rp.Valid[r.ID] && !rp.OffTree(r)
}

// notShareable says why a record cannot go into a bundle, or "".
func notShareable(l *core.Loaded, r *core.Record) string {
	switch {
	case r.Type == shareType:
		return "it is the record of a share, which is not shared again"
	case neverListed(r):
		return "it is a " + r.Type + " record: your identity and pairing records travel with every bundle, and are not chosen"
	case r.Subject != l.ID:
		return "it is not about you"
	case !l.Current(r.ID):
		return "it is " + l.Status(r.ID)
	case core.IsID(r.Author) && r.Author != l.ID:
		return "another identity signed it, and a bundle carries only your identity records"
	}
	return ""
}

// sharedFile is one stored file that a shared record names.
type sharedFile struct {
	blob, record, mediaType, recordType string
	digest                              string // SHA-256 of the plaintext, with -files
	data                                []byte // the plaintext, with -files
}

func cmdShare(s *core.Store, args []string) {
	fs := flag.NewFlagSet("share", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	outFlag := fs.String("out", "", "the folder to write, new or empty")
	typ := fs.String("type", "", "share every current record of this type, or prefix ending in *")
	since := fs.String("since", "", "with -type: from this time or date")
	until := fs.String("until", "", "with -type: before this time or date")
	files := fs.Bool("files", false, "also share the files that the records keep, in plaintext")
	note := fs.String("note", "", "a line for the reader, stored in the share record")
	yes := fs.Bool("yes", false, "write without the review")
	pos, err := parseInterspersed(fs, args)
	if err != nil || *outFlag == "" || (len(pos) == 0 && *typ == "") || (*typ == "" && (*since != "" || *until != "")) {
		usage()
	}
	f := core.Filter{Type: *typ}
	if f.Since, err = parseBound(*since); err != nil {
		badArg(err)
	}
	if f.Until, err = parseBound(*until); err != nil {
		badArg(err)
	}
	out, err := filepath.Abs(*outFlag)
	if err != nil {
		fail(err)
	}
	if err := bundleTarget(out); err != nil {
		fail(err)
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
	// A type picks readings, never a record that keeps a file: 'lab.*' must
	// not sweep up the lab report itself, with the person's name in it. Such
	// a record is shared when it is named by its id.
	keepsFile := 0
	if *typ != "" {
		for _, r := range recs {
			switch {
			case notShareable(l, r) != "":
			case len(r.Blobs) > 0:
				keepsFile++
			default:
				chosen[r.ID] = r
			}
		}
		if len(chosen) == 0 {
			fail(fmt.Errorf("no current record of type %s in that time (a record that keeps a file is shared by its id, not by type)", *typ))
		}
	}
	for _, id := range pos {
		r, err := l.Resolve(id)
		if err != nil {
			fail(err)
		}
		if why := notShareable(l, r); why != "" {
			fail(fmt.Errorf("%s cannot be shared: %s", r.ID, why))
		}
		chosen[r.ID] = r
	}
	// A record is read with the record it is about: a comparison with its
	// reading, an estimate with its meal. That one comes along, and only
	// that one: what it is about in turn stays, unless it is chosen too.
	broughtBy := map[string]string{}
	var leftOut []string
	for _, id := range slices.Sorted(maps.Keys(chosen)) {
		for _, target := range chosen[id].Targets("about") {
			t := l.Report.Records[target]
			switch {
			case chosen[target] != nil:
			case t == nil:
				leftOut = append(leftOut, fmt.Sprintf("  not shared: %s, which %s is about: it is not in this log", target[7:19], id[7:19]))
			case notShareable(l, t) != "":
				leftOut = append(leftOut, fmt.Sprintf("  not shared: %s, which %s is about: %s", target[7:19], id[7:19], notShareable(l, t)))
			default:
				broughtBy[target] = id
			}
		}
	}
	for target := range broughtBy {
		chosen[target] = l.Report.Records[target]
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
	distinct := map[string]bool{}
	for _, r := range shared {
		for _, b := range r.Blobs {
			sf := sharedFile{blob: b.Hash, record: r.ID, mediaType: b.MediaType, recordType: r.Type}
			if *files {
				data, err := s.GetBlob(b.Hash)
				if err != nil {
					fail(fmt.Errorf("the file of %s cannot be read: %w", r.ID[7:19], err))
				}
				sum := sha256.Sum256(data)
				sf.digest, sf.data = hex.EncodeToString(sum[:]), data
				if stated := statedDigest(r, b.Hash); stated != "" && stated != sf.digest {
					fail(fmt.Errorf("the file of %s does not match the SHA-256 in its record", r.ID[7:19]))
				}
				distinct[sf.digest] = true
			}
			kept = append(kept, sf)
		}
	}
	// The records that show which keys speak for the person, and which
	// agents the person paired, for the readings an agent signed. The share
	// record lists them too, so that none can be taken out or put in.
	authors := map[string]bool{}
	for _, r := range shared {
		authors[r.Author] = true
	}
	var proof []*core.Record
	pairings := 0
	for _, id := range slices.Sorted(maps.Keys(l.Report.Records)) {
		r := l.Report.Records[id]
		switch {
		case !l.Report.Valid[id]:
		case onChain(l.Report, r, l.ID):
			proof = append(proof, r)
		case isPairing(r, l.ID) && authors[r.ValueKey()] && l.Current(id):
			proof = append(proof, r)
			pairings++
		}
	}
	// The person sees what leaves before anything is written, where the
	// question is asked. Without a terminal and without -yes nothing is
	// written: an assistant that runs this through a tool cannot disclose
	// for the person.
	labels := l.AgentLabels()
	if !*yes {
		if !isTerminal(os.Stdin) || !isTerminal(os.Stderr) {
			fail(errors.New("share needs a terminal: run bean share yourself to see what leaves, or pass -yes to write without review"))
		}
		w := os.Stderr
		fmt.Fprintf(w, "To share, into %s:\n", out)
		for _, r := range shared {
			line := fmt.Sprintf("  %s  %s  %s  %s", whenText(r), r.Type, safe(strings.TrimSpace(recordText(r, l.Report.Records)+"  "+byText(r, l.ID, labels))), r.ID[7:19])
			if by := broughtBy[r.ID]; by != "" {
				line += "  (comes along: " + by[7:19] + " is about it)"
			}
			fmt.Fprintln(w, line)
		}
		for _, line := range leftOut {
			fmt.Fprintln(w, line)
		}
		for _, sf := range kept {
			if *files {
				fmt.Fprintf(w, "  THE WHOLE FILE that %s (%s) keeps, as you imported it: %s, %d bytes, in plaintext. A lab report holds your name and birth date.\n", sf.record[7:19], sf.recordType, safe(sf.mediaType), len(sf.data))
			} else {
				fmt.Fprintf(w, "  the file that %s (%s) keeps: named, not shared (-files shares it)\n", sf.record[7:19], sf.recordType)
			}
		}
		if keepsFile > 0 {
			fmt.Fprintf(w, "  not shared: %d records of that type that keep a file. Name one by its id to share it.\n", keepsFile)
		}
		fmt.Fprintf(w, "  also: your %d identity records and %d pairings, and the share record itself\n", len(proof)-pairings, pairings)
		fmt.Fprintln(w, "A bundle is a disclosure: it is not encrypted, and once sent it cannot be taken back. Each record carries your identity id, so two bundles of yours can be linked.")
		fmt.Fprintf(w, "Share %d records? [y/N] ", len(shared))
		if !confirmed(bufio.NewReader(os.Stdin)) {
			fail(errors.New("nothing was written: not shared"))
		}
		// The log may have changed while the question waited.
		if l, err = s.Load(); err != nil {
			fail(err)
		}
		for _, r := range shared {
			if now := l.Report.Records[r.ID]; now == nil || notShareable(l, now) != "" {
				fail(fmt.Errorf("nothing was written: %s changed while you decided. Run bean share again", r.ID[7:19]))
			}
		}
	}
	// The share record: the person's signed list of what the bundle holds.
	var ids, proofIDs, fileList []any
	for _, r := range shared {
		ids = append(ids, r.ID)
	}
	for _, r := range proof {
		proofIDs = append(proofIDs, r.ID)
	}
	// The files that travel, and no others: a file that stays home is named
	// by its own record, and needs no word here.
	for _, sf := range kept {
		if *files {
			fileList = append(fileList, map[string]any{"blob": sf.blob, "record": sf.record, "sha256": sf.digest, "bytes": float64(len(sf.data))})
		}
	}
	ctx := map[string]any{"records": ids, "proof": proofIDs}
	if fileList != nil {
		ctx["files"] = fileList
	}
	if *note != "" {
		ctx["note"] = *note
	}
	line := core.Canonical(map[string]any{
		"type":    shareType,
		"value":   map[string]any{"n": float64(len(shared)), "unit": "{record}"},
		"times":   map[string]any{"observed": map[string]any{"start": s.Now().Truncate(time.Minute).Format(time.RFC3339), "precision": "minute"}},
		"source":  map[string]any{"system": shareSystem, "adapter": core.Adapter, "ref": nil},
		"context": ctx,
	})
	if len(line) > core.MaxRecordSize-4096 {
		fail(fmt.Errorf("%d records are too many for one share: its list would not fit one record. Share fewer at a time, for example by -since and -until", len(shared)))
	}
	shareID, shareBytes, err := s.SignPartial(line)
	if err != nil {
		fail(err)
	}
	// The bundle is written next to its place, checked as its reader will
	// check it, and moved there whole. Only then does the share record go
	// into the log: a failure leaves no bundle and no record of one.
	tmp, err := os.MkdirTemp(filepath.Dir(out), ".bean-share-*")
	if err != nil {
		fail(err)
	}
	gone := func(err error) {
		os.RemoveAll(tmp)
		fail(fmt.Errorf("nothing was shared: %w", err))
	}
	write := map[string][]byte{shareID: shareBytes}
	for _, r := range append(shared, proof...) {
		write[r.ID] = r.Bytes
	}
	for id, data := range write {
		p := filepath.Join(tmp, filepath.FromSlash(core.PathOf(id)))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			gone(err)
		}
		if err := os.WriteFile(p, data, 0o600); err != nil {
			gone(err)
		}
	}
	if len(fileList) > 0 {
		if err := os.MkdirAll(filepath.Join(tmp, "files"), 0o700); err != nil {
			gone(err)
		}
		for _, sf := range kept {
			if err := os.WriteFile(filepath.Join(tmp, "files", sf.digest), sf.data, 0o600); err != nil {
				gone(err)
			}
		}
	}
	if lines, ok := checkBundle(tmp); !ok {
		for _, line := range lines {
			fmt.Fprintln(os.Stderr, safe(line))
		}
		gone(errors.New("the bundle does not pass bean check-bundle"))
	}
	if err := os.Remove(out); err != nil && !errors.Is(err, os.ErrNotExist) {
		gone(err)
	}
	if err := os.Rename(tmp, out); err != nil {
		gone(err)
	}
	if err := s.AddSigned(shareID, shareBytes); err != nil {
		fail(fmt.Errorf("the bundle is written, in %s, and its share record could not be added to your log: %w", out, err))
	}
	fmt.Printf("shared %d records and %d files in %s\n", len(shared), len(distinct), out)
	fmt.Printf("share record %s\n", shareID)
	fmt.Println("The bundle is not encrypted. Anyone can check it with bean check-bundle, or without bean: tools/second/checkbundle.py.")
}

// bundleTarget refuses a place a bundle must not go: inside the log, onto
// a file, or into a folder that holds anything.
func bundleTarget(out string) error {
	if rel, err := filepath.Rel(logDir, out); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s is inside your log: a bundle goes somewhere else", out)
	}
	entries, err := os.ReadDir(out)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err != nil:
		return err
	case len(entries) > 0:
		return fmt.Errorf("%s is not empty: a bundle goes into a new or empty folder", out)
	}
	return nil
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

// bundleRecords reads the record files of a bundle: plain files two levels
// below objects/, as in a log. It reads nothing else of the folder, opens
// nothing that is not a plain file, and reads no file larger than a record
// can be. bad lists what it did not read.
func bundleRecords(dir string) (files map[string][]byte, bad []string, err error) {
	files = map[string][]byte{}
	shards, err := os.ReadDir(filepath.Join(dir, "objects"))
	if err != nil {
		return nil, nil, err
	}
	for _, sh := range shards {
		if strings.HasPrefix(sh.Name(), ".") {
			continue
		}
		if !sh.IsDir() {
			bad = append(bad, "objects/"+sh.Name())
			continue
		}
		entries, err := os.ReadDir(filepath.Join(dir, "objects", sh.Name()))
		if err != nil {
			return nil, nil, err
		}
		for _, e := range entries {
			rel := "objects/" + sh.Name() + "/" + e.Name()
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			info, err := e.Info()
			if err != nil || !e.Type().IsRegular() || info.Size() > core.MaxSuccessionSize {
				bad = append(bad, rel)
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, "objects", sh.Name(), e.Name()))
			if err != nil {
				return nil, nil, err
			}
			files[rel] = data
		}
	}
	return files, bad, nil
}

// checkBundle checks a bundle by the five rules of docs/SHARE.md, and
// changes nothing in it. It returns the lines to print and whether the
// bundle passed.
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
	// The folder: objects/ and, when it holds files, files/, real folders
	// and not links that lead out of it, and nothing else. Names that
	// start with a dot are not part of a bundle, as in a log: a file
	// browser leaves such files behind.
	if info, err := os.Lstat(filepath.Join(dir, "objects")); err != nil || !info.IsDir() {
		bad("%s is not a bundle: it has no objects folder", dir)
		return done(nil)
	}
	top, err := os.ReadDir(dir)
	if err != nil {
		bad("the folder cannot be read: %s", err)
		return done(nil)
	}
	// Anything else in the folder is not part of the bundle. It is never
	// read, so it cannot change the verdict, and it is named: pointing bean
	// at a bundle with -dir leaves a local folder there.
	var ignored []string
	for _, e := range top {
		switch name := e.Name(); {
		case strings.HasPrefix(name, "."):
		case name == "objects":
		case name == "files":
			if !e.IsDir() {
				bad("files is not a folder: a link or a file in its place is not followed")
			}
		default:
			ignored = append(ignored, "not part of the bundle, and not read: "+name)
		}
	}
	if len(problems) > 0 {
		return done(nil)
	}
	// Rule 1: every record file passes the checks of a log, with nothing
	// left over: no error, no warning, no record of a newer format.
	files, unread, err := bundleRecords(dir)
	if err != nil {
		bad("the records cannot be read: %s", err)
		return done(nil)
	}
	for _, p := range unread {
		bad("%s is not a record file: not a plain file, or too large", p)
	}
	rp := core.VerifyFiles(files)
	var lines []string
	for _, where := range slices.Sorted(maps.Keys(rp.Errors)) {
		lines = append(lines, fmt.Sprintf("error: %s: %s", where, rp.Errors[where]))
	}
	if len(rp.Errors) > 0 {
		bad("a record file fails a check of a log (the error lines say which)")
	}
	if len(rp.Unknown) > 0 || rp.Skipped > 0 || len(rp.TooNew) > 0 {
		bad("a record is of a newer format than this bean knows, so it cannot be checked")
	}
	if len(rp.Compromised) > 0 || len(rp.Incomplete) > 0 {
		bad("an identity in the bundle is frozen, or a key change is not whole")
	}
	// Rule 2: exactly one share record, signed by a person about themselves.
	var share *core.Record
	for _, id := range slices.Sorted(maps.Keys(rp.Records)) {
		if r := rp.Records[id]; r.Type == shareType {
			if share != nil {
				bad("more than one share record: a bundle holds one")
				return done(lines)
			}
			share = r
		}
	}
	if share == nil || !rp.Valid[share.ID] {
		bad("no valid share record: nothing says what this bundle holds")
		return done(lines)
	}
	person := share.Author
	if !core.IsID(person) || share.Subject != person || share.Source.System != shareSystem {
		bad("the share record is not signed by a person about themselves")
		return done(lines)
	}
	// Its lists, read strictly: a list that is not as SHARE.md says is not
	// read in a kinder way.
	idList := func(key string) (map[string]bool, bool) {
		out := map[string]bool{}
		list, ok := share.Context[key].([]any)
		for _, x := range list {
			id, _ := x.(string)
			if !core.IsID(id) || out[id] {
				return nil, false
			}
			out[id] = true
		}
		return out, ok
	}
	listed, ok1 := idList("records")
	proof, ok2 := idList("proof")
	if n, _ := share.Value["n"].(float64); !ok1 || !ok2 || n != float64(len(listed)) {
		bad("the share record's lists are damaged")
		return done(lines)
	}
	// The files it holds: each entry names a stored file of a listed record,
	// with the SHA-256 and the size of its plaintext.
	type entry struct {
		digest string
		size   float64
	}
	account := map[string]entry{} // blob id and record id -> the share's word
	want := map[string]float64{}  // SHA-256 -> size
	rawFiles, hasFiles := share.Context["files"]
	entries, isList := rawFiles.([]any)
	damaged := hasFiles && !isList
	for _, x := range entries {
		m, _ := x.(map[string]any)
		blob, _ := m["blob"].(string)
		record, _ := m["record"].(string)
		digest, _ := m["sha256"].(string)
		size, hasSize := m["bytes"].(float64)
		if _, twice := account[blob+" "+record]; twice || !core.IsID(blob) || !listed[record] || !core.IsID("sha256:"+digest) || !hasSize || size < 0 || size != float64(int64(size)) {
			damaged = true
			continue
		}
		if prev, ok := want[digest]; ok && prev != size {
			damaged = true
		}
		account[blob+" "+record] = entry{digest, size}
		want[digest] = size
	}
	if damaged {
		bad("the share record's list of files is damaged")
		return done(lines)
	}
	// Rule 1, the warnings. One is expected: a listed record that corrects
	// an earlier record is "held" by a log's verifier when that earlier
	// record is not there, and a share leaves it out on purpose. The reader
	// is told how many such corrections there are. Any other warning fails.
	corrections := 0
	for _, id := range slices.Sorted(maps.Keys(rp.Warnings)) {
		r := rp.Records[id]
		expected := r != nil && listed[id] && rp.Held[id] && len(r.Targets("supersedes")) > 0
		for _, w := range rp.Warnings[id] {
			expected = expected && strings.HasPrefix(w, "held: ")
		}
		for _, target := range r.Targets("supersedes") {
			expected = expected && rp.Records[target] == nil
		}
		if expected {
			corrections++
			continue
		}
		for _, w := range rp.Warnings[id] {
			lines = append(lines, fmt.Sprintf("warning: %s: %s", id, w))
		}
		bad("%s has a warning that a bundle may not have", id)
	}
	// Rule 3: the records are exactly the listed ones, each valid, about
	// the person and of a kind that is shared, plus the proof: the person's
	// identity records and pairings that the share record names. A record
	// that an agent or another key signed is valid, and counted apart: the
	// person chose to share it, and did not write it.
	paired := map[string]bool{}
	for id := range proof {
		if r := rp.Records[id]; r != nil && isPairing(r, person) && rp.Valid[id] {
			paired[r.ValueKey()] = true
		}
	}
	byPerson, byAgent, byOther := 0, 0, 0
	for _, id := range slices.Sorted(maps.Keys(rp.Records)) {
		r := rp.Records[id]
		switch {
		case id == share.ID:
		case listed[id] && proof[id]:
			bad("%s is listed as a record and as proof", id)
		case proof[id]:
			if !onChain(rp, r, person) && !(isPairing(r, person) && rp.Valid[id]) {
				bad("%s is named as proof, and is not a valid identity record or pairing of the person", id)
			}
		case !listed[id]:
			bad("%s is in the bundle, and the share record does not list it", id)
		case !rp.Valid[id]:
			bad("%s is listed, and is not valid", id)
		case r.Subject != person:
			bad("%s is listed, and is about someone else", id)
		case neverListed(r):
			bad("%s is listed, and is a %s record, which is not shared", id, r.Type)
		case r.Author == person:
			byPerson++
		case paired[r.Author]:
			byAgent++
		default:
			byOther++
		}
	}
	for _, id := range slices.Sorted(maps.Keys(listed)) {
		if rp.Records[id] == nil {
			bad("%s is listed, and is not in the bundle", id)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(proof)) {
		if rp.Records[id] == nil {
			bad("%s is named as proof, and is not in the bundle", id)
		}
	}
	// Rule 4: each entry is for a stored file that its listed record names,
	// and is the file that record states, where it states one. A stored file
	// with no entry stays home: it is counted, not faulted.
	named := 0
	for _, id := range slices.Sorted(maps.Keys(listed)) {
		r := rp.Records[id]
		if r == nil {
			continue
		}
		for _, b := range r.Blobs {
			e, ok := account[b.Hash+" "+id]
			if !ok {
				named++
				continue
			}
			delete(account, b.Hash+" "+id)
			if stated := statedDigest(r, b.Hash); stated != "" && e.digest != stated {
				bad("the file of %s is not the one its record states", id)
			}
		}
	}
	if len(account) > 0 {
		bad("the share record lists a file that no listed record names")
	}
	// Rule 5: files/ holds exactly the included files.
	included := len(want)
	found, err := os.ReadDir(filepath.Join(dir, "files"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		bad("the files cannot be read: %s", err)
	}
	for _, f := range found {
		if strings.HasPrefix(f.Name(), ".") {
			continue
		}
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
		fmt.Sprintf("corrections: %d of the listed records replace an earlier record, which is not in the bundle", corrections),
	)
	if n, _ := share.Context["note"].(string); n != "" {
		lines = append(lines, "note: "+n)
	}
	lines = append(lines, ignored...)
	// What it holds, shown only when every check passed: a reader should
	// not read values out of a bundle that failed.
	if len(problems) == 0 {
		lines = append(lines, "It holds:")
		var rows []*core.Record
		for id := range listed {
			rows = append(rows, rp.Records[id])
		}
		sort.Slice(rows, func(i, j int) bool {
			wi, wj := core.When(rows[i]), core.When(rows[j])
			if !wi.Equal(wj) {
				return wi.Before(wj)
			}
			return rows[i].ID < rows[j].ID
		})
		for _, r := range rows {
			by := ""
			switch {
			case r.Author == person:
			case paired[r.Author]:
				by = "  [signed by a paired agent]"
			default:
				by = "  [signed by another key]"
			}
			lines = append(lines, fmt.Sprintf("  %s  %s  %s%s  %s", whenText(r), r.Type, recordText(r, rp.Records), by, r.ID[7:19]))
		}
	}
	lines = append(lines, bundleCaveats...)
	return done(lines)
}

// bundleCaveats are what no bundle can show. Every check prints them.
var bundleCaveats = []string{
	"These are selected records, not a complete history: a correction or a deletion that the share leaves out does not show.",
	"A signature shows which identity signed, not that a value is true, nor who that identity is.",
	"The key above is the one that signed. A bundle cannot show that it is still the person's key: a key change made since is not in it. When it matters, ask the person for their current key (bean whoami).",
}
