#!/usr/bin/env python3
"""Independent check of vectors V1 to V12 (SPEC.md section 14).

This script shares no code with the Go implementation. It uses the Python
standard library and the `cryptography` package. Run from the repo root:

    python3 -I tools/checkvectors.py
"""
import hashlib
import hmac
import json
import math
import sys
from decimal import Decimal

from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, ed25519, utils
from cryptography.hazmat.primitives.ciphers.aead import AESSIV
from cryptography.hazmat.primitives.kdf.hkdf import HKDF

MAX_SAFE = 2**53 - 1


def raw_pub(pub):
    """Raw public key bytes; works before cryptography 40 added public_bytes_raw."""
    return pub.public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw)

P256_N = 0xFFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551
failures = 0


def check(name, got, want):
    global failures
    if got != want:
        failures += 1
        print(f"FAIL {name}\n  got  {got!r}\n  want {want!r}")
    else:
        print(f"ok   {name}")


# ---- JCS --------------------------------------------------------------

class BadJSON(Exception):
    pass


def parse(text):
    """Strict parse. Returns (value, error_string)."""
    dup = []

    def pairs(items):
        keys = [k for k, _ in items]
        if len(keys) != len(set(keys)):
            dup.append(True)
        return dict(items)

    def const(s):
        raise BadJSON(s)

    try:
        v = json.loads(text, object_pairs_hook=pairs, parse_constant=const)
    except (ValueError, BadJSON, RecursionError):
        return None, "bad json"
    if dup:
        return None, "duplicate key"

    def walk(x):
        if isinstance(x, bool) or x is None:
            return None
        if isinstance(x, (int, float)):
            f = float(x)
            if math.isinf(f) or math.isnan(f):
                return "bad number"
            if f == math.trunc(f) and abs(f) < 1e21 and abs(f) > MAX_SAFE:
                return "bad number"
            return None
        if isinstance(x, str):
            if any(0xD800 <= ord(c) <= 0xDFFF for c in x):
                return "bad string"
            return None
        if isinstance(x, list):
            errs = [walk(e) for e in x]
        else:
            errs = [walk(e) for e in x.values()]
        for want in ("bad number", "bad string"):
            if want in errs:
                return want
        return None

    err = walk(v)
    if err:
        return None, err
    return v, None


def es_number(f):
    f = float(f)
    if f == 0:
        return "0"
    sign = "-" if f < 0 else ""
    d = Decimal(repr(abs(f))).as_tuple()
    digits = "".join(str(x) for x in d.digits).rstrip("0") or "0"
    # strip trailing zeros adjusts exponent
    stripped = len(d.digits) - len(digits)
    k = len(digits)
    n = k + d.exponent + stripped
    if k <= n <= 21:
        s = digits + "0" * (n - k)
    elif 0 < n <= 21:
        s = digits[:n] + "." + digits[n:]
    elif -6 < n <= 0:
        s = "0." + "0" * (-n) + digits
    else:
        e = n - 1
        s = digits[0] + ("." + digits[1:] if k > 1 else "") + "e" + ("+" if e >= 0 else "-") + str(abs(e))
    return sign + s


def es_string(s):
    out = ['"']
    for c in s:
        if c == '"':
            out.append('\\"')
        elif c == "\\":
            out.append("\\\\")
        elif c == "\b":
            out.append("\\b")
        elif c == "\t":
            out.append("\\t")
        elif c == "\n":
            out.append("\\n")
        elif c == "\f":
            out.append("\\f")
        elif c == "\r":
            out.append("\\r")
        elif ord(c) < 0x20:
            out.append("\\u%04x" % ord(c))
        else:
            out.append(c)
    out.append('"')
    return "".join(out)


def canonical(v):
    if v is None:
        return "null"
    if v is True:
        return "true"
    if v is False:
        return "false"
    if isinstance(v, (int, float)):
        return es_number(v)
    if isinstance(v, str):
        return es_string(v)
    if isinstance(v, list):
        return "[" + ",".join(canonical(e) for e in v) + "]"
    keys = sorted(v.keys(), key=lambda k: k.encode("utf-16-be"))
    return "{" + ",".join(es_string(k) + ":" + canonical(v[k]) for k in keys) + "}"


