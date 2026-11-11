#!/usr/bin/env python3
"""check.py: check the folder that a device hands over.

    check.py <folder>

docs/DEVICE.md has the format. This is one file that needs Python 3 and
the `cryptography` package, and shares no code with bean: a firmware team
runs it on what their device writes. It prints `device folder ok` and exits
0, or each problem and exits 1.

The folder:
    device.json      whose folder it is, the device's time, a certificate
    captures.jsonl   one signed statement a line, a run with no gap
    raw/<sha256>     the raw data that the statements name
"""
import hashlib
import json
import os
import re
import sys
from datetime import datetime

from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric import ec, ed25519
from cryptography.hazmat.primitives.asymmetric.utils import encode_dss_signature

P256_N = 0xFFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551
HEX64 = re.compile(r"^[0-9a-f]{64}$")
ID = re.compile(r"^sha256:[0-9a-f]{64}$")
KIND = re.compile(r"^[a-z0-9][a-z0-9_-]*(\.[a-z0-9_-]+)*$")
TIME = re.compile(r"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$")


class Bad(Exception):
    pass


def plain(v):
    """Text of printable ASCII with no quote and no backslash, or a whole
    number that is not below zero."""
    if isinstance(v, str):
        return all(0x20 <= ord(c) <= 0x7E and c not in '"\\' for c in v)
    return isinstance(v, int) and not isinstance(v, bool) and 0 <= v <= 2**53 - 1


