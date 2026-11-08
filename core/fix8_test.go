package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Checks for the fixes of audit 8 beyond the attack tests. Synthetic data only.

// A record of a newer format signed by the person's own key still unlocks
// the blobs it names (B1 must not lock the person out).
func TestFix8NewerFormatOwnKeyExportsBlob(t *testing.T) {
	s, id := initStore(t)
	h, err := s.PutBlob([]byte("OWN FILE"))
	if err != nil {
		t.Fatal(err)
	}
	k := s.mustLocal(t).Signer
	o := reClaim(k, id, id, "note", map[string]any{"s": "x"})
	o["v"] = 1.0
	o["blobs"] = []any{map[string]any{"hash": h, "role": "x", "media_type": "text/plain"}}
	rePlant(t, s, o, k)
	dir := filepath.Join(t.TempDir(), "out")
	res, err := s.ExportAll(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pt, err := os.ReadFile(filepath.Join(dir, "blobs", h[7:])); err != nil || string(pt) != "OWN FILE" {
		t.Errorf("own newer-format record: blob not exported: %q %v %+v", pt, err, res)
	}
	if len(res.Unreferenced) != 0 {
		t.Errorf("listed as unreferenced: %v", res.Unreferenced)
	}
}

// gc erases the blobs before the hidden record. A failure in between leaves
// the hidden record, and the next gc finishes.
func TestFix8GCErasesBlobsFirst(t *testing.T) {
	s, id := initStore(t)
	x, err := s.Add("photo", "rash", AddOptions{Photo: []byte("PHOTO"), PhotoType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	l, _ := s.Load()
	hash := l.Report.Records[x].Blobs[0].Hash
	loc := s.mustLocal(t)
	rePlant(t, s, NewTombstone(id, loc.Signer.Public(), x, "", "x@1", "2026-11-06T00:00:00Z"), loc.Signer)
	if l, _ := s.Load(); !l.Hidden[x] {
		t.Fatal("setup: not hidden")
	}
	s.Now = func() time.Time { return testClock.Add(30 * 24 * time.Hour) }
	shard := filepath.Dir(s.path(BlobPathOf(hash)))
	os.Chmod(shard, 0o500)
	_, _, gerr := s.GC()
	os.Chmod(shard, 0o700)
	if gerr == nil {
		t.Fatal("setup: gc erased the blob")
	}
	if _, err := os.Stat(hiddenPath(s.Dir, x)); err != nil {
		t.Errorf("gc removed the hidden record before its blob: %v", err)
	}
	r, b, err := s.GC()
	if err != nil || r != 1 || b != 1 {
		t.Errorf("second gc: %d records, %d blobs, %v", r, b, err)
	}
	if _, err := os.Stat(s.path(BlobPathOf(hash))); err == nil {
		t.Error("blob left")
	}
}

// import-export reads only the identity of a manifest, whatever its size.
func TestFix8ImportExportLargeManifest(t *testing.T) {
	s, _ := initStore(t)
	s.Add("weight", "81", AddOptions{})
	dir := filepath.Join(t.TempDir(), "out")
	if _, err := s.ExportAll(dir); err != nil {
		t.Fatal(err)
	}
	mp := filepath.Join(dir, "manifest.json")
	b, _ := os.ReadFile(mp)
	// About 120,000 blob entries: more than 16 MiB, as a log of that many
	// files writes.
	var sb strings.Builder
	sb.WriteString(`{"blobs":[`)
	for i := 0; i < 120000; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `{"id":"sha256:%064x","media_type":"image/jpeg","plaintext_sha256":"%064x"}`, i, i)
	}
	sb.WriteString(`],` + strings.TrimPrefix(string(b), `{"blobs":[],`))
	if sb.Len() < 16<<20 {
		t.Fatalf("setup: manifest %d bytes", sb.Len())
	}
	os.WriteFile(mp, []byte(sb.String()), 0o600)
	if _, err := s.ImportExportAll(dir); err != nil {
		t.Errorf("large manifest: %v", err)
	}
	// Two identities, or trailing bytes, are refused.
	os.WriteFile(mp, []byte(strings.TrimSuffix(string(b), "}")+`,"identity":"x"}`), 0o600)
	if _, err := s.ImportExportAll(dir); err == nil {
		t.Error("two identity fields accepted")
	}
	os.WriteFile(mp, append(b, []byte(`{}`)...), 0o600)
	if _, err := s.ImportExportAll(dir); err == nil {
		t.Error("trailing value accepted")
	}
}

// Purge says which files it keeps, and how many other records name each.
func TestFix8PurgeReportsKeptBlob(t *testing.T) {
	s, id := initStore(t)
	x, _ := s.Add("photo", "rash", AddOptions{Photo: []byte("PHOTO"), PhotoType: "image/jpeg"})
	l, _ := s.Load()
	hash := l.Report.Records[x].Blobs[0].Hash
	k := s.mustLocal(t).Signer
	for i := 0; i < 2; i++ {
		o := reClaim(k, id, id, "note", map[string]any{"s": fmt.Sprint(i)})
		o["blobs"] = []any{map[string]any{"hash": hash, "role": "x", "media_type": "image/jpeg"}}
		rePlant(t, s, o, k)
	}
	res, err := s.PurgeAll(x, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Kept) != 1 || res.Kept[0] != (KeptBlob{ID: hash, Names: 2}) {
		t.Errorf("kept: %v", res.Kept)
	}
	y, _ := s.Add("photo", "other", AddOptions{Photo: []byte("PHOTO 2"), PhotoType: "image/jpeg"})
	if res, err := s.PurgeAll(y, ""); err != nil || len(res.Kept) != 0 {
		t.Errorf("a purge that erases its file reports kept: %v %v", res.Kept, err)
	}
}

// The B6 rule holds for sync too: a stale copy that still has a purged photo
// does not bring it back because a stranger's record names it.
func TestFix8SyncStrangerBringsBackPurgedPhoto(t *testing.T) {
	s, id := initStore(t)
	x, _ := s.Add("photo", "rash", AddOptions{Photo: []byte("DELETED PHOTO"), PhotoType: "image/jpeg"})
	l, _ := s.Load()
	hash := l.Report.Records[x].Blobs[0].Hash
	old := cloneLocal(t, s)
	if _, err := s.Purge(x, ""); err != nil {
		t.Fatal(err)
	}
	k := reKey(t)
	o := reClaim(k, k.Public(), id, "note", map[string]any{"s": "x"})
	o["blobs"] = []any{map[string]any{"hash": hash, "role": "x", "media_type": "image/jpeg"}}
	rePlant(t, s, o, k)
	if _, err := s.Sync(old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetBlob(hash); err == nil {
		t.Errorf("sync from a stale copy brought the purged photo back, because a stranger's record names it")
	}
}