def cbytes(v):
    return canonical(v).encode("utf-8")


def load(name):
    with open("vectors/" + name, encoding="utf-8") as f:
        return json.load(f)


def check_jcs(name):
    vec = load(name)
    for c in vec["cases"]:
        v, err = parse(c["input"])
        if "error" in c:
            check(f"{name} {c['input'][:30]!r} error", err, c["error"])
        else:
            check(f"{name} {c['input'][:30]!r}", (err, canonical(v) if err is None else None), (None, c["canonical"]))


check_jcs("v1-jcs-numbers.json")
check_jcs("v2-jcs-key-order.json")
check_jcs("v3-jcs-strings.json")

# ---- ids, keys, signatures ------------------------------------------


def record_id(rec):
    body = {k: v for k, v in rec.items() if k not in ("id", "sig")}
    return "sha256:" + hashlib.sha256(cbytes(body)).hexdigest(), cbytes(body)


def id_raw(i):
    return bytes.fromhex(i[7:])


def verify(kid, rid, sig):
    alg, hx = kid.split(":")
    salg, shx = sig.split(":")
    if alg != salg or len(shx) != 128 or shx != shx.lower():
        return "bad signature"
    raw = bytes.fromhex(shx)
    msg = id_raw(rid)
    try:
        if alg == "ed25519":
            ed25519.Ed25519PublicKey.from_public_bytes(bytes.fromhex(hx)).verify(raw, msg)
        elif alg == "p256":
            r, s = int.from_bytes(raw[:32], "big"), int.from_bytes(raw[32:], "big")
            if s > P256_N // 2:
                return "bad signature"
            pub = ec.EllipticCurvePublicKey.from_encoded_point(ec.SECP256R1(), bytes.fromhex(hx))
            pub.verify(utils.encode_dss_signature(r, s), msg, ec.ECDSA(hashes.SHA256()))
        else:
            return "unsupported key algorithm"
    except InvalidSignature:
        return "bad signature"
    return "accept"


v4 = load("v4-record-id.json")
for r in v4["records"]:
    rec = r["record"]
    rid, body = record_id(rec)
    check(f"v4 {r['name']} id_bytes", body.decode(), r["id_bytes"])
    check(f"v4 {r['name']} id", rid, r["id"])
    check(f"v4 {r['name']} file_bytes", canonical(rec), r["file_bytes"])
    check(f"v4 {r['name']} sig", verify(rec["kid"], rec["id"], rec["sig"]), "accept")

v5 = load("v5-ed25519.json")
priv = ed25519.Ed25519PrivateKey.from_private_bytes(bytes.fromhex(v5["private_seed"]))
check("v5 kid", "ed25519:" + raw_pub(priv.public_key()).hex(), v5["kid"])
check("v5 sig", "ed25519:" + priv.sign(id_raw(v5["id"])).hex(), v5["sig"])

v6 = load("v6-p256.json")
pk = ec.derive_private_key(int(v6["private_scalar"], 16), ec.SECP256R1())
from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat
check("v6 kid", "p256:" + pk.public_key().public_bytes(Encoding.X962, PublicFormat.CompressedPoint).hex(), v6["kid"])
check("v6 low-S", verify(v6["kid"], v6["id"], v6["low_s_sig"]), "accept")
check("v6 high-S", verify(v6["kid"], v6["id"], v6["high_s_sig"]), "bad signature")
# The high-S twin must still be a mathematically valid ECDSA signature.
raw = bytes.fromhex(v6["high_s_sig"][5:])
pk.public_key().verify(utils.encode_dss_signature(int.from_bytes(raw[:32], "big"), int.from_bytes(raw[32:], "big")), id_raw(v6["id"]), ec.ECDSA(hashes.SHA256()))
print("ok   v6 high-S twin is a valid ECDSA signature (rejected only by low-S rule)")
# The reference signer uses RFC 6979 nonces, so low_s_sig is reproducible.
try:
    der = pk.sign(id_raw(v6["id"]), ec.ECDSA(hashes.SHA256(), deterministic_signing=True))
    r6, s6 = utils.decode_dss_signature(der)
    n256 = 0xFFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551
    if s6 > n256 // 2:
        s6 = n256 - s6
    check("v6 RFC 6979 low-S sig", "p256:" + r6.to_bytes(32, "big").hex() + s6.to_bytes(32, "big").hex(), v6["low_s_sig"])
