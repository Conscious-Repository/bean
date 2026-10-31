package main

import (
	"math/big"
	"strings"

	"github.com/Conscious-Repository/bean/core"
)

// V23 pins the Ed25519 edge cases (SPEC 4.4 and 4.5, change N2): a key is
// a canonical point that is not of small order, and S is below the group
// order. A second implementation must give the same result for each case.
func init() { extra = append(extra, genV23) }

func genV23(signer core.Signer, _ map[string]any) {
	identity := "01" + strings.Repeat("00", 31)            // the identity point, small order
	nonCanonical := "ee" + strings.Repeat("ff", 30) + "7f" // y = p + 1, the identity point again
	notPoint := "02" + strings.Repeat("00", 31)            // y = 2 is on no point of the curve
	universal := identity + strings.Repeat("00", 32)       // R = identity, S = 0
	id1 := "sha256:" + strings.Repeat("00", 32)
	id2 := "sha256:" + strings.Repeat("07", 32)
	good, err := signer.Sign(id1)
	if err != nil {
		panic(err)
	}
	// S = L: the same R, S raised by the group order.
	l, _ := new(big.Int).SetString("7237005577332262213973186563042994240857116359379907606001950938285454250989", 10)
	raw := hexb(strings.TrimPrefix(good, "ed25519:"))
	s := new(big.Int).SetBytes(reverse(raw[32:]))
	s.Add(s, l)
	sBytes := reverse(s.FillBytes(make([]byte, 32)))
	highS := "ed25519:" + hexs(raw[:32]) + hexs(sBytes)
	type c struct{ name, kid, id, sig string }
	cases := []c{
		{"good signature", signer.Public(), id1, good},
		{"S above the group order", signer.Public(), id1, highS},
		{"small-order key, one signature for every id (1)", "ed25519:" + identity, id1, "ed25519:" + universal},
		{"small-order key, one signature for every id (2)", "ed25519:" + identity, id2, "ed25519:" + universal},
		{"non-canonical encoding of a small-order key", "ed25519:" + nonCanonical, id1, "ed25519:" + universal},
		{"key that is not a curve point", "ed25519:" + notPoint, id1, good},
	}
	var out []any
	for _, x := range cases {
		key, verify := "ok", "accept"
		if _, err := core.ParseKey(x.kid); err != nil {
			key = err.Error()
		}
		if err := core.Verify(x.kid, x.id, x.sig); err != nil {
			verify = err.Error()
		}
		out = append(out, map[string]any{"name": x.name, "kid": x.kid, "id": x.id, "sig": x.sig, "parse_key": key, "verify": verify})
	}
	write("v23-ed25519-edges.json", map[string]any{"spec": "SPEC.md 4.4 and 4.5", "cases": out})
}

func reverse(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[len(b)-1-i] = b[i]
	}
	return out
}

func hexs(b []byte) string {
	const digits = "0123456789abcdef"
	var sb strings.Builder
	for _, x := range b {
		sb.WriteByte(digits[x>>4])
		sb.WriteByte(digits[x&15])
	}
	return sb.String()
}
