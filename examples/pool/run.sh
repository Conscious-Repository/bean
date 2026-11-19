#!/usr/bin/env bash
# run.sh: a trial run of the network leg of a pool, on this machine only
# (docs/POOL-PLAN.md, part 2). It is a test of the path, and no pool of
# people. Every contributor is made up: the archive is the test vector of
# tools/pool/vector.
#
# One script plays two people. The operator uploads three files to a file
# host and publishes one pointer on a relay. The other user finds the
# pointer, fetches the files, builds the pack again, and compares a made-up
# reading with it.
#
# The host and the relay are one test server of `nak`, which this script
# starts on 127.0.0.1 and stops at its end. It talks to no other host: it
# refuses every URL that is not on that server, it uses no proxy, and it
# builds bean without a fetch.
#
# Every command that signs gets a key that was made for that one command.
# No key of a log signs for the relay or the host, and `nak` never signs
# with the default key of this machine.
#
# Needs: nak, curl, Go, Python 3 with the packages of tools/requirements.txt.
# The port is 10547, or $POOL_PORT.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
PY=${PY:-python3}
V=$ROOT/tools/pool/vector
PORT=${POOL_PORT:-10547}
case "$PORT" in ''|*[!0-9]*) echo "POOL_PORT is not a port number"; exit 2;; esac
BASE=http://127.0.0.1:$PORT
RELAY=ws://127.0.0.1:$PORT
MAX=67108864  # no file that is fetched is larger than 64 MiB
for tool in nak curl go "$PY"; do
  command -v "$tool" >/dev/null 2>&1 || { echo "this trial run needs $tool, and it is not here: README.md says how to go on without nak"; exit 2; }
done
# `nak` signs with this variable when it is set, and with a key of this
# machine when it is not. So it is cleared here, and set for one command
# at a time.
unset NOSTR_SECRET_KEY BEAN_DIR

