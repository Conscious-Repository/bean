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

SYN=("$DATA/DEMO_J.XPT" "$DATA/TCHOL_J.XPT" "$DATA/HDL_J.XPT" "$DATA/BMX_J.XPT")
bean-pack-nhanes -unknown-files -out "$T/syn.pack" "${SYN[@]}" >"$T/build.txt" || { echo "the pack was not built"; exit 1; }
DIGEST=$(sed -n 's/^sha256 //p' "$T/build.txt")
# The same files, in another order, give the same bytes.
bean-pack-nhanes -unknown-files -out "$T/again.pack" "${SYN[3]}" "${SYN[2]}" "${SYN[0]}" "${SYN[1]}" "$DATA/DR1TOT_J.XPT" >/dev/null
cmp -s "$T/syn.pack" "$T/again.pack" || say "two builds from the same files differ"
# A pack is written to a new file, and never from a file not on record
# without being told.
bean-pack-nhanes -unknown-files -out "$T/syn.pack" "${SYN[@]}" >/dev/null 2>&1 && say "a pack was written over a file"
bean-pack-nhanes -out "$T/strict.pack" "${SYN[@]}" >/dev/null 2>&1 && say "a pack was built from files not on record, without -unknown-files"

"$PY" -I "$HERE/check.py" "$T/syn.pack" >"$T/py.txt" 2>&1 || say "check.py refuses the pack: $(cat "$T/py.txt")"
grep -qx "sha256 $DIGEST" "$T/py.txt" || say "check.py gives another SHA-256 than the builder"
"$PY" -I "$HERE/rebuild_nhanes.py" "$T/syn.pack" "${SYN[@]}" "$DATA/DR1TOT_J.XPT" >"$T/re.txt" 2>&1 || say "rebuild_nhanes.py: $(tail -1 "$T/re.txt")"
grep -q '"BMXWT"' "$T/syn.pack" || say "the pack of the synthetic files has no body measure"
# The same pack written another way is not what the builder writes: the
# rebuild compares bytes, and says so.
"$PY" -I -c 'import json, sys; json.dump(json.load(open(sys.argv[1])), open(sys.argv[2], "w"), indent=1)' "$T/syn.pack" "$T/spaced.pack"
"$PY" -I "$HERE/rebuild_nhanes.py" "$T/spaced.pack" "${SYN[@]}" >"$T/sp.txt" 2>&1 && say "rebuild_nhanes.py passes a pack that is not written as the builder writes it"
grep -q "not written as the builder writes it" "$T/sp.txt" || say "rebuild_nhanes.py does not say that a pack is not in canonical form: $(tail -1 "$T/sp.txt")"
# A file that the builder uses and the pack does not name: the pack was
# built from fewer files than are given.
bean-pack-nhanes -unknown-files -out "$T/fewer.pack" "${SYN[0]}" "${SYN[1]}" >/dev/null
"$PY" -I "$HERE/rebuild_nhanes.py" "$T/fewer.pack" "${SYN[@]}" >/dev/null 2>&1 && say "rebuild_nhanes.py passes a pack that was built from fewer files than it is given"
# The rebuild covers every member: a pack with true tables and one other
# thing changed is not the pack that the files give.
i=0
for change in 's/, not a reference range//' 's/"type":"lab.hdl-cholesterol"/"type":"lab.ldl-cholesterol"/' 's/"type":"apple-health.body-mass","unit":"kg"/"type":"apple-health.body-mass","unit":"lb"/' 's/adults by sex/people by sex/' 's/"version":"1"/"version":"2"/' 's/"text":"male, age 30-39"/"text":"male, age 30-40"/' 's/"n":\([0-9]*\)/"n":1\1/'; do
  i=$((i + 1))
  sed "$change" "$T/syn.pack" >"$T/lie$i.pack"
  cmp -s "$T/syn.pack" "$T/lie$i.pack" && { say "the change $change changes nothing"; continue; }
  "$PY" -I "$HERE/rebuild_nhanes.py" "$T/lie$i.pack" "${SYN[@]}" >"$T/lie.txt" 2>&1 && say "rebuild_nhanes.py passes a pack that the files do not give: $change"
  grep -q "^DIFFERS: " "$T/lie.txt" || say "rebuild_nhanes.py does not say what differs: $change: $(tail -1 "$T/lie.txt")"
