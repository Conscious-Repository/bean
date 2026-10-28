package core

import (
	"encoding/hex"
	"errors"
	"os"
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
	l, err := s.Load()
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
		skip[x] = true
	}
	for rid, r := range l.Report.Records {
		if r.Kid == outgoing && r.IsDestructive() && !r.IsIdentity() && l.Report.Valid[rid] {
			if skip[rid] {
				res.NotSealed = append(res.NotSealed, rid)
			} else {
				res.Sealed = append(res.Sealed, rid)
			}
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
		return res, fr.Err
	}
	if _, err := writeIfAbsent(s.path(PathOf(sid)), bytes, 0o600); err != nil {
		return res, err
	}
	if err := writeFileAtomic(s.path(fileSignKey), []byte(keyLine), 0o600); err != nil {
		return res, err
	}
	res.Succession, res.NewKey = sid, newSigner.Public()
	return res, nil
}

// Restore rebuilds local/ on a new host from the 24 words and a synced
// copy of the log (ROADMAP.md, before M8). The signing key stayed on the
// lost host, so Restore always rotates: it finds the identity whose genesis
// commits to recovery generation 0 of these words, writes the identity
// secret, and writes a succession to a new signing key exactly as Rotate
// does, with the same default seals. It returns the identity id.
func (s *Store) Restore(words []string, noSeal []string, key NewKey) (RotateResult, string, error) {
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
	l, err := s.Load()
	if err != nil {
		return RotateResult{}, "", err
	}
	c0 := Commitment(RecoverySigner(seed, 0).Public())
	var id string
	for _, gid := range sortedKeys(l.Report.Chains) {
		if chain := l.Report.Chains[gid]; len(chain) > 0 && chain[0].Recovery() == c0 {
			id = gid
		}
	}
	if id == "" {
		return RotateResult{}, "", errors.New("input missing: genesis for these words")
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
