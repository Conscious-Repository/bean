package core

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Version is the version of this implementation. It follows the spec
// version until the format freezes at v1.
const Version = "0.5.0"

// Adapter is the source.adapter string the core writes on its own records.
const Adapter = "bean@" + Version

// HoldDays is the delete hold (SPEC.md 7.6). With change set C14 it gates
// gc, not an automatic erase.
const HoldDays = 7

var (
	ErrNotInitialized     = errors.New("not initialized")
	ErrAlreadyInitialized = errors.New("already initialized")
	ErrNotSubject         = errors.New("not subject")
	ErrNotAuthor          = errors.New("not author")
	ErrBadTarget          = errors.New("bad target")
)

// Store is one replica folder.
type Store struct {
	Dir string
	// Now returns the current time. Tests and $BEAN_NOW inject it.
	Now func() time.Time
	// mu is the in-process half of the folder lock (lock.go).
	mu sync.Mutex
}

// Open points at a folder. It does not read anything.
func Open(dir string) *Store {
	return &Store{Dir: dir, Now: time.Now}
}

func (s *Store) path(rel string) string { return filepath.Join(s.Dir, filepath.FromSlash(rel)) }

func (s *Store) nowString() string { return s.Now().UTC().Format("2006-01-02T15:04:05Z") }

// Local is the content of local/.
type Local struct {
	Identity string
	Secret   []byte
	Signer   Signer
}

// ReadLocal loads local/. It returns not initialized when local/identity is
// absent.
func (s *Store) ReadLocal() (*Local, error) {
	id, err := s.Identity()
	if err != nil {
		return nil, err
	}
	l := &Local{Identity: id}
	sb, err := os.ReadFile(s.path(fileSecret))
	if err != nil {
		return nil, err
	}
	l.Secret, err = hex.DecodeString(strings.TrimSpace(string(sb)))
	if err != nil || len(l.Secret) != 32 {
		return nil, errors.New("bad field: local/identity-secret")
	}
	kb, err := os.ReadFile(s.path(fileSignKey))
	if err != nil {
		return nil, err
	}
	if l.Signer, err = signerFromLine(string(kb)); err != nil {
		return nil, err
	}
	return l, nil
}

// signerFromLine parses a local key line, "<alg>:<64 hex secret>".
func signerFromLine(line string) (Signer, error) {
	alg, hx, _ := strings.Cut(strings.TrimSpace(line), ":")
	raw, err := hex.DecodeString(hx)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("bad field: local/signing-key")
	}
	switch alg {
	case "ed25519":
		return NewEd25519Signer(raw), nil
	case "secp256k1":
		return NewSecp256k1Signer(raw)
	case "p256":
		return NewP256Signer(raw)
	}
	return nil, ErrUnsupportedKeyAlgorithm
}

// NewKey is the signing key that init, rotate and restore install. The
// zero value is a new random Ed25519 key. Alg "secp256k1" with a Secret is
// an existing Nostr key (the bytes of an nsec); without a Secret it is a
// new random one.
type NewKey struct {
	Alg    string
	Secret []byte
}

// signer makes the key and the line for local/signing-key.
func (k NewKey) signer() (Signer, string, error) {
	alg := k.Alg
	if alg == "" {
		alg = "ed25519"
	}
	secret := k.Secret
	for secret == nil {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, "", err
		}
		if alg == "secp256k1" {
			if _, err := NewSecp256k1Signer(b); err != nil {
				continue // zero or above the curve order: draw again
			}
		}
		secret = b
	}
	line := alg + ":" + hex.EncodeToString(secret) + "\n"
	switch alg {
	case "ed25519":
		if len(secret) != 32 {
			return nil, "", ErrBadKey
		}
		return NewEd25519Signer(secret), line, nil
	case "secp256k1":
		sg, err := NewSecp256k1Signer(secret)
		return sg, line, err
	}
	return nil, "", ErrUnsupportedKeyAlgorithm
}

