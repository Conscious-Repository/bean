# bean

Keep your health record as a folder that you own. Every lab result, reading, meal, symptom and note is a small signed file. You can copy the folder anywhere, check it without a server, and let an AI or a doctor read it on your terms.

```
$ bean add symptom.headache 7 -unit '{score}' -note "after a salty lunch"
sha256:1b59a10ca53607dac3516c7a8c8798b34597b54f97b704075cd00217c2ecee29
$ bean ls -since 2026-10-01
2026-10-24 15:00  symptom.headache  7 {score}  # after a salty lunch   1b59a10ca536
2026-10-27        weight            81.2 kg                            31fe480169a8
2026-10-28 13:00  food              ramen, extra broth    [assistant]  f21a3641fc82
```

This repository is the reference implementation of the **bean** protocol: [CORE.md](CORE.md) in short, [SPEC.md](SPEC.md) in full. The command is `bean`.

> **Status: pre-release.** The record format is a draft. It freezes at `v` 0 after two weeks of real use with no change to [CORE.md](CORE.md). Until then, keep your source files (PDFs, exports), so that you can import them again if the format changes.

## Principles

- **You own the folder.** No account, no server, no company in the middle. Copy it, back it up, delete it.
- **Every record is a claim, not a fact.** A lab result, a watch reading and "headache 7/10" each have an author, a time and a source.
- **Nothing is edited.** A correction is a new record that replaces the old one. A deletion is a record that every copy obeys.
- **Signatures, not trust.** Every record is signed and named by its hash. `bean verify` finds any change.
- **You outlive your keys.** 24 words on paper recover your identity on a new machine and retire a lost key.
- **Leave at any time.** `bean export` writes everything as plaintext.

## Install

| You need | For | Install on macOS |
|---|---|---|
| Go 1.24 or later | Building `bean` | `brew install go` |
| `pdftotext` | The Labcorp importer only | `brew install poppler` |

Build and install the commands. The repository is private for now, so you need access to it on GitHub.

```
git clone https://github.com/Conscious-Repository/bean.git
cd bean
go install ./cmd/...
```

This puts `bean`, `bean-import-labcorp` and `bean-import-csv` in `$(go env GOPATH)/bin`, usually `~/go/bin`. If your shell cannot find `bean`, add that folder to your `PATH`:

```
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc && source ~/.zshrc
bean version
```

If the build fails with `package crypto/hkdf is not in GOROOT`, your Go is too old. Check with `go version`.

## Quick start

1. Choose a folder for your log and tell `bean` about it:

   ```
   echo 'export BEAN_DIR=~/health' >> ~/.zshrc && source ~/.zshrc
   ```

2. Create your identity:

   ```
   bean init -nostr new
   ```

   It prints your identity, your npub and 24 words. **Write the 24 words on paper now.** `bean` shows them only once. They recover your identity on a new machine. Never type them into a crypto wallet. Write your identity id (the `sha256:...` on the first line) on the same paper: it is not secret, and `restore -identity` needs it if someone puts a copy of your public commitment in the folder.

3. Add something:

   ```
   bean add weight 81.2 -unit kg
   bean add symptom.headache 7 -unit '{score}' -note "after a salty lunch"
   ```

4. Read it back and check it:

   ```
   bean ls
   bean verify
   ```

Without `-nostr new`, `init` makes an Ed25519 key instead of a Nostr key. `-nostr <file>` uses a Nostr secret key that you already have. A separate npub for your health log is the safer choice. The secret key is stored on this machine. A key that you replace can never sign this log again.

## Everyday use

Add a record with a type and a value. A number with a unit can be averaged later. Text keeps your words.

```
bean add food "2 eggs, sourdough, coffee" -note breakfast
bean add drink.water 0.5 -unit L
bean add supplement "magnesium glycinate" -note "400 mg"
bean add exercise.run 5 -unit km -at "2026-10-28 07:15"
bean add note "drank 1 L of water, gone by 5 pm" -about 1b59a1
bean add food "toast" -photo breakfast.jpg
```

