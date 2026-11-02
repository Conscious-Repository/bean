#!/usr/bin/env python3
"""Experimental Bean adoption through existing CLI; no Bean key/crypto implementation."""
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import tempfile

from verify import LIMITS, decode, parse, read_file, verify

NAMES = ('receipt.jws', 'raw.bin', 'settings.json')
ROLES = ('signature', 'raw-export', 'acquisition-settings')
MEDIA = ('application/jose', 'application/octet-stream', 'application/json')


def bean(log, *args, data=None):
    return subprocess.run([os.environ.get('BEAN_BIN', 'bean'), '-dir', str(log), *args],
                          input=data, capture_output=True, check=True).stdout


def require_verified(directory):
    report, code = verify(directory)
    if code:
        raise ValueError('capture verification failed: ' + report['status'])
    return report


def ingest(capture, log):
    require_verified(capture)
    # Copy bounded bytes through an anchored descriptor, then verify the snapshot.
    # No subsequent reads of the mutable source are used for storage or the claim.
    with tempfile.TemporaryDirectory(prefix='bean-evidence-') as temp:
        stage = Path(temp)
        fd = os.open(capture, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            for name in NAMES:
                content, _ = read_file(fd, name)
                (stage / name).write_bytes(content)
        finally:
            os.close(fd)
        report = require_verified(stage)
        receipt = (stage / 'receipt.jws').read_bytes()
        parts = receipt.decode().split('.')
        header, payload = parse(decode(parts[0])), parse(decode(parts[1]))
        native_id = header['jwk']['x'] + ':' + payload['capture']
        digest = 'sha256:' + hashlib.sha256(receipt).hexdigest()
        # A signer reusing a nonce must not turn different bytes into another measurement.
        for line in bean(log, 'ls', '-type', 'instrument.ingest', '-author', 'me', '-json').splitlines():
            old = json.loads(line)
            context = old.get('context', {})
            if context.get('capture_key_nonce') == native_id and context.get('receipt_sha256') != digest:
                raise ValueError('conflicting receipt for signer/capture nonce')
        blobs = []
        files = {}
        for name, role, media in zip(NAMES, ROLES, MEDIA):
            bid = bean(log, 'blob', 'put', str(stage / name)).decode().strip()
            # Existing PutBlob may encounter corruption: read back before adopting.
            if bean(log, 'blob', 'get', bid) != (stage / name).read_bytes():
                raise ValueError('blob readback mismatch')
            files[name] = bid
            blobs.append({'hash': bid, 'role': role, 'media_type': media})
        claim = {'type': 'instrument.ingest', 'value': {'s': 'synthetic host-software evidence adoption'},
                 'times': {'observed': None},
                 'source': {'system': 'instrument-evidence-experiment',
                            'adapter': 'bean-import-evidence@experiment',
                            'ref_input': ['bean/v0/ref', 'id', 'instrument-evidence-experiment', native_id, 0]},
                 'context': {'receipt_sha256': digest, 'capture_key_nonce': native_id,
                             'files': files, 'verification': report}, 'blobs': blobs}
        return bean(log, 'append', data=json.dumps(claim).encode() + b'\n').decode().strip()


def bounded(fd, name, limit):
    if not re.fullmatch(r'[0-9a-f]{64}(?:\.json)?', name):
        raise ValueError('unsafe export name')
    f = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=fd)
    try:
        info = os.fstat(f)
        if not stat.S_ISREG(info.st_mode) or info.st_size > limit:
            raise ValueError('unsafe/large export file')
        with os.fdopen(f, 'rb', closefd=False) as stream:
            content = stream.read(limit + 1)
        if len(content) > limit:
            raise ValueError('large export file')
        return content
    finally:
        os.close(f)


def recover(export, record_id, output):
    if not re.fullmatch('sha256:[0-9a-f]{64}', record_id):
        raise ValueError('record id')
    destination = Path(output)
    if destination.exists() or destination.is_symlink():
        raise ValueError('output must not exist')
    root = os.open(export, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        records = os.open('records', os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=root)
        try:
            claim = json.loads(bounded(records, record_id[7:] + '.json', 1048576))
        finally:
            os.close(records)
        if claim['type'] != 'instrument.ingest':
            raise ValueError('not an adoption')
        files = claim['context']['files']
        if set(files) != set(NAMES):
            raise ValueError('file map')
        blobs = os.open('blobs', os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=root)
        try:
            with tempfile.TemporaryDirectory(prefix='.evidence-', dir=destination.parent) as temp:
                stage = Path(temp) / 'capture'
                stage.mkdir(mode=0o700)
                for name in NAMES:
                    bid = files[name]
                    if not isinstance(bid, str) or not re.fullmatch('sha256:[0-9a-f]{64}', bid):
                        raise ValueError('blob id')
                    (stage / name).write_bytes(bounded(blobs, bid[7:], LIMITS[name]))
                digest = 'sha256:' + hashlib.sha256((stage / 'receipt.jws').read_bytes()).hexdigest()
                if digest != claim['context']['receipt_sha256']:
                    raise ValueError('receipt digest mismatch')
                report = require_verified(stage)
                # Export metadata locates files; the original JWS independently authenticates
                # evidence. This command does not certify Bean owner authority (bean verify does).
                stage.rename(destination)
                return report
        finally:
            os.close(blobs)
    finally:
        os.close(root)


if __name__ == '__main__':
    try:
        if len(sys.argv) == 4 and sys.argv[1] == 'ingest':
            print(ingest(sys.argv[2], sys.argv[3]))
        elif len(sys.argv) == 5 and sys.argv[1] == 'recover':
            print(json.dumps(recover(*sys.argv[2:]), sort_keys=True))
        else:
            raise ValueError('usage: adopt.py ingest <capture> <bean-log> | recover <plaintext-export> <record-id> <new-output>')
    except (ValueError, OSError, KeyError, TypeError, subprocess.CalledProcessError) as error:
        print('adoption failed: ' + str(error), file=sys.stderr)
        sys.exit(1)
