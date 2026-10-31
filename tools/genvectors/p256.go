package main

// A software P-256 signer for V6 only. The core verifies P-256 but does
// not write it: a P-256 key is a hardware key in a device, which signs
// itself (DECISIONS 150).

import (
	"crypto"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"math/big"
)

// newP256 makes a key from a 32-byte scalar.
func newP256(d []byte) (*ecdsa.PrivateKey, error) {
	ek, err := ecdh.P256().NewPrivateKey(d)
	if err != nil {
		return nil, errors.New("bad key")
	}
	pub := ek.PublicKey().Bytes() // 0x04 || X || Y
	k := &ecdsa.PrivateKey{D: new(big.Int).SetBytes(d)}
	k.Curve = elliptic.P256()
	k.X, k.Y = new(big.Int).SetBytes(pub[1:33]), new(big.Int).SetBytes(pub[33:])
	return k, nil
}

// p256Public is the tagged key: the SEC1 compressed point.
func p256Public(k *ecdsa.PrivateKey) string {
	return "p256:" + hex.EncodeToString(elliptic.MarshalCompressed(elliptic.P256(), k.X, k.Y))
}

// p256Sign signs the 32 id bytes: r || s with low-S (SPEC.md 4.5). A nil
// random source gives RFC 6979 nonces (Go 1.24 and later).
func p256Sign(k *ecdsa.PrivateKey, id string) string {
	raw, err := hex.DecodeString(id[len("sha256:"):])
	if err != nil || len(raw) != 32 {
		panic("bad id")
	}
	h := sha256.Sum256(raw)
	der, err := k.Sign(nil, h[:], crypto.SHA256)
	if err != nil {
		panic(err)
	}
	var rs struct{ R, S *big.Int }
	if rest, err := asn1.Unmarshal(der, &rs); err != nil || len(rest) != 0 {
		panic("bad signature")
	}
	if half := new(big.Int).Rsh(k.Curve.Params().N, 1); rs.S.Cmp(half) > 0 {
		rs.S.Sub(k.Curve.Params().N, rs.S)
	}
	out := make([]byte, 64)
	rs.R.FillBytes(out[:32])
	rs.S.FillBytes(out[32:])
	return "p256:" + hex.EncodeToString(out)
}
