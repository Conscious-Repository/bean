package core

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

// testLog is an in-memory log for verifier tests.
type testLog struct {
	files map[string][]byte
	n     int
}

type testIdentity struct {
	seed    []byte
	signer  Signer
	id      string
	headKey string
}

func newTestLog() *testLog { return &testLog{files: map[string][]byte{}} }

func (l *testLog) put(obj map[string]any, s Signer) string {
	id, b, err := SignRecord(obj, s)
	if err != nil {
		panic(err)
	}
	l.files[PathOf(id)] = b
	return id
}

func (l *testLog) nonce() string {
	l.n++
	return fmt.Sprintf("%032x", l.n)
}

func (l *testLog) newIdentity(seedByte byte) *testIdentity {
	seed := bytes.Repeat([]byte{seedByte}, 32)
	sk := bytes.Repeat([]byte{seedByte + 0x40}, 32)
	s := NewEd25519Signer(sk)
	g := NewGenesis(s.Public(), Commitment(RecoverySigner(seed, 0).Public()), "test@0", "2026-10-27T00:00:00Z")
	g["nonce"] = l.nonce()
	id := l.put(g, s)
	return &testIdentity{seed: seed, signer: s, id: id, headKey: s.Public()}
}

func (l *testLog) claim(i *testIdentity, typ string, n float64, ref string) string {
	c := map[string]any{
		"v": 0.0, "nonce": l.nonce(), "type": typ, "subject": i.id, "author": i.id, "kid": i.signer.Public(),
		"value":  map[string]any{"n": n, "unit": "ms"},
		"times":  map[string]any{"observed": nil, "imported": "2026-10-27T00:00:01Z"},
		"source": map[string]any{"system": "test", "adapter": "test@0", "ref": nil},
	}
	if ref != "" {
		c["source"].(map[string]any)["ref"] = ref
	}
	return l.put(c, i.signer)
}

func (l *testLog) supersede(i *testIdentity, target string) string {
	c := map[string]any{
		"v": 0.0, "nonce": l.nonce(), "type": "hrv.rmssd", "subject": i.id, "author": i.id, "kid": i.signer.Public(),
		"value":  map[string]any{"n": 1.0, "unit": "ms"},
		"refs":   []any{map[string]any{"rel": "supersedes", "id": target}},
		"times":  map[string]any{"observed": nil, "imported": "2026-10-27T00:00:02Z"},
		"source": map[string]any{"system": "test", "adapter": "test@0", "ref": nil},
	}
	return l.put(c, i.signer)
}

func (l *testLog) tombstone(i *testIdentity, target string) string {
	t := NewTombstone(i.id, i.signer.Public(), target, "test", "test@0", "2026-10-27T00:00:03Z")
	t["nonce"] = l.nonce()
	return l.put(t, i.signer)
}

// rotate writes a recovery-signed succession from parent to a new key.
func (l *testLog) rotate(i *testIdentity, parent string, gen int, sealed []string) (string, Signer) {
	nk := NewEd25519Signer(bytes.Repeat([]byte{byte(0x80 + gen)}, 32))
	rec := RecoverySigner(i.seed, gen)
	s := NewSuccession(i.id, rec.Public(), parent, nk.Public(), Commitment(RecoverySigner(i.seed, gen+1).Public()), sealed, "test@0", "2026-10-27T00:00:04Z")
	s["nonce"] = l.nonce()
	return l.put(s, rec), nk
}

func (l *testLog) verify() *Report { return VerifyFiles(l.files) }

func wantError(t *testing.T, rp *Report, id, want string) {
	t.Helper()
	if got := rp.Errors[id]; got != want {
		t.Errorf("%s: got error %q want %q", id[:16], got, want)
	}
}

func wantWarning(t *testing.T, rp *Report, id, prefix string) {
	t.Helper()
	for _, w := range rp.Warnings[id] {
		if strings.HasPrefix(w, prefix) {
			return
		}
	}
	t.Errorf("%s: no warning %q (have %v)", id[:16], prefix, rp.Warnings[id])
}

func wantClean(t *testing.T, rp *Report) {
	t.Helper()
	if len(rp.Errors) != 0 {
		t.Errorf("unexpected errors: %v", rp.Errors)
	}
}

