package core

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RotateResult is the output of Rotate.
type RotateResult struct {
	Succession string
	NewKey     string
	Sealed     []string
	NotSealed  []string // destructive records of the outgoing key left unsealed by request
}

// Rotate writes a recovery-signed succession from the chain head to a new
// random signing key (SPEC.md 5.3 to 5.5 as changed by C5 and C6). words
// are the 24 paper words. Every valid destructive record of the outgoing key
// in this replica is sealed unless its id is in noSeal.
func (s *Store) Rotate(words []string, noSeal []string, key NewKey) (RotateResult, error) {
	var res RotateResult
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
	seed, err := WordsToSeed(words)
	if err != nil {
		return res, ErrSeedMismatch
	}
	defer func() {
		for i := range seed {
			seed[i] = 0
		}
	}()
	if hex.EncodeToString(IdentitySecret(seed)) != hex.EncodeToString(l.Local.Secret) {
		return res, ErrSeedMismatch
	}
	return s.rotateFrom(l, l.Local.Identity, seed, noSeal, key)
}

// rotateFrom writes the succession from the head of identity id and stores
// the new signing key. The caller has checked the seed.
func (s *Store) rotateFrom(l *Loaded, id string, seed []byte, noSeal []string, key NewKey) (RotateResult, error) {
	var res RotateResult
	if contains(l.Report.Compromised, id) {
		return res, errors.New("compromised: " + id)
	}
	chain := l.Report.Chains[id]
	if len(chain) == 0 {
		return res, errors.New("input missing: " + id)
	}
	head := chain[len(chain)-1]
	gen := len(chain) - 1
	rec := RecoverySigner(seed, gen)
	if Commitment(rec.Public()) != head.Recovery() {
		return res, ErrSeedMismatch
	}
	outgoing := head.ValueKey()
	skip := map[string]bool{}
	for _, x := range noSeal {
		if r, err := l.Resolve(x); err == nil {
			x = r.ID // a 6+ hex prefix works, as everywhere else
		}
		skip[x] = true
	}
	// C26 (Benjamin decision, 2026-10-30): the succession seals every valid
	// record of the outgoing key, not only destructive ones. A record of a
	// retired key that the succession does not seal is invalid.
	//
	// The seal set is every record of the outgoing key in this folder,
	// valid now or not, plus every target that a tombstone or supersede in
	// this folder names. A record that is invalid now for another reason,
	// for example a purged target, must still be sealed: when its tombstone
	// dies (-no-seal), the target comes back and must not die of a missing
	// seal. A sealed id that names no record of the outgoing key has no
	// effect (SPEC 5.4), so the extra target ids are harmless.
	seal := map[string]bool{}
	for rid, r := range l.Report.Records {
		if r.Kid == outgoing && r.Author == id && !r.IsIdentity() {
			seal[rid] = true
		}
		for _, rel := range []string{"purges", "supersedes"} {
			for _, t := range r.Targets(rel) {
				if t2, ok := l.Report.Records[t]; !ok || (t2.Kid == outgoing && t2.Author == id) {
					seal[t] = true
				}
			}
		}
	}
	for _, h := range l.Holds {
		if t, ok := l.Report.Records[h.Target]; !ok || (t.Kid == outgoing && t.Author == id) {
			seal[h.Target] = true
		}
	}
	for rid := range seal {
		if skip[rid] {
			res.NotSealed = append(res.NotSealed, rid)
		} else {
			res.Sealed = append(res.Sealed, rid)
		}
	}
	sort.Strings(res.Sealed)
	sort.Strings(res.NotSealed)
	newSigner, keyLine, err := key.signer()
	if err != nil {
		return res, err
	}
	// 5.8: a key on the path from genesis cannot come back. The verifier
	// would reject the succession; refuse before writing it.
	for _, n := range chain {
		if n.ValueKey() == newSigner.Public() || n.Kid == newSigner.Public() {
			return res, errors.New("key reuse")
		}
	}
	succ := NewSuccession(id, rec.Public(), head.ID, newSigner.Public(), Commitment(RecoverySigner(seed, gen+1).Public()), res.Sealed, Adapter, s.nowString())
	sid, bytes, err := SignRecord(succ, rec)
	if err != nil {
		return res, err
	}
	if fr := CheckFile(PathOf(sid), bytes); fr.Err != nil {
		if errors.Is(fr.Err, ErrTooLarge) {
			return res, fmt.Errorf("too large: %d records to seal; a succession holds about 230,000", len(res.Sealed))
		}
		return res, fr.Err
	}
	// Crash order (DECISIONS 110): the new key goes to disk first, then the
	// succession, then the key replaces local/signing-key. A crash at any
	// point leaves a state that the next load finishes or discards
	// (finishRotation); the new key is never lost while a succession names it.
	if err := writeFileAtomic(s.path(fileSignKeyNext), []byte(keyLine), 0o600); err != nil {
		return res, err
	}
	if _, err := writeIfAbsent(s.path(PathOf(sid)), bytes, 0o600); err != nil {
		return res, err
	}
	if hookAfterSuccession != nil {
		hookAfterSuccession()
	}
	if err := renameSync(s.path(fileSignKeyNext), s.path(fileSignKey)); err != nil {
		return res, err
	}
	res.Succession, res.NewKey = sid, newSigner.Public()
	return res, nil
}

