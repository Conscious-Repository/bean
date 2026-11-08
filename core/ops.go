package core

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AppendResult is the output of Append.
type AppendResult struct {
	Added, Skipped int
	Corrected      int       // of Added: new values for readings, written with -corrections
	Changed        []Changed // readings with a new value that were not written
	IDs            []string
}

// Changed is a reading that a source gives a new value for. bean does not know
// which value is right: an older report, a parser bug or a mistaken file look
// the same as a lab's corrected report. So it never replaces a value by itself.
type Changed struct {
	ID, Type string // the record the log shows now
	Old, New string // canonical values
	NewType  string // the type in the input
	// Reason is why -corrections cannot write it either, or "" when the person's
	// word (-corrections) would write it.
	Reason string
}

var forbiddenPartial = []string{"v", "nonce", "author", "kid", "id", "sig"}

// Append reads JSONL partial records (SPEC.md 9.2), completes, dedupes,
// signs and writes them (12.3 bean append). On the first error it stops;
// records before it stay written.
func (s *Store) Append(in io.Reader) (AppendResult, error) { return s.appendAs(in, false, false) }

// AppendCorrections is Append for a person who checked the changed readings
// that Append reported: each new value supersedes the current record.
func (s *Store) AppendCorrections(in io.Reader) (AppendResult, error) {
	return s.appendAs(in, false, true)
}

func (s *Store) appendAs(in io.Reader, agent, corrections bool) (AppendResult, error) {
	unlock, err := s.lock()
	if err != nil {
		return AppendResult{}, err
	}
	defer unlock()
	l, err := s.load(false)
	if err != nil {
		return AppendResult{}, err
	}
	w, err := s.writerFor(l, agent)
	if err != nil {
		return AppendResult{}, err
	}
	w.corrections = corrections
	return s.appendWith(l, w, in)
}

// writerFor is the person (agent false) or this machine's agent key.
func (s *Store) writerFor(l *Loaded, agent bool) (writer, error) {
	if agent {
		a, err := s.ReadAgent()
		if err != nil {
			return writer{}, err
		}
		return writer{signer: a.Signer, author: a.Signer.Public(), identity: a.Identity, secret: a.Secret}, nil
	}
	if l.Local == nil {
		if _, err := s.Identity(); err == nil {
			return writer{}, errors.New("no personal key on this replica (agent-only): sign with the agent key instead (-agent)")
		}
		return writer{}, ErrNotInitialized
	}
	return writer{signer: l.Local.Signer, author: l.Local.Identity, identity: l.Local.Identity, secret: l.Local.Secret}, nil
}

// writer is who signs appended records: the person, or an agent key that
// writes as a bare-key author about the person.
type writer struct {
	signer   Signer
	author   string
	identity string // the default subject
	secret   []byte // nil on a replica without the identity secret
	// corrections: the person checked the changed readings of this input.
	corrections bool
}

