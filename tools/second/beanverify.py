#!/usr/bin/env python3
"""Independent minimal bean verifier, written only from CORE.md and SPEC.md
(v0.5 as merged in PR #2), without reading the Go code. Usage:
beanverify.py <log dir> prints the SPEC 12.1 verify lines and exits with
the 12.1 code. It does not read local/hidden, so it never prints
"hidden pending gc". Each place where the spec did not say what to do is
marked GUESS(n); SPEC.md now answers them (DECISIONS 134), except
GUESS(2), the Ed25519 point rules (SPEC 17 item 11)."""
import hashlib, json, os, re, stat, sys
from datetime import datetime
from decimal import Decimal
from cryptography.hazmat.primitives.asymmetric import ed25519, ec
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric.utils import encode_dss_signature
from cryptography.exceptions import InvalidSignature

MAXDEPTH = 256
MAXINT = 2**53 - 1
MIB = 1 << 20
ID_RE = re.compile(r"^sha256:[0-9a-f]{64}$")
TYPE_RE = re.compile(r"^[a-z0-9][a-z0-9_-]*(\.[a-z0-9_-]+)*$")
PATH_RE = re.compile(r"^objects/([0-9a-f]{2})/([0-9a-f]{62})\.json$")
REF_RE = re.compile(r"^hmac-sha256:[0-9a-f]{64}$")
IMPORTED_RE = re.compile(r"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$")
P256_N = 0xFFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551

class Fail(Exception):
    pass

# ---------- JSON with the checks of 3.8 steps 3 to 6 ----------
class Num(float):
    pass

def parse(data):
    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError:
        raise Fail("bad json")
    if text.startswith("\ufeff"):
        raise Fail("bad json")  # GUESS(1): a BOM is not "one JSON object"
    dup = []
    badnum = []
    def pairs(items):
        keys = [k for k, _ in items]
        if len(keys) != len(set(keys)):
            dup.append(True)
        return dict(items)
    def pint(s):
        f = float(s)
        if abs(f) > MAXINT:
            badnum.append(s)
        return Num(f)
    def pfloat(s):
        f = float(s)
        if f in (float("inf"), float("-inf")):
            badnum.append(s)
            return Num(0)
        if es_number(f).lstrip("-").isdigit() and abs(f) > MAXINT:
            badnum.append(s)  # an integer by canonical form, out of range
        return Num(f)
    def pconst(s):
        raise Fail("bad json")
    try:
        obj = json.loads(text, object_pairs_hook=pairs, parse_int=pint, parse_float=pfloat, parse_constant=pconst)
    except Fail:
        raise
    except (ValueError, RecursionError):
        raise Fail("bad json")
    if not isinstance(obj, dict):
        raise Fail("bad json")
    # 3.8 step 3: the record object is depth 0; a value deeper than 256 is bad json.
    stack = [(obj, 0)]
    while stack:
        x, d = stack.pop()
        if d > MAXDEPTH:
            raise Fail("bad json")
        if isinstance(x, dict):
            stack.extend((v, d + 1) for v in x.values())
        elif isinstance(x, list):
            stack.extend((v, d + 1) for v in x)
    if dup:
        raise Fail("duplicate key")
    if badnum:
        raise Fail("bad number")
    def walk(x):
        if isinstance(x, str):
            if any(0xD800 <= ord(c) <= 0xDFFF for c in x):
                raise Fail("bad string")
        elif isinstance(x, dict):
            for k, v in x.items():
                walk(k); walk(v)
        elif isinstance(x, list):
            for v in x:
                walk(v)
    walk(obj)
    return obj

def es_number(f):
    if f == 0:
        return "0"
    if f != f or f in (float("inf"), float("-inf")):
        raise Fail("bad number")
    sign = "-" if f < 0 else ""
    d = Decimal(repr(abs(f)))
    _, digits, exp = d.normalize().as_tuple()
    ds = "".join(map(str, digits))
    k = len(ds)
    n = exp + k
    if k <= n <= 21:
        s = ds + "0" * (n - k)
    elif 0 < n <= 21:
        s = ds[:n] + "." + ds[n:]
    elif -6 < n <= 0:
        s = "0." + "0" * (-n) + ds
    else:
        e = n - 1
        s = ds[0] + ("." + ds[1:] if k > 1 else "") + "e" + ("+" if e > 0 else "-") + str(abs(e))
    return sign + s

