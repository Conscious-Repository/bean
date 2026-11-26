# Getting started

From nothing to a working log: install, identity, a checked lab import, and a comparison with public data. Then a share and an AI assistant, both optional, a backup, and a tested exit. About 30 minutes without the two optional steps.

## You need

- A Mac with [Homebrew](https://brew.sh). Linux works with the same tools.
- An encrypted disk. On a Mac, turn on FileVault. bean signs records but does not encrypt the record files.
- A lab report PDF, if you have one. bean reads the layouts it knows; today, the Labcorp patient report and the Quest Diagnostics report. To practice, use `cmd/bean-import-lab/testdata/real-layout.txt` (Labcorp) or `quest-layout.html` (Quest): real layouts with synthetic content. Practice in a throwaway folder, not in your log: `bean -dir /tmp/practice init`, then `bean -dir /tmp/practice import lab ...`.
- Pen and paper for 24 words.

## Working with an AI assistant

An assistant can run commands and explain errors. It must never see your 24 words, and it does not need your health data to help you install bean.

An assistant that runs commands sees their output, and the output goes to its provider. That includes a terminal built into an AI app or editor, and `!` commands in Claude Code. Steps marked **Yourself** go in a separate terminal window, such as Terminal.app, that no assistant reads.

Give the assistant these rules first:

> Help me set up bean by following docs/ONBOARDING.md. Rules:
>
> 1. Never ask for my 24 words. Never run `bean init`, `bean rotate` or `bean restore`: I run them myself, in a terminal you cannot read. If I start to type or paste the words to you, stop me. I also run `bean import` myself: it shows my values for me to check. Never pass `-yes` for me.
> 2. Do not look at my health data. Do not open my lab reports or log files, and do not run `bean ls`, `bean show`, `bean compare`, `bean coverage`, `bean check-bundle`, `bean export` or `bean blob get` unless I ask. Never run `bean share`, `bean reply` or `bean accept`: I decide what leaves and what comes in, at my own terminal. Use exit codes and counts: the `added` line of an import, the `summary` line of `bean verify`. Never ask for my sex or my birth, and never add a `person.sex` or `person.birth` record for me.
> 3. If no reading was read, ask for the counts of `bean-import-lab -count`, as the guide shows, not the report or its values.
> 4. Keep my data on my machine. Do not upload files or paste my records anywhere. Connecting you to my log through MCP is where my records start to reach your provider. Do it only when I say so, read-only first (`bean mcp`, no `-write` or `-blobs`). Ask before `-write` or `-blobs`.
> 5. Never run `bean address`, and never read or copy a key file. A key file opens every package that was sealed to its address. Never pass `-key` or `-address-words` for me. I type the address words myself: they are my check that an address is my reader's.

## 1. Install

```
brew install go poppler
git clone https://github.com/Conscious-Repository/bean.git
cd bean
go install ./cmd/...
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc && source ~/.zshrc
bean version
```

Poppler provides `pdftotext`, which reads PDFs. `go install` puts `bean`, `bean-import-lab`, `bean-import-apple-health`, `bean-import-csv`, `bean-import-nhanes` and `bean-pack-nhanes` in `$(go env GOPATH)/bin`, or in `$GOBIN` if you set it. `bean version` should say `bean 0.8.4 (records v 0)` or later. An older version means another `bean` comes first on your `PATH`: `which -a bean` lists them. `package crypto/hkdf is not in GOROOT` means your Go is too old: `brew upgrade go`.

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

`verify` ends with `summary: 2 valid, 0 invalid, 0 skipped, 0 held, 0 duplicate`. The count includes your identity record. After you delete a record, `verify` also prints a `held` warning for the deletion's own record, and counts it as `held`: that is normal. The [type vocabulary](../cmd/bean/vocabulary.md) lists the usual types and units.

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

Run the same import again: it shows the readings and asks again, then says `already kept ...` and `added 0, skipped 71`. If the report says the collection time was not given, only the day is recorded.

`-yes` saves without the review, for scripts. Without a terminal and without `-yes`, `bean import` writes nothing and shows no value: that is what happens when an AI assistant runs it.

### If no reading was read

`bean import lab` reads a report only in a layout that it knows, and labs print many. When it cannot read a report for sure, it writes none of the values, and it keeps the file. The report is then in your log, encrypted, under its SHA-256. bean says `no reading was read`, and why, with the words of your report masked. `bean purge <id>` takes the file out again, and bean then refuses that same file in a later import. A file with no line of column names, such as `Test`, `Result` and `Reference Range`, is not kept as a lab report. A scan, which holds a picture of the page and no text, is such a file. `bean blob put <file>` keeps a file of any kind.

To say which layouts bean cannot read, send counts and no values. This writes nothing, and prints no value, no name of a test and no word of your report:

```
bean-import-lab -count ~/Downloads/lab-result.pdf
```

Its lines say how far each reader got: how many results, and at which line or place on a page it stopped. What a refusal quotes from your report shows as `x` and `9`, 20 characters at most: its shape, not its words. Until bean reads your layout, `bean add` and `bean import csv` take the values that you type or keep in a table.

A `.txt` from `pdftotext -layout report.pdf report.txt` imports the same way. Apple Health and CSV: see the [README](../README.md#import-your-data).

## 5. Compare with public data

A reference pack is one file of public data, with a table for each test and each group of people. It comes to your machine, and your reading stays there. Today you build the survey's pack from eight public files of the US CDC, about 9 MB. Use a folder of its own, not the folder of bean's code:

```
mkdir -p ~/nhanes && cd ~/nhanes
for f in DEMO_J TCHOL_J HDL_J GHB_J CBC_J BIOPRO_J FERTIN_J BMX_J; do
  curl -fLO https://wwwn.cdc.gov/Nchs/Data/Nhanes/Public/2017/DataFiles/$f.xpt
done
bean-pack-nhanes -out nhanes.pack *.xpt
shasum -a 256 nhanes.pack
bean compare -pack nhanes.pack
```

For current signed clinical-panel matches, use the [evidence-qualified core packs](EXTERNAL-PACKS.md#evidence-qualified-core-packs). The following local builder exercise reproduces the historical type-and-unit pack.

1. `bean-pack-nhanes` ends with `This is the pack that bean has on record.` The SHA-256 is the second one in [REFERENCE.md](REFERENCE.md#how-a-pack-travels), which starts `5cae07db`. The builder refuses a file that is not the one on record: fetch that file again.
2. The last command lists the groups, and the type names and units that the pack takes.

**Yourself:** choose the group that describes you. bean does not know yours, and never guesses. In a practice log, put `-dir /tmp/practice` after `bean`.

```
bean compare -pack nhanes.pack -cohort male-30-39 -type 'lab.*'
```

Read N, the number of people, before the percentile. A row says where a value lies in that sample, and not whether it is healthy. A reading in another unit gets no number.

Nothing is written until you add `-save`. Then each result is a record that you signed. It names your reading, the pack and the method, so another person can compute it again. It also gives the reading's value away to anyone who has the record and the pack.

- **What bean checks,** and what no check shows: [ARCHITECTURE.md](ARCHITECTURE.md#what-bean-checks-in-a-comparison).
- **A pack from a host.** You fetch one file and check the same SHA-256, and the rest is as above ([FIRST-RUN.md](FIRST-RUN.md#each-compares-with-a-pack)).
- **A group of your own,** by ethnicity, body or food: [COMPARE.md](COMPARE.md).
- **The group of each reading's own date.** One run with `-cohort` sets an old reading against the one group that you name. You can state your sex and your birth once, as two records, and then leave `-cohort` out ([REFERENCE.md](REFERENCE.md#the-cohort-from-two-records)). Both records are optional. Whoever reads your log reads them, and so does an assistant that you connect.
- **Keep the pack file.** Your log names it and does not hold it.

## 6. Share with one person (optional)

**Yourself,** in each case: a share shows your values, and it cannot be taken back once it is opened.

- **A reader with no bean,** such as a doctor. `bean share -seal -out for-ana.bean <id>` writes one file that opens with six words, in a browser or with bean. Send the file one way and say the words another.
- **A peer who runs bean.** [FIRST-RUN.md](FIRST-RUN.md) is one page for the two of you: a sealed package, a signed reply, and the comparison of step 5. No six words are read out.

The reader checks that each record is yours and unchanged. They cannot check that a value is true, or that you sent everything. [SHARE.md](SHARE.md) has the limits, and [examples/blood](../examples/blood/README.md) runs an import, a comparison with the survey's files and a share on a synthetic report.

## 7. Connect an AI assistant (optional)

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

## 8. Back up

The log is `objects/` and `blobs/`. Any copy of the two is a backup: Time Machine, a drive, another machine. Also copy `local/purged-refs.jsonl` once it exists: it records which imports you deleted, so a later import does not bring them back.

```
mkdir -p /Volumes/Backup/health
cp -R ~/health/objects ~/health/blobs /Volumes/Backup/health/
bean -dir /Volumes/Backup/health verify
```

`verify` checks the copy without keys. Never copy the rest of `local/` to another machine: it holds this machine's keys, and the 24 words rebuild them. Lost laptop: see the [README](../README.md#backups-and-a-lost-laptop).

## 9. Leave, and come back

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
2. If every record on the list is yours, run it again with `-seal-all`. A line `DELETES A RECORD NOT IN THIS FOLDER` is a deletion of your own: the record is gone, and its deletion stays. Type the 24 words and press Ctrl-D. They show on screen as you type: clear the scrollback afterwards (Cmd-K).
3. bean rebuilds your keys, makes a new signing key, and imports the files: `imported <n> files from the export`.

Check it:

```
bean -dir ~/health-restored verify
bean -dir ~/health-restored blob get <stored as id> | shasum -a 256
```

The hash matches your original. To use the restored log, point `BEAN_DIR` and your MCP config at `~/health-restored`. Never `bean init` for this: a new identity cannot read your export.

Saved comparisons, the reference files you kept and the records of your shares are in the export like every record. A comparison with the survey's files can be checked from the export alone ([COMPARE.md](COMPARE.md)). A comparison with a reference pack also needs the pack. Your log names it and does not hold it, so keep that file with your export. The bundles and packages that you sent or received are outside your log: keep those yourself. A key file from `bean address` is outside your log too, and the 24 words do not bring it back. Without it, no package that was sealed to its address opens again. bean does not encrypt a key file, so keep it as any secret key is kept.

## Troubleshooting

| Message | Do this |
|---|---|
| `command not found: bean` | Add `$(go env GOPATH)/bin` to `PATH` (step 1). |
| `bean: not initialized`, `no log in ...` | Set `BEAN_DIR` (step 2) or pass `-dir ~/health`. Rebuilding? `bean restore` (step 9), not `bean init`. |
| `pdftotext not found` | `brew install poppler`, or import a `.txt` from `pdftotext -layout`. |
| `unknown time zone` | A zone name such as `America/Chicago`, not `CST`. |
| `the file was not accepted, and nothing was written` | The file is not one that the importer takes at all, or you did not say yes. |
| `no reading was read` | A line the importer could not read for sure. The file is kept. See [If no reading was read](#if-no-reading-was-read). |
| `bean: input missing: <id>` | Use the record id from `bean ls` or the `kept` line, not the `stored as` id. |
| `bean has no record of this pack` | The file is not one of the two packs that bean has on record. For one of those two, check the SHA-256 (step 5) and fetch or build it again. For any other pack, compare its SHA-256 with the one that its publisher states. Then pass `-unknown-pack`: each result says so. |
| `not comparable: ...` in a row | The row says why, for example a unit that the pack does not take. bean converts no unit. |
| `changed: ... this input says ...` | A newer report changed a value. If it is right, import again with `-corrections`. |
| `bean: undecided: ...` from `restore` | Expected on the first run. Read the list, then `-seal-all` (step 9). |

Every command, flag and exit code: [COMMANDS.md](COMMANDS.md).
