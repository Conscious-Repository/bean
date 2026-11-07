#!/usr/bin/env bash
# End-to-end and adversarial suite (DECISIONS 106 to 117). Every log is a
# throwaway in a temp folder with fresh keys. Usage: tools/smoke.sh
# Env: SMOKE_N (perf records, default 50000; above 200000 the rotation writes
# seal continuations and is killed three times), SMOKE_CRASH (kill loops, default 30).
set -uo pipefail
cd "$(dirname "$0")/.."
ROOT=$PWD
W=$(mktemp -d)
trap 'rm -rf "$W"' EXIT
export PATH="$W/bin:$PATH" BEAN_NOW=2026-10-30T12:00:00Z
unset BEAN_DIR
mkdir -p "$W/bin"
go build -o "$W/bin/" ./cmd/... ./tools/adversary || exit 1
PASS=0 FAIL=0
ok() { PASS=$((PASS + 1)); echo "ok   $1"; }
bad() { FAIL=$((FAIL + 1)); echo "FAIL $1"; }
# want <name> <exit code> <pattern or ""> <cmd...>: run, check code and combined output.
want() {
	local name=$1 code=$2 pat=$3; shift 3
	local out rc
	out=$("$@" 2>&1); rc=$?
	if [ "$rc" != "$code" ]; then bad "$name: exit $rc, want $code: $(echo "$out" | tail -3)"; return; fi
	if [ -n "$pat" ] && ! grep -qE -- "$pat" <<<"$out"; then bad "$name: no /$pat/ in: $(echo "$out" | tail -3)"; return; fi
	ok "$name"
}
newlog() { mkdir -p "$1"; bean -dir "$1" init 2>/dev/null >"$1.init"; awk 'NR>1{print $2}' "$1.init" >"$1.words"; }
claim() { printf '{"type":"hr","value":{"n":%s,"unit":"bpm"},"times":{"observed":null},"source":{"system":"smoke","adapter":"smoke@1","ref":null}}\n' "$1"; }
verify_code() { bean -dir "$1" verify >/dev/null 2>&1; echo $?; }

echo "== concurrent rotations on one folder (B1)"
L=$W/rot; newlog "$L"; claim 1 | bean -dir "$L" append >/dev/null
for i in 1 2 3 4; do bean -dir "$L" rotate -seal-all <"$L.words" >/dev/null 2>&1 & done; wait
want "four concurrent rotations leave one valid chain" 0 "summary" bean -dir "$L" verify
n=$(grep -rl '"bean.succession"' "$L/objects" | wc -l)
[ "$n" = 4 ] && ok "four successions, no fork" || bad "successions: $n"

echo "== concurrent appends and purges (B1)"
L=$W/conc; newlog "$L"
for i in $(seq 1 12); do (id=$(bean -dir "$L" add hr "$i" -unit bpm 2>/dev/null); [ $((i % 2)) = 0 ] && [ -n "$id" ] && bean -dir "$L" purge "$id" >/dev/null) & done; wait
want "log valid after 12 concurrent writers" 0 "" bean -dir "$L" verify
n=$(bean -dir "$L" ls -json | grep -c '"type":"hr"')
[ "$n" = 6 ] && ok "six records left after six purges" || bad "records left: $n"

echo "== blobs: loose blobs survive, purged blobs go (B2, C25)"
L=$W/blob; newlog "$L"
printf 'loose pdf bytes' >"$W/loose.pdf"; LOOSE=$(bean -dir "$L" blob put "$W/loose.pdf")
printf 'time,value\n2026-10-20T03:00:00Z,1\n' >"$W/one.csv"; B1=$(bean -dir "$L" blob put "$W/one.csv")
bean-import-csv -type hr -blob "$B1" "$W/one.csv" | bean -dir "$L" append >/dev/null
want "csv importer without -blob warns (raw first, 9.3)" 0 "no -blob" bean-import-csv -type hr "$W/one.csv"
if bean-import-csv -type hr -blob "$B1" "$W/one.csv" 2>&1 >/dev/null | grep -q "no -blob"; then bad "csv importer with -blob warns"; else ok "csv importer with -blob does not warn"; fi
X=$(bean -dir "$L" add hr 5 -unit bpm)
bean -dir "$L" purge "$X" >/dev/null
want "loose blob kept after an unrelated purge" 0 "" bean -dir "$L" blob get "$LOOSE"
T=$(bean -dir "$L" ls -json | python3 -c 'import json,sys;print([r["id"] for r in map(json.loads, sys.stdin) if r.get("blobs")][0])' 2>/dev/null)
if [ -n "$T" ]; then bean -dir "$L" purge "$T" >/dev/null; want "purged record's own blob is erased" 1 "" bean -dir "$L" blob get "$B1"; else bad "could not find the blob record"; fi
want "loose blob kept after gc" 0 "" env BEAN_NOW=2026-12-30T12:00:00Z bean -dir "$L" blob get "$LOOSE"
BEAN_NOW=2026-12-30T12:00:00Z bean -dir "$L" gc >/dev/null 2>&1
want "loose blob kept after a late gc" 0 "" bean -dir "$L" blob get "$LOOSE"

