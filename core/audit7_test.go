package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// A1: a thief with only the 24 words (no signing key) forks the chain, signs
// tombstones with the new branch key, and lists them in the branch's own
// sealed list. A branch seal of a record of its own key means nothing, but
// frozenView counts it, so the person's older, sealed records vanish.
func TestAudit7WordsOnlyThiefEmptiesFrozenLog(t *testing.T) {
	s, id, words := initWithWords(t)
	r0, _ := s.Add("lab.a", "1", AddOptions{}) // genesis key
	if _, _, err := s.AgentInit("", "assistant"); err != nil {
		t.Fatal(err)
	}
	ag, _ := s.Add("food", "toast", AddOptions{Agent: true})
	if _, err := s.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	l, _ := s.Load()
	s1 := l.Report.Chains[id][1]
	// The person rotates again: chain G, S1, S2p.
	if _, err := s.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	if got := reList(t, s, "lab.a"); len(got) != 1 {
		t.Fatalf("setup: %d", len(got))
	}
	// The thief: words only.
	seed, _ := WordsToSeed(words)
	kt := reKey(t)
	tomb := NewTombstone(id, kt.Public(), r0, "", "x@1", "2026-11-06T00:00:00Z")
	tid, tb, _ := SignRecord(tomb, kt)
	tid2, tb2, _ := SignRecord(NewTombstone(id, kt.Public(), ag, "", "x@1", "2026-11-06T00:00:00Z"), kt)
	writeFileAtomic(s.path(PathOf(tid2)), tb2, 0o600)
	other := reKey(t)
	list := []string{tid, tid2}
	if list[0] > list[1] {
		list[0], list[1] = list[1], list[0]
	}
	succ := NewSuccession(id, RecoverySigner(seed, 1).Public(), s1.ID, kt.Public(), Commitment(other.Public()), list, "x@1", "2026-11-06T00:00:00Z")
	sid, sb, err := SignRecord(succ, RecoverySigner(seed, 1))
	if err != nil {
		t.Fatal(err)
	}
	writeFileAtomic(s.path(PathOf(tid)), tb, 0o600)
	writeFileAtomic(s.path(PathOf(sid)), sb, 0o600)
	l, _ = s.Load()
	t.Logf("frozen %v; r0: valid %v, status %q; tombstone error %q", l.IsFrozen(), l.Report.Valid[r0], l.Status(r0), l.Report.Errors[tid])
	if got := reList(t, s, "lab.a"); len(got) != 1 {
		t.Errorf("a tombstone of a key below the fork, sealed only by its own branch, hid the person's sealed record: ls shows %d rows", len(got))
	}
	t.Logf("agent record: status %q", l.Status(ag))
	if got := reList(t, s, "food"); len(got) != 1 {
		t.Errorf("the same hid the paired agent's record, which no freeze rule puts in doubt: ls shows %d rows", len(got))
	}
	// Context: the same words-only thief can fork at the genesis with an empty
	// list instead, by the rules of DECISIONS 191, and that also hides r0.
	s2, id2, w2 := initWithWords(t)
	q0, _ := s2.Add("lab.a", "1", AddOptions{})
	s2.Rotate(w2, Seals{All: true}, NewKey{})
	s2.Rotate(w2, Seals{All: true}, NewKey{})
	s2.Load()
	seed2, _ := WordsToSeed(w2)
	succ2 := NewSuccession(id2, RecoverySigner(seed2, 0).Public(), id2, reKey(t).Public(), Commitment(reKey(t).Public()), nil, "x@1", "2026-11-06T00:00:00Z")
	sid2, sb2, _ := SignRecord(succ2, RecoverySigner(seed2, 0))
	writeFileAtomic(s2.path(PathOf(sid2)), sb2, 0o600)
	l2, _ := s2.Load()
	t.Logf("fork at genesis instead: frozen %v, rows %d, status %q", l2.IsFrozen(), len(reList(t, s2, "lab.a")), l2.Status(q0))
}

