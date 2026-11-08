# Command reference

Every command and flag of `bean` and the importers. For a first run, read the [README](../README.md) first.

## Conventions

```
bean [-dir <folder>] <command> [arguments]
```

- `-dir` is the log folder. If you do not give it, `bean` uses `$BEAN_DIR`, then the current folder.
- Flags can come before or after the arguments of `add` and `ls`. For other commands, put flags first.
- An **id** is `sha256:` and 64 hex digits. Commands that take an id also take its first 6 or more hex digits, if no other record starts with them.
- A **time** is RFC 3339 (`2026-10-28T13:00:00-05:00`), `YYYY-MM-DD HH:MM`, or `YYYY-MM-DD`. The last two use your local time zone.
- Output goes to stdout. Errors go to stderr and start with `bean:`.

| Exit code | Meaning |
|---|---|
| 0 | Success. For `verify`: no errors. |
| 1 | The command failed, or `verify` found errors. |
| 2 | Usage error: unknown command, bad flag or bad argument, for example a time that `ls -since`, `ls -until` or `add -at` cannot read. |
| 3 | Your identity has a record of a newer format version than this `bean` knows. Reads stop, and `export` writes every file first. Update `bean`, or if you did not write that record, run `bean rotate`. Another identity that is too new only gets its `too new` line. |
| 4 | Your identity is frozen: two rotations were made from the same point. Reads still print, and warn you. Writes stop. Run `bean migrate`. |
| 128 + n | `import` was stopped by signal n and removed its private copy of the file: 129 for SIGHUP, 130 for SIGINT (Ctrl-C), 131 for SIGQUIT, 143 for SIGTERM (`kill`). |

| Environment variable | Meaning |
|---|---|
| `BEAN_DIR` | The log folder, if `-dir` is not given. |
| `BEAN_NOW` | Overrides the clock (RFC 3339). For tests only. |

## Start

### `init [-nostr new|-|<file>] [-force]`

Creates your identity and an empty log in the folder. It refuses inside a git work tree, because a commit could publish your keys; `-force` overrides. Prints `identity <id>`, then `npub <npub>` if you chose a Nostr key, then the 24 words.

- No flag: the signing key is a new random Ed25519 key.
- `-nostr new`: the signing key is a new Nostr (secp256k1) key, so the log has its own npub.
- `-nostr <file>`: use the Nostr secret key in the file (`nsec1...` or 64 hex digits).
- `-nostr -`: type or paste the secret key. The terminal does not show it.

Write the 24 words on paper. `bean` shows them only once. They are the only way to recover your identity on a new machine. Do not type them into a crypto wallet.

### `whoami`

Prints your identity id and your signing key. It also prints the npub of a Nostr key, and the agent key of this machine if there is one. On an agent-only copy it prints `key none (agent-only copy)`.

## Write

### `add <type> <value> [flags]`

Adds one record about you. Prints its id.

- `<type>` names what the record is, for example `food` or `symptom.headache`. Lowercase letters, digits, `_`, `-` and dots. See the [type vocabulary](../cmd/bean/vocabulary.md).
- `<value>`: a number becomes `{"n": ...}`, `true`, `false`, `yes` or `no` becomes `{"b": ...}`, and anything else becomes text, `{"s": ...}`. Put text with spaces in quotes.

| Flag | Meaning |
|---|---|
| `-unit <unit>` | Unit of a number, for example `kg`, `L`, `mg`, `{score}`. |
| `-at <time>` | When it happened. Default: now, to the minute. A time it cannot read exits 2 and writes nothing. |
| `-note <text>` | Free text. Stored in `context.note`. |
| `-about <id>` | The record that this one comments on. |
| `-corrects <id>` | Your earlier record that this one replaces, for example a wrong value. `ls` then shows only the new one. You can correct only records that you wrote. |
| `-photo <file>` | Attach an image, stored encrypted. Only image files are accepted, up to 25 MiB. |
| `-agent` | Sign with the agent key of this machine, not your key. |

