package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Checks for the fixes of attack round 9 beyond the attack tests. Synthetic
// data only.

// newerFormat writes a v: 1 record of the person signed by k that names blob.
func newerFormat(t *testing.T, s *Store, id string, k Signer, blob string) string {
	t.Helper()
	o := reClaim(k, id, id, "note", map[string]any{"s": "x"})
	o["v"] = 1.0
	o["blobs"] = []any{map[string]any{"hash": blob, "role": "x", "media_type": "image/jpeg"}}
	return rePlant(t, s, o, k)
}

// The person can purge a record this build cannot judge, and a stale copy
// does not bring it back.
func TestFix9PurgeNewerFormatRecord(t *testing.T) {
	s, id := initStore(t)
	h, err := s.PutBlob([]byte("NEWER PHOTO"))
	if err != nil {
		t.Fatal(err)
	}
	k := s.mustLocal(t).Signer
	x := newerFormat(t, s, id, k, h)
	stale := cloneLocal(t, s)
	res, err := s.PurgeAll(x, "")
	if err != nil {
		t.Fatalf("purge of a newer-format record: %v", err)
	}
	if len(res.Kept) != 0 {
		t.Errorf("kept: %v", res.Kept)
	}
	if _, err := os.Stat(s.path(PathOf(x))); err == nil {
		t.Error("record file left")
	}
	if _, err := s.GetBlob(h); err == nil {
		t.Error("its file still reads")
	}
	sr, err := s.Sync(stale)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.path(PathOf(x))); err == nil || sr.Refused != 1 {
		t.Errorf("sync brought the purged record back: %+v", sr)
	}
	if _, err := s.GetBlob(h); err == nil {
		t.Error("sync brought its file back")
	}
	// A copy that has the record and the tombstone leaves it out of export.
	if _, err := stale.Sync(s); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "out")
	if _, err := stale.ExportAll(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "records", x[7:]+".json")); err == nil {
		t.Error("export wrote a record the person deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, "blobs", h[7:])); err == nil {
		t.Error("export decrypted the file of a record the person deleted")
	}
}

// A newer-format record of the current key still unlocks; one of a key the
// person retired does not.
func TestFix9NewerFormatOnlyGateKeys(t *testing.T) {
	s, id, words := initWithWords(t)
	old := s.mustLocal(t).Signer
	if _, err := s.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	head := s.mustLocal(t).Signer
	hOld, _ := s.PutBlob([]byte("OLD KEY"))
	hHead, _ := s.PutBlob([]byte("HEAD KEY"))
	newerFormat(t, s, id, old, hOld)
	newerFormat(t, s, id, head, hHead)
	dir := filepath.Join(t.TempDir(), "out")
	res, err := s.ExportAll(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "blobs", hHead[7:])); err != nil {
		t.Errorf("head key's newer-format record: file not exported: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "blobs", hOld[7:])); err == nil {
		t.Error("retired key's newer-format record unlocked a file")
	}
}

// Purge counts only records it can reach. A deleted record that waits for gc
// keeps the file, but is not one to purge.
func TestFix9PurgeKeptCountsReachable(t *testing.T) {
	s, id := initStore(t)
	x, _ := s.Add("photo", "rash", AddOptions{Photo: []byte("PHOTO"), PhotoType: "image/jpeg"})
	l, _ := s.Load()
	hash := l.Report.Records[x].Blobs[0].Hash
	loc := s.mustLocal(t)
	o := reClaim(loc.Signer, id, id, "note", map[string]any{"s": "y"})
	o["blobs"] = []any{map[string]any{"hash": hash, "role": "x", "media_type": "image/jpeg"}}
	y := rePlant(t, s, o, loc.Signer)
	// y is deleted by a tombstone that came in by sync: hidden, waiting for gc.
	rePlant(t, s, NewTombstone(id, loc.Signer.Public(), y, "", "x@1", "2026-11-06T00:00:00Z"), loc.Signer)
	if l, _ := s.Load(); !l.Hidden[y] {
		t.Fatal("setup: not hidden")
	}
	res, err := s.PurgeAll(x, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Kept) != 1 || res.Kept[0] != (KeptBlob{ID: hash, Names: 0, Others: 1}) {
		t.Errorf("kept: %+v", res.Kept)
	}
}

// A conflict copy that a sync tool writes after the purge is withheld by
// export and erased by gc, with the file it names.
func TestFix9GCErasesLateConflictCopy(t *testing.T) {
	s, _ := initStore(t)
	x, _ := s.Add("photo", "rash", AddOptions{Photo: []byte("PHOTO"), PhotoType: "image/jpeg"})
	l, _ := s.Load()
	hash := l.Report.Records[x].Blobs[0].Hash
	src := s.path(PathOf(x))
	b, _ := os.ReadFile(src)
	blob, _ := os.ReadFile(s.path(BlobPathOf(hash)))
	if _, err := s.PurgeAll(x, ""); err != nil {
		t.Fatal(err)
	}
	conflict := strings.TrimSuffix(src, ".json") + " (conflicted copy).json"
	if err := os.WriteFile(conflict, b, 0o600); err != nil {
		t.Fatal(err)
	}
	putFile(t, s, BlobPathOf(hash), blob)
	dir := filepath.Join(t.TempDir(), "out")
	er, err := s.ExportAll(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(er.Withheld) != 1 || len(er.Invalid) != 0 {
		t.Errorf("export: withheld %v, invalid %v", er.Withheld, er.Invalid)
	}
	r, bl, err := s.GC()
	if err != nil || r != 1 || bl != 1 {
		t.Errorf("gc: %d records, %d blobs, %v", r, bl, err)
	}
	if _, err := os.Stat(conflict); err == nil {
		t.Error("conflict copy left")
	}
	if _, err := s.GetBlob(hash); err == nil {
		t.Error("file left")
	}
}

// A copy halted by its head key judges deletions without the halt: a stale
// copy does not bring back a purged record or its photo.
func TestFix9SyncIntoHeadHaltedCopy(t *testing.T) {
	s, id := initStore(t)
	x, _ := s.Add("photo", "rash", AddOptions{Photo: []byte("PHOTO"), PhotoType: "image/jpeg"})
	l, _ := s.Load()
	hash := l.Report.Records[x].Blobs[0].Hash
	stale := cloneLocal(t, s)
	if _, err := s.Purge(x, ""); err != nil {
		t.Fatal(err)
	}
	thiefHalt(t, s, id, s.mustLocal(t).Signer)
	mustTooNew(t, s, id, true)
	if _, err := s.Sync(stale); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.path(PathOf(x))); err == nil {
		t.Error("sync brought the purged record back into a head-halted copy")
	}
	if _, err := s.GetBlob(hash); err == nil {
		t.Error("sync brought the purged photo back into a head-halted copy")
	}
}

// A deep manifest is refused, without a token stack of its size.
func TestFix9ManifestDepth(t *testing.T) {
	s, _ := initStore(t)
	dir := filepath.Join(t.TempDir(), "out")
	if _, err := s.ExportAll(dir); err != nil {
		t.Fatal(err)
	}
	mp := filepath.Join(dir, "manifest.json")
	b, _ := os.ReadFile(mp)
	deep := `{"a":` + strings.Repeat("[", 1000) + strings.Repeat("]", 1000) + "," + strings.TrimPrefix(string(b), "{")
	os.WriteFile(mp, []byte(deep), 0o600)
	if _, err := s.ImportExportAll(dir); err == nil || !strings.Contains(err.Error(), "manifest.json") {
		t.Errorf("deep manifest: %v", err)
	}
}
