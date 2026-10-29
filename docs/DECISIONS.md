# Decisions

Every choice made where the spec was silent, every spec error found, and every design decision, numbered in the order they were made. Code comments and other documents cite them by number ("DECISIONS 76").

The rule, from the original build brief ([history/AGENT_BRIEF.md](history/AGENT_BRIEF.md)): follow the spec's DEFAULT; if there is none, pick the simplest deterministic rule, write it here, and continue.

## M1 (bytes)

1. **Module name.** The folder and Go module are `bean`. The CLI stays `hlog` as SPEC.md 12 says, until the project takes its neutral name.

2. **What counts as an "integer" for the ±(2^53−1) rule (SPEC 4.1).** The spec rejects "an integer outside the range" but does not say whether `9.007199254740992e15` (an exponent literal with an integral value) is an integer. Choice: a number is an integer when its *canonical form* is an integer literal, which is any integral double with magnitude below 1e21 (ES toString switches to exponent form at 1e21). So `9007199254740992`, `9.007199254740992e15` and `9007199254740992.0` are all `bad number`; `1e21` is accepted and canonicalizes to `1e+21`. This makes the writer and the parser agree by construction. Vector V1 pins it. Proposed spec text: "An integer is a number whose canonical form (4.1 rule 7) has no `.` and no `e`."

3. **Error priority when a file has several faults (SPEC 3.8 steps 3–6).** The parser reads the whole file once and reports the fault that comes first in the validation order: `bad json`, then `duplicate key`, then `bad number`, then `bad string`. A structural fault stops parsing, so a duplicate key after a syntax error is never seen; this matches "stop at the first failure".

4. **Nesting depth.** Not in the spec. The parser refuses more than 256 levels with `bad json` (denial-of-service guard, same spirit as the 1 MiB limit). Propose adding to 15 (Security).

5. **P-256 signing nonces.** The software signer uses Go's `ecdsa.Sign` (random nonce), then normalizes to low-S. RFC 6979 is a SHOULD; it is not in the standard library and would need a dependency or a hand-rolled HMAC-DRBG. Deferred; verification is unaffected. Vector V6 therefore records *a* valid low-S signature, and the checker verifies it rather than recomputing it.

6. **`elliptic.UnmarshalCompressed` is marked deprecated in Go** but is the only standard-library way to parse a SEC1 compressed point in Go 1.24. Kept. Revisit when `ecdsa.ParseUncompressedPublicKey`-style helpers cover compressed points.

