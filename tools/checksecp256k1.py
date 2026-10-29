#!/usr/bin/env python3
"""Independent check of vector V18 (secp256k1 keys, change note C21).

Pure Python standard library: BIP-340 Schnorr written from the BIP text,
BIP-173 bech32 for NIP-19, and the NIP-01 event id of the wrapper event.
It shares no code with the Go implementation or its libraries. Run from the
repo root:

    python3 -I tools/checksecp256k1.py
"""
import hashlib
import json
import sys

P = 2**256 - 2**32 - 977
N = 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141
G = (0x79BE667EF9DCBBAC55A06295CE870B07029BFCDB2DCE28D959F2815B16F81798,
     0x483ADA7726A3C4655DA4FBFC0E1108A8FD17B448A68554199C47D08FFB10D4B8)
failures = 0


def check(name, got, want):
    global failures
    if got != want:
        failures += 1
        print(f"FAIL {name}\n  got  {got!r}\n  want {want!r}")
    else:
        print(f"ok   {name}")


# ---- BIP-340 ------------------------------------------------------------

def point_add(a, b):
    if a is None:
        return b
    if b is None:
        return a
    if a[0] == b[0] and a[1] != b[1]:
        return None
    if a == b:
        lam = 3 * a[0] * a[0] * pow(2 * a[1], P - 2, P) % P
    else:
        lam = (b[1] - a[1]) * pow(b[0] - a[0], P - 2, P) % P
    x = (lam * lam - a[0] - b[0]) % P
    return (x, (lam * (a[0] - x) - a[1]) % P)


def point_mul(pt, n):
    r = None
    for i in range(256):
        if (n >> i) & 1:
            r = point_add(r, pt)
        pt = point_add(pt, pt)
    return r


def tagged_hash(tag, msg):
    t = hashlib.sha256(tag.encode()).digest()
    return hashlib.sha256(t + t + msg).digest()


