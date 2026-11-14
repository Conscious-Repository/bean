# Upgrading

Read this before you run a new build of `bean` on a log that you already have. It lists the changes that alter what an existing log shows. Records, ids and signatures never change. A new build reads the same files and can judge them differently.

## Check a new build on copies of your log

This is the one way to see what a new build changes in your log. Each build gets its own copy, because commands hide and restore files (SPEC.md 7.6). Save this as `bean-check.sh`. Set `log` to your folder and `repo` to your checkout of the bean source. Set `ref` to the build to try (`origin/main`, or a branch such as `origin/round3`). Then run `bash bean-check.sh`.

```
set -euo pipefail
log=~/health; repo=~/src/bean; ref=origin/main
d=$(mktemp -d "$HOME/bean-check.XXXXXX")      # new, empty, mode 0700
trap 'rm -rf "$d"' EXIT                         # the copies hold your keys
git -C "$repo" fetch -q origin
mkdir "$d/src"
git -C "$repo" archive "$ref" | tar -x -C "$d/src"
(cd "$d/src" && go build -o "$d/bean-new" ./cmd/bean)
cp "$(command -v bean)" "$d/bean-old"          # the build you use today
cp -R "$log" "$d/old"
cp -R "$log" "$d/new"
for b in old new; do
  rc=0; "$d/bean-$b" -dir "$d/$b" verify > "$d/$b-verify.txt" || rc=$?
  echo "exit $rc" >> "$d/$b-verify.txt"
  "$d/bean-$b" -dir "$d/$b" ls -all -json | sort > "$d/$b-records.txt"
done
same=yes
diff "$d/old-verify.txt" "$d/new-verify.txt" || same=no
diff "$d/old-records.txt" "$d/new-records.txt" || same=no
echo "same: $same"
```

The script stops with an error if a step fails, for example the fetch or the build. Then it has checked nothing.
The first `diff` shows new errors, warnings and exit codes.
The second shows each record that only one build shows. A line with `<` is valid only under the old build. A line with `>` is valid only under the new build.
`same: yes` means the new build shows your log exactly as the old one does.
The script deletes its folder when it ends.
Do not run `purge` or `gc` on the copies. Only they erase anything.

## Changes

### 0.6.0

- `purge` no longer writes the import ref of the deleted record into the tombstone. Anyone with your identity secret could test guesses of the deleted reading against it. Only `local/purged-refs.jsonl` keeps the ref. Back that file up. Without it, a new import of the same source data brings the reading back. Tombstones from the build of 2026-10-30 keep `context.purged_ref`. Nothing reads it.
- `bean sync -max-mb` is gone. Sync only from your own copies.
- A rotation can no longer name a current or earlier signing key as its next recovery key. Nor can it name a recovery key already on the chain (`key reuse` or `recovery not advanced`, SPEC 5.8). bean never writes such a rotation. If another tool did, that rotation and the rotations after it stop counting. The key that it would have retired stays the head key, so its later records become valid. The records of the new key become invalid (`key not in chain`).
- `bean version` says 0.6.0. `export` writes `"spec": "bean-v0"` in `manifest.json`. Nothing reads that string.
- On an agent-only copy, `verify` fails with `own identity missing` when the person's genesis is gone.
- `gc` erases a target whose first tombstone became invalid while another valid tombstone still deletes it. It waits 7 days from that other tombstone's hold.
- A record of a too-new identity gets no `hidden pending gc` warning.

### Compare by date (2026-11-14)

- The table of `bean compare` starts each row with the date the sample was taken, as `bean ls` shows it. The rows are in order of that date, then of type. A script that read the table by column, or by a line that starts with `lab.`, reads it one column off. Saved comparisons do not change.

### Compare and share (2026-11-10)

- `sync` refuses a folder that is not a copy of your log. Read a bundle that someone sent with `check-bundle`.
- A correction or deletion of a reading deletes the saved comparisons of that reading. `purge`, `add -corrects`, `append`, `import`, `sync`, `import-export` and `verify` do this, and say how many.
- `verify` and `gc` refuse extra arguments with exit 2. Before, they ignored them.
- `bean-import-lab` 0.5.0 reads Quest Diagnostics reports.

### Rules N1 to N4 (2026-10-30)

Four rules changed. bean never wrote a record that the new rules reject. Most effects make records invalid, but some make records valid. A record of an unknown version that is not in canonical form no longer halts its identity. The records and tombstones of that identity count again. A deletion with a small-order signature is now invalid, so its target comes back. The check above shows both kinds.

- N1: one time syntax (SPEC 3.5). A time is `YYYY-MM-DDTHH:MM:SS`, an optional dot and 1 to 9 digits, then `Z` or `±HH:MM`, with real ranges. A comma, the offset `+24:00`, the year 0000 and more than 9 fraction digits are now invalid. `bean add -at`, `bean mcp` and `bean-import-csv` take only this form. The CSV importer keeps other time text in `context.raw_time`.
- N2: Ed25519 as libsodium checks it (SPEC 4.4, 4.5). A key that is not the canonical encoding of a curve point, or that has small order, is `bad key`. A signature whose R has small order is `bad signature`. Random keys never hit these cases. The object cache of an older build is dropped once.
- N3: the canonical check comes before the version check (SPEC 3.8 steps 13 and 14). A file of a newer `v` that is not in canonical form is `not canonical`. It does not sync and cannot halt an identity.
- N4: a signing key is never a recovery key (SPEC 5.8). A genesis whose `value.key` is its own recovery key is `key reuse`. So is a succession whose `value.key` is the key that signs it or its own next recovery key.

