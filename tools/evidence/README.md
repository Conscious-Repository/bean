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
receipt digest and blob addresses are separately recorded. Sequential conflicting
reuse of a nonce is rejected; simultaneous conflicting imports are subject to
Bean's ordinary dedupe (one primary, not another measurement). Orphan encrypted
blobs can remain after a crash before append. Retry is safe. Do not delete blobs
manually from a real log. Existing purge/dedupe semantics still apply.

`recover` uses export metadata only to locate files, then checks the original
receipt hash, signature and evidence. It does not attest owner authority; use
`bean verify` or the independent Bean verifier for that. It refuses existing output
and publishes the recovered directory only after verification. No identity secret
or AION account is needed to verify a plaintext export.

Tests cover exact fixture regeneration, both signers/verifiers, tampering, bounds,
symlinks/FIFOs/traversal, duplicate/reordered entries, missing method, unknown
issuer, fabricated host evidence, repeat adoption, conflicting nonce, missing
artifact, corrupt stored/exported blob, failed signing and crash/retry. The exit
drill deletes the source log/capture and verifies byte-identical recovery in a
fresh directory. That simulates a fresh machine; a physical second machine and
an actual AION acquisition remain untested. CI repeats this with networking disabled.

Review lenses: minimality (two files, no new dependency, no core change), adversary
(host can lie before signing, cannot change committed bytes undetected; source
trust/freshness/accuracy remain unestablished), and exit (plaintext export + offline
verification + documented computation, with no account or licence gate). No
independent human/security audit or physical accuracy validation is claimed.

Validation on macOS (Go 1.27.3, cryptography 50.0.2): `make evidence` ran 37 tests
with only the two Linux network-namespace assertions skipped. `make fmt vet build
test vectors second demo differential` passed: independent Bean conformance
45/45, scenario agreement 14/14, differential 331 same / 0 different (274 with
rotation). `make lint vuln` passed; govulncheck found no vulnerabilities. Both
verifiers reject small-order/noncanonical key and R encodings, including the
identity-key forgery accepted by the underlying libraries alone. Hosted CI is
the authoritative Linux/offline run; see the implementation PR checks.
