#!/usr/bin/env python3
"""Driver lifecycle tests; simulations make no device-origin claims."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

from acquisition import SimulatedDevice, transfer
from verify import decode, verify


class Acquisition(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.device = SimulatedDevice()

    def finish(self):
        self.device.start(sample_rate_hz=1001)
        self.device.acquire(2)
        self.device.acquire(2)
        return self.device.finalize()

    def test_complete_applied_settings_and_dual_verification(self):
        source = self.finish()
        out = self.root / 'capture'
        transfer(source, out)
        self.assertEqual(json.loads((out / 'settings.json').read_bytes())['sample_rate_hz'], 1000)
        report, code = verify(out)
        self.assertEqual(code, 0)
        self.assertEqual(report['mean_square_iq'], 3317.25)
        self.assertEqual(report['assurance'], 'host-software')
        go = subprocess.run([os.environ['EVIDENCE_BIN'], 'verify', str(out)], capture_output=True, check=True)
        self.assertEqual(json.loads(go.stdout), report)

    def test_premature_completion_abort_and_overflow(self):
        self.device.start()
        self.device.acquire(1)
        with self.assertRaisesRegex(ValueError, 'incomplete'): self.device.finalize()
        self.device.abort()
        with self.assertRaises(ValueError): self.device.finalize()
        self.device.reset()
        self.device.start()
        with self.assertRaisesRegex(ValueError, 'overflow'): self.device.acquire(5)
        with self.assertRaises(ValueError): self.device.finalize()

    def test_reset_new_identity_and_finalize_retry(self):
        first = self.finish()
        self.assertEqual(self.device.finalize().receipt, first.receipt)
        self.device.reset()
        with self.assertRaises(ValueError): self.device.finalize()
        second = self.finish()
        a = json.loads(decode(first.receipt.decode().split('.')[1]))
        b = json.loads(decode(second.receipt.decode().split('.')[1]))
        self.assertNotEqual(a['capture'], b['capture'])
        # A detached completed result survives device reset and remains historical evidence.
        transfer(first, self.root / 'old')
        self.assertEqual(verify(self.root / 'old')[1], 0)

    def test_no_host_supplied_signing_input_and_state_changes(self):
        self.device.start()
        with self.assertRaises(ValueError): self.device.start()
        with self.assertRaises(TypeError): self.device.finalize(b'host supplied hash')
        with self.assertRaises(ValueError): self.device.acquire(-1)
        self.device.acquire(4)
        source = self.device.finalize()
        with self.assertRaises(ValueError): self.device.acquire(1)
        self.assertIsInstance(source.read('raw.bin', 0, 4), bytes)
        with self.assertRaises(ValueError): source.read('../raw.bin', 0, 4)

    def test_transfer_interruption_tampering_missing_extra_and_retry(self):
        original = self.finish()
        class Faulty:
            receipt = original.receipt
            def __init__(self, fault): self.fault = fault
            def read(self, name, offset, count):
                if name == 'raw.bin':
                    if self.fault == 'disconnect': raise OSError('disconnected')
                    if self.fault == 'missing': return b''
                    if self.fault == 'extra' and offset == 16: return b'x'
                result = original.read(name, offset, count)
                if self.fault == 'tamper' and name == 'raw.bin' and offset == 0:
                    return bytes([result[0] ^ 1]) + result[1:]
                return result
        out = self.root / 'capture'
        for fault in ('disconnect', 'missing', 'extra', 'tamper'):
            with self.subTest(fault=fault), self.assertRaises((ValueError, OSError)):
                transfer(Faulty(fault), out)
            self.assertFalse(out.exists())
        transfer(original, out)
        with self.assertRaises(FileExistsError): transfer(original, out)
        self.assertEqual(verify(out)[1], 0)

    def test_fragmented_transfer(self):
        original = self.finish()
        class Fragmented:
            receipt = original.receipt
            def read(self, name, offset, count): return original.read(name, offset, min(count, 3))
        out = self.root / 'capture'
        transfer(Fragmented(), out)
        self.assertEqual(verify(out)[1], 0)

    def test_publication_failure_refuses_partial_bundle_and_fresh_retry(self):
        from unittest.mock import patch
        original = self.finish()
        replace = os.replace
        names = ('raw.bin', 'settings.json', 'receipt.jws')
        for index, failed_name in enumerate(names):
            with self.subTest(failed_name=failed_name):
                out = self.root / ('partial-' + str(index))
                def fail_publish(source, destination):
                    if Path(destination).name == failed_name:
                        raise OSError('injected publication failure')
                    return replace(source, destination)
                with patch('acquisition.os.replace', side_effect=fail_publish):
                    with self.assertRaisesRegex(OSError, 'injected publication failure'):
                        transfer(original, out)
                self.assertEqual({p.name for p in out.iterdir()}, set(names[:index]))
                self.assertFalse((out / 'receipt.jws').exists())
                report, code = verify(out)
                self.assertEqual(code, 2)
                self.assertFalse(report['signature_valid'])
                self.assertFalse(report['evidence_complete'])
                go = subprocess.run([os.environ['EVIDENCE_BIN'], 'verify', str(out)],
                                    capture_output=True, check=False)
                self.assertEqual((go.returncode, json.loads(go.stdout)), (code, report))
                with self.assertRaises(FileExistsError):
                    transfer(original, out)
                fresh = self.root / ('retry-' + str(index))
                successful = transfer(original, fresh)
                self.assertEqual((fresh / 'receipt.jws').read_bytes(), original.receipt)
                self.assertEqual((fresh / 'raw.bin').read_bytes(), original.raw)
                self.assertEqual((fresh / 'settings.json').read_bytes(), original.settings)
                self.assertTrue(successful['evidence_complete'])
                go = subprocess.run([os.environ['EVIDENCE_BIN'], 'verify', str(fresh)],
                                    capture_output=True, check=True)
                self.assertEqual(json.loads(go.stdout), successful)


if __name__ == '__main__':
    unittest.main()
