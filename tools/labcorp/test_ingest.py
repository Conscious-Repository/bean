import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

from ingest import ingest


def report(value):
    def columns(parts):
        line = ''
        for col, text in parts:
            line = line.ljust(col) + text
        return line
    return '\n'.join(['Specimen ID: SYNTHETIC-SPECIMEN', 'Date Collected: 01/02/2025 0815 Local', 'Basic Panel',
        columns([(6, 'Test'), (56, 'Current Result and Flag'), (92, 'Previous Result and Date'), (139, 'Units'), (158, 'Reference Interval')]),
        columns([(6, 'Glucose'), (57, value), (140, 'mg/dL'), (160, '70-99')])]).encode()


class LabIngestion(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(); self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name); self.log = self.root / 'log'
        self.bean = [os.environ['BEAN_BIN'], '-dir', str(self.log)]
        subprocess.run(self.bean + ['init'], capture_output=True, check=True)
        self.source = self.root / 'report.txt'; self.source.write_bytes(report('100'))

    def test_original_amendment_retry_and_exit(self):
        self.assertEqual(ingest(self.source, self.log)['append'], 'added 2, skipped 0')
        self.assertEqual(ingest(self.source, self.log)['append'], 'added 0, skipped 2')
        self.source.write_bytes(report('200'))
        self.assertEqual(ingest(self.source, self.log)['append'], 'added 1, skipped 1')
        # Amendment retained as a new raw report, not silently applied to old reading.
        reading = json.loads(subprocess.check_output(self.bean + ['ls', '-type', 'lab.glucose', '-json']))
        self.assertEqual(reading['value']['n'], 100)
        subprocess.run(self.bean + ['verify'], capture_output=True, check=True)
        out = self.root / 'export'
        subprocess.run(self.bean + ['export', str(out)], capture_output=True, check=True)
        self.source.unlink()
        self.assertEqual({p.read_bytes() for p in (out / 'blobs').iterdir()}, {report('100'), report('200')})

    def test_invalid_report_no_writes(self):
        self.source.write_bytes(b'PRIVATE-INVALID-REPORT')
        with self.assertRaisesRegex(ValueError, 'no Bean writes'):
            ingest(self.source, self.log)
        self.assertFalse(subprocess.check_output(self.bean + ['ls', '-type', 'lab.report', '-json']))
        self.assertFalse([p for p in (self.log / 'blobs').rglob('*') if p.is_file()])

if __name__ == '__main__':
    unittest.main()
