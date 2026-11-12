// Package bundle checks a bundle: the folder that bean share writes. It
// reads no log and no key, and changes nothing. docs/SHARE.md has the
// format and the five rules.
//
// The check does not touch the disk: it takes a bundle as an Input. ReadDir
// makes one from a folder, FromFiles and FromZip from bytes already in
// memory, which is how the page that checks a bundle in a browser
// (tools/page) runs the same check as bean check-bundle.
package bundle

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/pack"
	"github.com/Conscious-Repository/bean/internal/view"
)

// ShareType is the type of the record that lists what a bundle holds.
const ShareType = "share"

// Caveats are what no bundle can show. Every check prints them.
var Caveats = []string{
	"These are selected records, not a complete history: a correction or a deletion that the share leaves out does not show.",
	"A signature shows which identity signed, not that a value is true, nor who that identity is.",
	"The check shows the key that signed. A bundle cannot show that it is still the person's key: a key change made since is not in it. When it matters, ask the person for their current key (bean whoami).",
}

// IsPairing reports a pairing in force that the person signed about
// themselves: an agent.pair claim that does not say "unpaired", as bean ls
// reads one. A device's own kind of pairing comes with device capture
// (docs/EXTENSIONS.md 11), in both places at once.
func IsPairing(r *core.Record, person string) bool {
	status, _ := r.Value["status"].(string)
	return r.Type == "agent.pair" && r.Author == person && r.Subject == person && r.ValueKey() != "" && status != "unpaired"
}

// StatedDigest is the SHA-256 that a record states for the one file it
// keeps (context.sha256, as lab.report does), or "".
func StatedDigest(r *core.Record, blob string) string {
	if len(r.Blobs) != 1 || r.Value["blob"] != blob {
		return ""
	}
	s, _ := r.Context["sha256"].(string)
	return s
}

// NeverListed reports the kinds of record that a share does not list:
// identity records and pairings travel as proof, and a deletion or the
// record of a share is not shared.
func NeverListed(r *core.Record) bool {
	return r.IsIdentity() || r.Type == "bean.tombstone" || r.Type == "agent.pair" || r.Type == ShareType
}

// OnChain reports an identity record of the person that their chain holds.
func OnChain(rp *core.Report, r *core.Record, person string) bool {
	return r.IsIdentity() && (r.ID == person || r.Subject == person) && rp.Valid[r.ID] && !rp.OffTree(r)
}

// File is one entry of a bundle's files/ folder.
type File struct {
	Name    string
	Regular bool          // a plain file, not a link or a folder
	Size    int64         // in bytes
	Digest  func() string // its SHA-256 in hex, or "" when it cannot be read
	// Read returns the file's bytes. A reader that was shown a passed
	// bundle saves a carried file through it.
	Read func() ([]byte, error)
}

// Input is a bundle as it was read, before any check.
type Input struct {
	// Fatal says why the folder cannot be read as a bundle at all. Nothing
	// else is looked at then.
	Fatal []string
	// Problems are faults found while reading: a record file that is not a
	// plain file.
	Problems []string
	// Ignored names what else the folder holds. It is not part of the
	// bundle and was not read.
	Ignored []string
	// Records are the record files, by their path: objects/ab/cdef.json.
	Records map[string][]byte
	// Files are the entries of files/.
	Files []File
	// FilesFault says why files/ could not be listed, or "".
	FilesFault string
	// Locked reports a sealed package that did not open: no words, wrong
	// words, or a changed file. Opened reports one that its words opened.
	Locked, Opened bool
}

// Carried is one file that a bundle carries.
type Carried struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	Type   string `json:"type"`  // the type of the record that keeps it
	Media  string `json:"media"` // the media type that record states
	Record string `json:"record"`
}

// Row is one shared record, as a reader sees it.
type Row struct {
	When string `json:"when"` // as the sender recorded it, with their offset from UTC
	Type string `json:"type"`
	Text string `json:"text"`
	By   string `json:"by"` // "" when the person signed it; otherwise who did
	ID   string `json:"id"`
	// Mark is what the check itself says about a comparison: its pack is
	// not on record, the claim misstates its pack, or it names no
	// reference. Text ends with the same words.
	Mark string `json:"mark"`
}

