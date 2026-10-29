package core

import "strings"

// PathOf returns the objects/ path of a record id (SPEC.md 7.1).
func PathOf(id string) string {
	h := id[7:]
	return "objects/" + h[:2] + "/" + h[2:] + ".json"
}

// BlobPathOf returns the blobs/ path of a blob id.
func BlobPathOf(id string) string {
	h := id[7:]
	return "blobs/" + h[:2] + "/" + h[2:]
}

// SignRecord fills id and sig on a record object (SPEC.md 7.2 steps 1 to 4)
// and returns the id and the canonical file bytes. The signer's public key
// MUST already be in kid.
func SignRecord(obj map[string]any, s Signer) (string, []byte, error) {
	delete(obj, "id")
	delete(obj, "sig")
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

// IsCoreType reports whether a type is reserved for the core.
func IsCoreType(t string) bool { return strings.HasPrefix(t, "bean.") }
