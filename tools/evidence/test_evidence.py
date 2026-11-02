#!/usr/bin/env python3
"""Cross-language mutation corpus. No network or compiler required at test time."""
import base64
import copy
import hashlib
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import tempfile
import unittest

from fixture import b64, canonical, fixture, sign
from verify import verify

GO = os.environ['EVIDENCE_BIN']
BEAN = os.environ.get('BEAN_BIN')
HERE = Path(__file__).resolve().parent


class Evidence(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.capture = self.root / 'capture'
        fixture(self.capture)
        self.h, self.p = [json.loads(base64.urlsafe_b64decode(x + b'=' * (-len(x) % 4)))
                          for x in (self.capture / 'receipt.jws').read_bytes().split(b'.')[:2]]

    def receipt(self):
        (self.capture / 'receipt.jws').write_bytes(sign(self.h, self.p))

    def agree(self, code, signature=False, evidence=False, replay=False, directory=None):
        directory = directory or self.capture
        p = subprocess.run([GO, 'verify', str(directory)], capture_output=True, check=False)
        got = json.loads(p.stdout)
        independent, independent_code = verify(directory)
        self.assertEqual((p.returncode, got), (independent_code, independent))
        self.assertEqual(p.returncode, code, got)
        self.assertEqual(got['signature_valid'], signature)
        self.assertEqual(got['evidence_complete'], evidence)
        self.assertEqual(got['replay_available'], replay)
        self.assertFalse(got['source_trusted'])
        self.assertFalse(got['accuracy_validated'])
        self.assertEqual(got['assurance'], 'host-software')
        self.assertEqual(got['freshness'], 'unknown')
        return got

    def test_positive_and_replay(self):
        self.assertEqual(self.agree(0, True, True, True)['mean_square_iq'], 3317.25)
        self.agree(0, True, True, True)

    @unittest.skipUnless(os.environ.get('EVIDENCE_OFFLINE') == '1', 'Linux network namespace CI only')
    def test_network_is_disabled(self):
        with socket.socket() as connection:
            connection.settimeout(1)
            self.assertNotEqual(connection.connect_ex(('1.1.1.1', 443)), 0)
        self.agree(0, True, True, True)

    def test_python_cli(self):
        run = subprocess.run([sys.executable, str(HERE / 'verify.py'), 'verify', str(self.capture)],
                             capture_output=True, check=True)
        self.assertEqual(json.loads(run.stdout), verify(self.capture)[0])

    def test_sample_order_mutation(self):
        path = self.capture / 'raw.bin'
        data = path.read_bytes()
        path.write_bytes(data[4:8] + data[:4] + data[8:])
        self.agree(5, True)

    def test_vector_reproducible(self):
        for name in ('receipt.jws', 'raw.bin', 'settings.json'):
            self.assertEqual((self.capture / name).read_bytes(), (HERE / 'fixture' / name).read_bytes())

    def test_go_signer(self):
        other = self.root / 'go'
        subprocess.run([GO, 'fixture', str(other)], check=True)
        self.agree(0, True, True, True, other)
        self.assertNotEqual((other / 'receipt.jws').read_bytes(), (self.capture / 'receipt.jws').read_bytes())
        self.assertNotEqual(subprocess.run([GO, 'fixture', str(other)], capture_output=True).returncode, 0)

    def test_artifact_mutations(self):
        for name in ('raw.bin', 'settings.json'):
            for mode in ('bit', 'missing', 'truncate', 'extend', 'large', 'symlink', 'fifo'):
                with self.subTest(name=name, mode=mode):
                    fixture(self.capture)
                    path = self.capture / name
                    data = path.read_bytes()
                    if mode == 'bit': path.write_bytes(bytes([data[0] ^ 1]) + data[1:])
                    if mode == 'truncate': path.write_bytes(data[:-1])
                    if mode == 'extend': path.write_bytes(data + b'0')
                    if mode == 'large':
                        with path.open('wb') as f: f.truncate(2**30)
                    if mode in ('missing', 'symlink', 'fifo'): path.unlink()
                    if mode == 'symlink': path.symlink_to('/etc/passwd')
                    if mode == 'fifo': os.mkfifo(path)
                    self.agree(5, True)
                    if path.exists() or path.is_symlink(): path.unlink()

    def test_swap(self):
        a, b = self.capture / 'raw.bin', self.capture / 'settings.json'
        x, y = a.read_bytes(), b.read_bytes()
        a.write_bytes(y); b.write_bytes(x)
        self.agree(5, True)

    def test_manifest_mutations(self):
        original = copy.deepcopy(self.p)
        changes = [lambda p: p['artifacts'].reverse(),
                   lambda p: p['artifacts'].append(p['artifacts'][0]),
                   lambda p: p['artifacts'].__setitem__(1, p['artifacts'][0]),
                   lambda p: p['artifacts'][0].update(name='../raw.bin'),
                   lambda p: p['artifacts'][0].update(name='/etc/passwd'),
                   lambda p: p['artifacts'][0].update(length=True),
                   lambda p: p['artifacts'][0].update(length=0),
                   lambda p: p['artifacts'][0].update(length=2**40),
                   lambda p: p.update(complete=False),
                   lambda p: p.pop('complete'),
                   lambda p: p.update(assurance='device-hardware'),
                   lambda p: p.update(capture='x'),
                   lambda p: p.update(patient='forbidden')]
        for change in changes:
            self.p = copy.deepcopy(original); change(self.p); self.receipt()
            self.agree(2, True)

    def test_unsupported(self):
        self.h['alg'] = 'none'; self.receipt(); self.agree(3)
        self.h['alg'] = 'Ed25519'; self.p['profile'] = 'future'; self.receipt(); self.agree(3)

    def test_signature(self):
        parts = (self.capture / 'receipt.jws').read_bytes().split(b'.')
        parts[2] = b64(bytes(64)).encode()
        (self.capture / 'receipt.jws').write_bytes(b'.'.join(parts)); self.agree(4)
        self.h['jwk']['x'] = b64(bytes(range(32))); self.receipt(); self.agree(4)

    def test_small_order_and_noncanonical_points(self):
        prime = 2**255 - 19
        y8 = int('05fc536d880238b13933c6d305acdfd5f098eff289f4c345b027b2c28f95e826', 16)
        for y in (0, 1, prime-1, y8, prime-y8, prime, prime+1):
            for sign_bit in (0, 1):
                pub = (y | (sign_bit << 255)).to_bytes(32, 'little')
                self.h['jwk']['x'] = b64(pub)
                msg = b64(canonical(self.h)) + '.' + b64(canonical(self.p))
                # Identity R / S=0 is a real trivial forgery for the identity key.
                (self.capture / 'receipt.jws').write_text(msg + '.' + b64(bytes([1]) + bytes(63)))
                self.agree(4)
        fixture(self.capture)
        parts = (self.capture / 'receipt.jws').read_text().split('.')
        for sig in (bytes([1])+bytes(63), bytes([255])*64):
            (self.capture / 'receipt.jws').write_text('.'.join(parts[:2]) + '.' + b64(sig))
            self.agree(4)

    def test_unknown_issuer_can_fabricate(self):
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
        stranger = Ed25519PrivateKey.generate()
        self.h['jwk']['x'] = b64(stranger.public_key().public_bytes_raw())
        message = b64(canonical(self.h)) + '.' + b64(canonical(self.p))
        (self.capture / 'receipt.jws').write_text(message + '.' + b64(stranger.sign(message.encode())))
        self.agree(0, True, True, True)

    def test_extra_and_directory_symlink(self):
        alias = self.root / 'alias'; alias.symlink_to(self.capture)
        self.agree(2, directory=alias)
        (self.capture / 'extra').write_text('extra'); self.agree(2, True)

    def test_receipt_limits_and_parser(self):
        for data in (b'x' * 16385, b'a.b.c\n', b'a.b.c.d', b'...'):
            (self.capture / 'receipt.jws').write_bytes(data); self.agree(2)
        for payload in (b'{"profile":"a","profile":"b"}', b'[' * 20 + b']' * 20,
                        b'{"profile":"\\ud800"}', b'null', b'{"a":1.0}', b'{"a":-0}', b'{"a":9007199254740992}'):
            receipt = b64(canonical(self.h)) + '.' + b64(payload) + '.' + b64(bytes(64))
            (self.capture / 'receipt.jws').write_text(receipt); self.agree(2)

    def test_replay_unavailable(self):
        for settings in ({'method':'missing'}, {'format':'complex-i16-le','frames':99,
                          'sample_rate_hz':1000,'method':'mean-square-iq-counts@1'}):
            data = canonical(settings)
            (self.capture / 'settings.json').write_bytes(data)
            self.p['artifacts'][1].update(length=len(data), sha256=hashlib.sha256(data).hexdigest())
            self.receipt(); self.agree(0, True, True)


@unittest.skipUnless(BEAN, 'BEAN_BIN required for adoption tests')
class Adoption(Evidence):
    # Inherit the same evidence tests, and exercise the existing Bean CLI boundary.
    def setUp(self):
        super().setUp()
        self.log = self.root / 'log'
        self.log.mkdir()
        self.run_bean('init')

    def run_bean(self, *args):
        return subprocess.run([BEAN, '-dir', str(self.log), *args], capture_output=True, check=True).stdout

    def claims(self):
        return [json.loads(line) for line in self.run_bean('ls', '-type', 'instrument.ingest', '-json').splitlines()]

    def test_adopt_export_fresh_directory(self):
        from adopt import ingest, recover
        self.assertEqual(ingest(self.capture, self.log), 'added 1, skipped 0')
        self.assertEqual(ingest(self.capture, self.log), 'added 0, skipped 1')
        claims = self.claims(); self.assertEqual(len(claims), 1)
        claim = claims[0]
        self.assertEqual(claim['author'], claim['subject'])
        self.run_bean('verify')
        # The independent Bean verifier also checks owner authority/signature.
        subprocess.run([sys.executable, str(HERE.parent / 'second' / 'beanverify.py'), str(self.log)],
                       check=True, capture_output=True)
        export = self.root / 'exit'
        self.run_bean('export', str(export))
        fresh = self.root / 'fresh-machine'; fresh.mkdir()
        shutil.copytree(export, fresh / 'exit')
        # Neither original log, identity secret nor capture remains available.
        original = {p.name: p.read_bytes() for p in self.capture.iterdir()}
        shutil.rmtree(self.log); shutil.rmtree(self.capture); shutil.rmtree(export)
        recovered = fresh / 'capture'
        report = recover(fresh / 'exit', claim['id'], recovered)
        self.assertEqual(report['mean_square_iq'], 3317.25)
        self.agree(0, True, True, True, recovered)
        for name, content in original.items():
            self.assertEqual((recovered / name).read_bytes(), content)

    def test_failed_verification_no_claim_or_blobs(self):
        from adopt import ingest
        (self.capture / 'raw.bin').unlink()
        with self.assertRaises(ValueError): ingest(self.capture, self.log)
        self.assertEqual(self.claims(), [])
        self.assertFalse(list((self.log / 'blobs').rglob('*')))

    def test_crash_retry_and_failed_append(self):
        from adopt import ingest
        from unittest.mock import patch
        import adopt
        real = adopt.bean
        def crash(log, *args, **kwargs):
            if args[0] == 'append': raise RuntimeError('simulated crash after blobs, before claim')
            return real(log, *args, **kwargs)
        with patch.object(adopt, 'bean', crash), self.assertRaises(RuntimeError):
            ingest(self.capture, self.log)
        self.assertEqual(self.claims(), [])
        self.assertTrue(list((self.log / 'blobs').rglob('*')))
        self.assertEqual(ingest(self.capture, self.log), 'added 1, skipped 0')
        # An actual Bean signing failure also creates no new claim.
        key = self.log / 'local' / 'signing-key'
        saved = key.read_bytes(); key.unlink()
        fresh = self.root / 'new-capture'
        subprocess.run([GO, 'fixture', str(fresh)], check=True)
        with self.assertRaises(subprocess.CalledProcessError): ingest(fresh, self.log)
        key.write_bytes(saved)
        self.assertEqual(len(self.claims()), 1)
        self.assertEqual(ingest(fresh, self.log), 'added 1, skipped 0')

    def test_corrupt_blob_and_export(self):
        from adopt import ingest, recover
        ingest(self.capture, self.log)
        claim = self.claims()[0]
        export = self.root / 'exit'; self.run_bean('export', str(export))
        bid = claim['context']['files']['raw.bin'][7:]
        (export / 'blobs' / bid).write_bytes(b'corrupt')
        with self.assertRaises(ValueError): recover(export, claim['id'], self.root / 'bad-recovery')
        self.assertFalse((self.root / 'bad-recovery').exists())
        blob = self.log / 'blobs' / bid[:2] / bid[2:]
        blob.write_bytes(b'corrupt')
        with self.assertRaises(subprocess.CalledProcessError): self.run_bean('export', str(self.root / 'corrupt-exit'))

    def test_conflicting_nonce(self):
        from adopt import ingest
        ingest(self.capture, self.log)
        self.p['artifacts'][0]['sha256'] = hashlib.sha256(bytes(16)).hexdigest()
        (self.capture / 'raw.bin').write_bytes(bytes(16)); self.receipt()
        with self.assertRaisesRegex(ValueError, 'conflicting'): ingest(self.capture, self.log)
        self.assertEqual(len(self.claims()), 1)


if __name__ == '__main__':
    unittest.main()
