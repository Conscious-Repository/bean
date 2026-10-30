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
	NotSealed  []string // records of the outgoing key left unsealed by request
	Parts      []string // seal continuations (C28), in part order
	Past       []string // records of an unknown version by the outgoing key that halted the identity (C29)
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
	var halt []string
	if contains(l.Report.TooNew, id) {
		// C29: the 24 words go past a halt that only the head key caused.
		// The succession retires that key, so its records become ungated
		// (8.2) on every verifier. A halt by a recovery key needs an update.
		halt = l.Report.HeadHalt[id]
		if len(halt) == 0 {
			return res, errors.New("too new: " + id + ": update bean before you rotate")
		}
		l = l.past(id)
		if contains(l.Report.Compromised, id) {
			return res, errors.New("compromised: " + id)
		}
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
	// The halting records are records of the outgoing key in this folder,
	// so C26 seals them too unless the person excludes them. No verifier
	// that knows their version can then lose them; this one skips them.
	for _, x := range halt {
		seal[x] = true
	}
	res.Past = halt
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
	// C28: the first sealChunk ids go in the succession, the rest in
	// bean.seal continuations of sealChunk ids each, signed by the same
	// recovery key. value.parts tells a reader how many to expect.
	first, rest := res.Sealed, [][]string(nil)
	if len(first) > sealChunk {
		first = res.Sealed[:sealChunk]
		for i := sealChunk; i < len(res.Sealed); i += sealChunk {
			rest = append(rest, res.Sealed[i:min(i+sealChunk, len(res.Sealed))])
		}
	}
	if len(rest) > MaxSealParts {
		return res, fmt.Errorf("too large: %d records to seal; one rotation seals at most %d", len(res.Sealed), sealChunk*(MaxSealParts+1))
	}
	succ := NewSuccession(id, rec.Public(), head.ID, newSigner.Public(), Commitment(RecoverySigner(seed, gen+1).Public()), first, Adapter, s.nowString())
	if len(rest) > 0 {
		succ["value"].(map[string]any)["parts"] = float64(len(rest))
	}
	sid, bytes, err := SignRecord(succ, rec)
	if err != nil {
		return res, err
	}
	if fr := CheckFile(PathOf(sid), bytes); fr.Err != nil {
		return res, fr.Err
	}

	type file struct {
		id   string
		data []byte
	}
	var parts []file
	for k, ids := range rest {
		pid, pb, err := SignRecord(NewSeal(id, rec.Public(), sid, k+1, ids, Adapter, s.nowString()), rec)
		if err != nil {
			return res, err
		}
		if fr := CheckFile(PathOf(pid), pb); fr.Err != nil {
			return res, fr.Err
		}
		parts = append(parts, file{pid, pb})
	}
	// Past a halt, prove before the first write that the succession ends
	// it: with the new files, the identity must not be too new and the
	// succession must be its head. Fail closed otherwise.
	if len(halt) > 0 {
		after := make(map[string][]byte, len(l.Files)+len(parts)+1)
		for p, b := range l.Files {
			after[p] = b
		}
		after[PathOf(sid)] = bytes
		for _, f := range parts {
			after[PathOf(f.id)] = f.data
		}
		rp := verifyFiles(after, nil, "")
		if c := rp.Chains[id]; contains(rp.TooNew, id) || len(c) == 0 || c[len(c)-1].ID != sid {
			return res, errors.New("too new: " + id + ": the rotation would not end the halt; update bean")
		}
	}
	// Crash order (DECISIONS 110, 124): local/rotation-pending names the
	// succession, then the new key goes to local/signing-key.next, then the
	// continuations, then the succession, then the key replaces
	// local/signing-key. The succession is written last of the records, so
	// a folder that has it has every part. A crash at any point leaves a
	// state that the next load finishes or undoes (finishRotation).
	if err := writeFileAtomic(s.path(fileRotationPending), []byte(sid+"\n"), 0o600); err != nil {
		return res, err
	}
	if err := writeFileAtomic(s.path(fileSignKeyNext), []byte(keyLine), 0o600); err != nil {
		return res, err
	}
	for _, f := range parts {
		if _, err := writeIfAbsent(s.path(PathOf(f.id)), f.data, 0o600); err != nil {
			return res, err
		}
		if hookAfterPart != nil {
			hookAfterPart()
		}
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
	if err := os.Remove(s.path(fileRotationPending)); err != nil {
		return res, err
	}
	for _, f := range parts {
		res.Parts = append(res.Parts, f.id)
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
	// C29: an identity halted only by its head key has no chain here, but
	// the 24 words may go past the halt. Look at it with the halt ignored.
	for _, gid := range l.Report.TooNew {
		if len(l.Report.HeadHalt[gid]) == 0 {
			continue
		}
		if chain := l.past(gid).Report.Chains[gid]; len(chain) > 0 && chain[0].Recovery() == c0 {
			matches = append(matches, gid)
		}
	}
	sort.Strings(matches)
	var id string
	switch {
	case identity != "":
		if !contains(matches, identity) && contains(l.Report.TooNew, identity) {
			return RotateResult{}, "", errors.New("too new: " + identity + ": update bean before you restore")
		}
		if !contains(matches, identity) {
			return RotateResult{}, "", errors.New("input missing: genesis " + identity + " for these words")
		}
		id = identity
	case identity == "" && len(matches) == 0 && len(l.Report.TooNew) > 0:
		// A too-new identity has no chain here; the genesis may be one of them.
		return RotateResult{}, "", errors.New("too new: " + strings.Join(l.Report.TooNew, ", ") + ": update bean before you restore")
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

// fileRotationPending names the succession that a running rotation
// writes (DECISIONS 124), so a load after a crash can undo its parts.
const fileRotationPending = "local/rotation-pending"

// sealChunk is the number of ids per succession or continuation (C28).
// 200,000 ids are about 14 MiB, under MaxSuccessionSize. Tests lower it;
// nothing outside this package can set it.
var sealChunk = 200000

// hookAfterSuccession and hookAfterPart let a test stop a rotation between
// its writes.
var hookAfterSuccession, hookAfterPart func()

// finishRotation completes or discards a rotation that a crash stopped.
// If a chain head names the key in local/signing-key.next, the succession
// is on disk and the key moves into place. If no chain node names it, the
// succession never reached the disk and the key is discarded. The caller
// holds the lock. It reports whether local/signing-key changed.
//
// local/rotation-pending names the succession being written. If that
// succession is not on disk, the rotation did not happen, and the
// continuations that name it are removed: no reader could ever use them,
// because only that succession can make them count. It reports in removed
// whether it deleted any record file.
func (s *Store) finishRotation(rp *Report) (changed, removed bool, err error) {
	if b, err := os.ReadFile(s.path(fileRotationPending)); err == nil {
		sid := strings.TrimSpace(string(b))
		_, statErr := os.Stat(s.path(PathOf(sid)))
		_, nextErr := os.Stat(s.path(fileSignKeyNext))
		switch {
		case errors.Is(statErr, os.ErrNotExist):
			// The succession never reached the disk: undo its parts. A
			// rotation past a halt (C29) wrote them for an identity that is
			// still too new, so look among its dropped records too.
			for _, id := range append(sortedKeys(rp.Records), sortedKeys(rp.Dropped)...) {
				r := rp.Records[id]
				if r == nil {
					r = rp.Dropped[id]
				}
				if r.Type == "bean.seal" && r.Targets("seals")[0] == sid {
					if err := os.Remove(s.path(PathOf(id))); err != nil && !errors.Is(err, os.ErrNotExist) {
						return false, removed, err
					}
					removed = true
				}
			}
			if err := os.Remove(s.path(fileRotationPending)); err != nil {
				return false, removed, err
			}
		case errors.Is(nextErr, os.ErrNotExist):
			// The rotation finished; only the note is left.
			if err := os.Remove(s.path(fileRotationPending)); err != nil {
				return false, removed, err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, false, err
	}
	b, err := os.ReadFile(s.path(fileSignKeyNext))
	if errors.Is(err, os.ErrNotExist) {
		return false, removed, nil
	}
	if err != nil {
		return false, removed, err
	}
	pub, err := keyLinePublic(string(b))
	if err != nil {
		return false, removed, err
	}
	named := false
	for _, chain := range rp.Chains {
		for i, n := range chain {
			if n.ValueKey() != pub {
				continue
			}
			named = true
			if i == len(chain)-1 {
				if err := renameSync(s.path(fileSignKeyNext), s.path(fileSignKey)); err != nil {
					return false, removed, err
				}
				if err := os.Remove(s.path(fileRotationPending)); err != nil && !errors.Is(err, os.ErrNotExist) {
					return true, removed, err
				}
				return true, removed, nil
			}
		}
	}
	if !named {
		return false, removed, os.Remove(s.path(fileSignKeyNext))
	}
	return false, removed, nil
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

// past returns l judged again with the head-key halt of identity id
// ignored (C29). Only rotateFrom uses it, to seal and write the succession
// that ends the halt.
func (l *Loaded) past(id string) *Loaded {
	c := *l
	c.Report = verifyFiles(l.Files, nil, id)
	return &c
}
