package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"sort"
	"strconv"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/pack"
)

// bean check-bundle -pack computes a shared comparison again, by the four
// steps of docs/REFERENCE.md, "The claim". tools/compare/reproduce.py is a
// second implementation that shares no code with this one, and
// tools/pack/run.sh holds the two to one verdict on each claim.
//
// A claim that computes again shows that its numbers follow from its
// reading and its pack. It does not show that the reading is true, or that
// the pack is right.

// givenPacks are the files that the reader gave with -pack, by SHA-256: a
// pack that bean reads, or why it is not one.
type givenPacks struct {
	packs map[string]*pack.Pack
	bad   map[string]string
	path  map[string]string
}

// readGivenPacks reads each file that -pack names. A file that cannot be
// read, or that is larger than a pack can be, is an error of use. A file that is not a pack is kept by its SHA-256,
// with the reason: a claim that names it differs.
func readGivenPacks(paths []string) givenPacks {
	g := givenPacks{packs: map[string]*pack.Pack{}, bad: map[string]string{}, path: map[string]string{}}
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			badArg(fmt.Errorf("-pack: %w", err))
		}
		// No more is kept than a pack can be, and every byte is hashed: a
		// pack is named by the SHA-256 of all of its bytes.
		h := sha256.New()
		data, err := io.ReadAll(io.TeeReader(io.LimitReader(f, pack.MaxBytes+1), h))
		f.Close()
		if err != nil {
			badArg(fmt.Errorf("-pack %s: %w", path, err))
		}
		// A larger file is no pack, and a pipe may never end.
		if len(data) > pack.MaxBytes {
			badArg(fmt.Errorf("-pack %s: a pack is at most %d MiB, and this file is larger. Get the pack from its publisher", path, pack.MaxBytes>>20))
		}
		digest := hex.EncodeToString(h.Sum(nil))
		g.path[digest] = path
		p, err := pack.Read(data)
		if err != nil {
			g.bad[digest] = err.Error()
			continue
		}
		g.packs[digest] = p
	}
	return g
}

// cannotCheck is a check that cannot be made: an input is not at hand. It
// is not a claim that differs.
type cannotCheck struct{ text string }

func (e cannotCheck) Error() string { return e.text }

func differs(format string, a ...any) error { return fmt.Errorf(format, a...) }

// The members of a record, and of the context of a claim by a pack
// (docs/REFERENCE.md, "The claim").
var (
	recordMembers    = []string{"v", "id", "type", "subject", "author", "kid", "nonce", "sig", "times", "value", "refs", "source", "context"}
	claimMembers     = []string{"reading", "reference", "cohort", "method", "result", "caveats"}
	referenceMembers = []string{"pack", "title", "source", "measure", "label", "unit", "cohort"}
	resultMembers    = []string{"n", "percentile", "below", "equal", "p25", "p50", "p75"}
)

// hasExactly reports an object with exactly these members.
func hasExactly(v any, members ...string) bool {
	obj, ok := v.(map[string]any)
	if !ok || len(obj) != len(members) {
		return false
	}
	for _, m := range members {
		if _, has := obj[m]; !has {
			return false
		}
	}
	return true
}

// aboutOnly reports a list of refs that is one entry: about the reading.
func aboutOnly(v any, reading string) bool {
	list, ok := v.([]any)
	return ok && len(list) == 1 && isAbout(list[0], reading)
}

func isAbout(v any, reading string) bool {
	ref, _ := v.(map[string]any)
	return hasExactly(ref, "rel", "id") && ref["rel"] == "about" && ref["id"] == reading
}

func number(x float64) string { return strconv.FormatFloat(x, 'f', -1, 64) }

