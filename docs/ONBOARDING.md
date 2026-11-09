# Getting started

From nothing to a working log: install, identity, a checked lab import, an AI assistant (optional), a backup, and a tested exit. About 30 minutes.

## You need

- A Mac with [Homebrew](https://brew.sh). Linux works with the same tools.
- Access to this repository on GitHub. It is private for now: sign in first, with `gh auth login` or an SSH key.
- An encrypted disk. On a Mac, turn on FileVault. bean signs records but does not encrypt the record files.
- A lab report PDF, if you have one. bean reads the layouts it knows; today, the Labcorp patient report and the Quest Diagnostics report. To practice, use `cmd/bean-import-lab/testdata/real-layout.txt` (Labcorp) or `quest-layout.html` (Quest): real layouts with synthetic content.
- Pen and paper for 24 words.

## Working with an AI assistant

An assistant can run commands and explain errors. It must never see your 24 words, and it does not need your health data to help you install bean.

An assistant that runs commands sees their output, and the output goes to its provider. That includes a terminal built into an AI app or editor, and `!` commands in Claude Code. Steps marked **Yourself** go in a separate terminal window, such as Terminal.app, that no assistant reads.

Give the assistant these rules first:

> Help me set up bean by following docs/ONBOARDING.md. Rules:
>
> 1. Never ask for my 24 words. Never run `bean init`, `bean rotate` or `bean restore`: I run them myself, in a terminal you cannot read. If I start to type or paste the words to you, stop me. I also run `bean import` myself: it shows my values for me to check. Never pass `-yes` for me.
> 2. Do not look at my health data. Do not open my lab reports or log files, and do not run `bean ls`, `bean show`, `bean export` or `bean blob get` unless I ask. Use exit codes and counts: the `added` line of an import, the `summary` line of `bean verify`.
> 3. If an import is refused, ask for the shape of the line, as the guide shows, not its contents.
> 4. Keep my data on my machine. Do not upload files or paste my records anywhere. Connecting you to my log through MCP is where my records start to reach your provider: do it only when I say so, read-only first (`bean mcp`, no `-write` or `-blobs`). Ask before `-write` or `-blobs`.

## 1. Install

```
brew install go poppler
git clone https://github.com/Conscious-Repository/bean.git
cd bean
go install ./cmd/...
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc && source ~/.zshrc
bean version
```

Poppler provides `pdftotext`, which reads PDFs. `go install` puts `bean`, `bean-import-lab`, `bean-import-apple-health` and `bean-import-csv` in `$(go env GOPATH)/bin`, or in `$GOBIN` if you set it. `bean version` should say `bean 0.6.0 (records v 0)` or later. An older version means another `bean` comes first on your `PATH`: `which -a bean` lists them. `package crypto/hkdf is not in GOROOT` means your Go is too old: `brew upgrade go`.

## 2. Create your log

```
echo 'export BEAN_DIR=~/health' >> ~/.zshrc && source ~/.zshrc
```

**Yourself:**

```
bean init -nostr new
```

It prints your identity (`identity sha256:...`), your npub, and 24 numbered words, and says where the log is.

1. **Write the 24 words on paper.** bean shows them only once. They are the only way to recover your identity. Never type them into a chat, an AI, a crypto wallet or a cloud note.
2. Write the identity id (the `sha256:...` on the first line) on the same paper. It is not secret. `restore -identity` needs it if a folder holds more than one identity.
3. Clear the terminal and its scrollback: Cmd-K in Terminal.app. `clear` alone keeps the scrollback.

`-nostr new` gives the log its own Nostr key. Do not reuse your social key.

## 3. Add a record and check it

```
bean add weight 81.2 -unit kg
bean ls
bean verify
```

`verify` ends with `summary: 2 valid, 0 invalid, 0 skipped, 0 held, 0 duplicate`. The count includes your identity record. The [type vocabulary](../cmd/bean/vocabulary.md) lists the usual types and units.

## 4. Import a lab report

**Yourself:**

```
bean import lab -tz America/Chicago ~/Downloads/lab-result.pdf
```

`-tz` is the zone where the blood was drawn: `America/New_York`, `America/Chicago`, `America/Denver`, `America/Los_Angeles`, `Europe/London`. Not `CST`. Without it, only the day is recorded. A Quest report prints its zone and needs no `-tz`.

bean reads the report and shows what it read, before it writes anything:

```
Read from lab-result.pdf:
  lab.wbc  5.5 x10E3/uL
  lab.chloride  95 mmol/L  Low
  lab.glucose  not performed
  ...
Save 71 readings and keep the file? [y/N]
```

Compare each value, unit and flag with the PDF. The importer reads the PDF's text layout; you are the last check. Values show as printed (`1.190`, `3.0`). A test the lab did not perform shows `not performed`. Type `y` to save. Anything else writes nothing.

Then:

```
kept lab.report sha256:5eea…: file sha256 10d7…, stored as sha256:6192…
added 71, skipped 0
bean-import-lab: 71 results, 2 without a value. Importing the same report again adds nothing; a result whose value changed needs -corrections
```

- `kept`: the PDF is stored, encrypted, before the readings. The first id is the record that keeps it: `bean purge <id>` erases the PDF. `file sha256` matches `shasum -a 256` of your PDF. `stored as` is the encrypted copy, which `bean blob get` reads.
- `added`: the results.
- The last line is the importer's summary.

Run the same import again: `already kept ...` and `added 0, skipped 71`. If the report says the collection time was not given, only the day is recorded.

`-yes` saves without the review, for scripts. Without a terminal and without `-yes`, `bean import` writes nothing and shows no value: that is what happens when an AI assistant runs it.

### If the import is refused

Nothing is written. The error names a line, for example `line 42: "x10E3/uL" runs across two columns`: the importer refuses what it cannot read for sure. The error can hold one of your values, so do not paste it publicly. Share the shape of the line instead, with letters and digits masked:

```
pdftotext -layout ~/Downloads/lab-result.pdf - | sed -n '42p' | tr '0-9' '9' | tr 'A-Za-z' 'x'
```

Use the line number from the error. The output looks like `      xxx 99        9.99        x99x9/xx        9.99-9.99`: enough to add the shape to the importer.

A `.txt` from `pdftotext -layout report.pdf report.txt` imports the same way. Apple Health and CSV: see the [README](../README.md#import-your-data).

## 5. Connect an AI assistant (optional)

`bean mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server over stdio. What a tool returns goes to the assistant's provider; use a local model if that matters to you. Start read-only.

| Command | The assistant can |
|---|---|
| `bean mcp` | List types, query and summarize records, read one record. |
| `bean mcp -write` | Also log entries (meals, symptoms, notes) with this machine's agent key. Not lab results. It cannot correct or delete your records, or sign as you. |
| `bean mcp -blobs` | Also read stored text files, such as a report imported as `.txt`, with your name and birth date. A PDF gives only its size. |

AI clients do not read your shell profile, so `~` and `BEAN_DIR` do not work there. Use the absolute paths from `which bean` and `echo $HOME/health`.

Claude Code (`-s user`: every folder, not just this one):

```
claude mcp add -s user bean -- "$(which bean)" -dir "$HOME/health" mcp
```

Claude Desktop: add to `~/Library/Application Support/Claude/claude_desktop_config.json`, then restart it:

```json
{
  "mcpServers": {
    "bean": {
      "command": "/Users/you/go/bin/bean",
      "args": ["-dir", "/Users/you/health", "mcp"]
    }
  }
}
```

Other clients: the same command and arguments as a stdio server.

Check the server before you connect a client:

```
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"check","version":"0"}}}' '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | bean mcp
```

Two JSON lines. The second lists `list_types`, `query_records`, `stats` and `get_record`.

To let the assistant log for you, add `-write -label assistant`. In Claude Code:

```
claude mcp remove -s user bean
claude mcp add -s user bean -- "$(which bean)" -dir "$HOME/health" mcp -write -label assistant
```

Its records show as `[assistant]` in `bean ls`. One machine has one agent key with one label: see the [README](../README.md#log-from-an-ai-assistant).

## 6. Back up

The log is `objects/` and `blobs/`. Any copy of the two is a backup: Time Machine, a drive, another machine. Also copy `local/purged-refs.jsonl` once it exists: it records which imports you deleted, so a later import does not bring them back.

```
mkdir -p /Volumes/Backup/health
cp -R ~/health/objects ~/health/blobs /Volumes/Backup/health/
bean -dir /Volumes/Backup/health verify
```

`verify` checks the copy without keys. Never copy the rest of `local/` to another machine: it holds this machine's keys, and the 24 words rebuild them. Lost laptop: see the [README](../README.md#backups-and-a-lost-laptop).

## 7. Leave, and come back

```
bean export ~/bean-export
```

Plain files, not encrypted. Keep them private.

- `records/`: every record, as JSON.
- `blobs/`: your stored files, decrypted, each named by its `stored as` id, no extension. Copy a PDF to a `.pdf` name to open it.
- `manifest.json`: your identity, the record count, and each stored file with its media type and SHA-256.

`shasum -a 256 ~/bean-export/blobs/*` shows the same hash as your original PDF.

**Yourself:** turn the export back into a log, in an empty folder, with your 24 words.

```
bean -dir ~/health-restored restore -export ~/bean-export -nostr new
```

1. The first run copies the records, shows what your old key signed, and stops with `bean: undecided: ...`, exit 1. That is expected: nothing is kept until you decide.
2. If every record on the list is yours, run it again with `-seal-all`. Type the 24 words and press Ctrl-D. They show on screen as you type: clear the scrollback afterwards (Cmd-K).
3. bean rebuilds your keys, makes a new signing key, and imports the files: `imported <n> files from the export`.

Check it:

```
bean -dir ~/health-restored verify
bean -dir ~/health-restored blob get <stored as id> | shasum -a 256
```

The hash matches your original. To use the restored log, point `BEAN_DIR` and your MCP config at `~/health-restored`. Never `bean init` for this: a new identity cannot read your export.

## Troubleshooting

| Message | Do this |
|---|---|
| `command not found: bean` | Add `$(go env GOPATH)/bin` to `PATH` (step 1). |
| `bean: not initialized`, `no log in ...` | Set `BEAN_DIR` (step 2) or pass `-dir ~/health`. Rebuilding? `bean restore` (step 7), not `bean init`. |
| `pdftotext not found` | `brew install poppler`, or import a `.txt` from `pdftotext -layout`. |
| `unknown time zone` | A zone name such as `America/Chicago`, not `CST`. |
| `the file was not accepted, and nothing was written` | A line the importer could not read for sure. See [If the import is refused](#if-the-import-is-refused). |
| `bean: input missing: <id>` | Use the record id from `bean ls` or the `kept` line, not the `stored as` id. |
| `changed: ... this file says ...` | A newer report changed a value. If it is right, import again with `-corrections`. |
| `bean: undecided: ...` from `restore` | Expected on the first run. Read the list, then `-seal-all` (step 7). |

Every command, flag and exit code: [COMMANDS.md](COMMANDS.md).
