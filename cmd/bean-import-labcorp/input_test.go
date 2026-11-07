package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

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

// Successful staged output must remain valid after Bean adds its envelope.
func FuzzLabcorp(f *testing.F) {
	f.Add(fixture())
	f.Add("hello\nworld\n")
	f.Add(strings.ReplaceAll(fixture(), "01/02/2025", "01/02/0000"))
	signer := core.NewEd25519Signer(bytes.Repeat([]byte{0x42}, 32))
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 64<<10 || !utf8.ValidString(text) {
			return
		}
		rep, err := parseReport(text)
		if err != nil {
			return
		}
		rows := rep.records("", nil)
		var staged bytes.Buffer
		if err := writeRecords(&staged, rows); err != nil {
			if staged.Len() != 0 {
				t.Fatal("rejected output left an importable prefix")
			}
			return
		}
		for _, r := range rows {
			b := core.Canonical(r)
			if _, err := core.Parse(b); err != nil {
				t.Fatalf("record not parseable: %v", err)
			}
			if err := checkSignedLabPartial(r, signer); err != nil {
				t.Fatalf("accepted reading fails signed field verification: %v", err)
			}
		}
	})
}
