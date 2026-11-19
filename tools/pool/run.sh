#!/usr/bin/env bash
# run.sh: the pool path with made-up contributors (docs/POOL-PLAN.md, part 1).
# It is a test of the path, and no pool of people.
#
# Two tests:
#   - "The pack is what this builder gives from this archive and this survey
#     pack." The builder runs on the two committed files of vector/, and its
#     pack must have the SHA-256 that vector/pool.sha256 states.
#   - "The path runs, and these are left out." Forty made-up logs each seal
#     one saved comparison to the operator's address. The operator opens
#     the packages, and the builder writes an archive and a pack. Beside the
#     good packages the script hands in bad ones, and each must get its line
#     and stay out of the pack.
#
# Everything here is for made-up logs only: `-yes`, address words that are
# read from the operator's own output, and one key file for every sender.
# A person never shares so. The script also hides what each command prints,
# which no person should do with a share of their own. It uses a scratch
# folder of its own and never the caller's log.
#
# With POOL_VECTOR_OUT set to a new folder, it also writes there what
# vector/ holds, from this run's forty good bundles: README.md says when.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
PY=${PY:-python3}
DATA=$ROOT/internal/nhanes/testdata
V=$HERE/vector
URL=https://raw.githubusercontent.com/Conscious-Repository/bean/main/tools/pool/vector
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
(cd "$ROOT" && go build -o "$T/bin/" ./cmd/bean ./cmd/bean-pack-nhanes) || { echo "build failed"; exit 1; }
export PATH="$T/bin:$PATH" BEAN_NOW=2026-11-09T12:00:00Z
unset BEAN_DIR
rc=0
say() { echo "FAIL: $*"; rc=1; }
value_of() { echo $((35 + ($1 * 7) % 30)); }  # the made-up value of log $1: some logs share one
sha() { "$PY" -I -B -c 'import hashlib, sys; print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())' "$1"; }
builder() { "$PY" -I -B "$HERE/build.py" "$@"; }
# No text of a pool's pack that a builder writes names the survey or its
# publisher. The measures keep the survey pack's ids, labels and units.
names_no_survey() {
  "$PY" -I -B - "$1" <<'PYEOF'
import json, sys
p = json.load(open(sys.argv[1]))
texts = [p["title"], p["source"]["name"], p["source"]["publisher"], p["source"]["home"], p["builder"]["name"]] + p["caveats"]
for part in ("measures", "cohorts"):
    for x in p[part].values():
        texts += x["caveats"]
texts += [c["text"] for c in p["cohorts"].values()]
bad = [t for t in texts for word in ("NHANES", "CDC", "National Center", "Health Statistics") if word.lower() in t.lower()]
sys.exit("a text of the pool's pack names the survey: %r" % bad[0] if bad else 0)
PYEOF
}

# ---------- test 1: the committed vector ----------
# A run that writes a new vector does not hold the old one.
if [ -z "${POOL_VECTOR_OUT:-}" ]; then
WANT=$(tr -d ' \n' <"$V/pool.sha256")
builder -synthetic -survey "$V/SYNTHETIC-survey.pack" -survey-url "$URL/SYNTHETIC-survey.pack" -archive-url "$URL/pool-bundles.zip" \
  -out "$T/vector.pack" "$V/pool-bundles.zip" >"$T/v.txt" 2>&1 || say "the builder does not build from the committed archive: $(tail -1 "$T/v.txt")"
[ -f "$T/vector.pack" ] && [ "$(sha "$T/vector.pack")" = "$WANT" ] || say "the pack from the committed archive and survey pack does not have the SHA-256 on record ($WANT)"
"$PY" -I -B "$ROOT/tools/pack/check.py" "$T/vector.pack" >"$T/vc.txt" 2>&1 || say "check.py refuses the vector's pack: $(tail -1 "$T/vc.txt")"
names_no_survey "$T/vector.pack" || say "the vector's pack names the survey"
grep -q "wrote the archive" "$T/v.txt" && say "the builder wrote an archive when it was given one"
fi

