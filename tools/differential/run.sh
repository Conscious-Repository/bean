#!/usr/bin/env bash
# run.sh [old rev]: builds bean at <old rev> (default origin/main) and from
# this tree, makes a corpus of logs, and diffs what the two binaries print
# on a fresh copy of each log. A change that should not change any outcome
# must show no difference. The corpus:
#   - 300 random logs (corpus.go: rotations, forks, seal parts, retired,
#     recovery and stranger keys, unknown versions, lost and broken files);
#   - the second verifier's scenario and craft logs (tools/second);
#   - one log per FuzzVerifyFiles input in the local Go fuzz cache, if any.
# Commands: verify, ls -all -json, ls -all, ls, and for logs that have
# their 24 words, rotate followed by verify (ids and keys masked).
# ALLOW is an optional regex: lines that match it are dropped from both
# outputs (use it only for a difference that a commit means to make).
# A commit that means to change what the reads and rotate print, and must not
# change what verify prints, names its base commit in reads-change-from. Only
# for that base, only the verify output is compared. Once main moves on, the
# file has no effect.
set -u
REPO=$(cd "$(dirname "$0")/../.." && pwd)
OLD=${1:-origin/main}
PY=${PY:-python3}; export PY
COUNT=${COUNT:-300}
ALLOW=${ALLOW:-'^$'}
T=$(mktemp -d); trap 'git -C "$REPO" worktree remove --force "$T/old" >/dev/null 2>&1; rm -rf "$T"' EXIT
git -C "$REPO" worktree add -q --detach "$T/old" "$OLD" || exit 1
mkdir -p "$T/bin/old" "$T/bin/new"
(cd "$T/old" && go build -o "$T/bin/old/" ./cmd/bean) || exit 1
(cd "$REPO" && go build -o "$T/bin/new/" ./cmd/bean && go build -o "$T/corpus" ./tools/differential) || exit 1
C=$T/corpus-logs; mkdir -p "$C"
"$T/corpus" "$C" "$COUNT" 1 || exit 1
FZ=$(go env GOCACHE)/fuzz/github.com/Conscious-Repository/bean/core/FuzzVerifyFiles
[ -d "$FZ" ] && "$T/corpus" -fuzz "$FZ" "$C"
bash "$REPO/tools/second/scenarios.sh" "$T/sc" >/dev/null 2>&1 && for d in "$T/sc"/*/; do
  n=$(basename "$d"); { [ "$n" = bin ] || [ "$n" = go ]; } && continue; cp -r "$d" "$C/sc-$n"; [ -f "$T/sc/$n.words" ] && cp "$T/sc/$n.words" "$C/sc-$n.words"
done
"$PY" "$REPO/tools/second/craft.py" "$T/craft" >/dev/null 2>&1 && for d in "$T/craft"/*/; do cp -r "$d" "$C/craft-$(basename "$d")"; done
export BEAN_NOW=2026-10-30T12:00:00Z
SCOPE=all
[ "$(git -C "$REPO" rev-parse "$OLD")" = "$(cat "$REPO/tools/differential/reads-change-from" 2>/dev/null)" ] && SCOPE=verify
# The prompt for the 24 words is interface text, not a result: its wording
# may change without a read changing.
mask() { sed -E 's/(sha256|ed25519|p256|secp256k1):[0-9a-f]+/\1:X/g; s/npub1[0-9a-z]+/npub1X/g' | grep -aEv "$ALLOW" | grep -av 'the 24 words'; }
run() { # run <bin> <log copy> <out file>
  local b=$1 d=$2 o=$3
  { for c in "verify" "ls -all -json" "ls -all" "ls"; do
      if [ "$SCOPE" = verify ] && [ "$c" != verify ]; then "$b" -dir "$d" $c >/dev/null 2>&1; continue; fi
      echo "## $c"; "$b" -dir "$d" $c 2>&1 | grep -aEv "$ALLOW"; echo "exit ${PIPESTATUS[0]}"
    done
    if [ -f "$d.words" ]; then
      # A build from after DECISIONS 186 needs the decision on seals.
      seal=""; "$b" help | grep -q -- -seal-all && seal="-seal-all"
      if [ "$SCOPE" = verify ]; then "$b" -dir "$d" rotate $seal <"$d.words" >/dev/null 2>&1; else
      echo "## rotate"; "$b" -dir "$d" rotate $seal <"$d.words" 2>&1 | mask | sort; echo "exit ${PIPESTATUS[0]}"; fi
      echo "## verify after rotate"; "$b" -dir "$d" verify 2>&1 | mask | sort; echo "exit ${PIPESTATUS[0]}"
    fi; } >"$o"
}
same=0; differ=0
for d in "$C"/*/; do
  n=$(basename "$d")
  for v in old new; do # the same path for both, so messages that name it agree
    rm -rf "$T/w"; mkdir -p "$T/w"; cp -r "$d" "$T/w/$n"; [ -f "$C/$n.words" ] && cp "$C/$n.words" "$T/w/$n.words"
    run "$T/bin/$v/bean" "$T/w/$n" "$T/$v.txt"
  done
  if cmp -s "$T/old.txt" "$T/new.txt"; then same=$((same + 1)); else
    differ=$((differ + 1)); echo "DIFFER $n"; diff "$T/old.txt" "$T/new.txt" | head -20 | sed 's/^/    /'
  fi
done
echo "differential vs $OLD: same $same, differ $differ ($(ls "$C"/*.words 2>/dev/null | wc -l) with rotate)"
[ "$differ" = 0 ]
