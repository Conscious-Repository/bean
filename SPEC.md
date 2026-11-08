# bean core specification, draft

Status: draft. This text is not frozen. [CORE.md](CORE.md) section 6 tells when it freezes.
Date: 2026-10-30.
This text defines record format `v` 0. [UPGRADING.md](UPGRADING.md) lists the changes that alter results for an existing log.

Reference implementation: the `bean` repository.
License: CC0 1.0 (section 19).
Name: bean.

This specification uses Simplified Technical English (ASD-STE100).
The key words MUST, MUST NOT, REQUIRED, SHOULD, SHOULD NOT and MAY have the meanings in RFC 2119 and RFC 8174.
Text marked DEFAULT is a decision that can change until the format freezes (8.1).
An implementation MUST follow a DEFAULT until the specification changes it.
[CORE.md](CORE.md) is a short summary of the rules that two implementations must agree on.

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
14. Test vectors
15. Security considerations
16. Privacy considerations
17. Known limits
18. Out of scope
19. Governance and license
- Appendix: defaults and open items

## 1. Scope and goals

### 1.1 Scope

This specification defines the bean core.
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
Programs do not change records. Programs only add records.

### 1.3 Five ideas

1. A record is a claim, not a fact.
2. Each record has a fingerprint (the id, a hash) and a signature.
3. The log is a folder. Sync copies the files that a replica does not have. There is no server, no account and no conflict.
4. Programs never edit. Programs only add. A correction supersedes a record. A deletion is a tombstone that every copy obeys.
5. A person is an identity. The identity id never changes. Keys rotate. An offline paper seed recovers the identity.

### 1.4 Goals

- G1. Two conforming verifiers that hold the same set of files MUST get the same validity result for every record.
- G2. The verifier MUST NOT need a network, an account, a server or a trusted clock.
- G3. The person MUST be able to leave at any time with plaintext data (`bean export`).
- G4. Any index MUST be a cache that a program can rebuild from the files alone.
- G5. The core MUST be source-agnostic. A device format is an adapter. Special code for one source in the core is a bug.
- G6. No program erases user data without a direct command from the person on that replica (`purge`, `gc`; 7.6).
- G7. A reader MUST be able to build the trajectory of one stream (subject, source system, type) over `times.observed` from the files alone.
- G8. The core MUST NOT depend on any one company. Apps and subscriptions are layers above the core.

## 2. Terms

Each term has one meaning in this document.
Other documents and code comments MUST use these terms with these meanings.

| Term | Meaning |
|---|---|
| log | A folder that holds records and blobs for one or more identities. |
| replica | One copy of a log on one machine. |
| record | One JSON object stored as one file in `objects/`. |
| claim | A record that is not a destructive record and not an identity record. |
| identity record | A record of type `bean.genesis`, `bean.succession` or `bean.seal`. |
| destructive record | A record that has a `supersedes` ref, or a record of type `bean.tombstone`. |
| supersede record | A record that has one or more `supersedes` refs. |
| tombstone | A record of type `bean.tombstone`. It has one `purges` ref. |
| target | The record that a `supersedes` ref or a `purges` ref names. |
| blob | One encrypted file in `blobs/` that holds raw evidence. |
| id | The string `sha256:` followed by 64 lowercase hex digits. It names a record or a blob. |
| tagged key | A public key string with an algorithm tag, for example `ed25519:<hex>`. |
| signing key | A key pair that signs records. Devices hold the private half. |
| recovery key | An Ed25519 key pair that the paper seed derives. It signs only successions and seal parts (5.8). |
| commitment | The SHA-256 hash of a tagged recovery public key, as an id string. |
| paper seed | 32 random bytes, written on paper as 24 words. Devices never store it. |
| identity | A person or agent. Its identity id is the id of its genesis record. |
| identity secret | 32 bytes that the paper seed derives. It derives the blob key and the dedupe key. |
| genesis | The first record of an identity. Type `bean.genesis`. |
| succession | A record that names the next signing key of an identity. Type `bean.succession`. |
| node | A genesis or a succession, seen as a point in the succession tree. |
| succession tree | All well-signed nodes of one identity, linked by `succeeds` refs. |
| parent | The node that a succession names in its `succeeds` ref. |
| chain | The path of nodes from genesis that the verifier selects (section 6.3). |
| head | The last node of the chain. |
| head key | The `value.key` of the head. |
| chain key | The `value.key` of any node on the chain. |
| fork | Two or more well-signed successions that have the same parent. |
| fork point | The chain node with two or more children. Everything below it is invalid (`frozen`). |
| seal | To list a record id in the `value.sealed` list of a succession or of one of its seal continuations. |
| frozen | The state of an identity after a recovery conflict (section 5.7). A verifier reports it as `compromised`. |
| bare key | A tagged key used directly as an author. A bare key never rotates. |
| author | The identity id or bare key that makes the claim. |
| subject | The identity id of the person that the record is about. |
| kid | The tagged key that made the signature. |
| valid | A record that passes every check in sections 3 and 6. |
| invalid | A record that fails a check. A verifier reports it with an error string. |
| held | A destructive record whose target is not in the replica, or a succession whose parent is not in the replica. The verifier keeps it and checks it again when the target or parent arrives. |
| too new | The state of an identity after the verifier finds a `bean.*` record with an unknown `v`. |
| skip | To keep a file, sync it, and not interpret it. |
| hide | To stop showing, serving and syncing a target. The bytes stay on disk. |
| erase | To delete the bytes of a target from disk. |
| hold | The time from receipt of a valid tombstone to erasure. 7 days. |
| adapter | A separate program that reads one source format and writes records as JSONL. |
| native id | The id that a source system gives to one reading. |
| dedupe ref | The value of `source.ref`. A keyed hash (section 4.8). |
| occurrence index | A counter that separates identical readings (section 4.8). |
| verifier | A program that checks records against this specification. |
| writer | A program that makes and signs records. |
| export | A plaintext copy of a log (section 7.9). |
| index | A cache that a program builds from the files. It is never the truth. |
| host | The machine that runs `bean` and holds the identity secret. |
| device | A capture machine with its own key, for example a sensor (Extension 2). |

## 3. Record format

### 3.1 General

A record is one JSON object.
A writer MUST store a record as the RFC 8785 canonical bytes of the full object, with `id` and `sig` included (section 4.1).
The file has no byte order mark and no final newline.
The maximum file size is 1 MiB. DEFAULT.

### 3.2 Fields

| Field | JSON type | Required | Constraints |
|---|---|---|---|
| `v` | integer | yes | Record format version. This draft uses `0`. |
| `nonce` | string | yes | 32 lowercase hex digits (128 bits) from a cryptographic random source. |
| `type` | string | yes | 1 to 128 ASCII characters. Pattern `^[a-z0-9][a-z0-9_-]*(\.[a-z0-9_-]+)*$`. DEFAULT. |
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
The one exception is `value` of a `bean.*` record: 3.3 gives its keys exactly, and an extra key is `bad field: value`.

A pattern in this specification (a regular expression) MUST match the whole string. `$` is the end of the string, never a position before a final newline. `\d` and the character classes are ASCII only.

There are no `assurance`, `kind`, `prev`, `quality`, `coding` or `times.recorded` fields.
Put coding (LOINC, SNOMED), quality flags, source timestamps and device details in `context`.
Suggested `context` keys: `device`, `site`, `conditions`, `coding`, `source_times`, `quality`, `raw_time`, `reason`.
These keys are conventions. The core does not check them.

### 3.3 Reserved types

| Type | Purpose | Extra constraints |
|---|---|---|
| `bean.genesis` | Starts an identity. | No `subject`. `author` = `kid` = `value.key`. No `refs` (absent or an empty array). `value` = `{"key": tagged key, "recovery": commitment}`. `times.observed` = null. `source` = `{"system":"self","adapter":<name@version>,"ref":null}`. |
| `bean.succession` | Names the next signing key. | `subject` = `author` = the identity id. Exactly one ref, rel `succeeds`. `value` = `{"key": tagged key, "recovery": commitment, "sealed": [ids]}`, plus `"parts": n` (an integer from 1 to 1000) only when n continuations follow. |
| `bean.seal` | Continues the `sealed` list of a succession. | `subject` = `author` = the identity id. `kid` = the `kid` of the succession. Exactly one ref, rel `seals`, that names the succession. `value` = `{"part": k, "sealed": [ids]}`, k an integer from 1 to 1000, `sealed` non-empty, unique and sorted. `times.observed` = null. No `blobs` (absent or an empty array). `source` = `{"system":"self","adapter":<name@version>,"ref":null}`. |
| `bean.tombstone` | Deletes a target. | `subject` = `author`. Exactly one ref, rel `purges`. No `value`. No `blobs` (absent or an empty array). An optional reason goes in `context.reason` (string, at most 200 Unicode code points). |

Any other type that starts with `bean.` is reserved for future core versions.
A writer MUST NOT make one.
At `v` 0, a verifier MUST treat an unknown `bean.*` type as invalid with error `unknown core type`.
At `v` 0, a record of type `hlog.genesis`, `hlog.succession` or `hlog.tombstone` is invalid with error `old format`. These are the core types from before the rename to bean. The check comes after `nonce` and `type` in step 15 (3.8).

### 3.4 value

`value` is an object.
For types outside `bean.*`, these shapes are RECOMMENDED:

| Shape | Meaning |
|---|---|
| `{"n": number, "unit": string}` | A number. `unit` SHOULD be UCUM. |
| `{"s": string}` | A string. |
| `{"b": boolean}` | A boolean. |
| `{"blob": id, "unit": string}` | A series or a large payload in a blob. |

The core checks only one rule for `value` outside `bean.*`.
If `value.blob` is present, it MUST be an id that is also a `hash` in `blobs`.
Otherwise the record is invalid with error `blob not listed`.
The core does not bless a series format. The method that reads the blob defines the format.

### 3.5 times

`times.imported` is REQUIRED and MUST NOT be null.
The writer sets it when the record enters the log.
Format: `YYYY-MM-DDTHH:MM:SSZ`, UTC, second precision. It is a time (below) with no fraction and the offset `Z`.

`times.observed` is REQUIRED as a key. Its value is null or an interval.
An interval is `{"start": time, "end": time, "precision": p}`. `end` is optional.
`start` and `end` are times.
A time is a string that matches `^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?(Z|[+-][0-9]{2}:[0-9]{2})$` as a whole, with these ranges:

- Year 0001 to 9999, month 01 to 12, a day that exists in that month (February 29 only in a leap year of the Gregorian calendar).
- Hour 00 to 23, minute 00 to 59, second 00 to 59. There is no leap second.
- Offset hour 00 to 23, offset minute 00 to 59. `-00:00` and `+00:00` mean the same as `Z`.

This is a strict subset of RFC 3339: uppercase `T` and `Z` only, a dot before the fraction, at most 9 fraction digits.
Any other string is invalid with `bad field: <name>`.
If `end` is present, `end` MUST NOT be earlier than `start`. Compare the two instants exactly, with all fraction digits.
`p` is one of `year`, `month`, `day`, `hour`, `minute`, `second`, `millisecond`. DEFAULT.