# ---------- test 2: the path runs ----------
SYN=("$DATA/DEMO_J.XPT" "$DATA/TCHOL_J.XPT" "$DATA/HDL_J.XPT" "$DATA/BMX_J.XPT")
bean-pack-nhanes -unknown-files -out "$T/syn.pack" "${SYN[@]}" >/dev/null 2>&1 || { echo "the survey pack was not built"; exit 1; }
bean-pack-nhanes -unknown-files -out "$T/other.pack" "${SYN[0]}" "${SYN[2]}" >/dev/null 2>&1 || { echo "the second survey pack was not built"; exit 1; }
SURVEY=$(sha "$T/syn.pack")

# The operator makes an address. The key file stays with the operator.
bean address -out "$T/operator.key" >"$T/address.txt" 2>&1 || { echo "the operator's address was not made"; exit 1; }
ADDR=$(sed -n 's/^address \(age1[0-9a-z]*\)$/\1/p' "$T/address.txt")
WORDS=$(sed -n 's/^address words //p' "$T/address.txt")
[ -n "$ADDR" ] && [ -n "$WORDS" ] || { echo "the address or its words were not read"; exit 1; }

mkdir "$T/logs" "$T/in" "$T/open"
b() { local n=$1; shift; bean -dir "$T/logs/$n" "$@"; }
new_log() { mkdir "$T/logs/$1" && b "$1" init </dev/null >/dev/null 2>&1 || { echo "init of log $1 failed"; exit 1; }; }
compare() { b "$1" compare -pack "${3:-$T/syn.pack}" -unknown-pack -cohort "$2" -type lab.hdl-cholesterol -save >/dev/null 2>&1; }
# share <log> <name of the package> [-note <text>] [ids or -type ...]
share() {
  local n=$1 name=$2; shift 2
  b "$n" share -for "$ADDR" -address-words "$WORDS" -yes -out "$T/in/$name.bean" "$@" </dev/null >/dev/null 2>&1 || say "log $n: the share $name was not made"
}
unseal() { "$PY" -I -B "$ROOT/tools/second/unseal.py" -key "$T/operator.key" "$T/in/$1.bean" "$T/open/$1" >/dev/null 2>&1; }
claims() { b "$1" ls -json -type comparison.percentile; }

# Forty good contributors: one reading, one saved comparison, one record
# that is not shared, and one share of the comparison with its reading.
for i in $(seq 1 40); do
  new_log "$i"
  b "$i" add lab.hdl-cholesterol "$(value_of "$i")" -unit mg/dL >/dev/null 2>&1
  b "$i" add weight $((55 + i)) -unit kg >/dev/null 2>&1
  compare "$i" female-20-29 || say "log $i: no comparison was saved"
  share "$i" "c$i" -note "synthetic contributor" -type 'comparison.*'
  unseal "c$i" || say "the package of log $i did not open"
done

# One more good contributor, alone in another cohort: it is taken, and its
# cell is dropped, since a cell needs more identities than one.
new_log 41; b 41 add lab.hdl-cholesterol 50 -unit mg/dL >/dev/null 2>&1; compare 41 male-20-29 || say "log 41: no comparison was saved"
share 41 c41 -note "synthetic contributor" -type 'comparison.*'; unseal c41 || say "the package of log 41 did not open"

# The bad ones. Each is a log of its own, so that its identity is in no
# good bundle.
# A package that is damaged: the opening step names it, and hands on nothing.
new_log damaged; b damaged add lab.hdl-cholesterol 50 -unit mg/dL >/dev/null 2>&1; compare damaged female-20-29
share damaged damaged -note "synthetic contributor" -type 'comparison.*'
"$PY" -I -B -c 'import sys; d = bytearray(open(sys.argv[1], "rb").read()); d[len(d) // 2] ^= 1; open(sys.argv[1], "wb").write(d)' "$T/in/damaged.bean"
unseal damaged && say "a damaged package opened"
[ -e "$T/open/damaged" ] && [ -n "$(ls -A "$T/open/damaged" 2>/dev/null)" ] && say "a damaged package left an opened folder"
rm -rf "$T/open/damaged"

