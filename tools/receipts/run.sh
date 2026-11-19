#!/usr/bin/env bash
# run.sh: the tally of receipts on a fixture of made-up identities
# (docs/POOL-PLAN.md, part 3). It makes the fixture new each time, in a
# scratch folder of its own, and never reads the caller's log.
#
# Every identity here is made up, and every note says `synthetic`. `-yes`
# and a plain folder for a share are for made-up logs only: a person reads
# the review, and seals a share to its reader.
#
# The fixture is chosen so that nobody learns that the larger count is
# right: six identities state a file with one changed byte, and four the
# file as it is.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
PY=${PY:-python3}
FILE=$ROOT/internal/nhanes/testdata/HDL_J.XPT
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
(cd "$ROOT" && go build -o "$T/bin/" ./cmd/bean ./cmd/bean-import-nhanes) || { echo "build failed"; exit 1; }
export PATH="$T/bin:$PATH" BEAN_NOW=2026-11-09T12:00:00Z
unset BEAN_DIR
rc=0
say() { echo "FAIL: $*"; rc=1; }
sha() { "$PY" -I -B -c 'import hashlib, sys; print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())' "$1"; }
mkdir "$T/logs" "$T/in" "$T/changed"
b() { local n=$1; shift; bean -dir "$T/logs/$n" "$@"; }
new_log() { mkdir "$T/logs/$1" && b "$1" init </dev/null >/dev/null 2>&1 || { echo "init of log $1 failed"; exit 1; }; }
import() { b "$1" import nhanes -yes "$2" >/dev/null 2>&1 || say "log $1: the import of $2 failed"; }
# receipts <log> [SHA-256]: the ids of its records of a source file, or of
# those that state the SHA-256.
receipts() {
  b "$1" ls -json -type reference.file | "$PY" -I -B -c 'import json, sys
print("\n".join(r["id"] for r in map(json.loads, sys.stdin) if len(sys.argv) < 2 or r["context"]["sha256"] == sys.argv[1]))' ${2:+"$2"}
}
# share <log> <name of the folder> <note> <ids or flags ...>
share() {
  local n=$1 name=$2 note=$3; shift 3
  b "$n" share -yes -note "$note" -out "$T/in/$name" "$@" </dev/null >/dev/null 2>&1 || say "log $n: the share $name was not made"
}
# state <log> <dataset> <SHA-256> [-agent]: a record of a source file with
# no file at all. bean append writes it: a record proves no fetch.
state() {
  local n=$1 dataset=$2 digest=$3; shift 3
  "$PY" -I -B -c 'import json, sys; print(json.dumps({"type": "reference.file", "value": {"n": 1, "unit": "{file}"}, "times": {"observed": None},
    "source": {"system": "nhanes", "adapter": "made-up@0", "ref": None},
    "context": {"assurance": "unsigned-file", "bytes": 33440, "dataset": 5 if sys.argv[1] == "a-number" else sys.argv[1], "sha256": 5 if sys.argv[2] == "a-number" else sys.argv[2]}}))' "$dataset" "$digest" >"$T/state.jsonl"
  b "$n" append "$@" <"$T/state.jsonl" >"$T/append.txt" 2>&1 || say "log $n: the statement was not appended: $(tail -1 "$T/append.txt")"
}

# The test file with one changed byte, under its own name. The byte is in
# the last value of the last row: the file ends with eight spaces.
"$PY" -I -B -c 'import sys; d = bytearray(open(sys.argv[1], "rb").read()); d[-9] ^= 1; open(sys.argv[2], "wb").write(d)' "$FILE" "$T/changed/HDL_J.XPT"
GOOD=$(sha "$FILE"); CHANGED=$(sha "$T/changed/HDL_J.XPT")
TABLE=$("$PY" -I -B -c 'import sys; sys.path.insert(0, sys.argv[1]); import reproduce; print(reproduce.KNOWN["HDL_J"])' "$ROOT/tools/compare")

