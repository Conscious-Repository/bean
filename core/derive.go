package core

import (
	"crypto/hkdf"
	"crypto/sha256"
	"strconv"
)

// HKDF labels (SPEC.md 4.7).
const (
	labelIdentitySecret = "hlog/v0/identity-secret"
	labelRecoveryKey    = "hlog/v0/recovery-key/" // + generation
	labelBlobKey        = "hlog/v0/blob-key"
	labelDedupeKey      = "hlog/v0/dedupe-key"
)

func hkdfSHA256(ikm, salt []byte, info string, n int) ([]byte, error) {
	return hkdf.Key(sha256.New, ikm, salt, info, n)
}

func derive(ikm []byte, info string, n int) []byte {
	out, err := hkdfSHA256(ikm, nil, info, n)
	if err != nil {
		panic("hkdf: " + err.Error())
	}
	return out
}

// IdentitySecret derives the 32-byte identity secret from the paper seed.
func IdentitySecret(seed []byte) []byte { return derive(seed, labelIdentitySecret, 32) }

// RecoverySeed derives the Ed25519 private seed of recovery generation g.
func RecoverySeed(seed []byte, g int) []byte {
	return derive(seed, labelRecoveryKey+strconv.Itoa(g), 32)
}

// RecoverySigner returns the recovery signer of generation g.
func RecoverySigner(seed []byte, g int) Signer {
	return NewEd25519Signer(RecoverySeed(seed, g))
}

// BlobKey derives the 64-byte AES-SIV key from the identity secret.
func BlobKey(identitySecret []byte) []byte { return derive(identitySecret, labelBlobKey, 64) }

// DedupeKey derives the 32-byte HMAC key from the identity secret.
func DedupeKey(identitySecret []byte) []byte { return derive(identitySecret, labelDedupeKey, 32) }
