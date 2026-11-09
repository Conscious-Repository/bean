#!/usr/bin/env bash
# run.sh: saves comparisons with bean on the synthetic NHANES files, then
# checks that an independent Python implementation (reproduce.py, with
# pandas and numpy) accepts every record and computes every number of
# every comparison again: from the records and the files, and from a bean
# export folder alone. It then checks that reproduce.py notices a changed
# result, a changed reading and a changed file.
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
repro() { "$PY" -I "$HERE/reproduce.py" "$@"; }
# all <what> <paths>: every comparison must be reproduced.
all() {
  what=$1; shift
  repro "$@" >"$T/out.txt"; code=$?
  got=$(grep -c '^reproduced ' "$T/out.txt")
  if [ "$code" != 0 ] || [ "$got" != "$n" ]; then
    echo "DIFFER $what: reproduce.py exit $code, reproduced $got of $n"; grep -v '^reproduced ' "$T/out.txt" | head -20; exit 1
  fi
}
all "records and files" "$T/records.jsonl" "$DATA"/*.XPT
# Leaving: an export folder is all that a person without bean needs.
bean export "$T/export" >/dev/null 2>&1 || { echo "export failed"; exit 1; }
all "an export folder" "$T/export"
# none <what> <paths>: nothing may be reproduced, and the exit code is 1.
none() {
  what=$1; shift
  repro "$@" >"$T/out.txt"; code=$?
  if [ "$code" != 1 ] || grep -q '^reproduced ' "$T/out.txt"; then
    echo "DIFFER: reproduce.py accepted $what (exit $code)"; head -5 "$T/out.txt"; exit 1
  fi
}
# change <type> <python statement on r>: rewrites the first record of that
# type, as stored bytes with the old id and signature, and keeps only it and
# the records it needs.
change() {
  "$PY" -I - "$T/records.jsonl" "$T/changed.jsonl" "$1" "$2" <<'PYEOF'
import json, sys
src, dst, typ, stmt = sys.argv[1:5]
records = [json.loads(l) for l in open(src)]
claim = next(r for r in records if r["type"] == "comparison.percentile")
reading = next(r for r in records if r["id"] == claim["context"]["reading"])
r = claim if typ == "claim" else reading
exec(stmt)
with open(dst, "w") as out:
    for x in (claim, reading):
        out.write(json.dumps(x, sort_keys=True, separators=(",", ":"), ensure_ascii=False) + "\n")
PYEOF
}
change claim 'pass'
repro "$T/changed.jsonl" "$DATA"/*.XPT >"$T/out.txt" || { echo "DIFFER: the unchanged pair of records was refused"; cat "$T/out.txt"; exit 1; }
# A changed number, in the result or in the reading, breaks the record's id.
change claim 'r["context"]["result"]["p50"] += 1'
none "a changed result" "$T/changed.jsonl" "$DATA"/*.XPT
change reading 'r["value"]["n"] += 1'
none "a changed reading" "$T/changed.jsonl" "$DATA"/*.XPT
# A changed file has another SHA-256 than the claim names.
"$PY" -I -c 'import sys; d = bytearray(open(sys.argv[1], "rb").read()); d[-200] ^= 1; open(sys.argv[2], "wb").write(d)' "$DATA/DEMO_J.XPT" "$T/DEMO_J.XPT"
none "a file with another SHA-256" "$T/records.jsonl" "$DATA/TCHOL_J.XPT" "$DATA/HDL_J.XPT" "$DATA/BMX_J.XPT" "$DATA/DR1TOT_J.XPT" "$T/DEMO_J.XPT"
echo "compare: reproduce.py checks and computes all $n comparisons again from $(basename "$DATA"), also from an export alone, and refuses a changed result, reading and file"
