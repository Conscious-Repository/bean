package bundle

import (
	"bytes"
	"strings"
	"testing"

	"filippo.io/age"
)

// TestUnsealRefusesACostlierHeader: bean seals with one work factor. A
// header that asks for more is refused for that, before any work is done:
// a changed header must not keep a reader's machine busy.
func TestUnsealRefusesACostlierHeader(t *testing.T) {
	words, err := NewWords()
	if err != nil {
		t.Fatal(err)
	}
	packed, err := PackFiles(map[string][]byte{"objects/aa/bb.json": []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := Seal(packed, words)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Unseal(sealed, words); err != nil || !bytes.Equal(got, packed) {
		t.Fatalf("a package does not open with its words: %v", err)
	}
	// A package of a lower cost opens: the sender chose it, and the words
	// are what protect the package. tools/second/unseal.py does the same.
	to, err := age.NewScryptRecipient(words)
	if err != nil {
		t.Fatal(err)
	}
	to.SetWorkFactor(10)
	var cheap bytes.Buffer
	w, _ := age.Encrypt(&cheap, to)
	w.Write(packed)
	w.Close()
	if got, err := Unseal(cheap.Bytes(), words); err != nil || !bytes.Equal(got, packed) {
		t.Errorf("a package of a lower cost: %v", err)
	}
	// The header's stanza line is "-> scrypt <salt> <work factor>".
	head, rest, _ := bytes.Cut(sealed[len(sealHeader):], []byte("\n"))
	if !bytes.HasSuffix(head, []byte(" 16")) {
		t.Fatalf("the header is not as expected: %q", head)
	}
	costly := append([]byte(sealHeader), bytes.TrimSuffix(head, []byte("16"))...)
	costly = append(append(append(costly, "18"...), '\n'), rest...)
	if _, err := Unseal(costly, words); err == nil || !strings.Contains(err.Error(), "work factor") {
		t.Errorf("a header that asks for more work: %v", err)
	}
}
