package core

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Export, import-export and the manifest of an export (SPEC.md 7.9): the
// plaintext way out of a log, and the way back in.

// Export writes a plaintext copy (7.9). dir must be empty or absent.
func (s *Store) Export(dir string) (records, blobs int, err error) {
	res, err := s.ExportAll(dir)
	return res.Records, res.Blobs, err
}

// ExportAll is Export with the full result. It writes the export into a
// new folder beside dir and moves it to dir when it is whole, so a run that
// stops part way leaves no half export at dir. A folder that such a run
// left behind holds plaintext: the next export names it and stops, for the
// person to delete.
func (s *Store) ExportAll(dir string) (ExportResult, error) {
	res := ExportResult{NotExported: map[string]string{}}
	// The target is absent, or an empty folder, or a link to one. Anything
	// else stays as it is: an export never takes the place of a file.
	dir, err := filepath.Abs(dir)
	if err != nil {
		return res, err
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real // a link to a folder: the export goes where it leads
	}
	// A folder that an unfinished export left is named before anything else
	// is said: it holds plaintext, and the person must know that it is there.
	// By name, not by pattern: a folder's name can hold the signs of one.
	parent, base := filepath.Dir(dir), filepath.Base(dir)
	siblings, _ := os.ReadDir(parent)
	for _, e := range siblings {
		if strings.HasPrefix(e.Name(), base+partialSuffix) {
			return res, fmt.Errorf("an export that did not finish left %s: it holds records in plaintext. Delete it, then export again", filepath.Join(parent, e.Name()))
		}
	}
	exists := false
	switch info, err := os.Lstat(dir); {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return res, err
	case !info.IsDir(): // a file, or a link that leads nowhere
		return res, errors.New("export dir is not a folder: " + dir)
	default:
		entries, err := os.ReadDir(dir)
		if err != nil {
			return res, err
		}
		if len(entries) > 0 {
			return res, errors.New("export dir not empty: " + dir)
		}
		exists = true
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return res, err
	}
	tmp, err := os.MkdirTemp(parent, base+partialSuffix)
	if err != nil {
		return res, err
	}
	res, err = s.exportInto(tmp)
	if !res.Written {
		os.RemoveAll(tmp)
		return res, err
	}
	// Into place. A target that is absent takes the folder whole. An empty
	// folder that is there stays the folder it is, for a person who stands
	// in it: what the export holds moves into it, the manifest last, since
	// the manifest is what says that an export is whole.
	if rerr := moveExport(tmp, dir, exists); rerr != nil {
		os.RemoveAll(tmp)
		return ExportResult{NotExported: map[string]string{}}, rerr
	}
	return res, err
}

// moveExport moves a whole export from the folder it was written in to its
// place. into says that dir is there, and empty. When a move fails, what
// was moved is taken out again, so that dir is left as it was.
func moveExport(tmp, dir string, into bool) error {
	if !into {
		return os.Rename(tmp, dir)
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Name() != "manifest.json" {
			names = append(names, e.Name())
		}
	}
	names = append(names, "manifest.json")
	for i, name := range names {
		if err := os.Rename(filepath.Join(tmp, name), filepath.Join(dir, name)); err != nil {
			for _, moved := range names[:i] {
				os.RemoveAll(filepath.Join(dir, moved))
			}
			return err
		}
	}
	os.Remove(tmp) // empty now: the export is whole whether this goes or not
	return nil
}

// partialSuffix ends the name of the folder that an export writes into
// before it moves it into place.
const partialSuffix = ".partial-"

