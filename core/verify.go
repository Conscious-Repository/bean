package core

import (
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Sizes (SPEC.md 3.1, change set C6).
const (
	MaxRecordSize     = 1 << 20
	MaxSuccessionSize = 16 << 20
)

var pathPattern = regexp.MustCompile(`^objects/([0-9a-f]{2})/([0-9a-f]{62})\.json$`)

var (
	ErrBadPath      = errors.New("bad path")
	ErrTooLarge     = errors.New("too large")
	ErrBadV         = errors.New("bad v")
	ErrBadType      = errors.New("bad type")
	ErrNotCanonical = errors.New("not canonical")
	ErrIDMismatch   = errors.New("id mismatch")
)

// Known record versions (SPEC.md 8.1).
var knownVersions = map[int]bool{0: true}

// FileResult is the outcome of SPEC.md 3.8 steps 1 to 15 for one file.
type FileResult struct {
	Record  *Record // set when steps 1 to 15 pass, or for an unknown version after step 13
	Err     error   // first failure
	Unknown bool    // unknown v: Record has only the envelope
}

// CheckFile applies SPEC.md 3.8 steps 1 to 15 (order from change sets C2
// and N3: envelope, id, signature and canonical bytes come before the
// version check).
func CheckFile(path string, data []byte) FileResult { return checkFile(path, data, false) }

// checkFile is CheckFile. proven skips only the signature arithmetic of
// step 12, for bytes this replica already proved (the object cache);
// every other step still runs.
func checkFile(path string, data []byte, proven bool) FileResult {
	m := pathPattern.FindStringSubmatch(path)
	if m == nil {
		return FileResult{Err: ErrBadPath}
	}
	pathID := "sha256:" + m[1] + m[2]
	if len(data) > MaxSuccessionSize {
		return FileResult{Err: ErrTooLarge}
	}
	v, err := Parse(data)
	if err != nil {
		return FileResult{Err: err}
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return FileResult{Err: ErrBadJSON}
	}
	r := &Record{Path: path, Raw: obj, Bytes: data}
	// 7. v
	vf, ok := obj["v"].(float64)
	if !ok || vf != float64(int(vf)) || vf < 0 {
		return FileResult{Err: ErrBadV}
	}
	r.V = int(vf)
	// 8. type
	if r.Type, ok = obj["type"].(string); !ok {
		return FileResult{Err: ErrBadType}
	}
	if r.Type != "bean.succession" && r.Type != "bean.seal" && len(data) > MaxRecordSize {
		return FileResult{Err: ErrTooLarge}
	}
	// 9. envelope strings
	if r.Author, ok = obj["author"].(string); !ok {
		return FileResult{Err: badField("author")}
	}
	if r.Kid, ok = obj["kid"].(string); !ok {
		return FileResult{Err: badField("kid")}
	}
	// 10. id
	if r.ID, ok = obj["id"].(string); !ok || r.ID != pathID || RecordID(obj) != r.ID {
		return FileResult{Err: ErrIDMismatch}
	}
	// 11. key, 12. signature
	if _, err := ParseKey(r.Kid); err != nil {
		return FileResult{Err: err}
	}
	if r.Sig, ok = obj["sig"].(string); !ok {
		return FileResult{Err: ErrBadSignature}
	}
	if !proven {
		if err := Verify(r.Kid, r.ID, r.Sig); err != nil {
			return FileResult{Err: err}
		}
	}
	// 13. canonical, for every version (N3): JCS does not depend on v.
	if string(Canonical(obj)) != string(data) {
		return FileResult{Err: ErrNotCanonical}
	}
	// 14. version
	if !knownVersions[r.V] {
		return FileResult{Record: r, Unknown: true}
	}
	// 15. fields
	if err := r.checkFields(); err != nil {
		return FileResult{Err: err}
	}
	return FileResult{Record: r}
}

// Report is the verifier output for one set of files (SPEC.md 6).
type Report struct {
	Records     map[string]*Record // every record that passed steps 1 to 15, by id
	Valid       map[string]bool    // ids of valid records (purged targets excluded)
	Errors      map[string]string  // id or path -> error string
	Warnings    map[string][]string
	TooNew      []string             // identity ids
	Compromised []string             // frozen identity ids
	Chains      map[string][]*Record // identity id -> chain nodes
	Superseded  map[string][]string  // target id -> valid supersede record ids (sorted)
	Purged      map[string]string    // target id -> tombstone id
	Held        map[string]bool      // ids of held destructive records
	Incomplete  []string             // chain successions whose seal set is incomplete (C28)
	Duplicates  map[string]string    // duplicate id -> primary id
	Skipped     int
	// HeadHalt: too-new identity -> the ids of the records that halt it,
	// when every one of them is signed by the head key (8.2, C29). Only
	// then may rotate and restore go past the halt.
	HeadHalt map[string][]string
	// Dropped: records of too-new identities, which 6.3 does not evaluate.
	// A crashed rotation still needs to find its own seal parts there.
	Dropped map[string]*Record
	// Unknown: records of an unknown version. Nothing evaluates them, but
	// they are in the folder and an export must carry them.
	Unknown []*Record
	// OnChain: the nodes of every chain. A succession that is not one is not
	// the identity's record, whatever its author field says: no step checks
	// its key (DECISIONS 187).
	OnChain map[string]bool
	// What readers show for a frozen identity (DECISIONS 191). None of this
	// changes Valid, Errors or the verify output.
	Kept           map[string]bool     // corrections that are "frozen", but every branch kept them
	KeptSuperseded map[string]bool     // targets of those corrections
	Doubted        map[string]bool     // claims of the key at the fork that not every branch kept, and claims of keys below it
	BranchClaims   map[string]bool     // those claims of keys below the fork
	DeletedInDoubt map[string]bool     // targets of "frozen" tombstones that a branch or an older key stands behind
	DoubtedFix     map[string]bool     // corrections that are "frozen" and that not every branch kept
	DisputedBy     map[string][]string // target -> those corrections
}

// InDoubt reports a record that a freeze keeps from the default reads, but that
// a person must be able to see.
func (rp *Report) InDoubt(id string) bool {
	return rp.Doubted[id] || rp.DeletedInDoubt[id] || rp.DoubtedFix[id]
}

// OffTree reports an identity record that no key vouches for.
func (rp *Report) OffTree(r *Record) bool {
	return (r.Type == "bean.succession" && !rp.OnChain[r.ID]) || (r.Type == "bean.seal" && rp.Held[r.ID])
}

func (rp *Report) warn(id, msg string) { rp.Warnings[id] = append(rp.Warnings[id], msg) }

func (rp *Report) fail(id, msg string) {
	if _, has := rp.Errors[id]; !has {
		rp.Errors[id] = msg
	}
	delete(rp.Valid, id)
}

// VerifyFiles runs SPEC.md section 6 on a set of files (path -> bytes). The
// result depends only on the set.
func VerifyFiles(files map[string][]byte) *Report { return verifyFiles(files, nil) }

// parallel runs f(0) to f(n-1) on all CPUs. Callers write results by
// index, so the outcome does not depend on scheduling.
func parallel(n int, f func(i int)) {
	var next atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < runtime.GOMAXPROCS(0); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := int(next.Add(1) - 1); i < n; i = int(next.Add(1) - 1) {
				f(i)
			}
		}()
	}
	wg.Wait()
}