done

# The four vectors of the method in docs/REFERENCE.md, asked of check.py.
"$PY" -I - "$T/syn.pack" "$T" <<'EOF3' || { echo "the vector packs were not made"; exit 1; }
import json, os, sys
p = json.load(open(sys.argv[1]))
cell = p["cells"][0]
p["cells"] = [cell]
for i, (values, weights) in enumerate([([1, 2, 3, 4], [1, 1, 1, 1]), ([10, 20, 30], [1, 1, 8]), ([10, 20], [0.3, 0.1])]):
    cell.update(n=30, values=values, weights=weights)
    json.dump(p, open(os.path.join(sys.argv[2], "vector%d.pack" % i), "w"))
open(os.path.join(sys.argv[2], "vector.cell"), "w").write(cell["measure"] + " " + cell["cohort"])
EOF3
read -r VM VC <"$T/vector.cell"
vector() {
  got=$("$PY" -I "$HERE/check.py" "$T/vector$1.pack" place "$VM" "$VC" "$2" 2>&1)
  [ "$got" = "$3" ] || say "vector $1 at $2: check.py gives '$got', the document gives '$3'"
}
vector 0 2 "n=30 percentile=37.5 below=25 equal=25 p25=1 p50=2 p75=3"
vector 0 2.5 "n=30 percentile=50 below=50 equal=0 p25=1 p50=2 p75=3"
vector 1 30 "n=30 percentile=60 below=20 equal=80 p25=30 p50=30 p75=30"
vector 2 15 "n=30 percentile=75 below=75 equal=0 p25=10 p50=10 p75=20"

# Comparisons that bean saves with the pack, computed again in Python.
b() { bean -dir "$T/log" "$@"; }
mkdir "$T/log"; b init >/dev/null 2>&1 || { echo "init failed"; exit 1; }
for v in 127 150 187 193 200.5 250 400; do b add lab.cholesterol-total $v -unit mg/dL -at 2026-11-01 >/dev/null; done
for v in 29 44 52 60 72; do b add lab.hdl-cholesterol $v -unit mg/dL -at 2026-11-01 >/dev/null; done
for v in 55.5 70 81.2 104; do b add weight $v -unit kg -at 2026-11-01 >/dev/null; done
# A reading of a type that the pack does not take, with the value, the unit
# and the time of one that it does: a forged claim about it computes.
b add lab.ldl-cholesterol 187 -unit mg/dL -at 2026-11-01 >/dev/null
b agent init -label assistant >/dev/null 2>&1
b add -agent lab.cholesterol-total 181 -unit mg/dL -at 2026-11-01 >/dev/null || { echo "agent add failed"; exit 1; }
saved=0
# The pack of the synthetic files is not one that bean has on record: bean
# uses it only when told to, and each claim then says so.
b compare -pack "$T/syn.pack" -cohort male-30-39 -type 'lab.*' >"$T/cmp.txt" 2>&1 && say "bean compares with a pack that it has no record of, without -unknown-pack"
grep -q "pass -unknown-pack" "$T/cmp.txt" || say "bean does not say how to use a pack that it has no record of"
for cohort in $(b compare -pack "$T/syn.pack" | sed -n 's/^  \([a-z]*-[0-9]*-[a-z0-9]*\): .*/\1/p'); do
  for type in 'lab.*' weight; do
    b compare -pack "$T/syn.pack" -unknown-pack -cohort "$cohort" -type "$type" -save >"$T/cmp.txt" 2>&1 && saved=$((saved + $(grep -c '^saved ' "$T/cmp.txt")))
  done