func (s *Store) appendWith(l *Loaded, w writer, in io.Reader) (AppendResult, error) {
	var res AppendResult
	if err := l.canWrite(w); err != nil {
		return res, err
	}
	var dedupeKey []byte
	if w.secret != nil {
		dedupeKey = DedupeKey(w.secret)
	}
	purged, err := s.readPurgedRefs()
	if err != nil {
		return res, err
	}
	// Existing valid refs of this author and, per 9.7, the ids they belong
	// to. Dedupe is per author (C27): another key's record with the same
	// ref never hides or blocks this author's record.
	// values holds every value a ref has had, so a value the log already knows
	// is skipped, also after a correction. currents holds the records of a ref
	// that no record supersedes, and existing is the one that supersedes_ref
	// names: the first current one, or, when a correction without this ref
	// replaced them all, the first one that has it.
	existing := map[string]string{}
	currents := map[string][]string{}
	values := map[string]map[string]bool{}
	for _, id := range sortedKeys(l.Report.Records) {
		r := l.Report.Records[id]
		if !l.Report.Valid[id] || r.Source.Ref == "" || r.Author != w.author {
			continue
		}
		key := refLine(r.Subject, r.Source.System, r.Source.Ref)
		if values[key] == nil {
			values[key] = map[string]bool{}
			existing[key] = id
		}
		values[key][string(Canonical(r.Raw["value"]))] = true
		if len(l.Report.Superseded[id]) == 0 {
			if currents[key] == nil {
				existing[key] = id
			}
			currents[key] = append(currents[key], id)
		}
	}
	seen := map[string]bool{}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), MaxRecordSize+1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		v, perr := Parse([]byte(line))
		obj, ok := v.(map[string]any)
		if perr != nil || !ok {
			return res, fmt.Errorf("bad json: line %d", lineNo)
		}
		for _, k := range forbiddenPartial {
			if _, has := obj[k]; has {
				return res, fmt.Errorf("bad field: %s: line %d", k, lineNo)
			}
		}
		obj["v"] = 0.0
		obj["nonce"] = NewNonce()
		obj["author"] = w.author
		obj["kid"] = w.signer.Public()
		if _, has := obj["subject"]; !has {
			obj["subject"] = w.identity
		}
		times, _ := obj["times"].(map[string]any)
		if times == nil {
			times = map[string]any{"observed": nil}
			obj["times"] = times
		}
		times["imported"] = s.nowString()
		src, _ := obj["source"].(map[string]any)
		if src == nil {
			return res, fmt.Errorf("bad field: source: line %d", lineNo)
		}
		ref, hasInput, err := inputRef(src, "ref_input", dedupeKey, lineNo)
		if err != nil {
			return res, err
		}
		if hasInput {
			src["ref"] = ref
		}
		if _, has := src["ref"]; !has {
			src["ref"] = nil
		}
		// C17: supersede by ref.
		sref, hasSuper, err := inputRef(src, "supersedes_ref", dedupeKey, lineNo)
		if err != nil {
			return res, err
		}
		if hasSuper {
			subject, _ := obj["subject"].(string)
			system, _ := src["system"].(string)
			target, found := existing[refLine(subject, system, sref)]
			if !found {
				res.Skipped++
				continue
			}
			refs, _ := obj["refs"].([]any)
			obj["refs"] = append(refs, map[string]any{"rel": "supersedes", "id": target})
		}
		// Authority for supersedes (6.6).
		supersedes := map[string]bool{}
		if refs, ok := obj["refs"].([]any); ok {
			for _, x := range refs {
				m, _ := x.(map[string]any)
				if m["rel"] == "supersedes" {
					tid, _ := m["id"].(string)
					if t, ok := l.Report.Records[tid]; ok && t.Author != w.author {
						return res, ErrNotAuthor
					}
					supersedes[tid] = true
				}
			}
		}
		// Dedupe (9.5, 9.7).
		if ref, ok := src["ref"].(string); ok {
			subject, _ := obj["subject"].(string)
			system, _ := src["system"].(string)
			key := refLine(subject, system, ref)
			if purged[key] || seen[key] {
				res.Skipped++
				continue
			}
			seen[key] = true // the first line of an input decides for that ref
			if known := values[key]; known != nil && !supersedes[existing[key]] {
				value := string(Canonical(obj["value"]))
				if known[value] {
					res.Skipped++
					continue
				}
				current := l.Report.Records[existing[key]]
				typ, _ := obj["type"].(string)
				reason := ""
				switch {
				case typ != current.Type:
					reason = "its type differs, so it is not the same reading. Two sources may share ids: give each its own -system"
				case hasNoValue(obj["value"]):
					reason = "the input has no value, and an empty value never replaces one"
				case len(currents[key]) == 0:
					reason = "a correction without this ref replaced the reading, so there is no current record to correct"
				}
				if !w.corrections || reason != "" {
					res.Skipped++
					res.Changed = append(res.Changed, Changed{current.ID, current.Type, string(Canonical(current.Raw["value"])), value, typ, reason})
					continue
				}
				refs, _ := obj["refs"].([]any)
				for _, id := range currents[key] { // duplicates too, or a copy would hide the fix
					refs = append(refs, map[string]any{"rel": "supersedes", "id": id})
				}
				obj["refs"] = refs
				res.Corrected++
			}
		}
		id, bytes, err := SignRecord(obj, w.signer)
		if err != nil {
			return res, err
		}
		fr := CheckFile(PathOf(id), bytes)
		if fr.Err != nil {
			return res, fmt.Errorf("%s: line %d", fr.Err, lineNo)
		}
		if err := l.canWriteRecord(fr.Record); err != nil {
			return res, fmt.Errorf("%s: line %d", err, lineNo)
		}
		wrote, err := writeIfAbsent(s.path(PathOf(id)), bytes, 0o600)
		if err != nil {
			return res, err
		}
		if wrote {
			res.Added++
			res.IDs = append(res.IDs, id)
		}
	}
	if err := sc.Err(); err != nil {
		return res, err
	}
	return res, nil
}

// hasNoValue reports a value that says nothing, like {"n": null}: a test not
// performed must not replace a result.
func hasNoValue(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return true
	}
	n, has := m["n"]
	return len(m) == 0 || (has && n == nil)
}

// inputRef removes source.<key> (ref_input or supersedes_ref) from src
// and returns the dedupe ref it stands for (9.5).
func inputRef(src map[string]any, key string, dedupeKey []byte, lineNo int) (string, bool, error) {
	raw, has := src[key]
	if !has {
		return "", false, nil
	}
	delete(src, key)
	if dedupeKey == nil {
		return "", true, fmt.Errorf("no identity secret on this replica: line %d", lineNo)
	}
	arr, ok := raw.([]any)
	if !ok {
		return "", true, fmt.Errorf("bad field: source.%s: line %d", key, lineNo)
	}
	return DedupeRef(dedupeKey, arr), true, nil
}

// Purge writes a tombstone for id (or a 6+ hex prefix of it) and erases
// the target at once (7.7).
func (s *Store) Purge(id, reason string) (string, error) {
	res, err := s.PurgeAll(id, reason)
	return res.Tombstone, err
}

// PurgeResult is the output of PurgeAll.
type PurgeResult struct {
	Tombstone string
	// Kept: the target's stored blobs that other record files name, by id.
	// Purge does not erase them (C25), so the files are still readable.
	Kept []KeptBlob
}