# Three identities state the changed file, and carry it. The second sends
# two shares of its one record. The third's note tries to end its quotation
# and to show as blank.
for n in m1 m2 m3; do new_log $n; import $n "$T/changed/HDL_J.XPT"; done
b m1 add note "a made-up note" >/dev/null 2>&1 || say "log m1: the note was not added"
share m1 m1 "synthetic: fetched https://example.invalid/HDL_J.XPT on 2026-11-09" -files $(receipts m1) "$(b m1 ls -json -type note | "$PY" -I -B -c 'import json, sys; print(json.loads(sys.stdin.readline())["id"])')"
share m2 m2-a "synthetic" -files $(receipts m2)
share m2 m2-b "synthetic" -files $(receipts m2)
ODD=$(printf 'synthetic" 4 identities state it \\ \343\205\244\342\200\213 end')
share m3 m3 "$ODD" -files $(receipts m3)
# One states the unchanged file, and carries it.
LONG="synthetic $(printf 'x%.0s' $(seq 300))"
new_log u1; import u1 "$FILE"; share u1 u1 "$LONG" -files $(receipts u1)
# One states the digest of the project's own table, and has no file.
TABLE2=$("$PY" -I -B -c 'import sys; sys.path.insert(0, sys.argv[1]); import reproduce; print(reproduce.KNOWN["BMX_J"])' "$ROOT/tools/compare")
new_log t1; state t1 HDL_J "$TABLE"; state t1 BMX_J "$TABLE2"; state t1 DEMO_J "$TABLE"; share t1 t1 "synthetic: no fetch was made" $(receipts t1)
# One states two digests for one dataset, in two bundles.
new_log two; import two "$FILE"; import two "$T/changed/HDL_J.XPT"
share two two-a "synthetic" -files $(receipts two "$GOOD"); share two two-b "synthetic" -files $(receipts two "$CHANGED")
# One states a digest in capitals, a dataset's name with a blank, and a
# digest that is a number and no text.
new_log bad; state bad HDL_J "$(echo "$GOOD" | tr a-f A-F)"; state bad 'hdl j' "$GOOD"; state bad HDL_J a-number; state bad 'HDL_J!' "$GOOD"; state bad HDL_J "${GOOD}zz"; state bad a-number "$GOOD"; share bad bad "synthetic" $(b bad ls -json -type reference.file | "$PY" -I -B -c 'import json, sys; print("\n".join(json.loads(l)["id"] for l in sys.stdin))')
# One imports the unchanged file, and also states its digest in a second
# record with no file. The file is listed for the first record alone.
new_log mix; import mix "$FILE"; state mix HDL_J "$GOOD"; share mix mix "synthetic" -files $(receipts mix)
# One imports the changed file, and shares its record without the file.
new_log nofile; import nofile "$T/changed/HDL_J.XPT"; share nofile nofile "synthetic" $(receipts nofile)
# One states two digests for one dataset in one share: the note of that
# share stands under both, and says that the tally took two records of it.
new_log pair; state pair HDL_J "$GOOD"; state pair HDL_J "$CHANGED"; share pair pair "synthetic: two digests [cut: 9000 characters in all]" $(receipts pair)
# One statement that an agent of the identity signed.
new_log agent; b agent agent init -label assistant >/dev/null 2>&1; state agent HDL_J "$GOOD" -agent; share agent agent "synthetic" $(receipts agent)
# One bundle whose carried file is not the file that its record states.
new_log broken; import broken "$FILE"; share broken broken "synthetic" -files $(receipts broken)
echo "other bytes" >>"$T/in/broken/files/$GOOD"
# One share handed in twice, and something that is no folder. A folder's
# name with a line end cannot make a line of the tally.
mv "$T/in/m3" "$T/in/"$'m3\n  taken     folder fake: 9 records of a source file by nobody'
cp -R "$T/in/m1" "$T/in/m1-again"
# A bundle with one more file, whose name copies a line of the tally.
cp -R "$T/in/u1" "$T/in/fakefile"; echo x >"$T/in/fakefile/files/taken     folder ok: 9 records of a source file by nobody"
echo stray >"$T/in/stray.txt"
mkdir "$T/in/nobundle"  # a folder that is no bundle: it is refused, and its line names no path