// verifyFiles is VerifyFiles; proven names paths whose signatures this
// replica has already checked. Each step is one pass; none of them looks
// at the records of an identity more than once per pass.
func verifyFiles(files map[string][]byte, proven map[string]bool) *Report {
	rp := &Report{
		Records: map[string]*Record{}, Valid: map[string]bool{}, Errors: map[string]string{},
		Warnings: map[string][]string{}, Chains: map[string][]*Record{}, Superseded: map[string][]string{},
		Purged: map[string]string{}, Held: map[string]bool{}, Duplicates: map[string]string{},
		HeadHalt: map[string][]string{}, Dropped: map[string]*Record{}, OnChain: map[string]bool{},
		Kept: map[string]bool{}, KeptSuperseded: map[string]bool{}, Doubted: map[string]bool{}, DeletedInDoubt: map[string]bool{},
		DoubtedFix: map[string]bool{}, DisputedBy: map[string][]string{}, BranchClaims: map[string]bool{},
	}
	unknown := rp.checkFiles(files, proven) // step 1
	rp.Unknown = unknown
	trees := rp.buildTrees(sortedKeys(rp.Records)) // steps 2 and 3
	rp.gate(unknown, trees)                        // 8.2
	ids := rp.dropTooNew(trees)
	position := rp.freeze(trees)                 // steps 4 and 5
	sealedBy := sealSets(rp, ids)                // step 5b
	rp.checkKeys(ids, trees, position, sealedBy) // steps 6 to 8
	rp.checkSupersedes(ids)                      // step 9
	rp.checkTombstones(ids)                      // step 10
	rp.mapSuperseded(ids)
	rp.checkSubjects(ids, trees) // step 11
	rp.findDuplicates(ids)       // step 12
	rp.frozenView(ids, trees, position)
	return rp
}

