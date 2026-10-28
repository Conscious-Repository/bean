# bean (working name "hlog")

A health log is a folder of signed, append-only JSON records. See SPEC.md.

Status: M1 to M4 complete (bytes, verifier, folder and sync, CLI and CSV adapter). Identity rotation (M5) waits on the design review. Spec text is CC0 1.0 (LICENSE-SPEC); code is Apache-2.0 (LICENSE).

    go test ./...                      # RFC 8785, 4493, 5297, 5869 and BIP-39 vectors
    go run ./tools/genvectors          # regenerate vectors/ V1..V12
    python3 -I tools/checkvectors.py   # independent check with the cryptography package
    go build ./cmd/... && ./demo.sh    # two-folder walkthrough

Quick start:

    hlog -dir ~/health init                       # writes 24 words; put them on paper
    BLOB=$(hlog -dir ~/health blob put export.csv)
    hlog-import-csv -type hrv.rmssd -unit ms -blob $BLOB export.csv | hlog -dir ~/health append
    hlog -dir ~/health verify

Layout: `core/` is the core package. `vectors/` holds the published test vectors. `tools/` holds the vector generator and the second-implementation check. `OPEN_QUESTIONS.md` records every choice made where the spec was silent.