// Restore rebuilds local/ on a new host from the 24 words and a synced
// copy of the log (docs/ROADMAP.md, before M8). The signing key stayed on the
// lost host, so Restore always rotates: it finds the identity whose genesis
// commits to recovery generation 0 of these words, writes the identity
// secret, and writes a succession to a new signing key exactly as Rotate
// does, with the same default seals. It returns the identity id.
func (s *Store) Restore(words []string, noSeal []string, key NewKey) (RotateResult, string, error) {
	return s.RestoreIdentity(words, noSeal, key, "")
}

// RestoreIdentity is Restore for a folder that holds more than one genesis
// committing to these words (DECISIONS 109). identity "" accepts only a
// single match.
func (s *Store) RestoreIdentity(words []string, noSeal []string, key NewKey, identity string) (RotateResult, string, error) {
	unlock, err := s.lock()
	if err != nil {
		return RotateResult{}, "", err
	}
	defer unlock()
	if _, err := os.Stat(s.path(fileIdentity)); err == nil {
		return RotateResult{}, "", ErrAlreadyInitialized
	}
	seed, err := WordsToSeed(words)
	if err != nil {
		return RotateResult{}, "", ErrSeedMismatch
	}
	defer func() {
		for i := range seed {
			seed[i] = 0
		}
	}()
	l, err := s.load(false)
	if err != nil {
		return RotateResult{}, "", err
	}
	c0 := Commitment(RecoverySigner(seed, 0).Public())
	var matches []string
	for _, gid := range sortedKeys(l.Report.Chains) {
		if chain := l.Report.Chains[gid]; len(chain) > 0 && chain[0].Recovery() == c0 {
			matches = append(matches, gid)
		}
	}
	var id string
	switch {
	case identity != "":
		if !contains(matches, identity) {
			return RotateResult{}, "", errors.New("input missing: genesis " + identity + " for these words")
		}
		id = identity
	case len(matches) == 0:
		return RotateResult{}, "", errors.New("input missing: genesis for these words")
	case len(matches) > 1:
		// The recovery commitment is public, so anyone who can write to the
		// folder can copy it into a genesis of their own. Never guess.
		return RotateResult{}, "", errors.New("ambiguous: " + strings.Join(matches, " ") + ": more than one genesis commits to these words; pass -identity with the id that bean whoami showed on your old machine")
	default:
		id = matches[0]
	}
	secret := IdentitySecret(seed)
	if err := writeFileAtomic(s.path(fileSecret), []byte(hex.EncodeToString(secret)+"\n"), 0o600); err != nil {
		return RotateResult{}, "", err
	}
	res, err := s.rotateFrom(l, id, seed, noSeal, key)
	if err != nil {
		return res, "", err
	}
	// The identity file goes last: until it exists the folder reads as not
	// initialized, so a failed restore can simply run again.
	if err := writeFileAtomic(s.path(fileIdentity), []byte(id+"\n"), 0o600); err != nil {
		return res, "", err
	}
	return res, id, nil
}

// ParseWords splits pasted paper words on whitespace and strips numbering
// like "1." so the person can paste the init output.
func ParseWords(text string) []string {
	var out []string
	for _, f := range strings.Fields(text) {
		f = strings.TrimRight(f, ".")
		if _, isNum := isDigits(f); isNum {
			continue
		}
		out = append(out, strings.ToLower(f))
	}
	return out
}

func isDigits(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// fileSignKeyNext holds the new signing key while a rotation is between
// its two writes (DECISIONS 110).
const fileSignKeyNext = "local/signing-key.next"

// hookAfterSuccession lets a test stop a rotation between its writes.
var hookAfterSuccession func()

// finishRotation completes or discards a rotation that a crash stopped.
// If a chain head names the key in local/signing-key.next, the succession
// is on disk and the key moves into place. If no chain node names it, the
// succession never reached the disk and the key is discarded. The caller
// holds the lock. It reports whether local/signing-key changed.
func (s *Store) finishRotation(rp *Report) (bool, error) {
	b, err := os.ReadFile(s.path(fileSignKeyNext))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	pub, err := keyLinePublic(string(b))
	if err != nil {
		return false, err
	}
	named := false
	for _, chain := range rp.Chains {
		for i, n := range chain {
			if n.ValueKey() != pub {
				continue
			}
			named = true
			if i == len(chain)-1 {
				return true, renameSync(s.path(fileSignKeyNext), s.path(fileSignKey))
			}
		}
	}
	if !named {
		return false, os.Remove(s.path(fileSignKeyNext))
	}
	return false, nil
}

// keyLinePublic returns the tagged public key of a local key line.
func keyLinePublic(line string) (string, error) {
	sg, err := signerFromLine(line)
	if err != nil {
		return "", err
	}
	return sg.Public(), nil
}

// renameSync renames and flushes the directory.
func renameSync(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return err
	}
	return syncDir(filepath.Dir(to))
}
