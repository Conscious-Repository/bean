# Command reference

Every command and flag of `bean` and the importers. For a first run, follow [ONBOARDING.md](ONBOARDING.md).

## Conventions

```
bean [-dir <folder>] <command> [arguments]
```

- `-dir` is the log folder. If you do not give it, `bean` uses `$BEAN_DIR`, then the current folder.
- Flags can come before or after the arguments of `add`, `ls`, `compare`, `share`, `agent` and `migrate`. `import` takes `-yes` and `-corrections` anywhere. For other commands, put flags first.
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
| 128 + n | `import` was stopped by signal n and removed its private copy of the file. The code is 129 for SIGHUP, 130 for SIGINT (Ctrl-C), 131 for SIGQUIT and 143 for SIGTERM (`kill`). |

| Environment variable | Meaning |
|---|---|
| `BEAN_DIR` | The log folder, if `-dir` is not given. |
| `BEAN_NOW` | Overrides the clock (RFC 3339). For tests only. |

## Start

### `init [-nostr new|-|<file>] [-force]`

Creates your identity and an empty log in the folder. It refuses inside a git work tree, because a commit could publish your keys; `-force` overrides. Prints `identity <id>`, then `npub <npub>` if you chose a Nostr key, then the 24 words. Says on stderr where the log is.

- No flag: the signing key is a new random Ed25519 key.
- `-nostr new`: the signing key is a new Nostr (secp256k1) key, so the log has its own npub.
- `-nostr <file>`: use the Nostr secret key in the file (`nsec1...` or 64 hex digits).
- `-nostr -`: type or paste the secret key. The terminal does not show it.

Write the 24 words on paper. `bean` shows them only once. They are the only way to recover your identity on a new machine. Do not type them into a crypto wallet, a chat or an AI. Run `init` yourself, not through an assistant: an assistant that runs commands sees their output.

### `whoami`

Prints your identity id and your signing key. It prints your identity words: the first 88 bits of the id as eight words that you can read out. They are not a secret, and a reader's check of a bundle shows them. It also prints the npub of a Nostr key, and the agent key of this machine if there is one. On an agent-only copy it prints `key none (agent-only copy)`.

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
| `-corrects <id>` | Your earlier record that this one replaces, for example a wrong value. `ls` then shows only the new one. You can correct only records that you wrote. A record is corrected once: for a record that was corrected already, bean names the record to correct instead. Without `-at` the correction is dated now, and bean says so. |
| `-photo <file>` | Attach an image, stored encrypted. Only image files are accepted, up to 25 MiB. |
| `-agent` | Sign with the agent key of this machine, not your key. |

### `append [-agent | -corrections]`

Reads records from stdin, one JSON object per line, as an importer writes them. Signs and stores each one. Prints `added <n>, skipped <m>`. A record that is already in the log, or that you deleted, is skipped, so you can import the same file again safely. `skipped` counts every line not written: one already in the log, a deleted one, or a repeat of an earlier line of the same input. It also counts a correction whose target is not in the log, and each reading named on a `changed:` line (below). When there is a `changed:` line, `append` exits 1. `-agent` signs with the agent key.

### `blob put <file>`

Encrypts and stores a file, for example the PDF an import came from. Prints the blob id. Give that id to an importer with `-blob`. The file can be up to 256 MiB, the most that `import-export` reads back, so every export of the log can be read again. A larger file is refused with exit 1.

### `import <source> [importer flags] [-corrections] [-yes] <file>`

Keeps a provider file, then adds its readings. Runs `bean-import-<source>`, found next to `bean` or on `PATH`, for example `bean import lab -tz America/Chicago report.pdf`. In order, and it stops at the first failure:

1. It copies the file privately, at most 256 MiB, works out the id the encrypted copy will have, and runs the importer on the copy. The importer's output goes to a private file, at most 1 GiB of readings plus 1 MiB. If the importer refuses the file, nothing is written.
2. It checks the importer's first record: it must keep this file (`lab.report`, `apple-health.export`, `csv.file`, `reference.file`) and carry `source.ref_input`, not a ready-made `source.ref` or a `source.supersedes_ref`. It must pass the checks `append` makes of a record: its fields, its size, your key. If it does not, or you deleted that record before with `purge`, nothing is written, not even the file. A file you deleted stays deleted under any `-system`, on a copy whose `local/purged-refs.jsonl` remembers the deletion. A copy that only received the deletion by sync, and never held the record, stores the file again. A file kept under another archive type is not caught.
3. It shows the readings, one line each (type, value as printed, unit, flag), and asks `Save <n> readings and keep the file? [y/N]`. Only `y` or `yes` goes on; anything else writes nothing. The review lists the first 300 readings and counts the rest. `-yes` skips the review, for scripts. Without a terminal and without `-yes` it writes nothing and shows no value: it stops before the importer runs. The readings would land in a script's log or an AI assistant's context. A terminal is what the system says is one, on both ends: the review is printed and answered there. A closed input, `/dev/null` or a pipe on the output is not one.
4. It stores the copy, encrypted, and reads it back. The bytes must match.
5. It adds the first record and checks that the log keeps it. The record must be a current record of yours, about you, that names the blob and the file's SHA-256. The blob must still read back as the file. This check proves every record file, as `verify` does, because the object cache vouches for a file by its size and time.
6. It adds the readings, then checks the file record again.

The error says what was written. A failure in step 6 can leave some readings; run the same command again, it adds only what is missing. Ctrl-C, `kill`, a closed terminal (SIGHUP) or SIGQUIT stops the importer and every process it started, such as `pdftotext`. That includes a process still running after the importer exited. bean removes the private copy, and the exit code is 128 plus the signal number. A process the importer started that keeps its output open after it exits fails the import after one second, and is stopped. The importer runs in a process group of its own. So a SIGKILL to bean's group (`timeout -s KILL`, a job runner cancelling a step) does not reach it. bean, killed, can neither stop it nor remove the private copy. The copy stays in the temporary folder until that folder is cleaned. On Linux the kernel then kills the importer, but not processes it started. On macOS nothing does: the importer runs to its end, within its own limits (30 seconds for `pdftotext` in `bean-import-lab`). Stop bean with a signal it can catch. When the import worked, the importer's last message comes last, after its name, with control characters shown as `\xNN`. When it failed, only the error is printed. A changed reading works as in `append`: nothing is written for it, the command exits 1, and `-corrections` writes it. A reading that `-corrections` still does not write has another type, or no value, under a ref the log has. Give each source its own `-system`.