echo "== fork freezes; gc and writes refuse (DECISIONS 107, 108)"
A=$W/fa; newlog "$A"; B=$W/fb; mkdir -p "$B"; cp -r "$A/local" "$B/"; bean -dir "$B" sync "$A" >/dev/null
PB=$(bean -dir "$A" blob put "$W/loose.pdf")
bean -dir "$A" rotate -seal-all <"$A.words" >/dev/null 2>&1; bean -dir "$B" rotate -seal-all <"$A.words" >/dev/null 2>&1
bean -dir "$A" sync "$B" >/dev/null 2>&1
want "verify reports the freeze" 4 "compromised: " bean -dir "$A" verify
want "append on a frozen identity refuses" 4 "compromised: " sh -c "$(declare -f claim); claim 9 | bean -dir '$A' append"
want "gc on a frozen identity refuses" 4 "compromised: " env BEAN_NOW=2026-12-30T12:00:00Z bean -dir "$A" gc
want "blob kept on the frozen log" 0 "" bean -dir "$A" blob get "$PB"

echo "== retired key (DECISIONS 112)"
A=$W/ra; newlog "$A"; claim 1 | bean -dir "$A" append >/dev/null
OLD=$W/rold; mkdir -p "$OLD"; cp -r "$A/local" "$OLD/"
bean -dir "$A" rotate -seal-all <"$A.words" >/dev/null 2>&1
want "log valid after rotation; old claim sealed" 0 "" bean -dir "$A" verify
RID=$(adversary old-key-claim "$A" "$OLD")
want "claim by the retired key is not sealed" 1 "$RID: not sealed" bean -dir "$A" verify
rm -f "$A/$(echo "$RID" | sed -E 's/^sha256:(..)(.*)$/objects\/\1\/\2.json/')"
bean -dir "$OLD" sync "$A" >/dev/null 2>&1
want "a copy with the retired key refuses to write" 1 "retired key" sh -c "$(declare -f claim); claim 2 | bean -dir '$OLD' append"

echo "== a thief with the current key halts; the 24 words go past it (C29)"
A=$W/ha; newlog "$A"; claim 1 | bean -dir "$A" append >/dev/null
STOLEN=$W/hstolen; mkdir -p "$STOLEN"; cp -r "$A/local" "$STOLEN/"
HID=$(adversary halt "$A" "$STOLEN")
want "a halt by the current key: too new" 3 "too new: " bean -dir "$A" verify
want "append refuses while halted" 3 "too new: " sh -c "$(declare -f claim); claim 2 | bean -dir '$A' append"
want "rotate with the 24 words goes past the halt" 0 "past halt $HID" bean -dir "$A" rotate -seal-all <"$A.words"
want "after the rotation the halt is ungated" 0 "ungated: $HID" bean -dir "$A" verify
want "append works again" 0 "" sh -c "$(declare -f claim); claim 3 | bean -dir '$A' append"
H2=$(adversary halt "$A" "$STOLEN")
want "a second halt by the stolen key is ungated" 0 "ungated: $H2" bean -dir "$A" verify

echo "== foreign genesis on restore (DECISIONS 109)"
A=$W/ga; newlog "$A"; claim 1 | bean -dir "$A" append >/dev/null
ID=$(awk 'NR==1{print $2}' "$A.init")
adversary stranger-genesis "$A" >/dev/null
N=$W/gn; mkdir -p "$N"; cp -r "$A/objects" "$A/blobs" "$N/" 2>/dev/null; cp -r "$A/objects" "$N/"
want "restore with two matching genesis records is ambiguous" 1 "ambiguous: " bean -dir "$N" restore -seal-all <"$A.words"
[ ! -e "$N/local/identity" ] && ok "ambiguous restore wrote no identity" || bad "ambiguous restore wrote local/identity"
want "restore -identity picks the person" 0 "identity $ID" bean -dir "$N" restore -seal-all -identity "$ID" <"$A.words"
want "restored log verifies" 0 "" bean -dir "$N" verify

echo "== crash during append and rotate (DECISIONS 110)"
A=$W/ca; newlog "$A"
for i in $(seq 1 3000); do claim "$i"; done >"$W/big.jsonl"
CR=${SMOKE_CRASH:-30}; cbad=0
for i in $(seq 1 "$CR"); do
	bean -dir "$A" append <"$W/big.jsonl" >/dev/null 2>&1 & p=$!
	sleep "0.0$((RANDOM % 9 + 1))"; kill -9 $p 2>/dev/null; wait $p 2>/dev/null
	c=$(verify_code "$A"); [ "$c" = 0 ] || { cbad=$((cbad + 1)); bean -dir "$A" verify | tail -3; }
