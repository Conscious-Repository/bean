// Package reply checks a reply: the notes that a peer signs about records
// that a person shared with them, sealed and sent back. The person takes
// them into their own log, marked with who signed them.
//
// docs/SHARE.md ("A signed reply") is the format in words.
package reply

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"unicode/utf8"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/bundle"
)

// Type is the type of the record that lists a reply's notes. The replier
// signs it as themselves, about themselves: it is their record of what
// they sent, as a share record is.
const Type = "reply"

// NoteType is the type of every note of a reply.
const NoteType = "note"

// System is source.system of a reply's records.
const System = "bean-reply"

// MaxNote is the most bytes that the text of one note holds, and MaxNotes
// the most notes of one reply.
const (
	MaxNote  = 4000
	MaxNotes = 200
)

// Note is one note of a reply.
type Note struct {
	ID    string // the note's own id
	About string // the id of the record that it is about
	Text  string
	Bytes []byte // the record file, as signed
}

// Result is what a check of a reply found.
type Result struct {
	OK bool
	// Unread reports that what was given could not be read at all, and
	// Locked a sealed package that did not open.
	Unread, Locked bool
	Problems       []string
	// Identity is the identity that replied, Words that id as eight words,
	// and Key the key that signed: the head of that identity's chain, by
	// the identity records in the reply.
	Identity, Words, Key string
	// Chain are the ids of the identity's chain in the reply, in order.
	// The last is its head: the record that names the current key. A
	// person who saw this identity before holds the head that they saw,
	// and a reply must show a chain that holds it.
	Chain []string
	// To is the identity that the reply is for, and Share the id of the
	// share record that it answers.
	To, Share string
	// ReplyID is the id of the reply record, and When the time it states.
	ReplyID, When string
	// Notes are the notes, in id order. They are set only when every check
	// passed.
	Notes []Note
}