A writer MUST set `times.observed` to null when the source does not give a time.
A writer MUST NOT guess or default a time.
A writer puts an unparseable source time in `context.raw_time` as a string.

### 3.6 source

`source.system` is a string of 1 to 64 bytes in UTF-8, for example `oura` or `self`.
`source.adapter` is `name@version`, for example `bean-import-oura@1.0.0`. The verifier checks only the form: one `@`, with at least one character before and after it, and no space, TAB (U+0009), LF (U+000A), FF (U+000C) or CR (U+000D) (regular expression `^[^@\t\n\f\r ]+@[^@\t\n\f\r ]+$`). Other characters, for example U+000B and U+00A0, are allowed. Error: `bad field: source.adapter`.
`source.ref` is a dedupe ref (section 4.8) or null.
`source.ref` is null for a record that the author makes directly, with no source file.
`source.ref` is null for a device-signed record (Extension 2).

### 3.7 refs

Each item is `{"rel": rel, "id": id}`.
The core knows five rel values:

| rel | Meaning | Core checks |
|---|---|---|
| `succeeds` | This succession follows the named node. | Section 6. |
| `supersedes` | This record corrects the target. | Section 6.6. |
| `purges` | This tombstone deletes the target. | Section 6.7. |
| `seals` | This seal part continues the named succession. | Section 5.4. |
| `about` | This record comments on, attests or disputes the target. | None. |

A record with any other rel is invalid with error `unknown rel`.
Attestations, disputes and notes are ordinary claims with rel `about` and a type that says what they are.
`succeeds` is allowed only on `bean.succession`.
`purges` is allowed only on `bean.tombstone`.
`seals` is allowed only on `bean.seal`.
A record MUST NOT have two refs with the same rel and id.

### 3.8 Validation order

The fields `v`, `type`, `author`, `kid`, `id` and `sig` are the envelope. Their meaning and constraints do not change between record versions. A verifier of any version checks the envelope, the id, the signature and the canonical bytes before it reads `v`.

A verifier MUST check each file in this order.
At the first failure, the verifier reports the error string and stops checking that file.

1. The path matches `objects/<2 hex>/<62 hex>.json`. Error: `bad path`. The files and other entries are those of 7.1.
2. The file size is at most 16 MiB. Error: `too large`.
3. The bytes are valid UTF-8 and parse as one JSON object. No value is nested deeper than 256 levels: the record object is at depth 0, and a value inside an object or an array is one level deeper than it. Error: `bad json`.
4. The object has no duplicate keys at any depth. Error: `duplicate key`.
5. Every number is an integer in the range -(2^53 - 1) to 2^53 - 1, or a finite double that canonicalizes (section 4.1). An integer is a number whose canonical form has no `.` and no `e`. Error: `bad number`.
6. No string holds an escaped lone surrogate. Error: `bad string`.
7. `v` is present and is a non-negative integer. Error: `bad v`.
8. `type` is present and is a string. If `type` is not `bean.succession` or `bean.seal`, the file size is at most 1 MiB. Error: `bad type`, `too large`.
9. `author` and `kid` are present and are strings. Error: `bad field: author`, `bad field: kid`.
10. `id` is a string equal to the recomputed id and to the id from the path. Error: `id mismatch`.
11. `kid` parses as a supported tagged key. Error: `unsupported key algorithm` or `bad key`.
12. `sig` verifies for `kid` over the id bytes (section 4.5). Error: `bad signature`.
13. The file bytes are equal to the canonical bytes of the parsed object. Error: `not canonical`. Canonical bytes (4.1) do not depend on `v`.
14. If `v` is not a version that this verifier knows, apply section 8 and stop.
15. All required fields are present with the correct JSON types and constraints in 3.2 to 3.7. Error: `bad field: <name>`, where `<name>` is the dotted path of the failing field, for example `times.observed.start` or `value.parts`. When more than one field fails, which one is named is not normative. Conformance tests compare the name only for files with one bad field.
16. Apply the authority rules in section 6.

Steps 1 to 15 depend only on the file.
Step 16 depends on the set of files.

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

Number rules for bean:

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
| `secp256k1` | 32-byte BIP-340 x-only public key (the Nostr public key) | 64 | Allowed for signing keys and bare-key authors, so a Nostr key can sign records. Verifiers MUST support it. A recovery key is never secp256k1. |

A verifier MUST reject uppercase hex or a wrong length. Error: `bad key`. For `p256` and `secp256k1`, a verifier MUST also reject an invalid point with `bad key`; for `secp256k1`, the point is invalid when BIP-340 `lift_x` fails. For `ed25519`, a key is invalid with `bad key` when it is not the canonical encoding of a curve point (RFC 8032 5.1.3: y below p, and not x = 0 with the sign bit set), or when the point has small order ([8]A is the neutral point). This is the key check of libsodium. Vector V23 has these cases.
A verifier MUST reject any other tag. Error: `unsupported key algorithm`.

### 4.5 Signatures

The signed message is the 32 raw bytes of the record id hash.
Decode the 64 hex digits after `sha256:` to get these bytes.
The `sig` tag MUST be equal to the `kid` tag. Error: `bad signature`.

Ed25519:

- `sig` = `ed25519:` + 128 lowercase hex digits (the 64-byte RFC 8032 signature).
- Sign with RFC 8032 Ed25519 (pure, no prehash, no context).
- Verify as libsodium `crypto_sign_verify_detached` does. The key passed 4.4. Reject `S` ≥ L (the group order). Reject an `R` that decodes to a point of small order. Compute k = SHA-512(R || A || id bytes) mod L and R' = [S]B − [k]A, with no cofactor. The signature is good only when the 32-byte encoding of R' equals the R bytes of the signature. Vector V23 and conformance case 18d have the edge cases.

P-256:

- `sig` = `p256:` + 128 lowercase hex digits. This is `r || s`, each 32 bytes, big-endian (IEEE P1363 form).
- The ECDSA hash input is SHA-256 of the 32-byte message. Hardware signers hash the message, so this matches them.
- `s` MUST be at most n/2 (low-S). A signer MUST replace `s` with `n - s` when `s` is higher. A verifier MUST reject a high `s`.
- Software signers SHOULD use RFC 6979 deterministic nonces.
- Hardware signers MAY use random nonces. Most hardware keystores do not let the caller select RFC 6979. DEFAULT.

secp256k1:

- The signature is a Nostr event signature, so any Nostr signer (a NIP-46 remote signer, a NIP-07 browser extension) can sign a record.
- The wrapper event is `[0, <public key hex>, <created_at>, 29944, [["alt","bean record signature"]], <record id>]`. Its bytes are the NIP-01 serialization: a JSON array with no whitespace. Every field is ASCII, so nothing is escaped. The `alt` tag (NIP-31) gives a signer's approval prompt a readable summary.
- The signed message is the SHA-256 of the wrapper bytes (the Nostr event id). Sign with BIP-340 Schnorr.
- `sig` = `secp256k1:` + `<created_at>` + `:` + 128 lowercase hex digits (the 64-byte BIP-340 signature).
- `<created_at>` is a decimal integer from 0 to 2^53 - 1 with no leading zeros. It has no meaning to the verifier. Software signers SHOULD use 0. A remote signer MAY use another value.
- Kind 29944 is in the NIP-01 ephemeral range. Relays do not store a wrapper if someone publishes one.
- Kind 29944 is not in the NIPs registry. A NIP-46 signer can give permission for this kind only (`sign_event:29944`).

A signature is not part of the id.
Thus two valid files with the same id can have different `sig` bytes when a signer uses random nonces.
A replica MUST keep the first file it got for an id that passes steps 1 to 15 of 3.8. A replica MUST NOT replace that file during sync.

### 4.6 Recovery commitment

A commitment is `sha256:` + hex SHA-256 of the ASCII bytes of the tagged recovery public key string.
Example input: the 72 ASCII bytes of `ed25519:` followed by 64 hex digits.

### 4.7 Key derivation (HKDF)

All derivations use HKDF-SHA-256 (RFC 5869).
The salt is empty. RFC 5869 then uses 32 zero bytes.
The `info` value is the ASCII label below, with no terminator.

| Input key material | info label | Output length | Output use |
|---|---|---|---|
| paper seed (32 bytes) | `bean/v0/identity-secret` | 32 | identity secret |
| paper seed (32 bytes) | `bean/v0/recovery-key/<g>` | 32 | Ed25519 private seed of recovery key generation `g` |
| identity secret (32 bytes) | `bean/v0/blob-key` | 64 | AES-SIV key (section 4.9) |
| identity secret (32 bytes) | `bean/v0/dedupe-key` | 32 | HMAC key for dedupe refs (section 4.8) |

`<g>` is a decimal integer with no leading zeros, starting at `0`.
Generation 0 is committed in the genesis. Each recovery uses generation `g` and commits generation `g + 1` (section 5.5).

### 4.8 Dedupe ref

The dedupe ref is HMAC-SHA-256 with the dedupe key.
The HMAC input is the canonical bytes (4.1) of one JSON array.

If the source gives a native id:

    ["bean/v0/ref", "id", <source.system>, <native id as string>, <occurrence>]

If the source gives no native id:

    ["bean/v0/ref", "tuple", <source.system>, <type>, <times.observed>, <value>, <occurrence>]

Rules:

1. `<native id as string>` is the native id written as a JSON string. Write a numeric native id in decimal.
2. `<times.observed>` is the exact JSON value of `times.observed` in the record, or null.
3. `<value>` is the exact JSON object of `value` in the record.
4. `<occurrence>` is an integer that starts at 0.
5. In one adapter run, two readings are identical when their arrays, without the occurrence, have equal canonical bytes.
6. The first identical reading gets occurrence 0. The next gets 1, in source order.
7. Do not use the file hash. Do not use the row position.
8. When the source has a native id, an adapter MUST keep the HMAC input the same across adapter versions for the same source reading. A tuple input changes when a parser fix changes the value. Section 9.7 tells how to handle that case.
9. Write `source.ref` as `hmac-sha256:` followed by 64 lowercase hex digits. DEFAULT.

Thus two exports that overlap make the same refs for the same readings, and the second import adds nothing.

### 4.9 Blob encryption (AES-SIV)

A writer MUST encrypt every blob before it hashes and stores it.

1. Use AES-SIV as in RFC 5297 with AES-256. This is AES-SIV-CMAC-512.
2. Use the 64-byte blob key. The first 32 bytes are K1, the CMAC (S2V) key. The last 32 bytes are K2, the CTR key.
3. Use one associated data string: the ASCII bytes `bean/v0/blob`. DEFAULT.
4. Use no nonce. The encryption is deterministic.
5. The blob bytes are `V || C`, with the 16-byte synthetic IV `V` first.
6. The blob id is `sha256:` + hex SHA-256 of the blob bytes.

