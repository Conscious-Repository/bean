package core

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AppendResult is the output of Append.
type AppendResult struct {
	Added, Skipped int
	IDs            []string
}

var forbiddenPartial = []string{"v", "nonce", "author", "kid", "id", "sig"}

// Append reads JSONL partial records (SPEC.md 9.2), completes, dedupes,
// signs and writes them (12.3 hlog append). On the first error it stops;
// records before it stay written.
func (s *Store) Append(in io.Reader) (AppendResult, error) {
	var res AppendResult
	l, err := s.Load()
	if err != nil {
		return res, err
	}
	w, err := s.writerFor(l, false)
	if err != nil {
		return res, err
	}
	return s.appendWith(l, w, in)
}

// writerFor is the person (agent false) or this machine's agent key.
func (s *Store) writerFor(l *Loaded, agent bool) (writer, error) {
	if agent {
		a, err := s.ReadAgent()
		if err != nil {
			return writer{}, err
		}
		return writer{a.Signer, a.Signer.Public(), a.Identity, a.Secret}, nil
	}
	if l.Local == nil {
		if _, err := s.Identity(); err == nil {
			return writer{}, errors.New("no personal key on this replica (agent-only): sign with the agent key instead (-agent)")
		}
		return writer{}, ErrNotInitialized
	}
	return writer{l.Local.Signer, l.Local.Identity, l.Local.Identity, l.Local.Secret}, nil
}

// writer is who signs appended records: the person, or an agent key that
// writes as a bare-key author about the person.
type writer struct {
	signer   Signer
	author   string
	identity string // the default subject
	secret   []byte // nil on a replica without the identity secret
}