# A record whose signature is bad, in an opened folder.
new_log badsig; b badsig add lab.hdl-cholesterol 51 -unit mg/dL >/dev/null 2>&1; compare badsig female-20-29
share badsig badsig -note "synthetic contributor" -type 'comparison.*'; unseal badsig
"$PY" -I -B - "$T/open/badsig" <<'PYEOF' || say "the bad signature was not made"
import glob, json, sys
for path in glob.glob(sys.argv[1] + "/objects/*/*.json"):
    data = open(path, "rb").read()
    r = json.loads(data)
    if r["type"] == "comparison.percentile":
        sig = r["sig"]
        flipped = sig[:-1] + ("A" if sig[-1] != "A" else "B")
        open(path, "wb").write(data.replace(sig.encode(), flipped.encode()))
        sys.exit(0)
sys.exit(1)
PYEOF

# A claim with false numbers, signed by the person: bean append writes it.
# forge <log> <python that changes the claim r> [-agent]
forge() {
  local n=$1 change=$2; shift 2
  claims "$n" | "$PY" -I -B -c '
import json, sys
r = json.loads(sys.stdin.readline())
c = r["context"]
exec(sys.argv[1])
print(json.dumps({"type": r["type"], "value": r["value"], "times": {"observed": r["times"]["observed"]},
                  "refs": r["refs"], "source": dict(r["source"], ref=None), "context": c}))' "$change" >"$T/forged.jsonl"
  b "$n" append "$@" <"$T/forged.jsonl" >"$T/append.txt" 2>&1 || { say "log $n: the forged claim was not appended: $(tail -1 "$T/append.txt")"; return; }
}
# The id of the claim that is not the first one that was saved.
second_claim() { claims "$1" | "$PY" -I -B -c 'import json, sys; ids = [json.loads(l)["id"] for l in sys.stdin]; print(ids[-1] if len(ids) > 1 else "")'; }
first_claim() { claims "$1" | "$PY" -I -B -c 'import json, sys; print(json.loads(sys.stdin.readline())["id"])'; }

new_log false; b false add lab.hdl-cholesterol 52 -unit mg/dL >/dev/null 2>&1; compare false female-20-29
TRUE=$(first_claim false)
forge false 'c["result"]["percentile"] = 99.9; r["value"]["n"] = 99.9'
FORGED=$(claims false | "$PY" -I -B -c 'import json, sys; print([json.loads(l)["id"] for l in sys.stdin if json.loads(l)["value"]["n"] == 99.9][0])' 2>/dev/null)
[ -n "$FORGED" ] && [ "$FORGED" != "$TRUE" ] || say "the claim with false numbers was not made"
share false false -note "synthetic contributor" "$FORGED"; unseal false

# A claim that names another survey pack.
new_log otherpack; b otherpack add lab.hdl-cholesterol 53 -unit mg/dL >/dev/null 2>&1; compare otherpack female-20-29 "$T/other.pack"
share otherpack otherpack -note "synthetic contributor" -type 'comparison.*'; unseal otherpack

# A reading that an agent signed, with a claim of the person about it.
new_log agentreading; b agentreading agent init -label assistant >/dev/null 2>&1
b agentreading add -agent lab.hdl-cholesterol 54 -unit mg/dL >/dev/null 2>&1 || say "the agent's reading was not added"
compare agentreading female-20-29
share agentreading agentreading -note "synthetic contributor" "$(first_claim agentreading)"; unseal agentreading