Equal plaintext gives equal blob bytes and an equal blob id for one identity secret.
Go has no AES-SIV in its standard library.
An implementation MAY use a vetted AES-SIV library or build CMAC (RFC 4493), S2V and CTR from `crypto/aes` and `crypto/cipher`.
Every implementation MUST match vector V11 (14). Code that builds AES-SIV from CMAC, S2V and CTR MUST also pass the RFC 4493 and RFC 5297 Appendix A vectors. A library that accepts only 64-byte keys cannot run RFC 5297 Appendix A, whose keys are 32 bytes. For such a library the gate is V11 plus the 512-bit-key groups of the Wycheproof AES-SIV-CMAC tests. The reference implementation builds AES-SIV, passes RFC 4493 and RFC 5297, and matches OpenSSL on V11.
Implementations MUST support blobs of at least 256 MiB. DEFAULT.

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
The CLI MUST tell the person that the words are for bean only.
DEFAULT.

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
4. Make a new random signing key. Use Ed25519 unless the person brings a Nostr key (secp256k1, from an nsec).
5. Write a genesis with `value.key` = the signing public key and `value.recovery` = the commitment to recovery generation 0.
6. Store the identity id, the identity secret and the signing private key in `local/`.
7. Erase the paper seed and the recovery private key from memory and disk.

The signing key is random or the person's own Nostr key. The paper seed does not derive it.
One host holds the signing key. A second device is paired as a bare device key (Extension 2). Copying the signing key between devices is not supported in v0.

### 5.3 Rotation

There is one kind of succession. It is signed by the recovery key that the parent commits to (5.5). A signing key cannot write a succession. Error: `bad succession`.

The person rotates when a device is lost, a key is old, or a device leaves. Rotation needs the paper seed. Thus a stolen device can add claims, but it cannot take over the identity, because it cannot write a succession.

### 5.4 Sealed rotation

A record signed by the head key can be valid.
A record signed by an older chain key is valid only when the succession that ends that key seals it. This applies to claims and to destructive records.
The succession that ends a key is the next node on the chain after the node that names the key.
Only that succession can seal records of that key.
`bean rotate` MUST NOT seal a record without a decision from the person. The person syncs before a rotation, so a thief's records can be in this replica, and a seal cannot be undone. If the replica holds any record of the outgoing key, and the person did not give `-seal-all`, the command MUST show those records, write nothing, and stop. `-seal-all` seals every record of the outgoing key. `-no-seal <id>` excludes one. An id that names no record of the outgoing key is an error.
A record of the outgoing key also seals the targets that it names and that this replica does not hold, unless the person excludes them. A record of any other author MUST NOT cause a seal. Otherwise a stranger's record could name a record that the thief signed earlier and delivers later. DEFAULT.
The maximum file size of a `bean.succession` or a `bean.seal` is 16 MiB. DEFAULT.

Seal continuations. One succession holds about 230,000 ids. When the seal set is larger, the writer puts the first ids in the succession and the rest in `bean.seal` records (3.3), parts 1 to n, and sets `value.parts` = n on the succession. The reference writer puts 200,000 ids in each record. The ids of all parts together, with the succession's own list, are the seal set of the succession. The order and the split do not change the set.

- A part is valid only when it names a succession on the chain, has the same `subject` and the same `kid` as that succession, and has `part` ≤ `value.parts` of the succession. Otherwise it is invalid. Error: `bad seal`. Thus only the recovery key that signed the succession can sign a part, and a part never moves to another identity or another rotation.
- When the named succession is not in R, the part is held. Report `held: <id>` as a warning. It has no effect.
- Two parts with the same `part` for one succession are both invalid. Error: `bad seal`. Only the seed holder can sign them.
- The seal set is complete when each part number from 1 to `value.parts` has exactly one valid part. An incomplete seal set seals nothing: every record of the outgoing key is `not sealed` until the missing parts arrive. Report `incomplete seals: <succession id>` as a warning. Nothing is erased (7.6 step 5), and the records are valid again when the parts arrive.
- A succession on a branch below a fork point is `frozen`. Its parts are `bad seal`.
- A tombstone or supersede record cannot name a part (6.6, 6.7).

A writer MUST write every part before the succession (5.5 step 8). Sync copies parts first (7.4). Thus a folder that has the succession nearly always has every part, and a folder that lacks some only fails closed.

`value.sealed` is an array of record ids.
The ids MUST be unique and sorted in ascending order of their ASCII bytes. Error: `bad field: value.sealed`.
An id in `sealed` that does not name a record of the outgoing key has no effect.

Thus every replica with the same files gets the same result. No trusted time is necessary.
A thief who steals a key cannot keep any record after a rotation, unless the person seals it. A retired key cannot add a valid record after the rotation, with any `times.observed`.
A key holds at most 200,200,000 records, the succession and 1,000 parts. `bean rotate` fails with `too large` above this limit (17).

### 5.5 Write a succession with the paper seed

The person rotates when a device is lost, a key is stolen, or a device leaves. Sync all devices first. Records of the outgoing key that are not synced are not sealed and become invalid (5.4).

1. Enter the 24 words on one device.
2. The parent is the chain head (6.3). Its position on the chain, counting the genesis as 0, is the recovery generation `g`.
3. Check that the identity secret derived from the words equals `local/identity-secret`, and that the commitment of recovery generation `g` equals the parent's `value.recovery`. Error: `seed does not match`.
4. If the identity is frozen (5.7), stop. Error: `compromised: <id>`.
4a. If the identity is too new (8.2), stop with `too new: <id>` (exit 3), except in this case: every record that makes the identity too new is signed by the head key, none by a recovery key and none is a genesis. Then evaluate the identity as if those records were ungated, apply step 4 to that result, and go on. The halting records are records of the parent key in this replica, so step 6 seals them unless the person excludes them. Before the first write, check that the folder with the new succession and parts is not too new for the identity and that the succession is its head; else stop with `too new: <id>` and write nothing. This happens, for example, when the folder has a record of an unknown `v` by recovery generation `g + 1`, the key that the new succession commits to. The succession retires the head key, so the halting records are ungated (8.2 condition 3) on every verifier, in any order of arrival.
5. Make a new signing key.
6. Write a succession with:
   - `refs` = `[{"rel":"succeeds","id":<parent id>}]`.
   - `value.key` = the new tagged public key.
   - `value.recovery` = the commitment to recovery generation `g + 1`.
   - `value.sealed` = the sorted ids of every record of the parent key in this replica, valid or not, except identity records, plus the target ids of the tombstones, supersede records and holds in this replica that do not name a record of another key, minus any the person excludes. An invalid or purged record is sealed too: if its tombstone dies, it comes back valid.
7. Sign with recovery key generation `g`. The `kid` is the recovery public key. This reveals the key.
8. Store the new signing key in `local/`. Write in this order: `local/rotation-pending` with the succession id, `local/signing-key.next` with the new key, the seal parts (5.4), the succession, then rename `local/signing-key.next` to `local/signing-key` and remove `local/rotation-pending`. A load that finds `local/rotation-pending` and no file for the succession it names removes the parts that name that succession and the note: the rotation is undone. A load that finds `local/signing-key.next` renames it when the chain head names its key, and removes it when no chain node names its key. Thus a crash at any point leaves a folder that can write, and a folder never has a succession without its parts from its own writer.
9. Erase the paper seed and the recovery private keys.

The identity secret does not change. See 17 for this limit.

### 5.6 Forks

A fork is two or more well-signed successions with the same parent.
Every succession is recovery-signed, so a fork means the same recovery key signed twice on different branches.
A fork freezes the identity (5.7). There is no tie-break rule.

A stolen signing key cannot fork an identity, because it cannot write a succession.

### 5.7 Freeze

Two valid successions with the same parent (both in N, 6.3 step 3) freeze the identity. A succession that fails 6.3 step 3 or 5.8 causes no freeze.
The same recovery key signed twice on different branches. This means the paper seed is in two sets of hands, or the person rotated on two devices without syncing.

For a frozen identity, a verifier MUST:

1. Report `compromised: <identity id>`.
2. Treat every succession below the fork point as invalid. Error: `frozen`.
3. Treat every destructive record of the identity as invalid. Error: `frozen`.
4. Treat every claim signed by a key that is not on the path from genesis to the fork point as invalid. Error: `frozen`.
5. Restore any target that a now-invalid tombstone hides (7.6).

A frozen identity cannot rotate. The person MUST make a new identity.
DEFAULT for items 2 to 4.

### 5.8 Key rules

The path of a succession S is the nodes from the genesis to its parent P. The keys of the path are the `value.key` of each node and the `kid` of each succession on it. The recovery commitments of the path are the `value.recovery` of each node. Commitments are as in 4.6.

A verifier checks S in this order:

1. If `S.value.recovery` is a recovery commitment of the path, S is invalid. Error: `recovery not advanced`.
2. S is invalid with `key reuse` when `S.value.key` is a key of the path, when the commitment to `S.value.key` is `S.value.recovery` or a recovery commitment of the path, or when `S.value.recovery` is the commitment to a key of the path.

A genesis G is invalid with `key reuse` when the commitment to `G.value.key` is `G.value.recovery`.

A recovery key of identity I signs only successions and seal parts of I. A record whose `author` is I and whose `kid` is a recovery key committed on the chain of I, and that is no succession or seal part, is invalid. Error: `key not in chain`. A record whose `author` is the recovery key itself is a bare-key claim like any other (conformance case 17c).

### 5.9 Identity secret

The identity secret is 32 bytes. The paper seed derives it (4.7).
It never appears in the log.
It derives the blob key and the dedupe key.
Each device of the person holds it in `local/identity-secret`.
If the identity secret leaks, the person MUST make a new identity and a new log, and re-encrypt all data into it.

### 5.10 Restore on a new host

A person who lost the host, but has the paper and a copy of `objects/` and `blobs/`, restores:

1. Sync every copy of the log that still exists into one folder. A copy that lacks a succession another device wrote makes a fork, and a fork freezes the identity (5.7).
2. Enter the 24 words. Find the genesis whose `value.recovery` is the commitment of recovery generation 0 of the words. Error: `input missing: genesis for these words`. A commitment is public, so anyone can copy it into a genesis of their own. If two or more genesis records match, stop. Error: `ambiguous: <ids>`. The person then names the identity id (`-identity`, 12.3), for example from an old export or a printout.
3. Write `local/identity-secret` from the words.
4. Write a succession from the chain head to a new signing key, as 5.5 steps 2 to 9 do, with the same seals.
5. Write `local/identity` last. Until it exists, the folder is not initialized, so a failed restore can run again.

If the identity is frozen, restore cannot rotate. It writes `local/identity-secret` and `local/identity`, fails with `compromised: <id>`, and writes no signing key. The copy can then read its blobs, `bean export` and `bean migrate`.

### 5.11 Custodians, agents and exit

This section adds no record type.

