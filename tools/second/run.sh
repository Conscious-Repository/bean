#!/usr/bin/env bash
# run.sh: builds logs with bean (scenarios.sh) and with an independent
# Python writer (craft.py), then checks that bean verify and the second
# verifier (beanverify.py) print the same lines and exit codes on each.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
PY=${PY:-python3}; export PY
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
bash "$HERE/scenarios.sh" "$T/logs" >/dev/null || { echo "scenarios failed"; exit 1; }
"$PY" "$HERE/craft.py" "$T/craft" >/dev/null || { echo "craft failed"; exit 1; }
rc=0
bash "$HERE/diff.sh" "$T/logs" "$T/logs/bin/bean" || rc=1
bash "$HERE/diff.sh" "$T/craft" "$T/logs/bin/bean" || rc=1
exit $rc
