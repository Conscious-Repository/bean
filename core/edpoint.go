package core

import (
	"math/big"
	"sync"
)

// The strict Ed25519 rules of SPEC.md 4.4 and 4.5 (change N2), in the
// words of libsodium's crypto_sign_verify_detached: a public key must be
// the canonical encoding of a curve point that is not of small order, and
// the R half of a signature must not be of small order. crypto/ed25519
// checks none of this, so the point arithmetic is done here with big
// integers. It runs once per key and once per signature, so speed does
// not matter.

var (
	edP    = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))
	edD    = edMul(big.NewInt(-121665), edInv(big.NewInt(121666)))
	edSqrt = new(big.Int).Exp(big.NewInt(2), new(big.Int).Div(new(big.Int).Sub(edP, big.NewInt(1)), big.NewInt(4)), edP)
)

func edMod(x *big.Int) *big.Int { return x.Mod(x, edP) }

func edMul(a, b *big.Int) *big.Int { return edMod(new(big.Int).Mul(a, b)) }

func edInv(a *big.Int) *big.Int {
	return new(big.Int).Exp(a, new(big.Int).Sub(edP, big.NewInt(2)), edP)
}

// edDecode decodes a 32-byte point encoding (RFC 8032 5.1.3). It returns
// false for a string that is no point and for a non-canonical encoding:
// y not below p, or x = 0 with the sign bit set.
func edDecode(b []byte) (x, y *big.Int, ok bool) {
	if len(b) != 32 {
		return nil, nil, false
	}
	le := make([]byte, 32)
	for i := range b {
		le[31-i] = b[i]
	}
	sign := le[0] >> 7
	le[0] &= 0x7f
	y = new(big.Int).SetBytes(le)
	if y.Cmp(edP) >= 0 {
		return nil, nil, false
	}
	yy := edMul(y, y)
	u := edMod(new(big.Int).Sub(yy, big.NewInt(1)))
	v := edMod(new(big.Int).Add(edMul(edD, yy), big.NewInt(1)))
	xx := edMul(u, edInv(v))
	x = new(big.Int).Exp(xx, new(big.Int).Div(new(big.Int).Add(edP, big.NewInt(3)), big.NewInt(8)), edP)
	if edMul(x, x).Cmp(xx) != 0 {
		x = edMul(x, edSqrt)
	}
	if edMul(x, x).Cmp(xx) != 0 {
		return nil, nil, false
	}
	if x.Sign() == 0 && sign == 1 {
		return nil, nil, false
	}
	if uint8(x.Bit(0)) != sign {
		x = new(big.Int).Sub(edP, x)
	}
	return x, y, true
}

// edSmallOrder reports whether [8]P is the neutral point (0, 1).
func edSmallOrder(x, y *big.Int) bool {
	for range 3 {
		// Doubling on -x^2 + y^2 = 1 + d x^2 y^2 (complete formula).
		xy := edMul(x, y)
		t := edMul(edD, edMul(xy, xy))
		x2 := edMul(edMod(new(big.Int).Add(xy, xy)), edInv(edMod(new(big.Int).Add(big.NewInt(1), t))))
		y2 := edMul(edMod(new(big.Int).Add(edMul(y, y), edMul(x, x))), edInv(edMod(new(big.Int).Sub(big.NewInt(1), t))))
		x, y = x2, y2
	}
	return x.Sign() == 0 && y.Cmp(big.NewInt(1)) == 0
}

// edKeys remembers edStrictKey by key: a log has few keys and many records.
var edKeys sync.Map

// edStrictKey reports whether b is a canonical point encoding that is not
// of small order.
func edStrictKey(b []byte) bool {
	if ok, seen := edKeys.Load(string(b)); seen {
		return ok.(bool)
	}
	x, y, ok := edDecode(b)
	ok = ok && !edSmallOrder(x, y)
	edKeys.Store(string(b), ok)
	return ok
}

// edSmallY holds y mod p of the eight points of small order: 1 and p-1
// (x = 0), 0 (order 4) and the two y values of the points of order 8.
var edSmallY = func() map[string]bool {
	out := map[string]bool{}
	for _, s := range []string{"1", "0"} {
		y, _ := new(big.Int).SetString(s, 10)
		out[y.String()] = true
	}
	out[new(big.Int).Sub(edP, big.NewInt(1)).String()] = true
	y8, _ := new(big.Int).SetString("05fc536d880238b13933c6d305acdfd5f098eff289f4c345b027b2c28f95e826", 16)
	out[y8.String()] = true
	out[new(big.Int).Sub(edP, y8).String()] = true
	return out
}()

// edSmallOrderR reports whether the R half of a signature encodes a point
// of small order, in any encoding, as libsodium's blocklist does. Every y
// in edSmallY is on the curve with both signs, so y alone decides. An R
// that is no point fails the ordinary check.
func edSmallOrderR(r []byte) bool {
	le := make([]byte, 32)
	for i := range r {
		le[31-i] = r[i]
	}
	le[0] &= 0x7f
	y := new(big.Int).SetBytes(le)
	return edSmallY[y.Mod(y, edP).String()]
}
