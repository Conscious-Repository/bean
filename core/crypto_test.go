package core

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

func h(s string) []byte {
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		panic(err)
	}
	return b
}

// RFC 4493 section 4 test vectors.
func TestCMACRFC4493(t *testing.T) {
	key := h("2b7e1516 28aed2a6 abf71588 09cf4f3c")
	m := h("6bc1bee2 2e409f96 e93d7e11 7393172a ae2d8a57 1e03ac9c 9eb76fac 45af8e51 30c81c46 a35ce411 e5fbc119 1a0a52ef f69f2445 df4f9b17 ad2b417b e66c3710")
	cases := []struct {
		n    int
		want string
	}{
		{0, "bb1d6929 e9593728 7fa37d12 9b756746"},
		{16, "070a16b4 6b4d4144 f79bdd9d d04a287c"},
		{40, "dfa66747 de9ae630 30ca3261 1497c827"},
		{64, "51f0bebf 7e3b9d92 fc497417 79363cfe"},
	}
	c, err := newCMAC(key)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(c.k1, h("fbeed618 35713366 7c85e08f 7236a8de")) || !bytes.Equal(c.k2, h("f7ddac30 6ae266cc f90bc11e e46d513b")) {
		t.Errorf("subkeys wrong: k1=%x k2=%x", c.k1, c.k2)
	}
	for _, tc := range cases {
		got := c.mac(m[:tc.n])
		if !bytes.Equal(got, h(tc.want)) {
			t.Errorf("len %d: got %x want %s", tc.n, got, tc.want)
		}
	}
}

// RFC 5297 Appendix A.1 (deterministic) and A.2 (nonce-based).
func TestAESSIVRFC5297(t *testing.T) {
	// A.1
	key := h("fffefdfc fbfaf9f8 f7f6f5f4 f3f2f1f0 f0f1f2f3 f4f5f6f7 f8f9fafb fcfdfeff")
	ad := h("10111213 14151617 18191a1b 1c1d1e1f 20212223 24252627")
	pt := h("11223344 55667788 99aabbcc ddee")
	want := h("85632d07 c6e8f37f 950acd32 0a2ecc93 40c02b96 90c4dc04 daef7f6a fe5c")
	got, err := sivEncrypt(key, [][]byte{ad}, pt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("A.1: got %x want %x", got, want)
	}
	back, err := sivDecrypt(key, [][]byte{ad}, got)
	if err != nil || !bytes.Equal(back, pt) {
		t.Errorf("A.1 decrypt: %x %v", back, err)
	}
	got[len(got)-1] ^= 1
	if _, err := sivDecrypt(key, [][]byte{ad}, got); err == nil {
		t.Error("A.1: tampered ciphertext accepted")
	}

	// A.2
	key = h("7f7e7d7c 7b7a7978 77767574 73727170 40414243 44454647 48494a4b 4c4d4e4f")
	ad1 := h("00112233 44556677 8899aabb ccddeeff deaddada deaddada ffeeddcc bbaa9988 77665544 33221100")
	ad2 := h("10203040 50607080 90a0")
	nonce := h("09f91102 9d74e35b d84156c5 635688c0")
	pt = h("74686973 20697320 736f6d65 20706c61 696e7465 78742074 6f20656e 63727970 74207573 696e6720 5349562d 414553")
	want = h("7bdb6e3b 432667eb 06f4d14b ff2fbd0f cb900f2f ddbe4043 26601965 c889bf17 dba77ceb 094fa663 b7a3f748 ba8af829 ea64ad54 4a272e9c 485b62a3 fd5c0d")
	got, err = sivEncrypt(key, [][]byte{ad1, ad2, nonce}, pt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("A.2: got %x want %x", got, want)
	}
}

// RFC 5869 Appendix A test cases 1 to 3.
func TestHKDFRFC5869(t *testing.T) {
	cases := []struct {
		ikm, salt, info, okm string
		l                    int
	}{
		{
			"0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b",
			"000102030405060708090a0b0c",
			"f0f1f2f3f4f5f6f7f8f9",
			"3cb25f25faacd57a90434f64d0362f2a2d2d0a90cf1a5a4c5db02d56ecc4c5bf34007208d5b887185865",
			42,
		},
		{
			"000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f404142434445464748494a4b4c4d4e4f",
			"606162636465666768696a6b6c6d6e6f707172737475767778797a7b7c7d7e7f808182838485868788898a8b8c8d8e8f909192939495969798999a9b9c9d9e9fa0a1a2a3a4a5a6a7a8a9aaabacadaeaf",
			"b0b1b2b3b4b5b6b7b8b9babbbcbdbebfc0c1c2c3c4c5c6c7c8c9cacbcccdcecfd0d1d2d3d4d5d6d7d8d9dadbdcdddedfe0e1e2e3e4e5e6e7e8e9eaebecedeeeff0f1f2f3f4f5f6f7f8f9fafbfcfdfeff",
			"b11e398dc80327a1c8e7f78c596a49344f012eda2d4efad8a050cc4c19afa97c59045a99cac7827271cb41c65e590e09da3275600c2f09b8367793a9aca3db71cc30c58179ec3e87c14c01d5c1f3434f1d87",
			82,
		},
		{
			"0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b",
			"",
			"",
			"8da4e775a563c18f715f802a063c5a31b8a11f5c5ee1879ec3454e5f3c738d2d9d201395faa4b61a96c8",
			42,
		},
	}
	for i, c := range cases {
		got, err := hkdfSHA256(h(c.ikm), h(c.salt), string(h(c.info)), c.l)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, h(c.okm)) {
			t.Errorf("case %d: got %x want %s", i+1, got, c.okm)
		}
	}
}

// BIP-39 vector from the reference test set (Trezor python-mnemonic).
func TestBIP39Words(t *testing.T) {
	entropy := h("f585c11aec520db57dd353c69554b21a89b20fb0650966fa0a9d6f74fd989d8f")
	want := "void come effort suffer camp survey warrior heavy shoot primary clutch crush open amazing screen patrol group space point ten exist slush involve unfold"
	got, err := SeedToWords(entropy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, " ") != want {
		t.Errorf("got %q", strings.Join(got, " "))
	}
	back, err := WordsToSeed(got)
	if err != nil || !bytes.Equal(back, entropy) {
		t.Errorf("round trip: %x %v", back, err)
	}
	got[23] = "zoo"
	if _, err := WordsToSeed(got); err == nil {
		t.Error("bad checksum accepted")
	}
	zero := make([]byte, 32)
	w, _ := SeedToWords(zero)
	wantZero := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon art"
	if strings.Join(w, " ") != wantZero {
		t.Errorf("zero entropy: got %q", strings.Join(w, " "))
	}
}
