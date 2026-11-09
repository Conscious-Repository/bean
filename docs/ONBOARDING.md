# Getting started with bean

This guide takes you from nothing to a working health log: bean installed, your identity created, a lab report imported and checked, an AI assistant connected (optional), a backup in place, and a tested way to leave. Plan for about 30 minutes.

You can follow it on your own, or with an AI assistant helping you. If you use an assistant, give it the rules in [Working with an AI assistant](#working-with-an-ai-assistant) first.

## What you need

- A Mac with [Homebrew](https://brew.sh). Linux also works; install the same tools with your package manager.
- Access to this repository on GitHub. It is private for now.
- An encrypted disk. On a Mac, turn on FileVault in System Settings, Privacy & Security. bean signs your records but does not encrypt the record files themselves, so the disk encryption protects them at rest.
- Optionally, a Labcorp patient report as a PDF. Superpower results are Labcorp reports.
- A pen and paper for your 24 recovery words.

## Working with an AI assistant

An assistant can run commands, explain errors, and read your log through bean's MCP server. It must never see your recovery words, and it does not need to see your health data to help you install bean. If you use one, paste these rules into the conversation before you start:

> Help me set up bean by following docs/ONBOARDING.md in the bean repository. Follow these rules:
>
> 1. Never ask me for my 24 recovery words, and never run a command that prints or reads them (`bean init`, `bean rotate`, `bean restore`). I run those myself in a terminal that you cannot read. If I start to paste the words, tell me to stop.
> 2. Do not open or read my lab reports or my log files to help with setup. If an import fails, ask me for the shape of the line the error names (where each column starts, and what kind of text is there), not for its contents.
> 3. Connect to bean read-only first (`bean mcp` without `-write` or `-blobs`). Ask me before you enable writing or reading stored files.
> 4. Keep all my data on my machine. Do not upload files or paste my records into other services.

Rule 1 matters most. Coding assistants that run commands for you see the output of those commands, and that output goes to the assistant's provider. Run `bean init`, `bean rotate` and `bean restore` yourself, in your own terminal window.

## 1. Install bean

Install Go and Poppler (Poppler provides `pdftotext`, which the Labcorp importer uses to read PDFs):

```bash
brew install go poppler
```

Clone the repository and install the commands:

```bash
git clone https://github.com/Conscious-Repository/bean.git
cd bean
go install ./cmd/...
```

This builds `bean` and its importers (`bean-import-labcorp`, `bean-import-apple-health` and `bean-import-csv`) into `$(go env GOPATH)/bin`, which is usually `~/go/bin`. Make sure your shell can find them:

```bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
source ~/.zshrc
bean version
```

`bean version` should print `bean 0.6.0 (records v 0)` or later. If the build fails with `package crypto/hkdf is not in GOROOT`, your Go is too old; run `brew upgrade go`.

## 2. Create your log

Choose a folder for your log. This guide uses `~/health`. Tell bean about it, so that you do not have to pass `-dir` to every command:

```bash
echo 'export BEAN_DIR=~/health' >> ~/.zshrc
source ~/.zshrc
```

Now create your identity. **Run this yourself, in your own terminal, not through an AI assistant:**

```bash
bean init -nostr new
```

It prints your identity id (`identity sha256:...`), a Nostr public key (`npub ...`), and 24 numbered words, and it tells you where your log is.

1. **Write the 24 words on paper.** bean shows them only once. They are the only way to recover your identity on a new machine or after a lost laptop. Never type them into a chat, an AI assistant, a crypto wallet or a cloud note.
2. Write the identity id (the `sha256:...` on the first line) on the same paper. It is not secret, and you need it if you ever restore from a folder that holds more than one identity.
3. Clear the terminal (`clear`) once the words are on paper.

`-nostr new` creates a separate Nostr key for this log. Use a key of its own rather than your social Nostr key.

## 3. Add a record and check the log

```bash
bean add weight 81.2 -unit kg
bean ls
bean verify
```

`bean ls` lists your records, newest last. `bean verify` checks every signature and ends with a summary line such as `summary: 3 valid, 0 invalid, 0 skipped, 0 held, 0 duplicate`. The count includes the records that make up your identity, so it is higher than the number of things you added.

The [type vocabulary](../cmd/bean/vocabulary.md) lists the usual record types (`food`, `symptom.headache`, `exercise.run`, and so on) and their units.

## 4. Import a lab report

```bash
bean import labcorp -tz America/Chicago ~/Downloads/lab-result.pdf
```

`-tz` is the time zone of the place where your blood was drawn, written as a zone name: `America/New_York`, `America/Chicago`, `America/Denver`, `America/Los_Angeles`, `Europe/London`, and so on. Abbreviations such as `CST` do not work. Without `-tz`, bean records only the day of collection.

A successful import prints three lines:

```
kept lab.report sha256:5eea…: file sha256 10d7…, stored as sha256:68f4…
added 71, skipped 0
bean-import-labcorp: 71 results, 2 without a value. Importing the same report again adds nothing; a result whose value changed needs -corrections
```

- The first line says that bean stored your PDF, encrypted, before it added anything else. The first id is the record that keeps the PDF; `bean purge` takes that id if you ever want the PDF erased. `file sha256` is the hash of your PDF as `shasum -a 256` shows it.
- The second line counts the results that were added.

Check the results against your PDF:

```bash
bean ls -type 'lab.*'
```

Go through the list and compare each value, unit and flag with the report. The importer reads the PDF's text layout, and you are the final check that it read it correctly. A test that the lab did not perform shows as `not performed`. If the report says that the collection time was not given, bean records only the day.

Run the same import again. It should print `added 0, skipped 71`: importing a report twice adds nothing.

**If the import is refused,** nothing is written, and the error names a line of the report, for example `line 42: "x10E3/uL" runs across two columns`. The importer refuses any line it cannot read with certainty, rather than guess. The error message can contain one of your values, so do not paste it into a public issue. Describe the shape of the line instead (which columns hold text, and where they start), or share it privately with the maintainers.

A text file made with `pdftotext -layout report.pdf report.txt` works the same way as the PDF.

Other sources: Apple Health (`bean import apple-health export.zip`) and CSV files (`bean import csv -type <type> -unit <unit> file.csv`) are described in the [README](../README.md#import-your-data).

## 5. Connect an AI assistant (optional)

`bean mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server. Any AI client that supports MCP servers over stdio can use it to read your log. Start read-only.

What you choose determines what the assistant can do:

| Command | The assistant can |
|---|---|
| `bean mcp` | List your record types, query and summarize your records, and read one record in full. |
| `bean mcp -write` | Also log entries (meals, symptoms, notes) signed with this machine's agent key, not your key. It cannot write lab results, correct or delete your records, or sign as you. |
| `bean mcp -blobs` | Also read the text of stored files, such as a kept lab report. A kept report includes your name and birth date. |

Whatever a tool returns is sent to the assistant's provider. If that is a problem for you, use an MCP client with a local model.

AI clients do not read your shell profile, so `BEAN_DIR` and `~` do not work in their configuration. Use absolute paths. Find them with:

```bash
which bean
echo $HOME/health
```

**Claude Code:**

```bash
claude mcp add bean -- "$(which bean)" -dir "$HOME/health" mcp
```

**Claude Desktop:** edit `~/Library/Application Support/Claude/claude_desktop_config.json` and add bean under `mcpServers`, then restart Claude Desktop:

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

**Other clients:** configure a stdio server with the same command and arguments.

To check that the server works before you connect a client:

```bash
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"check","version":"0"}}}' '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | bean mcp
```

It prints two JSON lines; the second lists the tools `list_types`, `query_records`, `stats` and `get_record`.

When you want the assistant to log things for you, add `-write` and a label to the arguments (for example `"mcp", "-write", "-label", "assistant"`). Its records then show as `[assistant]` in `bean ls`. One machine has one agent key with one label; see [Log from an AI assistant](../README.md#log-from-an-ai-assistant).

## 6. Back up

Your log is the `objects/` and `blobs/` folders inside `~/health`. Any copy of those two folders is a backup: Time Machine, an external drive, or another machine. Also back up `local/purged-refs.jsonl` if it exists; it records which imports you deleted, so that a later import does not bring them back.

Do not copy the rest of `local/` to other machines. It holds this machine's keys. Your 24 words rebuild them.

To restore after a lost laptop, see [Backups and a lost laptop](../README.md#backups-and-a-lost-laptop).

## 7. Leave, and come back

You can take everything out of bean at any time:

```bash
bean export ~/bean-export
```

The export is a plain folder: `records/` holds every record as JSON, `blobs/` holds your stored files decrypted, and `manifest.json` lists each file with its media type (for example `application/pdf`) and SHA-256. The files are named by their hash, without an extension; rename a PDF to `.pdf` to open it. The export is not encrypted, so keep it somewhere private.

Check that your PDF came out unchanged: `shasum -a 256 ~/bean-export/blobs/*` should show the same hash as `shasum -a 256` of your original file.

To turn an export back into a working log, use an empty folder and your 24 words. **Run this yourself:**

```bash
bean -dir ~/health-restored restore -export ~/bean-export -nostr new
```

The first run copies the records and shows what your old key signed, then stops. Read the list; if every record is yours, run the same command again with `-seal-all`, type or paste the 24 words, and press Ctrl-D. bean rebuilds your keys, makes a new signing key, and imports the files from the export. Do not run `bean init` for this: it creates a new identity, which cannot read your export.

## Troubleshooting

| Message | What to do |
|---|---|
| `command not found: bean` | Add `$(go env GOPATH)/bin` to your `PATH` (step 1). |
| `bean: not initialized` / `no log in ...` | Set `BEAN_DIR` (step 2), or pass `-dir ~/health`. |
| `pdftotext not found` | `brew install poppler`, or import a `.txt` made with `pdftotext -layout`. |
| `unknown time zone` | Use a zone name such as `America/Chicago`, not `CST`. |
| `the file was not accepted, and nothing was written` | The importer refused a line it could not read. See step 4. |
| `bean: input missing: <id>` | Use the record id from `bean ls` or from the `kept` line, not the `stored as` blob id. |
| `changed: ... this file says ...` | A newer report changed a value. If the new report is right, run the import again with `-corrections`. |

[docs/COMMANDS.md](COMMANDS.md) describes every command, flag and exit code.
