# Getting started with bean

This guide takes you from nothing to a working health log: bean installed, your identity created, a lab report imported and checked, an AI assistant connected (optional), a backup in place, and a tested way to leave. Plan for about 30 minutes.

You can follow it on your own, or with an AI assistant helping you. If you use an assistant, give it the rules in [Working with an AI assistant](#working-with-an-ai-assistant) first.

## What you need

- A Mac with [Homebrew](https://brew.sh). Linux also works; install the same tools with your package manager.
- Access to this repository on GitHub. It is private for now, so sign in to GitHub on the command line first, for example with `gh auth login` (from `brew install gh`) or an SSH key.
- An encrypted disk. On a Mac, turn on FileVault in System Settings, Privacy & Security. bean signs your records but does not encrypt the record files themselves, so the disk encryption protects them at rest.
- A Labcorp patient report as a PDF, if you have one. Superpower results are Labcorp reports. If you do not have one, you can practice with the synthetic report in the repository, `cmd/bean-import-labcorp/testdata/real-layout.txt`.
- A pen and paper for your 24 recovery words.

## Working with an AI assistant

An assistant can run commands, explain errors, and later read your log through bean's MCP server. It must never see your recovery words, and it does not need to see your health data to help you install bean.

An assistant that runs commands sees their output, and that output is sent to the assistant's provider. This includes a terminal panel built into an AI app or an editor, and commands you run with the `!` prefix in Claude Code. So for the steps marked **You do this yourself**, use a separate terminal window, such as Terminal.app, that the assistant is not connected to.

If you use an assistant, paste these rules into the conversation before you start:

> Help me set up bean by following docs/ONBOARDING.md in the bean repository. Follow these rules:
>
> 1. Never ask me for my 24 recovery words, and never run a command that prints or reads them (`bean init`, `bean rotate`, `bean restore`). I run those myself, in a separate terminal window that you cannot read. If I start to type or paste the words to you, tell me to stop.
> 2. Do not look at my health data. Do not open my lab reports or log files, and do not run commands that print records or values (`bean ls`, `bean show`, `bean export`, `bean blob get`) unless I ask you to. To check progress, use exit codes and counts, such as the `added` line of an import or the summary line of `bean verify`.
> 3. If an import is refused, ask me for the shape of the line the error names, as described in the guide, not for its contents.
> 4. Keep my data on my machine. Do not upload files or paste my records into other services. Connecting you to my log through MCP is the point where my records start to reach your provider, so do it only when I say so, and read-only first (`bean mcp` without `-write` or `-blobs`). Ask me before you enable `-write` or `-blobs`.

## 1. Install bean

Install Go and Poppler. Poppler provides `pdftotext`, which the Labcorp importer uses to read PDFs.

```bash
brew install go poppler
```

Clone the repository and install the commands:

```bash
git clone https://github.com/Conscious-Repository/bean.git
cd bean
go install ./cmd/...
```

This builds `bean` and its importers (`bean-import-labcorp`, `bean-import-apple-health` and `bean-import-csv`) into `$(go env GOPATH)/bin`, which is usually `~/go/bin`. If you have set `GOBIN`, they go there instead. Make sure your shell can find them:

```bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
source ~/.zshrc
bean version
```

`bean version` should print `bean 0.6.0 (records v 0)` or later. If it prints an older version, another `bean` comes first on your `PATH`; `which -a bean` lists them all. If the build fails with `package crypto/hkdf is not in GOROOT`, your Go is too old; run `brew upgrade go`.

## 2. Create your log

Choose a folder for your log. This guide uses `~/health`. Tell bean about it, so that you do not have to pass `-dir` to every command:

```bash
echo 'export BEAN_DIR=~/health' >> ~/.zshrc
source ~/.zshrc
```

**You do this yourself.** In a separate terminal window, create your identity:

```bash
bean init -nostr new
```

It prints your identity id (`identity sha256:...`), a Nostr public key (`npub ...`), and 24 numbered words, and it tells you where your log is.

1. **Write the 24 words on paper.** bean shows them only once, and they are the only way to recover your identity on a new machine or after a lost laptop. Never type them into a chat, an AI assistant, a crypto wallet or a cloud note.
2. Write the identity id (the `sha256:...` on the first line) on the same paper. It is not secret, and you need it if you ever restore from a folder that holds more than one identity.
3. Clear the terminal and its scrollback once the words are on paper. In Terminal.app, press Cmd-K; the `clear` command alone keeps the scrollback.

`-nostr new` creates a separate Nostr key for this log. Use a key of its own rather than your social Nostr key.

## 3. Add a record and check the log

```bash
bean add weight 81.2 -unit kg
bean ls
bean verify
```

`bean ls` lists your records, oldest first. `bean verify` checks every signature and ends with a summary line such as `summary: 2 valid, 0 invalid, 0 skipped, 0 held, 0 duplicate`. The count includes the record that creates your identity, so it is higher than the number of things you added.

The [type vocabulary](../cmd/bean/vocabulary.md) lists the usual record types (`food`, `symptom.headache`, `exercise.run`, and so on) and their units.

## 4. Import a lab report

```bash
bean import labcorp -tz America/Chicago ~/Downloads/lab-result.pdf
```

`-tz` is the time zone of the place where your blood was drawn, written as a zone name: `America/New_York`, `America/Chicago`, `America/Denver`, `America/Los_Angeles`, `Europe/London`, and so on. Abbreviations such as `CST` do not work. Without `-tz`, bean records only the day of collection.

A successful import prints three lines:

```
kept lab.report sha256:5eea…: file sha256 10d7…, stored as sha256:6192…
added 71, skipped 0
bean-import-labcorp: 71 results, 2 without a value. Importing the same report again adds nothing; a result whose value changed needs -corrections
```

- The first line says that bean stored your PDF, encrypted, before it added anything else. The first id is the record that keeps the PDF; `bean purge` takes that id if you ever want the PDF erased. `file sha256` is the hash of your PDF as `shasum -a 256` reports it. The `stored as` id names the encrypted copy, which `bean blob get` reads back.
- The second line counts the results that were added.
- The third line is the importer's own summary.

Run the same import again. It should print `already kept ...` and `added 0, skipped 71`: importing a report twice adds nothing.

**You do this yourself:** check the results against your PDF.

```bash
bean ls -type 'lab.*' | sort -k3
```

The list holds one line per result, plus one `lab.report` line for the stored PDF. `sort -k3` orders the results by test name, which makes them easier to find in the report. Compare each value, unit and flag with the report. The importer reads the PDF's text layout, and you are the final check that it read it correctly. A number is shown in its shortest form (`61` for a printed `61.0`); `bean show <id>` gives the value as printed, as `raw_value`. A test that the lab did not perform shows as `not performed`. If the report says that the collection time was not given, bean records only the day.

### If the import is refused

Nothing is written, and the error names a line of the report, for example `line 42: "x10E3/uL" runs across two columns`. The importer refuses any line that it cannot read with certainty, rather than guess. The error can contain one of your values, so do not paste it into a public issue. Instead, share the shape of the line, which keeps its columns but hides its letters and digits:

```bash
pdftotext -layout ~/Downloads/lab-result.pdf - | sed -n '42p' | tr '0-9' '9' | tr 'A-Za-z' 'x'
```

Replace `42` with the line number from the error. The output looks like `      xxx 99        9.99        x99x9/xx        9.99-9.99`, which is enough for the maintainers to add the shape to the importer.

A text file made with `pdftotext -layout report.pdf report.txt` can be imported the same way as the PDF.

Other sources: Apple Health (`bean import apple-health export.zip`) and CSV files (`bean import csv -type <type> -unit <unit> file.csv`) are described in the [README](../README.md#import-your-data).

## 5. Connect an AI assistant (optional)

`bean mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server. Any AI client that supports MCP servers over stdio can use it to read your log. Whatever a tool returns is sent to the assistant's provider; if that is a problem for you, use an MCP client with a local model. Start read-only.

What you choose determines what the assistant can do:

| Command | The assistant can |
|---|---|
| `bean mcp` | List your record types, query and summarize your records, and read one record in full. |
| `bean mcp -write` | Also log entries (meals, symptoms, notes) signed with this machine's agent key, not your key. It cannot write lab results, correct or delete your records, or sign as you. |
| `bean mcp -blobs` | Also read the text of stored text files, such as a lab report imported as a `.txt`, which includes your name and birth date. For a PDF it gets only the file's size. |

AI clients do not read your shell profile, so `BEAN_DIR` and `~` do not work in their configuration. Use absolute paths. Find them with:

```bash
which bean
echo $HOME/health
```

**Claude Code.** `-s user` makes the server available in every folder, not only the one you run the command in:

```bash
claude mcp add -s user bean -- "$(which bean)" -dir "$HOME/health" mcp
```

**Claude Desktop.** Edit `~/Library/Application Support/Claude/claude_desktop_config.json`, add bean under `mcpServers`, and restart Claude Desktop:

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

**Other clients.** Configure a stdio server with the same command and arguments.

To check that the server works before you connect a client, run:

```bash
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"check","version":"0"}}}' '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | bean mcp
```

It prints two JSON lines. The second lists the tools `list_types`, `query_records`, `stats` and `get_record`.

When you want the assistant to log things for you, add `-write` and a label to the arguments, for example `"mcp", "-write", "-label", "assistant"`. In Claude Code, remove the server and add it again:

```bash
claude mcp remove -s user bean
claude mcp add -s user bean -- "$(which bean)" -dir "$HOME/health" mcp -write -label assistant
```

The assistant's records then show as `[assistant]` in `bean ls`. One machine has one agent key with one label; see [Log from an AI assistant](../README.md#log-from-an-ai-assistant).

## 6. Back up

Your log is the `objects/` and `blobs/` folders inside `~/health`. Any copy of those two folders is a backup: Time Machine, an external drive, or another machine. Also back up `local/purged-refs.jsonl` once it exists; it records which imports you deleted, so that a later import does not bring them back. For example, to an external drive:

```bash
mkdir -p /Volumes/Backup/health
cp -R ~/health/objects ~/health/blobs /Volumes/Backup/health/
bean -dir /Volumes/Backup/health verify
```

`verify` checks the copy. It runs without keys, so it reports `summary:` with every record valid.

Do not copy the rest of `local/` to other machines. It holds this machine's keys, and your 24 words rebuild them.

To restore after a lost laptop, see [Backups and a lost laptop](../README.md#backups-and-a-lost-laptop).

## 7. Leave, and come back

You can take everything out of bean at any time:

```bash
bean export ~/bean-export
```

The export is a plain folder. It is not encrypted, so keep it somewhere private.

- `records/` holds every record as JSON.
- `blobs/` holds your stored files, decrypted. Each file is named by its stored id (the `stored as` id), without an extension.
- `manifest.json` lists your identity, the number of records, and each stored file with its media type (for example `application/pdf`) and the SHA-256 of its content. To open a PDF, copy it to a name ending in `.pdf`.

Check that your PDF came out unchanged: `shasum -a 256 ~/bean-export/blobs/*` should show the same hash as `shasum -a 256` of your original file.

**You do this yourself:** to turn an export back into a working log, use an empty folder and your 24 words.

```bash
bean -dir ~/health-restored restore -export ~/bean-export -nostr new
```

1. The first run copies the records into the folder, shows what your old key signed, and stops with `bean: undecided: ...` and exit status 1. This is expected: bean does not keep anything until you decide.
2. Read the list. If every record is yours, run the same command again with `-seal-all`. Type the 24 words from your paper, then press Ctrl-D. The words are shown on the screen as you type, so clear the terminal and its scrollback afterwards (Cmd-K in Terminal.app).
3. bean rebuilds your keys, makes a new signing key, and imports the files from the export. It ends with `imported <n> files from the export`.

Check the restored log:

```bash
bean -dir ~/health-restored verify
bean -dir ~/health-restored blob get <the stored as id> | shasum -a 256
```

The hash should match your original file. To use the restored log from now on, point `BEAN_DIR`, and any MCP configuration, at `~/health-restored`.

Do not run `bean init` for this: it creates a new identity, which cannot read your export.

## Troubleshooting

| Message | What to do |
|---|---|
| `command not found: bean` | Add `$(go env GOPATH)/bin` to your `PATH` (step 1). |
| `bean: not initialized` / `no log in ...` | Set `BEAN_DIR` (step 2) or pass `-dir ~/health`. If you are rebuilding a log, use `bean restore` (step 7), not `bean init`. |
| `pdftotext not found` | Run `brew install poppler`, or import a `.txt` made with `pdftotext -layout`. |
| `unknown time zone` | Use a zone name such as `America/Chicago`, not `CST`. |
| `the file was not accepted, and nothing was written` | The importer refused a line it could not read with certainty. See [If the import is refused](#if-the-import-is-refused). |
| `bean: input missing: <id>` | Use the record id from `bean ls` or from the `kept` line, not the `stored as` id. |
| `changed: ... this file says ...` | A newer report changed a value. If the new report is right, run the import again with `-corrections`. |
| `bean: undecided: ...` from `restore` | Expected on the first run. Read the list, then run the command again with `-seal-all` (step 7). |

[docs/COMMANDS.md](COMMANDS.md) describes every command, flag and exit code.
