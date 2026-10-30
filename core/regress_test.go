package core

import (
	"crypto"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"os"
	"strings"
	"sync"
	"testing"
)

// Regression tests for the audit of b7a1626 (bean-audit-report/AUDIT.md).
// Each one failed on b7a1626.

// M1, C27: another key's record with the same ref is not a duplicate of
// the person's record and cannot hide it.
func TestDedupePerAuthor(t *testing.T) {
	s, ident := initStore(t)
	mine := appendLines(t, s, partial("lab.glucose", 90, []any{"bean/v0/ref", "id", "test", "spec-1/glucose", 0.0})).IDs[0]
	if _, _, err := s.AgentInit("", "assistant"); err != nil {
		t.Fatal(err)
	}
	a, _ := s.ReadAgent()
	l, _ := s.Load()
	ref := l.Report.Records[mine].Source.Ref
	var fake string
	for {
		obj := map[string]any{"v": 0.0, "nonce": NewNonce(), "type": "lab.glucose", "subject": ident,
			"author": a.Signer.Public(), "kid": a.Signer.Public(),
			"value":  map[string]any{"n": 250.0, "unit": "ms"},
			"times":  map[string]any{"observed": map[string]any{"start": "2026-10-20T00:00:00Z", "precision": "day"}, "imported": "2026-10-28T12:00:00Z"},
			"source": map[string]any{"system": "test", "adapter": "test@1", "ref": ref}}
		id, b, err := SignRecord(obj, a.Signer)
		if err != nil {
			t.Fatal(err)
		}
		if id < mine { // sorts first, so it would win the old 3-tuple group
			writeFileAtomic(s.path(PathOf(id)), b, 0o600)
			fake = id
			break
		}
	}
	l, _ = s.Load()
	if !l.Current(mine) || len(l.Report.Duplicates[mine]) != 0 {
		t.Errorf("person's record hidden as a duplicate of another key's record")
	}
	if !l.Current(fake) {
		t.Errorf("the agent record is its own group")
	}
	// The person's own re-import still dedupes against the person's record.
	if r := appendLines(t, s, partial("lab.glucose", 90, []any{"bean/v0/ref", "id", "test", "spec-1/glucose", 0.0})); r.Added != 0 {
		t.Errorf("re-import added %d", r.Added)
	}
}

// M2, C26: a record of the retired key that the succession does not seal
// is invalid.
func TestRetiredKeyClaimInvalid(t *testing.T) {
	s, ident, words := initWithWords(t)
	old := s.mustLocal(t).Signer
	before := appendLines(t, s, partial("hrv.rmssd", 1, nil)).IDs[0]
	if _, err := s.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	obj := map[string]any{"v": 0.0, "nonce": NewNonce(), "type": "weight", "subject": ident,
		"author": ident, "kid": old.Public(), "value": map[string]any{"n": 200.0, "unit": "kg"},
		"times":  map[string]any{"observed": nil, "imported": "2026-10-29T12:00:00Z"},
		"source": map[string]any{"system": "self", "adapter": "thief@1", "ref": nil}}
	id, b, _ := SignRecord(obj, old)
	writeFileAtomic(s.path(PathOf(id)), b, 0o600)
	l, _ := s.Load()
	if l.Report.Valid[id] || l.Report.Errors[id] != "not sealed" {
		t.Errorf("retired key record: valid=%v err=%q", l.Report.Valid[id], l.Report.Errors[id])
	}
	if !l.Report.Valid[before] {
		t.Errorf("record sealed at rotation lost: %q", l.Report.Errors[before])
	}
}

// B2, C25: a purge never erases a blob that no erased record named.
func TestPurgeKeepsLooseBlob(t *testing.T) {
	s, _ := initStore(t)
	bid, err := s.PutBlob([]byte("lab order pdf bytes"))
	if err != nil {
		t.Fatal(err)
	}
	x := appendLines(t, s, partial("hr", 60, nil)).IDs[0]
	if _, err := s.Purge(x, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.path(BlobPathOf(bid))); err != nil {
		t.Errorf("loose blob erased by an unrelated purge: %v", err)
	}
}