7. **AES-SIV is hand-rolled** per the brief (constraint 6). The v0.4 change set (C3) allows a vetted library instead. The RFC 5297 A.1 and A.2 vectors pass, and the Python checker (OpenSSL's AES-SIV) agrees on V11 including a 1 MiB blob, so the hand-rolled version stays until someone names a library they prefer.

8. **BIP-39 word list license.** The list is from bitcoin/bips, `bip-0039/english.txt`, SHA-256 `2f5eed53…dbda` (matches the checksum published in the BIP-39 wordlist README). BIP-39 is BSD-2-Clause. Recorded in `core/bip39.go`.

9. **`WordsToSeed` lowercases and trims each word.** The spec does not say. Rationale: paper is read aloud and typed by hand.

10. **Vector V4 uses fixed nonces** (`…0001`, `…0002`, `…0003`) so the vector is reproducible. Real writers use `NewNonce`.

## Spec errors found

- SPEC 14 V1 lists "the RFC 8785 number samples". Two of those samples are commonly misquoted; the authoritative strings for `0x41b3de4355555556` and `…57` are `333333333.3333334` and `333333333.33333343`. Both Go `strconv` and Python `repr` agree. The vector file carries the literal inputs rather than the bit patterns, so there is nothing to misquote.

## M2 (verifier)

11. **Successions are built in M2, not M5**, using the seed-only rule from the v0.4 change set (C5 to C11). With no current-signed successions the tree code is about 60 lines, so a stub would have cost more than the real thing. If the meeting restores current-key rotation, `checkSuccession` and the fork branch of chain selection are the only places that change.

12. **Size check order (C6).** The 16 MiB succession limit is applied before parsing (step 2) and the 1 MiB record limit after the type is known (step 8), both with `too large`. The spec puts the size check at step 2 only, which cannot know the type yet.

13. **Purged targets in the summary.** 12.1 counts valid, invalid, skipped, held and duplicate. A purged target is none of these ("not valid data"). It is left out of every count. Propose adding `<p> purged` to the summary line.

14. **Records of a too-new identity are counted as skipped**, along with the unknown-version files themselves. 8.2 says "do not evaluate"; skipped is the nearest bucket.

15. **Held destructive records and `superseded by sibling` are warnings on the record, held records stay valid**, as 6.6 and 6.7 say. A held tombstone's target does not exist locally, so there is nothing to purge until sync (7.5, M3).

16. **A tombstone of an invalid record still purges it.** 6.7 does not say whether the target must be valid. Simplest rule: the bytes are hidden either way, which is what the person wanted.

17. **`frozen` for off-chain keys.** 5.7 item 4 says claims signed by keys below the lowest common ancestor are `frozen`; a key that was never in the tree stays `key not in chain`. The verifier keeps both strings.

18. **Blob checks (`blob corrupt`, `blob missing`) are not in `VerifyFiles`.** They need the `blobs/` directory and belong to the folder layer (M3). `VerifyFiles` depends only on the record files, which keeps 6.2 literally true.

## Canon from the 2026-10-27 conversation with Rob

Recorded so the build reflects it. Rob did not review the eight one-pager questions; they stay open.

- **"This is just file storage."** Hash every file, address by hash, never edit, add new files. Confirms section 7. Where the folder lives (disk, S3, CDN) is outside the core.
- **Identity.** Rob's default is a Nostr keypair (npub/nsec) with NIP-46 remote signing and re-sign-everything on custodian exit. The genesis-hash identity was not discussed. Open: should the signing key be allowed to be a secp256k1 Schnorr key so one identity flows through relays and signers? That needs a dependency (no stdlib support). Decision pending.
- **Sharing** is a routing problem (grant a known identity access to a known file). Two patterns: login page on a privileged server, or security in the file. Out of scope for v0, confirmed.
- **Schema.** Needed; make it expandable; optional fields are null when absent. Add an adapter-level convention for measurement uncertainty (coefficient of variance) for the NMR source; not core.
- **Claims.** Every record is a claim, including device readings. Vocabulary stands.
- **Trusted devices.** Sign with a tool once at setup; it writes until revoked. Extension 2 (device.pair) matches.
- **Interpretation loop.** Blob in; person or AI returns an interpretation claim that references it (`about`). Already supported.
- **First use case.** Whoop JSON via API, pulled on a schedule. M4 target confirmed.
- **Event-driven** downstream processing preferred over cron. App layer; the index could expose a change feed later.
- **Naming.** Say "core", not "kernel".

## M3 (folder and sync)

19. **Hidden targets live in `local/hidden/<64hex>.json`.** The spec says "hide" without saying how. Moving the file out of `objects/` means sync tools and the verifier of other replicas never see it, and a restore is a rename back. Hidden files are still fed to the local verifier so the tombstone's subject check (6.7) can run.

20. **A local purge leaves no hold entry.** 7.6 step 1 erases at once, so there is nothing for gc to do. The tombstone alone carries the fact.

21. **Sync reads the source with the same loader as the destination**, so the source's hold rules run on the source folder. Syncing from a read-only medium would fail on the first hidden write; add a `-read-only` source mode if that comes up.

22. **`local/cache/index.json`** is the canonical bytes of `{"spec", "records": [...]}` sorted by id with type, subject, author, kid, times, system, ref, valid, hidden, superseded_by, purged_by and error. It is the only index in v0. Query format stays an open question (readers decide).

23. **Export manifest says `hlog-core-v0.4`.** The spec text says v0.3; the string tracks the spec version the writer implements.

24. **`Sync` returns a `Replaced` count** for C13 replacements, in addition to the three counts of 12.3.

## M4 (CLI and CSV adapter)

25. **`hlog blob get <id>`** is added (not in 12.3). Export covers exit, but a person who wants one raw file back should not need a whole export.

26. **CSV adapter fields.** `value` parses as a number into `{"n": ...}`, else as `{"text": ...}`; empty is `{"n": null}` (9.6). Unknown columns are copied into `context` as strings. Identical readings without a native id get increasing occurrence numbers within one file so they are not merged (4.8 rule 6).

27. **`hlog append` prints the counts before the error** when it stops early, since records before the error stay written (12.3).

28. **Exit codes for `purge`, `append`, and others** are 1 on any error; only `verify` uses 3 and 4. `fail()` maps a `too new:` or `compromised:` message to 3 or 4 for commands that surface them later (M5).

## M5 (rotation)

29. **One command, `hlog rotate`, reads the 24 words on stdin.** `hlog recover` is gone (C5). The recovery generation is the position of the head in the chain, so the person never types a generation number.

30. **Seals default to all valid destructive records of the outgoing key in this replica**; `-no-seal <id>` excludes one. A destructive record on another replica that was not synced first is `not sealed` afterwards, which is why the command prints "sync all devices before you rotate".

31. **Sync converges in two passes** when a succession and a tombstone it invalidates travel in the same sync: the tombstone set is computed before copying (7.4 step 2), so the target comes back on the next pass (7.4 step 5). Spec-correct; recorded because it surprised a test.

32. **The paper words are parsed leniently**: numbering like "1." is dropped, case is ignored. The init output can be pasted back as is.

## M6 (read and manual entry, docs/ROADMAP.md)

33. **`hlog add` with no `-at` sets `times.observed` to now at minute precision.** SPEC 3.5 says a writer MUST NOT guess a time. A person typing a symptom reports the present, so now is their claim, not a guess. Adapters still write null for a missing source time.
34. **`hlog ls` and `hlog show` read the verified folder, not `local/cache/index.json`.** They call the same loader as `verify`, so they cannot show a stale or edited cache. Cost: every `ls` checks every signature. Revisit with a real Apple Health import.
35. **`hlog ls` shows only current records by default:** valid, not hidden, not superseded, not a duplicate, not an `hlog.*` record. `-all` adds superseded, duplicate and core records. Purged and invalid records are never listed; `verify` reports those.
36. **Times in `ls` are local, and `-since`/`-until` use the local zone for dates.** A record keeps its own offset on disk; only the display converts.
37. **`hlog show` and `-about` take an id prefix of 6 or more hex digits.** An ambiguous prefix fails with `ambiguous id: <prefix>` (a new error string, CLI only).
38. **Manual values:** a number becomes `{"n", "unit"}`, `true`/`yes`/`false`/`no` become `{"b"}`, anything else `{"s"}`. NaN and infinities stay text. The note goes in `context.note`; `source` is `{"system":"self","adapter":"hlog@0.4.0","ref":null}`, so manual entries never dedupe.

## M9 (hlog mcp, docs/ROADMAP.md)

39. **The MCP server is read-only.** Five tools: `list_types`, `query_records`, `stats`, `get_record`, `read_blob`, all marked `readOnlyHint`. A write tool (an AI adding a note) waits until npub signing exists, so an agent can sign as its own identity rather than the person's (Rob's PSR point: agents get their own key).
40. **Every tool call loads and verifies the folder**, like `hlog ls`. A record added while the server runs shows on the next call. Cost grows with the log; revisit after the Apple Health import.
41. **`read_blob` serves only blobs that a current record names.** A blob whose records are all purged is refused even before `gc` erases it.
42. **`stats` reads only `value.n`.** Series blobs (`value.blob`) and text are counted as `non_numeric`. Mixed units are reported with a warning, never converted. Buckets use the local zone; weeks start on Monday.
43. **Protocol version:** the server answers with the client's version when it is one of 2024-11-05, 2025-03-26, 2025-06-18 or 2025-11-25, else 2025-06-18. Newline-delimited JSON-RPC on stdio; nothing else is written to stdout.