# A claim that an agent signed, about a reading of the person.
new_log agentclaim; b agentclaim add lab.hdl-cholesterol 55 -unit mg/dL >/dev/null 2>&1; compare agentclaim female-20-29
b agentclaim agent init -label assistant >/dev/null 2>&1
TRUE=$(first_claim agentclaim)
forge agentclaim 'pass' -agent
AGENT=$(b agentclaim ls -all -json -type comparison.percentile | "$PY" -I -B -c 'import json, sys; print([r["id"] for r in map(json.loads, sys.stdin) if r["author"] != r["subject"]][0])' 2>/dev/null)
[ -n "$AGENT" ] || say "the agent's claim was not made"
share agentclaim agentclaim -note "synthetic contributor" "$AGENT"; unseal agentclaim

# A reading whose value pack/0 cannot hold.
new_log toolarge; b toolarge add lab.hdl-cholesterol 2000000000000000 -unit mg/dL >/dev/null 2>&1; compare toolarge female-20-29
share toolarge toolarge -note "synthetic contributor" -type 'comparison.*'; unseal toolarge

# A bundle with no note, and one with another note.
new_log nonote; b nonote add lab.hdl-cholesterol 56 -unit mg/dL >/dev/null 2>&1; compare nonote female-20-29
share nonote nonote -type 'comparison.*'; unseal nonote
new_log othernote; b othernote add lab.hdl-cholesterol 57 -unit mg/dL >/dev/null 2>&1; compare othernote female-20-29
share othernote othernote -note "synthetic contributor, and more" -type 'comparison.*'; unseal othernote

# Two different shares of one identity.
new_log twice; b twice add lab.hdl-cholesterol 58 -unit mg/dL >/dev/null 2>&1; compare twice female-20-29
share twice twice-a -note "synthetic contributor" -type 'comparison.*'; unseal twice-a
share twice twice-b -note "synthetic contributor" -type 'comparison.*'; unseal twice-b

# Two readings of one measure that count.
new_log tworeadings; b tworeadings add lab.hdl-cholesterol 59 -unit mg/dL >/dev/null 2>&1
b tworeadings add lab.hdl-cholesterol 60 -unit mg/dL -at 2026-11-01 >/dev/null 2>&1; compare tworeadings female-20-29
share tworeadings tworeadings -note "synthetic contributor" -type 'comparison.*'; unseal tworeadings

# One reading with claims in two cohorts.
new_log twocohorts; b twocohorts add lab.hdl-cholesterol 61 -unit mg/dL >/dev/null 2>&1
compare twocohorts female-20-29; compare twocohorts male-20-29
share twocohorts twocohorts -note "synthetic contributor" -type 'comparison.*'; unseal twocohorts

# Two shares of one identity, of which one has no note: the identity has
# two different bundles, whatever their notes.
new_log mixed; b mixed add lab.hdl-cholesterol 62 -unit mg/dL >/dev/null 2>&1; compare mixed female-20-29
share mixed mixed-a -note "synthetic contributor" -type 'comparison.*'; unseal mixed-a
share mixed mixed-b -type 'comparison.*'; unseal mixed-b

# A bundle that holds more than a comparison and its reading.
new_log extra; b extra add lab.hdl-cholesterol 63 -unit mg/dL >/dev/null 2>&1; b extra add weight 71 -unit kg >/dev/null 2>&1
compare extra female-20-29
share extra extra -note "synthetic contributor" "$(first_claim extra)" "$(b extra ls -json -type weight | "$PY" -I -B -c 'import json, sys; print(json.loads(sys.stdin.readline())["id"])')"; unseal extra

# Something in the folder of bundles that is no folder.
echo stray >"$T/open/stray.txt"

# The same bundle handed in twice.
cp -R "$T/open/c3" "$T/open/c3-again"