except TypeError:
    print("skip v6 RFC 6979: this cryptography has no deterministic_signing")

v7 = load("v7-commitment.json")
check("v7 commitment", "sha256:" + hashlib.sha256(v7["tagged_key"].encode("ascii")).hexdigest(), v7["commitment"])

# ---- BIP-39 -----------------------------------------------------------

with open("core/bip39-english.txt", encoding="utf-8") as f:
    words = f.read().split("\n")[:2048]
check("bip39 wordlist sha256", hashlib.sha256(open("core/bip39-english.txt", "rb").read()).hexdigest(),
      "2f5eed53a4727b4bf8880d8f3f199efc90e58503646d9ff8eff3a2ed3b24dbda")
v8 = load("v8-paper-seed.json")
ent = bytes.fromhex(v8["entropy"])
bits = bin(int.from_bytes(ent, "big"))[2:].zfill(256) + bin(hashlib.sha256(ent).digest()[0])[2:].zfill(8)
check("v8 words", " ".join(words[int(bits[i * 11:(i + 1) * 11], 2)] for i in range(24)), v8["words"])

# ---- HKDF -------------------------------------------------------------


def hk(ikm, info, n):
    return HKDF(algorithm=hashes.SHA256(), length=n, salt=None, info=info.encode("ascii")).derive(ikm)


v9 = load("v9-hkdf.json")
seed = bytes.fromhex(v9["paper_seed"])
isec = hk(seed, "bean/v0/identity-secret", 32)
check("v9 identity_secret", isec.hex(), v9["identity_secret"])
for g in range(3):
    rs = hk(seed, f"bean/v0/recovery-key/{g}", 32)
    check(f"v9 recovery seed {g}", rs.hex(), v9["recovery_private_seeds"][g])
    pub = raw_pub(ed25519.Ed25519PrivateKey.from_private_bytes(rs).public_key()).hex()
    check(f"v9 recovery key {g}", "ed25519:" + pub, v9["recovery_keys"][g])

v10 = load("v10-derived-keys.json")
blob_key = hk(isec, "bean/v0/blob-key", 64)
dedupe_key = hk(isec, "bean/v0/dedupe-key", 32)
check("v10 blob_key", blob_key.hex(), v10["blob_key"])
check("v10 dedupe_key", dedupe_key.hex(), v10["dedupe_key"])

# ---- AES-SIV ----------------------------------------------------------

v11 = load("v11-aes-siv.json")
siv = AESSIV(blob_key)
for b in v11["blobs"]:
    if b["name"] == "1mib":
        pt = bytes.fromhex("deadbeef") * 262144
    else:
        pt = bytes.fromhex(b.get("plaintext_hex", ""))
    out = siv.encrypt(pt, [v11["associated_data"].encode("ascii")])
    if "output_hex" in b:
        check(f"v11 {b['name']} output", out.hex(), b["output_hex"])
    check(f"v11 {b['name']} sha256", hashlib.sha256(out).hexdigest(), b["output_sha256"])
    check(f"v11 {b['name']} blob_id", "sha256:" + hashlib.sha256(out).hexdigest(), b["blob_id"])

# ---- dedupe refs ------------------------------------------------------

v12 = load("v12-dedupe-ref.json")
for c in v12["cases"]:
    cb = cbytes(c["input"])
    check(f"v12 input_canonical {cb[:40]!r}", cb.decode(), c["input_canonical"])
    check(f"v12 ref {cb[:40]!r}", "hmac-sha256:" + hmac.new(dedupe_key, cb, hashlib.sha256).hexdigest(), c["ref"])

# ---- V17 bad files (parse and envelope level) -------------------------

