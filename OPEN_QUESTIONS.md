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
