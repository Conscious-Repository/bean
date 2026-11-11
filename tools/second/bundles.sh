#!/usr/bin/env bash
# bundles.sh: makes share bundles with bean, good and damaged, and checks
# that bean check-bundle and the Python checker (checkbundle.py, which follows
# docs/SHARE.md and shares no code with bean) give the same verdict on each:
# both pass, or both refuse. The page that checks a bundle in a browser
# (tools/page) is built and asked too, without a browser: it must print what
# bean prints, from a folder and from a zip file. It also computes a shared
# comparison again from its bundle alone.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
PY=${PY:-python3}
NODE=${NODE:-node}
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
(cd "$ROOT" && go build -o "$T/bin/" ./cmd/bean ./cmd/bean-import-nhanes) || { echo "build failed"; exit 1; }
command -v "$NODE" >/dev/null || { echo "node is needed, to run the page's check without a browser"; exit 1; }
(cd "$ROOT" && go run ./tools/page -o "$T/bean-check.html" >/dev/null) || { echo "page build failed"; exit 1; }
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
# A thief's copy of the log, with the key that the rotation retires.
cp -R "$L" "$T/stolen"
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
  # The page that checks a bundle in a browser carries bean's own check, so
  # it must print bean's own lines. A fourth word, disk, marks damage that
  # only a disk can hold, a link or a pipe: a browser hands a page the
  # bytes of plain files and nothing else, so the page is not asked.
  [ "${4:-}" = disk ] && return
  "$NODE" "$ROOT/tools/page/check.mjs" "$T/bean-check.html" "$2" >"$T/js.txt" 2>&1; j=$?
  if [ "$j" != "$g" ] || ! cmp -s "$T/go.txt" "$T/js.txt"; then
    echo "DIFFER $1: the page (exit $j) does not say what bean says (exit $g)"; diff "$T/go.txt" "$T/js.txt" | head -6 | sed 's/^/    /'; rc=1
  fi
  # A failed bundle shows no note of the sender: a note could say "bundle ok".
  if [ "$want" = 1 ] && grep -q '^note: ' "$T/go.txt" "$T/py.txt"; then
    echo "DIFFER $1: a failed bundle shows the sender's note"; rc=1
  fi
  # On a good bundle the two must also count the same records, files and signers.
  if [ "$want" = 0 ] && [ "$(grep -E '^(share|records:|signed:|corrections:) ' "$T/go.txt")" != "$(grep -E '^(share|records:|signed:|corrections:) ' "$T/py.txt")" ]; then
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
# As a zip file, which is how a folder travels: the page reads it packed,
# also when it was packed on a Mac, which adds a tree of its own.
"$PY" -I -c 'import sys, zipfile, os
z = zipfile.ZipFile(sys.argv[2], "w", zipfile.ZIP_DEFLATED)
for base, _, names in sorted(os.walk(sys.argv[1])):
    for n in sorted(names):
        full = os.path.join(base, n); rel = "for-ana/" + os.path.relpath(full, sys.argv[1])
        z.write(full, rel); z.writestr("__MACOSX/" + os.path.dirname(rel) + "/._" + n, b"x")
z.writestr("for-ana/.DS_Store", b"x")' "$T/cmpf" "$T/cmpf.zip"
n=$((n + 1)); bean check-bundle "$T/cmpf" >"$T/go.txt" 2>&1
"$NODE" "$ROOT/tools/page/check.mjs" "$T/bean-check.html" "$T/cmpf.zip" >"$T/js.txt" 2>&1 && cmp -s "$T/go.txt" "$T/js.txt" || { echo "DIFFER: the page reads a zip file differently from its folder"; diff "$T/go.txt" "$T/js.txt" | head -6; rc=1; }
"$PY" -I -c 'import sys; d = bytearray(open(sys.argv[1], "rb").read()); i = d.index(b"objects/") + 200; d[i] ^= 1; open(sys.argv[2], "wb").write(d)' "$T/cmpf.zip" "$T/bad.zip"
n=$((n + 1)); "$NODE" "$ROOT/tools/page/check.mjs" "$T/bean-check.html" "$T/bad.zip" >"$T/js.txt" 2>&1 && { echo "DIFFER: the page passed a zip file with a changed byte"; rc=1; }

