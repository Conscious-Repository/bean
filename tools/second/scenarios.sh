#!/usr/bin/env bash
# Builds logs with the Go binaries for the second verifier (run.sh).
set -u
REPO=$(cd "$(dirname "$0")/../.." && pwd)
OUT=${1:?usage: scenarios.sh <out dir>}
rm -rf "$OUT"; mkdir -p "$OUT/bin"
(cd "$REPO" && go build -o "$OUT/bin/" ./cmd/... ./tools/adversary) || exit 1
export PATH="$OUT/bin:$PATH" BEAN_NOW=2026-10-30T12:00:00Z
unset BEAN_DIR
newlog() { mkdir -p "$1"; bean -dir "$1" init 2>/dev/null >"$1.init"; awk 'NR>1{print $2}' "$1.init" >"$1.words"; }
claim() { printf '{"type":"hr","value":{"n":%s,"unit":"bpm"},"times":{"observed":{"start":"2026-10-2%sT07:00:00Z","precision":"minute"}},"source":{"system":"smoke","adapter":"smoke@1","ref":null}}\n' "$1" "$2"; }
# 1. basic: claims, csv with blob, supersede, purge, sibling supersedes
L=$OUT/basic; newlog $L
A=$(bean -dir $L add weight 81.2 -unit kg); B=$(bean -dir $L add weight 82 -unit kg)
printf 'time,value,id\n2026-10-20T03:00:00Z,61,r1\n2026-10-20T04:00:00Z,62,r2\n' > $OUT/hr.csv
BL=$(bean -dir $L blob put $OUT/hr.csv)
bean-import-csv -type hr -system csvtest -blob "$BL" $OUT/hr.csv | bean -dir $L append >/dev/null
for n in 1 2; do printf '{"type":"weight","value":{"n":8%s,"unit":"kg"},"refs":[{"rel":"supersedes","id":"%s"}],"times":{"observed":null},"source":{"system":"self","adapter":"t@1","ref":null}}\n' $n "$A" | bean -dir $L append >/dev/null; done
bean -dir $L purge "$B" >/dev/null
printf '{"type":"note","value":{"s":"about"},"refs":[{"rel":"about","id":"%s"}],"times":{"observed":null},"source":{"system":"self","adapter":"t@1","ref":null}}\n' "$A" | bean -dir $L append >/dev/null
# 2. rotate twice with -no-seal and a restore elsewhere
L=$OUT/rotate; newlog $L
for i in 1 2 3; do claim $i $i | bean -dir $L append >/dev/null; done
X=$(bean -dir $L add hr 70 -unit bpm)
bean -dir $L rotate -seal-all -no-seal "$X" <$L.words >/dev/null 2>&1
claim 4 4 | bean -dir $L append >/dev/null
bean -dir $L rotate -seal-all <$L.words >/dev/null 2>&1
R=$OUT/restore; mkdir -p $R; cp -r $L/objects $R/; bean -dir $R restore -seal-all <$L.words >/dev/null 2>&1
claim 5 5 | bean -dir $R append >/dev/null
# 3. retired key claim
L=$OUT/retired; newlog $L; claim 1 1 | bean -dir $L append >/dev/null
O=$OUT/retired-old; mkdir -p $O; cp -r $L/local $O/
bean -dir $L rotate -seal-all <$L.words >/dev/null 2>&1
adversary old-key-claim $L $O >/dev/null
# 4. fork
A1=$OUT/fork; newlog $A1; claim 1 1 | bean -dir $A1 append >/dev/null
B1=$OUT/fork-b; mkdir -p $B1; cp -r $A1/local $B1/; bean -dir $B1 sync $A1 >/dev/null
T=$(bean -dir $A1 add hr 5 -unit bpm); bean -dir $A1 sync $B1 >/dev/null
bean -dir $A1 rotate -seal-all <$A1.words >/dev/null 2>&1; bean -dir $B1 rotate -seal-all <$A1.words >/dev/null 2>&1
bean -dir $A1 sync $B1 >/dev/null
# 5. duplicates: two hosts import the same csv before they sync, plus an agent
L=$OUT/dups; newlog $L; M=$OUT/dups-b; mkdir -p $M; cp -r $L/local $M/; bean -dir $M sync $L >/dev/null
bean-import-csv -type hr -system csvtest $OUT/hr.csv 2>/dev/null | bean -dir $L append >/dev/null
bean-import-csv -type hr -system csvtest $OUT/hr.csv 2>/dev/null | bean -dir $M append >/dev/null
bean -dir $L sync $M >/dev/null
bean -dir $L agent init -label bot >/dev/null 2>&1
claim 9 9 | bean -dir $L append -agent >/dev/null 2>&1
# 6. stranger genesis
L=$OUT/stranger; newlog $L; claim 1 1 | bean -dir $L append >/dev/null; adversary stranger-genesis $L
# 7. seal parts (Go test with a chunk of 3)
mkdir -p $OUT/go && (cd $REPO && BEAN_SECOND_OUT=$OUT/go go test ./core -run TestSecondGen -count=1 >/dev/null) && mv $OUT/go/parts $OUT/parts
# 8. a removed part (incomplete seal set)
cp -r $OUT/parts $OUT/parts-missing
P=$(grep -rl '"type":"bean.seal"' $OUT/parts-missing/objects | head -1); rm -f "$P"
# 9. a held tombstone, a corrupt file and junk
L=$OUT/junk; newlog $L; C=$(bean -dir $L add hr 1 -unit bpm)
Q=$OUT/junk-b; mkdir -p $Q; cp -r $L/local $Q/; bean -dir $Q sync $L >/dev/null; bean -dir $Q purge "$C" >/dev/null
TS=$(cd $Q && grep -rl bean.tombstone objects | head -1); mkdir -p $L/$(dirname $TS); cp $Q/$TS $L/$TS
rm -f $L/$(echo $C | sed 's/sha256:\(..\)\(.*\)/objects\/\1\/\2.json/')
mkdir -p $L/objects/00; printf '{"a":1}' > $L/objects/00/$(printf '0%.0s' $(seq 62)).json
printf 'x' > $L/objects/zz.json
# 10. a thief with the current key halts one log; the 24 words go past it in another (C29)
L=$OUT/halt-open; newlog $L; claim 1 1 | bean -dir $L append >/dev/null; adversary halt $L $L >/dev/null
L=$OUT/halt-passed; newlog $L; claim 1 1 | bean -dir $L append >/dev/null
S=$OUT/halt-stolen; mkdir -p $S; cp -r $L/local $S/; adversary halt $L $S >/dev/null
bean -dir $L rotate -seal-all <$L.words >/dev/null 2>&1; claim 2 2 | bean -dir $L append >/dev/null; adversary halt $L $S >/dev/null
ls -d $OUT/*/ | grep -v bin
