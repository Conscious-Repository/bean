#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT
go build -o "$T/bean" ./cmd/bean
go build -o "$T/importer" ./cmd/bean-import-labcorp
BEAN_BIN="$T/bean" LABCORP_BIN="$T/importer" "${PY:-python3}" tools/labcorp/test_ingest.py -v
