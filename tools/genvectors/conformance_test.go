package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Every conformance folder gives the verify output in its expected.txt.
func TestConformanceFolders(t *testing.T) {
	dirs := sortedDirs(filepath.Join("..", "..", conformanceDir, "*"))
	if len(dirs) == 0 {
		t.Fatal("no conformance folders")
	}
	for _, dir := range dirs {
		want, err := os.ReadFile(filepath.Join(dir, "expected.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if got := conformanceResult(readObjects(dir)); got != string(want) {
			t.Errorf("%s:\ngot\n%s\nwant\n%s", filepath.Base(dir), got, want)
		}
	}
}