// frozenView works out what a reader should show for a frozen identity. The
// verifier's rule stands: every correction and deletion is "frozen". But a
// person who restored from a stale backup and kept everything would see old
// values come back, deleted records return, and new claims of a key that a
// thief may hold. So readers follow what every branch at the fork agreed on,
// and a deletion stays in effect on doubt only if an older key signed it, or the
// key at the fork signed it and a branch seals it: otherwise one signature with
// the key at the fork or a key below it, which a thief may hold, could hide the
// whole log. A hidden record is not lost. The
// verify output still says what the rule says.
func (rp *Report) frozenView(ids []string, trees map[string]*tree, position map[string]map[string]int) {
	forked := false
	for _, t := range trees {
		forked = forked || t.forked
	}
	if !forked {
		return
	}
	parts := map[string][]*Record{} // succession id -> its seal parts, in one pass
	for _, id := range ids {
		if r := rp.Records[id]; r.Type == "bean.seal" {
			parts[r.Targets("seals")[0]] = append(parts[r.Targets("seals")[0]], r)
		}
	}
	// For each identity, how many branches seal each id: one pass over the seal
	// lists, so a stranger's own forks cost no more than their size.
	sealedBy := map[string]map[string]int{}
	branches := map[string]int{}
	for gid, t := range trees {
		if !t.forked {
			continue
		}
		sealedBy[gid] = map[string]int{}
		for _, c := range t.children[t.chain[len(t.chain)-1].ID] {
			branches[gid]++
			set := map[string]bool{}
			for _, x := range c.Sealed() {
				set[x] = true
			}
			for _, r := range parts[c.ID] {
				if r.Kid == c.Kid {
					for _, x := range r.Sealed() {
						set[x] = true
					}
				}
			}
			for x := range set {
				sealedBy[gid][x]++
			}
		}
	}
	count := func(gid, id string) int { return sealedBy[gid][id] }
	every := func(gid, id string) bool { return sealedBy[gid][id] == branches[gid] }
	keptTargets := map[string][]string{} // target -> corrections that every branch kept
	for _, id := range ids {
		r := rp.Records[id]
		if sealedBy[r.Author] == nil || r.IsIdentity() {
			continue
		}
		p, ok := position[r.Author][r.Kid]
		if !ok {
			continue
		}
		head := len(trees[r.Author].chain) - 1
		frozen := rp.Errors[id] == "frozen"
		switch {
		case r.Type == "bean.tombstone":
			// A branch seals the records of the key at the fork, which it retires.
			// Its seal says nothing of a record of a key below the fork: anyone with
			// the words can list their own tombstones there (DECISIONS 196, 206).
			if t := rp.Records[r.Targets("purges")[0]]; frozen && t != nil && !t.IsIdentity() && t.Type != "bean.tombstone" && t.Subject == r.Subject && (p < head || (p == head && count(r.Author, id) > 0)) {
				rp.DeletedInDoubt[t.ID] = true
			}
		case r.IsDestructive():
			if !frozen {
				continue
			}
			keep := p != offChain && (p < head || every(r.Author, id))
			if keep {
				rp.Kept[id] = true
			} else {
				rp.DoubtedFix[id] = true
			}
			for _, t := range r.Targets("supersedes") {
				if target := rp.Records[t]; target != nil && target.Author == r.Author {
					if keep {
						keptTargets[t] = append(keptTargets[t], id)
					} else {
						rp.DisputedBy[t] = append(rp.DisputedBy[t], id)
					}
				}
			}
		case frozen && p == offChain:
			rp.Doubted[id] = true
			rp.BranchClaims[id] = true
		case rp.Valid[id] && p == head && !every(r.Author, id):
			rp.Doubted[id] = true
		}
	}
	// A correction replaces its target only while it can be shown itself: a
	// reading must not vanish because its correction is hidden too.
	for t, list := range keptTargets {
		for _, id := range list {
			if !rp.DeletedInDoubt[id] {
				rp.KeptSuperseded[t] = true
			}
		}
	}
}

