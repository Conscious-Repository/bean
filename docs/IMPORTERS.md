# Importers

An importer turns one kind of file into bean records. It is a separate program, named `bean-import-<name>`, in any language. `bean import <name> [flags] <file>` finds it next to `bean` or on your `PATH` and runs it. bean does the rest: it keeps the file, checks what the importer wrote, shows it to you, and signs only what you confirm.

bean does not need to know your source. Anyone can write an importer for theirs, and anyone can check what it writes.

## Built in

| Command | Reads | Notes |
|---|---|---|
| `bean import lab` | Lab report PDFs, in the layouts it knows: the Labcorp and Quest Diagnostics patient reports | [README](../cmd/bean-import-lab/README.md) |
| `bean import apple-health` | An Apple Health `export.zip` or `export.xml` | [README](../cmd/bean-import-apple-health/README.md) |
| `bean import csv` | A CSV with a `value` column | [COMMANDS](COMMANDS.md) |
| `bean import nhanes` | A public NHANES data file (`.XPT`, 2017-2018), kept as reference data for `bean compare`. No readings. | [COMPARE](COMPARE.md), [notes](../cmd/bean-import-nhanes/README.md) |

## Community importers

| Command | Reads | Repository | Maintainer |
|---|---|---|---|
| (none yet) | | | |

To add yours, open a pull request that adds a row, with a link to its source and a synthetic sample file. A row here is not an endorsement. Read an importer before you run it: it reads your file. bean checks what it writes, not what it does.

## The contract

`bean import <name> [flags] <file>` runs:

```
bean-import-<name> -blob <id> [flags] <private copy of file>
```

- `-blob <id>` is the id the file will have once stored. The importer must accept it.
- Its other flags are its own. bean passes them through, except `-yes` and `-corrections`, which are bean's.
- The file is a private copy. `TMPDIR` is a private folder that bean removes afterwards.
- No network, no keys, no log. The importer never signs and never reads the log.
- Readings about you, and nothing else. bean signs the importer's output with your key, so `bean import` holds each line to this. A line is of no type that only you or bean write: none that starts with `bean.`, and not `agent.pair`, `share` or `reply`. It names no `subject` but you. It has no `refs` and no `source.supersedes_ref`: it deletes no record, and corrects none by id or by ref. A changed value of a reading that you have is held back, and you pass it with `-corrections`. A line that breaks this stops the import. An importer runs as you on your machine, so run only one that you trust. This rule limits a mistake, and what a script passes with `-yes`.

It writes JSON lines to stdout ([SPEC.md](../SPEC.md) 9.2, partial records), then exits 0.

**Line 1 keeps the file:**

```json
{"type": "<name>.report", "value": {"blob": "<id>"}, "times": {"observed": null},
 "source": {"system": "<source>", "adapter": "bean-import-<name>@<version>",
            "ref_input": ["bean/v0/ref", "tuple", "<source>", "<name>.report", null, {"blob": "<id>"}, 0]},
 "context": {"sha256": "<hex of the file>", "bytes": <size>},
 "blobs": [{"hash": "<id>", "role": "raw-export", "media_type": "<type>"}]}
```

`value.blob` and `blobs[].hash` must be the `-blob` id. `source.ref_input` is required; a ready-made `source.ref` or a `source.supersedes_ref` is refused.

**Every other line is a reading:**

```json
{"type": "lab.glucose", "value": {"n": 95, "unit": "mg/dL"},
 "times": {"observed": {"start": "2026-11-03T08:53:00-06:00", "precision": "minute"}},
 "source": {"system": "<source>", "adapter": "bean-import-<name>@<version>",
            "ref_input": ["bean/v0/ref", "id", "<source>", "<the source's own id for this reading>", 0]},
 "context": {"test": "GLUCOSE", "report_sha256": "<hex of the file>"}}
```

- A reading names the file by its SHA-256 in `context`, not in `blobs`, so that purging the file's record erases the file.
- `source.ref_input` makes the import repeatable: the same file again adds nothing. Use the source's own id when it has one (`"id"`), else the reading itself (`"tuple"`).
- Types come from [cmd/bean/vocabulary.md](../cmd/bean/vocabulary.md), or are the source's own, lowercased with `-` for spaces (`lab.bun-creatinine-ratio`).

## Rules

1. **Refuse a reading that you cannot read for sure, never the file.** A wrong value in a health record is worse than none, so write no reading that you are not sure of. The file is still the person's. With `-blob`, write the first line, which keeps the file, and exit 0 with one line on stderr that says why nothing was read. Without `-blob`, exit non-zero and write nothing.
2. **All the readings or none.** Stage your output; never write half of it. A report whose readings cannot all be read for sure gives the first line alone.
3. **Keep the source's words.** The value as printed (`raw_value`), the test name as printed (`test`), the flag as printed. No interpretation, no unit conversion.
4. **No identity in readings.** Names, birth dates, patient and account ids stay in the kept file, not in readings.
5. **Bounded.** Limit input size, lines, records and output.
6. **Deterministic.** The same file gives the same output.
7. **Synthetic test data only.** Never commit a real report. Make the sample by replacing every value, name and id of a real file and keeping its layout.

## Test yours

```
bean-import-<name> -blob sha256:$(printf '0%.0s' {1..64}) sample.file | head
bean -dir /tmp/try init
bean -dir /tmp/try import <name> sample.file
bean -dir /tmp/try verify
```

Import the same file twice: the second run adds nothing.
