# hlog core, in two pages

What two independent implementations must agree on, and nothing else. [SPEC.md](SPEC.md) (v0.5) is the full text with error strings and conformance tests; where they differ, SPEC.md wins and this page has a bug. Draft, not frozen.

**The idea.** A health log is a folder of signed claims about one person. Nobody edits a file; programs only add. Sync is copying the files the other copy lacks. A person is an identity that outlives their keys. Validity is a pure function of the set of files: no server, no clock, no order.

## 1. A record

One JSON object, stored as its RFC 8785 canonical bytes (no BOM, no final newline) at `objects/<h0h1>/<h2…h63>.json`, where `h` is the 64 hex digits after `sha256:` in its id. At most 16 MiB before parsing, and 1 MiB once the type is known unless it is a succession. Every number is an IEEE double that round-trips; a number whose canonical form has no `.` and no `e` is an integer and must lie within ±(2^53−1), so `9.007199254740992e15` is rejected and `1e21` is not. Strings are UTF-8 with no lone surrogates. Duplicate keys are invalid. Unknown fields are kept, hashed and ignored. All hex, in ids, keys and signatures, is lowercase.

| Field | Meaning |
|---|---|
| `v` | Format version, `0` in this draft. |
| `nonce` | 32 lowercase hex digits, random per record. |
| `type` | `^[a-z0-9][a-z0-9_-]*(\.[a-z0-9_-]+)*$`, at most 128 characters. `hlog.*` is reserved. |
| `subject` | Identity id the claim is about. Absent only on a genesis. |
| `author` | Identity id, or a tagged key used directly (a *bare key*: a device or an agent). |
| `kid` | Tagged public key that made `sig`. |
| `value` | Optional object. Conventions: `{"n", "unit"}`, `{"s"}`, `{"b"}`, `{"blob"}`. |
| `times` | `imported` (`YYYY-MM-DDTHH:MM:SSZ`, required) and `observed` (null, or `{"start", "end"?, "precision"}`, RFC 3339). |
| `source` | `system`, `adapter` (`name@version`), `ref` (dedupe ref or null). |
| `refs` | Optional `[{"rel", "id"}]`. `rel` is `succeeds`, `supersedes`, `purges` or `about`; anything else is invalid. |
| `blobs` | Optional `[{"hash", "role", "media_type"}]`. A `value.blob` must be listed here. |
| `context` | Optional, free form, never interpreted. |
| `id` | `sha256:` + hex SHA-256 of the canonical bytes of the object without `id` and `sig`. Must equal the path. |
| `sig` | Tagged signature over the 32 raw bytes of the id hash. |

**Keys and signatures.** A tagged key is `<alg>:<lowercase hex>`.

