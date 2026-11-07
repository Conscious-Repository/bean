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
| 2 | Usage error: unknown command, bad flag or bad argument. |
| 3 | The log has a record of a newer format version than this `bean` knows. Update `bean`. |
| 4 | Your identity is frozen: two rotations were made from the same point. Commands that read still print, and warn you. Commands that write stop. Run `bean migrate`. |

| Environment variable | Meaning |
|---|---|
| `BEAN_DIR` | The log folder, if `-dir` is not given. |
| `BEAN_NOW` | Overrides the clock (RFC 3339). For tests only. |

## Start

### `init [-nostr new|-|<file>]`

Creates your identity and an empty log in the folder. Prints `identity <id>`, then `npub <npub>` if you chose a Nostr key, then the 24 words.

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
| `-at <time>` | When it happened. Default: now, to the minute. |
| `-note <text>` | Free text. Stored in `context.note`. |
| `-about <id>` | The record that this one comments on. |
| `-corrects <id>` | Your earlier record that this one replaces, for example a wrong value. `ls` then shows only the new one. You can correct only records that you wrote. |
| `-photo <file>` | Attach an image, stored encrypted. Only image files are accepted, up to 25 MiB. |
| `-agent` | Sign with the agent key of this machine, not your key. |

### `append [-agent]`

Reads records from stdin, one JSON object per line, as an importer writes them. Signs and stores each one. Prints `added <n>, skipped <m>`. A record that is already in the log, or that you deleted, is skipped, so you can import the same file again safely. `-agent` signs with the agent key.

### `blob put <file>`

Encrypts and stores a file, for example the PDF an import came from. Prints the blob id. Give that id to an importer with `-blob`.

## Read

### `ls [flags]`

Lists current records in time order: when, type, value, who wrote it if not you, and the first 12 hex digits of the id. A corrected record shows only its newest version. Deleted records never show.

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

### `verify`

Reads and checks every file and signature, and every stored file's hash. Run it after you copy a log with another tool, or restore from a backup. Exit code 0 means no errors. It prints one line for each problem, then a summary:

| Line | Meaning |
|---|---|
| `error: <id or path>: <reason>` | A record or file is not valid. The reasons are listed in SPEC.md 12.2. |
| `warning: <id>: held: <id>` | A correction or deletion whose target this copy does not have. After you purge a record, its tombstone stays held: the target is erased. |
| `warning: <id>: duplicate: <id> of <id>` | The same reading was imported twice, for example on two machines. Readers show the first. |
| `warning: <id>: hidden pending gc: <id>` | A record deleted from another copy. `gc` erases it after 7 days. |
| `warning: <id>: unresolved author: <id>` | A record by an identity whose first record this copy does not have. |
| `warning: <id>: unattested: <id>` | A claim that a retired key signed and nothing vouches for. `ls` hides it. If it is yours, run `bean checkpoint -accept <id>`. |
| `missing: <id> of <identity>` | A checkpoint names this record, but this copy does not have it and no deletion explains it. Exit code 1 if the identity is yours. Sync it from another copy. If it is gone for good, run `bean purge <full id>`. |
| `summary: <v> valid, <i> invalid, <s> skipped, <h> held, <d> duplicate` | The counts. `, <u> unattested` and `, <m> missing` follow when they are not zero. |

## AI and agents

### `mcp [-write] [-label <name>]`