// Init creates an identity (SPEC.md 5.2) with the given signing key; the
// zero NewKey is a new random Ed25519 key. It returns the identity id and
// the 24 words, which the caller shows once.
func (s *Store) Init(key NewKey) (string, []string, error) {
	unlock, err := s.lock()
	if err != nil {
		return "", nil, err
	}
	defer unlock()
	if _, err := os.Stat(s.path(fileIdentity)); err == nil {
		return "", nil, ErrAlreadyInitialized
	}
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return "", nil, err
	}
	words, _ := SeedToWords(seed)
	secret := IdentitySecret(seed)
	recovery := RecoverySigner(seed, 0).Public()
	signer, keyLine, err := key.signer()
	if err != nil {
		return "", nil, err
	}
	g := NewGenesis(signer.Public(), Commitment(recovery), Adapter, s.nowString())
	id, bytes, err := SignRecord(g, signer)
	if err != nil {
		return "", nil, err
	}
	if fr := CheckFile(PathOf(id), bytes); fr.Err != nil {
		return "", nil, fr.Err
	}
	if _, err := writeIfAbsent(s.path(PathOf(id)), bytes, 0o600); err != nil {
		return "", nil, err
	}
	if err := writeFileAtomic(s.path(fileSecret), []byte(hex.EncodeToString(secret)+"\n"), 0o600); err != nil {
		return "", nil, err
	}
	if err := writeFileAtomic(s.path(fileSignKey), []byte(keyLine), 0o600); err != nil {
		return "", nil, err
	}
	if err := writeFileAtomic(s.path(fileIdentity), []byte(id+"\n"), 0o600); err != nil {
		return "", nil, err
	}
	for i := range seed {
		seed[i] = 0
	}
	return id, words, nil
}

// Hold is one entry of local/holds.json.
type Hold struct {
	Target   string `json:"target"`
	Received string `json:"received"`
}

func (s *Store) readHolds() (map[string]Hold, error) {
	out := map[string]Hold{}
	b, err := os.ReadFile(s.path(fileHolds))
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	v, err := Parse(b)
	if err != nil {
		return nil, err
	}
	m, _ := v.(map[string]any)
	for tid, raw := range m {
		h, _ := raw.(map[string]any)
		t, _ := h["target"].(string)
		r, _ := h["received"].(string)
		out[tid] = Hold{t, r}
	}
	return out, nil
}

func (s *Store) writeHolds(h map[string]Hold) error {
	m := map[string]any{}
	for tid, x := range h {
		m[tid] = map[string]any{"target": x.Target, "received": x.Received}
	}
	return writeFileAtomic(s.path(fileHolds), append(Canonical(m), '\n'), 0o600)
}

func (s *Store) readPurgedRefs() (map[string]bool, error) {
	out := map[string]bool{}
	f, err := os.Open(s.path(filePurgedRef))
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			out[line] = true
		}
	}
	return out, sc.Err()
}

func refLine(subject, system, ref string) string {
	return string(Canonical([]any{subject, system, ref}))
}

func (s *Store) addPurgedRef(r *Record) error {
	if r == nil || r.Source.Ref == "" {
		return nil
	}
	line := refLine(r.Subject, r.Source.System, r.Source.Ref)
	have, err := s.readPurgedRefs()
	if err != nil {
		return err
	}
	if have[line] {
		return nil
	}
	return appendLineSync(s.path(filePurgedRef), line)
}

// Loaded is a replica after the hold rules ran.
type Loaded struct {
	Files  map[string][]byte // objects/ plus hidden records, by objects/ path
	Hidden map[string]bool   // ids stored in local/hidden
	Report *Report
	Holds  map[string]Hold
	Local  *Local // nil when not initialized
}

// Load reads the folder, verifies it, and applies the hold rules (SPEC.md
// 7.6 as changed by C14): a newly purged target is hidden, never erased on
// a timer; a target whose tombstone became invalid is restored. It skips
// the signature check of files the object cache vouches for (cache.go).
// It holds the folder lock. On a folder that cannot be written it reads
// without the lock and changes nothing.
func (s *Store) Load() (*Loaded, error) {
	unlock, err := s.lock()
	if err != nil {
		if readOnlyFolder(err) {
			return s.loadWith(false, true)
		}
		return nil, err
	}
	defer unlock()
	return s.load(false)
}