1. A custodian is a host that holds the signing key and the identity secret for a person, for example a hosted service. Section 5.2 applies to it. It is the one host of that identity.
2. A copy of a secret cannot be taken back. A custodian that hands over its key or the paper seed can keep a copy. Thus a handover is not an exit.
3. To leave a custodian, the person makes a new identity on a host that the person controls (5.2). The data is copied as new records that the new identity signs. The new records have new ids, because `author` is part of the hashed bytes (4.2).
4. After the exit, a former custodian or another service can still write about the person with its own key, as a device (Extension 2) or an agent (item 5). It never holds the secret of the new identity.
5. An AI agent writes with its own key (`bean agent`): a bare key or its own identity. It MUST NOT hold or use the signing key of a person. Its claims about a person have `author` = the agent key and `subject` = the person, so readers can see which records an agent wrote.
6. A frozen identity (5.7) uses the same exit as item 3.
7. The new identity MAY write one ordinary claim with rel `about` to the genesis of the old identity, to link the two. Each re-signed copy MAY have rel `about` to its original id. The claim type is a vocabulary term (9.9), not a `bean.*` type. The core gives these claims no special meaning. A dedicated exit record is not part of v0.

Restore always rotates. The old signing key stayed on the lost host and is not trusted.

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
If the commitment to `G.value.key` equals `G.value.recovery`, G is invalid with `key reuse` and starts no identity (5.8).
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

Step 5b. Collect seal sets.
For each succession Y on the chain, the seal set of Y is `Y.value.sealed` plus the `sealed` lists of the valid parts of Y (5.4). If the parts of Y are not complete, the seal set of Y is empty. Report `incomplete seals: <Y id>` as a warning.

Step 6. Resolve the signing key of each other record.
For each record D in R that is not an identity record:

1. If `D.author` is a tagged key, `D.kid` MUST equal `D.author`. Otherwise `D` is invalid. Error: `bad signature`. A bare key has no chain. Treat it as always the head key.
2. If `D.author` names no identity (no genesis in R, or one that starts no identity, 6.3), D is unresolved. Report `unresolved author: <id>` as a warning. Do not treat D as valid.
3. If `D.kid` is the `value.key` of a chain node at position p, record p for D.
4. Else if `D.kid` is the `value.key` of a node below a fork point, D is invalid. Error: `frozen`.
5. Else D is invalid. Error: `key not in chain`.

Step 7. Check seals.
For each record D that is not an identity record, with an identity author and position p:

1. If the node at p is the head, D passes this step.
2. Else let Y be the chain node at position p + 1.
3. If `D.id` is in the seal set of Y (step 5b), D passes this step.
4. Else D is invalid. Error: `not sealed`.

Records with a bare-key author pass this step.

Step 8. Apply the frozen rules in 5.7 to each frozen identity.

Step 9. Check supersede records (6.6).

Step 10. Check tombstones (6.7).

Compute step 10 before step 9: a purged supersede record does not supersede, and a purged target is not valid. A tombstone that names a supersede record is checked like any other tombstone.

Step 11. Check subjects.
For each valid claim D whose `subject` names no identity (6.3), report `unresolved subject: <id>` as a warning.
D stays valid. The subject check is a warning because a lab can write about a person whose genesis the replica does not hold.

Step 12. Find duplicates.
Group the valid records (purged targets are not valid) by (`subject`, `author`, `source.system`, `source.ref`) where `source.ref` is not null. Do not include a record that a valid supersede record names. A parser fix (9.7) keeps the ref of the record that it corrects. Without this rule, the verifier could report the fix as a duplicate of its own target.
In each group, the record with the lowest id is the primary.
Report the others as `duplicate: <id> of <primary id>`.
Duplicates stay valid. Indexes and readers SHOULD ignore them.

### 6.4 Validity is order-free

Each step uses only R and the results of earlier steps.
No step uses time or arrival order.
Two verifiers with the same F MUST report the same valid set, the same errors and the same warnings.

### 6.5 Claims from rotated-out keys

A claim signed by a rotated-out chain key is valid only when the succession that retired the key seals it (5.4).

### 6.6 Supersede rules

For each supersede record D, for each `supersedes` target T:

1. If T is not in R, D is held for this target. D is valid. Report `held: <D id>` as a warning.
2. If T is a `bean.*` record, D is invalid. Error: `bad target`.
3. If `T.author` is not equal to `D.author`, D is invalid. Error: `not author`.
4. Otherwise the target is superseded. Readers SHOULD show D in place of T. T stays in the log.
5. When two or more valid supersede records name the same target, readers SHOULD show the one with the lowest id. The verifier reports the others as `superseded by sibling: <lowest id>` (a warning). DEFAULT.
6. A tombstone of a supersede record removes the supersede. The target is shown again unless another valid supersede names it.

A supersede record can name more than one target. All targets MUST pass.

### 6.7 Tombstone rules

For each tombstone D with target T:

1. `D.author` equals `D.subject`. Step 15 of 3.8 already checks this (`bad field: author`, 3.3).
2. If T is not in R, or the author or subject of T is too new (8.2), D is held. D is valid. Section 7.5 tells how sync uses a held tombstone.
3. If T is a `bean.genesis`, `bean.succession`, `bean.seal` or `bean.tombstone`, D is invalid. Error: `bad target`.
4. If `T.subject` is not equal to `D.subject`, D is invalid. Error: `not subject`.
5. Otherwise D is a valid tombstone for T. T is purged.

A purged target is not valid data. A reader MUST NOT show it.
A purged target that is hidden in this replica gets the warning `hidden pending gc: <id>`, unless its author or subject is too new (7.6 step 6). `bean gc` erases it (7.6).

### 6.8 Trust is reader policy

The core checks signatures and the rules of this section. It does not rank authors.
A reader decides how much to trust a claim from another author: a device, an agent, a lab, a doctor or a peer. Trust levels and webs of trust are reader policy, not protocol.

### 6.8 What readers show

These rules change no valid set, error or verify line. They say what a reader shows the person, or an AI.

1. A succession that is not on a chain, and a held seal part, are not records of the identity. No step checks their keys. A reader MUST NOT show them, or list them as a reply to another record.
2. If the local identity is too new, a command that reads MUST stop with `too new: <id>` and exit code 3. It MUST NOT show an empty list. `bean export` still writes every file (7.9).
3. If the local identity is frozen, a command that reads MUST print its result, say on standard error that the identity is frozen, and exit with code 4. An MCP result has a `frozen` field.
4. For a frozen identity, these rules follow what every branch at the fork agreed on, because a person who restored from a stale backup keeps everything and must not see old values come back:
   - A correction that is `frozen` is shown in place of its target when a key on the path to the fork signed it, and either an earlier succession sealed it or every succession at the fork seals it. Otherwise the target stays, and its status names the correction.
   - A claim of the key at the fork is shown only when every succession at the fork seals it. A thief can still sign with that key. A claim of a key below the fork is `frozen` and is not shown.
   - A target of a `frozen` tombstone is not shown when an older key signed the tombstone, or at least one succession at the fork seals it. Any other tombstone has no effect here: otherwise one signature from a thief, with the key at the fork or a key below it, could hide the log. The bytes stay on disk.
   - A correction replaces its target only while the correction itself can be shown. A duplicate is shown when its primary cannot be. A reading must not vanish behind a record that is in doubt.
   - `bean ls -all` shows these records, marked `[in doubt]`. `bean export` writes them, and judges deletions without a record of a newer format by the key at the fork.

## 7. Folder layout and sync

### 7.1 Paths

| Path | Content | Synced |
|---|---|---|
| `objects/<h0h1>/<h2..h63>.json` | One record. `h` is the 64 hex digits of the id. | yes |
| `blobs/<h0h1>/<h2..h63>` | One blob (`V || C`). No file extension. | yes |
| `local/identity` | The identity id, ASCII, one line. | never |
| `local/identity-secret` | 64 lowercase hex digits, one line. File mode 0600. | never |
| `local/signing-key` | `<alg>:<hex private key>`, one line. File mode 0600. For Ed25519, the hex is the 32-byte seed. For secp256k1, it is the 32-byte secret key (the bytes of an nsec). | never |
| `local/holds.json` | Hold state (7.6). | never |
| `local/hidden/<64 hex>.json` | Hidden records (7.6). | never |
| `local/purged-refs.jsonl` | Purged dedupe refs of this replica (7.8). | never |
| `local/cache/` | Caches (7.10). | never |
| `local/lock` | The folder lock (7.11). Empty. | never |
| `local/signing-key.next` | The new signing key while a rotation runs (5.5 step 8). | never |
| `local/rotation-pending` | The id of the succession that a running rotation writes, one line. Written first, removed last. The next load finishes or undoes the rotation. | never |

Directories use mode 0700. Writers MUST write each file to a temporary name in the same directory and then rename it.
A sync tool MUST NOT copy `local/`.
Names that start with `.` under `objects/` and `blobs/` are not part of the log: temporary files (7.2) and what operating systems and sync tools leave (`.DS_Store`, iCloud `.icloud` stubs, `.stversions`). A verifier ignores them. Every other entry under `objects/` and `blobs/` is part of the log. A plain file one directory down is checked (3.8), and a wrong name is `bad path`. Anything else (a file directly in `objects/`, a symlink, a directory one level down, a device) is `bad path` and is not read. Paths outside `objects/` and `blobs/` are not part of the log.

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

1. Compute the valid set of the union of A's and B's record files (section 6). Judge B's files by their bytes on disk, not a cache.
2. Collect the valid tombstones of that union. A tombstone that only A's own view finds valid does not count.
3. For each record file in A that B does not have, `bean.seal` files first:
   1. If the file fails a step from 1 to 15 of 3.8, do not copy it. A file with an unknown `v` stops at step 14 and passes.
   2. If a valid tombstone of A or B names it, apply 7.5. Do not copy it.
   3. If B hides it under a hold, do not copy it.
   4. Otherwise copy it.
   5. If B holds a file for that id that fails a step from 1 to 15 of 3.8 and the file in A passes, replace B's file.
4. For each blob in A that B does not have, or that B has with a stored hash that does not match its path:
   1. If the SHA-256 of A's bytes does not match the path, do not copy it.
   2. If only purged or hidden record files name it, do not copy it. A record file names a blob in `blobs[].hash` or `value.blob`, valid or not. If a purged or hidden record file of the union names it, copy it only when a live record of the local identity or of a paired agent names it, by the rule of 7.9 step 2. Any record can name any blob.
   3. Otherwise copy it. If B has a corrupt copy, replace it atomically with A's verified bytes.
5. Compute the valid set of B again. Apply 7.6 to new tombstones.

Sync MUST copy files with an unknown `v`. Newer verifiers need them.
Sync copies a file that passes steps 1 to 15 also when section 6 finds the record invalid. Validity is a verifier result, not a sync rule. A file that fails steps 1 to 15 is not copied, and a stored copy that fails them is replaced. Thus a reformatted copy of a record (same id and signature, other whitespace or key order) cannot spread. A replica that got one heals at the next sync from a good copy.
Sync only from copies that you or the people you trust control (15).
A succession and a tombstone that it invalidates can arrive in the same sync. The target then comes back on the next sync (step 5). Two passes are sufficient.
Sync with rsync, git, Syncthing or a USB drive is allowed if the person then runs `bean verify` on each replica. Such tools do not apply the tombstone exception. `bean verify` then hides or erases the target (7.6).

### 7.5 Held tombstones

A tombstone can arrive before its target.
Then the tombstone is held (6.7 step 2).
When a target file arrives in a sync, check the tombstone with the target (6.7 steps 3 and 4) before the copy.