On success the first line is `kept <type> <record id>: file sha256 <hash>, stored as <blob id>`, or `already kept ...` when the file was kept before. `purge <record id>` erases the file. The hash is what `shasum -a 256` gives for your file. `blob get <blob id>` reads the encrypted copy back.

Readings name the file by its SHA-256 (`context.report_sha256`, `export_sha256`, `file_sha256`), not as a blob. So `purge` of the record that keeps the file erases the file. Readings from importers before `bean-import-lab@0.2.0`, `bean-import-apple-health@0.2.0` and `bean-import-csv@0.6.0` still name the blob, and the file stays until you purge them too.

## Read

### `ls [flags]`

Lists current records in time order: when, type, value, who wrote it if not you, and the first 12 hex digits of the id. A corrected record shows only its newest version. Deleted records never show.

A record that keeps a file, such as `lab.report` or `apple-health.export`, shows its value as `file <media type> <blob id start>`. A `reference.file` shows `file <data set> <blob id start>`. A comparison shows in words, with its reading, cohort and n. A test the lab did not perform shows as `not performed`. A value in a blob with a unit shows as `series <blob id start> <unit>`. A time that `-since` or `-until` cannot read exits 2.

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

Writes the decrypted file to stdout: `bean blob get <id> > report.pdf`. The id is a blob id, such as the `stored as` id of `bean import`, in full or its first 6 or more hex digits.

### `append`, changed readings

If an input gives a new value for a reading you already have, `append` writes nothing for it, prints a `changed:` line, and exits 1. bean cannot tell a lab's corrected report from an older report, a parser bug, or two sources that share an id. If the input is newer and right, run `append -corrections`: the new value replaces the old one as a correction, and the output says `1 of them new values`. It refuses a different type or an empty value, with or without the flag. Then the `changed:` line names the input's type and says why, for example `(type lab.ldl): not written, because its type differs`, and the error does not send you to `-corrections`. For two sources that share ids, give each its own `-system`. A value the reading has had is always skipped, so an old report cannot bring it back. This holds only with the reading's own type and time. A reading of another type or time under the same id is a `changed:` line even when its value is the same. `-corrections` does not write it. Two files that number their rows alike under one `-system` give this; give each its own `-system`. Two lines of one input with the same id and another reading stop the input with an error.

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

## Compare

### `compare -sex male|female -age <from>-<to> [-where <rule>]... [-unknown-files] [-save] <id>... | -type <type>`

Shows where a reading lies among the people of a cohort in NHANES 2017-2018, from the files you kept with `bean import nhanes`. [COMPARE.md](COMPARE.md) has the files, the columns, the limits and the method.

- `-sex` and `-age` are required. `-age 30-39` includes both ends; 80 means 80 and over. Without them the command exits 2: bean never guesses a cohort.
- `-where <DATASET>.<VARIABLE>=<value>` or `=<from>-<to>` adds a rule, and may repeat: `-where DEMO_J.RIDRETH3=3 -where BMX_J.BMXBMI=20-27`. The file is `DEMO_J`, `BMX_J` or `DR1TOT_J`. A rule on `DR1TOT_J` makes the computation use the dietary day one weight.
- Give one or more record ids, or `-type` for every current reading of a type or prefix (`'lab.*'`). With `-type`, readings of a type bean has no reference data for are counted, not listed.
- `-unknown-files` also uses a kept file whose SHA-256 is not the one on record. Without it such a copy is refused: it may be a download that was cut short.
- `-save` writes each result as a signed `comparison.percentile` record and prints `saved <id>`. Without it nothing is written. The same inputs again add nothing. A comparison you deleted before is saved again when you ask again. The last line is `added <n>, already there <m>`.

`purge`, `add -corrects`, `append`, `import`, `sync`, `import-export` and `verify` delete the saved comparisons of a reading that was corrected or deleted. They do so also when they fail part way, and say how many on stderr. `compare` and `share` do the same before anything else.

Each row starts with the date the sample was taken, as `bean ls` shows it. Then come the reading, its value, the percentile, and the shares of the cohort with a lower and with the same value. Then come the cohort's 25th to 75th percentile and median, the number of people, and a note. The note is `small cohort` under 100 people, `file not on record`, or `not comparable:` and why, with no number. Without `-pack`, a cohort of fewer than 30 people gives no number. A cell of a pack has no least size. The rows are in order of when the sample was taken, then of type, so the readings of one draw sit together. Under the table come what to do about a missing result and the types bean has no reference data for. Last come the caveats that every number carries. Exit 1 when nothing could be compared.

A comparison says where a value lies in a sample. It does not say whether the value is healthy.

### `compare -pack <file> [[-cohort <cohort> [-save]] [-accept-pack <sha256> | -unknown-pack] <id>... | -type <type>]`

Compares with a reference pack: one file of public reference data, with a table for each measure and each cohort on its menu. [REFERENCE.md](REFERENCE.md) has the format, the method and how a pack is built.