// Check checks a reply by the rules of docs/SHARE.md. It does not know the
// log of the person that the reply is for: whether the share is theirs,
// and whether each note is about a record of it, is for the caller.
func Check(in bundle.Input) Result {
	var res Result
	bad := func(format string, a ...any) { res.Problems = append(res.Problems, fmt.Sprintf(format, a...)) }
	done := func() Result {
		sort.Strings(res.Problems)
		res.OK = len(res.Problems) == 0
		if !res.OK {
			res.Notes = nil
		}
		return res
	}
	if len(in.Fatal) > 0 {
		res.Problems, res.Unread, res.Locked = slices.Clone(in.Fatal), true, in.Locked
		return done()
	}
	res.Problems = slices.Clone(in.Problems)
	// Rule 1: records and nothing else, and every record file passes the
	// checks of a log with nothing left over.
	if len(in.Files) > 0 || in.FilesFault != "" || len(in.Ignored) > 0 {
		bad("a reply holds records and nothing else: no file, and nothing beside the records")
	}
	rp := core.VerifyFiles(in.Records)
	for _, where := range slices.Sorted(maps.Keys(rp.Errors)) {
		bad("%s: %s", where, rp.Errors[where])
	}
	if len(rp.Unknown) > 0 || rp.Skipped > 0 || len(rp.TooNew) > 0 {
		bad("a record is of a newer format than this bean knows, so it cannot be checked")
	}
	if len(rp.Compromised) > 0 || len(rp.Incomplete) > 0 {
		bad("an identity in the reply is frozen, or a key change is not whole")
	}
	if len(res.Problems) > 0 {
		return done()
	}
	// Rule 2: exactly one reply record, signed by a person about
	// themselves, with the key that is the head of their chain.
	var manifest *core.Record
	for _, id := range slices.Sorted(maps.Keys(rp.Records)) {
		if r := rp.Records[id]; r.Type == Type {
			if manifest != nil {
				bad("more than one reply record: a reply holds one")
				return done()
			}
			manifest = r
		}
	}
	if manifest == nil || !rp.Valid[manifest.ID] {
		bad("no valid reply record: nothing says what this reply holds")
		return done()
	}
	who := manifest.Author
	chain := rp.Chains[who]
	if !core.IsID(who) || manifest.Subject != who || len(chain) == 0 {
		bad("the reply record is not signed by a person about themselves")
		return done()
	}
	key := manifest.Kid
	if chain[len(chain)-1].ValueKey() != key {
		bad("the reply record is not signed by the current key of its identity")
		return done()
	}
	// Rule 3: what the reply record says. Its lists are read strictly.
	to, _ := manifest.Context["to"].(string)
	share, _ := manifest.Context["share"].(string)
	list, isList := manifest.Context["notes"].([]any)
	listed := map[string]bool{}
	for _, x := range list {
		id, _ := x.(string)
		if !core.IsID(id) || listed[id] {
			isList = false
		}
		listed[id] = true
	}
	n, _ := manifest.Value["n"].(float64)
	switch {
	case !core.IsID(to) || !core.IsID(share) || !isList || n != float64(len(listed)):
		bad("the reply record's lists are damaged")
		return done()
	case to == who:
		bad("the reply is for the identity that wrote it")
		return done()
	case len(listed) == 0 || len(listed) > MaxNotes:
		bad("a reply holds from 1 to %d notes", MaxNotes)
		return done()
	}
	// Rule 1, the warnings. One is expected: a note is about a person whose
	// identity records are not in the reply, so a log's verifier cannot
	// resolve its subject. That holds for a listed note and for the person
	// that the reply is for. Any other warning fails.
	expected := "unresolved subject: " + to
	for _, id := range slices.Sorted(maps.Keys(rp.Warnings)) {
		if ws := rp.Warnings[id]; !listed[id] || len(ws) != 1 || ws[0] != expected {
			bad("%s: %s", id, ws)
		}
	}
	// Rule 4: every other record is an identity record of the replier, on
	// their chain, or a note that the reply record lists.
	for _, id := range slices.Sorted(maps.Keys(rp.Records)) {
		r := rp.Records[id]
		switch {
		case r == manifest:
		case r.IsIdentity():
			if !bundle.OnChain(rp, r, who) {
				bad("%s is an identity record that is not on the replier's chain", id)
			}
		case listed[id]:
			text, problem := note(r, key, to)
			if problem != "" {
				bad("%s is listed as a note, and %s", id, problem)
				continue
			}
			res.Notes = append(res.Notes, Note{ID: id, About: r.Targets("about")[0], Text: text, Bytes: r.Bytes})
			delete(listed, id)
		default:
			bad("%s is in the reply, and the reply record does not list it", id)
		}
	}
	for id := range listed {
		bad("%s is listed, and is not in the reply", id)
	}
	res.Identity, res.Words, res.Key, res.To, res.Share, res.ReplyID = who, core.IDWords(who), key, to, share, manifest.ID
	for _, node := range chain {
		res.Chain = append(res.Chain, node.ID)
	}
	res.When = "-"
	if obs, ok := manifest.Observed.(map[string]any); ok {
		res.When, _ = obs["start"].(string)
	}
	return done()
}

// note checks one note of a reply and returns its text, or what is wrong
// with it. A note is the replier's key speaking by itself about the person
// that the reply is for: text about one record, and nothing more. It has
// no power over any record.
func note(r *core.Record, key, to string) (text, problem string) {
	text, isText := r.Value["s"].(string)
	switch {
	case r.Type != NoteType:
		return "", "its type is not " + NoteType
	case r.Author != key || r.Kid != key:
		return "", "it is not signed by the replier's key, by itself"
	case r.Subject != to:
		return "", "it is not about the person that the reply is for"
	case len(r.Refs) != 1 || r.Refs[0].Rel != "about" || !core.IsID(r.Refs[0].ID):
		return "", "it is not about exactly one record"
	case len(r.Value) != 1 || !isText || text == "" || len(text) > MaxNote || !utf8.ValidString(text):
		return "", fmt.Sprintf("its value is not a text of 1 to %d bytes", MaxNote)
	case len(r.Blobs) > 0 || len(r.Context) > 0:
		return "", "it holds more than a text"
	}
	return text, ""
}
