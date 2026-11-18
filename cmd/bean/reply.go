package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/bundle"
	"github.com/Conscious-Repository/bean/internal/reply"
)

// bean reply and bean accept (docs/SHARE.md, "A signed reply").
//
// A person shares records with a peer. The peer signs notes about those
// records with their own key and sends them back, sealed. The person
// checks who signed, and takes the notes into their log under the name
// they give that peer.
//
// A note is the peer's key speaking by itself about the person: text about
// one record of the share, and nothing more. It corrects nothing and
// deletes nothing. The reply also holds the peer's identity records and one
// reply record, signed by the peer as themselves. They show that the key is
// the current key of an identity, which the person confirms by its eight
// identity words. Only the notes go into the person's log.

// notSealedLine and withWordsLine say that a key file was named and is not
// used. The key file is not read then.
const (
	notSealedLine = "It is not sealed. -key is not used."
	withWordsLine = "It is sealed with words. -key is not used: give the six words."
)

// readPackage reads a bundle or a reply from a folder or a file. One scan
// of the file's header decides what it asks for. For a package that is
// sealed with words it asks for the six words on in. A package that is
// sealed to a key opens with the key file that keyFile names, or "".
func readPackage(path string, in *bufio.Reader, keyFile string) bundle.Input {
	info, err := os.Stat(path)
	if err != nil {
		return bundle.Input{Fatal: []string{fmt.Sprintf("%s cannot be read: %s", path, err)}}
	}
	if info.IsDir() {
		if keyFile != "" {
			fmt.Fprintln(os.Stderr, notSealedLine)
		}
		return bundle.ReadDir(path)
	}
	if info.Size() > 2<<30 {
		return bundle.Input{Fatal: []string{fmt.Sprintf("%s is too large to be a package", path)}}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return bundle.Input{Fatal: []string{fmt.Sprintf("%s cannot be read: %s", path, err)}}
	}
	words := ""
	switch asks, _ := bundle.Asks(data); {
	case asks == bundle.AsksKey && keyFile != "":
		// The one key of that file is tried, and no other.
		key, err := readKey(keyFile)
		if err != nil {
			return bundle.Input{Fatal: []string{fmt.Sprintf("%s was not opened: %s", path, err)}, Locked: true, Asks: asks}
		}
		return bundle.FromPackageKey(path, data, key)
	case asks == bundle.AsksWords:
		if keyFile != "" {
			fmt.Fprintln(os.Stderr, withWordsLine)
		}
		fmt.Fprintln(os.Stderr, "Type the six words of this package, then Enter:")
		words, _ = in.ReadString('\n')
	case asks == "" && keyFile != "":
		fmt.Fprintln(os.Stderr, notSealedLine)
	}
	return bundle.FromPackage(path, data, words)
}

