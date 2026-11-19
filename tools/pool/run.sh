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
# A person never shares so. The script uses a scratch folder of its own and
# never the caller's log.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
PY=${PY:-python3}
DATA=$ROOT/internal/nhanes/testdata
V=$HERE/vector
URL=https://raw.githubusercontent.com/Conscious-Repository/bean/main/tools/pool/vector
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
(cd "$ROOT" && go build -o "$T/bin/" ./cmd/bean ./cmd/bean-pack-nhanes) || { echo "build failed"; exit 1; }
export PATH="$T/bin:$PATH" BEAN_NOW=2026-11-09T12:00:00Z PYTHONDONTWRITEBYTECODE=1
unset BEAN_DIR
rc=0
say() { echo "FAIL: $*"; rc=1; }
sha() { "$PY" -I -c 'import hashlib, sys; print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())' "$1"; }
builder() { "$PY" -I "$HERE/build.py" "$@"; }
# No text of a pool's pack that a builder writes names the survey or its
# publisher. The measures keep the survey pack's ids, labels and units.
names_no_survey() {
  "$PY" -I - "$1" <<'PYEOF'
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
WANT=$(tr -d ' \n' <"$V/pool.sha256")
builder -synthetic -survey "$V/SYNTHETIC-survey.pack" -survey-url "$URL/SYNTHETIC-survey.pack" -archive-url "$URL/pool-bundles.zip" \
  -out "$T/vector.pack" "$V/pool-bundles.zip" >"$T/v.txt" 2>&1 || say "the builder does not build from the committed archive: $(tail -1 "$T/v.txt")"
[ -f "$T/vector.pack" ] && [ "$(sha "$T/vector.pack")" = "$WANT" ] || say "the pack from the committed archive and survey pack does not have the SHA-256 on record ($WANT)"
"$PY" -I "$ROOT/tools/pack/check.py" "$T/vector.pack" >"$T/vc.txt" 2>&1 || say "check.py refuses the vector's pack: $(tail -1 "$T/vc.txt")"
names_no_survey "$T/vector.pack" || say "the vector's pack names the survey"
grep -q "wrote the archive" "$T/v.txt" && say "the builder wrote an archive when it was given one"

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
unseal() { "$PY" -I "$ROOT/tools/second/unseal.py" -key "$T/operator.key" "$T/in/$1.bean" "$T/open/$1" >/dev/null 2>&1; }
claims() { b "$1" ls -json -type comparison.percentile; }

# Forty good contributors: one reading, one saved comparison, one record
# that is not shared, and one share of the comparison with its reading.
for i in $(seq 1 40); do
  new_log "$i"
  b "$i" add lab.hdl-cholesterol $((35 + (i * 7) % 50)) -unit mg/dL >/dev/null 2>&1
  b "$i" add weight $((55 + i)) -unit kg >/dev/null 2>&1
  compare "$i" female-20-29 || say "log $i: no comparison was saved"
  share "$i" "c$i" -note "synthetic contributor" -type 'comparison.*'
  unseal "c$i" || say "the package of log $i did not open"
done

# The bad ones. Each is a log of its own, so that its identity is in no
# good bundle.
# A package that is damaged: the opening step names it, and hands on nothing.
new_log damaged; b damaged add lab.hdl-cholesterol 50 -unit mg/dL >/dev/null 2>&1; compare damaged female-20-29
share damaged damaged -note "synthetic contributor" -type 'comparison.*'
"$PY" -I -c 'import sys; d = bytearray(open(sys.argv[1], "rb").read()); d[len(d) // 2] ^= 1; open(sys.argv[1], "wb").write(d)' "$T/in/damaged.bean"
unseal damaged && say "a damaged package opened"
[ -e "$T/open/damaged" ] && [ -n "$(ls -A "$T/open/damaged" 2>/dev/null)" ] && say "a damaged package left an opened folder"
rm -rf "$T/open/damaged"

# A record whose signature is bad, in an opened folder.
new_log badsig; b badsig add lab.hdl-cholesterol 51 -unit mg/dL >/dev/null 2>&1; compare badsig female-20-29
share badsig badsig -note "synthetic contributor" -type 'comparison.*'; unseal badsig
"$PY" -I - "$T/open/badsig" <<'PYEOF' || say "the bad signature was not made"
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
  claims "$n" | "$PY" -I -c '
import json, sys
r = json.loads(sys.stdin.readline())
c = r["context"]
exec(sys.argv[1])
print(json.dumps({"type": r["type"], "value": r["value"], "times": {"observed": r["times"]["observed"]},
                  "refs": r["refs"], "source": dict(r["source"], ref=None), "context": c}))' "$change" >"$T/forged.jsonl"
  b "$n" append "$@" <"$T/forged.jsonl" >"$T/append.txt" 2>&1 || { say "log $n: the forged claim was not appended: $(tail -1 "$T/append.txt")"; return; }
}
# The id of the claim that is not the first one that was saved.
second_claim() { claims "$1" | "$PY" -I -c 'import json, sys; ids = [json.loads(l)["id"] for l in sys.stdin]; print(ids[-1] if len(ids) > 1 else "")'; }
first_claim() { claims "$1" | "$PY" -I -c 'import json, sys; print(json.loads(sys.stdin.readline())["id"])'; }

new_log false; b false add lab.hdl-cholesterol 52 -unit mg/dL >/dev/null 2>&1; compare false female-20-29
TRUE=$(first_claim false)
forge false 'c["result"]["percentile"] = 99.9; r["value"]["n"] = 99.9'
FORGED=$(claims false | "$PY" -I -c 'import json, sys; print([json.loads(l)["id"] for l in sys.stdin if json.loads(l)["value"]["n"] == 99.9][0])' 2>/dev/null)
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
AGENT=$(b agentclaim ls -all -json -type comparison.percentile | "$PY" -I -c 'import json, sys; print([r["id"] for r in map(json.loads, sys.stdin) if r["author"] != r["subject"]][0])' 2>/dev/null)
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