### `append [-agent]`

Reads records from stdin, one JSON object per line, as an importer writes them. Signs and stores each one. Prints `added <n>, skipped <m>`. A record that is already in the log, or that you deleted, is skipped, so you can import the same file again safely. `-agent` signs with the agent key.

### `blob put <file>`

Encrypts and stores a file, for example the PDF an import came from. Prints the blob id. Give that id to an importer with `-blob`. The file can be up to 256 MiB, the most that `import-export` reads back, so every export of the log can be read again. A larger file is refused with exit 1.

### `import <source> [importer flags] [-corrections] <file>`

Keeps a provider file, then adds its readings. Runs `bean-import-<source>`, found next to `bean` or on `PATH`, for example `bean import labcorp -tz America/Chicago report.pdf`. In order, and it stops at the first failure:

1. It copies the file privately, at most 256 MiB, works out the id the encrypted copy will have, and runs the importer on the copy. The importer's output goes to a private file, at most 1 GiB of readings plus 1 MiB. If the importer refuses the file, nothing is written.
2. It checks the importer's first record: it must keep this file (`lab.report`, `apple-health.export`, `csv.file`) and carry `source.ref_input`, not a ready-made `source.ref`. If it does not, or you deleted that record before with `purge`, nothing is written, not even the file. A file you deleted stays deleted under any `-system`.
3. It stores the copy, encrypted, and reads it back. The bytes must match.
4. It adds the first record and checks that the log keeps it: a current record of yours, about you, that names the blob and the file's SHA-256, and a blob that still reads back as the file. This check proves every record file, as `verify` does, because the object cache vouches for a file by its size and time.
5. It adds the readings, then checks the file record again.

The error says what was written. A failure in step 5 can leave some readings; run the same command again, it adds only what is missing. Ctrl-C, `kill`, a closed terminal (SIGHUP) or SIGQUIT stops the importer and removes the private copy; the exit code is 128 plus the signal number. When the import worked, the importer's last message comes last, after its name, with control characters shown as `\xNN`. When it failed, only the error is printed. A changed reading works as in `append`: nothing is written for it, the command exits 1, and `-corrections` writes it. A reading that `-corrections` still does not write has another type or no value under a ref the log has: give each source its own `-system`.

Readings name the file by its SHA-256 (`context.report_sha256`, `export_sha256`, `file_sha256`), not as a blob. So `purge` of the record that keeps the file erases the file. Readings from importers before `bean-import-labcorp@0.2.0`, `bean-import-apple-health@0.2.0` and `bean-import-csv@0.6.0` still name the blob, and the file stays until you purge them too.

## Read

### `ls [flags]`

Lists current records in time order: when, type, value, who wrote it if not you, and the first 12 hex digits of the id. A corrected record shows only its newest version. Deleted records never show.

A record that keeps a file, such as `lab.report` or `apple-health.export`, shows its value as `file <media type> <blob id start>`. A value in a blob with a unit shows as `series <blob id start> <unit>`. A time that `-since` or `-until` cannot read exits 2.

