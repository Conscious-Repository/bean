# Handoff brief

For Ben, or for the next agent that opens this repo. Updated 2026-10-28.

## What this is

`bean` is the reference implementation of the hlog core specification v0.4 (`SPEC.md`). A health log is a folder of signed, append-only JSON records. Validity is a pure function of the file set. Sync is set union. The identity is the hash of its first record.

The goal and the decisions behind the current direction are in `ROADMAP.md`. Read it first.

## State

- Done: the core (M1 to M5: bytes, verifier, folder and sync, CLI, seed-only rotation), `ls`/`show`/`add` (M6), `hlog-import-labcorp` (part of M7), `hlog mcp` (M9), and `hlog restore`.
- Next: Apple Health importer (needs Ben's `export.zip`), multi-column CSV, npub signing (M8), share bundles and trust claims (M10).
- Go 1.24+, standard library only, no dependencies. `pdftotext` (poppler) is needed at run time for the Labcorp importer only.
- Repository: private, `bean` on Ben's GitHub account. Do not make it public until Rob has reviewed the spec (SPEC 19).

```
core/                    the protocol: JCS, keys, signatures, AES-SIV, BIP-39, verifier,
                         folder, sync, rotate/restore, and the read side (query.go)
cmd/hlog/                the CLI; read.go (ls, show, add), mcp.go (MCP server)
cmd/hlog-import-csv/     CSV importer
cmd/hlog-import-labcorp/ Labcorp patient report importer (Superpower panels)
tools/                   vector generator and the independent Python check
vectors/                 published test vectors V1..V12, V17
demo.sh                  two-folder walkthrough; run it after any change
```

## Checks before every commit

```
gofmt -l . && go vet ./... && go test ./... && ./demo.sh
```

## Rules that kept the code small

- Standard library first. Ask before any dependency (M8 is allowed one, for secp256k1).
- Tests and vectors before code. Every crypto primitive has an RFC vector; every behavior has a conformance test.
- The core knows no source. Each importer is its own `cmd/hlog-import-*`, written against a real export, never from memory.
- Anything the spec does not say gets the simplest deterministic rule and a numbered entry in `OPEN_QUESTIONS.md`. Spec errors found in testing go there too (see 57).
- Test data is synthetic. Never commit real health data, keys or paper words. The Labcorp test builds a fake report in code.

## Things that will bite you

- `hlog rotate` and `hlog restore` need every copy synced first. A destructive record that was not synced is `not sealed` afterwards, and a restore from a stale copy can fork, which freezes the identity.
- A sync that carries a succession and a tombstone it invalidates converges in two passes. Run sync twice if something looks hidden that should not be.
- `local/` never syncs. If you copy a folder with `cp -r`, delete `local/cache`, `local/holds.json` and `local/purged-refs.jsonl` on the copy.
- Records are plaintext JSON. Only blobs are encrypted. Keep the folder on an encrypted volume.
- `$HLOG_NOW` overrides the clock for every command. Unset it outside tests.
- `hlog ls` and every MCP call verify the whole folder each time. Fine for thousands of records; measure after the Apple Health import.
- `SPEC.md` and `AGENT_BRIEF.md` are read-only files. The brief's milestone list is out of date; `ROADMAP.md` replaces it.
