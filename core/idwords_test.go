package core

import (
	"strings"
	"testing"
)

// TestIDWords: an id reads as eight words of the list, the first 88 bits
// in order, and what is not an id reads as nothing.
func TestIDWords(t *testing.T) {
	zeros := strings.TrimSpace(strings.Repeat("abandon ", 8))
	for id, want := range map[string]string{
		"sha256:" + strings.Repeat("0", 64): zeros,
		"sha256:" + strings.Repeat("f", 64): strings.TrimSpace(strings.Repeat("zoo ", 8)),
		// 0x8000... : the first bit set gives word 1024, then zeros.
		"sha256:8" + strings.Repeat("0", 63): "length" + zeros[len("abandon"):],
		// Bit 88 is the last of the eighth word, and bit 89 is not read.
		"sha256:" + strings.Repeat("0", 21) + "1" + strings.Repeat("0", 42): zeros[:len(zeros)-len("abandon")] + "ability",
		"sha256:" + strings.Repeat("0", 22) + "f" + strings.Repeat("f", 41): zeros,
		"sha256:" + strings.Repeat("0", 63):                                 "",
		"sha256:" + strings.Repeat("G", 64):                                 "",
		strings.Repeat("0", 64):                                             "",
	} {
		if got := IDWords(id); got != want {
			t.Errorf("IDWords(%s) = %q, want %q", id, got, want)
		}
	}
}