- If the tombstone is valid for the target, do not copy the target. Add the target dedupe ref to `local/purged-refs.jsonl` (7.8).
- If the tombstone is invalid for the target, copy the target. The tombstone becomes invalid.

Thus a tombstone from a stranger cannot block a record of another subject.

### 7.6 Delete hold

When a replica first finds a valid tombstone T for a target that it stores, also a hidden one:

1. If this replica wrote T (`bean purge`), erase the target at once.
2. Otherwise hide the target at once, if it is not hidden yet: move it to `local/hidden/`. Hidden records are still input to the local verifier (6.2) and never sync.
3. Record the receipt time in `local/holds.json` as `{"<T id>": {"target": "<target id>", "received": "<UTC time>"}}`.
4. A replica MUST NOT erase a hidden target on a timer.
5. `bean gc` erases a hidden target when a still-valid tombstone for it has a hold at least 7 days old, and drops every hold of that target. It then erases each blob that an erased target names, when no remaining record file names it. It never erases a blob that no erased record named. If the local identity is too new (8.2) or frozen (5.7), or any seal set is incomplete (5.4), `gc` erases nothing. Error: `too new: <id>`, `compromised: <id>` or `incomplete seals: <ids>`. Validity can change after an update or a fork, and erasure cannot be undone. Another too-new identity does not stop `gc`. Its records are never valid (8.2), and a tombstone whose target it wrote or is about is held (6.7). So it can only hold a tombstone of another identity, never make one erase.
6. `bean verify` reports each hidden target as `hidden pending gc: <id>` (a warning), unless its author or subject is too new (8.2).

A lost `local/holds.json` starts each hold again. Erasure is then later, never earlier.

When T becomes invalid while the target is hidden, restore the target.
T can become invalid after a rotation that does not seal T, or a freeze.
Restore means: move the target back to `objects/` and remove the hold entry.
After erasure, the replica cannot restore the bytes. Another replica that still holds them can sync them back, because T is now invalid.

Any command that loads the log (`verify`, `append`, `sync`, `export`, `rotate`, `purge`, `gc`) MUST apply this section.
The hold length is 7 days. DEFAULT. Nothing is erased without the person running `gc` or `purge`.

### 7.7 Purge

`bean purge <id>` does these steps:

1. Check that the local identity is the subject of the target.
2. Write a tombstone signed by the head key. The tombstone names only the target id. It MUST NOT carry the target's `source` or dedupe ref (15).
3. Add the target dedupe ref to `local/purged-refs.jsonl`.
4. Erase each blob that the target names, when no remaining record file names it.
5. Erase the target at once.

Blobs go before the target. A failure between the two then leaves the target and its tombstone, and 7.6 hides it. The other order leaves a blob that no record names.

The tombstone stays in the log forever.
A program MUST NOT tombstone a genesis, a succession or a tombstone.

### 7.8 Purged refs

`local/purged-refs.jsonl` has one line per purged target.
Each line is the canonical bytes of `["<subject>", "<source.system>", "<source.ref>"]`.
A replica writes a line when it hides, erases or does not copy a purged target that has a non-null `source.ref`.
`bean append` MUST skip a record whose (`subject`, `source.system`, `source.ref`) is in this file. A replica removes the line when the target comes back because its tombstone became invalid, unless another hidden record has that ref or a tombstone of that subject names a target that this replica does not hold. A line names only a ref, and an erased record cannot be checked, so the replica keeps the line when it is unsure.
Thus a new import after a purge adds nothing on this replica.
A replica that never had the target, or that lost this file, does not know the ref, and a new import of the same source data adds it again (17).

### 7.9 Export

`bean export <dir>` writes a plaintext copy of the log for exit.

1. Write each record file that is not purged or hidden to `<dir>/records/<64 hex>.json`. Use the stored bytes. This includes records of an unknown version and records of a too-new identity: an export needs no judgment of them.
2. Decrypt each blob that an exported record of the local identity or of an agent it paired names, and each blob that no record file names. Write the plaintext to `<dir>/blobs/<64 hex of the blob id>`. A blob that only another author's records name is not decrypted. A record that this build cannot judge (an unknown version, a too-new identity) counts as the local identity's only when its `kid` is a key of a node of that identity's tree or a recovery key a node commits to, and as a paired agent's only when its `kid` is its `author`. If a blob is not available, continue. If a record of the local identity or of a paired agent names it, add its id to `missing_blobs` in the manifest (sorted); otherwise count it in `missing_blobs_of_others`. One file MUST NOT stop an exit.
3. Write the stored bytes of each record file that fails a step from 1 to 15 of 3.8 to `<dir>/invalid/<its path>`, and list them in `invalid_records` as `{"path", "error"}`. A file whose path id is hidden, or is the target of any tombstone in the log, is not written: list it in `withheld_records` as `{"path", "reason"}`. List blobs that no record names in `unreferenced_blobs`, and stored blobs not decrypted in `not_exported_blobs` as `{"id", "reason"}`.
4. Write `<dir>/manifest.json` with the canonical bytes of `{"spec": "bean-v0", "identity": <id>, "records": <count>, "blobs": [{"id": <blob id>, "plaintext_sha256": <hex>, "media_type": <string>}]}`, plus the lists above when they are not empty. Sort `blobs` by `id`. The `spec` string names the export format. It does not change with the software version.

After it writes, the command fails with `too new: <id>` if the local identity is too new, or with `blob missing: <id>` if a blob is missing that the local identity or an agent it paired names. A blob that another author names does not fail it: anyone can name an absent blob.
The export is not encrypted. The CLI MUST say this before it writes.
`bean import-export <dir>` reverses the export for a log with the same identity secret. It reads `manifest.json` first and refuses an export of another identity. It needs only the `identity` field and reads the file as a stream. It applies 7.4 steps 2 and 3.2 to the union of the log and the export, so a record the person deleted does not come back. A blob that a deleted record names is not imported, unless a live record of the local identity or of a paired agent names it, by the rule of step 2.
It encrypts each blob again. The deterministic encryption gives the same blob ids. The reference CLI refuses symbolic links, special file entries, record exports over 16 MiB, a manifest over 4 GiB, and plaintext blob exports over 256 MiB. It confines file reads to the selected export directory. These are CLI resource limits, not core validity rules.

### 7.10 Caches

A cache is a file in `local/cache/`, for example an index.
A program MUST be able to delete it and rebuild it from `objects/` and `blobs/`. A lost cache costs only time.
The reference implementation has one cache: the record files that it already proved (`local/cache/objects`). `bean verify` ignores it.

### 7.11 Lock

Every command that writes to the folder, and `verify`, holds an exclusive lock on `local/lock` for its whole run. The reference implementation uses `flock`. Two commands on one folder thus never interleave: two rotations cannot fork the identity, and a purge cannot race a sync. A folder that cannot be written (read-only media) is read without the lock and without changes. The lock is local: it does not protect two hosts that write to one network or synced folder at once, and some network file systems ignore `flock`. The reference implementation has no cross-process lock on Windows; only one process at a time may use a folder there. These are known limits (17).

### 7.12 Write checks

A write MUST NOT return an id for a record that the verifier of the same folder rejects. A copy that has `local/` but no genesis yet can write. Its records are `unresolved author` until the genesis arrives, and `verify` says `own identity missing`. While the own identity has no chain, `gc` refuses and no hidden target is restored. `append`, `add`, `purge` and the agent writes refuse when the author identity is too new (`too new: <id>`), frozen (`compromised: <id>`), has an invalid genesis in this folder, or when the local signing key is not the chain head key (`retired key: ...`). `append` refuses identity records (`bad type`). `SignRecord` refuses strings that are not UTF-8 and numbers that are not finite (`bad string`, `bad number`).

## 8. Versioning

### 8.1 Known versions

This specification defines record version `v` = 0.
The format freezes at `v` = 0. After the freeze, no rule for a `v` 0 record changes.
A later change that alters the validity of an existing record needs a new `v` (8.4). That version MUST say how it treats `v` 0 records, and an identity with records of both versions.
A verifier knows a version only if it implements every rule of that version.

### 8.2 Unknown version

A verifier MUST NOT interpret a record whose `v` it does not know.

- If `type` starts with `bean.`, the affected identity is too new.
- Otherwise the verifier skips the record.

The record reached step 14 of 3.8, so its envelope, id, signature and bytes are good. The affected identity is:

1. For `bean.genesis`: the record's own id, when `author` = `kid`.
2. For other `bean.*` types: the `author` field I, when I has a genesis in R and `kid` is the head key of I or a recovery public key whose commitment (4.6) is the `value.recovery` of a node on the chain of I. The chain is followed from the genesis through nodes with one child; at a fork the fork point is the head (6.3 step 5). When the chain is the genesis alone, the head key is the genesis key.
3. Otherwise no identity is too new. If `author` is an identity with a genesis in R that is not too new, report `ungated: <id>` as a warning. Skip the record.

Only the head key or the seed holder can change the rules for an identity. A rotated-out key, a stranger or a forged file cannot halt it. There is no `too new: log`.
DEFAULT.

For a too-new identity, the verifier MUST:

1. Report `too new: <identity id>`.
2. Not evaluate any record whose `author` or `subject` is that identity, and not report errors or warnings for it. This includes records of other authors about it.
3. Not erase or hide anything for that identity.
4. Process all other identities as usual.

So an identity J that makes itself too new stops only the evaluation of records about J. Records of other authors with `subject` = J are not evaluated either. No other identity changes.

Commands that write for a too-new identity, or a record whose `subject` is a too-new identity, MUST stop with exit code 3. The one exception is a rotation past a halt by the head key (5.5 step 4a).

### 8.3 Unknown fields

At a known `v`, a verifier MUST keep unknown fields, include them in the hash and not interpret them (3.2).

### 8.4 Version change policy

A change that changes the validity of an existing record needs a new `v`.
A new optional field that verifiers can ignore does not need a new `v`.
A new `bean.*` type needs a new `v`.

## 9. Adapters

### 9.1 Contract

An adapter is a separate program. Its name is `bean-import-<source>`.
It reads one source file or folder. It writes JSONL to standard output.
`bean append` reads that JSONL, completes, signs and stores each record.

    bean-import-oura export.zip | bean append

An adapter MUST NOT sign records. `bean append` signs.
The core MUST NOT contain code for a specific source. Device formats are adapters.

### 9.2 Adapter output

Each JSONL line is a partial record. It MUST have `type`, `times.observed`, `source` and, for a reading, `value`.
It MAY have `context`, `refs` with rel `about` or `supersedes`, and `blobs`.
It MUST NOT have `v`, `nonce`, `author`, `kid`, `id`, `sig` or `times.imported`. `bean append` sets them.
If it has no `subject`, `bean append` sets the local identity id.

### 9.3 Blob first

1. Store the raw source file as a blob before any record (`bean blob put <file>` prints the blob id). DEFAULT.
2. Put `{"hash": <blob id>, "role": "raw-export", "media_type": <type>}` in `blobs` of each record from that file.

### 9.4 Granularity

