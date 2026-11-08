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

Nonzero numbers that underflow or change decimal value when converted to the
record number representation remain text, as do numeric tokens longer than 256
characters. Specimen IDs containing a slash are rejected to keep the
specimen/test identity unambiguous. Unsupported report layouts need a new
fixture and an adapter update.

## How a report is read

Since 0.3.0 the report is read by a grammar (docs/DECISIONS.md 222, 224). Every
line of the extracted text must be one of the kinds below, or the report is
refused, and the error names the line. Nothing is skipped because it looks
unimportant. A line is split into tokens at runs of two or more spaces; a
token's column is where it starts.

Lines that may appear anywhere:

- A blank line.
- A page header or footer line, matched whole: `<name>  DOB: MM/DD/YYYY
  Patient Report`; `©YYYY Laboratory Corporation of America® Holdings`,
  optionally followed by `This document contains private and confidential
  health information.`; `All Rights Reserved - Enterprise Report Version
  2.00`, optionally followed by `If you have received this document in error
  please call NNN-NNN-NNNN`; and, indented, `Date Created and Stored MM/DD/YY
  HHMM <zone> Final Report Page N of M`. Text that only contains `DOB:` or
  `Date Created` is not boilerplate.
- A field line: every token is a known field, `Label: value`, with one space
  after the colon: `Patient ID`, `Age`, `Account Number`, `Specimen ID`,
  `Sex`, `Ordering Physician`, `Date Collected` (`MM/DD/YYYY`, optionally
  `HHMM Local`), `Date Received`, `Date Reported` (`MM/DD/YYYY`, optionally
  `HHMM <zone>`), `Fasting` (`Yes` or `No`) and `Ordered Items`. Only fields
  are read for the specimen id, collection date and time, report date and
  fasting, and every one of them in the report must agree: two specimen ids,
  two collection days or times, two report days or two fasting answers are
  refused. A field at the margin or in the patient details that does not have
  its shape (`Date Collected: 01/02/20250`) is refused.

Before the first column header: a panel heading (one token at the margin)
or the header. An indented line there is refused: a row has no columns yet.

The column header is exactly five tokens, `Test`, `Current Result and Flag`,
`Previous Result and Date`, `Units`, `Reference Interval`, with `Test`
indented. Every header in the report must give the same columns, so a page
laid out further right or compressed differently is refused. A line that
starts with `Test` and is not that header is refused. Each column is a band
that starts two columns left of its header name and ends where the next one
starts. Columns are counted in characters.

In the results:

- A panel heading: one token at the margin that ends left of the result
  column. `<panel> (Cont.)` continues the panel. Any other line at the margin
  is refused.
- A result row: the name starts at `Test` (give or take one column) and ends
  at least 4 columns before `Current Result`, or the report is refused: the
  value may have run into the name. A name that ends in `:` (`Note:`) is a
  label, and is refused. A second token in the Test band must be a two-digit
  lab code. Every other token goes to the band its start is in: the result
  band holds a value and an optional flag, or a flag alone, which leaves the
  row with no value; the previous band a value, an optional flag and a date;
  the units band one word with a letter, `%` or `/`; the reference band one
  token. A token in the Test band that is not a code, a units token with a
  space or without a letter, `%` or `/` (`9-20`, `>59`), or a result or flag
  outside the grammar below is refused. A token that starts within two
  columns of where a band starts, on either side, is refused: a reference
  interval one column left of its band would be read as the unit, a previous
  result two columns left of its band as the current flag.
- Every token of a row or value line must be in a field of the result read
  from it. The parser lays out the name, code, value, flag, previous result,
  units and range it kept, each at the column it read it from, and the line
  must come back. So a third token in the result band, or a second in the
  units or reference band, is in no field, and the report is refused.
- `Please note` or `Please Note:` with an optional lab code and nothing else:
  its text follows as comment lines. In the panel of the result above it, the
  text is that result's comment; otherwise it is a report note.
- A value line: only under a row with nothing after its name and code, with
  only blank, page header, field, column header or `(Cont.)` lines between.
  All its tokens are in the result, previous, units and reference bands, read
  as on a row. A value line anywhere else (units under a row that has a
  value, a value under a `Please note`, a value before its name) is refused.
- A comment line: it starts right of where names start and left of the
  result band, under a result of the same panel or a `Please note`, and is
  one token of text, or `Note:` and one more token. Anything with more
  columns is refused.

