#!/usr/bin/env bash
# Build both packs from CDC's uppercase .XPT files into a new serving directory.
set -euo pipefail
if [[ $# != 2 ]]; then
  echo "usage: bash prepare.sh <CDC input directory> <new pack directory>" >&2
  exit 2
fi
src=$(cd "$1" && pwd)
mkdir -- "$2"
dest=$(cd "$2" && pwd)
repo=$(cd "$(dirname "$0")/../.." && pwd)
stage=$(mktemp -d "$dest/.build.XXXXXX")
trap 'rm -rf "$stage"' EXIT
cd "$repo"
go build -o "$stage/builder" ./cmd/bean-pack-nhanes
small=10555d6769421d4305cbcd4ff14960d8b966ddd829a50581a9584a60a84d29e2
large=5cae07db64295d565ffae7c251c17fdb9f043dc0638868557275f8b0b0311efa
files=("$src"/{DEMO_J,TCHOL_J,HDL_J,GHB_J,CBC_J,BMX_J}.XPT)
"$stage/builder" -out "$stage/$small" "${files[@]}"
"$stage/builder" -out "$stage/$large" "${files[@]}" "$src/BIOPRO_J.XPT" "$src/FERTIN_J.XPT"
(cd "$stage" && printf '%s  %s\n' "$small" "$small" "$large" "$large" | shasum -a 256 -c -)
for digest in "$small" "$large"; do
  mv "$stage/$digest" "$dest/$digest"
done
