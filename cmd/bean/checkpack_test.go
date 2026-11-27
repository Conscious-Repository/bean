package main

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// packClaims is a log with comparisons saved with the test pack: one for
// a cholesterol, one for an HDL and one for a weight, in male-30-39. It
// also holds a reading of LDL, which the pack does not take, with the value
// and the unit of the cholesterol. flags go to bean compare.
type packClaims struct {
	dir, pack, digest string
	ldl, weight       string
	// claim is the saved claim about the cholesterol, as bean ls -json
	// shows it.
	claim map[string]any
}

func newPackClaims(t *testing.T, flags ...string) packClaims {
	t.Helper()
	pc := packClaims{dir: t.TempDir()}
	pc.pack, pc.digest = bodyPack(t)
	run(t, pc.dir, "", "init")
	var ids []string
	for _, a := range [][]string{
		{"lab.cholesterol-total", "193", "-unit", "mg/dL"},
		{"lab.hdl-cholesterol", "52", "-unit", "mg/dL"},
		{"weight", "70", "-unit", "kg"},
		{"lab.ldl-cholesterol", "193", "-unit", "mg/dL"},
	} {
		out, errs, code := run(t, pc.dir, "", append(append([]string{"add"}, a...), "-at", "2026-11-01")...)
		if code != 0 {
			t.Fatalf("%v: %s", a, errs)
		}
		ids = append(ids, strings.TrimSpace(out))
	}
	pc.weight, pc.ldl = ids[2], ids[3]
	args := append([]string{"compare", "-pack", pc.pack, "-cohort", "male-30-39", "-save"}, flags...)
	if out, errs, code := run(t, pc.dir, "", append(args, ids[:3]...)...); code != 0 || !strings.Contains(out, "added 3,") {
		t.Fatalf("compare: %d\n%s\n%s", code, out, errs)
	}
	for _, c := range lsClaims(t, pc.dir) {
		if c["context"].(map[string]any)["reading"] == ids[0] {
			pc.claim = c
		}
	}
	if pc.claim == nil {
		t.Fatal("no claim about the cholesterol")
	}
	return pc
}

// lsClaims are the comparisons of a log, as bean ls -json shows them.
func lsClaims(t *testing.T, dir string) []map[string]any {
	t.Helper()
	out, errs, code := run(t, dir, "", "ls", "-json", "-type", "comparison.percentile")
	if code != 0 {
		t.Fatalf("ls: %s", errs)
	}
	var claims []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var c map[string]any
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatal(err)
		}
		claims = append(claims, c)
	}
	return claims
}

// bundleOf shares every comparison of a log, with what comes along, and
// returns the bundle's folder.
func bundleOf(t *testing.T, dir string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "bundle")
	if o, errs, code := run(t, dir, "", "share", "-yes", "-out", out, "-type", "comparison.*"); code != 0 {
		t.Fatalf("share: %d\n%s\n%s", code, o, errs)
	}
	return out
}

// copyLog is a copy of a log, to forge a claim in.
func copyLog(t *testing.T, dir string) string {
	t.Helper()
	to := filepath.Join(t.TempDir(), "log")
	if err := os.CopyFS(to, os.DirFS(dir)); err != nil {
		t.Fatal(err)
	}
	return to
}

var verdictLine = regexp.MustCompile(`(?m)^  ([0-9a-f]{12})  (computes again|DIFFERS|CANNOT CHECK|not computed here): `)

// verdicts are the words of check-bundle -pack for each claim, by the
// start of its id.
func verdicts(out string) map[string]string {
	v := map[string]string{}
	for _, m := range verdictLine.FindAllStringSubmatch(out, -1) {
		v[m[1]] = m[2]
	}
	return v
}

