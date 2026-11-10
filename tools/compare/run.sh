#!/usr/bin/env bash
# run.sh: saves comparisons with bean on the synthetic NHANES files, then
# checks that an independent Python implementation (reproduce.py, with
# pandas) accepts every record and computes every number of
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
# The synthetic files are not the files on record: bean uses them only when
# told to. CDC's own files need no flag.
UNKNOWN=; [ -n "${BEAN_NHANES_DIR:-}" ] || UNKNOWN=-unknown-files
# CDC serves .xpt; the synthetic files are .XPT.
xpt() { for e in XPT xpt; do [ -f "$DATA/$1.$e" ] && { echo "$DATA/$1.$e"; return; }; done; echo "$DATA/$1.XPT"; }
FILES=(); for f in TCHOL_J HDL_J DEMO_J BMX_J DR1TOT_J; do FILES+=("$(xpt $f)"); done
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
(cd "$ROOT" && go build -o "$T/bin/" ./cmd/bean ./cmd/bean-import-nhanes) || { echo "build failed"; exit 1; }
export PATH="$T/bin:$PATH"
bean() { "$T/bin/bean" -dir "$T/log" "$@"; }
bean init >/dev/null 2>&1 || { echo "init failed"; exit 1; }
for f in "${FILES[@]}"; do
  bean import -yes nhanes "$f" >/dev/null 2>&1 || { echo "import $f failed"; exit 1; }
done
for v in 127 150 187 193 200.5 250 400; do bean add lab.cholesterol-total $v -unit mg/dL -at 2026-11-01 >/dev/null; done
for v in 29 44 52 60 72; do bean add lab.hdl-cholesterol $v -unit mg/dL -at 2026-11-01 >/dev/null; done
# A reading that an agent signed: its comparison carries that as a caveat.
bean agent init -label assistant >/dev/null 2>&1
bean add -agent lab.cholesterol-total 181 -unit mg/dL -at 2026-11-01 >/dev/null || { echo "agent add failed"; exit 1; }
compare() { bean compare $UNKNOWN -save -type 'lab.*' "$@" >/dev/null 2>"$T/err" || { echo "compare $* failed: $(cat "$T/err")"; exit 1; }; }
compare -sex male -age 30-39
compare -sex female -age 6-80
compare -sex female -age 20-59 -where DEMO_J.RIDRETH3=3-4
compare -sex male -age 20-80 -where BMX_J.BMXBMI=22-30
compare -sex female -age 18-80 -where DR1TOT_J.DR1TPROT=60-140 -where DR1TOT_J.DR1TFIBE=10-40
compare -sex male -age 18-80 -where DEMO_J.RIDRETH3=3 -where BMX_J.BMXBMI=18-35 -where DR1TOT_J.DR1TKCAL=1500-3000
# A rule with a large number and one with decimals: the cohort's text must be
# written the same way by both.
compare -sex female -age 18-80 -where DR1TOT_J.DR1TKCAL=0-1000000 -where BMX_J.BMXBMI=18.5-40
bean ls -all -json >"$T/records.jsonl"
n=$(grep -c '"type":"comparison.percentile"' "$T/records.jsonl")
[ "$n" -ge 70 ] || { echo "only $n comparisons were saved"; exit 1; }
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
all "records and files" "$T/records.jsonl" "${FILES[@]}"
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
repro "$T/changed.jsonl" "${FILES[@]}" >"$T/out.txt" || { echo "DIFFER: the unchanged pair of records was refused"; cat "$T/out.txt"; exit 1; }
# A changed number, in the result or in the reading, breaks the record's id.
change claim 'r["context"]["result"]["p50"] += 1'
none "a changed result" "$T/changed.jsonl" "${FILES[@]}"
change reading 'r["value"]["n"] += 1'
none "a changed reading" "$T/changed.jsonl" "${FILES[@]}"
# A changed file has another SHA-256 than the claim names.
"$PY" -I -c 'import sys; d = bytearray(open(sys.argv[1], "rb").read()); d[-200] ^= 1; open(sys.argv[2], "wb").write(d)' "$(xpt DEMO_J)" "$T/DEMO_J.XPT" || { echo "could not change a file"; exit 1; }
# The changed file must still read as a file: the refusal must come from its
# SHA-256, not from a file that cannot be opened.
"$PY" -I -c 'import sys, pandas; pandas.read_sas(sys.argv[1], format="xport")' "$T/DEMO_J.XPT" || { echo "the changed file does not read"; exit 1; }
none "a file with another SHA-256" "$T/records.jsonl" "$(xpt TCHOL_J)" "$(xpt HDL_J)" "$(xpt BMX_J)" "$(xpt DR1TOT_J)" "$T/DEMO_J.XPT"
# A file that was not given is a check that cannot be made, not a result
# that differs.
repro "$T/records.jsonl" "$(xpt TCHOL_J)" "$(xpt DEMO_J)" >"$T/out.txt"; code=$?
if [ "$code" != 1 ] || grep -q '^DIFFERS ' "$T/out.txt" || ! grep -q '^CANNOT CHECK ' "$T/out.txt"; then
  echo "DIFFER: a missing file is not reported as a check that cannot be made"; head -3 "$T/out.txt"; exit 1