// Conformance test 1: record byte flip.
func TestByteFlip(t *testing.T) {
	l := newTestLog()
	i := l.newIdentity(1)
	for k := 0; k < 3; k++ {
		l.claim(i, "hrv.rmssd", 40+float64(k), "")
	}
	rp := l.verify()
	wantClean(t, rp)
	if len(rp.Valid) != 4 {
		t.Fatalf("valid %d", len(rp.Valid))
	}
	for p, b := range l.files {
		if !bytes.Contains(b, []byte(`"n":41`)) {
			continue
		}
		l.files[p] = bytes.Replace(b, []byte(`"n":41`), []byte(`"n":49`), 1)
		rp = l.verify()
		if e := rp.Errors[p]; e != "id mismatch" {
			t.Errorf("flipped byte: %q", e)
		}
		if rp.ExitCode(nil) != 1 {
			t.Error("exit code")
		}
	}
}

// Conformance tests 7 and 8: supersede by author and by non-author.
func TestSupersede(t *testing.T) {
	l := newTestLog()
	i, j := l.newIdentity(1), l.newIdentity(2)
	target := l.claim(i, "hrv.rmssd", 40, "")
	d := l.supersede(i, target)
	rp := l.verify()
	wantClean(t, rp)
	if got := rp.Superseded[target]; len(got) != 1 || got[0] != d {
		t.Errorf("superseded %v", got)
	}
	bad := l.supersede(j, target)
	rp = l.verify()
	wantError(t, rp, bad, "not author")
	if !rp.Valid[target] || !rp.Valid[d] {
		t.Error("target or valid supersede lost")
	}
	// Sibling rule (C12): second valid supersede gets a warning, lowest id is primary.
	d2 := l.supersede(i, target)
	rp = l.verify()
	lo, hi := d, d2
	if hi < lo {
		lo, hi = hi, lo
	}
	wantWarning(t, rp, hi, "superseded by sibling: "+lo)
	// Held supersede: target not in the log.
	held := l.supersede(i, "sha256:"+strings.Repeat("ab", 32))
	rp = l.verify()
	wantWarning(t, rp, held, "held: ")
	if !rp.Valid[held] {
		t.Error("held supersede must stay valid")
	}
}

// Conformance test 14 and 6.7: tombstone targets and subject rule.
func TestTombstone(t *testing.T) {
	l := newTestLog()
	i, j := l.newIdentity(1), l.newIdentity(2)
	c := l.claim(i, "hrv.rmssd", 40, "")
	tomb := l.tombstone(i, c)
	rp := l.verify()
	wantClean(t, rp)
	if rp.Purged[c] != tomb || rp.Valid[c] {
		t.Error("target not purged")
	}
	succ, nk := l.rotate(i, i.id, 0, nil)
	i.signer = nk
	for _, target := range []string{i.id, succ, tomb} {
		x := l.tombstone(i, target)
		rp = l.verify()
		wantError(t, rp, x, "bad target")
	}
	// Stranger tombstone: J names I's claim.
	c2 := l.claim(i, "hrv.rmssd", 41, "")
	x := l.tombstone(j, c2)
	rp = l.verify()
	wantError(t, rp, x, "not subject")
	if !rp.Valid[c2] {
		t.Error("stranger tombstone purged a record")
	}
	// Held tombstone.
	h := l.tombstone(i, "sha256:"+strings.Repeat("cd", 32))
	rp = l.verify()
	wantWarning(t, rp, h, "held: ")
	if !rp.Valid[h] || !rp.Held[h] {
		t.Error("held tombstone")
	}
}

