package core

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// RotateResult is the output of Rotate.
type RotateResult struct {
	Succession string
	NewKey     string
	Sealed     []string
	NotSealed  []string // records of the outgoing key that the person did not seal
}

// Seals says which records of the outgoing key a rotation keeps. A thief with
// that key can write corrections, deletions, checkpoints and claims that look
// like the person's, and the person is told to sync before a rotation, so all
// of them can be here. Only the person can tell them apart: nothing is sealed
// by default, and a rotation with an undecided record writes nothing.
type Seals struct {
	All    bool     // seal every record not named in NoSeal
	None   bool     // seal only the records named in Seal
	Seal   []string // ids or 6+ hex prefixes
	NoSeal []string
}

// UndecidedError lists the records that a rotation needs a decision on.
type UndecidedError struct{ Records []*Record }

func (e *UndecidedError) Error() string {
	return fmt.Sprintf("undecided: %d records of the outgoing key: choose with -seal-all, -seal-none, -seal or -no-seal", len(e.Records))
}

// pendingSeals lists the records of the identity's head key that a rotation
// must decide on: those that need a seal, and the claims that no checkpoint of
// that key covers. A covered claim follows its checkpoint.
func pendingSeals(l *Loaded, id string) []*Record {
	chain := l.Report.Chains[id]
	if len(chain) == 0 {
		return nil
	}
	outgoing := chain[len(chain)-1].ValueKey()
	covered := map[string]bool{}
	for rid, r := range l.Report.Records {
		if r.IsCheckpoint() && r.Author == id && r.Kid == outgoing && l.Report.Valid[rid] {
			for _, x := range r.Covers() {
				covered[x] = true
			}
		}
	}
	var out []*Record
	for rid, r := range l.Report.Records {
		// A record that a tombstone hides counts too: if the person undoes a
		// thief's deletion, the record comes back and must still be attested.
		_, purged := l.Report.Purged[rid]
		if r.Author == id && r.Kid == outgoing && !r.IsIdentity() && (l.Report.Valid[rid] || purged) && (r.NeedsSeal() || !covered[rid]) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// decide splits the pending records. An id that names no pending record is an
// error: with -seal-all, a mistyped -no-seal would seal the thief's record.
func (c Seals) decide(l *Loaded, pending []*Record) (seal, keepOut, undecided []*Record, err error) {
	if c.All && c.None {
		return nil, nil, nil, errors.New("choose one of -seal-all and -seal-none")
	}
	isPending := map[string]bool{}
	for _, r := range pending {
		isPending[r.ID] = true
	}
	resolve := func(ids []string) (map[string]bool, error) {
		out := map[string]bool{}
		for _, x := range ids {
			r, err := l.Resolve(x)
			if err != nil {
				return nil, err
			}
			if !isPending[r.ID] {
				return nil, errors.New("input missing: " + x + " is not a record of the outgoing key that needs a decision")
			}
			out[r.ID] = true
		}
		return out, nil
	}
	yes, err := resolve(c.Seal)
	if err != nil {
		return nil, nil, nil, err
	}
	no, err := resolve(c.NoSeal)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, r := range pending {
		switch {
		case yes[r.ID] && no[r.ID]:
			return nil, nil, nil, errors.New("both -seal and -no-seal: " + r.ID)
		case yes[r.ID], !no[r.ID] && c.All:
			seal = append(seal, r)
		case no[r.ID], c.None:
			keepOut = append(keepOut, r)
		default:
			undecided = append(undecided, r)
		}
	}
	return seal, keepOut, undecided, nil
}

// Undecided returns the records that a rotation with these choices still needs
// a decision on, so a command can list them before it asks for the 24 words.
func (s *Store) Undecided(c Seals) ([]*Record, *Loaded, error) {
	l, err := s.Load()
	if err != nil {
		return nil, nil, err
	}
	if l.Me == "" {
		return nil, l, nil
	}
	_, _, undecided, err := c.decide(l, pendingSeals(l, l.Me))
	return undecided, l, err
}

// Rotate writes a recovery-signed succession from the chain head to a new
// random signing key (SPEC.md 5.3 to 5.5). words are the 24 paper words.
func (s *Store) Rotate(words []string, c Seals, key NewKey) (RotateResult, error) {
	var res RotateResult
	unlock, err := s.lock()
	if err != nil {
		return res, err
	}
	defer unlock()
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
	return s.rotateFrom(l, l.Local.Identity, seed, c, key)
}

// rotateFrom writes the succession from the head of identity id and stores
// the new signing key. The caller has checked the seed and holds the lock.
func (s *Store) rotateFrom(l *Loaded, id string, seed []byte, c Seals, key NewKey) (RotateResult, error) {
	var res RotateResult
	if contains(l.Report.TooNew, id) {
		return res, errors.New("too new: " + id)
	}
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
	seal, keepOut, undecided, err := c.decide(l, pendingSeals(l, id))
	if err != nil {
		return res, err
	}
	if len(undecided) > 0 {
		return res, &UndecidedError{undecided}
	}
	for _, r := range seal {
		res.Sealed = append(res.Sealed, r.ID)
	}
	for _, r := range keepOut {
		res.NotSealed = append(res.NotSealed, r.ID)
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
// copy of the log (docs/ROADMAP.md, before M8). The signing key stayed on the
// lost host, so Restore always rotates: it finds the identity whose genesis
// commits to recovery generation 0 of these words, writes a succession to a
// new signing key as Rotate does, then the identity secret. It returns the
// identity id. identity picks the genesis when more than one fits the words:
// the recovery commitment is public, so a stranger can copy it into a genesis.
func (s *Store) Restore(words []string, c Seals, key NewKey, identity string) (RotateResult, string, error) {
	unlock, err := s.lock()
	if err != nil {
		return RotateResult{}, "", err
	}
	defer unlock()
	// An agent-only copy has local/identity and no signing key. It can restore.
	if _, err := os.Stat(s.path(fileSignKey)); err == nil {
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
	var fits []string
	for _, gid := range sortedKeys(l.Report.Chains) {
		if chain := l.Report.Chains[gid]; len(chain) > 0 && chain[0].Recovery() == c0 {
			fits = append(fits, gid)
		}
	}
	if identity != "" {
		if !contains(fits, identity) {
			return RotateResult{}, "", errors.New("input missing: genesis " + identity + " does not fit these words")
		}
		fits = []string{identity}
	}
	if len(fits) > 1 {
		return RotateResult{}, "", errors.New("ambiguous identity: these words fit " + strings.Join(fits, ", ") + ". A stranger can copy a genesis. Pick yours with -identity: it is the id that bean init printed, and the subject of your records.")
	}
	var id string
	if len(fits) == 1 {
		id = fits[0]
	}
	if id == "" {
		// A too-new identity has no chain to search.
		for _, gid := range l.Report.TooNew {
			if fr := CheckFile(PathOf(gid), l.Files[PathOf(gid)]); fr.Record != nil {
				if v, _ := fr.Record.Raw["value"].(map[string]any); v["recovery"] == c0 {
					return RotateResult{}, "", errors.New("too new: " + gid)
				}
			}
		}
		return RotateResult{}, "", errors.New("input missing: genesis for these words")
	}
	res, err := s.rotateFrom(l, id, seed, c, key)
	if err != nil {
		return res, "", err
	}
	secret := IdentitySecret(seed)
	if err := writeFileAtomic(s.path(fileSecret), []byte(hex.EncodeToString(secret)+"\n"), 0o600); err != nil {
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