# A claim whose text no plain terminal can show: it is kept out of the main
# run, and the builder must print it and not stop on it.
mkdir "$T/uni"
new_log unicode; b unicode add lab.hdl-cholesterol 64 -unit mg/dL >/dev/null 2>&1; compare unicode female-20-29
forge unicode 'c["reference"]["measure"] = "\u540d\u524d"'
UNI=$(claims unicode | "$PY" -I -B -c 'import json, sys; print([r["id"] for r in map(json.loads, sys.stdin) if r["context"]["reference"]["measure"] != "LBDHDD"][0])' 2>/dev/null)
share unicode unicode -note "synthetic contributor" "$UNI"
"$PY" -I -B "$ROOT/tools/second/unseal.py" -key "$T/operator.key" "$T/in/unicode.bean" "$T/uni/unicode" >/dev/null 2>&1 || say "the package with the claim in other letters did not open"
"$PY" -I -B - "$HERE" -synthetic -survey "$T/syn.pack" -survey-url u -archive-url u -out "$T/uni.pack" -archive "$T/uni.zip" "$T/uni" >"$T/uni.txt" 2>&1 <<'PYEOF'
import io, sys
sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="ascii")
sys.path.insert(0, sys.argv[1])
import build
code = build.main(["build.py"] + sys.argv[2:])
sys.stdout.flush()
sys.exit(code)
PYEOF
grep -q "does not compute again" "$T/uni.txt" && grep -q "^STOPPED: no cell" "$T/uni.txt" || say "the builder stops on a text that a plain terminal cannot show: $(tail -1 "$T/uni.txt")"

builder -synthetic -survey "$T/syn.pack" -survey-url https://example.invalid/survey.pack -archive-url https://example.invalid/pool-bundles.zip \
  -out "$T/pool.pack" -archive "$T/pool.zip" "$T/open" >"$T/build.txt" 2>&1 || say "the builder wrote no pack: $(tail -1 "$T/build.txt")"
line() { grep -q "$1" "$T/build.txt" || say "the builder has no line for $2"; }
grep -q "^survey pack sha256 $SURVEY\$" "$T/build.txt" || say "the builder does not print the SHA-256 of the survey pack"
[ "$(grep -c '^taken ' "$T/build.txt")" = 41 ] || say "the builder took $(grep -c '^taken ' "$T/build.txt") identities, and 41 are good"
grep -q '^dropped   cell LBDHDD pool-male-20-29: 1 identities' "$T/build.txt" || say "the builder does not drop, and name, a cell of one identity"
grep -q 'pool-male-20-29' "$T/pool.pack" && say "the pack holds a cohort whose cell was dropped"
grep -q '^cell      LBDHDD pool-female-20-29: n 40$' "$T/build.txt" || say "the cell does not hold the forty good identities"
line '^left out  folder badsig: the bundle fails its check' "a record with a bad signature"
line 'this tool computes' "a claim with false numbers"
line 'not signed by the person' "a claim that an agent signed"
line 'names another pack' "a claim that names another survey pack"
[ "$(grep -c "the identity's own key did not sign its reading" "$T/build.txt")" = 1 ] || say "the builder does not name exactly one reading that an agent signed"
line 'cannot hold the value' "a value that pack/0 cannot hold"
line '^left out  .*: folder nonote: the note' "a bundle with no note"
line '^left out  .*: folder othernote: the note' "a bundle with another note"
line 'holds more than comparisons' "a bundle with another record"
line '^not read  stray.txt' "a file among the folders"
[ "$(grep -c 'two different bundles of one identity' "$T/build.txt")" = 2 ] || say "the builder does not leave out both identities that have two bundles"
line 'two readings of one measure that count' "two readings of one measure"
line 'a reading with claims in two cohorts' "claims in two cohorts"
line '^the same  folder c3-again' "the same bundle handed in twice"
# The agent's claim fails the check of a claim: the person did not sign it.
[ "$(grep -c '^left out  .*: no claim that counts$' "$T/build.txt")" = 5 ] || say "five identities have no claim that counts, and the builder names $(grep -c 'no claim that counts$' "$T/build.txt")"

"$PY" -I -B "$ROOT/tools/pack/check.py" "$T/pool.pack" >"$T/pc.txt" 2>&1 || say "check.py refuses the pool's pack: $(tail -1 "$T/pc.txt")"
names_no_survey "$T/pool.pack" || say "the pool's pack names the survey"
grep -q '"pool-female-20-29":{"caveats":\[[^]]*\],"text":"synthetic pool: ' "$T/pool.pack" || say "the cohort's id or text does not say that it is a synthetic pool"

# The cell is what the script's own values give: each distinct value once,
# with the number of logs that have it as its weight.
"$PY" -I -B - "$T/pool.pack" <<'PYEOF' || say "the cell's values, weights and n are not what the forty logs give"
import json, sys
cell = json.load(open(sys.argv[1]))["cells"][0]
values = [35 + (i * 7) % 30 for i in range(1, 41)]
distinct = sorted(set(values))
ok = cell["n"] == 40 and cell["values"] == distinct and cell["weights"] == [values.count(v) for v in distinct] and max(cell["weights"]) > 1
sys.exit(0 if ok else 1)
PYEOF

# The archive holds the 41 bundles that were taken, and no other. The
# record that was not shared is in no bundle.
"$PY" -I -B - "$T/pool.zip" <<'PYEOF' || say "the archive is not the 41 bundles that were taken"
import json, sys, zipfile
z = zipfile.ZipFile(sys.argv[1])
tops = {n.split("/")[0] for n in z.namelist()}
types = [json.loads(z.read(n))["type"] for n in z.namelist() if "/objects/" in n]
ok = len(tops) == 41 and "weight" not in types and types.count("comparison.percentile") == 41 and all(i.compress_type == 0 for i in z.infolist())
sys.exit(0 if ok else 1)
PYEOF

# The same bundles in another order, and under other names, give the same
# archive and the same pack.
mkdir "$T/shuffled"
i=0
for d in "$T"/open/c[0-9]*; do
  case "$d" in *-again) continue ;; esac
  i=$((i + 1)); cp -R "$d" "$T/shuffled/z$((100 - i))"