- With only `-pack`, it shows what the pack holds: its SHA-256, its source and the files it was built from. Then come the types and units it takes, and its cohorts. It says whether this is the pack that bean has on record.
- `-cohort` names one cohort of the pack's menu, such as `male-30-39`. The output and `-save` are as above. A saved claim names the pack by its SHA-256.
- With readings and no `-cohort`, each row gets the cohort of the age on the reading's own date. bean takes it from two records, `person.sex` and `person.birth`, which you state with `bean add`. Both are optional. A COHORT column shows each row's cohort. [REFERENCE.md](REFERENCE.md#the-cohort-from-two-records) has the rules, and says who can read the two records.
- That way shows, and saves no comparison: `-save` needs `-cohort`. As with `-cohort`, bean first deletes the saved comparisons of a reading that is gone. It works only with a pack on record. `-accept-pack` and `-unknown-pack` do not change that. With `-cohort`, bean reads neither record.
- With no record that counts, the command exits 2 and names `-cohort`. When a record breaks a rule, it exits 1. It names the record by id, and never its value.
- A pack that bean has no record of is refused, with a warning and its SHA-256. Whoever built such a pack chose its tables, its caveats and what each reading is set against.
- `-accept-pack <sha256>` uses such a pack when the file has that SHA-256. The value is 64 hex digits in lower case, with no prefix. Pass the SHA-256 that the pack's publisher states. When the file has another, bean stops and shows both.
- `-unknown-pack` uses any such file as it is. Pass one of the two flags, not both.
- With either flag, each row says `pack NOT on record`, and each saved claim has that caveat. A claim is the same with either flag. bean stores no record of the flag.
- `-accept-pack` with a pack on record checks the SHA-256 as well, and changes nothing else.
- The pack decides which types and units it takes. The output shows which measure each reading was set against. A reading in another unit gets a row that says so, and no number.
- A pack can require signed fields of a reading: its source system, its test and its panel. A reading without them gets a row that names them, and no number. The row starts with `required signed fields:` or `required signed evidence:`. An importer writes those fields from a report. `bean add` writes none, so a reading typed by hand cannot meet such a rule.
- A line says how many of the selected readings were compared: `Compared 2 of 5 selected readings.`
- A caveat that the pack states shows as a quote, in the list of a pack and in a comparison: `the pack says: "..."`. A pack can state any words. Only the other caveats are bean's.
- bean reads the file and goes nowhere. `-sex`, `-age`, `-where` and `-unknown-files` are for the files, and are refused with `-pack`.

`bean-pack-nhanes -out <new file> <DEMO_J.xpt> <a test's .xpt>...` builds the NHANES pack from files that you downloaded. The same files give the same pack, byte for byte.

## Share

### `share -out <folder or file> [-seal | -for <address> [-address-words "<eight words>"]] [-files] [-note <text>] [-yes] <id>... | -type <type> [-since <time>] [-until <time>]`

Writes chosen records into a new or empty folder, a bundle, that anyone can check without your log or keys. With `-seal` it writes one new file instead: the bundle, encrypted, with six words that open it. It prints the words once and does not keep them. With `-for` it writes one new file that is sealed to a reader's address. [SHARE.md](SHARE.md) says what goes in, what a reader can tell, and the format.

- `-for <address>` seals to a reader's address, and implies `-seal`. The reader makes the address with `bean address`. bean takes it only as the lower-case text that the age library writes, and stops for any other text.
- `-address-words` are the eight address words that the reader gave you. Without the flag bean asks for them on a terminal, and `-yes` never stands in for them. Without a terminal and without the flag it writes nothing. When they are not the words of the address, it writes nothing.
- With `-for`, bean prints no six words. It makes the file, new and empty, then adds the share record, then writes the package. If the file cannot be written whole, it says what it did.

- Give record ids, or `-type` with optional `-since` and `-until`. Only current records about you can be shared, and not one that another identity signed. A record brings the record it is about, one step and no further. It never brings a `person.sex` or `person.birth` record: name that one by its id, or pick it by its type. A type never picks a record that keeps a file, such as a lab report: name that one by its id.
- `-files` also writes, in plaintext, the file of each record you named by its id. It also writes each reference file among the records that bean has on record, by the SHA-256 of its bytes. A file never goes because its record came along or was picked by type. A kept copy of a survey file that is not the one on record goes only when you name its record. What stays home is said on stderr. Only your record or a paired agent's opens a file: bean refuses `-files` for a named record of any other key.
- `-note` is a line for the reader, kept in the share record.
- It lists what will leave, on stderr, and asks `Share <n> records? [y/N]`. `-yes` skips that. Without a terminal and without `-yes` it writes nothing.
- The folder must be new or empty, and not inside your log.

It builds the bundle beside its place, checks it as its reader will, and moves it there. Then it adds one `share` record to your log: the signed list of what the bundle holds, which is in the bundle too. A share that fails leaves neither. A bundle is not encrypted, and once sent it cannot be taken back. It prints `shared <n> records and <m> files in <folder>`, then `share record <id>`.

### `address -out <new file> | -key <file>`

Makes a key file for packages that are sealed to you, or shows the address of one. [SHARE.md](SHARE.md) ("Seal it to a reader's address") says what a key file is and what it costs.

- `-out` makes a key and writes it into a new file with mode 0600. It writes over no file. The file is a standard age identity file. When a log is found, a place inside the log's folder is refused.
- `-key` shows the address of a key file, also of one that `age-keygen` made. It prints nothing secret. When the mode of the file lets other users read it, bean warns on standard error.
- Both print `address <address>`, then `address words <eight words>`, then what you must know before you give the address away.
- It needs no log. bean keeps no key file and no list of them: `bean restore` and `bean export` do not hold one.

### `check-bundle [-key <file>] [-unpack <new folder>] [-pack <file>]... [-accept-pack <sha256>]... <folder or file>`

Checks a bundle, shows what it holds when it passes, and changes nothing in it. Given a file, it reads a zip file of a bundle's folder, or a sealed package. One scan of the file's header decides what it asks for. For a package that is sealed with words it asks for the six words, and reads them from standard input. A package that is sealed to an address opens with `-key <file>`, your key file with exactly one identity. Flags come before the file's name. With wrong words, without the key or with another key, the last line is `the package did not open: nothing was checked`, exit 1. When a key opened the package, the check first says that anyone who has your address can seal to it. With `-unpack`, a package that passes is written into a new folder as a bundle, with the files it carries. It needs no log, and reads only the bundle's `objects` and `files` folders; anything else in the folder is named and not read. It prints any record that fails, then the share record's id and the identity and key that signed it. It prints how many records and files it holds. It prints who signed the records: the person, an agent the person paired, or another key. It prints how many are corrections of a record that is not in the bundle. When every check passed it lists the records under `It holds:`, a comparison in words with its reading, cohort and n. Then `bundle ok`, exit 0, or `bundle FAILED: <n> problems`, exit 1. It says each time what no bundle can show: that it is complete, that a value is true, and who the identity is. Nor can a bundle show that the key that signed is still the person's. `tools/second/checkbundle.py` does the same check without bean.

With `-pack <file>`, bean also computes each comparison by a pack again, by the four steps of [REFERENCE.md](REFERENCE.md) ("The claim"). The flag may repeat, one for each pack. Without it the check prints what it printed before.

- The lines come after `bundle ok`. Each comparison gets one: `computes again`, `DIFFERS` and why, or `CANNOT CHECK` and what to get.
- `CANNOT CHECK` is a check that cannot be made: the pack with that SHA-256 was not given, or the reading is not in the bundle.
- A comparison with the survey's files is `not computed here`, and the check does not pass. `tools/compare/reproduce.py` computes it, given the files.
- A pack that bean has no record of counts only with `-accept-pack <sha256>`, as in `compare`. Take the value from the pack's publisher. The flag may repeat.
- `-accept-pack` with a pack on record, or with a pack that no comparison used, fails the check. So does `reproduce.py`.
- A file that is not a pack is named, and each comparison that names it differs. A file that cannot be read is an error of use. So is one over 64 MiB.
- The check says once what it shows: the numbers follow from the reading and the pack. It does not show that the reading is true, or that the pack is right.
- Exit 0 when every comparison computes again, and each pack is on record or accepted. Exit 3 when the bundle passed and that is not so.
- Exit 1 stays a bundle that failed. Then nothing is computed, and the last line says so. An error of use exits 2.
- With `-unpack`, a package that passes is written also when the exit is 3.
- Treat any exit but 0 as not checked. A bundle with a comparison of the survey's files exits 3 by design: bean does not compute it.

`reproduce.py` is a second implementation that shares no code with bean. `make pack` gives both the same records, and they must agree on each claim. `check-bundle -pack` places a value with the code of `bean compare`: a fault there would be in both.

A reader with no terminal runs the same check in a browser. `make page` builds `bean-check.html`, one file that reads a bundle's folder or a zip file of it and sends nothing ([SHARE.md](SHARE.md)).

### `reply -to <what you received> [-key <file>] [-for <address> [-address-words "<eight words>"] [-yes]] -out <new file> <id> <note> [<id> <note>]...`

Signs notes about records that someone shared with you, and seals them for that person. [SHARE.md](SHARE.md) ("A signed reply") says what a reply holds and what it shows.

- `-to` is a sealed package, or a bundle's folder or zip file. It must pass `check-bundle`. For a sealed package bean asks for its six words on standard input. A package that is sealed to your address opens with `-key <file>`. Then bean first says what a key shows, and what to ask the signer.
- Each `<id>` names a record of that share: its id from the check, or the first 6 or more hex digits. The id of the share record names the share itself. Each `<note>` is a text of 1 to 4000 bytes. Put `--` before the first id when a note starts with `-`.
- It needs your own log and key: a note is signed with your key, about the other person.
- Without `-for`, it writes one new file, sealed with six words, and prints the words and your identity words. That is so also when the share came sealed to your address. Your log gets a `reply` record: to whom, to which share, how many notes. The notes are in the file only.
- `-for <address>` seals the reply to an address that the person you reply to gave you. bean takes an address from this flag only. It asks for that person's eight address words before it opens what you received, or takes `-address-words`. `-yes` never stands in for them.
- With `-for`, bean says before it writes that you cannot open the reply again, and what such a seal means. On a terminal it then asks `Write the reply with <n> notes? [y/N]`. `-yes` skips that. Without a terminal and without `-yes` it writes nothing. `-address-words` and `-yes` go with `-for` only.
- With `-for` it prints no six words. It makes the file new and empty, then adds the `reply` record, which says `"seal": "recipient"`, then writes the bytes.

### `accept [-label <name>] [-words "<eight words>"] [-yes | -key <file>] <reply>`

Checks a reply to a share of yours and takes its notes into your log. It asks for the six words. A reply that is sealed to your address opens with `-key <file>`, your key file. Then bean first says that anyone who has your address can seal to it. Without `-key`, bean says that the file is sealed to a key. Then it shows the share, the eight identity words of whoever signed, and each note with the record that it is about. Then it asks.

- `-label` is the name under which this person's notes show. The first reply of an identity needs it. An identity keeps its name, and a name belongs to one identity. A name has 1 to 64 characters: the letters a to z, digits, single spaces, and `. ' _ - @`. It does not start with the word `peer`. The label of an agent follows the same rule, and is not the name of a peer.
- `-words` are the identity words that the person read to you. When they are not the signer's, nothing is taken.
- `-yes` skips the question, for scripts, and needs `-words`. Without a terminal and without `-yes` nothing is taken.
- `-yes` does not go with `-key`: exit 2. A program that holds your key file could make the identity words from the reply itself. So `accept -key` asks on a terminal, and takes nothing without one.
- It takes notes about records of that share and nothing else. A reply that holds anything more is refused whole. So is a reply for another person, or of a key that you unpaired.
- It refuses a reply that shows its identity's chain of keys shorter than you have seen it, or with another history. That is how a retired key shows itself.
- It adds the notes and one `agent.pair` record of yours that says the key is a peer's, with the name. A note that you deleted is not taken again.

Exit 1 when the reply was refused. A reply whose notes are all in your log already takes nothing, and exits 0.

### `unpair <name>`

Takes your word back from every key that you paired under a name, a peer's or an agent's. It prints each key. The records that those keys signed stay, and show as an unpaired key's. A later reply of such a key is refused. Exit 1 when no key has that name.

## AI and agents

### `mcp [-write] [-blobs] [-label <name>]`

Runs a [Model Context Protocol](https://modelcontextprotocol.io) server on stdin and stdout, for any AI client that supports MCP. Without `-write` it can only read. Without `-blobs` it does not offer `read_blob`. A stored file such as a kept report holds your name and birth date, and what a tool returns goes to the AI provider. The read tools show only records about you; `include_others` adds the other people in the folder. Each row names its `author` and `subject`: `self`, `agent:<label>` or an identity id. Its tools:

| Tool | Returns |
|---|---|
| `list_types` | Every type, with its count, units and date range. |
| `query_records` | Records by type and time. |
| `stats` | Count, minimum, maximum, mean, median and spread of one type, by day, week or month if asked. Readings by different authors or in different units are in separate `groups`. |
| `get_record` | One non-deleted, valid record, including superseded history, its status and replies. Blob IDs are metadata. |
| `read_blob` | With `-blobs` only. Takes the blob id in full or as its first 6 or more hex digits. Only blobs of current records of yours or of a paired agent; superseded-only history and a stranger's record do not grant content access. Text is returned; for binary files only size. |
| `log_entry` | With `-write` only. Adds a record signed by the agent key, of a type from the vocabulary only. The vocabulary is the built-in one plus `vocabulary.md` in the bean folder, if it exists (cmd/bean/vocabulary.md, "Your own types"). It can also comment on a record (`about`) or replace one of the agent's own records (`corrects`). It refuses `lab.*`, `apple-health.*` and `csv.file`: only an importer that keeps the source file writes those. It also refuses `reference.*`, `comparison.*` and `share`: only `bean import`, `bean compare` and `bean share` write those. |

With `-write`, the first start makes the agent key of this machine and, on your own machine, trusts it. `-label` names that agent (default: `agent@<computer name>`). Without `-label`, a key that already exists keeps its name. A label that differs from the key's pairing is refused with exit 1, as in `agent init`.

### `agent init [-label <name>] [-identity <id>]`

Makes the agent key of this machine. Prints the agent key and its npub.

- On your own machine it also trusts the key (writes an `agent.pair` record).
- On a copy without your key, for example a cloud server, it prints the `bean agent pair` command to run on your own machine.
- In a folder with copied records but no identity, it takes the only identity there. Use `-identity` if there are several.
- If the key is already paired, it prints that pairing. A different `-label` is refused with exit 1, and the error names the label the key has. One machine has one agent key, and every assistant on it signs with it: a second name would show one assistant's records under another's name.
- If you unpaired the key, it refuses. Run `agent pair` to trust it again.

### `agent pair <npub> [-label <name>]`

Run on your own machine. Records that you trust this agent key to write about you. `bean ls` then shows its records with the label. It prints `paired <id>`, the id of the pairing record.

If the key is already paired, the same label or no `-label` prints that pairing. A different label is refused with exit 1 and changes nothing. bean does not rename an agent, because every record it wrote would then show the new name. A key you unpaired gets a new pairing.

## Copies and keys

### `sync <folder>`

Copies to this log every record and file that the other folder has and this one does not. It checks each file first, and replaces a stored copy that fails the checks. It reads this log's files from disk, not from the cache, so it also repairs a file that was changed in place. It does not copy a record that a valid deletion names. It judges each deletion on the files of both folders together, not on what the other folder says. A copy that has not seen your rotation takes the old key's deletions for valid. Run it both ways to make two copies the same. Sync only from your own copies. It refuses a folder that is not one. That is a folder whose `local/identity` names someone else, or one with no `local/` that does not hold your identity record. A bundle someone sent is such a folder: read that with `check-bundle`. This catches a mistake; it does not stop a folder built to pass it. It prints `copied <r> records, <b> blobs, refused <n> purged`, and `replaced <n> bad copies` when it repaired any.

- The other folder must hold a log (`local/identity` or `objects/`). If it does not, `sync` fails with `no log in <folder>`, so a typo does not look like a sync with nothing to copy.
- A record file of the other folder that fails its checks is not copied, nor a file that this log needs whose hash is wrong. Then `sync` copies the rest, prints `skipped <n> bad record files, <m> bad blobs in <folder>`, and exits 1. Run `bean verify` in that folder. A stranger's broken file there has the same effect.
- It copies only the files that a record names. A file from `blob put` that no record names stays where it is; `export` writes it.

### `export <folder>`

Writes a plaintext copy of the log into an empty or new folder, with a `manifest.json`. The copy is every record that you did not delete, and your stored files. It holds no key and nothing of `local/`, so no reader's key file and no `local/purged-refs.jsonl`. It holds no package and no reference pack. The export is not encrypted. Prints `exported <r> records, <b> blobs (plaintext)` when it ran to its end. Into a folder that is not empty it fails at once and writes nothing. bean writes the export into a new folder beside the one you name, called `<folder>.partial-<digits>`, and moves it into place when it is whole. So the folder you name holds a whole export or nothing. A name that is a file, or a link that leads nowhere, is refused and stays as it is. If a run is stopped part way, that other folder stays, with records in plaintext. The next export names it and stops: delete it, then export again.

| In the export | What it holds |
|---|---|
| `records/` | Every record file that passes its checks, also of a newer format. Deleted records stay behind. |
| `blobs/` | Your stored files, decrypted. |
| `invalid/<path>` | Each record file that fails its checks, as it is. `export` prints how many, and exits 0: nothing is left behind. Run `bean verify`. A damaged copy of a deleted record stays behind: a tombstone names its id, or the record is hidden. A file under another name, such as a sync tool's conflict copy (`<name>.sync-conflict-….json`, `<name> (conflicted copy).json`, `<name> 2.json`), stands for the id its bytes give. |
| `manifest.json` | The identity, the counts and each blob's SHA-256. Also, if any: `missing_blobs` (named by a record of yours or of a paired agent, but not here). `missing_blobs_of_others` (how many absent files only other records name; anyone can name any id). `unreferenced_blobs` (no record names them, for example from `blob put`; they are in `blobs/` too). `not_exported_blobs` (stored here, each with its reason). `invalid_records`, and `withheld_records` (damaged copies of deleted records). |

A file is decrypted only if a record of yours or of a paired agent names it. That record must be valid, kept or in doubt. Some records this bean cannot judge: a newer format, or an identity with records from a newer bean. Such a record counts only if your current key or a recovery key from your words signed it. For a paired agent, the agent's own key must have signed it. Its author field alone proves nothing, and a key you retired is the key a thief holds. A pairing of such an identity counts by the same rule. A stranger's record, or one that is not valid, can name any file, also one you deleted. Such a file is listed in `not_exported_blobs` and not written. The export fails at the end if a file of yours is missing, or if your identity has records from a newer bean. Everything else is in the folder by then.

### `import-export <folder>`

Reads an export into a log of the same identity. Prints `added <r> records, <b> blobs, refused <p> purged`.

- It reads `manifest.json` first, one token at a time, and keeps only the identity, so a large log's export reads back. A single value over 1 MiB, or nesting deeper than 16, is refused. An export of another identity fails with `export of identity <id>, this log is <id>`, and a folder without the file is `not an export`.
- It follows the rule of `sync`: a record that a valid deletion names is refused. So an older export does not bring back what you purged since. Nor does it bring back a file that a deleted record names. The exception is a file that a live record of yours or of a paired agent names too. A stranger's record does not count.
- It does not read `invalid/`.
- It needs a log of the same identity. In a folder with no log it fails and names `restore -export`: `bean init` would make a new identity, which cannot read the export.

### `rotate [-list] [-seal-all] [-no-seal <id>]... [-no-seal-file <file>] [-nostr new|<file>]`

Changes your signing key. Reads the 24 words from stdin. Sync every copy first.

The sync can bring in records from a thief, and a seal cannot be undone, so `rotate` keeps nothing until you decide. Without `-seal-all` it shows what the old key signed and stops, before it asks for the words. It shows every deletion, correction and pairing, and every claim that takes the place of another record (`REPLACES`). It shows every record that they name, and a summary of the claims. A deletion or correction that names `A RECORD NOT IN THIS FOLDER` needs care: if you keep it, that record counts when it arrives. `-list` shows every claim too.

| Flag | Effect |
|---|---|
| `-seal-all` | Keep every record of the old key in this folder. Use it alone only if nobody else had the key. |
| `-no-seal <id>` | With `-seal-all`: do not keep this record. A deletion or correction that you do not keep is undone. A claim that you do not keep stops being valid. A wrong id is an error. |
| `-no-seal-file <file>` | The same, for a file with one id per line. |
| `-list` | Show every record of the old key. Write nothing. |

With nothing to decide on, for example on a new log, no flag is needed. Records the old key makes after the rotation are not valid. `-nostr` makes the new key a Nostr key, from a file because stdin carries the words. Above 200,000 records it also prints one `seal part <id>` line per extra seal record (C28); copy those with the rest of the log. It refuses above 200,200,000 records (`too large`). If a thief with your current key halted the log (`too new`), rotate goes past the halt. It prints one `past halt <id>` line per halting record. The rotation retires that key, so the halt ends on every copy. A halt from a newer bean of your own (signed with the 24 words) is not passed: update bean.

### `restore [-list] [-seal-all] [-no-seal <id>]... [-no-seal-file <file>] [-nostr new|<file>] [-identity <id>] [-export <folder>]`

Rebuilds `local/` on a new machine. Copy `objects/` and `blobs/` from your backup into the folder, then run it and give the 24 words on stdin. It finds your identity, writes the identity secret, and moves you to a new signing key. Your identity id and records do not change. The lost key can no longer add, delete or correct anything. Sync every copy you still have into the folder first. The flags work as for `rotate`. If more than one identity matches the words, it fails with `ambiguous` and writes nothing; pass `-identity <id>`.

`-export <folder>` rebuilds a log from an export, in an empty folder. It copies the export's records first, checked as in `import-export`, and takes the identity from `manifest.json`. Then it asks for your decision and the 24 words, as without the flag. After the restore it imports the export's files with the recovered key: `imported <n> files from the export`. Run it yourself, not through an assistant: it reads your 24 words.

### `migrate [-also <id>]... [-also-file <file>] [-nostr new|<file>] <empty folder>`, `migrate -list`

Starts a new identity from a frozen log. On any other log it refuses: run `rotate`. On a new machine, run `restore` first: it fails with `compromised`, but it gives this copy what `migrate` needs. It copies your own records that `ls` shows, signs them with the new key, and shows 24 new words once. Each record keeps the old id in `context.migrated_from`. A record that only one side of the freeze corrected moves with its old value and names the correction in `context.migrated_disputed_by`, so check those. An agent's records, a stranger's, and claims in doubt do not move: signing them would vouch for them. The command counts them, and they stay in the old log. `migrate -list` shows them. What you logged after the fork is among them: if a claim is yours, name it with `-also <id>` or `-also-file <file>`. A thief can hold the same key, so read before you name it. Nothing links the two identities by signature. Set up agents again. A later import of an old export adds its records again, because the new identity makes different dedupe refs.

## Delete

### `purge [-reason <text>] <id>`

Deletes a record about you. Writes a tombstone that every copy obeys, and erases the record here at once. `sync` and `import-export` refuse the record after that, also from an older copy or export. Its files are erased too, unless another record names them. Then `purge` prints `the file <id> is kept: <n> other records name it`, and the file is still readable. Purge those records too to erase it. It counts only records it can reach. Other files that keep it are counted on a second line. Such a file is a deleted record waiting for `gc`, a record about someone else, or a damaged file. A file under another name, such as a sync tool's conflict copy, is not a record. It keeps no file, and `purge` erases a copy of the record with it. A record of a newer format, which this bean cannot judge, can be purged too. `export` writes a kept file only if a valid record of yours or of a paired agent names it. Files are erased before the record. If purge fails in between, the record and its tombstone stay. The next command hides the record, and `gc` erases it after the hold. The tombstone does not keep the import ref of the record. `local/purged-refs.jsonl` does, so a later import of the same data adds nothing on this copy, but it does on a copy without that file. You cannot purge identity records or other tombstones. It prints `purged, tombstone <id>`.

### `gc`

Erases records that a deletion from another copy hid at least 7 days ago, and the files that only those records used. It also erases a file under another name, such as a sync tool's conflict copy. The file must be a copy of a record that it erases or that is erased already. Only a copy of your own deleted record counts as a record, and only it takes the files it names along. Any other copy is removed alone. It erases the files first, so a failure leaves the hidden record for the next `gc`. Nothing is erased without `gc` or `purge`. It refuses while your identity has records from a newer bean or is frozen, or a rotation's seal parts are missing (`incomplete seals`). It prints `erased <r> records, <b> blobs`.

## Other

### `help`, `version`

Print the help or the version.

## Importers

Importers read one source format and write records to stdout. [IMPORTERS.md](IMPORTERS.md) has the contract for writing one. They never sign and never see your keys. Run them with `bean import <source>`, which keeps the file first; run alone, they print the records for `bean append`.

### `bean-import-lab [-blob <id>] [-tz <zone>] <report.pdf | report.txt | report.html>`

Reads a lab report PDF in a layout it knows: the Labcorp patient report (`cmd/bean-import-lab/labcorp.go`) and the Quest Diagnostics report (`quest.go`). A report in any other layout is kept, encrypted, and none of its values is read: bean says `no reading was read`, and why. A file that does not look like a lab report is refused, and nothing is written. Needs `pdftotext` (`brew install poppler`), or what it makes: `-layout` text for Labcorp, `-bbox-layout` output for Quest. One record per result: type `lab.<test name>`, value and unit, and the flag, reference interval, panel, lab and comments. Your name, birth date and patient ids are not copied.

A Quest report is read by word position (`cmd/bean-import-lab/README.md`, DECISIONS 235): its trademark footer and a `Test Name  In Range  Out Of Range  Reference Range  Lab` header make it Quest. Every line must be a kind it knows, or the report is refused and the error names the page and how far down it is.

- Columns: each header sets them, from the x of its words; a word belongs to the column it starts in. Rows start at fixed indents and nest by indent. A row with a value is a test, and one without is a group heading. A bare deeper line under a heading is the rest of its name. A heading with one test under it is that test, its name wrapped. Comments start further right and belong to the test above, across pages.
- Values: In Range holds a value or `SEE NOTE:` (kept with no value and the note). Out Of Range holds a value and `H`, `L`, `HH`, `LL` or `A`. Reference Range: a range (`50-195`, `<150`, `> OR = 40`), a unit (`mg/dL`, `% of total Hgb`), and `(calc)`, which gives `context.calculated`. A range on the comment line under a test with none (`Reference Range <90`) is its range.
- Identity: the specimen, the groups and the name. The same test in two panels with two readings is two readings of one type; the same reading twice is one.
- Time: the printed US zone (`CST`, `EDT`, ...) gives the minute; any other zone gives the day. `-tz` is not needed, and one that disagrees is refused.
- A Quest report as `-layout` text is refused: give the PDF.

A Labcorp report is read by a grammar (`cmd/bean-import-lab/README.md`, DECISIONS 222 and 224). Every line must be a kind it knows, or the report is refused and the error names the line. Nothing is skipped because it looks unimportant.

- Lines: page header lines, matched whole (your name with `DOB:` and `Patient Report`, the copyright line, `All Rights Reserved ...`, `Date Created and Stored ... Page n of m`); field lines, every token a known `Label: value` (`Specimen ID`, `Date Collected`, `Date Reported`, `Fasting`, `Patient ID` and a few more); panel headings, one token at the margin; the column header, exactly `Test`, `Current Result and Flag`, `Previous Result and Date`, `Units`, `Reference Interval`, each header setting the columns of the rows under it; result rows, a name at `Test` that does not end in `:` and each other token in the column it lies in (units and ranges are centered under their names), a token across two columns refused; every token of a row or value line must be in a field of the result (a third token in the result column or a second unit is refused); a value line under a row with nothing after its name; comments, from 40 columns right of `Test`, in any shape (a line between the test column and there is refused, as it may be the rest of a wrapped name); `Please note` rows and their text. After `Disclaimer`, `Icon Legend` or `Performing Labs` only a few kinds of line may follow. They are page header and field lines, lab list lines (`01: BN - Labcorp ..., ...`), and one token of text at the margin. `Patient Details` may follow too, with lines that line up with its columns.
- Values: a number with an optional `<`, `>`, `<=`, `>=`; `1+` to `4+`; a range like `0-5`; or a titer like `1:40`. A value can also be one of a short list of results (`Negative`, `Positive`, `Non Reactive`, `Not Detected`, `Trace`, `Clear` and a few more). Flags: `H`, `L`, `A`, `High`, `Low`, `Abnormal`, `Critical`, `Panic`, `Alert`. A flag is never a value. A flag alone in the result column is refused, since the value may sit in the name. So `Abnormal` as a result is refused too. Anything else in the result column is refused, a free-text result or `1,234` too. A unit holds a `/` or `%`, or is a known bare unit (`fL`, `pg`). It starts with a letter or `%`, and holds no result word. So a range or a reference word in the units column (`9-20`, `Yellow`) is refused.
- Fields: the specimen id, collection date and time, report date and fasting are read from fields only, and every one in the report must agree. Two specimens or two collection days are refused.
- Characters: a line it reads holds printable ASCII only. A tab, a Unicode space, an invisible mark or an accented letter is refused, in a name, a value or a comment. The form feed that `pdftotext` puts at the start of a page is read. Four places may hold accents, but no control or invisible character. They are your name in its header line, the copyright line (© and ®), the ordering physician, and the patient's and physician's columns of the patient details.
- A result with no value is taken only on two conditions. The first line under it says `Test not performed`, `Not performed`, `Test cancelled` or `Cancelled` (or `canceled`). And every word of its name after the first is letters and hyphens, and not a value or a flag. So `Glucose 101`, `Lipoprotein (a)` and `Vitamin A` with no value are refused, with or without a comment.

A word of a name after the first may not look like a value or flag (`Glucose 101`, `Urine Glucose 3+`, `ANA Titer 1:160`, `HCV Ab Reactive`, `Beta 2 Microglobulin`). The exception is a numbered test: `Interleukin` or `Factor` and one or two digits, `CA 125`, `CA 15-3`, `CA 19-9`, `CA 27.29`, `CA 72-4`, `Cyfra 21-1`. A two-digit number at the end of a name is a lab code only if the report's list of performing labs names it. So `Interleukin 10` stays `lab.interleukin-10`. If the list names lab 10, it is refused. Without a list it can read whole, any number in the code column is refused: it may be the value. Two results that map to one type are refused, and so is a test whose type would be `lab.report`. The same reading listed in two panels is one result only when its value, flag, unit, range, lab and comment all match.

The shapes come from a real report's layout (`cmd/bean-import-lab/testdata/real-layout.txt`, with synthetic content). Another report may hold a line the grammar does not know yet. It is refused by line number, so try yours before you rely on it. When the report says the collection time was not on the requisition, only the day is recorded.

- `-blob`: the blob id of the PDF, from `bean blob put`. With it, the first record is a `lab.report` that keeps the PDF, and each reading has its SHA-256 in `context.report_sha256`. `bean import` passes it.
- `-tz`: the time zone where the sample was taken, for example `America/Chicago`. Without it, only the day of a Labcorp report is recorded, because the report does not say the zone. A Quest report says it.

### `bean-import-apple-health -blob <id> [-records [-types <t>,...]] <export.zip | export.xml>`

Reads an Apple Health export. By default it writes one `apple-health.export` record that keeps the file, with counts per type. With `-records` it also writes one record per reading of 16 supported types, or of the `-types` you name. Apple writes a percentage as a fraction with unit `%`, for the `*Percentage` types and walking steadiness alike. Those readings say unit `1` (0.272 is 27.2%) and keep `context.source_unit: "%"`; a value outside 0..1, or another unit, is refused. Unit `%` on any other type is refused, and so is a quantity with no unit. Blood pressure and food are Correlations: they are counted and kept in the file, and a selected reading inside one is refused. Your name, birth date, source and device names are not copied; the file keeps them. Run it as `bean import apple-health [-records] export.zip`.

### `bean-import-nhanes -blob <id> <file.XPT>`

Keeps one public NHANES file of the 2017-2018 cycle (SAS transport, a name ending in `_J`) as reference data for `bean compare`. Run it as `bean import nhanes TCHOL_J.XPT`. It writes one `reference.file` record and no readings: the file is about other people. `context` has the file's `sha256` and `bytes` and the data set name. It says whether the file is the one on record; `bean compare` uses another copy only with `-unknown-files`. It refuses a file that is not transport version 5, holds more than one data set, has no `SEQN`, or is of another cycle. It also refuses a name or label that is not plain ASCII. `bean purge` of the record erases the file.

### `bean-import-csv -type <type> [-unit <unit>] [-system <name>] [-blob <id>] [-precision <p>] <file.csv>`

`-system` names the source (default `csv`). `-precision` is the precision of the `time` column (default `second`).

Reads a CSV file with a header row. Column `value` is required. Column `time` (RFC 3339) and column `id` (the source's id for the row) are optional. Each non-empty cell of another column goes into `context` as text, under the column's name in lower case. One type per file. With `-blob` the first record is a `csv.file` that keeps the file, at most 256 MiB. Each reading has the file's SHA-256 in `context.file_sha256`, and a column of that name is refused. Run it as `bean import csv -type <type> [-unit <unit>] file.csv`.

It refuses the file, names the line, and writes nothing when:

- a row has more or fewer fields than the header. An unquoted `1,234` or `72,5` is two fields; put such a number in quotes, and it is kept as text.
- two column names are the same ignoring letter case and spaces at either end (`Value` and `value `), or a column is named `raw_time` or `unit`. The unit of every value comes from `-unit`: import the rows of each unit apart.
- two rows have the same `id`. One id names one reading. Across files, a row whose id another file already used under the same `-system` is a `changed:` line when its time differs.
- with `-blob` (so under `bean import`): a `time` that is not RFC 3339 with an offset. Also a `value` written as a number that cannot be stored exactly. Such a number has more digits than a float keeps (`0.30000000000000001`). Or it is a nonzero number that would be 0 (`1e-400`), or one too large (`1e17`, `9007199254740993`). An empty `time` is no time and is taken.

Without `-blob` it keeps its older behaviour for those two: an unread time goes to `context.raw_time`, and an inexact number stays text.

## Files in a log folder

| Path | Content | Copy it? |
|---|---|---|
| `objects/` | Signed records, plain JSON, named by their hash. | Yes. |
| `blobs/` | Stored files, encrypted, named by their hash. | Yes. |
| `local/identity` | Your identity id. | No. |
| `local/identity-secret` | The key for stored files and dedupe. | No. |
| `local/lock` | The folder lock. | No. |
| `local/signing-key` | Your signing key. | No. |
| `local/agent-key` | The agent key of this machine. | No. |
| `local/hidden/`, `local/holds.json` | Deletion state of this copy. A lost `holds.json` starts each wait again. | No. |
| `local/purged-refs.jsonl` | The import refs of purged records. Without it, a new import of the same data brings them back. | Back it up; do not sync it. |
| `local/cache/` | Caches. Safe to delete. | No. |

## Replay selected lab sources

`bean coverage -sources FILE -assets DIR -cohort ID` checks current lab readings against local, digest-pinned sources.
Add `-json` for per-reading evidence, or `-report SHA256` to select one original report.
Each percentile names its caveats by number, and the list follows the rows. A caveat of a pack and the words of an assay guide each show as a quote, with the SHA-256 of the file.
The output holds your values and record ids. An assistant runs this command only when you ask.
With `-json`, `caveats_from_pack` counts the caveats, from the first, that the pack states. bean wrote the others. `percentile_means` says what a percentile is.
A key of the selection or of a guide has exactly the letters of its name. A key in another case, or with a letter that folds to one of the name, is refused anywhere in the file.
The [reader trial](../examples/coverage/README.md) includes a source selection and five synthetic readings.
