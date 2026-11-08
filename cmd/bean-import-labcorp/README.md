# Labcorp report importer

Supports the observed Labcorp patient-report layout, not arbitrary blood-test
PDFs or image-only scans. PDF input requires `pdftotext` (Poppler); equivalent
`pdftotext -layout` text is accepted. Tests contain only synthetic reports.

Run it with `bean import labcorp [-tz <zone>] [-corrections] <report>`. bean
copies the input privately, works out the blob id, runs this importer, checks
that its first record keeps this file and was not purged, then stores the copy
as an encrypted blob, reads it back, adds the `lab.report` record, and checks
that the log keeps it before it adds a reading. See `bean help` and
docs/COMMANDS.md.

With `-blob`, the standalone importer emits a `lab.report` archive first, then
individual `lab.<test-name>` results. The archive's plaintext SHA-256 and length
cover the exact PDF or text input. Only the archive lists the blob. Each result
names the report by `context.report_sha256`, so purging the `lab.report` erases
the file. Results from 0.1.x list the blob too; the file stays until they are
purged as well. The caller-supplied blob reference is trusted
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

- Two results that map to one `lab.<test>` type. The same test listed twice,
  as when two panels share it, is one result only when its value, flag, unit,
  range, previous result, lab code and comment all match.
- A test name that ends within 4 columns of the header's "Current Result":
  the value, a flag or a categorical result may have run into the name. Where
  the name ends decides, so a comment under the row changes nothing.
- An indented line before the first column header: a row there has no
  columns to be read by.
- A two-digit number at the end of a name is a lab code only when the
  report's Performing Labs list names it. Without a list, or with a list line
  it cannot read, only a two-digit token in the code column is a lab code,
  and only when a value follows it on the row. With nothing after it, it may
  be the value of a compressed row (`eGFR  90`): refused. `Interleukin 10`
  keeps its number.
- A line with a byte outside printable ASCII (0x20 to 0x7E): a tab, a
  Unicode space, an invisible mark, an accented letter, a lone CR. Columns
  are read from spaces, one byte each; any other character can hide where a
  column starts or shift it. Three things are allowed: the form feed that
  `pdftotext` puts at the start of each page (it has no width), CRLF line
  ends, and any character in a page header line (the copyright line with ©
  and ®, the patient's name) or in the patient details after the end of the
  results. Nothing in those is read as a result. The specimen id, fasting
  field and lab names read from them must still be ASCII. So a comment with
  `®`, `µ` or `≥` is refused.
- A result with no value, unless the first line under it starts with `Test
  not performed`, `Not performed`, `Test cancelled`, `Test canceled`,
  `Cancelled` or `Canceled`. Even then, it is refused when a word of its name
  after the first may be a value: it starts with a digit, `<`, `>`, `=` or
  `+`, or with `-` and a digit; it holds `/`, or `:` between digits; it is a
  flag (`H`, `L`, `A`, `High`, `Low`, `Abnormal`, `Critical`, `Panic`) or a
  result word (`Negative`, `Trace`, `Clear`, `Immune`, `TNP` and a few more).
  The value may sit in the name, one space after it or under a header whose
  columns are further right. A lab code the list names was taken off the name
  before this check. `Vitamin A` and `Interleukin 10` with no value are
  refused too.
- After `Disclaimer`, `Icon Legend` or `Performing Labs`, where the results
  end: a column header; an indented line (1 to 19 spaces) of two or more
  columns that is not a lab list line (`01: ...`) or a page header line; an
  indented line of one column with a word that may be a value; and a line
  indented 20 or more under such an indented line. A result there would be
  lost. A lab address wrapped onto an indented line is refused too.

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
encrypted copy may remain, no record), archive (an encrypted copy may remain, no
reading), retention (no reading), or
readings (some may be written). A purged archive is checked before the file is
stored, and so are the checks Append makes of the archive record: if either
fails, nothing is written, not even the encrypted copy. A concurrent writer can
still change the log between checks.

Export via `bean export` yields the original plaintext bytes. Verify their digest
against `lab.report`; verify owner authority separately with `bean verify`.
Purging one reading does not erase its content from a retained report archive.
Purging the `lab.report` erases the file, unless another record still names it.

Run `go test ./cmd/bean-import-labcorp ./cmd/bean`. Tests exercise mixed
metadata, page breaks, bounded/private extraction, output failures, storage,
repeat imports, amendments and byte-identical export of both report versions.
