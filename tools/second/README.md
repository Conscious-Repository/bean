# A second verifier

`beanverify.py` is a bean verifier written from `CORE.md` and `SPEC.md` only, without reading the Go code. It is about 600 lines of Python and needs the `cryptography` package. It shows that a stranger can build a compatible verifier from the documents, and it finds places where the documents leave a choice open. Later fixes (DECISIONS 158) made it follow the text that now states each of those choices, and the conformance folders check it.

    python3 tools/second/beanverify.py <log folder>

prints the lines of `bean verify` (SPEC.md 12.1) and exits with the same code. It does not read `local/hidden/`, so it never prints `hidden pending gc`.

`run.sh` (or `make second`) builds logs two ways and checks that `bean verify` and `beanverify.py` agree, line for line and on the exit code:

- `scenarios.sh` drives the `bean` binaries: claims, blobs, supersede and purge, two rotations with `-no-seal`, a restore, a retired-key claim, a fork, duplicates from two hosts, an agent, a stranger genesis, seal parts and a missing part, a held tombstone and junk files.
- `craft.py` is an independent writer (the HKDF derivations of SPEC.md 4.7, JCS and Ed25519 in Python). It writes edge cases that the CLI cannot: tombstones of supersede records, bad targets, purged duplicates, unresolved authors, an unknown `v`, bad successions, number and time edge cases, seal part edge cases and authority errors.

- `cases.py` writes the edge-case logs of `vectors/conformance` (SPEC.md 14, V24): nesting depth, empty arrays, byte and code point lengths, whole-string patterns, step order, too-new identities, recovery keys as signing keys, Ed25519 keys and unknown `v`. `run.sh` checks that it still writes them byte for byte, and that `beanverify.py` prints each folder's `expected.txt`, which `tools/genvectors` writes with bean.

Each place where the spec did not say what to do is marked `GUESS(n)` in `beanverify.py`. A disagreement or a guess is a spec defect; DECISIONS 134 lists the ones that were fixed.