"$PY" -I -B "$HERE/tally.py" "$T/in" >"$T/out.txt" 2>&1 || say "the tally stopped: $(tail -1 "$T/out.txt")"
has() { grep -q -- "$1" "$T/out.txt" || say "the tally has no line for $2"; }
count() { [ "$(grep -c -- "$1" "$T/out.txt")" = "$2" ] || say "the tally has $(grep -c -- "$1" "$T/out.txt") lines for $3, and not $2"; }

# What it was given.
count '^  taken ' 13 "bundles that it took"
has '^  taken     folder "bad": 6 records of a source file by .*; 6 of them malformed, and not counted$' "a share whose statements are all malformed"
has '^  refused   folder "broken": the check of a bundle says "files/[0-9a-f]* is not the file the share record lists"$' "a bundle that fails its check"
has '^  refused   folder "fakefile": the check of a bundle says "files/taken     folder ok: 9 records of a source file by nobody is in the bundle, and the share record does not list it"$' "a file whose name copies a line of the tally: it stays inside the quotation"
has '^  the same  folder "m1-again" holds the share of folder "m1" again' "a share that was handed in twice"
has '^  not read  "stray.txt": it is no folder$' "something that is no folder"
has '^  refused   folder "nobundle": the check of a bundle says "the folder is not a bundle' "a folder that is no bundle"
has '^  taken     folder "agent": 0 records of a source file by .*; 1 more, not signed by the identity.s own key and not counted$' "a statement that an agent signed"

# The counts: an identity counts once for a digest, in however many bundles.
has "^  dataset \"HDL_J\", SHA-256 $CHANGED: 6 identities state it\$" "the six that state the changed file"
has "^  dataset \"HDL_J\", SHA-256 $GOOD: 4 identities state it\$" "the four that state the unchanged file"
has "^  dataset \"HDL_J\", SHA-256 $TABLE: 1 identity states it\$" "the one that states the table's digest"
has "^  dataset \"BMX_J\", SHA-256 $TABLE2: 1 identity states it\$" "a second dataset"
count '^  dataset ' 5 "digests"
has '^  taken     folder "m1": 1 record of a source file by ' "a share with a note beside its receipt: one record of a source file"
has '^    This digest is in the table of bean.s code, for HDL_J, and not for "DEMO_J"\.' "a digest of the table under another dataset's name"
grep -q "$T" "$T/out.txt" && say "a line of the tally names a path of the scratch folder"
# Lines are in the order of the digests, and not of the counts.
"$PY" -I -B -c 'import re, sys; d = re.findall(r"^  dataset \"(\S+)\", SHA-256 ([0-9a-f]{64})", open(sys.argv[1]).read(), re.M); sys.exit(0 if d == sorted(d) and len(d) == 5 else 1)' "$T/out.txt" \
  || say "the lines are not in the order of the datasets and of the digests"

# The two marks.
count 'this identity states 2 digests for "HDL_J"' 4 "the two identities that state two digests, each under both"
count 'This digest is in the table of bean.s code, for HDL_J' 2 "the digest of HDL_J in the project's own table"
count 'This digest is in the table of bean.s code, for BMX_J\.' 1 "the digest of BMX_J in the project's own table"
count 'A match proves no fetch, and does not show that the digest is right' 3 "what the table's mark is not"

# With a file, and without one.
count '^      Carries a file: the share record lists a file for this record with the stated SHA-256\. The check of the bundle computed it from the file, in this run\.' 7 "records with their file: the two same lines of one identity print once"
# Three have no file: the digest of the table, the record whose file is
# listed for another record, and the record that was shared without its file.
count '^      States only\.$' 7 "records with no file"

