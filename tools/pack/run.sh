#!/usr/bin/env bash
# run.sh: builds a reference pack (docs/REFERENCE.md) from the synthetic
# NHANES files and holds bean and the Python tools, which share no code
# with it, to one verdict:
#   - check.py reads the pack by the rules of the document;
#   - rebuild_nhanes.py builds every table again from the files;
#   - reproduce.py computes every comparison that bean saved with the pack;
#   - a pack with one thing wrong is refused by bean and by check.py;
#   - a claim that was changed is refused by reproduce.py.
#
# With BEAN_NHANES_DIR set to a folder of CDC's own files, it also builds
# the pack of the files on record and checks its SHA-256.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
PY=${PY:-python3}
DATA=$ROOT/internal/nhanes/testdata
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
(cd "$ROOT" && go build -o "$T/bin/" ./cmd/bean ./cmd/bean-pack-nhanes) || { echo "build failed"; exit 1; }
export PATH="$T/bin:$PATH" BEAN_NOW=2026-11-09T12:00:00Z
unset BEAN_DIR
rc=0
say() { echo "FAIL: $*"; rc=1; }

SYN=("$DATA/DEMO_J.XPT" "$DATA/TCHOL_J.XPT" "$DATA/HDL_J.XPT")
bean-pack-nhanes -unknown-files -out "$T/syn.pack" "${SYN[@]}" >"$T/build.txt" || { echo "the pack was not built"; exit 1; }
DIGEST=$(sed -n 's/^sha256 //p' "$T/build.txt")
# The same files, in another order, give the same bytes.
bean-pack-nhanes -unknown-files -out "$T/again.pack" "${SYN[2]}" "${SYN[0]}" "${SYN[1]}" >/dev/null
cmp -s "$T/syn.pack" "$T/again.pack" || say "two builds from the same files differ"
# A pack is written to a new file, and never from a file not on record
# without being told.
bean-pack-nhanes -unknown-files -out "$T/syn.pack" "${SYN[@]}" >/dev/null 2>&1 && say "a pack was written over a file"
bean-pack-nhanes -out "$T/strict.pack" "${SYN[@]}" >/dev/null 2>&1 && say "a pack was built from files not on record, without -unknown-files"

"$PY" -I "$HERE/check.py" "$T/syn.pack" >"$T/py.txt" 2>&1 || say "check.py refuses the pack: $(cat "$T/py.txt")"
grep -qx "sha256 $DIGEST" "$T/py.txt" || say "check.py gives another SHA-256 than the builder"
"$PY" -I "$HERE/rebuild_nhanes.py" "$T/syn.pack" "${SYN[@]}" >"$T/re.txt" 2>&1 || say "rebuild_nhanes.py: $(tail -1 "$T/re.txt")"

# Comparisons that bean saves with the pack, computed again in Python.
b() { bean -dir "$T/log" "$@"; }
mkdir "$T/log"; b init >/dev/null 2>&1 || { echo "init failed"; exit 1; }
for v in 127 150 187 193 200.5 250 400; do b add lab.cholesterol-total $v -unit mg/dL -at 2026-11-01 >/dev/null; done
for v in 29 44 52 60 72; do b add lab.hdl-cholesterol $v -unit mg/dL -at 2026-11-01 >/dev/null; done
b agent init -label assistant >/dev/null 2>&1
b add -agent lab.cholesterol-total 181 -unit mg/dL -at 2026-11-01 >/dev/null || { echo "agent add failed"; exit 1; }
saved=0
for cohort in $(b compare -pack "$T/syn.pack" | sed -n 's/^  \([a-z]*-[0-9]*-[a-z0-9]*\): .*/\1/p'); do
  b compare -pack "$T/syn.pack" -cohort "$cohort" -type 'lab.*' -save >"$T/cmp.txt" 2>&1 && saved=$((saved + $(grep -c '^saved ' "$T/cmp.txt")))
