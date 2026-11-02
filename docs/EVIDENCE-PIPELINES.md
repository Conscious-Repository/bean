# Evidence pipelines while hardware is unsettled

This is an implementation plan, not a new protocol. The receipt in
INSTRUMENT-EVIDENCE-ADR.md is disposable. Bean core stays source-independent.

## Software now

`tools/evidence/acquisition.py` models start, acquire, finalize, abort/reset and
bounded reads from a completed capture. Finalize takes no host-provided digest;
it binds the simulated acquisition buffer and settings actually applied. Retrying
finalize returns the same immutable receipt. Reset starts a new nonce. Aborted,
overflowed or incomplete acquisitions cannot produce a complete receipt.

All of this runs in host-controlled Python with a public test key. Neither method
encapsulation nor a signature demonstrates hardware isolation, freshness, accuracy
or who was measured. The four-frame I/Q signal is a test fixture, not an AION
measurement format. The verifier/replay reports keep these distinctions explicit.

Software can now exercise transfer interruptions, truncation, extra bytes,
corruption, completion retry, Bean adoption/deduplication and offline recovery.
Publication writes the receipt last; a filesystem failure can leave a partial
output, which fails verification. Retry in a new destination. There is no durable
resume protocol or cross-device capture consensus here.

## Hardware team, independently

Before a real acquisition, supply:

| Needed context | Acceptance question |
| --- | --- |
| Sensor, ADC, FPGA/MCU, DMA and host data-path diagram, including mutable buffers | Where can samples/settings change before the signed digest is finalized? |
| Board/firmware revision, boot/update/debug policy and acquisition state machine | What running code owns completion, overflow and signing? Can the host bypass it? |
| Key storage/provisioning/export policy, signing API, algorithms and native envelope examples | Can the host ask the device key to sign arbitrary bytes/hashes? How is a public key associated with this device? |
| One unchanged raw capture, exact framing/order/endianness/scaling, actual sample rate | Can another implementation decode the same samples? |
| Applied gain, filters/decimation, sequence/timing and completion/error indicators | Are these values acquired from the running hardware and bound with the samples? |
| Calibration/noise/field inputs and one named computation with expected result/tolerance | Can an offline implementation reproduce that result, without implying clinical accuracy? |
| Power-loss, abort, overflow, retry and reset behavior | Can an incomplete capture be mislabeled complete or a capture identity reused? |

Key isolation alone is insufficient. The real acceptance test is that acquisition
firmware binds the actual completed buffer and applied settings before host
modification, and host-controlled input cannot obtain the same device-origin
assurance. Evidence supporting this boundary must be supplied and reviewed; a
self-declared assurance string cannot establish it.

When hardware is ready, add its native envelope verifier/driver alongside the
experiment. Reuse behavioral tests where appropriate, then test one real capture,
its tampered/aborted variants and recovery on a physical second machine. Do not
require hardware to emit the synthetic JWS profile or use Ed25519 if its native
supported signing mechanism differs. Reconcile limits/encoding at that point.

## Apple Health next; other sources follow the same principle

Apple Health is the selected first importer. Its implementation is gated on a
real `export.xml` or `export.zip` supplied outside the repository (CONTRIBUTING.md).
No such file has been supplied yet. Inspect it locally; commit only synthetic
fixtures that reproduce its structure. Do not infer a native identifier merely
from a field name, or assume the export provides one.

Preserve the original file as an encrypted Bean blob and emit ordinary claims
through `cmd/bean-import-apple-health`, with versioned adapter provenance,
source-provided times/units and explicit handling for missing/ambiguous fields.
Start with low-frequency readings. Decide dedupe and revision/correction behavior
from the actual export; test repeated imports, overlapping exports and changed
readings. Keep dense series as source data until batching semantics are tested.
An Apple Health export may contain readings originally contributed by other apps;
preserve that provenance without calling it cryptographic device attestation.

WHOOP, Oura and Withings each get a source-specific package only after a real
export/API sample and revision behavior are available. Imported source statements,
owner adoption signatures, host-generated evidence signatures and independently
established device-origin evidence are separate claims. None should silently
upgrade the others. External sources need not mimic I/Q capture artifacts.

## Ordering and merge gates

1. Merge the tested software harness/kit and local ingestion fixes, without
   changing core or declaring a stable external hardware interface.
2. Implement Apple Health from the supplied export. Hardware investigation can
   continue independently; neither blocks the other's fixtures or tests.
3. Integrate the real hardware adapter after the acquisition-boundary review and
   real replay/exit drill. Keep unsupported assurance explicit.
4. Design selective sharing and attestation around actual consumption needs and
   threat models after these producers work. Homomorphic encryption is deferred:
   no current test requires computation on encrypted data, and it would not prove
   that a device measured the signed input. Revisit only for a specific private
   computation with named parties, leakage limits and measurable cost.

The week’s achievable software milestone is a reproducible loop and an actionable
hardware handoff. Hardware-origin assurance cannot be completed before the
hardware/firmware boundary and real capture are available.