done
b ls -json -type comparison.percentile | grep -q '"measure":"BMXWT"' || say "bean saved no comparison of a weight"
[ "$saved" -ge 50 ] || say "bean saved only $saved comparisons with the pack"
b ls -json >"$T/records.jsonl"
"$PY" -I "$ROOT/tools/compare/reproduce.py" -accept-pack "$DIGEST" "$T/records.jsonl" "$T/syn.pack" >"$T/rp.txt" 2>&1 || say "reproduce.py does not get what bean saved: $(tail -2 "$T/rp.txt")"
# A reader who has not named the pack is told that it is not on record,
# and the run fails: a comparison computes again with any pack.
"$PY" -I "$ROOT/tools/compare/reproduce.py" "$T/records.jsonl" "$T/syn.pack" >"$T/rpu.txt" 2>&1 && say "reproduce.py passes comparisons with a pack that is not on record"
grep -q "^NOT on record: pack $DIGEST" "$T/rpu.txt" || say "reproduce.py does not say that the pack is not on record"
# The reader names another pack: that accepts nothing.
"$PY" -I "$ROOT/tools/compare/reproduce.py" -accept-pack "$(printf 'a%.0s' $(seq 64))" "$T/records.jsonl" "$T/syn.pack" >"$T/rpa.txt" 2>&1 && say "reproduce.py passes comparisons when the reader accepts another pack"
grep -q "^NOT on record: pack $DIGEST" "$T/rpa.txt" || say "reproduce.py takes any -accept-pack for the pack it was given"
# A shared bundle of such comparisons, and the pack under another name: a
# pack is what its SHA-256 says.
b share -yes -out "$T/bundle" -type 'comparison.*' >/dev/null 2>&1 || say "the comparisons were not shared"
bean check-bundle "$T/bundle" >"$T/cb.txt" 2>&1
grep -q "get the pack with that SHA-256" "$T/cb.txt" || say "check-bundle does not tell the reader that a pack is needed"
grep -q "(NOT on record)" "$T/cb.txt" || say "check-bundle does not mark a comparison with a pack that is not on record"
cp "$T/syn.pack" "$T/reference-data.bin"
"$PY" -I "$ROOT/tools/compare/reproduce.py" -accept-pack "$DIGEST" "$T/bundle" "$T/reference-data.bin" >"$T/rpb.txt" 2>&1 || say "reproduce.py does not compute a shared comparison again from the bundle and the pack: $(tail -2 "$T/rpb.txt")"
# Without the pack the claims cannot be checked, and that is said.
"$PY" -I "$ROOT/tools/compare/reproduce.py" "$T/records.jsonl" >"$T/rp0.txt" 2>&1 && say "reproduce.py passes claims without their pack"
grep -q "CANNOT CHECK" "$T/rp0.txt" || say "reproduce.py does not say that it cannot check without the pack"
# A claim that was changed: a result, the cohort's text, a caveat.
for change in 's/"percentile":\([0-9.]*\)/"percentile":99.9/' 's/"text":"male, age 30-39"/"text":"male, age 20-29"/' 's/you chose the cohort: [^"]*"/"/'; do
  sed "$change" "$T/records.jsonl" >"$T/changed.jsonl"
  cmp -s "$T/records.jsonl" "$T/changed.jsonl" && { say "the change $change changes nothing"; continue; }
  "$PY" -I "$ROOT/tools/compare/reproduce.py" -accept-pack "$DIGEST" "$T/changed.jsonl" "$T/syn.pack" >/dev/null 2>&1 && say "reproduce.py passes a changed claim: $change"
done

