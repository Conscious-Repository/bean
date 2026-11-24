"""Cache the public files named in a Bean coverage selection."""
import argparse
from hashlib import sha256
from http.client import HTTPSConnection
import json
import os
from pathlib import Path
import re
from tempfile import NamedTemporaryFile
from urllib.parse import urlsplit


def fetch(selection, directory, origin):
    host = urlsplit(origin)
    if (host.scheme != "https" or not host.hostname or host.username is not None
            or host.path not in ("", "/") or host.query or host.fragment):
        raise ValueError("expected an HTTPS origin")
    if selection["format"] != "bean-coverage-sources/1":
        raise ValueError("unknown source selection format")
    directory.mkdir(parents=True, exist_ok=True)
    seen = set()
    for kind, limit in [("packs", 64 << 20), ("guides", 8 << 20)]:
        for pin in selection[kind]:
            digest = pin["sha256"]
            if not re.fullmatch("[0-9a-f]{64}", digest) or digest in seen:
                raise ValueError("invalid or duplicate digest")
            seen.add(digest)
            dest = directory / digest
            if dest.exists() and dest.stat().st_size <= limit and sha256(dest.read_bytes()).hexdigest() == digest:
                continue
            route = "/" + digest if kind == "packs" else "/assays/" + digest + ".json"
            conn = HTTPSConnection(host.hostname, host.port, timeout=45)
            try:
                conn.request("GET", route)
                response = conn.getresponse()
                if response.status != 200:
                    raise ValueError(f"{route} returned HTTP {response.status}")
                data = response.read(limit + 1)
            finally:
                conn.close()
            if len(data) > limit or sha256(data).hexdigest() != digest:
                raise ValueError(f"invalid download for {digest}")
            with NamedTemporaryFile(dir=directory, delete=False) as f:
                try:
                    f.write(data)
                    f.close()
                    os.replace(f.name, dest)
                finally:
                    Path(f.name).unlink(missing_ok=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("selection", type=Path)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--origin", default="https://stele.aion.bio")
    args = parser.parse_args()
    fetch(json.loads(args.selection.read_bytes()), args.directory, args.origin)
