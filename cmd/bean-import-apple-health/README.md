# Apple Health file ingestion

This adapter reads a real Apple Health `export.xml` or `export.zip` layout. It
requires no bridge, Apple account login, phone app, network or new dependency.
Only synthetic fixtures are committed. Bean core is unchanged.

## Default: preserve the raw export

Install `bean` and this importer side by side (`go install ./cmd/bean
./cmd/bean-import-apple-health`), then, in an existing log outside this
repository:

```
bean import apple-health export.zip
```

`bean import` snapshots the original file privately, validates it before writing
anything to Bean, stores the exact input as an encrypted blob, checks plaintext
readback, then appends one `apple-health.export` claim. Its value references that
blob; context records original-file SHA-256, byte length, per-type record counts,
activity-summary count and `assurance=unsigned-export`. It checks that the archive
claim about the owner survived append/dedupe; an owner-authored claim about
another subject cannot satisfy this check. Identical bytes retry as `added 0, skipped 1`.
A changed export is a separate snapshot, not a replacement or extra measurement.
No numerical values are summed. Archive entries cannot be mistaken for steps.

`bean import` does not initialize a log, handle keys or invent an identity. Use
`bean -dir /private/new/log init` if you need a new log, and keep the recovery
words offline. Raw exports and exported Bean blobs contain personal data.

The standalone command follows other importers' JSONL convention:

```
/tmp/bean-import-apple-health -blob sha256:REPLACE_WITH_BLOB_ID export.zip > claims.jsonl
```

Here `-blob` is a caller-provided reference to the **exact input** stored with
`bean blob put`. The standalone converter cannot authenticate that relationship:
Bean blob IDs hash ciphertext, not plaintext. `bean import` enforces readback.
Do not pipe converter output into append unless the converter exited successfully.
The converter stages its output until the complete XML/ZIP CRC passes validation,
but an output-device failure can still interrupt stdout delivery.

## Optional individual readings

```
bean import apple-health -records -types apple-health.steps,apple-health.distance-walking-running export.zip
```

Omit `-types` to emit all supported readings. The archive claim is still included.
Large exports may contain hundreds of thousands of readings, each a separate
Bean record in this mode; archive-only is the default. No daily batching or
cross-source aggregation is inferred.

Sixteen observed source types are supported: height, body mass, steps,
walking/running distance, basal/active energy, flights climbed, headphone exposure,
walking double support/speed/step length/asymmetry/steadiness, sleep-duration goal,
sleep analysis and headphone exposure events. See the `metrics` table in main.go
for exact names. Unknown types and other sections (including activity summaries)
remain in the original blob; unmapped Record types are counted. These are not
promised to cover every Apple Health export type.

Readings retain the original quantity/unit or category constant, start/end offset
and creation time separately. The adapter performs no clinical interpretation,
unit conversion, sleep-stage inference or source-priority aggregation. Category
values remain opaque text. Numbers must retain the same decimal value when serialized canonically (up to
256 source characters and exponent magnitude 1000); rounding away extra decimal
digits is rejected. Ordinary decimals such as 0.1 and trailing zeros remain
valid. Invalid numeric/time fields in selected readings fail
the entire conversion before any stdout; archive-only can preserve those source
anomalies without making reading claims. A nonzero source quantity that would
underflow to zero is rejected instead of creating a zero-valued reading.

The supplied XML has no sample UUID/native ID. Standard Bean tuple dedupe uses
type, interval, value/unit and occurrence. It preserves equal-reading multiplicity
within one import and suppresses repeats of an unchanged snapshot. It is
**best-effort across different exports**: identical readings from different
sources cannot be reliably distinguished, changed values become new claims, and
absence does not prove deletion. No automatic correction/purge is performed.
Do not mix these rows with future UUID-bearing HealthKit/API data as if identity
were proven. Raw archive snapshots remain separately recoverable even when
reading claims dedupe.

Names, birth dates, `sourceName`, device strings and arbitrary metadata are not
copied into normalized claims. They remain in the original encrypted archive.
Each reading records the original Apple type and its one-based Record position
in that archive, making source inspection possible without publishing those
identifiers in claim context. These are unsigned source assertions; Bean's owner
signature authenticates adoption, not Apple or device origin, accuracy or subject.

## Limits, failure and exit

Inputs must be regular files up to 256 MiB; symlinks and special files are rejected
without waiting for FIFO writers. Decompressed export.xml is limited to
256 MiB, ZIP declared contents to 512 MiB, members to 4096, Record elements to
2 million, XML depth to 32 and attributes per element to 64 KiB. ZIP member paths,
symlinks and ambiguous export.xml members are rejected. No files are extracted;
DTD entities are not fetched or expanded; this is not full DTD validation.
Repeated DOCTYPE and misplaced/repeated XML declarations are rejected. Complete XML EOF and the selected export.xml member CRC are
checked. Other ZIP members are preserved without interpreting their contents. Temporary snapshots/output are private and removed on ordinary exit;
process death can leave local temporary files. Optional reading JSONL is limited
to 1 GiB in its private spool, in addition to source snapshot space. An overflow
fails before stdout; use archive-only mode or select fewer types with `-types`.
`bean import` validates before any Bean writes, so this failure does not
partially import readings.

`bean import` appends the archive first, checks it is retained, then appends
optional readings and checks again. Each check proves every record file, because
a cached size/mtime match alone cannot establish that the archive is still on
disk, and reads the blob back. An invalid unrelated record does not block the
import; `bean verify` reports it. These steps are not a transaction against
concurrent writers or later external mutation.

A crash after blob storage can leave an encrypted orphan blob. Retry is safe.
Purged archive refs are not resurrected; `bean import` reports failure if append
cannot retain the archive, before adding readings. Re-storing a purged source
can still leave an encrypted orphan blob; `bean import` never deletes blobs. A multi-record append is not a transaction: a Bean
signing/storage failure may retain a prefix, and retry dedupes that prefix.

`bean export` recovers the original file bytes without this importer. Verify their
SHA-256 and length against the archive claim; the claim's authority is checked
with `bean verify`. Export is plaintext. Purging a reading does **not** remove its
content from a retained raw archive; original-file retention is a separate choice.

Run `go test ./cmd/bean` for synthetic encrypted adoption/retry/exit tests, and
`go test ./cmd/bean-import-apple-health` for parser, bounds, privacy and malformed
input tests. The real export is only inspected locally and never used in CI.

Failures from `bean import` name the step: validation means nothing was
written; storage can leave an encrypted orphan blob; archive or retention means
no reading was written; readings can mean a partial import. Retry is safe under
Bean dedupe, but cannot resurrect a deliberately purged archive. The same steps
keep a Labcorp report; parsing and source identity rules stay separate.
