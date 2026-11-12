package core

import (
	"strings"
	"testing"
)

// TestIDWords: an id reads as six words of the list, the first 66 bits in
// order, and what is not an id reads as nothing.
func TestIDWords(t *testing.T) {
	for id, want := range map[string]string{
		"sha256:" + strings.Repeat("0", 64): "abandon abandon abandon abandon abandon abandon",
		"sha256:" + strings.Repeat("f", 64): "zoo zoo zoo zoo zoo zoo",
		// 0x8000... : the first bit set gives word 1024, then zeros.
		"sha256:8" + strings.Repeat("0", 63): "length abandon abandon abandon abandon abandon",
		// Bits 65 and 66 are the last two of the sixth word.
		"sha256:" + strings.Repeat("0", 16) + "c" + strings.Repeat("0", 47): "abandon abandon abandon abandon abandon about",
		"sha256:" + strings.Repeat("0", 16) + "3" + strings.Repeat("f", 47): "abandon abandon abandon abandon abandon abandon",
		"sha256:" + strings.Repeat("0", 63):                                 "",
		"sha256:" + strings.Repeat("G", 64):                                 "",
		strings.Repeat("0", 64):                                             "",
	} {
		if got := IDWords(id); got != want {
			t.Errorf("IDWords(%s) = %q, want %q", id, got, want)
		}
	}
}
