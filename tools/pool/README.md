# A synthetic pool: a test of the path

This folder builds the pack of a pool from made-up contributors. It is a test of the path, and no pool of people. [POOL-PLAN.md](../../docs/POOL-PLAN.md) says what it is for, and what it may claim.

**Do not run the builder on shares of real people.** It writes an archive that shows every record of every bundle. The flag `-synthetic` and the note below are words: they stop an accident, and prove nothing.

bean does not diagnose. Nothing here says whether a value is healthy.

## Run it

    make pool

It needs Go, Python 3 and the packages of `tools/requirements.txt`. It uses a scratch folder of its own, and never your log.

It prints one line when both tests pass. A line that starts `FAIL:` names what did not hold.

## What is here

- **`build.py`** is the builder. It reads opened bundles, and writes an archive and a pack.
- **`run.sh`** is the test. It makes the made-up contributors, plays the operator, and runs the builder.
- **`vector/`** holds three committed files that pin the builder.

## Two words

The operator is whoever runs the test: here, `run.sh`. The operator makes an address with `bean address -out`, and holds its key file.

The builder is `build.py`. It holds no key, and reads opened bundles only.

## What a contributor does

Each made-up log gets one reading, one saved comparison with a survey pack, and one more record. It shares the comparison, sealed to the operator's address. The command below is for a made-up log only. A person never shares so:

    bean share -for <address> -address-words "…" -yes -note "synthetic contributor" -out <new file> -type 'comparison.*'

The share holds the comparison and its reading. It holds no other reading of the log. It also holds the identity's records, and the time of the share with its offset from UTC.

The note is for a made-up log only. On a share of a person it would be false. It is this builder's only gate against a share of a real person.

`-yes`, address words read from the operator's own output, and one key file for every sender are for made-up logs only. A person never shares so.

## What the builder does

    python3 tools/pool/build.py -synthetic -survey <survey pack> -survey-url <URL> -archive-url <URL> -out <new pack> -archive <new archive> <folder of opened bundles>
    python3 tools/pool/build.py -synthetic -survey <survey pack> -survey-url <URL> -archive-url <URL> -out <new pack> <archive>

It stops without `-synthetic`. With it, the operator says that every contributor is made up. The builder cannot check that, and the pack says so.

- It checks each bundle with `tools/second/checkbundle.py`.
- It computes each claim again with `tools/compare/reproduce.py`, and the survey pack.
- It checks its own pack with `tools/pack/check.py`.

Its own rules are in [POOL-PLAN.md](../../docs/POOL-PLAN.md), under "The builder". It prints at least one line for each identity or folder: taken, or left out and why, with the folder's name. For every problem of a bundle that fails its check, run `tools/second/checkbundle.py <folder>`. A line on a claim that does not compute again holds the value of its reading, as the check of a claim prints it.

An opened bundle is what `tools/second/unseal.py -key <key file> <package> <new folder>` writes. `run.sh` does that for each package.

**An archive shows every record of every bundle in it, whole:** each value, each identity id and key, and each time. The builder says so before it writes one.

Of a bundle that it takes, the builder reads the record files under `objects/`. The archive holds those, and nothing else of the folder. It does not hold the `README.txt` that bean writes into a bundle, or a file whose name starts with a dot.

From a folder it writes the archive first, and then the pack that names it. From an archive it writes no archive. It stops unless it takes every bundle in that archive, and unless the file is the archive that it writes of them, byte for byte.

It writes over nothing.

## What the pack says of itself

- A cohort's id starts with `pool-`, and its text with `synthetic pool:`.
- Its title says the same. Its source's name, publisher and home say `none: synthetic`.
- Its caveats are the builder's own. They say that n counts identity ids, and not people.
- Its `source.files` names two files: the archive as `pool-bundles.zip`, and the survey pack that the claims name as `survey.pack`.

bean has no record of such a pack. `bean compare` takes it only with `-unknown-pack`, and says so with each result.

bean's own lines under a result are wrong for this pack. It prints "fewer than 100 people" and "in this sample". Those are forty keys of one script. For the cell of one identity, bean prints that identity's value as the median and as the middle half. It says to compare the SHA-256 "with the one that its publisher states", and this pack has no publisher. The measure keeps the survey pack's own label.

## The test vector

