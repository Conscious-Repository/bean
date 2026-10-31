#!/usr/bin/env python3
"""cases.py <dir> writes the edge-case logs of vectors/conformance (SPEC 14,
V24): one folder per case, each with an objects/ folder of record files.
The keys are fixed test keys, so the output is the same on every run.
tools/genvectors writes expected.txt in each folder with the bean verify
lines and exit code; run.sh checks the second verifier against them."""
import hashlib, json, os, sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from beanverify import canonical, record_id, commitment
from cryptography.hazmat.primitives.asymmetric import ed25519
from cryptography.hazmat.primitives.kdf.hkdf import HKDF
from cryptography.hazmat.primitives import hashes, serialization
OUT = sys.argv[1]
if os.path.exists(OUT): sys.exit("cases.py: %s exists; remove it first" % OUT)
def hkdf(ikm, info, n): return HKDF(hashes.SHA256(), n, None, info.encode()).derive(ikm)
class Key:
    def __init__(self, seed):
        self.sk = ed25519.Ed25519PrivateKey.from_private_bytes(seed)
        self.pub = "ed25519:" + self.sk.public_key().public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw).hex()
    def sign(self, rid): return "ed25519:" + self.sk.sign(bytes.fromhex(rid[7:])).hex()
class RawKey:  # a fixed public key and a fixed signature for every id
    def __init__(self, pub, sig): self.pub = "ed25519:" + pub; self.s = "ed25519:" + sig
    def sign(self, rid): return self.s
n = [0]
def rec(key, **f):
    n[0] += 1
    r = {"v": 0, "nonce": "%032x" % (0xBADC0DE00 + n[0]), "kid": key.pub,
         "times": {"observed": None, "imported": "2026-10-30T12:00:00Z"},
         "source": {"system": "self", "adapter": "sat@1", "ref": None}}
    r.update(f)
    for k in [k for k, v in r.items() if v is DEL]: del r[k]
    r["id"] = record_id(r); r["sig"] = key.sign(r["id"]); return r
DEL = object()
class Log:
    def __init__(self, name):
        self.dir = os.path.join(OUT, name); os.makedirs(os.path.join(self.dir, "objects"))
    def put(self, r, raw=None):
        h = r["id"][7:]; p = os.path.join(self.dir, "objects", h[:2]); os.makedirs(p, exist_ok=True)
        open(os.path.join(p, h[2:] + ".json"), "wb").write(raw if raw is not None else canonical(r).encode()); return r["id"]
class Ident:
    def __init__(self, log, b, key=None):
        self.log = log; self.seed = bytes([b]) * 32; self.key = key or Key(bytes([b + 0x40]) * 32)
        g = rec(self.key, type="bean.genesis", author=self.key.pub, value={"key": self.key.pub, "recovery": commitment(self.rec(0).pub)})
        self.id = log.put(g); self.head = self.id; self.gen = 0; self.g = g
    def rec(self, g): return Key(hkdf(self.seed, "bean/v0/recovery-key/%d" % g, 32))
    def claim(self, x, key=None, **f):
        d = dict(type="hr", subject=self.id, author=self.id, value={"n": x, "unit": "bpm"}); d.update(f)
        return self.log.put(rec(key or self.key, **d))
    def rotate(self, sealed, parent=None, newkey=None, parts=None, newpub=None):
        g = self.gen; nk = newkey or Key(bytes([0xA0 + self.gen + 7 * (parent is not None)]) * 32)
        v = {"key": newpub or nk.pub, "recovery": commitment(self.rec(g + 1).pub), "sealed": sorted(sealed)}
        if parts: v["parts"] = parts
        r = rec(self.rec(g), type="bean.succession", subject=self.id, author=self.id, refs=[{"rel": "succeeds", "id": parent or self.head}], value=v)
        self.log.put(r)
        if parent is None: self.head = r["id"]; self.gen += 1; self.oldkey = self.key; self.key = nk
        return r["id"]
def tomb(i, t, key=None, **f): return i.log.put(rec(key or i.key, type="bean.tombstone", subject=i.id, author=i.id, refs=[{"rel": "purges", "id": t}], **f))
def future(i, key, t="bean.tombstone"):
    r = rec(key, type=t, subject=i.id, author=i.id, refs=[{"rel": "purges", "id": i.id}], v=9); return i.log.put(r)
