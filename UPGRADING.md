# Upgrading

Read this before you run a new build of `bean` on a log that you already have. It lists the changes that alter what an existing log shows. Records, ids and signatures never change: a new build reads the same files and can judge them differently.

## Check a new build on copies of your log

This is the one way to see what a new build changes in your log. `verify` and the other commands hide and restore files (SPEC.md 7.6), so each build gets its own copy. Set `log` to your folder and `repo` to your checkout of the bean source:

```
log=~/health; repo=~/src/bean
d=$(mktemp -d "$HOME/bean-check.XXXXXX")     # new and empty each run, mode 0700
git -C "$repo" fetch -q origin
mkdir "$d/src" && git -C "$repo" archive origin/main | tar -x -C "$d/src"
(cd "$d/src" && go build -o "$d/bean-new" ./cmd/bean)
cp "$(command -v bean)" "$d/bean-old"         # the build you use today
cp -R "$log" "$d/old" && cp -R "$log" "$d/new"
for b in old new; do
  "$d/bean-$b" -dir "$d/$b" verify > "$d/$b-verify.txt"; echo "exit $?" >> "$d/$b-verify.txt"
  "$d/bean-$b" -dir "$d/$b" ls -all -json | sort > "$d/$b-records.txt"
done
diff "$d/old-verify.txt" "$d/new-verify.txt"
diff "$d/old-records.txt" "$d/new-records.txt"
```

The first `diff` shows new errors, warnings and exit codes. The second shows every record that is shown in one build and not in the other: lines with `<` are valid only under the old build, lines with `>` are valid only under the new one (for example a tombstone that now applies, or a record that a deletion no longer hides). No output from both means the new build shows your log exactly as the old one does. Delete the folder when you are done, because the copies hold your keys: `rm -rf "$d"`.

Only `purge` and `gc` erase anything, so do not run them on the copies to test.

## Changes

### This release

- `purge` no longer writes the import ref of the deleted record into the tombstone. The ref was a fingerprint of the deleted reading: anyone with your identity secret could test guesses against it. The ref is kept only in `local/purged-refs.jsonl` of the copy that purged. Back that file up; without it, a new import of the same source data brings the reading back. Tombstones written by the build of 2026-10-30 that carry `context.purged_ref` stay as they are; nothing reads that field now.
- `bean sync -max-mb` is gone. `bean sync` has no size limit; sync only from your own copies.
- A rotation can no longer name a current or earlier signing key, or a recovery key already committed on the chain, as its next recovery key (`key reuse` or `recovery not advanced`, SPEC 5.8). bean never writes such a rotation.
- On an agent-only replica, `verify` fails with `own identity missing` when the person's genesis is gone, as on the person's own copy.
- `gc` erases a target whose hold's tombstone became invalid while another valid tombstone still deletes it.
- A record of a too-new identity no longer gets a `hidden pending gc` warning.

### Rules N1 to N4 (2026-10-30)

Four rules changed. bean never wrote a record that the new rules reject. Most effects make records invalid, but some make records valid: a record of an unknown version that is not in canonical form no longer halts its identity, so that identity's records, and its tombstones, count again; and a deletion with a small-order signature is now invalid, so its target comes back. Use the check above to see both kinds.

- N1: one time syntax (SPEC 3.5). A time is `YYYY-MM-DDTHH:MM:SS`, an optional dot and 1 to 9 digits, then `Z` or `±HH:MM`, with real ranges. A comma, an offset of `+24:00`, the year 0000 and more than 9 fraction digits are now invalid. `bean add -at`, `bean mcp` and `bean-import-csv` take only this form; the CSV importer keeps any other time text in `context.raw_time`.
- N2: Ed25519 as libsodium checks it (SPEC 4.4, 4.5). A key that is not the canonical encoding of a curve point, or that has small order, is `bad key`. A signature whose R has small order is `bad signature`. Random keys never hit these cases. The object cache of an older build is dropped once.
- N3: the canonical check comes before the version check (SPEC 3.8 steps 13 and 14). A file of a newer `v` that is not in canonical form is now `not canonical`: it is not synced and cannot halt an identity.
- N4: a signing key is never a recovery key (SPEC 5.8). A genesis whose `value.key` is its own recovery key, and a succession whose `value.key` is the key that signs it or its own next recovery key, are `key reuse`.

### Too new, purged refs and output (2026-10-30)

- `verify` exits 3, and `gc` refuses, only when your own identity has records from a newer bean. Another identity that is too new, for example a stranger's genesis of a newer format, only prints `too new: <id>`.
- `purge` wrote the import ref of the deleted record into the tombstone, and `bean sync` had `-max-mb`. Both are gone again (this release).
- `reindex` and `local/cache/index.json` are gone. Nothing read the file. You can delete it.
- Verify output: a correction with missing targets prints one `held` line, and none when it is invalid; a retired key's deletion that the rotation did not seal is `not sealed` also when the identity is frozen; an identity that is too new gets no other lines; a tombstone with blobs names `blobs`; a record of an `hlog.*` core type says `old format` only. No record changes between valid and invalid.

### Change notes C25 to C29 and K1 (builds from 2026-10-30)

If you never ran `rotate` or `restore` on your log, nothing changes for you.

- C25: erasure follows record files, not validity. `purge` and `gc` erase a blob only when they erase a record that names it and no other record file names it.
- C26: a retired key writes nothing valid. A succession seals every record of the outgoing key. A record of an older key that the next succession does not seal is `not sealed`. On a log that was rotated with an earlier build, the records of the old key were not sealed, so they are now `not sealed`.
- C27: duplicate groups include the author. A record of one key is never a duplicate of a record of another key.
- C28: seal continuations. A rotation that must seal more than about 200,000 records writes `bean.seal` parts. Builds before C28 cannot read such a rotation.
- K1: only the head key, or a recovery key that the chain commits to, can make an identity too new. Other records of a newer format are `ungated`.
- C29: `rotate` and `restore` go past a halt by the head key.

### The rename to bean (2026-10-28)

A log from before the rename (types `hlog.*`) fails with `old format`. Make a new log with `bean init`.
