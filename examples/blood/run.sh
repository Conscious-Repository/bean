#!/usr/bin/env bash
# The first loop, end to end, on a synthetic lab report:
#
#   1. make a throwaway log and import the report, keeping the file;
#   2. fetch the public NHANES files from CDC and keep them;
#   3. compare the readings with men of 30 to 39, and save the comparisons;
#   4. share them as a bundle;
#   5. check the bundle and compute every comparison again, without bean;
#   6. seal the same records into one file for one reader, and open it;
#   7. as that reader, sign a note about one record, and take it into the log.
#
#   examples/blood/run.sh [work folder]
#   examples/blood/run.sh --synthetic [work folder]
#
# It needs Go and curl; step 5 also needs python3 with tools/requirements.txt
# installed, and is skipped without it. Nothing is written outside the work
# folder (a new temporary one by default). Set BEAN_NHANES_DIR to a folder
# that already holds the CDC files to skip the download.
#
# --synthetic uses the small made-up files in internal/nhanes/testdata
# instead of CDC's, so it runs offline: CI does this. Its numbers describe
# nobody.
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
SYNTHETIC=; [ "${1:-}" = --synthetic ] && { SYNTHETIC=1; shift; }
WORK=${1:-$(mktemp -d)}
mkdir -p "$WORK"; WORK=$(cd "$WORK" && pwd)
say() { printf '\n== %s\n' "$*"; }

say "Build bean into $WORK/bin"
(cd "$ROOT" && go build -o "$WORK/bin/" ./cmd/...)
export PATH="$WORK/bin:$PATH" BEAN_DIR="$WORK/log"
mkdir -p "$BEAN_DIR"

say "1. A throwaway identity, and the report"
# A real identity prints 24 recovery words to write on paper. This one is
# thrown away with the folder, so its words go nowhere.
bean init >/dev/null 2>&1
bean import -yes lab -tz America/Chicago "$HERE/report.txt"

say "2. The public reference files"
if [ -n "$SYNTHETIC" ]; then
  DATA=$ROOT/internal/nhanes/testdata; FILES="DEMO_J TCHOL_J HDL_J BMX_J"; UNKNOWN=-unknown-files; WANT=$HERE/expected-synthetic.txt
  echo "(synthetic files: made up, not CDC's)"
else
  DATA=${BEAN_NHANES_DIR:-$WORK/nhanes}; FILES="DEMO_J TCHOL_J HDL_J GHB_J CBC_J BMX_J"; UNKNOWN=; WANT=$HERE/expected.txt
  mkdir -p "$DATA"
  for f in $FILES; do
    [ -f "$DATA/$f.xpt" ] || [ -f "$DATA/$f.XPT" ] || curl -fL --retry 3 -o "$DATA/$f.xpt" "https://wwwn.cdc.gov/Nchs/Data/Nhanes/Public/2017/DataFiles/$f.xpt"
  done
fi
XPT=()
for f in $FILES; do
  p=$DATA/$f.xpt; [ -f "$p" ] || p=$DATA/$f.XPT
  XPT+=("$p")
  bean import -yes nhanes "$p" >/dev/null && echo "kept $f"
done

say "3. Compare with men of 30 to 39, then with those of a BMI from 20 to 27"
bean compare $UNKNOWN -sex male -age 30-39 -type 'lab.*' -save >"$WORK/compare.txt"
grep -v '^saved ' "$WORK/compare.txt"
bean compare $UNKNOWN -sex male -age 30-39 -where BMX_J.BMXBMI=20-27 -type 'lab.*' >"$WORK/compare-bmi.txt"
cat "$WORK/compare-bmi.txt"

say "4. Share the saved comparisons"
bean share -yes -note "the example bundle" -out "$WORK/bundle" -type 'comparison.*'

