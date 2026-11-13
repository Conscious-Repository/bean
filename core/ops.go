package core

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
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

// AppendImported is Append for the output of an importer that bean itself
// ran (bean import). The person's key signs it, so each line is held to
// what an importer may write (importable): a reading about the person, and
// nothing with power over another record.
func (s *Store) AppendImported(in io.Reader, corrections bool) (AppendResult, error) {
	return s.appendFrom(in, false, corrections, true)
}

func (s *Store) appendAs(in io.Reader, agent, corrections bool) (AppendResult, error) {
	return s.appendFrom(in, agent, corrections, false)
}

func (s *Store) appendFrom(in io.Reader, agent, corrections, importer bool) (AppendResult, error) {
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
	w.corrections, w.importer = corrections, importer
	return s.appendWith(l, w, in)
}

// importable reports why a partial record is not one that an importer may
// write, or nil. An importer reads a provider's file and writes readings
// about the person (docs/IMPORTERS.md). bean signs its output with the
// person's key. So its record is of no type that only the person or bean
// writes, is about nobody else, and names no other record: it deletes
// nothing, corrects nothing by id or by ref, and pairs no key.
func importable(obj map[string]any, identity string) error {
	typ, _ := obj["type"].(string)
	if IsCoreType(typ) || typ == "agent.pair" || typ == "share" || typ == "reply" {
		return fmt.Errorf("an importer writes readings, not a record of type %q", typ)
	}
	if subject, has := obj["subject"]; has && subject != identity {
		return errors.New("an importer writes about the person of this log, and about nobody else")
	}
	if refs, has := obj["refs"]; has {
		if list, ok := refs.([]any); !ok || len(list) > 0 {
			return errors.New("an importer's record names no other record: it deletes and corrects nothing by id")
		}
	}
	// Nor by ref. A changed value of the same reading is a correction that
	// the person passes with -corrections, after they saw it. A line that
	// names the reading it replaces would pass without that.
	if src, _ := obj["source"].(map[string]any); src != nil {
		if _, has := src["supersedes_ref"]; has {
			return errors.New("an importer's record carries no source.supersedes_ref: a changed reading is a correction that you pass with -corrections")
		}
	}
	return nil
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
	// importer: the input is the output of an importer that bean ran.
	importer bool
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
	// values holds every value a ref has had, with its type, so a value the
	// log already knows is skipped, also after a correction, but only under
	// the current record's type. currents holds the records of a ref
	// that no record supersedes, and existing is the one that supersedes_ref
	// names: the first current one, or, when a correction without this ref
	// replaced them all, the first one that has it.
	existing := map[string]string{}
	currents := map[string][]string{}
	currentTypes := map[string]map[string]bool{} // types of the current records of a ref
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
		values[key][typedValue(r.Type, r.Raw)] = true
		if len(l.Report.Superseded[id]) == 0 {
			if currents[key] == nil {
				existing[key] = id
				currentTypes[key] = map[string]bool{}
			}
			currents[key] = append(currents[key], id)
			currentTypes[key][r.Type] = true
		}
	}
	seen := map[string]string{} // ref -> the reading of the first line of this input that has it
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), MaxRecordSize+1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		obj, src, err := s.completePartial(w, []byte(line), dedupeKey, lineNo)
		if err != nil {
			return res, err
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
			typ, _ := obj["type"].(string)
			reading := typedValue(typ, obj)
			if purged[key] {
				res.Skipped++
				continue
			}
			// The first line of an input decides for its ref. A later line
			// with the same reading is a repeat; one with another reading
			// would be lost, so the input is refused there.
			if first, ok := seen[key]; ok {
				if first != reading {
					return res, fmt.Errorf("line %d: another line of this input has the same source id with another type, value or time; give each reading its own id", lineNo)
				}
				res.Skipped++
				continue
			}
			seen[key] = reading
			if known := values[key]; known != nil && !supersedes[existing[key]] {
				value := string(Canonical(obj["value"]))
				current := l.Report.Records[existing[key]]
				// Already there: the ref has had this type, value and time,
				// and a current record of the ref has this type. A reading
				// of another type or time under the same ref is never
				// skipped in silence.
				// With no current record (a correction without this ref
				// replaced them all), the type is the reading's own.
				ownType := currentTypes[key][typ] || len(currents[key]) == 0 && typ == current.Type
				if known[reading] && ownType {
					res.Skipped++
					continue
				}
				reason := ""
				switch {
				case !ownType:
					reason = "its type differs, so it is not the same reading. Two sources may share ids: give each its own -system"
				case !sameObserved(obj, current.Raw):
					reason = "its time differs, so it may be another reading under the same id. Two files may number their rows alike: give each its own -system"
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

// completePartial turns one partial record line (SPEC.md 9.2) into the
// record that w would sign: version, nonce, author, kid, default subject,
// import time and the ref from ref_input. Append and CheckPartial both use
// it, so a check before a write sees what the write signs.
func (s *Store) completePartial(w writer, line, dedupeKey []byte, lineNo int) (obj, src map[string]any, err error) {
	v, perr := Parse(line)
	obj, ok := v.(map[string]any)
	if perr != nil || !ok {
		return nil, nil, fmt.Errorf("bad json: line %d", lineNo)
	}
	for _, k := range forbiddenPartial {
		if _, has := obj[k]; has {
			return nil, nil, fmt.Errorf("bad field: %s: line %d", k, lineNo)
		}
	}
	if w.importer {
		if err := importable(obj, w.identity); err != nil {
			return nil, nil, fmt.Errorf("%w: line %d", err, lineNo)
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
	src, _ = obj["source"].(map[string]any)
	if src == nil {
		return nil, nil, fmt.Errorf("bad field: source: line %d", lineNo)
	}
	ref, hasInput, err := inputRef(src, "ref_input", dedupeKey, lineNo)
	if err != nil {
		return nil, nil, err
	}
	if hasInput {
		src["ref"] = ref
	}
	if _, has := src["ref"]; !has {
		src["ref"] = nil
	}
	return obj, src, nil
}

// typedValue is the key of Append's value history: the type, the canonical
// value and the canonical observed time of a record or a partial record. A
// type holds no NUL, and canonical JSON has none raw, so they cannot run
// together. The time counts: two files that number their rows alike give
// two readings one id, and the second is not "already there".
func typedValue(typ string, raw map[string]any) string {
	return typ + "\x00" + string(Canonical(raw["value"])) + "\x00" + observedOf(raw)
}

func observedOf(raw map[string]any) string {
	times, _ := raw["times"].(map[string]any)
	return string(Canonical(times["observed"]))
}

func sameObserved(a, b map[string]any) bool { return observedOf(a) == observedOf(b) }

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

// KeptBlob is a blob that purge kept. Names counts the other records that
// name it and that purge can reach. Others counts the other record files that
// name it and that purge cannot reach: deleted records waiting for gc,
// records about someone else, and files that fail their checks.
type KeptBlob struct {
	ID     string
	Names  int
	Others int
}

// PurgeAll is Purge with the full result.
func (s *Store) PurgeAll(id, reason string) (PurgeResult, error) {
	unlock, err := s.lock()
	if err != nil {
		return PurgeResult{}, err
	}
	defer unlock()
	l, err := s.load(false)
	if err != nil {
		return PurgeResult{}, err
	}
	return s.purgeLoaded(l, id, reason)
}

// PurgeMany purges each of ids as Purge does, under one lock and one load
// of the log, and returns how many it purged. It stops at the first that
// fails. Purging many records one by one reads the whole log for each. The
// caller chose ids from an earlier load, so a record that is no longer
// current, because another command deleted it since, is passed over.
func (s *Store) PurgeMany(ids []string, reason string) (int, error) {
	unlock, err := s.lock()
	if err != nil {
		return 0, err
	}
	defer unlock()
	l, err := s.load(false)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if !l.Current(id) {
			continue
		}
		if _, err := s.purgeLoaded(l, id, reason); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// purgeLoaded purges one record of the loaded log. The caller holds the
// lock. It keeps l.Files true to the folder, so a second call on the same
// load sees what the first erased.
func (s *Store) purgeLoaded(l *Loaded, id, reason string) (PurgeResult, error) {
	var res PurgeResult
	if l.Local == nil {
		return res, ErrNotInitialized
	}
	// A record this build cannot judge is the person's to delete too: a
	// tombstone may name any id (step 10 holds it), and such a record can
	// keep a file (DECISIONS 217).
	all := l.allRecords()
	target, err := resolveIn(all, id)
	if err != nil {
		return res, err
	}
	id = target.ID
	if target.IsIdentity() || target.Type == "bean.tombstone" {
		return res, ErrBadTarget
	}
	if subjectOf(target) != l.Local.Identity {
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
	// A file with a bad path is not a record and keeps no blob. A copy of the
	// target under another name (a sync tool's conflict copy) goes with it.
	delete(l.Files, PathOf(id))
	paths := []string{s.path(PathOf(id)), hiddenPath(s.Dir, id)}
	for _, p := range copiesOf(l.Files, map[string]bool{id: true}) {
		delete(l.Files, p)
		paths = append(paths, s.path(p))
	}
	candidates := namedBlobs(target.Bytes)
	for _, b := range target.Blobs {
		candidates = append(candidates, b.Hash)
	}
	// Only a record that purge can reach is worth naming: one about the person
	// that is not deleted already. The rest still keep the file.
	gone := l.Report.unjudgedPurged()
	reach := func(p string) bool {
		rid := recordFileID(p, nil)
		r := all[rid]
		_, purged := l.Report.Purged[rid]
		return r != nil && !purged && !l.Hidden[rid] && !gone[rid] && !r.IsIdentity() && r.Type != "bean.tombstone" && subjectOf(r) == l.Local.Identity
	}
	_, kept, err := s.eraseBlobs(l.Files, candidates, reach)
	if err != nil {
		return res, err
	}
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return res, err
		}
	}
	res.Tombstone = tid
	for _, h := range sortedKeys(kept) {
		res.Kept = append(res.Kept, kept[h])
	}
	return res, nil
}

// resolveIn is Resolve over records.
func resolveIn(records map[string]*Record, id string) (*Record, error) {
	hexPart := strings.TrimPrefix(id, "sha256:")
	if r, ok := records["sha256:"+hexPart]; ok {
		return r, nil
	}
	if len(hexPart) < 6 || !isLowerHex(hexPart, len(hexPart)) {
		return nil, errors.New("input missing: " + id + ": give a record's id, or its first 6 or more hex digits")
	}
	var found *Record
	for rid, r := range records {
		if strings.HasPrefix(rid[7:], hexPart) {
			if found != nil {
				return nil, errors.New("ambiguous id: " + id)
			}
			found = r
		}
	}
	if found == nil {
		return nil, errors.New("input missing: " + id + ": no record has this id")
	}
	return found, nil
}

// subjectOf is the subject of r. The fields of a newer format are not read,
// so its subject is the string in the file.
func subjectOf(r *Record) string {
	if r.V != 0 {
		s, _ := r.Raw["subject"].(string)
		return s
	}
	return r.Subject
}

// recordFileID is the id that a file of objects/ stands for: the id in its
// path, or, for a file whose name is not a record path (a sync tool's
// conflict copy), the id computed from its bytes. It is "" when there is
// none. The id field of a file with a bad path is not trusted.
func recordFileID(p string, data []byte) string {
	if m := pathPattern.FindStringSubmatch(p); m != nil {
		return "sha256:" + m[1] + m[2]
	}
	v, err := Parse(data)
	obj, ok := v.(map[string]any)
	if err != nil || !ok {
		return ""
	}
	return RecordID(obj)
}

// copiesOf returns the paths of files with a bad path whose bytes stand
// for one of ids, sorted.
func copiesOf(files map[string][]byte, ids map[string]bool) []string {
	var out []string
	for p, data := range files {
		if !pathPattern.MatchString(p) && ids[recordFileID(p, data)] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
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
	u := pastHeadHalt(verifyFiles(union, proven), union, proven, b.ID)
	gone := u.unjudgedPurged()
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
	// leaves a succession without its parts (C28). The other identity
	// records go next: a first sync that stops halfway then leaves records
	// whose author resolves, not a log that lists nothing. Order changes
	// nothing else: the result is a set union.
	order := sortedKeys(good)
	rank := func(p string) int {
		switch {
		case isSealPath(a, p):
			return 0
		case checked[p].Record != nil && checked[p].Record.IsIdentity():
			return 1
		}
		return 2
	}
	sort.SliceStable(order, func(i, j int) bool { return rank(order[i]) < rank(order[j]) })
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
		if _, purged := u.Purged[id]; purged || gone[id] { // 3.2 and 7.5
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
	bv := b2.blobView()
	unlocks := blobUnlocker(b2.ID, bv.Report, bv.fileLabelsPastHalt())
	recs := b2.allRecords()
	gone2 := bv.Report.unjudgedPurged()
	for p, data := range b2.Files {
		m := pathPattern.FindStringSubmatch(p)
		if m == nil {
			continue
		}
		id := "sha256:" + m[1] + m[2]
		if _, purged := bv.Report.Purged[id]; purged || bv.Hidden[id] || gone2[id] {
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
		if _, purged := u.Purged[id]; purged || gone[id] || b.Hidden[id] || (a.Hidden[id] && !undone(id)) {
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
	// Withheld: record files that fail their checks and whose id is hidden or
	// named by a tombstone. For a file with a bad path (a sync tool's conflict
	// copy), the id is the one its bytes give. They are a damaged copy of a
	// deleted record, so they stay behind.
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

// unjudgedPurged returns the ids of records this build cannot judge (a newer
// format, a too-new identity) that a valid tombstone about their subject
// names. Step 10 holds such a tombstone, since its target is not a record it
// judges, but the person deleted the record all the same: sync, import-export
// and export leave it out (DECISIONS 217). It changes no verdict.
func (rp *Report) unjudgedPurged() map[string]bool {
	out := map[string]bool{}
	var unjudged map[string]*Record
	for tid, r := range rp.Records {
		if r.Type != "bean.tombstone" || !rp.Valid[tid] {
			continue
		}
		t := r.Targets("purges")[0]
		if rp.Records[t] != nil {
			continue
		}
		if unjudged == nil {
			unjudged = map[string]*Record{}
			for id, x := range rp.Dropped {
				unjudged[id] = x
			}
			for _, x := range rp.Unknown {
				unjudged[x.ID] = x
			}
		}
		if x := unjudged[t]; x != nil && !x.IsIdentity() && x.Type != "bean.tombstone" && subjectOf(x) == r.Subject {
			out[t] = true
		}
	}
	return out
}

// blobUnlocker returns whether a record may decrypt the blobs it names. The
// person (owner) or a paired agent (labels) signed it, and it is valid, kept
// or in doubt in vr, or vr cannot judge it (a newer format, a too-new
// identity). The author field of a record that vr cannot judge is only a
// string. So the person's such record unlocks only when the gate of 8.2
// trusts its key: the head key, or a recovery key a chain node commits to. A
// retired key is the key a thief holds (DECISIONS 217). A paired agent's
// unlocks only with the agent's own key (DECISIONS 211).
func blobUnlocker(owner string, vr *Report, labels map[string]string) func(*Record) bool {
	return func(r *Record) bool {
		if r.Author != owner && labels[r.Author] == "" {
			return false
		}
		if vr.Records[r.ID] == nil {
			if r.Author == owner {
				trusted, _ := vr.gateTrusts(owner, r.Kid)
				return trusted
			}
			return r.Kid == r.Author
		}
		return (vr.Valid[r.ID] || vr.Kept[r.ID] || vr.InDoubt(r.ID)) && !vr.OffTree(r)
	}
}

// blobView is the view of l that judges deletions and files. A halt by the
// head key alone is set aside, as rotate does: without that, every record of
// the person is unjudged, and only the head key's would unlock. A halt by a
// recovery key is not set aside: only an update can go past the seed holder.
func (l *Loaded) blobView() *Loaded {
	if l.IsTooNew() && len(l.Report.HeadHalt[l.ID]) > 0 {
		return l.past(l.ID)
	}
	return l
}

// pastHeadHalt is blobView for the union that sync and import-export judge:
// when the head key alone halts identity id there, u is judged again without
// the records that halt it. Without that, no deletion of the person counts,
// and a stale copy brings a purged record back.
func pastHeadHalt(u *Report, union map[string][]byte, proven map[string]bool, id string) *Report {
	if id == "" || !contains(u.TooNew, id) || len(u.HeadHalt[id]) == 0 {
		return u
	}
	return verifyFiles(overlay(union, u.HeadHalt[id], nil), proven)
}

// fileLabelsPastHalt is FileLabels, plus, for a too-new identity, the agents that
// its records this build cannot judge pair. Such a pairing counts only when
// the gate trusts its key, and when no other such record of the person
// supersedes it (DECISIONS 217).
func (l *Loaded) fileLabelsPastHalt() map[string]string {
	out := l.FileLabels()
	if !l.IsTooNew() {
		return out
	}
	var pairs []*Record
	superseded := map[string]bool{}
	for _, r := range l.Report.Dropped {
		if r.Author != l.ID || r.Subject != l.ID {
			continue
		}
		if trusted, _ := l.Report.gateTrusts(l.ID, r.Kid); !trusted {
			continue
		}
		for _, t := range r.Targets("supersedes") {
			superseded[t] = true
		}
		if r.Type == "agent.pair" && r.ValueKey() != "" {
			pairs = append(pairs, r)
		}
	}
	// A peer among them unlocks nothing, also when the key has an agent's
	// pairing too: the same rule as FileLabels.
	peer := map[string]bool{}
	for _, r := range pairs {
		st, _ := r.Value["status"].(string)
		role, _ := r.Value["role"].(string)
		if !superseded[r.ID] && st != "unpaired" && role == "peer" {
			peer[r.ValueKey()] = true
		}
	}
	for _, r := range pairs {
		label, _ := r.Value["label"].(string)
		st, _ := r.Value["status"].(string)
		if superseded[r.ID] || st == "unpaired" || out[r.ValueKey()] != "" || peer[r.ValueKey()] {
			continue
		}
		out[r.ValueKey()] = ShownLabel(label)
	}
	return out
}
