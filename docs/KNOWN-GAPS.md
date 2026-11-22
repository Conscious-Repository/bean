# Known limits and gaps

What bean does not do well today, said out loud. [ROADMAP.md](ROADMAP.md) says what comes next. Each gap names the entry of [DECISIONS.md](DECISIONS.md) that found it.

## Limits

- **The lab importer is proven on one real report of each layout.** Thirteen attack rounds found ways to make it store a wrong value, fewer each round, but not none. It refuses what it cannot read, so it refuses unfamiliar layouts, and it shows what it read before it saves.
- **A signature proves who signed, not that the number is right.** It does not prove that a lab or device produced the number, nor anyone's credentials.
- **A shared subset proves authenticity, not completeness.** Record files are readable JSON; only blobs are encrypted. Share by bundle, never by copying the log folder. The subject id is stable, so two bundles from one person can be linked.
- **Regulatory.** Storing, verifying and displaying are kept apart from interpreting. A percentile against public data is a function that needs its own review before any clinical use. bean does not interpret, predict or advise.
- **A comparison is as good as its cohort.** NHANES is one country, one two-year window, about 9,000 people. A lab and the survey measure with different instruments. A narrow cohort has few people, and no interval of uncertainty is computed yet. Every result says its n and its caveats.

## Known gaps

- After `purge` and `gc`, the summary counts the tombstone as `1 held` for good. This is what the spec says, but it reads like a fault.
- From the re-audit of `ff4ef77` (DECISIONS 194):
  - A removed record or a removed newest succession goes unnoticed; the signed sealed lists could catch part of it.
  - A mode-000 file from rsync stops every command, and a 1 GiB sparse file costs 1 GiB per load.
  - A stranger's incomplete seal set blocks `gc`.
  - A retired key as a bare author stays current.
  - A correction undone at rotation keeps its purged ref. Two corrections of one target both show when they come from two copies; `bean add -corrects` refuses the second on one copy.
  - After a thief's link in a chain of `-corrections` is refused, the two records left can become duplicates, and the lower id wins.
  - Purging a correction brings the older value back. `-corrections` cannot tell an older report from a newer one.
  - Two current records with one ref migrate as one: the other is counted as left out, but `migrate -list` does not show it.
  - While a purge with an erased target exists, undoing another deletion keeps its purged ref, so a later import of that reading is skipped.
  - `stats` fails on values whose sum overflows.
  - Importers: a day-precision date lands in the wrong local day, and a note line can become a lab value (DECISIONS 201).
- From the seventh to thirteenth audits (DECISIONS 203 to 229):
  - `sync` proves every file of this copy on each run, which costs about as much as `verify`.
  - `sync` does not copy a blob that no record names; only `export` carries it.
  - `export` does not decrypt the files of an agent that the person unpaired.
  - A source with a stranger's broken file makes every `sync` from it exit 1.
  - The purge check in `bean import` reads only this copy's `local/purged-refs.jsonl`.
- From the reviews of reference packs (DECISIONS 283, 284):
  - A check of a bundle does not compute a comparison again. A claim with false numbers that names the pack on record reads like a true one there. `reproduce.py` with the pack finds it (path 4, step 3).
  - The Python check of a bundle, `checkbundle.py`, does not mark a comparison's pack. `reproduce.py` does.
  - The list of packs on record is in bean's code. A second publisher needs a signature, not a line in that list.
  - A pull request's build of the check page names the merge commit, which nobody can build again. The build on `main` and on a tag names a real commit.
  - A cell's n is not checked against its weights: with sample weights the two are not tied.
  - Two packs can place one reading differently, and both claims are kept. Each row names its pack.
  - `bean compare -pack <file>` needs a log even to show what a pack holds.
  - `bean add` with a whole number above 9007199254740991 says `bad json`.
- From the reviews of the share review (DECISIONS 324):
  - `view.Safe` shows a space other than U+0020, a combining mark and a few rare blank characters as they are. A value can look blank, struck through, or like another value. It cannot hide a name: the letters are shown.
  - A field of more than 50 different numbers or times shows its count. Many numbers could still spell a message.
- From the reviews of the plan for reference data (DECISIONS 321):
  - For a pack in a format that it does not know, bean says that the file is not a pack of version 0.
  - `bean purge` of the record that keeps a file erases the file, also when a saved comparison is about that record. The same file cannot be kept again on that copy.
  - A pack that is not in canonical form is accepted, as another pack.
  - Nothing marks a pack on record that is later found wrong.
- No way to test the 24 words without rotating. `net/http` is in the binary only to detect image types. CI does not run `lint`, `vuln` or fuzzing.
