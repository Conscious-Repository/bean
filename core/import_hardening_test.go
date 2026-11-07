package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncRepairsCorruptBlob(t *testing.T) {
	source, _ := initStore(t)
	blob, err := source.PutBlob([]byte("trusted medical source"))
	if err != nil {
		t.Fatal(err)
	}
	rec := map[string]any{
		"type":   "lab.source",
		"value":  map[string]any{"blob": blob},
		"times":  map[string]any{"observed": nil},
		"source": map[string]any{"system": "test", "adapter": "test@1", "ref": nil},
		"blobs":  []any{map[string]any{"hash": blob, "role": "raw-export", "media_type": "text/plain"}},
	}
	appendLines(t, source, string(Canonical(rec)))
	target := cloneLocal(t, source)

	good, err := os.ReadFile(source.path(BlobPathOf(blob)))
	if err != nil {
		t.Fatal(err)
	}
	bad := bytes.Clone(good)
	bad[len(bad)-1] ^= 0xff
	if err := os.WriteFile(target.path(BlobPathOf(blob)), bad, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := target.GetBlob(blob); err == nil {
		t.Fatal("corrupted destination blob passed verification")
	}
	res, err := target.Sync(source)
	if err != nil || res.Replaced != 1 || res.Blobs != 0 {
		t.Fatalf("repair: %+v %v", res, err)
	}
	repaired, err := os.ReadFile(target.path(BlobPathOf(blob)))
	if err != nil || !bytes.Equal(repaired, good) {
		t.Fatalf("blob not repaired: %v", err)
	}
	verifyOK(t, target)
	res, err = target.Sync(source)
	if err != nil || res.Replaced != 0 || res.Blobs != 0 {
		t.Fatalf("repeated repair not idempotent: %+v %v", res, err)
	}

	// An invalid source must never overwrite a damaged destination.
	other := cloneLocal(t, target)
	if err := os.WriteFile(other.path(BlobPathOf(blob)), bad, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source.path(BlobPathOf(blob)), bad, 0o600); err != nil {
		t.Fatal(err)
	}
	res, err = other.Sync(source)
	if err != nil || res.Replaced != 0 {
		t.Fatalf("accepted bad source: %+v %v", res, err)
	}
	left, err := os.ReadFile(other.path(BlobPathOf(blob)))
	if err != nil || !bytes.Equal(left, bad) {
		t.Fatalf("damaged source changed destination: %v", err)
	}
}

func TestImportExportRejectsUnsafeEntries(t *testing.T) {
	for _, sub := range []string{"blobs", "records"} {
		t.Run(sub, func(t *testing.T) {
			s, id := initStore(t)
			dir := t.TempDir()
			for _, d := range []string{"blobs", "records"} {
				if err := os.Mkdir(filepath.Join(dir, d), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			writeManifest(t, dir, id)
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(outside, []byte("private"), 0o600); err != nil {
				t.Fatal(err)
			}
			name := strings.Repeat("a", 64)
			if sub == "records" {
				name += ".json"
			}
			if err := os.Symlink(outside, filepath.Join(dir, sub, name)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, _, err := s.ImportExport(dir); err == nil || !strings.Contains(err.Error(), "bad path") {
				t.Fatalf("accepted symlink in %s: %v", sub, err)
			}
		})
	}
}

func TestImportExportRejectsOversizedFilesBeforeRead(t *testing.T) {
	for _, tc := range []struct {
		sub  string
		size int64
	}{
		{"blobs", maxImportBlobBytes + 1},
		{"records", MaxSuccessionSize + 1},
	} {
		t.Run(tc.sub, func(t *testing.T) {
			s, id := initStore(t)
			dir := t.TempDir()
			for _, sub := range []string{"blobs", "records"} {
				if err := os.Mkdir(filepath.Join(dir, sub), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			writeManifest(t, dir, id)
			name := strings.Repeat("a", 64)
			if tc.sub == "records" {
				name += ".json"
			}
			p := filepath.Join(dir, tc.sub, name)
			f, err := os.Create(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.Truncate(tc.size); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			if _, _, err := s.ImportExport(dir); err == nil || !strings.Contains(err.Error(), "too large") {
				t.Fatalf("accepted oversized %s: %v", tc.sub, err)
			}
		})
	}
}