def es_string(s):
    out = ['"']
    for c in s:
        o = ord(c)
        if c == '"': out.append('\\"')
        elif c == "\\": out.append("\\\\")
        elif c == "\b": out.append("\\b")
        elif c == "\t": out.append("\\t")
        elif c == "\n": out.append("\\n")
        elif c == "\f": out.append("\\f")
        elif c == "\r": out.append("\\r")
        elif o < 0x20: out.append("\\u%04x" % o)
        else: out.append(c)
    out.append('"')
    return "".join(out)

def canonical(v):
    if v is None: return "null"
    if v is True: return "true"
    if v is False: return "false"
    if isinstance(v, (int, float)): return es_number(float(v))
    if isinstance(v, str): return es_string(v)
    if isinstance(v, list): return "[" + ",".join(canonical(x) for x in v) + "]"
    if isinstance(v, dict):
        keys = sorted(v, key=lambda k: k.encode("utf-16-be"))
        return "{" + ",".join(es_string(k) + ":" + canonical(v[k]) for k in keys) + "}"
    raise TypeError(type(v))

def is_int(v):
    return isinstance(v, float) and not isinstance(v, bool) and v == int(v) and "e" not in es_number(v) and "." not in es_number(v)

def record_id(obj):
    c = {k: v for k, v in obj.items() if k not in ("id", "sig")}
    return "sha256:" + hashlib.sha256(canonical(c).encode("utf-8")).hexdigest()

# ---------- keys and signatures (4.4, 4.5) ----------
SP = 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEFFFFFC2F
SN = 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141
SG = (0x79BE667EF9DCBBAC55A06295CE870B07029BFCDB2DCE28D959F2815B16F81798,
      0x483ADA7726A3C4655DA4FBFC0E1108A8FD17B448A68554199C47D08FFB10D4B8)
def _add(P, Q):
    if P is None: return Q
    if Q is None: return P
    if P[0] == Q[0] and (P[1] != Q[1] or P[1] == 0): return None
    if P == Q: lam = 3 * P[0] * P[0] * pow(2 * P[1], SP - 2, SP) % SP
    else: lam = (Q[1] - P[1]) * pow(Q[0] - P[0], SP - 2, SP) % SP
    x = (lam * lam - P[0] - Q[0]) % SP
    return (x, (lam * (P[0] - x) - P[1]) % SP)
def _mul(P, n):
    R = None
    while n:
        if n & 1: R = _add(R, P)
        P = _add(P, P); n >>= 1
    return R