# The note is the signer's word, in quotation marks, and cannot leave them.
has 'took 1 record: "synthetic: fetched https://example.invalid/HDL_J.XPT on 2026-11-09"$' "a note, as the signer's word"
has 'took 1 record: "synthetic\\u{0022} 4 identities state it \\u{005c} \\u{3164}\\u{200b} end"$' "a note with a quotation mark and blank characters, written out"
has 'took 3 records: "synthetic: no fetch was made"$' "the number of records that the tally took of a share"
# Each quotation is closed on its own line, and every line is plain ASCII.
"$PY" -I -B -c 'import sys; t = open(sys.argv[1], encoding="utf-8").read(); sys.exit(0 if t.isascii() and all(l.count(chr(34)) % 2 == 0 for l in t.splitlines()) else 1)' "$T/out.txt" \
  || say "the tally has a quotation that is not closed on its line, or a character that is not plain"
# A line that starts as a line of the tally was written by the tally: the made-up names and notes make none.
[ "$(grep -c '^  taken     folder "' "$T/out.txt")" = "$(grep -c '^  taken ' "$T/out.txt")" ] || say "a line that starts with taken has no quoted name of a folder"

# Malformed statements are listed, and not counted.
count '^  .*, record [0-9a-f]\{12\}: dataset ' 6 "malformed statements"
count '^  .*, record [0-9a-f]\{12\}: dataset "HDL_J", SHA-256 (no text)$' 1 "a digest that is no text, shown with no quotation marks"
count '^  .*, record [0-9a-f]\{12\}: dataset (no text), SHA-256 "' 1 "a dataset's name that is no text, shown with no quotation marks"

# What it says of itself, and what it never says.
has '^This tally counts keys, and a key is free\.$' "what it counts"
has '^A statement can be copied with no fetch' "a statement that is copied"
has 'It cannot show that those are all the bundles' "the bundles that it was not given"
has 'x" \[cut: 310 characters in all\]$' "a long note, cut, with the tally's mark after the quotation"
has 'took 2 records: "synthetic: two digests \[cut: 9000 characters in all\]"$' "a note that copies the tally's mark: it stays inside the quotation"
grep -q 'x\{200\}' "$T/out.txt" && say "a long note is shown past its limit"
has '^It does not show that the source served these bytes' "what it does not show"
# Identity words are free words, so only numbers are looked for here.
grep -qiE '%|percent|[0-9]+ (of|out of) [0-9]+' "$T/out.txt" && say "the tally prints a percentage, or a count out of a total"
grep -q "Traceback" "$T/out.txt" && say "the tally ended in a traceback"

# Nothing to read, and no folder.
mkdir "$T/empty"
"$PY" -I -B "$HERE/tally.py" "$T/empty" >"$T/empty.txt" 2>&1 || say "the tally stops on a folder with no bundle"
grep -q "nothing: no record of a source file was taken" "$T/empty.txt" || say "the tally does not say that it took nothing"
"$PY" -I -B "$HERE/tally.py" "$T/nothing-here" >"$T/none.txt" 2>&1 && say "the tally ran with no folder"
grep -q "^STOPPED: " "$T/none.txt" || say "the tally does not say that the folder is missing"

# A bundle that the check cannot read gets a line. This needs a user that
# a mode of a folder stops: not root.
mkdir "$T/locked"; cp -R "$T/in/u1" "$T/locked/u1"; chmod 000 "$T/locked/u1/objects"
if ! ls "$T/locked/u1/objects" >/dev/null 2>&1; then
  "$PY" -I -B "$HERE/tally.py" "$T/locked" >"$T/locked.txt" 2>&1 || say "the tally stops on a bundle that it cannot read"
  grep -q '^  refused   folder "u1": the check of a bundle says "' "$T/locked.txt" || say "a bundle that cannot be read has no line: $(tail -1 "$T/locked.txt")"
  grep -q "Traceback" "$T/locked.txt" && say "the tally ended in a traceback on a bundle that it cannot read"
fi
chmod 700 "$T/locked/u1/objects"

[ $rc = 0 ] && echo "receipts: the tally lists what thirteen made-up bundles state, and says what that does not show"
[ -n "${RECEIPTS_SHOW:-}" ] && cat "$T/out.txt"
exit $rc