def parse(line):
    """One JSON object in canonical form: keys in order, no space, no
    escape, whole numbers only."""
    def pairs(items):
        if len({k for k, _ in items}) != len(items):
            raise Bad("a key is there twice")
        return dict(items)

    def no_fraction(_):
        raise Bad("a number is not a whole number")

    try:
        obj = json.loads(line.decode("utf-8"), object_pairs_hook=pairs, parse_float=no_fraction, parse_constant=no_fraction)
    except (ValueError, UnicodeDecodeError):
        raise Bad("not one JSON object")
    if not isinstance(obj, dict):
        raise Bad("not one JSON object")
    if json.dumps(obj, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode("utf-8") != line:
        raise Bad("not in canonical form: keys in order, no space, no escape")
    return obj


def object_id(obj):
    rest = {k: v for k, v in obj.items() if k != "sig"}
    data = json.dumps(rest, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
    return "sha256:" + hashlib.sha256(data).hexdigest()


def is_key(s):
    if not isinstance(s, str):
        return False
    try:
        if s.startswith("p256:") and len(s) == 5 + 66 and s[5:7] in ("02", "03"):
            ec.EllipticCurvePublicKey.from_encoded_point(ec.SECP256R1(), bytes.fromhex(s[5:]))
            return True
        if s.startswith("ed25519:") and len(s) == 8 + 64:
            bytes.fromhex(s[8:])
            return True
    except ValueError:
        pass
    return False


def signed_by(key, oid, sig):
    """The signature of a bean record (SPEC.md 4.5), over the 32 bytes of
    the id. P-256: ECDSA with SHA-256, r and s as 32 bytes each, s in the
    lower half."""
    alg = key.split(":")[0]
    if not isinstance(sig, str) or not re.fullmatch(alg + ":[0-9a-f]{128}", sig):
        return False
    raw, msg = bytes.fromhex(sig.split(":")[1]), bytes.fromhex(oid[7:])
    try:
        if alg == "ed25519":
            ed25519.Ed25519PublicKey.from_public_bytes(bytes.fromhex(key[8:])).verify(raw, msg)
        else:
            r, s = int.from_bytes(raw[:32], "big"), int.from_bytes(raw[32:], "big")
            if r == 0 or s == 0 or s > P256_N // 2:
                return False
            pub = ec.EllipticCurvePublicKey.from_encoded_point(ec.SECP256R1(), bytes.fromhex(key[5:]))
            pub.verify(encode_dss_signature(r, s), msg, ec.ECDSA(hashes.SHA256()))
    except (InvalidSignature, ValueError):
        return False
    return True


def a_time(t):
    if t == "":
        return True
    if not isinstance(t, str) or not TIME.match(t):
        return False
    try:
        datetime.strptime(t, "%Y-%m-%dT%H:%M:%SZ")
        return True
    except ValueError:
        return False


def statement(line):
    obj = parse(line)
    must = {"bean", "device", "kind", "prev", "raw", "selftest", "seq", "settings", "sig", "time"}
    if not must <= set(obj) or not set(obj) <= must | {"calibration"}:
        raise Bad("a member is missing, or one is there that the format does not have")
    if obj["bean"] != "capture/0":
        raise Bad('bean is not "capture/0"')
    dev = obj["device"]
    if not isinstance(dev, dict) or set(dev) != {"fw", "hw", "key", "model"}:
        raise Bad("device needs fw, hw, key and model, and nothing else")
    for k in ("fw", "hw", "model"):
        if not isinstance(dev[k], str) or not plain(dev[k]) or dev[k] == "":
            raise Bad("device.%s is not plain text" % k)
    if not is_key(dev["key"]):
        raise Bad("device.key is not a P-256 or Ed25519 key")
    if not isinstance(obj["kind"], str) or not KIND.match(obj["kind"]):
        raise Bad("kind is not a name such as ultrasound.bmode")
    if "calibration" in obj and not (isinstance(obj["calibration"], str) and plain(obj["calibration"]) and obj["calibration"]):
        raise Bad("calibration is not plain text")
    if obj["selftest"] not in ("pass", "fail", "none"):
        raise Bad("selftest is not pass, fail or none")
    if not a_time(obj["time"]):
        raise Bad("time is not empty and not like 2026-11-11T17:00:00Z")
    seq = obj["seq"]
    if not isinstance(seq, dict) or set(seq) != {"boot", "capture"} or not all(isinstance(seq[k], int) and plain(seq[k]) for k in seq) or seq["capture"] < 1:
        raise Bad("seq.boot and seq.capture are whole numbers, and capture starts at 1")
    prev = obj["prev"]
    if not isinstance(prev, str) or (prev != "" and not ID.match(prev)):
        raise Bad("prev is not empty and not an id")
    if (prev == "") != (seq["capture"] == 1):
        raise Bad("prev is empty for capture 1 and for no other")
    settings = obj["settings"]
    if not isinstance(settings, dict) or not all(plain(k) and plain(v) for k, v in settings.items()):
        raise Bad("settings holds something that is not plain text or a whole number")
    raw = obj["raw"]
    if not isinstance(raw, list) or not raw:
        raise Bad("raw names no file")
    for r in raw:
        if (not isinstance(r, dict) or set(r) != {"bytes", "name", "sha256"} or not isinstance(r["name"], str) or not plain(r["name"]) or r["name"] == ""
                or not isinstance(r["sha256"], str) or not HEX64.match(r["sha256"]) or not isinstance(r["bytes"], int) or not plain(r["bytes"])):
            raise Bad("a raw entry needs a name, a sha256 of 64 hex digits and bytes")
    oid = object_id(obj)
    if not signed_by(dev["key"], oid, obj["sig"]):
        raise Bad("the signature is not the device key's signature of this statement")
    return obj, oid


def certificate(cert, key):
    names = {"bean", "hw", "issuer", "key", "model", "sig"}
    if not isinstance(cert, dict) or set(cert) != names or not all(isinstance(cert[k], str) and plain(cert[k]) for k in names):
        raise Bad("the certificate needs bean, hw, issuer, key, model and sig, as plain text, and nothing else")
    if cert["bean"] != "device-cert/0" or cert["key"] != key or not is_key(cert["issuer"]):
        raise Bad("the certificate is not one for this device key")
    if not signed_by(cert["issuer"], object_id(cert), cert["sig"]):
        raise Bad("the certificate is not signed by its issuer")
    return cert["issuer"]


def check(folder):
    lines, problems = [], []
    try:
        with open(os.path.join(folder, "device.json"), "rb") as f:
            dev = parse(f.read().rstrip(b"\n"))
        if not {"bean", "key", "time"} <= set(dev) or not set(dev) <= {"bean", "key", "time", "cert"} or dev["bean"] != "device/0":
            raise Bad("it is not a device object: bean, key, time, and cert if there is one")
        if not is_key(dev["key"]):
            raise Bad("key is not a P-256 or Ed25519 key")
    except OSError:
        return lines, ["device.json cannot be read"]
    except Bad as err:
        return lines, ["device.json: %s" % err]
    key = dev["key"]
    lines.append("device " + key)
    if not a_time(dev["time"]):
        problems.append("device.json: time is not empty and not like 2026-11-11T17:00:00Z")
    if "cert" in dev:
        try:
            lines.append("certificate: issued by " + certificate(dev["cert"], key))
        except Bad as err:
            problems.append("device.json: %s" % err)
    else:
        lines.append("certificate: none")
    try:
        with open(os.path.join(folder, "captures.jsonl"), "rb") as f:
            data = f.read()
    except OSError:
        return lines, problems + ["captures.jsonl cannot be read"]
    prev, named, n = None, {}, 0
    rows = data[:-1].split(b"\n") if data.endswith(b"\n") else data.split(b"\n")
    for i, line in enumerate(rows if data else [], 1):
        try:
            st, oid = statement(line)
        except Bad as err:
            problems.append("captures.jsonl line %d: %s" % (i, err))
            prev = None
            continue
        n += 1
        if st["device"]["key"] != key:
            problems.append("captures.jsonl line %d: signed by another key than the device's" % i)
        if prev is not None:
            if st["seq"]["capture"] != prev[0]["seq"]["capture"] + 1 or st["prev"] != prev[1]:
                problems.append("captures.jsonl line %d: it does not follow the line before it" % i)
            if st["seq"]["boot"] < prev[0]["seq"]["boot"]:
                problems.append("captures.jsonl line %d: seq.boot went back" % i)
        else:
            lines.append("first capture: %d" % st["seq"]["capture"])
        for r in st["raw"]:
            if named.get(r["sha256"], r["bytes"]) != r["bytes"]:
                problems.append("captures.jsonl line %d: raw file %s is named with two sizes" % (i, r["sha256"]))
            named[r["sha256"]] = r["bytes"]
        prev = (st, oid)
    lines.append("captures: %d" % n)
    if n == 0:
        problems.append("captures.jsonl holds no statement")
    raw = os.path.join(folder, "raw")
    names = sorted(os.listdir(raw)) if os.path.isdir(raw) and not os.path.islink(raw) else None
    if names is None:
        problems.append("the raw folder cannot be read")
    for name in names or []:
        if name.startswith("."):
            continue
        if name not in named:
            problems.append("raw/%s is named by no statement" % name)
            continue
        size, path = named.pop(name), os.path.join(raw, name)
        if os.path.islink(path) or not os.path.isfile(path) or os.path.getsize(path) != size:
            problems.append("raw/%s is not a plain file of the size its statement gives" % name)
            continue
        h = hashlib.sha256()
        with open(path, "rb") as f:
            for chunk in iter(lambda: f.read(1 << 20), b""):
                h.update(chunk)
        if h.hexdigest() != name:
            problems.append("raw/%s does not have the SHA-256 of its name" % name)
    for name in sorted(named):
        problems.append("raw/%s is named by a statement, and is not there" % name)
    lines.append("raw files: %d" % len(names or []))
    return lines, problems


def main(argv):
    if len(argv) != 2:
        sys.stderr.write(__doc__)
        return 2
    lines, problems = check(argv[1])
    for line in lines:
        print(line)
    for p in sorted(problems):
        print("problem: " + p)
    if problems:
        print("device folder FAILED: %d problems" % len(problems))
        return 1
    print("device folder ok")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