Runs a [Model Context Protocol](https://modelcontextprotocol.io) server on stdin and stdout, for any AI client that supports MCP. Without `-write` it can only read. Its tools:

| Tool | Returns |
|---|---|
| `list_types` | Every type, with its count, units and date range. |
| `query_records` | Records by type and time. |
| `stats` | Count, minimum, maximum, mean, median and spread of one type, by day, week or month if asked. |
| `get_record` | One record, its status and its replies. |
| `read_blob` | A stored text file. For other files, only the size. |
| `log_entry` | With `-write` only. Adds a record signed by the agent key. It can also comment on a record (`about`) or replace one of the agent's own records (`corrects`). |

With `-write`, the first start makes the agent key of this machine and, on your own machine, trusts it. `-label` names that agent (default: `agent@<computer name>`).

### `agent init [-label <name>] [-identity <id>]`

Makes the agent key of this machine. Prints the agent key and its npub.

- On your own machine it also trusts the key (writes an `agent.pair` record).
- On a copy without your key, for example a cloud server, it prints the `bean agent pair` command to run on your own machine.
- In a folder with copied records but no identity, it takes the only identity there. Use `-identity` if there are several.

### `agent pair <npub> [-label <name>]`

Run on your own machine. Records that you trust this agent key to write about you. `bean ls` then shows its records with the label.

## Copies and keys

### `sync <folder>`

Copies to this log every record and file that the other folder has and this one does not. It checks each signature first. It does not copy a record that a valid deletion names. Run it both ways to make two copies the same. Prints `copied <r> records, <b> blobs, refused <p> purged`.

### `export <folder>`

Writes a plaintext copy of every record and file into an empty or new folder, with a `manifest.json`. The export is not encrypted.

### `import-export <folder>`

Reads an export into a log that has the same identity secret.

### `rotate [-seal-all | -seal-none] [-seal <id>]... [-no-seal <id>]... [-nostr new|<file>]`

Changes your signing key. Reads the 24 words from stdin. Sync every copy first.

The sync can bring in records from a thief, so `rotate` keeps nothing without your decision. It lists each deletion, correction and checkpoint of the old key, and each claim that no checkpoint covers. If a record has no decision, it stops before it asks for the words, and it writes nothing.

| Flag | Effect |
|---|---|
| `-seal-all` | Keep every record in the list. Use it only if the key was not stolen. |
| `-seal-none` | Keep only the records that `-seal` names. |
| `-seal <id>` | Keep this record. Repeat it for more records. |
| `-no-seal <id>` | Do not keep this record. A deletion or correction that you do not keep is undone. A claim that you do not keep is unattested. |

`-nostr` makes the new key a Nostr key, from a file because stdin carries the words.

### `restore [-seal-all | -seal-none] [-seal <id>]... [-no-seal <id>]... [-nostr new|<file>] [-identity <id>]`

Rebuilds `local/` on a new machine. Copy `objects/` and `blobs/` from your backup into the folder, then run it and give the 24 words on stdin. It finds your identity, moves you to a new signing key, and writes the identity secret. The flags are the same as for `rotate`. Your identity id and records do not change. The lost key can no longer delete or correct anything. Sync every copy you still have into the folder first. If the words fit more than one genesis, it stops: someone may have copied yours. `-identity <id>` picks yours, the id that `bean init` printed. It also works on an agent-only copy.

### `checkpoint [-accept <id>]...`

Writes a signed list of the record ids that no checkpoint covers yet. A covered record that is removed then shows in `verify`. Your writes add a checkpoint when 256 or more records are uncovered, or the oldest is more than a day old, so you seldom need this command. `-accept <id>` vouches for an unattested claim.

### `migrate [-nostr new|<file>] <empty folder>`

Starts a new identity from this log, for example after a freeze. It copies what `ls` shows, signs it with the new key, and shows 24 new words once. Each record keeps the old id in `context.migrated_from`. It leaves out unattested claims and pairings, and says how many claims it left out. Nothing links the two identities by signature. Agents must be set up again. A later import of an old export adds its records again, because the new identity makes different dedupe refs.

## Delete

### `purge [-reason <text>] <id>`

Deletes a record about you. Writes a tombstone that every copy obeys, and erases the record here at once. A later import of the same data adds nothing. You cannot purge identity records or other tombstones.

### `gc`

Erases records that a deletion from another copy hid at least 7 days ago, and files that no record uses. Nothing is erased without `gc` or `purge`. It refuses to run on a frozen or too-new identity, or without the genesis, because it cannot tell what is still needed.

## Other

### `reindex`

Deletes `local/cache/` and writes `local/cache/index.json` again: one summary line per record, for other programs to read.

### `help`, `version`

Print the help or the version.

## Importers

Importers read one source format and write records to stdout for `bean append`. They never sign and never see your keys.

### `bean-import-labcorp [-blob <id>] [-tz <zone>] <report.pdf | report.txt>`

Reads a Labcorp patient report, which is also what Superpower gives you. Needs `pdftotext` (`brew install poppler`), or the text it makes. One record per result: type `lab.<test name>`, value and unit, and the flag, reference interval, panel, lab and comments. Your name, birth date and patient ids are not copied.

- `-blob`: the blob id of the PDF, from `bean blob put`.
- `-tz`: the time zone where the sample was taken, for example `America/Chicago`. Without it, only the day is recorded, because the report does not say the zone.

### `bean-import-csv -type <type> [-unit <unit>] [-system <name>] [-blob <id>] [-precision <p>] <file.csv>`

Reads a CSV file with a header row. Column `value` is required. Column `time` (RFC 3339) and column `id` (the source's id for the row) are optional. Other columns go into `context`. One type per file.

## Files in a log folder

| Path | Content | Copy it? |
|---|---|---|
| `objects/` | Signed records, plain JSON, named by their hash. | Yes. |
| `blobs/` | Stored files, encrypted, named by their hash. | Yes. |
| `local/identity` | Your identity id. | No. |
| `local/identity-secret` | The key for stored files and dedupe. | No. |
| `local/signing-key` | Your signing key. | No. |
| `local/agent-key` | The agent key of this machine. | No. |
| `local/hidden/`, `local/holds.json`, `local/purged-refs.jsonl` | Deletion state of this copy. | No. |
| `local/cache/` | Caches. Safe to delete. | No. |
| `local/lock` | Held while a command writes. Safe to delete when no command runs. | No. |
