# Instrument evidence: implementation plan and Codex handoff

Status: PROPOSAL. Companion to [INSTRUMENT-EVIDENCE-CONTRACT.md](INSTRUMENT-EVIDENCE-CONTRACT.md). No implementation or core protocol changes are authorized by this document alone.

## The product goal

Prove this end-to-end flow with the smallest real AION acquisition:

Instrument captures exact bytes → instrument commits to those bytes in a public-key signed receipt → person receives receipt and bytes without an AION account → person imports them to Bean without changing the original receipt → independent offline verifier confirms source key and raw evidence and reproduces one agreed quantity → person discloses only selected material to one recipient.

This is NOT a generic medical-record standard, a global trust system, or a clinical-quality certification.

## Non-negotiable design boundaries

- Do not change CORE.md, SPEC.md, object IDs, existing signatures, identity rotation, or the Blob encryption definition.
- Do not force a subject Bean identity into an instrument before capture. The original receipt is subject-independent.
- Do not derive device identity from a self-reported model string or equate valid signatures with accuracy.
- Use vetted COSE_Sign1 and deterministic CBOR libraries, not new home-grown signing, encryption or encoding.
- Put source-specific acquisition fields in a source-specific profile and immutable acquisition-config artifacts, not in Bean core.
- The device/acquisition process hashes the bytes being emitted. It must not sign a hash reported by an untrusted host without an authenticated data path.
- All paths and files are untrusted. Reject path traversal, special files, missing/duplicate entries and unbounded reads.
- No AION-only endpoint, licence server, account or internet is required for verification.
- Default privacy: no public relay publishing, no plaintext log on untrusted cloud storage, no covert sharing of all records.
- Every report separates signature validity, completeness, trust, reproducibility and measurement accuracy.
- Preserve the original COSE receipt and raw evidence byte-for-byte throughout storage, export and disclosure.

## Dependency graph and milestones

### M0: Wire-contract review and golden fixture

Produce:

- A single typed, deterministic CBOR payload definition and COSE protected-header profile, with exact public-key encoding, key IDs, allowed algorithms, signed-message construction and rejection rules.
- A minimal synthetic raw-signal artifact and acquisition-config artifact, plus one completed COSE receipt.
- Byte-for-byte golden fixture files and expected typed verification outcomes (positive and negative).
- A standalone implementation-neutral conformance document; implementation decisions never silently override it.

Accept when two engineers can independently encode the same payload bytes and identify what the signature authenticates. Do not freeze this format on review alone: exercise it against a real instrument acquisition.

### M1: Verify without Bean or the manufacturer

Suggested new paths (names are negotiable):

- instrument/receipt.go — COSE/CBOR parse, strict profile checks, public-key extraction and signature verification
- instrument/evidence.go — streaming file hashing, declared sizes, roles and order checks
- instrument/report.go — structured outcomes with separate cryptographic and trust statuses
- cmd/bean-instrument-verify/main.go — headless offline CLI over a receipt folder
- vectors/instrument/ — golden positive and negative fixtures
- tools/instrument/second_verify.py — independent Python verifier built from the written profile, not translated from the Go code

Required tests: tampered sample, truncated/reordered/missing chunk, wrong key, non-deterministic CBOR, unknown critical field, invalid signature, changed config/calibration, duplicate or replayed capture ID with local state, and no network.

Definition of done: a clean checkout independently verifies fixture bytes with two implementations. Unknown issuer trust is reported truthfully, not a hard-coded AION endorsement.

### M2: Preserve under the person's control

Add a separate ingest adapter, suggested cmd/bean-instrument-ingest/.

- Input: receipt folder from M1, not an AION API response.
- Verify M1 evidence before ingesting. Ingest failure must not result in a claim labeled successfully verified.
- Store the original receipt and bounded raw chunks in encrypted Bean blobs.
- Create an owner-signed instrument.ingest claim with receipt hash, exact artifact indices, plaintext hashes, and Blob IDs. Do not forge a device-authored Bean claim.
- Verify round-trip: owner export, new independent machine, same original receipt bytes, same raw bytes and same verdict.
- Test crash between blob writes and claim creation, duplicate ingest, partial bundle, corrupted local blob, and non-owner author. Make idempotent retry possible.

Use bounded raw chunks (initially approximately 4 MiB); avoid a large unbounded PutBlob call or rewriting the Bean core's AES-SIV implementation for the first release.

### M3: Produce one independently reproducible measurement

- Identify one actual AION raw acquisition and its documented sample/sequence format.
- Preserve capture configuration, necessary calibration snapshots, environmental/noise channels when available, and any filters.
- Add one independently implemented processing method with versioned code/build digest, explicit input digests, params, output, units and uncertainty.
- Store result as a separate signed claim, attributed to the actual processing author.
- Verify against the raw artifact with tolerances justified by the algorithm; do not imply metrological or clinical validation.

