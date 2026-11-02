#!/usr/bin/env python3
"""Build a platform-specific offline verification kit; never include personal data."""
import argparse
import hashlib
import json
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import tempfile

HERE = Path(__file__).resolve().parent
REPO = HERE.parent.parent


def build(binary, wheels, output):
    binary, wheels, output = Path(binary), Path(wheels), Path(output)
    wheel_files = sorted(wheels.glob('*.whl'))
    if not binary.is_file() or not any(p.name.startswith('cryptography-50.0.2-') for p in wheel_files):
        raise ValueError('provide the built verifier and pinned cryptography wheelhouse')
    if any(p.is_symlink() or not p.is_file() for p in wheel_files):
        raise ValueError('unsafe wheelhouse')
    output.mkdir(mode=0o700)  # Refuse existing destinations, including symlinks.
    for sub in ('bin', 'source', 'fixture', 'wheels'):
        (output / sub).mkdir()
    shutil.copy2(binary, output / 'bin' / 'bean-evidence')
    for name in ('verify.py', 'adopt.py'):
        shutil.copy2(HERE / name, output / name)
    for name in ('raw.bin', 'settings.json', 'receipt.jws'):
        shutil.copy2(HERE / 'fixture' / name, output / 'fixture' / name)
    for source, target in ((REPO / 'cmd/bean-evidence/main.go', 'source/main.go'),
                           (REPO / 'docs/INSTRUMENT-EVIDENCE-ADR.md', 'METHOD-AND-PROFILE.md'),
                           (REPO / 'tools/requirements.txt', 'requirements.txt'),
                           (REPO / 'LICENSE', 'LICENSE'), (REPO / 'LICENSE-SPEC', 'LICENSE-SPEC')):
        shutil.copy2(source, output / target)
    for path in wheel_files:
        shutil.copy2(path, output / 'wheels' / path.name)
    (output / 'README.txt').write_text('''Offline evidence verification kit (experimental, host-software only)

This kit contains a host-platform Go verifier, standalone Go source, independent
Python verifier/recovery adapter, exact synthetic vector, calculation/profile
specification, licences and a platform-specific Python wheelhouse. It contains
no personal capture, Bean identity, signing key or account token. The fixture key
is public. Matching signatures never establish source trust or clinical accuracy.

Requires the recorded OS/architecture and compatible Python already installed.
The Go binary needs neither Python nor Go installed. For another platform, build
another kit there; this is not a universal binary distribution or a Python runtime.

1. ./bin/bean-evidence verify fixture
2. python3 -m venv runtime
3. runtime/bin/python -m pip install --no-index --find-links wheels -r requirements.txt
4. runtime/bin/python verify.py verify fixture
Both outputs must show mean_square_iq=3317.25, evidence_complete=true and
signature_valid=true, with source_trusted=false and accuracy_validated=false.
Run both verifiers on your recovered capture too.

To recover from an ordinary PLAINTEXT Bean export without the original log/keys:
  runtime/bin/python adopt.py recover /path/to/export sha256:RECORD_ID /new/capture
Recovery authenticates capture evidence only; owner_authority is not_checked.
The capture receipt does not establish which person was measured.
Protect the plaintext export. Keep this kit alongside it, not in each receipt.

To rebuild the standalone Go verifier with a compatible Go toolchain:
  GOWORK=off go build -o bin/bean-evidence source/main.go

inventory.json records build platform, Python version, source revision/dirty
state and exact file SHA-256 values. It detects accidental substitution relative
to a separately trusted copy; it is NOT a signed release endorsement. Archive the
kit and its trusted digest through your own controlled channel. No network access
is required after kit preparation. A physical second-machine test remains a
separate deployment acceptance step.
''')
    # Verify using only copied wheels in a fresh environment, not installed packages.
    with tempfile.TemporaryDirectory(prefix='bean-kit-check-') as temp:
        subprocess.run([sys.executable, '-m', 'venv', temp], check=True, capture_output=True)
        py = str(Path(temp) / 'bin/python')
        subprocess.run([py, '-m', 'pip', 'install', '--no-index', '--find-links',
                        str(output / 'wheels'), '-r', str(output / 'requirements.txt')],
                       check=True, capture_output=True)
        go_report = subprocess.check_output([str((output / 'bin/bean-evidence').resolve()),
                                            'verify', str(output / 'fixture')])
        py_report = subprocess.check_output([py, str(output / 'verify.py'), 'verify',
                                            str(output / 'fixture')])
        if json.loads(go_report) != json.loads(py_report) or json.loads(go_report).get('mean_square_iq') != 3317.25:
            raise ValueError('kit fixture verification disagrees')
    revision = subprocess.check_output(['git', '-C', str(REPO), 'rev-parse', 'HEAD'], text=True).strip()
    dirty = bool(subprocess.check_output(['git', '-C', str(REPO), 'status', '--porcelain']))
    hashes = {str(path.relative_to(output)): hashlib.sha256(path.read_bytes()).hexdigest()
              for path in sorted(output.rglob('*')) if path.is_file()}
    inventory = {'kind': 'bean-evidence-offline-kit-experiment', 'platform': platform.platform(),
                 'architecture': platform.machine(), 'python': sys.version,
                 'source_revision': revision, 'source_tree_dirty': dirty, 'sha256': hashes}
    (output / 'inventory.json').write_text(json.dumps(inventory, indent=2, sort_keys=True) + '\n')
    return output


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True)
    parser.add_argument('--wheels', required=True)
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    print(build(args.binary, args.wheels, args.output))
