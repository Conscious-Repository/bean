package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A blob whose content does not match its name is refused before any
// file is written (SPEC 7.9).
func TestImportExportBadBlobWritesNothing(t *testing.T) {
	a, _ := initStore(t)
	x := t.TempDir()
	for _, d := range []string{"blobs", "records"} {
		if err := os.MkdirAll(filepath.Join(x, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(x, "blobs", strings.Repeat("0", 64)), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.ImportExport(x); err == nil || !strings.HasPrefix(err.Error(), "blob corrupt") {
		t.Fatalf("import: %v", err)
	}
	var n int
	filepath.WalkDir(a.path(dirBlobs), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	if n != 0 {
		t.Errorf("%d blob files left", n)
	}
}