# A correction travels without the record it replaces, and the reader is told.
K1=$(b add weight 82.5 -unit kg -at 2026-11-02 -corrects "$W1")
share "$T/corr" "$K1";                      both "a correction, without what it corrects" "$T/corr" ok
grep -q '^corrections: 1 of the listed records' "$T/go.txt" || { echo "DIFFER: a correction is not counted as one"; rc=1; }
[ -f "$(obj "$T/corr" "$W1")" ] && { echo "DIFFER: a corrected record was shared"; rc=1; }

# A shared comparison computes again from its bundle alone.
"$PY" -I "$ROOT/tools/compare/reproduce.py" "$T/cmpf" >"$T/repro.txt" 2>&1 || { echo "DIFFER: reproduce.py on a bundle"; tail -3 "$T/repro.txt"; rc=1; }
grep -q '^reproduced ' "$T/repro.txt" || { echo "DIFFER: nothing reproduced from a bundle"; rc=1; }

# Damage: each copy of a good bundle with one change must be refused by both.
damage() { rm -rf "$T/d"; cp -R "$1" "$T/d"; }
damage "$T/cmpf"; rm "$(obj "$T/d" "$R1")";                                   both "a listed record removed" "$T/d" bad
grep -q '^note: for a friend' <(bean check-bundle "$T/type") || { echo "DIFFER: a good bundle does not show its note"; rc=1; }
damage "$T/type"; mkdir -p "$T/d/files"; echo x >"$T/d/files/stray";          both "a bundle with a note, and a stray file" "$T/d" bad
damage "$T/cmpf"; f=$(obj "$T/d" "$W1"); mkdir -p "$(dirname "$f")"; cp "$(obj "$L" "$W1")" "$f"; both "a record added" "$T/d" bad
damage "$T/cmpf"; f=$(ls "$T/d/files" | head -1); printf x >>"$T/d/files/$f";  both "a file changed" "$T/d" bad
damage "$T/cmpf"; f=$(ls "$T/d/files" | head -1); rm "$T/d/files/$f";          both "a file removed" "$T/d" bad
damage "$T/cmpf"; echo extra >"$T/d/files/$(printf 'a%.0s' $(seq 64))";        both "a file added" "$T/d" bad
damage "$T/cmpf"; f=$(obj "$T/d" "$R1"); sed -i.bak 's/"n":193/"n":163/' "$f"; rm -f "$f.bak"; both "a value changed" "$T/d" bad
damage "$T/cmpf"; S=$(grep -l '"type":"share"' "$T"/plain/objects/*/*.json); f="$T/d/objects/$(basename "$(dirname "$S")")"; mkdir -p "$f"; cp "$S" "$f/"; both "a second share record" "$T/d" bad
damage "$T/cmpf"; rm $(grep -l '"type":"share"' "$T"/d/objects/*/*.json);      both "no share record" "$T/d" bad
damage "$T/cmpf"; rm $(grep -l '"type":"bean.genesis"' "$T"/d/objects/*/*.json); both "no identity record" "$T/d" bad
# The share record names the identity and pairing records it travels with:
# none can be taken out, and none put in.
damage "$T/agent"; rm $(grep -l '"type":"agent.pair"' "$T"/d/objects/*/*.json); both "an agent's reading without its pairing" "$T/d" bad
damage "$T/plain"; P=$(grep -l '"type":"agent.pair"' "$T"/agent/objects/*/*.json); f="$T/d/objects/$(basename "$(dirname "$P")")"; mkdir -p "$f"; cp "$P" "$f/"; both "a pairing added" "$T/d" bad
damage "$T/rot"; rm $(grep -l '"type":"bean.succession"' "$T"/d/objects/*/*.json); both "a rotation's record removed" "$T/d" bad
# A stranger's record, with the stranger's own identity, slipped in.
X=$T/stranger; mkdir -p "$X"; bean -dir "$X" init >/dev/null 2>&1; bean -dir "$X" add weight 60 -unit kg >/dev/null
damage "$T/plain"; cp -R "$X"/objects/* "$T/d/objects/";                       both "a stranger's records added" "$T/d" bad
# What a file browser leaves behind is not part of a bundle.
damage "$T/cmpf"; touch "$T/d/.DS_Store" "$T/d/files/.DS_Store";                both "dot files left by a file browser" "$T/d" ok
# Nor is anything else in the folder: it is not read, so it changes nothing.
# Pointing bean at a bundle with -dir leaves a local folder there.
damage "$T/cmpf"; mkdir "$T/d/local"; mkfifo "$T/d/local/identity"; echo x >"$T/d/README.txt"; both "a local folder and a stray file beside the bundle" "$T/d" ok disk
damage "$T/cmpf"; bean -dir "$T/d" ls >/dev/null 2>&1;                           both "a bundle that someone ran bean ls on" "$T/d" ok
# No link leads out of the bundle.
damage "$T/cmpf"; mv "$T/d/files" "$T/elsewhere"; ln -s "$T/elsewhere" "$T/d/files"; both "files as a link out of the bundle" "$T/d" bad disk; rm -rf "$T/elsewhere"
damage "$T/plain"; mv "$T/d/objects" "$T/elsewhere"; ln -s "$T/elsewhere" "$T/d/objects"; both "objects as a link out of the bundle" "$T/d" bad disk; rm -rf "$T/elsewhere"
damage "$T/cmpf"; f=$(ls "$T/d/files" | head -1); cp "$T/d/files/$f" "$T/keep"; rm "$T/d/files/$f"; ln -s "$T/keep" "$T/d/files/$f"; both "a file as a link" "$T/d" bad disk
damage "$T/cmpf"; rm -rf "$T/d/files"; echo x >"$T/d/files";                    both "files as a plain file" "$T/d" bad
damage "$T/plain"; f="$T/d/objects/42"; mkdir -p "$f"; mkfifo "$f/$(printf 'a%.0s' $(seq 62)).json"; both "a pipe for a record file" "$T/d" bad disk
damage "$T/plain"; f="$T/d/objects/42"; mkdir -p "$f"; truncate -s 17M "$f/$(printf 'a%.0s' $(seq 62)).json"; both "a record file too large to be one" "$T/d" bad

# Records that a stranger forged and slipped into a good bundle. forge signs
# what it is given with a new key, as a record file of any shape.
ME=$(b whoami | awk '/^identity/{print $2; exit}')
forge() {
  "$PY" -I - "$HERE" "$T/d" "$1" <<'PYEOF'
import json, os, sys
sys.path.insert(0, sys.argv[1])
import beanverify
from cryptography.hazmat.primitives.asymmetric import ed25519
from cryptography.hazmat.primitives import serialization
key = ed25519.Ed25519PrivateKey.generate()
pub = key.public_key().public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw).hex()
obj = beanverify.parse(sys.argv[3].encode())
obj.setdefault("v", beanverify.Num(0))
obj.setdefault("nonce", "00112233445566778899aabbccddeeff")
obj.setdefault("kid", "ed25519:" + pub)
obj.setdefault("times", {"observed": None, "imported": "2026-11-09T12:00:00Z"})
obj.setdefault("source", {"system": "x", "adapter": "x@1", "ref": None})
rid = beanverify.record_id(obj)
obj["id"] = rid
obj["sig"] = "ed25519:" + key.sign(bytes.fromhex(rid[7:])).hex()
d = os.path.join(sys.argv[2], "objects", rid[7:9])
os.makedirs(d, exist_ok=True)
open(os.path.join(d, rid[9:] + ".json"), "w").write(beanverify.canonical(obj))
PYEOF
}
Z=sha256:$(printf '0%.0s' $(seq 64))
damage "$T/plain"; forge '{"v":1,"type":"lab.cholesterol-total","author":"'$ME'","subject":"'$ME'","value":{"n":999,"unit":"mg/dL"}}'
both "a forged record of a newer format" "$T/d" bad
damage "$T/plain"; forge '{"type":"bean.seal","author":"'$ME'","subject":"'$ME'","refs":[{"rel":"seals","id":"'$Z'"}],"value":{"part":1,"sealed":["'$Z'"]},"context":{"message":"cholesterol 999"}}'
both "a forged identity record that names the person" "$T/d" bad
damage "$T/plain"; forge '{"v":1,"type":"bean.genesis","value":{"anything":"at all"}}'
both "a forged identity of a newer format" "$T/d" bad
damage "$T/plain"; forge '{"type":"lab.cholesterol-total","author":"'$ME'","subject":"'$ME'","value":{"n":999,"unit":"mg/dL"}}'
both "a forged record under the person's name" "$T/d" bad

# A share record that the person's own key signed, and that is not as
# SHARE.md says: bean append signs anything. Each is refused by both, and
# the Python checker says so without a traceback.
craft() {
  "$PY" -I - "$T/plain" "$1" >"$T/crafted.jsonl" <<'PYEOF'
import glob, json, sys
r = next(x for x in (json.load(open(p)) for p in glob.glob(sys.argv[1] + "/objects/*/*.json")) if x["type"] == "share")
c, v = r["context"], r["value"]
exec(sys.argv[2])
print(json.dumps({"type": "share", "value": v, "times": {"observed": r["times"]["observed"]},
                  "source": {"system": "bean-share", "adapter": "x@1", "ref": None}, "context": c}))