// Conformance tests 23, 24 and 31: unknown versions.
func TestUnknownVersion(t *testing.T) {
	l := newTestLog()
	i, j := l.newIdentity(1), l.newIdentity(2)
	l.claim(i, "hrv.rmssd", 40, "")
	jc := l.claim(j, "hrv.rmssd", 50, "")
	// 23: ordinary type, v 99.
	c := map[string]any{"v": 99.0, "nonce": l.nonce(), "type": "hrv.rmssd", "subject": i.id, "author": i.id, "kid": i.signer.Public(), "future": []any{1.0}}
	l.put(c, i.signer)
	rp := l.verify()
	wantClean(t, rp)
	if rp.Skipped != 1 || len(rp.TooNew) != 0 {
		t.Errorf("skipped %d toonew %v", rp.Skipped, rp.TooNew)
	}
	// 31: core type, v 99, signed by a key that is not I's.
	s := map[string]any{"v": 99.0, "nonce": l.nonce(), "type": "hlog.succession", "subject": i.id, "author": i.id, "kid": j.signer.Public()}
	l.put(s, j.signer)
	rp = l.verify()
	if len(rp.TooNew) != 0 {
		t.Errorf("stranger-signed future succession made I too new")
	}
	// 24: core type, v 99, signed by I's chain key.
	s2 := map[string]any{"v": 99.0, "nonce": l.nonce(), "type": "hlog.succession", "subject": i.id, "author": i.id, "kid": i.signer.Public()}
	l.put(s2, i.signer)
	// A broken file of J must still be reported.
	l.files["objects/00/"+strings.Repeat("0", 62)+".json"] = []byte("{")
	rp = l.verify()
	if len(rp.TooNew) != 1 || rp.TooNew[0] != i.id {
		t.Fatalf("too new %v", rp.TooNew)
	}
	if !rp.Valid[jc] || !rp.Valid[j.id] {
		t.Error("J not checked")
	}
	if rp.Valid[i.id] {
		t.Error("I evaluated while too new")
	}
	if rp.Errors["objects/00/"+strings.Repeat("0", 62)+".json"] != "bad json" {
		t.Error("J file error lost")
	}
	if rp.ExitCode(nil) != 3 {
		t.Errorf("exit %d", rp.ExitCode(nil))
	}
	lines := rp.Lines()
	if !strings.HasPrefix(lines[len(lines)-1], "summary: ") || !contains(lines[:len(lines)-1], "too new: "+i.id) {
		t.Errorf("lines %v", lines)
	}
}

// Seed-only rotation, seals, and freeze (change set C5 to C11, 5.4).
func TestChain(t *testing.T) {
	l := newTestLog()
	i := l.newIdentity(1)
	c1 := l.claim(i, "hrv.rmssd", 40, "")
	oldTomb := l.tombstone(i, c1)         // destructive by the key about to rotate out
	c0 := l.claim(i, "hrv.rmssd", 39, "") // target of an unsealed tombstone
	unsealed := l.tombstone(i, c0)
	succ, nk := l.rotate(i, i.id, 0, []string{oldTomb})
	oldKey := i.signer
	i.signer = nk
	c2 := l.claim(i, "hrv.rmssd", 41, "")
	rp := l.verify()
	if len(rp.Chains[i.id]) != 2 || rp.Chains[i.id][1].ID != succ {
		t.Fatalf("chain %v", rp.Chains[i.id])
	}
	if !rp.Valid[c2] || !rp.Valid[oldTomb] || rp.Valid[c1] {
		t.Error("sealed tombstone or new claim wrong")
	}
	wantError(t, rp, unsealed, "not sealed")
	if !rp.Valid[c0] {
		t.Error("unsealed tombstone purged its target")
	}
	// A claim by the old key is still valid (6.5).
	i.signer = oldKey
	old := l.claim(i, "hrv.rmssd", 42, "")
	i.signer = nk
	rp = l.verify()
	if !rp.Valid[old] {
		t.Error("old chain key claim invalid")
	}
	// A signing key cannot write a succession (C5).
	bad := NewSuccession(i.id, nk.Public(), succ, NewEd25519Signer(bytes.Repeat([]byte{9}, 32)).Public(), Commitment("ed25519:"+strings.Repeat("00", 32)), nil, "test@0", "2026-10-27T00:00:05Z")
	bad["nonce"] = l.nonce()
	badID := l.put(bad, nk)
	rp = l.verify()
	wantError(t, rp, badID, "bad succession")
	// Recovery key must not sign claims (5.8).
	rec := RecoverySigner(i.seed, 1)
	rc := map[string]any{"v": 0.0, "nonce": l.nonce(), "type": "hrv.rmssd", "subject": i.id, "author": i.id, "kid": rec.Public(),
		"value": map[string]any{"n": 1.0}, "times": map[string]any{"observed": nil, "imported": "2026-10-27T00:00:06Z"},
		"source": map[string]any{"system": "test", "adapter": "test@0", "ref": nil}}
	rcID := l.put(rc, rec)
	rp = l.verify()
	wantError(t, rp, rcID, "key not in chain")
	// Freeze: a second recovery succession off the same parent.
	l.rotate(i, i.id, 0, nil)
	rp = l.verify()
	if len(rp.Compromised) != 1 || rp.Compromised[0] != i.id {
		t.Fatalf("compromised %v", rp.Compromised)
	}
	wantError(t, rp, succ, "frozen")
	wantError(t, rp, oldTomb, "frozen")
	wantError(t, rp, c2, "frozen")
	if !rp.Valid[c0] || !rp.Valid[old] {
		t.Error("trunk claims must survive a freeze")
	}
	if rp.ExitCode([]string{i.id}) != 4 {
		t.Error("exit 4")
	}
}