done
builder -synthetic -survey "$T/syn.pack" -survey-url https://example.invalid/survey.pack -archive-url https://example.invalid/pool-bundles.zip \
  -out "$T/pool2.pack" -archive "$T/pool2.zip" "$T/shuffled" >"$T/build2.txt" 2>&1 || say "the builder wrote no pack from the same bundles in another order"
cmp -s "$T/pool.zip" "$T/pool2.zip" || say "the same bundles in another order give another archive"
cmp -s "$T/pool.pack" "$T/pool2.pack" || say "the same bundles in another order give another pack"

# The archive alone, with the survey pack, gives the same pack again.
builder -synthetic -survey "$T/syn.pack" -survey-url https://example.invalid/survey.pack -archive-url https://example.invalid/pool-bundles.zip \
  -out "$T/pool3.pack" "$T/pool.zip" >"$T/build3.txt" 2>&1 || say "the builder wrote no pack from its own archive: $(tail -1 "$T/build3.txt")"
cmp -s "$T/pool.pack" "$T/pool3.pack" || say "the archive gives another pack than the folders did"

# What the builder refuses to do.
builder -survey "$T/syn.pack" -survey-url u -archive-url u -out "$T/no.pack" "$T/pool.zip" >"$T/no.txt" 2>&1 && say "the builder ran without -synthetic"
builder -synthetic -survey "$T/syn.pack" -survey-url u -archive-url u -out "$T/pool.pack" "$T/pool.zip" >/dev/null 2>&1 && say "the builder wrote over a pack"
builder -synthetic -survey "$T/other.pack" -survey-url u -archive-url u -out "$T/no2.pack" "$T/pool.zip" >"$T/no2.txt" 2>&1 && say "the builder wrote a pack from an archive whose claims name another survey pack"
grep -q "all of it is taken" "$T/no2.txt" || say "the builder does not say that it takes an archive whole or not at all"
# An archive with a path that leaves its folder.
"$PY" -I -B -c 'import sys, zipfile; z = zipfile.ZipFile(sys.argv[1], "w"); z.writestr("../escape", b"x"); z.close()' "$T/escape.zip"
mkdir "$T/tmp"
TMPDIR="$T/tmp" builder -synthetic -survey "$T/syn.pack" -survey-url u -archive-url u -out "$T/no3.pack" "$T/escape.zip" >"$T/no3.txt" 2>&1 && say "the builder read an archive with a path that leaves its folder"
grep -q "not a stored file of a bundle" "$T/no3.txt" || say "the builder does not refuse an entry that leaves the folder for what it is: $(tail -1 "$T/no3.txt")"
[ -e "$T/tmp/escape" ] && say "an entry of an archive was written outside its folder"
# An archive that is damaged, packed, or not written as the builder writes
# one, is refused with a line, and never with a traceback.
"$PY" -I -B - "$T/pool.zip" "$T" <<'PYEOF' || say "the hostile archives were not made"
import sys, zipfile
src, t = sys.argv[1], sys.argv[2]
data = bytearray(open(src, "rb").read())
z = zipfile.ZipFile(src)
first = z.infolist()[0]
entries = [(i.filename, z.read(i)) for i in z.infolist()]


