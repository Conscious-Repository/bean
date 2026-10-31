# Upgrading

Read this before you run a new build of `bean` on a log that you already have. It lists the changes that alter what an existing log shows. Records, ids and signatures never change: a new build reads the same files and can judge them differently.

## Try a new build on a copy first

`verify` and the other commands hide and restore files (SPEC.md 7.6), so try a new build on copies:

```
cp -R ~/health /tmp/bean-old && cp -R ~/health /tmp/bean-new
bean -dir /tmp/bean-old verify > /tmp/old.txt
/path/to/new/bean -dir /tmp/bean-new verify > /tmp/new.txt
diff /tmp/old.txt /tmp/new.txt
```

Only `purge` and `gc` erase anything. Delete the two copies when you are done: they hold your keys.

## Changes

### Narrowed rules N1 to N4 (this release)

Four rules got stricter. bean never wrote a record that breaks them, so a log that only bean wrote does not change. Check yours before you upgrade. This only reads the folder:

```
go run ./tools/narrowscan ~/health
```

It prints the path and the rule of each record that would become invalid, and `scanned <n> files, 0 hits` when nothing changes.

- N1: one time syntax (SPEC 3.5). A time is `YYYY-MM-DDTHH:MM:SS`, an optional dot and 1 to 9 digits, then `Z` or `±HH:MM`, with real ranges. A comma, an offset of `+24:00`, the year 0000 and more than 9 fraction digits are now invalid. `bean add -at`, `bean mcp` and `bean-import-csv` take only this form; the CSV importer keeps any other time text in `context.raw_time`.
- N2: Ed25519 as libsodium checks it (SPEC 4.4, 4.5). A key that is not the canonical encoding of a curve point, or that has small order, is `bad key`. A signature whose R has small order is `bad signature`. Random keys never hit these cases. The object cache of an older build is dropped once.
- N3: the canonical check comes before the version check (SPEC 3.8 steps 13 and 14). A file of a newer `v` that is not in canonical form is now `not canonical`: it is not synced and cannot halt an identity.
- N4: a signing key is never a recovery key (SPEC 5.8). A genesis whose `value.key` is its own recovery key, and a succession whose `value.key` is the key that signs it or its own next recovery key, are `key reuse`.

### Too new, purged refs and output (the release before)

- `verify` exits 3, and `gc` refuses, only when your own identity has records from a newer bean. Another identity that is too new, for example a stranger's genesis of a newer format, only prints `too new: <id>`.
- `purge` writes the import ref of the deleted record into the tombstone (`context.purged_ref`), so a re-import on any copy skips it, also after `local/` is lost. A purge made with an older build keeps the ref only in `local/purged-refs.jsonl` of the copy that purged it. Keep that file when you move to a new machine, or a re-import there brings those records back.
- `bean sync` refuses a source with more than 2 GiB of record files. Pass `-max-mb` to raise the limit.
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
