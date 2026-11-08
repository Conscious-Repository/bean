package core

import (
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"
)

// badIDPath names no file a store ever writes. PathOf and BlobPathOf
// return it for a string that is not an id, so a bad id read from input
// is "not found" instead of a panic or a path outside the shard.
const badIDPath = "bad-id"

// PathOf returns the objects/ path of a record id (SPEC.md 7.1).
func PathOf(id string) string {
	if !IsID(id) {
		return "objects/" + badIDPath
	}
	h := id[7:]
	return "objects/" + h[:2] + "/" + h[2:] + ".json"
}

// BlobPathOf returns the blobs/ path of a blob id.
func BlobPathOf(id string) string {
	if !IsID(id) {
		return "blobs/" + badIDPath
	}
	h := id[7:]
	return "blobs/" + h[:2] + "/" + h[2:]
}

// SignRecord fills id and sig on a record object (SPEC.md 7.2 steps 1 to 4)
// and returns the id and the canonical file bytes. The signer's public key
// MUST already be in kid.
func SignRecord(obj map[string]any, s Signer) (string, []byte, error) {
	delete(obj, "id")
	delete(obj, "sig")
	// Canonical would write invalid UTF-8 as U+FFFD and has no form for
	// NaN or Inf, so the stored record would differ from what the caller
	// gave. Refuse instead.
	if err := CheckValue(obj); err != nil {
		return "", nil, err
	}
	id := RecordID(obj)
	sig, err := s.Sign(id)
	if err != nil {
		return "", nil, err
	}
	obj["id"], obj["sig"] = id, sig
	return id, Canonical(obj), nil
}

// NewGenesis builds an unsigned genesis object (SPEC.md 3.3, 5.2 step 5).
func NewGenesis(signingKey, recoveryCommitment, adapter, imported string) map[string]any {
	return map[string]any{
		"v": 0.0, "nonce": NewNonce(), "type": "bean.genesis",
		"author": signingKey, "kid": signingKey,
		"value":  map[string]any{"key": signingKey, "recovery": recoveryCommitment},
		"times":  map[string]any{"observed": nil, "imported": imported},
		"source": map[string]any{"system": "self", "adapter": adapter, "ref": nil},
	}
}

// NewTombstone builds an unsigned tombstone (SPEC.md 3.3).
func NewTombstone(identity, kid, target, reason, adapter, imported string) map[string]any {
	t := map[string]any{
		"v": 0.0, "nonce": NewNonce(), "type": "bean.tombstone",
		"subject": identity, "author": identity, "kid": kid,
		"refs":   []any{map[string]any{"rel": "purges", "id": target}},
		"times":  map[string]any{"observed": nil, "imported": imported},
		"source": map[string]any{"system": "self", "adapter": adapter, "ref": nil},
	}
	if reason != "" {
		t["context"] = map[string]any{"reason": reason}
	}
	return t
}

// NewSuccession builds an unsigned succession (SPEC.md 3.3, 5.5 step 6).
// kid is the recovery public key that will sign it.
func NewSuccession(identity, kid, parent, newKey, newCommitment string, sealed []string, adapter, imported string) map[string]any {
	s := make([]any, len(sealed))
	for i, x := range sealed {
		s[i] = x
	}
	return map[string]any{
		"v": 0.0, "nonce": NewNonce(), "type": "bean.succession",
		"subject": identity, "author": identity, "kid": kid,
		"refs":   []any{map[string]any{"rel": "succeeds", "id": parent}},
		"value":  map[string]any{"key": newKey, "recovery": newCommitment, "sealed": s},
		"times":  map[string]any{"observed": nil, "imported": imported},
		"source": map[string]any{"system": "self", "adapter": adapter, "ref": nil},
	}
}

// NewSeal builds an unsigned seal continuation (C28): part k of the seal
// set of succession sid, signed by the same recovery key (kid).
func NewSeal(identity, kid, sid string, part int, sealed []string, adapter, imported string) map[string]any {
	ids := make([]any, len(sealed))
	for i, x := range sealed {
		ids[i] = x
	}
	return map[string]any{
		"v": 0.0, "nonce": NewNonce(), "type": "bean.seal",
		"subject": identity, "author": identity, "kid": kid,
		"refs":   []any{map[string]any{"rel": "seals", "id": sid}},
		"value":  map[string]any{"part": float64(part), "sealed": ids},
		"times":  map[string]any{"observed": nil, "imported": imported},
		"source": map[string]any{"system": "self", "adapter": adapter, "ref": nil},
	}
}

// IsCoreType reports whether a type is reserved for the core.
func IsCoreType(t string) bool { return strings.HasPrefix(t, "bean.") }

// CheckValue reports a string that is not valid UTF-8 (ErrBadString) or a
// number that JSON cannot carry exactly (ErrBadNumber) anywhere in v.
func CheckValue(v any) error {
	switch x := v.(type) {
	case string:
		if !utf8.ValidString(x) {
			return ErrBadString
		}
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) || !numberInRange(x) {
			return ErrBadNumber
		}
	case []any:
		for _, e := range x {
			if err := CheckValue(e); err != nil {
				return err
			}
		}
	case map[string]any:
		for k, e := range x {
			if !utf8.ValidString(k) {
				return ErrBadString
			}
			if err := CheckValue(e); err != nil {
				return err
			}
		}
	}
	return nil
}

// ExactNumber reports whether f can be a JSON number in a record: finite,
// and an integer value only up to 2^53 in magnitude (SPEC 4.1). Importers
// keep any other number as text.
func ExactNumber(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0) && numberInRange(f)
}

// SameDecimal reports whether n, written as canonical JSON, has the same
// decimal value as raw, the number as a source printed it. A source's
// "0.30000000000000001" or "1e-400" reads as a float64 that prints as another
// decimal; an importer refuses it, or keeps raw as text, rather than store a
// number the source did not write. raw is at most 256 bytes, with an exponent
// of at most 1000 either way, so the comparison is bounded.
func SameDecimal(raw string, n float64) bool {
	if len(raw) > 256 {
		return false
	}
	if i := strings.IndexAny(raw, "eE"); i >= 0 {
		exponent, err := strconv.ParseInt(raw[i+1:], 10, 32)
		if err != nil || exponent < -1000 || exponent > 1000 {
			return false
		}
	}
	source, ok := new(big.Rat).SetString(raw)
	if !ok {
		return false
	}
	emitted, ok := new(big.Rat).SetString(string(Canonical(n)))
	return ok && source.Cmp(emitted) == 0
}