T0 = "2026-10-30T12:00:00Z"
def obs(start, end=None, p="second"):
    o = {"start": start, "precision": p}
    if end: o["end"] = end
    return {"observed": o, "imported": T0}

# 01 nesting depth 300 inside context: bad json (3.8 step 3)
L = Log("01-depth-300"); I = Ident(L, 1)
deep = 0
for _ in range(300): deep = [deep]
I.claim(1, context={"deep": deep})
# 02 tombstone and 03 seal part with "blobs": [] ("no blobs" allows an empty array, 3.3)
L = Log("02-tomb-empty-blobs"); I = Ident(L, 2); c = I.claim(1); tomb(I, c, blobs=[])
L = Log("03-seal-empty-blobs"); I = Ident(L, 3); cs = sorted(I.claim(x) for x in range(3))
sid = I.rotate(cs[:1], parts=1)
L.put(rec(I.rec(0), type="bean.seal", subject=I.id, author=I.id, refs=[{"rel": "seals", "id": sid}], value={"part": 1, "sealed": cs[1:]}, blobs=[]))
# 04 source.system of 40 x "é" (40 characters, 80 bytes)
L = Log("04-system-chars"); I = Ident(L, 4); I.claim(1, source={"system": "é" * 40, "adapter": "sat@1", "ref": None})
# 10..12 whole-string patterns and the adapter whitespace set (3.6)
L = Log("10-nonce-newline"); I = Ident(L, 12); I.claim(1, nonce="%032x\n" % 7)
L = Log("11-type-newline"); I = Ident(L, 13); I.claim(1, type="hr\n")
L = Log("12-adapter-space"); I = Ident(L, 14); I.claim(1, source={"system": "x", "adapter": "a b@1", "ref": None})
L = Log("12b-adapter-nbsp"); I = Ident(L, 15); I.claim(1, source={"system": "x", "adapter": "a\u00a0b@1", "ref": None})
L = Log("12c-adapter-vt"); I = Ident(L, 16); I.claim(1, source={"system": "x", "adapter": "a\u000bb@1", "ref": None})
# 13 supersede with two missing targets; and one missing + one not author
L = Log("13-sup-two-missing"); I = Ident(L, 17)
I.claim(2, refs=[{"rel": "supersedes", "id": "sha256:" + "1" * 64}, {"rel": "supersedes", "id": "sha256:" + "2" * 64}])
L = Log("13b-sup-missing-and-notauthor"); I = Ident(L, 18); J = Ident(L, 19); cj = J.claim(1)
I.claim(2, refs=[{"rel": "supersedes", "id": "sha256:" + "1" * 64}, {"rel": "supersedes", "id": cj}])
# 14 frozen identity + unsealed destructive record of a retired trunk key
L = Log("14-frozen-unsealed-destructive"); I = Ident(L, 20); c = I.claim(1); k0 = I.key
t = tomb(I, c)                       # tombstone by key 0
I.rotate([c])                        # seals c but not t
I.rotate([], parent=I.head)          # two successions below the head: a fork
I.rotate([], parent=I.head, newkey=Key(b"\x33" * 32))
# 15 too-new identity (head-key halt) with a held succession and 16 with a bad succession
L = Log("15-toonew-held-succ"); I = Ident(L, 21); I.claim(1); future(I, I.key)
L.put(rec(I.rec(1), type="bean.succession", subject=I.id, author=I.id, refs=[{"rel": "succeeds", "id": "sha256:" + "3" * 64}], value={"key": Key(b"\x34" * 32).pub, "recovery": commitment(I.rec(2).pub), "sealed": []}))
L = Log("16-toonew-bad-succ"); I = Ident(L, 22); I.claim(1); future(I, I.key)
L.put(rec(Key(b"\x35" * 32), type="bean.succession", subject=I.id, author=I.id, refs=[{"rel": "succeeds", "id": I.id}], value={"key": Key(b"\x36" * 32).pub, "recovery": commitment(I.rec(1).pub), "sealed": []}))
# 17 signing key equal to a recovery key (5.8)
L = Log("17-key-is-recovery"); I = Ident(L, 23); I.rotate([], newkey=I.rec(0)); I.claim(1)
L = Log("17b-key-is-next-recovery"); I = Ident(L, 24); I.rotate([], newkey=I.rec(1)); I.claim(1)
L = Log("17c-bare-recovery-key"); I = Ident(L, 25); I.rotate([]); r0 = I.rec(0)
L.put(rec(r0, type="hr", subject=I.id, author=r0.pub, value={"n": 1}))
# 18 Ed25519 small-order and non-canonical keys: one signature for every id
ID_PT = "01" + "00" * 31; ID_NC = "ee" + "ff" * 30 + "7f"   # identity point, and y = p + 1 (non-canonical identity)
L = Log("18-ed-small-order-bare"); I = Ident(L, 26)
k = RawKey(ID_PT, ID_PT + "00" * 32); L.put(rec(k, type="hr", subject=I.id, author=k.pub, value={"n": 1}))
L = Log("18b-ed-noncanon-bare"); I = Ident(L, 27)
k = RawKey(ID_NC, ID_PT + "00" * 32); L.put(rec(k, type="hr", subject=I.id, author=k.pub, value={"n": 1}))
L = Log("18c-ed-small-order-genesis"); Ident(L, 28, key=RawKey(ID_PT, ID_PT + "00" * 32))
# 19 Ed25519 value.key that is not a curve point (4.4, 4.5)
def notpoint():
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
    p = 2**255 - 19; d = (-121665 * pow(121666, p - 2, p)) % p
    for y in range(2, 1000):
        u = (y * y - 1) % p; v = (d * y * y + 1) % p; x2 = u * pow(v, p - 2, p) % p
        if x2 != 0 and pow(x2, (p - 1) // 2, p) != 1: return y.to_bytes(32, "little").hex()
NP = notpoint()
L = Log("19-ed-notpoint-succession-key"); I = Ident(L, 29); s1 = I.rotate([], newpub="ed25519:" + NP)
L.put(rec(I.rec(1), type="bean.succession", subject=I.id, author=I.id, refs=[{"rel": "succeeds", "id": s1}], value={"key": Key(b"\x37" * 32).pub, "recovery": commitment(I.rec(2).pub), "sealed": []}))
L = Log("19b-ed-notpoint-bare-author"); I = Ident(L, 30); I.claim(1, author="ed25519:" + NP)
# 20 succession value with an extra unknown key: bad field (3.2, 3.3)
L = Log("20-succ-extra-value-key"); I = Ident(L, 31)
L.put(rec(I.rec(0), type="bean.succession", subject=I.id, author=I.id, refs=[{"rel": "succeeds", "id": I.id}], value={"key": Key(b"\x38" * 32).pub, "recovery": commitment(I.rec(1).pub), "sealed": [], "x": 1}))
# 21 unknown v with non-canonical bytes (whitespace): skipped (3.8 step 13)
L = Log("21-unknown-v-noncanonical"); I = Ident(L, 32); I.claim(1)
r = rec(I.key, type="hr", subject=I.id, author=I.id, value={"n": 1}, v=7); L.put(r, raw=json.dumps(r, indent=1).encode())
# 22 tombstone reason of 200 astral characters (400 UTF-16 units)
L = Log("22-reason-astral"); I = Ident(L, 33); c = I.claim(1); tomb(I, c, context={"reason": "\U0001F600" * 200})
# 23 duplicate key through an escape
L = Log("23-dup-escaped"); I = Ident(L, 34)
r = rec(I.key, type="hr", subject=I.id, author=I.id, value={"n": 1}); raw = canonical(r).replace('"n":1', '"n":1,"\\u006e":2')
L.put(r, raw=raw.encode())
# 24 number underflow 1e-400 in an unknown-v record (envelope passes, then skip)
L = Log("24-underflow"); I = Ident(L, 35)
r = rec(I.key, type="hr", subject=I.id, author=I.id, value={"n": 0}, v=7); L.put(r, raw=canonical(r).replace('"n":0', '"n":1e-400').encode())
# 25 v written as 1.0 / 7e0 in an unknown-v record
L = Log("25-v-float"); I = Ident(L, 36)
r = rec(I.key, type="bean.tombstone", subject=I.id, author=I.id, refs=[{"rel": "purges", "id": I.id}], v=7)
L.put(r, raw=canonical(r).replace('"v":7', '"v":7.0').encode())
print("wrote", len(os.listdir(OUT)), "cases")