// cmdReply is bean reply: the peer's side.
func cmdReply(s *core.Store, args []string) {
	fs := flag.NewFlagSet("reply", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	to := fs.String("to", "", "what you received: a sealed package, or a bundle's folder or zip file")
	out := fs.String("out", "", "the reply to write: a new file")
	keyFile := fs.String("key", "", "when what you received is sealed to a key: the key file that opens it")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "A note that starts with - is read as an option. Put -- before the first id.")
	}
	if err != nil || *to == "" || *out == "" || len(pos) == 0 || len(pos)%2 != 0 {
		usage()
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "key" && *keyFile == "" {
			badArg(errors.New("-key names a key file"))
		}
	})
	if abs, err := filepath.Abs(*out); err == nil && insideLog(abs) {
		fail(fmt.Errorf("%s is inside your log: a reply goes somewhere else", *out))
	}
	if _, err := os.Lstat(*out); err == nil {
		fail(fmt.Errorf("%s is there already: a reply is a new file", *out))
	}
	stdin := bufio.NewReader(os.Stdin)
	// What was received must pass its check: a note is about a record of a
	// share that is what its sender signed.
	got := bundle.Check(readPackage(*to, stdin, *keyFile))
	if !got.OK {
		for _, line := range got.Lines() {
			fmt.Fprintln(os.Stderr, safe(line))
		}
		if got.Locked {
			fail(errors.New("nothing was written: what you reply to did not open"))
		}
		fail(errors.New("nothing was written: what you reply to does not pass bean check-bundle"))
	}
	// What a key shows, and what it does not, before a note is signed: a
	// package that someone sealed again reads like any other.
	for _, line := range got.ByKey {
		fmt.Fprintln(os.Stderr, safe(line))
	}
	l, err := s.Load()
	if err != nil {
		fail(err)
	}
	if l.ID == "" || l.Local == nil {
		fail(errors.New("a reply is signed with your own key: make a log first with bean init, on the machine that holds your key"))
	}
	if warnFrozen(l) {
		os.Exit(4)
	}
	if l.ID == got.Identity {
		fail(errors.New("this share is your own: a reply is for another person's share"))
	}
	if len(pos)/2 > reply.MaxNotes {
		fail(fmt.Errorf("a reply holds at most %d notes", reply.MaxNotes))
	}
	// Each pair is a record of the share, by its id or the start of it,
	// and the text of a note about it. The share itself can be named too.
	about := append(slices.Clone(got.Listed), got.Share)
	key := l.Local.Signer.Public()
	now := s.Now().UTC()
	files := map[string][]byte{}
	var noteIDs []any
	for i := 0; i < len(pos); i += 2 {
		target, err := oneOf(about, pos[i])
		if err != nil {
			badArg(err)
		}
		text := pos[i+1]
		if strings.TrimSpace(text) == "" || len(text) > reply.MaxNote {
			badArg(fmt.Errorf("a note is a text of 1 to %d bytes", reply.MaxNote))
		}
		id, data, err := core.SignRecord(map[string]any{
			"v": 0.0, "nonce": core.NewNonce(), "type": reply.NoteType,
			"subject": got.Identity, "author": key, "kid": key,
			"value":  map[string]any{"s": text},
			"times":  map[string]any{"observed": map[string]any{"start": now.Truncate(time.Minute).Format(time.RFC3339), "precision": "minute"}, "imported": now.Format("2006-01-02T15:04:05Z")},
			"source": map[string]any{"system": reply.System, "adapter": core.Adapter, "ref": nil},
			"refs":   []any{map[string]any{"rel": "about", "id": target}},
		}, l.Local.Signer)
		if err != nil {
			fail(err)
		}
		files[core.PathOf(id)] = data
		noteIDs = append(noteIDs, id)
	}
	// The reply record: your own record of what you sent, and to whom. It
	// lists the notes and does not hold their text.
	line := core.Canonical(map[string]any{
		"type":    reply.Type,
		"value":   map[string]any{"n": float64(len(noteIDs)), "unit": "{note}"},
		"times":   map[string]any{"observed": map[string]any{"start": now.Truncate(time.Minute).Format(time.RFC3339), "precision": "minute"}},
		"source":  map[string]any{"system": reply.System, "adapter": core.Adapter, "ref": nil},
		"context": map[string]any{"to": got.Identity, "share": got.Share, "notes": noteIDs, "seal": "passphrase"},
	})
	replyID, replyBytes, err := s.SignPartial(line)
	if err != nil {
		fail(err)
	}
	files[core.PathOf(replyID)] = replyBytes
	// Your identity records: they show that your key speaks for you.
	for id, r := range l.Report.Records {
		if bundle.OnChain(l.Report, r, l.ID) {
			files[core.PathOf(id)] = r.Bytes
		}
	}
	// Checked, packed and sealed in memory, opened and checked once more as
	// its reader will, and only then written, as a new file.
	if res := reply.Check(bundle.FromFiles(*out, files)); !res.OK {
		for _, p := range res.Problems {
			fmt.Fprintln(os.Stderr, "problem: "+safe(p))
		}
		fail(errors.New("nothing was written: the reply does not pass its own check"))
	}
	data, words, err := sealPackage(*out, files, func(in bundle.Input) bool { return reply.Check(in).OK })
	if err == nil {
		err = writeNew(*out, data)
	}
	if err != nil {
		fail(fmt.Errorf("nothing was written: %w", err))
	}
	if err := s.AddSigned(replyID, replyBytes); err != nil {
		// Its words were never shown, so nobody can open it.
		os.Remove(*out)
		fail(fmt.Errorf("nothing was sent: the reply record could not be added to your log, and the reply is removed: %w", err))
	}
	fmt.Printf("wrote %d notes in %s\n", len(noteIDs), *out)
	fmt.Printf("reply record %s\n", replyID)
	fmt.Println("The reply is sealed. It opens with these six words:")
	fmt.Println("  " + words)
	fmt.Println("They are a secret between you and the person you reply to. Give them by another way than the file, such as a phone call.")
	fmt.Println("\nYour identity words, which are not a secret:\n  " + core.IDWords(l.ID))
	fmt.Println("The person sees these words before they accept your notes. Read yours to them, so that they know the notes are yours.")
	fmt.Printf("They also see the start of this reply's id, %s: tell them, so that they know which reply this is.\n", replyID[7:19])
	fmt.Println("The reply holds your identity records. They show your identity id and your key, when you made your log, and when you changed a key.")
	fmt.Println("bean keeps a record that you replied, and not the notes: keep this file and its words if you want to read them again.")
	fmt.Println("That record names the person's identity id, their share and your notes. It travels when you share all of your records.")
	fmt.Println("The person takes your notes into their log with: bean accept " + *out)
}

