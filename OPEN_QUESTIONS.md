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
