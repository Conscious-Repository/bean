# Open questions and choices made during the build

Rule from AGENT_BRIEF.md: follow the DEFAULT; if none, pick the simplest deterministic rule, write it here, continue.

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

## M6 (read and manual entry, ROADMAP.md)

33. **`hlog add` with no `-at` sets `times.observed` to now at minute precision.** SPEC 3.5 says a writer MUST NOT guess a time. A person typing a symptom reports the present, so now is their claim, not a guess. Adapters still write null for a missing source time.
34. **`hlog ls` and `hlog show` read the verified folder, not `local/cache/index.json`.** They call the same loader as `verify`, so they cannot show a stale or edited cache. Cost: every `ls` checks every signature. Revisit with a real Apple Health import.
35. **`hlog ls` shows only current records by default:** valid, not hidden, not superseded, not a duplicate, not an `hlog.*` record. `-all` adds superseded, duplicate and core records. Purged and invalid records are never listed; `verify` reports those.
36. **Times in `ls` are local, and `-since`/`-until` use the local zone for dates.** A record keeps its own offset on disk; only the display converts.
37. **`hlog show` and `-about` take an id prefix of 6 or more hex digits.** An ambiguous prefix fails with `ambiguous id: <prefix>` (a new error string, CLI only).
38. **Manual values:** a number becomes `{"n", "unit"}`, `true`/`yes`/`false`/`no` become `{"b"}`, anything else `{"s"}`. NaN and infinities stay text. The note goes in `context.note`; `source` is `{"system":"self","adapter":"hlog@0.4.0","ref":null}`, so manual entries never dedupe.

## M9 (hlog mcp, ROADMAP.md)

39. **The MCP server is read-only.** Five tools: `list_types`, `query_records`, `stats`, `get_record`, `read_blob`, all marked `readOnlyHint`. A write tool (an AI adding a note) waits until npub signing exists, so an agent can sign as its own identity rather than the person's (Rob's PSR point: agents get their own key).
40. **Every tool call loads and verifies the folder**, like `hlog ls`. A record added while the server runs shows on the next call. Cost grows with the log; revisit after the Apple Health import.
41. **`read_blob` serves only blobs that a current record names.** A blob whose records are all purged is refused even before `gc` erases it.
42. **`stats` reads only `value.n`.** Series blobs (`value.blob`) and text are counted as `non_numeric`. Mixed units are reported with a warning, never converted. Buckets use the local zone; weeks start on Monday.
43. **Protocol version:** the server answers with the client's version when it is one of 2024-11-05, 2025-03-26, 2025-06-18 or 2025-11-25, else 2025-06-18. Newline-delimited JSON-RPC on stdio; nothing else is written to stdout.

## M7 (Labcorp adapter, ROADMAP.md)

44. **PDF text comes from `pdftotext -layout` (poppler), run as a separate program.** Go's standard library cannot extract PDF text, and a PDF parser would be the biggest dependency in the repo. The adapter also accepts the text file, so a person without poppler can run pdftotext elsewhere. Superpower panels are Labcorp reports, so one adapter covers them.
45. **Types are `lab.<slug of the Labcorp test name>`, with no LOINC codes.** The patient report prints no codes, and mapping from memory is the thing the brief forbids. Results from Quest or another lab will not share types with Labcorp until a mapping file exists.
46. **Native id is `<specimen id>/<test name>`.** The specimen id reaches `hlog append` only inside `source.ref_input` and is gone after the HMAC. Patient name, birth date, patient id, account number and physician are never copied. A test checks this.
47. **Collection time.** The report prints "Date Collected: MM/DD/YYYY HHMM Local" with no zone. Without `-tz` the adapter writes the day (`T00:00:00Z`, precision `day`) and keeps the printed time in `context.raw_time`. With `-tz <IANA zone>` it writes the minute in that zone. Ben's 2025-05-31 report also says the requisition had no collection time and the receipt date was used; that note goes in `context.report_note` on every record.
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