// checkFiles is step 1. It returns the records of an unknown version.
func (rp *Report) checkFiles(files map[string][]byte, proven map[string]bool) []*Record {
	paths := sortedKeys(files)
	results := make([]FileResult, len(paths))
	parallel(len(paths), func(i int) { results[i] = checkFile(paths[i], files[paths[i]], proven[paths[i]]) })
	var unknown []*Record
	for i, fr := range results {
		switch {
		case fr.Err != nil:
			rp.Errors[paths[i]] = fr.Err.Error()
		case fr.Unknown:
			unknown = append(unknown, fr.Record)
		default:
			rp.Records[fr.Record.ID] = fr.Record
		}
	}
	return unknown
}

// tree is the succession tree of one identity (6.3 step 3) and its chain
// from the genesis through single children (step 5).
type tree struct {
	nodes    map[string]*Record   // node id -> node, the genesis included
	children map[string][]*Record // node id -> accepted successions
	chain    []*Record            // genesis to head; at a fork the fork point is the head
	forked   bool
}

// buildTrees is steps 2 and 3 (seed-only rule, change set C5 to C11). Each
// succession is checked once, from its parent; one that no walk reaches is
// held.
func (rp *Report) buildTrees(ids []string) map[string]*tree {
	trees := map[string]*tree{}
	byParent := map[string]map[string][]*Record{} // identity -> parent id -> successions
	var succs []*Record
	for _, id := range ids {
		r := rp.Records[id]
		switch r.Type {
		case "bean.genesis":
			// 5.8 (N4): the signing key is not the recovery key.
			if Commitment(r.ValueKey()) == r.Recovery() {
				rp.fail(id, "key reuse")
				continue
			}
			trees[id] = &tree{nodes: map[string]*Record{id: r}, children: map[string][]*Record{}}
		case "bean.succession":
			succs = append(succs, r)
			if byParent[r.Author] == nil {
				byParent[r.Author] = map[string][]*Record{}
			}
			p := r.Targets("succeeds")[0]
			byParent[r.Author][p] = append(byParent[r.Author][p], r)
		}
	}
	reached := map[string]bool{}
	for gid, t := range trees {
		// Depth first with a stack of its own, so a long chain cannot
		// exhaust the goroutine stack. On the path from the genesis (5.8),
		// used counts the keys, commits the commitments to those keys,
		// and recs the recovery commitments.
		type frame struct {
			p    *Record
			next int // index of the next child to check
		}
		g := t.nodes[gid]
		p := path{used: map[string]int{}, commits: map[string]int{}, recs: map[string]int{}}
		p.add(g, 1)
		for stack := []frame{{p: g}}; len(stack) > 0; {
			top := &stack[len(stack)-1]
			kids := byParent[gid][top.p.ID]
			if top.next == len(kids) {
				p.add(top.p, -1)
				stack = stack[:len(stack)-1]
				continue
			}
			s := kids[top.next]
			top.next++
			reached[s.ID] = true
			if err := p.check(s, top.p); err != nil {
				rp.fail(s.ID, err.Error())
				continue
			}
			t.nodes[s.ID] = s
			t.children[top.p.ID] = append(t.children[top.p.ID], s)
			p.add(s, 1)
			stack = append(stack, frame{p: s})
		}
		for x := g; ; {
			t.chain = append(t.chain, x)
			c := t.children[x.ID]
			if len(c) != 1 {
				t.forked = len(c) > 1
				break
			}
			x = c[0]
		}
	}
	for _, s := range succs {
		if _, ok := trees[s.Author]; ok && !reached[s.ID] {
			rp.warn(s.ID, "held: "+s.ID)
			rp.Held[s.ID] = true
		}
	}
	return trees
}

// path holds the keys and commitments of the nodes from a genesis to the
// node that the walk is at (5.8).
type path struct{ used, commits, recs map[string]int }

// add counts node n in (d = 1) or out (d = -1) of the path.
func (p path) add(n *Record, d int) {
	for _, k := range []string{n.ValueKey(), n.Kid} {
		p.used[k] += d
		p.commits[Commitment(k)] += d
	}
	p.recs[n.Recovery()] += d
}

