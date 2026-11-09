#!/usr/bin/env bash
# run.sh: saves comparisons with bean on the synthetic NHANES files, then
# checks that an independent Python implementation (reproduce.py, with
# pandas) computes every number of every one again. It then changes one
# number of a claim and checks that reproduce.py notices.
#
# With BEAN_NHANES_DIR set to a folder of CDC's own files, it runs the same
# check on them.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
PY=${PY:-python3}
DATA=${BEAN_NHANES_DIR:-$ROOT/internal/nhanes/testdata}
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
(cd "$ROOT" && go build -o "$T/bin/" ./cmd/bean ./cmd/bean-import-nhanes) || { echo "build failed"; exit 1; }
export PATH="$T/bin:$PATH"
bean() { "$T/bin/bean" -dir "$T/log" "$@"; }
bean init >/dev/null 2>&1 || { echo "init failed"; exit 1; }
for f in TCHOL_J HDL_J DEMO_J BMX_J DR1TOT_J; do
  bean import -yes nhanes "$DATA/$f.XPT" >/dev/null 2>&1 || { echo "import $f failed"; exit 1; }
done
for v in 127 150 187 193 200.5 250 400; do bean add lab.cholesterol-total $v -unit mg/dL -at 2026-11-01 >/dev/null; done
for v in 29 44 52 60 72; do bean add lab.hdl-cholesterol $v -unit mg/dL -at 2026-11-01 >/dev/null; done
n=0
compare() { bean compare -save -type 'lab.*' "$@" >/dev/null 2>"$T/err" || { echo "compare $* failed: $(cat "$T/err")"; exit 1; }; n=$((n + 12)); }
compare -sex male -age 30-39
compare -sex female -age 6-80
compare -sex female -age 20-59 -where DEMO_J.RIDRETH3=3-4
compare -sex male -age 20-80 -where BMX_J.BMXBMI=22-30
compare -sex female -age 18-80 -where DR1TOT_J.DR1TPROT=60-140 -where DR1TOT_J.DR1TFIBE=10-40
compare -sex male -age 18-80 -where DEMO_J.RIDRETH3=3 -where BMX_J.BMXBMI=18-35 -where DR1TOT_J.DR1TKCAL=1500-3000
bean ls -json >"$T/records.jsonl"
"$PY" -I "$HERE/reproduce.py" "$T/records.jsonl" "$DATA"/*.XPT >"$T/out.txt"; code=$?
got=$(grep -c '^reproduced ' "$T/out.txt")
if [ "$code" != 0 ] || [ "$got" != "$n" ]; then
  echo "DIFFER: reproduce.py exit $code, reproduced $got of $n"; grep -v '^reproduced ' "$T/out.txt" | head -20; exit 1
fi
# One changed number in one claim must be noticed.
"$PY" -I - "$T/records.jsonl" "$T/changed.jsonl" <<'PYEOF'
import json, sys
done = False
with open(sys.argv[1]) as f, open(sys.argv[2], "w") as out:
    for line in f:
        r = json.loads(line)
        if r["type"] == "comparison.percentile" and not done:
            r["context"]["result"]["p50"] += 1
            done = True
        out.write(json.dumps(r) + "\n")
PYEOF
"$PY" -I "$HERE/reproduce.py" "$T/changed.jsonl" "$DATA"/*.XPT >"$T/changed.txt"; code=$?
if [ "$code" != 1 ] || [ "$(grep -c '^DIFFERS ' "$T/changed.txt")" != 1 ]; then
  echo "DIFFER: reproduce.py did not notice a changed result (exit $code)"; exit 1
fi
# A file that is not the one the claim names must be refused.
"$PY" -I -c 'import sys; d = bytearray(open(sys.argv[1], "rb").read()); d[-200] ^= 1; open(sys.argv[2], "wb").write(d)' "$DATA/DEMO_J.XPT" "$T/DEMO_J.XPT"
"$PY" -I "$HERE/reproduce.py" "$T/records.jsonl" "$DATA/TCHOL_J.XPT" "$DATA/HDL_J.XPT" "$DATA/BMX_J.XPT" "$DATA/DR1TOT_J.XPT" "$T/DEMO_J.XPT" >"$T/wrong.txt"; code=$?
if [ "$code" != 1 ] || grep -q '^reproduced ' "$T/wrong.txt"; then
  echo "DIFFER: reproduce.py accepted a file with another SHA-256 (exit $code)"; exit 1
fi
echo "compare: reproduce.py computes all $n comparisons again from $(basename "$DATA"), and notices a changed result and a changed file"
