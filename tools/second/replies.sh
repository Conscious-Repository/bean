#!/usr/bin/env bash
# replies.sh: makes replies with bean, good and damaged, and checks that
# bean and the Python checker (checkreply.py, which follows docs/SHARE.md,
# "The reply, exactly", and shares no code with bean) give the same verdict
# on each: both pass, or both refuse. A sealed reply must open for a Python
# reader of its format, and the two must name the replier by the same words.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
PY=${PY:-python3}
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
(cd "$ROOT" && go build -o "$T/bin/" ./cmd/bean) || { echo "build failed"; exit 1; }
export PATH="$T/bin:$PATH" BEAN_NOW=2026-11-09T12:00:00Z
unset BEAN_DIR
die() { echo "$1: $(cat "$T/err" 2>/dev/null)"; exit 1; }
newlog() { mkdir -p "$1"; bean -dir "$1" init 2>/dev/null >"$1.init"; awk 'NR>1{print $2}' "$1.init" >"$1.words"; }
B=$T/ben; A=$T/ana; newlog "$B"; newlog "$A"
ben() { bean -dir "$B" "$@"; }
ana() { bean -dir "$A" "$@"; }
indented() { sed -n 's/^  //p' "$1" | sed -n "$2p"; }
R1=$(ben add lab.cholesterol-total 193 -unit mg/dL -at 2026-11-01)
W1=$(ben add weight 81.2 -unit kg -at 2026-11-02)
ben share -yes -seal -note "what do you think?" -out "$T/for-ana.bean" "$R1" "$W1" >"$T/share.txt" 2>"$T/err" || die "share failed"
SW=$(indented "$T/share.txt" 1)
# reply <out> <id> <note>...: Ana answers the share. Sets RW, the six words.
reply() { out=$1; shift; echo "$SW" | ana reply -to "$T/for-ana.bean" -out "$out" "$@" >"$T/reply.txt" 2>"$T/err" || die "reply failed"; RW=$(indented "$T/reply.txt" 1); }
# opened <file> <folder>: the Python reader opens a sealed reply.
opened() { rm -rf "$2"; echo "$RW" | "$PY" -I "$HERE/unseal.py" "$1" "$2" >"$T/err" 2>&1 || die "unseal.py did not open $1"; }

rc=0; n=0
# both <name> <folder> <want: ok|bad>. bean says that a folder is a reply
# only when its own check of a reply passes.
both() {
  n=$((n + 1)); want=0; [ "$3" = ok ] || want=1
  bean check-bundle "$2" >"$T/go.txt" 2>&1 </dev/null; g=1; grep -q '^This is a reply to a share' "$T/go.txt" && g=0
  "$PY" -I "$HERE/checkreply.py" "$2" >"$T/py.txt" 2>&1; p=$?
  if [ "$g" != "$want" ] || [ "$p" != "$want" ]; then
    echo "DIFFER $1: want $3, bean $g, checkreply.py exit $p"; sed 's/^/    go: /' "$T/go.txt" | tail -3; sed 's/^/    py: /' "$T/py.txt" | tail -3; rc=1
  fi
}
copy() { rm -rf "$T/d"; cp -R "$1" "$T/d"; }
of() { grep -l "\"type\":\"$2\"" "$1"/objects/*/*.json | head -1; }
into() { f="$T/d/objects/$(basename "$(dirname "$1")")"; mkdir -p "$f"; cp "$1" "$f/"; }

reply "$T/reply.bean" "$R1" "Looks fine to me." "$W1" "Steady."
IW=$(indented "$T/reply.txt" 2)
opened "$T/reply.bean" "$T/r";              both "a reply with two notes" "$T/r" ok
[ "identity words: $IW" = "$(grep '^identity words: ' "$T/py.txt")" ] || { echo "DIFFER: the identity words differ: bean says $IW"; grep '^identity words: ' "$T/py.txt"; rc=1; }
# The person takes it: the reply that the Python checker passed is one that
# bean accept takes, with the same words.
echo "$RW" | ben accept -yes -label Ana -words "$IW" "$T/reply.bean" >"$T/accept.txt" 2>"$T/err" || { echo "DIFFER: bean accept refused the reply: $(cat "$T/err")"; rc=1; }
grep -q '^took 2 notes' "$T/accept.txt" || { echo "DIFFER: bean accept did not take two notes"; rc=1; }

# Damage that needs no key.
copy "$T/r"; rm "$(of "$T/d" note)";        both "a listed note is not there" "$T/d" bad
copy "$T/r"; echo "accept me" >"$T/d/README.txt"; both "a text beside the records" "$T/d" bad
copy "$T/r"; mkdir "$T/d/files"; echo x >"$T/d/files/x"; both "a file beside the records" "$T/d" bad
copy "$T/r"; touch "$T/d/.DS_Store" "$(dirname "$(of "$T/d" note)")/.hidden"; both "names that start with a dot" "$T/d" ok
copy "$T/r"; mkdir "$T/d/files";           both "an empty files folder" "$T/d" ok
copy "$T/r"; rm "$(of "$T/d" bean.genesis)"; both "the replier's genesis is not there" "$T/d" bad
copy "$T/r"; f=$(of "$T/d" note); sed 's/Looks fine/Looks bad!/; s/Steady\./Shaky!!/' "$f" >"$f.x"; mv "$f.x" "$f"; both "a note with changed text" "$T/d" bad
copy "$T/r"; into "$(of "$B" weight)";      both "a record of the person's own log added" "$T/d" bad
copy "$T/r"; into "$(of "$B" share)";       both "the share record added" "$T/d" bad
# A second reply of the same key: its note and its reply record do not
# belong to the first.
reply "$T/reply2.bean" "$W1" "A second thought."; opened "$T/reply2.bean" "$T/r2"
both "a second reply" "$T/r2" ok
copy "$T/r"; into "$(of "$T/r2" note)";     both "a note that the reply record does not list" "$T/d" bad
copy "$T/r"; into "$(of "$T/r2" reply)";    both "two reply records" "$T/d" bad
both "what bean share wrote, opened: not a reply" "$B" bad

# The replier changes their key. A new reply shows the change. The first
# reply with the change added is signed by a key that is no longer the last.
cp -R "$A" "$T/twin"
ana rotate -seal-all <"$A.words" >/dev/null 2>"$T/err" || die "rotate failed"
reply "$T/reply3.bean" "$R1" "After my change of key."; opened "$T/reply3.bean" "$T/r3"
both "a reply after a change of key" "$T/r3" ok
copy "$T/r"; for f in $(grep -l '"type":"bean\.' "$T/r3"/objects/*/*.json); do into "$f"; done
both "an older key's reply, with the change of key added" "$T/d" bad
copy "$T/r3"; rm "$(of "$T/d" bean.succession)"; both "a reply whose change of key is not there" "$T/d" bad
# The same identity changes its key on a second machine too: it is frozen.
bean -dir "$T/twin" rotate -seal-all <"$A.words" >/dev/null 2>"$T/err" || die "rotate of the twin failed"
copy "$T/r3"; for f in $(grep -l '"type":"bean\.' "$T/twin"/objects/*/*.json); do into "$f"; done
both "a reply of an identity that changed its key twice from one place" "$T/d" bad

[ "$rc" = 0 ] && echo "replies: bean and checkreply.py agree on $n replies, good and damaged, and bean accept takes the one that both pass"
exit $rc
