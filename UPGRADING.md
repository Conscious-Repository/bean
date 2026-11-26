# Upgrading

Read this before you run a new build of `bean` on a log that you already have. It lists the changes that alter what an existing log shows. Records, ids and signatures never change. A new build reads the same files and can judge them differently.

## Check a new build on copies of your log

This is the one way to see what a new build changes in your log. Each build gets its own copy, because commands hide and restore files (SPEC.md 7.6). Save this as `bean-check.sh`. Set `log` to your folder and `repo` to your checkout of the bean source. Set `ref` to the build to try (`origin/main`, or a branch such as `origin/round3`). Then run `bash bean-check.sh`.

```
set -euo pipefail
log=~/health; repo=~/src/bean; ref=origin/main
d=$(mktemp -d "$HOME/bean-check.XXXXXX")      # new, empty, mode 0700
trap 'rm -rf "$d"' EXIT                         # the copies hold your keys
git -C "$repo" fetch -q origin
mkdir "$d/src"
git -C "$repo" archive "$ref" | tar -x -C "$d/src"
(cd "$d/src" && go build -o "$d/bean-new" ./cmd/bean)
cp "$(command -v bean)" "$d/bean-old"          # the build you use today
cp -R "$log" "$d/old"
cp -R "$log" "$d/new"
for b in old new; do
  rc=0; "$d/bean-$b" -dir "$d/$b" verify > "$d/$b-verify.txt" || rc=$?
  echo "exit $rc" >> "$d/$b-verify.txt"
  "$d/bean-$b" -dir "$d/$b" ls -all -json | sort > "$d/$b-records.txt"
done
same=yes
diff "$d/old-verify.txt" "$d/new-verify.txt" || same=no
diff "$d/old-records.txt" "$d/new-records.txt" || same=no
echo "same: $same"
```

The script stops with an error if a step fails, for example the fetch or the build. Then it has checked nothing.
The first `diff` shows new errors, warnings and exit codes.
The second shows each record that only one build shows. A line with `<` is valid only under the old build. A line with `>` is valid only under the new build.
`same: yes` means the new build shows your log exactly as the old one does.
The script deletes its folder when it ends.
Do not run `purge` or `gc` on the copies. Only they erase anything.

## Changes

### 0.8.5 (2026-11-26)

No record changes, and `bean verify` and `bean ls` print the same for an existing log. `bean compare -pack` can take each reading's cohort from two records that you state ([DECISIONS 348](docs/DECISIONS.md)).

- **With `-cohort`, nothing changes.** bean prints the same lines and saves the same claim. It reads neither of the two records.
- **`bean compare -pack` with readings and no `-cohort` was an error.** It still is, unless a `person.sex` and a `person.birth` record count, and the pack is one on record. A record counts when your identity signed it about you, with the source `self` and no ref. `bean add` with no `-agent` writes it so. The error starts with the same words: `name a cohort of the pack with -cohort:`. It then names the two records. For a pack that is not on record, it says that this way needs a pack on record. The exit code is 2, as before. One order changes. For a pack on record, bean now reads the log first: with no reading that matches, it says so and exits 1.
- **With both records, that command prints a table.** Each row has the cohort of the age on the reading's own date, in a new COHORT column. [REFERENCE.md](docs/REFERENCE.md#the-cohort-from-two-records) has the rules. It saves nothing: `-save` needs `-cohort`, as before.
- **If your log already has a record of type `person.sex` or `person.birth`,** bean now reads it. Until now bean gave these types no meaning. `bean ls -type 'person.*'` shows what your log holds. A record that breaks a rule stops that command with exit 1, and not 2. The stop names the record by id. With `-cohort`, bean reads neither.
- **The two records are optional, and they are ordinary records.** Whoever reads your log reads them: a copy, an export, a share of `*`, and an assistant with `bean mcp`. A share of `lab.*` or of a comparison does not carry them.
- **An earlier release gives the two records no meaning.** It lists them with `bean ls`, like any record. There, `bean compare -pack` with no `-cohort` stays an error.
- **Copy the rule for your assistant again.** [ONBOARDING.md](docs/ONBOARDING.md) now tells it not to ask for your sex or birth, and not to add these records. A record that it adds with a plain `bean add` is signed under your identity.
- **No saved comparison reads differently.** No text of a claim changes, and the two packs on record are the same two.

