# Apple Health file ingestion

This adapter reads a real Apple Health `export.xml` or `export.zip` layout. It
requires no bridge, Apple account login, phone app, network or new dependency.
Only synthetic fixtures are committed. Bean core is unchanged.

## Default: preserve the raw export

Build the two commands:

```
go build -o /tmp/bean ./cmd/bean
go build -o /tmp/bean-import-apple-health ./cmd/bean-import-apple-health
```

Use an existing private Bean log outside this repository:

```
BEAN_BIN=/tmp/bean APPLE_HEALTH_BIN=/tmp/bean-import-apple-health \
  python3 tools/apple-health/ingest.py /absolute/path/export.zip /absolute/path/bean-log
```

The helper snapshots the original file privately, validates it before writing
anything to Bean, stores the exact input as an encrypted blob, checks plaintext
readback, then appends one `apple-health.export` claim. Its value references that
blob; context records original-file SHA-256, byte length, per-type record counts,
activity-summary count and `assurance=unsigned-export`. It checks that the archive
claim survived append/dedupe. Identical bytes retry as `added 0, skipped 1`.
A changed export is a separate snapshot, not a replacement or extra measurement.
No numerical values are summed. Archive entries cannot be mistaken for steps.

The helper does not initialize a log, handle keys or invent an identity. Use
`bean -dir /private/new/log init` if you need a new log, and keep the recovery
words offline. Raw exports and exported Bean blobs contain personal data.

The standalone command follows other importers' JSONL convention:

```
/tmp/bean-import-apple-health -blob sha256:REPLACE_WITH_BLOB_ID export.zip > claims.jsonl
```

Here `-blob` is a caller-provided reference to the **exact input** stored with
`bean blob put`. The standalone converter cannot authenticate that relationship:
Bean blob IDs hash ciphertext, not plaintext. The helper above enforces readback.
Do not pipe converter output into append unless the converter exited successfully.
The converter stages its output until the complete XML/ZIP CRC passes validation,
but an output-device failure can still interrupt stdout delivery.

## Optional individual readings

```
BEAN_BIN=/tmp/bean APPLE_HEALTH_BIN=/tmp/bean-import-apple-health \
  python3 tools/apple-health/ingest.py export.zip /private/bean-log \
  --records --types apple-health.steps,apple-health.distance-walking-running
```

Omit `--types` to emit all supported readings. The archive claim is still included.
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
values remain opaque text. Invalid numeric/time fields in selected readings fail
the entire conversion before any stdout; archive-only can preserve those source
anomalies without making reading claims.

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

Inputs must be regular files up to 256 MiB; decompressed export.xml is limited to
256 MiB, ZIP declared contents to 512 MiB, members to 4096, Record elements to
2 million, XML depth to 32 and attributes per element to 64 KiB. ZIP member paths,
symlinks and ambiguous export.xml members are rejected. No files are extracted;
DTD entities are not fetched or expanded. Complete XML EOF and the selected export.xml member CRC are
checked. Other ZIP members are preserved without interpreting their contents. Temporary snapshots/output are private and removed on ordinary exit;
process death can leave local temporary files. Optional readings can require
substantial temporary disk space.

A crash after blob storage can leave an encrypted orphan blob. Retry is safe.
Purged archive refs are not resurrected; the helper reports failure if append
cannot retain the archive. A multi-record append is not a transaction: a Bean
signing/storage failure may retain a prefix, and retry dedupes that prefix.

`bean export` recovers the original file bytes without this importer. Verify their
SHA-256 and length against the archive claim; the claim's authority is checked
with `bean verify`. Export is plaintext. Purging a reading does **not** remove its
content from a retained raw archive; original-file retention is a separate choice.

Run `make apple-health` for synthetic encrypted adoption/retry/exit tests, and
`go test ./cmd/bean-import-apple-health` for parser, bounds, privacy and malformed
input tests. The real export is only inspected locally and never used in CI.

Failures from the helper identify the phase without echoing source data.
Validation failure means no Bean writes were attempted; storage failure can leave
encrypted orphan blobs; append/retention failure can mean a partial import. Retry
is safe under Bean dedupe, but cannot resurrect a deliberately purged archive.
The byte-preservation steps are shared with the Labcorp helper in
`tools/source_ingest.py`; parsing and source identity rules stay separate.