// load is Load for a caller that holds the lock; full proves every file
// and rebuilds the object cache.
func (s *Store) load(full bool) (*Loaded, error) { return s.loadWith(full, false) }

// loadReadOnly verifies the folder without changing it: no hides, no
// restores, no cache. Sync reads its source this way.
func (s *Store) loadReadOnly() (*Loaded, error) { return s.loadWith(false, true) }

func (s *Store) loadWith(full, readOnly bool) (*Loaded, error) {
	if !readOnly {
		removeStaleTemps(s.path(dirObjects), s.path(dirBlobs), s.path(dirLocal))
	}
	local, err := s.ReadLocal()
	if err != nil && !errors.Is(err, ErrNotInitialized) && !errors.Is(err, os.ErrNotExist) {
		return nil, err // an agent-only replica has no signing key; it still loads
	}
	o, err := s.readObjects(full)
	for try := 0; errors.Is(err, errMoved) && try < 5; try++ {
		o, err = s.readObjects(full)
	}
	if err != nil {
		return nil, err
	}
	holds, err := s.readHolds()
	if err != nil {
		return nil, err
	}
	files, hidden := o.files, o.hidden
	rp := verifyFiles(files, o.proven)
	for _, p := range o.bad {
		rp.Errors[p] = ErrBadPath.Error()
	}
	if readOnly {
		for target := range rp.Purged {
			hidden[target] = true
		}
		return &Loaded{Files: files, Hidden: hidden, Report: rp, Holds: holds, Local: local}, nil
	}
	s.writeObjCache(o, rp)
	if done, removed, err := s.finishRotation(rp); err != nil {
		return nil, err
	} else if removed {
		// An undone rotation removed its continuations: judge again.
		return s.loadWith(full, readOnly)
	} else if done {
		if local, err = s.ReadLocal(); err != nil && !errors.Is(err, ErrNotInitialized) {
			return nil, err
		}
	}
	changed := false
	// New valid purges: hide the target.
	for target, tid := range rp.Purged {
		if hidden[target] {
			continue
		}
		if _, ok := files[PathOf(target)]; !ok {
			continue
		}
		if _, held := holds[tid]; !held {
			holds[tid] = Hold{Target: target, Received: s.nowString()}
			changed = true
		}
		if err := os.MkdirAll(s.path(dirHidden), 0o700); err != nil {
			return nil, err
		}
		// Another bean process (an MCP server, say) may have moved it first.
		if err := os.Rename(s.path(PathOf(target)), hiddenPath(s.Dir, target)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		hidden[target] = true
		if err := s.addPurgedRef(rp.Records[target]); err != nil {
			return nil, err
		}
	}
	// Restores: a hidden target comes back only when every tombstone that
	// hid it is judged invalid. A tombstone that is unresolved, held, too
	// new or missing keeps it hidden (fail closed, audit 4 M4).
	for target := range hidden {
		if _, still := rp.Purged[target]; still {
			continue
		}
		if _, exists := files[PathOf(target)]; !exists || heldHidden(holds, rp, target) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(s.path(PathOf(target))), 0o700); err != nil {
			return nil, err
		}
		if err := os.Rename(hiddenPath(s.Dir, target), s.path(PathOf(target))); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		delete(hidden, target)
		for tid, h := range holds {
			if h.Target == target {
				delete(holds, tid)
				changed = true
			}
		}
	}
	if changed {
		if err := s.writeHolds(holds); err != nil {
			return nil, err
		}
	}
	return &Loaded{Files: files, Hidden: hidden, Report: rp, Holds: holds, Local: local}, nil
}

