#!/usr/bin/env bash
# Two-folder demo (SPEC.md M4 exit): init, blob put, import, append, sync,
# purge, gc, export, import-export, verify on both. Uses a fixed clock.
set -euo pipefail

export BEAN_NOW=2026-10-28T12:00:00Z
W=$(mktemp -d)
trap 'rm -rf "$W"' EXIT
export PATH="$W/bin:$PATH"
mkdir -p "$W/bin"
go build -o "$W/bin/bean" ./cmd/bean
go build -o "$W/bin/bean-import-csv" ./cmd/bean-import-csv

A="$W/a"; B="$W/b"; mkdir -p "$A" "$B"
echo "== init A"
bean -dir "$A" init 2>/dev/null >"$W/a.init"; head -1 "$W/a.init"
cp -r "$A/local" "$B/local"

echo "== import a CSV into A"
cat > "$W/hrv.csv" <<EOF
time,value,id
2026-10-20T03:00:00Z,42.5,r1
2026-10-21T03:00:00Z,39.0,r2
2026-10-22T03:00:00Z,,r3
2026-10-23T03:00:00Z,45.1,r4
EOF
BLOB=$(bean -dir "$A" blob put "$W/hrv.csv")
echo "blob $BLOB"
bean-import-csv -type hrv.rmssd -system csv -unit ms -blob "$BLOB" "$W/hrv.csv" | bean -dir "$A" append

echo "== re-import adds nothing"
bean-import-csv -type hrv.rmssd -system csv -unit ms -blob "$BLOB" "$W/hrv.csv" | bean -dir "$A" append

echo "== append one claim on B, then sync both ways"
echo '{"type":"weight.kg","value":{"n":81.2},"times":{"observed":{"start":"2026-10-28T07:00:00Z","precision":"minute"}},"source":{"system":"manual","adapter":"demo@1","ref":null}}' | bean -dir "$B" append
bean -dir "$B" sync "$A"
bean -dir "$A" sync "$B"
diff <(cd "$A" && find objects blobs -type f | sort) <(cd "$B" && find objects blobs -type f | sort) && echo "file sets identical"

echo "== verify both"
bean -dir "$A" verify
bean -dir "$B" verify

echo "== purge one record on A, sync to B; B hides it"
S=$(bean -dir "$A" ls -type hrv.rmssd | head -1 | awk '{print $NF}')
F=$(ls "$A"/objects/"${S:0:2}"/"${S:2}"*.json); F=${F#"$A"/objects/}; TARGET="sha256:${F%%/*}$(basename "$F" .json)"
bean -dir "$A" purge -reason "demo" "$TARGET"
bean -dir "$B" sync "$A"
bean -dir "$B" verify | grep -c "hidden pending gc" | sed 's/^/hidden on B: /'

echo "== 8 days later, gc on B"
BEAN_NOW=2026-11-05T12:00:00Z bean -dir "$B" gc
BEAN_NOW=2026-11-05T12:00:00Z bean -dir "$B" verify | tail -1

echo "== export A and import into a fresh C"
C="$W/c"; mkdir -p "$C"; cp -r "$A/local" "$C/local"; rm -rf "$C/local/cache" "$C/local/holds.json" "$C/local/purged-refs.jsonl"
bean -dir "$A" export "$W/x" 2>/dev/null
bean -dir "$C" import-export "$W/x"
bean -dir "$C" verify | tail -1
echo "== done"
