# Roadmap

What comes next. Work that is done is in the git history, and the reasons are in [DECISIONS.md](DECISIONS.md). Nothing here is final.

## Goal

1. The first user imports their own data and uses it every week. If that does not happen, stop and find out why.
2. Technical users (Nostr and quantified-self people) can install bean, import their data, sign, share and trust, through a CLI and their own AI agent.

## Next

1. **Hardware-first instrument evidence proof.** Use [the minimum viable contract](INSTRUMENT-EVIDENCE-CONTRACT.md) and [Codex handoff](INSTRUMENT-EVIDENCE-CODEX-HANDOFF.md). Make one actual capture with exact raw bytes and settings, cryptographically commit to those bytes at a declared acquisition trust boundary, verify it with two independent offline implementations, and preserve the unchanged evidence in a person's Bean. Do not freeze another wire format or call a host-controlled signature device attestation. This is a narrow proof, not the full historical Extension 2.
2. Importers from real export files, never from memory.
   - `bean-import-apple-health export.xml`. Low-frequency types (weight, blood pressure, VO2 max, labs): one record per reading, with the native id from the XML. High-frequency types (heart rate, steps, energy): one record per type and day, with a series blob. This settles the batch interval for Apple Health (SPEC appendix item 15).
   - `bean-import-csv -map "<column>=<type>[:<unit>]"`, repeatable, so that one row can make several records. Today the importer reads one `value` column.
3. Freeze at `v` 0 (CORE.md section 6). After two weeks of real use (lab results, Apple Health, chat and agent logging) with no change to CORE.md, every DEFAULT in SPEC.md is final.
4. Release: a `go install` path and a neutral home ([GOVERNANCE-PROPOSAL.md](GOVERNANCE-PROPOSAL.md)).
5. Share and trust.
   - `bean share -type ... -since ... -out bundle/` writes signed record files, the plaintext blobs they name, and a manifest. `-to <npub>` encrypts the bundle to the recipient.
   - `bean check-bundle <dir>` verifies every signature without a log.
   - `bean attest <id>` and `bean trust <identity or npub> <1-100>` write ordinary claims with `about` refs. Readers compute trust; the core does not.
6. NIP-46 remote signing, so that the nsec never sits on the host.

## Known gaps

- After `purge` and `gc`, the summary counts the tombstone as `1 held` for good. This is what the spec says, but it reads like a fault.

## Not planned

Hosted or custodial accounts, a UI, relay publishing of records, encryption of record files at rest (use an encrypted volume), Whoop and Oura importers until there is a real export, and the generalized Extension 1 and Extension 2. The narrow real-instrument evidence proof above is planned, without implementing those entire extensions.