// recomputeClaim checks one claim by the four steps of docs/REFERENCE.md,
// with the records of the bundle and the packs given. It returns what the
// claim compares when it computes again. Otherwise it returns a
// cannotCheck, or an error that says what differs. unrecorded collects
// each pack that bean has no record of and that a claim was computed with.
// The checks run in the order that reproduce.py runs them, so that the two
// give one verdict on each claim.
func recomputeClaim(c *core.Record, records map[string]*core.Record, g givenPacks, unrecorded map[string]bool) (string, error) {
	ctx, _ := c.Raw["context"].(map[string]any)
	method, ok := ctx["method"].(map[string]any)
	if !ok {
		return "", differs("the claim names no method")
	}
	if id, _ := method["id"].(string); id != pack.MethodID || (method["version"] != "1" && method["version"] != "2") {
		return "", differs("a method this bean does not compute again: %v@%v", method["id"], method["version"])
	}
	// The reading is in the bundle, and the claim is about it.
	rid, ok := ctx["reading"].(string)
	if !ok {
		return "", differs("the claim's reading is not a record id")
	}
	reading := records[rid]
	if reading == nil {
		return "", cannotCheck{fmt.Sprintf("the reading %s is not in the bundle. Ask the sender for a bundle that holds it", short(rid))}
	}
	// A bundle that passes holds no reading of another person: a hand-built
	// one could. reproduce.py refuses it too.
	if reading.Subject != c.Subject {
		return "", differs("the claim and its reading are about two people")
	}
	rv, _ := reading.Raw["value"].(map[string]any)
	value, ok := rv["n"].(float64)
	if !ok || value < 0 {
		return "", differs("the reading is not a number of zero or more")
	}
	// The record around the claim is the one that bean compare writes. Its
	// own time is not read: take the time from the reading (DECISIONS 347).
	if !hasExactly(c.Raw, recordMembers...) {
		return "", differs("the claim does not have exactly the members of a record")
	}
	source, _ := c.Raw["source"].(map[string]any)
	if !hasExactly(source, "system", "adapter", "ref") || source["system"] != compareSystem || source["ref"] != nil {
		return "", differs("the claim's source is not a comparison of bean")
	}
	if !hasExactly(c.Raw["times"], "observed", "imported") {
		return "", differs("the claim's times are not the two that a record has")
	}
	if c.Author != c.Subject {
		return "", differs("the claim is not signed by the person it is about")
	}
	for _, m := range []struct {
		name    string
		v       any
		members []string
	}{{"context", ctx, claimMembers}, {"reference", ctx["reference"], referenceMembers}, {"cohort", ctx["cohort"], []string{"text"}},
		{"method", method, []string{"id", "version"}}, {"result", ctx["result"], resultMembers}} {
		if !hasExactly(m.v, m.members...) {
			return "", differs("the %s of the claim does not have exactly the members that the format names", m.name)
		}
	}
	ref := ctx["reference"].(map[string]any)
	want := ctx["result"].(map[string]any)
	// Step 1: the pack has the SHA-256 that the claim names.
	digest, _ := ref["pack"].(string)
	if !pack.IsDigest(digest) {
		return "", differs("reference.pack is not a SHA-256")
	}
	if why, bad := g.bad[digest]; bad {
		return "", differs("the pack %s is not a pack that bean reads: %s", digest[:12], why)
	}
	if !aboutOnly(c.Raw["refs"], rid) {
		return "", differs("the claim's refs are not the one reading it is about")
	}
	p := g.packs[digest]
	if p == nil {
		return "", cannotCheck{fmt.Sprintf("the pack with SHA-256 %s was not given. Get that pack, and pass its file with -pack", digest)}
	}
	mid, okM := ref["measure"].(string)
	cid, okC := ref["cohort"].(string)
	if !okM || !okC {
		return "", differs("reference.measure and reference.cohort are not ids")
	}
	// A cell names a measure and a cohort of the pack.
	cell := p.Cell(mid, cid)
	if cell == nil {
		return "", differs("the pack has no cell for measure %s and cohort %s", mid, cid)
	}
	measure, cohort := p.Measures[mid], p.Cohorts[cid]
	// Step 2: the reading matches an entry of the measure.
	unit, hasUnit := reading.Value["unit"]
	unitText := ""
	if hasUnit {
		unitText = fmt.Sprint(unit)
	}
	if id, _, _ := p.MatchRecord(reading); id != mid {
		return "", differs("the pack does not set %s in %s against %s", reading.Type, unitText, mid)
	}
	// Step 3: the texts and the caveats are the pack's.
	for _, t := range []struct{ name, got, want string }{
		{"title", text(ref["title"]), p.Title}, {"source", text(ref["source"]), p.Source},
		{"label", text(ref["label"]), measure.Label}, {"unit", text(ref["unit"]), measure.Unit},
		{"cohort.text", text(ctx["cohort"].(map[string]any)["text"]), cohort.Text},
	} {
		if t.got != t.want {
			// Both texts are quotes: the claim's words are the sender's.
			claim := "says: " + quoted(t.got)
			if t.got == text(nil) {
				claim = "holds no text"
			}
			return "", differs("%s is not the pack's: the claim %s, and the pack says: %s", t.name, claim, quoted(t.want))
		}
	}
	if method["id"] != pack.MethodID || method["version"] != cell.MethodVersion() {
		return "", differs("the method is not the one that this cell takes: %s@%s", pack.MethodID, cell.MethodVersion())
	}
	// A claim means the same for ever. That the pack was not on record is
	// what the signer's bean knew that day, and a pack can be put on
	// record later: for a pack on record the claim may say so, in its
	// place, or not (DECISIONS 319). For any other pack it must say so.
	caveats := p.PlacementCaveats(mid, cid, cell.N, reading.Author != c.Author)
	allowed := [][]string{caveats}
	onRecord := pack.IsKnown(digest)
	if onRecord {
		at := len(caveats)
		if reading.Author != c.Author {
			at--
		}
		allowed = append(allowed, slices.Concat(caveats[:at], []string{pack.CaveatNotOnRecord}, caveats[at:]))
	} else {
		unrecorded[digest] = true
	}
	known := "on record"
	if !onRecord {
		known = "NOT on record"
	}
	what := fmt.Sprintf("%s %s %s against %s in pack %s (%s)", reading.Type, number(value), unitText, mid, digest[:12], known)
	// Step 4: the method gives every number of the result.
	res, err := cell.Place(value)
	if err != nil {
		return "", differs("%s: %v", what, err)
	}
	got := map[string]float64{"n": float64(res.N), "percentile": res.Percentile, "below": res.Below, "equal": res.Equal, "p25": res.P25, "p50": res.P50, "p75": res.P75}
	for _, k := range resultMembers {
		if x, ok := want[k].(float64); !ok || x != got[k] {
			return "", differs("%s: the signed claim says %s, bean computes %s", what, core.Canonical(want), core.Canonical(anyMap(got)))
		}
	}
	cv, _ := c.Raw["value"].(map[string]any)
	if n, ok := cv["n"].(float64); !hasExactly(cv, "n", "unit") || !ok || n != res.Percentile || cv["unit"] != "{percentile}" {
		return "", differs("%s: the claim's value is not the percentile in {percentile}", what)
	}
	have, ok := texts(ctx["caveats"])
	if !ok || !slices.ContainsFunc(allowed, func(a []string) bool { return slices.Equal(a, have) }) {
		return "", differs("%s: the caveats are not the ones that the pack and the reading give", what)
	}
	// The pack's own words are a quote.
	what = fmt.Sprintf("%s %s %s against %s (the pack says: %s), cohort %s (the pack says: %s), in pack %s (%s)", reading.Type, number(value), unitText, mid, strconv.Quote(measure.Label), cid, strconv.Quote(cohort.Text), digest[:12], known)
	return fmt.Sprintf("%s: percentile %s, n %d", what, number(res.Percentile), res.N), nil
}

