package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
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
	packed, err := bundle.PackFiles(files)
	var words string
	if err == nil {
		words, err = bundle.NewWords()
	}
	var data []byte
	if err == nil {
		data, err = bundle.Seal(packed, words)
	}
	if err == nil && !reply.Check(bundle.FromPackage(*out, data, words)).OK {
		err = errors.New("the sealed reply does not open and pass the check")
	}
	if err != nil {
		fail(fmt.Errorf("nothing was written: %w", err))
	}
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		_, err = f.Write(data)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(*out)
		}
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
	fmt.Println("bean keeps a record that you replied, and not the notes: keep this file and its words if you want to read them again.")
	fmt.Println("The person takes your notes into their log with: bean accept " + *out)
}

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
	yes := fs.Bool("yes", false, "do not ask: for scripts")
	pos, err := parseInterspersed(fs, args)
	if err != nil || len(pos) != 1 {
		usage()
	}
	stdin := bufio.NewReader(os.Stdin)
	in := readPackage(pos[0], stdin)
	res := reply.Check(in)
	if !res.OK {
		for _, p := range res.Problems {
			fmt.Println("problem: " + safe(p))
		}
		if res.Locked {
			fail(errors.New("the reply did not open: nothing was checked, and nothing was taken"))
		}
		if !res.Unread && bundle.Check(in).OK {
			fail(errors.New("this is a share, not a reply: read it with bean check-bundle"))
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
	if res.Key == l.Local.Signer.Public() {
		fail(errors.New("this reply is signed by your own key: nothing was taken"))
	}
	// A key that the person unpaired speaks under no name again, whatever
	// its identity records say: a stolen key can show an older chain.
	if l.Unpaired(res.Key) {
		fail(errors.New("you unpaired the key that signed this reply: nothing was taken. If the person changed their key, ask them for a reply that their new key signed"))
	}
	labels := l.AgentLabels()
	fmt.Printf("A reply to your share %s, of %s.\n", share.ID[7:19], safe(sharedWhenOf(share)))
	if n, _ := share.Context["note"].(string); n != "" {
		fmt.Printf("  Your note on that share: %q\n", safe(n))
	}
	fmt.Printf("It was written at %s, and signed by the identity with these words:\n  %s\n", safe(res.When), res.Words)
	fmt.Println("Ask the person who sent it to read their identity words to you. If the words are not the same, do not accept it.")
	fmt.Printf("%d notes:\n", len(res.Notes))
	have := 0
	for _, n := range res.Notes {
		if !shared[n.About] {
			fail(fmt.Errorf("the note %s is about %s, which is not a record of that share: nothing was taken", n.ID[7:19], n.About[7:19]))
		}
		what := "your share itself"
		if target := l.Report.Records[n.About]; target != nil && n.About != share.ID {
			what = target.Type + " " + recordText(target, func(id string) *core.Record { return l.Report.Records[id] })
			if !l.Current(n.About) {
				what += " (" + l.Status(n.About) + ")"
			}
		} else if n.About != share.ID {
			what = "a record that is not in this log now"
		}
		fmt.Printf("  about %s (%s):\n    %s\n", safe(what), n.About[7:19], safe(n.Text))
		if l.Report.Records[n.ID] != nil {
			have++
		}
	}
	fmt.Println("A note is the word of the person who wrote it. bean does not check what it says, and it is not medical advice.")
	// The name: the one this key has, or the one that another key of the
	// same identity has, or the one given now.
	name := labels[res.Key]
	switch {
	case name != "" && *label != "" && *label != name:
		fail(fmt.Errorf("this key is already paired as %q: bean does not rename it. Leave -label out", name))
	case name != "":
		fmt.Printf("You paired this key before, as %q.\n", safe(name))
	default:
		if name = l.PeerLabel(res.Identity); name != "" && (*label == "" || *label == name) {
			fmt.Printf("This is a new key of the identity that you paired as %q: its identity records show the change of key.\n", safe(name))
		} else if name = *label; name == "" {
			fail(errors.New("nothing was taken: give -label <name>, the name under which this person's notes show in your log"))
		}
	}
	if have == len(res.Notes) {
		fmt.Println("Your log holds every note of this reply already.")
		return
	}
	if !*yes {
		if !isTerminal(os.Stdin) || !isTerminal(os.Stderr) {
			fail(errors.New("nothing was taken: bean accept asks before it writes, and there is no terminal to ask on. Read the list above, then pass -yes"))
		}
		fmt.Fprintf(os.Stderr, "Take these %d notes into your log, as notes of %q? [y/N] ", len(res.Notes), name)
		line, _ := stdin.ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			fail(errors.New("nothing was taken"))
		}
	}
	pairID, err := s.PairPeer(res.Key, name, res.Identity)
	if err != nil {
		fail(err)
	}
	added := 0
	for _, n := range res.Notes {
		if l.Report.Records[n.ID] != nil {
			continue
		}
		if err := s.AddFromKey(n.ID, n.Bytes); err != nil {
			fail(fmt.Errorf("%d notes were taken, and %s was not: %w", added, n.ID[7:19], err))
		}
		added++
		fmt.Println("took", n.ID)
	}
	fmt.Printf("took %d notes of %q, already there %d\n", added, name, have)
	fmt.Printf("paired %s\n", pairID)
	// The identity's own records show the keys that it retired. A key of
	// it that the person paired before speaks under the name no longer.
	for _, old := range res.Retired {
		if labels[old] == "" {
			continue
		}
		if keys, err := s.Unpair(old); err != nil {
			fmt.Fprintf(os.Stderr, "bean: the key that %q retired could not be unpaired: %s\n", name, safe(err.Error()))
		} else if len(keys) > 0 {
			fmt.Printf("unpaired the key that %q used before: their identity records show that they changed it. The notes that it signed stay.\n", name)
		}
	}
	fmt.Println("They show in bean ls with that name, and bean show <id> lists the notes about a record.")
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

// sharedWhenOf is the time that a share record states.
func sharedWhenOf(share *core.Record) string {
	if obs, ok := share.Observed.(map[string]any); ok {
		if start, _ := obs["start"].(string); start != "" {
			return start
		}
	}
	return "an unknown time"
}
