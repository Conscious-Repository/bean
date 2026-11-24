#!/bin/sh
# Replay five synthetic readings in a fresh disposable identity.
set -eu
umask 077
if [ "$#" -ne 2 ]; then
  echo "Usage: trial.sh BEAN_BINARY ASSET_DIRECTORY" >&2
  exit 2
fi
example=$(CDPATH="" cd -- "$(dirname -- "$0")" && pwd)
trial=$(mktemp -d)
trap 'rm -rf "$trial"' EXIT
if ! "$1" -dir "$trial/log" init >/dev/null 2>"$trial/init.err"; then
  cat "$trial/init.err" >&2
  exit 1
fi
"$1" -dir "$trial/log" append < "$example/readings.jsonl" >/dev/null
"$1" -dir "$trial/log" coverage -cohort male-20-29 -sources "$example/trial.sources.json" -assets "$2" -json