// check applies SPEC.md 6.3 step 3 and 5.8 to succession s of parent par.
// No key serves as both a signing key and a recovery key, and no key or
// recovery commitment on the path comes back (DECISIONS 172).
func (p path) check(s, par *Record) error {
	if Commitment(s.Kid) != par.Recovery() {
		return errors.New("bad succession")
	}
	rec := s.Recovery()
	if p.recs[rec] > 0 {
		return errors.New("recovery not advanced")
	}
	c := Commitment(s.ValueKey())
	if p.used[s.ValueKey()] > 0 || c == rec || p.recs[c] > 0 || p.commits[rec] > 0 {
		return errors.New("key reuse")
	}
	return nil
}

// gate is SPEC.md 8.2 with change K1: an unknown-v bean.* record halts
// its author identity only when it is signed by the head key or by a
// recovery key that a chain node commits to. Only the head key or the
// seed holder can change the rules for an identity; a rotated-out key, a
// stranger or a forged file cannot halt it.
func (rp *Report) gate(unknown []*Record, trees map[string]*tree) {
	tooNew := map[string]bool{}
	byRecovery := map[string]bool{} // halted by a record of a recovery key or a genesis
	commits := map[string]map[string]bool{}
	var ungated []*Record
	for _, r := range unknown {
		rp.Skipped++
		if !strings.HasPrefix(r.Type, "bean.") {
			continue
		}
		if r.Type == "bean.genesis" {
			if r.Author == r.Kid {
				tooNew[r.ID] = true
				byRecovery[r.ID] = true
			}
			continue
		}
		t, ok := trees[r.Author]
		if !ok {
			continue
		}
		if commits[r.Author] == nil {
			commits[r.Author] = map[string]bool{}
			for _, x := range t.chain {
				commits[r.Author][x.Recovery()] = true
			}
		}
		switch {
		case commits[r.Author][Commitment(r.Kid)]:
			tooNew[r.Author] = true
			byRecovery[r.Author] = true
		case t.chain[len(t.chain)-1].ValueKey() == r.Kid:
			tooNew[r.Author] = true
			rp.HeadHalt[r.Author] = append(rp.HeadHalt[r.Author], r.ID)
		default:
			ungated = append(ungated, r)
		}
	}
	for gid, ids := range rp.HeadHalt {
		if byRecovery[gid] {
			delete(rp.HeadHalt, gid) // only an update can go past the seed holder
		} else {
			sort.Strings(ids)
		}
	}
	// A too-new identity has no warnings of its own records (8.2 item 2).
	for _, r := range ungated {
		if !tooNew[r.Author] {
			rp.warn(r.ID, "ungated: "+r.ID)
		}
	}
	rp.TooNew = sortedKeys(tooNew)
}

// dropTooNew removes every record of a too-new identity and returns the
// sorted ids of the records left, which start out valid unless they failed.
func (rp *Report) dropTooNew(trees map[string]*tree) []string {
	for _, id := range rp.TooNew {
		delete(trees, id)
	}
	tooNew := func(id string) bool { return contains(rp.TooNew, id) }
	for id, r := range rp.Records {
		if tooNew(r.Author) || tooNew(r.Subject) || (r.Type == "bean.genesis" && tooNew(r.ID)) {
			rp.Dropped[id] = r
			delete(rp.Records, id)
			delete(rp.Errors, id) // step 3 ran before the gate; 8.2 item 2
			delete(rp.Warnings, id)
			delete(rp.Held, id)
			rp.Skipped++
		}
	}
	ids := sortedKeys(rp.Records)
	for _, id := range ids {
		if _, failed := rp.Errors[id]; !failed {
			rp.Valid[id] = true
		}
	}
	return ids
}

// offChain marks a key of a tree node that is below a freeze point.
const offChain = 1 << 30

// freeze is steps 4 and 5: any fork is a freeze (C7, C11), and a node off
// the chain is frozen. It returns the chain position of each key.
func (rp *Report) freeze(trees map[string]*tree) map[string]map[string]int {
	position := map[string]map[string]int{}
	for gid, t := range trees {
		rp.Chains[gid] = t.chain
		if t.forked {
			rp.Compromised = append(rp.Compromised, gid)
		}
		pos := map[string]int{}
		onChain := map[string]bool{}
		for i, c := range t.chain {
			pos[c.ValueKey()] = i
			onChain[c.ID] = true
			rp.OnChain[c.ID] = true
		}
		for id, s := range t.nodes {
			if !onChain[id] {
				rp.fail(id, "frozen")
				pos[s.ValueKey()] = offChain
			}
		}
		position[gid] = pos
	}
	sort.Strings(rp.Compromised)
	return position
}