// KeptBlob is a blob that purge kept, and the number of other record files
// that name it.
type KeptBlob struct {
	ID    string
	Names int
}

// PurgeAll is Purge with the full result.
func (s *Store) PurgeAll(id, reason string) (PurgeResult, error) {
	var res PurgeResult
	unlock, err := s.lock()
	if err != nil {
		return res, err
	}
	defer unlock()
	l, err := s.load(false)
	if err != nil {
		return res, err
	}
	if l.Local == nil {
		return res, ErrNotInitialized
	}
	target, err := l.Resolve(id)
	if err != nil {
		return res, err
	}
	id = target.ID
	if target.IsIdentity() || target.Type == "bean.tombstone" {
		return res, ErrBadTarget
	}
	if target.Subject != l.Local.Identity {
		return res, ErrNotSubject
	}
	if err := l.canWrite(writer{signer: l.Local.Signer, author: l.Local.Identity}); err != nil {
		return res, err
	}
	t := NewTombstone(l.Local.Identity, l.Local.Signer.Public(), id, reason, Adapter, s.nowString())
	tid, bytes, err := SignRecord(t, l.Local.Signer)
	if err != nil {
		return res, err
	}
	if fr := CheckFile(PathOf(tid), bytes); fr.Err != nil {
		return res, fr.Err
	}
	if _, err := writeIfAbsent(s.path(PathOf(tid)), bytes, 0o600); err != nil {
		return res, err
	}
	if err := s.addPurgedRef(target); err != nil {
		return res, err
	}
	// A local purge erases at once: there is nothing to hold. Erase the
	// target's own blobs, unless another record file names them (C25). Never
	// erase any other blob here. The blobs go before the record file: a
	// failure in between then leaves the record and its tombstone, so the next
	// load hides it and gc erases it. The other order left a blob that no
	// record names, which gc never erases and export writes out.
	delete(l.Files, PathOf(id))
	var candidates []string
	for _, b := range target.Blobs {
		candidates = append(candidates, b.Hash)
	}
	_, kept, err := s.eraseBlobs(l.Files, candidates, true)
	if err != nil {
		return res, err
	}
	for _, p := range []string{s.path(PathOf(id)), hiddenPath(s.Dir, id)} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return res, err
		}
	}
	res.Tombstone = tid
	for _, h := range sortedKeys(kept) {
		res.Kept = append(res.Kept, KeptBlob{ID: h, Names: kept[h]})
	}
	return res, nil
}

// SyncResult is the output of Sync.
type SyncResult struct {
	Records, Blobs, Refused, Replaced int // Replaced counts repaired record files and blobs.
	// BadRecords counts record files of the source that fail their checks, and
	// BadBlobs the blobs of the source that this copy needed and that do not
	// match their hash. Sync copies none of them.
	BadRecords, BadBlobs int
}

