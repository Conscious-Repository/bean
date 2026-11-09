# NHANES reference files

`bean import nhanes <file.XPT>` keeps one public NHANES data file as reference data for `bean compare`. [docs/COMPARE.md](../../docs/COMPARE.md) says which files to download and how they are used.

- It reads SAS transport, version 5, with its own reader (`internal/xpt`): one data set, numbers of 8 bytes. Anything else is refused.
- It reads the 2017-2018 cycle: data set names ending in `_J`. Another cycle has other weights and variables, and is refused until it has a row in `internal/nhanes`.
- It writes one `reference.file` record and no readings. The file is about other people, not you.
- The record holds the file's SHA-256, size, data set name, cycle, CDC's URL, row count and each variable with its label. Nothing in it is CDC's signature: you sign for having kept the file, and anyone can check the SHA-256 against CDC's copy.
- It says whether the file is the one CDC served on 2026-11-09. One it does not know still imports, and each comparison with it says so.
- `bean purge <record id>` erases the kept file.

Tests use the synthetic files in `internal/nhanes/testdata`, made by `internal/nhanes/synthetic_test.go`. They have CDC's layout and variable names and no person's data; their header says `SYNTH`.