// Result is what a check found.
type Result struct {
	OK bool `json:"ok"`
	// Unread reports that what was given could not be read as a bundle at
	// all: the wrong folder, not a zip file. Nothing was checked.
	Unread bool `json:"unread"`
	// Locked reports a sealed package that did not open, and Opened one
	// that its words opened. Only someone who knows the words can have
	// sealed a package that they open.
	Locked bool `json:"locked"`
	Opened bool `json:"opened"`
	// Identity is the id of the identity that signed the share record, and
	// Words that id as six words that a person can read out. Key is the
	// key that signed, and When the time that the share record states.
	// They are set when every check passed.
	Identity string `json:"identity"`
	Words    string `json:"words"`
	Key      string `json:"key"`
	When     string `json:"when"`
	// Share is the id of the share record. With When it says which share
	// this is: a person can make many, and a reader may be handed an older
	// one.
	Share string `json:"share"`
	// Head is what the check says before the records: who shared, when, and
	// the counts.
	Head []string `json:"head"`
	// Note is what the person wrote for the reader, set when every check
	// passed. Head holds it too.
	Note string `json:"note"`
	// Ignored names what lay in the bundle's folder and is not part of the
	// bundle: it was not read and not checked. Head holds it too.
	Ignored []string `json:"ignored"`
	// Files is how many files the bundle carries, set when every check
	// passed.
	Files int `json:"files"`
	// Carried lists those files, set when every check passed: each is the
	// file that a listed record keeps, with the SHA-256 that the share
	// record gives it.
	Carried []Carried `json:"carried"`
	// Rows are the shared records. They are set only when every check
	// passed: a reader should not read values out of a bundle that failed.
	Rows []Row `json:"rows"`
	// Tail is what a reader must keep in mind: the comparisons' own caveats,
	// and what no bundle can show.
	Tail     []string `json:"tail"`
	Problems []string `json:"problems"`
}

// Lines is the result as bean check-bundle prints it.
func (res Result) Lines() []string {
	lines := slices.Clone(res.Head)
	if res.Rows != nil {
		lines = append(lines, "It holds:")
	}
	for _, row := range res.Rows {
		by := ""
		if row.By != "" {
			by = "  " + row.By
		}
		lines = append(lines, fmt.Sprintf("  %s  %s  %s%s  %s", row.When, row.Type, row.Text, by, row.ID))
	}
	lines = append(lines, res.Tail...)
	for _, p := range res.Problems {
		lines = append(lines, "problem: "+p)
	}
	if !res.OK {
		return append(lines, fmt.Sprintf("bundle FAILED: %d problems", len(res.Problems)))
	}
	return append(lines, "bundle ok")
}