## M7 (Labcorp adapter, docs/ROADMAP.md)

44. **PDF text comes from `pdftotext -layout` (poppler), run as a separate program.** Go's standard library cannot extract PDF text, and a PDF parser would be the biggest dependency in the repo. The adapter also accepts the text file, so a person without poppler can run pdftotext elsewhere. Superpower panels are Labcorp reports, so one adapter covers them.
45. **Types are `lab.<slug of the Labcorp test name>`, with no LOINC codes.** The patient report prints no codes, and mapping from memory is the thing the brief forbids. Results from Quest or another lab will not share types with Labcorp until a mapping file exists.
46. **Native id is `<specimen id>/<test name>`.** The specimen id reaches `hlog append` only inside `source.ref_input` and is gone after the HMAC. Patient name, birth date, patient id, account number and physician are never copied. A test checks this.
47. **Collection time.** The report prints "Date Collected: MM/DD/YYYY HHMM Local" with no zone. Without `-tz` the adapter writes the day (`T00:00:00Z`, precision `day`) and keeps the printed time in `context.raw_time`. With `-tz <IANA zone>` it writes the minute in that zone. The first real report tested also says the requisition had no collection time and the receipt date was used; that note goes in `context.report_note` on every record.
48. **Values.** A plain number is `{"n", "unit"}` and the printed digits are kept in `context.raw_value` when they differ (`1.190`, `3.0`). Anything else is `{"s"}` (`<0.5`, `Negative`). A test the lab did not perform is `{"n": null}` with the lab's reason in `context.lab_comment`.
49. **Units and reference intervals are told apart by column.** Measured on the real report: units start 18 to 25 columns left of the "Reference Interval" header, intervals 3 to 8 columns right of it. The cut is 8 columns left of the header. The "Previous Result" column goes to `context.previous_result` as printed; no real report with previous results has been seen yet.
50. **The lab order PDF is not imported.** It holds no results. Store it with `hlog blob put` if the paper trail matters.
51. **`hlog ls` shows `context.flag`** after the value (`22.4 umol/L  High`) and prints `no value` for `{"n": null}`.

