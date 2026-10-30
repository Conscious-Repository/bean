# Security

This page says what bean protects, what it does not protect, and where each key lives. [SPEC.md](SPEC.md) sections 15 and 16 have the full analysis.

## What bean protects

- **Authorship and integrity.** Every record is signed and named by the hash of its content. If one byte changes, `bean verify` reports it. A copy of the log cannot be changed without detection.
- **Your identity against a stolen device.** A device holds your signing key, not your 24 words. A thief with the device can add records in your name. The thief cannot take over your identity: only the 24 words can change the key (`bean rotate` or `bean restore`).
- **Your records against a thief's writes.** When you rotate, the records that your copy already has stay valid: the rotation seals them. Any record that the old key made elsewhere, made after the rotation, or that you exclude with `-no-seal`, stops being valid. A thief's records and deletions die this way. A copy that still has the old key refuses to write (`retired key`).
- **Your records against your agents.** An agent signs with its own key. It can add records about you. It cannot correct, delete or sign as you.
- **Your exit.** `bean export` writes everything as plaintext. No service is needed to read your data.

## What bean does not protect

- **Records are plain JSON on disk.** Anyone who can read the folder can read the types, times and values. Keep the log on an encrypted disk (FileVault on macOS). A machine that holds a copy, for example a cloud server for an agent, can read the whole log.
- **Stored files are encrypted with one key per identity.** If `local/identity-secret` leaks, anyone with the files can decrypt them. Rotation does not change this key. Make a new identity if it leaks.
- **The 24 words are the master key.** Whoever has them can take over your identity. If two people rotate from the same point, the identity freezes and you must make a new one.
- **If you lose the 24 words and the machine, you lose the identity.** Your records stay readable on any copy, but you cannot change keys again.
- **An AI client sees what it reads.** `bean mcp` sends the records it returns to the AI provider. Use a local model if that matters to you.
- **Times are claims.** `times.observed` says when the author says something happened. A key that you rotated out can no longer sign valid records, but the head key can write any time.
- **The folder lock is local.** Commands on one folder take a lock on `local/lock`. Two machines that write to one network folder at the same time are not protected; sync between separate folders instead. On Windows the lock does nothing.
- **gc erases for good.** `gc` refuses while the log has records from a newer bean or the identity is frozen, because validity can still change then.
- **Equal files look equal.** Encryption is deterministic, so someone with the files can see that two of them hold the same content.

## Keys

| Key | Where | What it can do |
|---|---|---|
| The 24 words | On paper. Never on a device. | Change your signing key; recover your identity. |
| Signing key | `local/signing-key`, mode 0600 | Add, correct and delete your records. |
| Identity secret | `local/identity-secret`, mode 0600 | Decrypt stored files; make dedupe refs. |
| Agent key | `local/agent-key`, one per machine | Add records about you, as the agent. |

`local/` never syncs. A backup of `local/` is as sensitive as the keys in it. You do not need one: the 24 words rebuild `local/` (`bean restore`).

## Report a problem

Use GitHub's private vulnerability reporting on this repository (Security, then Report a vulnerability). Do not open a public issue for a security problem.