// checkKeys is steps 6 to 8: resolve the signing key, check the seal of a
// retired key, and apply the freeze.
func (rp *Report) checkKeys(ids []string, trees map[string]*tree, position map[string]map[string]int, sealedBy map[string]map[string]bool) {
	for _, id := range ids {
		r := rp.Records[id]
		if r.IsIdentity() {
			continue
		}
		if IsTaggedKey(r.Author) {
			if r.Kid != r.Author {
				rp.fail(id, "bad signature")
			}
			continue
		}
		t, ok := trees[r.Author]
		if !ok {
			rp.warn(id, "unresolved author: "+r.Author)
			delete(rp.Valid, id)
			continue
		}
		p, ok := position[r.Author][r.Kid]
		if !ok {
			rp.fail(id, "key not in chain")
			continue
		}
		// Step 6.4: off-chain keys exist only below a fork.
		if p == offChain {
			rp.fail(id, "frozen")
			continue
		}
		// Step 7 (C26): every record of a retired key needs the seal of
		// the succession that retired it.
		if p < len(t.chain)-1 && !sealedBy[t.chain[p+1].ID][id] {
			rp.fail(id, "not sealed")
			continue
		}
		// Step 8: a frozen identity keeps the claims of chain keys and
		// loses its destructive records.
		if t.forked && r.IsDestructive() {
			rp.fail(id, "frozen")
		}
	}
}

// checkSupersedes is step 9.
func (rp *Report) checkSupersedes(ids []string) {
	for _, id := range ids {
		r := rp.Records[id]
		if !rp.Valid[id] || r.IsIdentity() {
			continue
		}
		held := false
		for _, t := range r.Targets("supersedes") {
			target, ok := rp.Records[t]
			switch {
			case !ok:
				held = true
			case strings.HasPrefix(target.Type, "bean."):
				rp.fail(id, "bad target")
			case target.Author != r.Author:
				rp.fail(id, "not author")
			}
			if !rp.Valid[id] {
				break
			}
		}
		// One warning per record, and only for a record that stays valid.
		if held && rp.Valid[id] {
			rp.warn(id, "held: "+id)
			rp.Held[id] = true
		}
	}
}

// checkTombstones is step 10.
func (rp *Report) checkTombstones(ids []string) {
	for _, id := range ids {
		r := rp.Records[id]
		if !rp.Valid[id] || r.Type != "bean.tombstone" {
			continue
		}
		t := r.Targets("purges")[0]
		target, ok := rp.Records[t]
		switch {
		case !ok:
			rp.warn(id, "held: "+id)
			rp.Held[id] = true
		case target.IsIdentity() || target.Type == "bean.tombstone":
			rp.fail(id, "bad target")
		case target.Subject != r.Subject:
			rp.fail(id, "not subject")
		default:
			if prev, has := rp.Purged[t]; !has || id < prev {
				rp.Purged[t] = id
			}
		}
	}
	for t := range rp.Purged {
		delete(rp.Valid, t)
	}
}

// mapSuperseded records the valid supersedes after purges: a purged
// supersede record no longer counts.
func (rp *Report) mapSuperseded(ids []string) {
	for _, id := range ids {
		if !rp.Valid[id] {
			continue
		}
		for _, t := range rp.Records[id].Targets("supersedes") {
			if _, ok := rp.Records[t]; ok {
				rp.Superseded[t] = append(rp.Superseded[t], id)
			}
		}
	}
	for _, list := range rp.Superseded {
		// ids is sorted, so each list is too.
		for _, id := range list[1:] {
			rp.warn(id, "superseded by sibling: "+list[0])
		}
	}
}

// checkSubjects is step 11.
func (rp *Report) checkSubjects(ids []string, trees map[string]*tree) {
	for _, id := range ids {
		r := rp.Records[id]
		if rp.Valid[id] && r.Subject != "" && !r.IsIdentity() {
			if _, ok := trees[r.Subject]; !ok {
				rp.warn(id, "unresolved subject: "+r.Subject)
			}
		}
	}
}

