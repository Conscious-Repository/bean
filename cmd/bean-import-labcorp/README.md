# Labcorp report importer

Supports the observed Labcorp patient-report layout, not arbitrary blood-test
PDFs or image-only scans. PDF input requires `pdftotext` (Poppler); equivalent
`pdftotext -layout` text is accepted. Tests contain only synthetic reports.

Run it with `bean import labcorp [-tz <zone>] [-corrections] <report>`. bean
copies the input privately, runs this importer to validate it before any write,
stores the copy as an encrypted blob, reads it back, adds the `lab.report`
record, and checks that the log keeps it before it adds a reading. See
`bean help` and docs/COMMANDS.md.

With `-blob`, the standalone importer emits a `lab.report` archive first, then
individual `lab.<test-name>` results. The archive's plaintext SHA-256 and length
cover the exact PDF or text input. The caller-supplied blob reference is trusted
by the standalone converter; `bean import` checks that it names the stored copy.
Without `-blob`, the converter remains an extraction-only compatibility path and
warns that the source is not preserved.

## Identity and amended reports

Readings retain existing specimen/test/occurrence native identity. Re-importing
an unchanged report skips duplicates. An amended source gets a separate archive,
so its original bytes survive export even when readings dedupe against earlier
values. **This does not apply amendments.** Review the preserved reports and use
explicit Bean corrections to change prior claims. bean states this
policy. A changed reading is not written: `bean import` exits 1 and names it,
and `-corrections` writes it as a correction.

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

A report that cannot be read for sure is refused, never guessed:

- Two results that map to one `lab.<test>` type. The same test listed twice
  with the same reading, as when two panels share it, is one result.
- A result with no value and no comment whose name ends in a number: the
  value may have run into the name.
- A two-digit number at the end of a name, or in the code column, is a lab
  code only when the report's Performing Labs list names it, or when the
  report has no such list. `Interleukin 10` keeps its number.

Not settled without a real report: a note line in the result column can still
read as a result. Test with your own report before you rely on it.

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
bean appends the archive, checks retention, appends the readings and checks
again. This is not a transaction: a failure while readings are written can
leave some of them, and the same command again adds only what is missing.
Errors say which step failed: validation (nothing written), storage (an
encrypted copy may remain, no record), archive or retention (no reading), or
readings (some may be written). A purged archive is not brought back, and its
readings are not added; the encrypted copy stored before that check stays,
named by no record. A concurrent writer can still change the log between checks.

Export via `bean export` yields the original plaintext bytes. Verify their digest
against `lab.report`; verify owner authority separately with `bean verify`.
Purging one reading does not erase its content from a retained report archive.

Run `go test ./cmd/bean-import-labcorp ./cmd/bean`. Tests exercise mixed
metadata, page breaks, bounded/private extraction, output failures, storage,
repeat imports, amendments and byte-identical export of both report versions.
