#!/usr/bin/env python3
"""Shared byte-preservation steps for the Apple Health and Labcorp helpers.

The caller chooses an existing private Bean log. No keys, signatures, source
identity or encryption are implemented here. Original input is never modified.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import tempfile


class IngestError(ValueError):
    """Safe phase information; the underlying error remains available as __cause__."""
    MESSAGES = {
        'validation': 'validation failed; no Bean writes attempted',
        'storage': 'storage or readback failed; encrypted blobs may remain; no claims appended; retry is safe',
        'append': 'append failed; partial ingestion is possible; retry is safe',
        'retention': 'archive not retained or retention check failed; ingestion may be partial; retry is safe',
    }

    def __init__(self, phase):
        self.phase = phase
        super().__init__(self.MESSAGES[phase])


def digest(path):
    h = hashlib.sha256()
    with Path(path).open('rb') as stream:
        for block in iter(lambda: stream.read(1 << 20), b''):
            h.update(block)
    return h.hexdigest()


def ingest_source(source, log, *, importer, args, limit, archive_type, assurance):
    progress = {'phase': 'validation'}
    try:
        return _ingest(source, log, importer, args, limit, archive_type, assurance, progress)
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        raise IngestError(progress['phase']) from error


def _ingest(source, log, importer, args, limit, archive_type, assurance, progress):
    bean = [os.environ.get('BEAN_BIN', 'bean'), '-dir', str(log)]
    with tempfile.TemporaryDirectory(prefix='bean-source-ingest-') as temp:
        root = Path(temp)
        snapshot = root / 'source'
        fd = os.open(source, os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW)
        with os.fdopen(fd, 'rb') as incoming, snapshot.open('xb') as outgoing:
            info = os.fstat(incoming.fileno())
            if not stat.S_ISREG(info.st_mode) or info.st_size > limit:
                raise ValueError('source must be a regular file within the source limit')
            count = 0
            while block := incoming.read(1 << 20):
                count += len(block)
                if count > limit:
                    raise ValueError('source exceeds limit')
                outgoing.write(block)
        expected = digest(snapshot)
        # Validate before writing any blob or claim. Spool rather than buffering
        # hundreds of thousands of optional readings in RAM or emitting a prefix.
        output = root / 'claims.jsonl'
        with output.open('wb') as stream:
            subprocess.run([importer, '-blob', 'sha256:' + '0' * 64, *args, str(snapshot)],
                           stdout=stream, stderr=subprocess.PIPE, check=True)
        progress['phase'] = 'storage'
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
            progress['phase'] = 'append'
            result = subprocess.run(bean + ['append'], stdin=stream, capture_output=True, check=True)
        # Confirm that the archive reference actually survived dedupe/purge rules.
        progress['phase'] = 'retention'
        rows = subprocess.check_output(bean + ['ls', '-type', archive_type, '-author', 'me', '-json'], stderr=subprocess.PIPE)
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
                'sha256': expected, 'bytes': count, 'assurance': assurance}

