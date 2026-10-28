# bean

Your health record as a folder you own. Every reading, lab result, symptom and note is a small signed JSON file. You can copy the folder anywhere, check it without any server, and hand it to an AI or a doctor on your terms.

`bean` is the reference implementation of the **hlog core specification** ([SPEC.md](SPEC.md), v0.5 draft). The command is `hlog`. Both names are placeholders.

> **Status: pre-release, private.** The spec is a draft awaiting review. The record format may change before v1, so keep your source files (PDFs, exports) and treat your first log as provisional. See [ROADMAP.md](ROADMAP.md).

## The idea in five lines

1. **A record is a claim, not a fact.** A Labcorp result, a watch reading and "headache 8/10" are all claims with an author, a time and a source.
2. **Every record is signed and named by its hash.** Change one byte and verification fails.
3. **The log is a folder.** Sync is copying the files a copy is missing. No server, no account, no conflicts.
4. **Nothing is edited.** A correction is a new record that supersedes the old one. A deletion is a tombstone that every copy obeys.
5. **You are an identity that outlives your keys.** 24 words on paper recover you on a new machine and retire a lost key.

## Install

Needs Go 1.24 or later. The Labcorp importer also needs `pdftotext` (`brew install poppler`).

```
git clone <this repo> bean && cd bean
go build -o ~/bin/ ./cmd/...        # hlog, hlog-import-csv, hlog-import-labcorp
```

## Quick start

```
hlog -dir ~/health init             # prints your identity and 24 words, once
export HLOG_DIR=~/health
```

To sign with a Nostr key instead, use `init -nostr new` (a new npub just for this log) or `init -nostr <file>` (an nsec you already have). `hlog whoami` prints the npub. A dedicated npub is the safer choice for now: the nsec sits in `local/`, and once a key is rotated out it can never sign this log again.

Write the 24 words on paper. They are the only way to recover the identity on a new machine. They are for hlog only: never type them into a crypto wallet.

Add things by hand:

```
hlog add symptom.headache 8 -unit '{score}' -note "after salty lunch"   # now
hlog add weight 81.2 -unit kg -at 2026-10-27
hlog add note "drank 1L water, gone by 5pm" -about <id of the headache>
```

Import a blood panel. Superpower and Labcorp hand out Labcorp patient report PDFs:

```
BLOB=$(hlog blob put lab-result.pdf)          # the PDF itself, encrypted
hlog-import-labcorp -tz America/Chicago -blob $BLOB lab-result.pdf | hlog append
```

Read it back:

```
hlog ls                                      # everything current, in time order
hlog ls -type 'lab.*'                        # flags show as High / Low
hlog ls -type weight -since 2026-10-01 -json # stored bytes, for jq or a script
hlog show 96d4ef                             # one record, its status and replies
hlog verify                                  # re-prove every file: the fsck
```

## Log from chat, through your agents

The point is one canonical log of what you do, so capture has to be as cheap as sending a message. Your AI agents write to the log with their own keys, never yours:

```
claude mcp add hlog -- hlog -dir ~/health mcp -write -label claude
```

Then tell Claude "had ramen with extra broth around 1" or "took 400 mg magnesium". It logs the entry with `log_entry`, following the [type vocabulary](cmd/hlog/vocabulary.md), and puts any calorie or sodium estimate in a separate `food.estimate` that points at the meal. In `hlog ls` its entries show as `[claude]`.

An agent on another machine, for example Hermes on a cloud server, gets a copy of the log without your key:

```
# on the cloud machine, after syncing objects/ and blobs/ there
hlog -dir ~/health agent init -label hermes          # prints Hermes's npub
hlog -dir ~/health add -agent food "ramen, extra broth" -at "2026-10-28 13:00"

# on your machine
hlog agent pair npub1... -label hermes                # you vouch for that key
```

An agent can add entries. It cannot correct or delete yours, and it cannot sign as you. Record files are plain JSON, so any machine with a copy can read the whole log. Attaching or reading photos there needs your identity secret, which you copy over only if you choose to.

`hlog add -agent` signs a command-line entry with this machine's agent key. `hlog add -photo meal.jpg` attaches an image, stored encrypted.

## Ask an AI about your log