### 0.8.4 (2026-11-25)

No record changes, and `bean verify` and `bean ls` print the same for an existing log. bean reads three more pack formats, and `bean coverage` prints more ([DECISIONS 346](docs/DECISIONS.md)).

- **bean reads `pack/2`, `pack/3` and `pack/4`.** Release 0.8.3 refuses each: `unsupported pack format: expected "pack/0" or "pack/1"`. A pack that 0.8.3 reads compares the same. [REFERENCE.md](docs/REFERENCE.md#the-file) says what each format adds.
- **A `pack/2` match can need signed fields of the reading.** They are the source system and context fields, such as the test, the panel and the fasting state. A reading without them gets no percentile. bean prints the fields that the pack requires.
- **A `pack/3` table can hold a reported zero.** A comparison with such a table has method version 2. Every other table keeps version 1.
- **A `pack/4` pack can match a ratio that a report prints with no unit.** The reading must have no `unit` member. The pack must name the source system, the test and the panel.
- **bean has no record of any pack in these formats.** `bean compare -pack` takes one only with `-unknown-pack`, and each result says so. The two packs on record are the same two as before.
- **Release 0.8.3 lists a comparison that it cannot compute again.** It shows a comparison that 0.8.4 saved with a `pack/3` or `pack/4` pack, and `bean verify` passes its signature. It cannot read that pack.
- **`bean coverage` is new since 0.8.3.** Its plain output names the caveats of each percentile by number, and says what a percentile means. A caveat or a source name of a pack is a quote, with the SHA-256 of the pack. So are the words of an assay guide. A quote is cut at 400 bytes. A measure id of a pack is not a quote. Its formats are drafts and can change.
- **A pack with the same number of measures can be another file.** [EXTERNAL-PACKS.md](docs/EXTERNAL-PACKS.md) lists packs of 13, 20 and 23 measures in `pack/2`. Each has another SHA-256 than the two on record, and bean has no record of them.
- **`bean compare -pack` shows a caveat that the pack states as a quote:** `the pack says: "..."`. Only the other caveats are bean's words. The cohort's text is in quote marks too, and the list of a pack starts with `The pack's title:`. A saved comparison holds the same texts as before.
- **A refusal names the fields that a pack requires.** It starts with `required signed fields:` or `required signed evidence:`. Release 0.8.3 said only that the source, test or panel does not match. `bean add` writes no such fields: import the report.
- **`tools/compare/reproduce.py` refuses a saved comparison with another source system, or with a member more.** One that bean saved passes as before. The `reproduce.py` of release 0.8.3 prints `DIFFERS` for a comparison with a `pack/2`, `pack/3` or `pack/4` pack, since it cannot read the pack.
- **`bean coverage` refuses a selection or a guide with a key in upper case.** [COMMANDS.md](docs/COMMANDS.md) names two new members of its `-json` output.
- **Tell your assistant not to run `bean coverage`** unless you ask. It prints your values. [ONBOARDING.md](docs/ONBOARDING.md) has the rule to copy again.

### 0.8.3 (2026-11-23)

Bean reads both `pack/0` and `pack/1`. A `pack/1` publisher can require the signed source system, printed test name and panel alongside the type and unit.

- The two packs that bean has on record use `pack/0`: 13 measures (`10555d67…`) and 20 measures (`5cae07db…`). You can keep using the copies you already have.
- The 23-measure iron pack uses `pack/1`. Check its [external publisher digest](docs/EXTERNAL-PACKS.md) before using `-unknown-pack`.
- A Quest “% SATURATION” row compares only when its signed fields match the iron panel rule. A missing or different panel yields no percentile.
- Existing imports already contain their signed fields. Run the comparison on your existing log.

### 0.8.2 (2026-11-21)

A reference pack may have a cell of any number of people. Nothing in a log changes, and no saved comparison reads differently ([docs/REFERENCE.md](docs/REFERENCE.md), DECISIONS 333).

- **A pack with a cell of fewer than 30 people is read.** Until 0.8.1 bean refused the whole pack. A result from such a cell has the caveat of a small cohort, and N shows the number.
- **Whoever builds such a pack, and whoever reads it, both need 0.8.2.** An earlier bean refuses the pack: `… has an n that is not a whole number from 30 to 2147483647`. The `check.py` of an earlier release refuses it too: `… has an n that is not from 30 to 2147483647`. Its `reproduce.py` cannot compute a comparison that was saved with such a pack.
- **With only the survey's packs, there is nothing to do.** Update before you open a pack with a small cell.
- **The survey's packs do not change.** Their builder still leaves out a cell of fewer than 30 people. `bean compare` without `-pack` still gives no number for a cohort of fewer than 30.
- **One line changes.** For a cohort that a pack has no table for, bean says: `the pack has no table of this measure for this cohort`. It gave a reason before, which was one of several.
- **`tools/pack/check.py` reads the same packs.** Its line for an n that the format does not take is now: `… has an n that is not a whole number from 1 to 2147483647`.

### 0.8.1 (2026-11-18)

A reply can be sealed to the address of the person who shared. Nothing in a log changes. `bean share` and a reply with six words are as before ([docs/SHARE.md](docs/SHARE.md#a-reply-to-an-address), DECISIONS 330).

- **New flags.** `-for <address>`, `-address-words` and `-yes` on `bean reply`, and `-key <file>` on `bean accept`. No flag that existed changes its meaning.
- **`-yes` does not go with `-key` on `bean accept`.** It is a usage error, exit 2. `bean accept -key` asks on a terminal.
- **Both people update first.** Release 0.8.0 does not take a reply that is sealed to an address. `bean accept` prints `this file is sealed to a key. bean accept takes a reply, and this bean seals a reply with words: nothing was taken`, exit 1. With `-key` it prints its usage line, exit 2. Its `bean reply -for` prints `A note that starts with - is read as an option. Put -- before the first id.` and its usage line, exit 2.
- **One line of `bean accept` changes.** For a file that is sealed to a key, without `-key`, it now says: `this file is sealed to a key. Open it with bean accept -key <your key file>: nothing was taken`. Exit 1, as before.
- **`bean check-bundle` on a reply that a key opened** names `bean accept -key <their key file>`. For any other reply it names `bean accept`, as before.
- **A `reply` record can say `"seal": "recipient"`.** It names no address. An earlier bean reads it as any other `reply` record: no check reads that member.

### 0.8.0 (2026-11-18)

A package can be sealed to a reader's address. Nothing in a log changes, and `bean share -seal` with its six words is as before ([docs/SHARE.md](docs/SHARE.md), DECISIONS 327).

- **New commands and flags.** `bean address` makes a reader's key file, or shows its address. `bean share -for <address>` seals to it. `-key <file>` on `bean check-bundle` and on `bean reply -to` opens such a package. No flag that existed changes its meaning.
- **Both people update first.** A reader with release 0.7.2, or with its page, cannot open a package that is sealed to an address. What they say is wrong for such a file. A bean that has `bean address` can open one.
- **What `bean check-bundle` of release 0.7.2 prints for such a file.** First `Type the six words of this package, then Enter:`, and such a package has no words. With no words typed it prints `problem: <file> is sealed: it opens with the six words that its sender gave you`. With six words of the list it prints `problem: <file> was not opened: it is not sealed with words: this is another kind of age file`. Then `the package did not open: nothing was checked`, exit 1.
- **What the page of release 0.7.2 shows for such a file.** The box `This file is sealed. Type the six words that its sender gave you, with a space between them:`. After six words it says `This file did not open.` Then it says `Nothing is known about “<file>”. Check the six words and type them again. If they are right, the file was changed on the way: ask the sender for a new one.`
- **Nothing is read wrongly.** Release 0.7.2 opens no such file, and shows no record of one.
- **A share record can say `"seal": "recipient"`.** It names no address. An earlier bean reads such a record as any other share record: no check of a log or of a bundle reads that member.
- **What changes for a file that you have.** A package with six words opens as before, with the same lines. An `age` file that is not such a package is now refused before bean asks for words. bean says what the file asks for: a key, or something that bean does not open. The page asks its check what a file asks for, before it shows the box for words.
- **A place that reaches your log's folder through a link is now refused.** So is one that names the folder in other capitals, on a disk that does not tell them apart. That holds for a bundle, a reply and a key file. Before, bean judged the path as it was written. `check-bundle -unpack` has no such check, as before.
- **An `age` file in armor is refused by name.** Before, bean said that it could not read it as a zip file.
- **A key file is not in your log.** `bean restore`, `bean export` and a copy of the log do not hold it. If you lose the file, you cannot open what was sealed to it.
- **A reply still makes six words.** `bean accept` takes no file that is sealed to a key.

### 0.7.2 (2026-11-17)

One fault is mended, and one table changes. Nothing in a log changes.

- **A share with `-files` carried a kept file as a public file by its record's type alone.** A record of the type `reference.file` can name any file. `bean append` writes such a record, and you or an assistant of yours could have run it. With `-files`, a share took the file of each such record that a shared record was about, and the review called it "the public file". Nobody had to name the record. A review of the plan for reference data found this (DECISIONS 321, 322).
- **Now a file is public only when bean has its SHA-256 on record:** a file of the survey as CDC serves it. bean checks the bytes against that SHA-256 before the file leaves. Any other kept file goes only when you name its record by its id.
- **What changes for you.** A kept copy of a survey file that is not the one on record no longer goes with a comparison. That is a copy that `bean compare` uses only with `-unknown-files`, or a file of a data set that bean has no SHA-256 for. `bean share` names each such file and says what to add to the command: the id of its record. A script that shared such files does not fail. It sends fewer files, and says so on stderr.
- **Whether a file of yours left this way.** If you kept files only with `bean import`, no private file left: `bean import nhanes` keeps only a file that reads as a file of the survey. A `reference.file` record that names another kind of file comes from `bean append`. To be sure, look at each share that you made with `-files`. Run these yourself, or ask your assistant to.
  1. `bean ls -type share` lists your shares.
  2. `bean show <id>` shows a share. Under `context`, `files` has one entry for each file that the share carried, with the `sha256` of its bytes and the id of its `record`. A share that carried no file has no entry there.
  3. Compare each `sha256` with the list below. A file with one of these is a file of the survey, as CDC serves it.
  4. Any other file went because you named its record, or by this fault. What a record says of itself, such as a `dataset`, proves nothing here: `bean append` writes that too.

  If a share carried a file that you did not mean to send, its reader has that file. bean cannot take it back: ask the reader to delete it.

  ```
  c0b46e0345ea19404928656277c8b0d10b0cca348a9b2fe4fc3c67e8b7ee73ec  DEMO_J
  0291a6a4f6d82dac8392c8c992b519946824dfeda01647935520cf506ed9f4ab  TCHOL_J
  9f4eb41b89f0c9f3d6262f873e671eeb2f2eb40581a8426b2a3c8fd4fc19439e  HDL_J
  35f07094573a0061a03ed609a5a363b34eb1b1c7065d1623b43d72e132a8a654  GHB_J
  00f964098dc91272e415344554cbf1b627ecaeb2eda201518527446a7d81e742  CBC_J
  8d675e42d8826ac98714b2c3dd4c5138a5e353fb4424f7eff5e6db4a01ce838a  BMX_J
  390ae5205acfbcb87e0ca3d9aabd1c801b60657d3f2802ac08d5b3ed93f0af6d  DR1TOT_J
  5bcd5722c1892883b96a9d7fed0befadabacae313f800fadc0133cd5dd00c4c6  BIOPRO_J
  6bc5775797aafbd627316b6208418275fb8951cd571276e1a64436769afa16d5  FERTIN_J
  ```
- The table of `bean compare` starts each row with the date the sample was taken, as `bean ls` shows it. The rows are in order of that date, then of type. A script that read the table by column, or by a line that starts with `lab.`, reads it one column off. Saved comparisons do not change.

### 0.7.1 (2026-11-16)

One fault is mended. Nothing in a log changes.

- A comparison that you saved with a pack that was not on record says so in its caveats. When a later bean put that pack on record, a check of the comparison said `the claim MISSTATES this pack`, and `tools/compare/reproduce.py` said that it differs. Both were wrong: the caveat was true on its day. Now such a comparison reads with no mark, and computes again. Release 0.7.0 has this fault for a comparison that an earlier build saved with the pack of 20 measures.

### 0.7.0 (2026-11-16)

The first release with a tag. Nothing in a log changes.

- `bean version` says 0.7.0, and names the commit that the program was built from. Two builds of one version are told apart by it.
- A record that bean writes itself says `bean@0.7.0` in `source.adapter`. A comparison that you saved with 0.6.0 is not saved a second time.

- `bean compare` compares seven more tests: six of the biochemistry profile, and ferritin. It needs the files `BIOPRO_J` and `FERTIN_J` in the log ([docs/COMPARE.md](docs/COMPARE.md)). Without them, a reading of such a test now says `not comparable: needs BIOPRO_J`. Before, its type was named under `No reference data`. The line that lists the types with reference data is longer.
- A second reference pack is on record: 20 measures, from eight files. The first pack stays on record, and a comparison that you saved with it still checks.
- `bean import lab` keeps a report that it cannot read, with none of its values, and asks first. Before, it wrote nothing. `bean purge <id>` takes the file out, and bean then refuses that same file.

### 0.6.0

- `purge` no longer writes the import ref of the deleted record into the tombstone. Anyone with your identity secret could test guesses of the deleted reading against it. Only `local/purged-refs.jsonl` keeps the ref. Back that file up. Without it, a new import of the same source data brings the reading back. Tombstones from the build of 2026-10-30 keep `context.purged_ref`. Nothing reads it.
- `bean sync -max-mb` is gone. Sync only from your own copies.
- A rotation can no longer name a current or earlier signing key as its next recovery key. Nor can it name a recovery key already on the chain (`key reuse` or `recovery not advanced`, SPEC 5.8). bean never writes such a rotation. If another tool did, that rotation and the rotations after it stop counting. The key that it would have retired stays the head key, so its later records become valid. The records of the new key become invalid (`key not in chain`).
- `bean version` says 0.6.0. `export` writes `"spec": "bean-v0"` in `manifest.json`. Nothing reads that string.
- On an agent-only copy, `verify` fails with `own identity missing` when the person's genesis is gone.
- `gc` erases a target whose first tombstone became invalid while another valid tombstone still deletes it. It waits 7 days from that other tombstone's hold.
- A record of a too-new identity gets no `hidden pending gc` warning.

### Compare and share (2026-11-10)

- `sync` refuses a folder that is not a copy of your log. Read a bundle that someone sent with `check-bundle`.
- A correction or deletion of a reading deletes the saved comparisons of that reading. `purge`, `add -corrects`, `append`, `import`, `sync`, `import-export` and `verify` do this, and say how many.
- `verify` and `gc` refuse extra arguments with exit 2. Before, they ignored them.
- `bean-import-lab` 0.5.0 reads Quest Diagnostics reports.

### Rules N1 to N4 (2026-10-30)

Four rules changed. bean never wrote a record that the new rules reject. Most effects make records invalid, but some make records valid. A record of an unknown version that is not in canonical form no longer halts its identity. The records and tombstones of that identity count again. A deletion with a small-order signature is now invalid, so its target comes back. The check above shows both kinds.

- N1: one time syntax (SPEC 3.5). A time is `YYYY-MM-DDTHH:MM:SS`, an optional dot and 1 to 9 digits, then `Z` or `±HH:MM`, with real ranges. A comma, the offset `+24:00`, the year 0000 and more than 9 fraction digits are now invalid. `bean add -at`, `bean mcp` and `bean-import-csv` take only this form. The CSV importer keeps other time text in `context.raw_time`.
- N2: Ed25519 as libsodium checks it (SPEC 4.4, 4.5). A key that is not the canonical encoding of a curve point, or that has small order, is `bad key`. A signature whose R has small order is `bad signature`. Random keys never hit these cases. The object cache of an older build is dropped once.
- N3: the canonical check comes before the version check (SPEC 3.8 steps 13 and 14). A file of a newer `v` that is not in canonical form is `not canonical`. It does not sync and cannot halt an identity.
- N4: a signing key is never a recovery key (SPEC 5.8). A genesis whose `value.key` is its own recovery key is `key reuse`. So is a succession whose `value.key` is the key that signs it or its own next recovery key.

### Too new and output (2026-10-30)

- `verify` exits 3, and `gc` refuses, only when your own identity has records from a newer bean. Another too-new identity, for example a stranger's genesis of a newer format, only prints `too new: <id>`.
- `reindex` and `local/cache/index.json` are gone. Nothing read the file. You can delete it.
- Verify output changed, but no record changed between valid and invalid:
  - a correction with missing targets prints one `held` line, and none when it is invalid;
  - a retired key's deletion that the rotation did not seal is `not sealed`, also when the identity is frozen;
  - a too-new identity gets no other lines;
  - a tombstone with blobs names `blobs`;
  - a record of an `hlog.*` core type says only `old format`.

### Lab importer and onboarding (2026-11-09)

- `bean import labcorp` is now `bean import lab`, and the binary is `bean-import-lab` (adapter `bean-import-lab@0.4.0`). Readings keep `source.system` `labcorp`, so an import made before dedupes with one made now.
- `bean import` shows what it read and asks before it saves. In a script, pass `-yes`. Without a terminal and without `-yes` it writes nothing.
- `bean mcp` offers `read_blob` only with `-blobs`, and `log_entry` refuses lab results. `bean restore -export` rebuilds a log from an export.

### Seventh audit (2026-11-07)

Reads change only on frozen logs and for a reading whose correction is hidden; no record changes between valid and invalid (DECISIONS 203 to 224).

- `purge` says when a file stays: `the file <id> is kept: N other records name it`, and a second line for deleted records that wait for `gc`, records about someone else and damaged files. It can now purge a record of a newer format.
- `purge` and `gc` remove a sync tool's conflict copy of the deleted record (`name (conflicted copy).json`, `.sync-conflict-…`). A file under such a name never keeps a blob.
- `export` lists `withheld_records` (damaged files a deletion names, left out) and `missing_blobs_of_others` (a count). A record of a newer format unlocks a file only when the right key signed it. That key is the key of the chain head, or a recovery key the chain commits to.
- `import-export` reads `manifest.json` as a stream: at most 1 MiB per JSON token, nesting at most 16.
- `bean import` exits 128 plus the signal number when stopped, and stops the importer's whole process group. The Labcorp importer reads a report by a strict grammar (0.3.0). It refuses lines it does not know, including tabs and non-ASCII. It also refuses a name that may hold its value, and a value it does not know. Try it on your own report first.
- A key paired under two names, from two of your machines, answers to either name.

- On a frozen log, a deletion signed by a key below the fork no longer hides every record it names. A record that no freeze rule puts in doubt, such as an agent's reading, stays visible.
- A reading whose only correction is hidden shows again, with a status that says why.
- `sync` fails with `no log in <src>` when the source holds no log, and exits 1 with `skipped N bad record files, M bad blobs` when the source is damaged. It judges deletions on both copies together, and it repairs a damaged file even when its size and time are unchanged.
- `export` writes record files that fail their checks to `invalid/`, and files no record names to `blobs/` (`unreferenced_blobs`). It decrypts a file only for a record of yours or of a paired agent. A file that only a stranger's record names is listed in `not_exported_blobs`.
- `import-export` needs `manifest.json`, refuses an export of another identity, and refuses records you purged (`refused N purged`). Hand-made export folders need a manifest.
- A `changed:` line names the input's type, and says why when `-corrections` cannot write it either.
- One agent key per machine has one label. `mcp -write -label B` or `agent pair -label B` on a key paired as A is refused; use the same label or none. `mcp -write` without `-label` keeps the key's name.
- `blob put` refuses a file over 256 MiB. A bad time in `ls -since`, `ls -until` or `add -at` exits 2. `ls` shows a kept file as `file <media type> <hex>`, not `series`.
- Importers: Labcorp 0.2.0, Apple Health 0.2.0 and CSV 0.6.0 name the kept file by digest (`report_sha256`, `export_sha256`, `file_sha256`), not in `blobs`. So purging the file record erases the file. Readings from older versions still name the file in `blobs`, so the file stays until they are purged too. Labcorp refuses a name that runs into the result column, rows before the header, and a duplicate test that differs in comment or lab. Apple Health keeps exports with correlations, and stores percentages as fractions with unit `1`. `bean import csv` works.
- `bean import` checks the importer's first record and your deletions before it stores the file, so a file you purged is not stored again.

### Re-audit fixes (2026-11-07)

No validity rule changes: `bean verify` prints the same lines for the same files. Commands change:

- `rotate` and `restore` need `-seal-all` once the old key has signed anything. Without it they show what the old key signed and stop. A wrong `-no-seal` id is now an error. Update your scripts.
- An import with a new value for a reading you already have now says so and exits 1. It used to skip it in silence. `append -corrections` writes it.
- `ls`, `show`, `export` and the MCP tools exit 3 for a too-new identity instead of showing nothing. For a frozen identity they warn and exit 4.
- `export` writes everything it can, also past a missing stored file, and records of a newer format.
- For a frozen identity, reads show what both sides kept, and `ls -all` marks the rest. New: `bean migrate`, for a frozen identity only.

### Change notes C25 to C29 and K1 (builds from 2026-10-30)

If you never ran `rotate` or `restore` on your log, nothing changes for you.

- C25: erasure follows record files, not validity. `purge` and `gc` erase a blob only when they erase a record that names it and no other record file names it.
- C26: a retired key writes nothing valid. A succession seals every record of the outgoing key. A record of an older key that the next succession does not seal is `not sealed`. On a log that was rotated with an earlier build, the records of the old key were not sealed, so they are now `not sealed`.
- C27: duplicate groups include the author. A record of one key is never a duplicate of a record of another key.
- C28: seal continuations. A rotation that must seal more than about 200,000 records writes `bean.seal` parts. Builds before C28 cannot read such a rotation.
- K1: only the head key, or a recovery key that the chain commits to, can make an identity too new. Other records of a newer format are `ungated`.
- C29: `rotate` and `restore` go past a halt by the head key.

### The rename to bean (2026-10-28)

A log from before the rename (types `hlog.*`) fails with `old format`. Make a new log with `bean init`.
