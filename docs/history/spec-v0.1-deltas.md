> **Historical.** Appendix A of SPEC.md up to v0.5: the differences between this specification and the v0.1 prototype (`/workspace/hlog`) that came before the `bean` repository. Removed from SPEC.md because it does not describe the protocol.

# Differences from the v0.1 prototype

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

