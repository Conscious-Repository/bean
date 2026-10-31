package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// Seal continuations (C28).

func withChunk(t *testing.T, n int) {
	t.Helper()
	old := sealChunk
	sealChunk = n
	t.Cleanup(func() { sealChunk = old })
}

// sealLog makes a log with n claims and rotates it with a chunk of 2, so
// the seal set spreads over a succession and several continuations.
func sealLog(t *testing.T, n int) (s *Store, id string, words []string, claims []string, res RotateResult) {
	t.Helper()
	withChunk(t, 2)
	s, id, words = initWithWords(t)
	var lines []string
	for i := 0; i < n; i++ {
		lines = append(lines, partial("hr", float64(i), nil))
	}
	claims = appendLines(t, s, lines...).IDs
	res, err := s.Rotate(words, nil, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	return
}

func recoveryOf(t *testing.T, words []string, g int) Signer {
	t.Helper()
	seed, err := WordsToSeed(words)
	if err != nil {
		t.Fatal(err)
	}
	return RecoverySigner(seed, g)
}

func TestContinuationSeals(t *testing.T) {
	s, id, _, claims, res := sealLog(t, 7)
	if len(res.Parts) != 3 || len(res.Sealed) != 7 {
		t.Fatalf("parts %d sealed %d", len(res.Parts), len(res.Sealed))
	}
	l := verifyOK(t, s)
	succ := l.Report.Records[res.Succession]
	if succ.Parts() != 3 || len(succ.Sealed()) != 2 {
		t.Errorf("succession parts %d sealed %d", succ.Parts(), len(succ.Sealed()))
	}
	for k, pid := range res.Parts {
		p := l.Report.Records[pid]
		if p.Part() != k+1 || p.Kid != succ.Kid || p.Subject != id || !l.Report.Valid[pid] {
			t.Errorf("part %d: %+v valid=%v", k+1, p.Value, l.Report.Valid[pid])
		}
	}
	for _, c := range claims {
		if !l.Current(c) {
			t.Errorf("%s: %q", c[:16], l.Report.Errors[c])
		}
	}
	if _, err := os.Stat(s.path(fileRotationPending)); !os.IsNotExist(err) {
		t.Error("pending note left")
	}
}

// No continuations below the limit: the succession has no value.parts and
// is byte-compatible with builds before C28.
func TestNoPartsBelowLimit(t *testing.T) {
	s, _, words := initWithWords(t)
	appendLines(t, s, partial("hr", 1, nil))
	res, err := s.Rotate(words, nil, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	l := verifyOK(t, s)
	if len(res.Parts) != 0 {
		t.Error("parts below the limit")
	}
	if _, has := l.Report.Records[res.Succession].Value["parts"]; has {
		t.Error("value.parts below the limit")
	}
}

func TestMissingPartSealsNothing(t *testing.T) {
	s, _, _, claims, res := sealLog(t, 7)
	p := s.path(PathOf(res.Parts[1]))
	data, _ := os.ReadFile(p)
	os.Remove(p)
	l, _ := s.Load()
	if !contains(l.Report.Incomplete, res.Succession) {
		t.Fatalf("incomplete %v", l.Report.Incomplete)
	}
	wantWarning(t, l.Report, res.Succession, "incomplete seals: ")
	if _, code, _ := s.VerifyLines(); code != 1 {
		t.Errorf("verify exit %d with an incomplete seal set", code)
	}
	for _, c := range claims {
		if l.Report.Errors[c] != "not sealed" {
			t.Errorf("%s: %q, want not sealed", c[:16], l.Report.Errors[c])
		}
	}
	s.Now = func() time.Time { return testClock.Add(30 * 24 * time.Hour) }
	if _, _, err := s.GC(); err == nil || !strings.HasPrefix(err.Error(), "incomplete seals: ") {
		t.Errorf("gc with an incomplete seal set: %v", err)
	}
	// The part comes back; everything is valid again. Nothing was erased.
	putFile(t, s, PathOf(res.Parts[1]), data)
	l = verifyOK(t, s)
	for _, c := range claims {
		if !l.Current(c) {
			t.Errorf("%s after the part came back: %q", c[:16], l.Report.Errors[c])
		}
	}
}

func putSeal(t *testing.T, s *Store, m map[string]any, sg Signer) string {
	t.Helper()
	id, b, err := SignRecord(m, sg)
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, s, PathOf(id), b)
	return id
}

func TestDuplicatePartSealsNothing(t *testing.T) {
	s, id, words, claims, res := sealLog(t, 7)
	rec := recoveryOf(t, words, 0)
	dup := putSeal(t, s, NewSeal(id, rec.Public(), res.Succession, 1, []string{claims[0]}, "x@1", "2026-10-30T00:00:00Z"), rec)
	l, _ := s.Load()
	if l.Report.Errors[dup] != "bad seal" || l.Report.Errors[res.Parts[0]] != "bad seal" || !contains(l.Report.Incomplete, res.Succession) {
		t.Errorf("dup %q part1 %q incomplete %v", l.Report.Errors[dup], l.Report.Errors[res.Parts[0]], l.Report.Incomplete)
	}
	if l.Report.Errors[claims[3]] != "not sealed" {
		t.Errorf("claim with a doubled part: %q", l.Report.Errors[claims[3]])
	}
}

func TestForgedParts(t *testing.T) {
	s, id, words, claims, res := sealLog(t, 7)
	oldKey := s.mustLocal(t) // already the new key; build the old one from a clone
	_ = oldKey
	l0, _ := s.Load()
	succ := l0.Report.Records[res.Succession]
	other, oid, owords := initWithWords(t)
	_ = other
	orec := recoveryOf(t, owords, 0)
	rec1 := recoveryOf(t, words, 1) // the next recovery key, not the succession's
	cases := map[string]map[string]any{}
	// Signed by a chain signing key, not the recovery key.
	signer := NewEd25519Signer([]byte(strings.Repeat("k", 32)))
	cases["signing key"] = NewSeal(id, signer.Public(), res.Succession, 4, []string{claims[0]}, "x@1", "2026-10-30T00:00:00Z")
	// Another identity's recovery key, about another subject.
	cases["other identity"] = NewSeal(oid, orec.Public(), res.Succession, 4, []string{claims[0]}, "x@1", "2026-10-30T00:00:00Z")
	// Another identity's recovery key, naming this subject.
	cases["other key"] = NewSeal(id, orec.Public(), res.Succession, 4, []string{claims[0]}, "x@1", "2026-10-30T00:00:00Z")
	// This identity's next recovery key.
	cases["wrong generation"] = NewSeal(id, rec1.Public(), res.Succession, 4, []string{claims[0]}, "x@1", "2026-10-30T00:00:00Z")
	// A part number the succession did not announce.
	rec := recoveryOf(t, words, 0)
	cases["part out of range"] = NewSeal(id, rec.Public(), res.Succession, 4, []string{claims[0]}, "x@1", "2026-10-30T00:00:00Z")
	// Naming a claim instead of a succession.
	cases["not a succession"] = NewSeal(id, rec.Public(), claims[0], 1, []string{claims[1]}, "x@1", "2026-10-30T00:00:00Z")
	signers := map[string]Signer{"signing key": signer, "other identity": orec, "other key": orec, "wrong generation": rec1, "part out of range": rec, "not a succession": rec}
	ids := map[string]string{}
	for name, m := range cases {
		ids[name] = putSeal(t, s, m, signers[name])
	}
	l := verifyOK0(t, s)
	for name, pid := range ids {
		if l.Report.Errors[pid] != "bad seal" {
			t.Errorf("%s: %q", name, l.Report.Errors[pid])
		}
	}
	// The real set is still complete; no claim changed.
	if len(l.Report.Incomplete) != 0 || succ.Parts() != 3 {
		t.Errorf("incomplete %v", l.Report.Incomplete)
	}
	for _, c := range claims {
		if !l.Current(c) {
			t.Errorf("%s: %q", c[:16], l.Report.Errors[c])
		}
	}
}

// verifyOK0 loads and returns the report without requiring exit 0.
func verifyOK0(t *testing.T, s *Store) *Loaded {
	t.Helper()
	l, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// Parts and the succession can arrive in any order across syncs.
func TestPartsOutOfOrderAcrossSync(t *testing.T) {
	a, _, _, claims, res := sealLog(t, 7)
	b := bareLocal(t, a)
	// Deliver the claims and the succession by hand, without the parts.
	la, _ := a.Load()
	for p, data := range la.Files {
		m := pathPattern.FindStringSubmatch(p)
		if m != nil && !slices.Contains(res.Parts, "sha256:"+m[1]+m[2]) {
			putFile(t, b, p, data)
		}
	}
	lb, _ := b.Load()
	if !contains(lb.Report.Incomplete, res.Succession) {
		t.Fatalf("without parts: incomplete %v", lb.Report.Incomplete)
	}
	for _, c := range claims {
		if lb.Report.Errors[c] != "not sealed" {
			t.Errorf("without parts: %s: %q", c[:16], lb.Report.Errors[c])
		}
	}
	// Then the parts, one by one in reverse order.
	for i := len(res.Parts) - 1; i >= 0; i-- {
		putFile(t, b, PathOf(res.Parts[i]), la.Files[PathOf(res.Parts[i])])
		lb, _ = b.Load()
		if done := len(lb.Report.Incomplete) == 0; done != (i == 0) {
			t.Fatalf("after part %d: incomplete %v", i+1, lb.Report.Incomplete)
		}
	}
	verifyOK(t, b)
	// A sync of a fresh folder copies the parts first; the result is the same.
	c := bareLocal(t, a)
	if _, err := c.Sync(a); err != nil {
		t.Fatal(err)
	}
	verifyOK(t, c)
}

// A crash after some parts and before the succession undoes the rotation
// on the next load; a crash after the succession finishes it.
func TestRotateCrashBetweenParts(t *testing.T) {
	withChunk(t, 2)
	s, _, words := initWithWords(t)
	var lines []string
	for i := 0; i < 7; i++ {
		lines = append(lines, partial("hr", float64(i), nil))
	}
	appendLines(t, s, lines...)
	key := s.mustLocal(t).Signer.Public()
	snap := newStore(t)
	n := 0
	hookAfterPart = func() {
		if n++; n == 2 {
			os.CopyFS(snap.Dir, os.DirFS(s.Dir))
		}
	}
	defer func() { hookAfterPart = nil }()
	if _, err := s.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	hookAfterPart = nil
	count := func(st *Store) int {
		k := 0
		filepath.Walk(st.path("objects"), func(p string, fi os.FileInfo, err error) error {
			if b, _ := os.ReadFile(p); fi != nil && !fi.IsDir() && strings.Contains(string(b), `"type":"bean.seal"`) {
				k++
			}
			return nil
		})
		return k
	}
	if count(snap) != 2 {
		t.Fatalf("snapshot has %d parts", count(snap))
	}
	l := verifyOK(t, snap)
	if l.Local.Signer.Public() != key || len(l.Report.Chains[l.Local.Identity]) != 1 {
		t.Error("rotation not undone")
	}
	if count(snap) != 0 {
		t.Errorf("%d orphan parts left", count(snap))
	}
	for _, f := range []string{fileSignKeyNext, fileRotationPending} {
		if _, err := os.Stat(snap.path(f)); !os.IsNotExist(err) {
			t.Errorf("%s left", f)
		}
	}
	appendLines(t, snap, partial("hr", 99, nil))
	if _, err := snap.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	verifyOK(t, snap)
}

// Continuations are core records: no correction, tombstone or append.
func TestSealCannotBeTargeted(t *testing.T) {
	s, _, _, _, res := sealLog(t, 5)
	part := res.Parts[0]
	if _, err := s.Purge(part, ""); err == nil {
		t.Error("purge of a seal part")
	}
	fix := strings.Replace(partial("hr", 2, nil), `"type":"hr"`, `"refs":[{"id":"`+part+`","rel":"supersedes"}],"type":"hr"`, 1)
	if _, err := s.Append(strings.NewReader(fix + "\n")); err == nil {
		t.Error("supersede of a seal part")
	}
	line := `{"type":"bean.seal","value":{"part":1,"sealed":["` + part + `"]},"refs":[{"rel":"seals","id":"` + res.Succession + `"}],"times":{"observed":null},"source":{"system":"self","adapter":"t@1","ref":null}}`
	if _, err := s.Append(strings.NewReader(line + "\n")); err == nil {
		t.Error("append wrote a seal part")
	}
	verifyOK(t, s)
}

func TestSealFields(t *testing.T) {
	rec := NewEd25519Signer([]byte(strings.Repeat("r", 32)))
	id := "sha256:" + strings.Repeat("1", 64)
	sid := "sha256:" + strings.Repeat("2", 64)
	a, b := "sha256:"+strings.Repeat("3", 64), "sha256:"+strings.Repeat("4", 64)
	good := func() map[string]any {
		return NewSeal(id, rec.Public(), sid, 1, []string{a, b}, "x@1", "2026-10-30T00:00:00Z")
	}
	check := func(name string, m map[string]any, want string) {
		t.Helper()
		pid, data, err := SignRecord(m, rec)
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		if fr := CheckFile(PathOf(pid), data); fr.Err != nil {
			got = fr.Err.Error()
		}
		if got != want {
			t.Errorf("%s: got %q want %q", name, got, want)
		}
	}
	check("good", good(), "")
	m := good()
	m["value"].(map[string]any)["part"] = 0.0
	check("part 0", m, "bad field: value")
	m = good()
	m["value"].(map[string]any)["part"] = 1.5
	check("part 1.5", m, "bad field: value")
	m = good()
	m["value"].(map[string]any)["part"] = float64(MaxSealParts + 1)
	check("part too big", m, "bad field: value")
	m = good()
	m["value"].(map[string]any)["sealed"] = []any{}
	check("empty", m, "bad field: value.sealed")
	m = good()
	m["value"].(map[string]any)["sealed"] = []any{b, a}
	check("unsorted", m, "bad field: value.sealed")
	m = good()
	m["value"].(map[string]any)["extra"] = 1.0
	check("extra field", m, "bad field: value")
	m = good()
	m["refs"] = []any{map[string]any{"rel": "succeeds", "id": sid}}
	check("succeeds ref", m, "bad field: refs")
	m = good()
	m["source"].(map[string]any)["ref"] = "sha256:" + strings.Repeat("5", 64)
	check("dedupe ref", m, "bad field: source.ref")
	m = good()
	m["subject"] = "sha256:" + strings.Repeat("6", 64)
	check("subject", m, "bad field: author")
	m = good()
	m["refs"] = []any{map[string]any{"rel": "seals", "id": sid}, map[string]any{"rel": "about", "id": a}}
	check("two refs", m, "bad field: refs")
	m = good()
	m["times"] = map[string]any{"observed": map[string]any{"start": "2026-10-20T00:00:00Z", "precision": "day"}, "imported": "2026-10-30T00:00:00Z"}
	check("observed time", m, "bad field: times.observed")
	m = good()
	m["source"].(map[string]any)["system"] = "csv"
	check("source system", m, "bad field: source")
	m = good()
	m["refs"] = []any{map[string]any{"rel": "seals", "id": "x"}}
	check("bad ref id", m, "bad field: refs")
	sc0 := NewSuccession(id, rec.Public(), sid, rec.Public(), sid, []string{a}, "x@1", "2026-10-30T00:00:00Z")
	sc0["value"].(map[string]any)["parts"] = float64(MaxSealParts + 1)
	check("parts too big", sc0, "bad field: value.parts")
	// "seals" only on bean.seal.
	c := map[string]any{"v": 0.0, "nonce": NewNonce(), "type": "hr", "subject": id, "author": id, "kid": rec.Public(),
		"refs": []any{map[string]any{"rel": "seals", "id": sid}}, "times": map[string]any{"observed": nil, "imported": "2026-10-30T00:00:00Z"},
		"source": map[string]any{"system": "s", "adapter": "a@1", "ref": nil}}
	check("seals on a claim", c, "bad field: refs")
	// value.parts on a succession.
	sc := NewSuccession(id, rec.Public(), sid, rec.Public(), sid, []string{a}, "x@1", "2026-10-30T00:00:00Z")
	sc["value"].(map[string]any)["parts"] = 0.0
	check("parts 0", sc, "bad field: value.parts")
	sc["value"].(map[string]any)["parts"] = 2.0
	check("parts 2", sc, "")
}

var _ = sort.Strings

// bareLocal copies a's local files into a new folder without any record.
func bareLocal(t *testing.T, a *Store) *Store {
	t.Helper()
	b := newStore(t)
	for _, f := range []string{fileIdentity, fileSecret, fileSignKey} {
		data, err := os.ReadFile(a.path(f))
		if err != nil {
			t.Fatal(err)
		}
		if err := writeFileAtomic(b.path(f), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return b
}

func putFile(t *testing.T, s *Store, p string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(s.path(p)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(s.path(p), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// Parts whose succession has not arrived are held; nothing changes yet.
func TestHeldParts(t *testing.T) {
	a, _, _, claims, res := sealLog(t, 7)
	b := bareLocal(t, a)
	la, _ := a.Load()
	for p, data := range la.Files {
		if !strings.Contains(string(data), `"type":"bean.succession"`) || !strings.Contains(string(data), `"succeeds"`) {
			putFile(t, b, p, data)
		}
	}
	lb, _ := b.Load()
	for _, pid := range res.Parts {
		wantWarning(t, lb.Report, pid, "held: ")
		if !lb.Report.Held[pid] {
			t.Error("part not held")
		}
	}
	// The old key is still current on B, so its claims stand.
	for _, c := range claims {
		if !lb.Report.Valid[c] {
			t.Errorf("%s: %q", c[:16], lb.Report.Errors[c])
		}
	}
}

// Parts of the two branches of a fork are never valid; the trunk stands.
func TestPartsOnForkedBranches(t *testing.T) {
	withChunk(t, 2)
	a, _, words := initWithWords(t)
	var lines []string
	for i := 0; i < 7; i++ {
		lines = append(lines, partial("hr", float64(i), nil))
	}
	claims := appendLines(t, a, lines...).IDs
	b := cloneLocal(t, a)
	ra, err := a.Rotate(words, nil, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	rb, err := b.Rotate(words, nil, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	a.Sync(b)
	l, _ := a.Load()
	for _, pid := range append(ra.Parts, rb.Parts...) {
		if l.Report.Valid[pid] {
			t.Errorf("branch part %s valid", pid[:16])
		}
	}
	for _, c := range claims {
		if !l.Report.Valid[c] {
			t.Errorf("trunk claim %s: %q", c[:16], l.Report.Errors[c])
		}
	}
}

// A crash after the succession and before the key rename finishes the
// rotation on the next load.
func TestRotateCrashAfterSuccessionWithParts(t *testing.T) {
	withChunk(t, 2)
	s, _, words := initWithWords(t)
	var lines []string
	for i := 0; i < 7; i++ {
		lines = append(lines, partial("hr", float64(i), nil))
	}
	claims := appendLines(t, s, lines...).IDs
	snap := newStore(t)
	hookAfterSuccession = func() { os.CopyFS(snap.Dir, os.DirFS(s.Dir)) }
	defer func() { hookAfterSuccession = nil }()
	res, err := s.Rotate(words, nil, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	hookAfterSuccession = nil
	l := verifyOK(t, snap)
	if l.Local.Signer.Public() != l.Report.Records[res.Succession].ValueKey() {
		t.Error("rotation not finished")
	}
	for _, f := range []string{fileSignKeyNext, fileRotationPending} {
		if _, err := os.Stat(snap.path(f)); !os.IsNotExist(err) {
			t.Errorf("%s left", f)
		}
	}
	for _, c := range claims {
		if !l.Current(c) {
			t.Errorf("%s: %q", c[:16], l.Report.Errors[c])
		}
	}
}

// Kill loop: a child process rotates with a small chunk and is killed at
// a random moment. Every survivor must load to a verified log, written
// by either the old or the new key, with every claim current.
func TestRotateKillLoop(t *testing.T) {
	if dir := os.Getenv("BEAN_KILL_ROTATE"); dir != "" {
		sealChunk = 20
		s := Open(dir)
		words := strings.Fields(os.Getenv("BEAN_KILL_WORDS"))
		if _, err := s.Rotate(words, nil, NewKey{}); err != nil {
			os.Exit(3)
		}
		os.Exit(0)
	}
	if testing.Short() {
		t.Skip("kill loop")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	base, _, words := initWithWords(t)
	var lines []string
	for i := 0; i < 600; i++ {
		lines = append(lines, partial("hr", float64(i), nil))
	}
	claims := appendLines(t, base, lines...).IDs
	finished, undone := 0, 0
	for i := 0; i < 40; i++ {
		s := newStore(t)
		if err := os.CopyFS(s.Dir, os.DirFS(base.Dir)); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(exe, "-test.run=^TestRotateKillLoop$")
		cmd.Env = append(os.Environ(), "BEAN_KILL_ROTATE="+s.Dir, "BEAN_KILL_WORDS="+strings.Join(words, " "))
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Duration(i%20) * 4 * time.Millisecond)
		cmd.Process.Kill()
		cmd.Wait()
		l, err := s.Load()
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if lines, code, _ := s.VerifyLines(); code != 0 {
			t.Fatalf("run %d: exit %d\n%s", i, code, strings.Join(lines, "\n"))
		}
		for _, c := range claims {
			if !l.Current(c) {
				t.Fatalf("run %d: %s: %q", i, c[:16], l.Report.Errors[c])
			}
		}
		if len(l.Report.Chains[l.Local.Identity]) == 2 {
			finished++
		} else {
			undone++
		}
		// The survivor can always rotate again.
		if _, err := s.Rotate(words, nil, NewKey{}); err != nil {
			t.Fatalf("run %d: rotate after kill: %v", i, err)
		}
		verifyOK(t, s)
	}
	t.Logf("finished %d undone %d", finished, undone)
}

// A hand-written tombstone or supersede of a part is `bad target`, and the
// seal set stays complete.
func TestSealTargetsInVerifier(t *testing.T) {
	s, _, _, claims, res := sealLog(t, 5)
	part := res.Parts[0]
	tomb := `{"type":"bean.tombstone","refs":[{"rel":"purges","id":"` + part + `"}],"times":{"observed":null},"source":{"system":"self","adapter":"t@1","ref":null}}`
	tid := writeRaw(t, s, tomb)
	sup := strings.Replace(partial("hr", 9, nil), `"source"`, `"refs":[{"id":"`+part+`","rel":"supersedes"}],"source"`, 1)
	sid := writeRaw(t, s, sup)
	l, _ := s.Load()
	if l.Report.Errors[tid] != "bad target" || l.Report.Errors[sid] != "bad target" {
		t.Errorf("tombstone %q supersede %q", l.Report.Errors[tid], l.Report.Errors[sid])
	}
	if !l.Report.Valid[part] || len(l.Report.Incomplete) != 0 || !l.Visible(part) {
		t.Error("part lost")
	}
	for _, c := range claims {
		if !l.Current(c) {
			t.Errorf("%s: %q", c[:16], l.Report.Errors[c])
		}
	}
}

// Restore shares the rotation code: a crash between parts undoes it, and
// restore runs again.
func TestRestoreCrashBetweenParts(t *testing.T) {
	withChunk(t, 2)
	a, id, words := initWithWords(t)
	var lines []string
	for i := 0; i < 7; i++ {
		lines = append(lines, partial("hr", float64(i), nil))
	}
	claims := appendLines(t, a, lines...).IDs
	b := newStore(t)
	if _, err := b.Sync(a); err != nil {
		t.Fatal(err)
	}
	snap := newStore(t)
	n := 0
	hookAfterPart = func() {
		if n++; n == 1 {
			os.CopyFS(snap.Dir, os.DirFS(b.Dir))
		}
	}
	defer func() { hookAfterPart = nil }()
	if _, _, err := b.Restore(words, nil, NewKey{}, ""); err != nil {
		t.Fatal(err)
	}
	hookAfterPart = nil
	// The snapshot is not initialized; its load undoes the parts.
	if _, err := snap.Load(); err != nil && !strings.Contains(err.Error(), "not initialized") {
		t.Fatal(err)
	}
	res, got, err := snap.Restore(words, nil, NewKey{}, "")
	if err != nil || got != id {
		t.Fatalf("restore again: %v %s", err, got)
	}
	l := verifyOK(t, snap)
	if len(res.Parts) != 3 || len(l.Report.Chains[id]) != 2 {
		t.Errorf("parts %d chain %d", len(res.Parts), len(l.Report.Chains[id]))
	}
	k := 0
	for _, r := range l.Report.Records {
		if r.Type == "bean.seal" {
			k++
		}
	}
	if k != 3 || len(l.Report.Held) != 0 {
		t.Errorf("%d parts on disk, held %v", k, l.Report.Held)
	}
	for _, c := range claims {
		if !l.Current(c) {
			t.Errorf("%s: %q", c[:16], l.Report.Errors[c])
		}
	}
}
