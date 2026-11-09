#!/usr/bin/env bash
# bundles.sh: makes share bundles with bean, good and damaged, and checks
# that bean check-bundle and the Python checker (checkbundle.py, which follows
# docs/SHARE.md and shares no code with bean) give the same verdict on each:
# both pass, or both refuse. It also computes a shared comparison again from
# its bundle alone.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
PY=${PY:-python3}
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
(cd "$ROOT" && go build -o "$T/bin/" ./cmd/bean ./cmd/bean-import-nhanes) || { echo "build failed"; exit 1; }
export PATH="$T/bin:$PATH" BEAN_NOW=2026-11-09T12:00:00Z
unset BEAN_DIR
DATA=$ROOT/internal/nhanes/testdata
L=$T/log
b() { bean -dir "$L" "$@"; }
mkdir -p "$L"; b init 2>/dev/null >"$T/init"; awk 'NR>1{print $2}' "$T/init" >"$T/words"
for f in TCHOL_J DEMO_J; do b import -yes nhanes "$DATA/$f.XPT" >/dev/null 2>&1 || { echo "import failed"; exit 1; }; done
R1=$(b add lab.cholesterol-total 193 -unit mg/dL -at 2026-11-01)
W1=$(b add weight 81.2 -unit kg -at 2026-11-02)
N1=$(b add note "felt fine" -about "$W1" -at 2026-11-02)
b compare -unknown-files -sex male -age 30-39 -save "$R1" >/dev/null 2>&1 || { echo "compare failed"; exit 1; }
C1=$(b ls -json -type comparison.percentile | "$PY" -I -c 'import sys, json; print(json.loads(sys.stdin.readline())["id"])')
# A reading that an agent signed, and a key rotation after the first records.
b agent init -label assistant >/dev/null 2>&1
A1=$(b add -agent symptom.headache 4 -unit '{score}' -at 2026-11-03)
b rotate -seal-all <"$T/words" >/dev/null 2>&1 || { echo "rotate failed"; exit 1; }
R2=$(b add lab.cholesterol-total 201 -unit mg/dL -at 2026-11-08)

rc=0; n=0
# both <name> <folder> <want: ok|bad>: the two checkers must give that verdict.
both() {
  n=$((n + 1))
  bean check-bundle "$2" >"$T/go.txt" 2>&1; g=$?
  "$PY" -I "$HERE/checkbundle.py" "$2" >"$T/py.txt" 2>&1; p=$?
  want=0; [ "$3" = ok ] || want=1
  if [ "$g" != "$want" ] || [ "$p" != "$want" ]; then
    echo "DIFFER $1: want $3, bean exit $g, checkbundle.py exit $p"; sed 's/^/    go: /' "$T/go.txt" | tail -4; sed 's/^/    py: /' "$T/py.txt" | tail -4; rc=1
  fi
  # On a good bundle the two must also count the same records, files and signers.
  if [ "$want" = 0 ] && [ "$(grep -E '^(share|records:|signed:) ' "$T/go.txt")" != "$(grep -E '^(share|records:|signed:) ' "$T/py.txt")" ]; then
    echo "DIFFER $1: the two count differently"; grep -E '^(records:|signed:) ' "$T/go.txt" "$T/py.txt" | sed 's/^/    /'; rc=1
  fi
}
share() { out=$1; shift; b share -yes -out "$out" "$@" >/dev/null 2>"$T/err" || { echo "share $* failed: $(cat "$T/err")"; exit 1; }; }
obj() { echo "$1/objects/$(echo "$2" | cut -c8-9)/$(echo "$2" | cut -c10-).json"; }