The first output can be a modest signal feature rather than quantitative tissue-state inference. Do not select an endpoint only because it looks compelling in a demo.

### M4: Recipient-selected disclosure

Create a separate sharing bundle specification/CLI only after M1–M3 pass.

- Person chooses explicit receipt(s), outputs, claims and raw artifacts.
- Encrypt to a recipient-specific encryption key using a precisely named RFC 9180 HPKE suite and conventional authenticated bulk encryption as required by the envelope design.
- Receiver decrypts and runs the M1 independent verification offline.
- Include tests for wrong recipient, missing artifact, corruption, unknown key, omission of other private records, and attempted share of signing/recovery keys.
- Do not promise revocation of plaintext already disclosed; document linkability via device key and signed metadata.

FHIR/VC adapters are exchange projections only. They must not become a dependency of original physical evidence verification.

### M5: Device provenance, secure hardware and measurement validation

When the selected hardware is known:

- Determine whether the measurement data can be hashed inside the ADC/FPGA/MCU acquisition boundary and where the signing key is protected.
- Add hardware-backed key generation and signing when practical. Software-signed prototypes must report their lower assurance accurately.
- Carry offline manufacturer endorsement/firmware measurement evidence separately, using established mechanisms where supported; support owner-trusted keys without a vendor account.
- Specify key rotation/revocation and what can be proven without trusted timestamps.
- Independently test accuracy, longitudinal repeatability, drift, motion, calibration, instrument substitution and reproducibility. These are empirical measurements, not consequences of signatures.

## Three parallel reviewer roles

Use three independent review passes after each milestone. They cannot override failing tests with philosophy.

**Linus reviewer:** Is there an unnecessary new format, dependency, special-case path, schema duplication or premature abstraction? Can another engineer implement from the spec without seeing Go source? Do conformance vectors cover exact bytes and failures? Is the interface small and headless? Does it change Bean core unnecessarily?

**Satoshi reviewer:** Can an adversarial host fabricate, swap, truncate, omit, replay or reorder artifacts and still get a successful verdict? Can an attacker impersonate a real instrument with a self-signed key? Are signature, key endorsement, firmware state, clock freshness and scientific truth kept separate? Is verification independent of AION's infrastructure?

**BuildTall reviewer:** Can the user copy the original evidence, signature and calibration, move to another computer, verify offline and choose a recipient without manufacturer permission? Does the workflow work as CLI/API with no AION service? Do files and keys remain individually controlled? Are user-specific source and trust decisions independent of any central registry?

## Codex implementation instruction

Repository: Conscious-Repository/bean, default branch main.

Read, in order:

1. CORE.md and SPEC.md to understand what cannot change;
2. docs/EXTENSIONS.md (device draft and computed-results draft);
3. docs/INSTRUMENT-EVIDENCE-CONTRACT.md (this design proposal's companion);
4. this document;
5. SECURITY.md, CONTRIBUTING.md, Makefile, tools/second/README.md.

Work on a dedicated branch. Begin with M0 and M1, not M4 or M5. Use existing project conventions: tests before implementation, short named functions, standard library where practical, stable errors, synthetic fixtures, and an independent cross-language verifier. Inspect dependencies for current support and licence suitability before adding them. Do not write proprietary crypto.

Deliver for each milestone:

- an explicit change list with files and design decisions;
- a fixture/test that fails before the fix;
- passing CI, relevant adversarial tests and independent verifier agreement;
- a short reviewer report for Linus, Satoshi and BuildTall;
- a PR that describes any limitation and does not claim hardware attestation if the signer is only software.

Do not automatically merge security-critical wire-format changes before the independent verifier passes. Do not change Bean core validity rules without a separately justified protocol-version decision and two-implementation proof.

## Context needed to connect actual AION hardware

The generic receipt format, test vectors, synthetic signer/verifier and Bean ingestion can advance without hardware context.

The real-device stage requires a representative capture with its exact original bytes and encoding, a description of where hashing/signing can execute in the measurement chain, and the available configuration/calibration/reconstruction artifacts. Determine these from real firmware and hardware, not a presumed spec.

## Stop conditions

- Device cannot commit to the authentic acquisition bytes: stop and describe the untrusted boundary; do not call a host signature a device attestation.
- The raw evidence or necessary configuration cannot be exported: stop; the sovereignty/reproducibility goal is not met.
- An external verifier must contact AION to succeed: stop; the design has introduced vendor dependency.
- A valid cryptographic proof is reported as clinical or physical accuracy: stop; separate assurance categories.
- A design requires altering Bean core for convenience: stop and justify why a separate adoption claim and receipt are insufficient.
