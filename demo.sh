#!/usr/bin/env bash
# Two-folder demo (SPEC.md M4 exit): init, blob put, import, append, sync,
# purge, gc, export, import-export, verify on both. Uses a fixed clock.
set -euo pipefail

export HLOG_NOW=2026-10-28T12:00:00Z
W=$(mktemp -d)
trap 'rm -rf "$W"' EXIT
export PATH="$W/bin:$PATH"
mkdir -p "$W/bin"
go build -o "$W/bin/hlog" ./cmd/hlog
go build -o "$W/bin/hlog-import-csv" ./cmd/hlog-import-csv

A="$W/a"; B="$W/b"; mkdir -p "$A" "$B"
echo "== init A"
hlog -dir "$A" init 2>/dev/null | head -1
cp -r "$A/local" "$B/local"

echo "== import a CSV into A"
cat > "$W/hrv.csv" <<EOF
time,value,id
2026-10-20T03:00:00Z,42.5,r1
2026-10-21T03:00:00Z,39.0,r2
2026-10-22T03:00:00Z,,r3
2026-10-23T03:00:00Z,45.1,r4
EOF
BLOB=$(hlog -dir "$A" blob put "$W/hrv.csv")
echo "blob $BLOB"
hlog-import-csv -type hrv.rmssd -system csv -unit ms -blob "$BLOB" "$W/hrv.csv" | hlog -dir "$A" append

echo "== re-import adds nothing"
hlog-import-csv -type hrv.rmssd -system csv -unit ms -blob "$BLOB" "$W/hrv.csv" | hlog -dir "$A" append

echo "== append one claim on B, then sync both ways"
echo '{"type":"weight.kg","value":{"n":81.2},"times":{"observed":{"start":"2026-10-28T07:00:00Z","precision":"minute"}},"source":{"system":"manual","adapter":"demo@1","ref":null}}' | hlog -dir "$B" append
hlog -dir "$B" sync "$A"
hlog -dir "$A" sync "$B"
diff <(cd "$A" && find objects blobs -type f | sort) <(cd "$B" && find objects blobs -type f | sort) && echo "file sets identical"

echo "== verify both"
hlog -dir "$A" verify
hlog -dir "$B" verify

echo "== purge one record on A, sync to B; B hides it"
TARGET=$(hlog -dir "$A" reindex >/dev/null && python3 -c "import json;d=json.load(open('$A/local/cache/index.json'));print([r['id'] for r in d['records'] if r['type']=='hrv.rmssd'][0])")
hlog -dir "$A" purge -reason "demo" "$TARGET"
hlog -dir "$B" sync "$A"
hlog -dir "$B" verify | grep -c "hidden pending gc" | sed 's/^/hidden on B: /'

echo "== 8 days later, gc on B"
HLOG_NOW=2026-11-05T12:00:00Z hlog -dir "$B" gc
HLOG_NOW=2026-11-05T12:00:00Z hlog -dir "$B" verify | tail -1

echo "== export A and import into a fresh C"
C="$W/c"; mkdir -p "$C"; cp -r "$A/local" "$C/local"; rm -rf "$C/local/cache" "$C/local/holds.json" "$C/local/purged-refs.jsonl"
hlog -dir "$A" export "$W/x" 2>/dev/null
hlog -dir "$C" import-export "$W/x"
hlog -dir "$C" verify | tail -1
echo "== done"
