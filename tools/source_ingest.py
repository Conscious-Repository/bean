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
        'log': 'existing log verification failed; no source blobs or claims written; repair the log before retrying',
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
        progress['phase'] = 'log'
        subprocess.run(bean + ['verify'], stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, check=True)
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
        # Unbuffered: the remaining bytes are later read by a subprocess through
        # this file descriptor, so Python must not read ahead past the first line.
        with output.open('rb', buffering=0) as stream, tempfile.TemporaryFile() as archive:
            # Both converters emit the raw archive first. Refuse to append readings
            # when that archive was skipped by purge/dedupe or was not retained.
            first = stream.readline((1 << 20) + 1)
            if len(first) > 1 << 20 or json.loads(first).get('type') != archive_type:
                raise ValueError('missing or oversized first archive claim')
            archive.write(first)
            archive.seek(0)
            progress['phase'] = 'append'
            added, skipped = append_counts(bean, archive)
            progress['phase'] = 'retention'
            check_retained(bean, archive_type, bid, expected, count)
            if stream.tell() < output.stat().st_size:
                progress['phase'] = 'append'
                more_added, more_skipped = append_counts(bean, stream)
                added += more_added
                skipped += more_skipped
                progress['phase'] = 'retention'
                check_retained(bean, archive_type, bid, expected, count)
        return {'append': f'added {added}, skipped {skipped}', 'blob': bid,
                'sha256': expected, 'bytes': count, 'assurance': assurance}


def append_counts(bean, stream):
    result = subprocess.run(bean + ['append'], stdin=stream, capture_output=True, check=True)
    match = re.fullmatch(rb'added ([0-9]+), skipped ([0-9]+)\n?', result.stdout)
    if not match:
        raise ValueError('unexpected append result')
    return int(match[1]), int(match[2])


def check_retained(bean, archive_type, bid, expected, count):
    # Ordinary ls can trust cached objects with unchanged size/mtime. Verify
    # actual bytes before interpreting its view as a retained, valid adoption.
    subprocess.run(bean + ['verify'], stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, check=True)
    rows = subprocess.check_output(bean + ['ls', '-type', archive_type, '-author', 'me', '-json'], stderr=subprocess.PIPE)
    for line in rows.splitlines():
        claim = json.loads(line)
        if (claim.get('subject') == claim.get('author')
                and claim.get('value', {}).get('blob') == bid
                and claim.get('context', {}).get('sha256') == expected
                and claim.get('context', {}).get('bytes') == count
                and any(b.get('hash') == bid for b in claim.get('blobs', []))):
            return
    raise ValueError('archive not retained; it may have been purged or removed concurrently')
