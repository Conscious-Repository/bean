# Synthetic evidence proof (experiment only)

Implements Tasks 0–3 of draft [PR #15](https://github.com/Conscious-Repository/bean/pull/15).
Read [the ADR](../../docs/INSTRUMENT-EVIDENCE-ADR.md) for the complete provisional
profile, trust boundary and precise hardware inputs required. This does not freeze
a wire format, implement historical Extension 2, or change Bean core.

Requires Go from go.mod and Python with `tools/requirements.txt` installed.
The experiment's file-safety APIs target Linux/macOS. Install dependencies and
build before going offline. Verifiers never fetch keys or call an AION service.
The Python verifier was written from the ADR and fixture before the Go verifier;
it uses Python cryptography/OpenSSL, not Go code or a translated Go implementation.
The generator also signs with Python; the CLI signs with Go. Neither imports core.

```
python3 -m venv /tmp/bean-proof-env
/tmp/bean-proof-env/bin/pip install -r tools/requirements.txt
make evidence PY=/tmp/bean-proof-env/bin/python
# Linux with unshare/sudo: run the entire corpus in an isolated network namespace
EVIDENCE_OFFLINE=1 make evidence PY=/tmp/bean-proof-env/bin/python
```

End-to-end, in a private directory OUTSIDE this repository:

```
go build -o /tmp/bean ./cmd/bean
go build -o /tmp/bean-evidence ./cmd/bean-evidence
export BEAN_BIN=/tmp/bean
# Use a fresh directory; never put an actual Bean log or secret in this repo.
PROOF_DIR=$(mktemp -d)
/tmp/bean-evidence fixture "$PROOF_DIR/capture"
/tmp/bean-evidence verify "$PROOF_DIR/capture"
/tmp/bean-proof-env/bin/python tools/evidence/verify.py verify "$PROOF_DIR/capture"
/tmp/bean -dir "$PROOF_DIR/log" init
/tmp/bean-proof-env/bin/python tools/evidence/adopt.py ingest "$PROOF_DIR/capture" "$PROOF_DIR/log"
# Retry: added 0, skipped 1
/tmp/bean-proof-env/bin/python tools/evidence/adopt.py ingest "$PROOF_DIR/capture" "$PROOF_DIR/log"
/tmp/bean -dir "$PROOF_DIR/log" verify
/tmp/bean -dir "$PROOF_DIR/log" ls -type instrument.ingest -json
/tmp/bean -dir "$PROOF_DIR/log" export "$PROOF_DIR/export"
# Copy export to another machine. It contains PLAINTEXT; protect it.
# Use the full adoption record id printed by ls above:
/tmp/bean-proof-env/bin/python tools/evidence/adopt.py recover "$PROOF_DIR/export" sha256:REPLACE_WITH_RECORD_ID "$PROOF_DIR/recovered"
/tmp/bean-evidence verify "$PROOF_DIR/recovered"
/tmp/bean-proof-env/bin/python tools/evidence/verify.py verify "$PROOF_DIR/recovered"
```

Both verifiers output signature_valid=true, evidence_complete=true,
source_trusted=false, replay_available=true, accuracy_validated=false,
assurance=host-software, freshness=unknown, mean_square_iq=3317.25. Exit codes
are documented in the ADR. Integrity can succeed with replay unavailable;
adoption records that distinction and does not call it a measurement.

The fixed fixture uses a **public test seed** and a fixed nonce solely for byte
reproducibility. `fixture` uses that same public seed but generates a fresh random
nonce and refuses to overwrite an existing directory. It writes the receipt last.
This is not a production signing command. Anyone can fabricate these signals and
sign them. Re-signing arbitrary host data cannot demonstrate device origin.

`adopt.py` stores a verified private snapshot as ordinary owner-encrypted Bean
blobs and asks `bean append` to sign the adoption. It checks stored blob readback.
No instrument author is impersonated. Dedupe uses signer key + nonce; the original
receipt digest and blob addresses are separately recorded. Concurrent local adapters serialize check/append using a separate local lock.
A post-append check rejects conflicting nonce reuse or malformed retained artifact
references even when another writer bypasses that lock. This is local
coordination, not distributed consensus. Orphan encrypted
blobs can remain after a crash before append. Retry is safe. Do not delete blobs
manually from a real log. Existing purge/dedupe semantics still apply.

`recover` uses export metadata only to locate files, then checks the original
receipt hash, signature and evidence. It does not attest owner authority; use
`bean verify` or the independent Bean verifier for that. It refuses existing output
and publishes the recovered directory only after verification. No identity secret
or AION account is needed to verify a plaintext export.

Tests cover exact fixture regeneration, both signers/verifiers, tampering, bounds,
symlinks/FIFOs/traversal, duplicate/reordered entries, missing method, unknown
issuer, fabricated host evidence, concurrent adoption/conflicting nonce, missing
artifact, corrupt stored/exported blob, failed signing and crash/retry.
The exit drill deletes the original capture/log and runs a copied kit in a fresh
Python environment with network access disabled and the checkout inaccessible.
macOS uses sandbox-exec; root Linux CI uses mount/network namespaces. Unsupported
local isolation environments skip that one drill. A physical second machine and
an actual AION acquisition remain untested.

Build a platform-specific kit outside the repository (preparation can download
wheels; set EVIDENCE_WHEELS to reuse a prepared wheelhouse):

```
make evidence-kit PY=/tmp/bean-proof-env/bin/python OUT=/absolute/new/kit
/tmp/bean-proof-env/bin/python tools/evidence/acquisition.py simulate /absolute/new/capture
```

The kit includes both verifier implementations, method/profile, public fixture,
licenses and offline wheels. Building checks both verifiers against the fixture
using a fresh Python environment installed only from those wheels. Failed builds
may leave an incomplete output; discard it and choose a new destination. Its
inventory hashes are not release authentication. Compatible OS/CPU and Python
must already be available; it does not ship a runtime.

The simulator exercises applied settings, completion, abort/reset, nonce rotation,
immutable completion retries and fragmented/interrupted/corrupt transfer. It is
host software with a public key, not a secure device. Transfer verification fails
before publication on read faults; a publication failure can leave an incomplete
directory without a receipt. Use a new destination for retry. See
[the pipeline plan](../../docs/EVIDENCE-PIPELINES.md) for hardware acceptance gates
and the Apple Health importer handoff.

Review lenses: minimality (two files, no new dependency, no core change), adversary
(host can lie before signing, cannot change committed bytes undetected; source
trust/freshness/accuracy remain unestablished), and exit (plaintext export + offline
verification + documented computation, with no account or licence gate). No
independent human/security audit or physical accuracy validation is claimed.

Run `make check` for the complete local checks. CI runs all `test_*.py` evidence
suites offline, plus Bean tests, independent conformance and differential checks.
See the implementation PR for results at the current revision. Both verifiers
reject small-order/noncanonical key and R encodings, including the identity-key
forgery accepted by the underlying libraries alone.
