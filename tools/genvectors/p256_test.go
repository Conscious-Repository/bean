package main

import (
	"crypto"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
)

// The V6 signer uses RFC 6979 nonces, so V6 is reproducible.
func TestP256Deterministic(t *testing.T) {
	d, _ := hex.DecodeString("C9AFA9D845BA75166B5C215767B1D6934E50C3DB36E89B127B8A622B120F6721")
	k, err := newP256(d)
	if err != nil {
		t.Fatal(err)
	}
	// RFC 6979 A.2.5, P-256 with SHA-256, message "sample".
	h := sha256.Sum256([]byte("sample"))
	der, err := k.Sign(nil, h[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	var rs struct{ R, S *big.Int }
	asn1.Unmarshal(der, &rs)
	if fmt.Sprintf("%X", rs.R) != "EFD48B2AACB6A8FD1140DD9CD45E81D69D2C877B56AAF991C34D0EA84EAF3716" ||
		fmt.Sprintf("%X", rs.S) != "F7CB1C942D657C41D436C7A1B6E29F65F3E900DBB9AFF4064DC4AB2F843ACDA8" {
		t.Fatalf("not RFC 6979: r=%X s=%X", rs.R, rs.S)
	}
	if want := "p256:0360fed4ba255a9d31c961eb74c6356d68c049b8923b61fa6ce669622e60f29fb6"; p256Public(k) != want {
		t.Errorf("public %s", p256Public(k))
	}
	id := "sha256:" + strings.Repeat("ab", 32)
	a, b := p256Sign(k, id), p256Sign(k, id)
	if a != b {
		t.Error("two signatures differ")
	}
	if err := core.Verify(p256Public(k), id, a); err != nil {
		t.Error(err)
	}
}
