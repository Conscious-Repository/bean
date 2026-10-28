# hlog core specification v0.4, draft

Status: draft for review with Rob Long. This text is not frozen.
Date: 2026-10-28. v0.4 applies the change set recorded in the build notes (C1 to C20): seed-only rotation, a frozen envelope, verify-before-copy sync, and no erasure on a timer. The reference implementation is the `bean` repository.
Editor: Benjamin Anderson (AION Biosciences, steward).
License: CC0 1.0 (see section 19).
Name: "hlog" is a placeholder. The project will take a neutral name.

This specification uses Simplified Technical English (ASD-STE100 style).
The key words MUST, MUST NOT, REQUIRED, SHOULD, SHOULD NOT and MAY are used as in RFC 2119 and RFC 8174.
Text marked "DEFAULT, Rob to confirm" is a concrete decision that the review can change.
An implementation MUST follow a DEFAULT until the spec changes it.

## Contents

1. Scope and goals
2. Terms
3. Record format
4. Encoding and cryptography
5. Identity
6. Authority rules
7. Folder layout and sync
8. Versioning
9. Adapters
10. Extension 1: computed results
11. Extension 2: device capture
12. CLI reference
13. Conformance tests
14. Test vectors to publish
15. Security considerations
16. Privacy considerations
17. Known limits
18. Out of scope
19. Governance and license
- Appendix A: deltas from the reference code at /workspace/hlog
- Appendix B: open items

## 1. Scope and goals

### 1.1 Scope

This specification defines the hlog core.
The core is the smallest set of rules that two independent implementations must agree on.
The core has three parts:

1. Signed records in a folder.
2. An identity that outlives its keys.
3. Rules about who may add, correct, delete and rotate.

The core does not interpret data.
Computed results are Extension 1 (section 10).
Device capture is Extension 2 (section 11).

### 1.2 Definition

A health log is a folder of signed claims about one person.
The person owns the folder.
The person syncs the folder by copying files.
Nobody edits a record. Programs only add records.

### 1.3 Five ideas

1. A record is a claim, not a fact.
2. Each record has a fingerprint (the id, a hash) and a signature.
3. The log is a folder. Sync copies the files that a replica does not have. There is no server, no account and no conflict.
4. Programs never edit. Programs only add. A correction supersedes a record. A deletion is a tombstone that every copy obeys.
5. A person is an identity. The identity id never changes. Keys rotate. An offline paper seed recovers the identity.

### 1.4 Goals

- G1. Two conforming verifiers that hold the same set of files MUST get the same validity result for every record.
- G2. The verifier MUST NOT need a network, an account, a server or a trusted clock.
- G3. The person MUST be able to leave at any time with plaintext data (`hlog export`).
- G4. Any index MUST be a cache that a program can rebuild from the files alone.
- G5. The core MUST be source-agnostic. A device format is an adapter. Special code for one source in the core is a bug.

### 1.5 Customer zero and kill rule

Benjamin Anderson is customer zero.
He imports his own Oura, Whoop and Apple Health data.
Kill rule: stop the project if he does not use it every week by week 6.
This rule is a project rule, not a protocol rule.

## 2. Terms

Each term has one meaning in this document.
Other documents and code comments MUST use these terms with these meanings.

| Term | Meaning |
|---|---|
| log | A folder that holds records and blobs for one or more identities. |
| replica | One copy of a log on one machine. |
| record | One JSON object stored as one file in `objects/`. |
| claim | A record that is not a destructive record and not an identity record. |
| identity record | A record of type `hlog.genesis` or `hlog.succession`. |
| destructive record | A record that has a `supersedes` ref, or a record of type `hlog.tombstone`. |
| supersede record | A record that has one or more `supersedes` refs. |
| tombstone | A record of type `hlog.tombstone`. It has one `purges` ref. |
| target | The record that a `supersedes` ref or a `purges` ref names. |
| blob | One encrypted file in `blobs/` that holds raw evidence. |
| id | The string `sha256:` followed by 64 lowercase hex digits. It names a record or a blob. |
| tagged key | A public key string with an algorithm tag, for example `ed25519:<hex>`. |
| signing key | A key pair that signs records. Devices hold the private half. |
| recovery key | An Ed25519 key pair that the paper seed derives. It signs only successions. |
| commitment | The SHA-256 hash of a tagged recovery public key, as an id string. |
| paper seed | 32 random bytes, written on paper as 24 words. Devices never store it. |
| identity | A person or agent. Its identity id is the id of its genesis record. |
| identity secret | 32 bytes that the paper seed derives. It derives the blob key and the dedupe key. |
| genesis | The first record of an identity. Type `hlog.genesis`. |
| succession | A record that names the next signing key of an identity. Type `hlog.succession`. |
| node | A genesis or a succession, seen as a point in the succession tree. |
| succession tree | All well-signed nodes of one identity, linked by `succeeds` refs. |
| parent | The node that a succession names in its `succeeds` ref. |
| chain | The path of nodes from genesis that the verifier selects (section 6.3). |
| head | The last node of the chain. |
| head key | The `value.key` of the head. |
| chain key | The `value.key` of any node on the chain. |
| fork | Two or more well-signed successions that have the same parent. |
| fork point | The chain node with two or more children. Everything below it is invalid (`frozen`). |
| seal | To list a destructive record id in the `value.sealed` list of a succession. |
| frozen | The state of an identity after a recovery conflict (section 6.4). A verifier reports it as `compromised`. |
| bare key | A tagged key used directly as an author. A bare key never rotates. |
| author | The identity id or bare key that makes the claim. |
| subject | The identity id of the person that the record is about. |
| kid | The tagged key that made the signature. |
| valid | A record that passes every check in sections 3 and 6. |
| invalid | A record that fails a check. A verifier reports it with an error string. |
| held | A destructive record whose target is not in the replica. The verifier keeps it and checks it again when the target arrives. |
| too new | The state of an identity after the verifier finds an `hlog.*` record with an unknown `v`. |
| skip | To keep a file, sync it, and not interpret it. |
| hide | To stop showing, serving and syncing a target. The bytes stay on disk. |
| erase | To delete the bytes of a target from disk. |
| hold | The time from receipt of a valid tombstone to erasure. 7 days. |
| adapter | A separate program that reads one source format and writes records as JSONL. |
| native id | The id that a source system gives to one reading. |
| dedupe ref | The value of `source.ref`. A keyed hash (section 4.8). |
| occurrence index | A counter that separates exactly identical readings (section 4.8). |
| verifier | A program that checks records against this specification. |
| writer | A program that makes and signs records. |
| export | A plaintext copy of a log (section 7.9). |
| index | A cache that a program builds from the files. It is never the truth. |
| host | The machine that runs `hlog` and holds the identity secret. |
| device | A capture machine with its own key, for example a sensor (Extension 2). |

## 3. Record format

### 3.1 General

A record is one JSON object.
A writer MUST store a record as the RFC 8785 canonical bytes of the full object, with `id` and `sig` included (section 4.1).
The file has no byte order mark and no final newline.
The maximum file size is 1 MiB. DEFAULT, Rob to confirm.

### 3.2 Fields

| Field | JSON type | Required | Constraints |
|---|---|---|---|
| `v` | integer | yes | Record format version. This draft uses `0`. |
| `nonce` | string | yes | 32 lowercase hex digits (128 bits) from a cryptographic random source. |
| `type` | string | yes | 1 to 128 characters. Pattern `^[a-z0-9][a-z0-9_-]*(\.[a-z0-9_-]+)*$`. DEFAULT, Rob to confirm. |
| `subject` | string | yes, except genesis | An identity id. A genesis MUST NOT have `subject`. |
| `author` | string | yes | An identity id or a tagged public key (bare key). |
| `kid` | string | yes | The tagged public key that signed this record. |
| `value` | object | no | See 3.4. |
| `times` | object | yes | Keys `observed` and `imported`. See 3.5. |
| `source` | object | yes | Keys `system`, `adapter`, `ref`. See 3.6. |
| `refs` | array | no | Items `{"rel": string, "id": id}`. See 3.7. |
| `blobs` | array | no | Items `{"hash": id, "role": string, "media_type": string}`. |
| `context` | object | no | Free form. The core never interprets it. |
| `id` | string | yes | `sha256:` + hex SHA-256 of the canonical bytes without `id` and `sig`. |
| `sig` | string | yes | Tagged signature (section 4.5). |

A record MAY have other top-level fields.
A verifier MUST keep them and MUST include them in the hash.
A verifier MUST NOT interpret them.
The same rule applies to unknown keys inside `value`, `times`, `source`, the `refs` items and the `blobs` items.

There are no `assurance`, `kind`, `prev`, `quality`, `coding` or `times.recorded` fields.
Put coding (LOINC, SNOMED), quality flags, source timestamps and device details in `context`.
Suggested `context` keys: `device`, `site`, `conditions`, `coding`, `source_times`, `quality`, `raw_time`, `reason`.
These keys are conventions. The core does not check them.

### 3.3 Reserved types

| Type | Purpose | Extra constraints |
|---|---|---|
| `hlog.genesis` | Starts an identity. | No `subject`. `author` = `kid` = `value.key`. No `refs`. `value` = `{"key": tagged key, "recovery": commitment}`. `times.observed` = null. `source` = `{"system":"self","adapter":<name@version>,"ref":null}`. |
| `hlog.succession` | Names the next signing key. | `subject` = `author` = the identity id. Exactly one ref, rel `succeeds`. `value` = `{"key": tagged key, "recovery": commitment, "sealed": [ids]}`. |
| `hlog.tombstone` | Deletes a target. | `subject` = `author`. Exactly one ref, rel `purges`. No `value`. No `blobs`. An optional reason goes in `context.reason` (string, at most 200 characters). |

