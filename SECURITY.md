# Security

This page says what bean protects, what it does not protect, and where each key lives. [SPEC.md](SPEC.md) sections 15 and 16 have the full analysis.

## What bean protects

- **Authorship and integrity.** Every record is signed and named by the hash of its content. If one byte changes, `bean verify` reports it. Removing a whole record is different: see below.
- **Your identity against a stolen device.** A device holds your signing key, not your 24 words. A thief with the device can add records in your name. The thief cannot take over your identity: only the 24 words can change the key (`bean rotate` or `bean restore`).
- **Your records against a thief's writes.** A rotation shows what the old key signed in this folder, and keeps nothing until you decide. `-seal-all` keeps everything; use it only if the key was not stolen. `-no-seal <id>` or `-no-seal-file <file>` refuses records. Any record that the old key made elsewhere, made after the rotation, or that you refuse, stops being valid. **A seal cannot be undone, and you cannot undo a deletion you keep, so read the list.** A copy that still has the old key refuses to write (`retired key`).
- **Your records against your agents.** An agent signs with its own key. It can add records about you. It cannot correct, delete or sign as you.
- **Your exit.** `bean export` writes everything as plaintext, also files that no record names and record files that fail their checks. No service is needed to read your data.
- **Your deletions against strangers' records.** Anyone who can read the folder can see a stored file's id, and a stranger's record can name it, so the file outlives a purge. `export` and the MCP `read_blob` decrypt a file only for a valid record of yours or of a paired agent. `sync` and `import-export` judge a deletion on both folders together. So an older copy or export cannot bring back what you purged. And a copy that has not seen a rotation cannot apply the old key's deletions.

## What bean does not protect

- **Records are plain JSON on disk.** Anyone who can read the folder can read the types, times and values. Keep the log on an encrypted disk (FileVault on macOS). A machine that holds a copy, for example a cloud server for an agent, can read the whole log.
- **Stored files are encrypted with one key per identity.** If `local/identity-secret` leaks, anyone with the files can decrypt them. Rotation does not change this key. Make a new identity if it leaks.
- **Removing a record is not detected.** Someone who can delete files in a copy can remove a record, and `bean verify` still exits 0. Deleting the newest succession from a copy also makes the retired key current again on that copy. Keep other copies: a sync brings the files back.
- **The 24 words are the master key.** Whoever has them can take over your identity. If two people rotate from the same point, the identity freezes for good. `bean ls` then warns and shows what both sides kept. A deletion signed by a key of either new branch hides nothing. `bean migrate` starts a new identity with your own records, even if a thief then halted the log. With a lost laptop, `bean restore` and the 24 words still give a frozen copy what it needs for that. Nothing links the two by signature.
- **If you lose the 24 words and the machine, you lose the identity.** Your records stay readable on any copy, but you cannot change keys again.
- **An AI client sees what it reads.** `bean mcp` sends the records it returns to the AI provider. It reads stored files, such as a kept report with your name and birth date, only with `-blobs`. It never writes lab results or other imported readings. Use a local model if that matters to you.
- **An AI assistant that runs commands sees their output.** Run `bean init`, `bean rotate` and `bean restore` yourself: they print or read your 24 words.
- **Times are claims.** `times.observed` says when the author says something happened. A key that you rotated out can no longer sign valid records, but the head key can write any time.
- **The folder lock works on one machine only, and not on Windows.** Give each machine its own folder. Never use one network or synced folder (SMB, NFS, iCloud Drive, Dropbox): two `rotate` or `restore` runs there can fork and freeze the identity. On Windows, run one `bean` command at a time per folder, and stop `bean mcp` before `rotate`, `restore`, `purge` or `gc`. SPEC.md 7.11 has the details.
- **Sync only from your own copies.** `bean sync` copies every record file of the source, and every later command reads them. A stranger's copy can fill your disk or make every command slow.
- **Back up `local/purged-refs.jsonl`.** A purge erases the record and writes a tombstone that names only its id. The file is the only place that remembers which import the record came from. The deleted data comes back as a new record if you lose it. The same happens if you import the same source data on a copy that never had the record. The tombstone does not carry this ref on purpose. The tombstone syncs everywhere, and the ref would let anyone with your identity secret test guesses of what you deleted.
- **gc erases for good.** `gc` refuses while your identity has records from a newer bean, is frozen, or has an incomplete seal set. Validity can still change then.
- **A thief can flood the old key.** Every record of the old key in the folder is sealed by default. So a thief who writes millions of records makes `rotate` slow. Above 200,200,000 records `rotate` refuses (`too large`). Rotate from a copy that does not hold the thief's records, for example a backup from before the theft. Records that are not in the folder are not sealed, so they stay invalid.
- **A thief with your current key can halt the log until you rotate.** The thief can write one record in a made-up newer format. Every copy that syncs it then says `too new`, and `ls`, `append`, `gc` and the agent writes refuse. `bean export` still writes every file. No data is lost. `bean rotate` (or `bean restore` on a new machine) with the 24 words goes past the halt and prints `past halt <id>`. After the rotation the record is `ungated` on every copy. A key that you rotated out, or a stranger's key, cannot halt the log at all.
- **A missing seal part fails closed.** Someone who can delete files in a copy can delete a `bean.seal` part. Then every record of the retired key in that copy is not valid until the part comes back from another copy. Nothing is erased, and no one without the 24 words can add or replace a part.
- **Equal files look equal.** Encryption is deterministic, so someone with the files can see that two of them hold the same content.

**A bundle is a disclosure.** A plain bundle is not encrypted. `bean share -seal` encrypts it with six words, which protects it on its way. A reader who has opened it holds a copy for good, and whoever learns both the file and the words can open it. Two bundles of yours can be linked by your identity id ([docs/SHARE.md](docs/SHARE.md)).

**A package that is sealed to your address opens for whatever runs as you.** `bean share -for` seals a package to a reader's address, and the reader's key file opens it. A program that runs as you, such as an assistant with a shell, opens such a package with no word typed. Then another person's values are in its context. The six words were also a gate: such a program does not have them. A stolen key file opens every package that was sealed to it and still exists anywhere. This bean does not open a key file that is protected with a passphrase ([docs/SHARE.md](docs/SHARE.md)).

**A saved comparison gives its reading away.** The record holds the cohort and the result. With the public files, anyone who has the record can work the value out.

**A page that checks a bundle is only as good as its sender.** A forged page can call anything good. Get `bean-check.html` from the project's [releases](https://github.com/Conscious-Repository/bean/releases), or build it with `make page`, and compare its SHA-256.

## Keys

| Key | Where | What it can do |
|---|---|---|
| The 24 words | On paper. Never on a device. | Change your signing key; recover your identity. |
| Signing key | `local/signing-key`, mode 0600 | Add, correct and delete your records. |
| Identity secret | `local/identity-secret`, mode 0600 | Decrypt stored files; make dedupe refs. |
| Agent key | `local/agent-key`, one per machine | Add records about you, as the agent. |
| A reader's key file | One file that you place, mode 0600. Not in a log. It has no paper copy. | Open every package that was sealed to its address. The address cannot be taken back. |

`local/` never syncs. A backup of `local/` is as sensitive as the keys in it. The 24 words rebuild the keys (`bean restore`). They do not rebuild `local/purged-refs.jsonl`, so back up that file (above). They do not rebuild a reader's key file either. `bean restore`, `bean export` and a copy of the log do not hold it.

## Report a problem

Use GitHub's private vulnerability reporting on this repository (Security, then Report a vulnerability). Do not open a public issue for a security problem.