func (s *Store) appendWith(l *Loaded, w writer, in io.Reader) (AppendResult, error) {
	var res AppendResult
	var dedupeKey []byte
	if w.secret != nil {
		dedupeKey = DedupeKey(w.secret)
	}
	purged, err := s.readPurgedRefs()
	if err != nil {
		return res, err
	}
	// Existing valid triples and, per 9.7, the ids they belong to.
	existing := map[string]string{}
	for id, r := range l.Report.Records {
		if l.Report.Valid[id] && r.Source.Ref != "" {
			existing[refLine(r.Subject, r.Source.System, r.Source.Ref)] = id
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
		if _, has := src["ref_input"]; has && dedupeKey == nil {
			return res, fmt.Errorf("no identity secret on this replica: line %d", lineNo)
		}
		if ri, has := src["ref_input"]; has {
			arr, ok := ri.([]any)
			if !ok {
				return res, fmt.Errorf("bad field: source.ref_input: line %d", lineNo)
			}
			src["ref"] = DedupeRef(dedupeKey, arr)
			delete(src, "ref_input")
		}
		if _, has := src["ref"]; !has {
			src["ref"] = nil
		}
		// C17: supersede by ref.
		if _, has := src["supersedes_ref"]; has && dedupeKey == nil {
			return res, fmt.Errorf("no identity secret on this replica: line %d", lineNo)
		}
		if sr, has := src["supersedes_ref"]; has {
			arr, ok := sr.([]any)
			if !ok {
				return res, fmt.Errorf("bad field: source.supersedes_ref: line %d", lineNo)
			}
			subject, _ := obj["subject"].(string)
			system, _ := src["system"].(string)
			target, found := existing[refLine(subject, system, DedupeRef(dedupeKey, arr))]
			delete(src, "supersedes_ref")
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
			if prev, has := existing[key]; (has && !supersedes[prev]) || purged[key] || seen[key] {
				res.Skipped++
				continue
			}
			seen[key] = true
		}
		id, bytes, err := SignRecord(obj, w.signer)
		if err != nil {
			return res, err
		}
		if fr := CheckFile(PathOf(id), bytes); fr.Err != nil {
			return res, fmt.Errorf("%s: line %d", fr.Err, lineNo)
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

// Purge writes a tombstone for id (or a 6+ hex prefix of it) and erases
// the target at once (7.7).
func (s *Store) Purge(id, reason string) (string, error) {
	l, err := s.Load()
	if err != nil {
		return "", err
	}
	if l.Local == nil {
		return "", ErrNotInitialized
	}
	target, err := l.Resolve(id)
	if err != nil {
		return "", err
	}
	id = target.ID
	if target.IsIdentity() || target.Type == "bean.tombstone" {
		return "", ErrBadTarget
	}
	if target.Subject != l.Local.Identity {
		return "", ErrNotSubject
	}
	t := NewTombstone(l.Local.Identity, l.Local.Signer.Public(), id, reason, Adapter, s.nowString())
	tid, bytes, err := SignRecord(t, l.Local.Signer)
	if err != nil {
		return "", err
	}
	if fr := CheckFile(PathOf(tid), bytes); fr.Err != nil {
		return "", fr.Err
	}
	if _, err := writeIfAbsent(s.path(PathOf(tid)), bytes, 0o600); err != nil {
		return "", err
	}
	if err := s.addPurgedRef(target); err != nil {
		return "", err
	}
	for _, p := range []string{s.path(PathOf(id)), hiddenPath(s.Dir, id)} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	l.Holds[tid] = Hold{Target: id, Received: s.nowString()}
	// A local purge erases at once; the hold entry records it so gc and
	// restores agree, but there is nothing hidden to erase or restore.
	delete(l.Holds, tid)
	if err := s.writeHolds(l.Holds); err != nil {
		return "", err
	}
	l2, err := s.Load()
	if err != nil {
		return "", err
	}
	if _, err := s.eraseOrphanBlobs(l2); err != nil {
		return "", err
	}
	return tid, nil
}

// SyncResult is the output of Sync.
type SyncResult struct {
	Records, Blobs, Refused, Replaced int
}

// Sync copies from src into this store (SPEC.md 7.4 with change set C13:
// verify through the signature before copying, replace a stored file that
// fails its signature when the incoming one passes).
func (s *Store) Sync(src *Store) (SyncResult, error) {
	var res SyncResult
	a, err := src.Load()
	if err != nil {
		return res, err
	}
	b, err := s.Load()
	if err != nil {
		return res, err
	}
	// Valid tombstones of A and B by target id.
	tombs := map[string]*Record{}
	for _, l := range []*Loaded{a, b} {
		for _, id := range sortedKeys(l.Report.Records) {
			r := l.Report.Records[id]
			if r.Type == "bean.tombstone" && l.Report.Valid[id] {
				tombs[r.Targets("purges")[0]] = r
			}
		}
	}
	for _, p := range sortedKeys(a.Files) {
		data := a.Files[p]
		fr := CheckFile(p, data)
		if fr.Err != nil {
			continue // 3.1: fails steps 1 to 12
		}
		id := fr.Record.ID
		if a.Hidden[id] {
			continue
		}
		if have, ok := b.Files[p]; ok {
			if string(have) == string(data) {
				continue
			}
			if CheckFile(p, have).Err != nil { // C13: replace a bad copy
				if err := writeFileAtomic(s.path(p), data, 0o600); err != nil {
					return res, err
				}
				res.Replaced++
			}
			continue
		}
		if b.Hidden[id] {
			continue // 3.3
		}
		if t, ok := tombs[id]; ok && !fr.Unknown { // 3.2 and 7.5
			target := fr.Record
			if !target.IsIdentity() && target.Type != "bean.tombstone" && target.Subject == t.Subject {
				if err := s.addPurgedRef(target); err != nil {
					return res, err
				}
				res.Refused++
				continue
			}
		}
		if err := writeFileAtomic(s.path(p), data, 0o600); err != nil {
			return res, err
		}
		res.Records++
	}
	// Blobs (7.4 step 4): named by a record of the union that is not purged
	// or hidden.
	b2, err := s.Load()
	if err != nil {
		return res, err
	}
	named := map[string]bool{}
	for id, r := range b2.Report.Records {
		if !b2.Visible(id) {
			continue
		}
		for _, bl := range r.Blobs {
			named[bl.Hash] = true
		}
	}
	paths, err := listShardFiles(src.Dir, dirBlobs)
	if err != nil {
		return res, err
	}
	for _, p := range paths {
		id := blobIDOfPath(p)
		if id == "" || !named[id] {
			continue
		}
		if _, err := os.Stat(s.path(p)); err == nil {
			continue
		}
		data, err := os.ReadFile(src.path(p))
		if err != nil {
			return res, err
		}
		if HashID(data) != id {
			continue
		}
		if err := writeFileAtomic(s.path(p), data, 0o600); err != nil {
			return res, err
		}
		res.Blobs++
	}
	return res, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Export writes a plaintext copy (7.9). dir must be empty or absent.
func (s *Store) Export(dir string) (records, blobs int, err error) {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return 0, 0, errors.New("export dir not empty")
	}
	l, err := s.Load()
	if err != nil {
		return 0, 0, err
	}
	if l.Local == nil {
		return 0, 0, ErrNotInitialized
	}
	type manBlob struct{ id, sha, mt string }
	blobSet := map[string]manBlob{}
	for _, id := range sortedKeys(l.Report.Records) {
		r := l.Report.Records[id]
		if _, purged := l.Report.Purged[id]; purged || l.Hidden[id] {
			continue
		}
		if err := writeFileAtomic(filepath.Join(dir, "records", id[7:]+".json"), r.Bytes, 0o600); err != nil {
			return 0, 0, err
		}
		records++
		for _, bl := range r.Blobs {
			if _, done := blobSet[bl.Hash]; done {
				continue
			}
			pt, err := s.GetBlob(bl.Hash)
			if err != nil {
				return 0, 0, err
			}
			if err := writeFileAtomic(filepath.Join(dir, "blobs", bl.Hash[7:]), pt, 0o600); err != nil {
				return 0, 0, err
			}
			sum := sha256.Sum256(pt)
			blobSet[bl.Hash] = manBlob{bl.Hash, hex.EncodeToString(sum[:]), bl.MediaType}
			blobs++
		}
	}
	var list []any
	for _, id := range sortedKeys(blobSet) {
		mb := blobSet[id]
		list = append(list, map[string]any{"id": mb.id, "plaintext_sha256": mb.sha, "media_type": mb.mt})
	}
	if list == nil {
		list = []any{}
	}
	man := map[string]any{"spec": "bean-core-v0.5", "identity": l.Local.Identity, "records": float64(records), "blobs": list}
	if err := writeFileAtomic(filepath.Join(dir, "manifest.json"), Canonical(man), 0o600); err != nil {
		return 0, 0, err
	}
	return records, blobs, nil
}

// ImportExport reverses Export into a log with the same identity secret.
func (s *Store) ImportExport(dir string) (records, blobs int, err error) {
	if _, err := s.ReadLocal(); err != nil {
		return 0, 0, err
	}
	entries, err := os.ReadDir(filepath.Join(dir, "blobs"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, 0, err
	}
	for _, e := range entries {
		pt, err := os.ReadFile(filepath.Join(dir, "blobs", e.Name()))
		if err != nil {
			return 0, 0, err
		}
		id, err := s.PutBlob(pt)
		if err != nil {
			return 0, 0, err
		}
		if id[7:] != e.Name() {
			return 0, 0, errors.New("blob corrupt: sha256:" + e.Name())
		}
		blobs++
	}
	entries, err = os.ReadDir(filepath.Join(dir, "records"))
	if err != nil {
		return 0, 0, err
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, "records", e.Name()))
		if err != nil {
			return 0, 0, err
		}
		p := PathOf("sha256:" + strings.TrimSuffix(e.Name(), ".json"))
		if fr := CheckFile(p, data); fr.Err != nil {
			return 0, 0, fmt.Errorf("%s: %s", e.Name(), fr.Err)
		}
		wrote, err := writeIfAbsent(s.path(p), data, 0o600)
		if err != nil {
			return 0, 0, err
		}
		if wrote {
			records++
		}
	}
	return records, blobs, nil
}

// Reindex deletes local/cache and rebuilds the index (7.10). The index is
// the canonical bytes of a sorted list of record summaries.
func (s *Store) Reindex() (int, error) {
	if err := os.RemoveAll(s.path(dirCache)); err != nil {
		return 0, err
	}
	l, err := s.Load()
	if err != nil {
		return 0, err
	}
	var rows []any
	for _, id := range sortedKeys(l.Report.Records) {
		r := l.Report.Records[id]
		row := map[string]any{
			"id": id, "type": r.Type, "author": r.Author, "kid": r.Kid,
			"imported": r.Imported, "observed": r.Observed, "system": r.Source.System,
			"valid": l.Visible(id), "hidden": l.Hidden[id],
		}
		if r.Subject != "" {
			row["subject"] = r.Subject
		}
		if r.Source.Ref != "" {
			row["ref"] = r.Source.Ref
		}
		if sup := l.Report.Superseded[id]; len(sup) > 0 {
			row["superseded_by"] = sup[0]
		}
		if tid, ok := l.Report.Purged[id]; ok {
			row["purged_by"] = tid
		}
		if e, ok := l.Report.Errors[id]; ok {
			row["error"] = e
		}
		rows = append(rows, row)
	}
	if rows == nil {
		rows = []any{}
	}
	idx := map[string]any{"spec": "bean-core-v0.5", "records": rows}
	if err := writeFileAtomic(s.path(fileIndex), append(Canonical(idx), '\n'), 0o600); err != nil {
		return 0, err
	}
	return len(rows), nil
}
