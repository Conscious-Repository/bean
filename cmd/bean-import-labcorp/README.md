# Labcorp report importer

Supports the observed Labcorp patient-report layout, not arbitrary blood-test
PDFs or image-only scans. PDF input requires `pdftotext` (Poppler); equivalent
`pdftotext -layout` text is accepted. Tests contain only synthetic reports.

Use `tools/labcorp/ingest.py source.pdf /private/bean-log --timezone America/Chicago`
with BEAN_BIN and LABCORP_BIN pointing to built commands. The log must already
exist outside the repository. The helper privately snapshots input, validates
before Bean writes, stores the exact source as an encrypted blob, checks plaintext
readback and confirms that an archive claim about the owner remains present. It shares these
byte-preservation steps with the Apple Health helper; parsers remain independent.

With `-blob`, the standalone importer emits a `lab.report` archive first, then
individual `lab.<test-name>` results. The archive's plaintext SHA-256 and length
cover the exact PDF or text input. The caller-supplied blob reference is trusted
by the standalone converter; use the helper to verify storage correspondence.
Without `-blob`, the converter remains an extraction-only compatibility path and
warns that the source is not preserved.

## Identity and amended reports

Readings retain existing specimen/test/occurrence native identity. Re-importing
an unchanged report skips duplicates. An amended source gets a separate archive,
so its original bytes survive export even when readings dedupe against earlier
values. **This does not apply amendments.** Review the preserved reports and use
explicit Bean corrections to change prior claims. The CLI and helper state this
policy; they cannot infer that a duplicate skip was an identical result.

A missing result remains null; categorical or out-of-range numbers remain text.
Original units, reference ranges, flags and comments are preserved. No LOINC
normalization or clinical interpretation is performed. A timezone option is the
collection site's timezone; without it the observation has day precision. A
nonexistent or repeated local minute during a clock change also retains day
precision and the printed time, rather than choosing an unsupported instant.

Conflicting specimen IDs or collection dates/times are rejected instead of mixing
reports. Repeated headers from the same report remain valid, including wrapped
values across pages. Failed column parsing cannot partially change a result.
Short text in the comment column stays a comment; long aligned categorical
results and reference ranges are preserved. Ambiguous populated result columns
fail validation instead of becoming missing values. Column separation follows
the header spacing, including compressed Poppler output. Nonzero numbers that
underflow or change decimal value when converted to the record number
representation remain text. Numeric tokens longer than 256 characters also
remain text. Reordered table headers, including repeated headers, and malformed
collection-date labels fail explicitly. Specimen IDs containing a slash are
rejected to keep the existing specimen/test identity unambiguous. Unsupported report
layouts can still need a new fixture and adapter update.

## Privacy, limits and failure

Recognized patient-name/birth-date/ID headers are excluded. Free-text medical
comments can contain identifiers and are deliberately not silently rewritten.
The original archive retains all original content. Routine status messages omit
collection dates, source text and external converter diagnostics. Owner adoption
signatures do not prove Labcorp/device origin or measurement accuracy.

Inputs and extracted text are each limited to 32 MiB. PDF extraction runs against
a private copy of the input, has a 30-second timeout, a one-second pipe-close wait bound and bounded output.
Extracted reports allow at most 100,000 lines and 10,000 result rows. Reports
are staged before stdout; an invalid result or oversize output emits no prefix.
Each partial record reserves 4 KiB within the 1 MiB record limit for Bean to
add its identity, signature, import time and dedupe fields.
A failed stdout write exits nonzero; failures during delivery may leave a prefix.
The helper stages JSONL before append, but Bean append is not a transaction:
append failure can retain a prefix, and retry dedupes it.

The helper verifies actual log bytes before source writes and confirms archive
retention before appending readings. It verifies again after readings. Full-log
verification adds cost and refuses ingestion if unrelated records or blobs are
invalid. A concurrent writer or later external mutation can still change state.

Helper errors distinguish validation (no Bean writes), existing-log verification
(no source blobs or claims written), storage (possible encrypted orphan blob),
and append/retention (possible partial ingestion). A deliberately purged archive
is not resurrected, and its readings are not newly appended. Re-storing that
source before detecting purge can leave an encrypted orphan blob; the helper
does not delete shared blobs on its own. A temporary snapshot can survive process death.

Export via `bean export` yields the original plaintext bytes. Verify their digest
against `lab.report`; verify owner authority separately with `bean verify`.
Purging one reading does not erase its content from a retained report archive.

Run `go test ./cmd/bean-import-labcorp` and `make labcorp`. Tests exercise mixed
metadata, page breaks, bounded/private extraction, output failures, storage,
repeat imports, amendments and byte-identical export of both report versions.