## Restore (before M8)

52. **`hlog restore` rebuilds `local/` on a new host from the 24 words and a synced copy of `objects/` and `blobs/`.** SPEC 5.5 assumed the device still had `local/identity-secret`, so a lost laptop with only the paper and a backup had no way back. Restore finds the genesis whose `value.recovery` is the commitment to recovery generation 0 of the words, writes the identity secret, and then rotates exactly as `hlog rotate` does (same generation rule, same default seals, `-no-seal`). The lost key stops being the head key, so it can no longer delete or correct; its claims stay valid (6.5). Proposed spec text: a new 5.10 "Restore on a new host".
53. **Restore always rotates.** The old signing key is on the lost host, and a key that may be in someone else's hands should not stay the head key. There is no restore-without-rotate.
54. **`local/identity` is written last.** Until it exists the folder reads as `not initialized`, so a restore that fails halfway can run again. A failed restore that already wrote its succession just adds one more generation on the retry; it never forks.
55. **New error string `input missing: genesis for these words`** when the folder has no genesis for the words (wrong log, or nothing synced yet). Wrong or misspelled words stay `seed does not match`.
56. **A stale backup can freeze the identity.** If the folder lacks a succession that another device wrote, restore writes a second child of an older node; when the two meet, that is a fork and the identity freezes (5.7). The command says "sync every copy of the log you still have into this folder first" before it reads the words.

## Spec error found while testing M6