// Check checks a bundle by the five rules of docs/SHARE.md.
func Check(in Input) Result {
	var res Result
	bad := func(format string, a ...any) { res.Problems = append(res.Problems, fmt.Sprintf(format, a...)) }
	done := func() Result {
		sort.Strings(res.Problems)
		res.OK = len(res.Problems) == 0
		return res
	}
	if len(in.Fatal) > 0 {
		res.Problems, res.Unread, res.Locked = slices.Clone(in.Fatal), true, in.Locked
		return done()
	}
	res.Opened = in.Opened
	res.Problems = slices.Clone(in.Problems)
	// Rule 1: every record file passes the checks of a log, with nothing
	// left over: no error, no warning, no record of a newer format.
	rp := core.VerifyFiles(in.Records)
	for _, where := range slices.Sorted(maps.Keys(rp.Errors)) {
		res.Head = append(res.Head, fmt.Sprintf("error: %s: %s", where, rp.Errors[where]))
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
		if r := rp.Records[id]; r.Type == ShareType {
			if share != nil {
				bad("more than one share record: a bundle holds one")
				return done()
			}
			share = r
		}
	}
	if share == nil || !rp.Valid[share.ID] {
		bad("no valid share record: nothing says what this bundle holds")
		return done()
	}
	person := share.Author
	if !core.IsID(person) || share.Subject != person {
		bad("the share record is not signed by a person about themselves")
		return done()
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
		return done()
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
		return done()
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
			res.Head = append(res.Head, fmt.Sprintf("warning: %s: %s", id, w))
		}
		bad("%s has a warning that a bundle may not have", id)
	}
	// Rule 3: the records are exactly the listed ones, each valid, about
	// the person and of a kind that is shared, plus the proof: the person's
	// identity records and pairings that the share record names. A record
	// that an agent or another key signed is valid, and counted apart: the
	// person chose to share it, and did not write it.
	// A key that the person paired under two labels, on two copies, shows
	// under both, in the order of the pairings' ids: the same bundle reads
	// the same on every check.
	paired := map[string]string{} // key -> the labels the person gave it
	for _, id := range slices.Sorted(maps.Keys(proof)) {
		if r := rp.Records[id]; r != nil && IsPairing(r, person) && rp.Valid[id] {
			label, _ := r.Value["label"].(string)
			if label == "" {
				label = "agent"
			}
			switch has := paired[r.ValueKey()]; {
			case has == "":
				paired[r.ValueKey()] = label
			case !slices.Contains(strings.Split(has, ", "), label):
				paired[r.ValueKey()] = has + ", " + label
			}
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
			if !OnChain(rp, r, person) && !(IsPairing(r, person) && rp.Valid[id]) {
				bad("%s is named as proof, and is not a valid identity record or pairing of the person", id)
			}
		case !listed[id]:
			bad("%s is in the bundle, and the share record does not list it", id)
		case !rp.Valid[id]:
			bad("%s is listed, and is not valid", id)
		case r.Subject != person:
			bad("%s is listed, and is about someone else", id)
		case NeverListed(r):
			bad("%s is listed, and is a %s record, which is not shared", id, r.Type)
		case r.Author == person:
			byPerson++
		case paired[r.Author] != "":
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
	var carried []Carried
	for _, id := range slices.Sorted(maps.Keys(listed)) {
		r := rp.Records[id]
		if r == nil {
			continue
		}
		once := map[string]bool{}
		for _, b := range r.Blobs {
			if once[b.Hash] {
				continue // a record may name one file twice
			}
			once[b.Hash] = true
			e, ok := account[b.Hash+" "+id]
			if !ok {
				named++
				continue
			}
			delete(account, b.Hash+" "+id)
			carried = append(carried, Carried{SHA256: e.digest, Bytes: int64(e.size), Type: r.Type, Media: b.MediaType, Record: id[7:19]})
			if stated := StatedDigest(r, b.Hash); stated != "" && e.digest != stated {
				bad("the file of %s is not the one its record states", id)
			}
		}
	}
	if len(account) > 0 {
		bad("the share record lists a file that no listed record names")
	}
	// Rule 5: files/ holds exactly the included files.
	included := len(want)
	if in.FilesFault != "" {
		bad("%s", in.FilesFault)
	}
	for _, f := range in.Files {
		size, ok := want[f.Name]
		if !ok {
			bad("files/%s is in the bundle, and the share record does not list it", f.Name)
			continue
		}
		delete(want, f.Name)
		if !f.Regular {
			bad("files/%s is not a plain file", f.Name)
			continue
		}
		// The size first: a file of another size is not read at all.
		if float64(f.Size) != size || f.Digest() != f.Name {
			bad("files/%s is not the file the share record lists", f.Name)
		}
	}
	for digest := range want {
		bad("files/%s is listed, and is not in the bundle", digest)
	}
	when := "-"
	if obs, ok := share.Observed.(map[string]any); ok {
		when, _ = obs["start"].(string)
	}
	res.Head = append(res.Head,
		fmt.Sprintf("share %s", share.ID),
		fmt.Sprintf("signed by identity %s, with key %s, at %s", person, share.Kid, when),
		fmt.Sprintf("identity words: %s", core.IDWords(person)),
		fmt.Sprintf("records: %d listed; files: %d included, %d named and not included", len(listed), included, named),
		fmt.Sprintf("signed: %d by the person, %d by an agent the person paired, %d by another key", byPerson, byAgent, byOther),
		fmt.Sprintf("corrections: %d of the listed records replace an earlier record, which is not in the bundle", corrections),
	)
	// The person's note is shown only with a bundle that passed: on a
	// failed one it could say "bundle ok" above the verdict.
	if n, _ := share.Context["note"].(string); n != "" && len(res.Problems) == 0 {
		res.Head = append(res.Head, "note: "+n)
	}
	for _, key := range slices.Sorted(maps.Keys(paired)) {
		res.Head = append(res.Head, fmt.Sprintf("paired by the person: %s, key %s", paired[key], core.Npub(key)))
	}
	for _, name := range in.Ignored {
		res.Head = append(res.Head, "not part of the bundle, and not read: "+name)
	}
	res.Ignored = in.Ignored
	// What it holds, shown only when every check passed.
	if len(res.Problems) == 0 {
		res.Note, _ = share.Context["note"].(string)
		res.Identity, res.Words, res.Key, res.When, res.Share = person, core.IDWords(person), share.Kid, when, share.ID
		res.Files = included
		res.Carried = carried
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
		inBundle := func(id string) *core.Record {
			if listed[id] {
				return rp.Records[id]
			}
			return nil
		}
		var caveats, packs []string
		res.Rows = []Row{}
		for _, r := range rows {
			by := ""
			switch {
			case r.Author == person:
			case paired[r.Author] != "":
				by = "[" + paired[r.Author] + "]"
			default:
				by = "[signed by another key: " + core.Npub(r.Author) + "]"
			}
			res.Rows = append(res.Rows, Row{When: sharedWhen(r), Type: r.Type, Text: view.Record(r, inBundle), By: by, ID: r.ID[7:19], Mark: view.Mark(r)})
			// A comparison with a reference pack names it by SHA-256.
			if ref, ok := r.Context["reference"].(map[string]any); ok && strings.HasPrefix(r.Type, "comparison.") {
				digest, _ := ref["pack"].(string)
				title, _ := ref["title"].(string)
				// A title is the sender's word. It is shown only for a pack
				// that bean does not know, and as what the sender calls it.
				line := fmt.Sprintf("  - %s: %s", digest, pack.OnRecord(digest))
				if !pack.IsKnown(digest) {
					line += fmt.Sprintf(". The sender calls it %q. Whoever built it chose its tables, its caveats and what each reading is set against", title)
				}
				if len(digest) == 64 && !slices.Contains(packs, line) {
					packs = append(packs, line)
				}
			}
			list, _ := r.Context["caveats"].([]any)
			for _, x := range list {
				if text, _ := x.(string); text != "" && strings.HasPrefix(r.Type, "comparison.") && !slices.Contains(caveats, text) {
					caveats = append(caveats, text)
				}
			}
		}
		// A comparison is shown as its claim states it. This check proves
		// who signed the claim, not its arithmetic.
		if len(caveats) > 0 {
			res.Tail = append(res.Tail, "The comparisons above are shown as claimed, and are not computed again here: tools/compare/reproduce.py does that. What they say of themselves, in the sender's words and the words of what they were made with, not this check's:")
			for _, text := range caveats {
				res.Tail = append(res.Tail, "  - "+text)
			}
		}
		// The reader needs the pack to compute such a comparison again, and
		// the title is the sender's word: the SHA-256 is what names a pack.
		if len(packs) > 0 {
			res.Tail = append(res.Tail, "A comparison by a pack was made with a file of reference data that is not in this bundle. To compute it again, get the pack with that SHA-256 and give it to tools/compare/reproduce.py with the bundle. The SHA-256 names the pack:")
			res.Tail = append(res.Tail, packs...)
		}
	}
	res.Tail = append(res.Tail, Caveats...)
	return done()
}

// sharedWhen is a record's observed time as the record states it, with its
// own offset from UTC: a reader in another time zone sees the time the
// sender recorded, not a shifted one.
func sharedWhen(r *core.Record) string {
	obs, ok := r.Observed.(map[string]any)
	if !ok {
		return "-"
	}
	start, _ := obs["start"].(string)
	t, err := time.Parse(time.RFC3339Nano, start)
	if err != nil {
		return start
	}
	switch obs["precision"] {
	case "year":
		return t.Format("2006")
	case "month":
		return t.Format("2006-01")
	case "day":
		return t.Format("2006-01-02")
	}
	return t.Format("2006-01-02 15:04 -07:00")
}
