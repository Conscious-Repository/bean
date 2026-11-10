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
	"github.com/Conscious-Repository/bean/internal/bundle"
	"github.com/Conscious-Repository/bean/internal/nhanes"
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
	shareSystem = "bean-share"
	// bundleReadme is written beside objects/ in every bundle. It holds no
	// value and nothing a checker reads: it tells a person who has only the
	// folder what it is and how to read it.
	bundleReadme = `This folder is a bean bundle: health records that one person chose to share.

It is not a document to open. The records are small signed files under
objects/, and files/ holds any files shared with them. To read it, check it:
a check proves that every record is what the person signed and that nothing
was added or removed, and then lists what the bundle holds.

  With bean (https://github.com/Conscious-Repository/bean):
      bean check-bundle <this folder>

  Without bean, with Python 3:
      python3 tools/second/checkbundle.py <this folder>

  With no install, in a browser: open the page bean-check.html and choose
  this folder, or a zip file of it. The page runs the same check on your
  own device and sends nothing. Get the page from the bean project when it
  matters: a page that came with this folder is only as good as its sender.

Do not read values from this folder before it passes a check, and do not
trust this note: anyone who carried the folder could have changed it. A
check does not read it.

What a check cannot show: that these are all of the person's records, that a
value is true, who the person is, or that the key that signed is still
theirs. Ask the person for their current key when it matters.
`
)

// notShareable says why a record cannot go into a bundle, or "". deleted is
// what the person deleted (deletedByPerson).
func notShareable(l *core.Loaded, deleted map[string]bool, r *core.Record) string {
	switch {
	case staleComparison(l, deleted, r):
		return "the reading it compares was corrected or deleted"
	case r.Type == bundle.ShareType:
		return "it is the record of a share, which is not shared again"
	case bundle.NeverListed(r):
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
	public                              bool   // a reference file the person kept: -files alone shares it
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
	// A comparison whose reading is gone must not leave: clean up first.
	sweepComparisons(s)
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
	deleted := deletedByPerson(l)
	// What the person chose, and which of it they named one by one.
	chosen := map[string]*core.Record{}
	named := map[string]bool{}
	// A type picks readings, never a record that keeps a file: 'lab.*' must
	// not sweep up the lab report itself, with the person's name in it. Such
	// a record is shared when it is named by its id.
	keepsFile := 0
	if *typ != "" {
		for _, r := range recs {
			switch {
			case notShareable(l, deleted, r) != "":
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
		if why := notShareable(l, deleted, r); why != "" {
			fail(fmt.Errorf("%s cannot be shared: %s", r.ID, why))
		}
		chosen[r.ID], named[r.ID] = r, true
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
			case notShareable(l, deleted, t) != "":
				leftOut = append(leftOut, fmt.Sprintf("  not shared: %s, which %s is about: %s", target[7:19], id[7:19], notShareable(l, deleted, t)))
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
	// The files those records keep. With -files a file travels only when the
	// person named its record by id, or when it is a public reference file
	// that the person kept: never because a record came along, or was picked
	// by type. A lab report holds a name and a birth date, and a note about
	// it must not carry it out.
	var kept []sharedFile
	distinct := map[string]bool{}
	seen := map[string]bool{}
	for _, r := range shared {
		public := r.Type == nhanes.FileType && r.Author == l.ID
		travels := *files && (named[r.ID] || public)
		for _, b := range r.Blobs {
			if seen[b.Hash+" "+r.ID] {
				continue // a record may name one file twice
			}
			seen[b.Hash+" "+r.ID] = true
			sf := sharedFile{blob: b.Hash, record: r.ID, mediaType: b.MediaType, recordType: r.Type, public: public}
			if travels {
				data, err := s.GetBlob(b.Hash)
				if err != nil {
					fail(fmt.Errorf("the file of %s cannot be read: %w", r.ID[7:19], err))
				}
				sum := sha256.Sum256(data)
				sf.digest, sf.data = hex.EncodeToString(sum[:]), data
				if stated := bundle.StatedDigest(r, b.Hash); stated != "" && stated != sf.digest {
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
		case bundle.OnChain(l.Report, r, l.ID):
			proof = append(proof, r)
		case bundle.IsPairing(r, l.ID) && authors[r.ValueKey()] && l.Current(id):
			proof = append(proof, r)
			pairings++
		}
	}
	// The person sees what leaves before anything is written, where the
	// question is asked. Without a terminal and without -yes nothing is
	// written: an assistant that runs this through a tool cannot disclose
	// for the person.
	labels := l.AgentLabels()
	// What stays home is said with or without the review.
	for _, sf := range kept {
		switch {
		case sf.data != nil:
		case sf.public:
			leftOut = append(leftOut, fmt.Sprintf("  not shared: the file that %s (%s) keeps. Add -files to share it.", sf.record[7:19], sf.recordType))
		default:
			leftOut = append(leftOut, fmt.Sprintf("  not shared: the file that %s (%s) keeps. With -files, name that record by its id to share its file.", sf.record[7:19], sf.recordType))
		}
	}
	if keepsFile > 0 {
		leftOut = append(leftOut, fmt.Sprintf("  not shared: %d records of that type that keep a file. Name one by its id to share it.", keepsFile))
	}
	if *yes {
		for _, line := range leftOut {
			fmt.Fprintln(os.Stderr, strings.TrimSpace(line))
		}
	}
	if !*yes {
		if !isTerminal(os.Stdin) || !isTerminal(os.Stderr) {
			fail(errors.New("share needs a terminal: run bean share yourself to see what leaves, or pass -yes to write without review"))
		}
		w := os.Stderr
		fmt.Fprintf(w, "To share, into %s:\n", out)
		for _, r := range shared {
			line := fmt.Sprintf("  %s  %s  %s  %s", whenText(r), r.Type, safe(strings.TrimSpace(recordText(r, currentRecord(l))+"  "+byText(r, l.ID, labels))), r.ID[7:19])
			if by := broughtBy[r.ID]; by != "" {
				line += "  (comes along: " + by[7:19] + " is about it)"
			}
			fmt.Fprintln(w, line)
		}
		for _, sf := range kept {
			if sf.data != nil {
				fmt.Fprintf(w, "  THE WHOLE FILE that %s (%s) keeps, as you imported it: %s, %d bytes, in plaintext. A lab report holds your name and birth date.\n", sf.record[7:19], sf.recordType, safe(sf.mediaType), len(sf.data))
			}
		}
		for _, line := range leftOut {
			fmt.Fprintln(w, line)
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
			if now := l.Report.Records[r.ID]; now == nil || notShareable(l, deleted, now) != "" {
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
		if sf.data != nil {
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
		"type":    bundle.ShareType,
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
	// Ctrl-C must not leave a folder of plaintext behind.
	untrap := removeOnSignal(tmp)
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
			if sf.data == nil {
				continue
			}
			if err := os.WriteFile(filepath.Join(tmp, "files", sf.digest), sf.data, 0o600); err != nil {
				gone(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, "README.txt"), []byte(bundleReadme), 0o600); err != nil {
		gone(err)
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
	untrap()
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

// checkBundle checks the bundle in a folder by the five rules of
// docs/SHARE.md, and changes nothing in it. It returns the lines to print
// and whether the bundle passed. The check itself is internal/bundle, which
// the page that checks a bundle in a browser runs too.
func checkBundle(dir string) ([]string, bool) {
	res := bundle.Check(bundle.ReadDir(dir))
	return res.Lines(), res.OK
}
