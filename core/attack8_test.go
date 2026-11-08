package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// B1: export decrypts a blob for "a record this build cannot judge" without
// checking that the person signed it. The author field of a record of a
// newer format is a bare string: a stranger writes author = the person's id,
// signs with their own key, and names the blob of a photo the person will
// purge. The record keeps the blob through purge (C25), and export decrypts
// the deleted photo.
func TestAttack8UnknownVersionForgedAuthorUnlocksPurgedBlob(t *testing.T) {
	s, id := initStore(t)
	photo, err := s.Add("photo", "rash", AddOptions{Photo: []byte("DELETED PHOTO"), PhotoType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	l, _ := s.Load()
	hash := l.Report.Records[photo].Blobs[0].Hash
	k := reKey(t) // a stranger
	o := reClaim(k, id, id, "note", map[string]any{"s": "x"})
	o["v"] = 1.0 // a newer format: this build checks only the envelope
	o["blobs"] = []any{map[string]any{"hash": hash, "role": "x", "media_type": "image/jpeg"}}
	fid := rePlant(t, s, o, k)
	if _, err := s.Purge(photo, ""); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "out")
	res, err := s.ExportAll(dir)
	pt, rerr := os.ReadFile(filepath.Join(dir, "blobs", hash[7:]))
	t.Logf("forged v1 record %s author=%v kid=stranger; export %+v err=%v; deleted photo in export: %q %v", fid[:15], o["author"] == id, res, err, pt, rerr)
	if rerr == nil {
		t.Errorf("export decrypted the purged photo for a stranger's record of a newer format that claims the person as author")
	}
}

// B2: purge erases the record file before its blobs. If the blob erase does
// not happen (crash, kill, permission error), the photo is left with no record
// that names it, gc never erases it (it erases only blobs of erased records),
// and since DECISIONS 208 every export decrypts it as "unreferenced".
func TestAttack8HalfPurgedPhotoExportedAsUnreferenced(t *testing.T) {
	s, _ := initStore(t)
	photo, err := s.Add("photo", "rash", AddOptions{Photo: []byte("DELETED PHOTO"), PhotoType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	l, _ := s.Load()
	hash := l.Report.Records[photo].Blobs[0].Hash
	shard := filepath.Dir(s.path(BlobPathOf(hash)))
	// Stand-in for a crash between the two erasures: the blob cannot be removed.
	os.Chmod(shard, 0o500)
	_, perr := s.Purge(photo, "")
	os.Chmod(shard, 0o700)
	t.Logf("purge: %v", perr)
	// Since the fix the blobs go first: the purge fails, and the record and
	// its tombstone stay, so a load hides the record.
	if l, _ := s.Load(); !l.Hidden[photo] {
		if _, err := os.Stat(s.path(PathOf(photo))); err == nil {
			t.Fatal("the record is still visible after a failed purge")
		}
	}
	gr, gb, gerr := s.GC()
	t.Logf("gc: %d records, %d blobs, %v", gr, gb, gerr)
	dir := filepath.Join(t.TempDir(), "out")
	res, err := s.ExportAll(dir)
	pt, rerr := os.ReadFile(filepath.Join(dir, "blobs", hash[7:]))
	t.Logf("export %+v %v; deleted photo in export: %q %v", res, err, pt, rerr)
	if rerr == nil {
		t.Errorf("a photo of a purged record went out in the plaintext export as unreferenced")
	}
}

// B3: a damaged copy of a purged record (stale backup copied back with rsync,
// bit rot) fails its checks, so no tombstone can hide it. Since DECISIONS 208,
// export copies it as it is into invalid/: the deleted reading is in the
// plaintext export, although the tombstone that names its id is right there.
func TestAttack8DamagedPurgedRecordExportedRaw(t *testing.T) {
	s, _ := initStore(t)
	x, err := s.Add("diagnosis.private", "SECRET DIAGNOSIS", AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(s.path(PathOf(x)))
	if _, err := s.Purge(x, ""); err != nil {
		t.Fatal(err)
	}
	damaged := strings.Replace(string(b), `"type":`, `"type" :`, 1) // one byte: not canonical
	os.MkdirAll(filepath.Dir(s.path(PathOf(x))), 0o700)
	if err := os.WriteFile(s.path(PathOf(x)), []byte(damaged), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "out")
	res, err := s.ExportAll(dir)
	t.Logf("export %+v %v", res, err)
	found := ""
	filepath.Walk(filepath.Join(dir, "invalid"), func(p string, i os.FileInfo, e error) error {
		if e == nil && !i.IsDir() {
			if d, _ := os.ReadFile(p); strings.Contains(string(d), "SECRET DIAGNOSIS") {
				found = p
			}
		}
		return nil
	})
	t.Logf("purged reading in export: %q", found)
	if found != "" {
		t.Errorf("export wrote a purged record's content to invalid/")
	}
}

// B4: import-export now reads manifest.json with a 16 MiB cap. A stranger's
// records may name any number of absent blobs; each one is listed in
// missing_blobs. About 30 records of 1 MiB make the person's own export
// impossible to import again.
func TestAttack8StrangerMakesExportUnimportable(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	s, id := initStore(t)
	s.Add("weight", "81", AddOptions{})
	k := reKey(t)
	n := 0
	for r := 0; r < 30; r++ {
		var bl []any
		for i := 0; i < 8500; i++ {
			bl = append(bl, map[string]any{"hash": fmt.Sprintf("sha256:%064x", n), "role": "x", "media_type": "x"})
			n++
		}
		o := reClaim(k, k.Public(), id, "note", map[string]any{"s": "x"})
		o["blobs"] = bl
		rePlant(t, s, o, k)
	}
	dir := filepath.Join(t.TempDir(), "out")
	res, err := s.ExportAll(dir)
	info, _ := os.Stat(filepath.Join(dir, "manifest.json"))
	t.Logf("export: %d records, %d missing, err %v; manifest %d bytes", res.Records, len(res.Missing), err, info.Size())
	fresh := Open(t.TempDir())
	_ = fresh
	// Import into the same log: the export of this very identity.
	_, ierr := s.ImportExportAll(dir)
	t.Logf("import-export of own export: %v", ierr)
	if ierr != nil {
		t.Errorf("the person's own export cannot be imported again: %v", ierr)
	}
}

// B5: a blob id named only by a record file that fails its checks is listed in
// not_exported_blobs, and the CLI says "stored files were not decrypted",
// also when no such blob is stored. A damaged file of random ids makes the
// manifest claim files that do not exist.
func TestAttack8NotExportedListsAbsentBlobs(t *testing.T) {
	s, _ := initStore(t)
	s.Add("weight", "81", AddOptions{})
	ghost := fmt.Sprintf("sha256:%064x", 7)
	p := "objects/aa/" + strings.Repeat("b", 62) + ".json"
	os.MkdirAll(filepath.Dir(s.path(p)), 0o700)
	os.WriteFile(s.path(p), []byte(`{"blobs":[{"hash":"`+ghost+`"}]}`), 0o600)
	_, statErr := os.Stat(s.path(BlobPathOf(ghost)))
	dir := filepath.Join(t.TempDir(), "out")
	res, err := s.ExportAll(dir)
	t.Logf("blob stored: %v; export NotExported=%v Missing=%v err=%v", statErr == nil, res.NotExported, res.Missing, err)
	if res.NotExported[ghost] != "" && statErr != nil {
		t.Errorf("not_exported_blobs lists a blob that is not stored")
	}
}

// B6: import-export skips a blob only when no live record names it. A
// stranger's record that names the purged photo's blob (any synced record can
// name any id; the id is in the person's record and in the export manifest)
// makes an older export bring the deleted photo back onto disk, where
// blob get decrypts it. DECISIONS 207 says it no longer does.
func TestAttack8ImportExportStrangerBringsBackPurgedPhoto(t *testing.T) {
	s, id := initStore(t)
	x, _ := s.Add("photo", "rash", AddOptions{Photo: []byte("DELETED PHOTO"), PhotoType: "image/jpeg"})
	l, _ := s.Load()
	hash := l.Report.Records[x].Blobs[0].Hash
	old := filepath.Join(t.TempDir(), "old")
	if _, _, err := s.Export(old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Purge(x, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.path(BlobPathOf(hash))); err == nil {
		t.Fatal("setup: purge kept the photo")
	}
	k := reKey(t) // arrives later by sync, from any copy a stranger can write
	o := reClaim(k, k.Public(), id, "note", map[string]any{"s": "x"})
	o["blobs"] = []any{map[string]any{"hash": hash, "role": "x", "media_type": "image/jpeg"}}
	rePlant(t, s, o, k)
	res, err := s.ImportExportAll(old)
	pt, gerr := s.GetBlob(hash)
	t.Logf("import-export %+v %v; blob get after: %q %v", res, err, pt, gerr)
	if gerr == nil {
		t.Errorf("an older export brought the purged photo back, because a stranger's record names it")
	}
}

// B7: a load that restores targets writes local/purged-refs.jsonl only after
// every rename. If that write fails (crash, full disk), the targets are back
// and their lines stay: no later load restores them again, so the line stays
// for good and a re-import of the reading is skipped in silence.
func TestAttack8InterruptedRestoreKeepsPurgedRef(t *testing.T) {
	b, id := initStore(t)
	_ = id
	in := `{"type":"weight","value":{"n":81,"unit":"kg"},"times":{"observed":null},"source":{"system":"scale","adapter":"x@1","ref_input":["bean/v0/ref","id","scale","w1"]}}` + "\n"
	ar, err := b.Append(strings.NewReader(in))
	if err != nil || len(ar.IDs) != 1 {
		t.Fatal(ar, err)
	}
	x := ar.IDs[0]
	// A tombstone from a stranger's key with the person's id would be invalid.
	// Use a real one, then make it invalid: hide it by hand as another replica
	// would, then remove the tombstone (stand-in for a rotation that does not
	// seal it).
	loc := b.mustLocal(t)
	tid := rePlant(t, b, NewTombstone(id, loc.Signer.Public(), x, "", "x@1", "2026-11-06T00:00:00Z"), loc.Signer)
	l, _ := b.Load()
	if !l.Hidden[x] {
		t.Fatal("setup: not hidden")
	}
	refs, _ := b.readPurgedRefs()
	t.Logf("hidden; purged-refs lines %d", len(refs))
	os.Remove(b.path(PathOf(tid)))
	os.Remove(b.path(fileHolds))
	// The next load restores x. Make the purged-refs write fail.
	if os.Getenv("CTRL") == "" {
		os.Chmod(b.path("local"), 0o500)
	}
	_, lerr := b.load(false)
	os.Chmod(b.path("local"), 0o700)
	t.Logf("load: %v", lerr)
	l, _ = b.Load()
	refs, _ = b.readPurgedRefs()
	t.Logf("after: hidden %v, present %v, purged-refs lines %d", l.Hidden[x], l.Report.Records[x] != nil, len(refs))
	if len(refs) != 0 {
		t.Errorf("the restored reading's ref line stays for good")
	}
}