// Sync copies from src into this store (SPEC.md 7.4 with change set C13:
// verify through the signature before copying, replace a stored file that
// fails its signature when the incoming one passes). Deletions are judged on
// the union of both folders, never on the source's own verdict: a copy that
// has not seen a rotation takes a retired key's tombstone for valid.
func (s *Store) Sync(src *Store) (SyncResult, error) {
	var res SyncResult
	if !isLog(src) {
		return res, errors.New("no log in " + src.Dir)
	}
	unlock, err := s.lock()
	if err != nil {
		return res, err
	}
	defer unlock()
	// The source is read without its lock and without changes: a sync never
	// writes to the folder it copies from.
	a, err := src.loadReadOnly()
	if err != nil {
		return res, err
	}
	// This copy is read from disk, not through the object cache, so a file
	// damaged in place with its size and time kept is seen and repaired.
	b, err := s.load(true)
	if err != nil {
		return res, err
	}
	// Check every file of the source once. The source's own cache is not
	// trusted: it is a file in the source.
	paths := sortedKeys(a.Files)
	results := make([]FileResult, len(paths))
	parallel(len(paths), func(i int) { results[i] = CheckFile(paths[i], a.Files[paths[i]]) })
	good := map[string][]byte{}
	checked := map[string]FileResult{}
	for i, p := range paths {
		if results[i].Err != nil {
			res.BadRecords++ // 7.4: fails a step from 1 to 15
			continue
		}
		good[p], checked[p] = a.Files[p], results[i]
	}
	for p := range a.Report.Errors {
		if _, file := a.Files[p]; !file && !strings.HasPrefix(p, "sha256:") {
			res.BadRecords++ // not a plain file
		}
	}
	// The union, judged once. Every file in it passed its checks in this call,
	// so no signature is checked twice.
	union := overlay(b.Files, nil, good)
	proven := map[string]bool{}
	for p := range union {
		if _, bad := b.Report.Errors[p]; good[p] != nil || !bad {
			proven[p] = true
		}
	}
	u := verifyFiles(union, proven)
	tombsOf := map[string][]string{} // target -> tombstones in the union
	for _, id := range sortedKeys(u.Records) {
		if r := u.Records[id]; r.Type == "bean.tombstone" {
			tombsOf[r.Targets("purges")[0]] = append(tombsOf[r.Targets("purges")[0]], id)
		}
	}
	// undone: the source hid id, and the union judges every tombstone of it
	// invalid. Anything else keeps it hidden (fail closed).
	undone := func(id string) bool {
		for _, tid := range tombsOf[id] {
			if _, bad := u.Errors[tid]; !bad {
				return false
			}
		}
		return len(tombsOf[id]) > 0
	}
	refs, err := s.openPurgedRefs()
	if err != nil {
		return res, err
	}
	// Seal continuations go first, so a sync that stops halfway never
	// leaves a succession without its parts (C28). Order changes nothing
	// else: the result is a set union.
	order := sortedKeys(good)
	sort.SliceStable(order, func(i, j int) bool { return isSealPath(a, order[i]) && !isSealPath(a, order[j]) })
	for _, p := range order {
		data := good[p]
		fr := checked[p]
		id := fr.Record.ID
		if have, ok := b.Files[p]; ok {
			if string(have) == string(data) {
				continue
			}
			if _, bad := b.Report.Errors[p]; bad { // C13: replace a bad copy where it is
				dest := s.path(p)
				if _, err := os.Lstat(dest); err != nil && b.Hidden[id] {
					dest = hiddenPath(s.Dir, id)
				}
				if err := writeFileAtomic(dest, data, 0o600); err != nil {
					return res, err
				}
				res.Replaced++
			}
			continue
		}
		if b.Hidden[id] {
			continue // 3.3
		}
		if _, purged := u.Purged[id]; purged { // 3.2 and 7.5
			refs.add(fr.Record)
			res.Refused++
			continue
		}
		if a.Hidden[id] && !undone(id) {
			continue
		}
		if err := writeFileAtomic(s.path(p), data, 0o600); err != nil {
			return res, err
		}
		res.Records++
	}
	if err := refs.flush(); err != nil {
		return res, err
	}
	// Blobs (7.4 step 4 as changed by C25): named by a record file of the
	// union that is not purged or hidden, valid now or not. A blob that no
	// record names, such as one from blob put, is not copied.
	b2, err := s.load(false)
	if err != nil {
		return res, err
	}
	// A blob that a deleted record names is copied only when a live record of
	// the person or a paired agent names it: any record can name any id, so a
	// stranger's record does not bring back a purged photo (DECISIONS 213).
	named := map[string]bool{}
	ofDeleted, ofOwnLive := map[string]bool{}, map[string]bool{}
	unlocks := blobUnlocker(b2.ID, b2.Report, b2.AgentLabels())
	recs := b2.allRecords()
	for p, data := range b2.Files {
		m := pathPattern.FindStringSubmatch(p)
		if m == nil {
			continue
		}
		id := "sha256:" + m[1] + m[2]
		if _, purged := b2.Report.Purged[id]; purged || b2.Hidden[id] {
			for _, h := range namedBlobs(data) {
				ofDeleted[h] = true
			}
			continue
		}
		own := recs[id] != nil && unlocks(recs[id])
		for _, h := range namedBlobs(data) {
			named[h] = true
			ofOwnLive[h] = ofOwnLive[h] || own
		}
	}
	// A purged record erased here is still in the source: judge the union.
	for p, data := range union {
		m := pathPattern.FindStringSubmatch(p)
		if m == nil {
			continue
		}
		id := "sha256:" + m[1] + m[2]
		if _, purged := u.Purged[id]; purged || b.Hidden[id] || (a.Hidden[id] && !undone(id)) {
			for _, h := range namedBlobs(data) {
				ofDeleted[h] = true
			}
		}
	}
	for h := range ofDeleted {
		if !ofOwnLive[h] {
			delete(named, h)
		}
	}
	blobFiles, _, err := listShards(src.Dir, dirBlobs)
	if err != nil {
		return res, err
	}
	for _, f := range blobFiles {
		p := f.rel
		id := blobIDOfPath(p)
		if id == "" || !named[id] {
			continue
		}
		// A present file is not necessarily the blob named by its path.
		// Check it before skipping, just as record sync checks signatures.
		dest := s.path(p)
		repair := false
		if info, err := os.Lstat(dest); err == nil {
			if info.Mode().IsRegular() {
				got, err := fileHashID(dest)
				if err != nil {
					return res, err
				}
				if got == id {
					continue
				}
			}
			repair = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return res, err
		}
		data, err := os.ReadFile(src.path(p))
		if err != nil {
			return res, err
		}
		if HashID(data) != id {
			res.BadBlobs++
			continue // Never replace a damaged copy with a damaged source.
		}
		if err := writeFileAtomic(dest, data, 0o600); err != nil {
			return res, err
		}
		if repair {
			res.Replaced++
		} else {
			res.Blobs++
		}
	}
	return res, nil
}

// isLog reports whether a folder holds a log: an identity, or an objects/
// folder (an agent-only copy before agent init).
func isLog(s *Store) bool {
	if _, err := s.Identity(); err == nil {
		return true
	}
	info, err := os.Stat(s.path(dirObjects))
	return err == nil && info.IsDir()
}

