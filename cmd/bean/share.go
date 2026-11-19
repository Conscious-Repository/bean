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
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"filippo.io/age"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/bundle"
	"github.com/Conscious-Repository/bean/internal/nhanes"
	"github.com/Conscious-Repository/bean/internal/pack"
	"github.com/Conscious-Repository/bean/internal/reply"
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
	public                              bool   // a reference file whose record states a SHA-256 on record: -files alone shares it, once the bytes are held to that SHA-256
	offRecord                           bool   // a reference file whose record states no SHA-256 on record: it goes only by its record's id
	named                               bool   // the person named the record by its id
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
	seal := fs.Bool("seal", false, "write one file that only its reader can open, with six words")
	forFlag := fs.String("for", "", "seal to this address of the reader: one file that the key file of that address opens")
	wordsFlag := fs.String("address-words", "", "with -for: the eight address words that the reader gave you")
	pos, err := parseInterspersed(fs, args)
	if err != nil || *outFlag == "" || (len(pos) == 0 && *typ == "") || (*typ == "" && (*since != "" || *until != "")) {
		usage()
	}
	// A flag that was given counts, also with an empty value: a script whose
	// variable is empty must not write a plain folder in the place of a
	// sealed file.
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	if given["address-words"] && !given["for"] {
		badArg(errors.New("-address-words goes with -for <address>"))
	}
	// -for seals to a reader's address, and implies -seal. The address is
	// read with the age library, and only in the one spelling that the
	// library writes.
	var to *age.X25519Recipient
	if given["for"] {
		if to, err = bundle.ParseAddress(*forFlag); err != nil {
			fail(fmt.Errorf("nothing was written: %w", err))
		}
		*seal = true
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
	if _, err := os.Lstat(out); *seal && err == nil {
		fail(fmt.Errorf("%s is there already: a sealed package is a new file", out))
	}
	if err := bundleTarget(out); err != nil {
		fail(err)
	}
	stdin := bufio.NewReader(os.Stdin)
	// The gate against a wrong address, before anything is written to the
	// log. bean does not show the address words here: the person gives the
	// eight that the reader gave them. -yes never stands in for them.
	if to != nil {
		askAddressWords(to, *wordsFlag, given["address-words"], stdin, shareGate)
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
	c := chooseForShare(s, l, recs, pos, *typ, *files)
	l = reviewShare(s, l, c, out, *files, *seal, to != nil, *yes, *note, stdin)
	shared, kept, proof, distinct := c.shared, c.kept, c.proof, c.distinct
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
	// The person's own record of how the bundle left. It does not hold the
	// words.
	switch {
	case to != nil:
		// It names no address, and no hash of one.
		ctx["seal"] = bundle.SealRecipient
	case *seal:
		ctx["seal"] = bundle.SealPassphrase
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
	// From here a signal does not end bean by itself. Before the move it is
	// obeyed between two writes, never during one: a remover that ran beside
	// the writer left a folder of plaintext behind. From the move on it
	// waits: a bundle in place with no share record in the log is a
	// disclosure the log does not know.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, importSignals...)
	defer signal.Stop(sig)
	// The bundle, in memory: every record file under its path, each shared
	// file under its SHA-256, and the note for a person who has only the
	// folder.
	bundleFiles := map[string][]byte{core.PathOf(shareID): shareBytes, "README.txt": []byte(bundleReadme)}
	for _, r := range append(shared, proof...) {
		bundleFiles[core.PathOf(r.ID)] = r.Bytes
	}
	for _, sf := range kept {
		if sf.data != nil {
			bundleFiles["files/"+sf.digest] = sf.data
		}
	}
	tmp := ""
	stopped := func() {
		select {
		case got := <-sig:
			if tmp != "" {
				os.RemoveAll(tmp)
			}
			fmt.Fprintf(os.Stderr, "bean: stopped by %s; nothing was shared, and nothing of it is left\n", got)
			code := 1
			if n, ok := got.(syscall.Signal); ok {
				code = 128 + int(n)
			}
			os.Exit(code)
		default:
		}
	}
	words := ""
	if *seal {
		// A sealed package never touches the disk in plaintext: it is
		// checked, packed and encrypted in memory.
		res := bundle.Check(bundle.FromFiles(out, bundleFiles))
		if !res.OK {
			for _, line := range res.Lines() {
				fmt.Fprintln(os.Stderr, safe(line))
			}
			fail(errors.New("nothing was shared: the bundle does not pass bean check-bundle"))
		}
	}
	if to != nil {
		// Sealed to an address, and opened once more with the file's own
		// key. Such a package can be opened the moment it is written. So the
		// share record goes into the log before the first byte goes into the
		// file, and the log never knows less than what left.
		data, err := sealPackageTo(out, bundleFiles, to, func(in bundle.Input) bool { return bundle.Check(in).OK })
		if err != nil {
			fail(fmt.Errorf("nothing was shared: %w", err))
		}
		stopped() // the last moment: after this the share runs to its end
		if err := writeForKey(out, data, shareWritten, shareID, func() error { return s.AddSigned(shareID, shareBytes) }, fillFile); err != nil {
			fail(err)
		}
	} else if *seal {
		// Sealed with six words, opened and checked once more as its reader
		// will, and only then written, as a new file.
		data, w, err := sealPackage(out, bundleFiles, func(in bundle.Input) bool { return bundle.Check(in).OK })
		if err != nil {
			fail(fmt.Errorf("nothing was shared: %w", err))
		}
		words = w
		stopped() // the last moment: after this the share runs to its end
		if err := writeNew(out, data); err != nil {
			fail(fmt.Errorf("nothing was shared: %w", err))
		}
	} else {
		// A plain bundle is written next to its place, checked as its
		// reader will check it, and moved there whole.
		var err error
		if tmp, err = os.MkdirTemp(filepath.Dir(out), ".bean-share-*"); err != nil {
			fail(err)
		}
		gone := func(err error) {
			os.RemoveAll(tmp)
			fail(fmt.Errorf("nothing was shared: %w", err))
		}
		for _, name := range slices.Sorted(maps.Keys(bundleFiles)) {
			stopped()
			p := filepath.Join(tmp, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
				gone(err)
			}
			if err := os.WriteFile(p, bundleFiles[name], 0o600); err != nil {
				gone(err)
			}
		}
		if lines, ok := checkBundle(tmp); !ok {
			for _, line := range lines {
				fmt.Fprintln(os.Stderr, safe(line))
			}
			gone(errors.New("the bundle does not pass bean check-bundle"))
		}
		stopped() // the last moment: after this the share runs to its end
		if err := os.Remove(out); err != nil && !errors.Is(err, os.ErrNotExist) {
			gone(err)
		}
		if err := os.Rename(tmp, out); err != nil {
			gone(err)
		}
	}
	// For a package that is sealed to an address, the record is in the log
	// already.
	if to == nil {
		if err := s.AddSigned(shareID, shareBytes); err != nil {
			if *seal {
				// Its words were never shown, so nobody can open it.
				os.Remove(out)
				fail(fmt.Errorf("nothing was shared: the share record could not be added to your log, and the package is removed: %w", err))
			}
			fail(fmt.Errorf("the bundle is written, in %s, and its share record could not be added to your log: %w", out, err))
		}
	}
	fmt.Printf("shared %d records and %d files in %s\n", len(shared), len(distinct), out)
	fmt.Printf("share record %s\n", shareID)
	who := "\nYour identity words, which are not a secret:\n  " + core.IDWords(l.ID) + "\nThe reader's check shows the identity words of whoever signed. Read yours to the reader, so that they know the records are yours.\nThe check also shows this share's time and the start of its id, " + shareID[7:19] + ": tell the reader, so that they know which share this is."
	if to != nil {
		fmt.Println("The package is sealed to the address that you gave. Whoever holds the key file of that address can open it.")
		fmt.Println("You cannot open this file again, unless the address is your own. To keep a copy, do not delete those records, or keep a bean export.")
		fmt.Println(who)
		fmt.Println("The reader opens it with bean check-bundle -key <their key file>, or with the age tool. A bean that has no bean address does not open it.")
		return
	}
	if *seal {
		fmt.Println("The package is sealed. It opens with these six words:")
		fmt.Println("  " + words)
		fmt.Println("They are a secret between you and the reader. Give them by another way than the file, such as a phone call. bean does not keep them: if they are lost, share again.")
		fmt.Println(who)
		fmt.Println("The reader opens it with bean check-bundle, or in a browser with bean-check.html.")
		return
	}
	fmt.Println(who)
	fmt.Println("The bundle is not encrypted: anyone who gets the folder can read it. Share with -seal to write one file that only its reader can open.")
}

// shareChoice is what a share holds, decided before anything is asked or
// written.
type shareChoice struct {
	deleted   map[string]bool         // ids that the person deleted
	chosen    map[string]*core.Record // what goes, by id
	broughtBy map[string]string       // a record that came along -> the one that is about it
	leftOut   []string                // why a record that another is about stays
	shared    []*core.Record          // chosen, in the order of a listing
	kept      []sharedFile            // the files those records keep
	distinct  map[string]bool         // the files that travel, by SHA-256
	proof     []*core.Record          // identity records and pairings
	pairings  int                     // how many of proof are pairings
	keepsFile int                     // records that a type did not pick: they keep a file
}

// chooseForShare decides what a share holds: the records that the person
// named or that a type picks, the record that each is about, the files
// that travel, and the proof of who signed. It writes nothing. A choice
// that cannot be shared ends the command.
func chooseForShare(s *core.Store, l *core.Loaded, recs []*core.Record, pos []string, typ string, files bool) shareChoice {
	deleted := deletedByPerson(l)
	// What the person chose, and which of it they named one by one.
	chosen := map[string]*core.Record{}
	named := map[string]bool{}
	// A type picks readings, never a record that keeps a file: 'lab.*' must
	// not sweep up the lab report itself, with the person's name in it. Such
	// a record is shared when it is named by its id.
	// Nor does a type pick a record that someone else signed: a peer's
	// note, or a record of a key that the person never paired, which a sync
	// tool can put into the folder. The person's own record of the share
	// vouches for the list, so such a record goes when they name it.
	keepsFile, byOthers := 0, 0
	mine := l.FileLabels() // the person's agents
	if typ != "" {
		for _, r := range recs {
			switch {
			case notShareable(l, deleted, r) != "":
			case len(r.Blobs) > 0:
				keepsFile++
			case r.Author != l.ID && mine[r.Author] == "":
				byOthers++
			default:
				chosen[r.ID] = r
			}
		}
		if byOthers > 0 {
			fmt.Fprintf(os.Stderr, "%d records of type %s that someone else signed stay home: share such a record by its id\n", byOthers, typ)
		}
		if len(chosen) == 0 {
			fail(fmt.Errorf("no current record of type %s in that time (a record that keeps a file, or that someone else signed, is shared by its id, not by type)", typ))
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
	//
	// A reference file is public when bean has its SHA-256 on record: anyone
	// can then get the same bytes from their publisher. The type of the
	// record says nothing, since a record of any type can name any file
	// (DECISIONS 322). The record states the SHA-256, and a file that
	// travels is held to it below.
	var kept []sharedFile
	distinct := map[string]bool{}
	seen := map[string]bool{}
	// Only a record of the person or of a paired agent opens a file, as in
	// an export. Anyone can write a record that names a file of the log,
	// also a file whose record the person deleted. In a log with records of
	// a newer bean this list is empty: an agent's file then stays home
	// here, and an export still carries it.
	opens := mine
	for _, r := range shared {
		reference := r.Type == nhanes.FileType && r.Author == l.ID
		public := reference && len(r.Blobs) == 1 && nhanes.OnRecord(bundle.StatedDigest(r, r.Blobs[0].Hash))
		travels := files && (named[r.ID] || public)
		if travels && len(r.Blobs) > 0 && r.Author != l.ID && opens[r.Author] == "" {
			fail(fmt.Errorf("%s names a file, and it is not your record or a paired agent's: such a record opens no file, so nothing was shared. Share it without -files, or delete it: bean purge %s", r.ID[7:19], r.ID[7:19]))
		}
		for _, b := range r.Blobs {
			if seen[b.Hash+" "+r.ID] {
				continue // a record may name one file twice
			}
			seen[b.Hash+" "+r.ID] = true
			sf := sharedFile{blob: b.Hash, record: r.ID, mediaType: b.MediaType, recordType: r.Type, public: public, offRecord: reference && !public, named: named[r.ID]}
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
	return shareChoice{deleted, chosen, broughtBy, leftOut, shared, kept, distinct, proof, pairings, keepsFile}
}

// reviewShare says what stays home, shows the person what leaves, and asks.
// Without a terminal and without -yes it ends the command. It returns the
// log as it is after the question: it may have changed while the person
// decided, and then nothing is shared.
func reviewShare(s *core.Store, l *core.Loaded, c shareChoice, out string, files, seal, forKey, yes bool, note string, stdin *bufio.Reader) *core.Loaded {
	var err error
	deleted, chosen, broughtBy, leftOut := c.deleted, c.chosen, c.broughtBy, c.leftOut
	shared, kept, proof, pairings, keepsFile := c.shared, c.kept, c.proof, c.pairings, c.keepsFile
	// The person sees what leaves before anything is written, where the
	// question is asked. Without a terminal and without -yes nothing is
	// written: an assistant that runs this through a tool cannot disclose
	// for the person.
	labels := l.PairedLabels()
	// What stays home is said with or without the review.
	publicHome, travels := 0, 0
	for _, sf := range kept {
		switch {
		case sf.data != nil:
			travels++
		case sf.public:
			publicHome++
		case sf.offRecord:
			// Only what bean knows: what the record states. Nothing was read.
			how := "Run the same command again with -files, and add this id after the other arguments: " + sf.record[7:19] + "."
			switch {
			case sf.named:
				how = "You named it: run the same command again with -files."
			case files:
				how = "Run the same command again, and add this id after the other arguments: " + sf.record[7:19] + "."
			}
			leftOut = append(leftOut, fmt.Sprintf("  not shared: the file that %s (%s) keeps. Its record does not state the SHA-256 of a file that bean has on record, so the file goes only with -files and when you name that record. %s Without the file, a reader cannot compute again a comparison that read it.", sf.record[7:19], sf.recordType, how))
		default:
			leftOut = append(leftOut, fmt.Sprintf("  not shared: the file that %s (%s) keeps. With -files, name that record by its id to share its file.", sf.record[7:19], sf.recordType))
		}
	}
	// The public files are said once, with what their absence means. No
	// file was read, so this says what the records state and no more.
	if publicHome > 0 {
		which := fmt.Sprintf("the files that %d reference.file records keep. Each record states the SHA-256 of a file that bean has on record", publicHome)
		if publicHome == 1 {
			which = "the file that 1 reference.file record keeps. The record states the SHA-256 of a file that bean has on record"
		}
		leftOut = append(leftOut, fmt.Sprintf("  not shared: %s. Add -files to share them: bean then holds the bytes to that SHA-256. Without them a reader needs the files from their publisher to compute a comparison again.", which))
	}
	// A comparison with a reference pack names the pack and does not hold
	// it: the reader needs that file to compute the comparison again.
	var packs []string
	for _, id := range slices.Sorted(maps.Keys(chosen)) {
		ref, _ := chosen[id].Context["reference"].(map[string]any)
		if digest, _ := ref["pack"].(string); pack.IsDigest(digest) && strings.HasPrefix(chosen[id].Type, "comparison.") && !slices.Contains(packs, digest) {
			packs = append(packs, digest)
		}
	}
	for _, digest := range packs {
		leftOut = append(leftOut, fmt.Sprintf("  not shared: the reference pack %s. A comparison names it, and a reader needs that file to compute the comparison again: send it too, or say where it is.", digest))
	}
	if files && travels == 0 {
		leftOut = append(leftOut, "  -files: no record that you named keeps a file, so no file goes")
	}
	if keepsFile > 0 {
		leftOut = append(leftOut, fmt.Sprintf("  not shared: %d records of that type that keep a file. Name one by its id to share it.", keepsFile))
	}
	if yes {
		for _, line := range leftOut {
			fmt.Fprintln(os.Stderr, strings.TrimSpace(line))
		}
	}
	if !yes {
		if !isTerminal(os.Stdin) || !isTerminal(os.Stderr) {
			fail(errors.New("share needs a terminal: run bean share yourself to see what leaves, or pass -yes to write without review"))
		}
		w := os.Stderr
		fmt.Fprintf(w, "To share, into %s:\n", out)
		// What the person chose comes first, then what comes along with it.
		row := func(r *core.Record) string {
			return fmt.Sprintf("  %s  %s  %s  %s", whenText(r), r.Type, safe(strings.TrimSpace(recordText(r, currentRecord(l))+"  "+byText(r, l.ID, labels))), r.ID[7:19])
		}
		for _, r := range shared {
			if broughtBy[r.ID] == "" {
				fmt.Fprintln(w, row(r))
			}
		}
		for _, r := range shared {
			if by := broughtBy[r.ID]; by != "" {
				fmt.Fprintln(w, row(r)+"  (comes along: "+by[7:19]+" is about it)")
			}
		}
		listed, whose := slices.Clone(shared), "these records"
		for _, r := range proof {
			if r.Type == "agent.pair" {
				listed, whose = append(listed, r), "these records and their pairings"
			}
		}
		if lines := fieldLines(listed); len(lines) > 0 {
			fmt.Fprintf(w, "Beside what each row shows, %s hold these fields. The reader gets every value, also one that a line below does not print:\n", whose)
			for _, line := range lines {
				fmt.Fprintln(w, line)
			}
		}
		if note != "" {
			fmt.Fprintf(w, "  with your note for the reader: %s\n", safe(note))
		}
		for _, sf := range kept {
			switch {
			case sf.data == nil:
			case sf.public:
				fmt.Fprintf(w, "  the public file that %s (%s) keeps: %d bytes. Its SHA-256 is one that bean has on record for a file of the survey.\n", sf.record[7:19], sf.recordType, len(sf.data))
			case sf.offRecord:
				fmt.Fprintf(w, "  THE WHOLE FILE that %s (%s) keeps, which you named: %s, %d bytes, in plaintext. Its record does not state a file that bean has on record, so bean cannot say what it holds.\n", sf.record[7:19], sf.recordType, safe(sf.mediaType), len(sf.data))
			default:
				fmt.Fprintf(w, "  THE WHOLE FILE that %s (%s) keeps, as you imported it: %s, %d bytes, in plaintext. A lab report holds your name and birth date.\n", sf.record[7:19], sf.recordType, safe(sf.mediaType), len(sf.data))
			}
		}
		for _, line := range leftOut {
			fmt.Fprintln(w, line)
		}
		fmt.Fprintf(w, "  also: your %d identity records and %d pairings, and the share record itself\n", len(proof)-pairings, pairings)
		fmt.Fprintln(w, "Each record also carries the time when it was entered, your key and the version of bean. Your identity records show when you made your log, when you changed a key, and how many records you had at each change.")
		if seal {
			fmt.Fprintln(w, "A sealed package is a disclosure: it is encrypted on its way, and once its reader has opened it, it cannot be taken back. Each record carries your identity id, so two packages of yours can be linked.")
		} else {
			fmt.Fprintln(w, "A bundle is a disclosure: it is not encrypted, and once sent it cannot be taken back. Each record carries your identity id, so two bundles of yours can be linked. With -seal it is one encrypted file.")
		}
		if forKey {
			for _, line := range forKeyLines {
				fmt.Fprintln(w, line)
			}
		}
		fmt.Fprintln(w, "To leave a record out, answer n. Then share again with only the records that you want: name them by their ids, or narrow -type with -since and -until.")
		fmt.Fprintf(w, "Share %d records? [y/N] ", len(shared))
		if !confirmed(stdin) {
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
	return l
}

// forKeyLines are what the review says of a package that is sealed to an
// address, before it asks.
var forKeyLines = []string{
	"The package is sealed to an address. Whoever holds the key file of that address can open it.",
	"bean checked only that the address words belong to the address.",
	"A reader with only a browser, or with an earlier bean, cannot open it. Such a reader needs a package that is sealed with words.",
	"A program that runs as the reader opens it with no word typed.",
	"It stays closed only while the key file stays with the reader and the cipher holds. That is so for as long as any copy of the file exists.",
	"You cannot open this file again, unless the address is your own. To keep a copy, do not delete those records, or keep a bean export.",
	"Your share record names no address. A -note can say who the package is for. The note is signed, and travels to the reader and to whoever gets the bundle later.",
}

// addressGate reports whether words are the address words of the address.
// It catches a wrong or swapped address only when the words came by another
// channel than the address.
func addressGate(to *age.X25519Recipient, words string) bool {
	return bundle.CleanAddressWords(words) == bundle.AddressWords(to.String())
}

// gateWords are the words of the gate's lines: the command, and whom the
// address words came from.
type gateWords struct {
	cmd, whom, whose, Whom string
}

var (
	shareGate = gateWords{"bean share -for", "the reader", "the reader's", "The reader"}
	replyGate = gateWords{"bean reply -for", "the person you reply to", "that person's", "That person"}
)

// askAddressWords is the gate against a wrong address: the person gives the
// eight address words that they got with the address, by -address-words
// (given) or on a terminal. bean does not show the words of the address.
// When the words are missing or not those of the address, it ends the
// command, and nothing was written.
func askAddressWords(to *age.X25519Recipient, words string, given bool, stdin *bufio.Reader, g gateWords) {
	if !given {
		if !isTerminal(os.Stdin) || !isTerminal(os.Stderr) {
			fail(fmt.Errorf("nothing was written: %s needs the eight address words of %s address, and there is no terminal to ask on. Pass them with -address-words. %s gets them from bean address -key <file>", g.cmd, g.whose, g.Whom))
		}
		fmt.Fprintf(os.Stderr, "Type the eight address words that %s gave you, then Enter:\n", g.whom)
		words, _ = stdin.ReadString('\n')
	}
	if bundle.CleanAddressWords(words) == "" {
		fail(fmt.Errorf("nothing was written: no address words were given. %s needs the eight address words of %s address. %s gets them from bean address -key <file>", g.cmd, g.whose, g.Whom))
	}
	if !addressGate(to, words) {
		fail(fmt.Errorf("nothing was written: these are not the address words of this address. If you mistyped a word, run the command again. If not, ask %s for the address and its words again", g.whom))
	}
}

// sealPackageTo packs files and seals them to a reader's address. bean
// opens the result once more with the file's own key: passes says whether
// what came out is what was meant. It does not open the one stanza that is
// for the reader.
func sealPackageTo(name string, files map[string][]byte, to *age.X25519Recipient, passes func(bundle.Input) bool) ([]byte, error) {
	packed, err := bundle.PackFiles(files)
	if err != nil {
		return nil, err
	}
	data, back, err := bundle.SealTo(name, packed, to)
	if err != nil {
		return nil, err
	}
	if !passes(back) {
		return nil, errors.New("the sealed package does not open and pass the check")
	}
	return data, nil
}

// written names what writeForKey writes, in the lines that it says: a
// share or a reply.
type written struct {
	none   string // what nothing was: "nothing was shared"
	record string // the record of the log: "share record"
	file   string // the file: "the package"
}

var (
	shareWritten = written{"nothing was shared", "share record", "the package"}
	replyWritten = written{"nothing was sent", "reply record", "the reply"}
)

// writeForKey writes a package that is sealed to a key, in this order: the
// file, new and empty; the record of the log, with record; the bytes, with
// fill. If the file cannot be made, nothing was written. If the record
// fails, the empty file is removed. After that the log knows of the
// package, whatever becomes of the file. It says only what it did: a part
// of such a file can open, so a file that was not written whole is not
// "nothing".
func writeForKey(path string, data []byte, w written, id string, record func() error, fill func(*os.File, []byte) error) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("%s: %w", w.none, err)
	}
	if err := record(); err != nil {
		f.Close()
		if os.Remove(path) != nil {
			return fmt.Errorf("%s: the %s could not be added to your log: %w. The empty file %s could not be removed: delete it", w.none, w.record, err, path)
		}
		return fmt.Errorf("%s: the %s could not be added to your log, and the empty file is removed: %w", w.none, w.record, err)
	}
	err = fill(f, data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		return nil
	}
	what := fmt.Sprintf("%s was not written whole into %s: %s. Your log holds its %s, %s. A part of such a file can open with the reader's key", w.file, path, err, w.record, id)
	if os.Remove(path) != nil {
		return fmt.Errorf("%s. bean could not remove the file: delete it", what)
	}
	return fmt.Errorf("%s. bean removed the file. If its place does not sync, no part of it left: then bean purge %s deletes the record", what, id[7:19])
}

// fillFile writes data into a file and syncs it.
func fillFile(f *os.File, data []byte) error {
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}

// sealPackage packs files and seals them with new words. It opens the
// result once more with those words, as its reader will: passes says
// whether what came out is what was meant. It returns the sealed bytes and
// the words. A share and a reply are sealed the same way.
func sealPackage(name string, files map[string][]byte, passes func(bundle.Input) bool) ([]byte, string, error) {
	packed, err := bundle.PackFiles(files)
	if err != nil {
		return nil, "", err
	}
	words, err := bundle.NewWords()
	if err != nil {
		return nil, "", err
	}
	data, err := bundle.Seal(packed, words)
	if err != nil {
		return nil, "", err
	}
	if !passes(bundle.FromPackage(name, data, words)) {
		return nil, "", errors.New("the sealed package does not open and pass the check")
	}
	return data, words, nil
}

// writeNew writes a file that is made new, or not at all: a file that is
// at this path already is not replaced, and a file that cannot be written
// whole is removed.
func writeNew(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}

// insideLog reports a path that lies in the log's folder: nothing that is
// sent to another person is written there, and no key file. The path is
// judged as text, and then by what is on the disk: a path that reaches the
// folder through a link lies in it too, and so does one that is written in
// other capitals on a disk that does not tell them apart.
func insideLog(out string) bool {
	rel, err := filepath.Rel(logDir, out)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return true
	}
	log, err := os.Stat(logDir)
	if err != nil {
		return false
	}
	// The file itself may not be there yet: its folder is. With the links
	// of that folder followed, the log's folder is the folder itself or
	// one of the folders above it, as the disk names them.
	dir := filepath.Dir(out)
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	for {
		if info, err := os.Stat(dir); err == nil && os.SameFile(info, log) {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// bundleTarget refuses a place a bundle must not go: inside the log, onto
// a file, or into a folder that holds anything.
func bundleTarget(out string) error {
	if insideLog(out) {
		return fmt.Errorf("%s is inside your log: a bundle goes somewhere else", out)
	}
	if info, err := os.Lstat(out); err == nil && !info.IsDir() {
		return fmt.Errorf("%s is a file: a bundle goes into a new or empty folder", out)
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
	fs := flag.NewFlagSet("check-bundle", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	unpack := fs.String("unpack", "", "for a package that passes: write its bundle into this new folder")
	keyFile := fs.String("key", "", "for a package that is sealed to a key: the key file that opens it")
	if err := fs.Parse(args); err != nil || fs.NArg() != 1 {
		usage()
	}
	keyGiven := false
	fs.Visit(func(f *flag.Flag) { keyGiven = keyGiven || f.Name == "key" })
	if keyGiven && *keyFile == "" {
		badArg(errors.New("-key names a key file"))
	}
	path := fs.Arg(0)
	var lines []string
	var ok bool
	var in bundle.Input
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		lines, ok, in = checkPackage(path, *keyFile)
	} else if *unpack != "" {
		fail(errors.New("-unpack is for a package that is one file: a folder is unpacked already"))
	} else {
		// Said only of a folder that is there: bean claims what it checked.
		if info, err := os.Stat(path); *keyFile != "" && err == nil && info.IsDir() {
			fmt.Fprintln(os.Stderr, notSealedLine)
		}
		in = bundle.ReadDir(path)
		res := bundle.Check(in)
		lines, ok = res.Lines(), res.OK
	}
	for _, line := range lines {
		fmt.Println(safe(line))
	}
	if !ok {
		// A reply is not a bundle, and its reader has another command.
		if reply.Check(in).OK {
			how := "bean accept"
			if in.Asks == bundle.AsksKey {
				how = "bean accept -key <their key file>"
			}
			fmt.Println("This is a reply to a share, not a bundle: the person it is for reads it with " + how + ".")
		}
		os.Exit(1)
	}
	if *unpack != "" {
		if err := unpackBundle(in, *unpack); err != nil {
			fail(err)
		}
		if len(in.Files) > 0 {
			fmt.Printf("unpacked into %s: its files are in %s\n", *unpack, filepath.Join(*unpack, "files"))
		} else {
			fmt.Printf("unpacked into %s: it carries no file\n", *unpack)
		}
	}
}

// unpackBundle writes a bundle that passed its check into a new folder: its
// records, and the files it carries, each read and hashed once more. So a
// reader gets a carried file out of a sealed package, and the folder checks
// again as any bundle does.
func unpackBundle(in bundle.Input, dir string) error {
	if err := os.Mkdir(dir, 0o700); err != nil {
		return fmt.Errorf("nothing was unpacked: %w", err)
	}
	write := func(rel string, data []byte) error {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
		return os.WriteFile(p, data, 0o600)
	}
	for rel, data := range in.Records {
		if err := write(rel, data); err != nil {
			return err
		}
	}
	for _, f := range in.Files {
		data, err := f.Read()
		if sum := sha256.Sum256(data); err != nil || hex.EncodeToString(sum[:]) != f.Name {
			return fmt.Errorf("the file %s could not be read again as it was checked", f.Name)
		}
		if err := write("files/"+f.Name, data); err != nil {
			return err
		}
	}
	return nil
}

// checkPackage checks a bundle that is one file: a zip file of the folder,
// or a sealed package. For one that is sealed with words it asks for the
// six words, and reads them from standard input. One that is sealed to a
// key opens with keyFile.
func checkPackage(path, keyFile string) ([]string, bool, bundle.Input) {
	in := readPackage(path, bufio.NewReader(os.Stdin), keyFile)
	res := bundle.Check(in)
	return res.Lines(), res.OK, in
}

// checkBundle checks the bundle in a folder by the five rules of
// docs/SHARE.md, and changes nothing in it. It returns the lines to print
// and whether the bundle passed. The check itself is internal/bundle, which
// the page that checks a bundle in a browser runs too.
func checkBundle(dir string) ([]string, bool) {
	res := bundle.Check(bundle.ReadDir(dir))
	return res.Lines(), res.OK
}
