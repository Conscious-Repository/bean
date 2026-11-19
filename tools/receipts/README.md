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
3. `bean ls -type reference.file` shows the record's id.
4. `bean share -for <the asker's address> -address-words "…" <id>`. Add `-files` to carry the file as well.

`bean share -for` is in bean 0.8.0 and later.

To state where and when you fetched it, add a note:

    -note "fetched <URL> on <day>"

A share has one note. So share one dataset in a share that has such a note.

**What the bundle shows of you.**

- Your identity id and your identity records, with the second at which you made your log.
- The time of the share, with your offset from UTC.
- The time of each import, and the size of each file.
- Your note. Every tally of the bundle quotes it. A URL or a day in it travels with the bundle.

The bundle is sealed to the asker's address for its way there. The asker opens it, may show it or pass it on, and cannot give it back.

The import keeps each file in your log. `bean purge` of it is final.

You may use an identity that you made for this alone.

## The tally

    python3 tools/receipts/tally.py <folder of opened bundles>

It reads opened bundles: one bundle in each folder below the folder that it is given. Whoever was sent a sealed package opens it first.

- It checks each bundle with `tools/second/checkbundle.py`. A bundle that fails is refused whole.
- It takes only records that the identity's own key signed.
- It takes a SHA-256 only as 64 lower-case hex digits, and a dataset's name only in the form that bean's importer has. Anything else is listed as malformed, and not counted.
- For each dataset and SHA-256, it lists the identities that state it, by their eight words. An identity counts once, in however many bundles.
- The lines are in the order of the digests, and not of the counts.
- It prints no total over a dataset, and no percentage.

**Two lines for a record.** `States only.` Or that the bundle carries a file whose SHA-256, computed by the tally, equals the stated one. Nothing shows that it is the file that the signer kept, or a file of the survey.

**Two marks.** An identity that states two digests for one dataset is marked under each. A digest that is in the table of bean's code is marked, with the dataset that the table gives. That table is from the project's own fetches of November 2026. The mark is no sign of the right digest.

**The note** is quoted as the signer's word, cut at 200 characters. Every character but plain ASCII is written out, and so are a quotation mark and a backslash. So a note cannot end its quotation, show as blank, or look like a line of the tally.

It says each time what it does not show:

- It counts keys, and a key is free.
- A statement can be copied with no fetch.
- It cannot show that it was given all the bundles.
- It does not show that the source served these bytes, and nothing about whether the data is true.

## The test

    make receipts

`run.sh` makes a fixture of made-up identities, and runs the tally on it. Every note in it says `synthetic`.

The fixture is chosen so that nobody learns that the larger count is right. Four identities state a test file with one changed byte. Two state the unchanged test file.

| What is handed in | What must happen |
|---|---|
| Three identities that state the changed file, and carry it | They count, with a fourth below. |
| One of them, in two shares | It counts once. |
| One share, in two folders | It is one share. |
| One identity that states the unchanged file, and carries it | It counts. |
| One that states a digest of bean's table, with no file | `States only.`, and the mark of the table. |
| One that states both files, in two bundles | It is listed under each digest, and marked. |
| A digest in upper case, and a dataset's name with a space | Malformed, and not counted. |
| A statement that an agent of the identity signed | Not counted, and the line says so. |
| A bundle whose carried file has other bytes | The bundle is refused whole. |
| A note with a quotation mark, a backslash and blank characters | Each is written out. |
| A file among the folders | It is named, and not read. |

## What this does not show

- That a key is a person. One script made every key here.
- That a count means anything. Whoever wants a larger count makes more keys.
- That the tally is right. One program counts, and no second one.
