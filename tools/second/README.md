# A second verifier

`beanverify.py` is a bean verifier written from `CORE.md` and `SPEC.md` only, without reading the Go code. It is about 600 lines of Python and needs the `cryptography` package. It shows that a stranger can build a compatible verifier from the documents, and it finds places where the documents leave a choice open.

    python3 tools/second/beanverify.py <log folder>

prints the lines of `bean verify` (SPEC.md 12.1) and exits with the same code. It does not read `local/hidden/`, so it never prints `hidden pending gc`.

`run.sh` (or `make second`) builds logs two ways and checks that `bean verify` and `beanverify.py` agree, line for line and on the exit code:

- `scenarios.sh` drives the `bean` binaries: claims, blobs, supersede and purge, two rotations with `-no-seal`, a restore, a retired-key claim, a fork, duplicates from two hosts, an agent, a stranger genesis, seal parts and a missing part, a held tombstone and junk files.
- `craft.py` is an independent writer (the HKDF derivations of SPEC.md 4.7, JCS and Ed25519 in Python). It writes edge cases that the CLI cannot: tombstones of supersede records, bad targets, purged duplicates, unresolved authors, an unknown `v`, bad successions, number and time edge cases, seal part edge cases and authority errors.

Each place where the spec did not say what to do is marked `GUESS(n)` in `beanverify.py`. A disagreement or a guess is a spec defect; DECISIONS 134 lists the ones that were fixed.
