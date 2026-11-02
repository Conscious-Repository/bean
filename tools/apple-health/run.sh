#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT
go build -o "$T/bean" ./cmd/bean
go build -o "$T/importer" ./cmd/bean-import-apple-health
BEAN_BIN="$T/bean" APPLE_HEALTH_BIN="$T/importer" "${PY:-python3}" tools/apple-health/test_ingest.py -v
