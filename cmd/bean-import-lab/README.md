# Lab report importer

Reads lab report PDFs in the layouts it knows: the Labcorp patient report
(`labcorp.go`) and the Quest Diagnostics report (`quest.go`). Each layout
has a file of its own. Any other report is refused.

Supports the observed layouts, not arbitrary blood-test PDFs or image-only
scans. PDF input requires `pdftotext` (Poppler). For a Labcorp report,
equivalent `pdftotext -layout` text is accepted; for a Quest report,
equivalent `pdftotext -bbox-layout` output (`.html`). Tests contain only
synthetic reports.

Run it with `bean import lab [-tz <zone>] [-corrections] <report>`. bean
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

## Which layout

The importer reads the PDF with `pdftotext -layout` first. A page footer
line `Quest, Quest Diagnostics, the associated logo and all associated Quest
Diagnostics marks are the trademarks of Quest Diagnostics.` and a column
header `Test Name  In Range  Out Of Range  Reference Range  Lab` (or `Test
Name  Result  Reference Range  Lab`) make it a Quest report. Anything else
goes to the Labcorp grammar, which refuses what it does not know. A Quest
report given as `-layout` text is refused: give the PDF.

## How a Labcorp report is read

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
  health information` (with `protected by state and federal law` and a full
  stop, or without); `All Rights Reserved - Enterprise Report Version
  2.00`, optionally followed by `If you have received this document in error
  please call NNN-NNN-NNNN`; and, indented, `Date Created and Stored MM/DD/YY
  HHMM <zone> Final Report Page N of M`. Text that only contains `DOB:` or
  `Date Created` is not boilerplate.
- A field line: every token is a known field, `Label: value`, with one space
  after the colon: `Patient ID`, `Age`, `Account Number`, `Specimen ID`,
  `Sex`, `Ordering Physician`, `Date Collected` (`MM/DD/YYYY`, optionally
  `HHMM Local`), `Date Received`, `Date Reported` (`MM/DD/YYYY`, optionally
  `HHMM <zone>`), `Date Received` and `Date Entered` (optionally `HHMM Local`
  or `HHMM <zone>`), `Fasting` (`Yes`, `No` or `Not Given`), `Ordered Items`,
  `Control ID` and `Alternate Control Number` (which may be empty). The
  `Ordered Items` list may wrap onto the margin lines after it while each
  line ends in `;`; those lines are not read. Only fields
  are read for the specimen id, collection date and time, report date and
  fasting, and every one of them in the report must agree: two specimen ids,
  two collection days or times, two report days or two fasting answers are
  refused. A field at the margin or in the patient details that does not have
  its shape (`Date Collected: 01/02/20250`) is refused.

Before the first column header: a panel heading (one token at the margin)
or the header. An indented line there is refused: a row has no columns yet.

The column header is exactly five tokens, `Test`, `Current Result and Flag`,
`Previous Result and Date`, `Units`, `Reference Interval`, with `Test`
indented. Each header sets the columns of the rows under it: on a real
report they move by a few characters from page to page. A line that starts
with `Test` and is not that header is refused. Each column is a band. The
result band starts two columns left of its name, where values are printed
from; the other bands meet halfway between one name's end and the next
name's start, since Labcorp centers units and reference intervals under
their names. A token belongs to the band it lies in, and a token that runs
across where two bands meet is refused: its column is not sure. Columns are
counted in characters.

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
  the units band one unit; the reference band one token. A unit starts with
  a letter or `%`, holds a `/` or `%` or is a known bare unit (`fL`, `pg`),
  and no part of it is a result word (`Negative/Trace`); a number may be part
  of it (`mL/min/1.73`). A token in the Test band that is not a code, a unit
  this rule does not take (`9-20`, `>59`, `Yellow`, `<5/hpf`), or a result or
  flag outside the grammar below is refused.
- Every token of a row or value line must be in a field of the result read
  from it. The parser lays out the name, code, value, flag, previous result,
  units and range it kept, each at the column it read it from, and the line
  must come back. So a third token in the result band, or a second in the
  units or reference band, is in no field, and the report is refused.
- `Please note` or `Please Note:` with an optional lab code and nothing else:
  its text follows as comment lines. In the panel of the result above it, the
  text is that result's comment; otherwise it is a report note.
- A line that starts between the test column and the comment column, 40
  columns right of `Test`, is refused: a wrapped test name would sit there,
  and the reading above would be stored under half its name. Labcorp prints
  its comments 43 columns right of `Test` on every page.
- From the comment column on, under a row with nothing after its name and
  code, a line whose tokens all read whole as that row's values is its value
  line (a range alone is one too, for a test not performed). Any other line
  from the comment column on is a comment of the result above it, in any
  number of columns: the lab's notes, its table of risk categories, the text
  of a `Please note`. No row starts there, so no reading is lost to a
  comment. A row whose values do not read whole then has no value, which is
  refused unless the lab says the test was not performed.
- No comment starts in the units or reference column: a line that starts
  there and is not a row's values is refused, since a unit or range would be
  lost.
- A comment never reads as values: a line that reads as result columns but
  is not a waiting row's values is refused (the rest of a row on the line
  below it), and so is one whose columns after the first hold a value or a
  unit (a row whose name starts in the comment column).

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
Burlington, 1447 York Court, ...`); one token of text at the margin; the
Icon Legend line (`Out of Reference Range`, `Critical or Alert`, whose icons
do not come through); and
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