def entry(name, mode=0o100644):
    info = zipfile.ZipInfo(name, (1980, 1, 1, 0, 0, 0))
    info.create_system, info.external_attr = 3, mode << 16
    return info


at = first.header_offset + 30 + len(first.filename.encode()) + 5   # inside the first file's bytes
flipped = bytearray(data); flipped[at] ^= 1
open(t + "/flipped.zip", "wb").write(flipped)
with zipfile.ZipFile(t + "/packed.zip", "w", zipfile.ZIP_DEFLATED) as out:
    for name, content in entries:
        out.writestr(name, content)
with zipfile.ZipFile(t + "/comment.zip", "w", zipfile.ZIP_STORED) as out:
    for name, content in entries:
        out.writestr(entry(name), content)
    out.comment = b"more than the builder writes"
with zipfile.ZipFile(t + "/link.zip", "w", zipfile.ZIP_STORED) as out:
    for i, (name, content) in enumerate(entries):
        out.writestr(entry(name, 0o120777 if i == 0 else 0o100644), content)
open(t + "/junk.zip", "wb").write(b"junk before the archive\n" + bytes(data))
import warnings
warnings.simplefilter("ignore")
with zipfile.ZipFile(t + "/twice.zip", "w", zipfile.ZIP_STORED) as out:
    for i, (name, content) in enumerate(entries):
        out.writestr(entry(name), content)
        if i == 0:
            out.writestr(entry(name), content)
with zipfile.ZipFile(t + "/folder.zip", "w", zipfile.ZIP_STORED) as out:
    out.writestr(entry(first.filename.split("/")[0] + "/", 0o040755), b"")
    for name, content in entries:
        out.writestr(entry(name), content)
PYEOF
refused() {
  builder -synthetic -survey "$T/syn.pack" -survey-url https://example.invalid/survey.pack -archive-url https://example.invalid/pool-bundles.zip \
    -out "$T/$1.pack" "$T/$1.zip" >"$T/$1.txt" 2>&1 && say "the builder wrote a pack from an archive that is $2"
  grep -q "^STOPPED: .*$3" "$T/$1.txt" || say "an archive that is $2 is not refused for what it is: $(tail -1 "$T/$1.txt")"
  [ -e "$T/$1.pack" ] && say "an archive that is $2 left a pack"
}
refused flipped "damaged" "damaged at"
refused packed "packed" "not a stored file of a bundle"
refused link "a link in one entry" "not a stored file of a bundle"
refused comment "written with a comment" "byte for byte"
refused junk "written after other bytes" "byte for byte"
refused twice "written with one entry twice" "not a stored file of a bundle"
refused folder "written with an entry for a folder" "not a stored file of a bundle"
builder -synthetic -survey "$T/syn.pack" -survey-url u -archive-url u -out "$T/no7.pack" -archive "$T/no7.zip" "$T/pool.zip" >"$T/no7.txt" 2>&1 && say "the builder took -archive with an archive"
[ -e "$T/no7.zip" ] || [ -e "$T/no7.pack" ] && say "the builder wrote a file when it was given an archive and -archive"

