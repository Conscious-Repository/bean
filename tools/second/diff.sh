#!/usr/bin/env bash
# diff.sh <logs dir> <bean binary>: runs bean verify (on a copy) and the
# second verifier on each log in the dir and diffs lines and exit codes.
# "hidden pending gc" warnings come from local/hidden, which the second
# verifier does not read, so they are filtered out.
set -u
LOGS=${1:?usage: diff.sh <logs dir> <bean binary>}; BEAN=${2:?usage}
PY=${PY:-python3}
HERE=$(cd "$(dirname "$0")" && pwd)
export BEAN_NOW=2026-10-30T12:00:00Z
agree=0; differ=0
for d in "$LOGS"/*/; do
  n=$(basename "$d"); [ "$n" = bin ] || [ "$n" = go ] && continue
  [ -d "$d/objects" ] || continue
  t=$(mktemp -d); cp -r "$d" "$t/x"
  "$PY" "$HERE/beanverify.py" "$d" >"$t/py.txt"; pc=$?
  "$BEAN" -dir "$t/x" verify 2>&1 | grep -v ': hidden pending gc: ' >"$t/go.txt"; gc=${PIPESTATUS[0]}
  if diff -q "$t/go.txt" "$t/py.txt" >/dev/null && [ "$pc" = "$gc" ]; then
    agree=$((agree + 1)); echo "agree  $n (exit $gc)"
  else
    differ=$((differ + 1)); echo "DIFFER $n (bean exit $gc, second exit $pc)"; diff "$t/go.txt" "$t/py.txt" | sed 's/^/    /' | head -20
  fi
  rm -rf "$t"
done
echo "agree $agree, differ $differ"
[ "$differ" = 0 ]