| alg | Public key | Signature |
|---|---|---|
| `ed25519` | 32 bytes | `ed25519:<128 hex>`, RFC 8032 over the 32 id bytes, verified cofactorless with non-canonical S rejected (as Go's `crypto/ed25519.Verify`). |
| `p256` | 33-byte compressed point | `p256:<128 hex>` = r‖s, ECDSA with SHA-256 of the 32 id bytes, s ≤ n/2. For hardware device keys. |
| `secp256k1` | 32-byte BIP-340 x-only key (a Nostr key) | `secp256k1:<created_at>:<128 hex>`, BIP-340 over SHA-256 of the wrapper `[0,"<pubkey hex>",<created_at>,29944,[],"<record id>"]` in NIP-01 serialization (compact JSON; every field is ASCII, so it equals the JCS form). That hash is a Nostr event id, so any Nostr signer can sign. `created_at` is a decimal integer from 0 to 2^53−1 with no leading zeros, meaningless to the verifier. |

**Check order.** Path, size, JSON, duplicate keys, numbers, strings; then `v`, `type`, `author` and `kid` are present; then the id, the key and the signature. These envelope fields never change between versions. Only then the version: an unknown `v` on an `hlog.*` record signed by a key of that identity makes the identity *too new* (do not evaluate it); otherwise the record is skipped and still synced. Then canonical bytes, then fields.

## 2. Identity

| Type | Rules |
|---|---|
| `hlog.genesis` | No `subject`, no `refs`. `author` = `kid` = `value.key`. `value.recovery` = commitment to recovery key 0. Its id *is* the identity id, forever. |
| `hlog.succession` | `subject` = `author` = identity. One `succeeds` ref to its parent. `value` = `{"key": new signing key, "recovery": commitment to generation g+1, "sealed": sorted ids}`. Signed by recovery key g, where g is the parent's position on the chain (genesis = 0). |

The **paper seed** is 32 random bytes, shown as 24 BIP-39 words (no PBKDF2, no passphrase). HKDF-SHA-256 with an empty salt derives, by `info` label: `hlog/v0/identity-secret` (32 bytes), `hlog/v0/recovery-key/<g>` (an Ed25519 seed), and from the identity secret `hlog/v0/blob-key` (64) and `hlog/v0/dedupe-key` (32). A **commitment** is `sha256:` + hex SHA-256 of the ASCII tagged recovery key. Devices never store the seed; only the seed can rotate.

**Chain.** From the genesis, follow the one child at each step. Two children of one node is a **fork**: the identity is frozen (every succession below the fork, every destructive record, and every claim by an off-trunk key is invalid). A succession may not reuse any key on its path, and must advance `recovery`.

## 3. Authority

| Action | Who | Key |
|---|---|---|
| Add a claim | Anyone. Readers decide trust. | Any chain key of the author, or a bare key (`kid` = `author`). |
| Correct: `supersedes` | The target's author. Not an `hlog.*` target. | Head key, or an older chain key sealed by the next succession. |
| Delete: `hlog.tombstone`, one `purges` ref, `author` = `subject` | The target's subject. Not a genesis, succession or tombstone. | Same as correct. |
| Rotate | The identity. | The recovery key the head commits to. |

A recovery key signs only successions. When several valid records supersede one target, readers show the lowest id. Valid records with the same `(subject, source.system, source.ref)`, ignoring superseded ones, are duplicates; the lowest id is primary. A claim whose author has no genesis in the set is unresolved, not valid.

## 4. Folder and sync

`objects/` and `blobs/` sync; `local/` (keys, holds, caches) never does. Sync from A to B copies every file B lacks that passes the checks through the signature, except the target of a valid tombstone. The copy that wrote the tombstone erases the target at once; any other copy that holds it hides it and erases it only on an explicit `gc` after a 7-day hold. A tombstone that later becomes invalid (an unsealed one after a rotation) restores its target. rsync, git or a USB drive are fine if every copy then runs `verify`.

**Blobs** are raw source files: AES-SIV (RFC 5297, AES-256, the 64-byte blob key, associated data `hlog/v0/blob`, no nonce), stored as `V‖C` at `blobs/<2>/<62>`, named by the SHA-256 of those bytes. Deterministic, so the same file gives the same blob on every copy.

**Dedupe refs** let two copies import the same export without duplicates: `source.ref` = `hmac-sha256:` + hex HMAC-SHA-256 with the dedupe key over the canonical array `["hlog/v0/ref","id",<system>,<native id>,<occurrence>]`, or `["hlog/v0/ref","tuple",<system>,<type>,<observed>,<value>,<occurrence>]` when the source has no native id. The native id is a JSON string (a numeric id in decimal); `<observed>` and `<value>` are the record's exact `times.observed` (possibly null) and `value`. `<occurrence>` starts at 0 and counts exactly identical readings, by their array without the occurrence, within one import, in source order.

Error and warning strings, and the conformance tests, are in SPEC.md 12.2 and 13.

## 5. Not core

Indexes and caches, the CLI, the MCP server, importers, the type vocabulary, agents (bare-key claims plus an `agent.pair` claim by the subject, a convention), computed results and device capture (SPEC.md extensions). Each can change without a new `v`. Anything that changes the validity of an existing record needs one.

## 6. Questions for Rob

1. **Nostr signing.** Is the wrapper `[0, pubkey, created_at, 29944, [], record id]` the right shape for NIP-46 and NIP-07 signers, and is kind 29944 (ephemeral, unassigned as far as we know) acceptable?
2. **Own record format.** Records are not NIP-01 events; a Nostr key can sign them. Keep it, or make each record an event?
3. **Seed-only rotation.** Only the paper seed can rotate; any fork freezes the identity. Too strict, or right for health data?
4. **Freeze.** What must change before `v` becomes `1` and this page stops moving?