# A claim that is signed and whose numbers compute, but that is not one the
# method makes with this pack, must be refused. Each is written with bean
# append, so its id and its signature are good.
forged=0
forge() {
  forged=$((forged + 1))
  "$PY" -I - "$T/records.jsonl" "$2" >"$T/forged.jsonl" <<'PYEOF'
import json, sys
records = [json.loads(l) for l in open(sys.argv[1])]
person = lambda x: x["author"] == x["subject"]
tc = next(x for x in records if x["type"] == "lab.cholesterol-total" and x["value"]["n"] == 187 and person(x))
# The comparison in one named cohort: the log holds one for each cohort, in
# the order of their ids, and "another cohort" below must change it.
r = next(x for x in records if x["type"] == "comparison.percentile" and x["context"]["reading"] == tc["id"] and x["context"]["reference"]["cohort"] == "male-30-39")
other = next(x for x in records if x["type"] == "weight")
ldl = next(x for x in records if x["type"] == "lab.ldl-cholesterol")
c = r["context"]
exec(sys.argv[2])
print(json.dumps({"type": r["type"], "value": r["value"], "times": {"observed": r["times"]["observed"]},
                  "refs": r["refs"], "source": dict(r["source"], ref=None), "context": c}))
PYEOF
  mkdir "$T/flog$forged"; cp -R "$T/log/." "$T/flog$forged/"
  bean -dir "$T/flog$forged" append <"$T/forged.jsonl" >/dev/null 2>&1 || { say "forge $1: the claim was not appended"; return; }
  bean -dir "$T/flog$forged" ls -all -json >"$T/forged-log.jsonl"
  [ "$(grep -c . "$T/forged-log.jsonl")" = $(($(b ls -all -json | grep -c .) + 1)) ] || { say "forge $1: the forged claim was not added"; return; }
  "$PY" -I "$ROOT/tools/compare/reproduce.py" -accept-pack "$DIGEST" "$T/forged-log.jsonl" "$T/syn.pack" >"$T/fo.txt" 2>&1 && say "reproduce.py passes a forged claim: $1"
  [ "$(grep -c '^DIFFERS' "$T/fo.txt")" = 1 ] || say "forge $1: reproduce.py does not refuse exactly the forged claim: $(grep -c '^DIFFERS' "$T/fo.txt") refused"
}
forge "a second record it is about" 'r["refs"].append({"rel": "about", "id": other["id"]})'
forge "a reading of a type that the measure does not take" 'c["reading"] = other["id"]; r["refs"] = [{"rel": "about", "id": other["id"]}]'
forge "a reading of another type with the same value and unit" 'c["reading"] = ldl["id"]; r["refs"] = [{"rel": "about", "id": ldl["id"]}]'
forge "another title" 'c["reference"]["title"] = "NHANES 2017-2018 (reference range)"'
forge "another source" 'c["reference"]["source"] = "a clinical reference"'
forge "another label" 'c["reference"]["label"] = "LDL Cholesterol (mg/dL)"'
forge "another unit" 'c["reference"]["unit"] = "mmol/L"'
forge "another cohort" 'c["reference"]["cohort"] = "female-30-39"'
forge "a member more" 'c["note"] = "clinically validated"'
forge "a caveat more" 'c["caveats"].append("clinically validated")'
forge "the caveat of a pack not on record taken out" 'c["caveats"] = [x for x in c["caveats"] if not x.startswith("bean has no record")]'
forge "another n" 'c["result"]["n"] += 1'
forge "another quartile" 'c["result"]["p75"] = c["result"]["p50"]'
forge "another value" 'r["value"]["n"] = 99.9'

# A pack can be put on record after a claim was saved with it. The claim
# then still says that bean had no record of the pack, and it must still
# compute again: a signed record means the same for ever (DECISIONS 319).
"$PY" -I - "$ROOT" "$DIGEST" "$T/records.jsonl" "$T/syn.pack" >"$T/later.txt" 2>&1 <<'PYEOF' || say "reproduce.py refuses claims that were saved before their pack was put on record: $(tail -2 "$T/later.txt")"
import sys
sys.path.insert(0, sys.argv[1] + "/tools/compare")
import reproduce
reproduce.KNOWN_PACKS[sys.argv[2]] = "a pack that was put on record after the claims were saved"
sys.exit(reproduce.main(["reproduce.py"] + sys.argv[3:]))
PYEOF
grep -q "on record" "$T/later.txt" && ! grep -q "NOT on record" "$T/later.txt" || say "reproduce.py did not take the pack as one on record in the check of claims saved before"

