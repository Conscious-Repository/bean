# ADR: synthetic host-signed evidence experiment

Status: disposable spike implementing Tasks 0–3 of draft PR #15, based on
main. This is neither a frozen wire format nor an implementation of Extension 2.
No Bean core rule changes. No real AION acquisition has been tested.

The simulated boundary is host memory/files after acquisition. The host owns
both mutable samples and the signing seed. It can fabricate every sample and
sign a fresh lie. The verifier always reports `host-software`, never device
attestation, even if a signer claims otherwise. Signature validity authenticates
only possession of the embedded key; source trust and accuracy remain false.

We evaluated COSE_Sign1 (RFC 9052), which remains a candidate for the real
hardware. This spike uses JWS Compact Serialization (RFC 7515) with the fully
specified `Ed25519` algorithm identifier (RFC 9864, updating RFC 8037).
Go crypto/ed25519 and Python cryptography provide independent implementations;
no new dependency is needed. The small JWS wrapper is deliberately restricted,
not a general JOSE library. Hardware COSE support or an existing device receipt
can overturn this choice. Do not extend this experiment into a production SDK.

## Written experiment profile (for independent implementations)

A directory contains exactly `receipt.jws`, `raw.bin`, `settings.json`.
Files must be regular, never symlinks; the directory itself is not a symlink.
Receipt limit 16384 bytes, raw limit 1048576 bytes, settings limit 4096 bytes.
Limits are experiment implementation limits, not Bean validity rules.

Receipt is exactly three unpadded canonical base64url segments separated by dots,
without trailing newline. Signature input is the ASCII first two segments joined
by a dot. Signature is 64 bytes, public key 32 bytes. Protected header has exactly
`alg`, `typ`, `jwk`; alg is `Ed25519`, typ is `bean-evidence-experiment`.
JWK has exactly `kty: OKP`, `crv: Ed25519`, `x: <base64url public key>`.
No URL lookup, certificate, implicit trust, unprotected headers or algorithm fallback.
Additionally, both public key and signature R must have canonical y < 2^255-19
and must not encode a small-order point. Mask the sign bit: reject y = 0, 1,
p-1, t, p-t where t = 0x05fc536d880238b13933c6d305acdfd5f098eff289f4c345b027b2c28f95e826.
Library verification checks the remaining curve/signature equation. This explicit
restriction is necessary because both tested libraries accept the identity-key,
identity-R, zero-S forgery under their default verification rules.

Payload has exactly `profile: bean-evidence-experiment`, `capture` (32 lowercase
hex nonce), `complete: true`, `assurance: host-software`, and `artifacts`.
Artifacts is exactly two entries in order: role `raw`, name `raw.bin`; then role
`settings`, name `settings.json`. Each entry has exactly name, role, length
(positive integer within its limit), sha256 (64 lowercase hex). No arbitrary
paths or optional hidden artifacts. Every digest commits to original bytes.
Receipt JSON is compact, keys sorted lexically, ASCII with no escapes, nesting
at most 8, integers only within ±(2^53−1), no negative zero; decode/re-encode must match. This narrow restriction
removes parser ambiguity in the experiment; it does not define general JCS.
Check supported header/profile, signature, manifest structure, then file bytes.
Hash files in bounded chunks before numerical interpretation.

Settings for replay: exactly `format: complex-i16-le`, `frames` (positive integer),
`sample_rate_hz` (positive integer), `method: mean-square-iq-counts@1`.
These describe interleaved signed 16-bit little-endian I,Q pairs in sample order.
Raw length must equal frames * 4. Method: sum(I*I + Q*Q) / frames, in ADC-counts².
Integer sum fits exactly within 2^53 at this experiment's size limit. No physical
units, calibration or clinical claim. Missing/unknown method or invalid settings
leaves integrity intact but replay unavailable. A calibrated result will require
a different experimental profile binding its actual calibration assets.

JSON output separates signature_valid, evidence_complete, source_trusted,
replay_available, accuracy_validated, assurance, freshness (`unknown`), status,
and optional mean_square_iq. Exit 0: integrity complete (replay can be unavailable);
2: invalid receipt/bundle; 3: unsupported profile/algorithm; 4: signature invalid;
5: evidence mismatch/missing; 1: CLI/adapter operational failure.

## Fixture and adoption

The deterministic fixture is a toy decaying quadrature signal, not AION's raw
format: (100,0), (0,50), (-25,0), (0,-12), at 1000 samples/s. Result: 3317.25.
Its public test seed is bytes 00..1f. A fixed nonce is used ONLY for reproducible
vectors. `fixture` generates a fresh nonce; no production signing interface is
provided. Receipt is written last after both artifacts have been finalized.

Adoption uses existing bean blob put / append. Verify a private staged snapshot
first; encrypt those same bytes; only then append instrument.ingest. Native id
is embedded public key + capture nonce (not file hash), occurrence 0. Receipt
SHA-256 in context is over original envelope bytes. Retry uses Bean dedupe;
crash before append may leave harmless encrypted orphan blobs; no measurement
claim exists until append succeeds. Concurrent local adapters serialize
check/append with a separate local lock;
a post-append digest/artifact check detects conflicting or malformed retained
claims from writers bypassing that lock. This is not cross-device coordination.
A reused signer nonce is a conflicting capture, not a new
measurement; operators must inspect the original preserved receipt. The adapter
records adoption, not a biological measurement. Export recovery reports
verification_scope=capture_evidence_only and owner_authority=not_checked;
verify the Bean log separately to establish owner authority.
Export is plaintext: protect it.

## Hardware gate and deferred work

Obtain one unmodified AION raw capture and exact byte layout, framing/order,
integer/float scaling, endianness, sample rate and completion/overflow behavior;
settings actually applied (not requested settings), sequence/timing, gain and
filter/decimation; calibration/noise/field files needed by one chosen computation;
a sensor/ADC → FPGA/MCU → DMA/host data-path diagram with mutable buffers and
where hashes are finalized; board/firmware revisions; signing API, supported
algorithms/envelopes, secure-element/key provisioning/export policy and who can
request signatures. Establish whether the host can submit arbitrary hashes to
that key. Key isolation alone is not acquisition attestation. Device-origin
requires firmware binding finalized ADC data and applied settings to the receipt
inside an authenticated acquisition boundary before host modification, plus
independent evidence supporting that boundary. Pairing/endorsements, trustworthy
time, firmware attestation, registry, sharing and physical accuracy are deferred.
A real acquisition and a second physical machine remain exit gates.

References: https://www.rfc-editor.org/rfc/rfc7515,
https://www.rfc-editor.org/rfc/rfc8037,
https://www.rfc-editor.org/rfc/rfc9864,
https://www.rfc-editor.org/rfc/rfc9052.
