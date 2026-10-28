# bean (working name "hlog")

A health log is a folder of signed, append-only JSON records. See SPEC.md.

Status: M1 to M6 and M9 complete (bytes, verifier, folder and sync, CLI and CSV adapter, seed-only rotation, read and manual entry, MCP server). Next steps are in ROADMAP.md. Spec text is CC0 1.0 (LICENSE-SPEC); code is Apache-2.0 (LICENSE).

    go test ./...                      # RFC 8785, 4493, 5297, 5869 and BIP-39 vectors
    go run ./tools/genvectors          # regenerate vectors/ V1..V12
    python3 -I tools/checkvectors.py   # independent check with the cryptography package
    go build ./cmd/... && ./demo.sh    # two-folder walkthrough

Quick start:

    hlog -dir ~/health init                       # writes 24 words; put them on paper
    BLOB=$(hlog -dir ~/health blob put export.csv)
    hlog-import-csv -type hrv.rmssd -unit ms -blob $BLOB export.csv | hlog -dir ~/health append
    hlog -dir ~/health verify

    export HLOG_DIR=~/health
    hlog add symptom.headache 8 -unit '{score}' -note "salt?"   # now, minute precision
    hlog add weight 81.2 -unit kg -at 2026-10-27
    hlog ls -type 'hrv.*' -since 2026-10-01                    # -json for the stored bytes
    hlog show d1afbb945f                                       # any 6+ hex prefix

Let an AI read the log (read-only MCP server on stdio):

    claude mcp add hlog -- hlog -dir ~/health mcp

For Claude Desktop, add `{"command": "/path/to/hlog", "args": ["-dir", "/Users/you/health", "mcp"]}` under `mcpServers`. Tools: `list_types`, `query_records`, `stats`, `get_record`, `read_blob`. Whatever the tools return goes to the model provider; use a local model if that matters for the data.

Layout: `core/` is the core package. `vectors/` holds the published test vectors. `tools/` holds the vector generator and the second-implementation check. `OPEN_QUESTIONS.md` records every choice made where the spec was silent.
