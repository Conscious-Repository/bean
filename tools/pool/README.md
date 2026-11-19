# A synthetic pool: a test of the path

This folder builds the pack of a pool from made-up contributors. It is a test of the path, and no pool of people. [POOL-PLAN.md](../../docs/POOL-PLAN.md) says what it is for, and what it may claim.

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

Each made-up log gets one reading, one saved comparison with a survey pack, and one more record. It shares the comparison, sealed to the operator's address:

    bean share -for <address> -address-words "…" -yes -note "synthetic contributor" -out <new file> -type 'comparison.*'

The share holds the comparison and its reading. It holds no other reading of the log.

`-yes`, address words read from the operator's own output, and one key file for every sender are for made-up logs only. A person never shares so.

## What the builder does

    python3 tools/pool/build.py -synthetic -survey <survey pack> -survey-url <URL> -archive-url <URL> -out <new pack> -archive <new archive> <folder of opened bundles>
    python3 tools/pool/build.py -synthetic -survey <survey pack> -survey-url <URL> -archive-url <URL> -out <new pack> <archive>

It stops without `-synthetic`. With it, the operator says that every contributor is made up. The builder cannot check that, and the pack says so.

- It checks each bundle with `tools/second/checkbundle.py`.
- It computes each claim again with `tools/compare/reproduce.py`, and the survey pack.
- It checks its own pack with `tools/pack/check.py`.

Its own rules are in [POOL-PLAN.md](../../docs/POOL-PLAN.md), under "The builder". It prints one line for each identity or folder: taken, or left out and why.

From a folder it writes the archive first, and then the pack that names it. From an archive it writes no archive. It stops unless it takes every bundle in that archive.

It writes over nothing.

## What the pack says of itself

- A cohort's id starts with `pool-`, and its text with `synthetic pool:`.
- Its title says the same. Its source's name, publisher and home say `none: synthetic`.
- Its caveats are the builder's own. They say that n counts identity ids, and not people.
- Its `source.files` names two files: the archive, and the survey pack that the claims name.

bean has no record of such a pack. `bean compare` takes it only with `-unknown-pack`, and says so with each result.

bean's own lines under a result are wrong for this pack. It prints "fewer than 100 people" and "in this sample". Those are forty keys of one script.

## The test vector

`vector/` holds one archive of forty made-up bundles, the survey pack that their claims name, and `pool.sha256`.

`make pool` runs the builder on the two files. The pack must have the SHA-256 that `pool.sha256` states. That is all that "reproducible" means here: the same program and the same two files give the same bytes.

**`SYNTHETIC-survey.pack` calls itself the survey's.** It was built from the made-up files in `internal/nhanes/testdata`, with the survey's own builder, which is not changed here. So its title and its publisher are the survey's. Its data is made up. It is here for this test alone.

The pool pack of the vector names the two files by their URLs in this repository. Those are the repository's own, and no host that anyone is told to use. The pinned SHA-256 changes if the repository moves.

## What the test hands in, and what must happen

Forty good packages give one cell of forty identities. Beside them:

| What is handed in | What must happen |
|---|---|
| A package that is damaged | It does not open. The script names it, and the builder never sees it. |
| A bundle with a record whose signature is bad | The folder is left out: the bundle fails its check. |
| A claim with false numbers | The claim is not counted, and its identity is left out. |
| A claim that names another survey pack | The same. |
| A reading that an agent signed, and a claim that an agent signed | The same. |
| A reading whose value `pack/0` cannot hold | The same. |
| A bundle with no note, and one with another note | The folder is left out. |
| Two different shares of one identity | The identity is left out. |
| Two readings of one measure | The identity is left out. |
| One reading with claims in two cohorts | The identity is left out. |
| The same bundle twice | It is taken once. |

The test also holds these.

- The record that was not shared is in no bundle of the archive.
- The same bundles in another order give the same archive and the same pack.
- The archive alone, with the survey pack, gives the same pack again.
- An archive with a path that leaves its folder is refused, and nothing is written outside.
- One more made-up log compares with the pack, and bean's output says `synthetic pool`.

## What this does not show

- That a contributor is a person. A key is free, and one script made every key here.
- That a share was meant for the operator. Nothing binds a share to its reader, and the builder reads opened folders.
- That the builder is right. One program builds the pack twice, and no second builder exists.
