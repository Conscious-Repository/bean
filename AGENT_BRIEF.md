# Agent brief: build hlog v1 from SPEC.md

Paste this brief with SPEC.md. SPEC.md is the source of truth. If this brief and SPEC.md differ, follow SPEC.md and report the difference.

## Task

Build a conforming implementation of "hlog core specification v0.3, draft" (SPEC.md).
Build the core, the `hlog` CLI, `hlog-import-csv`, Extension 1 and Extension 2.
Do not start the Oura, Whoop or Apple Health adapters until the owner gives you real export samples.

## Constraints

1. Use Go 1.24 or later. Go 1.24 has `crypto/hkdf` in the standard library.
2. Use the standard library first. Ask before you add a dependency. Do not add SQLite.
3. Use `flag` for the CLI. Do not use a CLI framework.
4. The core MUST NOT use the network. No HTTP, no DNS, no telemetry.
5. Write tests and vectors before the code that they test.
6. Build AES-SIV (RFC 5297) and CMAC (RFC 4493) from `crypto/aes` and `crypto/cipher`. Pass the RFC test vectors first.
7. Embed the BIP-39 English word list as a file. Record its source and license in the repo. Check its SHA-256 against the published list.
8. Use a strict JSON parser. Reject duplicate keys, invalid UTF-8, lone surrogates and integers outside plus or minus (2^53 - 1). `encoding/json` alone does not do this. Write a small tokenizer or wrap `json.Decoder` with token checks.
9. Make every result depend only on the set of files. Sort every map iteration. Inject the clock (`$HLOG_NOW`).
10. Keep code small and plain. One package for the core, one for each command.
11. Run `gofmt`, `go vet` and `go test ./...` before each commit. All must pass.
12. Use the exact error strings and exit codes in SPEC.md section 12.
13. Do not invent test vector values. Compute them with your code. Then check them with a second, independent implementation (for example a short Python script with the `cryptography` package) before you mark them final.

## Milestones

Do the milestones in order. Do not start the next milestone until the exit criteria pass.

M1. Bytes.
- Build: canonical JSON writer and strict parser, record id, nonce, tagged keys, Ed25519 and P-256 signatures with low-S, commitment, HKDF labels, AES-SIV, BIP-39 words, dedupe ref.
- Exit: RFC 8785, RFC 5869, RFC 5297 and RFC 4493 vectors pass. Vectors V1 to V12 exist in `vectors/` and a second implementation agrees.

M2. Verifier.
- Build: validation order (3.8), succession tree, chain selection, seals, freeze, supersede and tombstone rules, too-new per identity, duplicates (section 6, section 8).
- Exit: conformance tests 1, 7 to 10, 13, 14, 17 to 24 and 30 pass. Vectors V13 to V15 and V17 exist.

M3. Folder and sync.
- Build: paths, atomic writes, sync with the tombstone exception, held tombstones, delete hold, purge, purged refs, index cache (section 7).
- Exit: conformance tests 2 to 6, 11, 12, 15, 16 and 25 pass.

M4. CLI, CSV adapter and export.
- Build: every core command in section 12, `hlog-import-csv`, `hlog blob put`, `hlog export`, `hlog import-export`.
- Exit: conformance test 26 passes. A shell demo script runs init, import, sync, purge, rotate, recover and export on two folders.

M5. Extension 1.
- Build: `derive median`, `replay`, the median method spec text and its vectors.
- Exit: conformance tests 28 and 29 pass. Vector V16 exists.

M6. Extension 2.
- Build: `pair`, `import-device`, the `device.clock` record, P-256 device keys.
- Exit: conformance test 27 passes.

## Do not build

- Servers, accounts, user interfaces, Nostr or Blossom transport, `secp256k1` keys.
- Grants, sharing, selective disclosure, zero-knowledge proofs, trusted time or witness records.
- A type registry or a blessed series format.
- `hlog compare`, the puck app or a clinician view.
- Special code for one source in the core.

## Rules for the repo

- Work in a new private repository that the owner names. Do not change `/workspace/hlog`. It is the old v0.1 prototype.
- Never read from or write to `consciousrepo/consciousrepo`.
- License: spec CC0 1.0, code Apache-2.0. Add both license files.
- Do not commit personal health data, keys, paper seeds or identity secrets. Test data must be synthetic.
- Do not push, open pull requests or publish without the owner's approval for each push.

## When the spec is unclear

1. Follow the DEFAULT in SPEC.md.
2. If there is no DEFAULT, select the simplest deterministic rule.
3. Write the question and your choice in `OPEN_QUESTIONS.md`.
4. Continue. Do not stop for a reply.

## Report at each milestone

- What you built, with file paths and line counts.
- Test and vector results, copied from the real command output.
- Every place where you followed a DEFAULT or made a choice.
- Every place where SPEC.md was wrong or unclear, with a proposed fix.