`vector/` holds one archive of 41 made-up bundles, the survey pack that their claims name, and `pool.sha256`.

`make pool` runs the builder on the two files. The pack must have the SHA-256 that `pool.sha256` states. To do it by hand, take the line `URL=` of `run.sh`, and add `/SYNTHETIC-survey.pack` and `/pool-bundles.zip` to it. The two URLs are part of the pack's bytes. That is all that "reproducible" means here: the same program and the same two files give the same bytes.

**`SYNTHETIC-survey.pack` calls itself the survey's.** It was built from the made-up files in `internal/nhanes/testdata`, with the survey's own builder, which is not changed here. So its title and its publisher are the survey's. Its data is made up. It is here for this test alone.

The pool pack of the vector names the two files by their URLs in this repository. Those are the repository's own, and no host that anyone is told to use. The pinned SHA-256 changes if the repository moves.

**To make the vector again.** Each run makes new keys, so no run gives the committed archive again. A new vector is a new archive:

    POOL_VECTOR_OUT=<new folder> make pool

That writes the three files into the new folder. Copy them over those of `vector/`, and run `make pool` once more. Do it when the builder's rule changes, or the survey pack's bytes.

## What the test hands in, and what must happen

Forty good packages give one cell of forty identities. Some of the forty logs have one value, so the cell has weights above one. One more good package is alone in another cohort: it is taken, and its cell has one identity. A cell has no least number of them. Beside them:

| What is handed in | What must happen |
|---|---|
| A package that is damaged | It does not open, and the builder never sees it. |
| A bundle with a record whose signature is bad | The folder is left out: the bundle fails its check. |
| A claim with false numbers | The claim is not counted, and its identity is left out. |
| A claim that names another survey pack | The same. |
| One claim that counts, and one that does not, in one bundle | The identity is left out. |
| A claim whose reading is a list, and no id | The identity is left out, and the builder does not stop. |
| A claim with no context at all | The same. |
| A reading that an agent signed, and a claim that an agent signed | The same. |
| A reading whose value `pack/0` cannot hold: too large, or too small | The same. |
| A bundle with no note, and one with another note | The identity is left out, and the line names the folder. |
| A bundle with one more record than a comparison and its reading | The same. |
| Two different shares of one identity, and two of which one has no note | The identity is left out. |
| A file among the folders of bundles | It is named, and not read. |
| Two readings of one measure | The identity is left out. |
| One reading with claims in two cohorts | The identity is left out. |
| A reading with a photo, shared with its file | The identity is left out: the bundle holds more than comparisons and their readings. |
| A reading with a photo, shared without its file | The same: the reading keeps a file. |
| Files whose names start with a dot, in a good bundle | The bundle is taken. The archive does not hold them. |
| The same bundle twice | It is taken once. |
| One claim twice, with two times | Its reading counts once. |
| A folder that is no bundle, with a line end in its name | It is left out, and its name is written out on one line. |

The test also holds these.

- The record that was not shared is in no bundle of the archive.
- The same bundles in another order give the same archive and the same pack.
- The archive alone, with the survey pack, gives the same pack again.
- An archive with a path that leaves its folder is refused, and nothing is written outside.
- A bundle whose records are larger together than its limit is left out. The limit is 1 MiB, and the test holds it with a lower number.
- When the builder's own check refuses the pack, nothing is written: no archive either.
- An archive that is damaged or packed is refused. So is one with a link, with one entry twice, with an entry for a folder, with a comment, or with other bytes before it. So is one of a version that Python's reader does not know.
- When the pack cannot be written, the archive does not stay behind. A bundle that the check cannot read gets a line. The test holds both only for a user who is not root.
- A claim in letters that a plain terminal cannot show does not stop the builder.
- No run of the builder ends in a traceback.
- One more made-up log compares with the pack, and bean's output says `synthetic pool`.

## What this does not show

- That a contributor is a person. A key is free, and one script made every key here.
- That a share was meant for the operator. Nothing binds a share to its reader, and the builder reads opened folders.
- That the builder is right. One program builds the pack twice, and no second builder exists.
- That a contributor stays in. Whoever holds another share of an identity can hand it in, and that identity is then left out.
- That the limit of an archive's size holds at its real number, 64 MiB. The test holds it with a lower number.