- `-at` says when it happened. Without it, the time is now.
- `-about <id>` attaches a record to another one. An id can be shortened to its first 6 or more hex digits.
- `-photo` attaches an image, stored encrypted.
- The [type vocabulary](cmd/bean/vocabulary.md) lists the usual types and units, so that your records line up over time. To add types of your own, put a `vocabulary.md` in your bean folder ([Your own types](cmd/bean/vocabulary.md#your-own-types)).

Find things:

```
bean ls -type food -since 2026-10-01      # one type, from a date
bean ls -type 'lab.*'                     # every lab result; flags show as High or Low
bean show 1b59a1                          # one record, its status and its replies
bean ls -json -type weight                # records as stored, for jq or a script
```

Correct or delete:

```
bean add weight 81.2 -unit kg -corrects 31fe48    # replaces your record 31fe48
bean purge 1b59a1                                 # deletes record 1b59a1 from every copy
```

## Import your data

### Blood work (Labcorp, Superpower)

The Labcorp importer supports the patient-report layout used by Labcorp and
some Superpower reports. Preserve the original report and verify storage readback
using an existing private Bean log:

```
go build -o /tmp/bean ./cmd/bean
go build -o /tmp/bean-import-labcorp ./cmd/bean-import-labcorp
BEAN_BIN=/tmp/bean LABCORP_BIN=/tmp/bean-import-labcorp \
  python3 tools/labcorp/ingest.py lab-result.pdf /private/bean-log --timezone America/Chicago
```

This adds one `lab.report` archive plus individual results. Identical input retries
without duplicates. An amended report is kept as a new archive, but readings with
an existing specimen/test identity dedupe: **amendments require explicit correction**,
not an assumption that import replaced prior values. See the
[Labcorp importer notes](cmd/bean-import-labcorp/README.md).

Results retain values, units, flags, reference intervals, panels and comments.
Recognized name, birth-date and patient-ID headers are excluded; preserved medical
comments can still contain identifiers. `--timezone` is where the sample was taken;
without it, only the collection day is recorded. PDF extraction requires Poppler.

### A CSV file

```
BLOB=$(bean blob put export.csv)
bean-import-csv -type hrv.rmssd -unit ms -blob $BLOB export.csv | bean append
```

Keep the raw file first: then a better importer can read it again later. Without `-blob` the importer warns.

The CSV needs a header row and a `value` column. A `time` column (RFC 3339) and an `id` column are optional. Other columns are kept as context.

### Coming next

Apple Health. Each importer is written against a real export file. See [docs/ROADMAP.md](docs/ROADMAP.md).

## Log from an AI assistant

`bean mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server. Any AI client that supports MCP can use it to read your log and add to it. The server runs this command:

```
bean -dir ~/health mcp -write -label assistant
```

Add it to your client as an MCP server that uses stdio. Most clients take a JSON entry of this form:

```json
"bean": {"command": "/Users/you/go/bin/bean", "args": ["-dir", "/Users/you/health", "mcp", "-write", "-label", "assistant"]}
```

Then tell the assistant what happened, for example "had ramen with extra broth around 1" or "took 400 mg magnesium". You can also ask questions: "is my resting heart rate going up this month?" The assistant logs your words with the correct type and time. It puts a calorie or sodium estimate in a separate record that points to the meal. Your words do not change. Its records show as `[assistant]` in `bean ls`. Use a different `-label` for each assistant. The assistant can log only the types in the vocabulary and in your own `vocabulary.md`, if you have one.

Without `-write`, the server can only read. With `-write`, the assistant signs with its own **agent key**, not your key. An agent can add records about you. It cannot correct or delete your records, and it cannot sign as you. The server sends the records it returns to the AI provider. Use a local model if that is a problem for you.

## More than one machine

Only `objects/` and `blobs/` are the log. `local/` holds the keys of one machine: never sync it.

### Keep two copies the same

On one machine, with the other copy mounted or copied to a folder:

```
bean sync /path/to/other/copy
```

Over the network, copy with rsync, then check:

```
rsync -a ~/health/objects ~/health/blobs server:health/
ssh server '~/go/bin/bean -dir ~/health verify'
rsync -a server:health/objects server:health/blobs ~/health/
bean verify
```

Sync only from your own copies: `bean sync` copies every record file of the source. Never use `rsync --delete`: the log only grows, and deletions travel as records. Syncthing also works if you share `objects/` and `blobs/` only. Run `bean verify` after a copy made with another tool; it applies deletions that came with the copy.

Each machine needs its own folder. Do not point two machines at one shared or network folder: the folder lock works only inside one machine ([SECURITY.md](SECURITY.md)).

### Run an agent on another machine

For example, an assistant on a cloud server. The server gets a copy of the log and its own agent key, but not your key and not your 24 words.

1. Copy `objects/` and `blobs/` to the server, as above.
2. On the server, make the agent key:

   ```
   bean -dir ~/health agent init -label server
   ```

   It prints the agent's npub and the command for the next step.
3. On your own machine, trust the agent:

   ```
   bean agent pair npub1... -label server
   ```

4. The agent logs with `bean add -agent ...`, `bean append -agent`, or `bean mcp -write`. Sync both ways as above. Its records show as `[server]`.

Anyone who can read the server's folder can read your records: they are plain JSON. Photos and other stored files stay encrypted unless you also copy `local/identity-secret` to the server.

### Backups and a lost laptop

Any copy of `objects/` and `blobs/` is a backup: Time Machine, a drive, another machine. The 24 words rebuild the keys in `local/`. Also back up `local/purged-refs.jsonl`: it is the only record of which imports you deleted. Without it, importing the same source data again brings deleted readings back ([SECURITY.md](SECURITY.md)).

If you lose the laptop:

1. Install `bean` on the new machine.
2. Copy every backup and copy that you still have into one folder:

   ```
   mkdir -p ~/health && cp -R /Volumes/Backup/health/objects /Volumes/Backup/health/blobs ~/health/
   ```

3. Rebuild `local/` and move to a new signing key:

   ```
   bean -dir ~/health restore -nostr new
   ```

   Type or paste the 24 words, then press Ctrl-D.

4. Put back your backup of the deleted-imports file:

   ```
   cp /Volumes/Backup/health/local/purged-refs.jsonl ~/health/local/
   ```

Your identity and records stay the same. The lost key can no longer add, correct or delete anything: a record it signs after the rotation is not valid. Records of the old key that are in this folder when you restore stay valid, because the rotation seals them. Use `bean rotate` the same way on a machine that you still have, for example when a key is old.

If `restore` says `ambiguous`, the folder holds more than one identity made from these words' public commitment (someone can copy it). Pass your identity id: `bean -dir ~/health restore -identity sha256:...`. Your identity id is on the first line that `init` printed, in any export's `manifest.json`, and in `bean whoami` on an old machine.

Sync every copy before you rotate or restore. Records that a copy made with the old key and did not sync before the rotation stop being valid. A log of more than 200,000 records per key rotates into a succession plus `seal part` records; copy all of them. Until every part is in a copy, `bean verify` there says `incomplete seals` and the old key's records do not count, but nothing is deleted. Never rotate or restore on two machines from the same point: that freezes the identity, and you must make a new one.

### Upgrading

Read [UPGRADING.md](UPGRADING.md) before you run a new build on your log. It lists the changes that alter results, and how to try a new build on a copy first.

## How it works

A record is a JSON file in `objects/`, named by the SHA-256 hash of its content and signed with your key. Your identity is the hash of your first record, so it never changes when keys change. Corrections and deletions are new records. The protocol says who may make them, so every copy reaches the same result from the same files. Stored files (PDFs, photos) are encrypted in `blobs/`. Sync copies the files that one copy has and the other does not.

Every record is checked when it is first read, then cached, the same way git trusts its object store. `bean verify` checks everything again. At 20,000 records, `add` and an AI query take about 0.2 s and `verify` about 1 s. At 230,000 records on one machine (measured 2026-10-30), `add` takes about 7 s, `verify` about 11 s, `rotate` about 9 s, and each needs about 2.5 GB of memory. Every write goes to disk before it counts.

[CORE.md](CORE.md) is a short summary of the protocol. [SPEC.md](SPEC.md) has every rule: enough to write a second implementation.

## Security

bean protects who wrote what, and protects your identity against a stolen device. It does not hide your records from someone who can read the folder: keep the log on an encrypted disk. [SECURITY.md](SECURITY.md) lists what is and is not protected, where each key lives, and how to report a problem.

## Documentation

| Document | For |
|---|---|
| [docs/COMMANDS.md](docs/COMMANDS.md) | Every command, flag and exit code. |
| [cmd/bean/vocabulary.md](cmd/bean/vocabulary.md) | The usual record types and units. |
| [SECURITY.md](SECURITY.md) | What is protected, and the keys. |
| [CORE.md](CORE.md) | The protocol, in short. |
| [UPGRADING.md](UPGRADING.md) | Changes that alter results for an existing log. |
| [SPEC.md](SPEC.md) | The full protocol: every rule, error and conformance test. |
| [docs/ROADMAP.md](docs/ROADMAP.md) | What is done and what comes next. |
| [docs/DECISIONS.md](docs/DECISIONS.md) | Every design decision, numbered, with the reason. |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Build, test and change the code. |

## License

The specification text is CC0 1.0 ([LICENSE-SPEC](LICENSE-SPEC)). The code is Apache-2.0 ([LICENSE](LICENSE)).
