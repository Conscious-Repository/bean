#!/usr/bin/env bash
# smoke.sh: builds the page that checks a bundle in a browser, makes a good
# bundle and a damaged one from the synthetic example, and tries the page in
# a real browser (smoke.cjs). tools/second/bundles.sh asks the page's check
# about many more bundles without a browser; this tries what only a browser
# has: the page's own script, its buttons, and its content policy.
# It needs node with Playwright and Chromium, which CI does not have.
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
PY=${PY:-python3}
NODE=${NODE:-node}
"$NODE" -e 'require("playwright")' 2>/dev/null || { echo "page: this needs node with Playwright"; exit 1; }
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
(cd "$ROOT" && go run ./tools/page -o "$T/bean-check.html" >/dev/null)
bash "$ROOT/examples/blood/run.sh" --synthetic "$T/ex" >/dev/null 2>&1
mkdir "$T/b"
cp -R "$T/ex/bundle" "$T/b/good"
cp -R "$T/ex/bundle" "$T/b/bad"
# One byte of one record changed.
f=$(ls "$T"/b/bad/objects/*/*.json | head -1)
"$PY" -I -c 'import sys; d = bytearray(open(sys.argv[1], "rb").read()); d[len(d) // 2] ^= 1; open(sys.argv[1], "wb").write(d)' "$f"
(cd "$T/b" && "$PY" -I -c 'import shutil; shutil.make_archive("good", "zip", ".", "good")')
# A sealed package of the same records with their files, and its six words.
"$T/ex/bin/bean" -dir "$T/ex/log" share -yes -seal -files -out "$T/b/sealed.bean" -type 'comparison.*' >"$T/seal.txt" 2>/dev/null
WORDS=$(sed -n 's/^  //p' "$T/seal.txt" | head -1)
# A bundle with a row that the check marks: a comparison that the person
# signed by hand and that names no reference data.
cp -R "$T/ex/log" "$T/marked-log"
READING=$("$T/ex/bin/bean" -dir "$T/marked-log" ls -json -type lab.cholesterol-total | "$PY" -I -c 'import sys, json; print(json.loads(sys.stdin.readline())["id"])')
printf '{"type":"comparison.percentile","source":{"system":"by-hand","adapter":"test@1","ref":null},"times":{"observed":{"start":"2026-11-01T00:00:00Z","precision":"day"}},"value":{"n":95,"unit":"{percentile}"},"refs":[{"rel":"about","id":"%s"}],"context":{"reading":"%s","cohort":{"text":"male, age 30-39"},"result":{"n":339}}}\n' "$READING" "$READING" |
  "$T/ex/bin/bean" -dir "$T/marked-log" append >/dev/null
"$T/ex/bin/bean" -dir "$T/marked-log" share -yes -out "$T/b/marked" -type 'comparison.*' >/dev/null 2>&1
(cd "$T/b" && "$PY" -I -c 'import shutil; shutil.make_archive("marked", "zip", ".", "marked")')
"$NODE" "$HERE/smoke.cjs" "$T/bean-check.html" "$T/b" "$WORDS"