func text(v any) string {
	s, ok := v.(string)
	if !ok {
		return "\x00not text"
	}
	return s
}

// texts returns a list of texts, and false when v is not one.
func texts(v any) ([]string, bool) {
	list, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := []string{}
	for _, x := range list {
		s, ok := x.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func anyMap(m map[string]float64) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func short(id string) string {
	if len(id) >= 19 {
		return id[7:19]
	}
	return id
}

// recomputeLines computes again each comparison of a bundle that passed.
// It returns the lines to print, and whether every check was made and
// every claim computes again.
func recomputeLines(in map[string][]byte, g givenPacks, accepted []string) ([]string, bool) {
	records := core.VerifyFiles(in).Records
	var claims []*core.Record
	for _, r := range records {
		if r.Type == compareType {
			claims = append(claims, r)
		}
	}
	sort.Slice(claims, func(i, j int) bool { return claims[i].ID < claims[j].ID })
	lines := []string{"Computed again with the packs given, by the four steps of docs/REFERENCE.md, \"The claim\":"}
	unrecorded := map[string]bool{}
	named := map[string]bool{}
	again, diff, cannot, files := 0, 0, 0, 0
	for _, c := range claims {
		// A comparison with the survey's files is not computed here.
		if m, _ := c.Context["method"].(map[string]any); m != nil && m["id"] == "nhanes-weighted-percentile" && m["version"] == "1" {
			files++
			lines = append(lines, fmt.Sprintf("  %s  not computed here: it says that it was made with the survey's files, not with a pack. To check it, give tools/compare/reproduce.py the bundle and those files", c.ID[7:19]))
			continue
		}
		by := ""
		if ref, _ := c.Context["reference"].(map[string]any); ref != nil {
			if d, _ := ref["pack"].(string); d != "" {
				named[d] = true
				by = d
			}
		}
		text, err := recomputeClaim(c, records, g, unrecorded)
		var cc cannotCheck
		switch {
		case err == nil:
			again++
			// The numbers follow from the pack. A pack that bean has no
			// record of counts only when the reader accepts it, and the
			// claim's own line says so: a reader may stop at that line.
			word := "computes again"
			if unrecorded[by] && !slices.Contains(accepted, by) {
				word = "computes again, with a pack NOT accepted"
			}
			lines = append(lines, fmt.Sprintf("  %s  %s: %s", c.ID[7:19], word, text))
		case errors.As(err, &cc):
			cannot++
			lines = append(lines, fmt.Sprintf("  %s  CANNOT CHECK: %s", c.ID[7:19], err))
		default:
			diff++
			lines = append(lines, fmt.Sprintf("  %s  DIFFERS: %s", c.ID[7:19], err))
		}
	}
	// A claim that is not computed here is not checked: the run does not
	// pass with it.
	ok := diff == 0 && cannot == 0 && files == 0 && again > 0
	for _, digest := range slices.Sorted(maps.Keys(g.bad)) {
		lines = append(lines, fmt.Sprintf("REFUSED: the file %s, SHA-256 %s: %s. Get the pack from its publisher: this file is not one", g.path[digest], digest, g.bad[digest]))
		ok = false
	}
	for _, digest := range slices.Sorted(maps.Keys(g.packs)) {
		if !named[digest] {
			lines = append(lines, fmt.Sprintf("the pack %s (SHA-256 %s) is named by no comparison here", g.path[digest], digest))
		}
	}
	// A comparison computes again with any pack that is well formed. What
	// it means depends on who built the pack.
	accepts := false
	for _, digest := range slices.Sorted(maps.Keys(unrecorded)) {
		if slices.Contains(accepted, digest) {
			accepts = true
			lines = append(lines, fmt.Sprintf("accepted with -accept-pack: the pack %s, which bean has no record of. Its tables, its caveats and what it sets a reading against are its builder's word", digest))
			continue
		}
		lines = append(lines, fmt.Sprintf("NOT on record: the pack %s. bean has no record of it: its tables, its caveats and what it sets a reading against are its builder's word. If you trust its publisher, compare this SHA-256 with the one that they state. Then pass -accept-pack <sha256> with the SHA-256 that the publisher states", digest))
		ok = false
	}
	for _, digest := range slices.Compact(slices.Sorted(slices.Values(accepted))) {
		if unrecorded[digest] {
			continue
		}
		var why string
		switch {
		case pack.IsKnown(digest):
			why = "it names a pack that bean has on record, which needs no -accept-pack"
		case g.packs[digest] == nil:
			why = "no pack that was given with -pack has that SHA-256"
		case !named[digest]:
			why = "no comparison here names that pack"
		default:
			why = "each comparison with that pack stopped at an earlier check, above"
		}
		lines = append(lines, fmt.Sprintf("-accept-pack %s does not count: %s", digest, why))
		ok = false
	}
	if len(claims) == 0 {
		lines = append(lines, "the bundle holds no comparison: there is nothing to compute again. The check of the bundle passed, as bundle ok says. Without -pack, its exit is 0")
	}
	lines = append(lines,
		"A comparison that computes again shows that its numbers follow from its reading and its pack. It does not show that the reading is true, or that the pack is right.",
		fmt.Sprintf("comparisons: %d compute again, %d differ, %d cannot be checked, %d with the survey's files not computed here", again, diff, cannot, files))
	if diff > 0 {
		lines = append(lines, "A comparison that DIFFERS is not what its reading and its pack give. Ask the sender, and do not rely on its numbers.")
	}
	switch {
	case ok && accepts:
		lines = append(lines, "every comparison by a pack computes again. A pack that bean has no record of counts only by -accept-pack, as a line above says")
	case ok:
		lines = append(lines, "every comparison by a pack computes again")
	default:
		lines = append(lines, "computed again: NOT passed. The lines above say what differs or is missing, and what you can do")
	}
	return lines, ok
}