# The same bundle handed in twice.
cp -R "$T/open/c3" "$T/open/c3-again"

builder -synthetic -survey "$T/syn.pack" -survey-url https://example.invalid/survey.pack -archive-url https://example.invalid/pool-bundles.zip \
  -out "$T/pool.pack" -archive "$T/pool.zip" "$T/open" >"$T/build.txt" 2>&1 || say "the builder wrote no pack: $(tail -1 "$T/build.txt")"
line() { grep -q "$1" "$T/build.txt" || say "the builder has no line for $2"; }
grep -q "^survey pack sha256 $SURVEY\$" "$T/build.txt" || say "the builder does not print the SHA-256 of the survey pack"
[ "$(grep -c '^taken ' "$T/build.txt")" = 40 ] || say "the builder took $(grep -c '^taken ' "$T/build.txt") identities, and 40 are good"
grep -q '^cell      LBDHDD pool-female-20-29: n 40$' "$T/build.txt" || say "the cell does not hold the forty good identities"
line '^left out  folder badsig: the bundle fails its check' "a record with a bad signature"
line 'does not compute again' "a claim with false numbers"
line 'names another pack' "a claim that names another survey pack"
[ "$(grep -c "the identity's own key did not sign" "$T/build.txt")" = 1 ] || say "the builder does not name exactly one reading that an agent signed"
line 'cannot hold the value' "a value that pack/0 cannot hold"
line '^left out  folder nonote: the note' "a bundle with no note"
line '^left out  folder othernote: the note' "a bundle with another note"
line 'two different bundles of one identity' "two shares of one identity"
line 'two readings of one measure that count' "two readings of one measure"
line 'a reading with claims in two cohorts' "claims in two cohorts"
line '^the same  folder c3-again' "the same bundle handed in twice"
# The agent's claim fails the check of a claim: the person did not sign it.
[ "$(grep -c '^left out  .*: no claim that counts$' "$T/build.txt")" = 5 ] || say "five identities have no claim that counts, and the builder names $(grep -c 'no claim that counts$' "$T/build.txt")"

"$PY" -I "$ROOT/tools/pack/check.py" "$T/pool.pack" >"$T/pc.txt" 2>&1 || say "check.py refuses the pool's pack: $(tail -1 "$T/pc.txt")"
names_no_survey "$T/pool.pack" || say "the pool's pack names the survey"
grep -q '"pool-female-20-29":{"caveats":\[[^]]*\],"text":"synthetic pool: ' "$T/pool.pack" || say "the cohort's id or text does not say that it is a synthetic pool"

# The archive holds the forty bundles that were taken, and no other. The
# record that was not shared is in no bundle.
"$PY" -I - "$T/pool.zip" <<'PYEOF' || say "the archive is not the forty bundles that were taken"
import json, sys, zipfile
z = zipfile.ZipFile(sys.argv[1])
tops = {n.split("/")[0] for n in z.namelist()}
types = [json.loads(z.read(n))["type"] for n in z.namelist() if "/objects/" in n]
ok = len(tops) == 40 and "weight" not in types and types.count("comparison.percentile") == 40 and all(i.compress_type == 0 for i in z.infolist())
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
"$PY" -I -c 'import sys, zipfile; z = zipfile.ZipFile(sys.argv[1], "w"); z.writestr("../escape", b"x"); z.close()' "$T/escape.zip"
mkdir "$T/tmp"
TMPDIR="$T/tmp" builder -synthetic -survey "$T/syn.pack" -survey-url u -archive-url u -out "$T/no3.pack" "$T/escape.zip" >"$T/no3.txt" 2>&1 && say "the builder read an archive with a path that leaves its folder"
grep -q "not a file of a bundle" "$T/no3.txt" || say "the builder does not refuse an entry that leaves the folder for what it is: $(tail -1 "$T/no3.txt")"
[ -e "$T/tmp/escape" ] && say "an entry of an archive was written outside its folder"
# Nothing is written when the pack is there already: not the archive either.
builder -synthetic -survey "$T/syn.pack" -survey-url u -archive-url u -out "$T/pool.pack" -archive "$T/never.zip" "$T/shuffled" >"$T/no4.txt" 2>&1 && say "the builder wrote over a pack, from a folder"
[ -e "$T/never.zip" ] && say "the builder wrote an archive for a pack that it could not write"
[ -e "$T/no.pack" ] || [ -e "$T/no2.pack" ] || [ -e "$T/no3.pack" ] && say "a refused run left a pack"

# One more made-up log compares with the pool's pack. bean says what the
# pack is, in the pack's own words.
new_log reader; b reader add lab.hdl-cholesterol 61 -unit mg/dL >/dev/null 2>&1
b reader compare -pack "$T/pool.pack" -unknown-pack -cohort pool-female-20-29 -type lab.hdl-cholesterol >"$T/cmp.txt" 2>&1 || say "bean does not compare with the pool's pack: $(tail -1 "$T/cmp.txt")"
grep -q "synthetic pool" "$T/cmp.txt" || say "the comparison does not say that the pack is a synthetic pool"
grep -q "NO record of this pack" "$T/cmp.txt" || say "bean does not say that it has no record of the pool's pack"
grep -q " 40 " "$T/cmp.txt" || say "the comparison does not show the forty identities"

[ $rc = 0 ] && echo "pool: the vector's pack has the SHA-256 on record; the path runs with 40 made-up contributors, and 12 bad inputs are left out"
exit $rc
