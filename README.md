# bean

bean keeps your health record as a folder that you own. Every lab result, reading, meal, symptom and note is a small signed file. You can copy the folder anywhere, check it without a server, and let an AI assistant or a doctor read it on your terms.

```
$ bean add symptom.headache 7 -unit '{score}' -note "after a salty lunch"
sha256:1b59a10ca53607dac3516c7a8c8798b34597b54f97b704075cd00217c2ecee29
$ bean ls -since 2026-10-01
2026-10-24 15:00  symptom.headache  7 {score}  # after a salty lunch   1b59a10ca536
2026-10-27        weight            81.2 kg                            31fe480169a8
2026-10-28 13:00  food              ramen, extra broth    [assistant]  f21a3641fc82
```

This repository is the reference implementation of the **bean** protocol. [CORE.md](CORE.md) summarizes the protocol, and [SPEC.md](SPEC.md) defines it in full. The command is `bean`.

> **Status: pre-release.** The record format is a draft. It will be frozen at version 0 after two weeks of real use with no change to [CORE.md](CORE.md). Until then, keep your source files (PDFs and exports), so that you can import them again if the format changes.

**New here?** Follow [docs/ONBOARDING.md](docs/ONBOARDING.md). It takes you from installation to an imported lab report, an optional AI assistant, a backup, and a tested way to leave, and it includes rules for an AI assistant that helps you set up.

## Principles

- **You own the folder.** There is no account, no server, and no company in the middle. You can copy the folder, back it up, or delete it.
- **Every record is a claim, not a fact.** A lab result, a watch reading and "headache 7/10" each have an author, a time and a source.
- **Nothing is edited.** A correction is a new record that replaces the old one. A deletion is a record that every copy obeys.
- **Signatures, not trust.** Every record is signed and named by the hash of its content. `bean verify` detects any change.
- **You outlive your keys.** 24 words on paper recover your identity on a new machine and retire a lost key.
- **You can leave at any time.** `bean export` writes everything as plain files, and `bean restore -export` turns an export back into a log.

## Install

| You need | For | Install on macOS |
|---|---|---|
| Go 1.24 or later | Building bean | `brew install go` |
| `pdftotext` | The Labcorp importer | `brew install poppler` |

The repository is private for now, so you need access to it on GitHub.

```bash
git clone https://github.com/Conscious-Repository/bean.git
cd bean
go install ./cmd/...
```

This installs `bean` and its importers (`bean-import-labcorp`, `bean-import-apple-health` and `bean-import-csv`) in `$(go env GOPATH)/bin`, which is usually `~/go/bin`. If your shell cannot find `bean`, add that folder to your `PATH`:

```bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc && source ~/.zshrc
bean version
```

If the build fails with `package crypto/hkdf is not in GOROOT`, your Go is too old. Check it with `go version`.

## Quick start

1. Choose a folder for your log and tell bean about it:

   ```bash
   echo 'export BEAN_DIR=~/health' >> ~/.zshrc && source ~/.zshrc
   ```

2. Create your identity. Run this yourself, in a terminal window that no AI assistant is connected to, because it prints your recovery words:

   ```bash
   bean init -nostr new
   ```

   It prints your identity id, your npub, and 24 words, and it tells you where your log is. **Write the 24 words on paper now.** bean shows them only once, and they are the only way to recover your identity on a new machine. Never type them into a chat, an AI assistant or a crypto wallet. Write your identity id (the `sha256:...` on the first line) on the same paper: it is not secret, and `restore -identity` needs it if a folder ever holds more than one identity made from your words.

3. Add something:

   ```bash
   bean add weight 81.2 -unit kg
   bean add symptom.headache 7 -unit '{score}' -note "after a salty lunch"
   ```

4. Read it back and check it:

   ```bash
   bean ls
   bean verify
   ```

   The `verify` summary also counts the records that make up your identity, so it is higher than the number of records you added.

Without `-nostr new`, `init` creates an Ed25519 key instead of a Nostr key. `-nostr <file>` uses a Nostr secret key that you already have, but a separate npub for your health log is the safer choice. The secret key is stored on this machine. A key that you replace can never sign this log again.

## Everyday use

Add a record with a type and a value. A number with a unit can be averaged later; text keeps your own words.

