# Instrument evidence contract — proposal v0.1

Status: DESIGN PROPOSAL. Not implemented, not a change to Bean core v0, and not frozen.
Owner: Bean contributors. First customer: AION instrument prototype.
Decision gate: an actual acquisition, independently verified without an AION account.
This document is intended to be executable as conformance tests, not an aspirational architecture.

## 0. Axioms

1. The instrument is the original source of a measurement. The host is a courier and storage agent, not its author.
2. The instrument must commit to the exact original evidence before an untrusted host can modify that evidence.
3. A person can receive the complete evidence and proof, retain both, verify without a vendor, and move both to another application.
4. A third party must independently distinguish signature validity, device provenance, evidence availability, computation reproducibility, and biological accuracy. None implies the next.
5. No instrument must know a person's Bean identity, use AION's account system, or connect to the internet to produce a valid receipt.
6. Disclosure is initiated by the person; possessing a valid receipt is not permission to see the evidence.
7. No new crypto algorithms or authority are invented here. Trust is a decision of the verifier; AION may endorse devices but may not monopolize verification.
8. Add capabilities in an extension. Do not alter the meaning of an existing valid Bean record merely to accommodate instruments.

A successful outcome: a disconnected instrument captures raw bytes, signs one immutable evidence receipt, and hands both to a person. That person can import the artifacts into Bean, disclose selected evidence to another person, and have a completely independent implementation verify the original signature and reproduce an agreed measurement calculation.

## 1. The primitive: receipt, evidence, adoption

Three different artifacts are required; do not collapse their authorship.

**Instrument receipt.** A compact, instrument-signed statement committing to the full set of capture artifacts, acquisition context, and relevant declared instrument state. The receipt is independent of any patient, account, host, or Bean log. It must be issued by the device-side acquisition trust boundary, not re-created from host-observed metadata after the fact.

**Evidence.** Immutable byte sequences emitted by the instrument, including primary sensor samples and the acquisition/configuration files needed to interpret them. The receipt includes an exact length, format identifier, and SHA-256 digest for every required sequence. Encrypted storage IDs are not evidence IDs.

**Owner adoption.** An ordinary, user-signed Bean claim linking the verified receipt and its raw files to blobs under the person's control. It says the owner chose to preserve those artifacts; it does not replace or impersonate the instrument signature. A recipient can verify the original receipt without possessing the owner's signing key or Bean secret.

Why not require a device-authored Bean record? Bean v0 requires a subject identity. A device might not know the person at capture time. Forcing an identity into the measurement would break anonymous acquisition and make the original receipt unnecessarily linkable. An optional direct device-authored Bean claim may be added where prior subject pairing is available, but it must reference the same independently signed receipt.

The design uses a standard COSE_Sign1 receipt (RFC 9052, signature algorithms RFC 9053), not a second proprietary signature construction. The receipt payload uses deterministic CBOR (RFC 8949). The existing Bean record remains RFC 8785 canonical JSON. Each format has a different, explicit trust role; the conversion between them is verified, never silently normalized.

## 2. Threat model and actual guarantees

Assume a malicious or buggy host, damaged storage, arbitrary file substitution, missing/reordered bytes, a replayed capture, a counterfeit device key, deceptive metadata, an unreliable clock, and software defects in processing. Do not assume the manufacturer's firmware is benign. Model physical attacks and compromised signers separately.

| Adversary or failure | What the verifier can establish | What it cannot establish |
|---|---|---|
| Host changes any captured byte | Digest or length mismatch against signed receipt | Whether instrument ADC measured the claimed physical phenomenon |
| Host changes calibration/configuration | Signed artifact digest mismatch | Whether original calibration was correct |
| Host drops an artifact | A required artifact is missing | Whether an entirely withheld capture ever existed |
| Host substitutes another instrument | Signature key mismatch, unless verifier trusts both | Which physical unit owns a key without pairing/endorsement |
| Signer key stolen | Only that the key signed | That the actual original instrument produced the data |
| Firmware lies about signal | The software produced a correctly signed false statement | Biological or metrological truth from cryptography alone |
| Clock is wrong | Reported device time and optional clock comparison | Trusted chronology without a trusted clock or witness |
| Capture replay | Repeated capture ID detectable when prior state is present | Global freshness when verifying an isolated receipt offline |
| AION disappears | Verify with receipt, public key, bytes, open formats, and reference verifier | Recover bytes never delivered or proprietary computation never published |

