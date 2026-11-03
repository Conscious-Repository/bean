import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import zipfile

from ingest import IngestError, ingest

XML = b'''<?xml version="1.0"?><HealthData locale="en_US"><Me HKCharacteristicTypeIdentifierDateOfBirth="1900-01-01"/><Record type="HKQuantityTypeIdentifierStepCount" sourceName="Private Name" device="Private Device" unit="count" creationDate="2026-01-01 10:00:00 -0600" startDate="2026-01-01 09:00:00 -0600" endDate="2026-01-01 09:05:00 -0600" value="25"/></HealthData>'''

class Ingestion(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.log = self.root / 'log'
        self.bean = [os.environ['BEAN_BIN'], '-dir', str(self.log)]
        subprocess.run(self.bean + ['init'], capture_output=True, check=True)
        self.source = self.root / 'export.zip'
        with zipfile.ZipFile(self.source, 'w') as z:
            z.writestr('apple_health_export/export.xml', XML)

    def test_archive_retry_and_offline_exit(self):
        first = ingest(self.source, self.log)
        self.assertEqual(first['append'], 'added 1, skipped 0')
        second = ingest(self.source, self.log)
        self.assertEqual(second['append'], 'added 0, skipped 1')
        rows = subprocess.check_output(self.bean + ['ls', '-type', 'apple-health.export', '-json'])
        claim = json.loads(rows)
        self.assertNotIn(b'Private Name', rows)
        original = self.source.read_bytes()
        self.assertEqual(claim['context']['sha256'], hashlib.sha256(original).hexdigest())
        export = self.root / 'export'
        subprocess.run(self.bean + ['verify'], capture_output=True, check=True)
        subprocess.run(self.bean + ['export', str(export)], capture_output=True, check=True)
        self.source.unlink()
        recovered = list((export / 'blobs').iterdir())
        self.assertEqual(len(recovered), 1)
        self.assertEqual(recovered[0].read_bytes(), original)
        with zipfile.ZipFile(recovered[0]) as z:
            self.assertEqual(z.read('apple_health_export/export.xml'), XML)

    def test_optional_records_retry_and_changed_export(self):
        self.assertEqual(ingest(self.source, self.log, records=True)['append'], 'added 2, skipped 0')
        self.assertEqual(ingest(self.source, self.log, records=True)['append'], 'added 0, skipped 2')
        # A changed snapshot remains independently recoverable even when a reading dedupes.
        with zipfile.ZipFile(self.source, 'w') as z:
            z.writestr('apple_health_export/export.xml', XML.replace(b'en_US', b'en_GB'))
        self.assertEqual(ingest(self.source, self.log, records=True)['append'], 'added 1, skipped 1')
        subprocess.run(self.bean + ['verify'], capture_output=True, check=True)

    def test_malformed_input_writes_no_claim_or_blob(self):
        self.source.write_bytes(b'<HealthData><Record')
        with self.assertRaises(IngestError) as caught:
            ingest(self.source, self.log)
        self.assertEqual(caught.exception.phase, 'validation')
        self.assertIn('no Bean writes attempted', str(caught.exception))
        self.assertFalse(subprocess.check_output(self.bean + ['ls', '-type', 'apple-health.export', '-json']))
        self.assertFalse([p for p in (self.log / 'blobs').rglob('*') if p.is_file()])

    def test_purged_archive_is_not_reported_as_retained(self):
        ingest(self.source, self.log)
        record = json.loads(subprocess.check_output(self.bean + ['ls', '-type', 'apple-health.export', '-json']))
        subprocess.run(self.bean + ['purge', record['id']], capture_output=True, check=True)
        with self.assertRaisesRegex(ValueError, 'not retained'):
            ingest(self.source, self.log)

    def test_purged_archive_retry_does_not_append_new_readings(self):
        ingest(self.source, self.log)
        record = json.loads(subprocess.check_output(self.bean + ['ls', '-type', 'apple-health.export', '-json']))
        subprocess.run(self.bean + ['purge', record['id']], capture_output=True, check=True)
        with self.assertRaises(IngestError):
            ingest(self.source, self.log, records=True)
        self.assertFalse(subprocess.check_output(self.bean + ['ls', '-type', 'apple-health.steps', '-json']))

    def test_other_subject_archive_does_not_mask_purged_self_archive(self):
        ingest(self.source, self.log)
        record = json.loads(subprocess.check_output(self.bean + ['ls', '-type', 'apple-health.export', '-json']))
        bid = record['value']['blob']
        other = json.loads(subprocess.check_output([os.environ['APPLE_HEALTH_BIN'], '-blob', bid, str(self.source)]))
        other['subject'] = 'sha256:' + '7' * 64
        subprocess.run(self.bean + ['append'], input=json.dumps(other).encode(), capture_output=True, check=True)
        subprocess.run(self.bean + ['purge', record['id']], capture_output=True, check=True)
        with self.assertRaisesRegex(IngestError, 'not retained'):
            ingest(self.source, self.log)

    def test_malformed_competing_archive_dedupe_is_detected(self):
        bid = subprocess.check_output(self.bean + ['blob', 'put', str(self.source)]).decode().strip()
        record = json.loads(subprocess.check_output([os.environ['APPLE_HEALTH_BIN'], '-blob', bid, str(self.source)]))
        record['context']['sha256'] = '0' * 64
        subprocess.run(self.bean + ['append'], input=json.dumps(record).encode(), capture_output=True, check=True)
        with self.assertRaisesRegex(ValueError, 'not retained'):
            ingest(self.source, self.log)

    def test_cached_invalid_archive_is_not_reported_as_retained(self):
        ingest(self.source, self.log)
        record = json.loads(subprocess.check_output(self.bean + ['ls', '-type', 'apple-health.export', '-json']))
        path = self.log / 'objects' / (record['id'][7:9]) / (record['id'][9:] + '.json')
        before = path.stat()
        data = path.read_bytes()
        signature = record['sig'].encode()
        replacement = signature[:-1] + (b'0' if signature[-1:] != b'0' else b'1')
        path.write_bytes(data.replace(signature, replacement))
        os.utime(path, ns=(before.st_atime_ns, before.st_mtime_ns))
        stored = {p.relative_to(self.log) for p in (self.log / 'blobs').rglob('*') if p.is_file()}
        with zipfile.ZipFile(self.source, 'w') as archive:
            archive.writestr('apple_health_export/export.xml', XML.replace(b'en_US', b'en_GB'))
        with self.assertRaises(IngestError) as caught:
            ingest(self.source, self.log)
        self.assertEqual(caught.exception.phase, 'log')
        self.assertEqual({p.relative_to(self.log) for p in (self.log / 'blobs').rglob('*') if p.is_file()}, stored)

    def test_blob_corruption_after_readback_is_not_reported_as_success(self):
        original_run = subprocess.run
        def corrupt_before_append(command, **kwargs):
            if command == self.bean + ['append']:
                blobs = [p for p in (self.log / 'blobs').rglob('*') if p.is_file()]
                self.assertEqual(len(blobs), 1)
                blobs[0].write_bytes(b'SYNTHETIC-CORRUPTION')
            return original_run(command, **kwargs)
        with patch('ingest.subprocess.run', side_effect=corrupt_before_append):
            with self.assertRaises(IngestError):
                ingest(self.source, self.log)

    def test_corrupt_stored_blob_cannot_be_adopted(self):
        bid = subprocess.check_output(self.bean + ['blob', 'put', str(self.source)]).decode().strip()[7:]
        (self.log / 'blobs' / bid[:2] / bid[2:]).write_bytes(b'corrupt')
        with self.assertRaises(IngestError) as caught:
            ingest(self.source, self.log)
        self.assertEqual(caught.exception.phase, 'log')
        self.assertIn('no source blobs or claims written', str(caught.exception))
        self.assertFalse(subprocess.check_output(self.bean + ['ls', '-type', 'apple-health.export', '-json']))

    def test_append_failure_reports_partial_and_retry_recovers(self):
        first_record = XML[XML.index(b'<Record'):XML.index(b'</HealthData>')]
        with zipfile.ZipFile(self.source, 'w') as archive:
            archive.writestr('apple_health_export/export.xml', XML.replace(
                b'</HealthData>', first_record.replace(b'value="25"', b'value="26"') + b'</HealthData>'))
        original_run = subprocess.run
        append_calls = 0
        def partial_append(command, **kwargs):
            nonlocal append_calls
            if command == self.bean + ['append']:
                append_calls += 1
                if append_calls == 1:
                    return original_run(command, **kwargs)
                # Retain one reading after the archive, then fail before the next.
                first = kwargs['stdin'].readline()
                original_run(command, input=first, capture_output=True, check=True)
                raise subprocess.CalledProcessError(1, command, stderr=b'SYNTHETIC-PRIVATE-DETAIL')
            return original_run(command, **kwargs)
        with patch('ingest.subprocess.run', side_effect=partial_append):
            with self.assertRaises(IngestError) as caught:
                ingest(self.source, self.log, records=True)
        error = caught.exception
        self.assertEqual(error.phase, 'append')
        self.assertIn('partial ingestion is possible', str(error))
        self.assertNotIn('SYNTHETIC-PRIVATE-DETAIL', str(error))
        self.assertIsInstance(error.__cause__, subprocess.CalledProcessError)
        self.assertTrue(subprocess.check_output(self.bean + ['ls', '-type', 'apple-health.export', '-json']))
        readings = subprocess.check_output(self.bean + ['ls', '-type', 'apple-health.steps', '-json'])
        self.assertEqual(len(readings.splitlines()), 1)
        self.assertEqual(ingest(self.source, self.log, records=True)['append'], 'added 1, skipped 2')

    def test_cli_validation_message_is_phase_specific_and_private(self):
        source = self.root / 'SYNTHETIC-PRIVATE-FILENAME'
        source.write_bytes(b'<HealthData>SYNTHETIC-PRIVATE-CONTENT</HealthData>')
        import sys
        result = subprocess.run([sys.executable, str(Path(__file__).with_name('ingest.py')),
                                 str(source), str(self.log)], capture_output=True, text=True)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, '')
        self.assertEqual(result.stderr, 'ingestion failed: validation failed; no Bean writes attempted\n')

if __name__ == '__main__':
    unittest.main()
