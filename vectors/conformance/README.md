# Conformance folders

Each folder is one log: `objects/` holds the record files and `expected.txt` holds the `bean verify` lines and the exit code.
To test a verifier, copy a folder, run it on the copy with no `local/` folder, and compare its lines and exit code with `expected.txt`.
`tools/second/cases.py` writes the folders, `tools/genvectors` writes `expected.txt` with the Go verifier, and `tools/second/run.sh` checks the Python verifier against them.
SPEC 12.1 gives the line format and SPEC 14 (V24) the scope. Rotation, sync and tombstone behavior beyond these edge cases is tested in SPEC 13 and `tools/second/scenarios.sh`.