57. **Step 12 (duplicates) hid parser fixes about half the time.** A fix made with `source.supersedes_ref` (9.7) keeps the dedupe ref of the record it corrects. Step 12 grouped the two and made the lower id the primary, so when the fix had the higher id it became `duplicate:` while the original was superseded, and a reader showed neither. Found as a flaky `TestListHidesSupersededAndPurged`. Fix: step 12 leaves out records that a valid supersede record names. `TestParserFixIsNotADuplicate` fails without the fix. Proposed spec text for 6.3 step 12: "Group valid records that are not superseded by ...". Still open: when two hosts imported the same reading (known limit 4) and one of the copies is then fixed, the unfixed copy and the fix form a group and the lower id wins. Readers may then show the old value.

## M8 (npub signing, SPEC v0.5 change note C21)

58. **An npub signs through a Nostr wrapper event, not the raw record id.** NIP-46 remote signers and NIP-07 browser extensions only sign Nostr events, so a raw BIP-340 signature over the id would keep the nsec on the host forever. The wrapper is `[0, pubkey, created_at, 29944, [], record id]`; its event id is the BIP-340 message. Ben chose this on 2026-10-28.
59. **Kind 29944** is in the ephemeral range (relays do not store it) and is not taken by a NIP we know of. Decided in 87.
60. **`created_at` lives in the sig string** (`secp256k1:<created_at>:<hex>`), because a remote signer may set its own time. The software signer uses 0. It never affects validity. Decimal, no leading zeros, at most 2^53 - 1.
61. **One dependency: `github.com/btcsuite/btcd/btcec/v2` v2.3.4**, which brings `decred/dcrd/dcrec/secp256k1/v4`, `btcd/chaincfg/chainhash` and `decred/dcrd/crypto/blake256`. Hand-rolling BIP-340 point arithmetic was the alternative; a vetted library is safer for signing. `tools/checksecp256k1.py` is a pure-Python BIP-340 that reproduces BIP-340 vector 0, the NIP-19 examples and V18 without sharing any code.
62. **Recovery keys stay Ed25519** from the paper seed. Only signing keys and bare-key authors may be secp256k1. An agent with its own npub can write claims as a bare key (`TestAgentNpubClaim`).
63. **After a rotation or restore, an npub on the chain can never sign that log again** (5.8 key reuse; `rotate` and `restore` now refuse before writing). This is right while the nsec sits in `local/signing-key`: a lost laptop means a lost nsec. It is a reason to give the log a dedicated npub (`init -nostr new`) rather than the person's main one, until NIP-46 signing keeps the nsec off the host.
64. **`init -nostr -` turns terminal echo off with `stty`.** Go's standard library cannot; `golang.org/x/term` would be a second dependency.
65. **The export manifest and index say `hlog-core-v0.5`.** Old exports with `v0.4` import the same way; import-export does not read the string.
66. **`tools/checkvectors.py` uses `public_bytes(Raw, Raw)`** instead of `public_bytes_raw()` so it runs on cryptography before 40. It still needs an OpenSSL with AES-SIV (cryptography 50 in a venv works).

## Agents and chat logging (after M8)

