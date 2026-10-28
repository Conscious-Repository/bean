package core

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
)

var ErrSeedMismatch = ErrSeedWords

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
func (s *Store) Rotate(words []string, noSeal []string) (RotateResult, error) {
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
	id := l.Local.Identity
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
	skSeed := make([]byte, 32)
	if _, err := rand.Read(skSeed); err != nil {
		return res, err
	}
	newSigner := NewEd25519Signer(skSeed)
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
	if err := writeFileAtomic(s.path(fileSignKey), []byte("ed25519:"+hex.EncodeToString(skSeed)+"\n"), 0o600); err != nil {
		return res, err
	}
	res.Succession, res.NewKey = sid, newSigner.Public()
	return res, nil
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