fi
# A claim that is signed and whose numbers compute, but that is not one the
# method makes, must be refused: each of these is written with bean append,
# so its id and signature are good.
forge() {
  "$PY" -I - "$T/records.jsonl" "$1" >"$T/forged.jsonl" <<'PYEOF'
import json, sys
records = [json.loads(l) for l in open(sys.argv[1])]
r = next(x for x in records if x["type"] == "comparison.percentile" and len(x["context"]["cohort"]["rules"]) == 2)
c = r["context"]
exec(sys.argv[2])
src = dict(r["source"]); src.pop("ref", None); src["ref"] = None
print(json.dumps({"type": r["type"], "value": r["value"], "times": {"observed": r["times"]["observed"]},
                  "refs": r["refs"], "source": src, "context": c}))
PYEOF
  rm -rf "$T/flog"; cp -R "$T/log" "$T/flog"
  "$T/bin/bean" -dir "$T/flog" append <"$T/forged.jsonl" >/dev/null 2>&1 || { echo "forge: append failed"; exit 1; }
  "$T/bin/bean" -dir "$T/flog" ls -all -json >"$T/forged-log.jsonl"
  before=$(grep -c . "$T/records.jsonl"); after=$(grep -c . "$T/forged-log.jsonl")
  [ "$after" = $((before + 1)) ] || { echo "forge: the forged claim was not added"; exit 1; }
  repro "$T/forged-log.jsonl" "${FILES[@]}" >"$T/out.txt"; code=$?
  if [ "$code" != 1 ] || [ "$(grep -c '^DIFFERS ' "$T/out.txt")" != 1 ] || [ "$(grep -c '^reproduced ' "$T/out.txt")" != "$n" ]; then
    echo "DIFFER: reproduce.py accepted a claim with $2 (exit $code)"; grep -v '^reproduced ' "$T/out.txt" | head -5; exit 1
  fi
}
# The first is the plain case: the form is right and one number is not. It
# must be caught by computing, not by any other check.
forge 'c["result"]["p50"] += 1' "a median that is not the one the files give"
grep -q '^DIFFERS .*this tool computes' "$T/out.txt" || { echo "DIFFER: a wrong number was not caught by computing it"; grep '^DIFFERS' "$T/out.txt"; exit 1; }
forge 'c["cohort"]["text"] = "female, age 20-29"' "a cohort text that its rules do not say"
forge 'c["reference"]["weight"] = {"dataset": "DEMO_J", "variable": "RIDAGEYR"}' "a weight the method does not give"
forge 'c["caveats"] = c["caveats"][:1]' "caveats left out"
forge 'c["cohort"]["rules"] = c["cohort"]["rules"][1:]' "a cohort with no sex"
forge 'c["reference"]["files"] = c["reference"]["files"][:1]' "a file left out of the list"
echo "compare: reproduce.py checks and computes all $n comparisons again from $(basename "$DATA"), also from an export alone, and refuses a changed result, reading and file, and six signed claims that the method does not give"
