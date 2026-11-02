import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
import zipfile

from ingest import ingest

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
        with self.assertRaises((ValueError, subprocess.CalledProcessError)):
            ingest(self.source, self.log)
        self.assertFalse(subprocess.check_output(self.bean + ['ls', '-type', 'apple-health.export', '-json']))
        self.assertFalse([p for p in (self.log / 'blobs').rglob('*') if p.is_file()])

    def test_purged_archive_is_not_reported_as_retained(self):
        ingest(self.source, self.log)
        record = json.loads(subprocess.check_output(self.bean + ['ls', '-type', 'apple-health.export', '-json']))
        subprocess.run(self.bean + ['purge', record['id']], capture_output=True, check=True)
        with self.assertRaisesRegex(ValueError, 'not retained'):
            ingest(self.source, self.log)

    def test_malformed_competing_archive_dedupe_is_detected(self):
        bid = subprocess.check_output(self.bean + ['blob', 'put', str(self.source)]).decode().strip()
        record = json.loads(subprocess.check_output([os.environ['APPLE_HEALTH_BIN'], '-blob', bid, str(self.source)]))
        record['context']['sha256'] = '0' * 64
        subprocess.run(self.bean + ['append'], input=json.dumps(record).encode(), capture_output=True, check=True)
        with self.assertRaisesRegex(ValueError, 'not retained'):
            ingest(self.source, self.log)

    def test_corrupt_stored_blob_cannot_be_adopted(self):
        bid = subprocess.check_output(self.bean + ['blob', 'put', str(self.source)]).decode().strip()[7:]
        (self.log / 'blobs' / bid[:2] / bid[2:]).write_bytes(b'corrupt')
        with self.assertRaises((ValueError, subprocess.CalledProcessError)):
            ingest(self.source, self.log)
        self.assertFalse(subprocess.check_output(self.bean + ['ls', '-type', 'apple-health.export', '-json']))

if __name__ == '__main__':
    unittest.main()