// fileHashID checks a stored blob without reading its entire content into memory.
func fileHashID(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var h hash.Hash = sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ExportResult is the output of ExportAll. Every list is sorted.
type ExportResult struct {
	Records, Blobs int // files written to records/ and blobs/
	// Invalid: record files that fail their checks. Their bytes are copied as
	// they are to invalid/<path>, so nothing is left behind in silence.
	Invalid []string
	// Unreferenced: blobs that no record file names, such as one from blob put.
	// They are in blobs/ too.
	Unreferenced []string
	// NotExported: blob id -> why it was not decrypted. A record that is not
	// the person's or a paired agent's, or that is not valid, can name any
	// blob, also one the person deleted.
	NotExported map[string]string
	// Missing: blobs that a record of the person or a paired agent names and
	// that are not here or do not read.
	Missing []string
	// MissingOthers counts absent blobs that only other records name. Anyone
	// can name any number of them, so they are not listed.
	MissingOthers int
	// Withheld: record files that fail their checks and whose id a tombstone
	// names. They are a damaged copy of a deleted record, so they stay behind.
	Withheld []string
	Written  bool // the manifest was written: the export ran to its end
}

// allRecords returns every record file that passes steps 1 to 15, by id: the
// judged ones, those of too-new identities, and those of an unknown version.
func (l *Loaded) allRecords() map[string]*Record {
	all := map[string]*Record{}
	for id, r := range l.Report.Records {
		all[id] = r
	}
	for id, r := range l.Report.Dropped {
		all[id] = r
	}
	for _, r := range l.Report.Unknown {
		all[r.ID] = r
	}
	return all
}

// blobUnlocker returns whether a record may decrypt the blobs it names. The
// person (owner) or a paired agent (labels) signed it, and it is valid, kept
// or in doubt in vr, or vr cannot judge it (a newer format, a too-new
// identity). The author field of a record that vr cannot judge is only a
// string, so its key must be a key of the person's tree, or the paired
// agent's own key (DECISIONS 211).
func blobUnlocker(owner string, vr *Report, labels map[string]string) func(*Record) bool {
	return func(r *Record) bool {
		if r.Author != owner && labels[r.Author] == "" {
			return false
		}
		if vr.Records[r.ID] == nil {
			if r.Author == owner {
				return vr.signedByTree(owner, r.Kid)
			}
			return r.Kid == r.Author
		}
		return (vr.Valid[r.ID] || vr.Kept[r.ID] || vr.InDoubt(r.ID)) && !vr.OffTree(r)
	}
}

// Export writes a plaintext copy (7.9). dir must be empty or absent.
func (s *Store) Export(dir string) (records, blobs int, err error) {
	res, err := s.ExportAll(dir)
	return res.Records, res.Blobs, err
}

// ExportAll is Export with the full result.
func (s *Store) ExportAll(dir string) (ExportResult, error) {
	res := ExportResult{NotExported: map[string]string{}}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return res, errors.New("export dir not empty: " + dir)
	}
	unlock, err := s.lock()
	if err != nil {
		return res, err
	}
	defer unlock()
	l, err := s.load(false)
	if err != nil {
		return res, err
	}
	if l.ID == "" { // a copy without a signing key can still leave
		return res, ErrNotInitialized
	}
	// Every record file goes out, also of a too-new identity and of a newer
	// format: an export needs no judgment of them, and it is the way out.
	all := l.allRecords()
	// A halt by the head key alone can be set aside to judge deletions. Without
	// that view, a record could be left out on a tombstone that is not valid.
	view := l
	if l.IsTooNew() && len(l.Report.HeadHalt[l.ID]) > 0 {
		view = l.past(l.ID)
	}
	labels := view.AgentLabels()
	unlocks := blobUnlocker(l.ID, view.Report, labels)
	type wanted struct {
		mediaType string
		own       bool // a record that unlocks names it
	}
	want := map[string]*wanted{}
	other := map[string]string{} // blob -> reason, for blobs that no record unlocks
	for _, id := range sortedKeys(all) {
		r := all[id]
		if _, purged := view.Report.Purged[id]; purged || view.Hidden[id] {
			continue
		}
		if err := writeFileAtomic(filepath.Join(dir, "records", id[7:]+".json"), r.Bytes, 0o600); err != nil {
			return res, err
		}
		res.Records++
		named := r.Blobs
		if r.V != 0 { // a newer format: its fields were not read
			for _, h := range namedBlobs(r.Bytes) {
				if IsID(h) {
					named = append(named, Blob{Hash: h, MediaType: "application/octet-stream"})
				}
			}
		}
		own := unlocks(r)
		for _, bl := range named {
			w := want[bl.Hash]
			if w == nil {
				w = &wanted{mediaType: bl.MediaType}
				want[bl.Hash] = w
			}
			w.own = w.own || own
			if !own && other[bl.Hash] == "" {
				other[bl.Hash] = "named only by a record that is not valid"
				if r.Author != l.ID && labels[r.Author] == "" {
					other[bl.Hash] = "named only by a record that is not yours or a paired agent's"
				} else if view.Report.Records[r.ID] == nil {
					other[bl.Hash] = "named only by a record this build cannot judge, signed by a key that is not yours or a paired agent's"
				}
			}
		}
	}
	// Record files that fail their checks go out as they are, beside the rest.
	// A deleted one stays behind: hidden, or named by any tombstone of the log,
	// held or not valid included. A damaged copy of a purged record cannot be
	// judged, and its id is in the tombstone.
	tombed := map[string]bool{}
	for _, src := range []map[string]*Record{l.Report.Records, l.Report.Dropped} {
		for _, r := range src {
			if r.Type == "bean.tombstone" {
				for _, t := range r.Targets("purges") {
					tombed[t] = true
				}
			}
		}
	}
	invalid := map[string]string{} // path -> error
	for p, e := range l.Report.Errors {
		if strings.HasPrefix(p, "sha256:") {
			continue
		}
		if m := pathPattern.FindStringSubmatch(p); m != nil {
			if pid := "sha256:" + m[1] + m[2]; l.Hidden[pid] {
				continue
			} else if tombed[pid] {
				res.Withheld = append(res.Withheld, p)
				continue
			}
		}
		invalid[p] = e
	}
	sort.Strings(res.Withheld)
	for _, p := range sortedKeys(invalid) {
		res.Invalid = append(res.Invalid, p)
		data, ok := l.Files[p]
		if !ok {
			continue // not a plain file: nothing to copy
		}
		if err := writeFileAtomic(filepath.Join(dir, "invalid", filepath.FromSlash(p)), data, 0o600); err != nil {
			return res, err
		}
		for _, h := range namedBlobs(data) {
			if IsID(h) && want[h] == nil && other[h] == "" {
				other[h] = "named only by a record file that fails its checks"
			}
		}
	}
	// Blobs that no record file names at all, such as one from blob put. A blob
	// of a deleted record is named by its hidden file, so it stays behind.
	blobFiles, _, err := listShards(s.Dir, dirBlobs)
	if err != nil {
		return res, err
	}
	namedAny := map[string]bool{}
	for _, data := range l.Files {
		for _, h := range namedBlobs(data) {
			namedAny[h] = true
		}
	}
	unreferenced := map[string]bool{}
	for _, f := range blobFiles {
		if id := blobIDOfPath(f.rel); id != "" && !namedAny[id] {
			unreferenced[id] = true
		}
	}
	type manBlob struct{ id, sha, mt string }
	blobSet := map[string]manBlob{}
	missing := map[string]bool{}
	var missingErr error
	// write decrypts one blob into the export. readErr is about the blob here,
	// err about the export folder.
	write := func(id, mediaType string) (readErr, err error) {
		pt, readErr := s.GetBlob(id)
		if readErr != nil {
			return readErr, nil
		}
		if err := writeFileAtomic(filepath.Join(dir, "blobs", id[7:]), pt, 0o600); err != nil {
			return nil, err
		}
		sum := sha256.Sum256(pt)
		blobSet[id] = manBlob{id, hex.EncodeToString(sum[:]), mediaType}
		res.Blobs++
		return nil, nil
	}
	for _, id := range sortedKeys(want) {
		w := want[id]
		if _, err := os.Lstat(s.path(BlobPathOf(id))); errors.Is(err, os.ErrNotExist) || !IsID(id) {
			// Any stranger can name any number of absent blobs: only the
			// person's and paired agents' are listed, the rest counted.
			if !w.own {
				res.MissingOthers++
				continue
			}
			missing[id] = true
			if missingErr == nil {
				missingErr = errors.New("blob missing: " + id)
			}
			continue
		}
		if !w.own {
			continue // in other, with its reason
		}
		readErr, err := write(id, w.mediaType)
		if err != nil {
			return res, err
		}
		if readErr != nil {
			// One file must not stop the way out. The manifest names it, and the
			// command fails at the end.
			missing[id] = true
			if missingErr == nil {
				missingErr = readErr
			}
		}
	}
	for _, id := range sortedKeys(unreferenced) {
		readErr, err := write(id, "application/octet-stream")
		if err != nil {
			return res, err
		}
		if readErr != nil {
			other[id] = "no record names it, and it does not read: " + readErr.Error()
			continue
		}
		res.Unreferenced = append(res.Unreferenced, id)
	}
	// Only files stored here are "not exported": a record can name any id.
	for id, why := range other {
		if _, done := blobSet[id]; done || missing[id] || !IsID(id) {
			continue
		}
		if _, err := os.Lstat(s.path(BlobPathOf(id))); err != nil {
			continue
		}
		res.NotExported[id] = why
	}
	res.Missing = sortedKeys(missing)
	var list []any
	for _, id := range sortedKeys(blobSet) {
		mb := blobSet[id]
		list = append(list, map[string]any{"id": mb.id, "plaintext_sha256": mb.sha, "media_type": mb.mt})
	}
	if list == nil {
		list = []any{}
	}
	man := map[string]any{"spec": "bean-v0", "identity": l.ID, "records": float64(res.Records), "blobs": list}
	strs := func(ids []string) []any {
		var out []any
		for _, id := range ids {
			out = append(out, id)
		}
		return out
	}
	if len(res.Missing) > 0 {
		man["missing_blobs"] = strs(res.Missing)
	}
	if res.MissingOthers > 0 {
		man["missing_blobs_of_others"] = float64(res.MissingOthers)
	}
	if len(res.Withheld) > 0 {
		var m []any
		for _, p := range res.Withheld {
			m = append(m, map[string]any{"path": p, "reason": "deleted: a tombstone names this id"})
		}
		man["withheld_records"] = m
	}
	if len(res.Unreferenced) > 0 {
		man["unreferenced_blobs"] = strs(res.Unreferenced)
	}
	if len(res.NotExported) > 0 {
		var m []any
		for _, id := range sortedKeys(res.NotExported) {
			m = append(m, map[string]any{"id": id, "reason": res.NotExported[id]})
		}
		man["not_exported_blobs"] = m
	}
	if len(res.Invalid) > 0 {
		var m []any
		for _, p := range res.Invalid {
			m = append(m, map[string]any{"path": p, "error": invalid[p]})
		}
		man["invalid_records"] = m
	}
	if err := writeFileAtomic(filepath.Join(dir, "manifest.json"), Canonical(man), 0o600); err != nil {
		return res, err
	}
	res.Written = true
	if l.IsTooNew() {
		return res, l.TooNewError()
	}
	return res, missingErr
}