67. **An agent writes as its own bare secp256k1 key, with the person as subject.** Key in `local/agent-key`, one per machine, never synced. SPEC 6.1 already lets anyone add a claim; the verifier already stops a non-author from superseding and a non-subject from tombstoning, so an agent can add but never change or delete the person's records. No spec change.
68. **The person vouches for an agent with an `agent.pair` claim** (`value` = `{key, label}`), the shape of `device.pair` (SPEC 11.3). Readers decide trust; the core does not check pairing. `hlog ls` labels a paired agent's entries `[label]` and an unpaired key's `[unpaired npub1...]`. Unpair by superseding the pair claim with `value.status` = `"unpaired"`, as 11.3 says for devices.
69. **Agent-only replica.** `hlog agent init` works on a synced copy that has no `local/identity`: it takes the only genesis in the folder (or `-identity`), writes `local/identity` and `local/agent-key`, and nothing else. No personal signing key, no paper words. Without `local/identity-secret` the agent cannot attach photos, read blobs, or write `ref_input` (no dedupe key); copying that secret to the agent's machine is an explicit choice, and it opens every blob. Records are plaintext either way, so the agent's machine can read the whole log.
70. **On the person's machine `agent init` pairs at once.** On an agent-only replica it prints the `hlog agent pair <npub>` line to run on the person's machine.
71. **`hlog mcp` is read-only unless `-write`.** With `-write` it makes the agent key on first start (label `agent@<hostname>` unless `-label`) and adds one tool, `log_entry`: type, value (text, number or boolean), unit, at, note, about, details, photo_path. `food.estimate` must have `about`.
72. **Photos must sniff as images** (`net/http.DetectContentType` says `image/*`) and be at most 25 MiB. Without this, `photo_path` plus `read_blob` would let a prompt pull any text file on the machine into the log and back out. They are stored with role `photo`. `read_blob` returns only the size of a binary blob.
73. **The type vocabulary is `cmd/hlog/vocabulary.md`**, embedded in the binary and sent in the MCP instructions so the AI follows it. Conventions only; the core checks none of it.
74. **`ls` hides `agent.pair` and `device.pair`** unless `-all`; they are setup, not events. `-author agent` lists this machine's agent's entries; `-author` also takes an npub.
75. **Writing as the person on an agent-only replica fails** with "no personal key on this replica (agent-only): sign with the agent key instead (-agent)".

## Audit, 2026-10-28 (Linus pass)

Measured on a 20,000-record log before the pass: `add`, `ls`, `verify` and every MCP call took 4 to 5.5 s, because every command re-proved every signature on one core and opened every file.

76. **Object cache, `local/cache/objects`, the way git's index works.** Records are named by their hash and never edited, so a file whose size and modification time match the cache still holds bytes this replica already proved. Ordinary commands skip only the signature arithmetic for such files; every other check of 3.8 still runs. `hlog verify` ignores the cache, proves every file, and rebuilds it: verify is the fsck. Files modified in the last 2 seconds of wall-clock time are not cached ("racily clean" in git), and an edit that changes size or time is seen at once. An edit that keeps both size and time is seen only by `verify`, as with git. The cache is best effort and never the truth: a missing, garbled or unwritable cache only costs time. Format: one line per canonical record, `<path>\t<size>\t<mtime ns>\t<bytes>`.
77. **Per-file checks (3.8 steps 1 to 15) run on all CPUs.** Results are collected by index, so the report does not depend on scheduling; `VerifyFiles` stays a pure function of the files (G1). `TestParallelMatchesSerial` and the order-independence test pin it.
78. **Writes are durable.** `writeFileAtomic` flushes the temporary file before the rename and the directory after it. On macOS it calls plain `fsync`, not Go's `File.Sync`, which issues `F_FULLFSYNC`: measured 0.02 ms against 3.9 ms per file, and a 20,000-record import went from 11.4 s to 12.0 s instead of minutes. SQLite makes the same choice by default.
79. **`hlog-import-csv` wrote text values as `{"text": ...}`.** SPEC 3.4 says `{"s": ...}`. Fixed, and the CSV time is now kept as written with its own offset instead of converted to UTC. Adapter 0.5.0. This changes the dedupe refs of tuple-keyed rows, which is why it had to happen before any real log existed. The importer now has a test.
80. **`Init`, `Rotate` and `Restore` are one function each**, taking a `NewKey` (the zero value is a new Ed25519 key). The `InitWith`, `RotateTo` and `RestoreTo` variants are gone.
81. **One load per command on the write path.** `Add` resolves the `about` prefix from the same load it appends with; the CLI and the MCP server no longer load first to resolve it. `writerFor` is the one place that picks the person's key or the agent's.
82. **Concurrent hlog processes** (an MCP server beside a CLI) can load at the same moment after a purge arrives. A rename that finds the file already moved is success, and a file that vanishes between listing and reading makes the load list again (up to 5 times). `TestConcurrentLoads` failed without each fix and passes 200 runs and 20 runs under the race detector with them.
83. **`hlog append -agent`** pipes an importer's output in under this machine's agent key. On an agent-only replica it refuses importers that need dedupe refs (no identity secret), with that message.
84. **Leftovers removed:** the `ErrSeedWords` alias (one `ErrSeedMismatch`), `_ = errors.New` in the CLI, an unused field in the Labcorp importer. `Add` moved from `query.go` to `add.go`. `staticcheck` reports nothing; `deadcode` reports only `VerifyFiles`, kept as the pure verifier the spec and a second implementation test against.

