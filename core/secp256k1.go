package core

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

// secp256k1 signing keys (docs/ROADMAP.md M8, change note C21). The public key
// is the 32-byte BIP-340 x-only key, which is also the Nostr public key.
// The signature is not over the record id directly: it is a Nostr event
// signature over a wrapper event whose content is the record id, so any
// Nostr signer (a NIP-46 bunker, a NIP-07 browser extension) can sign a
// record. The wrapper is
//
//	[0, "<pubkey hex>", <created_at>, NostrKind, [["alt", WrapperAlt]], "<record id>"]
//
// serialized as NIP-01 does, and its SHA-256 is the BIP-340 message.
// created_at travels in the sig string: secp256k1:<created_at>:<128 hex>.

// NostrKind is the wrapper event kind. It is in the NIP-01 ephemeral
// range (20000 to 29999), so relays do not store a wrapper if one is ever
// published, and it is not in the NIPs registry. A NIP-46 signer can grant
// permission for this kind alone (sign_event:29944), so a client that logs
// health records cannot sign anything else in the person's name.
const NostrKind = 29944

// WrapperAlt is the NIP-31 alt tag: what a signer's approval prompt can
// show for an event kind it does not know.
const WrapperAlt = "bean record signature"

// maxCreatedAt keeps created_at inside the JSON integer range (SPEC 4.1).
const maxCreatedAt = 1<<53 - 1

// WrapperBytes is the NIP-01 serialization of the wrapper event. Every
// field is ASCII, so no escaping is needed.
func WrapperBytes(pubHex string, createdAt int64, id string) []byte {
	return []byte(`[0,"` + pubHex + `",` + strconv.FormatInt(createdAt, 10) + `,` + strconv.Itoa(NostrKind) + `,[["alt","` + WrapperAlt + `"]],"` + id + `"]`)
}

// WrapperID is the Nostr event id of the wrapper, the BIP-340 message.
func WrapperID(pubHex string, createdAt int64, id string) [32]byte {
	return sha256.Sum256(WrapperBytes(pubHex, createdAt, id))
}

func parseSecp256k1Key(hx string) (*btcec.PublicKey, error) {
	if !isLowerHex(hx, 64) {
		return nil, ErrBadKey
	}
	b, _ := hex.DecodeString(hx)
	pk, err := schnorr.ParsePubKey(b) // lift_x: fails when x is not on the curve
	if err != nil {
		return nil, ErrBadKey
	}
	return pk, nil
}

// parseSecp256k1Sig splits "<created_at>:<128 hex>".
func parseSecp256k1Sig(rest string) (int64, *schnorr.Signature, error) {
	ts, hx, ok := strings.Cut(rest, ":")
	if !ok || ts == "" || (len(ts) > 1 && ts[0] == '0') || !isLowerHex(hx, 128) {
		return 0, nil, ErrBadSignature
	}
	for _, c := range ts {
		if c < '0' || c > '9' {
			return 0, nil, ErrBadSignature
		}
	}
	created, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || created > maxCreatedAt {
		return 0, nil, ErrBadSignature
	}
	raw, _ := hex.DecodeString(hx)
	sig, err := schnorr.ParseSignature(raw)
	if err != nil {
		return 0, nil, ErrBadSignature
	}
	return created, sig, nil
}

func verifySecp256k1(pubHex string, pk *btcec.PublicKey, id, rest string) error {
	created, sig, err := parseSecp256k1Sig(rest)
	if err != nil {
		return err
	}
	msg := WrapperID(pubHex, created, id)
	if !sig.Verify(msg[:], pk) {
		return ErrBadSignature
	}
	return nil
}

type secp256k1Signer struct {
	priv   *btcec.PrivateKey
	pubHex string
}

// NewSecp256k1Signer makes a signer from a 32-byte secret key (the bytes
// of an nsec). It rejects zero and values at or above the curve order.
func NewSecp256k1Signer(secret []byte) (Signer, error) {
	if len(secret) != 32 {
		return nil, ErrBadKey
	}
	var s btcec.ModNScalar
	if overflow := s.SetByteSlice(secret); overflow || s.IsZero() {
		return nil, ErrBadKey
	}
	priv, _ := btcec.PrivKeyFromBytes(secret)
	return secp256k1Signer{priv, hex.EncodeToString(schnorr.SerializePubKey(priv.PubKey()))}, nil
}

func (s secp256k1Signer) Public() string { return "secp256k1:" + s.pubHex }

// Sign signs the wrapper with created_at 0 and fresh BIP-340 auxiliary
// randomness. created_at carries no meaning; validity never uses a clock.
func (s secp256k1Signer) Sign(id string) (string, error) {
	var aux [32]byte
	if _, err := rand.Read(aux[:]); err != nil {
		return "", err
	}
	return s.sign(id, 0, aux)
}

func (s secp256k1Signer) sign(id string, createdAt int64, aux [32]byte) (string, error) {
	if !IsID(id) {
		return "", ErrBadSignature
	}
	msg := WrapperID(s.pubHex, createdAt, id)
	sig, err := schnorr.Sign(s.priv, msg[:], schnorr.CustomNonce(aux))
	if err != nil {
		return "", err
	}
	return "secp256k1:" + strconv.FormatInt(createdAt, 10) + ":" + hex.EncodeToString(sig.Serialize()), nil
}

