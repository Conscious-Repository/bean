#!/usr/bin/env python3
"""Writes edge-case logs with an independent Python writer (SPEC 4.x),
one folder per case, for differential verification against the Go binary."""
import hashlib, json, os, shutil, sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from beanverify import canonical, record_id, commitment
from cryptography.hazmat.primitives.asymmetric import ed25519
from cryptography.hazmat.primitives.kdf.hkdf import HKDF
from cryptography.hazmat.primitives import hashes, serialization

OUT = sys.argv[1]
shutil.rmtree(OUT, ignore_errors=True)

def hkdf(ikm, info, n):
    return HKDF(hashes.SHA256(), n, None, info.encode()).derive(ikm)

class Key:
    def __init__(self, seed):
        self.sk = ed25519.Ed25519PrivateKey.from_private_bytes(seed)
        self.pub = "ed25519:" + self.sk.public_key().public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw).hex()
    def sign(self, rid):
        return "ed25519:" + self.sk.sign(bytes.fromhex(rid[7:])).hex()

n = [0]
def rec(key, **f):
    n[0] += 1
    r = {"v": 0, "nonce": "%032x" % (0xC0FFEE00 + n[0]), "kid": key.pub,
         "times": {"observed": None, "imported": "2026-10-30T12:00:00Z"},
         "source": {"system": "self", "adapter": "craft@1", "ref": None}}
    r.update(f)
    r["id"] = record_id(r)
    r["sig"] = key.sign(r["id"])
    return r

class Log:
    def __init__(self, name):
        self.dir = os.path.join(OUT, name); os.makedirs(os.path.join(self.dir, "objects"))
    def put(self, r, raw=None):
        h = r["id"][7:]
        p = os.path.join(self.dir, "objects", h[:2]); os.makedirs(p, exist_ok=True)
        open(os.path.join(p, h[2:] + ".json"), "wb").write(raw if raw is not None else canonical(r).encode())
        return r["id"]
    def raw(self, name, data):
        p = os.path.join(self.dir, name); os.makedirs(os.path.dirname(p), exist_ok=True); open(p, "wb").write(data)

class Ident:
    def __init__(self, log, b):
        self.log = log; self.seed = bytes([b]) * 32
        self.key = Key(bytes([b + 0x40]) * 32)
        g = rec(self.key, type="bean.genesis", author=self.key.pub, value={"key": self.key.pub, "recovery": commitment(self.rec(0).pub)})
        self.id = log.put(g); self.head = self.id; self.gen = 0; self.chain = [g]
    def rec(self, g):
        return Key(hkdf(self.seed, "bean/v0/recovery-key/%d" % g, 32))
    def claim(self, n_, key=None, **f):
        key = key or self.key
        d = dict(type="hr", subject=self.id, author=self.id, value={"n": n_, "unit": "bpm"}); d.update(f)
        return self.log.put(rec(key, **d))
    def rotate(self, sealed, parent=None, newkey=None, parts=None, put=True, gen=None):
        g = self.gen if gen is None else gen
        nk = newkey or Key(bytes([0x90 + self.gen + len(self.chain)]) * 32)
        v = {"key": nk.pub, "recovery": commitment(self.rec(g + 1).pub), "sealed": sorted(sealed)}
        if parts: v["parts"] = parts
        r = rec(self.rec(g), type="bean.succession", subject=self.id, author=self.id, refs=[{"rel": "succeeds", "id": parent or self.head}], value=v)
        if put: self.log.put(r)
        if parent is None:
            self.head = r["id"]; self.gen += 1; self.oldkey = self.key; self.key = nk
        return r["id"]

def tomb(i, target, key=None, subject=None):
    return i.log.put(rec(key or i.key, type="bean.tombstone", subject=subject or i.id, author=i.id, refs=[{"rel": "purges", "id": target}]))
def sup(i, target, n_, key=None):
    return i.claim(n_, key=key, refs=[{"rel": "supersedes", "id": target}])

