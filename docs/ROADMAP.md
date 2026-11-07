# Roadmap to v0.1

Decision numbers refer to [DECISIONS.md](DECISIONS.md).

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
- `bean-import-csv` takes one `value` column. A real multi-column export fails with `no value column`. (Its `{"text"}` values are fixed to `{"s"}`: DECISIONS 79.)
- No quick manual entry. Fixed in M6.
- After `purge` and `gc`, the summary reports the tombstone as `1 held` forever. Spec-correct, reads like a fault.

## Milestones

M6. Read and write by hand. Done 2026-10-28.
- `bean ls [-type t] [-since d] [-until d] [-author a] [-all] [-json]` from the verified folder (DECISIONS 34). Hides superseded, duplicate and purged records by default.
- `bean show <id>` prints one record and lists its blobs and its `about` replies.
- `bean add <type> <value> [-unit u] [-at time] [-note text]` for symptoms and one-off readings.
- Exit: a fresh log, five manual entries, `ls` and `show` work, conformance tests still pass.

M7. Real importers (needs the sample files).
- `bean-import-apple-health export.xml`. Low-frequency types (weight, blood pressure, VO2 max, labs) one record per reading, native id from the XML. High-frequency types (heart rate, steps, energy) one record per (type, day) with a series blob. This settles open item 16 for Apple Health.
- `bean-import-labcorp` for the blood panel. Done 2026-10-28: Superpower exports are Labcorp patient report PDFs. A real Superpower panel parses to 71 results; re-import adds 0. No LOINC yet (DECISIONS 44 to 51).
- `bean-import-csv -map "<column>=<type>[:<unit>]"`, repeatable, so one row can make several records.
- Exit: Ben's full Apple Health export and one panel import, re-import adds 0, `verify` passes.

Restore. Done 2026-10-28. `bean restore` rebuilds `local/` on a new host from the 24 words and a synced backup, then rotates (DECISIONS 52 to 56). Without it the paper seed could not recover a lost laptop.

M8. npub as signing key. Done 2026-10-28 (SPEC v0.5 C21, DECISIONS 58 to 66). Signs through a Nostr wrapper event so NIP-46 can come later without a format change.
- Tagged key `secp256k1:<64 hex x-only>`, BIP-340 Schnorr over the Nostr event id of a wrapper whose content is the record id. Spec 4.4 and 4.5 changed, vector V18, independent check `tools/checksecp256k1.py`.
- `bean init -nostr new|-|<nsec file>`; `rotate` and `restore` take `-nostr new|<nsec file>`. The recovery keys stay Ed25519 from the paper seed.
- `bean whoami` prints identity id and npub.
- Later: NIP-46 remote signing (Amber), so the nsec never sits on the host.

M9. `bean mcp`. Done 2026-10-28 (DECISIONS 39 to 43).
- Read-only MCP over stdio, standard library only. Tools: list types, query by type and time, get record, get blob text, summary statistics.
- Exit: Claude Code answers "is my resting heart rate trending up this month" from the log. Tested over stdio with the real binary; a live Claude Code run is still to do outside this session.

Agents and chat logging. Done 2026-10-28 (DECISIONS 67 to 75). `bean mcp -write` adds `log_entry` signed by a per-machine agent key; `bean agent init` makes an agent-only replica for a cloud agent (Hermes); `bean agent pair` vouches for one; `add -photo`; the type vocabulary is served to the AI.

M10. Share and trust.
- `bean share -type ... -since ... -out bundle/` writes signed record files, the plaintext blobs they name, and a manifest. `-to <npub>` encrypts the bundle to the recipient.
- `bean check-bundle <dir>` verifies every signature without a log.
- `bean attest <id>` and `bean trust <identity or npub> <1-100>` write ordinary claims (`about` refs). Readers compute trust; the core does not.

Core spec. Done 2026-10-28: CORE.md, the protocol in two pages, tested by an implementation written from it alone (DECISIONS 85). Its open questions are decided (DECISIONS 87 to 90).

Freeze at `v` = 1. When Ben has run his real log for two weeks (lab results, Apple Health, chat and agent logging) and nothing in CORE.md had to change. Then every DEFAULT in SPEC.md is final. Rob reviews before then; his comments can still change the format.

Audit blockers. Fixed 2026-11-06 (SPEC v0.6, DECISIONS 106 to 134): too-new brick, duplicate substitution, default seals, removed records, retired-key claims, trusted reads, write lock, freeze rules, `bean migrate`, vector V19.

Still open from that audit:
- Directory fsync for new shard folders, and F_FULLFSYNC for init, rotate and purge.
- A purged record returns on re-import after a restore: carry its dedupe ref in the tombstone.
- The Labcorp importer stores an unparsed value the same way as "not performed", and has seen one real report.
- Every command loads the whole log: 1.1 GB and 1.3 s for `add` at 100,000 records.
- Remove `net/http`, used only to detect image types, so the binary has no network stack.
- CI, fuzz tests in the repository, a second implementation of V19.
- An outside witness for the newest checkpoint (OpenTimestamps).

Documentation and first-run UX. Done 2026-10-28 (DECISIONS 91 to 100): README, docs/COMMANDS.md, SECURITY.md and CONTRIBUTING.md, checked by following the README from a clean home folder.

M11. Release.
- README quick start, `go install` path, name, GitHub home. Publish after Rob reviews (SPEC 19).

## Not in v0.1

Hosted or custodial accounts, a UI, relay publishing of records, encryption of record files at rest (use an encrypted volume), Whoop and Oura adapters (until there is a real export), Extension 1 and Extension 2.