// forge appends a claim made from the true one, signed by the person, to
// a copy of the log. It returns the copy and the start of the new claim's
// id. alone deletes the true claim first: bean keeps one of two claims that
// state the same, under the lowest id.
func (pc packClaims) forge(t *testing.T, alone bool, change func(r, ctx map[string]any)) (string, string) {
	t.Helper()
	dir := copyLog(t, pc.dir)
	var r map[string]any
	data, _ := json.Marshal(pc.claim)
	json.Unmarshal(data, &r)
	ctx := r["context"].(map[string]any)
	in := map[string]any{"type": r["type"], "value": r["value"], "times": map[string]any{"observed": r["times"].(map[string]any)["observed"]}, "refs": r["refs"], "source": r["source"], "context": ctx}
	change(in, ctx)
	if alone {
		if _, errs, code := run(t, dir, "", "purge", pc.claim["id"].(string)); code != 0 {
			t.Fatalf("purge: %s", errs)
		}
	}
	before := map[string]bool{}
	for _, c := range lsClaims(t, dir) {
		before[c["id"].(string)] = true
	}
	line, _ := json.Marshal(in)
	if out, errs, code := run(t, dir, string(line)+"\n", "append"); code != 0 || !strings.Contains(out, "added 1,") {
		t.Fatalf("append: %d\n%s\n%s", code, out, errs)
	}
	for _, c := range lsClaims(t, dir) {
		if id := c["id"].(string); !before[id] {
			return dir, id[7:19]
		}
	}
	t.Fatal("the forged claim is not in the log")
	return "", ""
}

