package core

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// RefInputNative builds the HMAC input array for a source with a native id
// (SPEC.md 4.8).
func RefInputNative(system, nativeID string, occurrence int) []any {
	return []any{"bean/v0/ref", "id", system, nativeID, float64(occurrence)}
}

// RefInputTuple builds the HMAC input array for a source without a native
// id. observed is the exact times.observed value (or nil) and value the
// exact value object of the record.
func RefInputTuple(system, typ string, observed, value any, occurrence int) []any {
	return []any{"bean/v0/ref", "tuple", system, typ, observed, value, float64(occurrence)}
}

// DedupeRef computes "hmac-sha256:" + hex HMAC-SHA-256 over the canonical
// bytes of the input array.
func DedupeRef(dedupeKey []byte, input []any) string {
	m := hmac.New(sha256.New, dedupeKey)
	m.Write(Canonical(input))
	return "hmac-sha256:" + hex.EncodeToString(m.Sum(nil))
}

func DedupeRefNative(dedupeKey []byte, system, nativeID string, occurrence int) string {
	return DedupeRef(dedupeKey, RefInputNative(system, nativeID, occurrence))
}

func DedupeRefTuple(dedupeKey []byte, system, typ string, observed, value any, occurrence int) string {
	return DedupeRef(dedupeKey, RefInputTuple(system, typ, observed, value, occurrence))
}

// IsRef reports whether s is a well-formed dedupe ref.
func IsRef(s string) bool {
	return strings.HasPrefix(s, "hmac-sha256:") && isLowerHex(s[12:], 64)
}
