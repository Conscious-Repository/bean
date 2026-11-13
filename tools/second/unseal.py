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
import re
import sys
import zipfile

from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.ciphers.aead import ChaCha20Poly1305
from cryptography.hazmat.primitives.kdf.hkdf import HKDF
from cryptography.hazmat.primitives.kdf.scrypt import Scrypt

INTRO = b"age-encryption.org/v1\n"
CHUNK = 64 * 1024
WORK = 16  # bean seals with this work factor, and opens none above it
MAX_OPEN = 1 << 30  # the most that a package may hold once it is open


class Refused(Exception):
    pass


def b64(text):
    """age writes base64 without padding, in one canonical form. Anything
    else is a header that was changed."""
    try:
        raw = base64.b64decode(text + "=" * (-len(text) % 4), validate=True)
    except ValueError:
        raise Refused("its header is damaged")
    if base64.b64encode(raw).decode().rstrip("=") != text:
        raise Refused("its header is damaged")
    return raw


def hkdf(key, salt, info):
    return HKDF(algorithm=hashes.SHA256(), length=32, salt=salt, info=info).derive(key)


def clean(words):
    """The words as a person types them: any letter case, with spaces,
    commas, hyphens or full stops between."""
    for mark in ",-.;":
        words = words.replace(mark, " ")
    return " ".join(words.lower().split())


def unseal(data, words):
    if not data.startswith(INTRO):
        raise Refused("it is not a sealed package")
    # The header: one scrypt stanza, then the MAC line.
    end = data.find(b"\n--- ")
    if end < 0:
        raise Refused("its header is damaged")
    mac_end = data.find(b"\n", end + 1)
    try:
        header, mac_text = data[:end + 4], data[end + 5:mac_end].decode("ascii")
        lines = data[len(INTRO):end].decode("ascii").split("\n")
    except UnicodeDecodeError:
        raise Refused("its header is damaged")
    mac = b64(mac_text)
    args = lines[0].split(" ")
    if len(args) != 4 or args[:2] != ["->", "scrypt"]:
        raise Refused("it is not sealed with words: this is another kind of age file")
    if not re.fullmatch(r"[1-9][0-9]*", args[3]) or int(args[3]) > WORK:
        raise Refused("its header is not one bean writes")
    salt, work = b64(args[2]), int(args[3])
    if len(salt) != 16 or len(lines) != 2:
        raise Refused("its header is not one bean writes")
    wrapped = b64(lines[1])
    key = Scrypt(salt=b"age-encryption.org/v1/scrypt" + salt, length=32, n=1 << work, r=8, p=1).derive(words.encode())
    try:
        file_key = ChaCha20Poly1305(key).decrypt(b"\x00" * 12, wrapped, None)
    except InvalidTag:
        raise Refused("these words do not open it, or it was changed on the way")
    want = hmac.new(hkdf(file_key, None, b"header"), header, hashlib.sha256).digest()
    if not hmac.compare_digest(want, mac):
        raise Refused("its header was changed")
    # The payload: a nonce, then chunks of 64 KiB, each with its own tag. The
    # last chunk is marked, so a package cut short does not open. Only an
    # empty payload has an empty last chunk.
    body = data[mac_end + 1:]
    nonce, body = body[:16], body[16:]
    stream = ChaCha20Poly1305(hkdf(file_key, nonce, b"payload"))
    out, count, total = [], 0, 0
    while True:
        chunk, body = body[:CHUNK + 16], body[CHUNK + 16:]
        last = not body
        if last and count > 0 and len(chunk) == 16:
            raise Refused("it was changed or cut short")
        try:
            out.append(stream.decrypt(count.to_bytes(11, "big") + (b"\x01" if last else b"\x00"), chunk, None))
        except InvalidTag:
            raise Refused("it was changed or cut short")
        count += 1
        total += len(out[-1])
        if total > MAX_OPEN:
            raise Refused("it holds more than a package can")
        if last:
            return b"".join(out)


def entries(packed):
    """The files of the packed bundle, by their path in the bundle's folder.
    Nothing is written until every name and size has passed."""
    try:
        z = zipfile.ZipFile(io.BytesIO(packed))
    except zipfile.BadZipFile:
        raise Refused("what it holds is not a bundle")
    names, total = {}, 0
    for info in z.infolist():
        name = info.filename.replace("\\", "/")
        if name.endswith("/") or name.startswith("__MACOSX/"):
            continue  # a folder, or what a Mac adds
        parts = name.split("/")
        if name.startswith("/") or any(p in ("", ".", "..") for p in parts):
            raise Refused("it holds a name that no folder can hold")
        if name in names:
            raise Refused("it holds two files with one name")
        total += info.file_size
        if total > MAX_OPEN:
            raise Refused("it holds more than a package can")
        names[name] = info
    # The bundle's own folder, or one folder that is the bundle.
    top = {n.split("/")[0] for n in names if not n.startswith(".")}
    root = ""
    if "objects" not in top:
        if len(top) != 1:
            raise Refused("what it holds is not a bundle")
        root = top.pop() + "/"
    out = {}
    for name, info in names.items():
        if not name.startswith(root):
            continue
        try:
            out[name[len(root):]] = z.read(info)
        except (zipfile.BadZipFile, ValueError, OSError):
            raise Refused("what it holds is damaged")
    return out


def main(argv):
    if len(argv) != 3:
        sys.stderr.write(__doc__)
        return 2
    words = clean(sys.stdin.readline())
    try:
        with open(argv[1], "rb") as f:
            data = f.read()
        files = entries(unseal(data, words))
        os.mkdir(argv[2])
    except FileExistsError:
        print("not opened: %s is there already: give a new folder" % argv[2])
        return 1
    except (Refused, ValueError, OSError) as err:
        print("not opened: %s" % err)
        return 1
    for name, content in files.items():
        path = os.path.join(argv[2], *name.split("/"))
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "wb") as f:
            f.write(content)
    print("opened into %s" % argv[2])
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