# A: tombstone of a supersede record; supersede of a purged target.
L = Log("tomb-of-sup"); I = Ident(L, 1)
a = I.claim(1); s = sup(I, a, 2); tomb(I, s)
b = I.claim(3); tomb(I, b); sup(I, b, 4)
# B: supersede and tombstone whose target is not sealed (invalid).
L = Log("target-invalid"); I = Ident(L, 2)
x = I.claim(1); I.rotate([]); sup(I, x, 2)
y_old = I.claim(5, key=I.oldkey); tomb(I, y_old)
# C: duplicates with a purged and a superseded member.
L = Log("dup-purged"); I = Ident(L, 3)
ref = "hmac-sha256:" + "ab" * 32
src = {"system": "csv", "adapter": "craft@1", "ref": ref}
d1 = I.claim(1, source=src); d2 = I.claim(1, source=src); d3 = I.claim(1, source=src)
tomb(I, min(d1, d2, d3)); sup(I, sorted([d1, d2, d3])[1], 9)
# D: unresolved subject and author, bare-key author, bare key with wrong kid.
L = Log("unresolved"); I = Ident(L, 4)
I.claim(1, subject="sha256:" + "1" * 64)
bare = Key(b"\x55" * 32)
L.put(rec(bare, type="hr", subject=I.id, author=bare.pub, value={"n": 1}))
L.put(rec(bare, type="hr", subject=I.id, author=Key(b"\x56" * 32).pub, value={"n": 2}))
L.put(rec(bare, type="hr", subject=I.id, author="sha256:" + "2" * 64, value={"n": 3}))
# E: unknown v: a claim (skip) and a core record of the identity (too new), plus a stranger's v1 genesis-less core record.
L = Log("too-new"); I = Ident(L, 5); I.claim(1)
J = Ident(L, 6); J.claim(2)
r = rec(J.key, type="hr", subject=J.id, author=J.id, value={"n": 1}); r2 = dict(r); r2["v"] = 1; r2.pop("id"); r2.pop("sig"); r2["id"] = record_id(r2); r2["sig"] = J.key.sign(r2["id"]); L.put(r2)
r = rec(I.key, type="bean.tombstone", subject=I.id, author=I.id, refs=[{"rel": "purges", "id": I.id}]); r.pop("id"); r.pop("sig"); r["v"] = 2; r["id"] = record_id(r); r["sig"] = I.key.sign(r["id"]); L.put(r)
st = Key(b"\x57" * 32); r = rec(st, type="bean.whatever", subject=J.id, author=J.id); r.pop("id"); r.pop("sig"); r["v"] = 3; r["id"] = record_id(r); r["sig"] = st.sign(r["id"]); L.put(r)
# E2 (K1): a v9 core record by a rotated-out key and by a stranger is ungated;
# one by the next recovery key (a newer rotation) halts the identity.
def future(ident, key, t="bean.tombstone"):
    r = rec(key, type=t, subject=ident.id, author=ident.id, refs=[{"rel": "purges", "id": ident.id}]); r.pop("id"); r.pop("sig"); r["v"] = 9
    r["id"] = record_id(r); r["sig"] = key.sign(r["id"]); return ident.log.put(r)
L = Log("too-new-retired"); I = Ident(L, 8); c = I.claim(1); I.rotate([c]); I.claim(2)
future(I, I.oldkey); future(I, Key(b"\x58" * 32))
L = Log("too-new-recovery"); I = Ident(L, 9); I.claim(1); future(I, I.rec(0), "bean.succession"); future(I, Key(b"\x59" * 32))  # ungated, but I is too new: no warning
# F: succession errors: bad succession, key reuse, recovery not advanced, held succession, unknown core type.
L = Log("succ-errors"); I = Ident(L, 7)
I.rotate([], parent=I.id, newkey=Key(b"\x60" * 32), gen=1)          # wrong recovery generation
I.rotate([], parent=I.id, newkey=I.key)                               # key reuse
r = rec(I.rec(0), type="bean.succession", subject=I.id, author=I.id, refs=[{"rel": "succeeds", "id": I.id}], value={"key": Key(b"\x61" * 32).pub, "recovery": I.chain[0]["value"]["recovery"], "sealed": []}); L.put(r)
I.rotate([], parent="sha256:" + "3" * 64, newkey=Key(b"\x62" * 32))  # held
L.put(rec(I.key, type="bean.thing", subject=I.id, author=I.id))
# G: numbers and canonical form.
L = Log("numbers"); I = Ident(L, 8)
for v in [1e21, 1e-7, 0.1, -5, 2**53 - 1, 123456789012.5, 5e-324]:
    I.claim(v)