// ImportResult is the output of ImportExportAll.
type ImportResult struct {
	Records, Blobs int
	Refused        int // records that a valid deletion names (as in Sync)
}

// ImportExport reverses Export into a log with the same identity secret.
func (s *Store) ImportExport(dir string) (records, blobs int, err error) {
	res, err := s.ImportExportAll(dir)
	return res.Records, res.Blobs, err
}

// ImportExportAll is ImportExport with the full result. It follows the rule of
// Sync: a record that a valid deletion of the log and the export together
// names is refused, and so is a blob that only deleted records name. Export
// directories are untrusted input. Read them through os.Root so a changed
// symlink cannot escape the selected folder.
func (s *Store) ImportExportAll(dir string) (ImportResult, error) {
	var res ImportResult
	unlock, err := s.lock()
	if err != nil {
		return res, err
	}
	defer unlock()
	local, err := s.ReadLocal()
	if err != nil {
		return res, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return res, err
	}
	defer root.Close()
	// An export says whose it is. Another identity's blobs do not decrypt
	// here, and its records are not this log's.
	who, err := readManifestIdentity(root)
	if errors.Is(err, os.ErrNotExist) {
		return res, errors.New("not an export: no manifest.json in " + dir)
	}
	if err != nil {
		return res, err
	}
	if who != local.Identity {
		return res, fmt.Errorf("export of identity %s, this log is %s", who, local.Identity)
	}
	blobEntries, err := exportEntries(dir, "blobs", true)
	if err != nil {
		return res, err
	}
	recordEntries, err := exportEntries(dir, "records", false)
	if err != nil {
		return res, err
	}
	secret, err := s.identitySecret()
	if err != nil {
		return res, err
	}
	incoming := map[string][]byte{}
	checked := map[string]*Record{}
	for _, e := range recordEntries {
		data, err := readExportFile(root, "records/"+e.Name(), MaxSuccessionSize)
		if err != nil {
			return res, err
		}
		p := PathOf("sha256:" + strings.TrimSuffix(e.Name(), ".json"))
		fr := CheckFile(p, data)
		if fr.Err != nil {
			return res, fmt.Errorf("%s: %s", e.Name(), fr.Err)
		}
		incoming[p], checked[p] = data, fr.Record
	}
	l, err := s.load(false)
	if err != nil {
		return res, err
	}
	// Deletions are judged on the union, as in Sync.
	union := overlay(l.Files, nil, incoming)
	u := verifyFiles(union, nil)
	deleted := func(id string) bool {
		_, purged := u.Purged[id]
		return purged || l.Hidden[id]
	}
	// A blob that a deleted record names comes back only when a live record of
	// the person or a paired agent names it. Any record can name any id, so a
	// stranger's live record does not bring back a purged photo.
	ul := *l
	ul.Files, ul.Report = union, u
	unlocks := blobUnlocker(l.ID, u, ul.AgentLabels())
	recs := ul.allRecords()
	ofDeleted, ofOwnLive := map[string]bool{}, map[string]bool{}
	for p, data := range union {
		m := pathPattern.FindStringSubmatch(p)
		if m == nil {
			continue
		}
		id := "sha256:" + m[1] + m[2]
		del := deleted(id)
		own := false
		if r := recs[id]; r != nil && !del {
			own = unlocks(r)
		}
		for _, h := range namedBlobs(data) {
			ofDeleted[h] = ofDeleted[h] || del
			ofOwnLive[h] = ofOwnLive[h] || own
		}
	}
	for _, e := range blobEntries {
		id := "sha256:" + e.Name()
		if ofDeleted[id] && !ofOwnLive[id] {
			continue // a deleted record names it, and no live record of the person
		}
		pt, err := readExportFile(root, "blobs/"+e.Name(), maxImportBlobBytes)
		if err != nil {
			return res, err
		}
		out, err := EncryptBlob(BlobKey(secret), pt)
		if err != nil {
			return res, err
		}
		if HashID(out) != id {
			return res, errors.New("blob corrupt: " + id)
		}
		wrote, err := writeIfAbsent(s.path(BlobPathOf(id)), out, 0o600)
		if err != nil {
			return res, err
		}
		if wrote {
			res.Blobs++
		}
	}
	refs, err := s.openPurgedRefs()
	if err != nil {
		return res, err
	}
	for _, p := range sortedKeys(incoming) {
		if _, have := l.Files[p]; have {
			continue // hidden records too (3.3)
		}
		r := checked[p]
		if _, purged := u.Purged[r.ID]; purged { // 3.2 and 7.5
			refs.add(r)
			res.Refused++
			continue
		}
		wrote, err := writeIfAbsent(s.path(p), incoming[p], 0o600)
		if err != nil {
			return res, err
		}
		if wrote {
			res.Records++
		}
	}
	return res, refs.flush()
}