say "5. Check the bundle, with bean and without"
bean check-bundle "$WORK/bundle"
if python3 -c 'import pandas, numpy, cryptography' 2>/dev/null; then
  python3 -I "$ROOT/tools/second/checkbundle.py" "$WORK/bundle" >"$WORK/checkbundle.txt"
  tail -1 "$WORK/checkbundle.txt"
  python3 -I "$ROOT/tools/compare/reproduce.py" "$WORK/bundle" "${XPT[@]}" >"$WORK/reproduce.txt"
  sed 's/^reproduced [0-9a-f]* /reproduced /' "$WORK/reproduce.txt" | grep -v '^signed by '
else
  echo "(python3 with tools/requirements.txt is not installed: the checks without bean are skipped)"
fi

say "6. Seal the same records for one reader, and open the package as that reader"
bean share -yes -seal -note "the example package" -out "$WORK/for-a-reader.bean" -type 'comparison.*' >"$WORK/seal.txt" 2>/dev/null
cat "$WORK/seal.txt"
WORDS=$(sed -n 's/^  //p' "$WORK/seal.txt" | head -1)
# The reader types the words. Here they are piped in.
echo "$WORDS" | bean check-bundle "$WORK/for-a-reader.bean" 2>/dev/null | tail -1
if echo "abandon ability able about above absent" | bean check-bundle "$WORK/for-a-reader.bean" >/dev/null 2>&1; then
  echo "a package opened with the wrong words"; exit 1
fi
if python3 -c 'import cryptography' 2>/dev/null; then
  rm -rf "$WORK/opened"
  echo "$WORDS" | python3 -I "$ROOT/tools/second/unseal.py" "$WORK/for-a-reader.bean" "$WORK/opened" | sed "s|$WORK/||"
  python3 -I "$ROOT/tools/second/checkbundle.py" "$WORK/opened" | tail -1
fi

say "7. The reader signs a note about one record, and you take it into your log"
# The reader has a log of their own: a second throwaway identity.
mkdir -p "$WORK/reader-log"
bean -dir "$WORK/reader-log" init >/dev/null 2>&1
ABOUT=$(bean ls -type lab.cholesterol-total | awk '{print $NF}' | head -1)
echo "$WORDS" | bean -dir "$WORK/reader-log" reply -to "$WORK/for-a-reader.bean" -out "$WORK/reply.bean" "$ABOUT" "Received. Can you send the report as well?" >"$WORK/reply.txt" 2>/dev/null
REPLYWORDS=$(sed -n 's/^  //p' "$WORK/reply.txt" | head -1)
READERWORDS=$(sed -n 's/^  //p' "$WORK/reply.txt" | sed -n 2p)
# You type the reply's words, hear the reader's identity words from them,
# and give the reader a name. Here the words are piped in and passed.
echo "$REPLYWORDS" | bean accept -yes -label reader -words "$READERWORDS" "$WORK/reply.bean" 2>/dev/null |
  sed "s|$WORK/||; s/ [0-9a-f]\{12\})/)/; s/sha256:[0-9a-f]*/sha256:.../; s/id starts [0-9a-f]\{12\}/id starts .../; s/key [a-z0-9]*:[0-9a-f]*/key .../"
bean ls -type note | sed 's/  *[0-9a-f]\{12\}$//'

# The numbers must be the ones this example was written with. Ids differ
# from run to run, so they are cut off, and the rows sorted. Rows with no
# number are left out.
rows() { grep -h '^lab\.' "$@" | grep -v 'not comparable' | sed 's/  *[0-9a-f]\{12\}$//; s/   */  /g' | sort; }
{ echo "# men of 30 to 39"; rows "$WORK/compare.txt"; echo "# and a BMI from 20 to 27"; rows "$WORK/compare-bmi.txt"; } >"$WORK/numbers.txt"
if diff "$WORK/numbers.txt" "$WANT" >"$WORK/diff.txt"; then
  say "Done: every number matches $(basename "$WANT"). The log, the files, the bundle and the package are in $WORK"
  echo "To be the reader in a browser: run 'make page', open bean-check.html, choose"
  echo "  $WORK/for-a-reader.bean"
  echo "and type the six words: $WORDS"
else
  say "The numbers differ from $(basename "$WANT"):"; cat "$WORK/diff.txt"; exit 1
fi