| Flag | Meaning |
|---|---|
| `-type <type>` | Only this type. End with `*` for a prefix: `-type 'lab.*'`. |
| `-since <time>` | From this time. |
| `-until <time>` | Before this time. |
| `-author <who>` | `me`, `agent` (this machine's agent), an identity id, a tagged key or an npub. |
| `-all` | Also show corrected versions, duplicates, pairing records and identity records. |
| `-json` | Print each record as stored, one per line. |

### `show <id>`

Prints one record in full, its status (`valid`, `superseded by ...`, `purged by ...`, or an error), its files, and the records that reply to it.

### `blob get <id>`

Writes the decrypted file to stdout: `bean blob get <id> > report.pdf`.

### `append`, changed readings

If an input gives a new value for a reading you already have, `append` writes nothing for it, prints a `changed:` line, and exits 1. bean cannot tell a lab's corrected report from an older report, a parser bug, or two sources that share an id. If the input is newer and right, run `append -corrections`: the new value replaces the old one as a correction, and the output says `1 of them new values`. It refuses a different type or an empty value, with or without the flag. Then the `changed:` line names the input's type and says why, for example `(type lab.ldl): not written, because its type differs`, and the error does not send you to `-corrections`. For two sources that share ids, give each its own `-system`. A value the reading has had is always skipped, so an old report cannot bring it back.

### `verify`

Reads and checks every file and signature, and every stored file's hash. Run it after you copy a log with another tool, or restore from a backup. Exit code 0 means no errors. It prints one line for each problem, then a summary:

| Line | Meaning |
|---|---|
| `error: <id or path>: <reason>` | A record or file is not valid. The reasons are listed in SPEC.md 12.2. |
| `warning: <id>: held: <id>` | A correction or deletion whose target this copy does not have. After you purge a record, its tombstone stays held: the target is erased. |
| `warning: <id>: duplicate: <id> of <id>` | The same reading was imported twice, for example on two machines. Readers show the first. |
| `warning: <id>: hidden pending gc: <id>` | A record deleted from another copy. `gc` erases it after 7 days. |
| `warning: <id>: unresolved author: <id>` | A record by an identity whose first record this copy does not have. |
| `warning: <id>: incomplete seals: <id>` | A rotation's `seal part` records are not all in this copy. The old key's records do not count until they arrive. Nothing is deleted. |
| `too new: <id>` | Records from a newer bean for this identity. Its records are not checked. Exit code 3 when it is yours: update bean. |
| `warning: <id>: ungated: <id>` | A record from a newer bean signed by a key that may not change the rules for the identity: an old key or a stranger. It is skipped and does not stop anything. |
| `compromised: <id>` | The identity forked and is frozen. Exit code 4 when it is yours. |
| `summary: <v> valid, <i> invalid, <s> skipped, <h> held, <d> duplicate` | The counts. |

## AI and agents

### `mcp [-write] [-label <name>]`

Runs a [Model Context Protocol](https://modelcontextprotocol.io) server on stdin and stdout, for any AI client that supports MCP. Without `-write` it can only read. The read tools show only records about you; `include_others` adds the other people in the folder. Each row names its `author` and `subject`: `self`, `agent:<label>` or an identity id. Its tools:

| Tool | Returns |
|---|---|
| `list_types` | Every type, with its count, units and date range. |
| `query_records` | Records by type and time. |
| `stats` | Count, minimum, maximum, mean, median and spread of one type, by day, week or month if asked. Readings by different authors or in different units are in separate `groups`. |
| `get_record` | One non-deleted, valid record, including superseded history, its status and replies. Blob IDs are metadata. |
| `read_blob` | Only blobs of current records of yours or of a paired agent; superseded-only history and a stranger's record do not grant content access. Text is returned; for binary files only size. |
| `log_entry` | With `-write` only. Adds a record signed by the agent key, of a type from the vocabulary only: the built-in one plus `vocabulary.md` in the bean folder, if it exists (cmd/bean/vocabulary.md, "Your own types"). It can also comment on a record (`about`) or replace one of the agent's own records (`corrects`). |

With `-write`, the first start makes the agent key of this machine and, on your own machine, trusts it. `-label` names that agent (default: `agent@<computer name>`). Without `-label`, a key that already exists keeps its name. A label that differs from the key's pairing is refused with exit 1, as in `agent init`.

### `agent init [-label <name>] [-identity <id>]`

Makes the agent key of this machine. Prints the agent key and its npub.

- On your own machine it also trusts the key (writes an `agent.pair` record).
- On a copy without your key, for example a cloud server, it prints the `bean agent pair` command to run on your own machine.
- In a folder with copied records but no identity, it takes the only identity there. Use `-identity` if there are several.
- If the key is already paired, it prints that pairing. A different `-label` is refused with exit 1, and the error names the label the key has. One machine has one agent key, and every assistant on it signs with it: a second name would show one assistant's records under another's name.
- If you unpaired the key, it refuses. Run `agent pair` to trust it again.

### `agent pair <npub> [-label <name>]`

Run on your own machine. Records that you trust this agent key to write about you. `bean ls` then shows its records with the label.

If the key is already paired, the same label or no `-label` prints that pairing. A different label is refused with exit 1 and changes nothing: bean does not rename an agent, because every record it wrote would then show the new name. A key you unpaired gets a new pairing.

## Copies and keys

### `sync <folder>`

Copies to this log every record and file that the other folder has and this one does not. It checks each file first, and replaces a stored copy that fails the checks. It reads this log's files from disk, not from the cache, so it also repairs a file that was changed in place. It does not copy a record that a valid deletion names. It judges each deletion on the files of both folders together, not on what the other folder says: a copy that has not seen your rotation takes the old key's deletions for valid. Run it both ways to make two copies the same. Prints `copied <r> records, <b> blobs, refused <p> purged`. Sync only from your own copies.

- The other folder must hold a log (`local/identity` or `objects/`). If it does not, `sync` fails with `no log in <folder>`, so a typo does not look like a sync with nothing to copy.
- A record file of the other folder that fails its checks is not copied, nor a file that this log needs whose hash is wrong. Then `sync` copies the rest, prints `skipped <n> bad record files, <m> bad blobs in <folder>`, and exits 1. Run `bean verify` in that folder. A stranger's broken file there has the same effect.
- It copies only the files that a record names. A file from `blob put` that no record names stays where it is; `export` writes it.

### `export <folder>`

Writes a plaintext copy of every record and file into an empty or new folder, with a `manifest.json`. The export is not encrypted. Prints `exported <r> records, <b> blobs (plaintext)` when it ran to its end. Into a folder that is not empty it fails at once and writes nothing.

| In the export | What it holds |
|---|---|
| `records/` | Every record file that passes its checks, also of a newer format. Deleted records stay behind. |
| `blobs/` | Your stored files, decrypted. |
| `invalid/<path>` | Each record file that fails its checks, as it is. `export` prints how many, and exits 0: nothing is left behind. Run `bean verify`. A damaged copy of a deleted record stays behind: a tombstone names its id. |
| `manifest.json` | The identity, the counts and each blob's SHA-256. Also, if any: `missing_blobs` (named by a record of yours or of a paired agent, but not here), `missing_blobs_of_others` (how many absent files only other records name; anyone can name any id), `unreferenced_blobs` (no record names them, for example from `blob put`; they are in `blobs/` too), `not_exported_blobs` (stored here, each with its reason), `invalid_records`, and `withheld_records` (damaged copies of deleted records). |

A file is decrypted only if a record of yours or of a paired agent names it, and that record is valid, kept or in doubt. A record of a newer format counts only if your key or the paired agent's key signed it: its author field alone proves nothing. A stranger's record, or one that is not valid, can name any file, also one you deleted: such a file is listed in `not_exported_blobs` and not written. The export fails at the end if a file of yours is missing, or if your identity has records from a newer bean. Everything else is in the folder by then.

### `import-export <folder>`

Reads an export into a log of the same identity. Prints `added <r> records, <b> blobs, refused <p> purged`.

- It reads `manifest.json` first, as a stream, and keeps only the identity, so a large log's export reads back. An export of another identity fails with `export of identity <id>, this log is <id>`, and a folder without the file is `not an export`.
- It follows the rule of `sync`: a record that a valid deletion names is refused, so an older export does not bring back what you purged since. Nor does it bring back a file that a deleted record names, unless a live record of yours or of a paired agent names it too. A stranger's record does not count.
- It does not read `invalid/`.

### `rotate [-list] [-seal-all] [-no-seal <id>]... [-nostr new|<file>]`

Changes your signing key. Reads the 24 words from stdin. Sync every copy first.

The sync can bring in records from a thief, and a seal cannot be undone, so `rotate` keeps nothing until you decide. Without `-seal-all` it shows what the old key signed and stops, before it asks for the words: every deletion, correction and pairing, every claim that takes the place of another record (`REPLACES`), with every record that they name, and a summary of the claims. A deletion or correction that names `A RECORD NOT IN THIS FOLDER` needs care: if you keep it, that record counts when it arrives. `-list` shows every claim too.

| Flag | Effect |
|---|---|
| `-seal-all` | Keep every record of the old key in this folder. Use it alone only if nobody else had the key. |
| `-no-seal <id>` | With `-seal-all`: do not keep this record. A deletion or correction that you do not keep is undone. A claim that you do not keep stops being valid. A wrong id is an error. |
| `-no-seal-file <file>` | The same, for a file with one id per line. |
| `-list` | Show every record of the old key. Write nothing. |

With nothing to decide on, for example on a new log, no flag is needed. Records the old key makes after the rotation are not valid. `-nostr` makes the new key a Nostr key, from a file because stdin carries the words. Above 200,000 records it also prints one `seal part <id>` line per extra seal record (C28); copy those with the rest of the log. It refuses above 200,200,000 records (`too large`). If a thief with your current key halted the log (`too new`), rotate goes past the halt and prints one `past halt <id>` line per halting record; the rotation retires that key, so the halt ends on every copy. A halt from a newer bean of your own (signed with the 24 words) is not passed: update bean.

### `restore [-list] [-seal-all] [-no-seal <id>]... [-nostr new|<file>] [-identity <id>]`

Rebuilds `local/` on a new machine. Copy `objects/` and `blobs/` from your backup into the folder, then run it and give the 24 words on stdin. It finds your identity, writes the identity secret, and moves you to a new signing key. Your identity id and records do not change. The lost key can no longer add, delete or correct anything. Sync every copy you still have into the folder first. The flags work as for `rotate`. If more than one identity matches the words, it fails with `ambiguous` and writes nothing; pass `-identity <id>`.

### `migrate [-also <id>]... [-also-file <file>] [-nostr new|<file>] <empty folder>`, `migrate -list`

Starts a new identity from a frozen log. On any other log it refuses: run `rotate`. On a new machine, run `restore` first: it fails with `compromised`, but it gives this copy what `migrate` needs. It copies your own records that `ls` shows, signs them with the new key, and shows 24 new words once. Each record keeps the old id in `context.migrated_from`. A record that only one side of the freeze corrected moves with its old value and names the correction in `context.migrated_disputed_by`, so check those. An agent's records, a stranger's, and claims in doubt do not move: signing them would vouch for them. The command counts them, and they stay in the old log. `migrate -list` shows them. What you logged after the fork is among them: if a claim is yours, name it with `-also <id>` or `-also-file <file>`. A thief can hold the same key, so read before you name it. Nothing links the two identities by signature. Set up agents again. A later import of an old export adds its records again, because the new identity makes different dedupe refs.

## Delete

### `purge [-reason <text>] <id>`

Deletes a record about you. Writes a tombstone that every copy obeys, and erases the record here at once. `sync` and `import-export` refuse the record after that, also from an older copy or export. Its files are erased too, unless another record names them. Then `purge` prints `the file <id> is kept: <n> other records name it`, and the file is still readable. Purge those records too to erase it. `export` writes a kept file only if a valid record of yours or of a paired agent names it. Files are erased before the record: if purge fails in between, the record and its tombstone stay, the next command hides the record, and `gc` erases it after the hold. The tombstone does not keep the import ref of the record. `local/purged-refs.jsonl` does, so a later import of the same data adds nothing on this copy, but it does on a copy without that file. You cannot purge identity records or other tombstones.

### `gc`

Erases records that a deletion from another copy hid at least 7 days ago, and the files that only those records used. It erases the files first, so a failure leaves the hidden record for the next `gc`. Nothing is erased without `gc` or `purge`. It refuses while your identity has records from a newer bean or is frozen, or a rotation's seal parts are missing (`incomplete seals`).

## Other

### `help`, `version`

Print the help or the version.

## Importers

Importers read one source format and write records to stdout. They never sign and never see your keys. Run them with `bean import <source>`, which keeps the file first; run alone, they print the records for `bean append`.

### `bean-import-labcorp [-blob <id>] [-tz <zone>] <report.pdf | report.txt>`

Reads a Labcorp patient report, which is also what Superpower gives you. Needs `pdftotext` (`brew install poppler`), or the text it makes. One record per result: type `lab.<test name>`, value and unit, and the flag, reference interval, panel, lab and comments. Your name, birth date and patient ids are not copied.

It refuses a report it cannot read for sure: two specimens, two results that map to one type, a test name that runs into the result column, or a result row before the column header. Columns are read from spaces only, so a line with a tab, form feed, vertical tab or no-break space is refused; the form feed that `pdftotext` puts at the start of a page is read. A result with no value whose name ends in a number or a result word (`Glucose 101`, `HCV Ab Non Reactive`) is refused: the value may be in the name. The results end at `Disclaimer`, `Icon Legend` or `Performing Labs`; a column header or a result row after that is refused, not dropped. A two-digit number at the end of a name is a lab code only if the report's list of performing labs names it, so `Interleukin 10` stays `lab.interleukin-10`. Without a list it can read whole, only a number in the code column is a lab code, and only when a value follows it on the row: else it may be the value, and the report is refused. The same reading listed in two panels is one result only when its value, flag, unit, range, lab and comment all match.

- `-blob`: the blob id of the PDF, from `bean blob put`. With it, the first record is a `lab.report` that keeps the PDF, and each reading has its SHA-256 in `context.report_sha256`. `bean import` passes it.
- `-tz`: the time zone where the sample was taken, for example `America/Chicago`. Without it, only the day is recorded, because the report does not say the zone.

### `bean-import-apple-health -blob <id> [-records [-types <t>,...]] <export.zip | export.xml>`

Reads an Apple Health export. By default it writes one `apple-health.export` record that keeps the file, with counts per type. With `-records` it also writes one record per reading of 16 supported types, or of the `-types` you name. Apple writes a percentage as a fraction with unit `%`, for the `*Percentage` types and walking steadiness alike. Any reading with unit `%` says unit `1` (0.272 is 27.2%) and keeps `context.source_unit: "%"`; a value outside 0..1 is refused. Blood pressure and food are Correlations: they are counted and kept in the file, and a selected reading inside one is refused. Your name, birth date, source and device names are not copied; the file keeps them. Run it as `bean import apple-health [-records] export.zip`.

### `bean-import-csv -type <type> [-unit <unit>] [-system <name>] [-blob <id>] [-precision <p>] <file.csv>`

Reads a CSV file with a header row. Column `value` is required. Column `time` (RFC 3339) and column `id` (the source's id for the row) are optional. Other columns go into `context`. One type per file. With `-blob` the first record is a `csv.file` that keeps the file, at most 256 MiB, and each reading has its SHA-256 in `context.file_sha256`; a column of that name is refused. Run it as `bean import csv -type <type> [-unit <unit>] file.csv`.

## Files in a log folder

| Path | Content | Copy it? |
|---|---|---|
| `objects/` | Signed records, plain JSON, named by their hash. | Yes. |
| `blobs/` | Stored files, encrypted, named by their hash. | Yes. |
| `local/identity` | Your identity id. | No. |
| `local/identity-secret` | The key for stored files and dedupe. | No. |
| `local/signing-key` | Your signing key. | No. |
| `local/agent-key` | The agent key of this machine. | No. |
| `local/hidden/`, `local/holds.json` | Deletion state of this copy. A lost `holds.json` starts each wait again. | No. |
| `local/purged-refs.jsonl` | The import refs of purged records. Without it, a new import of the same data brings them back. | Back it up; do not sync it. |
| `local/cache/` | Caches. Safe to delete. | No. |