done
[ $cbad = 0 ] && ok "$CR killed appends leave a valid log" || bad "$cbad of $CR killed appends left errors"
rbad=0
for i in $(seq 1 "$CR"); do
	bean -dir "$A" rotate -seal-all <"$A.words" >/dev/null 2>&1 & p=$!
	sleep "0.00$((RANDOM % 9 + 1))"; kill -9 $p 2>/dev/null; wait $p 2>/dev/null
	c=$(verify_code "$A"); claim 1 | bean -dir "$A" append >/dev/null 2>&1; a=$?
	[ "$c" = 0 ] && [ $a = 0 ] || { rbad=$((rbad + 1)); bean -dir "$A" verify | tail -3; }
done
[ $rbad = 0 ] && ok "$CR killed rotations leave a log that verifies and writes" || bad "$rbad of $CR killed rotations broke the log"
ls "$A"/objects/*/.tmp-* >/dev/null 2>&1 && echo "note: temp files from kills remain (removed after 1 h)"

echo "== corrupt, truncated and duplicate files"
A=$W/co; newlog "$A"; for i in 1 2 3; do claim "$i"; done | bean -dir "$A" append >/dev/null
F=$(find "$A/objects" -name '*.json' | head -1)
cp "$F" "$W/keep"; head -c 40 "$W/keep" >"$F"
want "truncated record is reported" 1 "bad json" bean -dir "$A" verify
cp "$W/keep" "$F"; cp "$F" "$(dirname "$F")/$(printf '0%.0s' $(seq 1 62)).json"
want "copied record under a wrong name is reported" 1 "bad path|id mismatch" bean -dir "$A" verify
rm "$(dirname "$F")/$(printf '0%.0s' $(seq 1 62)).json"
BL=$(bean -dir "$A" blob put "$W/loose.pdf"); BF=$(find "$A/blobs" -type f | head -1); printf 'x' >>"$BF"
want "corrupt blob is refused on read" 1 "" bean -dir "$A" blob get "$BL"
want "bad blob id is an error, not a panic" 1 "bad field" bean -dir "$A" blob get "sha256:../../etc/passwd"
printf '{"a":1,"a":2}' >"$F"
want "duplicate key is reported" 1 "duplicate key" bean -dir "$A" verify
cp "$W/keep" "$F"

echo "== old hlog records"
if git cat-file -e cdd4ed5 2>/dev/null; then
	mkdir -p "$W/old" && git archive cdd4ed5 | tar -x -C "$W/old" && (cd "$W/old" && go build -o "$W/oldbin/" ./cmd/... >/dev/null 2>&1)
	if [ -x "$W/oldbin/hlog" ]; then
		O=$W/ol; mkdir -p "$O"; "$W/oldbin/hlog" -dir "$O" init >/dev/null 2>&1; claim 1 | "$W/oldbin/hlog" -dir "$O" append >/dev/null 2>&1
		want "an hlog log says old format" 1 "old format" bean -dir "$O" verify
	else bad "could not build cdd4ed5"; fi
else echo "skip old hlog records: commit cdd4ed5 not in this clone"; fi

echo "== oversized and hostile input"
A=$W/big; newlog "$A"
python3 -c 'print("{\"type\":\"hr\",\"value\":{\"s\":\"" + "x"*(2<<20) + "\"},\"times\":{\"observed\":null},\"source\":{\"system\":\"s\",\"adapter\":\"a@1\",\"ref\":null}}")' >"$W/huge.jsonl"
want "a 2 MiB record line is refused" 1 "" bean -dir "$A" append <"$W/huge.jsonl"
printf 'value\n%s\n' "$(head -c 3000000 /dev/zero | tr '\0' 7)" >"$W/huge.csv"
bean-import-csv -type hr "$W/huge.csv" >"$W/huge.out" 2>&1; ok "csv importer survives a 3 MB field (exit $?)"
want "append of the huge csv row is refused" 1 "" bean -dir "$A" append <"$W/huge.out"
printf 'value\n1\xff\n' >"$W/bad.csv"
want "csv with invalid UTF-8 is refused" 1 "not UTF-8" bean-import-csv -type hr "$W/bad.csv"
want "-photo /dev/zero is refused" 1 "not a regular file" bean -dir "$A" add food "toast" -photo /dev/zero
want "log still valid" 0 "" bean -dir "$A" verify

echo "== MCP abuse"
bean -dir "$A" agent init -label smoke >/dev/null 2>&1
mcp_call() { printf '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"%s","arguments":%s}}\n' "$1" "$2"; }
{
	mcp_call log_entry '{"type":"food","value":"toast","photo_path":"/dev/zero"}'
	mcp_call log_entry '{"type":"food","value":"toast","photo_path":"../../../../etc/passwd"}'
	mcp_call log_entry '{"type":"food","value":"toast","photo_path":"/etc/passwd"}'
	mcp_call read_blob '{"id":"sha256:../../../etc/passwd"}'
	mcp_call read_blob '{"id":"x"}'
	mcp_call get_record '{"id":"../../local/signing-key"}'
	mcp_call query_records '{"type":"'"$(head -c 100000 /dev/zero | tr '\0' a)"'"}'
	echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":'
	echo 'not json'
} >"$W/mcp.in"
timeout 20 bean -dir "$A" mcp -write <"$W/mcp.in" >"$W/mcp.out" 2>&1; rc=$?
[ $rc != 124 ] && ok "mcp ends on hostile input (exit $rc)" || bad "mcp hung"
grep -q "BEGIN\|root:" "$W/mcp.out" && bad "mcp leaked a file" || ok "mcp leaked no file"
grep -q "not a regular file" "$W/mcp.out" && ok "mcp refuses /dev/zero" || bad "mcp /dev/zero: $(head -c 300 "$W/mcp.out")"
grep -c '"isError":true\|"error"' "$W/mcp.out" | awk '{ if ($1 >= 6) print "ok   mcp returned " $1 " errors"; else print "FAIL mcp errors: " $1 }'
want "log valid after mcp abuse" 0 "" bean -dir "$A" verify

echo "== performance at ${SMOKE_N:-50000} records"
P=$W/perf; newlog "$P"; N=${SMOKE_N:-50000}
python3 -c "
import sys
print('time,value,id')
for i in range($N): print(f'2026-01-01T00:{i//60%60:02d}:{i%60:02d}Z,{i%97},r{i}')" >"$W/perf.csv"
t0=$(date +%s%N); bean-import-csv -type hr "$W/perf.csv" | bean -dir "$P" append >/dev/null; t1=$(date +%s%N)
echo "append $N: $(( (t1 - t0) / 1000000 )) ms"
measure() { python3 - "$@" <<'PY'
import resource, subprocess, sys, time
t = time.time(); r = subprocess.run(sys.argv[1:], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
# ru_maxrss is in bytes on macOS and in KiB on Linux.
rss = resource.getrusage(resource.RUSAGE_CHILDREN).ru_maxrss // (1 << 20 if sys.platform == "darwin" else 1 << 10)
print(f"{time.time()-t:.2f} s, max RSS {rss} MiB, exit {r.returncode}")
PY
}
echo "ls:     $(measure bean -dir "$P" ls)"
echo "verify: $(measure bean -dir "$P" verify)"
echo "add:    $(measure bean -dir "$P" add hr 60 -unit bpm)"

echo "== rotation of the perf log (C28 seal continuations)"
before=$(bean -dir "$P" ls | wc -l)
[ "$N" -gt 200000 ] && cp -r "$P" "$W/perfk"
t0=$(date +%s%N); bean -dir "$P" rotate -seal-all <"$P.words" >"$W/rot.out" 2>&1; rc=$?; t1=$(date +%s%N)
RT=$(( (t1 - t0) / 1000000 ))
parts=$(grep -c '^seal part ' "$W/rot.out")
echo "rotate $before records: $RT ms, $parts seal parts"
want_parts=$(( before > 200000 ? (before - 1) / 200000 : 0 ))
[ $rc = 0 ] && grep -q "^sealed $before\$" "$W/rot.out" && ok "rotate seals all $before records" || bad "rotate: exit $rc, $(tail -2 "$W/rot.out")"
[ "$parts" = "$want_parts" ] && ok "$parts seal parts for $before records" || bad "seal parts: $parts, want $want_parts"
want "rotated perf log verifies" 0 "summary" bean -dir "$P" verify
after=$(bean -dir "$P" ls | wc -l)
[ "$after" = "$before" ] && ok "every record still current after the rotation" || bad "records after rotation: $after, want $before"
if [ "$N" -gt 200000 ]; then
	for k in 1 2 3; do
		K=$W/perfk$k; cp -r "$W/perfk" "$K"
		bean -dir "$K" rotate -seal-all <"$P.words" >/dev/null 2>&1 & pid=$!
		sleep "$(python3 -c "import random; print(random.uniform(0.2, $RT / 1000))")"; kill -9 $pid 2>/dev/null; wait $pid 2>/dev/null
		c=$(verify_code "$K"); n=$(bean -dir "$K" ls | wc -l)
		[ "$c" = 0 ] && [ "$n" = "$before" ] && ok "kill $k during a big rotation: verify 0, $n records" || bad "kill $k: verify $c, $n records"
		rm -rf "$K"
	done
fi

echo
echo "passed $PASS, failed $FAIL"
[ $FAIL = 0 ]
