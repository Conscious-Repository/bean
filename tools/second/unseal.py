#!/usr/bin/env python3
"""unseal.py: open a sealed bean package without bean.

    unseal.py <package> <new folder>      (the six words on standard input)

A sealed package is the folder of a bundle as one zip file, encrypted with
a passphrase in the age format (age-encryption.org/v1, scrypt recipient).
The passphrase is the six words in lower case, with one space between. This
reads that format from its specification and shares no code with bean or
with the age tool: `age -d` opens a package too. It writes the bundle's
folder, which checkbundle.py then checks. It exits 1 if the words do not
open the package, or the package was changed.
"""
import base64
import hashlib
import hmac
import io
import os
import sys
import zipfile

from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.ciphers.aead import ChaCha20Poly1305
from cryptography.hazmat.primitives.kdf.hkdf import HKDF
from cryptography.hazmat.primitives.kdf.scrypt import Scrypt

INTRO = b"age-encryption.org/v1\n"
CHUNK = 64 * 1024
MAX_WORK = 18  # bean seals with 16; more is not bean's, and costs minutes


class Refused(Exception):
    pass


def b64(text):
    """age writes base64 without padding."""
    return base64.b64decode(text + "=" * (-len(text) % 4), validate=True)


def hkdf(key, salt, info):
    return HKDF(algorithm=hashes.SHA256(), length=32, salt=salt, info=info).derive(key)


def unseal(data, words):
    if not data.startswith(INTRO):
        raise Refused("it is not a sealed package")
    # The header: one scrypt stanza, then the MAC line.
    end = data.find(b"\n--- ")
    if end < 0:
        raise Refused("its header is damaged")
    mac_end = data.find(b"\n", end + 1)
    header, mac = data[:end + 4], b64(data[end + 5:mac_end].decode("ascii"))
    lines = data[len(INTRO):end].decode("ascii").split("\n")
    args = lines[0].split(" ")
    if len(args) != 4 or args[:2] != ["->", "scrypt"] or not args[3].isdigit():
        raise Refused("it is not sealed with a passphrase")
    salt, work = b64(args[2]), int(args[3])
    if len(salt) != 16 or work > MAX_WORK:
        raise Refused("its header is not one bean writes")
    wrapped = b64("".join(lines[1:]))
    key = Scrypt(salt=b"age-encryption.org/v1/scrypt" + salt, length=32, n=1 << work, r=8, p=1).derive(words.encode())
    try:
        file_key = ChaCha20Poly1305(key).decrypt(b"\x00" * 12, wrapped, None)
    except InvalidTag:
        raise Refused("these words do not open it")
    want = hmac.new(hkdf(file_key, None, b"header"), header, hashlib.sha256).digest()
    if not hmac.compare_digest(want, mac):
        raise Refused("its header was changed")
    # The payload: a nonce, then chunks of 64 KiB, each with its own tag. The
    # last chunk is marked, so a package cut short does not open.
    body = data[mac_end + 1:]
    nonce, body = body[:16], body[16:]
    stream = ChaCha20Poly1305(hkdf(file_key, nonce, b"payload"))
    out, count = [], 0
    while True:
        chunk, body = body[:CHUNK + 16], body[CHUNK + 16:]
        last = not body
        try:
            out.append(stream.decrypt(count.to_bytes(11, "big") + (b"\x01" if last else b"\x00"), chunk, None))
        except InvalidTag:
            raise Refused("it was changed or cut short")
        count += 1
        if last:
            return b"".join(out)


def main(argv):
    if len(argv) != 3:
        sys.stderr.write(__doc__)
        return 2
    words = " ".join(sys.stdin.readline().lower().split())
    with open(argv[1], "rb") as f:
        data = f.read()
    try:
        packed = unseal(data, words)
    except (Refused, ValueError) as err:
        print("not opened: %s" % err)
        return 1
    os.mkdir(argv[2])
    with zipfile.ZipFile(io.BytesIO(packed)) as z:
        for name in z.namelist():
            # A name that leaves the folder is not written.
            parts = name.split("/")
            if name.startswith("/") or "\\" in name or any(p in ("", ".", "..") for p in parts):
                print("not opened: the package holds a name that no folder can hold")
                return 1
        for name in z.namelist():
            path = os.path.join(argv[2], *name.split("/"))
            os.makedirs(os.path.dirname(path), exist_ok=True)
            with open(path, "wb") as f:
                f.write(z.read(name))
    print("opened into %s" % argv[2])
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
