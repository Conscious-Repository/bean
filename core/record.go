package core

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// IDBytes returns the canonical bytes of the record without id and sig
// (SPEC.md 4.2 steps 1 to 3). The input is not changed.
func IDBytes(record any) []byte {
	m, ok := record.(map[string]any)
	if !ok {
		return Canonical(record)
	}
	copy := make(map[string]any, len(m))
	for k, v := range m {
		if k != "id" && k != "sig" {
			copy[k] = v
		}
	}
	return Canonical(copy)
}

// RecordID computes the id of a record (SPEC.md 4.2).
func RecordID(record any) string {
	return HashID(IDBytes(record))
}

// HashID returns "sha256:" + hex SHA-256 of data.
func HashID(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// IsID reports whether s is "sha256:" followed by 64 lowercase hex digits.
func IsID(s string) bool {
	return strings.HasPrefix(s, "sha256:") && isLowerHex(s[7:], 64)
}

func isLowerHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// idRaw returns the 32 hash bytes of an id. The caller checks IsID first.
func idRaw(id string) []byte {
	b, _ := hex.DecodeString(id[7:])
	return b
}

// NewNonce returns 16 random bytes as 32 lowercase hex digits (SPEC.md 4.3).
func NewNonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// Commitment is the id-form SHA-256 of the ASCII tagged key string
// (SPEC.md 4.6).
func Commitment(taggedKey string) string {
	return HashID([]byte(taggedKey))
}
