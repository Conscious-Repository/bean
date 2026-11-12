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

// readPackage reads a bundle or a reply from a folder or a file. For a
// sealed package it asks for the six words on in.
func readPackage(path string, in *bufio.Reader) bundle.Input {
	info, err := os.Stat(path)
	if err != nil {
		return bundle.Input{Fatal: []string{fmt.Sprintf("%s cannot be read: %s", path, err)}}
	}
	if info.IsDir() {
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
	if bundle.IsSealed(data) {
		fmt.Fprintln(os.Stderr, "Type the six words of this package, then Enter:")
		words, _ = in.ReadString('\n')
	}
	return bundle.FromPackage(path, data, words)
}

// cmdReply is bean reply: the peer's side.
func cmdReply(s *core.Store, args []string) {
	fs := flag.NewFlagSet("reply", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	to := fs.String("to", "", "what you received: a sealed package, or a bundle's folder or zip file")
	out := fs.String("out", "", "the reply to write: a new file")
	pos, err := parseInterspersed(fs, args)
	if err != nil || *to == "" || *out == "" || len(pos) == 0 || len(pos)%2 != 0 {
		usage()
	}
	if abs, err := filepath.Abs(*out); err == nil && insideLog(abs) {
		fail(fmt.Errorf("%s is inside your log: a reply goes somewhere else", *out))
	}
	if _, err := os.Lstat(*out); err == nil {
		fail(fmt.Errorf("%s is there already: a reply is a new file", *out))
	}
	stdin := bufio.NewReader(os.Stdin)
	// What was received must pass its check: a note is about a record of a
	// share that is what its sender signed.
	got := bundle.Check(readPackage(*to, stdin))
	if !got.OK {
		for _, line := range got.Lines() {
			fmt.Fprintln(os.Stderr, safe(line))
		}
		if got.Locked {
			fail(errors.New("nothing was written: what you reply to did not open"))
		}
		fail(errors.New("nothing was written: what you reply to does not pass bean check-bundle"))
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
	in := readPackage(pos[0], stdin)
	res := reply.Check(in)
	switch {
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
	// The reply must answer a share of this log's person, and each note
	// must be about a record of that share: a peer speaks about what they
	// were shown, and about nothing else.
	if res.To != l.ID {
		fail(errors.New("this reply is for another person: nothing was taken"))
	}
	share := l.Report.Records[res.Share]
	if share == nil || share.Type != bundle.ShareType || share.Author != l.ID || share.Subject != l.ID || !l.Report.Valid[share.ID] {
		fail(fmt.Errorf("this reply answers the share %s, which is not a share in this log: nothing was taken", res.Share[7:19]))
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
			fail(fmt.Errorf("the note %s is about %s, which is not a record of that share: nothing was taken", n.ID[7:19], n.About[7:19]))
		}
	}
	// A note that says it was written later than now is not taken: it
	// would sort after everything else in the log.
	latest := s.Now().UTC().Add(24 * time.Hour).Format("2006-01-02T15:04:05Z")
	for _, n := range res.Notes {
		if n.Written > latest {
			fail(fmt.Errorf("the note %s says that it was written at %s, which is later than now: nothing was taken", n.ID[7:19], safe(n.Written)))
		}
	}
	// What the person saw of this identity before. A key that they unpaired
	// speaks under no name again. And a reply must show the identity's chain
	// at least as far as the person has seen it: a shorter chain is how a
	// retired key shows itself, and another chain is a fork.
	if l.Unpaired(res.Key) {
		fail(errors.New("you unpaired the key that signed this reply: nothing was taken. If the person changed their key, ask them for a reply that their new key signed"))
	}
	for _, head := range l.PeerHeads(res.Identity) {
		if !slices.Contains(res.Chain, head) {
			fail(errors.New("this reply shows its identity as it was before a change of key that you have seen, or with another history: nothing was taken. The key that signed it was retired, or was never this person's. Ask the person for a reply that their current key signed"))
		}
	}
	// The name: the one this key has, or the one that another key of the
	// same identity has, or the one given now.
	labels := l.AgentLabels()
	known := l.PeerLabel(res.Identity)
	name, first := *label, false
	switch {
	case labels[res.Key] != "":
		name = strings.TrimPrefix(labels[res.Key], "peer ")
		if *label != "" && *label != name {
			fail(fmt.Errorf("this key is already paired as %q: bean does not rename it. Leave -label out", name))
		}
	case known != "":
		if name = known; *label != "" && *label != known {
			fail(fmt.Errorf("you paired this identity as %q: bean does not give it a second name. Leave -label out", known))
		}
	case name == "":
		fail(errors.New("nothing was taken: give -label <name>, the name under which this person's notes show in your log"))
	case l.NameTaken(name, res.Identity):
		fail(fmt.Errorf("the name %q is taken: a key of another person, or an agent, has it. Give another name with -label", name))
	default:
		first = true
	}
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
	fmt.Printf("It is signed by the key %s, which speaks for the identity with these words:\n  %s\n", safe(keyName(res.Key)), res.Words)
	switch {
	case labels[res.Key] != "":
		fmt.Printf("You paired this key before, as %q.\n", safe(name))
	case known != "":
		fmt.Printf("You paired this identity before, as %q. This is another key: the reply's identity records show a change of key after the one that you saw.\n", safe(name))
	default:
		fmt.Printf("You have not seen this identity before. Its notes will show as %q.\n", safe(name))
	}
	fmt.Println("Ask the person whether they sent a reply whose id starts so, and to read their identity words to you. If either is not the same, do not accept it.")
	fmt.Println("bean cannot tell whether this is still that person's current key: a reply shows the changes of key that were made before it. Someone who holds an older key of theirs shows the same words.")
	fmt.Println("A note is the word of the person who wrote it. bean does not check what it says, and it is not medical advice.")
	fmt.Printf("%d notes:\n", len(res.Notes))
	have, deleted := 0, 0
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
			deleted++
			state = " (you deleted this note: it is not taken again)"
		case l.DeletedByPerson(n.About):
			deleted++
			state = " (you deleted the record that it is about: it is not taken)"
		case l.Report.Records[n.ID] != nil:
			have++
			state = " (in your log already)"
		}
		fmt.Printf("  about %s (%s)%s:\n    %q\n", safe(what), n.About[7:19], state, safe(n.Text))
	}
	if have+deleted == len(res.Notes) {
		fmt.Println("Your log holds every note of this reply already, or you deleted it: nothing was taken.")
		return
	}
	if !*yes {
		if !isTerminal(os.Stdin) || !isTerminal(os.Stderr) {
			fail(errors.New("nothing was taken: bean accept asks before it writes, and there is no terminal to ask on. Read the list above, then pass -yes with -words"))
		}
		fmt.Fprintf(os.Stderr, "The identity words once more: %s\nTake these %d notes into your log, as notes of %q? [y/N] ", res.Words, len(res.Notes)-have-deleted, name)
		line, _ := stdin.ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			fail(errors.New("nothing was taken"))
		}
	}
	pairID, err := s.PairPeer(res.Key, name, res.Identity, res.Chain[len(res.Chain)-1])
	if err != nil {
		fail(err)
	}
	added := 0
	for _, n := range res.Notes {
		if l.Report.Records[n.ID] != nil || l.DeletedByPerson(n.ID) || l.DeletedByPerson(n.About) {
			continue
		}
		if err := s.AddFromKey(n.ID, n.Bytes); err != nil {
			fail(fmt.Errorf("%d notes were taken, and %s was not: %w", added, n.ID[7:19], err))
		}
		added++
		fmt.Println("took", n.ID)
	}
	fmt.Printf("took %d notes of %q, already there %d\n", added, name, have)
	if first || labels[res.Key] == "" {
		fmt.Printf("paired %s\n", pairID)
		fmt.Println("Your log now keeps this person's key and their identity id, in your record of the name. Both travel when you share one of their notes.")
	}
	fmt.Printf("The notes show in bean ls as [peer %s], and bean show <id> lists the notes about a record. bean unpair %s takes the name back.\n", safe(name), safe(name))
}

// keyName is how a key is shown: as an npub when it is a Nostr key, and as
// the tagged key when it is not.
func keyName(key string) string {
	if npub := core.Npub(key); npub != "" {
		return npub
	}
	return key
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