done
[ "$saved" -ge 50 ] || say "bean saved only $saved comparisons with the pack"
b ls -json >"$T/records.jsonl"
"$PY" -I "$ROOT/tools/compare/reproduce.py" "$T/records.jsonl" "$T/syn.pack" >"$T/rp.txt" 2>&1 || say "reproduce.py does not get what bean saved: $(tail -2 "$T/rp.txt")"
# Without the pack the claims cannot be checked, and that is said.
"$PY" -I "$ROOT/tools/compare/reproduce.py" "$T/records.jsonl" >"$T/rp0.txt" 2>&1 && say "reproduce.py passes claims without their pack"
grep -q "CANNOT CHECK" "$T/rp0.txt" || say "reproduce.py does not say that it cannot check without the pack"
# A claim that was changed: a result, the cohort's text, a caveat.
for change in 's/"percentile":\([0-9.]*\)/"percentile":99.9/' 's/"text":"male, age 30-39"/"text":"male, age 20-29"/' 's/you chose the cohort: [^"]*"/"/'; do
  sed "$change" "$T/records.jsonl" >"$T/changed.jsonl"
  cmp -s "$T/records.jsonl" "$T/changed.jsonl" && { say "the change $change changes nothing"; continue; }
  "$PY" -I "$ROOT/tools/compare/reproduce.py" "$T/changed.jsonl" "$T/syn.pack" >/dev/null 2>&1 && say "reproduce.py passes a changed claim: $change"
done

# Packs with one thing wrong. bean and check.py must both refuse each.
"$PY" -I - "$T/syn.pack" "$T/wrong" <<'EOF' || { echo "the wrong packs were not made"; exit 1; }
import copy, json, os, sys
src, out = sys.argv[1:3]
os.mkdir(out)
good = json.load(open(src))
def cell(p): return p["cells"][0]
def first(d): return d[sorted(d)[0]]
cases = {
    "format": lambda p: p.update(bean="pack/1"),
    "member-more": lambda p: p.update(signature="x"),
    "member-less": lambda p: p.pop("caveats"),
    "title-empty": lambda p: p.update(title=""),
    "title-newline": lambda p: p.update(title="a\nb"),
    "title-space": lambda p: p.update(title="a "),
    "files-none": lambda p: p["source"].update(files=[]),
    "digest-upper": lambda p: p["source"]["files"][0].update(sha256="A" * 64),
    "bytes-zero": lambda p: p["source"]["files"][0].update(bytes=0),
    "file-twice": lambda p: p["source"]["files"].append(p["source"]["files"][0]),
    "same-empty": lambda p: first(p["measures"]).update(same=[]),
    "n-huge": lambda p: cell(p).update(n=4000000000),
    "caveat-number": lambda p: p.update(caveats=[1]),
    "kind": lambda p: cell(p).update(kind="quantiles"),
    "cell-member": lambda p: cell(p).update(note="x"),
    "values-order": lambda p: cell(p).update(values=cell(p)["values"][::-1]),
    "value-twice": lambda p: cell(p)["values"].__setitem__(1, cell(p)["values"][0]),
    "value-zero": lambda p: cell(p)["values"].__setitem__(0, 0),
    "weight-negative": lambda p: cell(p)["weights"].__setitem__(0, -1.5),
    "weight-text": lambda p: cell(p)["weights"].__setitem__(0, "1"),
    "weights-overflow": lambda p: cell(p).update(weights=[1e308] * len(cell(p)["weights"])),
    "weight-missing": lambda p: cell(p)["weights"].pop(),
    "values-none": lambda p: cell(p).update(values=[], weights=[]),
    "n-small": lambda p: cell(p).update(n=29),
    "n-fraction": lambda p: cell(p).update(n=cell(p)["n"] + 0.5),
    "n-below-values": lambda p: cell(p).update(n=len(cell(p)["values"]) - 1, values=cell(p)["values"]),
    "measure-unknown": lambda p: cell(p).update(measure="NOPE"),
    "cohort-unknown": lambda p: cell(p).update(cohort="nobody"),
    "cell-twice": lambda p: p["cells"].append(copy.deepcopy(cell(p))),
    "cells-none": lambda p: p.update(cells=[]),
    "measures-none": lambda p: p.update(measures={}),
    "id-space": lambda p: p["cohorts"].update({"very old": first(p["cohorts"])}),
    "same-twice": lambda p: p["measures"][sorted(p["measures"])[1]]["same"].append(first(p["measures"])["same"][0]),
    "same-no-unit": lambda p: first(p["measures"])["same"][0].pop("unit"),
    "measure-member": lambda p: first(p["measures"]).update(code="2093-3"),
}
for name, change in cases.items():
    p = copy.deepcopy(good)
    change(p)
    with open(os.path.join(out, name + ".pack"), "w") as f:
        json.dump(p, f, separators=(",", ":"))
