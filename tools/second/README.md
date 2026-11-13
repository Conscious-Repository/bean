# A second verifier

`beanverify.py` is a bean verifier written from `CORE.md` and `SPEC.md` only, without reading the Go code. It is about 700 lines of Python and needs the `cryptography` package. It shows that a stranger can build a compatible verifier from the documents, and it finds places where the documents leave a choice open. Later fixes (DECISIONS 158) made it follow the text that now states each of those choices, and the conformance folders check it.

    python3 tools/second/beanverify.py <log folder>

prints the lines of `bean verify` (SPEC.md 12.1) and exits with the same code. It does not read `local/hidden/`, so it never prints `hidden pending gc`.

`run.sh` (or `make second`) builds logs two ways and checks that `bean verify` and `beanverify.py` agree, line for line and on the exit code:

- `scenarios.sh` drives the `bean` binaries. It covers claims, blobs, supersede and purge, two rotations with `-no-seal`, a restore, a retired-key claim and a fork. It also covers duplicates from two hosts, an agent, a stranger genesis, seal parts and a missing part, a held tombstone and junk files.
- `craft.py` is an independent writer (the HKDF derivations of SPEC.md 4.7, JCS and Ed25519 in Python). It writes edge cases that the CLI cannot: tombstones of supersede records, bad targets, purged duplicates, unresolved authors, an unknown `v` and bad successions. It also writes number, time and seal part edge cases, and authority errors.

- `cases.py` writes the edge-case logs of `vectors/conformance` (SPEC.md 14, V24). They cover nesting depth, empty arrays, byte and code point lengths, whole-string patterns and step order. They also cover too-new identities, recovery keys as signing keys, Ed25519 keys and unknown `v`. `run.sh` checks that it still writes them byte for byte, and that `beanverify.py` prints each folder's `expected.txt`, which `tools/genvectors` writes with bean.

`unseal.py <package> <new folder>` opens a sealed package with its six words, from the age format's specification. `checkbundle.py <folder>` checks a share bundle by the five rules of `docs/SHARE.md`, without bean. `bundles.sh` (or `make bundles`) checks that it and `bean check-bundle` agree on bundles good and damaged.

`checkreply.py <folder>` checks a reply by the six rules of `docs/SHARE.md`, without bean. `replies.sh` (also in `make bundles`) checks that it and bean agree on replies good and damaged, and that `bean accept` takes the reply that both pass. `forgereply.py` writes replies that Python signed with keys of its own: each wrong one breaks one rule of the text and nothing else. `writer.py` is the independent writer that `craft.py` and `forgereply.py` share.

Each place where the spec did not say what to do is marked `GUESS(n)` in `beanverify.py`. A disagreement or a guess is a spec defect; DECISIONS 134 lists the ones that were fixed.