// SignSecp256k1With signs with a chosen created_at and BIP-340 auxiliary
// data. Test vectors use it to make reproducible signatures; real signing
// goes through NewSecp256k1Signer.
func SignSecp256k1With(secret []byte, id string, createdAt int64, aux [32]byte) (string, error) {
	s, err := NewSecp256k1Signer(secret)
	if err != nil {
		return "", err
	}
	return s.(secp256k1Signer).sign(id, createdAt, aux)
}

// Bech32 (BIP-173) for NIP-19 npub and nsec strings.

const bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

func bech32Polymod(values []byte) uint32 {
	gen := [5]uint32{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}
	chk := uint32(1)
	for _, v := range values {
		top := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ uint32(v)
		for i := 0; i < 5; i++ {
			if (top>>i)&1 == 1 {
				chk ^= gen[i]
			}
		}
	}
	return chk
}

func bech32HRPExpand(hrp string) []byte {
	out := make([]byte, 0, 2*len(hrp)+1)
	for i := 0; i < len(hrp); i++ {
		out = append(out, hrp[i]>>5)
	}
	out = append(out, 0)
	for i := 0; i < len(hrp); i++ {
		out = append(out, hrp[i]&31)
	}
	return out
}

// convertBits regroups bits; pad is true when encoding.
func convertBits(data []byte, from, to uint, pad bool) ([]byte, error) {
	var acc, bits uint
	var out []byte
	maxv := uint(1)<<to - 1
	for _, b := range data {
		if uint(b)>>from != 0 {
			return nil, errors.New("bad bech32")
		}
		acc = acc<<from | uint(b)
		bits += from
		for bits >= to {
			bits -= to
			out = append(out, byte(acc>>bits&maxv))
		}
	}
	if pad {
		if bits > 0 {
			out = append(out, byte(acc<<(to-bits)&maxv))
		}
	} else if bits >= from || acc<<(to-bits)&maxv != 0 {
		return nil, errors.New("bad bech32")
	}
	return out, nil
}

// Bech32Encode encodes data under hrp, as NIP-19 does for npub and nsec.
func Bech32Encode(hrp string, data []byte) string {
	five, _ := convertBits(data, 8, 5, true)
	values := append(bech32HRPExpand(hrp), five...)
	pm := bech32Polymod(append(values, 0, 0, 0, 0, 0, 0)) ^ 1
	var b strings.Builder
	b.WriteString(hrp)
	b.WriteByte('1')
	for _, v := range five {
		b.WriteByte(bech32Charset[v])
	}
	for i := 0; i < 6; i++ {
		b.WriteByte(bech32Charset[pm>>(5*(5-i))&31])
	}
	return b.String()
}

// Bech32Decode returns the hrp and data of a bech32 string. It accepts
// all-lowercase or all-uppercase input.
func Bech32Decode(s string) (string, []byte, error) {
	bad := errors.New("bad bech32")
	if strings.ToLower(s) != s && strings.ToUpper(s) != s {
		return "", nil, bad
	}
	s = strings.ToLower(s)
	pos := strings.LastIndexByte(s, '1')
	if pos < 1 || pos+7 > len(s) || len(s) > 1000 {
		return "", nil, bad
	}
	hrp := s[:pos]
	var values []byte
	for i := pos + 1; i < len(s); i++ {
		d := strings.IndexByte(bech32Charset, s[i])
		if d < 0 {
			return "", nil, bad
		}
		values = append(values, byte(d))
	}
	if bech32Polymod(append(bech32HRPExpand(hrp), values...)) != 1 {
		return "", nil, bad
	}
	data, err := convertBits(values[:len(values)-6], 5, 8, false)
	if err != nil {
		return "", nil, bad
	}
	return hrp, data, nil
}

// ParseNsec reads a Nostr secret key: "nsec1..." or 64 hex digits.
func ParseNsec(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if isLowerHex(strings.ToLower(s), 64) {
		return hex.DecodeString(strings.ToLower(s))
	}
	hrp, data, err := Bech32Decode(s)
	if err != nil || hrp != "nsec" || len(data) != 32 {
		return nil, errors.New("bad nsec: want nsec1... or 64 hex digits")
	}
	return data, nil
}

// KeyFromNpub turns "npub1..." into a secp256k1 tagged key. Any other
// string is returned unchanged, so a tagged key passes through.
func KeyFromNpub(s string) (string, error) {
	if !strings.HasPrefix(strings.ToLower(s), "npub1") {
		return s, nil
	}
	hrp, data, err := Bech32Decode(s)
	if err != nil || hrp != "npub" || len(data) != 32 {
		return "", errors.New("bad npub")
	}
	return "secp256k1:" + hex.EncodeToString(data), nil
}

// Npub returns the NIP-19 npub of a secp256k1 tagged key, or "".
func Npub(taggedKey string) string {
	hx, ok := strings.CutPrefix(taggedKey, "secp256k1:")
	if !ok || !isLowerHex(hx, 64) {
		return ""
	}
	b, _ := hex.DecodeString(hx)
	return Bech32Encode("npub", b)
}
