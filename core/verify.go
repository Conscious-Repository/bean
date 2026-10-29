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
	Record  *Record // set when steps 1 to 15 pass, or for an unknown version after step 12
	Err     error   // first failure
	Unknown bool    // unknown v: Record has only the envelope
}

// CheckFile applies SPEC.md 3.8 steps 1 to 15 (order from change set C2:
// envelope, id and signature come before the version check).
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
	if r.Type != "bean.succession" && len(data) > MaxRecordSize {
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
	// 13. version
	if !knownVersions[r.V] {
		return FileResult{Record: r, Unknown: true}
	}
	// 14. canonical
	if string(Canonical(obj)) != string(data) {
		return FileResult{Err: ErrNotCanonical}
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
	Duplicates  map[string]string    // duplicate id -> primary id
	Skipped     int
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

// checkAll runs checkFile on every path on all CPUs. Results are by index,
// so the outcome does not depend on scheduling.
func checkAll(paths []string, files map[string][]byte, proven map[string]bool) []FileResult {
	out := make([]FileResult, len(paths))
	var next atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < runtime.GOMAXPROCS(0); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1) - 1)
				if i >= len(paths) {
					return
				}
				out[i] = checkFile(paths[i], files[paths[i]], proven[paths[i]])
			}
		}()
	}
	wg.Wait()
	return out
}

// verifyFiles is VerifyFiles; proven names paths whose signatures this
// replica has already checked.
func verifyFiles(files map[string][]byte, proven map[string]bool) *Report {
	rp := &Report{
		Records: map[string]*Record{}, Valid: map[string]bool{}, Errors: map[string]string{},
		Warnings: map[string][]string{}, Chains: map[string][]*Record{}, Superseded: map[string][]string{},
		Purged: map[string]string{}, Held: map[string]bool{}, Duplicates: map[string]string{},
	}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	// Step 1: check files.
	var unknown []*Record
	for i, fr := range checkAll(paths, files, proven) {
		p := paths[i]
		switch {
		case fr.Err != nil:
			rp.Errors[p] = fr.Err.Error()
		case fr.Unknown:
			unknown = append(unknown, fr.Record)
		default:
			rp.Records[fr.Record.ID] = fr.Record
		}
	}
	ids := make([]string, 0, len(rp.Records))
	for id := range rp.Records {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// Step 2: identities.
	genesis := map[string]*Record{}
	for _, id := range ids {
		r := rp.Records[id]
		if r.Type == "bean.genesis" {
			genesis[id] = r
		}
	}
	// Step 3: succession trees (seed-only rule, change set C5 to C11).
	nodes := map[string]map[string]*Record{} // identity -> node id -> node
	nodeKeys := map[string]map[string]bool{} // identity -> signing or recovery keys of nodes
	for gid, g := range genesis {
		n := map[string]*Record{gid: g}
		keys := map[string]bool{g.ValueKey(): true}
		pending := []*Record{}
		for _, id := range ids {
			r := rp.Records[id]
			if r.Type == "bean.succession" && r.Author == gid {
				pending = append(pending, r)
			}
		}
		for progress := true; progress; {
			progress = false
			var rest []*Record
			for _, s := range pending {
				p, ok := n[s.Targets("succeeds")[0]]
				if !ok {
					rest = append(rest, s)
					continue
				}
				progress = true
				if err := checkSuccession(s, p, n); err != nil {
					rp.fail(s.ID, err.Error())
					continue
				}
				n[s.ID] = s
				keys[s.ValueKey()] = true
				keys[s.Kid] = true
			}
			pending = rest
		}
		for _, s := range pending {
			rp.warn(s.ID, "held: "+s.ID)
			rp.Held[s.ID] = true
		}
		nodes[gid], nodeKeys[gid] = n, keys
	}
	// Too new (SPEC.md 8.2 with change set C15): an unknown-v hlog.* record
	// counts only when its signer is a key of the identity's known tree.
	tooNew := map[string]bool{}
	for _, r := range unknown {
		if !strings.HasPrefix(r.Type, "bean.") {
			rp.Skipped++
			continue
		}
		rp.Skipped++
		if r.Type == "bean.genesis" {
			if r.Author == r.Kid {
				tooNew[r.ID] = true
			}
			continue
		}
		if keys, ok := nodeKeys[r.Author]; ok && keys[r.Kid] {
			tooNew[r.Author] = true
		}
	}
	for id := range tooNew {
		rp.TooNew = append(rp.TooNew, id)
	}
	sort.Strings(rp.TooNew)
	// Drop everything of a too-new identity.
	for _, id := range ids {
		r := rp.Records[id]
		if tooNew[r.Author] || tooNew[r.Subject] || (r.Type == "bean.genesis" && tooNew[r.ID]) {
			delete(rp.Records, id)
			rp.Skipped++
		}
	}
	for gid := range tooNew {
		delete(genesis, gid)
		delete(nodes, gid)
	}
	ids = ids[:0]
	for id := range rp.Records {
		ids = append(ids, id)
		if _, failed := rp.Errors[id]; !failed {
			rp.Valid[id] = true
		}
	}
	sort.Strings(ids)

	// Steps 4 and 5: freeze and chain. Any fork is a freeze (C7, C11).
	frozen := map[string]*Record{} // identity -> lowest common ancestor
	position := map[string]map[string]int{}
	for gid, g := range genesis {
		n := nodes[gid]
		chain := []*Record{g}
		pos := map[string]int{g.ValueKey(): 0}
		x := g
		for {
			var children []*Record
			for _, s := range n {
				if s != g && s.Targets("succeeds")[0] == x.ID {
					children = append(children, s)
				}
			}
			if len(children) == 0 {
				break
			}
			if len(children) > 1 {
				frozen[gid] = x
				rp.Compromised = append(rp.Compromised, gid)
				break
			}
			x = children[0]
			chain = append(chain, x)
			pos[x.ValueKey()] = len(chain) - 1
		}
		rp.Chains[gid] = chain
		position[gid] = pos
		onChain := map[string]bool{}
		for _, c := range chain {
			onChain[c.ID] = true
		}
		for id, s := range n {
			if !onChain[id] {
				rp.fail(id, "frozen")
				pos[s.ValueKey()] = offChain
			}
		}
	}
	sort.Strings(rp.Compromised)

	// Step 6: resolve signing keys; step 7: seals; step 8: frozen rules.
	recPos := map[string]int{}
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
		if _, ok := genesis[r.Author]; !ok {
			rp.warn(id, "unresolved author: "+r.Author)
			delete(rp.Valid, id)
			continue
		}
		p, ok := position[r.Author][r.Kid]
		if !ok {
			rp.fail(id, "key not in chain")
			continue
		}
		recPos[id] = p
		chain := rp.Chains[r.Author]
		if lca, isFrozen := frozen[r.Author]; isFrozen {
			if r.IsDestructive() || p > recPos0(chain, lca) {
				rp.fail(id, "frozen")
				continue
			}
		} else if p == offChain {
			rp.fail(id, "key not in chain")
			continue
		}
		if r.IsDestructive() && p < len(chain)-1 {
			if !contains(chain[p+1].Sealed(), id) {
				rp.fail(id, "not sealed")
			}
		}
	}

	// Step 9: supersede rules.
	for _, id := range ids {
		r := rp.Records[id]
		if !rp.Valid[id] || r.IsIdentity() {
			continue
		}
		for _, t := range r.Targets("supersedes") {
			target, ok := rp.Records[t]
			switch {
			case !ok:
				rp.warn(id, "held: "+id)
				rp.Held[id] = true
			case strings.HasPrefix(target.Type, "bean."):
				rp.fail(id, "bad target")
			case target.Author != r.Author:
				rp.fail(id, "not author")
			}
			if !rp.Valid[id] {
				break
			}
		}
	}
	// Step 10: tombstones.
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
	// Supersede map after purges (a purged supersede record no longer counts).
	for _, id := range ids {
		r := rp.Records[id]
		if !rp.Valid[id] {
			continue
		}
		for _, t := range r.Targets("supersedes") {
			if _, ok := rp.Records[t]; ok {
				rp.Superseded[t] = append(rp.Superseded[t], id)
			}
		}
	}
	for t, list := range rp.Superseded {
		sort.Strings(list)
		rp.Superseded[t] = list
		for _, id := range list[1:] {
			rp.warn(id, "superseded by sibling: "+list[0])
		}
	}
	// Step 11: subjects.
	for _, id := range ids {
		r := rp.Records[id]
		if rp.Valid[id] && r.Subject != "" && !r.IsIdentity() {
			if _, ok := genesis[r.Subject]; !ok {
				rp.warn(id, "unresolved subject: "+r.Subject)
			}
		}
	}
	// Step 12: duplicates. A superseded record is left out: a parser fix
	// (9.7) keeps the ref of the record it corrects, and grouping the two
	// would hide the fix as a duplicate whenever its id is the higher one.
	groups := map[[3]string][]string{}
	for _, id := range ids {
		r := rp.Records[id]
		if rp.Valid[id] && r.Source.Ref != "" && !r.IsIdentity() && len(rp.Superseded[id]) == 0 {
			k := [3]string{r.Subject, r.Source.System, r.Source.Ref}
			groups[k] = append(groups[k], id)
		}
	}
	for _, g := range groups {
		sort.Strings(g)
		for _, id := range g[1:] {
			rp.Duplicates[id] = g[0]
			rp.warn(id, "duplicate: "+id+" of "+g[0])
		}
	}
	return rp
}