// A2: sync refuses a record because a tombstone is valid in the source, also
// when the same tombstone is invalid in the destination. A copy that has not
// seen the rotation yet judges a tombstone of the retired key valid, so a
// thief with that key drops the agent's readings from the person's main copy,
// with no word.
func TestAudit7SyncTrustsSourceTombstone(t *testing.T) {
	b, id, words := initWithWords(t)
	b.Add("weight", "81", AddOptions{})
	agent := reKey(t)
	if _, err := b.Pair(agent.Public(), "cloud"); err != nil {
		t.Fatal(err)
	}
	old := b.mustLocal(t).Signer
	secret := b.mustLocal(t).Secret
	d := t.TempDir()
	reCopy(t, b.path("objects"), filepath.Join(d, "objects"))
	a := Open(d) // the cloud copy, agent-only: it has not seen the rotation
	ref := DedupeRef(DedupeKey(secret), []any{"bean/v0/ref", "id", "labcorp", "SPEC1/Potassium", 0.0})
	o := reClaim(agent, agent.Public(), id, "lab.potassium", map[string]any{"n": 4.1})
	o["source"].(map[string]any)["ref"] = ref
	r := rePlant(t, a, o, agent)
	if _, err := b.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	tomb := rePlant(t, a, NewTombstone(id, old.Public(), r, "", "x@1", "2026-11-06T00:00:00Z"), old)
	res, err := b.Sync(a)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := b.Load()
	t.Logf("sync %+v; tombstone in destination: valid %v, error %q; reading present %v", res, l.Report.Valid[tomb], l.Report.Errors[tomb], l.Report.Records[r] != nil)
	if l.Report.Records[r] == nil {
		t.Errorf("a tombstone that is invalid here kept the agent's reading out")
	}
	// A second sync changes nothing while the source has not seen the rotation.
	res, _ = b.Sync(a)
	l, _ = b.Load()
	t.Logf("second sync %+v; reading present %v", res, l.Report.Records[r] != nil)
	_ = strings.TrimSpace
}

// A4: a stranger's own records and tombstones cost the person's copy work
// quadratic in their number on the first load: each hidden target re-reads the
// whole local/purged-refs.jsonl and appends one fsynced line.
func TestAudit7StrangerPurgesQuadratic(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	plant := func(n int) time.Duration {
		s, _, _ := initWithWords(t)
		k := reKey(t)
		g := NewGenesis(k.Public(), Commitment(reKey(t).Public()), "x@1", "2026-11-06T00:00:00Z")
		gid, gb, _ := SignRecord(g, k)
		writeFileAtomic(s.path(PathOf(gid)), gb, 0o600)
		for i := 0; i < n; i++ {
			o := reClaim(k, gid, gid, "note", map[string]any{"s": "x"})
			o["source"].(map[string]any)["ref"] = "hmac-sha256:" + strings.Repeat("0", 56) + fmt.Sprintf("%08x", i)
			id, b, _ := SignRecord(o, k)
			os.MkdirAll(filepath.Dir(s.path(PathOf(id))), 0o700)
			os.WriteFile(s.path(PathOf(id)), b, 0o600)
			tid, tb, _ := SignRecord(NewTombstone(gid, k.Public(), id, "", "x@1", "2026-11-06T00:00:00Z"), k)
			os.MkdirAll(filepath.Dir(s.path(PathOf(tid))), 0o700)
			os.WriteFile(s.path(PathOf(tid)), tb, 0o600)
		}
		start := time.Now()
		if _, err := s.Load(); err != nil {
			t.Fatal(err)
		}
		took := time.Since(start)
		// Written once, as a whole and sorted: not one appended and flushed
		// line per target, in map order.
		data, _ := os.ReadFile(s.path(filePurgedRef))
		lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
		if len(lines) != n || !sort.StringsAreSorted(lines) {
			t.Errorf("n=%d: local/purged-refs.jsonl has %d lines, sorted %v: written line by line", n, len(lines), sort.StringsAreSorted(lines))
		}
		start = time.Now()
		s.Load()
		t.Logf("n=%d stranger record+tombstone pairs: first load %v, second load %v", n, took, time.Since(start))
		return took
	}
	a := plant(1000)
	b := plant(4000)
	t.Logf("4x the files cost %.1fx the time", float64(b)/float64(a))
}

