#!/usr/bin/env python3
"""Independent offline verifier, written from the experiment ADR before Go code."""
import base64
import hashlib
import json
import os
import re
import stat
import struct
import sys
from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey

PROFILE = 'bean-evidence-experiment'
LIMITS = {'receipt.jws': 16384, 'raw.bin': 1048576, 'settings.json': 4096}


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=True).encode()


def parse(data):
    # The small profile needs no escaped/non-ASCII strings, floats or deep trees.
    if any(c > 127 for c in data) or b'\\' in data:
        raise ValueError('JSON subset')
    depth = 0
    quoted = False
    for c in data:
        if c == 34:
            quoted = not quoted
        if not quoted:
            depth += (c in (91, 123)) - (c in (93, 125))
            if depth > 8:
                raise ValueError('depth')
    value = json.loads(data, parse_float=lambda _: (_ for _ in ()).throw(ValueError('integer only')),
                       parse_constant=lambda _: (_ for _ in ()).throw(ValueError('finite only')))
    if canonical(value) != data:
        raise ValueError('noncanonical or duplicate JSON')
    return value


def decode(s):
    if not isinstance(s, str) or not re.fullmatch(r'[A-Za-z0-9_-]+', s):
        raise ValueError('base64url')
    out = base64.urlsafe_b64decode(s + '=' * (-len(s) % 4))
    if base64.urlsafe_b64encode(out).rstrip(b'=').decode() != s:
        raise ValueError('base64url')
    return out


def safe_point(encoded):
    if len(encoded) != 32:
        return False
    y = int.from_bytes(encoded, 'little') & (2**255 - 1)
    prime = 2**255 - 19
    order8_y = int('05fc536d880238b13933c6d305acdfd5f098eff289f4c345b027b2c28f95e826', 16)
    return y < prime and y not in {0, 1, prime - 1, order8_y, prime - order8_y}


def read_file(fd, name):
    # Fixed basenames, anchored directory descriptor, no symlink following.
    f = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=fd)
    try:
        info = os.fstat(f)
        if not stat.S_ISREG(info.st_mode) or info.st_size > LIMITS[name]:
            raise ValueError('unsafe or oversized file')
        chunks = []
        size = 0
        digest = hashlib.sha256()
        while True:
            block = os.read(f, min(65536, LIMITS[name] + 1 - size))
            if not block:
                break
            size += len(block)
            if size > LIMITS[name]:
                raise ValueError('oversized file')
            digest.update(block)
            chunks.append(block)
        return b''.join(chunks), digest.hexdigest()
    finally:
        os.close(f)


def verify(directory):
    result = dict(signature_valid=False, evidence_complete=False, source_trusted=False,
                  replay_available=False, accuracy_validated=False,
                  assurance='host-software', freshness='unknown', status='invalid')
    code = 2
    fd = None
    try:
        fd = os.open(directory, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        receipt, _ = read_file(fd, 'receipt.jws')
        parts = receipt.decode('ascii').split('.')
        if len(parts) != 3:
            raise ValueError('JWS')
        h, p = parse(decode(parts[0])), parse(decode(parts[1]))
        if not isinstance(h, dict) or not isinstance(p, dict):
            raise ValueError('objects required')
        if h.get('alg') != 'Ed25519' or h.get('typ') != PROFILE or p.get('profile') != PROFILE:
            result['status'] = 'unsupported'
            return result, 3
        if set(h) != {'alg', 'typ', 'jwk'} or not isinstance(h['jwk'], dict):
            raise ValueError('header')
        key = h['jwk']
        if set(key) != {'kty', 'crv', 'x'} or key['kty'] != 'OKP' or key['crv'] != 'Ed25519':
            raise ValueError('key')
        code = 4
        result['status'] = 'signature_invalid'
        pub, sig = decode(key['x']), decode(parts[2])
        if not safe_point(pub) or len(sig) != 64 or not safe_point(sig[:32]):
            raise ValueError('key/signature size')
        Ed25519PublicKey.from_public_bytes(pub).verify(sig, '.'.join(parts[:2]).encode())
        result['signature_valid'] = True
        code = 2
        result['status'] = 'invalid'
        if (set(p) != {'profile', 'capture', 'complete', 'assurance', 'artifacts'}
                or p['complete'] is not True or p['assurance'] != 'host-software'
                or not isinstance(p['capture'], str)
                or not re.fullmatch('[0-9a-f]{32}', p['capture'])
                or not isinstance(p['artifacts'], list) or len(p['artifacts']) != 2):
            raise ValueError('manifest')
        for a, role, name in zip(p['artifacts'], ('raw', 'settings'), ('raw.bin', 'settings.json')):
            if (not isinstance(a, dict) or set(a) != {'name', 'role', 'length', 'sha256'}
                    or a['name'] != name or a['role'] != role
                    or type(a['length']) is not int or not 0 < a['length'] <= LIMITS[name]
                    or not isinstance(a['sha256'], str) or not re.fullmatch('[0-9a-f]{64}', a['sha256'])):
                raise ValueError('artifact table')
        # Bounded enumeration: no unbounded listdir on attacker-controlled directories.
        with os.scandir(fd) as entries:
            names = []
            for entry in entries:
                names.append(entry.name)
                if len(names) > 3:
                    raise ValueError('extra file')
        if any(n not in LIMITS for n in names):
            raise ValueError('extra file')
        code = 5
        result['status'] = 'evidence_mismatch'
        data = {}
        for a in p['artifacts']:
            content, digest = read_file(fd, a['name'])
            if len(content) != a['length'] or digest != a['sha256']:
                raise ValueError('hash/length')
            data[a['name']] = content
        result.update(evidence_complete=True, status='complete')
        try:
            s = parse(data['settings.json'])
            if (isinstance(s, dict) and set(s) == {'format', 'frames', 'sample_rate_hz', 'method'}
                    and s['format'] == 'complex-i16-le' and s['method'] == 'mean-square-iq-counts@1'
                    and type(s['frames']) is int and 0 < s['frames'] <= 262144
                    and type(s['sample_rate_hz']) is int and 0 < s['sample_rate_hz'] <= 2147483647
                    and len(data['raw.bin']) == s['frames'] * 4):
                energy = sum(i*i + q*q for i, q in struct.iter_unpack('<hh', data['raw.bin']))
                result.update(replay_available=True, mean_square_iq=energy / s['frames'])
        except (ValueError, TypeError, RecursionError):
            pass
        return result, 0
    except (ValueError, OSError, TypeError, KeyError, InvalidSignature, RecursionError):
        return result, code
    finally:
        if fd is not None:
            os.close(fd)


if __name__ == '__main__':
    if len(sys.argv) != 3 or sys.argv[1] != 'verify':
        print('usage: verify.py verify <capture-directory>', file=sys.stderr)
        sys.exit(1)
    result, code = verify(sys.argv[2])
    print(json.dumps(result, sort_keys=True))
    sys.exit(code)
