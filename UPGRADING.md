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

### Too new, purged refs and output (this release)

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
