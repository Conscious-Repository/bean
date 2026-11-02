#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
if [ "$#" -gt 0 ] && { [ "$#" -ne 2 ] || [ "$1" != --kit ] || [ -z "$2" ]; }; then
  echo 'usage: run.sh [--kit /new/output/directory]' >&2
  exit 1
fi
T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT
go build -o "$T/evidence" ./cmd/bean-evidence
PYTHON=$(command -v "${PY:-python3}")
WHEELS=${EVIDENCE_WHEELS:-$T/wheels}
if [ -z "${EVIDENCE_WHEELS:-}" ]; then
  # Preparation only. All verification runs below require no package index.
  "$PYTHON" -m pip download --only-binary=:all: -r tools/requirements.txt -d "$WHEELS"
fi
WHEELS=$(cd "$WHEELS" && pwd)
if [ "$#" -eq 2 ]; then
  "$PYTHON" tools/evidence/kit.py --binary "$T/evidence" --wheels "$WHEELS" --output "$2"
  exit
fi
go build -o "$T/bean" ./cmd/bean
if [ "${EVIDENCE_OFFLINE:-0}" = 1 ]; then
  # Linux CI: no external interfaces or loopback route. The kit test also hides
  # the checkout in a child mount namespace and installs from copied wheels.
  sudo unshare --net -- env EVIDENCE_BIN="$T/evidence" BEAN_BIN="$T/bean" EVIDENCE_WHEELS="$WHEELS" EVIDENCE_OFFLINE=1 \
    "$PYTHON" -m unittest discover -s tools/evidence -p 'test_*.py' -v
else
  EVIDENCE_BIN="$T/evidence" BEAN_BIN="$T/bean" EVIDENCE_WHEELS="$WHEELS" \
    "$PYTHON" -m unittest discover -s tools/evidence -p 'test_*.py' -v
fi