// exportInto writes an export into dir, which is new and empty.
func (s *Store) exportInto(dir string) (ExportResult, error) {
	res := ExportResult{NotExported: map[string]string{}}
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
	view := l.blobView()
	labels := view.fileLabelsPastHalt()
	unlocks := blobUnlocker(l.ID, view.Report, labels)
	type wanted struct {
		mediaType string
		own       bool // a record that unlocks names it
	}
	want := map[string]*wanted{}
	other := map[string]string{} // blob -> reason, for blobs that no record unlocks
	gone := view.Report.unjudgedPurged()
	for _, id := range sortedKeys(all) {
		r := all[id]
		if _, purged := view.Report.Purged[id]; purged || view.Hidden[id] || gone[id] {
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
	// judged, and its id is in the tombstone. A file with a bad path, such as
	// a sync tool's conflict copy, stands for the id of its bytes
	// (DECISIONS 218).
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
		if pid := recordFileID(p, l.Files[p]); pid != "" && (l.Hidden[pid] || tombed[pid] || gone[pid]) {
			res.Withheld = append(res.Withheld, p)
			continue
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

// SeedFromExport copies the record files of an export into a folder that
// holds no log yet, so that restore can rebuild the keys from the 24 words
// and import-export can then bring the files back. It checks each record
// file as import-export does, and returns the export's identity and the
// number of record files written. A folder that already holds a log
// (local/identity) is refused: its records come in with import-export.
func (s *Store) SeedFromExport(dir string) (string, int, error) {
	unlock, err := s.lock()
	if err != nil {
		return "", 0, err
	}
	defer unlock()
	if _, err := s.Identity(); !errors.Is(err, ErrNotInitialized) {
		if err == nil {
			return "", 0, errors.New("this folder already holds a log: use import-export")
		}
		return "", 0, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", 0, err
	}
	defer root.Close()
	who, err := readManifestIdentity(root)
	if errors.Is(err, os.ErrNotExist) {
		return "", 0, errors.New("not an export: no manifest.json in " + dir)
	}
	if err != nil {
		return "", 0, err
	}
	entries, err := exportEntries(dir, "records", false)
	if err != nil {
		return "", 0, err
	}
	n := 0
	for _, e := range entries {
		data, err := readExportFile(root, "records/"+e.Name(), MaxSuccessionSize)
		if err != nil {
			return "", 0, err
		}
		p := PathOf("sha256:" + strings.TrimSuffix(e.Name(), ".json"))
		if fr := CheckFile(p, data); fr.Err != nil {
			return "", 0, fmt.Errorf("%s: %s", e.Name(), fr.Err)
		}
		if err := os.MkdirAll(filepath.Dir(s.path(p)), 0o700); err != nil {
			return "", 0, err
		}
		wrote, err := writeIfAbsent(s.path(p), data, 0o600)
		if err != nil {
			return "", 0, err
		}
		if wrote {
			n++
		}
	}
	return who, n, nil
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
	u := pastHeadHalt(verifyFiles(union, nil), union, nil, l.ID)
	gone := u.unjudgedPurged()
	deleted := func(id string) bool {
		_, purged := u.Purged[id]
		return purged || gone[id] || l.Hidden[id]
	}
	// A blob that a deleted record names comes back only when a live record of
	// the person or a paired agent names it. Any record can name any id, so a
	// stranger's live record does not bring back a purged photo.
	ul := *l
	ul.Files, ul.Report = union, u
	unlocks := blobUnlocker(l.ID, u, ul.fileLabelsPastHalt()) // u is judged past a head-key halt
	recs := ul.allRecords()
	use := blobUse{}
	use.mark(union, deleted, func(id string) bool { return recs[id] != nil && unlocks(recs[id]) })
	for _, e := range blobEntries {
		id := "sha256:" + e.Name()
		if use.withheld(id) {
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
		if _, purged := u.Purged[r.ID]; purged || gone[r.ID] { // 3.2 and 7.5
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
// one bounded token at a time, so a long list costs no memory; the bound
// only stops a folder that never ends. One blob entry is under 200 bytes.
const maxManifestBytes int64 = 4 << 30

// maxManifestToken bounds what the decoder reads for one token. A decoder
// holds a whole token in memory, so one long string would cost its full size
// (DECISIONS 218). An export writes ids, media types, paths and short
// reasons, all far below it.
const maxManifestToken = 1 << 20

// maxManifestDepth bounds the nesting of the manifest. An export nests three
// deep, and a decoder keeps one entry per open bracket.
const maxManifestDepth = 16

var errManifestToken = errors.New("too large: a token of manifest.json")

// tokenBudget is a reader that fails after left bytes. The caller sets left
// before each token.
type tokenBudget struct {
	r    io.Reader
	left int
}

func (b *tokenBudget) Read(p []byte) (int, error) {
	if b.left <= 0 {
		return 0, errManifestToken
	}
	if len(p) > b.left {
		p = p[:b.left]
	}
	n, err := b.r.Read(p)
	b.left -= n
	return n, err
}

// manifestTokens reads the tokens of a manifest with a bound on each.
type manifestTokens struct {
	dec    *json.Decoder
	budget *tokenBudget
}

func (m manifestTokens) next() (json.Token, error) {
	m.budget.left = maxManifestToken
	return m.dec.Token()
}

func (m manifestTokens) more() bool {
	m.budget.left = maxManifestToken
	return m.dec.More()
}

// readManifestIdentity returns the identity of an export's manifest.json. It
// reads the file one bounded token at a time and keeps only that field: the
// lists of a log with many blobs do not fit any small cap, and nothing else
// is needed. The manifest must be one JSON object with exactly one
// "identity" string.
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
	budget := &tokenBudget{r: bufio.NewReader(io.LimitReader(f, maxManifestBytes))}
	dec := json.NewDecoder(budget)
	dec.UseNumber()
	m := manifestTokens{dec, budget}
	// fail keeps the cause when a token was too long.
	fail := func(err error) (string, error) {
		if errors.Is(err, errManifestToken) {
			return "", errManifestToken
		}
		return "", bad
	}
	if t, err := m.next(); err != nil || t != json.Delim('{') {
		return fail(err)
	}
	who, seen := "", false
	for m.more() {
		t, err := m.next()
		key, ok := t.(string)
		if err != nil || !ok {
			return fail(err)
		}
		if key != "identity" {
			if err := skipJSONValue(m); err != nil {
				return fail(err)
			}
			continue
		}
		v, err := m.next()
		s, ok := v.(string)
		if err != nil || !ok || seen {
			return fail(err)
		}
		who, seen = s, true
	}
	if t, err := m.next(); err != nil || t != json.Delim('}') {
		return fail(err)
	}
	if _, err := m.next(); err != io.EOF {
		return fail(err) // more after the object
	}
	if !IsID(who) {
		return "", bad
	}
	return who, nil
}

// skipJSONValue reads one value of m without keeping it.
func skipJSONValue(m manifestTokens) error {
	depth := 0
	for {
		t, err := m.next()
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
		if depth > maxManifestDepth {
			return errors.New("too deep: manifest.json")
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
