#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT
go build -o "$T/evidence" ./cmd/bean-evidence
go build -o "$T/bean" ./cmd/bean
PYTHON=$(command -v "${PY:-python3}")
if [ "${EVIDENCE_OFFLINE:-0}" = 1 ]; then
  # Linux CI: a new network namespace, no external interfaces or loopback route.
  sudo unshare --net -- env EVIDENCE_BIN="$T/evidence" BEAN_BIN="$T/bean" EVIDENCE_OFFLINE=1 \
    "$PYTHON" tools/evidence/test_evidence.py -v
else
  EVIDENCE_BIN="$T/evidence" BEAN_BIN="$T/bean" "$PYTHON" tools/evidence/test_evidence.py -v
fi