```bash
bean add food "2 eggs, sourdough, coffee" -note breakfast
bean add drink.water 0.5 -unit L
bean add supplement "magnesium glycinate" -note "400 mg"
bean add exercise.run 5 -unit km -at "2026-10-28 07:15"
bean add note "drank 1 L of water, gone by 5 pm" -about 1b59a1
bean add food "toast" -photo breakfast.jpg
```

- `-at` sets when it happened. Without it, the time is now.
- `-about <id>` attaches a record to another one. Any id can be shortened to its first 6 or more hex digits.
- `-photo` attaches an image, which is stored encrypted.
- The [type vocabulary](cmd/bean/vocabulary.md) lists the usual types and units, so that your records line up over time. To add types of your own, put a `vocabulary.md` in your log folder ([Your own types](cmd/bean/vocabulary.md#your-own-types)).

Find things:

```bash
bean ls -type food -since 2026-10-01      # one type, from a date
bean ls -type 'lab.*'                     # every lab result; flags show as High or Low
bean show 1b59a1                          # one record, its status and its replies
bean ls -json -type weight                # records as stored, for jq or a script
```

Correct or delete:

```bash
bean add weight 81.2 -unit kg -corrects 31fe48    # replaces your record 31fe48
bean purge 1b59a1                                 # deletes record 1b59a1 from every copy
```

## Import your data

### Blood work (Labcorp, Superpower)

Superpower and Labcorp give you a Labcorp patient report as a PDF. Import it:

```bash
bean import labcorp -tz America/Chicago lab-result.pdf
bean ls -type 'lab.*'
```

bean first stores the PDF, encrypted, as a `lab.report` record, and checks that it reads back unchanged. It then adds one record for each result, with its value, unit, flag, reference interval, panel and the lab's comments. Your name, birth date and patient ids are not copied into the results; the stored PDF keeps everything.

- `-tz` is the time zone where the sample was taken, as a zone name such as `America/New_York`, `America/Chicago`, `America/Denver` or `America/Los_Angeles`. Abbreviations such as `CST` do not work. Without `-tz`, only the day is recorded. If the report says that the collection time was not given, only the day is recorded as well.
- The first line of the output names the record that keeps the PDF. To erase the PDF, `bean purge` that record id.
- Importing the same report again adds nothing. If a later report changes a value, for example when a final result replaces `Pending`, bean stores the new PDF, lists the changed readings, writes nothing for them, and exits with status 1. If the new report is right, run the import again with `-corrections`.
- Compare the imported values with the PDF. The importer reads the PDF's text layout and refuses any line it cannot read with certainty; when it refuses, nothing is written and the error names the line. The error can contain one of your values, so do not paste it publicly.
- A text file made with `pdftotext -layout report.pdf report.txt` works the same way as the PDF.

Each result names the PDF by its SHA-256 rather than as a stored file, so purging the `lab.report` erases the PDF. Results from importers older than 0.2.0 still name the PDF as a stored file; purge those as well to erase it. See [the Labcorp importer notes](cmd/bean-import-labcorp/README.md) for how a report is read.

### Apple Health

On your iPhone, open Health, tap your picture, and choose Export All Health Data. Then:

```bash
bean import apple-health export.zip              # keeps the export, encrypted
bean import apple-health -records export.zip     # also adds one record per reading
```

The export has no sample ids, so bean cannot tell a changed reading in a later export from a new one. Nothing is summed per day. Apple writes percentages as fractions, so walking double support of 0.272 is stored with unit `1` and means 27.2%. Blood pressure and food readings are stored in Apple's Correlations; they stay in the stored file and are not read. Purging the `apple-health.export` record erases the file. See [the Apple Health importer notes](cmd/bean-import-apple-health/README.md).

### A CSV file

```bash
bean import csv -type hrv.rmssd -unit ms export.csv
```

The CSV needs a header row and a `value` column. A `time` column (RFC 3339) and an `id` column are optional, and other columns are kept as context. bean stores the file before it adds the readings, so that a better importer can read it again later. The Importers section of [docs/COMMANDS.md](docs/COMMANDS.md) lists what the CSV importer refuses.

## Log from an AI assistant

`bean mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server. Any AI client that supports MCP servers over stdio can use it to read your log, and optionally to add to it.

| Command | The assistant can |
|---|---|
| `bean mcp` | List your record types, query and summarize your records, and read one record in full. |
| `bean mcp -write -label assistant` | Also log entries, signed with this machine's agent key. |
| `bean mcp -blobs` | Also read the text of stored text files, such as a lab report imported as a `.txt`, which includes your name and birth date. For a PDF it gets only the file's size. |

Start read-only. AI clients do not read your shell profile, so use absolute paths rather than `~` or `BEAN_DIR`. With Claude Code (`-s user` makes it available in every folder):

```bash
claude mcp add -s user bean -- "$(which bean)" -dir "$HOME/health" mcp
```

With Claude Desktop, add this to `~/Library/Application Support/Claude/claude_desktop_config.json` and restart the app (use the output of `which bean` and your own home folder):

```json
{"mcpServers": {"bean": {"command": "/Users/you/go/bin/bean", "args": ["-dir", "/Users/you/health", "mcp"]}}}
```

Other clients take the same command and arguments as a stdio server. [docs/ONBOARDING.md](docs/ONBOARDING.md#5-connect-an-ai-assistant-optional) shows how to check that the server works.

With `-write`, tell the assistant what happened, for example "had ramen with extra broth around 1" or "took 400 mg magnesium", or ask it questions such as "is my resting heart rate going up this month?" The assistant logs your words with the correct type and time, and puts a calorie or sodium estimate in a separate record that points to the meal, so your words do not change. Its records show as `[assistant]` in `bean ls`.

The assistant signs with this machine's **agent key**, not your key. It can add records about you, but it cannot correct or delete your records, sign as you, or write lab results or other imported readings: those come only from an importer that keeps the source file. It can log only the types in the vocabulary and in your own `vocabulary.md`. One machine has one agent key, so every assistant on it signs as the same agent under one label; choose a label that fits them all, such as `assistant`. bean refuses a second label for the same key, because it would show one assistant's records under another's name. To tell two assistants apart, run one of them on another machine ([below](#run-an-agent-on-another-machine)).

Whatever the server returns is sent to the AI provider. If that is a problem for you, use an MCP client with a local model.

## More than one machine

Only `objects/` and `blobs/` are the log. `local/` holds the keys of one machine; never sync it.

### Keep two copies the same

On one machine, with the other copy mounted or copied to a folder:

```bash
bean sync /path/to/other/copy
```

Over the network, copy with rsync and then check:

```bash
rsync -a ~/health/objects ~/health/blobs server:health/
ssh server '~/go/bin/bean -dir ~/health verify'
rsync -a server:health/objects server:health/blobs ~/health/
bean verify
```

Sync only from your own copies: `bean sync` copies every record file of the source. Never use `rsync --delete`: the log only grows, and deletions travel as records. Syncthing also works if you share only `objects/` and `blobs/`. Run `bean verify` after a copy made with another tool; it applies the deletions that came with the copy.

Each machine needs its own folder. Do not point two machines at one shared or network folder, because the folder lock works only within one machine ([SECURITY.md](SECURITY.md)).

### Run an agent on another machine

For example, an assistant on a cloud server. The server gets a copy of the log and its own agent key, but not your key and not your 24 words.

1. Copy `objects/` and `blobs/` to the server, as above.
2. On the server, create the agent key:

   ```bash
   bean -dir ~/health agent init -label server
   ```

   It prints the agent's npub and the command for the next step.
3. On your own machine, trust the agent:

   ```bash
   bean agent pair npub1... -label server
   ```

4. The agent logs with `bean add -agent ...`, `bean append -agent`, or `bean mcp -write`. Sync both ways as above. Its records show as `[server]`.

Anyone who can read the server's folder can read your records, because they are plain JSON. Photos and other stored files stay encrypted unless you also copy `local/identity-secret` to the server.

### Backups and a lost laptop

Any copy of `objects/` and `blobs/` is a backup: Time Machine, a drive, or another machine. The 24 words rebuild the keys in `local/`. Also back up `local/purged-refs.jsonl`: it is the only record of which imports you deleted, and without it, importing the same source data again brings deleted readings back ([SECURITY.md](SECURITY.md)).

A backup and an export are different things. A backup is a copy of the log itself. An export is plain files for leaving bean; `bean restore -export` can turn one back into a log ([docs/ONBOARDING.md](docs/ONBOARDING.md#7-leave-and-come-back)).

If you lose your laptop:

1. Install bean on the new machine.
2. Copy every backup and copy that you still have into one folder:

   ```bash
   mkdir -p ~/health && cp -R /Volumes/Backup/health/objects /Volumes/Backup/health/blobs ~/health/
   ```

3. Rebuild `local/` and move to a new signing key. Run this yourself, because it reads your recovery words:

   ```bash
   bean -dir ~/health restore -nostr new
   ```

   It shows what the old key signed and stops. Read the list. **If nobody else had the old key,** run it again with `-seal-all`, then type the 24 words and press Ctrl-D. Clear the terminal and its scrollback afterwards (Cmd-K in Terminal.app). **If the key may have been stolen,** add `-no-seal <id>` for each record that is not yours, or `-no-seal-file <file>` for many. `-list` shows every record. A record that you keep stays valid permanently, and you cannot undo a deletion that you keep.

4. Put back your backup of the deleted-imports file:

   ```bash
   cp /Volumes/Backup/health/local/purged-refs.jsonl ~/health/local/
   ```

Your identity and records stay the same. The lost key can no longer add, correct or delete anything: a record it signs after the rotation is not valid. Records of the old key that you keep stay valid, because the rotation seals them. Use `bean rotate` in the same way on a machine that you still have, for example when a key is old.

If `restore` reports `ambiguous`, the folder holds more than one identity made from these words' public commitment (anyone can copy that commitment into a folder). Pass your identity id: `bean -dir ~/health restore -identity sha256:...`. Your identity id is on the first line that `init` printed, in any export's `manifest.json`, and in the output of `bean whoami` on an old machine.

Sync every copy before you rotate or restore, and then check what the sync brought in. `bean rotate` works in the same way as `restore`: without `-seal-all` it only shows the list. Records that a copy made with the old key, and did not sync before the rotation, stop being valid. A log of more than 200,000 records per key rotates into a succession plus `seal part` records; copy all of them. Until every part is in a copy, `bean verify` there reports `incomplete seals` and the old key's records do not count, but nothing is deleted.

**Never rotate or restore on two machines from the same point.** That freezes the identity permanently; `bean migrate <empty folder>` then starts a new identity with your own records.

### Upgrading

Read [UPGRADING.md](UPGRADING.md) before you run a new build on your log. It lists the changes that alter results, and explains how to try a new build on a copy first.

## How it works

A record is a JSON file in `objects/`, named by the SHA-256 hash of its content and signed with your key. Your identity is the hash of your first record, so it does not change when your keys change. Corrections and deletions are new records. The protocol defines who may make them, so every copy reaches the same result from the same files. Stored files, such as PDFs and photos, are encrypted in `blobs/`. Sync copies the files that one copy has and the other does not.

Each record is checked when it is first read and then cached, in the same way that git trusts its object store. `bean verify` checks everything again. At 20,000 records, `add` and an AI query take about 0.2 s and `verify` takes about 1 s. At 230,000 records on one machine (measured on 2026-10-30), `add` takes about 7 s, `verify` about 11 s and `rotate` about 9 s, and each needs about 2.5 GB of memory. Every write reaches the disk before it counts.

[CORE.md](CORE.md) is a short summary of the protocol. [SPEC.md](SPEC.md) has every rule, in enough detail to write a second implementation.

## Security

bean protects who wrote what, and protects your identity against a stolen device. It does not hide your records from someone who can read the folder, so keep the log on an encrypted disk. [SECURITY.md](SECURITY.md) lists what is and is not protected, where each key lives, and how to report a problem.

## Documentation

| Document | For |
|---|---|
| [docs/ONBOARDING.md](docs/ONBOARDING.md) | Getting started, step by step, alone or with an AI assistant. |
| [docs/COMMANDS.md](docs/COMMANDS.md) | Every command, flag and exit code. |
| [cmd/bean/vocabulary.md](cmd/bean/vocabulary.md) | The usual record types and units. |
| [SECURITY.md](SECURITY.md) | What is protected, and the keys. |
| [CORE.md](CORE.md) | The protocol, in short. |
| [UPGRADING.md](UPGRADING.md) | Changes that alter results for an existing log. |
| [SPEC.md](SPEC.md) | The full protocol: every rule, error and conformance test. |
| [docs/ROADMAP.md](docs/ROADMAP.md) | What is done and what comes next. |
| [docs/DECISIONS.md](docs/DECISIONS.md) | Every design decision, numbered, with the reason. |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Building, testing and changing the code. |

## License

The specification text is CC0 1.0 ([LICENSE-SPEC](LICENSE-SPEC)). The code is Apache-2.0 ([LICENSE](LICENSE)).