func blobLine(bid string) string {
	return `{"type":"food","value":{"blob":"` + bid + `"},"blobs":[{"hash":"` + bid + `","role":"photo","media_type":"image/jpeg"}],"times":{"observed":null},"source":{"system":"self","adapter":"t@1","ref":null}}`
}

// B2: purge erases the target's own blob, but keeps it while another
// record file names it.
func TestPurgeErasesOwnBlobOnly(t *testing.T) {
	s, _ := initStore(t)
	bid, _ := s.PutBlob([]byte("photo"))
	ids := appendLines(t, s, blobLine(bid), strings.Replace(blobLine(bid), `"food"`, `"meal"`, 1)).IDs
	if _, err := s.Purge(ids[0], ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.path(BlobPathOf(bid))); err != nil {
		t.Fatal("blob erased while another record names it")
	}
	if _, err := s.Purge(ids[1], ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.path(BlobPathOf(bid))); !os.IsNotExist(err) {
		t.Errorf("blob of the last purged record kept: %v", err)
	}
}

// B2, DECISIONS 107: gc refuses while the log is too new.
func TestGCRefusesTooNew(t *testing.T) {
	s, ident := initStore(t)
	bid, _ := s.PutBlob([]byte("photo"))
	appendLines(t, s, blobLine(bid))
	loc := s.mustLocal(t)
	obj := map[string]any{"v": 1.0, "nonce": NewNonce(), "type": "bean.future", "subject": ident,
		"author": ident, "kid": loc.Signer.Public(), "times": map[string]any{"observed": nil, "imported": "2026-10-29T12:00:00Z"},
		"source": map[string]any{"system": "self", "adapter": "bean@1.0.0", "ref": nil}}
	id, b, _ := SignRecord(obj, loc.Signer)
	writeFileAtomic(s.path(PathOf(id)), b, 0o600)
	if _, _, err := s.GC(); err == nil || !strings.HasPrefix(err.Error(), "too new: ") {
		t.Errorf("gc on a too-new log: %v", err)
	}
	if _, err := os.Stat(s.path(BlobPathOf(bid))); err != nil {
		t.Errorf("blob erased because the identity is too new")
	}
}