import re
v17 = load("v17-bad-files.json")
for c in v17["cases"]:
    want = c["error"]
    data, path = c["bytes"], c["path"]
    m = re.match(r"^objects/([0-9a-f]{2})/([0-9a-f]{62})\.json$", path)
    if not m:
        got = "bad path"
    else:
        v, err = parse(data)
        if err:
            got = err
        elif not isinstance(v, dict) or not isinstance(v.get("v"), (int, float)) or isinstance(v.get("v"), bool):
            got = "bad v"
        elif not isinstance(v.get("type"), str):
            got = "bad type"
        else:
            rid, _ = record_id(v)
            if v.get("id") != rid or rid != "sha256:" + m[1] + m[2]:
                got = "bad field: author" if not isinstance(v.get("author"), str) else "id mismatch"
            else:
                alg = v["kid"].split(":")[0]
                hx = v["kid"].split(":")[1]
                if alg not in ("ed25519", "p256"):
                    got = "unsupported key algorithm"
                elif (alg == "ed25519" and len(hx) != 64) or (alg == "p256" and len(hx) != 66):
                    got = "bad key"
                elif verify(v["kid"], rid, v["sig"]) != "accept":
                    got = "bad signature"
                elif canonical(v) != data:
                    got = "not canonical"
                else:
                    got = "accept"
    check(f"v17 {c['name']}", got, want)

# ---- verifier rules C26 and C27 (2026-10-30) --------------------------


def load_files(files):
    recs = {}
    for f in files:
        rec, err = parse(f["bytes"])
        check(f"file {f['path'][-12:]} parse", err, None)
        rid, _ = record_id(rec)
        check(f"file {f['path'][-12:]} id", rid, rec["id"])
        check(f"file {f['path'][-12:]} path", "objects/" + rid[7:9] + "/" + rid[9:] + ".json", f["path"])
        check(f"file {f['path'][-12:]} canonical", canonical(rec), f["bytes"])
        if verify(rec["kid"], rid, rec["sig"]) == "accept":
            recs[rid] = rec
    return recs


def chain_of(recs):
    """The chain of the one identity in recs: genesis first. Linear chains only."""
    (gid, g), = [(i, r) for i, r in recs.items() if r["type"] == "bean.genesis" and r["kid"] == r["value"]["key"] == r["author"]]
    chain = [(gid, g)]
    while True:
        head_id, head = chain[-1]
        kids = [(i, r) for i, r in recs.items() if r["type"] == "bean.succession" and r["author"] == r["subject"] == gid
                and r["refs"] == [{"rel": "succeeds", "id": head_id}]
                and "sha256:" + hashlib.sha256(r["kid"].encode("ascii")).hexdigest() == head["value"]["recovery"]]
        if not kids:
            return gid, chain
        assert len(kids) == 1, "fork in a vector"
        chain.append(kids[0])


v19 = load("v19-retired-key-seals.json")
recs = load_files(v19["files"])
gid, chain = chain_of(recs)
pos = {node["value"]["key"]: p for p, (_, node) in enumerate(chain)}
for e in v19["expected"]:
    r = recs[e["id"]]
    if r["type"] in ("bean.genesis", "bean.succession"):
        got = "valid" if any(i == e["id"] for i, _ in chain) else "invalid"
    else:
        p = pos.get(r["kid"])
        if p is None:
            got = "key not in chain"
        elif p < len(chain) - 1 and e["id"] not in chain[p + 1][1]["value"]["sealed"]:
            got = "not sealed"
        else:
            got = "valid"
    check(f"v19 {e['name']}", got, e["result"])

v20 = load("v20-duplicates-per-author.json")
recs = load_files(v20["files"])
gid, chain = chain_of(recs)
groups = {}
for i, r in sorted(recs.items()):
    if r["type"].startswith("bean.") or r["source"]["ref"] is None:
        continue
    groups.setdefault((r["subject"], r["author"], r["source"]["system"], r["source"]["ref"]), []).append(i)
dups = sorted([{"id": i, "primary": g[0]} for g in groups.values() for i in g[1:]], key=lambda d: d["id"])
check("v20 duplicates", json.dumps(dups, sort_keys=True), json.dumps(sorted(v20["expected_duplicates"], key=lambda d: d["id"]), sort_keys=True))

print()
print("FAILURES:", failures)
sys.exit(1 if failures else 0)