After the pass, same log: `ls` 0.43 s (5.5), `add` 0.21 s (4.0), an MCP call 0.23 s (4.0), `verify` 0.6 s warm and 1.2 s cold (4.0). GC tuning was measured and gained nothing, so there is no knob for it.

## CORE.md (2026-10-28)

85. **CORE.md is the protocol in two pages** (about 1,350 words against 14,600 in SPEC.md): only what a second implementation must agree on, plus four questions for Rob. SPEC.md stays authoritative. It was tested by giving a fresh agent only CORE.md and the vectors: its own verifier, with JCS and BIP-340 written from scratch, passed V1 to V5, V7, V12 and V18. The eight things it had to guess (the integer rule, size check order, lowercase signatures, the created_at range, the wrapper serialization, Ed25519 strictness, the dedupe occurrence rule, which hex names the path) are now stated.
86. **V4 now says how its claim's dedupe ref was made**: `source_ref_input` and the V10 dedupe key. Additive; no record or id changed.

## The four questions for Rob, answered (2026-10-28)

Answered from the Rob transcript, the project's values and the Nostr specs, so Rob reviews decisions instead of making them. CORE.md section 6 is the short form.

87. **Wrapper kept, with a NIP-31 `alt` tag** `["alt","hlog record signature"]`. Facts: 29944 is not in the NIPs registry (nearest entries 28936 and 30000); NIP-01 says kinds 20000 to 29999 are ephemeral, "not expected to be stored by relays"; NIP-46 permissions take a kind (`sign_event:<kind>`), so a bunker can allow hlog to sign only this kind; NIP-46 does not say a signer keeps the client's `created_at`, which is why it lives in `sig`; NIP-31 says a custom-protocol kind should carry an `alt` tag. The tag changed V18; no real log existed yet. Sources: github.com/nostr-protocol/nips README.md, 01.md, 46.md, 31.md.
88. **Records stay their own format, not NIP-01 events.** Rob in the transcript: the NIP maintainers prefer local "micro standards" and medical data is the place for one; the existing medical NIPs stalled because maintainers do not want medical data on relays. Records need nested structure, device keys (Ed25519, P-256) and canonical ids independent of the key type. Nostr carries them later, encrypted, for sharing (M10).
89. **Seed-only rotation and freeze-on-fork stay.** They answer Ben's port-out question better than re-signing: a custodian holds the signing key, the person holds the words, and leaving is one rotation with history intact and the custodian's key retired. Rob's own point, that you can never know a custodian discarded a key, is exactly what retiring it by rotation solves. Cost: losing paper and device loses the identity; social recovery can come as an extension.
90. **Freeze criteria:** two weeks of Ben's real log (labs, Apple Health, chat and agent logging) with no change to CORE.md; then `v` = 1 and every DEFAULT final. Sharing, Apple Health batching and the vocabulary sit above the core and do not block it.

## New-user audit, 2026-10-28

Method: clone the repo into an empty home folder and follow the README word for word, twice; compare the README with those of age and restic; read the code for anything a first-time user would hit.