# Nothing is written when the pack is there already: not the archive either.
builder -synthetic -survey "$T/syn.pack" -survey-url u -archive-url u -out "$T/pool.pack" -archive "$T/never.zip" "$T/shuffled" >"$T/no4.txt" 2>&1 && say "the builder wrote over a pack, from a folder"
[ -e "$T/never.zip" ] && say "the builder wrote an archive for a pack that it could not write"
builder -synthetic -survey "$T/syn.pack" -survey-url u -archive-url u -out "$T/one.file" -archive "$T/one.file" "$T/shuffled" >"$T/no5.txt" 2>&1 && say "the builder took one file for the pack and the archive"
[ -e "$T/one.file" ] && say "the builder wrote a file that was named as the pack and as the archive"
builder -synthetic -survey "$T/nothing.pack" -survey-url u -archive-url u -out "$T/no6.pack" "$T/pool.zip" >"$T/no6.txt" 2>&1 && say "the builder ran with no survey pack"
grep -q "^STOPPED: -survey: " "$T/no6.txt" || say "the builder does not say which file is missing: $(tail -1 "$T/no6.txt")"
grep -l "Traceback" "$T"/*.txt >/dev/null 2>&1 && say "a run of the builder ended in a traceback: $(grep -l Traceback "$T"/*.txt | head -1)"
[ -e "$T/no.pack" ] || [ -e "$T/no2.pack" ] || [ -e "$T/no3.pack" ] && say "a refused run left a pack"

# One more made-up log compares with the pool's pack. bean says what the
# pack is, in the pack's own words.
new_log reader; b reader add lab.hdl-cholesterol 61 -unit mg/dL >/dev/null 2>&1
b reader compare -pack "$T/pool.pack" -unknown-pack -cohort pool-female-20-29 -type lab.hdl-cholesterol >"$T/cmp.txt" 2>&1 || say "bean does not compare with the pool's pack: $(tail -1 "$T/cmp.txt")"
grep -q "synthetic pool" "$T/cmp.txt" || say "the comparison does not say that the pack is a synthetic pool"
grep -q "NO record of this pack" "$T/cmp.txt" || say "bean does not say that it has no record of the pool's pack"
grep -q " 40 " "$T/cmp.txt" || say "the comparison does not show the forty identities"

# On request: what vector/ holds, from this run's forty good bundles.
if [ $rc = 0 ] && [ -n "${POOL_VECTOR_OUT:-}" ]; then
  mkdir "$POOL_VECTOR_OUT" || { echo "POOL_VECTOR_OUT names a folder that is there already"; exit 1; }
  cp "$T/pool.zip" "$POOL_VECTOR_OUT/pool-bundles.zip"; cp "$T/syn.pack" "$POOL_VECTOR_OUT/SYNTHETIC-survey.pack"
  builder -synthetic -survey "$T/syn.pack" -survey-url "$URL/SYNTHETIC-survey.pack" -archive-url "$URL/pool-bundles.zip" -out "$T/new-vector.pack" "$T/pool.zip" >/dev/null 2>&1 \
    && sha "$T/new-vector.pack" >"$POOL_VECTOR_OUT/pool.sha256" && echo "wrote a new vector into $POOL_VECTOR_OUT" || say "the new vector was not written"
fi

FIRST="the vector's pack has the SHA-256 on record"
[ -n "${POOL_VECTOR_OUT:-}" ] && FIRST="a new vector is written"
[ $rc = 0 ] && echo "pool: $FIRST; the path runs with 40 made-up contributors, and each bad input is left out"
exit $rc