func TestBareKeyAndDuplicates(t *testing.T) {
	l := newTestLog()
	i := l.newIdentity(1)
	dev := NewEd25519Signer(bytes.Repeat([]byte{7}, 32))
	c := map[string]any{"v": 0.0, "nonce": l.nonce(), "type": "hr.bpm", "subject": i.id, "author": dev.Public(), "kid": dev.Public(),
		"value": map[string]any{"n": 60.0}, "times": map[string]any{"observed": nil, "imported": "2026-10-27T00:00:06Z"},
		"source": map[string]any{"system": "device", "adapter": "dev@1", "ref": nil}}
	devID := l.put(c, dev)
	ref := DedupeRefNative(make([]byte, 32), "test", "1", 0)
	a := l.claim(i, "hrv.rmssd", 40, ref)
	b := l.claim(i, "hrv.rmssd", 40, ref)
	rp := l.verify()
	wantClean(t, rp)
	if !rp.Valid[devID] {
		t.Error("bare key claim")
	}
	lo, hi := a, b
	if hi < lo {
		lo, hi = hi, lo
	}
	if rp.Duplicates[hi] != lo {
		t.Errorf("duplicates %v", rp.Duplicates)
	}
	// Unresolved author and subject.
	ghost := "sha256:" + strings.Repeat("ef", 32)
	c2 := map[string]any{"v": 0.0, "nonce": l.nonce(), "type": "hr.bpm", "subject": ghost, "author": i.id, "kid": i.signer.Public(),
		"value": map[string]any{"n": 60.0}, "times": map[string]any{"observed": nil, "imported": "2026-10-27T00:00:06Z"},
		"source": map[string]any{"system": "device", "adapter": "dev@1", "ref": nil}}
	c2ID := l.put(c2, i.signer)
	rp = l.verify()
	wantWarning(t, rp, c2ID, "unresolved subject: "+ghost)
	if !rp.Valid[c2ID] {
		t.Error("unresolved subject must stay valid")
	}
}