r = rec(I.key, type="hr", subject=I.id, author=I.id, value={"n": 2**53}); L.put(r)
r = rec(I.key, type="hr", subject=I.id, author=I.id, value={"n": 1}); L.put(r, raw=json.dumps(r, sort_keys=True).encode())
r = rec(I.key, type="hr", subject=I.id, author=I.id, value={"n": 1}); L.put(r, raw=b"\xef\xbb\xbf" + canonical(r).encode())
r = rec(I.key, type="hr", subject=I.id, author=I.id, value={"s": "\ud800x"}) if False else None
raw = canonical(rec(I.key, type="hr", subject=I.id, author=I.id, value={"s": "x"})).replace('"x"', '"\\ud800"')
L.raw("objects/ab/" + "c" * 62 + ".json", raw.encode())
raw = canonical(rec(I.key, type="hr", subject=I.id, author=I.id, value={"n": 1})).replace('"n":1', '"n":1,"n":2')
L.raw("objects/ab/" + "d" * 62 + ".json", raw.encode())
# H: seal parts: duplicate slot, wrong kid, held, part above n, complete set of another chain node.
L = Log("parts-edge"); I = Ident(L, 9)
cs = sorted(I.claim(i) for i in range(5))
sid = I.rotate(cs[:1], parts=2)
rk = I.rec(0)
def part(k, ids, key=rk, target=None, subject=None):
    return L.put(rec(key, type="bean.seal", subject=subject or I.id, author=subject or I.id, refs=[{"rel": "seals", "id": target or sid}], value={"part": k, "sealed": sorted(ids)}))
part(1, cs[1:3]); part(2, cs[3:5]); part(2, cs[3:4], key=I.oldkey); part(3, cs[4:]); part(1, cs[4:], target="sha256:" + "4" * 64)
# I: stranger tombstone and supersede on someone else's claim; tombstone of a genesis; tombstone with a long reason.
L = Log("authority"); I = Ident(L, 10); J = Ident(L, 11)
c = I.claim(1); tomb(J, c, subject=J.id); sup(J, c, 2)
L.put(rec(J.key, type="bean.tombstone", subject=I.id, author=J.id, refs=[{"rel": "purges", "id": c}]))
tomb(I, I.id); L.put(rec(I.key, type="bean.tombstone", subject=I.id, author=I.id, refs=[{"rel": "purges", "id": c}], context={"reason": "x" * 201}))
# J: times edge cases.
L = Log("times"); I = Ident(L, 12)
I.claim(1, times={"observed": {"start": "2026-10-20T07:00:00.123+02:00", "end": "2026-10-20T05:00:00.124Z", "precision": "millisecond"}, "imported": "2026-10-30T12:00:00Z"})
I.claim(2, times={"observed": {"start": "2026-10-20T07:00:00Z", "end": "2026-10-20T06:59:59Z", "precision": "second"}, "imported": "2026-10-30T12:00:00Z"})
I.claim(3, times={"observed": {"start": "2026-02-30T07:00:00Z", "precision": "day"}, "imported": "2026-10-30T12:00:00Z"})
I.claim(4, times={"observed": None, "imported": "2026-10-30T12:00:00.5Z"})
I.claim(5, times={"observed": {"start": "2026-10-20", "precision": "day"}, "imported": "2026-10-30T12:00:00Z"})
I.claim(6, times={"observed": {"start": "2026-10-20T07:00:00Z", "precision": "week"}, "imported": "2026-10-30T12:00:00Z"})
I.claim(7, times={"observed": {"start": "2026-10-20T07:00:00Z", "precision": "day", "tz": "x"}, "imported": "2026-10-30T12:00:00Z"})
# K: refs edge cases.
L = Log("refs"); I = Ident(L, 13); c = I.claim(1)
I.claim(2, refs=[{"rel": "cites", "id": c}])
I.claim(3, refs=[{"rel": "about", "id": c}, {"rel": "about", "id": c}])
I.claim(4, refs=[{"rel": "succeeds", "id": c}])
I.claim(5, refs=[{"rel": "about", "id": c, "note": "x"}])
I.claim(6, value={"blob": "sha256:" + "5" * 64})
I.claim(7, value={"blob": "sha256:" + "5" * 64}, blobs=[{"hash": "sha256:" + "5" * 64, "role": "raw", "media_type": "text/csv"}])
I.claim(8, type="Bad.Type")
I.claim(9, extra_top={"k": 1})
I.claim(10, refs=[{"rel": "supersedes", "id": c}, {"rel": "supersedes", "id": "sha256:" + "6" * 64}])
print("crafted", len(os.listdir(OUT)), "logs in", OUT)
