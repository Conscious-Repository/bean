package core

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
)

func TestRecordID(t *testing.T) {
	v, err := Parse([]byte(`{"v":0,"type":"x.y","id":"sha256:00","sig":"ed25519:00","nonce":"00000000000000000000000000000000"}`))
	if err != nil {
		t.Fatal(err)
	}
	body := IDBytes(v)
	if string(body) != `{"nonce":"00000000000000000000000000000000","type":"x.y","v":0}` {
		t.Errorf("id bytes: %s", body)
	}
	id := RecordID(v)
	sum := sha256.Sum256(body)
	if id != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Errorf("id %s", id)
	}
	if !IsID(id) || IsID("sha256:"+strings.ToUpper(id[7:])) || IsID("sha256:00") {
		t.Error("IsID")
	}
}

func TestNonce(t *testing.T) {
	a, b := NewNonce(), NewNonce()
	if len(a) != 32 || a == b || strings.ToLower(a) != a {
		t.Errorf("nonce %s %s", a, b)
	}
}

func TestTaggedKeys(t *testing.T) {
	seed := make([]byte, 32)
	seed[0] = 1
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	tk := TagEd25519(pub)
	if !strings.HasPrefix(tk, "ed25519:") || len(tk) != 8+64 {
		t.Errorf("tagged %s", tk)
	}
	if _, err := ParseKey(tk); err != nil {
		t.Error(err)
	}
	bad := []struct{ k, want string }{
		{"ed25519:" + strings.ToUpper(tk[8:]), "bad key"},
		{tk[:len(tk)-2], "bad key"},
		{"x25519:" + tk[8:], "unsupported key algorithm"},
		{"secp256k1:" + strings.Repeat("0", 64), "bad key"},       // x = 0 is not on the curve
		{"secp256k1:" + strings.Repeat("0", 63) + "5", "bad key"}, // nor is x = 5
		{"secp256k1:" + strings.Repeat("f", 64), "bad key"},       // x above the field size
		{"secp256k1:" + strings.Repeat("0", 63) + "1", ""},        // x = 1 is on the curve
		{"secp256k1:" + strings.ToUpper(strings.Repeat("0", 63)+"a"), "bad key"},
		{"secp256k1:" + strings.Repeat("0", 62), "bad key"},
		{"rsa:00", "unsupported key algorithm"},
		{"p256:" + strings.Repeat("00", 33), "bad key"},
		{"p256:" + strings.Repeat("00", 32), "bad key"},
	}
	for _, b := range bad {
		_, err := ParseKey(b.k)
		if b.want == "" {
			if err != nil {
				t.Errorf("%s: %v", b.k, err)
			}
			continue
		}
		if err == nil || err.Error() != b.want {
			t.Errorf("%s: got %v want %s", b.k, err, b.want)
		}
	}
}

func TestEd25519SignVerify(t *testing.T) {
	seed := make([]byte, 32)
	seed[31] = 7
	s := NewEd25519Signer(seed)
	id := RecordID(mustParse(`{"a":1}`))
	sig, err := s.Sign(id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sig, "ed25519:") || len(sig) != 8+128 {
		t.Errorf("sig %s", sig)
	}
	if err := Verify(s.Public(), id, sig); err != nil {
		t.Error(err)
	}
	// Tag mismatch between kid and sig.
	if err := Verify(s.Public(), id, "p256:"+sig[8:]); err == nil || err.Error() != "bad signature" {
		t.Errorf("tag mismatch: %v", err)
	}
	// Wrong id.
	if err := Verify(s.Public(), RecordID(mustParse(`{"a":2}`)), sig); err == nil {
		t.Error("wrong id accepted")
	}
}

func TestP256LowS(t *testing.T) {
	d := make([]byte, 32)
	d[31] = 9
	s, err := NewP256Signer(d)
	if err != nil {
		t.Fatal(err)
	}
	id := RecordID(mustParse(`{"a":1}`))
	sig, err := s.Sign(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(s.Public(), id, sig); err != nil {
		t.Error(err)
	}
	// Build the high-S twin: s' = n - s. It is a valid ECDSA signature but
	// MUST be rejected (SPEC 4.5).
	raw, _ := hex.DecodeString(sig[5:])
	n := elliptic.P256().Params().N
	sv := new(big.Int).SetBytes(raw[32:])
	hs := new(big.Int).Sub(n, sv)
	hb := make([]byte, 32)
	hs.FillBytes(hb)
	high := "p256:" + hex.EncodeToString(append(raw[:32], hb...))
	// Sanity: standard ECDSA accepts the high-S twin.
	pub := s.key.Public().(*ecdsa.PublicKey)
	msg := sha256.Sum256(idRaw(id))
	if !ecdsa.Verify(pub, msg[:], new(big.Int).SetBytes(raw[:32]), hs) {
		t.Fatal("high-S twin is not a valid ECDSA signature; test is wrong")
	}
	if err := Verify(s.Public(), id, high); err == nil || err.Error() != "bad signature" {
		t.Errorf("high-S accepted: %v", err)
	}
}

func TestCommitment(t *testing.T) {
	k := "ed25519:" + strings.Repeat("ab", 32)
	sum := sha256.Sum256([]byte(k))
	if Commitment(k) != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Error("commitment")
	}
}

func TestDedupeRef(t *testing.T) {
	key := make([]byte, 32)
	a := DedupeRefNative(key, "oura", "12345", 0)
	b := DedupeRefNative(key, "oura", "12345", 1)
	c := DedupeRefNative(key, "oura", "12345", 0)
	if a == b || a != c || !strings.HasPrefix(a, "hmac-sha256:") || len(a) != 12+64 {
		t.Errorf("%s %s %s", a, b, c)
	}
	obs := mustParse(`{"start":"2026-01-01T00:00:00Z","precision":"day"}`)
	val := mustParse(`{"n":42,"unit":"ms"}`)
	d := DedupeRefTuple(key, "csv", "hrv.rmssd", obs, val, 0)
	e := DedupeRefTuple(key, "csv", "hrv.rmssd", nil, val, 0)
	if d == e {
		t.Error("observed time ignored")
	}
	if in := RefInputNative("oura", "12345", 0); string(Canonical(in)) != `["bean/v0/ref","id","oura","12345",0]` {
		t.Errorf("input %s", Canonical(in))
	}
}

func mustParse(s string) any {
	v, err := Parse([]byte(s))
	if err != nil {
		panic(err)
	}
	return v
}