## How a Quest report is read

Since 0.5.0, by word position (`bbox.go`, `quest.go`, `questread.go`;
DECISIONS 235). In the `-layout` text of a Quest report the columns drift
from page to page and row to row. `pdftotext -bbox-layout` gives each word
and its box in points, which is exact. Words whose tops lie within 1 point
are one line, left to right. Two words printed over each other are refused.

Every line is one of these kinds, or the report is refused and the error
names the page and how far down the line is:

- Page boilerplate: `Report Status: Final` and the patient's name under it,
  at the top right; the footer `CLIENT SERVICES: <phone>  SPECIMEN: <id>
  PAGE n OF m` (the page part may be a line of its own); the trademark line.
  Every page must have its page number, `n` must be the page and `m` the
  page count, and every footer must name the report's specimen. Another
  report status (`Preliminary`) is refused.
- The block under `Patient Information  Specimen Information  Client
  Information`. Each word goes to the column whose heading it starts under.
  The specimen column holds fields only: `Specimen:`, `Requisition:`, `Lab
  Ref #:`, `Collected:`, `Received:`, `Reported:` (`MM/DD/YYYY / HH:MM
  <zone>`, time and zone optional). The patient column holds `DOB:`, `AGE:`,
  `Gender:`, `Fasting:` (`Y` or `N`), `Phone:`, `Patient ID:`, `Health ID:`
  and one line with the name. The client column is not read. Only the
  specimen, the three times and Fasting are read, and they must agree on
  every page. A word across two columns is refused.
- A column header: `Test Name  In Range  Out Of Range  Reference Range  Lab`,
  or `Test Name  Result  Reference Range  Lab`. Each sets the columns of the
  rows under it: a column starts at the x of its first header word, and a
  word belongs to the column whose start it is at or right of, give or take
  1 point. On the real report the columns sit at the same x on every page.
- A section title: one phrase in the patient column, directly after the
  block and directly before a column header (`Endocrinology`). It is the
  group of the rows under it.
- At the margin: `COMMENTS:` and its text (a report note); `Endnote n`, and
  the lines under it to the end of the page, its text; `Physician Comments:`
  and the lines under it (report notes); `PERFORMING SITE:` and its lab
  lines, `<code>  <name>, <address> Laboratory Director: <name>, CLIA:
  <number>`. The name is the text before the first comma. Any other line at
  the margin is refused.
- In a table, by where the line starts. Rows start at an indent: 28.9, 39.7
  and 50.6 points in the main table, 26.3 in the alternate one. Comments
  start from 56 points (30 in the alternate table) on, in any shape, and
  belong to the test above them, across pages. A line between is refused.

A row is read from the left: the name is the words one space apart from
the indent; every other word goes to its column. A word after a gap in the
name column, a word across two columns, or a name that runs into the value
columns while the row has values, is refused. The Lab column holds one lab
code. A comment, endnote or note line that starts left of the value columns
and holds a value after a gap in a value column is refused: it reads as a
row, and as text its reading would be lost.