// TestCheckBundlePackHonest: each claim of a bundle computes again with
// its pack, and the check says what that does not show. A pack that bean
// has no record of counts only when the reader names it.
func TestCheckBundlePackHonest(t *testing.T) {
	pc := newPackClaims(t, "-accept-pack", mustDigest(t))
	bundle := bundleOf(t, pc.dir)
	out, errs, code := run(t, "", "", "check-bundle", "-pack", pc.pack, "-accept-pack", pc.digest, bundle)
	v := verdicts(out)
	if code != 0 || len(v) != 3 || slices.ContainsFunc(slices.Collect(maps.Values(v)), func(s string) bool { return s != "computes again" }) {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	for _, want := range []string{
		"bundle ok\nComputed again with the packs given",
		`against LBXTC (the pack says: "Total Cholesterol (mg/dL)"), cohort male-30-39 (the pack says: "male, age 30-39"), in pack ` + pc.digest[:12] + " (NOT on record)",
		"accepted with -accept-pack: the pack " + pc.digest,
		"It does not show that the reading is true, or that the pack is right.",
		"comparisons: 3 compute again, 0 differ, 0 cannot be checked",
		"\nevery comparison by a pack computes again\n",
		"After bundle ok, bean says which of them computes again with the packs given.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "not computed again here") || strings.Contains(strings.ToLower(out), "healthy") {
		t.Errorf("words that do not hold:\n%s", out)
	}
	// Without -pack the check prints what it printed before.
	plain, _, code := run(t, "", "", "check-bundle", bundle)
	if code != 0 || !strings.HasSuffix(plain, "bundle ok\n") || !strings.Contains(plain, "are not computed again here: tools/compare/reproduce.py does that.") {
		t.Errorf("no -pack: %d\n%s", code, plain)
	}
	// Without -accept-pack the numbers compute, and the check fails: the
	// pack is its builder's word.
	out, _, code = run(t, "", "", "check-bundle", "-pack", pc.pack, bundle)
	if code != 3 || len(verdicts(out)) != 3 || !strings.Contains(out, "NOT on record: the pack "+pc.digest+". bean has no record of it") ||
		!strings.Contains(out, "pass -accept-pack <sha256>") || strings.Contains(out, "-accept-pack "+pc.digest) || !strings.Contains(out, "computed again: NOT passed") {
		t.Errorf("no -accept-pack: %d\n%s", code, out)
	}
	// -accept-pack names another pack: nothing is accepted.
	other := strings.Repeat("a", 64)
	out, _, code = run(t, "", "", "check-bundle", "-pack", pc.pack, "-accept-pack", other, bundle)
	if code != 3 || !strings.Contains(out, "NOT on record: the pack "+pc.digest) || !strings.Contains(out, "-accept-pack "+other+" names no pack that bean has no record of") {
		t.Errorf("another -accept-pack: %d\n%s", code, out)
	}
}

// bodyPack is the test pack with the body measures too: it takes a weight.
func bodyPack(t *testing.T) (string, string) {
	t.Helper()
	return packFrom(t, "../../internal/nhanes/testdata", "TCHOL_J", "HDL_J", "DEMO_J", "BMX_J")
}

// mustDigest is the SHA-256 of bodyPack.
func mustDigest(t *testing.T) string {
	t.Helper()
	_, digest := bodyPack(t)
	return digest
}

// TestCheckBundlePackOnRecord: with a pack that bean has on record, no
// -accept-pack is needed, and one is refused as reproduce.py refuses it. A
// claim saved before its pack was put on record still computes again
// (DECISIONS 319).
func TestCheckBundlePackOnRecord(t *testing.T) {
	// Saved while bean had no record of the pack: the claims say so.
	before := newPackClaims(t, "-accept-pack", mustDigest(t))
	t.Setenv("BEAN_TEST_PACK_ON_RECORD", before.digest)
	now := newPackClaims(t)
	for name, pc := range map[string]packClaims{"before": before, "now": now} {
		bundle := bundleOf(t, pc.dir)
		out, errs, code := run(t, "", "", "check-bundle", "-pack", pc.pack, bundle)
		if v := verdicts(out); code != 0 || len(v) != 3 || !strings.Contains(out, "(on record): percentile") || strings.Contains(out, "NOT on record") {
			t.Errorf("%s: %d\n%s\n%s", name, code, out, errs)
		}
		out, _, code = run(t, "", "", "check-bundle", "-pack", pc.pack, "-accept-pack", pc.digest, bundle)
		if code != 3 || !strings.Contains(out, "-accept-pack "+pc.digest+" names no pack that bean has no record of") {
			t.Errorf("%s, -accept-pack: %d\n%s", name, code, out)
		}
	}
	// The caveat that the pack is not on record is allowed only in its
	// place: a claim with it anywhere else differs.
	dir, id := now.forge(t, true, func(r, ctx map[string]any) {
		ctx["caveats"] = append([]any{"bean has no record of this pack: nobody has checked its tables, its caveats, or what it sets a reading against"}, ctx["caveats"].([]any)...)
	})
	out, _, code := run(t, "", "", "check-bundle", "-pack", now.pack, bundleOf(t, dir))
	if code != 3 || verdicts(out)[id] != "DIFFERS" {
		t.Errorf("the caveat out of its place: %d\n%s", code, out)
	}
}

// TestCheckBundlePackForged: each kind of forged claim of tools/pack/run.sh
// is signed, and its numbers compute, or it changes one of them. The check
// refuses it, and only it.
func TestCheckBundlePackForged(t *testing.T) {
	pc := newPackClaims(t, "-accept-pack", mustDigest(t))
	for _, f := range []struct {
		name   string
		alone  bool
		change func(r, ctx map[string]any)
	}{
		{"a second record it is about", false, func(r, ctx map[string]any) {
			r["refs"] = append(r["refs"].([]any), map[string]any{"rel": "about", "id": pc.weight})
		}},
		{"a reading of a type that the measure does not take", false, func(r, ctx map[string]any) {
			ctx["reading"], r["refs"] = pc.weight, []any{map[string]any{"rel": "about", "id": pc.weight}}
		}},
		{"a reading of another type with the same value and unit", false, func(r, ctx map[string]any) {
			ctx["reading"], r["refs"] = pc.ldl, []any{map[string]any{"rel": "about", "id": pc.ldl}}
		}},
		{"another title", false, func(r, ctx map[string]any) { ref(ctx)["title"] = "NHANES 2017-2018 (reference range)" }},
		{"another source", false, func(r, ctx map[string]any) { ref(ctx)["source"] = "a clinical reference" }},
		{"another label", false, func(r, ctx map[string]any) { ref(ctx)["label"] = "LDL Cholesterol (mg/dL)" }},
		{"another unit", false, func(r, ctx map[string]any) { ref(ctx)["unit"] = "mmol/L" }},
		{"another cohort", false, func(r, ctx map[string]any) { ref(ctx)["cohort"] = "female-30-39" }},
		{"another cohort and its text", false, func(r, ctx map[string]any) {
			ref(ctx)["cohort"], ctx["cohort"] = "female-30-39", map[string]any{"text": "female, age 30-39"}
		}},
		{"another cohort text", false, func(r, ctx map[string]any) { ctx["cohort"] = map[string]any{"text": "male, age 20-29"} }},
		{"a measure the pack does not have", false, func(r, ctx map[string]any) { ref(ctx)["measure"] = "LBXLDL" }},
		{"a cohort the pack does not have", false, func(r, ctx map[string]any) { ref(ctx)["cohort"] = "everyone" }},
		{"a member more", false, func(r, ctx map[string]any) { ctx["note"] = "clinically validated" }},
		{"a member more in the result", false, func(r, ctx map[string]any) { res(ctx)["p90"] = 250.0 }},
		{"a caveat more", false, func(r, ctx map[string]any) { ctx["caveats"] = append(ctx["caveats"].([]any), "clinically validated") }},
		{"a caveat less", false, func(r, ctx map[string]any) { ctx["caveats"] = ctx["caveats"].([]any)[1:] }},
		{"the caveat of a pack not on record taken out", false, func(r, ctx map[string]any) {
			var keep []any
			for _, c := range ctx["caveats"].([]any) {
				if !strings.HasPrefix(c.(string), "bean has no record") {
					keep = append(keep, c)
				}
			}
			ctx["caveats"] = keep
		}},
		{"another n", false, func(r, ctx map[string]any) { res(ctx)["n"] = res(ctx)["n"].(float64) + 1 }},
		{"another quartile", false, func(r, ctx map[string]any) { res(ctx)["p75"] = res(ctx)["p50"] }},
		{"another percentile", false, func(r, ctx map[string]any) { res(ctx)["percentile"] = 99.9 }},
		{"another value", false, func(r, ctx map[string]any) { r["value"] = map[string]any{"n": 99.9, "unit": "{percentile}"} }},
		{"another unit of the value", false, func(r, ctx map[string]any) {
			r["value"] = map[string]any{"n": res(ctx)["percentile"], "unit": "%"}
		}},
		{"another method version", false, func(r, ctx map[string]any) {
			ctx["method"] = map[string]any{"id": "weighted-percentile", "version": "2"}
		}},
		{"another method", false, func(r, ctx map[string]any) { ctx["method"] = map[string]any{"id": "weighted-median", "version": "1"} }},
		{"a pack that is not a SHA-256", false, func(r, ctx map[string]any) { ref(ctx)["pack"] = "syn.pack" }},
		{"another source system", true, func(r, ctx map[string]any) {
			r["source"] = map[string]any{"system": "quest", "adapter": "bean@0.8.7", "ref": nil}
		}},
		{"a source ref", true, func(r, ctx map[string]any) {
			r["source"] = map[string]any{"system": "bean-compare", "adapter": "bean@0.8.7", "ref": "hmac-sha256:" + strings.Repeat("cd", 32)}
		}},
		{"a source with a member more", true, func(r, ctx map[string]any) {
			r["source"] = map[string]any{"system": "bean-compare", "adapter": "bean@0.8.7", "ref": nil, "x": "y"}
		}},
		{"a member more beside the context", true, func(r, ctx map[string]any) { r["x-note"] = "audited" }},
	} {
		dir, id := pc.forge(t, f.alone, f.change)
		out, errs, code := run(t, "", "", "check-bundle", "-pack", pc.pack, "-accept-pack", pc.digest, bundleOf(t, dir))
		v := verdicts(out)
		if code != 3 || v[id] != "DIFFERS" {
			t.Errorf("%s: %d, %s is %q\n%s\n%s", f.name, code, id, v[id], out, errs)
			continue
		}
		delete(v, id)
		for other, word := range v {
			if word != "computes again" {
				t.Errorf("%s: %s is %q too\n%s", f.name, other, word, out)
			}
		}
	}
}

func ref(ctx map[string]any) map[string]any { return ctx["reference"].(map[string]any) }
func res(ctx map[string]any) map[string]any { return ctx["result"].(map[string]any) }

// TestCheckBundlePackCannotCheck: a claim whose pack was not given, or
// whose reading is not in the bundle, is a check that cannot be made. The
// check says which pack to get, by its SHA-256.
func TestCheckBundlePackCannotCheck(t *testing.T) {
	pc := newPackClaims(t, "-accept-pack", mustDigest(t))
	bundle := bundleOf(t, pc.dir)
	// A pack with another SHA-256.
	otherPack, otherDigest, _ := editedPack(t, func(body map[string]any) { body["title"] = "another title" })
	out, _, code := run(t, "", "", "check-bundle", "-pack", otherPack, "-accept-pack", otherDigest, bundle)
	v := verdicts(out)
	if code != 3 || len(v) != 3 || !strings.Contains(out, "CANNOT CHECK: the pack with SHA-256 "+pc.digest+" was not given. Get that pack, and pass its file with -pack") ||
		!strings.Contains(out, "(SHA-256 "+otherDigest+") is named by no comparison here") || !strings.Contains(out, "-accept-pack "+otherDigest+" names no pack") ||
		!strings.Contains(out, "0 compute again, 0 differ, 3 cannot be checked") {
		t.Errorf("another pack: %d\n%s", code, out)
	}
	for id, word := range v {
		if word != "CANNOT CHECK" {
			t.Errorf("another pack: %s is %q", id, word)
		}
	}
	// A reading that is not in the bundle.
	missing := "sha256:" + strings.Repeat("ab", 32)
	dir, id := pc.forge(t, false, func(r, ctx map[string]any) {
		ctx["reading"], r["refs"] = missing, []any{map[string]any{"rel": "about", "id": missing}}
	})
	out, _, code = run(t, "", "", "check-bundle", "-pack", pc.pack, "-accept-pack", pc.digest, bundleOf(t, dir))
	if code != 3 || verdicts(out)[id] != "CANNOT CHECK" || !strings.Contains(out, "the reading abababababab is not in the bundle") {
		t.Errorf("a missing reading: %d\n%s", code, out)
	}
	// A file that is not a pack: the claims that name it differ.
	notPack := filepath.Join(t.TempDir(), "x.pack")
	os.WriteFile(notPack, []byte("pack"), 0o644)
	out, _, code = run(t, "", "", "check-bundle", "-pack", notPack, "-pack", pc.pack, "-accept-pack", pc.digest, bundle)
	if code != 3 || !strings.Contains(out, "REFUSED: the file "+notPack+" is not a pack that bean reads") || len(verdicts(out)) != 3 {
		t.Errorf("not a pack: %d\n%s", code, out)
	}
	// A bundle that fails: nothing is computed, and exit 1 keeps its meaning.
	os.WriteFile(filepath.Join(bundle, "objects", "zz.json"), []byte("{}"), 0o644)
	out, _, code = run(t, "", "", "check-bundle", "-pack", pc.pack, bundle)
	if code != 1 || len(verdicts(out)) != 0 || !strings.HasSuffix(out, "-pack: nothing was computed again, since the bundle did not pass its check\n") {
		t.Errorf("a bundle that fails: %d\n%s", code, out)
	}
}

// TestCheckBundlePackUsage: an error of use exits 2 and computes nothing.
func TestCheckBundlePackUsage(t *testing.T) {
	path, digest := testPack(t)
	b := t.TempDir()
	for name, a := range map[string][]string{
		"upper case":         {"-pack", path, "-accept-pack", strings.ToUpper(digest)},
		"a prefix":           {"-pack", path, "-accept-pack", "sha256:" + digest},
		"empty":              {"-pack", path, "-accept-pack="},
		"no pack":            {"-accept-pack", digest},
		"a pack that is not": {"-pack", filepath.Join(b, "none.pack")},
		"an empty -pack":     {"-pack="},
	} {
		out, errs, code := run(t, "", "", append(append([]string{"check-bundle"}, a...), b)...)
		if code != 2 || out != "" || errs == "" {
			t.Errorf("%s: %d\n%s\n%s", name, code, out, errs)
		}
	}
}

// TestCheckBundlePackSavedByARelease: the bundle that release 0.7.0 wrote.
// Its claim with the pack of the synthetic files computes again. Those with
// the packs on record cannot be checked without the files of those packs,
// and the comparison with the survey's files is not computed here.
func TestCheckBundlePackSavedByARelease(t *testing.T) {
	path, digest := bodyPack(t)
	out, errs, code := run(t, "", "", "check-bundle", "-pack", path, "-accept-pack", digest, "testdata/saved/0.7.0/bundle")
	want := map[string]string{
		"8bbf070bbcef": "computes again",
		"7367ad250318": "CANNOT CHECK", "ddb1945a136c": "CANNOT CHECK", "e91d29b6b523": "CANNOT CHECK",
		"86204a68af3a": "not computed here",
	}
	if v := verdicts(out); code != 3 || !maps.Equal(v, want) {
		t.Fatalf("%d: %v\n%s\n%s", code, v, out, errs)
	}
	for _, line := range []string{
		"CANNOT CHECK: the pack with SHA-256 10555d6769421d4305cbcd4ff14960d8b966ddd829a50581a9584a60a84d29e2 was not given",
		"CANNOT CHECK: the pack with SHA-256 5cae07db64295d565ffae7c251c17fdb9f043dc0638868557275f8b0b0311efa was not given",
		"comparisons: 1 compute again, 0 differ, 3 cannot be checked, 1 with the survey's files not computed here",
	} {
		if !strings.Contains(out, line) {
			t.Errorf("no %q in\n%s", line, out)
		}
	}
}

// TestCheckBundlePackEdges: a claim that is signed by a key other than the
// person's, a reading of zero in a table that takes none, a bundle with no
// comparison, and a bundle whose only comparison is with the survey's files.
func TestCheckBundlePackEdges(t *testing.T) {
	pc := newPackClaims(t, "-accept-pack", mustDigest(t))
	// A reading of zero, which the method does not place in this cell.
	out, errs, code := run(t, pc.dir, "", "add", "lab.cholesterol-total", "0", "-unit", "mg/dL", "-at", "2026-11-01")
	if code != 0 {
		t.Fatalf("add 0: %s", errs)
	}
	zero := strings.TrimSpace(out)
	dir, id := pc.forge(t, false, func(r, ctx map[string]any) {
		ctx["reading"], r["refs"] = zero, []any{map[string]any{"rel": "about", "id": zero}}
	})
	out, _, code = run(t, "", "", "check-bundle", "-pack", pc.pack, "-accept-pack", pc.digest, bundleOf(t, dir))
	if code != 3 || verdicts(out)[id] != "DIFFERS" || !strings.Contains(out, "the reading is not a number above zero") {
		t.Errorf("a reading of zero: %d\n%s", code, out)
	}
	// A claim that an agent signed about the person's reading.
	dir = copyLog(t, pc.dir)
	if _, errs, code := run(t, dir, "", "agent", "init", "-label", "assistant"); code != 0 {
		t.Fatalf("agent init: %s", errs)
	}
	if _, errs, code := run(t, dir, "", "purge", pc.claim["id"].(string)); code != 0 {
		t.Fatalf("purge: %s", errs)
	}
	ctx := pc.claim["context"]
	line, _ := json.Marshal(map[string]any{"type": pc.claim["type"], "value": pc.claim["value"], "times": map[string]any{"observed": pc.claim["times"].(map[string]any)["observed"]}, "refs": pc.claim["refs"], "source": pc.claim["source"], "context": ctx})
	if out, errs, code := run(t, dir, string(line)+"\n", "append", "-agent"); code != 0 || !strings.Contains(out, "added 1,") {
		t.Fatalf("append -agent: %d\n%s\n%s", code, out, errs)
	}
	shared := filepath.Join(t.TempDir(), "agent")
	run(t, dir, "", "share", "-yes", "-out", shared, "-type", "comparison.*")
	out, _, code = run(t, "", "", "check-bundle", "-pack", pc.pack, "-accept-pack", pc.digest, shared)
	if code != 3 || strings.Count(out, "  DIFFERS: ") != 1 || !strings.Contains(out, "DIFFERS: the claim is not signed by the person it is about") {
		t.Errorf("a claim of an agent: %d\n%s", code, out)
	}
	// A bundle with no comparison: nothing computes, and -pack says so.
	plain := t.TempDir()
	run(t, plain, "", "init")
	run(t, plain, "", "add", "weight", "70", "-unit", "kg")
	noClaims := filepath.Join(t.TempDir(), "none")
	run(t, plain, "", "share", "-yes", "-out", noClaims, "-type", "weight")
	out, _, code = run(t, "", "", "check-bundle", "-pack", pc.pack, noClaims)
	if code != 3 || !strings.Contains(out, "the bundle holds no comparison: there is nothing to compute again") || !strings.Contains(out, "named by no comparison here") {
		t.Errorf("no comparison: %d\n%s", code, out)
	}
}

// TestCheckBundlePackOrder: a check is made in the order that reproduce.py
// makes it, so that a claim that differs is not called one that cannot be
// checked. Each case here differs, and would be CANNOT CHECK or pass if a
// check were left out.
func TestCheckBundlePackOrder(t *testing.T) {
	pc := newPackClaims(t, "-accept-pack", mustDigest(t))
	missing := "sha256:" + strings.Repeat("ab", 32)
	out, errs, code := run(t, pc.dir, "", "add", "lab.cholesterol-total", "--", "-5", "-unit", "mg/dL", "-at", "2026-11-01")
	if code != 0 {
		t.Fatalf("add -5: %s", errs)
	}
	negative := strings.TrimSpace(out)
	// A file that is not a pack, named by a claim.
	notPack := filepath.Join(t.TempDir(), "x.pack")
	os.WriteFile(notPack, []byte("pack"), 0o644)
	// A pack with no table of the cholesterol for one cohort.
	gap, gapDigest, _ := editedPack(t, func(body map[string]any) {
		var keep []any
		for _, c := range body["cells"].([]any) {
			if cell := c.(map[string]any); cell["measure"] != "LBXTC" || cell["cohort"] != "male-30-39" {
				keep = append(keep, c)
			}
		}
		body["cells"] = keep
	})
	other, _ := testPack(t)
	for _, f := range []struct {
		name, why string
		packs     []string
		alone     bool
		change    func(r, ctx map[string]any)
	}{
		{"no context", "the claim names no method", nil, true, func(r, ctx map[string]any) { delete(r, "context") }},
		{"another method, and no reading", "a method this bean does not compute again", nil, false, func(r, ctx map[string]any) {
			ctx["method"], ctx["reading"], r["refs"] = map[string]any{"id": "weighted-median", "version": "1"}, missing, []any{map[string]any{"rel": "about", "id": missing}}
		}},
		{"a reading below zero, and no pack", "the reading is not a number of zero or more", []string{other}, false, func(r, ctx map[string]any) {
			ctx["reading"], r["refs"] = negative, []any{map[string]any{"rel": "about", "id": negative}}
		}},
		{"times with a member more", "the claim's times are not the two that a record has", nil, true, func(r, ctx map[string]any) {
			r["times"].(map[string]any)["x"] = 1.0
		}},
		{"a pack that is not one", "is not a pack that bean reads", []string{notPack}, false, func(r, ctx map[string]any) {
			ref(ctx)["pack"] = fileDigest(notPack)
		}},
		{"a cell that the pack does not have", "the pack has no cell for measure LBXTC and cohort male-30-39", []string{gap}, false, func(r, ctx map[string]any) {
			ref(ctx)["pack"] = gapDigest
		}},
	} {
		dir, id := pc.forge(t, f.alone, f.change)
		args := []string{"check-bundle", "-pack", pc.pack, "-accept-pack", pc.digest}
		for _, p := range f.packs {
			args = append(args, "-pack", p)
		}
		out, errs, code := run(t, "", "", append(args, bundleOf(t, dir))...)
		if code != 3 || verdicts(out)[id] != "DIFFERS" || !strings.Contains(out, f.why) {
			t.Errorf("%s: %d, %s is %q\n%s\n%s", f.name, code, id, verdicts(out)[id], out, errs)
		}
	}
}