// V17: one file per error string of 3.8.
func TestBadFiles(t *testing.T) {
	l := newTestLog()
	i := l.newIdentity(1)
	good := l.claim(i, "hrv.rmssd", 40, "")
	gb := l.files[PathOf(good)]
	mk := func(mutate func(m map[string]any), resign bool) (string, []byte) {
		v, _ := Parse(gb)
		m := v.(map[string]any)
		mutate(m)
		if resign {
			id, b, _ := SignRecord(m, i.signer)
			return PathOf(id), b
		}
		return PathOf(m["id"].(string)), Canonical(m)
	}
	cases := []struct {
		name string
		path string
		data []byte
		want string
	}{
		{"bad path", "objects/zz/" + strings.Repeat("0", 62) + ".json", gb, "bad path"},
		{"too large", PathOf(good), append(gb, bytes.Repeat([]byte(" "), MaxRecordSize)...), "too large"},
		{"bad json", PathOf(good), []byte("{"), "bad json"},
		{"duplicate key", PathOf(good), []byte(`{"v":0,"v":0}`), "duplicate key"},
		{"bad number", PathOf(good), []byte(`{"v":9007199254740992}`), "bad number"},
		{"bad string", PathOf(good), []byte(`{"v":"\udc00"}`), "bad string"},
		{"bad v", PathOf(good), []byte(`{"v":"0"}`), "bad v"},
		{"bad type", PathOf(good), []byte(`{"v":0}`), "bad type"},
		{"id mismatch", PathOf(good), bytes.Replace(gb, []byte(`"n":40`), []byte(`"n":41`), 1), "id mismatch"},
	}
	p, b := mk(func(m map[string]any) { m["kid"] = "secp256k1:00"; m["author"] = m["kid"] }, true)
	cases = append(cases, struct {
		name, path string
		data       []byte
		want       string
	}{"unsupported key algorithm", p, b, "unsupported key algorithm"})
	p, b = mk(func(m map[string]any) { m["kid"] = "ed25519:00" }, true)
	cases = append(cases, struct {
		name, path string
		data       []byte
		want       string
	}{"bad key", p, b, "bad key"})
	p, b = mk(func(m map[string]any) { m["sig"] = "ed25519:" + strings.Repeat("00", 64) }, false)
	cases = append(cases, struct {
		name, path string
		data       []byte
		want       string
	}{"bad signature", p, b, "bad signature"})
	// not canonical: same bytes with whitespace.
	cases = append(cases, struct {
		name, path string
		data       []byte
		want       string
	}{"not canonical", PathOf(good), bytes.Replace(gb, []byte(`"n":40`), []byte(`"n": 40`), 1), "not canonical"})
	fieldCases := []struct {
		name   string
		mutate func(m map[string]any)
		want   string
	}{
		{"bad field: nonce", func(m map[string]any) { m["nonce"] = "xyz" }, "bad field: nonce"},
		{"bad field: type", func(m map[string]any) { m["type"] = "Bad.Type" }, "bad field: type"},
		{"bad field: subject", func(m map[string]any) { delete(m, "subject") }, "bad field: subject"},
		{"bad field: times.imported", func(m map[string]any) { m["times"].(map[string]any)["imported"] = "2026-10-27T00:00:01+00:00" }, "bad field: times.imported"},
		{"bad field: times.observed", func(m map[string]any) { delete(m["times"].(map[string]any), "observed") }, "bad field: times.observed"},
		{"bad field: times.observed.end", func(m map[string]any) {
			m["times"].(map[string]any)["observed"] = map[string]any{"start": "2026-01-02T00:00:00Z", "end": "2026-01-01T00:00:00Z", "precision": "day"}
		}, "bad field: times.observed.end"},
		{"bad field: source.adapter", func(m map[string]any) { m["source"].(map[string]any)["adapter"] = "noversion" }, "bad field: source.adapter"},
		{"bad field: source.ref", func(m map[string]any) { m["source"].(map[string]any)["ref"] = "abc" }, "bad field: source.ref"},
		{"unknown rel", func(m map[string]any) { m["refs"] = []any{map[string]any{"rel": "attests", "id": good}} }, "unknown rel"},
		{"bad field: refs", func(m map[string]any) { m["refs"] = []any{map[string]any{"rel": "purges", "id": good}} }, "bad field: refs"},
		{"unknown core type", func(m map[string]any) { m["type"] = "hlog.future" }, "unknown core type"},
		{"blob not listed", func(m map[string]any) { m["value"] = map[string]any{"blob": good} }, "blob not listed"},
	}
	for _, fc := range fieldCases {
		p, b := mk(fc.mutate, true)
		cases = append(cases, struct {
			name, path string
			data       []byte
			want       string
		}{fc.name, p, b, fc.want})
	}
	for _, c := range cases {
		fr := CheckFile(c.path, c.data)
		got := ""
		if fr.Err != nil {
			got = fr.Err.Error()
		}
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	_ = hex.EncodeToString
}

// Conformance test 30: order independence. Go map iteration is random, so
// ten runs over the same set exercise different orders.
func TestOrderIndependence(t *testing.T) {
	l := newTestLog()
	i := l.newIdentity(1)
	c := l.claim(i, "hrv.rmssd", 40, "")
	l.tombstone(i, c)
	succ, nk := l.rotate(i, i.id, 0, nil)
	i.signer = nk
	l.claim(i, "hrv.rmssd", 41, "")
	l.rotate(i, succ, 1, nil)
	l.rotate(i, succ, 1, nil) // freeze
	first := strings.Join(l.verify().Lines(), "\n")
	for k := 0; k < 10; k++ {
		if got := strings.Join(l.verify().Lines(), "\n"); got != first {
			t.Fatalf("run %d differs:\n%s\n---\n%s", k, got, first)
		}
	}
	if !strings.Contains(first, "compromised: "+i.id) {
		t.Error(first)
	}
}
