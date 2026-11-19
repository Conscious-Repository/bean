# Receipts for a source file

A receipt is a record that exists already. `bean import nhanes <file>` writes one `reference.file` record: this identity imported a file with this SHA-256 and size, under this dataset's name, at this time.

`tally.py` reads bundles of such records, and lists what their keys state. [POOL-PLAN.md](../../docs/POOL-PLAN.md), part 3, says what that may claim.

**It counts keys, and a key is free.** It does not show that anyone fetched a file. It is never a confirmation.

## What a receipt states, and what it does not

All four things are the signer's word: the SHA-256, the size, the dataset's name and the time.

- A record can be written with no file at all. `bean append` writes one, and bean's own code holds the digests of the survey's files.
- bean fetched nothing. So it cannot check a URL, or the day of a download.
- A receipt exists only for a file that `bean import nhanes` takes. Those are the files of one cycle of one survey.

## If you are asked for a receipt

You run these at your own terminal. An assistant does not run `bean import` on your log.

1. Fetch the file with your own tool, from the source's own page.
2. `bean import nhanes <file>`. bean keeps the file in your log, and signs one record. It prints the SHA-256.
3. `bean ls -type reference.file` lists the record. Its line has two ids: the first is the kept file's, and the second is the record's.
4. `bean share -for <the asker's address> -address-words "…" -out <new file> <id>`. Add `-files` to carry the file as well.

`bean share -for` is in bean 0.8.0 and later.

To state where and when you fetched it, add a note:

    -note "fetched <URL> on <day>"

A share has one note, and a tally shows it under each record of the share. So share one record in a share that has such a note.

**What the bundle shows of you.**

- Your identity id and your identity records, with the second at which you made your log.
- The time of the share, with your offset from UTC.
- The time of each import, and the size of each file.
- Your note. A tally of the bundle quotes it. A URL or a day in it travels with the bundle.
- Your key, and the version of bean and of the importer: each record carries them.
- A pairing of an agent, with the agent's key and label, when that agent signed a record of the share.

The bundle is sealed to the asker's address for its way there. The asker opens it, may show it or pass it on, and cannot give it back.

The import keeps each file in your log. `bean purge` of it is final.

`bean share` shows every record and every field that leaves, and asks, before it writes. Read that list.

You may use an identity that you made for this alone.

## The tally

    python3 tools/receipts/tally.py <folder of opened bundles>

It reads opened bundles: one bundle in each folder below the folder that it is given. Whoever was sent a sealed package opens it first: `bean check-bundle -key <key file> -unpack <new folder> <package>`.

- It checks each bundle with `tools/second/checkbundle.py`. A bundle that fails is refused whole.
- It takes only records that the identity's own key signed.
- It takes a SHA-256 only as 64 lower-case hex digits, and a dataset's name only in the form that bean's importer has. Anything else is listed as malformed, and not counted.
- For each dataset and SHA-256, it lists the identities that state it, by their eight words. An identity counts once, in however many bundles.
- The lines are in the order of the datasets and of the digests, and never of the counts.
- It prints no total over a dataset, and no percentage.

**Two lines for a record.** `States only.` Or that the share record lists a file for this record with the stated SHA-256. The tally computes no SHA-256 itself: the check of the bundle computes it from the file, in the same run. Nothing shows that it is the file that the signer kept, or a file of the survey.

**Two marks.** An identity that states two digests for one dataset is marked under each. A digest that is in the table of bean's code is marked, with the dataset that the table gives. That table is from the project's own fetches of November 2026. A match proves no fetch, and does not show that the digest is right: a statement that was copied from the table matches too. The mark also says when the table has the digest for another dataset.

**In quotation marks stands only what the tally did not write.** That is a note, a malformed name or digest, a folder's name, and the reason that the check of a bundle gives. Outside them stand only the tally's own words. **The note** is quoted as the signer's word, cut at 200 characters. The tally's mark for a cut stands after the closing quotation mark, so a note cannot copy it. A note or a value that is no text is not put in quotation marks: its signer wrote no such words. Every character but plain ASCII is written out, and so are a quotation mark and a backslash. So a note cannot end its quotation, show as blank, or look like a line of the tally.

It says each time what it does not show:

- It counts keys, and a key is free.
- A statement can be copied with no fetch.
- It cannot show that it was given all the bundles.
- It does not show that the source served these bytes, and nothing about whether the data is true.

## The test

    make pool

`make pool` runs it after the test of the pool path. `bash tools/receipts/run.sh` runs it alone.

`run.sh` makes a fixture of made-up identities, and runs the tally on it. Every note in it says `synthetic`.

The fixture is chosen so that nobody learns that the larger count is right. Six identities state a test file with one changed byte. Four state the unchanged test file.

| What is handed in | What must happen |
|---|---|
| Three identities that state the changed file, and carry it | They count, with two more below. |
| One of them, in two shares with one note | It counts once, and its line prints once. |
| One of them shares a note record beside its receipt | The note record is no record of a source file. |
| One of them, in a folder whose name has a line end | The name is written out on one line. |
| One share, in two folders | It is one share. |
| One identity that states the unchanged file, and carries it | It counts. |
| One that states a digest of bean's table, with no file | `States only.`, and the mark of the table. |
| One that states both files, in two bundles | It is listed under each digest, and marked. |
| One that states both digests in one share, with no file | The same. The note stands under both, and says that the tally took two records of that share. |
| One that imports the unchanged file, and states its digest again in a record with no file | It counts once. The second record gets `States only.` |
| One that shares its record without the file | It counts, with `States only.` |
| A digest in upper case, one with two more letters, and one that is a number | Malformed, and not counted. |
| A dataset's name with a space, one with another character at its end, and one that is a number | The same. The line of the share says how many of its records are malformed. |
| A digest of the table under another dataset's name, and one of a second dataset | Each is listed, and the mark says for which dataset the table has it. |
| A statement that an agent of the identity signed | Not counted, and the line says so. |
| A bundle whose carried file has other bytes | The bundle is refused whole. |
| A bundle with one more file, whose name copies a line of the tally | The same. The name stays inside the quotation of the reason. |
| A note with a quotation mark, a backslash and blank characters | Each is written out. |
| A file among the folders | It is named, and not read. |

## What this does not show

- That a key is a person. One script made every key here.
- That a count means anything. Whoever wants a larger count makes more keys.
- That the tally is right. One program counts, and no second one.
