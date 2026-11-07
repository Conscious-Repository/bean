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

// ErrCannotList says that a command cannot show what a rotation would decide on
// before it has the words. The rotation itself still decides.
var ErrCannotList = errors.New("cannot list")

// UndecidedError says that a rotation has no decision on the records of the
// outgoing key. It wrote nothing.
type UndecidedError struct {
	Records []*Record // records of the outgoing key, oldest import first
	Halt    []string  // records of a newer format by that key (C29)
}

func (e *UndecidedError) Error() string {
	return fmt.Sprintf("undecided: %d records of the key you are replacing: check them, then use -seal-all, with -no-seal <id> for each one that is not yours", len(e.Records)+len(e.Halt))
}

// Seals is the person's decision on the records of the outgoing key. A thief
// with that key can write records that look like the person's, and the person
// syncs before a rotation, so they can be in this folder. Only the person can
// tell them apart, and a seal cannot be undone: the zero value keeps nothing
// and a rotation with it stops unless there is nothing to decide.
type Seals struct {
	All    bool     // keep every record, except those in NoSeal
	NoSeal []string // ids or 6+ hex prefixes of records not to keep
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
	return s.rotateFrom(l, l.Local.Identity, seed, c, key)
}

// rotateFrom writes the succession from the head of identity id and stores
// the new signing key. The caller has checked the seed. The steps: judge
// the identity (past a head-key halt, C29), choose the seal set (C26),
// sign the records (C28), prove that they end a halt, and write them in
// crash order.
func (s *Store) rotateFrom(l *Loaded, id string, seed []byte, c Seals, key NewKey) (RotateResult, error) {
	var res RotateResult
	l, halt, err := l.rotatable(id)
	if err != nil {
		return res, err
	}
	chain := l.Report.Chains[id]
	if len(chain) == 0 {
		return res, errors.New("input missing: " + id)
	}
	head, gen := chain[len(chain)-1], len(chain)-1
	rec := RecoverySigner(seed, gen)
	if Commitment(rec.Public()) != head.Recovery() {
		return res, ErrSeedMismatch
	}
	if res.Sealed, res.NotSealed, err = l.decide(id, head.ValueKey(), halt, c); err != nil {
		return res, err
	}
	res.Past = halt
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
	files, err := s.rotationRecords(id, rec, head.ID, newSigner.Public(), Commitment(RecoverySigner(seed, gen+1).Public()), res.Sealed)
	if err != nil {
		return res, err
	}
	sid := files[len(files)-1].id
	// Past a halt, prove before the first write that the succession ends
	// it: with the new files, the identity must not be too new and the
	// succession must be its head. Fail closed otherwise.
	if len(halt) > 0 {
		add := map[string][]byte{}
		for _, f := range files {
			add[PathOf(f.id)] = f.data
		}
		rp := verifyFiles(overlay(l.Files, nil, add), nil)
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
	for _, f := range files {
		if _, err := writeIfAbsent(s.path(PathOf(f.id)), f.data, 0o600); err != nil {
			return res, err
		}
		hook := hookAfterPart
		if f.id == sid {
			hook = hookAfterSuccession
		}
		if hook != nil {
			hook()
		}
	}
	if err := renameSync(s.path(fileSignKeyNext), s.path(fileSignKey)); err != nil {
		return res, err
	}
	if err := os.Remove(s.path(fileRotationPending)); err != nil {
		return res, err
	}
	for _, f := range files[:len(files)-1] {
		res.Parts = append(res.Parts, f.id)
	}
	res.Succession, res.NewKey = sid, newSigner.Public()
	return res, nil
}

// rotatable refuses a compromised identity and one that only an update
// can go past. For an identity halted by its head key alone (C29) it
// returns l judged without the halting records, and their ids: the
// succession retires that key, so the records become ungated (8.2) on
// every verifier.
func (l *Loaded) rotatable(id string) (*Loaded, []string, error) {
	var halt []string
	if contains(l.Report.TooNew, id) {
		halt = l.Report.HeadHalt[id]
		if len(halt) == 0 {
			return nil, nil, errors.New("too new: " + id + ": update bean before you rotate")
		}
		l = l.past(id)
	}
	if contains(l.Report.Compromised, id) {
		return nil, nil, errors.New("compromised: " + id)
	}
	return l, halt, nil
}

// decide applies the person's decision: the records of the outgoing key to
// seal and not to seal, or an UndecidedError when there is no decision.
func (l *Loaded) decide(id, outgoing string, halt []string, c Seals) (sealed, notSealed []string, err error) {
	if !c.All {
		recs := l.outgoingRecords(id, outgoing)
		if len(recs) > 0 || len(halt) > 0 {
			return nil, nil, &UndecidedError{recs, halt}
		}
	}
	return l.sealSet(id, outgoing, halt, c.NoSeal)
}

// outgoingRecords lists the records of the outgoing key in this folder, valid
// now or not, oldest import first. A hidden target is here too: if the person
// refuses the tombstone that hid it, the record must still be sealed.
func (l *Loaded) outgoingRecords(id, outgoing string) []*Record {
	var out []*Record
	for _, r := range l.Report.Records {
		if r.Kid == outgoing && r.Author == id && !r.IsIdentity() {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Imported != out[j].Imported {
			return out[i].Imported < out[j].Imported
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// sealSet is what the succession from key outgoing of identity id seals
// (C26): every record of the outgoing key in this folder, valid now or not,
// and the halting records, except those in noSeal. A kept record of that key
// also seals the targets it names that are not in this folder: a target that
// purge erased here must not die of a missing seal if it comes back from
// another copy. A target counts only through a kept record of the outgoing
// key. If any record could name an absent id, a thief could deliver pre-signed
// records after the rotation and have them count (DECISIONS 185).
//
// An id in noSeal that names no such record is an error, not a silent seal.
func (l *Loaded) sealSet(id, outgoing string, halt, noSeal []string) (sealed, notSealed []string, err error) {
	targets := func(r *Record) []string {
		var out []string
		for _, t := range append(r.Targets("purges"), r.Targets("supersedes")...) {
			if _, here := l.Report.Records[t]; !here {
				out = append(out, t)
			}
		}
		return out
	}
	recs := l.outgoingRecords(id, outgoing)
	candidates := map[string]bool{}
	for _, r := range recs {
		candidates[r.ID] = true
		for _, t := range targets(r) {
			candidates[t] = true
		}
	}
	for _, h := range l.Holds {
		if r, here := l.Report.Records[h.Target]; !here || (r.Kid == outgoing && r.Author == id) {
			candidates[h.Target] = true
		}
	}
	for _, x := range halt {
		candidates[x] = true
	}
	skip := map[string]bool{}
	sorted := sortedKeys(candidates)
	for _, x := range noSeal {
		full, err := resolveAmong(candidates, sorted, x)
		if err != nil {
			return nil, nil, err
		}
		skip[full] = true
	}
	seal := map[string]bool{}
	for _, r := range recs {
		if skip[r.ID] {
			continue
		}
		seal[r.ID] = true
		for _, t := range targets(r) {
			seal[t] = true
		}
	}
	for _, h := range l.Holds {
		if candidates[h.Target] {
			seal[h.Target] = true
		}
	}
	for _, x := range halt {
		seal[x] = true
	}
	for x := range skip {
		delete(seal, x)
	}
	return sortedKeys(seal), sortedKeys(skip), nil
}

// resolveAmong finds id, or the one id that starts with a 6+ hex prefix, in a
// set of full ids. sorted is the set in order.
func resolveAmong(set map[string]bool, sorted []string, x string) (string, error) {
	hexPart := strings.TrimPrefix(x, "sha256:")
	if set["sha256:"+hexPart] {
		return "sha256:" + hexPart, nil
	}
	if len(hexPart) >= 6 && isLowerHex(hexPart, len(hexPart)) {
		i := sort.SearchStrings(sorted, "sha256:"+hexPart)
		if i < len(sorted) && strings.HasPrefix(sorted[i][7:], hexPart) {
			if i+1 < len(sorted) && strings.HasPrefix(sorted[i+1][7:], hexPart) {
				return "", errors.New("ambiguous id: " + x)
			}
			return sorted[i], nil
		}
	}
	return "", errors.New("input missing: " + x + " is not a record of the key you are replacing")
}

// Undecided returns what a rotation with this decision still needs an answer
// on, so a command can show it before it asks for the 24 words. identity is
// for a restore; "" means local/identity, or the only genesis in the folder.
// It checks the ids in c too. When it cannot tell the identity without the
// words, it returns nothing and the rotation itself decides.
func (s *Store) Undecided(c Seals, identity string) (*UndecidedError, *Loaded, error) {
	l, err := s.Load()
	if err != nil {
		return nil, nil, err
	}
	id := identity
	if id == "" {
		id = l.ID
	}
	if id == "" {
		ids := map[string]bool{}
		for gid := range l.Report.Chains {
			ids[gid] = true
		}
		for gid := range l.Report.HeadHalt {
			ids[gid] = true
		}
		if len(ids) > 1 {
			return nil, l, fmt.Errorf("%w: this folder holds %d identities: pass -identity <id>", ErrCannotList, len(ids))
		}
		for only := range ids {
			id = only
		}
	}
	if id == "" {
		return nil, l, nil
	}
	pl, halt, err := l.rotatable(id)
	if err != nil {
		if l.ID == "" && strings.HasPrefix(err.Error(), "compromised") {
			return nil, l, fmt.Errorf("%w: this identity is frozen, so it cannot rotate. Give the words to restore, then run bean migrate", ErrCannotList)
		}
		return nil, l, err
	}
	chain := pl.Report.Chains[id]
	if len(chain) == 0 {
		return nil, l, nil
	}
	outgoing := chain[len(chain)-1].ValueKey()
	if _, _, err := pl.decide(id, outgoing, halt, c); err != nil {
		var ue *UndecidedError
		if errors.As(err, &ue) {
			return ue, pl, nil
		}
		return nil, l, err
	}
	return nil, pl, nil
}

// signedFile is a signed record and its id.
type signedFile struct {
	id   string
	data []byte
}

// rotationRecords signs the succession and its bean.seal continuations
// (C28): the first sealChunk ids go in the succession, the rest in
// continuations of sealChunk ids each, signed by the same recovery key;
// value.parts tells a reader how many to expect. The continuations come
// first and the succession last, in write order.
func (s *Store) rotationRecords(id string, rec Signer, parent, newKey, next string, sealed []string) ([]signedFile, error) {
	var chunks [][]string
	for i := 0; i < len(sealed); i += sealChunk {
		chunks = append(chunks, sealed[i:min(i+sealChunk, len(sealed))])
	}
	if len(chunks) == 0 {
		chunks = [][]string{nil}
	}
	if len(chunks)-1 > MaxSealParts {
		return nil, fmt.Errorf("too large: %d records to seal; one rotation seals at most %d", len(sealed), sealChunk*(MaxSealParts+1))
	}
	succ := NewSuccession(id, rec.Public(), parent, newKey, next, chunks[0], Adapter, s.nowString())
	if len(chunks) > 1 {
		succ["value"].(map[string]any)["parts"] = float64(len(chunks) - 1)
	}
	sid, sb, err := SignRecord(succ, rec)
	if err != nil {
		return nil, err
	}
	var out []signedFile
	for k, ids := range chunks[1:] {
		pid, pb, err := SignRecord(NewSeal(id, rec.Public(), sid, k+1, ids, Adapter, s.nowString()), rec)
		if err != nil {
			return nil, err
		}
		out = append(out, signedFile{pid, pb})
	}
	out = append(out, signedFile{sid, sb})
	for _, f := range out {
		if fr := CheckFile(PathOf(f.id), f.data); fr.Err != nil {
			return nil, fr.Err
		}
	}
	return out, nil
}

// Restore rebuilds local/ on a new host from the 24 words and a synced
// copy of the log (DECISIONS 52 and 53). The signing key stayed on the
// lost host, so Restore always rotates: it finds the identity whose genesis
// commits to recovery generation 0 of these words, writes the identity
// secret, and writes a succession to a new signing key exactly as Rotate
// does, with the same decision on seals. It returns the identity id. identity
// "" accepts only a single genesis that commits to these words; a folder
// with more than one needs the id (DECISIONS 109).
func (s *Store) Restore(words []string, c Seals, key NewKey, identity string) (RotateResult, string, error) {
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
		// Check the dropped genesis first: anyone can add halted identities
		// of their own, and judging each again costs a full verify.
		if g := l.Report.Dropped[gid]; len(l.Report.HeadHalt[gid]) == 0 || g == nil || g.Recovery() != c0 {
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
	// Decide before the first write, so a restore with no decision leaves
	// the folder as it was.
	pl, halt, rerr := l.rotatable(id)
	if rerr == nil {
		if chain := pl.Report.Chains[id]; len(chain) > 0 {
			if _, _, err := pl.decide(id, chain[len(chain)-1].ValueKey(), halt, c); err != nil {
				return RotateResult{}, "", err
			}
		}
	}
	// A frozen identity cannot rotate, but the words still open its files:
	// this copy gets what bean migrate and bean export need, and no signing key.
	if rerr != nil && strings.HasPrefix(rerr.Error(), "compromised") {
		if err := writeFileAtomic(s.path(fileSecret), []byte(hex.EncodeToString(IdentitySecret(seed))+"\n"), 0o600); err != nil {
			return RotateResult{}, "", err
		}
		if err := writeFileAtomic(s.path(fileIdentity), []byte(id+"\n"), 0o600); err != nil {
			return RotateResult{}, "", err
		}
		return RotateResult{}, id, errors.New("compromised: " + id + ": this identity is frozen, so it cannot rotate. This copy can read it now: run bean migrate <empty folder>")
	}
	secret := IdentitySecret(seed)
	if err := writeFileAtomic(s.path(fileSecret), []byte(hex.EncodeToString(secret)+"\n"), 0o600); err != nil {
		return RotateResult{}, "", err
	}
	res, err := s.rotateFrom(l, id, seed, c, key)
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
		if isDigits(f) {
			continue
		}
		out = append(out, strings.ToLower(f))
	}
	return out
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
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

// past returns l judged again without the records that halt identity id
// by its head key (C29). They are of an unknown version, so no other
// result depends on them. Only rotate and restore use it, to seal and
// write the succession that ends the halt.
func (l *Loaded) past(id string) *Loaded {
	c := *l
	c.Report = verifyFiles(overlay(l.Files, l.Report.HeadHalt[id], nil), nil)
	return &c
}

// overlay returns a copy of files without the records drop and with add.
func overlay(files map[string][]byte, drop []string, add map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(files)+len(add))
	for p, b := range files {
		out[p] = b
	}
	for _, id := range drop {
		delete(out, PathOf(id))
	}
	for p, b := range add {
		out[p] = b
	}
	return out
}
