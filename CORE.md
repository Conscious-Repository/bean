# bean core

This document gives the rules that two independent implementations must agree on. [SPEC.md](SPEC.md) (version 0.5) gives all rules, the error strings and the conformance tests. If the two documents are different, SPEC.md is correct. This is a draft. Section 6 tells when it freezes.

## 0. Summary

A health log is a folder of signed claims about one person. Programs add files to the folder. Programs do not change files. To sync two copies, copy the files that each copy does not have. A person is an identity. The identity stays the same when the keys change. The validity of each record depends only on the set of files. It does not depend on a server, a clock or the order of the files.

## 1. Records

### 1.1 Storage

1. A record is one JSON object.
2. Store the record as its RFC 8785 canonical bytes. Do not add a byte order mark or a final newline.
3. The path is `objects/<h0h1>/<h2…h63>.json`. `h` is the 64 hex digits after `sha256:` in the record id.
4. Before you parse a file, its size must be 16 MiB or less. After you know the type, the size must be 1 MiB or less, but a succession can be 16 MiB.
5. Every number is an IEEE 754 double that converts to the same canonical text and back. A number whose canonical form has no `.` and no `e` is an integer. An integer must be in the range ±(2^53−1). Thus `9.007199254740992e15` is invalid and `1e21` is valid.
6. Strings are UTF-8. A string must not contain an escaped lone surrogate.
7. An object must not contain the same key two times.
8. Keep unknown fields. Include them in the hash. Do not interpret them.
9. All hex digits in ids, keys and signatures are lowercase.

### 1.2 Fields

| Field | Meaning |
|---|---|
| `v` | Format version. This draft uses `0`. |
| `nonce` | 32 lowercase hex digits. Make a new random value for each record. |
| `type` | Matches `^[a-z0-9][a-z0-9_-]*(\.[a-z0-9_-]+)*$`. 128 characters or less. Types that start with `bean.` are reserved. |
| `subject` | The identity id that the claim is about. Only a genesis has no `subject`. |
| `author` | An identity id, or a tagged key used directly. A key used directly is a bare key, for example the key of a device or an agent. |
| `kid` | The tagged public key that made `sig`. |
| `value` | Optional object. The usual forms are `{"n", "unit"}`, `{"s"}`, `{"b"}` and `{"blob"}`. |
| `times` | `imported` is required, in the form `YYYY-MM-DDTHH:MM:SSZ`. `observed` is null, or `{"start", "end", "precision"}` in RFC 3339. `end` is optional. |
| `source` | `system`, `adapter` (`name@version`) and `ref` (a dedupe ref, or null). |
| `refs` | Optional list of `{"rel", "id"}`. `rel` is `succeeds`, `supersedes`, `purges` or `about`. Any other `rel` makes the record invalid. |
| `blobs` | Optional list of `{"hash", "role", "media_type"}`. If `value.blob` is present, it must also be in this list. |
| `context` | Optional object. The verifier does not interpret it. |
| `id` | `sha256:` and the hex SHA-256 of the canonical bytes of the object without `id` and `sig`. It must agree with the path. |
| `sig` | A tagged signature. |

### 1.3 Keys and signatures

A tagged key is `<alg>:<lowercase hex>`. The signed message comes from the 32 bytes of the hash in the record id.

| alg | Public key | Signature |
|---|---|---|
| `ed25519` | 32 bytes. | `ed25519:<128 hex>`. RFC 8032 over the 32 bytes. Verify without the cofactor and reject a non-canonical S, as Go `crypto/ed25519.Verify` does. |
| `p256` | 33-byte compressed point. For hardware keys in devices. | `p256:<128 hex>`, which is r and s. ECDSA with SHA-256 of the 32 bytes. s must be n/2 or less. |
| `secp256k1` | 32-byte BIP-340 x-only key. This is a Nostr public key. | `secp256k1:<created_at>:<128 hex>`. BIP-340 over the SHA-256 of the wrapper event (below). |

The wrapper event lets any Nostr signer sign a record:

```
[0,"<public key hex>",<created_at>,29944,[["alt","bean record signature"]],"<record id>"]
```

1. Serialize the wrapper as NIP-01 does: compact JSON. All fields are ASCII, so this is the same as the RFC 8785 form.
2. The SHA-256 of these bytes is a Nostr event id. Sign it with BIP-340.
3. `created_at` is a decimal integer from 0 to 2^53−1, with no leading zeros. The verifier does not interpret it.

### 1.4 Order of checks

Check each file in this order. Stop at the first failure.

1. Path, size, JSON, duplicate keys, numbers and strings.
2. `v`, `type`, `author` and `kid` are present.
3. The id, the key and the signature.
4. The version. The earlier checks use only the envelope fields. These fields do not change between versions. An unknown version has two cases:
   - The type starts with `bean.` and a key of that identity signed the record. The identity is too new. Do not evaluate its records.
   - Any other type. Skip the record, but sync it.
5. The canonical bytes.
6. The fields.

## 2. Identity

### 2.1 Identity records

| Type | Rules |
|---|---|
| `bean.genesis` | No `subject` and no `refs`. `author`, `kid` and `value.key` are the same key. `value.recovery` is the commitment to recovery key 0. The id of the genesis is the identity id. It does not change. |
| `bean.succession` | `subject` and `author` are the identity id. One `succeeds` ref names the parent. `value` is `{"key", "recovery", "sealed"}`: the new signing key, the commitment to recovery key g+1, and a sorted list of ids. Recovery key g signs it. g is the position of the parent on the chain. The genesis has position 0. |

