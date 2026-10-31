package core

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
)

var (
	ErrUnsupportedKeyAlgorithm = errors.New("unsupported key algorithm")
	ErrBadKey                  = errors.New("bad key")
	ErrBadSignature            = errors.New("bad signature")
)

// Key is a parsed tagged public key (SPEC.md 4.4).
type Key struct {
	Alg   string
	hex   string
	ed    ed25519.PublicKey
	ecdsa *ecdsa.PublicKey
	secp  *btcec.PublicKey
}

// ParseKey parses "<alg>:<lowercase hex>".
func ParseKey(tagged string) (*Key, error) {
	alg, hx, ok := strings.Cut(tagged, ":")
	if !ok {
		return nil, ErrUnsupportedKeyAlgorithm
	}
	switch alg {
	case "ed25519":
		if !isLowerHex(hx, 64) {
			return nil, ErrBadKey
		}
		b, _ := hex.DecodeString(hx)
		return &Key{Alg: alg, ed: ed25519.PublicKey(b)}, nil
	case "p256":
		if !isLowerHex(hx, 66) || (hx[:2] != "02" && hx[:2] != "03") {
			return nil, ErrBadKey
		}
		b, _ := hex.DecodeString(hx)
		x, y := elliptic.UnmarshalCompressed(elliptic.P256(), b)
		if x == nil {
			return nil, ErrBadKey
		}
		return &Key{Alg: alg, ecdsa: &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}}, nil
	case "secp256k1":
		pk, err := parseSecp256k1Key(hx)
		if err != nil {
			return nil, err
		}
		return &Key{Alg: alg, hex: hx, secp: pk}, nil
	default:
		return nil, ErrUnsupportedKeyAlgorithm
	}
}

// TagEd25519 returns the tagged form of an Ed25519 public key.
func TagEd25519(pub ed25519.PublicKey) string {
	return "ed25519:" + hex.EncodeToString(pub)
}

// Verify checks sig over the id bytes for the tagged key (SPEC.md 4.5).
// It returns bad signature on any failure, including a tag mismatch and a
// high-S P-256 signature. A secp256k1 signature is over a Nostr wrapper
// event (secp256k1.go).
func Verify(taggedKey, id, sig string) error {
	k, err := ParseKey(taggedKey)
	if err != nil {
		return err
	}
	if !IsID(id) {
		return ErrBadSignature
	}
	alg, hx, ok := strings.Cut(sig, ":")
	if !ok || alg != k.Alg {
		return ErrBadSignature
	}
	if k.Alg == "secp256k1" {
		return verifySecp256k1(k.hex, k.secp, id, hx)
	}
	if !isLowerHex(hx, 128) {
		return ErrBadSignature
	}
	raw, _ := hex.DecodeString(hx)
	msg := idRaw(id)
	switch k.Alg {
	case "ed25519":
		if !ed25519.Verify(k.ed, msg, raw) {
			return ErrBadSignature
		}
	case "p256":
		r := new(big.Int).SetBytes(raw[:32])
		s := new(big.Int).SetBytes(raw[32:])
		if s.Cmp(halfN) > 0 {
			return ErrBadSignature
		}
		h := sha256.Sum256(msg)
		if !ecdsa.Verify(k.ecdsa, h[:], r, s) {
			return ErrBadSignature
		}
	}
	return nil
}

var halfN = new(big.Int).Rsh(elliptic.P256().Params().N, 1)

// Signer signs record ids.
type Signer interface {
	Public() string
	Sign(id string) (string, error)
}

type ed25519Signer struct{ priv ed25519.PrivateKey }

// NewEd25519Signer makes a signer from a 32-byte RFC 8032 seed.
func NewEd25519Signer(seed []byte) Signer {
	return ed25519Signer{ed25519.NewKeyFromSeed(seed)}
}

func (s ed25519Signer) Public() string {
	return TagEd25519(s.priv.Public().(ed25519.PublicKey))
}

func (s ed25519Signer) Sign(id string) (string, error) {
	if !IsID(id) {
		return "", ErrBadSignature
	}
	return "ed25519:" + hex.EncodeToString(ed25519.Sign(s.priv, idRaw(id))), nil
}