Trust is a ladder, not one boolean:

- T0 — receipt signature is mathematically valid for the embedded public key.
- T1 — key was explicitly paired by the subject, using a separate owner-signed claim.
- T2 — key has independently checkable device/manufacturer endorsement accepted under the verifier's policy.
- T3 — measured boot/firmware attestation and calibration claims are independently appraised under a stated policy.
- T4 — interpretation is reproducible from the supplied bytes, calibration, method, and parameters.
- T5 — accuracy/reproducibility have been empirically validated against appropriate physical references.

No tool may label T0 as T2, or T4 as T5. Manufacturer endorsements are optional verification inputs, not a network gate. Do not introduce a universal device reputation score.

## 3. Receipt wire contract: proposed profile 0.1

This is a proposed single-signer, single-capture profile. The formal CBOR key map and vectors will be frozen only after the first independent implementation.

- A receipt is a CBOR tag-18 COSE_Sign1 object with an embedded payload.
- COSE protected headers MUST contain the algorithm and a device-key identifier. Unprotected headers MUST be empty. External AAD MUST be the empty byte string.
- The payload and protected-header map MUST use RFC 8949 core deterministic encoding, with definite-length maps/arrays and no duplicate keys. Reject alternate encodings, indefinite lengths, unrecognized critical headers, and unsupported algorithms.
- Only Ed25519 (COSE EdDSA) and P-256 with SHA-256 (COSE ES256) are admitted in this draft. The public key is an embedded public-only COSE_Key. Protected key identifier is the SHA-256 of the deterministic CBOR public COSE_Key bytes. The key ID is a locator, not manufacturer trust.
- The device MUST compute hashes of the bytes emitted by its acquisition path and sign a complete receipt only after the declared capture ends. A host-provided digest is not adequate if the device cannot authenticate it as the captured byte stream.
- A finished receipt MUST be no more than 1 MiB in this profile; all evidence files are streamed and hashed with bounded memory. The maximum accepted artifact count is 4096.
- The receipt hash is SHA-256 of its exact COSE_Sign1 bytes. The signed payload has a profile discriminator that prevents reuse as another kind of statement.
- The ES256 signing path SHOULD produce low-S signatures; the final conformance profile must explicitly fix acceptance and normalization before implementation freeze. No implementation may silently rewrite an already archived receipt.

Proposed payload fields (shown in JSON notation for readability; actual wire bytes are deterministic CBOR):

~~~json
{
  "profile": "bean.instrument.receipt/0.1",
  "capture_id": "16 random bytes (CBOR byte string)",
  "status": "complete",
  "device_key": "public COSE_Key (CBOR map)",
  "device": {
    "model": "device-model-id",
    "firmware_digest": "32 digest bytes or null",
    "firmware_basis": "attested | self-reported | unknown"
  },
  "clock": {
    "start": "device-claimed time or null",
    "end": "device-claimed time or null",
    "basis": "untrusted | trusted-external"
  },
  "artifacts": [
    {
      "index": 0,
      "role": "raw",
      "format": "stable-format-identifier",
      "length": 123456,
      "sha256": "32 digest bytes"
    },
    {
      "index": 1,
      "role": "acquisition-config",
      "format": "stable-format-identifier",
      "length": 512,
      "sha256": "32 digest bytes"
    }
  ],
  "calibration": {
    "status": "provided | not-applicable | unavailable",
    "artifact_indices": [2]
  }
}
~~~

The quotation marks around byte descriptions are illustrative. The final CBOR payload MUST have the actual data types described here; strings containing placeholders are not valid examples to sign.

Required interpretation:

1. "profile" is exact, "capture_id" is a device-generated 16-byte nonce, and "status" is "complete" for a completed measurement. Other statuses may be retained but MUST NOT verify as complete.
2. "artifacts" is nonempty, ordered by strictly increasing index starting at zero, without gaps. Every referenced file MUST exist once, be nonempty when its role requires measurements, and match its declared length and SHA-256 exactly. The receipt commits to the number and order of artifacts.
3. At least one "raw" artifact and one "acquisition-config" artifact are required. Raw format and acquisition settings must be enough to parse the acquisition without asking AION for an API response or schema.
4. Each artifact uses exact bytes. No reserialization, whitespace normalization, endianness conversion, or content-specific parsing may precede digest verification.
5. Calibration status is explicit. "provided" requires every listed calibration artifact to be present and verified; "unavailable" must not be interpreted as quantitative readiness. For quantitative MRI endpoints, require specific calibrations in a separate profile.
6. Firmware digest is a declaration unless accompanied by independently appraisable measured-boot evidence. The device model string is never proof of manufacturer identity.
7. The receipt contains no person's name, global subject ID, account number, or health reading by default. A stable device public key may still correlate captures; unlinkable device credentials are a future privacy profile.
8. A clock reading does not make the capture fresh or prove ordering. Counter-based replay checks may be added by device profile if nonvolatile monotonic state exists.

The signed receipt MAY include a versioned extension object, but a verifier MUST report it unsupported rather than interpret an unknown security-critical field. The initial schema should define which fields affect verification and refuse ambiguity. Binding to an acquisition software hash or calibration reference belongs inside the signed payload or inside a digest-committed artifact, never in unsigned host metadata.

## 4. Capture and transfer protocol

**Device-side sequence:**

1. Generate a random capture ID and instantiate a capture session. Select the exact encoding and provide it as a signed artifact.
2. Produce acquisition configuration, calibration context, and raw samples through a trusted acquisition path. Hash each exact file/stream incrementally as it is emitted, while counting bytes.
3. Determine whether all expected data and status checks completed. A partial/aborted capture cannot be labeled "complete".
4. Finalize the artifact table, device-declared state, and clock claims. Produce COSE_Sign1 with the device key. Seal the receipt after the data and digest table are final; never ask an untrusted host to retroactively state what the device captured.
5. Deliver the exact signed receipt and raw bytes to the holder over a local transport (files, USB, wired serial, or an authenticated wireless channel). No account, internet, or manufacturer service is required. The holder may copy them indefinitely.

**Transfer folder in the reference implementation:**

    capture/
      receipt.cose
      artifacts/
        0000
        0001
        ...
      endorsements/
        optional-files

Artifact filenames are indexed by the signed table. Transport filenames themselves are not security assertions. The receiver MUST reject symlinks, path traversal, extra undeclared data presented as required evidence, duplicate entries, truncated files, overlong files, and non-regular files. It MUST verify with bounded memory and fail closed. It may keep unknown optional transport files only as clearly unverified extras.

**Person-side sequence:**

1. Verify the receipt signature, deterministic encoding, accepted algorithms, and key-binding consistency, then verify every artifact digest/length, before treating the capture as complete.
2. Apply owner pairing or a locally chosen trust policy. The same signature can be cryptographically valid and untrusted.
3. Store the receipt and raw artifacts in the person's Bean blob store, encrypting with the existing owner-controlled blob key. The encrypted Blob IDs are *not* the raw evidence hashes.
4. Add a normal owner-signed Bean adoption claim, type "instrument.ingest", subject = author = owner identity. The claim includes the signed receipt's SHA-256, artifact indices, raw hashes, and local encrypted Blob IDs, with those blobs in the Bean "blobs" list. This claim is the owner's inventory, not an instrument signature.
5. On every later read, decrypt blobs and re-check the receipt and raw hashes. The ingest claim alone is insufficient to establish valid instrument provenance.
6. Preserve the originally signed receipt byte for byte. Never modify/re-sign it on behalf of the device.

