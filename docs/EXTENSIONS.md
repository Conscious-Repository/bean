# bean extensions (draft, not implemented)

These two extensions were part of SPEC.md up to v0.5. No code implements them, so they are not part of the core. `comparison.percentile` is not an Extension 1 result: it has its own method ([COMPARE.md](COMPARE.md)). They come back into the specification only with an implementation and test vectors. The section numbers are the old SPEC.md numbers.

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

    ["bean/v0/method", <id>, <version>, <sha256 hex of the method spec text>, <sha256 hex of the conformance vector file>]

DEFAULT.

### 10.3 Replay

Replay of result D:

1. Find the method with `D.method.id` and `D.method.version`. If none, fail with `method unknown`.
2. Compute the method hash. If it differs from `D.method.hash`, fail with `method mismatch`.
3. For each id in `D.inputs`, in order:
   1. If a valid tombstone names it, or the replica hides it, fail with `input purged: <id>`.
   2. If the record is not in the replica, fail with `input missing: <id>`.
   3. If the record is not valid, fail with `input invalid: <id>`.
4. Run the method on these inputs only, with `D.params`. Do not query the log for other records.
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

### 10.5 Application tests

Tests of applications that use computed results, for example a comparison of a 72-hour and a 28-day average, are not core tests.

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

To unpair, the subject supersedes the pair claim with `value.key` = the same key and `value.status` = `"unpaired"`. DEFAULT.
The core does not check pairing. A reader decides by its own policy whether to trust a device record.

### 11.4 Capture

The device signs each record when it captures the reading.
`times.observed` uses the device clock.
`times.imported` is the device clock time when it signs. DEFAULT.
`source.ref` is null. The device does not hold the identity secret. The record id is the dedupe key.
A device record MUST NOT have `blobs`. The device cannot encrypt with the blob key.
To keep raw device evidence, the host stores the blob and writes its own claim with rel `about` to the device record. DEFAULT.

### 11.5 Clock record

When the host imports device records, the host writes one claim:

- `type` = `device.clock`
- `author` = `subject` = the identity id
- `value` = `{"key": <device key>, "device_time": <device clock reading>, "host_time": <host clock reading>}`

Both readings are RFC 3339 UTC strings with millisecond precision.
The host reads both clocks within one transfer session.
Apps use this record to correct clock drift.
Do not change the original times in device records.

### 11.6 Transfer format

The device gives the host a JSONL file of signed records and one `clock.json` with `{"key": <device key>, "device_time": <time>}`.
`bean import-device <dir>` checks each signature, writes each record file unchanged, then writes the `device.clock` claim.
DEFAULT.

## Commands (old SPEC.md 12.3)

`bean pair <tagged key> [-label <text>]` (Extension 2)

- Action: write a `device.pair` claim (11.3).

`bean import-device <dir>` (Extension 2)

- Action: section 11.6.
- Output: `added <n>, clock offset <ms> ms`.

`bean derive median -type <type> [-as-of <time>]` (Extension 1)

- Action: select candidate records of `<type>` with `subject` = local identity and `times.imported` at or before `-as-of` (default: now). Order them by `times.imported`, then by id. Exclude superseded and duplicate records, and records whose `value.n` is not a number, with reasons. Write the result record (10.2).
- Output: the result id.

`bean replay <id>` (Extension 1)

- Action: section 10.3.
- Output: `replay ok <canonical bytes>`.

## Conformance tests (old SPEC.md 13)

27. P-256 device key.
    - Setup: a P-256 device key. Pair it.
    - Action: import device records signed with low-S. Add one record with a high-S signature.
    - Expected: the low-S records are valid. The high-S record is `bad signature`. One `device.clock` claim is written.
28. Replay on a second replica.
    - Setup: A with 4 claims and a median result D. B is a sync of A with the same identity secret.
    - Action: replay D on A and on B.
    - Expected: both print the same canonical bytes.
29. Replay with a purged input.
    - Action: purge one input of D. Replay twice.
    - Expected: both fail with `input purged: <id>`.