An adapter MAY write one record per reading, or one record per (source, type, interval) with the readings in a blob series (`value` = `{"blob": <id>}`). High-frequency sources such as watch heart rate SHOULD batch. The interval and the series format are adapter conventions, not core. The interval is open (Appendix).
Each record has `source.system`, `source.adapter` as `name@version`, and a dedupe ref in `source.ref` (4.8).
The adapter does not hold the dedupe key.
The adapter writes the HMAC input array (4.8) as `source.ref_input`.
`bean append` computes `source.ref` from it and removes `source.ref_input` before it signs.
`source.ref_input` never reaches the log. DEFAULT.

### 9.5 Dedupe

`bean append` MUST skip a record when:

- the writer has a valid record with the same (`subject`, `source.system`, `source.ref`), or
- that triple is a purged ref (7.8), or
- an earlier line of the same input has that triple.

If the writer's records with that triple never had the `value` of the new record, the source changed the reading. A lab's corrected report, an older report, a parser bug and two sources that share an id all look like this, so a program MUST NOT replace the value by itself and MUST NOT skip it in silence. `bean append` MUST skip it, name it on standard error, and exit 1 after the rest of the input.

`bean append -corrections` is the person's word that the input is newer and right. It writes the new record with a `supersedes` ref to every current record of that triple, when the type is the same and the value is not empty (`{"n": null}`). Otherwise it reports the reading as before. A value that the triple has had is always skipped, so an old report cannot bring its value back. In one input, the first line for a triple decides.

`bean append` reports the count of added and skipped records, and how many of the added records are new values. DEFAULT.

### 9.6 Unknown values

An adapter MUST write null when the source has no value for a time.
An adapter MUST put an unparseable time in `context.raw_time` and set `times.observed` to null.
An adapter MUST NOT invent values.

### 9.7 Parser fixes

An adapter cannot know record ids. To correct a record, a partial record carries `source.supersedes_ref` = the HMAC input array (4.8) of the record it corrects. `bean append` resolves it to the valid record with that ref and the same subject and system, adds a `supersedes` ref, and removes `source.supersedes_ref` before signing. If no record matches, `bean append` skips the line and counts it as skipped.
When the fixed record keeps the same HMAC input, its dedupe ref is the same as the old ref. `bean append` MUST NOT skip a record that supersedes the existing record with the same ref. A tuple ref changes when the value changes. This is correct.

### 9.8 Adapters

The reference implementation has `bean-import-csv` and `bean-import-labcorp`. Adapters for Apple Health, Oura and Whoop are open (Appendix).
The first planned source is Whoop: JSON from the Whoop API, fetched on a schedule by a separate job (9.10), stored as a blob first (9.3), then parsed. Its field mapping and native ids come from real responses, not from memory.
Each adapter document MUST say which source field is the native id, or that the source has none.

### 9.9 Vocabulary and uncertainty

The type vocabulary is one document (`cmd/bean/vocabulary.md`): each `type`, its `value`, its unit and the suggested `context` keys. The verifier does not check it. A new term is added to the document before an adapter uses it. A term is never redefined: a changed meaning is a new term. An app MAY add a person's own terms from a file in the folder (the reference MCP server reads `vocabulary.md`). Such a file is not part of the log (7.1) and does not redefine a document term.

Measurement uncertainty travels with the data. `context.uncertainty` is optional. Its form is `{"kind": "cv" | "sd", "value": <number>, "unit": <string>, "source": <string>}`; `unit` and `source` are optional. `"cv"` is the coefficient of variation as a fraction. `"sd"` is the standard deviation in `unit`, or in the unit of the value when `unit` is absent. `source` names the calibration or the document that gives the value. Two kinds cover the calibration data that device makers publish. An adapter writes it only from a published or measured number.

### 9.10 Credentials

A tool that fetches from a source API reads its credentials from the environment.
Credentials MUST NOT be in a record, a blob, an adapter output line or a tool binary.
A fetch job MUST remove credentials and authorization headers before it saves a response for import.
A fetch job is a separate program, not part of the core (18).

### 9.11 Signed source files and multi-file sources

Both cases use the existing blob and claim rules, so the core needs no special case.

1. A source file that comes with a detached signature file (for example `scan.dcm` and `scan.dcm.sig`): store both as blobs. Put both in `blobs` of each claim from that file. Use role `raw-export` for the file and role `signature` for the signature file. The core does not check the detached signature. Device signing of records is Extension 2.
2. A source with many files (for example a hospital export): write one claim per source file, with that file as a blob. Normal dedupe (9.5) applies. There is no manifest record type.

## 10. Extension 1: computed results

Not part of the core and not implemented. The draft is in [docs/EXTENSIONS.md](docs/EXTENSIONS.md).

## 11. Extension 2: device capture

Not part of the core and not implemented. The draft is in [docs/EXTENSIONS.md](docs/EXTENSIONS.md).

## 12. CLI reference

### 12.1 General

The reference CLI is `bean`. It uses Go `flag` syntax.
Every command takes `-dir <path>`. The default is `$BEAN_DIR`, then the current directory.
Commands write results to standard output and errors to standard error.
An error line is `bean: <error string>`.
Text output writes control characters, line and paragraph separators and bidirectional controls from records and file names as `\xNN` or `\uNNNN`. So a record cannot fake an output line or control the terminal. JSON output (`ls -json`, `bean mcp`) uses JSON escapes.
Every command that loads the log applies the hold rules (7.6) first. A hidden target is restored only when every tombstone that hid it is invalid; a tombstone that is unresolved, held, too new or missing keeps it hidden.

Exit codes:

| Code | Meaning |
|---|---|
| 0 | Success. `verify` found no errors. |
| 1 | The command failed, or `verify` found one or more errors or an incomplete seal set (5.4). |
| 2 | Usage error: unknown command, bad flag or bad argument. |
| 3 | Too new. The local identity is too new (8.2), or a write names a too-new identity. For `verify`, other identities were still checked. Another too-new identity only gives its `too new` line. |
| 4 | Compromised. A local identity is frozen (5.7). |

If more than one code applies, use the highest. DEFAULT.

Verify output lines:

    error: <id or path>: <error string>
    warning: <id>: <warning string>
    too new: <identity id>
    compromised: <identity id>
    summary: <v> valid, <i> invalid, <s> skipped, <h> held, <d> duplicate

Lines are sorted by their ASCII bytes, with the summary line last.

`<id or path>` is the path of the file, relative to the log folder (`objects/ab/<62 hex>.json`), for an error from 3.8 steps 1 to 15, and the record id for an error from section 6.

The summary counts:

- `v`: records valid at the end of 6.3. Purged targets are not valid. Superseded records and duplicates are valid.
- `i`: error lines.
- `s`: files with an unknown `v` (8.2), plus the records of identities that are too new.
- `h`: held destructive records and held successions.
- `d`: `duplicate:` warnings.

A record with an unresolved author is in none of these counts.

### 12.2 Error strings

A conforming CLI MUST use these exact strings. Ids follow after `: ` where shown.

`bad path`, `too large`, `bad json`, `duplicate key`, `bad number`, `bad string`, `bad v`, `bad type`, `not canonical`, `bad field: <name>`, `id mismatch`, `unsupported key algorithm`, `bad key`, `bad signature`, `unknown core type`, `unknown rel`, `blob not listed`, `bad succession`, `key reuse`, `recovery not advanced`, `key not in chain`, `not sealed`, `bad seal`, `incomplete seals: <ids>`, `frozen`, `not author`, `not subject`, `bad target`, `too new: <identity id>`, `compromised: <identity id>`, `seed does not match`, `input purged: <id>`, `input missing: <id>`, `input invalid: <id>`, `old format`, `ambiguous: <ids>`, `retired key`, `not initialized`, `already initialized`, `blob missing: <id>`, `blob corrupt: <id>`.

Warning strings: `held: <id>`, `unresolved author: <id>`, `unresolved subject: <id>`, `duplicate: <id> of <id>`, `superseded by sibling: <id>`, `hidden pending gc: <id>`, `incomplete seals: <id>` (5.4), `ungated: <id>` (8.2), `blob missing: <id>` (for a purged or invalid record).

`incomplete seals: <ids>` in the error list is the error of `gc` (7.6 step 5). In `verify` it is a warning.

### 12.3 Commands

`bean init`

- Input: none, or an nsec on standard input with `-nostr -`.
- Flags: `-nostr new` makes a new secp256k1 signing key. `-nostr <file>` uses the nsec (`nsec1...` or 64 hex digits) in the file. `-nostr -` reads the nsec from standard input.
- Action: section 5.2.
- Output: `identity <id>`, then `npub <npub>` for a secp256k1 key, then the 24 words on 24 numbered lines. Then a warning: write the words on paper, and use them for bean only.
- Errors: `already initialized` (exit 1).

`bean append`

- Input: JSONL partial records on standard input (9.2).
- Action: fill `v`, `nonce`, `author`, `kid`, `times.imported`, and `subject` if absent. Compute `source.ref` from `source.ref_input`. Check dedupe (9.5). Check authority for supersedes (6.6). Sign and write.
- Output: `added <n>, skipped <m>`.
- Errors: `bad json` with the line number, `not author`, `not initialized`. On the first error, stop. Records before the error stay written.

`bean blob put <file>`

- Action: encrypt and store the file (7.3).
- Output: the blob id.

`bean verify`

- Action: section 6 and the hold rules. Check every blob hash (`blob corrupt: <id>`) and every named blob (`blob missing: <id>`, a warning when the record is purged). When `local/identity` names an identity with no chain in the folder that is not too new, add `error: <id>: own identity missing: sync from another copy` (exit 1). The line is not counted in `i`.
- Output: 12.1 verify lines.

`bean rotate [-list] [-seal-all] [-no-seal <id>]... [-no-seal-file <file>] [-nostr new|<file>]`

- Input: the 24 words on standard input. Numbering and case are ignored. `-nostr` selects a new secp256k1 key, as for `init`. The nsec comes from a file, because standard input carries the words.
- Action: sections 5.4 and 5.5. Without `-seal-all`, if this replica holds a record of the outgoing key, show the deletions, corrections, pair claims and claims that replace a duplicate in full, with every record that they name, and a summary of the claims on standard error, do not read the words, write nothing, and exit 1. `-list` shows every record, writes nothing, and exits 0. `-seal-all` seals every record of the outgoing key in this replica except the ids that `-no-seal` or the lines of `-no-seal-file` name.
- Output: one line `sealed <id>` or `not sealed <id>` per record of the outgoing key, then one line `seal part <id>` per seal continuation (only above the limit of one succession), then one line `past halt <id>` per record that halted the identity (5.5 step 4a), then `rotated to <tagged key>`, then `sealed <n>`. Before reading input, print `sync all devices before you rotate` to standard error.
- Errors: `undecided: ...`, `input missing: <id> is not a record of the key you are replacing`, `ambiguous id: <id>`, `seed does not match`, `compromised: <id>` (exit 4), `key reuse`.

`bean restore [-list] [-seal-all] [-no-seal <id>]... [-no-seal-file <file>] [-nostr new|<file>] [-identity <id>]`