// maxManifestBytes bounds the manifest that import-export reads. It is read
// as a stream, so memory does not grow with it; the bound only stops a
// folder that never ends. One blob entry is under 200 bytes.
const maxManifestBytes int64 = 4 << 30

// readManifestIdentity returns the identity of an export's manifest.json. It
// reads the file as a stream and keeps only that field: the lists of a log
// with many blobs do not fit any small cap, and nothing else is needed. The
// manifest must be one JSON object with exactly one "identity" string.
func readManifestIdentity(root *os.Root) (string, error) {
	bad := errors.New("bad field: manifest.json identity")
	f, err := root.Open("manifest.json")
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("bad path: manifest.json")
	}
	if info.Size() > maxManifestBytes {
		return "", errors.New("too large: manifest.json")
	}
	dec := json.NewDecoder(bufio.NewReader(io.LimitReader(f, maxManifestBytes)))
	dec.UseNumber()
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return "", bad
	}
	who, seen := "", false
	for dec.More() {
		t, err := dec.Token()
		key, ok := t.(string)
		if err != nil || !ok {
			return "", bad
		}
		if key != "identity" {
			if err := skipJSONValue(dec); err != nil {
				return "", bad
			}
			continue
		}
		v, err := dec.Token()
		s, ok := v.(string)
		if err != nil || !ok || seen {
			return "", bad
		}
		who, seen = s, true
	}
	if t, err := dec.Token(); err != nil || t != json.Delim('}') {
		return "", bad
	}
	if _, err := dec.Token(); err != io.EOF {
		return "", bad // more after the object
	}
	if !IsID(who) {
		return "", bad
	}
	return who, nil
}

