# Roadmap to v0.1

Written 2026-10-28 after a review of the repo, the Rob conversation (2026-10-27) and the Oct 6, Oct 21 and Sep 22 notes. Nothing here is final.

## Goal

1. Ben imports his own data and uses it every week (the kill rule).
2. Technical peers (Rob, RJ, Nostr and quantified-self people) can install it, import their data, sign, share and trust, through a CLI and their own AI agent.

## Decisions (2026-10-28)

| Topic | Decision |
|---|---|
| Identity | An npub (secp256k1 Schnorr, BIP-340) may be the signing key. The genesis, paper seed and recovery layer stay. One dependency allowed for this. |
| First users | Technical peers. CLI plus an MCP server. No UI, no server, no custodian. |
| Sharing | A signed bundle the recipient can verify offline. Optional encryption to the recipient's key. No hosted link. |
| First data | Apple Health `export.xml` and a blood panel export. Real files only; no adapters from memory. |

## Gaps found by using it as a new person

- No way to read records back. Fixed in M6.
- `hlog-import-csv` takes one `value` column. A real multi-column export fails with `no value column`. It also writes text values as `{"text"}`, not the spec's `{"s"}`.
- No quick manual entry. Fixed in M6.
- After `purge` and `gc`, the summary reports the tombstone as `1 held` forever. Spec-correct, reads like a fault.

## Milestones

M6. Read and write by hand. Done 2026-10-28.
- `hlog ls [-type t] [-since d] [-until d] [-author a] [-all] [-json]` from the verified folder (OPEN_QUESTIONS 34). Hides superseded, duplicate and purged records by default.
- `hlog show <id>` prints one record and lists its blobs and its `about` replies.
- `hlog add <type> <value> [-unit u] [-at time] [-note text]` for symptoms and one-off readings.
- Exit: a fresh log, five manual entries, `ls` and `show` work, conformance tests still pass.

M7. Real importers (needs the sample files).
- `hlog-import-apple-health export.xml`. Low-frequency types (weight, blood pressure, VO2 max, labs) one record per reading, native id from the XML. High-frequency types (heart rate, steps, energy) one record per (type, day) with a series blob. This settles open item 16 for Apple Health.
- `hlog-import-labcorp` for the blood panel. Done 2026-10-28: Superpower exports are Labcorp patient report PDFs. Ben's 2025-05-31 panel parses to 71 results; re-import adds 0. No LOINC yet (OPEN_QUESTIONS 44 to 51).
- `hlog-import-csv -map "<column>=<type>[:<unit>]"`, repeatable, so one row can make several records.
- Exit: Ben's full Apple Health export and one panel import, re-import adds 0, `verify` passes.

Restore. Done 2026-10-28. `hlog restore` rebuilds `local/` on a new host from the 24 words and a synced backup, then rotates (OPEN_QUESTIONS 52 to 56). Without it the paper seed could not recover a lost laptop.

M8. npub as signing key.
- Tagged key `secp256k1:<64 hex x-only>`, BIP-340 Schnorr over the 32-byte id. Spec 4.4 and 4.5 change, new vector, second-implementation check.
- `hlog init -nostr` reads an nsec from stdin or `$HLOG_NSEC_FILE`. The recovery keys stay Ed25519 from the paper seed.
- `hlog whoami` prints identity id and npub.
- Later: NIP-46 remote signing (Amber), so the nsec never sits on the host.

M9. `hlog mcp`. Done 2026-10-28 (OPEN_QUESTIONS 39 to 43).
- Read-only MCP over stdio, standard library only. Tools: list types, query by type and time, get record, get blob text, summary statistics.
- Exit: Claude Code answers "is my resting heart rate trending up this month" from the log. Tested over stdio with the real binary; a live Claude Code run is still to do outside this session.

M10. Share and trust.
- `hlog share -type ... -since ... -out bundle/` writes signed record files, the plaintext blobs they name, and a manifest. `-to <npub>` encrypts the bundle to the recipient.
- `hlog check-bundle <dir>` verifies every signature without a log.
- `hlog attest <id>` and `hlog trust <identity or npub> <1-100>` write ordinary claims (`about` refs). Readers compute trust; the core does not.

M11. Release.
- README quick start, `go install` path, name, GitHub home. Publish after Rob reviews (SPEC 19).

## Not in v0.1

Hosted or custodial accounts, a UI, relay publishing of records, encryption of record files at rest (use an encrypted volume), Whoop and Oura adapters (until there is a real export), Extension 1 and Extension 2.