share "$T/plain" "$W1";                     both "one record" "$T/plain" ok
share "$T/about" "$N1";                     both "a note with what it is about" "$T/about" ok
[ -f "$(obj "$T/about" "$W1")" ] || { echo "DIFFER: the record a note is about did not come along"; rc=1; }
share "$T/cmp" "$C1";                       both "a comparison, files named" "$T/cmp" ok
share "$T/cmpf" -files "$C1";               both "a comparison with its files" "$T/cmpf" ok
share "$T/agent" "$A1";                     both "a reading an agent signed" "$T/agent" ok
grep -q '^signed: 0 by the person, 1 by an agent the person paired, 0 by another key' "$T/go.txt" || { echo "DIFFER: an agent's reading is not counted as one"; rc=1; }
share "$T/rot" "$R1" "$R2";                 both "records from before and after a key rotation" "$T/rot" ok
share "$T/type" -type 'lab.*' -note "for a friend"; both "by type" "$T/type" ok

# A shared comparison computes again from its bundle alone.
"$PY" -I "$ROOT/tools/compare/reproduce.py" "$T/cmpf" >"$T/repro.txt" 2>&1 || { echo "DIFFER: reproduce.py on a bundle"; tail -3 "$T/repro.txt"; rc=1; }
grep -q '^reproduced ' "$T/repro.txt" || { echo "DIFFER: nothing reproduced from a bundle"; rc=1; }

# Damage: each copy of a good bundle with one change must be refused by both.
damage() { rm -rf "$T/d"; cp -R "$1" "$T/d"; }
damage "$T/cmpf"; rm "$(obj "$T/d" "$R1")";                                   both "a listed record removed" "$T/d" bad
damage "$T/cmpf"; f=$(obj "$T/d" "$W1"); mkdir -p "$(dirname "$f")"; cp "$(obj "$L" "$W1")" "$f"; both "a record added" "$T/d" bad
damage "$T/cmpf"; f=$(ls "$T/d/files" | head -1); printf x >>"$T/d/files/$f";  both "a file changed" "$T/d" bad
damage "$T/cmpf"; f=$(ls "$T/d/files" | head -1); rm "$T/d/files/$f";          both "a file removed" "$T/d" bad
damage "$T/cmpf"; echo extra >"$T/d/files/$(printf 'a%.0s' $(seq 64))";        both "a file added" "$T/d" bad
damage "$T/cmpf"; f=$(obj "$T/d" "$R1"); sed -i.bak 's/"n":193/"n":163/' "$f"; rm -f "$f.bak"; both "a value changed" "$T/d" bad
damage "$T/cmpf"; S=$(grep -l '"type":"share"' "$T"/plain/objects/*/*.json); f="$T/d/objects/$(basename "$(dirname "$S")")"; mkdir -p "$f"; cp "$S" "$f/"; both "a second share record" "$T/d" bad
damage "$T/cmpf"; rm $(grep -l '"type":"share"' "$T"/d/objects/*/*.json);      both "no share record" "$T/d" bad
damage "$T/cmpf"; rm $(grep -l '"type":"bean.genesis"' "$T"/d/objects/*/*.json); both "no identity record" "$T/d" bad
# Without its pairing an agent's reading is still a valid record, and is
# counted as signed by another key.
damage "$T/agent"; rm $(grep -l '"type":"agent.pair"' "$T"/d/objects/*/*.json); both "an agent's reading without its pairing" "$T/d" ok
grep -q '^signed: 0 by the person, 0 by an agent the person paired, 1 by another key' "$T/go.txt" || { echo "DIFFER: an unpaired key is not counted as one"; rc=1; }
damage "$T/rot"; rm $(grep -l '"type":"bean.succession"' "$T"/d/objects/*/*.json); both "a rotation's record removed" "$T/d" bad
# A stranger's record, with the stranger's own identity, slipped in.
X=$T/stranger; mkdir -p "$X"; bean -dir "$X" init >/dev/null 2>&1; bean -dir "$X" add weight 60 -unit kg >/dev/null
damage "$T/plain"; cp -R "$X"/objects/* "$T/d/objects/";                       both "a stranger's records added" "$T/d" bad
both "a folder that is not a bundle" "$T/bin" bad
both "a whole log, not a bundle" "$L" bad

[ "$rc" = 0 ] && echo "bundles: bean check-bundle and checkbundle.py agree on $n bundles, good and damaged, and a shared comparison computes again from its bundle"
exit $rc
