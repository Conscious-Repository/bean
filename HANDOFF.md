# Handoff brief

For Ben, or for the next agent that opens this repo. Written 2026-10-28.

## What this is

`bean` is the reference implementation of the hlog core specification v0.4 (`SPEC.md`). A health log is a folder of signed, append-only JSON records. Validity is a pure function of the file set. Sync is set union. The identity is the hash of its first record.

Milestones M1 to M5 are done. The tool is usable for CSV imports today.

## State of the repo

- 7 commits on `main`, nothing pushed anywhere. The `.git` history is in the tarball you downloaded.
- Go 1.24, standard library only. No dependencies. One package (`core/`) plus two commands.
- 37 tests, all passing. `go test ./...` takes under a second.
- `python3 -I tools/checkvectors.py` is the independent check (needs the `cryptography` package). 91 checks, zero shared code with the Go.

```
core/        the core package (~2,100 lines without tests)
  jcs.go       canonical JSON (RFC 8785) and the strict parser
  record.go    ids, nonces, commitments
  keys.go      Ed25519, P-256 low-S, tagged keys
  derive.go    HKDF labels
  siv.go       AES-SIV (RFC 5297) from the standard library
  bip39.go     24-word paper seed
  ref.go       dedupe refs
  fields.go    record field checks (SPEC 3.2 to 3.7)
  verify.go    the verifier (SPEC 6), a pure function of path -> bytes
  fs.go        paths, atomic writes, listing
  store.go     a replica folder: init, load with holds, gc, blobs, verify lines
  ops.go       append, purge, sync, export, import-export, reindex
  rotate.go    seed-only rotation with seals
  write.go     record builders
cmd/hlog/              the CLI (SPEC 12)
cmd/hlog-import-csv/   the CSV adapter
tools/genvectors/      writes vectors/ V1..V12, V17
tools/checkvectors.py  the second implementation
vectors/               published test vectors
demo.sh                two-folder walkthrough; run it after any change
SPEC.md                v0.4, matches the code
OPEN_QUESTIONS.md      every judgment call, numbered 1 to 32, plus the Rob canon
AGENT_BRIEF.md         the original build brief (milestones are now stale; see below)
```

## How to run it locally

```
tar xzf bean-m5.tar.gz && cd bean
go test ./...
go build -o ~/bin/hlog ./cmd/hlog
go build -o ~/bin/hlog-import-csv ./cmd/hlog-import-csv
./demo.sh

mkdir ~/health && hlog -dir ~/health init        # write the 24 words on paper
export HLOG_DIR=~/health
BLOB=$(hlog blob put export.csv)
hlog-import-csv -type hrv.rmssd -unit ms -blob $BLOB export.csv | hlog append
hlog verify
```

CSV needs a header row with a `value` column. Optional `time` (RFC 3339) and `id` (native id). Other columns go into `context`.

## What changed from the brief

The brief said build to SPEC v0.3. The review found four holes in the identity layer, so the spec moved to v0.4 and the code follows v0.4. The differences:

1. Only the paper seed can rotate keys. A signing key cannot write a succession. Any fork is a freeze. (`hlog recover` is gone; `hlog rotate` reads the 24 words.)
2. The envelope (`v`, `type`, `author`, `kid`, `id`, `sig`) is checked, with the signature, before the version. A future-version file cannot brick a log unless a chain key signed it.
3. Sync verifies the signature before copying and replaces a stored copy with a bad signature when a good one arrives.
4. Nothing is erased on a timer. Tombstones from elsewhere hide the target in `local/hidden/`; `hlog gc` erases after the 7-day hold.

Also: `hlog gc` and `hlog blob get` exist; `source.supersedes_ref` lets an adapter correct a record it cannot name by id; successions may be 16 MiB so seals fit.

## What is deliberately not built

- Extension 1 (`derive median`, `replay`) and Extension 2 (`pair`, `import-device`). Nothing in the kill rule needs them. Build when a device or a reader exists.
- secp256k1 / Nostr keys. Needs a dependency. Add as an extension when a Nostr signer or relay needs to sign health records.
- Oura, Whoop, Apple Health adapters. Need real export samples. Do not build from memory; the brief is right about that.
- Sharing, grants, encryption of records at rest. Out of scope; Rob agreed it is a routing problem for later.

## What to do next, in order

1. **Use it for a week.** Import your own CSVs. The kill rule is the only metric.
2. **Get a Whoop export** (API JSON). Write `cmd/hlog-import-whoop`. Copy the shape of the CSV adapter. Decide the native id field from the file and record it in the adapter's doc comment.
3. **Send Rob the eight questions** in the meeting one-pager (the Claude doc "hlog meeting one-pager"). Questions 1 to 4 are already decided in v0.4; you are asking him to break them.
4. **Decide batching** for high-frequency sources with the data engineer (SPEC 9.4, open item 16). Until then, do not import watch heart rate one reading per file.
5. **Pick a name** and a GitHub home. Then push.

## Things that will bite you

- `hlog rotate` needs all devices synced first. A destructive record that was not synced is `not sealed` forever after. The command prints a warning; read it.
- A sync between a replica with a new succession and one with a now-invalid tombstone converges in two passes. Run sync twice if something looks hidden that should not be.
- `local/` never syncs. If you copy a folder with a plain `cp -r`, delete `local/cache`, `local/holds.json` and `local/purged-refs.jsonl` on the copy, or you carry hold state across replicas.
- Records are plaintext JSON on disk. Only blobs are encrypted. Put the folder on an encrypted volume.
- `$HLOG_NOW` overrides the clock for every command. Unset it outside tests.

## Working with an agent on this repo

Give it `SPEC.md`, `OPEN_QUESTIONS.md` and this file. The rules that kept the code small:

- Standard library first. Ask before any dependency.
- Tests and vectors before code. Every crypto primitive has an RFC vector; every behavior has a conformance test number.
- The core knows no source. Oura and Whoop live in `cmd/hlog-import-*`.
- Anything the spec does not say gets the simplest deterministic rule and a numbered line in `OPEN_QUESTIONS.md`.
- Run `gofmt -l .`, `go vet ./...`, `go test ./...` and `./demo.sh` before each commit.
