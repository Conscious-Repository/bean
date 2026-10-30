package core

import (
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

// The library itself against BIP-340 test vector 0.
func TestBIP340Vector0(t *testing.T) {
	sk := make([]byte, 32)
	sk[31] = 3
	priv, _ := btcec.PrivKeyFromBytes(sk)
	var aux [32]byte
	sig, err := schnorr.Sign(priv, make([]byte, 32), schnorr.CustomNonce(aux))
	if err != nil {
		t.Fatal(err)
	}
	want := "e907831f80848d1069a5371b402410364bdf1c5f8307b0084c55f1ce2dca821525f66a4a85ea8b71e482a74f382d2ce5ebeee8fdb2172f477df4900d310536c0"
	if got := hex.EncodeToString(sig.Serialize()); got != want {
		t.Errorf("sig %s", got)
	}
}

// NIP-19 examples.
func TestBech32(t *testing.T) {
	pub, _ := hex.DecodeString("7e7e9c42a91bfef19fa929e5fda1b72e0ebc1a4c1141673e2794234d86addf4e")
	if got := Bech32Encode("npub", pub); got != "npub10elfcs4fr0l0r8af98jlmgdh9c8tcxjvz9qkw038js35mp4dma8qzvjptg" {
		t.Errorf("npub %s", got)
	}
	nsec := "nsec1vl029mgpspedva04g90vltkh6fvh240zqtv9k0t9af8935ke9laqsnlfe5"
	sec, err := ParseNsec(nsec)
	if err != nil || hex.EncodeToString(sec) != "67dea2ed018072d675f5415ecfaed7d2597555e202d85b3d65ea4e58d2d92ffa" {
		t.Errorf("nsec %x %v", sec, err)
	}
	if sec2, err := ParseNsec("  " + strings.ToUpper(nsec) + "\n"); err != nil || !bytes.Equal(sec, sec2) {
		t.Errorf("uppercase nsec: %v", err)
	}
	if sec3, err := ParseNsec(hex.EncodeToString(sec)); err != nil || !bytes.Equal(sec, sec3) {
		t.Errorf("hex nsec: %v", err)
	}
	for _, bad := range []string{
		"npub10elfcs4fr0l0r8af98jlmgdh9c8tcxjvz9qkw038js35mp4dma8qzvjptg", // an npub, not an nsec
		nsec[:len(nsec)-1] + "6", // checksum
		"nsec1VL029mgpspedva04g90vltkh6fvh240zqtv9k0t9af8935ke9laqsnlfe5", // mixed case
		"", "nsec1", strings.Repeat("a", 63),
	} {
		if _, err := ParseNsec(bad); err == nil {
			t.Errorf("ParseNsec(%q) accepted", bad)
		}
	}
}

func TestSecp256k1Signatures(t *testing.T) {
	sec, _ := hex.DecodeString("67dea2ed018072d675f5415ecfaed7d2597555e202d85b3d65ea4e58d2d92ffa")
	s, err := NewSecp256k1Signer(sec)
	if err != nil {
		t.Fatal(err)
	}
	if Npub(s.Public()) == "" || !strings.HasPrefix(s.Public(), "secp256k1:") {
		t.Errorf("public %s", s.Public())
	}
	id := HashID([]byte("a record"))
	sig, err := s.Sign(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(s.Public(), id, sig); err != nil {
		t.Errorf("own signature: %v", err)
	}
	if err := Verify(s.Public(), HashID([]byte("another")), sig); err != ErrBadSignature {
		t.Errorf("other id: %v", err)
	}
	if err := Verify(TagEd25519(make([]byte, 32)), id, sig); err != ErrBadSignature {
		t.Errorf("tag mismatch: %v", err)
	}
	hx := sig[strings.LastIndexByte(sig, ':')+1:]
	for _, bad := range []string{
		"secp256k1:" + hx, "secp256k1:01:" + hx, "secp256k1:-1:" + hx, "secp256k1:9007199254740992:" + hx,
		"secp256k1:0:" + strings.ToUpper(hx), "secp256k1:0:" + hx[:126], "secp256k1:1:" + hx,
	} {
		if err := Verify(s.Public(), id, bad); err != ErrBadSignature {
			t.Errorf("%s: %v", bad[:16], err)
		}
	}
	// A wrapper signed at another created_at, as a remote signer may do.
	var aux [32]byte
	other, _ := SignSecp256k1With(sec, id, 1761566400, aux)
	if err := Verify(s.Public(), id, other); err != nil {
		t.Errorf("created_at 1761566400: %v", err)
	}
	for _, k := range [][]byte{make([]byte, 32), bytes.Repeat([]byte{0xff}, 32), sec[:31]} {
		if _, err := NewSecp256k1Signer(k); err == nil {
			t.Errorf("secret %x accepted", k)
		}
	}
}

// A whole log signed by an npub: init, claims, purge, rotation to a new
// npub, refusal to rotate back to an old key, and restore with an nsec.
func TestNostrLog(t *testing.T) {
	sec, _ := hex.DecodeString("67dea2ed018072d675f5415ecfaed7d2597555e202d85b3d65ea4e58d2d92ffa")
	s := newStore(t)
	id, words, err := s.Init(NewKey{Alg: "secp256k1", Secret: sec})
	if err != nil {
		t.Fatal(err)
	}
	first := s.mustLocal(t).Signer.Public()
	if !strings.HasPrefix(first, "secp256k1:") {
		t.Fatalf("signing key %s", first)
	}
	if b, _ := os.ReadFile(s.path(fileSignKey)); !strings.HasPrefix(string(b), "secp256k1:67dea2ed") {
		t.Errorf("local/signing-key = %q", b)
	}
	ids := appendLines(t, s, partial("hrv.rmssd", 1, nil), partial("hrv.rmssd", 2, nil)).IDs
	if _, err := s.Purge(ids[1], ""); err != nil {
		t.Fatal(err)
	}
	l := verifyOK(t, s)
	if l.Report.Records[id].Kid != first || l.Report.Records[ids[0]].Kid != first {
		t.Error("records not signed by the npub")
	}

	// Rotate to a new random npub; the purge by the old npub is sealed.
	res, err := s.Rotate(words, nil, NewKey{Alg: "secp256k1"})
	if err != nil {
		t.Fatal(err)
	}
	// C26: both claims (ids[1] is purged, and sealed in case its tombstone
	// dies) and the tombstone are sealed.
	if !strings.HasPrefix(res.NewKey, "secp256k1:") || res.NewKey == first || len(res.Sealed) != 3 {
		t.Errorf("rotate %+v", res)
	}
	appendLines(t, s, partial("hrv.rmssd", 3, nil))
	verifyOK(t, s)
	if _, err := s.Rotate(words, nil, NewKey{Alg: "secp256k1", Secret: sec}); err == nil || err.Error() != "key reuse" {
		t.Errorf("rotate back to the first npub: %v", err)
	}

	// Lost laptop. The first npub is on the chain, so it cannot come back
	// (5.8): its nsec sat on the lost machine. Restore moves to a new key,
	// and the npub-signed history still verifies.
	b := newStore(t)
	if _, err := b.Sync(s); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Restore(words, nil, NewKey{Alg: "secp256k1", Secret: sec}); err == nil || err.Error() != "key reuse" {
		t.Errorf("restore to a used npub: %v", err)
	}
	if _, err := b.ReadLocal(); err != ErrNotInitialized {
		t.Errorf("failed restore left local/: %v", err)
	}
	if _, got, err := b.Restore(words, nil, NewKey{}); err != nil || got != id {
		t.Fatalf("restore: %s %v", got, err)
	}
	if lb := verifyOK(t, b); len(lb.Report.Chains[id]) != 3 {
		t.Errorf("chain %d", len(lb.Report.Chains[id]))
	}
}

// An agent with its own npub writes a claim about the person as a bare
// key (Rob's point: agents sign as themselves, not as the person).
func TestAgentNpubClaim(t *testing.T) {
	s, ident := initStore(t)
	agentSecret := bytes.Repeat([]byte{7}, 32)
	agent, err := NewSecp256k1Signer(agentSecret)
	if err != nil {
		t.Fatal(err)
	}
	claim := map[string]any{
		"v": 0.0, "nonce": NewNonce(), "type": "note", "subject": ident, "author": agent.Public(), "kid": agent.Public(),
		"value":  map[string]any{"s": "resting heart rate rose 6/min over October"},
		"times":  map[string]any{"observed": nil, "imported": "2026-10-28T12:00:00Z"},
		"source": map[string]any{"system": "agent", "adapter": "test@1", "ref": nil},
	}
	cid, data, err := SignRecord(claim, agent)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(s.path(PathOf(cid)), data, 0o600); err != nil {
		t.Fatal(err)
	}
	l := verifyOK(t, s)
	if !l.Visible(cid) {
		t.Errorf("agent claim not valid: %s", l.Status(cid))
	}
}
