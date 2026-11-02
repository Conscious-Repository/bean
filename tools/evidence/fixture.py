#!/usr/bin/env python3
"""Public test key only. Writes a deterministic synthetic vector, never health data."""
import base64
import hashlib
import json
from pathlib import Path
import struct
import sys
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey


def canonical(v):
    return json.dumps(v, sort_keys=True, separators=(',', ':')).encode()


def b64(v):
    return base64.urlsafe_b64encode(v).rstrip(b'=').decode()


def sign(header, payload):
    message = b64(canonical(header)) + '.' + b64(canonical(payload))
    key = Ed25519PrivateKey.from_private_bytes(bytes(range(32)))
    return (message + '.' + b64(key.sign(message.encode()))).encode()


def fixture(directory):
    directory.mkdir(parents=True, exist_ok=True)
    key = Ed25519PrivateKey.from_private_bytes(bytes(range(32)))
    h = {'alg': 'Ed25519', 'typ': 'bean-evidence-experiment',
         'jwk': {'kty': 'OKP', 'crv': 'Ed25519', 'x': b64(key.public_key().public_bytes_raw())}}
    data = {'raw.bin': struct.pack('<8h', 100, 0, 0, 50, -25, 0, 0, -12),
            'settings.json': canonical({'format': 'complex-i16-le', 'frames': 4,
                                        'sample_rate_hz': 1000, 'method': 'mean-square-iq-counts@1'})}
    p = {'profile': 'bean-evidence-experiment', 'capture': '000102030405060708090a0b0c0d0e0f',
         'complete': True, 'assurance': 'host-software', 'artifacts': []}
    for role, name in [('raw', 'raw.bin'), ('settings', 'settings.json')]:
        (directory / name).write_bytes(data[name])
        p['artifacts'].append({'role': role, 'name': name, 'length': len(data[name]),
                               'sha256': hashlib.sha256(data[name]).hexdigest()})
    (directory / 'receipt.jws').write_bytes(sign(h, p))


if __name__ == '__main__':
    fixture(Path(sys.argv[1]))
