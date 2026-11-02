# Codex handoff — Bean instrument evidence proof

Status: ready for an executable spike. Proposal only; do not freeze a wire standard or merge a protocol-format change on this handoff's authority.

Repository: Conscious-Repository/bean
Branch to review: proposal/instrument-evidence-contract (draft PR #15)
Base for implementation: the latest main, with a new feature branch. Keep the proposal review separate from implementation.
Primary design: [INSTRUMENT-EVIDENCE-CONTRACT.md](INSTRUMENT-EVIDENCE-CONTRACT.md).

## Your assignment

Build the smallest independently verifiable evidence path for **one** instrument acquisition, without modifying Bean's core format.

Target demonstration:

    exact acquisition bytes + machine-readable settings
        ↓ captured/hashes at a declared trust boundary
    original instrument/host-signed receipt
        ↓ transfer offline
    independent verifier validates the signer + raw bytes
        ↓ ordinary owner-side Bean ingestion
    same original receipt/evidence recovered from a new machine

The goal is verifiable measurement **provenance and integrity**, not proof of biological accuracy. A software signer whose bytes are controlled by the host must be reported as software-controlled. Do not assert hardware signing unless the data path and key isolation genuinely establish it.

## Operating rules

1. Read CORE.md and SPEC.md, then docs/INSTRUMENT-EVIDENCE-CONTRACT.md, docs/EXTENSIONS.md, SECURITY.md, CONTRIBUTING.md and tools/second/README.md.
2. Do not change CORE.md, SPEC.md, current record validity, key rotation, record serialization, AES-SIV, or existing vectors merely to implement capture.
3. Start with a **small executable proof**, not a broad SDK or a polished universal schema.
4. Reuse existing standards and maintained implementations. Prefer a single provisional COSE_Sign1 profile if it fits the chosen signer; check whether an existing attestation format is already available on the real device. No proprietary signing algorithm or invented key registry.
5. Bind *exact* raw bytes, settings, and any calibration necessary for the test calculation. Hash bytes before transformations. Device trust and processing trust remain distinct.
6. Build test fixtures before writing integration code. Write one independent verifier from the written profile; do not translate the Go implementation line for line.
7. Keep every capability CLI-callable, offline, no API or manufacturer account. Do not publish health evidence to public relays.
8. Use clear outcome categories: signature_valid, evidence_complete, source_trusted, replay_available, accuracy_validated. Never collapse them into one "verified" boolean.
9. For each PR, summarize what changed, what was actually tested, what remains an assumption, and what hardware inputs are missing. Do not auto-merge failed checks.
10. Let the real device constrain the final receipt format; document why any field or cryptographic dependency is essential.

## First work package — perform now without AION firmware

### Task 0: map constraints, choose one provisional profile

Before implementing, write a short ADR (under docs/) with:

- The exact cryptographic trust boundary being simulated.
- Whether the provisional receipt uses COSE_Sign1 and why. If not, which existing standard is used and why.
- Signing algorithm and key encoding provided by the tested library. Use test-only keys; make no hardware-attestation claims.
- Required minimum fields: profile discriminator, capture nonce, complete ordered artifact table (role, byte length, SHA-256), signing public key/locator, and completion status.
- File semantics: original bytes, mandatory settings file, stable raw format, exact length, streaming hash checks; calibration is committed when needed for numerical interpretation.
- Which requirements are deferred and what real-hardware evidence could overturn this choice.

Do not write a final v1 protocol specification before a working independent cross-language proof.

### Task 1: minimal fixture and signer

Create a tiny synthetic acquisition representing the structure of a real NMR signal, with an explicitly documented sample format (signed integer or complex, sample rate, byte order, frame count). Add a small machine-readable acquisition-settings file. If the selected demo calculation requires calibration, include a frozen calibration file.

Produce a test receipt with a test-only signer. Hash the exact emitted raw and settings bytes, not a parsed/reformatted equivalent. Preserve the original signed envelope as bytes.

Implement the proof with a provisional profile that can be rejected on unsupported changes. Make sure a complete capture cannot be declared if a required artifact was not finalized.

### Task 2: offline independent verification

Implement:

- A minimal CLI: "verify <capture-directory>", consuming receipt + declared artifacts, with machine-readable result and distinct exit codes.
- Parsing, signature verification, strict digest/length checks, file-path safety, duplicate/missing/reordered artifact checks, memory limits and explicit failure outcomes.
- A second verifier in a different language built from the receipt contract and test fixtures.

Adversarial test matrix:

| Mutation | Must report |
|---|---|
| One bit in raw signal or settings | evidence mismatch |
| Missing/extra/truncated sample bytes | incomplete or mismatch |
| Swapped artifact or duplicate artifact index | invalid bundle/receipt |
| Invalid or mismatched signature/key | signature invalid |
| Unknown algorithm/unsupported receipt profile | unsupported, not valid |
| Valid signature with unknown issuer | signature valid; source trust unestablished |
| Malicious symlink/path traversal/large file | refused safely, bounded memory |
| No internet, AION services unavailable | same evidence/signature outcome |
| Same proof checked by the independent verifier | equivalent outcome |

Tests must show a host cannot replace raw bytes after the signer has committed to their hashes without detection. They cannot prove the actual physical acquisition was honest.

### Task 3: minimal Bean adoption

Implement this only after Task 2 passes:

- A simple CLI or adapter that verifies a capture directory first, stores the unchanged receipt and evidence as owner-encrypted Bean blobs, and writes a normal owner-signed adoption claim referencing the original receipt digest and encrypted blob IDs.
- No new Bean core record type or device-identity authority rule is required. Use an ordinary app-level type (for example instrument.ingest) and existing "about" semantics as appropriate.
- Repeat ingest safely without introducing false duplicate measurements.
- Export the person's data, reconstruct the original receipt/evidence on a fresh machine, verify the original signature and byte hashes with the independent verifier, and reproduce one trivial signal statistic.
- Negative tests: missing artifact, corrupt blob, failed adoption and crash/retry. A failed verification must never be described as a successful instrument measurement.

Deliver one PR, not several speculative extensions, containing executable code, fixture data, tests, ADR and a short README showing commands to run the proof end-to-end.

## Work that must wait for hardware context

Do **not** guess AION's firmware signing capability or MRI raw format.

A real capture requires:

1. One exact file or stream of unmodified acquisition data, plus byte layout, sample type, endianness and sample rate.
2. The acquisition settings actually used, and any calibration/noise/field artifacts required for the selected numerical output.
3. A diagram or explanation of the path from sensor/ADC → FPGA/MCU → host, including which component can hash bytes and which can access the signing key.
4. Any actual signing key constraints (secure element, MCU crypto accelerator, software signer only); do not fabricate manufacturer attestations.

At this gate, state precisely what is missing. If all signing currently occurs after an untrusted host can alter data, label the proof **host-signed** and identify what hardware/firmware change is needed to achieve device-origin evidence.

The test output should include one simple reproducible number calculated from the authentic raw sample and settings. It need not be T2, a diagnostic endpoint, or a clinical claim. Metrological accuracy is a separate experiment.

## Later work — explicitly out of current PR

After one real device acquisition succeeds and two independent verifiers agree:

- Decide and freeze the smallest wire format based on the deployed hardware, with byte-exact vectors and a versioning rule.
- Build a separate user-selected, recipient-encrypted sharing envelope using established recipient encryption (e.g., an explicit HPKE profile and authenticated bulk encryption). The recipient independently verifies the original proof offline.
- Consider optional owner pairing, offline manufacturer endorsement, revocation policy and measured-boot attestation. Treat all of these as policy/assurance layers, not core Bean validity rules.
- Build FHIR/VC projections only when there is a concrete integration partner. Do not make them necessary for evidence verification.

No blockchain, global ordering, blanket web-of-trust score, database-as-truth, new health ontology, mandatory cloud service or manufacturer-controlled permission to leave.

## Three review lenses — run independently after Task 3

**Linus:** Could any type, file, dependency or field be deleted without losing the working proof? Are the bytes and error behavior specified enough for another author to implement independently? Is the implementation small, idiomatic and test-driven? Was core left unchanged?

**Satoshi:** Can the host replace evidence, fake a key, replay a capture, withhold files, reassign authorship or assert false firmware state? Does the verifier distinguish key ownership from physical provenance and physical accuracy? Is verification possible with no AION infrastructure?

**BuildTall:** Can the person leave with the original proof, raw files and calculation method; restore and verify them offline; operate via CLI without accounts; and later disclose to a chosen peer without disclosing the rest of the log? Does any new dependency create enclosure?

The acceptance criterion is an executable artifact with a reproducible result and independently verified provenance, not how complete the design document appears.

## Stop conditions and reporting

Stop and report rather than invent a solution if:

- no device-side trusted acquisition/signing boundary is available (report a host-signed prototype honestly);
- the actual raw evidence or required configuration is impossible to export;
- an external verifier needs a private AION API or licence to validate the evidence;
- the chosen crypto library or profile is insufficiently specified for cross-language verification;
- a proposed convenience requires a Bean core validity change.

When ready, the Codex completion report should be at most: changed files, exact commands/tests passed, proof status, limitations, hardware inputs required, PR URL. Keep all code and detailed tests in the repository.