This works with the existing core: an instrument receipt is just an encrypted blob for persistence, and the ingestion record is a standard Bean claim. No new Bean validity rule or cryptographic primitive is needed.

The separate existing draft "device.pair" claim can express the owner's trust in an instrument public key. It MUST NOT convert a self-signed key into an AION-endorsed hardware identity.

## 5. Processing, biological interpretation and reproducibility

Do not conflate acquisition with processing.

- An instrument receipt authenticates exactly which evidence and acquisition context its signer committed to.
- A derived-result claim is a new signed event by the actor that computed it, referencing the exact receipt hash and/or prior derivation hash, input byte digests, method identifier + version + immutable digest, parameter digest, calibration artifacts, output and uncertainty.
- A device may sign its own computed result, but a host computation must be attributed to the host/app, not retroactively to the device.
- Every reported numeric measurement states its unit, sampling region and method, with uncertainty or an explicit "not estimated" marker. For MRI, this includes applicable field configuration, sequence, coil/sensitivity context, noise handling, reconstruction version, region selection and motion or drift quality flags when relevant.
- Exact reproducibility applies only to specified deterministic algorithms and supplied inputs. Floating-point numerical tolerance, proprietary algorithms or nondeterministic reconstruction must be stated explicitly; a claim of bit-exact replay is not automatic.
- A verifier can report "signature valid", "evidence complete", "method reproducible", and "physical/clinical accuracy assessed" independently. The last requires actual validation and cannot be inferred from a hash.

For the first AION proof, use a small raw measurement (such as a single documented NMR acquisition), include its acquisition settings and calibration inputs, and reproduce one simple numerical output in an independent application. Do not claim validated T2, clinical accuracy or FDA status until supported by separate evidence.

## 6. Disclosure and portability

The initial workflow can deliver a receipt and files offline to the owner. It does NOT claim confidentiality when those files travel over an unprotected channel.

Later, build a selective-sharing envelope on top of these artifacts:

- The person selects particular receipts, raw/processed evidence and optional user claims. Never implicitly include the full Bean log or local key material.
- Encrypt the selected material for the recipient using a standard HPKE construction (RFC 9180) with a dedicated encryption key, not an improvised conversion of Bean signing keys. Specify the exact suite, envelope encoding, AAD and version in the sharing extension.
- Include the original signed receipt inside the encrypted disclosure unmodified. The recipient verifies the COSE signature and byte digests after decrypting, and can independently compute results without AION services.
- If a disclosure excludes a required artifact, the recipient sees "incomplete", not "valid evidence".
- A delivered recipient can copy plaintext. Access revocation cannot erase past disclosures; future access and new keys are distinct.
- Plain SHA-256 values and stable device public keys can leak equality or link captures. Do not publish receipts or plaintext Bean records on public relays by default. Encrypt transport and restrict replica access. Do not market Bean v0's plaintext record folder as encrypted end to end.

Adopt FHIR Provenance and W3C Verifiable Credentials at interchange boundaries when useful; do not duplicate those standards in the receipt kernel. The receipt is a verifiable physical-source artifact, not a complete medical record or a trust registry.

## 7. Security invariants and conformance plan

A candidate implementation is not conformant until two independent implementations agree on positive and negative fixtures. Fixtures contain synthetic signals and test-only keys, never real patient data.