A value is a number with an optional `<`, `>`, `<=` or `>=` (`101`, `-2.0`,
`<0.5`); a dipstick grade `1+` to `4+`; a count range such as `0-5`; a titer
such as `1:40`; or one of `Negative`, `Positive`, `Reactive`, `Non Reactive`,
`Nonreactive`, `Non-Reactive`, `Detected`, `Not Detected`, `Normal`,
`Borderline`, `Equivocal`, `Indeterminate`, `Inconclusive`, `Pending`,
`Immune`, `Trace`, `Clear`, `Cloudy`, `Hazy`, `Turbid`, `Present`, `Absent`,
`None`, `TNP`, `QNS`, in any letter case. A flag is `H`, `L`, `A`, `High`,
`Low`, `Abnormal`, `Critical`, `Panic` or `Alert`. A flag word is never a
value: `Abnormal` alone in the result band is a flag, the row has no value,
and a flag with no value is refused, since the value may sit in the name. So
a report with `Abnormal` as a categorical result is refused. Anything else in
the result band is refused, so a free-text result (`No clinically
significant antibodies detected`) or a number with a thousands comma
(`1,234`) refuses the report. The lists can grow from real reports.

After `Disclaimer`, `Icon Legend` or `Performing Labs`, each a line of its
own at the margin, the results end. Then only these pass: page header and
field lines; another end heading; a lab list line (`01: BN - Labcorp
Burlington, 1447 York Court, ...`); one token of text at the margin; and
`Patient Details` (optionally with `Physician Details` and `Specimen Details`
on the same line) and the lines under it. A line of the patient details has
at most one token per heading column, each starting within two columns of its
heading. The patient's and physician's columns may hold any text a name can,
accents too; the specimen column holds fields only. A column header, an
indented line, or a line of two tokens at the margin is refused: a result
there would be lost. A lab line that starts `NN: ` but does not have that
shape (wrapped across two lines) is not refused, but then lab codes come from
the code column only (below).

Characters: a line that is read is printable ASCII (0x20 to 0x7E). A tab, a
Unicode space, an invisible mark, an accented letter or a lone CR is refused,
because it can hide where a column starts or shift it. The form feed that
`pdftotext` puts at the start of each page and CRLF line ends are allowed.
Only these may hold more: the patient's name in its header line, the
copyright line (© and ®), the ordering physician's name, and the patient's
and physician's columns of the patient details; even there a control
character, a space other than ' ' or an invisible format character is
refused. So a comment with `®`, `µ` or `≥` is refused. A line longer than
16 KiB is refused.

Then:

- Lab codes. With a Performing Labs list that reads whole, a code is one the
  list names: a code in the code column it does not name is refused, and two
  digits at the end of a name are taken off as the code only when the list
  names them, so `Interleukin 10` keeps its number. A numbered test (below)
  with no code in the code column whose number the list names is refused:
  `Interleukin 10` with lab 10 listed may be test 10 or lab 10. Without a
  list, or with a lab line it cannot read, any token in the code column is
  refused: it may be the value of a compressed row (`Bacteria  10
  Abnormal`). Such a report reads only when no row has a code.
- Two results that map to one `lab.<test>` type are refused. The same test
  listed twice, as when two panels share it, is one result only when its
  value, flag, unit, range, previous result, lab code and comment all match.
  A test whose type would be `lab.report`, the kept report's type, is
  refused.
- A result with no value is taken only when the first comment line under it
  starts with `Test not performed`, `Not performed`, `Test cancelled`, `Test
  canceled`, `Cancelled` or `Canceled`, and every word of its name after the
  first is letters and hyphens and not a value or flag. So `Glucose 101`,
  `Glucose (H)`, `Lipoprotein (a)`, `Vitamin A` and `Appearance Clear` with no
  value are refused. A name word of letters is taken as part of the name:
  `Nitrite Neg` with no value and `Test not performed` under it is read as a
  test of that name that was not performed.

Not settled without a real report: the field, boilerplate and disclaimer
shapes come from the synthetic fixture. Test with your own report before you
rely on it; a refusal names the line, so the grammar can grow.

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

A word of a test name after the first may not look like a reading: a value
(a number, `3+`, `0-5`, `1:40`, `Negative`, `Trace` and the other results
above), a spelled-out flag (High, Low, Abnormal, Critical, Panic, Alert), a
number with a comma or a range of decimals, a word that ends in `+`, has `:`
between digits or starts with `-` and a digit. Such a name is refused: the
value, and its flag, may sit in the name, one space after it (`Glucose 101
H`). The one exception is a numbered test of two words: `Interleukin` or
`Factor` and one or two digits, `CA 125`, `CA 15-3`, `CA 19-9`, `CA 27.29`,
`CA 72-4`, and `Cyfra 21-1`. So `Beta 2 Microglobulin` and `Factor 101` are
refused. Add a family or a number here when a real report needs it.
