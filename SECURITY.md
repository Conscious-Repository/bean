# Security

This page says what bean protects, what it does not protect, and where each key lives. [SPEC.md](SPEC.md) sections 15 and 16 have the full analysis.

## What bean protects

- **Authorship and integrity.** Every record is signed and named by the hash of its content. If one byte changes, `bean verify` reports it. Removing a whole record is different: see below.
- **Your identity against a stolen device.** A device holds your signing key, not your 24 words. A thief with the device can add records in your name. The thief cannot take over your identity: only the 24 words can change the key (`bean rotate` or `bean restore`).
- **Your records against a thief's writes.** A rotation shows what the old key signed in this folder, and keeps nothing until you decide. `-seal-all` keeps everything; use it only if the key was not stolen. `-no-seal <id>` or `-no-seal-file <file>` refuses records. Any record that the old key made elsewhere, made after the rotation, or that you refuse, stops being valid. **A seal cannot be undone, and you cannot undo a deletion you keep, so read the list.** A copy that still has the old key refuses to write (`retired key`).
- **Your records against your agents.** An agent signs with its own key. It can add records about you. It cannot correct, delete or sign as you.
- **Your exit.** `bean export` writes everything as plaintext. No service is needed to read your data.

## What bean does not protect

- **Records are plain JSON on disk.** Anyone who can read the folder can read the types, times and values. Keep the log on an encrypted disk (FileVault on macOS). A machine that holds a copy, for example a cloud server for an agent, can read the whole log.
- **Stored files are encrypted with one key per identity.** If `local/identity-secret` leaks, anyone with the files can decrypt them. Rotation does not change this key. Make a new identity if it leaks.
- **Removing a record is not detected.** Someone who can delete files in a copy can remove a record, and `bean verify` still exits 0. Deleting the newest succession from a copy also makes the retired key current again on that copy. Keep other copies: a sync brings the files back.
- **The 24 words are the master key.** Whoever has them can take over your identity. If two people rotate from the same point, the identity freezes for good. `bean ls` then warns and shows what both sides kept, and `bean migrate` starts a new identity with your own records, even if a thief then halted the log. With a lost laptop, `bean restore` and the 24 words still give a frozen copy what it needs for that. Nothing links the two by signature.
- **If you lose the 24 words and the machine, you lose the identity.** Your records stay readable on any copy, but you cannot change keys again.
- **An AI client sees what it reads.** `bean mcp` sends the records it returns to the AI provider. Use a local model if that matters to you.
- **Times are claims.** `times.observed` says when the author says something happened. A key that you rotated out can no longer sign valid records, but the head key can write any time.
- **The folder lock works on one machine only, and not on Windows.** Give each machine its own folder, never one network or synced folder (SMB, NFS, iCloud Drive, Dropbox): two `rotate` or `restore` runs there can fork and freeze the identity. On Windows, run one `bean` command at a time per folder, and stop `bean mcp` before `rotate`, `restore`, `purge` or `gc`. SPEC.md 7.11 has the details.
- **Sync only from your own copies.** `bean sync` copies every record file of the source, and every later command reads them. A stranger's copy can fill your disk or make every command slow.
- **Back up `local/purged-refs.jsonl`.** A purge erases the record and writes a tombstone that names only its id. The file is the only place that remembers which import the record came from. If you lose it, or import the same source data on a copy that never had the record, the deleted data comes back as a new record. The tombstone does not carry this ref on purpose: it syncs everywhere, and the ref would let anyone with your identity secret test guesses of what you deleted.
- **gc erases for good.** `gc` refuses while your identity has records from a newer bean or is frozen, or a seal set is incomplete, because validity can still change then.
- **A thief can flood the old key.** Every record of the old key in the folder is sealed by default, so a thief who writes millions of records makes `rotate` slow, and above 200,200,000 records `rotate` refuses (`too large`). Rotate from a copy that does not hold the thief's records, for example a backup from before the theft: records that are not in the folder are not sealed, so they stay invalid.
- **A thief with your current key can halt the log until you rotate.** The thief can write one record in a made-up newer format. Every copy that syncs it then says `too new`, and `ls`, `append`, `gc` and the agent writes refuse. `bean export` still writes every file. No data is lost. `bean rotate` (or `bean restore` on a new machine) with the 24 words goes past the halt: it prints `past halt <id>`, and after the rotation the record is `ungated` on every copy. A key that you rotated out, or a stranger's key, cannot halt the log at all.
- **A missing seal part fails closed.** Someone who can delete files in a copy can delete a `bean.seal` part. Then every record of the retired key in that copy is not valid until the part comes back from another copy. Nothing is erased, and no one without the 24 words can add or replace a part.
- **Equal files look equal.** Encryption is deterministic, so someone with the files can see that two of them hold the same content.

## Keys

| Key | Where | What it can do |
|---|---|---|
| The 24 words | On paper. Never on a device. | Change your signing key; recover your identity. |
| Signing key | `local/signing-key`, mode 0600 | Add, correct and delete your records. |
| Identity secret | `local/identity-secret`, mode 0600 | Decrypt stored files; make dedupe refs. |
| Agent key | `local/agent-key`, one per machine | Add records about you, as the agent. |

`local/` never syncs. A backup of `local/` is as sensitive as the keys in it. The 24 words rebuild the keys (`bean restore`). They do not rebuild `local/purged-refs.jsonl`, so back up that file (above).

## Report a problem

Use GitHub's private vulnerability reporting on this repository (Security, then Report a vulnerability). Do not open a public issue for a security problem.