- Input: the 24 words on standard input.
- Action: section 5.10. `-identity` picks one identity when the words match more than one genesis.
- Output: `identity <id>`, then the `rotate` output.
- Errors: `already initialized`, `undecided: ...`, `seed does not match`, `input missing: genesis for these words`, `ambiguous: <ids>`, `compromised: <id>` (exit 4), `key reuse`.

`bean migrate [-also <id>]... [-also-file <file>] [-nostr new|<file>] <empty folder>`, `bean migrate -list`

- Action: only for a frozen local identity, also past a record of a newer format by a key at the fork. Make a new identity in the folder. Write each record of the person that `ls` shows as a new record of the new identity: signing with the new key says that the person stands behind it, so an agent's, a stranger's and a claim in doubt do not move. `-also <id>` and `-also-file <file>` also move a claim of the person in doubt, for example what they logged after the fork. Two moved records never share a ref: what `ls` shows goes first. `-list` shows what stays behind and writes nothing. A reader takes the time of a moved record with no `times.observed` from `context.migrated_imported`. Encrypt its blobs again under the new key. Put the old id in `context.migrated_from` and the old `times.imported` in `context.migrated_imported`. If one side of the fork corrected the record, put the corrections in `context.migrated_disputed_by`. Keep `about` refs between moved records, and name a link that cannot move in `context.migrated_about`. Nothing links the two identities by signature: after a fork, two parties hold the seed.
- Output: `identity <id>`, the 24 new words, then `migrated <r> records, <b> blobs, <m> blobs missing`, then lines that count what is disputed and what it left out.
- Errors: `not initialized`, `not frozen: ...`, `migrate folder not empty`, `too new: <id>` (exit 3).

`bean whoami`

- Output: `identity <id>`, `key <tagged key>`, and `npub <npub>` for a secp256k1 key.

`bean gc`

- Action: section 7.6 step 5.
- Output: `erased <r> records, <b> blobs`.

`bean blob get <id>`

- Action: decrypt one blob to standard output.
- Errors: `blob missing: <id>`, `blob corrupt: <id>`.

`bean purge [-reason <text>] <id>`

- Action: section 7.7.
- Errors: `not subject`, `bad target`, `input missing: <id>` when the target is not in the replica.

`bean sync <source dir>`

- Action: section 7.4 into the `-dir` log.
- Output: `copied <r> records, <b> blobs, refused <p> purged`, then `replaced <n> bad copies` when n > 0.

`bean export <dir>`

- Action: section 7.9. `<dir>` MUST be empty or absent.
- Output: `exported <r> records, <b> blobs (plaintext)`.

`bean import-export <dir>`

- Action: section 7.9 reverse. The log MUST have the same identity secret.
- Output: `added <r> records, <b> blobs`.

`bean-import-<source> [flags] <file>`

- Output: JSONL partial records (9.2) on standard output.
- `bean-import-csv -type <type> -system <system> -blob <blob id> [-unit <unit>] <file.csv>` reads a header row. It uses column `value`, optional column `time` (a time of 3.5; any other text goes to `context.raw_time`) and optional column `id` (native id). Get `<blob id>` from `bean blob put <file.csv>` first.

Test clock: the reference CLI reads the current time from `$BEAN_NOW` (RFC 3339) when it is set. Tests use it for holds and `times.imported`. DEFAULT.

## 13. Conformance tests

Each test starts from empty folders unless the setup says otherwise.
"Verify passes" means exit code 0 and no `error:` lines.
A conforming implementation MUST pass every test.
The tests are numbered in the order they were added. The tests of the extensions are in docs/EXTENSIONS.md.

### Tamper

1. Record byte flip.
   - Setup: init log A. Append 3 claims.
   - Action: change one byte inside the `value` of one record file.
   - Expected: verify exits 1. It prints an error for that path (`not canonical`, `bad json` or `id mismatch`).
2. Blob byte flip.
   - Setup: log A with one blob and one record that names it.
   - Action: change one byte of the blob file.
   - Expected: verify exits 1 with `blob corrupt: <blob id>`.
### Sync and dedupe

4. Union of two folders.
   - Setup: logs A and B of one identity (B has a copy of `local/`). Append different claims to each.
   - Action: sync A to B and B to A.
   - Expected: both have the same `objects/` and `blobs/` file sets. Verify passes on both.
5. Same export on a second device.
   - Setup: A and B as in test 4, synced. Export file E.
   - Action: import E on A. Sync A to B. Import E on B.
   - Expected: the second import prints `added 0`.
6. Overlapping exports.
   - Setup: file E1 has readings 1 to 10. File E2 has readings 6 to 15. Run the test twice: once with native ids and once without (tuple refs). Include two identical readings in the overlap.
   - Action: import E1, then E2.
   - Expected: the log has 15 readings plus the identical duplicate reading, so 16 records. The second import adds only the readings 11 to 15.

### Supersede

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

### Tombstone

11. Propagate.
    - Setup: synced replicas A and B of identity I. Claim T on both.
    - Action: purge T on A. Sync A to B. Set `BEAN_NOW` 8 days later on B. Verify B.
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

### Rotate and recover

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
    - Action: rotate with `-seal-all`.
    - Expected: the succession lists D in `value.sealed`. D stays valid.
21. Double rotation freeze.
    - Setup: I at head H on replicas A and B.
    - Action: rotate on A and on B without syncing. Sync.
    - Expected: verify reports `compromised: <I>` and exits 4. Both successions and every destructive record of I are `frozen`. Claims of the fork-point key, and sealed claims of older trunk keys, stay valid. `rotate`, `append` and `gc` on a frozen identity fail with `compromised: <I>`.
22. Recovery key cannot sign claims.
    - Action: write a claim whose `kid` is a recovery public key of I.
    - Expected: `key not in chain`.

### Version

23. Unknown version, ordinary type.
    - Action: add a file with `v` = 99 and type `hrv.rmssd`.
    - Expected: verify passes with the file counted as skipped. Sync copies the file.
24. Unknown version, core type, per identity.
    - Setup: identities I and J in one log.
    - Action: add a `bean.succession` file with `v` = 99 and `author` = I.
    - Expected: `too new: <I>`. Exit 3 on a replica of I. Exit 1 on a replica of J, for the file error. All records of J are still checked. A file error in J is still reported.

### Delete hold

25. Rotation within the hold restores.
    - Setup: replicas A and B. A thief with I's head key writes tombstone D for T on A. Sync A to B. B hides T.
    - Action: on day 3 (`BEAN_NOW`), rotate on A with `-no-seal D`. Sync A to B.
    - Expected: D is `not sealed`. B moves T back to `objects/` and removes the hold. `gc` on day 30 erases nothing.
25b. No erase without gc.
    - Setup: as test 11 after the sync to B.
    - Action: set `BEAN_NOW` 8 days later. Run `verify` on B, then `gc`.
    - Expected: `verify` warns `hidden pending gc: <T>` and erases nothing. `gc` erases T and prints `erased 1 records`.
44. Stranger too new.
    - Setup: identity I. A stranger's genesis with `v` = 1 is synced into I's log.
    - Expected: verify on I exits 0 and prints `too new: <stranger>`. `gc` runs.
31. Too-new file without a chain signature.
    - Setup: identities I and J.
    - Action: add a `bean.succession` file with `v` = 99, `author` = I, signed by J's key.
    - Expected: no `too new` line. The file is skipped.

### Retired keys, duplicates and erasure

32. Retired key claim.
    - Setup: I appends claim T. I rotates. A copy of the old key writes claim D.
    - Expected: T is sealed and valid. D is `not sealed`. `append` on a replica that still holds the old key fails with `retired key`.
33. Duplicates per author.
    - Setup: I appends claim T with ref R. A paired agent key writes claim D about I with ref R and an id lower than T.
    - Expected: T and D are both current. Neither is a duplicate.
34. gc fails closed.
    - Setup: a log with a blob B named by claim T. Add a `v` = 99 core record of I. In a second log, fork I.
    - Action: `gc` on each.
    - Expected: `too new: <I>` and `compromised: <I>`. B stays on both.
35. Ambiguous restore.
    - Setup: a stranger genesis that copies I's recovery commitment.
    - Action: `restore` with I's words, then `restore -identity <I>`.
    - Expected: `ambiguous: ...` and no `local/` files. Then the restore of I passes.
36. Loose blob.
    - Setup: `blob put` a file. Append and purge an unrelated claim.
    - Expected: the blob stays.
37. Concurrent rotation.
    - Action: start four `rotate` runs on one folder at once.
    - Expected: one chain of length 5. No fork.
38. Crash during rotation.
    - Action: stop `rotate` after the succession is written and before `local/signing-key` changes. Run `verify`.
    - Expected: the new key is in `local/signing-key`. `local/signing-key.next` is gone. `append` passes.

### Seal continuations

39. Complete set.
    - Setup: I appends more claims than one record holds (tests MAY lower the chunk). I rotates.
    - Expected: one succession with `value.parts` = n and n parts. Verify passes. Every claim is valid.
40. Missing part.
    - Setup: test 39 with one part removed.
    - Expected: `incomplete seals: <succession id>`. Every claim of the old key is `not sealed`. `gc` fails with `incomplete seals`. When the part comes back, every claim is valid.
41. Duplicate and forged parts.
    - Setup: test 39 plus a second part 1 signed by the recovery key, and parts signed by a signing key, by another identity's recovery key, by the next recovery key, with a part number above n, and with a claim as target.
    - Expected: the forged parts are `bad seal` and change nothing. The two parts 1 are `bad seal` and the set is incomplete.
42. Out of order.
    - Setup: sync the succession first, then the parts one by one.
    - Expected: incomplete until the last part arrives, then the same result as test 39. Parts without their succession are held.
43. Crash during rotation with parts.
    - Action: stop `rotate` after some parts, and after the succession. Kill a rotation at random moments many times.
    - Expected: each folder verifies with exit 0, with the old or the new key. No part without its succession is left. `rotate` passes again.

### Exit and devices

26. Export round trip.
    - Setup: log A with claims, a blob and a tombstone.
    - Action: export to X. Make log B with the same identity secret and keys. Run `import-export X` on B.
    - Expected: B has the same record files and the same blob ids as A, minus the purged target. Verify passes on B. `manifest.json` lists every blob with its plaintext SHA-256.
### Edge cases

30. Order independence.
    - Setup: any log from tests 17 to 22.
    - Action: copy the files into a new folder in a random order 10 times. Verify each.
    - Expected: identical verify output each time.

## 14. Test vectors

Each vector is a JSON file in `vectors/` with inputs and expected outputs, or a folder in `vectors/conformance/` (V24).
Two independent implementations MUST produce each value before the vector is published.
The independent checks are `tools/checkvectors.py`, `tools/checksecp256k1.py` and the second verifier `tools/second/beanverify.py` (V24).

