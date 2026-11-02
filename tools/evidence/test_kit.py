#!/usr/bin/env python3
"""Build and run only copied kit files, with the checkout denied/hidden."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

from fixture import fixture
from kit import build, REPO
from adopt import ingest


class OfflineKit(unittest.TestCase):
    def test_unusable_wheelhouse_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            wheels = root / 'wheels'; wheels.mkdir()
            (wheels / 'cryptography-50.0.2-placeholder.whl').write_bytes(b'not a wheel')
            with self.assertRaises(subprocess.CalledProcessError):
                build(os.environ['EVIDENCE_BIN'], wheels, root / 'kit')
            self.assertFalse((root / 'kit/inventory.json').exists())

    def test_exit_with_repository_inaccessible(self):
        wheels = os.environ['EVIDENCE_WHEELS']
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            kit = build(os.environ['EVIDENCE_BIN'], wheels, root / 'kit')
            inventory = json.loads((kit / 'inventory.json').read_bytes())
            for name, digest in inventory['sha256'].items():
                self.assertEqual(hashlib.sha256((kit / name).read_bytes()).hexdigest(), digest)
            capture = root / 'capture'; fixture(capture)
            original = {p.name: p.read_bytes() for p in capture.iterdir()}
            log = root / 'log'
            bean = [os.environ['BEAN_BIN'], '-dir', str(log)]
            subprocess.run(bean + ['init'], capture_output=True, check=True)
            ingest(capture, log)
            records = subprocess.check_output(bean + ['ls', '-type', 'instrument.ingest', '-json'])
            record_id = json.loads(records)['id']
            subprocess.run(bean + ['export', str(root / 'export')], capture_output=True, check=True)
            shutil.rmtree(capture); shutil.rmtree(log)
            # A standalone runner outside the repository with only kit/export paths.
            runner = root / 'run.py'
            runner.write_text('''import json, pathlib, subprocess, sys
root = pathlib.Path(__file__).resolve().parent
kit = root / 'kit'
subprocess.run([sys.executable, '-m', 'venv', str(root / 'runtime')], check=True)
py = str(root / 'runtime/bin/python')
subprocess.run([py, '-m', 'pip', 'install', '--no-index', '--find-links', str(kit / 'wheels'), '-r', str(kit / 'requirements.txt')], check=True)
recovered = root / 'recovered'
r = subprocess.check_output([py, str(kit / 'adopt.py'), 'recover', str(root / 'export'), sys.argv[1], str(recovered)])
assert json.loads(r)['owner_authority'] == 'not_checked'
for capture in (kit / 'fixture', recovered):
    a = subprocess.check_output([str(kit / 'bin/bean-evidence'), 'verify', str(capture)])
    b = subprocess.check_output([py, str(kit / 'verify.py'), 'verify', str(capture)])
    assert json.loads(a) == json.loads(b)
    assert json.loads(a)['mean_square_iq'] == 3317.25
# Prove that the original repository is not readable in this process.
try:
    pathlib.Path(sys.argv[2], 'go.mod').read_bytes()
except (PermissionError, FileNotFoundError):
    pass
else:
    raise AssertionError('repository remains readable')
print('offline kit: recovered identical evidence, two verifiers agree, checkout inaccessible')
''')
            command = [sys.executable, str(runner), record_id, str(REPO)]
            if sys.platform == 'darwin':
                policy = '(version 1)(allow default)(deny network*)(deny file-read* (subpath ' + json.dumps(str(REPO)) + '))'
                command = ['/usr/bin/sandbox-exec', '-p', policy, *command]
            elif sys.platform == 'linux' and os.geteuid() == 0:
                empty = root / 'empty'; empty.mkdir()
                command = ['unshare', '--mount', '--net', '--', 'sh', '-c',
                           'set -e; mount --make-rprivate /; mount --bind "$1" "$2"; shift 2; exec "$@"',
                           'kit-isolation', str(empty), str(REPO), *command]
            else:
                self.skipTest('checkout isolation requires macOS sandbox-exec or root Linux mount namespace')
            env = dict(os.environ)
            for key in ('PYTHONPATH', 'PYTHONHOME', 'BEAN_DIR', 'BEAN_BIN', 'EVIDENCE_BIN'):
                env.pop(key, None)
            result = subprocess.run(command, cwd=root, env=env, text=True, capture_output=True, timeout=90)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            for name, data in original.items():
                self.assertEqual((root / 'recovered' / name).read_bytes(), data)


if __name__ == '__main__':
    unittest.main()