// maxReply is the largest file that bean accept reads.
const maxReply = 64 << 20

// oneOf finds the one id in ids that is given in full or by the first 6
// or more hex digits.
func oneOf(ids []string, given string) (string, error) {
	prefix := strings.TrimPrefix(given, "sha256:")
	if len(prefix) < 6 || strings.Trim(prefix, "0123456789abcdef") != "" {
		return "", fmt.Errorf("%q is not an id: give a record's id from the check, or its first 6 or more hex digits", given)
	}
	var found []string
	for _, id := range ids {
		if strings.HasPrefix(id[7:], prefix) && !slices.Contains(found, id) {
			found = append(found, id)
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("%s is not a record of this share: a note is about a record that was shared with you", given)
	case 1:
		return found[0], nil
	}
	return "", fmt.Errorf("%s is the start of %d ids of this share: give more digits", given, len(found))
}

// cmdAccept is bean accept: the side of the person who shared.
func cmdAccept(s *core.Store, args []string) {
	fs := flag.NewFlagSet("accept", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	label := fs.String("label", "", "the name under which this person's notes show in your log")
	heard := fs.String("words", "", "the identity words that the person read to you")
	yes := fs.Bool("yes", false, "do not ask: for scripts, with -words")
	pos, err := parseInterspersed(fs, args)
	if err != nil || len(pos) != 1 {
		usage()
	}
	// A script cannot compare words by eye, so it states the words that
	// the person heard.
	if *yes && *heard == "" {
		badArg(errors.New("-yes takes notes without a question, so it needs -words: the eight identity words that the person read to you"))
	}
	// A reply is one sealed file, and a small one: its notes, and the
	// identity records of whoever wrote them.
	if info, err := os.Stat(pos[0]); err != nil || !info.Mode().IsRegular() || info.Size() > maxReply {
		fail(fmt.Errorf("%s is not a reply: a reply is one sealed file of at most %d MiB", pos[0], maxReply>>20))
	}
	stdin := bufio.NewReader(os.Stdin)
	in := readPackage(pos[0], stdin, "")
	res := reply.Check(in)
	switch {
	case in.Asks == bundle.AsksKey:
		fail(errors.New("this file is sealed to a key. bean accept takes a reply, and this bean seals a reply with words: nothing was taken"))
	case res.Locked:
		fail(errors.New("the reply did not open: nothing was checked, and nothing was taken"))
	case !res.Unread && !in.Opened:
		fail(errors.New("this is not a sealed file: a reply is sealed, and nothing was taken"))
	case !res.OK && !res.Unread && bundle.Check(in).OK:
		fail(errors.New("this is a share, not a reply: read it with bean check-bundle"))
	case !res.OK:
		for _, p := range res.Problems {
			fmt.Println("problem: " + safe(p))
		}
		fail(errors.New("this is not a reply that passes its check: nothing was taken"))
	}
	l, err := s.Load()
	if err != nil {
		fail(err)
	}
	if l.ID == "" || l.Local == nil {
		fail(core.ErrNotInitialized)
	}
	if warnFrozen(l) {
		os.Exit(4)
	}
	a, err := againstLog(l, s.Now(), res, *label)
	if err != nil {
		fail(fmt.Errorf("%w: nothing was taken", err))
	}
	share, name := a.share, a.name
	if *heard != "" && strings.Join(strings.Fields(strings.ToLower(*heard)), " ") != res.Words {
		fail(errors.New("the identity words of whoever signed this reply are not the words that you gave: nothing was taken"))
	}
	fmt.Printf("A reply to your share %s, of %s.\n", share.ID[7:19], safe(bundle.When(share)))
	if n, _ := share.Context["note"].(string); n != "" {
		fmt.Printf("  Your note on that share: %q\n", safe(n))
	}
	fmt.Printf("The reply's id starts %s.", res.ReplyID[7:19])
	if res.When != "-" {
		fmt.Printf(" It says that it was written at %s.", safe(res.When))
	}
	fmt.Println()
	fmt.Printf("It is signed by the key %s, which speaks for the identity with these words:\n  %s\n", safe(bundle.KeyName(res.Key)), res.Words)
	switch a.seen {
	case "key":
		fmt.Printf("You paired this key before, as %q.\n", safe(name))
	case "identity":
		fmt.Printf("You paired this identity before, as %q. This is another key: the reply's identity records show a change of key after the one that you saw.\n", safe(name))
	default:
		fmt.Printf("You have not seen this identity before. Its notes will show as %q.\n", safe(name))
	}
	fmt.Println("Ask the person whether they sent a reply whose id starts so, and to read their identity words to you. If either is not the same, do not accept it.")
	fmt.Println("bean cannot tell whether this is still that person's current key: a reply shows the changes of key that were made before it. Someone who holds an older key of theirs shows the same words.")
	fmt.Println("A note is the word of the person who wrote it. bean does not check what it says, and it is not medical advice.")
	fmt.Printf("%d notes:\n", len(res.Notes))
	// What is taken is decided once, here, for the list and for the write.
	var take []reply.Note
	have := 0
	for _, n := range res.Notes {
		what := "your share itself"
		if target := l.Report.Records[n.About]; target != nil && n.About != share.ID {
			what = target.Type + " " + recordText(target, func(id string) *core.Record { return l.Report.Records[id] })
			if !l.Current(n.About) {
				what += " (" + l.Status(n.About) + ")"
			}
		} else if n.About != share.ID {
			what = "a record that is not in this log now"
		}
		state := ""
		switch {
		case l.DeletedByPerson(n.ID):
			state = " (you deleted this note: it is not taken again)"
		case l.DeletedByPerson(n.About):
			state = " (you deleted the record that it is about: it is not taken)"
		case l.Report.Records[n.ID] != nil:
			have++
			state = " (in your log already)"
		default:
			take = append(take, n)
		}
		fmt.Printf("  about %s (%s)%s:\n    %q\n", safe(what), n.About[7:19], state, safe(n.Text))
	}
	if len(take) == 0 {
		fmt.Println("Your log holds every note of this reply already, or you deleted it: nothing was taken.")
		return
	}
	if !*yes {
		if !isTerminal(os.Stdin) || !isTerminal(os.Stderr) {
			fail(errors.New("nothing was taken: bean accept asks before it writes, and there is no terminal to ask on. Read the list above, then pass -yes with -words"))
		}
		fmt.Fprintf(os.Stderr, "The identity words once more: %s\nTake these %d notes into your log, as notes of %q? [y/N] ", res.Words, len(take), name)
		line, _ := stdin.ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			fail(errors.New("nothing was taken"))
		}
	}
	pairID, err := s.PairPeer(res.Key, name, res.Identity, res.Chain[len(res.Chain)-1])
	if err != nil {
		fail(err)
	}
	for i, n := range take {
		if err := s.AddFromKey(n.ID, n.Bytes); err != nil {
			fail(fmt.Errorf("%d notes were taken, and %s was not: %w. Run bean accept again: it takes what is left", i, n.ID[7:19], err))
		}
		fmt.Println("took", n.ID)
	}
	fmt.Printf("took %d notes of %q, already there %d\n", len(take), name, have)
	if a.seen != "key" {
		fmt.Printf("paired %s\n", pairID)
		fmt.Println("Your log now keeps this person's key and their identity id, in your record of the name. Both travel when you share one of their notes.")
	}
	fmt.Printf("The notes show in bean ls as [peer %s], and bean show <id> lists the notes about a record. bean unpair %s takes the name back.\n", safe(name), safe(name))
}

// accepted is what a reply is to a log, once the log's own rules pass.
type accepted struct {
	share *core.Record // the share that the reply answers
	name  string       // the name under which its notes show
	seen  string       // what the person paired before: "key", "identity" or ""
}

// againstLog holds a reply that passed its own check to what only the
// person's log can tell: whose it is, which share it answers, what the
// person saw of its signer before, and under which name its notes show.
// It writes nothing.
func againstLog(l *core.Loaded, now time.Time, res reply.Result, label string) (accepted, error) {
	var a accepted
	// The reply must answer a share of this log's person, and each note
	// must be about a record of that share: a peer speaks about what they
	// were shown, and about nothing else.
	if res.To != l.ID {
		return a, errors.New("this reply is for another person")
	}
	share := l.Report.Records[res.Share]
	if share == nil || share.Type != bundle.ShareType || share.Author != l.ID || share.Subject != l.ID || !l.Report.Valid[share.ID] {
		return a, fmt.Errorf("this reply answers the share %s, which is not a share in this log", res.Share[7:19])
	}
	shared := map[string]bool{share.ID: true}
	list, _ := share.Context["records"].([]any)
	for _, x := range list {
		if id, ok := x.(string); ok {
			shared[id] = true
		}
	}
	for _, n := range res.Notes {
		if !shared[n.About] {
			return a, fmt.Errorf("the note %s is about %s, which is not a record of that share", n.ID[7:19], n.About[7:19])
		}
	}
	// A note that says it was written later than now is not taken: it
	// would sort after everything else in the log.
	latest := now.UTC().Add(24 * time.Hour).Format("2006-01-02T15:04:05Z")
	for _, n := range res.Notes {
		if n.Written > latest {
			return a, fmt.Errorf("the note %s says that it was written at %s, which is later than now", n.ID[7:19], safe(n.Written))
		}
	}
	// What the person saw of this identity before. A key that they unpaired
	// speaks under no name again. And a reply must show the identity's chain
	// at least as far as the person has seen it: a shorter chain is how a
	// retired key shows itself, and another chain is a fork.
	if l.Unpaired(res.Key) {
		return a, errors.New("you unpaired the key that signed this reply. If the person changed their key, ask them for a reply that their new key signed")
	}
	for _, head := range l.PeerHeads(res.Identity) {
		if !slices.Contains(res.Chain, head) {
			return a, errors.New("this reply shows its identity as it was before a change of key that you have seen, or with another history. The key that signed it was retired, or was never this person's. Ask the person for a reply that their current key signed")
		}
	}
	if label != "" {
		if err := core.CheckLabel(label); err != nil {
			return a, err
		}
	}
	// A key that the person paired as an agent of their own is not another
	// person's key. Its notes would show under the agent's name.
	if agent := l.PairedAsAgent(res.Key); agent != "" {
		return a, fmt.Errorf("the key that signed this reply is paired as your agent %q. A reply is from another person", agent)
	}
	// The name: the one this key has, or the one that another key of the
	// same identity has, or the one given now.
	paired := l.PeerNames()[res.Key]
	known := l.PeerLabel(res.Identity)
	name := label
	switch {
	case paired != "":
		a.seen = "key"
		if name = paired; label != "" && label != name {
			return a, fmt.Errorf("this key is already paired as %q: bean does not rename it. Leave -label out", name)
		}
	case known != "":
		a.seen = "identity"
		if name = known; label != "" && label != known {
			return a, fmt.Errorf("you paired this identity as %q: bean does not give it a second name. Leave -label out", known)
		}
	case name == "":
		return a, errors.New("give -label <name>, the name under which this person's notes show in your log")
	case l.NameTaken(name, res.Identity):
		return a, fmt.Errorf("the name %q is taken: a key of another person, or an agent, has it. Give another name with -label", name)
	}
	a.share, a.name = share, name
	return a, nil
}

// cmdUnpair is bean unpair: the person withdraws a name from the keys that
// carry it.
func cmdUnpair(s *core.Store, args []string) {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		usage()
	}
	keys, err := s.Unpair(args[0])
	if err != nil {
		fail(err)
	}
	if len(keys) == 0 {
		fail(fmt.Errorf("no key is paired as %q: bean ls shows the names in brackets", args[0]))
	}
	for _, key := range keys {
		fmt.Println("unpaired", key)
	}
	fmt.Println("The records that these keys signed stay in your log, and show as an unpaired key's. To delete one: bean purge <id>.")
}