// findDuplicates is step 12. A superseded record is left out: a parser fix
// (9.7) keeps the ref of the record it corrects, and grouping the two
// would hide the fix as a duplicate whenever its id is the higher one.
// The author is part of the group (C27): a record of one key is never a
// duplicate of another key's record, so no key can hide another key's
// record by copying its ref.
func (rp *Report) findDuplicates(ids []string) {
	first := map[[4]string]string{}
	for _, id := range ids {
		r := rp.Records[id]
		if !rp.Valid[id] || r.Source.Ref == "" || r.IsIdentity() || len(rp.Superseded[id]) > 0 {
			continue
		}
		k := [4]string{r.Subject, r.Author, r.Source.System, r.Source.Ref}
		if p, ok := first[k]; ok {
			rp.Duplicates[id] = p
			rp.warn(id, "duplicate: "+id+" of "+p)
		} else {
			first[k] = id
		}
	}
}

func contains(list []string, s string) bool {
	i := sort.SearchStrings(list, s)
	return i < len(list) && list[i] == s
}

// Lines renders the verify output of SPEC.md 12.1, sorted, summary last.
func (rp *Report) Lines() []string {
	var out []string
	for id, e := range rp.Errors {
		out = append(out, fmt.Sprintf("error: %s: %s", id, e))
	}
	for id, ws := range rp.Warnings {
		for _, w := range ws {
			out = append(out, fmt.Sprintf("warning: %s: %s", id, w))
		}
	}
	for _, id := range rp.TooNew {
		out = append(out, "too new: "+id)
	}
	for _, id := range rp.Compromised {
		out = append(out, "compromised: "+id)
	}
	sort.Strings(out)
	out = append(out, fmt.Sprintf("summary: %d valid, %d invalid, %d skipped, %d held, %d duplicate", len(rp.Valid), len(rp.Errors), rp.Skipped, len(rp.Held), len(rp.Duplicates)))
	return out
}

// ExitCode is the verify exit code (SPEC.md 12.1). local lists the identities
// held in local/: only their state is exit 3 (too new) or 4 (frozen). Any
// other too-new identity is a line, not a halt (8.2).
func (rp *Report) ExitCode(local []string) int {
	code := 0
	if len(rp.Errors) > 0 || len(rp.Incomplete) > 0 {
		code = 1
	}
	for _, l := range local {
		if contains(rp.TooNew, l) {
			code = max(code, 3)
		}
		if contains(rp.Compromised, l) {
			code = 4
		}
	}
	return code
}

// sealSets checks the bean.seal continuations (C28) and returns the seal
// set of each chain succession. It marks bad parts invalid.
func sealSets(rp *Report, ids []string) map[string]map[string]bool {
	onChain := map[string]bool{}
	for _, chain := range rp.Chains {
		for _, c := range chain[1:] {
			onChain[c.ID] = true
		}
	}
	parts := map[string]map[int][]*Record{} // succession id -> part -> parts
	for _, id := range ids {
		r := rp.Records[id]
		if r.Type != "bean.seal" {
			continue
		}
		sid := r.Targets("seals")[0]
		s, ok := rp.Records[sid]
		if !ok {
			rp.warn(id, "held: "+id)
			rp.Held[id] = true
			continue
		}
		// Same identity, same recovery key as the succession, a part
		// number the succession announces, and a succession on the chain.
		if s.Type != "bean.succession" || r.Subject != s.Subject || r.Kid != s.Kid || r.Part() > s.Parts() || !onChain[sid] || !rp.Valid[sid] {
			rp.fail(id, "bad seal")
			continue
		}
		if parts[sid] == nil {
			parts[sid] = map[int][]*Record{}
		}
		parts[sid][r.Part()] = append(parts[sid][r.Part()], r)
	}
	out := map[string]map[string]bool{}
	for _, gid := range sortedKeys(rp.Chains) {
		for _, c := range rp.Chains[gid][1:] {
			set := map[string]bool{}
			for _, x := range c.Sealed() {
				set[x] = true
			}
			complete := true
			for k := 1; k <= c.Parts(); k++ {
				list := parts[c.ID][k]
				if len(list) != 1 {
					complete = false
					for _, d := range list { // two parts for one slot: only a seed holder can sign them
						rp.fail(d.ID, "bad seal")
					}
					continue
				}
				for _, x := range list[0].Sealed() {
					set[x] = true
				}
			}
			if !complete {
				rp.warn(c.ID, "incomplete seals: "+c.ID)
				rp.Incomplete = append(rp.Incomplete, c.ID)
				set = map[string]bool{}
			}
			out[c.ID] = set
		}
	}
	sort.Strings(rp.Incomplete)
	return out
}