### 2.2 Paper seed and derived keys

1. The paper seed is 32 random bytes. Show it as 24 BIP-39 words. Do not use the BIP-39 PBKDF2 step or a passphrase.
2. Derive keys with HKDF-SHA-256 and an empty salt. The `info` labels are:
   - From the paper seed: `bean/v0/identity-secret` (32 bytes) and `bean/v0/recovery-key/<g>` (the 32-byte seed of an Ed25519 key).
   - From the identity secret: `bean/v0/blob-key` (64 bytes) and `bean/v0/dedupe-key` (32 bytes).
3. A commitment is `sha256:` and the hex SHA-256 of the ASCII text of a tagged recovery key.
4. Devices do not store the paper seed. Only the paper seed can change the signing key.

### 2.3 Chain and freeze

1. Start at the genesis. At each node, go to its one child. The last node is the head.
2. If a node has two children, that is a fork, and the identity is frozen. In a frozen identity, these records are invalid:
   - every succession below the fork,
   - every correction and deletion,
   - every claim that a key signed if the key is not on the path from the genesis to the fork.
3. A succession must not use a key that is already on its path. It must change `recovery`.

## 3. Authority

| Action | Who | Key |
|---|---|---|
| Add a claim | Any author. Readers decide whom to trust. | A key on the chain of the author, or a bare key with `kid` equal to `author`. |
| Correct (a record with a `supersedes` ref) | The author of the target. The target must not be an `bean.*` record. | The head key, or an older key on the chain whose next succession lists the record in `sealed`. |
| Delete (an `bean.tombstone` with one `purges` ref and `author` equal to `subject`) | The subject of the target. The target must not be a genesis, a succession or a tombstone. | The same as for a correction. |
| Rotate | The identity. | The recovery key to which the head commits. |

More rules:

1. A recovery key signs only successions.
2. If two or more valid records correct the same target, readers show the one with the lowest id.
3. Valid records that have the same `subject`, `source.system` and `source.ref` are duplicates. Do not count corrected records. The record with the lowest id is the primary.
4. A claim is not valid if the set has no genesis for its author.

## 4. Folder and sync

### 4.1 Sync

1. `objects/` and `blobs/` sync. `local/` holds keys, deletion state and caches. It does not sync.
2. To sync from copy A to copy B, copy each file that B does not have, if it passes the checks up to the signature.
3. Do not copy a record that a valid tombstone names.
4. The copy that wrote the tombstone erases the target at once. Any other copy that has the target hides it. It erases the target only when the person runs `gc`, 7 days or more later.
5. If a tombstone becomes invalid, for example after a rotation that does not seal it, restore the target.
6. You can also copy files with rsync, git or a USB drive. Then run `verify` on each copy.

### 4.2 Blobs

1. A blob is a raw source file, for example a PDF.
2. Encrypt it with AES-SIV (RFC 5297, AES-256) with the 64-byte blob key and the associated data `bean/v0/blob`. Do not use a nonce.
3. Store the bytes `V‖C` at `blobs/<2 hex>/<62 hex>`. The name is the SHA-256 of these bytes.
4. The encryption is deterministic, so the same file gives the same blob on every copy.

### 4.3 Dedupe refs

Dedupe refs let two copies import the same export without duplicates.

1. `source.ref` is `hmac-sha256:` and the hex HMAC-SHA-256, with the dedupe key, of the canonical bytes of one array.
2. If the source gives an id for the reading, the array is `["bean/v0/ref","id",<system>,<native id>,<occurrence>]`. The native id is a JSON string. Write a numeric id in decimal.
3. If the source gives no id, the array is `["bean/v0/ref","tuple",<system>,<type>,<observed>,<value>,<occurrence>]`. `<observed>` and `<value>` are the `times.observed` and `value` of the record. `<observed>` can be null.
4. `<occurrence>` starts at 0. It counts readings in one import whose arrays, without the occurrence, are the same. Count in source order.

The error strings, the warning strings and the conformance tests are in SPEC.md sections 12.2 and 13.

## 5. Not in the core

These parts are not in the core: indexes and caches, the CLI, the MCP server, importers, the type vocabulary, agents, computed results and device capture. An agent is a bare key that writes claims, and the subject writes an `agent.pair` claim for it. That is a convention, not a core rule. These parts can change without a new `v`. A change that makes an existing record valid or invalid needs a new `v`.

## 6. Decisions and freeze

1. Nostr keys sign through a wrapper event of kind 29944. This kind is not in the NIPs registry. It is in the NIP-01 ephemeral range, so relays do not store it. A NIP-46 signer can give permission for this kind only (`sign_event:29944`). Then a client that logs health records cannot sign other events for the person. The NIP-31 `alt` tag tells the signer what the event is. `created_at` is in `sig` because NIP-46 does not say that a signer keeps the value from the client.
2. Records are not Nostr events. Medical records must not go to relays by default. Records also have structure and device keys that a Nostr event cannot hold. A Nostr key can sign a record. A later extension can send records over Nostr, encrypted, for sharing.
3. Only the paper seed can change the signing key, and a fork freezes the identity. A custodian can keep the daily signing key while the person keeps the 24 words. To leave the custodian, the person rotates one time. The history stays, and nothing is signed again. A fork means that two parties have the seed. There is no rule to choose a branch, so a thief cannot win.
4. The format freezes after use, not after review. The owner runs a real log for two weeks, with lab results, Apple Health data and records from an AI assistant. If nothing in this document must change, `v` becomes `1` and every DEFAULT in SPEC.md becomes final. Reviews can change the format until then.