def lift_x(x):
    if x >= P:
        return None
    y_sq = (pow(x, 3, P) + 7) % P
    y = pow(y_sq, (P + 1) // 4, P)
    if pow(y, 2, P) != y_sq:
        return None
    return (x, y if y % 2 == 0 else P - y)


def b32(n):
    return n.to_bytes(32, "big")


def pubkey(sk):
    return b32(point_mul(G, sk)[0])


def sign(sk, msg, aux):
    pt = point_mul(G, sk)
    d = sk if pt[1] % 2 == 0 else N - sk
    t = (d ^ int.from_bytes(tagged_hash("BIP0340/aux", aux), "big")).to_bytes(32, "big")
    k0 = int.from_bytes(tagged_hash("BIP0340/nonce", t + b32(pt[0]) + msg), "big") % N
    R = point_mul(G, k0)
    k = k0 if R[1] % 2 == 0 else N - k0
    e = int.from_bytes(tagged_hash("BIP0340/challenge", b32(R[0]) + b32(pt[0]) + msg), "big") % N
    return b32(R[0]) + b32((k + e * d) % N)


def verify(pk, msg, sig):
    pt = lift_x(int.from_bytes(pk, "big"))
    r, s = int.from_bytes(sig[:32], "big"), int.from_bytes(sig[32:], "big")
    if pt is None or r >= P or s >= N:
        return False
    e = int.from_bytes(tagged_hash("BIP0340/challenge", sig[:32] + pk + msg), "big") % N
    R = point_add(point_mul(G, s), point_mul(pt, N - e))
    return R is not None and R[1] % 2 == 0 and R[0] == r


# ---- bech32 (BIP-173) --------------------------------------------------

CHARSET = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"


def polymod(values):
    gen = [0x3B6A57B2, 0x26508E6D, 0x1EA119FA, 0x3D4233DD, 0x2A1462B3]
    chk = 1
    for v in values:
        b = chk >> 25
        chk = (chk & 0x1FFFFFF) << 5 ^ v
        for i in range(5):
            chk ^= gen[i] if (b >> i) & 1 else 0
    return chk


def hrp_expand(hrp):
    return [ord(c) >> 5 for c in hrp] + [0] + [ord(c) & 31 for c in hrp]


def bech32_encode(hrp, data):
    acc, bits, five = 0, 0, []
    for b in data:
        acc, bits = (acc << 8) | b, bits + 8
        while bits >= 5:
            bits -= 5
            five.append((acc >> bits) & 31)
    if bits:
        five.append((acc << (5 - bits)) & 31)
    pm = polymod(hrp_expand(hrp) + five + [0] * 6) ^ 1
    return hrp + "1" + "".join(CHARSET[d] for d in five + [(pm >> 5 * (5 - i)) & 31 for i in range(6)])


# ---- the wrapper event (NIP-01) ----------------------------------------

def wrapper(pk_hex, created_at, kind, alt, record_id):
    return json.dumps([0, pk_hex, created_at, kind, [["alt", alt]], record_id], separators=(",", ":"), ensure_ascii=False).encode()


def check_sig(kid, record_id, sig_str, kind, alt):
    """Return "accept" or "bad signature" for a tagged secp256k1 signature."""
    alg, _, rest = sig_str.partition(":")
    ts, _, hx = rest.partition(":")
    if alg != "secp256k1" or not ts.isdigit() or (len(ts) > 1 and ts[0] == "0"):
        return "bad signature"
    if len(hx) != 128 or hx != hx.lower():
        return "bad signature"
    pk_hex = kid.split(":")[1]
    msg = hashlib.sha256(wrapper(pk_hex, int(ts), kind, alt, record_id)).digest()
    return "accept" if verify(bytes.fromhex(pk_hex), msg, bytes.fromhex(hx)) else "bad signature"


# ---- checks ------------------------------------------------------------

# BIP-340 test vector 0 (bip-0340/test-vectors.csv).
sig0 = sign(3, bytes(32), bytes(32)).hex().upper()
check("bip340 vector 0 pubkey", pubkey(3).hex().upper(), "F9308A019258C31049344F85F89D5229B531C845836F99B08601F113BCE036F9")
check("bip340 vector 0 sig", sig0, "E907831F80848D1069A5371B402410364BDF1C5F8307B0084C55F1CE2DCA821525F66A4A85EA8B71E482A74F382D2CE5EBEEE8FDB2172F477DF4900D310536C0")

# NIP-19 examples.
check("nip19 npub", bech32_encode("npub", bytes.fromhex("7e7e9c42a91bfef19fa929e5fda1b72e0ebc1a4c1141673e2794234d86addf4e")), "npub10elfcs4fr0l0r8af98jlmgdh9c8tcxjvz9qkw038js35mp4dma8qzvjptg")
check("nip19 nsec", bech32_encode("nsec", bytes.fromhex("67dea2ed018072d675f5415ecfaed7d2597555e202d85b3d65ea4e58d2d92ffa")), "nsec1vl029mgpspedva04g90vltkh6fvh240zqtv9k0t9af8935ke9laqsnlfe5")

with open("vectors/v18-secp256k1.json", encoding="utf-8") as f:
    v = json.load(f)
sk = int(v["secret_key"], 16)
pk = pubkey(sk)
kind = v["nostr_kind"]
alt = v["alt"]
check("v18 alt tag", alt, "bean record signature")
check("v18 kid", "secp256k1:" + pk.hex(), v["kid"])
check("v18 npub", bech32_encode("npub", pk), v["npub"])
check("v18 nsec", bech32_encode("nsec", bytes.fromhex(v["secret_key"])), v["nsec"])
for c in v["cases"]:
    t = c["created_at"]
    w = wrapper(pk.hex(), t, kind, alt, v["id"])
    check(f"v18 wrapper bytes created_at={t}", w.decode(), c["wrapper_bytes"])
    wid = hashlib.sha256(w).digest()
    check(f"v18 wrapper id created_at={t}", wid.hex(), c["wrapper_id"])
    want = f"secp256k1:{t}:" + sign(sk, wid, bytes.fromhex(c["aux_rand"])).hex()
    check(f"v18 sig created_at={t}", want, c["sig"])
    check(f"v18 verify created_at={t}", check_sig(v["kid"], v["id"], c["sig"], kind, alt), "accept")
for r in v["rejects"]:
    check(f"v18 reject {r['name']}", check_sig(v["kid"], r["id"], r["sig"], kind, alt), r["result"])

print()
print("FAILURES:", failures)
sys.exit(1 if failures else 0)