PYEOF
  shares() { b ls -json -type share | "$PY" -I -c 'import sys, json; print("\n".join(sorted(json.loads(l)["id"] for l in sys.stdin)))'; }
  shares >"$T/before.txt"
  b append <"$T/crafted.jsonl" >/dev/null 2>&1 || { echo "craft: append failed"; exit 1; }
  NEW=$(shares | comm -13 "$T/before.txt" -)
  [ "$(echo "$NEW" | grep -c sha256:)" = 1 ] || { echo "craft: no new share record for $2"; exit 1; }
  damage "$T/plain"; rm $(grep -l '"type":"share"' "$T"/d/objects/*/*.json)
  f=$(obj "$T/d" "$NEW"); mkdir -p "$(dirname "$f")"; cp "$(obj "$L" "$NEW")" "$f"
  both "a signed share record with $2" "$T/d" bad
  grep -q Traceback "$T/py.txt" && { echo "DIFFER: checkbundle.py crashed on a share record with $2"; rc=1; }
  b purge "$NEW" >/dev/null 2>&1
}
craft 'c["files"] = 5' "files that is not a list"
craft 'c["records"] = "x"' "records that is not a list"
craft 'c["files"] = [{"blob": "x", "record": c["records"][0], "sha256": "'$(printf 'a%.0s' $(seq 64))'", "bytes": 1}]' "a file entry with a bad blob id"
craft 'c["files"] = [{"blob": "'$Z'", "record": [1], "sha256": "'$(printf 'a%.0s' $(seq 64))'", "bytes": 1}]' "a file entry whose record is a list"
craft 'c["files"] = [{"blob": "'$Z'", "record": c["records"][0], "sha256": "'$(printf 'a%.0s' $(seq 64))'", "bytes": True}]' "bytes that is true"
craft 'c["files"] = [{"blob": "'$Z'", "record": c["records"][0], "sha256": "'$(printf 'a%.0s' $(seq 64))'"}]' "a file entry with no size"
craft 'c["files"] = [{"blob": "'$Z'", "record": c["records"][0], "sha256": "'$(printf 'a%.0s' $(seq 64))'", "bytes": 1}]' "a file that no record names"
craft 'v["n"] = 7' "a count that is not the number listed"
craft 'del c["proof"]' "no proof list"
craft 'c["proof"] = c["proof"][:-1] if len(c["proof"]) > 1 else []' "a proof list with one left out"

# A record that another identity signed about the person cannot travel: a
# bundle carries only the person's identity records. bean share says so
# before it writes anything.
printf '{"type":"note","subject":"%s","value":{"s":"seen in clinic"},"times":{"observed":null},"source":{"system":"clinic","adapter":"c@1","ref":null}}\n' "$ME" | bean -dir "$X" append >/dev/null 2>&1
# bean sync takes only a copy of this log, so the clinic's records arrive in one.
if b sync "$X" >/dev/null 2>&1; then echo "DIFFER: bean sync took a stranger's folder"; rc=1; fi
cp -R "$L/objects" "$T/mix-objects"; mkdir "$T/mix"; mv "$T/mix-objects" "$T/mix/objects"; cp -R "$X"/objects/* "$T/mix/objects/"
b sync "$T/mix" >/dev/null 2>&1
CL=$(b ls -all -json -type note | "$PY" -I -c 'import sys, json; print(next((r["id"] for r in map(json.loads, sys.stdin) if r["value"].get("s") == "seen in clinic"), ""))')
n=$((n + 1)); before=$(b ls -json -type share | grep -c .)
if [ -z "$CL" ] || b share -yes -out "$T/clinic" "$CL" >"$T/out.txt" 2>&1 || ! grep -q "cannot be shared" "$T/out.txt" || [ -e "$T/clinic" ] || [ "$(b ls -json -type share | grep -c .)" != "$before" ]; then
  echo "DIFFER: a record of another identity was not refused cleanly"; tail -2 "$T/out.txt"; rc=1
fi

# What no bundle can show. The thief's copy holds the key that the rotation
# retired. A bundle made with it passes: the rotation is not in it. Both
# checkers must say, on every bundle, that the key may no longer be the
# person's.
bean -dir "$T/stolen" add lab.cholesterol-total 999 -unit mg/dL >"$T/thief.id"
bean -dir "$T/stolen" share -yes -out "$T/thief" "$(cat "$T/thief.id")" >/dev/null 2>&1
both "a bundle signed with a key that was retired since (not detectable)" "$T/thief" ok
for f in go py; do grep -q 'cannot show that it is still the person.s key' "$T/$f.txt" || { echo "DIFFER: the $f checker does not say that the key may be retired"; rc=1; }; done
# With the rotation's record beside it, the same bundle fails.
damage "$T/thief"; S2=$(grep -l '"type":"bean.succession"' "$L"/objects/*/*.json | head -1); f="$T/d/objects/$(basename "$(dirname "$S2")")"; mkdir -p "$f"; cp "$S2" "$f/"; both "the same, with the rotation's record added" "$T/d" bad

