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
// signs and writes them (12.3 bean append). On the first error it stops;
// records before it stay written.
func (s *Store) Append(in io.Reader) (AppendResult, error) { return s.appendAs(in, false) }

func (s *Store) appendAs(in io.Reader, agent bool) (AppendResult, error) {
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
	existing := map[string]string{}
	for id, r := range l.Report.Records {
		if l.Report.Valid[id] && r.Source.Ref != "" && r.Author == w.author {
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
	unlock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	l, err := s.load(false)
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
	if err := l.canWrite(writer{signer: l.Local.Signer, author: l.Local.Identity}); err != nil {
		return "", err
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
	// A local purge erases at once: there is nothing to hold. Erase the target's own blobs, unless another record file names them
	// (C25). Never erase any other blob here.
	delete(l.Files, PathOf(id))
	var candidates []string
	for _, b := range target.Blobs {
		candidates = append(candidates, b.Hash)
	}
	if _, err := s.eraseBlobsOf(l.Files, candidates); err != nil {
		return "", err
	}
	return tid, nil
}

// SyncResult is the output of Sync.
type SyncResult struct {
	Records, Blobs, Refused, Replaced int // Replaced counts repaired record files and blobs.
}

// Sync copies from src into this store (SPEC.md 7.4 with change set C13:
// verify through the signature before copying, replace a stored file that
// fails its signature when the incoming one passes).
func (s *Store) Sync(src *Store) (SyncResult, error) {
	var res SyncResult
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
	b, err := s.load(false)
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
	// Seal continuations go first, so a sync that stops halfway never
	// leaves a succession without its parts (C28). Order changes nothing
	// else: the result is a set union.
	order := sortedKeys(a.Files)
	sort.SliceStable(order, func(i, j int) bool { return isSealPath(a, order[i]) && !isSealPath(a, order[j]) })
	for _, p := range order {
		data := a.Files[p]
		fr := CheckFile(p, data)
		if fr.Err != nil {
			continue // 7.4: fails a step from 1 to 15
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
	// Blobs (7.4 step 4 as changed by C25): named by a record file of the
	// union that is not purged or hidden, valid now or not.
	b2, err := s.load(false)
	if err != nil {
		return res, err
	}
	named := map[string]bool{}
	for p, data := range b2.Files {
		m := pathPattern.FindStringSubmatch(p)
		if m == nil {
			continue
		}
		id := "sha256:" + m[1] + m[2]
		if _, purged := b2.Report.Purged[id]; purged || b2.Hidden[id] {
			continue
		}
		for _, h := range namedBlobs(data) {
			named[h] = true
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

// Export writes a plaintext copy (7.9). dir must be empty or absent.
func (s *Store) Export(dir string) (records, blobs int, err error) {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return 0, 0, errors.New("export dir not empty")
	}
	unlock, err := s.lock()
	if err != nil {
		return 0, 0, err
	}
	defer unlock()
	l, err := s.load(false)
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
	man := map[string]any{"spec": "bean-v0", "identity": l.Local.Identity, "records": float64(records), "blobs": list}
	if err := writeFileAtomic(filepath.Join(dir, "manifest.json"), Canonical(man), 0o600); err != nil {
		return 0, 0, err
	}
	return records, blobs, nil
}

// ImportExport reverses Export into a log with the same identity secret.
// Export directories are untrusted input. Read them through os.Root so a
// changed symlink cannot escape the selected folder.
func (s *Store) ImportExport(dir string) (records, blobs int, err error) {
	unlock, err := s.lock()
	if err != nil {
		return 0, 0, err
	}
	defer unlock()
	if _, err := s.ReadLocal(); err != nil {
		return 0, 0, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return 0, 0, err
	}
	defer root.Close()
	blobEntries, err := exportEntries(dir, "blobs", true)
	if err != nil {
		return 0, 0, err
	}
	recordEntries, err := exportEntries(dir, "records", false)
	if err != nil {
		return 0, 0, err
	}
	secret, err := s.identitySecret()
	if err != nil {
		return 0, 0, err
	}
	for _, e := range blobEntries {
		pt, err := readExportFile(root, "blobs/"+e.Name(), maxImportBlobBytes)
		if err != nil {
			return records, blobs, err
		}
		out, err := EncryptBlob(BlobKey(secret), pt)
		if err != nil {
			return records, blobs, err
		}
		id := HashID(out)
		if id != "sha256:"+e.Name() {
			return records, blobs, errors.New("blob corrupt: sha256:" + e.Name())
		}
		wrote, err := writeIfAbsent(s.path(BlobPathOf(id)), out, 0o600)
		if err != nil {
			return records, blobs, err
		}
		if wrote {
			blobs++
		}
	}
	for _, e := range recordEntries {
		data, err := readExportFile(root, "records/"+e.Name(), MaxSuccessionSize)
		if err != nil {
			return records, blobs, err
		}
		rid := "sha256:" + strings.TrimSuffix(e.Name(), ".json")
		p := PathOf(rid)
		if fr := CheckFile(p, data); fr.Err != nil {
			return records, blobs, fmt.Errorf("%s: %s", e.Name(), fr.Err)
		}
		wrote, err := writeIfAbsent(s.path(p), data, 0o600)
		if err != nil {
			return records, blobs, err
		}
		if wrote {
			records++
		}
	}
	return records, blobs, nil
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