// A5: a record file damaged in place with its size and time kept is masked by
// the object cache, so sync never repairs it (CORE 4.1 item 2) and every read
// says nothing, while the bytes on disk, which rsync and backups copy, are bad.
func TestAudit7CacheMasksDamageFromSyncRepair(t *testing.T) {
	b, _, _ := initWithWords(t)
	r, _ := b.Add("weight", "81", AddOptions{})
	d := t.TempDir()
	reCopy(t, b.path("objects"), filepath.Join(d, "objects"))
	good := Open(d)
	p := b.path(PathOf(r))
	old := time.Now().Add(-time.Hour)
	os.Chtimes(p, old, old) // out of the racy window
	b.Load()                // the cache now vouches for the file
	info, _ := os.Stat(p)
	data, _ := os.ReadFile(p)
	bad := []byte(strings.Replace(string(data), `"81"`, `"18"`, 1))
	if string(bad) == string(data) {
		bad = append([]byte{}, data...)
		bad[len(bad)-3] ^= 1
	}
	os.WriteFile(p, bad, 0o600)
	os.Chtimes(p, info.ModTime(), info.ModTime())
	l, _ := b.Load()
	res, err := b.Sync(good)
	disk, _ := os.ReadFile(p)
	t.Logf("load: record valid %v; sync %+v %v; disk still damaged %v", l.Report.Valid[r], res, err, string(disk) != string(data))
	if string(disk) != string(data) {
		t.Errorf("sync from a good copy left a damaged record file in place")
	}
	lines, code, _ := b.VerifyLines()
	t.Logf("verify exit %d: %v", code, lines)
}

// A6: a stranger's record that names the blob of the person's record keeps the
// blob through purge (C25), and export then decrypts it with the person's key
// and writes the plaintext of the deleted file into the export folder.
func TestAudit7ExportWritesDeletedBlob(t *testing.T) {
	s, id, _ := initWithWords(t)
	photo, err := s.Add("photo", "rash", AddOptions{Photo: []byte("SENSITIVE PHOTO"), PhotoType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	l, _ := s.Load()
	hash := l.Report.Records[photo].Blobs[0].Hash // readable by anyone who can read the folder
	k := reKey(t)
	o := reClaim(k, k.Public(), k.Public(), "note", map[string]any{"s": "x"})
	delete(o, "subject")
	o["subject"] = id
	o["blobs"] = []any{map[string]any{"hash": hash, "role": "x", "media_type": "image/jpeg"}}
	sid := rePlant(t, s, o, k)
	if _, err := s.Purge(photo, ""); err != nil {
		t.Fatal(err)
	}
	// What MCP read_blob checks: a current record about the person names it.
	l, _ = s.Load()
	got, gerr := s.GetBlob(hash)
	t.Logf("after purge: stranger record current %v, subject self %v, GetBlob %q %v", l.Current(sid), l.Report.Records[sid].Subject == id, got, gerr)
	dir := filepath.Join(t.TempDir(), "out")
	recs, blobs, err := s.Export(dir)
	pt, rerr := os.ReadFile(filepath.Join(dir, "blobs", hash[7:]))
	t.Logf("export: %d records, %d blobs, %v; deleted photo in export: %q %v", recs, blobs, err, pt, rerr)
	if rerr == nil {
		t.Errorf("export wrote the plaintext of a file the person deleted, because a stranger's record names it")
	}
}

// A3: on a frozen log, a deletion in doubt hides the agent's correction, but
// the correction still supersedes its target (Superseded is built from Valid),
// so the agent's reading vanishes as well. DECISIONS 197 fixed this only for
// corrections that every branch kept (KeptSuperseded), not for valid ones.
func TestAudit7AgentReadingVanishesBehindHiddenCorrection(t *testing.T) {
	a, _, wa := initWithWords(t)
	if _, _, err := a.AgentInit("", "assistant"); err != nil {
		t.Fatal(err)
	}
	base, err := a.Add("food", "toast", AddOptions{Agent: true})
	if err != nil {
		t.Fatal(err)
	}
	fix, err := a.Add("food", "two toasts", AddOptions{Agent: true, Corrects: base})
	if err != nil {
		t.Fatal(err)
	}
	d := t.TempDir()
	reCopy(t, a.path("objects"), filepath.Join(d, "objects"))
	st := Open(d)
	if _, err := a.Purge(fix, ""); err != nil { // the person deletes the agent's correction
		t.Fatal(err)
	}
	if got := reList(t, a, "food"); len(got) != 1 || got[0].ID != base {
		t.Fatalf("before the freeze, deleting the correction shows the reading again: %d", len(got))
	}
	a.Rotate(wa, Seals{All: true}, NewKey{})
	st.Restore(wa, Seals{All: true}, NewKey{}, "")
	st.Sync(a)
	l, _ := st.Load()
	t.Logf("frozen %v; base: %q; fix: %q", l.IsFrozen(), l.Status(base), l.Status(fix))
	if got := reList(t, st, "food"); len(got) != 1 {
		t.Errorf("the agent's reading and its correction both vanished from ls: %d rows", len(got))
	}
}
