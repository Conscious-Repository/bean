# Roadmap

What comes next. Work that is done is in the git history, and the reasons are in [DECISIONS.md](DECISIONS.md). Nothing here is final.

## Goal

1. The first user imports their own data and uses it every week. If that does not happen, stop and find out why.
2. Technical users (Nostr and quantified-self people) can install bean, import their data, sign, share and trust, through a CLI and their own AI agent.

## Next

1. Importers from real export files, never from memory.
   - Apple Health raw XML/ZIP ingestion is implemented, with optional individual readings. The observed XML has no native IDs; cross-export identity remains approximate. Daily series and aggregates await explicit reconciliation rules; no batch interval is frozen. See `cmd/bean-import-apple-health/README.md`.
   - `bean-import-csv -map "<column>=<type>[:<unit>]"`, repeatable, so that one row can make several records. Today the importer reads one `value` column.
2. Freeze at `v` 0 (CORE.md section 6). After two weeks of real use (lab results, Apple Health, chat and agent logging) with no change to CORE.md, every DEFAULT in SPEC.md is final.
3. Release: a `go install` path and a neutral home ([GOVERNANCE-PROPOSAL.md](GOVERNANCE-PROPOSAL.md)).
4. Share and trust.
   - `bean share -type ... -since ... -out bundle/` writes signed record files, the plaintext blobs they name, and a manifest. `-to <npub>` encrypts the bundle to the recipient.
   - `bean check-bundle <dir>` verifies every signature without a log.
   - `bean attest <id>` and `bean trust <identity or npub> <1-100>` write ordinary claims with `about` refs. Readers compute trust; the core does not.
5. NIP-46 remote signing, so that the nsec never sits on the host.

## Known gaps

- After `purge` and `gc`, the summary counts the tombstone as `1 held` for good. This is what the spec says, but it reads like a fault.

## Not planned

Hosted or custodial accounts, a UI, relay publishing of records, encryption of record files at rest (use an encrypted volume), Whoop and Oura importers until there is a real export, Extension 1 and Extension 2.

## Direct provider adapters

No bridge or iPhone app is planned for the current file-ingestion slice. Next choose
an already-owned eligible provider account, preserve one authorized API response,
and implement its source-specific adapter. See [PROVIDER-INGESTION.md](PROVIDER-INGESTION.md)
for Oura, WHOOP and Withings access gates. Credentials, real responses and personal
exports stay outside the repository. General sharing/attestation remains separate.

## Importer hardening

Labcorp now preserves each original/amended report independently of reading
dedupe, rejects mixed specimen/date headers, and bounds PDF extraction. Explicit
review and application of amended readings remains future work; a newly retained
report does not mean old numeric results were replaced. The Labcorp and Apple
helpers verify source readback and report potentially partial append failures.

A second importer audit adds conservative DST handling, nonzero-underflow
protection, compressed-column parsing, explicit ambiguity failures, parser/output
resource limits, and self-subject archive-retention checks. Apple XML now has a
fuzz target. Actual Labcorp PDF layout validation remains necessary before
claiming support for a user's report; synthetic Poppler coverage is not that proof.
