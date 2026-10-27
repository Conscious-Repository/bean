# bean (working name "hlog")

A health log is a folder of signed, append-only JSON records. See SPEC.md.

Status: M1 (bytes layer) complete. Spec text is CC0 1.0 (LICENSE-SPEC); code is Apache-2.0 (LICENSE).

    go test ./...                      # RFC 8785, 4493, 5297, 5869 and BIP-39 vectors
    go run ./tools/genvectors          # regenerate vectors/ V1..V12
    python3 -I tools/checkvectors.py   # independent check with the cryptography package

Layout: `core/` is the core package. `vectors/` holds the published test vectors. `tools/` holds the vector generator and the second-implementation check. `OPEN_QUESTIONS.md` records every choice made where the spec was silent.
