# Contributing

For people and agents who change the code. Read [CORE.md](CORE.md) first: it is the protocol in two pages. [SPEC.md](SPEC.md) is the full text. [docs/ROADMAP.md](docs/ROADMAP.md) says what comes next, and [docs/DECISIONS.md](docs/DECISIONS.md) says why things are the way they are.

## Build and test

Needs Go 1.24 or later. Check with `go version`. An older Go fails with `package crypto/hkdf is not in GOROOT`.

```
go build ./...
go test -race ./...                  # unit, conformance, vector, CLI and concurrency tests
go vet ./... && gofmt -l .           # gofmt must print nothing
./demo.sh                            # two-folder walkthrough: import, sync, purge, gc, export
python3 -I tools/checksecp256k1.py   # independent BIP-340 and npub check, pure Python
python3 -I tools/checkvectors.py     # independent check of V1 to V17; needs a cryptography package with AES-SIV
```

Run all of them before every commit. When you change a test vector, run `go run ./tools/genvectors` and both Python checks. Vector V6 has a random P-256 nonce; restore it with `git checkout vectors/v6-p256.json` unless you meant to change it.

## Layout

```
core/                     the protocol: canonical JSON, keys and signatures,
                          the verifier, the folder, sync, rotate and restore,
                          agents, the object cache. Knows no data source.
cmd/bean/                 the CLI; read.go (ls, show, add), mcp.go (MCP server),
                          agent.go, keys.go, vocabulary.md (served to the AI)
cmd/bean-import-csv/      CSV importer
cmd/bean-import-labcorp/  Labcorp patient report importer
tools/genvectors/         writes vectors/
tools/check*.py           independent second implementations
vectors/                  published test vectors V1 to V18
```

## Rules

- **Standard library first.** Ask before you add a dependency. The only one is `btcec/v2`, for secp256k1 signatures.
- **Tests before code.** Every crypto primitive has an RFC or BIP vector. Every behavior has a test. A bug fix comes with a test that fails without the fix.
- **The core knows no source.** Each data source is its own `cmd/bean-import-<source>`. Write an importer from a real export file, never from memory.
- **Test data is synthetic.** Never commit real health data, keys or the 24 words. The Labcorp test builds a fake report in code.
- **When the spec is silent,** pick the simplest deterministic rule and add a numbered entry to `docs/DECISIONS.md`. Spec errors go there too.
- **Measure before you optimize.** The object cache came from a profile of a 20,000-record log (DECISIONS 76).

## Add an importer

1. Get a real export file. Keep it out of the repo.
2. Make `cmd/bean-import-<source>/main.go`. Read the file; write one JSON object per line to stdout.
3. Give each line these fields:
   - `type` and `value`.
   - `times.observed`, or null when the source has no time. Never invent a time.
   - `source.system` and `source.adapter` (`bean-import-<source>@<version>`).
   - `source.ref_input`: `["bean/v0/ref","id",<system>,<native id>,0]` when the source has an id for the reading. `bean append` turns it into the dedupe ref, so a second import adds nothing.
4. Put the original file in `blobs` with `bean blob put`, and take its id with a `-blob` flag.
5. Do not copy names, birth dates or patient ids into records.
6. Test with a synthetic file that has the same layout. Document the native id in the command's doc comment.

## Things that will bite you

- Ordinary commands trust `local/cache/objects` for files whose size and modification time have not changed (DECISIONS 76). An edit that keeps both is seen only by `bean verify`.
- `bean rotate` and `bean restore` need every copy synced first. A deletion that was not synced becomes `not sealed`, and a restore from a stale copy can fork the identity, which freezes it.
- A sync that carries a rotation and a deletion that it invalidates needs two passes. Run sync twice if something looks hidden that should not be.
- `local/` never syncs. If you copy a log with `cp -r`, delete `local/cache`, `local/holds.json` and `local/purged-refs.jsonl` in the copy.
- `$BEAN_NOW` overrides the clock for every command. Unset it outside tests.
- `SPEC.md` is a read-only file on purpose: change it only with a change note (C21 onward) and new vectors where a result changes.

## Commits

One subject line that says what changed, then a body that says why. Keep `docs/DECISIONS.md` and `docs/ROADMAP.md` current in the same commit as the code.
