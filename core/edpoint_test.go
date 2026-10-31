package core

import (
	"crypto/ed25519"
	"encoding/hex"
	"math/big"
	"math/rand"
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
		if edSmallOrderR(pub) != refSmallOrder(x, y) {
			t.Fatalf("disagree on %x", pub)
		}
	}
}

// refSmallOrder is the arithmetic that the y table replaces: [8]P is the
// neutral point, by three doublings with the complete formula for
// -x^2 + y^2 = 1 + d x^2 y^2.
func refSmallOrder(x, y *big.Int) bool {
	for range 3 {
		xy := edMul(x, y)
		t := edMul(edD, edMul(xy, xy))
		x2 := edMul(edMod(new(big.Int).Add(xy, xy)), edInv(edMod(new(big.Int).Add(big.NewInt(1), t))))
		y2 := edMul(edMod(new(big.Int).Add(edMul(y, y), edMul(x, x))), edInv(edMod(new(big.Int).Sub(big.NewInt(1), t))))
		x, y = x2, y2
	}
	return x.Sign() == 0 && y.Cmp(big.NewInt(1)) == 0
}

// edKeyCases is the agreement set: the libsodium blocklist with both sign
// bits, every y from p to 2^255 - 1 and the 64 highest y values with both
// signs, 20,000 random strings (about half are points, most of mixed
// order), and 2,000 real keys with their sign bit as is and flipped.
func edKeyCases() [][]byte {
	var out [][]byte
	for _, h := range []string{
		"0000000000000000000000000000000000000000000000000000000000000000",
		"0100000000000000000000000000000000000000000000000000000000000000",
		"26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05",
		"c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac037a",
		"ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f",
		"edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f",
		"eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f",
	} {
		for _, sign := range []byte{0, 0x80} {
			b, _ := hex.DecodeString(h)
			b[31] |= sign
			out = append(out, b)
		}
	}
	for i := range 64 {
		for _, sign := range []byte{0, 0x80} {
			b := make([]byte, 32)
			for j := range b {
				b[j] = 0xff
			}
			b[0], b[31] = byte(0xff-i), 0x7f|sign
			out = append(out, b)
		}
	}
	r := rand.New(rand.NewSource(1))
	for range 20000 {
		b := make([]byte, 32)
		r.Read(b)
		out = append(out, b)
	}
	for range 2000 {
		seed := make([]byte, 32)
		r.Read(seed)
		pub := []byte(ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey))
		flip := append([]byte{}, pub...)
		flip[31] ^= 0x80
		out = append(out, pub, flip)
	}
	return out
}

// The y table gives the same key verdict as the arithmetic on every case
// of edKeyCases (DECISIONS 170).
func TestEdSmallYMatchesArithmetic(t *testing.T) {
	cases := edKeyCases()
	if len(cases) < 24000 {
		t.Fatalf("%d cases", len(cases))
	}
	accepted := 0
	for _, b := range cases {
		x, y, ok := edDecode(b)
		want := ok && !refSmallOrder(x, y)
		if got := edStrictKey(b); got != want {
			t.Fatalf("%x: table %v, arithmetic %v", b, got, want)
		}
		if want {
			accepted++
		}
	}
	t.Logf("%d cases, %d keys accepted", len(cases), accepted)
}