// heldHidden reports whether a hold for target names a tombstone that
// this report did not judge invalid.
func heldHidden(holds map[string]Hold, rp *Report, target string) bool {
	for tid, h := range holds {
		if _, bad := rp.Errors[tid]; h.Target == target && !bad {
			return true
		}
	}
	return false
}

// ownMissing reports whether the identity of this copy has no chain in
// the folder: its genesis was never synced here, or it is lost. A too-new
// identity is reported as too new instead.
func (l *Loaded) ownMissing() bool {
	return l.Local != nil && len(l.Report.Chains[l.Local.Identity]) == 0 && !contains(l.Report.TooNew, l.Local.Identity)
}

// Visible reports whether a record is valid and not hidden.
func (l *Loaded) Visible(id string) bool { return l.Report.Valid[id] && !l.Hidden[id] }

// GC erases hidden targets whose tombstone is still valid and at least
// HoldDays old, and blobs that no valid unpurged record names. It returns
// the counts.
func (s *Store) GC() (records, blobs int, err error) {
	unlock, err := s.lock()
	if err != nil {
		return 0, 0, err
	}
	defer unlock()
	l, err := s.load(false)
	if err != nil {
		return 0, 0, err
	}
	// Fail closed (DECISIONS 107): erase nothing while the verifier cannot
	// fully judge the identities in this folder.
	if len(l.Report.TooNew) > 0 {
		return 0, 0, errors.New("too new: " + strings.Join(l.Report.TooNew, ", ") + ": update bean before gc")
	}
	if l.Local != nil && contains(l.Report.Compromised, l.Local.Identity) {
		return 0, 0, errors.New("compromised: " + l.Local.Identity + ": gc refused")
	}
	if l.ownMissing() {
		return 0, 0, errors.New("own identity missing: " + l.Local.Identity + ": sync from another copy before gc")
	}
	if len(l.Report.Incomplete) > 0 {
		return 0, 0, errors.New("incomplete seals: " + strings.Join(l.Report.Incomplete, ", ") + ": sync the missing seal parts before gc")
	}
	cutoff := s.Now().UTC().Add(-HoldDays * 24 * time.Hour)
	var candidates []string
	for tid, h := range l.Holds {
		recv, perr := time.Parse("2006-01-02T15:04:05Z", h.Received)
		if perr != nil || recv.After(cutoff) {
			continue
		}
		if l.Report.Purged[h.Target] != tid || !l.Hidden[h.Target] {
			continue
		}
		if r := l.Report.Records[h.Target]; r != nil {
			for _, b := range r.Blobs {
				candidates = append(candidates, b.Hash)
			}
		}
		if err := os.Remove(hiddenPath(s.Dir, h.Target)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return 0, 0, err
		}
		delete(l.Holds, tid)
		delete(l.Files, PathOf(h.Target))
		delete(l.Hidden, h.Target)
		records++
	}
	if err := s.writeHolds(l.Holds); err != nil {
		return 0, 0, err
	}
	blobs, err = s.eraseBlobsOf(l.Files, candidates)
	return records, blobs, err
}