| # | Vector | Inputs | Expected output |
|---|---|---|---|
| V1 | JCS numbers | The RFC 8785 number samples, plus -0, 2^53 - 1, 2^53, 1e21, 1e-7 | Canonical strings or `bad number`. Published. |
| V2 | JCS key order | Keys with non-ASCII and astral characters | Canonical bytes. Published. |
| V3 | JCS strings | Control characters, `/`, U+007F, a lone surrogate escape | Canonical bytes or `bad string`. Published. |
| V4 | Record id | Three full records (genesis, claim, tombstone) | Canonical bytes without `id` and `sig`, and the id. Published. |
| V5 | Ed25519 signature | A fixed private seed and a record id | `kid`, `sig`. Published. |
| V6 | P-256 signature | A fixed private key, a record id, a low-S and a high-S signature | Accept and reject results. The low-S signature uses an RFC 6979 nonce, so it is reproducible (regenerated 2026-10-30). Published. |
| V7 | Commitment | A tagged recovery public key | Commitment id. Published. |
| V8 | Paper seed | 32 fixed entropy bytes | The 24 words. Published. |
| V9 | HKDF | The V8 seed | Identity secret, recovery generations 0, 1 and 2 (tagged public keys). Published. |
| V10 | Derived keys | The V9 identity secret | Blob key (64 bytes) and dedupe key (32 bytes). Published. |
| V11 | AES-SIV | The V10 blob key and 3 plaintexts (empty, 1 byte, 1 MiB pattern) | `V || C` and the blob id. Published. |
| V12 | Dedupe ref | The V10 dedupe key, one native-id input, one tuple input, occurrences 0 and 1 | `source.ref` strings. Published. |
| V17 | Bad files | One file per error string in 3.8 | The error string. Published. |
| V19 | Retired-key seals | A fixed identity, a rotation that seals some claims, and claims of the retired key | Valid or `not sealed` per record. Published. |
| V20 | Duplicates per author | Records of two authors with one ref | The duplicate lines. Published. |
| V21 | Seal continuations | A fixed identity, a succession with 2 parts, a duplicate part, a forged part and a part for a missing succession | Valid, `bad seal`, `not sealed`, held or incomplete per record and per scenario. Published. |
| V22 | Too-new gate and going past a halt | One identity with records of `v` = 99 by the head key, a retired key, recovery keys 0 and 1 and a stranger, with and without a rotation | Too-new identities, `ungated` ids, valid ids, and the halting records that 5.5 step 4a passes, per scenario. Published. |
| V18 | secp256k1 | A fixed secret key and a record id, created_at 0 and 1761566400, zero aux | `kid`, npub, nsec, wrapper bytes and id, `sig`, and five rejects. Published. Checked by `tools/checksecp256k1.py`. |
| V23 | Ed25519 edge cases | A good signature, `S` = `S` + L, the small-order identity key with one signature for two ids, a non-canonical encoding of it, and a key that is not a point | `bad key` or not, and accept or `bad signature`, as 4.4 and 4.5 say. Published. Conformance case 18d adds a small-order `R` under a mixed-order key. |
| V24 | Conformance folders | Folders of record files in `vectors/conformance/`, written by `tools/second/cases.py`: nesting depth, empty arrays, byte and code point lengths, whole-string patterns, held and not-sealed order, too-new identities, recovery keys as signing keys, keys and recovery commitments that come back on a path, Ed25519 keys, unknown `v`, old types | `expected.txt`: the verify lines (12.1) and the exit code of a replica with no local identity. Published. |

Hand-built AES-SIV code MUST also pass the RFC 4493 and RFC 5297 Appendix A vectors (4.9). The HKDF code MUST pass the RFC 5869 Appendix A vectors.

## 15. Security considerations

- Key theft. A thief with the head key can add claims, supersede and tombstone. Recovery with the paper seed ends this. Every record of the thief that the person does not seal becomes invalid, and the retired key cannot add valid records after the rotation. Records the thief wrote and the person synced before the rotation are in the list that the rotation shows. A seal cannot be undone, so nothing is sealed without the person's decision (5.4).
- Forged identity records. No step checks the key of a succession that is not on a chain, so any key can write one that names the person as its author. It stays valid in the verify output, as a held record. A reader MUST NOT show it as the person's (6.8).
- Paper seed theft. The seed holder can recover and take the identity. If the owner also recovers from the same parent, the identity freezes. There is no way back from a freeze in v0.
- Seed exposure. Devices never store the seed. `init`, `rotate` and `restore` MUST erase it from memory after use, as far as the language allows.
- No trusted time. Validity never uses a clock. Only the delete hold uses the local clock, and it never changes validity.
- Stranger tombstones. A tombstone is checked against its target before sync refuses the target (7.5).
- Signature malleability. P-256 requires low-S. A replica keeps the first file it gets for an id (4.5).
- Denial of service. A replica MUST limit record size (3.1). Sync is a union, and every file of a source is input to every later command. So a source can fill the disk and the memory of a replica. One identity can write any number of records and 1,000 seal parts of 16 MiB. Sync only from copies that you or the people you trust control.
- Parser safety. JSON parsing MUST reject duplicate keys and bad numbers before any check uses the value.
- Equal blobs. Deterministic encryption shows when two blobs of one identity hold the same plaintext (16).
- Local files. `local/` holds the identity secret and the signing key. File mode MUST be 0600. Backups of `local/` are as sensitive as the keys.

## 16. Privacy considerations

- Linkability. The identity id appears in every record and every export. Recipients of two exports can link them. Per-recipient pseudonymous subjects are deferred (18).
- Equality leak. AES-SIV is deterministic. Anyone who sees two blobs can tell if their plaintexts are equal. The same is true for dedupe refs in one identity. This leak is the cost of dedupe and of a sync that gives the same result when it runs again.
- Metadata. Record files are plaintext JSON. `type`, `times`, `value` and `context` are visible to anyone with the folder. Only blobs are encrypted. Encryption of records for transport is a sharing extension (18).
- Identity secret exposure. Whoever has the identity secret can decrypt all blobs and test guesses against dedupe refs. Rotation does not change it. The fix is a new identity and a new log (17).
- Export. `bean export` writes plaintext. The person must protect the export folder.
- Tombstones. A tombstone shows that a record with that id existed. It does not show the content.

## 17. Known limits

1. Rotation needs the paper seed. A person who loses the paper and the device loses the identity. This limit is the cost of the rule that a stolen device cannot take over the identity.
2. Records are plaintext on disk. Only blobs are encrypted. Folder-level encryption is the sync tool's job in v0.
3. One key holds at most 200,200,000 records (5.4). A rotation of more than 200,000 records writes seal continuations.
4. An identity secret leak needs a new paper seed, a new identity and a new log. The old data must be re-encrypted into the new log.
5. A tombstone does not carry the dedupe ref of its target (7.7). A replica without the `local/purged-refs.jsonl` of the purging replica brings purged data back when it imports the same source data again. Back up that file with the log.
6. Two hosts that import the same source before they sync make duplicate records. Section 6.3 step 12 marks them. They stay in the log.
7. Tuple dedupe refs merge two real readings with the same type, observed time and value in two different exports when the occurrence counts match.
8. After erasure, a replica cannot restore a target from its own disk.
9. A frozen identity cannot recover. `bean migrate` starts a new identity from what the frozen one shows. Nothing links the two by signature.
10. A record of the head key that the rotating copy does not hold is not sealed, so the rotation makes it invalid (5.4).
11. The folder lock is local (7.11).
12. Every command reads and checks the whole log. Time and memory grow with the number of records: about 1 s and 0.2 GB of live heap at 50,000 records on a laptop-class machine. Batch dense series (9.4).
13. A thief with the head signing key can halt the identity with one `bean.*` record of an unknown `v` (8.2) until the person rotates with the paper seed (5.5 step 4a). Until then every copy that has the record refuses writes for the identity. When the fork point of a frozen identity signs the halt, `verify` reports `too new`, not `compromised`; `rotate` reports `compromised`.

## 18. Out of scope

- Computed results in the core (Extension 1 holds them).
- `assurance`, `kind`, `prev`, `quality`, `coding` and `times.recorded` fields.
- `attests` and `disputes` refs. Use `about` and a type.
- A blessed series format.
- A type registry. Type conventions are plain documents.
- Per-recipient pseudonymous subjects (deferred).
- Grants, sharing, selective disclosure and zero-knowledge proofs. Version 0 serves one person first. Direction for later: encrypt a share to the recipient's key, with no central password-gated link.
- Encryption of records at rest. Use operating system disk encryption or the sync tool. It needs key management that version 0 does not have.
- Remote signing (a NIP-46 style signer for the signing key). One host per identity keeps version 0 simple. The Nostr wrapper (4.5) lets a NIP-46 signer sign, but bean does not require one.
- Standing shares to research parties.
- Trusted time, witness records and global ordering.
- Multi-signature thresholds, delegation and witnesses as in KERI.
- A database in the core. Indexes are rebuildable caches (7.10).
- Fetching from source APIs inside the core. A fetch job is a separate program (9.10).
- Work on bean as a formal standard before the personal use case works.
- Nostr and Blossom transport (a later extension). `secp256k1` signing keys are in the core. Publication of records to relays is not.
- User interface, servers and accounts.
- Applications, for example comparison tools, device apps and clinician views.

## 19. Governance and license

- This specification is CC0 1.0.
- The reference code is Apache-2.0.
- No person or company owns bean. Where the repository lives and who merges changes are open ([docs/GOVERNANCE-PROPOSAL.md](docs/GOVERNANCE-PROPOSAL.md)).
- A change to this specification needs a written change note. If a change alters a result, it also needs new test vectors.

## Appendix: defaults and open items

Each item has a DEFAULT in the text, or is open. A DEFAULT can change until the format freezes (CORE.md section 6).

Decided:

1. Signing keys: Ed25519 is the default. P-256 is for hardware keys in devices. secp256k1 BIP-340 keys sign through a Nostr wrapper event, so NIP-46 and NIP-07 signers can sign. The wrapper uses kind 29944 and a NIP-31 `alt` tag.
2. Records have their own format. Nostr can carry them later, encrypted. The core has no NIP-01 event shape.
3. Every succession is signed with the paper seed. Any fork freezes the identity (5.6, 5.7).
4. The hold before erasure is 7 days (7.6). The hold controls `gc`. A timer does not erase anything.
5. One host holds the signing key. Other devices use bare device keys (Extension 2). Devices do not share a signing key.
6. Measurement uncertainty is `context.uncertainty` (9.9).
7. The format freezes at `v` = 0 (8.1).

Defaults:

8. One identity secret for each person. Keys for each blob come with the sharing extension.
9. Record size limit 1 MiB, succession and seal part size limit 16 MiB, 1,000 seal parts per succession, blob minimum 256 MiB (3.1, 4.9).
10. The `type` pattern and the precision values (3.2, 3.5).
11. The AES-SIV associated data label (4.9) and the `hmac-sha256:` ref prefix (4.8). An implementation can use a tested AES-SIV library. The reference implementation builds AES-SIV and agrees with OpenSSL.
12. The paper seed as 24 BIP-39 English words over 32 bytes (4.10).
13. The order of exit codes (12.1).

Open:

14. Native id fields for Apple Health, Oura and Whoop. Each adapter decides from a real export file, never from memory.
15. Batch interval and series format for sources with many readings (9.4).
