package core

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Version is the version of this implementation. It follows the spec
// version until the format freezes at v1.
const Version = "0.5.0"

// Adapter is the source.adapter string the core writes on its own records.
const Adapter = "hlog@" + Version

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
	// Now returns the current time. Tests and $HLOG_NOW inject it.
	Now func() time.Time
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
	idb, err := os.ReadFile(s.path(fileIdentity))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotInitialized
	}
	if err != nil {
		return nil, err
	}
	l := &Local{Identity: strings.TrimSpace(string(idb))}
	if !IsID(l.Identity) {
		return nil, errors.New("bad field: local/identity")
	}
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
	alg, hx, _ := strings.Cut(strings.TrimSpace(string(kb)), ":")
	raw, err := hex.DecodeString(hx)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("bad field: local/signing-key")
	}
	switch alg {
	case "ed25519":
		l.Signer = NewEd25519Signer(raw)
	case "secp256k1":
		if l.Signer, err = NewSecp256k1Signer(raw); err != nil {
			return nil, err
		}
	case "p256":
		l.Signer, err = NewP256Signer(raw)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrUnsupportedKeyAlgorithm
	}
	return l, nil
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
	if err := os.MkdirAll(s.path(dirLocal), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path(filePurgedRef), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
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
func (s *Store) Load() (*Loaded, error) { return s.load(false) }

// load is Load; full proves every file and rebuilds the object cache.
func (s *Store) load(full bool) (*Loaded, error) {
	local, err := s.ReadLocal()
	if err != nil && !errors.Is(err, ErrNotInitialized) && !errors.Is(err, os.ErrNotExist) {
		return nil, err // an agent-only replica has no signing key; it still loads
	}
	disk, hidden, files, proven, cacheLen, err := s.readObjects(full)
	for try := 0; errors.Is(err, errMoved) && try < 5; try++ {
		disk, hidden, files, proven, cacheLen, err = s.readObjects(full)
	}
	if err != nil {
		return nil, err
	}
	holds, err := s.readHolds()
	if err != nil {
		return nil, err
	}
	rp := verifyFiles(files, proven)
	s.writeObjCache(disk, files, rp, proven, cacheLen)
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
		// Another hlog process (an MCP server, say) may have moved it first.
		if err := os.Rename(s.path(PathOf(target)), hiddenPath(s.Dir, target)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		hidden[target] = true
		if err := s.addPurgedRef(rp.Records[target]); err != nil {
			return nil, err
		}
	}
	// Restores: hidden target no longer purged.
	for target := range hidden {
		if _, still := rp.Purged[target]; still {
			continue
		}
		if _, exists := files[PathOf(target)]; !exists {
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

// Visible reports whether a record is valid and not hidden.
func (l *Loaded) Visible(id string) bool { return l.Report.Valid[id] && !l.Hidden[id] }

// GC erases hidden targets whose tombstone is still valid and at least
// HoldDays old, and blobs that no valid unpurged record names. It returns
// the counts.
func (s *Store) GC() (records, blobs int, err error) {
	l, err := s.Load()
	if err != nil {
		return 0, 0, err
	}
	cutoff := s.Now().UTC().Add(-HoldDays * 24 * time.Hour)
	for tid, h := range l.Holds {
		recv, perr := time.Parse("2006-01-02T15:04:05Z", h.Received)
		if perr != nil || recv.After(cutoff) {
			continue
		}
		if l.Report.Purged[h.Target] != tid || !l.Hidden[h.Target] {
			continue
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
	blobs, err = s.eraseOrphanBlobs(l)
	return records, blobs, err
}

// eraseOrphanBlobs removes blobs that no valid, unpurged record names
// (7.6 step 5, 7.7 step 5).
func (s *Store) eraseOrphanBlobs(l *Loaded) (int, error) {
	named := map[string]bool{}
	for id, r := range l.Report.Records {
		if !l.Report.Valid[id] {
			continue
		}
		for _, b := range r.Blobs {
			named[b.Hash] = true
		}
	}
	paths, err := listShardFiles(s.Dir, dirBlobs)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range paths {
		id := blobIDOfPath(p)
		if id == "" || named[id] {
			continue
		}
		if err := os.Remove(s.path(p)); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

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
	l, err := s.load(true)
	if err != nil {
		return nil, 1, err
	}
	rp := l.Report
	paths, err := listShardFiles(s.Dir, dirBlobs)
	if err != nil {
		return nil, 1, err
	}
	present := map[string]bool{}
	for _, p := range paths {
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
	sort.Strings(lines[:len(lines)-1])
	return lines, code, nil
}
