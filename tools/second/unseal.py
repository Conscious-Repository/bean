#!/usr/bin/env python3
"""unseal.py: open a sealed bean package without bean.

    unseal.py <package> <new folder>      (the six words on standard input)
    unseal.py -key <key file> <package> <new folder>

A sealed package is the folder of a bundle as one zip file, encrypted in
the age format (age-encryption.org/v1). It is sealed in one of two ways.
With words: a scrypt recipient, whose passphrase is the six words in lower
case, with one space between. To an address: an X25519 recipient, which the
key file of that address opens. A key file is an age identity file with
exactly one identity. This reads the format from its specification and
shares no code with bean or with the age tool: `age -d` opens a package
too. It writes the bundle's folder, which checkbundle.py then checks. It
exits 1 if the words or the key do not open the package, or the package was
changed.
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
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric.x25519 import X25519PrivateKey, X25519PublicKey
from cryptography.hazmat.primitives.ciphers.aead import ChaCha20Poly1305
from cryptography.hazmat.primitives.kdf.hkdf import HKDF
from cryptography.hazmat.primitives.kdf.scrypt import Scrypt

INTRO = b"age-encryption.org/v1\n"
HEADER = 4096  # a header for one reader ends well before this
KEY_FILE = 64 << 10  # a key file is not larger than this
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


def scan(data):
    """What a file asks for, by its header alone: "words" or "key". The
    lines after the first, split on LF, up to the first line that starts
    "---", name the readers. A package names exactly one. It returns the
    kind, those lines, and where the "---" line starts."""
    if not data.startswith(INTRO):
        raise Refused("it is not a sealed package")
    lines, at, closed = [], len(INTRO), False
    for line in data[len(INTRO):HEADER].split(b"\n"):
        if line.startswith(b"---"):
            closed = True
            break
        if b"\r" in line:
            raise Refused("its header holds a carriage return, which no package has")
        lines.append(line)
        at += len(line) + 1
    if not closed:
        raise Refused("its header is damaged, or longer than a package has")
    readers = [line for line in lines if line.startswith(b"->")]
    if len(readers) != 1:
        raise Refused("its header has %d stanzas, and a package has one" % len(readers))
    if readers[0].startswith(b"-> scrypt "):
        return "words", lines, at
    if readers[0].startswith(b"-> X25519 "):
        return "key", lines, at
    raise Refused("it is sealed in a way that this tool does not open: not with words, and not to an address")


BECH32 = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"


def bech32(text, prefix):
    """The bytes that a Bech32 text holds under a prefix (BIP 173). One
    letter case, a right checksum, and no bits left over."""
    if text != text.lower() and text != text.upper():
        raise ValueError("mixed case")
    text = text.lower()
    hrp, sep, rest = text.rpartition("1")
    if not sep or hrp != prefix or len(rest) < 6 or any(c not in BECH32 for c in rest):
        raise ValueError("not this kind of text")
    values = [ord(c) >> 5 for c in hrp] + [0] + [ord(c) & 31 for c in hrp] + [BECH32.index(c) for c in rest]
    chk = 1
    for v in values:
        top = chk >> 25
        chk = (chk & 0x1FFFFFF) << 5 ^ v
        for i, gen in enumerate((0x3B6A57B2, 0x26508E6D, 0x1EA119FA, 0x3D4233DD, 0x2A1462B3)):
            if top >> i & 1:
                chk ^= gen
    if chk != 1:
        raise ValueError("bad checksum")
    acc, bits, out = 0, 0, bytearray()
    for c in rest[:-6]:
        acc, bits = acc << 5 | BECH32.index(c), bits + 5
        if bits >= 8:
            bits -= 8
            out.append(acc >> bits & 255)
    if bits >= 5 or acc & ((1 << bits) - 1):
        raise ValueError("bits left over")
    return bytes(out)


def read_key(path):
    """The one secret of a key file: an age identity file with exactly one
    identity. Empty lines and lines that start with # are not read. What
    this says of another file holds nothing of the file."""
    try:
        with open(path, "rb") as f:
            data = f.read(KEY_FILE + 1)
    except OSError as err:
        raise Refused("the key file cannot be read: %s" % err.strerror)
    if len(data) > KEY_FILE:
        raise Refused("the key file is not an age identity file with exactly one identity")
    if data.startswith(INTRO.rstrip()) or data.lstrip().startswith(b"-----BEGIN AGE ENCRYPTED FILE-----"):
        raise Refused("the key file is an encrypted age file, and this tool does not open such a key file")
    try:
        # A line ends at LF, and one CR before it is not part of the line, as
        # the age tool reads a key file. Nothing else ends a line.
        # A comment line is not read, so it may hold any bytes.
        lines = [line[:-1] if line.endswith(b"\r") else line for line in data.split(b"\n")]
        lines = [line for line in lines if line and not line.startswith(b"#")]
        if len(lines) != 1:
            raise ValueError("not one identity")
        text = lines[0].decode("ascii")
        if text != text.upper():
            raise ValueError("not in capitals")
        secret = bech32(text, "age-secret-key-")
        if len(secret) != 32:
            raise ValueError("not 32 bytes")
    except ValueError:
        raise Refused("the key file is not an age identity file with exactly one identity")
    return secret


def file_key_from_key(lines, secret):
    """The file key of a package that is sealed to an address: the X25519
    recipient stanza of the age specification."""
    args = lines[0].decode("ascii").split(" ")
    if len(args) != 3 or len(lines) != 2:
        raise Refused("its header is not one bean writes")
    share, wrapped = b64(args[2]), b64(lines[1].decode("ascii"))
    if len(share) != 32 or len(wrapped) != 32:
        raise Refused("its header is not one bean writes")
    key = X25519PrivateKey.from_private_bytes(secret)
    ours = key.public_key().public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw)
    try:
        shared = key.exchange(X25519PublicKey.from_public_bytes(share))
    except ValueError:
        raise Refused("its header is damaged")
    if shared == bytes(32):
        raise Refused("its header is damaged")
    wrap = HKDF(algorithm=hashes.SHA256(), length=32, salt=share + ours, info=b"age-encryption.org/v1/X25519").derive(shared)
    try:
        return ChaCha20Poly1305(wrap).decrypt(b"\x00" * 12, wrapped, None)
    except InvalidTag:
        raise Refused("it was sealed to another address, or it was changed on the way")


def file_key_from_words(lines, words):
    """The file key of a package that is sealed with words: the scrypt
    recipient stanza of the age specification."""
    args = lines[0].decode("ascii").split(" ")
    if len(args) != 4:
        raise Refused("its header is not one bean writes")
    if not re.fullmatch(r"[1-9][0-9]*", args[3]) or int(args[3]) > WORK:
        raise Refused("its header is not one bean writes")
    salt, work = b64(args[2]), int(args[3])
    if len(salt) != 16 or len(lines) != 2:
        raise Refused("its header is not one bean writes")
    wrapped = b64(lines[1].decode("ascii"))
    key = Scrypt(salt=b"age-encryption.org/v1/scrypt" + salt, length=32, n=1 << work, r=8, p=1).derive(words.encode())
    try:
        return ChaCha20Poly1305(key).decrypt(b"\x00" * 12, wrapped, None)
    except InvalidTag:
        raise Refused("these words do not open it, or it was changed on the way")


def unseal(data, words, secret=None):
    """The packed bundle of a sealed package. secret is the 32 bytes of a
    key file, or None for a package that opens with words."""
    kind, lines, at = scan(data)
    if kind == "key" and secret is None:
        raise Refused("it is sealed to a key, not with words: open it with -key <key file>")
    if kind == "words" and secret is not None:
        raise Refused("it is sealed with words, and a key does not open it: give the six words")
    # The header ends with the MAC line.
    mac_end = data.find(b"\n", at)
    if data[at:at + 4] != b"--- " or mac_end < 0:
        raise Refused("its header is damaged")
    try:
        header, mac_text = data[:at + 3], data[at + 4:mac_end].decode("ascii")
        for line in lines:
            line.decode("ascii")
    except UnicodeDecodeError:
        raise Refused("its header is damaged")
    mac = b64(mac_text)
    # The reader's stanza is the first line after the first, as age writes.
    if not lines[0].startswith(b"->"):
        raise Refused("its header is damaged")
    file_key = file_key_from_words(lines, words) if secret is None else file_key_from_key(lines, secret)
    if len(file_key) != 16:
        raise Refused("its header is not one bean writes")
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
    key_file = None
    if len(argv) == 5 and argv[1] == "-key":
        key_file, argv = argv[2], argv[:1] + argv[3:]
    if len(argv) != 3:
        sys.stderr.write(__doc__)
        return 2
    try:
        # The words are asked for only when no key file is named.
        secret = read_key(key_file) if key_file is not None else None
        words = clean(sys.stdin.readline()) if key_file is None else ""
        with open(argv[1], "rb") as f:
            data = f.read()
        files = entries(unseal(data, words, secret))
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