// B2, DECISIONS 107: gc refuses on a frozen identity.
func TestGCRefusesCompromised(t *testing.T) {
	s, _, words := initWithWords(t)
	b := cloneLocal(t, s)
	if _, err := s.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	bid, _ := s.PutBlob([]byte("photo after rotation"))
	appendLines(t, s, blobLine(bid))
	if _, err := b.Rotate(words, nil, NewKey{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sync(b); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.GC(); err == nil || !strings.HasPrefix(err.Error(), "compromised: ") {
		t.Errorf("gc on a frozen identity: %v", err)
	}
	if _, err := os.Stat(s.path(BlobPathOf(bid))); err != nil {
		t.Errorf("blob erased after fork freeze")
	}
}

// M3, DECISIONS 109: a stranger's genesis with a copied commitment makes
// restore ambiguous; -identity picks the person's own.
func TestRestoreAmbiguous(t *testing.T) {
	s, ident, words := initWithWords(t)
	l, _ := s.Load()
	rec := l.Report.Records[ident].Recovery()
	var evil string
	for i := 0; ; i++ {
		atk := NewEd25519Signer([]byte(strings.Repeat("a", 31) + string(rune('a'+i%26))))
		g := NewGenesis(atk.Public(), rec, "evil@1", "2026-10-28T12:00:00Z")
		id, b, _ := SignRecord(g, atk)
		if id > ident {
			writeFileAtomic(s.path(PathOf(id)), b, 0o600)
			evil = id
			break
		}
	}
	n := newStore(t)
	os.CopyFS(n.Dir, os.DirFS(s.Dir))
	os.RemoveAll(n.path(dirLocal))
	if _, _, err := n.Restore(words, nil, NewKey{}); err == nil || !strings.HasPrefix(err.Error(), "ambiguous: ") {
		t.Fatalf("restore with two matches: %v", err)
	}
	if _, err := n.ReadLocal(); err == nil {
		t.Fatal("ambiguous restore wrote local files")
	}
	if _, _, err := n.RestoreIdentity(words, nil, NewKey{}, "sha256:"+strings.Repeat("0", 64)); err == nil {
		t.Error("restore to an identity that does not match")
	}
	_, got, err := n.RestoreIdentity(words, nil, NewKey{}, ident)
	if err != nil || got != ident {
		t.Fatalf("restore -identity: %s %v", got, err)
	}
	_ = evil
	verifyOK(t, n)
}

// M4, DECISIONS 108: a write that the verifier would reject is refused.
func TestWriteRefusesTooNew(t *testing.T) {
	s, ident := initStore(t)
	loc := s.mustLocal(t)
	obj := map[string]any{"v": 1.0, "nonce": NewNonce(), "type": "bean.future", "subject": ident,
		"author": ident, "kid": loc.Signer.Public(), "times": map[string]any{"observed": nil, "imported": "2026-10-29T12:00:00Z"},
		"source": map[string]any{"system": "self", "adapter": "bean@1.0.0", "ref": nil}}
	id, b, _ := SignRecord(obj, loc.Signer)
	writeFileAtomic(s.path(PathOf(id)), b, 0o600)
	if _, err := s.Append(strings.NewReader(partial("hr", 1, nil) + "\n")); err == nil || !strings.HasPrefix(err.Error(), "too new: ") {
		t.Errorf("append on a too-new log: %v", err)
	}
}

func TestWriteRefusesIdentityRecords(t *testing.T) {
	s, ident := initStore(t)
	line := `{"type":"bean.tombstone","value":{"reason":""},"refs":[{"rel":"purges","id":"` + ident + `"}],"times":{"observed":null},"source":{"system":"self","adapter":"t@1","ref":null}}`
	if _, err := s.Append(strings.NewReader(line + "\n")); err == nil {
		t.Error("append wrote a tombstone of the genesis")
	}
	line = `{"type":"bean.succession","value":{},"times":{"observed":null},"source":{"system":"self","adapter":"t@1","ref":null}}`
	if _, err := s.Append(strings.NewReader(line + "\n")); err == nil {
		t.Error("append wrote a succession")
	}
	verifyOK(t, s)
}

// B1, DECISIONS 106: concurrent rotations on one folder never fork it.
func TestConcurrentRotateNoFork(t *testing.T) {
	s, id, words := initWithWords(t)
	appendLines(t, s, partial("hr", 1, nil))
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			h := Open(s.Dir) // separate handles share only the file lock
			h.Now = s.Now
			_, errs[i] = h.Rotate(words, nil, NewKey{})
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	l := verifyOK(t, s)
	if len(l.Report.Chains[id]) != 5 || len(l.Report.Compromised) != 0 {
		t.Errorf("chain %d compromised %v", len(l.Report.Chains[id]), l.Report.Compromised)
	}
}

// B1: concurrent appends and purges on one folder keep it valid.
func TestConcurrentWrites(t *testing.T) {
	s, _ := initStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			h := Open(s.Dir)
			h.Now = s.Now
			r, err := h.Append(strings.NewReader(partial("hr", float64(i), nil) + "\n"))
			if err != nil {
				t.Error(err)
				return
			}
			if i%2 == 0 {
				if _, err := h.Purge(r.IDs[0], ""); err != nil {
					t.Error(err)
				}
			}
		}(i)
	}
	wg.Wait()
	verifyOK(t, s)
}