### Too new and output (2026-10-30)

- `verify` exits 3, and `gc` refuses, only when your own identity has records from a newer bean. Another too-new identity, for example a stranger's genesis of a newer format, only prints `too new: <id>`.
- `reindex` and `local/cache/index.json` are gone. Nothing read the file. You can delete it.
- Verify output changed, but no record changed between valid and invalid:
  - a correction with missing targets prints one `held` line, and none when it is invalid;
  - a retired key's deletion that the rotation did not seal is `not sealed`, also when the identity is frozen;
  - a too-new identity gets no other lines;
  - a tombstone with blobs names `blobs`;
  - a record of an `hlog.*` core type says only `old format`.

### Lab importer and onboarding (2026-11-09)

- `bean import labcorp` is now `bean import lab`, and the binary is `bean-import-lab` (adapter `bean-import-lab@0.4.0`). Readings keep `source.system` `labcorp`, so an import made before dedupes with one made now.
- `bean import` shows what it read and asks before it saves. In a script, pass `-yes`. Without a terminal and without `-yes` it writes nothing.
- `bean mcp` offers `read_blob` only with `-blobs`, and `log_entry` refuses lab results. `bean restore -export` rebuilds a log from an export.

### Seventh audit (2026-11-07)

Reads change only on frozen logs and for a reading whose correction is hidden; no record changes between valid and invalid (DECISIONS 203 to 224).

- `purge` says when a file stays: `the file <id> is kept: N other records name it`, and a second line for deleted records that wait for `gc`, records about someone else and damaged files. It can now purge a record of a newer format.
- `purge` and `gc` remove a sync tool's conflict copy of the deleted record (`name (conflicted copy).json`, `.sync-conflict-…`). A file under such a name never keeps a blob.
- `export` lists `withheld_records` (damaged files a deletion names, left out) and `missing_blobs_of_others` (a count). A record of a newer format unlocks a file only when the right key signed it. That key is the key of the chain head, or a recovery key the chain commits to.
- `import-export` reads `manifest.json` as a stream: at most 1 MiB per JSON token, nesting at most 16.
- `bean import` exits 128 plus the signal number when stopped, and stops the importer's whole process group. The Labcorp importer reads a report by a strict grammar (0.3.0). It refuses lines it does not know, including tabs and non-ASCII. It also refuses a name that may hold its value, and a value it does not know. Try it on your own report first.
- A key paired under two names, from two of your machines, answers to either name.

- On a frozen log, a deletion signed by a key below the fork no longer hides every record it names. A record that no freeze rule puts in doubt, such as an agent's reading, stays visible.
- A reading whose only correction is hidden shows again, with a status that says why.
- `sync` fails with `no log in <src>` when the source holds no log, and exits 1 with `skipped N bad record files, M bad blobs` when the source is damaged. It judges deletions on both copies together, and it repairs a damaged file even when its size and time are unchanged.
- `export` writes record files that fail their checks to `invalid/`, and files no record names to `blobs/` (`unreferenced_blobs`). It decrypts a file only for a record of yours or of a paired agent. A file that only a stranger's record names is listed in `not_exported_blobs`.
- `import-export` needs `manifest.json`, refuses an export of another identity, and refuses records you purged (`refused N purged`). Hand-made export folders need a manifest.
- A `changed:` line names the input's type, and says why when `-corrections` cannot write it either.
- One agent key per machine has one label. `mcp -write -label B` or `agent pair -label B` on a key paired as A is refused; use the same label or none. `mcp -write` without `-label` keeps the key's name.
- `blob put` refuses a file over 256 MiB. A bad time in `ls -since`, `ls -until` or `add -at` exits 2. `ls` shows a kept file as `file <media type> <hex>`, not `series`.
- Importers: Labcorp 0.2.0, Apple Health 0.2.0 and CSV 0.6.0 name the kept file by digest (`report_sha256`, `export_sha256`, `file_sha256`), not in `blobs`. So purging the file record erases the file. Readings from older versions still name the file in `blobs`, so the file stays until they are purged too. Labcorp refuses a name that runs into the result column, rows before the header, and a duplicate test that differs in comment or lab. Apple Health keeps exports with correlations, and stores percentages as fractions with unit `1`. `bean import csv` works.
- `bean import` checks the importer's first record and your deletions before it stores the file, so a file you purged is not stored again.

### Re-audit fixes (2026-11-07)

No validity rule changes: `bean verify` prints the same lines for the same files. Commands change:

- `rotate` and `restore` need `-seal-all` once the old key has signed anything. Without it they show what the old key signed and stop. A wrong `-no-seal` id is now an error. Update your scripts.
- An import with a new value for a reading you already have now says so and exits 1. It used to skip it in silence. `append -corrections` writes it.
- `ls`, `show`, `export` and the MCP tools exit 3 for a too-new identity instead of showing nothing. For a frozen identity they warn and exit 4.
- `export` writes everything it can, also past a missing stored file, and records of a newer format.
- For a frozen identity, reads show what both sides kept, and `ls -all` marks the rest. New: `bean migrate`, for a frozen identity only.

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
