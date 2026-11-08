package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Attack round 9, on fix-audit7 (2d8e84f). Synthetic data only.

// A1: blobUnlocker lets a record this build cannot judge unlock when its kid
// is any key of a node of the person's tree (DECISIONS 211). That includes a
// key the person retired, which is the key a thief holds: the reason to
// rotate. A judged v0 record signed by the retired key after the rotation is
// not valid and unlocks nothing; the same record with v: 1 unlocks.
func TestAttack9RetiredKeyNewerFormatUnlocksPurgedPhoto(t *testing.T) {
	s, id, words := initWithWords(t)
	photo, err := s.Add("photo", "rash", AddOptions{Photo: []byte("DELETED PHOTO"), PhotoType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	l, _ := s.Load()
	hash := l.Report.Records[photo].Blobs[0].Hash
	k0 := s.mustLocal(t).Signer // stolen
	stale := cloneLocal(t, s)   // a synced copy from before the purge
	if _, err := s.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	if s.mustLocal(t).Signer.Public() == k0.Public() {
		t.Fatal("setup: key not rotated")
	}
	res, err := s.PurgeAll(photo, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("purge kept: %v", res.Kept)
	if _, err := s.GetBlob(hash); err == nil {
		t.Fatal("setup: blob not erased")
	}
	// The thief writes into the stale synced copy, with the retired key.
	forge := func(v float64) map[string]any {
		o := reClaim(k0, id, id, "note", map[string]any{"s": "x"})
		o["v"] = v
		o["blobs"] = []any{map[string]any{"hash": hash, "role": "x", "media_type": "image/jpeg"}}
		return o
	}
	j0 := rePlant(t, stale, forge(0), k0) // judged: contrast
	f1 := rePlant(t, stale, forge(1), k0)
	sr, err := s.Sync(stale)
	t.Logf("sync: %+v %v", sr, err)
	pt, gerr := s.GetBlob(hash)
	t.Logf("v0 record %s, v1 record %s; purged photo back in the main copy: %q %v", j0[:15], f1[:15], pt, gerr)
	if gerr == nil {
		t.Errorf("sync brought a purged photo back for a newer-format record signed by the retired key")
	}
	dir := filepath.Join(t.TempDir(), "out")
	er, err := s.ExportAll(dir)
	ept, rerr := os.ReadFile(filepath.Join(dir, "blobs", hash[7:]))
	t.Logf("export err=%v not_exported=%v; deleted photo in export: %q %v", err, er.NotExported, ept, rerr)
	if rerr == nil {
		t.Errorf("export decrypted the purged photo for a newer-format record signed by the retired key")
	}
	// And the person cannot purge the record that keeps it.
	if _, err := s.PurgeAll(f1, ""); err != nil {
		t.Logf("purge of the record that keeps the file: %v", err)
	}
	if res, err := s.PurgeAll(j0, ""); err == nil {
		t.Logf("purge of the v0 record: kept=%v", res.Kept)
	}
	if _, err := s.GetBlob(hash); err == nil {
		t.Errorf("after purging what it can, the purged photo still reads back")
	}
}

// A2: a sync tool's conflict copy of a record (Syncthing writes
// <name>.sync-conflict-<date>-<time>-<dev>.json beside the file) is loaded as
// a bad path. Purge leaves it, and it keeps the photo (it names the blob).
// Export writes it out under invalid/: the withheld_records rule only looks
// at files whose path is the tombstone's target.
func TestAttack9ConflictCopyOfPurgedRecordExported(t *testing.T) {
	s, _ := initStore(t)
	x, err := s.Add("photo", "rash on my genitals", AddOptions{Photo: []byte("DELETED PHOTO"), PhotoType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	l, _ := s.Load()
	hash := l.Report.Records[x].Blobs[0].Hash
	src := s.path(PathOf(x))
	b, _ := os.ReadFile(src)
	conflict := strings.TrimSuffix(src, ".json") + ".sync-conflict-20261107-101010-ABCDEFG.json"
	if err := os.WriteFile(conflict, b, 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := s.PurgeAll(x, "")
	t.Logf("purge: %+v %v", res, err)
	if err != nil {
		t.Fatal(err)
	}
	if r, bl, err := s.GC(); true {
		t.Logf("gc: %d %d %v", r, bl, err)
	}
	if _, err := s.GetBlob(hash); err == nil {
		t.Errorf("the purged photo still reads back after purge and gc (kept by the conflict copy)")
	}
	dir := filepath.Join(t.TempDir(), "out")
	er, err := s.ExportAll(dir)
	t.Logf("export: invalid=%v withheld=%v err=%v", er.Invalid, er.Withheld, err)
	found := ""
	filepath.Walk(filepath.Join(dir, "invalid"), func(p string, i os.FileInfo, e error) error {
		if e == nil && !i.IsDir() {
			if d, _ := os.ReadFile(p); strings.Contains(string(d), "genitals") {
				found = p
			}
		}
		return nil
	})
	if found != "" {
		rel, _ := filepath.Rel(dir, found)
		t.Errorf("the purged record's text is in the plaintext export: %s", rel)
	}
}

// A3: a too-new identity is meant to leave with its files (DECISIONS 211).
// When the halt is by the recovery key (the update a newer bean writes),
// export judges with the dropped report: no agent.pair is current, so a
// paired agent's photo is not decrypted, while the person's own one is.
func TestAttack9TooNewIdentityLosesAgentFiles(t *testing.T) {
	s, id, words := initWithWords(t)
	if _, _, err := s.AgentInit("", "claude"); err != nil {
		t.Fatal(err)
	}
	ax, err := s.Add("photo", "agent photo", AddOptions{Agent: true, Photo: []byte("AGENT PHOTO"), PhotoType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	px, err := s.Add("photo", "own photo", AddOptions{Photo: []byte("OWN PHOTO"), PhotoType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	l, _ := s.Load()
	ah, ph := l.Report.Records[ax].Blobs[0].Hash, l.Report.Records[px].Blobs[0].Hash
	seed, err := WordsToSeed(words)
	if err != nil {
		t.Fatal(err)
	}
	futureRecord(t, s, id, RecoverySigner(seed, 0)) // a newer bean's record
	mustTooNew(t, s, id, true)
	dir := filepath.Join(t.TempDir(), "out")
	res, err := s.ExportAll(dir)
	_, aerr := os.ReadFile(filepath.Join(dir, "blobs", ah[7:]))
	_, perr := os.ReadFile(filepath.Join(dir, "blobs", ph[7:]))
	t.Logf("export err=%v records=%d blobs=%d not_exported=%v; own photo: %v; agent photo: %v", err, res.Records, res.Blobs, res.NotExported, perr, aerr)
	if perr != nil {
		t.Fatal("setup: own photo not exported")
	}
	if aerr != nil {
		t.Errorf("a too-new identity's export leaves out the paired agent's photo")
	}
}

// A4: readManifestIdentity "reads the file as a stream, so memory does not
// grow with it". json.Decoder.Token holds a whole token: one long string
// value costs its full size in memory, up to the 4 GiB cap.
func TestAttack9ManifestOneTokenMemory(t *testing.T) {
	s, _ := initStore(t)
	s.Add("weight", "81", AddOptions{})
	dir := filepath.Join(t.TempDir(), "out")
	if _, err := s.ExportAll(dir); err != nil {
		t.Fatal(err)
	}
	mp := filepath.Join(dir, "manifest.json")
	b, _ := os.ReadFile(mp)
	const pad = 256 << 20
	f, err := os.Create(mp)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"a":"`)
	chunk := []byte(strings.Repeat("x", 1<<20))
	for i := 0; i < pad>>20; i++ {
		f.Write(chunk)
	}
	f.WriteString(`",` + strings.TrimPrefix(string(b), "{"))
	f.Close()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err = s.ImportExportAll(dir)
	runtime.ReadMemStats(&after)
	alloc := after.TotalAlloc - before.TotalAlloc
	t.Logf("import-export of a %d MiB manifest: err=%v, allocated %d MiB, heap sys %d MiB", pad>>20, err, alloc>>20, after.HeapSys>>20)
	if alloc > pad {
		t.Errorf("one manifest token cost %s of memory", fmt.Sprintf("%d MiB", alloc>>20))
	}
}