// eraseBlobsOf erases the candidate blobs (the blobs of records that were
// just erased) that no remaining record file names (SPEC.md 7.6 step 5
// and 7.7 step 5 as changed by C25). A record file counts whether or not
// it is valid now: validity can come back (a sync, a newer bean), erased
// bytes cannot. A file that does not parse counts when it contains the
// blob id at all. A blob that no record ever named is never erased.
func (s *Store) eraseBlobsOf(files map[string][]byte, candidates []string) (int, error) {
	if len(candidates) == 0 {
		return 0, nil
	}
	want := map[string]bool{}
	for _, c := range candidates {
		if IsID(c) {
			want[c] = true
		}
	}
	for _, data := range files {
		if len(want) == 0 {
			break
		}
		for _, h := range namedBlobs(data) {
			delete(want, h)
		}
	}
	n := 0
	for _, id := range sortedKeys(want) {
		err := os.Remove(s.path(BlobPathOf(id)))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// namedBlobs returns the blob ids that a record file names, without
// judging the record. For bytes that do not parse it returns every
// "sha256:<64 hex>" string in them.
func namedBlobs(data []byte) []string {
	v, err := Parse(data)
	if obj, ok := v.(map[string]any); err == nil && ok {
		var out []string
		arr, _ := obj["blobs"].([]any)
		for _, it := range arr {
			if m, ok := it.(map[string]any); ok {
				if h, ok := m["hash"].(string); ok {
					out = append(out, h)
				}
			}
		}
		if val, ok := obj["value"].(map[string]any); ok {
			if h, ok := val["blob"].(string); ok {
				out = append(out, h)
			}
		}
		return out
	}
	return idPattern.FindAllString(string(data), -1)
}

var idPattern = regexp.MustCompile(`sha256:[0-9a-f]{64}`)

// PutBlob encrypts and stores plaintext (7.3). It returns the blob id.
func (s *Store) PutBlob(plaintext []byte) (string, error) {
	secret, err := s.identitySecret()
	if err != nil {
		return "", err
	}
	out, err := EncryptBlob(BlobKey(secret), plaintext)
	if err != nil {
		return "", err
	}
	id := HashID(out)
	if _, err := writeIfAbsent(s.path(BlobPathOf(id)), out, 0o600); err != nil {
		return "", err
	}
	return id, nil
}

// GetBlob reads and decrypts a blob.
func (s *Store) GetBlob(id string) ([]byte, error) {
	if !IsID(id) {
		return nil, errors.New("bad field: blob id")
	}
	secret, err := s.identitySecret()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(s.path(BlobPathOf(id)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("blob missing: " + id)
	}
	if err != nil {
		return nil, err
	}
	if HashID(b) != id {
		return nil, errors.New("blob corrupt: " + id)
	}
	pt, err := DecryptBlob(BlobKey(secret), b)
	if err != nil {
		return nil, errors.New("blob corrupt: " + id)
	}
	return pt, nil
}

// VerifyLines runs verify (12.1 output) including blob checks and hidden
// warnings. It returns the lines and the exit code.
func (s *Store) VerifyLines() ([]string, int, error) {
	readOnly := false
	unlock, err := s.lock()
	if err != nil {
		if !readOnlyFolder(err) {
			return nil, 1, err
		}
		readOnly, unlock = true, func() {}
	}
	defer unlock()
	l, err := s.loadWith(true, readOnly)
	if err != nil {
		return nil, 1, err
	}
	rp := l.Report
	blobFiles, bad, err := listShards(s.Dir, dirBlobs)
	if err != nil {
		return nil, 1, err
	}
	for _, p := range bad {
		rp.Errors[p] = ErrBadPath.Error()
	}
	present := map[string]bool{}
	for _, f := range blobFiles {
		p := f.rel
		id := blobIDOfPath(p)
		b, err := os.ReadFile(s.path(p))
		if err != nil {
			return nil, 1, err
		}
		if id == "" || HashID(b) != id {
			if id == "" {
				id = p
			}
			rp.Errors[p] = "blob corrupt: " + id
			continue
		}
		present[id] = true
	}
	for id, r := range rp.Records {
		for _, b := range r.Blobs {
			if present[b.Hash] {
				continue
			}
			if _, purged := rp.Purged[id]; purged || !rp.Valid[id] {
				rp.warn(id, "blob missing: "+b.Hash)
			} else {
				rp.fail(id, "blob missing: "+b.Hash)
			}
		}
	}
	for target := range l.Hidden {
		rp.warn(target, "hidden pending gc: "+target)
	}
	var localIDs []string
	if l.Local != nil {
		localIDs = []string{l.Local.Identity}
	}
	lines := rp.Lines()
	code := rp.ExitCode(localIDs)
	if l.ownMissing() {
		lines = append([]string{"error: " + l.Local.Identity + ": own identity missing: sync from another copy"}, lines...)
		code = max(code, 1)
	}
	sort.Strings(lines[:len(lines)-1])
	return lines, code, nil
}
