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

// checkRoundTrip fails unless every value, flag, unit and reference interval
// of an accepted report is a whole token of its source line, at the column
// where the parser says it read it, and that line is printable ASCII.
func checkRoundTrip(t *testing.T, text string, rep *Report) {
	t.Helper()
	lines := splitLines(text)
	for _, r := range rep.Results {
		for i, v := range []string{r.Value, r.Flag, r.Units, r.Ref} {
			at := r.at[i]
			if v == "" {
				if at.line != 0 {
					t.Fatalf("%q: empty field %d read at %v", r.Name, i, at)
				}
				continue
			}
			if at.line < 1 || at.line > len(lines) {
				t.Fatalf("%q: %q read at no line (%v)", r.Name, v, at)
			}
			l := lines[at.line-1]
			if !printable(l) || at.col+len(v) > len(l) || l[at.col:at.col+len(v)] != v ||
				at.col > 0 && l[at.col-1] != ' ' || at.col+len(v) < len(l) && l[at.col+len(v)] != ' ' {
				t.Fatalf("%q: %q is not at line %d column %d: %q", r.Name, v, at.line, at.col, l)
			}
		}
	}
}

// Successful staged output must remain valid after Bean adds its envelope,
// and every result must be read from where the parser says it was.
func FuzzLab(f *testing.F) {
	f.Add(fixture())
	f.Add("hello\nworld\n")
	f.Add(strings.ReplaceAll(fixture(), "01/02/2025", "01/02/0000"))
	f.Add(ambiguityReport(row("Glucose", "01", "-", "", "", ""), vals("101", "High", "mg/dL", "65-99"), comment("Note:     Fasting")))
	signer := core.NewEd25519Signer(bytes.Repeat([]byte{0x42}, 32))
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 64<<10 || !utf8.ValidString(text) {
			return
		}
		rep, err := parseReport(text)
		if err != nil {
			return
		}
		checkRoundTrip(t, text, rep)
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