// M5, DECISIONS 110: a crash after the succession is written finishes the
// rotation on the next load.
func TestRotateCrashAfterSuccession(t *testing.T) {
	s, id, words := initWithWords(t)
	appendLines(t, s, partial("hr", 1, nil))
	snap := newStore(t)
	hookAfterSuccession = func() {
		if err := os.CopyFS(snap.Dir, os.DirFS(s.Dir)); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { hookAfterSuccession = nil }()
	res, err := s.Rotate(words, nil, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	// snap is the folder as a crash right after the succession left it.
	if _, err := os.Stat(snap.path(fileSignKeyNext)); err != nil {
		t.Fatal("no pending key in the snapshot")
	}
	l := verifyOK(t, snap)
	if l.Local.Signer.Public() != res.NewKey || l.Report.Chains[id][1].ValueKey() != res.NewKey {
		t.Error("rotation not finished on load")
	}
	if _, err := os.Stat(snap.path(fileSignKeyNext)); !os.IsNotExist(err) {
		t.Error("pending key left behind")
	}
	appendLines(t, snap, partial("hr", 2, nil))
	verifyOK(t, snap)
}

// M5: a crash before the succession discards the pending key.
func TestRotateCrashBeforeSuccession(t *testing.T) {
	s, _ := initStore(t)
	key := s.mustLocal(t).Signer.Public()
	pending := NewEd25519Signer([]byte(strings.Repeat("p", 32)))
	writeFileAtomic(s.path(fileSignKeyNext), []byte("ed25519:"+strings.Repeat("70", 32)+"\n"), 0o600)
	_ = pending
	l := verifyOK(t, s)
	if l.Local.Signer.Public() != key {
		t.Error("signing key changed")
	}
	if _, err := os.Stat(s.path(fileSignKeyNext)); !os.IsNotExist(err) {
		t.Error("pending key not discarded")
	}
}

// MINOR 1: a bad blob id is an error, not a panic.
func TestGetBlobBadID(t *testing.T) {
	s, _ := initStore(t)
	for _, id := range []string{"", "x", "sha256:", "sha256:../../etc/passwd", "sha256:" + strings.Repeat("A", 64)} {
		if _, err := s.GetBlob(id); err == nil {
			t.Errorf("GetBlob(%q) passed", id)
		}
	}
}

// MINOR: a core record from before the rename says so.
func TestOldFormatRecord(t *testing.T) {
	obj := map[string]any{"v": 0.0, "nonce": NewNonce(), "type": "hlog.genesis", "subject": nil}
	sg := NewEd25519Signer([]byte(strings.Repeat("o", 32)))
	obj["author"], obj["kid"] = sg.Public(), sg.Public()
	id, b, _ := SignRecord(obj, sg)
	if fr := CheckFile(PathOf(id), b); fr.Err != ErrOldFormat {
		t.Errorf("got %v", fr.Err)
	}
}

// Stale temporary files from a crash are removed.
func TestStaleTempsRemoved(t *testing.T) {
	s, _ := initStore(t)
	p := s.path("objects/.tmp-crash")
	writeFileAtomic(p, []byte("x"), 0o600)
	old := testClock.Add(-2 * 3600e9)
	os.Chtimes(p, old, old)
	s.Load()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("stale temp kept: %v", err)
	}
}

// MINOR: a bad id never panics in PathOf or BlobPathOf.
func TestPathOfBadID(t *testing.T) {
	for _, id := range []string{"", "sha256:", "sha256:ab", "x"} {
		if p := PathOf(id); !strings.HasSuffix(p, badIDPath) {
			t.Errorf("PathOf(%q) = %q", id, p)
		}
		if p := BlobPathOf(id); !strings.HasSuffix(p, badIDPath) {
			t.Errorf("BlobPathOf(%q) = %q", id, p)
		}
	}
}

// MINOR: SignRecord refuses invalid UTF-8, NaN and Inf instead of storing
// a different record than the caller gave.
func TestSignRecordRefusesBadValues(t *testing.T) {
	sg := NewEd25519Signer([]byte(strings.Repeat("s", 32)))
	for _, v := range []any{"a\xffb", math.NaN(), math.Inf(1), map[string]any{"\xff": 1.0}, []any{math.Inf(-1)}} {
		obj := map[string]any{"v": 0.0, "kid": sg.Public(), "value": v}
		if _, _, err := SignRecord(obj, sg); err == nil {
			t.Errorf("SignRecord(%v) passed", v)
		}
	}
}

// MINOR: P-256 software signatures are deterministic (RFC 6979).
func TestP256Deterministic(t *testing.T) {
	d, _ := hex.DecodeString("C9AFA9D845BA75166B5C215767B1D6934E50C3DB36E89B127B8A622B120F6721")
	sg, err := NewP256Signer(d)
	if err != nil {
		t.Fatal(err)
	}
	// RFC 6979 A.2.5, P-256 with SHA-256, message "sample".
	h := sha256.Sum256([]byte("sample"))
	der, err := sg.key.Sign(nil, h[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	var rs struct{ R, S *big.Int }
	asn1.Unmarshal(der, &rs)
	if fmt.Sprintf("%X", rs.R) != "EFD48B2AACB6A8FD1140DD9CD45E81D69D2C877B56AAF991C34D0EA84EAF3716" ||
		fmt.Sprintf("%X", rs.S) != "F7CB1C942D657C41D436C7A1B6E29F65F3E900DBB9AFF4064DC4AB2F843ACDA8" {
		t.Fatalf("not RFC 6979: r=%X s=%X", rs.R, rs.S)
	}
	if want := "p256:0360fed4ba255a9d31c961eb74c6356d68c049b8923b61fa6ce669622e60f29fb6"; sg.Public() != want {
		t.Errorf("public %s", sg.Public())
	}
	id := "sha256:" + strings.Repeat("ab", 32)
	a, _ := sg.Sign(id)
	b, _ := sg.Sign(id)
	if a != b {
		t.Error("two signatures differ")
	}
	if err := Verify(sg.Public(), id, a); err != nil {
		t.Error(err)
	}
}

// Round 1 re-audit: a thief's tombstone that the person excludes with
// -no-seal must give the target back valid, not "not sealed" (C26 with
// SPEC 7.6). The target was purged at rotation time, so a seal set of
// only valid records missed it.
func TestNoSealTombstoneRestoresTarget(t *testing.T) {
	a, _, words := initWithWords(t)
	tgt := appendLines(t, a, partial("hr", 1, nil)).IDs[0]
	thief := cloneLocal(t, a)
	thief.Sync(a)
	d, err := thief.Purge(tgt, "thief")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Sync(thief); err != nil {
		t.Fatal(err)
	}
	la, _ := a.Load()
	if !la.Hidden[tgt] {
		t.Fatal("A did not hide the target")
	}
	res, err := a.Rotate(words, []string{d}, NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.NotSealed, d) || !contains(res.Sealed, tgt) {
		t.Fatalf("sealed %v not sealed %v", res.Sealed, res.NotSealed)
	}
	la, _ = a.Load()
	if !la.Visible(tgt) {
		t.Errorf("target not restored: valid=%v err=%q hidden=%v", la.Report.Valid[tgt], la.Report.Errors[tgt], la.Hidden[tgt])
	}
	if la.Report.Errors[d] != "not sealed" {
		t.Errorf("thief tombstone: %q", la.Report.Errors[d])
	}
}

// M4 must not block a new copy that has local/ but has not synced yet.
func TestWriteBeforeFirstSync(t *testing.T) {
	a, _ := initStore(t)
	b := newStore(t)
	for _, f := range []string{fileIdentity, fileSecret, fileSignKey} {
		data, _ := os.ReadFile(a.path(f))
		writeFileAtomic(b.path(f), data, 0o600)
	}
	if r := appendLines(t, b, partial("hr", 1, nil)); r.Added != 1 {
		t.Fatal("append before the first sync")
	}
	if _, err := a.Sync(b); err != nil {
		t.Fatal(err)
	}
	verifyOK(t, a)
}