91. **Install with `go install ./cmd/...`** into `$(go env GOPATH)/bin`, and say how to put it on the `PATH`. The old README built into `~/bin`, which is not on the `PATH` by default, so `hlog` was not found. The README also names the error an old Go gives (`package crypto/hkdf is not in GOROOT`); go1.17 at `/usr/local/go` produced it in the walkthrough.
92. **The module path is `github.com/consciousrepo/bean`**, not `bean`, so `go install github.com/consciousrepo/bean/cmd/...@latest` works once the repository is public. Go practice.
93. **Version 0.5.0** in `core.Version`, shown by `hlog version`. `source.adapter` on records written by `hlog` is now `hlog@0.5.0`; it said `hlog@0.4.0` after the spec moved to v0.5.
94. **Help and errors.** `hlog help`, `-h` and `--help` print grouped help to stdout and exit 0 (they exited 2). An unknown command names itself instead of printing all the help. A usage error prints the syntax of that command only. `not initialized` is followed by the folder that was checked and what to do. The exact error strings of SPEC 12.2 do not change.
95. **Read commands in a folder with no log fail with `not initialized`.** `hlog ls` printed nothing and exited 0, so a new user could think their data was gone. A folder counts as a log when it has `local/identity` or an `objects/` folder (an agent-only copy before `agent init`). In an empty folder this check comes before usage errors.
96. **`hlog add -corrects <id>`**, and `corrects` in the MCP `log_entry` tool. A user could not correct a wrong entry from the CLI; it took a hand-written JSON line for `append`. The verifier already allows a correction only by the same author, so an agent cannot correct the person's records.
97. **`hlog purge` and `rotate -no-seal` take a shortened id**, as `show`, `-about` and `-corrects` do. `purge` did an exact lookup and failed with `input missing` on a prefix that `ls` had just printed.
98. **Messages are sentences.** The `init` warning and the agent-only pairing hint start with capitals and say "copy", not "replica". User documents say "copy"; SPEC.md keeps "replica".
99. **Documents.** `HANDOFF.md` became `CONTRIBUTING.md` (build, test, rules, adding an importer). `OPEN_QUESTIONS.md` became `docs/DECISIONS.md`, because most entries are decisions. `ROADMAP.md` moved to `docs/`. The original build brief moved to `docs/history/` with a note that it is out of date. New: `docs/COMMANDS.md` (every command, flag, exit code, verify line and file) and `SECURITY.md` (what is and is not protected, the keys, how to report a problem). The README follows the age and restic pattern: a short opening with a real example, principles, an install table, a numbered quick start, then everyday use, imports, chat, more than one machine, how it works, security and a document map. All user documents use Simplified Technical English: no descriptive sentence over 25 words, the imperative for steps, one term for one meaning.
100. **CLI tests run the real binary.** `cmd/hlog/cli_test.go` re-runs the test binary as `hlog` and checks help, version, unknown commands, usage errors, an empty folder and a first run, with exit codes.

## Plain-language pass, 2026-10-28

101. **CORE.md and SPEC.md are in Simplified Technical English.** Numbered steps for procedures, one statement per sentence, no descriptive sentence over 25 words, no bold run-in labels, no summary lines that start "Result:", and no filler ("exactly", "deliberate", "this is the price of"). CORE.md keeps every fact that the independent implementation needed (DECISIONS 85).
102. **SPEC.md holds only the protocol.** Removed: the review-specific wording ("DEFAULT, Rob to confirm" is now "DEFAULT", which can change until the v1 freeze), the editor's name, the "customer zero" project rule (it is in docs/ROADMAP.md), internal product names, and Appendix A, the differences from the v0.1 prototype. Appendix A moved to docs/history/spec-v0.1-deltas.md. Appendix B became "Appendix: defaults and open items", sorted into decided, default and open. Section 14 now says which vectors are published (V1 to V12, V17, V18); it said all were TBD.
103. **The user documents name no AI product.** The README section is "Log from an AI assistant": `hlog mcp` works with any MCP client, with one generic JSON entry. Examples use the labels `assistant` and `server`.

## Repository home, 2026-10-28

104. **The repository moved to the Conscious-Repository organization**: github.com/Conscious-Repository/bean, still private. The module path follows it (`github.com/Conscious-Repository/bean`), which replaces the path in 92. GitHub redirects the old URL, but clones should update their remote.