T=$(mktemp -d)
SERVER=
trap '[ -n "$SERVER" ] && kill "$SERVER" 2>/dev/null && wait "$SERVER" 2>/dev/null; rm -rf "$T"' EXIT
stop() { echo "STOPPED: $*"; exit 1; }
not_reproduced() { echo "NOT reproduced: $*"; exit 1; }
sha() { "$PY" -I -B -c 'import hashlib, sys; print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())' "$1"; }
size() { "$PY" -I -B -c 'import os, sys; print(os.path.getsize(sys.argv[1]))' "$1"; }
builder() { "$PY" -I -B "$ROOT/tools/pool/build.py" "$@"; }
# local_only <URL>: the script fetches from its own server, and from no other.
local_only() { case "$1" in "$BASE"/*) ;; *) stop "a URL is not on this script's own server, and the script fetches no other: $1";; esac; }
# web <curl arguments>: no proxy, whatever the environment or a curlrc says,
# and a URL is taken as it is written.
web() { curl -q -g -sS --noproxy '*' "$@"; }
# fetch <URL> <new file>: at most 64 MiB, however the host sends them.
fetch() {
  local_only "$1"
  web --fail "$1" 2>/dev/null | head -c $((MAX + 1)) >"$2"
  [ "${PIPESTATUS[0]}" = 0 ] && [ "$(size "$2")" -le "$MAX" ] || not_reproduced "the host did not serve $1, or served more than 64 MiB"
}
# new_key: a key for one command. It fails when nak prints no key, so that
# no command signs with the default key of this machine.
new_key() { local k; k=$(nak key generate </dev/null 2>/dev/null); [ "${#k}" = 64 ] && echo "$k"; }
# upload <file>: prints the URL that the host gave. The host must state the
# SHA-256 that this machine computes.
upload() {
  local k; k=$(new_key) || stop "nak made no key: nothing was signed"
  NOSTR_SECRET_KEY=$k nak blossom --server "$BASE" upload "$1" </dev/null >"$T/upload.json" 2>"$T/upload.err" || stop "the upload of $1 failed: $(tail -1 "$T/upload.err")"
  "$PY" -I -B -c 'import json, sys; d = json.load(open(sys.argv[1])); sys.exit(1) if d["sha256"] != sys.argv[2] else print(d["url"])' "$T/upload.json" "$(sha "$1")" \
    || stop "the host states another SHA-256 for $1 than this machine computes"
}

mkdir "$T/bin" "$T/operator" "$T/user"
# No fetch for the build either: Go takes its modules from this machine.
(cd "$ROOT" && GOPROXY=off GOTOOLCHAIN=local go build -o "$T/bin/" ./cmd/bean) >"$T/build.txt" 2>&1 \
  || stop "bean was not built without a fetch: run \`make build\` once, and then this script again. $(tail -1 "$T/build.txt")"
export PATH="$T/bin:$PATH"

# ---------- 1. a relay and a file host, on localhost ----------
web -o /dev/null "$BASE/" 2>/dev/null && stop "something listens on port $PORT already: the script starts its own server, and uses no other. Set POOL_PORT"
nak serve --blossom --hostname 127.0.0.1 --port "$PORT" >"$T/server.txt" 2>&1 </dev/null &
SERVER=$!
for _ in 1 2 3 4 5 6 7 8 9 10; do grep -q "relay running" "$T/server.txt" && break; sleep 0.5; done
grep -q "relay running" "$T/server.txt" || stop "the test server did not start: $(tail -1 "$T/server.txt")"
echo "1. A test relay and file host run at 127.0.0.1:$PORT ($(nak --version </dev/null 2>/dev/null)). They stop when this script ends."
echo "2. Each command that signs gets a key that nak makes for it. No key of a log is used, and no key of this machine."

# ---------- 3. the other user's log ----------
U=$T/user
mkdir "$U/log"
bean -dir "$U/log" init </dev/null >/dev/null 2>&1 || stop "the made-up log was not made"
bean -dir "$U/log" add lab.hdl-cholesterol 61 -unit mg/dL >/dev/null 2>&1 || stop "the made-up reading was not added"
echo "3. The other user has a made-up log with one made-up reading."

# ---------- the operator ----------
# 4. Upload the archive and the survey pack: the two committed files. Then
# build the pack with the URLs that the host gave, and upload the pack.
ARCHIVE_URL=$(upload "$V/pool-bundles.zip") || { echo "$ARCHIVE_URL"; exit 1; }
SURVEY_URL=$(upload "$V/SYNTHETIC-survey.pack") || { echo "$SURVEY_URL"; exit 1; }
builder -synthetic -survey "$V/SYNTHETIC-survey.pack" -survey-url "$SURVEY_URL" -archive-url "$ARCHIVE_URL" \
  -out "$T/operator/pool.pack" "$V/pool-bundles.zip" >"$T/operator/build.txt" 2>&1 || stop "the operator's build failed: $(tail -1 "$T/operator/build.txt")"
PACK_URL=$(upload "$T/operator/pool.pack") || { echo "$PACK_URL"; exit 1; }
echo "4. The operator uploaded the archive, the survey pack and the pool's pack."

# 5. One pointer of kind 1063, signed with a key that was made for it.
POINTER_KEY=$(new_key) || stop "nak made no key: nothing was signed"
POINTER_PUB=$(nak key public "$POINTER_KEY" </dev/null)
NOSTR_SECRET_KEY=$POINTER_KEY nak event -k 1063 -c "synthetic pool: made-up contributors, a test of the path" \
  -t "url=$PACK_URL" -t m=application/json -t "x=$(sha "$T/operator/pool.pack")" -t "size=$(size "$T/operator/pool.pack")" \
  "$RELAY" >"$T/operator/event.json" 2>"$T/operator/event.err" </dev/null || stop "the pointer was not published: $(tail -1 "$T/operator/event.err")"
grep -q "\"pubkey\":\"$POINTER_PUB\"" "$T/operator/event.json" || stop "the pointer does not carry the key that was made for it"
echo "5. The operator published one pointer of kind 1063. A key that was made for it signed it."

# ---------- the other user ----------
# From here the script uses the relay, the host and the repository, and
# nothing that the operator's part left on the disk.
# 6. Find the pointer, and fetch the pack. The pack must have the SHA-256
# and the size that the pointer names.
nak req -k 1063 "$RELAY" >"$U/events.jsonl" 2>"$U/req.err" </dev/null || stop "the relay did not answer: $(tail -1 "$U/req.err")"
grep '"content":"synthetic pool:' "$U/events.jsonl" >"$U/event.json"
[ "$(wc -l <"$U/event.json" | tr -d ' ')" = 1 ] || stop "the relay does not have exactly one pointer of a synthetic pool"
nak verify <"$U/event.json" >/dev/null 2>&1 || not_reproduced "the pointer's signature does not hold"
# tag <name>: the value of a tag of the pointer. A pointer that has the tag
# twice, or not at all, names nothing by it.
tag() {
  "$PY" -I -B -c 'import json, sys
found = [t[1] for t in json.load(open(sys.argv[1]))["tags"] if len(t) > 1 and t[0] == sys.argv[2]]
sys.exit(1) if len(found) != 1 else print(found[0])' "$U/event.json" "$1"
}
for name in url x size; do tag "$name" >/dev/null || not_reproduced "the pointer does not have the tag $name exactly once"; done
fetch "$(tag url)" "$U/pool.pack"
[ "$(sha "$U/pool.pack")" = "$(tag x)" ] && [ "$(size "$U/pool.pack")" = "$(tag size)" ] || not_reproduced "the host served other bytes than the pointer names"
"$PY" -I -B "$ROOT/tools/pack/check.py" "$U/pool.pack" >"$U/check.txt" 2>&1 || not_reproduced "the file that the pointer names is not a pack: $(tail -1 "$U/check.txt")"
echo "6. The other user found the pointer, and fetched the pack that it names."

# 7. The two files that the pack names, each held to the SHA-256 and the
# size that the pack states.
"$PY" -I -B - "$U/pool.pack" >"$U/sources.txt" <<'PYEOF' || not_reproduced "the pack does not name exactly an archive and a survey pack"
import json, sys
files = {f["name"]: f for f in json.load(open(sys.argv[1]))["source"]["files"]}
if sorted(files) != ["pool-bundles.zip", "survey.pack"]:
    sys.exit(1)
for name in sorted(files):
    f = files[name]
    print(name, f["sha256"], f["bytes"], f["url"])
PYEOF
while read -r name want bytes url; do
  fetch "$url" "$U/$name"
  [ "$(sha "$U/$name")" = "$want" ] && [ "$(size "$U/$name")" = "$bytes" ] || not_reproduced "the host served another $name than the pack names"
  case "$name" in pool-bundles.zip) AGAIN_ARCHIVE_URL=$url;; survey.pack) AGAIN_SURVEY_URL=$url;; esac
done <"$U/sources.txt"
echo "7. They fetched the archive and the survey pack that the pack names."

# 8. Build the pack again from those two files, with the two URLs as the
# fetched pack names them. This is the step that checks something.
builder -synthetic -survey "$U/survey.pack" -survey-url "$AGAIN_SURVEY_URL" -archive-url "$AGAIN_ARCHIVE_URL" \
  -out "$U/again.pack" "$U/pool-bundles.zip" >"$U/build.txt" 2>&1 || not_reproduced "the builder does not build from the files that the host served: $(tail -1 "$U/build.txt")"
[ "$(sha "$U/again.pack")" = "$(sha "$U/pool.pack")" ] || not_reproduced "the builder gives another pack from the archive and the survey pack than the host served"
echo "8. They built the pack again, and got the same bytes."

# 9. A comparison with the fetched pack. Nothing is saved in the log.
echo "9. They compare the made-up reading with the pack. What follows is bean's own output."
echo
bean -dir "$U/log" compare -pack "$U/pool.pack" -unknown-pack -cohort pool-female-20-29 -type lab.hdl-cholesterol || stop "bean did not compare with the fetched pack"
echo

echo "What this run checked:"
echo "- The host served the bytes that the pointer named: its SHA-256 and its size."
echo "- The pack is what this builder gives from the archive and the survey pack that the host served."
if cmp -s "$U/pool-bundles.zip" "$V/pool-bundles.zip" && cmp -s "$U/survey.pack" "$V/SYNTHETIC-survey.pack"; then
  echo "- Those two files are the same bytes as the two files of the test vector in this repository."
else
  echo "- Those two files are NOT the files of the test vector in this repository: the pack was built again from other files than the vector's."
fi
echo "What it cannot show:"
echo "- One operator ran the host and the relay, so the two agree by one hand."
echo "- Nothing shows who made the keys in the archive. Every contributor here is made up."
echo "Some of bean's own words above are wrong for this pack:"
echo "- \"people\" and \"sample\": n counts the keys of one script."
echo "- \"the one that its publisher states\": this pack has no publisher."