Rows form a tree by indent. A row with a value is a test. A row without
one is a group heading, and the rows indented under it are its group,
except:

- A row with no value, no code and nothing under it, one indent deeper and
  directly after a heading, is the rest of the heading's name
  (`COMPREHENSIVE METABOLIC` and `PANEL`).
- A heading with one row under it, a test with nothing under it, is that
  test with its name wrapped (`SEX HORMONE BINDING` and `GLOBULIN`). So a
  real group of one test is read as one test with the two names joined.
  The review shows it.

A test takes its group's lab code unless it prints its own. A test with
rows under it, a row with a range and no value, a heading with a comment
under it, and a row with no value and nothing under it are refused. A row
that is one phrase, with no column, directly under a comment and with no
row under it, is more of that comment: a long note may start at a row
indent (it does on the real report), and a phrase holds no value.

Values: In Range holds a value, or `SEE NOTE:`; Out Of Range a value and a
flag (`H`, `L`, `HH`, `LL`, `A`); Result a value and an optional flag. A
value is what the Labcorp grammar takes (below). A value in two columns, a
flag in In Range, or a value in Out Of Range with no flag is refused.
`SEE NOTE:` is Quest's way to say the value is in the note: the test is
kept with no value and the note as its `lab_comment`; with no note under
it, it is refused. The printed digits are kept as `raw_value`.

Reference Range: a range (`50-195`, `0.40-4.50`, `<150`, `>280`, `> OR =
40`, `< or = 12.9`), then a unit, then `(calc)`, each optional. A result
word alone (`NEGATIVE`) is a range too. The unit's first word is a unit as
the Labcorp grammar takes it; up to three words of letters may follow (`% of
total Hgb`, `ng/mL RBC`). Anything else is refused. `(calc)` gives
`context.calculated: true`. A test with no range whose first comment is
`Reference Range <range>` or `Reference range: <range>` takes that range.

Identity: the native id is the specimen, the groups and the test name, so
the same test in two panels with two readings (Albumin) is two readings of
one type. The same reading printed twice (same name, value, flag, unit,
range, lab, comment) is one. Two names for one type, or one test twice in
one group with two readings, are refused. A comment that cites `See
Endnote n` gets the endnote's text; a cited endnote the report does not
print is refused, and one no comment cites is a report note.

Time: `Collected:` prints a zone. `EST`, `EDT`, `CST`, `CDT`, `MST`, `MDT`,
`PST`, `PDT`, `AKST`, `AKDT` and `HST` give the minute at their fixed
offset. Any other zone gives the day, and `raw_time` keeps the printed
time. `-tz` is not needed; a `-tz` whose offset at that instant differs
from the printed zone's is refused.

The positions come from a real report: `testdata/quest-layout.html` keeps
every word where it was, with every name, id, date, address, value, flag
and comment replaced, the values digit for digit, the flags on other rows.
Tests read it directly. There is no synthetic Quest PDF; the PDF path is
checked against the real report only.

## Privacy, limits and failure

Recognized patient-name/birth-date/ID headers are excluded. Free-text medical
comments can contain identifiers and are deliberately not silently rewritten.
The original archive retains all original content. Routine status messages omit
collection dates, source text and external converter diagnostics. Owner adoption
signatures do not prove Labcorp/device origin or measurement accuracy.

Inputs and extracted text are each limited to 32 MiB. PDF extraction runs against
a private copy of the input, has a 30-second timeout, a one-second pipe-close wait bound and bounded output.
Extracted reports allow at most 100,000 lines and 10,000 result rows; word
positions at most 1,000 pages and 1,000,000 words. Reports
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

Run `go test ./cmd/bean-import-lab ./cmd/bean`. Tests exercise mixed
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

A result with a value under a "Test not performed" line is refused.

When a note of the report, or a comment under a result, says the date or
time of collection was not indicated on the requisition, the time the report
prints may be the receipt time. Then only the collection day is recorded, and
the printed time is kept as `raw_time`. If the collection and receipt days
are also the same, the day itself may be the receipt day, and each reading's
`context.collection_day` says so.

The shapes come from a real report: `testdata/real-layout.txt` is its
layout, column for column, with every name, id, date, value, flag and
comment replaced (DECISIONS 230). Another report may still hold a line the
grammar does not know; it is refused by line number.