def lift_x(x):
    if x >= SP: return None
    y2 = (pow(x, 3, SP) + 7) % SP
    y = pow(y2, (SP + 1) // 4, SP)
    if pow(y, 2, SP) != y2: return None
    return (x, y if y % 2 == 0 else SP - y)
def tagged(tag, m):
    t = hashlib.sha256(tag.encode()).digest()
    return hashlib.sha256(t + t + m).digest()
def schnorr_verify(msg, pub, sig):
    P = lift_x(int.from_bytes(pub, "big"))
    r = int.from_bytes(sig[:32], "big"); s = int.from_bytes(sig[32:], "big")
    if P is None or r >= SP or s >= SN: return False
    e = int.from_bytes(tagged("BIP0340/challenge", sig[:32] + pub + msg), "big") % SN
    R = _add(_mul(SG, s), _mul(P, SN - e))
    return R is not None and R[1] % 2 == 0 and R[0] == r

HEX = re.compile(r"^[0-9a-f]*$")
def parse_key(kid):
    if not isinstance(kid, str) or ":" not in kid:
        raise Fail("unsupported key algorithm")
    alg, h = kid.split(":", 1)
    if alg not in ("ed25519", "p256", "secp256k1"):
        raise Fail("unsupported key algorithm")
    want = {"ed25519": 64, "p256": 66, "secp256k1": 64}[alg]
    if len(h) != want or not HEX.fullmatch(h):
        raise Fail("bad key")
    b = bytes.fromhex(h)
    try:
        if alg == "ed25519":
            ed25519.Ed25519PublicKey.from_public_bytes(b)  # GUESS(2): which A encodings are invalid
        elif alg == "p256":
            if b[0] not in (2, 3): raise ValueError
            ec.EllipticCurvePublicKey.from_encoded_point(ec.SECP256R1(), b)
        else:
            if lift_x(int.from_bytes(b, "big")) is None: raise ValueError
    except ValueError:
        raise Fail("bad key")
    return alg, b

def check_sig(kid, rid, sig):
    alg, pub = parse_key(kid)
    msg = bytes.fromhex(rid[7:])
    if not isinstance(sig, str) or not sig.startswith(alg + ":"):
        raise Fail("bad signature")
    rest = sig[len(alg) + 1:]
    if alg == "secp256k1":
        m = re.fullmatch(r"^(0|[1-9][0-9]*):([0-9a-f]{128})$", rest)
        if not m or int(m.group(1)) > MAXINT: raise Fail("bad signature")
        created, hexsig = int(m.group(1)), m.group(2)
        wrapper = json.dumps([0, pub.hex(), created, 29944, [["alt", "bean record signature"]], rid], separators=(",", ":"))
        eid = hashlib.sha256(wrapper.encode()).digest()
        if not schnorr_verify(eid, pub, bytes.fromhex(hexsig)): raise Fail("bad signature")
        return
    if len(rest) != 128 or not HEX.fullmatch(rest): raise Fail("bad signature")
    sb = bytes.fromhex(rest)
    try:
        if alg == "ed25519":
            if int.from_bytes(sb[32:], "little") >= 2**252 + 27742317777372353535851937790883648493:
                raise Fail("bad signature")
            ed25519.Ed25519PublicKey.from_public_bytes(pub).verify(sb, msg)
        else:
            r, s = int.from_bytes(sb[:32], "big"), int.from_bytes(sb[32:], "big")
            if s > P256_N // 2 or r == 0 or s == 0: raise Fail("bad signature")
            k = ec.EllipticCurvePublicKey.from_encoded_point(ec.SECP256R1(), pub)
            k.verify(encode_dss_signature(r, s), msg, ec.ECDSA(hashes.SHA256()))
    except InvalidSignature:
        raise Fail("bad signature")

# ---------- fields (3.2 to 3.7, 3.3) ----------
def is_tagged_key(s):
    try:
        parse_key(s); return True
    except Fail:
        return False

def rfc3339(s):
    if not isinstance(s, str): return None
    m = re.fullmatch(r"^(\d{4}-\d{2}-\d{2})[Tt](\d{2}:\d{2}:\d{2})(\.\d+)?([Zz]|[+-]\d{2}:\d{2})$", s)
    if not m: return None
    try:
        frac = m.group(3) or ""
        tz = m.group(4).upper().replace("Z", "+00:00")
        return datetime.fromisoformat(m.group(1) + "T" + m.group(2) + (frac[:7] if frac else "") + tz)
    except ValueError:
        return None

def check_sealed_list(v, nonempty):
    if not isinstance(v, list) or any(not isinstance(x, str) or not ID_RE.fullmatch(x) for x in v):
        raise Fail("bad field: value.sealed")
    if v != sorted(set(v)) or (nonempty and not v):
        raise Fail("bad field: value.sealed")

def check_fields(r):
    # GUESS(3): the spec gives no order inside step 15 and no <name> for
    # most failures. Order here: 3.2 table order, then 3.3.
    if not isinstance(r.get("nonce"), str) or not re.fullmatch(r"^[0-9a-f]{32}$", r["nonce"]): raise Fail("bad field: nonce")
    t = r["type"]
    if not (1 <= len(t) <= 128) or not TYPE_RE.fullmatch(t): raise Fail("bad field: type")
    if t in ("hlog.genesis", "hlog.succession", "hlog.tombstone"): raise Fail("old format")  # 3.3
    if t.startswith("bean.") and t not in ("bean.genesis", "bean.succession", "bean.seal", "bean.tombstone"):
        raise Fail("unknown core type")
    if t == "bean.genesis":
        if "subject" in r: raise Fail("bad field: subject")
    elif not isinstance(r.get("subject"), str) or not ID_RE.fullmatch(r["subject"]): raise Fail("bad field: subject")
    a = r["author"]
    if not (ID_RE.fullmatch(a) or is_tagged_key(a)): raise Fail("bad field: author")
    if "value" in r and not isinstance(r["value"], dict): raise Fail("bad field: value")
    tm = r.get("times")
    if not isinstance(tm, dict) or "observed" not in tm or "imported" not in tm: raise Fail("bad field: times")
    imp = tm["imported"]
    if not isinstance(imp, str) or not IMPORTED_RE.fullmatch(imp) or rfc3339(imp) is None: raise Fail("bad field: times.imported")
    ob = tm["observed"]
    if ob is not None:
        if not isinstance(ob, dict) or "start" not in ob or "precision" not in ob: raise Fail("bad field: times.observed")
        st = rfc3339(ob["start"])
        if st is None: raise Fail("bad field: times.observed.start")
        if "end" in ob:
            en = rfc3339(ob["end"])
            if en is None or en < st: raise Fail("bad field: times.observed.end")
        if ob["precision"] not in ("year", "month", "day", "hour", "minute", "second", "millisecond"): raise Fail("bad field: times.observed.precision")
    src = r.get("source")
    if not isinstance(src, dict) or not all(k in src for k in ("system", "adapter", "ref")): raise Fail("bad field: source")
    if not isinstance(src["system"], str) or not 1 <= len(src["system"].encode("utf-8")) <= 64: raise Fail("bad field: source.system")  # 3.6: UTF-8 bytes
    if not isinstance(src["adapter"], str) or not re.fullmatch(r"[^@\t\n\f\r ]+@[^@\t\n\f\r ]+", src["adapter"]): raise Fail("bad field: source.adapter")  # 3.6: no @, space, TAB, LF, FF or CR
    if src["ref"] is not None and (not isinstance(src["ref"], str) or not REF_RE.fullmatch(src["ref"])): raise Fail("bad field: source.ref")
    refs = r.get("refs", [])
    if "refs" in r:
        if not isinstance(refs, list): raise Fail("bad field: refs")
        seen = set()
        for x in refs:
            if not isinstance(x, dict) or not isinstance(x.get("rel"), str) or not isinstance(x.get("id"), str) or not ID_RE.fullmatch(x["id"]):
                raise Fail("bad field: refs")
            # GUESS(5): 3.7 lists four rels; 3.3 and CORE 2.1 add `seals`.
            if x["rel"] not in ("succeeds", "supersedes", "purges", "about", "seals"): raise Fail("unknown rel")
            if (x["rel"], x["id"]) in seen: raise Fail("bad field: refs")
            seen.add((x["rel"], x["id"]))
            only = {"succeeds": "bean.succession", "purges": "bean.tombstone", "seals": "bean.seal"}
            if x["rel"] in only and t != only[x["rel"]]: raise Fail("bad field: refs")
    blobs = r.get("blobs")
    if "blobs" in r:
        if not isinstance(blobs, list): raise Fail("bad field: blobs")
        for b in blobs:
            if not isinstance(b, dict) or not isinstance(b.get("hash"), str) or not ID_RE.fullmatch(b["hash"]) or not isinstance(b.get("role"), str) or not isinstance(b.get("media_type"), str):
                raise Fail("bad field: blobs")
    if "context" in r and not isinstance(r["context"], dict): raise Fail("bad field: context")
    val = r.get("value")
    if not t.startswith("bean.") and isinstance(val, dict) and "blob" in val:
        if val["blob"] not in [b["hash"] for b in (blobs or [])]: raise Fail("blob not listed")
    selfsrc = src["system"] == "self" and src["ref"] is None
    if t == "bean.genesis":
        if not isinstance(val, dict) or set(val) != {"key", "recovery"} or not is_tagged_key(val["key"]) or not isinstance(val["recovery"], str) or not ID_RE.fullmatch(val["recovery"]): raise Fail("bad field: value")
        if not (a == r["kid"] == val["key"]): raise Fail("bad field: author")
        if refs: raise Fail("bad field: refs")
        if ob is not None: raise Fail("bad field: times.observed")
        if not selfsrc: raise Fail("bad field: source")
    elif t == "bean.succession":
        if r["subject"] != a or not ID_RE.fullmatch(a): raise Fail("bad field: author")
        if len(refs) != 1 or refs[0]["rel"] != "succeeds": raise Fail("bad field: refs")
        keys = {"key", "recovery", "sealed"}
        if isinstance(val, dict) and "parts" in val:
            keys.add("parts")
            p = val["parts"]
            if not is_int(p) or not 1 <= p <= 1000: raise Fail("bad field: value.parts")
        if not isinstance(val, dict) or set(val) != keys or not is_tagged_key(val["key"]) or not isinstance(val["recovery"], str) or not ID_RE.fullmatch(val["recovery"]): raise Fail("bad field: value")
        check_sealed_list(val["sealed"], False)
        # GUESS(6): 3.3 does not require times.observed null or a self source for a succession.
    elif t == "bean.seal":
        if r["subject"] != a or not ID_RE.fullmatch(a): raise Fail("bad field: author")
        if len(refs) != 1 or refs[0]["rel"] != "seals": raise Fail("bad field: refs")
        if not isinstance(val, dict) or set(val) != {"part", "sealed"} or not is_int(val["part"]) or not 1 <= val["part"] <= 1000: raise Fail("bad field: value")
        check_sealed_list(val["sealed"], True)
        if ob is not None: raise Fail("bad field: times.observed")
        if r.get("blobs"): raise Fail("bad field: blobs")
        if not selfsrc: raise Fail("bad field: source")
    elif t == "bean.tombstone":
        if r["subject"] != a: raise Fail("bad field: author")
        if len(refs) != 1 or refs[0]["rel"] != "purges": raise Fail("bad field: refs")
        if "value" in r: raise Fail("bad field: value")
        if r.get("blobs"): raise Fail("bad field: blobs")
        ctx = r.get("context") or {}
        if "reason" in ctx and (not isinstance(ctx["reason"], str) or len(ctx["reason"]) > 200): raise Fail("bad field: context.reason")

# ---------- one file (3.8) ----------
def check_file(path, data):
    if data is None or not PATH_RE.fullmatch(path): raise Fail("bad path")
    if len(data) > 16 * MIB: raise Fail("too large")
    r = parse(data)
    v = r.get("v")
    if not is_int(v) or v < 0: raise Fail("bad v")
    if not isinstance(r.get("type"), str): raise Fail("bad type")
    if r["type"] not in ("bean.succession", "bean.seal") and len(data) > MIB: raise Fail("too large")
    if not isinstance(r.get("author"), str): raise Fail("bad field: author")
    if not isinstance(r.get("kid"), str): raise Fail("bad field: kid")
    rid = record_id(r)
    m = PATH_RE.fullmatch(path)
    if r.get("id") != rid or rid != "sha256:" + m.group(1) + m.group(2): raise Fail("id mismatch")
    check_sig(r["kid"], rid, r.get("sig"))
    if v != 0:
        return r, "unknown v"
    if canonical(r).encode("utf-8") != data: raise Fail("not canonical")
    check_fields(r)
    return r, None

# ---------- the set (6.3) ----------
def commitment(k):
    return "sha256:" + hashlib.sha256(k.encode("ascii")).hexdigest()

def verify(files, blobfiles):
    errors, warnings = {}, {}
    def warn(i, s): warnings.setdefault(i, []).append(s)
    R, unknown = {}, {}
    for p, data in sorted(files.items()):
        try:
            r, st = check_file(p, data)
        except Fail as e:
            errors[p] = str(e)  # GUESS(7): `<id or path>`; the Go verifier uses the path for every file-level error
            continue
        if st == "unknown v": unknown[r["id"]] = r
        else: R[r["id"]] = r
    skipped = 0
    # Step 2 and 8.2: identities, too new.
    genesis = {i: r for i, r in R.items() if r["type"] == "bean.genesis"}
    def tree(gid):
        N = {gid: genesis[gid]}
        changed = True
        while changed:
            changed = False
            for i, s in sorted(R.items()):
                if s["type"] != "bean.succession" or s["subject"] != gid or s["author"] != gid or i in N or i in errors: continue
                pid = s["refs"][0]["id"]
                if pid not in N: continue
                P = N[pid]
                if commitment(s["kid"]) != P["value"]["recovery"]:
                    errors[i] = "bad succession"; continue
                path = []
                x = pid
                while True:
                    path.append(N[x])
                    if N[x]["type"] == "bean.genesis": break
                    x = N[x]["refs"][0]["id"]
                used = set()
                for n in path:
                    used.add(n["value"]["key"])
                    if n["type"] == "bean.succession": used.add(n["kid"])
                if s["value"]["key"] in used:
                    errors[i] = "key reuse"; continue
                if s["value"]["recovery"] == P["value"]["recovery"]:
                    errors[i] = "recovery not advanced"; continue
                N[i] = s; changed = True
        return N
    trees = {g: tree(g) for g in genesis}
    too_new = set(); ungated = []
    for i, r in unknown.items():
        skipped += 1
        if not r["type"].startswith("bean."): continue
        if r["type"] == "bean.genesis":
            if r["author"] == r["kid"]: too_new.add(i)
            continue
        T = trees.get(r["author"])
        if T is None: continue
        # 8.2 (K1): the head key, or a recovery key that a chain node commits to.
        x, gate = genesis[r["author"]], False
        while True:
            if commitment(r["kid"]) == x["value"]["recovery"]: gate = True; break
            kids = [n for n in T.values() if n["type"] == "bean.succession" and n["refs"][0]["id"] == x["id"]]
            if len(kids) != 1: gate = r["kid"] == x["value"]["key"]; break
            x = kids[0]
        if gate: too_new.add(r["author"])
        else: ungated.append((i, r["author"]))
    for i, a in ungated:
        if a not in too_new: warn(i, "ungated: " + i)  # 8.2: nothing of a too-new identity is reported
    for i, r in list(R.items()):
        if r["author"] in too_new or r.get("subject") in too_new or i in too_new:
            del R[i]; skipped += 1  # GUESS(8): records of a too-new identity count as skipped
            errors.pop(i, None); warnings.pop(i, None)  # 8.2 item 2: step 3 lines of it are not reported
    for g in too_new:
        genesis.pop(g, None); trees.pop(g, None)
    for i, s in R.items():
        if s["type"] == "bean.succession" and i not in errors and not any(i in T for T in trees.values()):
            warn(i, "held: " + i)
    # Step 5: chains.
    chains, frozen, offchain_keys = {}, set(), {}
    for g, N in trees.items():
        X, chain = g, [g]
        while True:
            C = sorted(i for i, n in N.items() if n["type"] == "bean.succession" and n["refs"][0]["id"] == X)
            if not C: break
            if len(C) > 1:
                frozen.add(g); break
            X = C[0]; chain.append(X)
        chains[g] = chain
        for i in N:
            if i not in chain:
                errors[i] = "frozen"
                offchain_keys.setdefault(g, set()).add(N[i]["value"]["key"])
    # Step 5b: seal sets.
    on_chain = {i for c in chains.values() for i in c[1:]}
    slots = {}
    for i, p in sorted(R.items()):
        if p["type"] != "bean.seal" or i in errors: continue
        sid = p["refs"][0]["id"]
        s = R.get(sid)
        if s is None:
            warn(i, "held: " + i); continue
        if s["type"] != "bean.succession" or sid not in on_chain or p["subject"] != s["subject"] or p["kid"] != s["kid"] or p["value"]["part"] > s["value"].get("parts", 0):
            errors[i] = "bad seal"; continue
        slots.setdefault((sid, int(p["value"]["part"])), []).append(i)
    sealset, incomplete = {}, []
    for g, c in chains.items():
        for sid in c[1:]:
            s = R[sid]
            ids, ok = set(s["value"]["sealed"]), True
            for k in range(1, int(s["value"].get("parts", 0)) + 1):
                got = slots.get((sid, k), [])
                if len(got) != 1:
                    ok = False
                    for x in got: errors[x] = "bad seal"
                    continue
                ids |= set(R[got[0]]["value"]["sealed"])
            if not ok:
                incomplete.append(sid); warn(sid, "incomplete seals: " + sid); ids = set()
            sealset[sid] = ids
    # Steps 6 and 7.
    identity_types = ("bean.genesis", "bean.succession", "bean.seal")
    for i, d in sorted(R.items()):
        if d["type"] in identity_types or i in errors: continue
        a = d["author"]
        if not ID_RE.fullmatch(a):
            if d["kid"] != a: errors[i] = "bad signature"
            continue
        if a not in genesis:
            warn(i, "unresolved author: " + a); errors.setdefault(i, None); continue  # GUESS(9): neither valid nor an error
        c = chains[a]
        pos = [p for p, nid in enumerate(c) if R[nid]["value"]["key"] == d["kid"]]
        if not pos:
            errors[i] = "frozen" if d["kid"] in offchain_keys.get(a, ()) else "key not in chain"; continue
        p = pos[0]
        if p < len(c) - 1 and i not in sealset[c[p + 1]]:
            errors[i] = "not sealed"
    # Step 8: frozen identities.
    def destructive(r):
        return r["type"] == "bean.tombstone" or any(x["rel"] == "supersedes" for x in r.get("refs", []))
    for i, d in R.items():
        if i in errors: continue
        if d["author"] in frozen and destructive(d): errors[i] = "frozen"
    valid = lambda i: i in R and i not in errors
    # Step 10 before step 9: a tombstone of a supersede removes it (6.6 item 6). GUESS(10)
    purged = set()
    for i, d in sorted(R.items()):
        if d["type"] != "bean.tombstone" or not valid(i): continue
        if d["author"] != d["subject"]: errors[i] = "not subject"; continue
        tid = d["refs"][0]["id"]
        T = R.get(tid)
        if T is None:
            warn(i, "held: " + i); continue
        if T["type"] in identity_types + ("bean.tombstone",): errors[i] = "bad target"; continue
        if T.get("subject") != d["subject"]: errors[i] = "not subject"; continue
        purged.add(tid)
    superseded = {}
    for i, d in sorted(R.items()):
        tg = [x["id"] for x in d.get("refs", []) if x["rel"] == "supersedes"]
        if not tg or not valid(i): continue
        held = False
        bad = None
        for tid in tg:
            T = R.get(tid)
            if T is None: held = True; continue
            if T["type"].startswith("bean."): bad = "bad target"; break
            if T["author"] != d["author"]: bad = "not author"; break
        if bad: errors[i] = bad; continue
        if held: warn(i, "held: " + i)
        if i in purged: continue
        for tid in tg:
            if tid in R: superseded.setdefault(tid, []).append(i)
    for tid, sups in superseded.items():
        if len(sups) > 1:
            low = min(sups)
            for x in sups:
                if x != low: warn(x, "superseded by sibling: " + low)
    # Step 11.
    for i, d in sorted(R.items()):
        if valid(i) and d["type"] not in identity_types and d["type"] != "bean.tombstone" and d.get("subject") not in genesis:
            warn(i, "unresolved subject: " + d["subject"])
    # Step 12.
    groups = {}
    for i, d in sorted(R.items()):
        if not valid(i) or i in purged or d["source"]["ref"] is None or i in superseded: continue  # GUESS(11): purged excluded
        groups.setdefault((d.get("subject"), d["author"], d["source"]["system"], d["source"]["ref"]), []).append(i)
    dups = 0
    for g in groups.values():
        for x in g[1:]:
            warn(x, "duplicate: %s of %s" % (x, g[0])); dups += 1
    # Blobs (12.3 verify).
    present = set()
    for p, data in blobfiles.items():
        if data is None:
            errors[p] = "bad path"; continue
        m = re.fullmatch(r"^blobs/([0-9a-f]{2})/([0-9a-f]{62})$", p)
        bid = "sha256:" + m.group(1) + m.group(2) if m else None
        if bid is None or hashlib.sha256(data).hexdigest() != bid[7:]:
            errors[bid or p] = "blob corrupt: " + (bid or p); continue
        present.add(bid)
    for i, d in sorted(R.items()):
        for b in d.get("blobs", []) or []:
            if b["hash"] in present: continue
            if i in purged or not valid(i): warn(i, "blob missing: " + b["hash"])
            else: errors[i] = "blob missing: " + b["hash"]
    lines = []
    for i, e in errors.items():
        if e: lines.append("error: %s: %s" % (i, e))
    for i, ws in warnings.items():
        for w in ws: lines.append("warning: %s: %s" % (i, w))
    for g in sorted(too_new): lines.append("too new: " + g)
    for g in sorted(frozen): lines.append("compromised: " + g)
    nvalid = sum(1 for i in R if valid(i) and i not in purged)  # GUESS(12): the summary is silent on purged targets
    ninvalid = sum(1 for e in errors.values() if e)
    nheld = sum(1 for ws in warnings.values() for w in ws if w.startswith("held: "))
    lines.sort()
    lines.append("summary: %d valid, %d invalid, %d skipped, %d held, %d duplicate" % (nvalid, ninvalid, skipped, nheld, dups))
    code = 1 if ninvalid or incomplete else 0
    return lines, code, {"valid": {i for i in R if valid(i)}, "errors": {i: e for i, e in errors.items() if e}, "purged": purged}

def load_dir(d):
    """The plain files exactly one directory below objects/ and blobs/
    (SPEC 7.1). Names that start with "." are not part of the log. Any
    other entry (a file directly in objects/, a symlink, a directory one
    level down) maps to None: a bad path, not read."""
    files, blobs = {}, {}
    for sub, out in (("objects", files), ("blobs", blobs)):
        base = os.path.join(d, sub)
        if not os.path.isdir(base): continue
        for shard in sorted(os.listdir(base)):
            sd = os.path.join(base, shard)
            if shard.startswith("."): continue
            if os.path.islink(sd) or not os.path.isdir(sd):
                out[sub + "/" + shard] = None; continue
            for n in sorted(os.listdir(sd)):
                full = os.path.join(sd, n)
                if n.startswith("."): continue
                st = os.lstat(full)
                out[sub + "/" + shard + "/" + n] = open(full, "rb").read() if stat.S_ISREG(st.st_mode) else None
    return files, blobs

def local_identity(d):
    try:
        return open(os.path.join(d, "local", "identity")).read().strip()
    except OSError:
        return None

if __name__ == "__main__":
    f, b = load_dir(sys.argv[1])
    lines, code, info = verify(f, b)
    lid = local_identity(sys.argv[1])
    # Exit 3 when the local identity (7.1 local/identity) is too new, 4 when
    # it is frozen (12.1). Other identities only print their line.
    if ("too new: %s" % lid) in lines: code = max(code, 3)
    if ("compromised: %s" % lid) in lines: code = 4
    # 12.3 verify: the local identity has no genesis here, and is not too new.
    if lid and lid not in info["valid"] and ("too new: %s" % lid) not in lines:
        lines = sorted(lines[:-1] + ["error: %s: own identity missing: sync from another copy" % lid]) + lines[-1:]
        code = max(code, 1)
    print("\n".join(lines))
    sys.exit(code)