// offChain marks a key of a tree node that is below a freeze point.
const offChain = 1 << 30

func recPos0(chain []*Record, node *Record) int {
	for i, c := range chain {
		if c == node {
			return i
		}
	}
	return -1
}

func contains(list []string, s string) bool {
	i := sort.SearchStrings(list, s)
	return i < len(list) && list[i] == s
}

// checkSuccession applies SPEC.md 6.3 step 3 and 5.8 with the seed-only rule.
func checkSuccession(s, p *Record, tree map[string]*Record) error {
	if Commitment(s.Kid) != p.Recovery() {
		return errors.New("bad succession")
	}
	if s.Recovery() == p.Recovery() {
		return errors.New("recovery not advanced")
	}
	// key reuse: along the path from genesis to the parent.
	for x := p; x != nil; {
		if x.ValueKey() == s.ValueKey() || x.Kid == s.ValueKey() {
			return errors.New("key reuse")
		}
		if x.Type == "bean.genesis" {
			break
		}
		x = tree[x.Targets("succeeds")[0]]
	}
	return nil
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
// held in local/, whose freeze is exit 4.
func (rp *Report) ExitCode(local []string) int {
	code := 0
	if len(rp.Errors) > 0 {
		code = 1
	}
	if len(rp.TooNew) > 0 {
		code = 3
	}
	for _, l := range local {
		if contains(rp.Compromised, l) {
			code = 4
		}
	}
	return code
}
