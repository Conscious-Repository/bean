#!/usr/bin/env bash
# run.sh: checks the two checks of a device's folder against each other.
# The software device (Go) and craft.py (Python) each write folders; the Go
# check (internal/capture) and check.py, which share no code, must give the
# same verdict on every one: good folders pass, and each folder with one
# thing wrong is refused by both. docs/DEVICE.md has the format.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
PY=${PY:-python3}
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
(cd "$ROOT" && go build -o "$T/sim" ./tools/device/sim) || { echo "build failed"; exit 1; }
rc=0; n=0
# both <name> <folder> <want: ok|bad>
both() {
  n=$((n + 1)); want=0; [ "$3" = ok ] || want=1
  "$T/sim" -check "$2" >"$T/go.txt" 2>&1; g=$?
  "$PY" -I "$HERE/check.py" "$2" >"$T/py.txt" 2>&1; p=$?
  if [ "$g" != "$want" ] || [ "$p" != "$want" ]; then
    echo "DIFFER $1: want $3, Go exit $g, check.py exit $p"; tail -3 "$T/go.txt" | sed 's/^/    go: /'; tail -3 "$T/py.txt" | sed 's/^/    py: /'; rc=1
  fi
}
"$T/sim" -out "$T/a" -n 4 -cert >/dev/null || { echo "sim failed"; exit 1; }
"$T/sim" -out "$T/b" -n 6 -from 4 >/dev/null || { echo "sim failed"; exit 1; }
both "a device's folder, with a certificate" "$T/a" ok
both "a later hand-over, from capture 4" "$T/b" ok
"$PY" -I "$HERE/craft.py" "$T/c" || { echo "craft failed"; exit 1; }
both "a folder that Python wrote" "$T/c" ok
"$PY" -I "$HERE/craft.py" "$T/e" ed25519 || { echo "craft failed"; exit 1; }
both "a folder signed with Ed25519" "$T/e" ok

# One thing wrong, written and signed by craft.py: both must refuse.
for case in $("$PY" -I "$HERE/craft.py" -cases); do
  rm -rf "$T/x"; "$PY" -I "$HERE/craft.py" "$T/x" "$case" || { echo "craft $case failed"; exit 1; }
  both "crafted: $case" "$T/x" bad
done

# Damage to a good folder: both must refuse.
damage() { rm -rf "$T/d"; cp -R "$T/a" "$T/d"; }
first() { ls "$T/d/raw" | head -1; }
# edit <sed script> <file>: GNU and BSD sed both take -i.bak. The backup goes,
# so that no case is refused for an extra file.
edit() { sed -i.bak "$1" "$2" && rm "$2.bak" || { echo "edit failed: $1"; exit 1; }; }
damage; f=$(first); printf x >>"$T/d/raw/$f";                                   both "a raw file with a byte added" "$T/d" bad
damage; f=$(first); "$PY" -I -c 'import sys; d = bytearray(open(sys.argv[1], "rb").read()); d[7] ^= 1; open(sys.argv[1], "wb").write(d)' "$T/d/raw/$f"; both "a raw file with a byte changed" "$T/d" bad
damage; rm "$T/d/raw/$(first)";                                                 both "a raw file removed" "$T/d" bad
damage; echo x >"$T/d/raw/extra";                                               both "a raw file that no statement names" "$T/d" bad
damage; edit 's/"gain_db":12/"gain_db":13/' "$T/d/captures.jsonl";              both "a setting changed after signing" "$T/d" bad
damage; edit '2d' "$T/d/captures.jsonl";                                        both "a statement removed from the middle" "$T/d" bad
damage; edit '1{h;d;};2{G;}' "$T/d/captures.jsonl";                             both "two statements in the wrong order" "$T/d" bad
damage; : >"$T/d/captures.jsonl";                                               both "no statement" "$T/d" bad
damage; rm "$T/d/device.json";                                                  both "no device.json" "$T/d" bad
damage; cp "$T/b/device.json" "$T/d/device.json";                               both "another device's device.json" "$T/d" bad
damage; cat "$T/b/captures.jsonl" >>"$T/d/captures.jsonl"; cp "$T"/b/raw/* "$T/d/raw/"; both "another device's statements added" "$T/d" bad
damage; edit 's/"model":"sim-1","sig"/"model":"sim-2","sig"/' "$T/d/device.json"; both "a certificate changed after signing" "$T/d" bad
damage; f=$(first); rm "$T/d/raw/$f"; ln -s /dev/null "$T/d/raw/$f";            both "a raw file as a link" "$T/d" bad
damage; mv "$T/d/raw" "$T/elsewhere"; ln -s "$T/elsewhere" "$T/d/raw";         both "the raw folder as a link" "$T/d" bad; rm -rf "$T/elsewhere"
damage; printf 'x\ndevice folder ok\n' >"$T/d/raw/$(printf 'x\ndevice folder ok')" 2>/dev/null; both "a raw file whose name is a line of output" "$T/d" bad
for f in go py; do grep -q '^device folder ok$' "$T/$f.txt" && { echo "DIFFER: a file's name drew a line of its own in the $f check"; rc=1; }; done

[ "$rc" = 0 ] && echo "device: the Go check and check.py agree on $n folders, written by Go and by Python, good and with one thing wrong"
exit $rc
