package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

// Invalid UTF-8 in a text report is refused.
func TestReadTextInvalidUTF8(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.txt")
	os.WriteFile(p, []byte("Labcorp\n\xff\n"), 0o600)
	if _, err := readText(p); err == nil || !strings.Contains(err.Error(), "not UTF-8") {
		t.Errorf("err %v", err)
	}
}

// parseReport never panics, and every record it makes is canonical JSON.
func FuzzLabcorp(f *testing.F) {
	f.Add(fixture())
	f.Add("hello\nworld\n")
	f.Fuzz(func(t *testing.T, text string) {
		rep, err := parseReport(text)
		if err != nil {
			return
		}
		for _, r := range rep.records("", nil) {
			b := core.Canonical(r)
			if _, err := core.Parse(b); err != nil {
				t.Fatalf("record not parseable: %q: %v", b, err)
			}
		}
	})
}
