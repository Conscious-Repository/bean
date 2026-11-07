# Security

This page says what bean protects, what it does not protect, and where each key lives. [SPEC.md](SPEC.md) sections 15 and 16 have the full analysis.

## What bean protects

- **Authorship and integrity.** Every record is signed and named by the hash of its content. If one byte changes, `bean verify` reports it. A record removed from a copy shows as `missing` when a checkpoint covers it. `bean` adds checkpoints as you write.
- **Your identity against a stolen device.** A device holds your signing key, not your 24 words. A thief with the device can add records in your name until you rotate. The thief cannot take over your identity or stop your rotation: only the 24 words can change the key (`bean rotate` or `bean restore`).
- **Your records against a thief's deletions.** A rotation lists every record that the old key signed since its last checkpoint, and every deletion and correction. Nothing stays in effect until you choose it. Use `-seal-all` only if the key was not stolen. Otherwise use `-seal-none` and `-seal <id>` for each record that is yours. What you do not keep stops being valid.
- **Your records against a retired key.** A key you replaced can still sign. Such a claim is `unattested`, and `bean ls` and the AI do not show it, unless you vouch for it with `bean checkpoint -accept <id>`.
- **Your records against your agents and strangers.** An agent signs with its own key. It can add records about you. It cannot correct, delete, sign as you, or hide a record of yours. Anyone can write a record that verifies, so `bean ls` and the AI show only your records and those of agents you paired. `bean ls -all` shows the rest, marked.
- **Your exit.** `bean export` writes everything as plaintext. No service is needed to read your data.

## What bean does not protect

- **Records are plain JSON on disk.** Anyone who can read the folder can read the types, times and values. Keep the log on an encrypted disk (FileVault on macOS). A machine that holds a copy, for example a cloud server for an agent, can read the whole log.
- **Stored files are encrypted with one key per identity.** If `local/identity-secret` leaks, anyone with the files can decrypt them. Rotation does not change this key. Make a new identity if it leaks.
- **The 24 words are the master key.** Whoever has them can take over your identity. If two people rotate from the same point, the identity freezes and you must make a new one.
- **If you lose the 24 words and the machine, you lose the identity.** Your records stay readable on any copy, but you cannot change keys again.
- **An AI client sees what it reads.** `bean mcp` sends the records it returns to the AI provider. Use a local model if that matters to you.
- **Times are claims.** `times.observed` and `times.imported` say what the author says. Nothing proves when a record was written. A thief with your current key can backdate a claim. It appears in the list at your next rotation unless a checkpoint of that key covers it, so read the list.
- **The newest records can vanish unnoticed.** A checkpoint shows that a covered record is gone. Someone who removes the newest checkpoint and what it covers leaves files that agree. Keep another copy.
- **A freeze has no way back.** If two rotations start from one point, for example after a restore from a stale backup, the identity freezes for good. `bean migrate` starts a new identity from it. Nothing links the two by signature.
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