// skipJSONValue reads one value of dec without keeping it.
func skipJSONValue(dec *json.Decoder) error {
	depth := 0
	for {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		switch t {
		case json.Delim('{'), json.Delim('['):
			depth++
		case json.Delim('}'), json.Delim(']'):
			depth--
		}
		if depth == 0 {
			return nil
		}
	}
}

// The reference import command supports 256 MiB raw blobs. The core
// format does not impose an upper bound on blobs (SPEC 4.9).
const maxImportBlobBytes int64 = 256 << 20

// exportEntries rejects anything except regular files in ordinary
// directories, including symbolic links and special files.
func exportEntries(root, sub string, optional bool) ([]os.DirEntry, error) {
	p := filepath.Join(root, sub)
	info, err := os.Lstat(p)
	if errors.Is(err, os.ErrNotExist) && optional {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("bad path: " + sub)
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			return nil, errors.New("bad path: " + sub + "/" + e.Name())
		}
		if sub == "blobs" && !IsID("sha256:"+e.Name()) {
			return nil, errors.New("bad path: " + sub + "/" + e.Name())
		}
		if sub == "records" && (!strings.HasSuffix(e.Name(), ".json") || !IsID("sha256:"+strings.TrimSuffix(e.Name(), ".json"))) {
			return nil, errors.New("bad path: " + sub + "/" + e.Name())
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		max := maxImportBlobBytes
		if sub == "records" {
			max = MaxSuccessionSize
		}
		if info.Size() > max {
			return nil, errors.New("too large: " + sub + "/" + e.Name())
		}
	}
	return entries, nil
}

// readExportFile performs the size and regular-file checks again on the
// opened handle; a directory listing's size is not a trusted bound.
func readExportFile(root *os.Root, name string, max int64) ([]byte, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("bad path: " + name)
	}
	if info.Size() > max {
		return nil, errors.New("too large: " + name)
	}
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, errors.New("too large: " + name)
	}
	return data, nil
}

// isSealPath reports whether p holds a bean.seal record of l.
func isSealPath(l *Loaded, p string) bool {
	m := pathPattern.FindStringSubmatch(p)
	if m == nil {
		return false
	}
	r := l.Report.Records["sha256:"+m[1]+m[2]]
	return r != nil && r.Type == "bean.seal"
}