| Case | Input/mutation | Required outcome |
|---|---|---|
| R01 | Known-good COSE receipt and two exact files | Cryptographic signature valid, evidence complete |
| R02 | One bit flipped in raw samples | EVIDENCE_DIGEST_MISMATCH |
| R03 | Calibration file replaced | EVIDENCE_DIGEST_MISMATCH |
| R04 | Acquisition settings missing or mutated | EVIDENCE_MISSING or EVIDENCE_DIGEST_MISMATCH |
| R05 | Length truncated or appended | EVIDENCE_LENGTH_MISMATCH |
| R06 | Artifact missing, duplicated or reordered in table | INCOMPLETE or BAD_RECEIPT |
| R07 | Signature altered, wrong key, algorithm mismatch | BAD_SIGNATURE or BAD_KEY |
| R08 | Same receipt on two machines | Same receipt hash and verification verdict |
| R09 | Valid self-signed key, no recognized pairing/endorsement | SIGNATURE_VALID, SOURCE_TRUST_UNESTABLISHED |
| R10 | Different owner adopts same original receipt | Original COSE bytes and receipt hash unchanged |
| R11 | Corrupt or missing owner blob | Receipt/evidence verification fails closed |
| R12 | Capture ID replay against local previous state | REPLAY_DETECTED; isolated offline verifier reports FRESHNESS_UNKNOWN |
| R13 | Untrusted clock or self-reported firmware | No trusted-time or measured-boot claim emitted |
| R14 | Malicious transfer path/symlink/huge size | Fail with bounded memory; no path escape |
| R15 | Partial capture incorrectly marked complete | Reject against signed expected artifact set; missing internal samples require device-side controls |
| R16 | No network, AION server or AION account | Complete independent verification still succeeds |
| R17 | Different authorized recipient key | Only selected recipient can decrypt a future encrypted bundle |
| R18 | Missing processing method or calibration | Cryptographic capture may pass; replay is explicitly unavailable |
| R19 | Two independent implementations on all fixtures | Identical typed outcomes; no unexplained interpretation differences |

Produce byte-for-byte fixture vectors for COSE signing, key IDs, artifact hashing, and signature failures, plus malicious folders and real-binary acquisition fixtures. Test compromised host assumptions with a host that attempts to replace evidence after the device has signed. Run a hardware fault/motion and calibration validation program separately; software conformance cannot prove metrology.

## 8. Non-goals

No new blockchain, token, global registry, centralized server, health ontology, private AION API dependency, universal reputation metric, user identity embedded in every capture, or clinical-accuracy claim. No automatic trust of firmware claims. No protocol-level deletion of a recipient's already disclosed plaintext.

Do not modify CORE.md or SPEC.md until a concrete need for a validity-rule change is demonstrated. A device evidence profile can be independently standardized and upgraded without that change.

## 9. Open design gates before implementation freeze

D1. Confirm that subject-independent signed receipts are necessary for at least one real instrument workflow. This draft favors them to avoid requiring an owner identity during acquisition.

D2. Identify where the acquisition bytes are hashed and where the signature key lives: FPGA/MCU, trusted secure element, host software, or an isolated trusted execution environment. The current prototype may only support software signing; label it truthfully.

D3. Agree on one *real* raw sample file and exact byte format plus acquisition configuration, calibration content, and one reproducible numerical output. Do not infer formats from marketing claims.

D4. Fix the COSE algorithm/key-ID/CBOR profile and publish vectors after independent implementation review. Confirm ES256 low-S treatment with actual selected hardware.

D5. Specify calibration trust and measured-boot provenance separately: self-reported hashes cannot be upgraded to hardware attestation merely through a label.

D6. Decide whether user pairing is local only, whether manufacturer endorsement is carried offline, and what to report for unknown or stale revocation state.

D7. For sharing, choose a precise HPKE suite and envelope format *after* raw receipt verification and user adoption work end to end.

## 10. Standards relied upon

- RFC 9052 / 9053 — COSE_Sign1 and algorithms: https://www.rfc-editor.org/rfc/rfc9052.html
- RFC 8949 — deterministic CBOR: https://www.rfc-editor.org/rfc/rfc8949.html
- RFC 9334 — RATS distinction between Evidence, Endorser, Verifier, Attestation Result, and relying-party policy: https://www.rfc-editor.org/rfc/rfc9334.html
- RFC 9180 — HPKE for future recipient-directed disclosure: https://www.rfc-editor.org/rfc/rfc9180.html
- C2PA technical specification — hash-based content binding and provenance patterns, as a reference rather than a dependency: https://spec.c2pa.org/specifications/specifications/2.1/specs/C2PA_Specification.html
- W3C VC Data Model 2.0 — credential issuance/disclosure interoperability, not a replacement for signed raw-signal evidence: https://www.w3.org/TR/vc-data-model-2.0/