Any other type that starts with `hlog.` is reserved for future core versions.
A writer MUST NOT make one.
At `v` 0, a verifier MUST treat an unknown `hlog.*` type as invalid with error `unknown core type`.

### 3.4 value

`value` is an object.
For types outside `hlog.*`, these shapes are RECOMMENDED:

| Shape | Meaning |
|---|---|
| `{"n": number, "unit": string}` | A number. `unit` SHOULD be UCUM. |
| `{"s": string}` | A string. |
| `{"b": boolean}` | A boolean. |
| `{"blob": id, "unit": string}` | A series or a large payload in a blob. |

The core checks only one rule for `value` outside `hlog.*`.
If `value.blob` is present, it MUST be an id that is also a `hash` in `blobs`.
Otherwise the record is invalid with error `blob not listed`.
The core does not bless a series format. The method that reads the blob defines the format.

### 3.5 times

`times.imported` is REQUIRED and MUST NOT be null.
The writer sets it when the record enters the log.
Format: `YYYY-MM-DDTHH:MM:SSZ`, UTC, second precision.

`times.observed` is REQUIRED as a key. Its value is null or an interval.
An interval is `{"start": time, "end": time, "precision": p}`. `end` is optional.
`start` and `end` are RFC 3339 date-time strings with a numeric offset or `Z`.
Fractional seconds are allowed.
If `end` is present, `end` MUST NOT be earlier than `start`.
`p` is one of `year`, `month`, `day`, `hour`, `minute`, `second`, `millisecond`. DEFAULT, Rob to confirm.

A writer MUST set `times.observed` to null when the source does not give a time.
A writer MUST NOT guess or default a time.
A writer puts an unparseable source time in `context.raw_time` as a string.

### 3.6 source

`source.system` is a string of 1 to 64 characters, for example `oura` or `self`.
`source.adapter` is `name@version`, for example `hlog-import-oura@1.0.0`.
`source.ref` is a dedupe ref (section 4.8) or null.
`source.ref` is null for a record that the author makes directly, with no source file.
`source.ref` is null for a device-signed record (section 11).

### 3.7 refs

Each item is `{"rel": rel, "id": id}`.
The core knows four rel values:

| rel | Meaning | Core checks |
|---|---|---|
| `succeeds` | This succession follows the named node. | Section 6. |
| `supersedes` | This record corrects the target. | Section 6.6. |
| `purges` | This tombstone deletes the target. | Section 6.7. |
| `about` | This record comments on, attests or disputes the target. | None. |

A record with any other rel is invalid with error `unknown rel`.
Attestations, disputes and notes are ordinary claims with rel `about` and a type that says what they are.
`succeeds` is allowed only on `hlog.succession`.
`purges` is allowed only on `hlog.tombstone`.
A record MUST NOT have two refs with the same rel and id.

### 3.8 Validation order

The fields `v`, `type`, `author`, `kid`, `id` and `sig` are the envelope. Their meaning and constraints do not change between record versions. A verifier of any version checks the envelope, the id and the signature before it reads `v`.

A verifier MUST check each file in this order.
At the first failure, the verifier reports the error string and stops checking that file.

1. The path matches `objects/<2 hex>/<62 hex>.json`. Error: `bad path`.
2. The file size is at most 16 MiB. Error: `too large`.
3. The bytes are valid UTF-8 and parse as one JSON object. Error: `bad json`.
4. The object has no duplicate keys at any depth. Error: `duplicate key`.
5. Every number is an integer in the range -(2^53 - 1) to 2^53 - 1, or a finite double that canonicalizes (section 4.1). An integer is a number whose canonical form has no `.` and no `e`. Error: `bad number`.
6. No string holds an escaped lone surrogate. Error: `bad string`.
7. `v` is present and is a non-negative integer. Error: `bad v`.
8. `type` is present and is a string. If `type` is not `hlog.succession`, the file size is at most 1 MiB. Error: `bad type`, `too large`.
9. `author` and `kid` are present and are strings. Error: `bad field: author`, `bad field: kid`.
10. `id` is a string equal to the recomputed id and to the id from the path. Error: `id mismatch`.
11. `kid` parses as a supported tagged key. Error: `unsupported key algorithm` or `bad key`.
12. `sig` verifies for `kid` over the id bytes (section 4.5). Error: `bad signature`.
13. If `v` is not a version that this verifier knows, apply section 8 and stop.
14. The file bytes are equal to the canonical bytes of the parsed object. Error: `not canonical`.
15. All required fields are present with the correct JSON types and constraints in 3.2 to 3.7. Error: `bad field: <name>`.
16. Apply the authority rules in section 6.

Steps 1 to 15 depend only on the file.
Step 16 depends on the set of files.
A parser MAY refuse nesting deeper than 256 levels with `bad json`.

## 4. Encoding and cryptography

### 4.1 Canonical JSON (RFC 8785, JCS)

The canonical bytes of a JSON value are its RFC 8785 serialization.
These rules apply:

1. Sort object keys by their UTF-16 code units, in ascending order. Do not use UTF-8 byte order.
2. Write no whitespace.
3. Write strings with `"`. Escape `"` as `\"` and `\` as `\\`.
4. Escape U+0008, U+0009, U+000A, U+000C and U+000D as `\b`, `\t`, `\n`, `\f`, `\r`.
5. Escape other code points below U+0020 as `\u00xx` with lowercase hex.
6. Write all other code points as UTF-8. Do not escape `/`, U+007F or non-ASCII characters.
7. Write numbers with the ECMAScript `Number.prototype.toString` algorithm (RFC 8785 section 3.2.2.3).
8. Write `-0` as `0`.

Number rules for hlog:

- A parser MUST read every JSON number as an IEEE 754 double.
- A verifier MUST reject an integer outside -(2^53 - 1) to 2^53 - 1. Error: `bad number`.
- A verifier MUST reject a number that does not round-trip to the same double. Error: `bad number`.
- A writer MUST put a longer integer or an exact decimal in a string.
- `v` MUST be written as an integer with no fraction and no exponent.

Strings MUST be valid UTF-8.
A verifier MUST reject an escaped lone surrogate, for example `\ud800`. Error: `bad string`.

### 4.2 Record id

1. Make a copy of the record object.
2. Remove the `id` key and the `sig` key from the copy.
3. Compute the canonical bytes of the copy.
4. Compute SHA-256 of those bytes.
5. Write the id as `sha256:` followed by 64 lowercase hex digits.

### 4.3 Nonce

A writer MUST get 16 bytes from a cryptographic random source (in Go, `crypto/rand`).
A writer MUST write them as 32 lowercase hex digits.
A writer MUST make a new nonce for each record.

### 4.4 Tagged keys

A tagged key is `<alg>:<lowercase hex>`.

| alg | Public key bytes | Hex length | Status |
|---|---|---|---|
| `ed25519` | 32-byte RFC 8032 public key | 64 | Default. All writers and verifiers MUST support it. |
| `p256` | 33-byte SEC1 compressed point (prefix 0x02 or 0x03) | 66 | Allowed for hardware-backed device keys. Verifiers MUST support it. |
| `secp256k1` | Reserved for a future Nostr transport extension | none | Verifiers MUST reject it at `v` 0 with `unsupported key algorithm`. |

A verifier MUST reject uppercase hex, a wrong length or an invalid point. Error: `bad key`.
A verifier MUST reject any other tag. Error: `unsupported key algorithm`.

### 4.5 Signatures

The signed message is the 32 raw bytes of the record id hash.
Decode the 64 hex digits after `sha256:` to get these bytes.
The `sig` tag MUST be equal to the `kid` tag. Error: `bad signature`.

Ed25519:

- `sig` = `ed25519:` + 128 lowercase hex digits (the 64-byte RFC 8032 signature).
- Sign with RFC 8032 Ed25519 (pure, no prehash, no context).
- Verify as Go `crypto/ed25519.Verify` does. It rejects a non-canonical `S`. DEFAULT, Rob to confirm.

P-256:

- `sig` = `p256:` + 128 lowercase hex digits. This is `r || s`, each 32 bytes, big-endian (IEEE P1363 form).
- The ECDSA hash input is SHA-256 of the 32-byte message. Hardware signers hash the message, so this matches them.
- `s` MUST be at most n/2 (low-S). A signer MUST replace `s` with `n - s` when `s` is higher. A verifier MUST reject a high `s`.
- Software signers SHOULD use RFC 6979 deterministic nonces.
- Hardware signers MAY use random nonces. Most hardware keystores do not let the caller select RFC 6979. DEFAULT, Rob to confirm.

A signature is not part of the id.
Thus two valid files with the same id can have different `sig` bytes when a signer uses random nonces.
A replica MUST keep the first file it got for an id. A replica MUST NOT replace it during sync.

### 4.6 Recovery commitment

A commitment is `sha256:` + hex SHA-256 of the ASCII bytes of the tagged recovery public key string.
Example input: the 72 ASCII bytes of `ed25519:` followed by 64 hex digits.

### 4.7 Key derivation (HKDF)

All derivations use HKDF-SHA-256 (RFC 5869).
The salt is empty. RFC 5869 then uses 32 zero bytes.
The `info` value is the ASCII label below, with no terminator.

| Input key material | info label | Output length | Output use |
|---|---|---|---|
| paper seed (32 bytes) | `hlog/v0/identity-secret` | 32 | identity secret |
| paper seed (32 bytes) | `hlog/v0/recovery-key/<g>` | 32 | Ed25519 private seed of recovery key generation `g` |
| identity secret (32 bytes) | `hlog/v0/blob-key` | 64 | AES-SIV key (section 4.9) |
| identity secret (32 bytes) | `hlog/v0/dedupe-key` | 32 | HMAC key for dedupe refs (section 4.8) |

`<g>` is a decimal integer with no leading zeros, starting at `0`.
Generation 0 is committed in the genesis. Each recovery uses generation `g` and commits generation `g + 1` (section 5.5).

### 4.8 Dedupe ref

The dedupe ref is HMAC-SHA-256 with the dedupe key.
The HMAC input is the canonical bytes (4.1) of one JSON array.

If the source gives a native id:

    ["hlog/v0/ref", "id", <source.system>, <native id as string>, <occurrence>]

If the source gives no native id:

    ["hlog/v0/ref", "tuple", <source.system>, <type>, <times.observed>, <value>, <occurrence>]

Rules:

1. `<native id as string>` is the native id written as a JSON string. Write a numeric native id in decimal.
2. `<times.observed>` is the exact JSON value of `times.observed` in the record, or null.
3. `<value>` is the exact JSON object of `value` in the record.
4. `<occurrence>` is an integer that starts at 0.
5. In one adapter run, two readings are identical when their arrays, without the occurrence, have equal canonical bytes.
6. The first identical reading gets occurrence 0. The next gets 1, in source order.
7. Do not use the file hash. Do not use the row position.
8. An adapter MUST keep the HMAC input stable across adapter versions for the same source reading when the source has a native id. A tuple input changes when a parser fix changes the value; 9.7 handles that case.
9. Write `source.ref` as `hmac-sha256:` followed by 64 lowercase hex digits. DEFAULT, Rob to confirm.

Result: two exports that overlap make the same refs for the same readings. The second import adds nothing.

### 4.9 Blob encryption (AES-SIV)

A writer MUST encrypt every blob before it hashes and stores it.

1. Use AES-SIV as in RFC 5297 with AES-256. This is AES-SIV-CMAC-512.
2. Use the 64-byte blob key. The first 32 bytes are K1, the CMAC (S2V) key. The last 32 bytes are K2, the CTR key.
3. Use one associated data string: the ASCII bytes `hlog/v0/blob`. DEFAULT, Rob to confirm.
4. Use no nonce. The encryption is deterministic.
5. The blob bytes are `V || C`, with the 16-byte synthetic IV `V` first.
6. The blob id is `sha256:` + hex SHA-256 of the blob bytes.

Equal plaintext gives equal blob bytes and an equal blob id for one identity secret.
Go has no AES-SIV in its standard library.
An implementation MAY use a vetted AES-SIV library or build CMAC (RFC 4493), S2V and CTR from `crypto/aes` and `crypto/cipher`.
Either way it MUST pass the RFC 5297 Appendix A vectors. The reference implementation builds it and matches OpenSSL on vector V11.
Implementations MUST support blobs of at least 256 MiB. DEFAULT, Rob to confirm.

### 4.10 Paper seed encoding

The paper seed is 32 bytes from a cryptographic random source.
The paper form is 24 words from the BIP-39 English word list.
The 24 words encode the 256 bits and an 8-bit checksum, as BIP-39 defines.
The HKDF input is the 32 entropy bytes.
Do not use the BIP-39 PBKDF2 step. Do not use a passphrase.

Reasons for this choice:

- The checksum finds most copy errors before the seed is used.
- The word list is public, fixed and tested. Each word has unique first four letters.
- Words are easier to copy and read aloud than 64 hex digits.

Risk: a person can type the words into a crypto wallet by mistake.
The wallet then sees the seed.
The CLI MUST tell the person that the words are for hlog only.
DEFAULT, Rob to confirm.

## 5. Identity

### 5.1 Overview

An identity is the id of its genesis record. This id never changes.
The genesis names the first signing key and commits to recovery key generation 0.
Successions rotate the signing key.
The paper seed derives the recovery keys and the identity secret.
Devices hold the identity secret and the current signing key. Devices never hold the paper seed.

### 5.2 Create an identity (init)

1. Get a new paper seed (4.10).
2. Show the 24 words to the person once.
3. Derive the identity secret and recovery key generation 0 (4.7).
4. Make a new random signing key. Use Ed25519 unless the device uses a hardware P-256 key.
5. Write a genesis with `value.key` = the signing public key and `value.recovery` = the commitment to recovery generation 0.
6. Store the identity id, the identity secret and the signing private key in `local/`.
7. Erase the paper seed and the recovery private key from memory and disk.

The signing key is random. The paper seed does not derive it.
One host holds the signing key. A second device is paired as a bare device key (11.3). Copying the signing key between devices is not supported in v0.

### 5.3 Rotation

There is one kind of succession. It is signed by the recovery key that the parent commits to (5.5). A signing key cannot write a succession. Error: `bad succession`.

The person rotates when a device is lost, a key is old, or a device leaves. Rotation needs the paper seed. This is deliberate: a stolen device can add claims, but it can never take over the identity, because it cannot write a succession.

### 5.4 Sealed rotation

A destructive record signed by the head key is valid.
A destructive record signed by an older chain key is valid only when the succession that ends that key seals it.
The succession that ends a key is the next node on the chain after the node that names the key.
Only that succession can seal records of that key.
`hlog rotate` MUST print every destructive record of the outgoing key and seal all of them by default. `-no-seal <id>` excludes one.
The maximum file size of an `hlog.succession` is 16 MiB, so `sealed` can hold about 230,000 ids. DEFAULT.

`value.sealed` is an array of record ids.
The ids MUST be unique and sorted in ascending order of their ASCII bytes. Error: `bad field: value.sealed`.
An id in `sealed` that does not name a destructive record of the outgoing key has no effect.

Result: every replica with the same files gets the same answer. No trusted time is necessary.
A thief who steals a key cannot keep a destructive record after a rotation, unless the person seals it.

### 5.5 Write a succession with the paper seed

The person rotates when a device is lost, a key is stolen, or a device leaves. Sync all devices first. Destructive records that are not synced are not sealed and become invalid (5.4).

1. Enter the 24 words on one device.
2. The parent is the chain head (6.3). Its position on the chain, counting the genesis as 0, is the recovery generation `g`.
3. Check that the identity secret derived from the words equals `local/identity-secret`, and that the commitment of recovery generation `g` equals the parent's `value.recovery`. Error: `seed does not match`.
4. If the identity is frozen (5.7), stop. Error: `compromised: <id>`.
5. Make a new signing key.
6. Write a succession with:
   - `refs` = `[{"rel":"succeeds","id":<parent id>}]`.
   - `value.key` = the new tagged public key.
   - `value.recovery` = the commitment to recovery generation `g + 1`.
   - `value.sealed` = the sorted ids of the parent key's valid destructive records in this replica, minus any the person excludes.
7. Sign with recovery key generation `g`. The `kid` is the recovery public key. This reveals the key.
8. Store the new signing key in `local/`.
9. Erase the paper seed and the recovery private keys.

The identity secret does not change. See 17 for this limit.

### 5.6 Forks

A fork is two or more well-signed successions with the same parent.
Every succession is recovery-signed, so a fork means the same recovery key signed twice on different branches.
A fork freezes the identity (5.7). There is no tie-break rule.

A stolen signing key cannot fork an identity, because it cannot write a succession.

### 5.7 Freeze

Two successions with the same parent freeze the identity.
The same recovery key signed twice on different branches. This means the paper seed is in two sets of hands, or the person rotated on two devices without syncing.

For a frozen identity, a verifier MUST:

1. Report `compromised: <identity id>`.
2. Treat every succession below the fork point as invalid. Error: `frozen`.
3. Treat every destructive record of the identity as invalid. Error: `frozen`.
4. Treat every claim signed by a key that is not on the path from genesis to the fork point as invalid. Error: `frozen`.
5. Restore any target that a now-invalid tombstone hides (7.6).

A frozen identity cannot rotate. The person MUST make a new identity.
DEFAULT, Rob to confirm (items 2 to 4).

### 5.8 Key rules

- A succession MUST NOT reuse a key in `value.key`. Error: `key reuse`. A key is reused when it is the signing key or recovery key of a node on the path from genesis to the parent.
- A succession MUST name a `value.recovery` that differs from the parent commitment. Error: `recovery not advanced`.
- A recovery key signs only successions. A record of any other type signed by a recovery key is invalid. Error: `key not in chain`.

### 5.9 Identity secret

The identity secret is 32 bytes. The paper seed derives it (4.7).
It never appears in the log.
It derives the blob key and the dedupe key.
Each device of the person holds it in `local/identity-secret`.
If the identity secret leaks, the person MUST make a new identity and a new log, and re-encrypt all data into it.

## 6. Authority rules

### 6.1 Who may do what

| Action | Who | Key |
|---|---|---|
| Add a claim | Anyone. Readers decide trust. | Any chain key of the author, or a bare key. |
| Correct (supersede) | The original author only. | The head key, or an older chain key with a seal. |
| Delete (tombstone) | The subject only. | The head key, or an older chain key with a seal. |
| Rotate | The identity. | The recovery key that the chain head commits to (the paper seed). |

The subject can comment on, attest or dispute a claim of another author.
The subject uses a new claim with rel `about` to do this.
The subject cannot supersede a claim of another author.

### 6.2 Input

The verifier gets the set F of record files in `objects/` plus the hidden records of this replica (7.6).
The result depends only on F.
The result does not depend on file order, file times, receipt times or the clock.
Local hold state changes only which bytes are on disk (7.6). It never changes validity.

### 6.3 Algorithm

The verifier MUST compute the same result as these steps.

Step 1. Check files.
Apply steps 1 to 15 of section 3.8 to each file.
Keep the files that pass. Call this set R.
Apply section 8 to files with an unknown `v`.

Step 2. Find identities.
For each genesis G in R: G passed its signature check with `kid` = `value.key`.
The identity id is the id of G.
If an identity is too new (8.2), do not process it. Report `too new: <identity id>`.

Step 3. Build the succession tree of each identity I.
Start with node set N = {G}.
Repeat until no node is added:

1. For each succession S in R with `subject` = `author` = I, and S not in N:
2. Let P be the node that the `succeeds` ref of S names. If P is not in N, wait for the next pass.
3. S is valid when the commitment of `S.kid` (4.6) = `P.value.recovery`. Otherwise S is invalid. Error: `bad succession`.
4. Apply the key rules in 5.8. If one fails, S is invalid with that error.
5. Otherwise add S to N.

A succession that never gets a parent in N stays held. Report `held: <id>` as a warning.

Step 4. (Merged into step 5.)

Step 5. Select the chain and detect a freeze.
Set node X = G. Repeat:

1. Let C be the set of nodes in N whose parent is X.
2. If C is empty, X is the head. Stop.
3. If C has one node, set X to that node.
4. If C has two or more nodes, the identity is frozen (5.7) and X is the fork point. Stop. X is the head.

The chain is the list of nodes from G to the head.
Each chain node has a position. G has position 0.
Every node in N that is not on the chain is below a fork point and is invalid. Error: `frozen`.

Step 6. Resolve the signing key of each other record.
For each record D in R that is not an identity record:

1. If `D.author` is a tagged key, `D.kid` MUST equal `D.author`. Otherwise `D` is invalid. Error: `bad signature`. A bare key has no chain. Treat it as always the head key.
2. If `D.author` is an identity id with no genesis in R, D is unresolved. Report `unresolved author: <id>` as a warning. Do not treat D as valid.
3. If `D.kid` is the `value.key` of a chain node at position p, record p for D.
4. Else if `D.kid` is the `value.key` of a node below a fork point, D is invalid. Error: `frozen`.
5. Else D is invalid. Error: `key not in chain`.

Step 7. Check seals for destructive records.
For each destructive record D with an identity author and position p:

1. If the node at p is the head, D passes this step.
2. Else let Y be the chain node at position p + 1.
3. If `D.id` is in `Y.value.sealed`, D passes this step.
4. Else D is invalid. Error: `not sealed`.

Destructive records with a bare-key author pass this step.

Step 8. Apply the frozen rules in 5.7 to each frozen identity.

Step 9. Check supersede records (6.6).

Step 10. Check tombstones (6.7).

Step 11. Check subjects.
For each claim D whose `subject` has no genesis in R, report `unresolved subject: <id>` as a warning.
D stays valid. The subject check is a warning because a lab can write about a person whose genesis the replica does not hold.

Step 12. Find duplicates.
Group valid records by (`subject`, `source.system`, `source.ref`) where `source.ref` is not null.
In each group, the record with the lowest id is the primary.
Report the others as `duplicate: <id> of <primary id>`.
Duplicates stay valid. Indexes and readers SHOULD ignore them.

### 6.4 Validity is order-free

Each step uses only R and the results of earlier steps.
No step uses time or arrival order.
Two verifiers with the same F MUST report the same valid set, the same errors and the same warnings.

### 6.5 Claims from rotated-out keys

A claim signed by any chain key is valid, even after rotation.
Thus a rotated-out key can still sign a claim with an old `times.observed`.
See 17.

### 6.6 Supersede rules

For each supersede record D, for each `supersedes` target T:

1. If T is not in R, D is held for this target. D is valid. Report `held: <D id>` as a warning.
2. If T is an `hlog.*` record, D is invalid. Error: `bad target`.
3. If `T.author` is not equal to `D.author`, D is invalid. Error: `not author`.
4. Otherwise the target is superseded. Readers SHOULD show D in place of T. T stays in the log.
5. When two or more valid supersede records name the same target and none supersedes another, readers SHOULD show the one with the lowest id. The verifier reports the others as `superseded by sibling: <lowest id>` (a warning). DEFAULT.
6. A tombstone of a supersede record removes the supersede. The target is shown again unless another valid supersede names it.

A supersede record can name more than one target. All targets MUST pass.

### 6.7 Tombstone rules

For each tombstone D with target T:

1. `D.author` MUST equal `D.subject`. Otherwise D is invalid. Error: `not subject`.
2. If T is not in R, D is held. D is valid. Section 7.5 tells how sync uses a held tombstone.
3. If T is an `hlog.genesis`, `hlog.succession` or `hlog.tombstone`, D is invalid. Error: `bad target`.
4. If `T.subject` is not equal to `D.subject`, D is invalid. Error: `not subject`.
5. Otherwise D is a valid tombstone for T. T is purged.

A purged target is not valid data. A reader MUST NOT show it.
A verifier reports a purged target that is hidden and past its hold as `hidden pending gc: <id>` (a warning). `hlog gc` erases it (7.6).

## 7. Folder layout and sync

### 7.1 Paths

| Path | Content | Synced |
|---|---|---|
| `objects/<h0h1>/<h2..h63>.json` | One record. `h` is the 64 hex digits of the id. | yes |
| `blobs/<h0h1>/<h2..h63>` | One blob (`V || C`). No file extension. | yes |
| `local/identity` | The identity id, ASCII, one line. | never |
| `local/identity-secret` | 64 lowercase hex digits, one line. File mode 0600. | never |
| `local/signing-key` | `<alg>:<hex private key>`, one line. File mode 0600. For Ed25519, the hex is the 32-byte seed. For P-256 hardware keys, it is a key handle. | never |
| `local/holds.json` | Hold state (7.6). | never |
| `local/hidden/<64 hex>.json` | Hidden records (7.6). | never |
| `local/purged-refs.jsonl` | Purged dedupe refs (7.8). | never |
| `local/cache/` | Indexes and other caches. | never |

Directories use mode 0700. Writers MUST write each file to a temporary name in the same directory and then rename it.
A sync tool MUST NOT copy `local/`.
Any other path in the log is not part of the log. A verifier ignores it.

### 7.2 Write a record

1. Build the record object without `id` and `sig`.
2. Compute the id (4.2).
3. Sign the id (4.5).
4. Add `id` and `sig`.
5. Write the canonical bytes to the path of the id.
6. If a file with that path exists, do not replace it.

### 7.3 Write a blob

1. Encrypt the plaintext (4.9).
2. Compute the blob id.
3. Write the bytes to the path of the blob id. If the file exists, do not replace it.
4. Write the blob before the record that names it.

### 7.4 Sync

Sync from replica A (source) to replica B (destination) is a set union with one exception.

1. Compute the valid set of A and of B (section 6).
2. Collect the valid tombstones of A and of B.
3. For each record file in A that B does not have:
   1. If the file fails steps 1 to 12 of 3.8 (through the signature), do not copy it.
   2. If a valid tombstone of A or B names it, apply 7.5. Do not copy it.
   3. If B hides it under a hold, do not copy it.
   4. Otherwise copy it.
   5. If B holds a file for that id that fails step 12 of 3.8 and the file in A passes, replace B's file.
4. For each blob in A that B does not have:
   1. If the SHA-256 of the bytes does not match the path, do not copy it.
   2. If only purged or hidden records name it, do not copy it.
   3. Otherwise copy it.
5. Compute the valid set of B again. Apply 7.6 to new tombstones.

Sync MUST copy files with an unknown `v`. Newer verifiers need them.
Sync MUST copy invalid records that pass steps 1 to 12 of 3.8. Validity is a verifier result, not a sync rule. A replica keeps the first file that passes the signature check for an id.
A succession and a tombstone it invalidates can travel in one sync; the target then comes back on the next sync (step 5). Two passes converge.
Sync with rsync, git, Syncthing or a USB drive is allowed if the person then runs `hlog verify` on each replica. Such tools do not apply the tombstone exception. `hlog verify` then hides or erases the target (7.6).

### 7.5 Held tombstones

A tombstone can arrive before its target.
Then the tombstone is held (6.7 step 2).
When a target file arrives in a sync, check the tombstone with the target (6.7 steps 3 and 4) before the copy.

- If the tombstone is valid for the target, do not copy the target. Add the target dedupe ref to `local/purged-refs.jsonl` (7.8).
- If the tombstone is invalid for the target, copy the target. The tombstone becomes invalid.

Result: a tombstone from a stranger cannot block a record of another subject.

### 7.6 Delete hold

When a replica first finds a valid tombstone T for a target that it stores:

1. If this replica wrote T (`hlog purge`), erase the target at once.
2. Otherwise hide the target at once: move it to `local/hidden/`. Hidden records are still input to the local verifier (6.2) and never sync.
3. Record the receipt time in `local/holds.json` as `{"<T id>": {"target": "<target id>", "received": "<UTC time>"}}`.
4. A replica MUST NOT erase a hidden target on a timer.
5. `hlog gc` erases every target hidden for at least 7 days under a still-valid tombstone, and every blob that no valid, unpurged record names.
6. `hlog verify` reports each hidden target as `hidden pending gc: <id>` (a warning).

When T becomes invalid while the target is hidden, restore the target.
T can become invalid after a rotation that does not seal T, or a freeze.
Restore means: move the target back to `objects/` and remove the hold entry.
After erasure, the replica cannot restore the bytes. Another replica that still holds them can sync them back, because T is now invalid.

Any command that loads the log (`verify`, `append`, `sync`, `export`, `reindex`, `rotate`, `purge`, `gc`) MUST apply this section.
The hold length is 7 days. DEFAULT, Rob to confirm. Nothing is erased without the person running `gc` or `purge`.

### 7.7 Purge

`hlog purge <id>` does these steps:

1. Check that the local identity is the subject of the target.
2. Write a tombstone signed by the head key.
3. Add the target dedupe ref to `local/purged-refs.jsonl`.
4. Erase the target at once.
5. Erase each blob that no valid, unpurged record names.

The tombstone stays in the log forever.
A program MUST NOT tombstone a genesis, a succession or a tombstone.

### 7.8 Purged refs

`local/purged-refs.jsonl` has one line per purged target.
Each line is the canonical bytes of `["<subject>", "<source.system>", "<source.ref>"]`.
A replica writes a line when it erases or does not copy a purged target that has a non-null `source.ref`.
`hlog append` MUST skip a record whose (`subject`, `source.system`, `source.ref`) is in this file.
Result: a re-import after a purge adds nothing on this replica.

### 7.9 Export

`hlog export <dir>` writes a plaintext copy of the log for exit.

1. Write each record file that is not purged or hidden to `<dir>/records/<64 hex>.json`. Use the stored bytes.
2. Decrypt each blob that an exported record names. Write the plaintext to `<dir>/blobs/<64 hex of the blob id>`.
3. Write `<dir>/manifest.json` with the canonical bytes of `{"spec": "hlog-core-v0.4", "identity": <id>, "records": <count>, "blobs": [{"id": <blob id>, "plaintext_sha256": <hex>, "media_type": <string>}]}`. Sort `blobs` by `id`.

The export is not encrypted. The CLI MUST say this before it writes.
`hlog import-export <dir>` reverses the export for a log with the same identity secret.
It encrypts each blob again. The deterministic encryption gives the same blob ids.

### 7.10 Indexes

An index is a cache in `local/cache/`.
A program MUST be able to delete it and rebuild it from `objects/` and `blobs/`.
A rebuild from the same files MUST give the same bytes.

## 8. Versioning

### 8.1 Known versions

This draft defines record version `v` = 0.
When the spec freezes, it will define `v` = 1. DEFAULT, Rob to confirm.
A verifier knows a version only if it implements every rule of that version.

### 8.2 Unknown version

A verifier MUST NOT interpret a record whose `v` it does not know.

- If `type` starts with `hlog.`, the affected identity is too new.
- Otherwise the verifier skips the record.

The record reached step 13 of 3.8, so its envelope, id and signature are good. The affected identity is:

1. For `hlog.genesis`: the record's own id, when `author` = `kid`.
2. For other `hlog.*` types: the `author` field, but only when `kid` is a signing key or recovery key of a node in that identity's known succession tree (6.3 step 3). Otherwise the record is skipped and the identity is not too new.

A file that no known key signed cannot make an identity too new. There is no `too new: log`.
DEFAULT.

For a too-new identity, the verifier MUST:

1. Report `too new: <identity id>`.
2. Not evaluate any record whose `author` or `subject` is that identity.
3. Not erase or hide anything for that identity.
4. Process all other identities as usual.

Commands that write for a too-new identity MUST stop with exit code 3.

### 8.3 Unknown fields

At a known `v`, a verifier MUST keep unknown fields, include them in the hash and not interpret them (3.2).

### 8.4 Version change policy

A change that changes the validity of an existing record needs a new `v`.
A new optional field that verifiers can ignore does not need a new `v`.
A new `hlog.*` type needs a new `v`.

## 9. Adapters

### 9.1 Contract

An adapter is a separate program. Its name is `hlog-import-<source>`.
It reads one source file or folder. It writes JSONL to standard output.
`hlog append` reads that JSONL, completes, signs and stores each record.

    hlog-import-oura export.zip | hlog append

An adapter MUST NOT sign records. `hlog append` signs.
The core MUST NOT contain code for a specific source. Device formats, including the NMR puck, are adapters.

### 9.2 Adapter output

Each JSONL line is a partial record. It MUST have `type`, `times.observed`, `source` and, for a reading, `value`.
It MAY have `context`, `refs` with rel `about` or `supersedes`, and `blobs`.
It MUST NOT have `v`, `nonce`, `author`, `kid`, `id`, `sig` or `times.imported`. `hlog append` sets them.
If it has no `subject`, `hlog append` sets the local identity id.

### 9.3 Blob first

1. Store the raw source file as a blob before any record (`hlog blob put <file>` prints the blob id). DEFAULT, Rob to confirm the command name.
2. Put `{"hash": <blob id>, "role": "raw-export", "media_type": <type>}` in `blobs` of each record from that file.

### 9.4 Granularity

An adapter MAY write one record per reading, or one record per (source, type, interval) with the readings in a blob series (`value` = `{"blob": <id>}`). High-frequency sources such as watch heart rate SHOULD batch. The interval and the series format are adapter conventions, not core. Open: the data engineer picks the interval.
Each record has `source.system`, `source.adapter` as `name@version`, and a dedupe ref in `source.ref` (4.8).
The adapter does not hold the dedupe key.
The adapter writes the HMAC input array (4.8) as `source.ref_input`.
`hlog append` computes `source.ref` from it and removes `source.ref_input` before it signs.
`source.ref_input` never reaches the log. DEFAULT, Rob to confirm.

### 9.5 Dedupe

`hlog append` MUST skip a record when:

- the index has a valid record with the same (`subject`, `source.system`, `source.ref`), or
- `local/purged-refs.jsonl` has that triple, or
- an earlier line of the same input has that triple.

`hlog append` reports the count of added and skipped records.

### 9.6 Unknown values

An adapter MUST write null when the source has no value for a time.
An adapter MUST put an unparseable time in `context.raw_time` and set `times.observed` to null.
An adapter MUST NOT invent values.

### 9.7 Parser fixes

An adapter cannot know record ids. To correct a record, a partial record carries `source.supersedes_ref` = the HMAC input array (4.8) of the record it corrects. `hlog append` resolves it to the valid record with that ref and the same subject and system, adds a `supersedes` ref, and removes `source.supersedes_ref` before signing. If no record matches, `hlog append` skips the line and counts it as skipped.
When the fixed record keeps the same HMAC input, its dedupe ref equals the old one; `hlog append` MUST NOT skip a record that supersedes the existing record with the same ref. A tuple ref changes when the value changes; that is expected.

### 9.8 Customer zero adapters

The first adapters are `hlog-import-csv`, `hlog-import-oura`, `hlog-import-whoop` and `hlog-import-apple-health`.
Each adapter document MUST say which source field is the native id, or that the source has none.
The mapping for Oura, Whoop and Apple Health is open (Appendix B).

## 10. Extension 1: computed results

### 10.1 Status

This extension is above the core line.
A computed result is an ordinary claim. The core does not check these fields.
A tool that claims Extension 1 support MUST follow this section.

Log a result when someone will act on it, share it, or compare it later.
Do not log every computation.

### 10.2 Fields

| Field | Type | Constraints |
|---|---|---|
| `method` | object | `{"id": string, "version": string, "hash": id}`. |
| `params` | object | Method parameters. |
| `inputs` | array of ids | The exact records read, in the order read. |
| `as_of` | string | UTC time, format of 3.5. The cutoff on `times.imported` of candidate inputs. |
| `excluded` | array | Items `{"id": id, "reason": string}`. Candidates the method did not use, with a reason. |
| `value` | object | The result. |

`method.hash` is `sha256:` + hex SHA-256 of the canonical bytes of:

    ["hlog/v0/method", <id>, <version>, <sha256 hex of the method spec text>, <sha256 hex of the conformance vector file>]

DEFAULT, Rob to confirm.

### 10.3 Replay

Replay of result D:

1. Find the method with `D.method.id` and `D.method.version`. If none, fail with `method unknown`.
2. Compute the method hash. If it differs from `D.method.hash`, fail with `method mismatch`.
3. For each id in `D.inputs`, in order:
   1. If a valid tombstone names it, or the replica hides it, fail with `input purged: <id>`.
   2. If the record is not in the replica, fail with `input missing: <id>`.
   3. If the record is not valid, fail with `input invalid: <id>`.
4. Run the method on exactly these inputs with `D.params`. Do not query the log for other records.
5. Compute the canonical bytes of the output.
6. If they differ from the canonical bytes of `D.value`, fail with `replay mismatch`.
7. Otherwise print the canonical bytes.

Replay MUST give the same result on every replica with the same files and the same identity secret.
The identity secret is necessary only when an input names a blob.

### 10.4 Reference method

The reference method is `median` version `1`.
It reads `value.n` of each input. It ignores inputs whose `value.n` is not a number and lists them in `excluded`.
For an even count, the median is the mean of the two middle values.
Output: `{"n": <median>, "count": <count>, "unit": <unit of the first input>}`.
The method spec text and its vectors are published files (section 14).

### 10.5 Product tests

The HRV 72-hour against 28-day comparison is a test for the first tool (`hlog compare`).
It is not a core test.

## 11. Extension 2: device capture

### 11.1 Status

This extension is above the core line.
The core treats device records as ordinary claims with a bare-key author.

### 11.2 Device keys

A device has its own key. The author of a device record is the bare device key.
The key is Ed25519 or P-256 (4.4).
A device SHOULD use a hardware-backed P-256 key when the platform has one, for example a phone secure element.
The device key never leaves the device.

### 11.3 Pairing

The subject pairs a device with an ordinary claim:

- `type` = `device.pair`
- `author` = `subject` = the identity id
- `value` = `{"key": <tagged device key>, "label": <string>}`

To unpair, the subject supersedes the pair claim with `value.key` = the same key and `value.status` = `"unpaired"`. DEFAULT, Rob to confirm.
The core does not check pairing. A reader decides by its own policy whether to trust a device record.

### 11.4 Capture

The device signs each record when it captures the reading.
`times.observed` uses the device clock.
`times.imported` is the device clock time when it signs. DEFAULT, Rob to confirm.
`source.ref` is null. The device does not hold the identity secret. The record id is the dedupe key.
A device record MUST NOT have `blobs`. The device cannot encrypt with the blob key.
To keep raw device evidence, the host stores the blob and writes its own claim with rel `about` to the device record. DEFAULT, Rob to confirm.

### 11.5 Clock record

When the host imports device records, the host writes one claim:

- `type` = `device.clock`
- `author` = `subject` = the identity id
- `value` = `{"key": <device key>, "device_time": <device clock reading>, "host_time": <host clock reading>}`

Both readings are RFC 3339 UTC strings with millisecond precision.
The host reads both clocks within one transfer session.
Apps use this record to correct clock drift.
Nobody changes the original times in device records.

### 11.6 Transfer format

The device gives the host a JSONL file of signed records and one `clock.json` with `{"key": <device key>, "device_time": <time>}`.
`hlog import-device <dir>` checks each signature, writes each record file unchanged, then writes the `device.clock` claim.
DEFAULT, Rob to confirm.

## 12. CLI reference

### 12.1 General

The reference CLI is `hlog`. It uses Go `flag` syntax.
Every command takes `-dir <path>`. The default is `$HLOG_DIR`, then the current directory.
Commands write results to standard output and errors to standard error.
An error line is `hlog: <error string>`.
Every command that loads the log applies the hold rules (7.6) first.

Exit codes:

| Code | Meaning |
|---|---|
| 0 | Success. `verify` found no errors. |
| 1 | The command failed, or `verify` found one or more errors. |
| 2 | Usage error: unknown command, bad flag or bad argument. |
| 3 | Too new. The command found an identity it cannot process (8.2). For `verify`, other identities were still checked. |
| 4 | Compromised. A local identity is frozen (5.7). |

If more than one code applies, use the highest. DEFAULT, Rob to confirm.

Verify output lines:

    error: <id or path>: <error string>
    warning: <id>: <warning string>
    too new: <identity id>
    compromised: <identity id>
    summary: <v> valid, <i> invalid, <s> skipped, <h> held, <d> duplicate

Lines are sorted by their ASCII bytes, with the summary line last.

### 12.2 Error strings

A conforming CLI MUST use these exact strings. Ids follow after `: ` where shown.

`bad path`, `too large`, `bad json`, `duplicate key`, `bad number`, `bad string`, `bad v`, `bad type`, `not canonical`, `bad field: <name>`, `id mismatch`, `unsupported key algorithm`, `bad key`, `bad signature`, `unknown core type`, `unknown rel`, `blob not listed`, `bad succession`, `key reuse`, `recovery not advanced`, `key not in chain`, `not sealed`, `frozen`, `not author`, `not subject`, `bad target`, `too new: <identity id>`, `compromised: <identity id>`, `seed does not match`, `input purged: <id>`, `input missing: <id>`, `input invalid: <id>`, `method unknown`, `method mismatch`, `replay mismatch`, `not initialized`, `already initialized`, `blob missing: <id>`, `blob corrupt: <id>`.

Warning strings: `held: <id>`, `unresolved author: <id>`, `unresolved subject: <id>`, `duplicate: <id> of <id>`, `superseded by sibling: <id>`, `hidden pending gc: <id>`, `blob missing: <id>` (for a purged or invalid record).

### 12.3 Commands

`hlog init`

- Input: none. Flags: `-p256` uses a hardware P-256 key if the platform has one.
- Action: section 5.2.
- Output: `identity <id>`, then the 24 words on 24 numbered lines. Then a warning: write the words on paper, and use them for hlog only.
- Errors: `already initialized` (exit 1).

`hlog append`

- Input: JSONL partial records on standard input (9.2).
- Action: fill `v`, `nonce`, `author`, `kid`, `times.imported`, and `subject` if absent. Compute `source.ref` from `source.ref_input`. Check dedupe (9.5). Check authority for supersedes (6.6). Sign and write.
- Output: `added <n>, skipped <m>`.
- Errors: `bad json` with the line number, `not author`, `not initialized`. On the first error, stop. Records before the error stay written.

`hlog blob put <file>`

- Action: encrypt and store the file (7.3).
- Output: the blob id.

`hlog verify`

- Action: section 6 and the hold rules. Check every blob hash (`blob corrupt: <id>`) and every named blob (`blob missing: <id>`, a warning when the record is purged).
- Output: 12.1 verify lines.

`hlog reindex`

- Action: delete `local/cache/` and rebuild it.
- Output: `indexed <n> records`.

`hlog rotate [-no-seal <id>]...`

- Input: the 24 words on standard input. Numbering and case are ignored.
- Action: section 5.5. Seal every valid destructive record of the outgoing key in this replica except the ids that `-no-seal` names.
- Output: one line `sealed <id>` or `not sealed <id>` per destructive record of the outgoing key, then `rotated to <tagged key>`, then `sealed <n>`. Before reading input, print `sync all devices before you rotate` to standard error.
- Errors: `seed does not match`, `compromised: <id>` (exit 4).

`hlog gc`

- Action: section 7.6 step 5.
- Output: `erased <r> records, <b> blobs`.

`hlog blob get <id>`

- Action: decrypt one blob to standard output.
- Errors: `blob missing: <id>`, `blob corrupt: <id>`.

`hlog purge [-reason <text>] <id>`

- Action: section 7.7.
- Errors: `not subject`, `bad target`, `input missing: <id>` when the target is not in the replica.

`hlog sync <source dir>`

- Action: section 7.4 into the `-dir` log.
- Output: `copied <r> records, <b> blobs, refused <p> purged`, then `replaced <n> bad copies` when n > 0.

`hlog export <dir>`

- Action: section 7.9. `<dir>` MUST be empty or absent.
- Output: `exported <r> records, <b> blobs (plaintext)`.

`hlog import-export <dir>`

- Action: section 7.9 reverse. The log MUST have the same identity secret.
- Output: `added <r> records, <b> blobs`.

`hlog pair <tagged key> [-label <text>]` (Extension 2)

- Action: write a `device.pair` claim (11.3).

`hlog import-device <dir>` (Extension 2)

- Action: section 11.6.
- Output: `added <n>, clock offset <ms> ms`.

`hlog derive median -type <type> [-as-of <time>]` (Extension 1)

- Action: select candidate records of `<type>` with `subject` = local identity and `times.imported` at or before `-as-of` (default: now). Order them by `times.imported`, then by id. Exclude superseded and duplicate records, and records whose `value.n` is not a number, with reasons. Write the result record (10.2).
- Output: the result id.

`hlog replay <id>` (Extension 1)

- Action: section 10.3.
- Output: `replay ok <canonical bytes>`.

`hlog-import-<source> [flags] <file>`

- Output: JSONL partial records (9.2) on standard output.
- `hlog-import-csv -type <type> -system <system> -blob <blob id> [-unit <unit>] <file.csv>` reads a header row. It uses column `value`, optional column `time` (RFC 3339) and optional column `id` (native id). Get `<blob id>` from `hlog blob put <file.csv>` first.

Test clock: the reference CLI reads the current time from `$HLOG_NOW` (RFC 3339) when it is set. Tests use it for holds and `times.imported`. DEFAULT, Rob to confirm.

## 13. Conformance tests

Each test starts from empty folders unless the setup says otherwise.
"Verify passes" means exit code 0 and no `error:` lines.
A conforming implementation MUST pass every test.
Tests 1 to 22 cover the v0.2 acceptance tests 1 to 6 and the new decisions. Tests 23 to 30 cover extensions and edge cases.

### Tamper and index (v0.2 test 1)

1. Record byte flip.
   - Setup: init log A. Append 3 claims.
   - Action: change one byte inside the `value` of one record file.
   - Expected: verify exits 1. It prints an error for that path (`not canonical`, `bad json` or `id mismatch`).
2. Blob byte flip.
   - Setup: log A with one blob and one record that names it.
   - Action: change one byte of the blob file.
   - Expected: verify exits 1 with `blob corrupt: <blob id>`.
3. Index rebuild.
   - Setup: log A with 10 claims, 1 supersede record and 1 tombstone.
   - Action: copy `local/cache/`. Run `reindex`. Compare.
   - Expected: the bytes are identical.

### Sync and dedupe (v0.2 test 2)

4. Union of two folders.
   - Setup: logs A and B of one identity (B has a copy of `local/`). Append different claims to each.
   - Action: sync A to B and B to A.
   - Expected: both have the same `objects/` and `blobs/` file sets. Verify passes on both.
5. Same export on a second device.
   - Setup: A and B as in test 4, synced. Export file E.
   - Action: import E on A. Sync A to B. Import E on B.
   - Expected: the second import prints `added 0`.
6. Overlapping exports.
   - Setup: file E1 has readings 1 to 10. File E2 has readings 6 to 15. Run the test twice: once with native ids and once without (tuple refs). Include two exactly identical readings in the overlap.
   - Action: import E1, then E2.
   - Expected: the log has 15 readings plus the identical duplicate reading, so 16 records. The second import adds only the readings 11 to 15.

### Supersede (v0.2 test 3)

7. Author with head key.
   - Setup: identity I appends claim T.
   - Action: I appends a record that supersedes T.
   - Expected: verify passes. T is superseded.
8. Non-author.
   - Setup: identities I and J share a log. I appends claim T.
   - Action: J appends a record that supersedes T. Then write the same record with a direct file write.
   - Expected: append fails with `not author`. Verify reports `not author` for the file.
9. Rotated-out key, not sealed.
   - Setup: I appends claim T. Replica B (same signing key K1) writes supersede record D of T. Replica A rotates to K2 before D arrives.
   - Action: sync both ways. Verify.
   - Expected: `not sealed` for D. T is not superseded.
10. Rotated-out key, sealed.
    - Setup: as test 9, but D is in the first replica before rotation.
    - Action: rotate. Verify.
    - Expected: the succession lists D in `value.sealed`. Verify passes. T is superseded.

### Tombstone (v0.2 test 4)

11. Propagate.
    - Setup: synced replicas A and B of identity I. Claim T on both.
    - Action: purge T on A. Sync A to B. Set `HLOG_NOW` 8 days later on B. Verify B.
    - Expected: A has erased T at once. B hides T after sync and erases it after 7 days. Verify passes on both.
12. Stale replica.
    - Setup: C is a copy of A from before the purge in test 11.
    - Action: sync C to A. Then sync A to C.
    - Expected: A does not get T back. C hides T.
13. Rotated-out key.
    - Setup: a copy of I's key K1 writes tombstone D for claim T. I rotates to K2 without D.
    - Action: verify.
    - Expected: `not sealed` for D. T is visible.
14. Core targets.
    - Action: write tombstones for the genesis, for a succession and for another tombstone of I.
    - Expected: `bad target` for each.
15. Re-import after purge.
    - Setup: import file E. Purge one record from E.
    - Action: import E again.
    - Expected: `added 0`.
16. Stranger tombstone.
    - Setup: identity J writes a tombstone that names claim T of identity I. Replica B has the tombstone but not T. Replica A has T.
    - Action: sync A to B.
    - Expected: B gets T. Verify on B reports `not subject` for the tombstone.

### Rotate and recover (v0.2 test 5)

17. Rotate twice.
    - Setup: I appends claim C1. Rotate. Append C2. Rotate. Append C3.
    - Expected: verify passes. C1, C2 and C3 have three different `kid` values. The identity id did not change. The chain has three nodes.
18. Signing key cannot rotate.
    - Setup: I at head H.
    - Action: write a succession with parent H signed by H's signing key.
    - Expected: `bad succession`. The chain is unchanged.
19. Thief's tombstone dies at rotation.
    - Setup: a copy of the head key writes tombstone D for T on replica B. Replica A rotates without D.
    - Action: sync both ways twice.
    - Expected: D is `not sealed` on both. T is visible on both.
20. Rotation with a sealed tombstone.
    - Setup: the head key writes tombstone D, then the person rotates.
    - Action: rotate with the default seals.
    - Expected: the succession lists D in `value.sealed`. D stays valid.
21. Double rotation freeze.
    - Setup: I at head H on replicas A and B.
    - Action: rotate on A and on B without syncing. Sync.
    - Expected: verify reports `compromised: <I>` and exits 4. Both successions and every destructive record of I are `frozen`. Claims on the trunk stay valid. `rotate` on a frozen identity fails with `compromised: <I>`.
22. Recovery key cannot sign claims.
    - Action: write a claim whose `kid` is a recovery public key of I.
    - Expected: `key not in chain`.

### Version (v0.2 test 6)

23. Unknown version, ordinary type.
    - Action: add a file with `v` = 99 and type `hrv.rmssd`.
    - Expected: verify passes with the file counted as skipped. Sync copies the file.
24. Unknown version, core type, per identity.
    - Setup: identities I and J in one log.
    - Action: add an `hlog.succession` file with `v` = 99 and `author` = I.
    - Expected: `too new: <I>`. Exit 3. All records of J are still checked. A file error in J is still reported.

### Delete hold

25. Rotation within the hold restores.
    - Setup: replicas A and B. A thief with I's head key writes tombstone D for T on A. Sync A to B. B hides T.
    - Action: on day 3 (`HLOG_NOW`), rotate on A with `-no-seal D`. Sync A to B.
    - Expected: D is `not sealed`. B moves T back to `objects/` and removes the hold. `gc` on day 30 erases nothing.
25b. No erase without gc.
    - Setup: as test 11 after the sync to B.
    - Action: set `HLOG_NOW` 8 days later. Run `verify` on B, then `gc`.
    - Expected: `verify` warns `hidden pending gc: <T>` and erases nothing. `gc` erases T and prints `erased 1 records`.
31. Too-new file without a chain signature.
    - Setup: identities I and J.
    - Action: add an `hlog.succession` file with `v` = 99, `author` = I, signed by J's key.
    - Expected: no `too new` line. The file is skipped.

### Exit and devices

26. Export round trip.
    - Setup: log A with claims, a blob and a tombstone.
    - Action: export to X. Make log B with the same identity secret and keys. Run `import-export X` on B.
    - Expected: B has the same record files and the same blob ids as A, minus the purged target. Verify passes on B. `manifest.json` lists every blob with its plaintext SHA-256.
27. P-256 device key.
    - Setup: a P-256 device key. Pair it.
    - Action: import device records signed with low-S. Add one record with a high-S signature.
    - Expected: the low-S records are valid. The high-S record is `bad signature`. One `device.clock` claim is written.

### Extension 1 and edge cases

28. Replay on a second replica.
    - Setup: A with 4 claims and a median result D. B is a sync of A with the same identity secret.
    - Action: replay D on A and on B.
    - Expected: both print the same canonical bytes.
29. Replay with a purged input.
    - Action: purge one input of D. Replay twice.
    - Expected: both fail with `input purged: <id>`.
30. Order independence.
    - Setup: any log from tests 17 to 22.
    - Action: copy the files into a new folder in a random order 10 times. Verify each.
    - Expected: identical verify output each time.

## 14. Test vectors to publish

The project MUST publish these vectors before v1 is frozen.
All values are TBD. Nobody has computed them yet.
Each vector is a JSON file in `vectors/` with inputs and expected outputs.
Two independent implementations MUST produce each value before the vector is published.

| # | Vector | Inputs | Expected output |
|---|---|---|---|
| V1 | JCS numbers | The RFC 8785 number samples, plus -0, 2^53 - 1, 2^53, 1e21, 1e-7 | Canonical strings or `bad number`. TBD |
| V2 | JCS key order | Keys with non-ASCII and astral characters | Canonical bytes. TBD |
| V3 | JCS strings | Control characters, `/`, U+007F, a lone surrogate escape | Canonical bytes or `bad string`. TBD |
| V4 | Record id | Three full records (genesis, claim, tombstone) | Canonical bytes without `id` and `sig`, and the id. TBD |
| V5 | Ed25519 signature | A fixed private seed and a record id | `kid`, `sig`. TBD |
| V6 | P-256 signature | A fixed private key, a record id, a low-S and a high-S signature | Accept and reject results. TBD |
| V7 | Commitment | A tagged recovery public key | Commitment id. TBD |
| V8 | Paper seed | 32 fixed entropy bytes | The 24 words. TBD |
| V9 | HKDF | The V8 seed | Identity secret, recovery generations 0, 1 and 2 (tagged public keys). TBD |
| V10 | Derived keys | The V9 identity secret | Blob key (64 bytes) and dedupe key (32 bytes). TBD |
| V11 | AES-SIV | The V10 blob key and 3 plaintexts (empty, 1 byte, 1 MiB pattern) | `V || C` and the blob id. TBD |
| V12 | Dedupe ref | The V10 dedupe key, one native-id input, one tuple input, occurrences 0 and 1 | `source.ref` strings. TBD |
| V13 | Chain selection | Folders for tests 17 to 22 | Chain ids, head, verify output. TBD |
| V14 | Sealed rotation | Folders for tests 9, 10, 13 and 20 | Verify output. TBD |
| V15 | Too new | Folder for test 24 | Verify output and exit code. TBD |
| V16 | Median method | Input records and `D.value` | Method hash and canonical output. TBD |
| V17 | Bad files | One file per error string in 3.8 | The error string. TBD |

The AES-SIV code MUST also pass the RFC 5297 Appendix A vectors. The HKDF code MUST pass the RFC 5869 Appendix A vectors.

## 15. Security considerations

- Key theft. A thief with the head key can add claims, supersede, tombstone and rotate. Recovery with the paper seed ends this. The thief's destructive records become invalid unless the person seals them. The thief's claims on the chain trunk stay valid (17).
- Paper seed theft. The seed holder can recover and take the identity. If the owner also recovers from the same parent, the identity freezes. There is no way back from a freeze in v0.
- Seed exposure. Devices never store the seed. `init` and `recover` MUST erase it from memory after use, as far as the language allows.
- No trusted time. Validity never uses a clock. Only the delete hold uses the local clock, and it never changes validity.
- Stranger tombstones. A tombstone is checked against its target before sync refuses the target (7.5).
- Signature malleability. P-256 requires low-S. A replica keeps the first file it gets for an id (4.5).
- Denial of service. A replica MUST limit record size (3.1). A verifier SHOULD limit the succession tree depth to 10,000 nodes. DEFAULT, Rob to confirm.
- Parser safety. JSON parsing MUST reject duplicate keys and bad numbers before any check uses the value.
- Equal blobs. Deterministic encryption shows when two blobs of one identity hold the same plaintext (16).
- Local files. `local/` holds the identity secret and the signing key. File mode MUST be 0600. Backups of `local/` are as sensitive as the keys.

## 16. Privacy considerations

- Linkability. The identity id appears in every record and every export. Recipients of two exports can link them. Per-recipient pseudonymous subjects are deferred (18).
- Equality leak. AES-SIV is deterministic. Anyone who sees two blobs can tell if their plaintexts are equal. The same holds for dedupe refs inside one identity. This is the cost of dedupe and idempotent sync.
- Metadata. Record files are plaintext JSON. `type`, `times`, `value` and `context` are visible to anyone with the folder. Only blobs are encrypted. Encryption of records for transport is a sharing extension (18).
- Identity secret exposure. Whoever has the identity secret can decrypt all blobs and test guesses against dedupe refs. Rotation does not change it. The fix is a new identity and a new log (17).
- Export. `hlog export` writes plaintext. The person must protect the export folder.
- Tombstones. A tombstone shows that a record with that id existed. It does not show the content.

## 17. Known limits

- Rotation needs the paper seed. A person who loses the paper and the device loses the identity. This is the price of a stolen device never being able to take over.
- Records are plaintext on disk. Only blobs are encrypted. Folder-level encryption is the sync tool's job in v0.

1. A rotated-out chain key can still sign a claim that verifies, with any `times.observed`. Witness records are a later extension.
2. An identity secret leak needs a new paper seed, a new identity and a new log. The old data must be re-encrypted into the new log.
3. A replica that never saw a purged target does not have its dedupe ref. A re-import on that replica brings the data back.
4. Two hosts that import the same source before they sync make duplicate records. Section 6.3 step 12 marks them. They stay in the log.
5. Tuple dedupe refs merge two real readings with the same type, observed time and value in two different exports when the occurrence counts match.
6. After erasure, a replica cannot restore a target from its own disk.
7. A frozen identity cannot recover.
8. Destructive records that a device signs and does not sync before a rotation become invalid.
9. Device records cannot carry encrypted blobs (11.4).

## 18. Out of scope

- Computed results in the core (Extension 1 holds them).
- `assurance`, `kind`, `prev`, `quality`, `coding` and `times.recorded` fields.
- `attests` and `disputes` refs. Use `about` and a type.
- A blessed series format.
- A type registry. Type conventions are plain documents.
- Per-recipient pseudonymous subjects (deferred).
- Grants, sharing, selective disclosure and zero-knowledge proofs.
- Trusted time, witness records and global ordering.
- Nostr and Blossom transport (a later extension, with `secp256k1` keys).
- User interface, servers and accounts.
- Products: `hlog compare`, the puck app and the clinician view.

## 19. Governance and license

- This specification is CC0 1.0.
- The reference code is Apache-2.0.
- The project takes a neutral name. "hlog" is a placeholder.
- AION Biosciences stewards the project. AION does not own it.
- The repository stays private on Benjamin Anderson's GitHub account until the review with Rob Long. A decision to publish comes after that review.
- Changes to this spec need a written change note and new test vectors when a result changes.

## Appendix A: deltas from the reference code at /workspace/hlog

The reference code at `/workspace/hlog` implements the earlier v0.1 draft. It does not implement this spec.
The build is on hold. This list is from a read of the code on 2026-10-27.

| # | Topic | Code now | This spec |
|---|---|---|---|
| A1 | Computed results | `derive median` and `replay` are in the core package, and `method`, `inputs`, `as_of`, `excluded` are core fields. | Extension 1 (section 10). |
| A2 | Median method hash | SHA-256 of a description string. | Hash of the spec text and vector file (10.2). |
| A3 | Value series | `value.series` `{hash, format, time_column, value_column}` plus `summary`. CSV is blessed. | `{"blob": id, "unit": ...}`. No blessed format. |
| A4 | Blob encryption | AES-256-GCM with a synthetic IV from HMAC. Key = HMAC of a random local secret with label `blob`. | AES-SIV (RFC 5297) with an HKDF key (4.9). |
| A5 | Secret | `local/secret` is 32 random bytes in hex. | Identity secret derived from the paper seed (4.7). |
| A6 | Recovery key | A random key stored in `local/recovery.key` on the device. | Derived from the paper seed. Never stored on a device. |
| A7 | Commitment | SHA-256 of the 32 raw public key bytes. | SHA-256 of the tagged key string (4.6). |
| A8 | Recovery commitment on rotation | Each rotation commits to a new recovery key, also for a current-key rotation. | A current-key rotation copies the commitment. Only recovery advances it (5.8). |
| A9 | Sealed rotation | No `sealed` field. Any chain key can supersede or tombstone at any time. | Head key, or a seal (5.4, 6.3 step 7). |
| A10 | Forks | A fork is an error (`succession does not extend its identity chain`). No recovery-wins rule. No freeze. | Fork rules, recovery wins, freeze (5.6, 5.7). |
| A11 | Dedupe ref | HMAC-SHA-256 over the CSV row text (fields joined by 0x1F) or the whole file. Bare hex. Key = HMAC of the local secret with label `ref`. | HMAC over a canonical array with native id or tuple and occurrence. `hmac-sha256:` prefix (4.8). |
| A12 | Dedupe scope | Unique (`source.system`, `source.ref`). | Unique (`subject`, `source.system`, `source.ref`). Duplicates marked, lowest id primary. |
| A13 | Purged refs | Not kept. A re-import after a purge brings the data back. | `local/purged-refs.jsonl` (7.8). |
| A14 | Delete hold | Purge and sync erase at once on every replica. | Hide, 7-day hold, restore (7.6). |
| A15 | Tombstone targets | A tombstone can name a succession (the subject check passes). Genesis is protected only because it has no subject. | Genesis, succession and tombstone are never targets (6.7). |
| A16 | Unknown version | Every record with an unknown `v` is skipped. | `hlog.*` makes the identity too new. Others are skipped (8.2). |
| A17 | Unknown `hlog.*` type and unknown rel | Not checked. | Invalid (3.3, 3.7). |
| A18 | Tombstone reason | `value.reason`. | `context.reason`. No `value`. |
| A19 | File bytes | Canonical bytes plus a final newline. Readers do not check that file bytes are canonical. | Canonical bytes, no final newline. `not canonical` error (3.8 step 10). |
| A20 | JSON parsing | `encoding/json` into `map[string]any`. Duplicate keys are accepted (last wins). Large integers lose precision. | Reject duplicate keys and out-of-range numbers (3.8). |
| A21 | Strings | Invalid UTF-8 is replaced silently by U+FFFD in the JCS writer. | Reject invalid UTF-8 and lone surrogates. |
| A22 | Key algorithms | Ed25519 only. | Ed25519 and P-256. `secp256k1` reserved. |
| A23 | Export | No `export` command. | `hlog export` and `hlog import-export` (7.9). |
| A24 | Devices | No pairing, no device import, no clock record. | Extension 2 (11). |
| A25 | Paths | `local/identity`, `local/secret`, `local/signing.key`, `local/recovery.key`, `index.json` at the log root. | `local/identity`, `local/identity-secret`, `local/signing-key`, `local/holds.json`, `local/purged-refs.jsonl`, `local/cache/`. |
| A26 | Index | `index.json` with a fingerprint of object file names only. | Cache in `local/cache/`. Byte-identical rebuild. |
| A27 | Adapter | `hlog-import` stores the blob itself (it reads `local/secret`). It has no native id column. | `hlog blob put` plus `hlog-import-csv -blob`. Native id column `id`. `source.ref_input` (9.4). |
| A28 | Sync | Honors a tombstone unless its target is visible in either replica with another subject. Erases at once. | Same check (7.5). Also hides under hold and records purged refs. |
| A29 | Exit codes | 0 or 1 only. | 0 to 4 (12.1). |
| A30 | Test clock | Package variable `Now`. | `$HLOG_NOW` for the CLI. |
| A31 | Bare-key authors | `kid` must equal `author`. | Same. |
| A32 | `subject` check | Any non-empty string. | Identity id. Missing genesis is a warning (6.3 step 11). |

## Appendix B: open items

Each item has a DEFAULT in the text. Rob can change any of them.

1. Curve choice: Ed25519 default, P-256 for hardware device keys, `secp256k1` reserved. Decided 2026-10-28: a Schnorr/secp256k1 signing key (a Nostr key) is added as an extension after v0, when a reader or signer needs one. It needs a dependency; the core stays at zero until then.
2. Own record format, with Nostr only as an encrypted carrier later. No NIP-01 event shape in the core.
3. One identity secret per person now. Per-blob keys come with the sharing extension.
4. Decided in v0.4: every succession is seed-signed; any fork is a freeze (5.6, 5.7). The lowest-id rule is gone. Rob: confirm, or name what KERI pre-rotation still adds.
5. Hold length of 7 days (7.6). Decided in v0.4: the hold gates `gc`; nothing is erased on a timer.
6. Record size limit of 1 MiB (16 MiB for successions) and blob minimum of 256 MiB.
7. `type` pattern and precision values (3.2, 3.5).
8. AES-SIV associated data label (4.9) and the `hmac-sha256:` ref prefix (4.8). A vetted AES-SIV library is allowed; the reference build is hand-rolled and matches OpenSSL.
9. Paper seed as BIP-39 English words over 32 bytes (4.10).
10. Decided in v0.4: one host holds the signing key; other devices are bare device keys (11.3). No shared signing key.
11. Native id fields for Oura, Whoop and Apple Health. Nobody has checked these export formats for this spec yet. Decided per adapter from a real export, never from memory.
16. Batching interval and series format for high-frequency sources (9.4). Data engineer to pick.
17. Measurement uncertainty convention (for example `context.cv`) for the NMR source. Adapter-level.
18. Query format for the index (7.10). Readers decide; v0 ships `local/cache/index.json` only.
12. Device record `times.imported` and blob handling (11.4).
13. Method hash input (10.2).
14. Exit code priority (12.1).
15. Record version number at freeze (8.1).