# A record that names one file twice shares it once, and counts it once.
BH=$(b ls -json -type reference.file | "$PY" -I -c 'import sys, json; print(json.loads(sys.stdin.readline())["value"]["blob"])')
printf '{"type":"note","value":{"text":"one file, named twice"},"times":{"observed":null},"source":{"system":"self","adapter":"t@1","ref":null},"blobs":[{"hash":"%s","media_type":"application/octet-stream","role":"a"},{"hash":"%s","media_type":"application/octet-stream","role":"b"}]}\n' "$BH" "$BH" | b append >/dev/null 2>&1 || { echo "append failed"; exit 1; }
TW=$(b ls -json -type note | "$PY" -I -c 'import sys, json; print(next(r["id"] for r in map(json.loads, sys.stdin) if r["value"].get("text") == "one file, named twice"))')
share "$T/twice" -files "$TW";               both "a record that names one file twice" "$T/twice" ok
grep -q '^records: 1 listed; files: 1 included, 0 named and not included' "$T/go.txt" || { echo "DIFFER: a file named twice is not counted once"; rc=1; }

# An agent that the person unpaired since: its reading still goes, and both
# checkers count it as another key's, with no pairing in the proof.
PF=$(grep -l '"type":"agent.pair"' "$L"/objects/*/*.json | head -1)
"$PY" -I -c 'import sys, json; r = json.load(open(sys.argv[1])); print(json.dumps({"type": "agent.pair", "value": {"key": r["value"]["key"], "status": "unpaired"}, "times": {"observed": None}, "source": {"system": "self", "adapter": "t@1", "ref": None}, "refs": [{"rel": "supersedes", "id": r["id"]}]}))' "$PF" | b append >/dev/null 2>&1 || { echo "append failed"; exit 1; }
share "$T/unpaired" "$A1";                  both "a reading of an agent unpaired since" "$T/unpaired" ok
grep -q '^signed: 0 by the person, 0 by an agent the person paired, 1 by another key' "$T/go.txt" || { echo "DIFFER: an unpaired agent's reading is counted as a paired one"; rc=1; }
grep -q '"type":"agent.pair"' "$T"/unpaired/objects/*/*.json && { echo "DIFFER: a pairing that was undone went as proof"; rc=1; }

both "a folder that is not a bundle" "$T/bin" bad
both "a whole log, not a bundle" "$L" bad

[ "$rc" = 0 ] && echo "bundles: bean check-bundle and checkbundle.py agree on $n bundles, good and damaged, the page that checks one in a browser says what bean says, and a shared comparison computes again from its bundle"
exit $rc