`hlog mcp` is a read-only [Model Context Protocol](https://modelcontextprotocol.io) server on stdin and stdout.

```
claude mcp add hlog -- hlog -dir ~/health mcp
```

For Claude Desktop, add this under `mcpServers`:

```json
"hlog": {"command": "/Users/you/bin/hlog", "args": ["-dir", "/Users/you/health", "mcp"]}
```

| Tool | What it returns |
|---|---|
| `list_types` | Every type with its count, units and date range. |
| `query_records` | Current records by type and time range. |
| `stats` | Count, min, max, mean, median and spread, optionally by day, week or month. |
| `get_record` | One record with its status and the records that reply to it. |
| `read_blob` | The original source file, if it is text. |

Without `-write` the tools cannot write. Whatever they return goes to the model provider. Use a local model if that matters to you.

## Backups and a lost laptop

| Folder | What it holds | Back it up? |
|---|---|---|
| `objects/` | Signed records, plain JSON. | Yes, anywhere. |
| `blobs/` | Source files, encrypted. | Yes, anywhere. |
| `local/` | Your keys and identity secret. | **No.** Never sync it. The paper words rebuild it. |

To keep a second copy, run `hlog sync <other folder>` both ways, or use rsync or Syncthing on `objects/` and `blobs/` and then run `hlog verify`.

If the laptop is lost, copy the backup to the new machine, then:

```
hlog -dir ~/health restore        # paste the 24 words
```

`restore` rebuilds `local/` and moves you to a new signing key. Your identity id and every record stay the same. The lost key can no longer delete or correct anything. Before you restore, sync every copy you still have into the folder.

`hlog rotate` does the same switch on a machine you still have, for example when a key is old.

## Commands

| Command | Does |
|---|---|
| `init` | Create an identity. Prints the 24 words once. `-nostr` signs with a Nostr key. |
| `whoami` | Identity, signing key, npub and agent key. |
| `add <type> <value>` | Write a claim by hand. `-unit`, `-at`, `-note`, `-about`, `-photo`, `-agent`. |
| `agent init`, `agent pair <npub>` | Make this machine's agent key; vouch for an agent from your machine. |
| `append` | Read JSONL records from an importer on stdin, sign and store them. Re-imports add nothing. `-agent` signs with the agent key. |
| `ls`, `show <id>` | Read the log. Any 6+ hex prefix works as an id. |
| `blob put <file>`, `blob get <id>` | Store or read an encrypted source file. |
| `verify` | Check the whole folder. Exit 0 is clean. |
| `purge <id>` | Delete a record everywhere it syncs. The bytes are erased here at once. |
| `gc` | Erase records hidden by someone else's purge after 7 days. |
| `sync <dir>` | Copy what the other folder has and this one lacks. |
| `rotate`, `restore` | New signing key from the paper words. `-nostr new` or `-nostr <nsec file>` makes it a Nostr key. |
| `export <dir>`, `import-export <dir>` | A plaintext copy of everything, and back. |
| `mcp` | The AI server. Read-only unless `-write`. |

Run `hlog` with no arguments for flags. `-dir` defaults to `$HLOG_DIR`, then the current folder.

## Importers

An importer reads one source format and writes JSONL to stdout. It never signs; `hlog append` does.

| Importer | Source |
|---|---|
| `hlog-import-labcorp` | Labcorp patient report PDF (Superpower panels). One record per result, typed `lab.<test name>`, with flag, reference interval and the lab's comments. Your name, birth date and patient ids are not copied. |
| `hlog-import-csv` | A CSV with a `value` column and optional `time` and `id` columns. One type per file. |

Next: Apple Health. Each importer is written against a real export, never from memory.

## Speed

Every record is named by its hash and never edited, so, like git, ordinary commands trust a file that has not changed since this machine last proved it, and only `hlog verify` re-proves everything. At 20,000 records `add` and an AI's query take about 0.2 s, `ls` 0.4 s and `verify` about 1 s. Writes are flushed to disk before they count.

## Privacy and security

- Record files are plain JSON. Anyone with the folder can read types, times and values. Keep it on an encrypted disk (FileVault).
- Source files in `blobs/` are encrypted with a key derived from your 24 words.
- `local/` holds the signing key and the identity secret, mode 0600.
- `hlog export` writes plaintext and says so first.
- A stolen key can add claims but cannot take over the identity: only the paper words can rotate.
- A Nostr signing key signs a small Nostr event whose content is the record id (kind 29944, not stored by relays). Remote signers such as Amber can sign that shape later without a format change.

## Documents

| File | For |
|---|---|
| [CORE.md](CORE.md) | The protocol in two pages: what a second implementation must agree on. Start here. |
| [SPEC.md](SPEC.md) | The full protocol (v0.5 draft): every rule, error string and conformance test. |
| [ROADMAP.md](ROADMAP.md) | The path to v0.1 and what is decided. |
| [OPEN_QUESTIONS.md](OPEN_QUESTIONS.md) | Every choice made where the spec was silent, numbered. |
| [HANDOFF.md](HANDOFF.md) | Where things stand, for the next person or agent. |
| [cmd/hlog/vocabulary.md](cmd/hlog/vocabulary.md) | Which types to use for food, drinks, supplements, exercise, sleep and symptoms. |

## Development

Standard library plus one dependency, `btcec/v2`, for secp256k1 (Nostr) signatures.

```
go test -race ./...                 # unit, conformance, RFC vector and concurrency tests
go vet ./... && gofmt -l .
./demo.sh                           # two-folder walkthrough: import, sync, purge, gc, export
go run ./tools/genvectors           # regenerate vectors/
python3 -I tools/checkvectors.py    # independent check (needs cryptography with AES-SIV)
python3 -I tools/checksecp256k1.py  # independent BIP-340 and npub check, pure Python
```

`core/` is the protocol: canonical JSON, signatures, the verifier, the folder. It knows no data source. `cmd/` holds the CLI and the importers. Test data is synthetic. Never commit real health data, keys or paper words.

## License

The specification text is CC0 1.0 ([LICENSE-SPEC](LICENSE-SPEC)). The code is Apache-2.0 ([LICENSE](LICENSE)).
