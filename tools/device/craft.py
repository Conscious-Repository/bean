#!/usr/bin/env python3
"""craft.py: a second writer of device folders, for tests.

    craft.py <new folder> [<case>]

It writes the folder of docs/DEVICE.md with its own key and its own code,
so that the Go check is tried on a folder that Go did not write. With a
case it writes one thing wrong, and signs it properly where a signature is
not the thing that is wrong: a check must refuse it for the right reason.
`craft.py -cases` lists the cases.
"""
import hashlib
import json
import os
import sys

from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives.asymmetric.utils import decode_dss_signature

N = 0xFFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551
CASES = ["fraction", "escape", "accent", "high-s", "extra", "zero", "gap", "prev", "other-key", "bool", "space",
         "nested", "boot-back", "time", "cert-other", "cert-forged", "no-raw", "null", "key-order", "big"]


def canon(obj):
    return json.dumps(obj, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()


def new_key():
    k = ec.generate_private_key(ec.SECP256R1())
    pub = k.public_key().public_bytes(serialization.Encoding.X962, serialization.PublicFormat.CompressedPoint)
    return k, "p256:" + pub.hex()


def sign(key, obj, high=False):
    rest = {k: v for k, v in obj.items() if k != "sig"}
    oid = hashlib.sha256(canon(rest)).digest()
    r, s = decode_dss_signature(key.sign(oid, ec.ECDSA(hashes.SHA256())))
    if (s > N // 2) != high:
        s = N - s
    obj["sig"] = "p256:" + r.to_bytes(32, "big").hex() + s.to_bytes(32, "big").hex()
    return "sha256:" + oid.hex()


def main(argv):
    if argv[1:] == ["-cases"]:
        print("\n".join(CASES))
        return 0
    out, case = argv[1], (argv[2] if len(argv) > 2 else "")
    os.makedirs(os.path.join(out, "raw"))
    key, pub = new_key()
    other, other_pub = new_key()
    lines, prev = [], ""
    for i in (1, 2, 3):
        raw = bytes((i * 17 + j) % 256 for j in range(1000))
        digest = hashlib.sha256(raw).hexdigest()
        with open(os.path.join(out, "raw", digest), "wb") as f:
            f.write(raw)
        st = {"bean": "capture/0", "device": {"fw": "0.1.0", "hw": "B", "key": pub, "model": "craft-1"}, "kind": "craft.ramp", "prev": prev,
              "raw": [{"bytes": len(raw), "name": "ramp.bin", "sha256": digest}], "selftest": "none", "seq": {"boot": 2, "capture": i},
              "settings": {"gain_db": 12, "probe": "none"}, "time": ""}
        signer, high, text = key, False, None
        if i == 2:
            if case == "fraction": st["settings"]["gain_db"] = 12.5
            if case == "escape": st["settings"]["probe"] = 'a"b'
            if case == "accent": st["device"]["model"] = "craft-é"
            if case == "high-s": high = True
            if case == "extra": st["note"] = "hello"
            if case == "gap": st["seq"]["capture"] = 3
            if case == "prev": st["prev"] = "sha256:" + "0" * 64
            if case == "other-key": signer, st["device"]["key"] = other, other_pub
            if case == "bool": st["selftest"] = True
            if case == "nested": st["settings"]["probe"] = {"type": "linear"}
            if case == "boot-back": st["seq"]["boot"] = 1
            if case == "time": st["time"] = "2026-13-40T00:00:00Z"
            if case == "no-raw": st["raw"] = []
            if case == "null": st["calibration"] = None
            if case == "big": st["settings"]["gain_db"] = 2**53
        if i == 1 and case == "zero":
            st["seq"]["capture"] = 0
        oid = sign(signer, st, high)
        line = canon(st)
        if i == 2 and case == "space":
            line = json.dumps(st, sort_keys=True).encode()  # the same object, with spaces
        if i == 2 and case == "key-order":
            line = json.dumps(st, separators=(",", ":")).encode()  # the same object, keys as written
        lines.append(line)
        prev = oid
        if case == "zero" and i == 1:
            prev = oid
    dev = {"bean": "device/0", "key": pub, "time": ""}
    if case in ("", "cert-other", "cert-forged"):
        factory, factory_pub = new_key()
        cert = {"bean": "device-cert/0", "hw": "B", "issuer": factory_pub, "key": other_pub if case == "cert-other" else pub, "model": "craft-1"}
        sign(other if case == "cert-forged" else factory, cert)
        dev["cert"] = cert
    with open(os.path.join(out, "captures.jsonl"), "wb") as f:
        f.write(b"\n".join(lines) + b"\n")
    with open(os.path.join(out, "device.json"), "wb") as f:
        f.write(canon(dev) + b"\n")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
