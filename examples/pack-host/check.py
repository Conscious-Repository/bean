"""Check one direct pack URL. Write verified bytes to stdout and the result to stderr."""
import argparse
from hashlib import sha256
from http.client import HTTPConnection, HTTPException, HTTPSConnection
import re
import sys
from urllib.parse import urlsplit

# Same bound as the pack reader in internal/pack.
MAX_BYTES = 64 << 20


def fetch(origin, digest):
    url = urlsplit(origin)
    if (not url.hostname or url.username is not None or url.path not in ("", "/")
            or url.query or url.fragment):
        raise ValueError("give the host origin, without a path or credentials")
    if url.scheme == "https":
        connection = HTTPSConnection
    elif url.scheme == "http" and url.hostname in ("127.0.0.1", "::1"):
        connection = HTTPConnection
    else:
        raise ValueError("use HTTPS, or HTTP on a loopback address for a rehearsal")
    if not re.fullmatch(r"[0-9a-f]{64}", digest):
        raise ValueError("expected SHA-256 must be 64 lowercase hex digits")

    for method in ("HEAD", "GET"):
        conn = connection(url.hostname, url.port, timeout=30)
        try:
            conn.request(method, "/" + digest)
            response = conn.getresponse()
            if response.status != 200:
                raise ValueError(f"{method} returned HTTP {response.status}")
            if method == "GET":
                data = response.read(MAX_BYTES + 1)
                if len(data) > MAX_BYTES:
                    raise ValueError("pack exceeds 64 MiB")
                if response.length:
                    raise ValueError("download ended before its declared length")
        finally:
            conn.close()
    if sha256(data).hexdigest() != digest:
        raise ValueError("download differs from the expected SHA-256")
    return data


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("origin")
    parser.add_argument("sha256", help="expected digest from an independent source")
    args = parser.parse_args()
    try:
        pack = fetch(args.origin, args.sha256)
        if sys.stdout.isatty():
            print("Redirect stdout to a new file to keep the checked pack.", file=sys.stderr)
        else:
            sys.stdout.buffer.write(pack)
        print(f"Verified {args.sha256} ({len(pack)} bytes)", file=sys.stderr)
    except (HTTPException, OSError, ValueError) as exc:
        sys.exit(f"pack check failed: {exc}")