# The comparisons that a release saved must compute again in every later
# build. cmd/bean/testdata/saved holds a bundle that each release wrote.
# Here: the one with the pack of the synthetic files, which this build must
# still build to the same bytes, and the one with the files themselves.
# Those with a pack of CDC's files are computed again below, where CDC's
# files are given.
for saved in "$ROOT"/cmd/bean/testdata/saved/*/bundle; do
  "$PY" -I "$ROOT/tools/compare/reproduce.py" -accept-pack "$DIGEST" "$saved" "$T/syn.pack" "$DATA/DEMO_J.XPT" "$DATA/TCHOL_J.XPT" >"$T/saved.txt" 2>&1
  grep -q '^DIFFERS' "$T/saved.txt" && say "a comparison that a release saved does not compute again: $(grep '^DIFFERS' "$T/saved.txt" | head -1)"
  [ "$(grep -c '^reproduced' "$T/saved.txt")" -ge 2 ] || say "reproduce.py computes fewer than 2 of the comparisons in $saved again: $(tail -1 "$T/saved.txt")"
done

# Packs with one thing wrong. bean and check.py must both refuse each.
"$PY" -I - "$T/syn.pack" "$T/wrong" <<'EOF' || { echo "the wrong packs were not made"; exit 1; }
import copy, json, os, sys
src, out = sys.argv[1:3]
os.mkdir(out)
good = json.load(open(src))
def cell(p): return p["cells"][0]
def first(d): return d[sorted(d)[0]]
def many(p): return next(c for c in p["cells"] if len(c["values"]) > 40)  # so that n stays above 30
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
    "weight-1e21": lambda p: cell(p)["weights"].__setitem__(0, 1e21),
    "weight-1e16": lambda p: cell(p)["weights"].__setitem__(0, 1e16),
    "weight-subnormal": lambda p: cell(p)["weights"].__setitem__(0, 5e-324),
    "value-tiny": lambda p: cell(p)["values"].__setitem__(0, 1e-13),
    "title-zero-width": lambda p: p.update(title="a\u200bb"),
    "title-filler": lambda p: p.update(title="\u3164"),
    "title-nbsp": lambda p: p.update(title="a\u00a0b"),
    "title-tag": lambda p: p.update(title="a\U000e0041b"),
    "title-bidi": lambda p: p.update(title="a\u202eb"),
    "cohort-text-escape": lambda p: first(p["cohorts"]).update(text="a\x1b[2Jb"),
    "same-share": lambda p: first(p["measures"])["same"].append({"type": "share", "unit": "{record}"}),
    "same-comparison": lambda p: first(p["measures"])["same"].append({"type": "comparison.percentile", "unit": "{percentile}"}),
    "weight-missing": lambda p: cell(p)["weights"].pop(),
    "values-none": lambda p: cell(p).update(values=[], weights=[]),
    "n-small": lambda p: cell(p).update(n=29),
    "n-fraction": lambda p: cell(p).update(n=cell(p)["n"] + 0.5),
    "n-below-values": lambda p: many(p).update(n=len(many(p)["values"]) - 1),
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
  # Both packs on record: the pack of six files, and the pack of eight.
  SIX="DEMO_J TCHOL_J HDL_J GHB_J CBC_J BMX_J"
  for set in "$SIX" "$SIX BIOPRO_J FERTIN_J"; do
    REAL=(); for f in $set; do for e in XPT xpt; do [ -f "$BEAN_NHANES_DIR/$f.$e" ] && REAL+=("$BEAN_NHANES_DIR/$f.$e"); done; done
    n=${#REAL[@]}
    bean-pack-nhanes -out "$T/real-$n.pack" "${REAL[@]}" >"$T/real.txt" 2>&1 || say "the pack of $n files of CDC was not built: $(cat "$T/real.txt")"
    bean -dir "$T/log" compare -pack "$T/real-$n.pack" 2>&1 | grep -q "that bean has on record" || say "the pack of $n files of CDC is not a pack on record"
    "$PY" -I "$HERE/check.py" "$T/real-$n.pack" >/dev/null 2>&1 || say "check.py refuses the pack of $n files of CDC"
    "$PY" -I "$HERE/rebuild_nhanes.py" "$T/real-$n.pack" "${REAL[@]}" >"$T/re.txt" 2>&1 || say "rebuild_nhanes.py on $n files of CDC: $(tail -1 "$T/re.txt")"
  done
  # With both packs of CDC's files, every comparison that a release saved
  # computes again. One of them was saved before its pack was on record.
  for saved in "$ROOT"/cmd/bean/testdata/saved/*/bundle; do
    "$PY" -I "$ROOT/tools/compare/reproduce.py" -accept-pack "$DIGEST" "$saved" "$T/syn.pack" "$T"/real-*.pack "$DATA/DEMO_J.XPT" "$DATA/TCHOL_J.XPT" >"$T/saved.txt" 2>&1 || say "the comparisons that a release saved do not all compute again: $(grep -v '^reproduced' "$T/saved.txt" | head -2)"
  done
  real=", and each pack of CDC's files is one on record"
fi
[ "$rc" = 0 ] && echo "pack: check.py reads the pack that bean builds and gives the method's four vectors, rebuild_nhanes.py builds the same pack byte for byte and refuses $i with one other member changed, reproduce.py computes $saved saved comparisons again, refuses $forged forged claims and fails on a pack that is not on record, and bean and check.py both refuse $wrong packs with one thing wrong$real"
exit $rc