raw = open(src).read()
for name, text in {
    "not-json": "pack",
    "a-list": "[]",
    "member-twice": raw.replace('{"bean":"pack/0",', '{"bean":"pack/0","bean":"pack/0",', 1),
    "cut-short": raw[: len(raw) // 2],
    "nan": raw.replace('"n":', '"n":NaN,"x":', 1),
    "bom": "\ufeff" + raw,
    "whole-number-too-large": raw.replace('"bytes":', '"bytes":9007199254740993,"x":', 1),
    "number-too-large": raw.replace('"weights":[', '"weights":[1e999,', 1),
}.items():
    with open(os.path.join(out, name + ".pack"), "w") as f:
        f.write(text)
EOF
wrong=0
for f in "$T"/wrong/*.pack; do
  wrong=$((wrong + 1))
  bean -dir "$T/log" compare -pack "$f" >/dev/null 2>&1; g=$?
  "$PY" -I "$HERE/check.py" "$f" >"$T/pyw.txt" 2>&1; p=$?
  [ "$g" != 0 ] && [ "$p" = 1 ] || say "$(basename "$f"): bean exit $g, check.py exit $p (both must refuse)"
  grep -q "Traceback" "$T/pyw.txt" && say "$(basename "$f"): check.py fails with a traceback"
done
# The pack that was not changed still passes: the changes are what is refused.
bean -dir "$T/log" compare -pack "$T/syn.pack" >/dev/null 2>&1 || say "bean refuses the good pack"
# A number written another way is the same number, and the cells may be in
# any order: both read such a pack, under another SHA-256.
"$PY" -I - "$T/syn.pack" "$T/other.pack" <<'EOF2'
import json, re, sys
p = json.load(open(sys.argv[1]))
p["cells"].reverse()
text = re.sub(r'"n":(\d+)', r'"n":\1.0', json.dumps(p, indent=1), count=1)
open(sys.argv[2], "w").write(text)
EOF2
bean -dir "$T/log" compare -pack "$T/other.pack" >/dev/null 2>&1 || say "bean refuses a pack that is written another way"
"$PY" -I "$HERE/check.py" "$T/other.pack" >/dev/null 2>&1 || say "check.py refuses a pack that is written another way"

real=""
if [ -n "${BEAN_NHANES_DIR:-}" ]; then
  REAL=(); for f in DEMO_J TCHOL_J HDL_J GHB_J CBC_J BMX_J; do for e in XPT xpt; do [ -f "$BEAN_NHANES_DIR/$f.$e" ] && REAL+=("$BEAN_NHANES_DIR/$f.$e"); done; done
  bean-pack-nhanes -out "$T/real.pack" "${REAL[@]}" >"$T/real.txt" 2>&1 || say "the pack of CDC's files was not built: $(cat "$T/real.txt")"
  bean -dir "$T/log" compare -pack "$T/real.pack" 2>&1 | grep -q "the NHANES pack that bean has on record" || say "the pack of CDC's files is not the pack on record"
  "$PY" -I "$HERE/check.py" "$T/real.pack" >/dev/null 2>&1 || say "check.py refuses the pack of CDC's files"
  "$PY" -I "$HERE/rebuild_nhanes.py" "$T/real.pack" "${REAL[@]}" >"$T/re.txt" 2>&1 || say "rebuild_nhanes.py on CDC's files: $(tail -1 "$T/re.txt")"
  real=", and the pack of CDC's files is the one on record"
fi
[ "$rc" = 0 ] && echo "pack: check.py reads the pack that bean builds, rebuild_nhanes.py builds every table again from the files, reproduce.py computes $saved saved comparisons again, and bean and check.py both refuse $wrong packs with one thing wrong$real"
exit $rc
