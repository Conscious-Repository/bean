package core

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

// The small-order and non-canonical encodings that libsodium refuses as
// keys, and real keys that it takes (N2).
func TestEdStrictKey(t *testing.T) {
	for _, hx := range []string{
		"0100000000000000000000000000000000000000000000000000000000000000", // neutral point
		"0100000000000000000000000000000000000000000000000000000000000080", // neutral, sign bit set
		"ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f", // order 2
		"0000000000000000000000000000000000000000000000000000000000000000", // order 4
		"0000000000000000000000000000000000000000000000000000000000000080", // order 4
		"26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05", // order 8
		"c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac037a", // order 8
		"edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f", // y = p
		"eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f", // y = p + 1
		"0200000000000000000000000000000000000000000000000000000000000000", // no point
	} {
		b, _ := hex.DecodeString(hx)
		if edStrictKey(b) {
			t.Errorf("accepted %s", hx)
		}
		if _, err := ParseKey("ed25519:" + hx); err != ErrBadKey {
			t.Errorf("%s: %v", hx, err)
		}
	}
	for i := range 200 {
		seed := make([]byte, 32)
		seed[0], seed[1] = byte(i), byte(i>>8)
		pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
		if !edStrictKey(pub) {
			t.Fatalf("refused real key %x", pub)
		}
	}
}

// A signature whose R is of small order is bad even where crypto/ed25519
// takes it: the universal signature of V23 under a key that is a point.
func TestEdSmallOrderR(t *testing.T) {
	for _, hx := range []string{
		"0100000000000000000000000000000000000000000000000000000000000000",
		"26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05",
	} {
		b, _ := hex.DecodeString(hx)
		if !edSmallOrderR(b) {
			t.Errorf("not small order: %s", hx)
		}
	}
	s := NewEd25519Signer(make([]byte, 32))
	id := HashID([]byte("x"))
	sig, err := s.Sign(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(s.Public(), id, sig); err != nil {
		t.Errorf("good signature: %v", err)
	}
}

// The y list of edSmallOrderR agrees with the arithmetic: every encoding
// whose point has small order is listed, and no other point.
func TestEdSmallOrderRMatchesArithmetic(t *testing.T) {
	for _, hx := range []string{
		"0100000000000000000000000000000000000000000000000000000000000000",
		"ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f",
		"0000000000000000000000000000000000000000000000000000000000000080",
		"26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc85",
		"c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac037a",
		"edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f",
		"eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	} {
		b, _ := hex.DecodeString(hx)
		if !edSmallOrderR(b) {
			t.Errorf("missed %s", hx)
		}
	}
	for i := range 300 {
		seed := make([]byte, 32)
		seed[0], seed[1] = byte(i), byte(i>>8)
		pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
		x, y, _ := edDecode(pub)
		if edSmallOrderR(pub) != edSmallOrder(x, y) {
			t.Fatalf("disagree on %x", pub)
		}
	}
}
