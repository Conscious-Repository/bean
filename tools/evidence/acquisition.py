#!/usr/bin/env python3
"""Internal acquisition/transfer experiment, NOT a hardware API or wire standard.

The public simulator key and process memory are host-controlled. Its lifecycle
models a future driver; Python encapsulation is not a security boundary.
"""
from dataclasses import dataclass
import hashlib
import json
import os
from pathlib import Path
import secrets
import struct
import sys
import tempfile

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from fixture import b64, canonical, sign
from verify import LIMITS, decode, parse, verify


@dataclass(frozen=True)
class FinalizedCapture:
    receipt: bytes
    raw: bytes
    settings: bytes

    def read(self, name, offset, count):
        if name not in ('raw.bin', 'settings.json') or offset < 0 or count <= 0 or count > 65536:
            raise ValueError('artifact read bounds')
        data = self.raw if name == 'raw.bin' else self.settings
        return data[offset:offset + count]


class SimulatedDevice:
    """Four synthetic I/Q frames, a public test key, and an acquisition-owned buffer.

    start -> acquire -> finalize, or abort/reset. finalize accepts no hash or
    bytes from the consumer. Only the driver appends simulated ADC samples.
    """
    def __init__(self):
        self.reset()

    def reset(self):
        self._state = 'idle'
        self._raw = bytearray()
        self._result = None
        self._capture_id = None
        self._settings = None

    def start(self, *, sample_rate_hz=1000):
        if self._state != 'idle':
            raise ValueError('reset required before starting another acquisition')
        if type(sample_rate_hz) is not int or not 900 <= sample_rate_hz <= 1100:
            raise ValueError('simulator supports requests near 1000 Hz only')
        self._capture_id = secrets.token_hex(16)
        # The fake device applies 1000 Hz, regardless of the nearby requested rate.
        self._settings = canonical({'format': 'complex-i16-le', 'frames': 4,
                                    'sample_rate_hz': 1000, 'method': 'mean-square-iq-counts@1'})
        self._state = 'capturing'
        return self._capture_id

    def acquire(self, frames):
        if self._state != 'capturing' or type(frames) is not int or frames <= 0:
            raise ValueError('invalid acquisition state/count')
        start = len(self._raw) // 4
        if start + frames > 4:
            self.abort()
            raise ValueError('acquisition overflow; capture aborted')
        signal = ((100, 0), (0, 50), (-25, 0), (0, -12))
        for i, q in signal[start:start + frames]:
            self._raw.extend(struct.pack('<hh', i, q))

    def abort(self):
        if self._state != 'capturing':
            raise ValueError('no active acquisition')
        self._raw.clear()
        self._state = 'aborted'

    def finalize(self):
        if self._state == 'complete':
            return self._result  # Retrying completion returns the same original receipt.
        if self._state != 'capturing' or len(self._raw) != 16:
            raise ValueError('incomplete acquisition; no receipt')
        raw = bytes(self._raw)
        key = Ed25519PrivateKey.from_private_bytes(bytes(range(32)))
        header = {'alg': 'Ed25519', 'typ': 'bean-evidence-experiment',
                  'jwk': {'kty': 'OKP', 'crv': 'Ed25519', 'x': b64(key.public_key().public_bytes_raw())}}
        artifacts = [{'name': name, 'role': role, 'length': len(data),
                      'sha256': hashlib.sha256(data).hexdigest()}
                     for name, role, data in (('raw.bin', 'raw', raw),
                                              ('settings.json', 'settings', self._settings))]
        payload = {'profile': 'bean-evidence-experiment', 'capture': self._capture_id,
                   'complete': True, 'assurance': 'host-software', 'artifacts': artifacts}
        self._result = FinalizedCapture(sign(header, payload), raw, self._settings)
        self._state = 'complete'
        return self._result


def transfer(source, output):
    """Consume finalized receipt plus bounded artifact reads; publish only after verify.

    This transport helper is specific to the current experiment. A future driver
    can return a native envelope through a separate verifier/transfer adapter.
    Read/verification failures leave no output; retry the same finalized source.
    Publication failure may leave a partial output without a receipt: use a new
    destination. This is not a durable crash-recovery protocol.
    """
    output = Path(output)
    if output.exists() or output.is_symlink():
        raise FileExistsError('output must not exist')
    receipt = source.receipt
    if not isinstance(receipt, bytes) or len(receipt) > LIMITS['receipt.jws']:
        raise ValueError('receipt size/type')
    try:
        payload = parse(decode(receipt.decode('ascii').split('.')[1]))
        artifacts = payload['artifacts']
    except (ValueError, IndexError, KeyError, UnicodeError) as error:
        raise ValueError('invalid transfer receipt') from error
    if not isinstance(artifacts, list) or len(artifacts) != 2:
        raise ValueError('artifact table')
    with tempfile.TemporaryDirectory(prefix='.evidence-transfer-', dir=output.parent) as temp:
        stage = Path(temp) / 'capture'
        stage.mkdir(mode=0o700)
        for artifact, name in zip(artifacts, ('raw.bin', 'settings.json')):
            if not isinstance(artifact, dict):
                raise ValueError('artifact table')
            length = artifact.get('length')
            if artifact.get('name') != name or type(length) is not int or not 0 < length <= LIMITS[name]:
                raise ValueError('artifact bounds')
            with (stage / name).open('xb') as out:
                offset = 0
                while offset < length:
                    requested = min(65536, length - offset)
                    block = source.read(name, offset, requested)
                    if not isinstance(block, bytes) or not 0 < len(block) <= requested:
                        raise ValueError('incomplete or oversized transfer block')
                    out.write(block)
                    offset += len(block)
                if source.read(name, length, 1) != b'':
                    raise ValueError('extra artifact bytes')
        (stage / 'receipt.jws').write_bytes(receipt)
        report, code = verify(stage)
        if code:
            raise ValueError('transfer verification failed: ' + report['status'])
        # mkdir reserves output without replacing another writer's directory.
        output.mkdir(mode=0o700)
        try:
            for name in ('raw.bin', 'settings.json', 'receipt.jws'):
                os.replace(stage / name, output / name)
        except OSError:
            # A crash can leave an incomplete directory; verifier refuses it.
            # Never overwrite/reuse that directory automatically.
            raise
        return report


if __name__ == '__main__':
    try:
        if len(sys.argv) != 3 or sys.argv[1] != 'simulate':
            raise ValueError('usage: acquisition.py simulate <new-capture-directory>')
        device = SimulatedDevice()
        device.start()
        device.acquire(4)
        print(json.dumps(transfer(device.finalize(), sys.argv[2]), sort_keys=True))
    except (ValueError, OSError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
