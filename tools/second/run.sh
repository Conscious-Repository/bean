#!/usr/bin/env bash
# run.sh: builds logs with bean (scenarios.sh) and with an independent
# Python writer (craft.py), then checks that bean verify and the second
# verifier (beanverify.py) print the same lines and exit codes on each.
# It then checks the second verifier against the expected results of the
# conformance folders (vectors/conformance, V24), and that cases.py still
# writes those folders byte for byte.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
PY=${PY:-python3}; export PY
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
bash "$HERE/scenarios.sh" "$T/logs" >/dev/null || { echo "scenarios failed"; exit 1; }
"$PY" "$HERE/craft.py" "$T/craft" >/dev/null || { echo "craft failed"; exit 1; }
rc=0
bash "$HERE/diff.sh" "$T/logs" "$T/logs/bin/bean" || rc=1
bash "$HERE/diff.sh" "$T/craft" "$T/logs/bin/bean" || rc=1
C="$ROOT/vectors/conformance"
"$PY" "$HERE/cases.py" "$T/cases" >/dev/null || { echo "cases failed"; exit 1; }
if diff -r -x expected.txt "$T/cases" "$C" >/dev/null; then echo "cases.py reproduces $C"; else echo "DIFFER cases.py output and $C"; rc=1; fi
pass=0; fail=0
for d in "$C"/*/; do
  n=$(basename "$d")
  { "$PY" "$HERE/beanverify.py" "$d"; echo "exit $?"; } >"$T/py.txt"
  if diff -q "$T/py.txt" "$d/expected.txt" >/dev/null; then
    pass=$((pass + 1))
  else
    fail=$((fail + 1)); echo "DIFFER conformance $n"; diff "$d/expected.txt" "$T/py.txt" | sed 's/^/    /' | head -20
  fi
done
echo "conformance: pass $pass, differ $fail"
[ "$fail" = 0 ] || rc=1
exit $rc
