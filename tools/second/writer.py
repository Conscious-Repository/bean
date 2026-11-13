#!/usr/bin/env python3
"""writer.py: an independent Python writer of bean records (SPEC 4.x):
keys, signatures, an identity with its key changes, and a folder of record
files. craft.py and forgereply.py write their cases with it. It shares no
code with bean."""
import os, sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from beanverify import canonical, record_id, commitment
from cryptography.hazmat.primitives.asymmetric import ed25519
from cryptography.hazmat.primitives.kdf.hkdf import HKDF
from cryptography.hazmat.primitives import hashes, serialization

OUT = [None]  # the folder that holds one folder per case

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
        self.dir = os.path.join(OUT[0], name); os.makedirs(os.path.join(self.dir, "objects"))
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
