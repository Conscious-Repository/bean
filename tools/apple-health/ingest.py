#!/usr/bin/env python3
"""Preserve an Apple Health export through existing Bean CLI commands.

The caller chooses an existing private Bean log. No keys, signatures, source
identity or encryption are implemented here. Original input is never modified.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import tempfile

LIMIT = 256 << 20


def digest(path):
    h = hashlib.sha256()
    with Path(path).open('rb') as stream:
        for block in iter(lambda: stream.read(1 << 20), b''):
            h.update(block)
    return h.hexdigest()


def ingest(source, log, records=False, types=''):
    bean = [os.environ.get('BEAN_BIN', 'bean'), '-dir', str(log)]
    importer = os.environ.get('APPLE_HEALTH_BIN', 'bean-import-apple-health')
    args = (['-records'] if records else []) + (['-types', types] if types else [])
    with tempfile.TemporaryDirectory(prefix='bean-apple-ingest-') as temp:
        root = Path(temp)
        snapshot = root / 'source'
        fd = os.open(source, os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW)
        with os.fdopen(fd, 'rb') as incoming, snapshot.open('xb') as outgoing:
            info = os.fstat(incoming.fileno())
            if not stat.S_ISREG(info.st_mode) or info.st_size > LIMIT:
                raise ValueError('source must be a regular file no larger than 256 MiB')
            count = 0
            while block := incoming.read(1 << 20):
                count += len(block)
                if count > LIMIT:
                    raise ValueError('source exceeds 256 MiB')
                outgoing.write(block)
        expected = digest(snapshot)
        # Validate before writing any blob or claim. Spool rather than buffering
        # hundreds of thousands of optional readings in RAM or emitting a prefix.
        output = root / 'claims.jsonl'
        with output.open('wb') as stream:
            subprocess.run([importer, '-blob', 'sha256:' + '0' * 64, *args, str(snapshot)],
                           stdout=stream, stderr=subprocess.PIPE, check=True)
        bid = subprocess.check_output(bean + ['blob', 'put', str(snapshot)], stderr=subprocess.PIPE).decode().strip()
        if not re.fullmatch(r'sha256:[0-9a-f]{64}', bid):
            raise ValueError('invalid stored blob id')
        readback = root / 'readback'
        with readback.open('wb') as stream:
            subprocess.run(bean + ['blob', 'get', bid], stdout=stream, stderr=subprocess.PIPE, check=True)
        if readback.stat().st_size != count or digest(readback) != expected:
            raise ValueError('stored blob readback differs from source')
        with output.open('wb') as stream:
            subprocess.run([importer, '-blob', bid, *args, str(snapshot)],
                           stdout=stream, stderr=subprocess.PIPE, check=True)
        with output.open('rb') as stream:
            result = subprocess.run(bean + ['append'], stdin=stream, capture_output=True, check=True)
        # Confirm that the archive reference actually survived dedupe/purge rules.
        rows = subprocess.check_output(bean + ['ls', '-type', 'apple-health.export', '-author', 'me', '-json'], stderr=subprocess.PIPE)
        retained = False
        for line in rows.splitlines():
            claim = json.loads(line)
            if (claim.get('value', {}).get('blob') == bid
                    and claim.get('context', {}).get('sha256') == expected
                    and claim.get('context', {}).get('bytes') == count
                    and any(b.get('hash') == bid for b in claim.get('blobs', []))):
                retained = True
        if not retained:
            raise ValueError('archive not retained; it may have been purged or removed concurrently')
        return {'append': result.stdout.decode().strip(), 'blob': bid,
                'sha256': expected, 'bytes': count, 'assurance': 'unsigned-export'}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source')
    parser.add_argument('log')
    parser.add_argument('--records', action='store_true')
    parser.add_argument('--types', default='')
    args = parser.parse_args()
    try:
        print(json.dumps(ingest(args.source, args.log, args.records, args.types), sort_keys=True))
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        # Do not expose file contents, paths, health values or identity secrets.
        if isinstance(error, subprocess.CalledProcessError):
            parser.exit(1, 'ingestion failed: Bean or importer rejected the operation\n')
        parser.exit(1, 'ingestion failed: input, storage or retained archive validation failed\n')
