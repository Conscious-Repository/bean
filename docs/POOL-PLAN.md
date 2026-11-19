# A demonstration of the pool path, and receipts for a source file: what is decided before it is built

This is for anyone who builds or reviews the demonstration. It says what will be built, in three parts, and what each part may claim. None of it is built yet. DECISIONS 332 has the account.

**It is a test of the path with made-up people.** It is not a pool, and it is not a host. bean does not diagnose, and nothing here says whether a value is healthy.

**It changes nothing in bean.** No command, no format and no record type changes. Each part is a script, a tool under `tools/`, a test vector or a document.

## The need

The owner asked for it on 2026-11-18, in these words.

```
Over the next four days, prioritize a working end-to-end BEAN demonstration in which independent
users contribute selectively shared, cryptographically signed synthetic health records to a pool
builder, which verifies and aggregates them into a reproducible reference pack, hosts it on Blossom,
and publishes a signed Nostr kind 1063 event so another user can discover, download, verify, and
compare against it locally. Additionally, implement signed source-download attestations: when users
independently download public datasets (e.g., from the CDC), BEAN records the source URL, timestamp,
and SHA-256 hash and signs a receipt. Because identical file bytes produce identical hashes, 100
independent downloads producing the same hash provide corroborating evidence that the source
consistently distributed those bytes. Over time, these attestations should support a decentralized,
Sybil-resistant web of trust for data-source provenance and reputation, while distinguishing
distribution consistency from scientific truth. Follow Linus, Satoshi, and Build Tall principles:
minimal core changes, explicit verification, open standards, reproducibility, and working systems
over speculative complexity. Use synthetic contributors and existing BEAN primitives; defer
homomorphic encryption and production-grade reputation scoring.
```

He was asked two things. Does a receipt stay the record that bean already writes, with no address or time of a download in it? Would he install one program, `nak`? He gave one answer to both: "common sense that adheres to my values". He then installed `nak` himself.

So the author made the choices below by the three paragons. Each is marked as the author's, with what would give the owner more. He can change any of them.

## What is measured

Each fact was checked in the code. A reviewer, or the author, ran each one with made-up data, but for F15, which was read.

**F1. The whole path runs with no change to bean.** A pack was built from the files in `internal/nhanes/testdata`. Forty logs each got a reading, a saved comparison with that pack, and one share to an address. That took 3.9 seconds, with no terminal. `unseal.py` opened each package, `checkbundle.py` passed each bundle, and `reproduce.py` computed each claim again.

**F2. A claim cannot be computed again without its survey pack.** Given a bundle alone, `reproduce.py` says `CANNOT CHECK`.

**F3. The same bundles gave the same pack bytes under these conditions only.** The same names. Zip entries in one order, with fixed times and modes. Folders named by the id of the share record. Whole numbers written as whole numbers.

**F4. A pool pack that copies the survey pack's caveats names NHANES and its instruments.** Under a result from a cell of forty, bean also prints `N 40`, "fewer than 100 people" and "in this sample".

**F5. The survey pack from the testdata calls itself the survey's.** Its title, publisher and URLs are the survey's own, beside the digests of test files. Its caveats say that its files are not the files on record. They do not say that the data is made up. The builder of that pack is not changed here.

**F6. A fresh run gives a new pack each time.** `bean init` makes new keys, and each record has a random part.

**F7. `pack/0` takes two entries in `source.files`.** Each has a name, a SHA-256, a size and a URL. The URL is free text, and is part of the pack's bytes.

**F8. `nak` 0.22.3 does the network leg on one machine.** `nak serve --blossom` starts a test relay with a Blossom server. `nak blossom upload` stored a file, and `curl` fetched the same bytes by their SHA-256. `nak event -k 1063` published a signed event, `nak req` found it, and `nak verify` passed it.

**F9. `nak` takes a key from the variable `NOSTR_SECRET_KEY`.** With no key it signs with a default key of the machine. `curl` can fetch from a Blossom server. An upload needs a signed event, which `curl` cannot make.

**F10. A signed record of a source file exists.** `bean import nhanes <file>` writes one `reference.file` record. It has the SHA-256 that bean computed, the size, the dataset's name and the time of the import. `bean share <id>` sends such records without their files, and `-type reference.file` is refused.

**F11. Such a record proves no fetch.** `bean append` wrote one with the project's own digest and no file, and its bundle passed. `BEAN_NOW` set its time to 2019.

**F12. A bundle whose carried file has another SHA-256 than its record states fails its check.** That holds when the record has one stored file and names it. Which carried file belongs to which record is the signer's own entry in the share record.

**F13. `bean share -note` puts one signed text into the share record.** It is free text, and one note for the whole share.

**F14. What a bundle of such records shows of its sender.** Their identity id and identity records, with the second at which they made their log. The time of the share, with their offset from UTC. The time of each import and the size of each file. The import keeps each file in their log, and `bean purge` of it is final.

**F15. A job of the repository fetches the survey's files again** and checks each digest. No second person has stated them.

## Rules that stay

- bean fetches nothing (DECISIONS 286, 328).
- A pointer is a hint, and never a second place for a SHA-256 (ROADMAP, "What a pointer may claim").
- A reason that a signer declares is the signer's word (DECISIONS 317).
- 0.9's status says first that no pool exists, and that no other entity hosts a pack (DECISIONS 328).
- A line that a tool prints claims only what the tool checked.

## Part 1: the pool path, offline and in CI

It is `tools/pool/` and `make pool`.

**Two words.** The operator is whoever runs the test: here, a script. The operator makes an address with `bean address -out` and holds its key file. The builder is one Python program, and holds no key.

### The script of contributors

- It makes forty logs in a scratch folder of its own. It does not use the caller's `BEAN_DIR`.
- Each log gets one reading, a saved comparison with the survey pack, and one more record that is not shared.
- Each log shares once: `bean share -for <address> -address-words "…" -yes -note "synthetic contributor" -out <new file> -type 'comparison.*'`.
- Its header says what is for made-up logs only: `-yes`, address words read from the operator's own output, and one key file for forty senders.
- It opens each package with the key file (`unseal.py`). A package that does not open is named and left out. That line is the script's.
- What it hands on is a folder of opened bundles. From there on, nothing can tell a share that was meant for the operator from one that was passed on.

### The builder

- **It reads opened bundles only,** as a folder or as an archive. So it has one path.
- **Its inputs are named.** The bundles. The survey pack. A URL for the archive and one for the survey pack, with no default. And a flag `-synthetic`, without which it stops.
- **It prints the SHA-256 of the survey pack that it was given.**
- **It checks each bundle** (`checkbundle.py`), and computes each claim again (`reproduce.py`). For that it names the survey pack by its SHA-256, with `-accept-pack`, since bean has no record of that pack. It gives `reproduce.py` a bundle's `objects` and `files`, and nothing else of the folder. It adds no check of a bundle or of a claim. It has one rule of its own: a reading whose value `pack/0` cannot hold is left out, and named.
- **Whose records count.** A claim and its reading count only when the identity's own key signed both. An agent's record does not count.
- **The note is a tripwire.** A bundle is left out unless its share's note is exactly `synthetic contributor`. Anyone can write that note. It guards against an accident, and proves nothing.
- **One reading for each identity and measure.** An identity with two different bundles is left out of the pool. So is an identity with two readings of one measure that count. So is an identity whose reading has claims in two cohorts. This is the test's rule, and the author's choice. What would give more: ROADMAP's rule, which takes the latest reading. ROADMAP keeps its own rule for a real pool, where a person must be able to correct a reading.
- **One bundle is one bundle.** A bundle is its `objects` and its `files`. Two folders with one share id that pass their check, and are the same in `objects` and `files`, are one bundle. It is taken once.
- **What it prints.** One line for each identity: taken, or left out and why. Each identity that it takes counts as one in its cell. It names each cell that it drops for having fewer than 30. A bundle that fails its check has no identity that can be trusted, so its line names the folder. The builder writes out safely every text that it quotes, so that no text can fake a line.

### The pack

- It is `pack/0`, unchanged.
- **It says what it is, where a reader sees it.** A cohort's id starts with `pool-`. A cohort's text starts with `synthetic pool:`. The title says the same. The source's name, publisher and home say `none: synthetic`.
- **Its caveats are the builder's own,** for the pack, for each measure and for each cohort. They say what the builder knows. "n counts identity ids, not people." "Whoever ran the builder said that every contributor is made up; the builder cannot check that."
- **Its measures keep the survey pack's ids, labels, units and `same` lists.**
- **A cell needs as many identities as `pack/0` asks: 30 today.** The builder reads that number from `tools/pack/check.py`, and keeps no copy of it. It leaves out a cell of fewer. The test vector puts 30 contributors or more into one cohort for one measure, and one cell is enough for the test.
- **A test holds this much.** No such text names the survey or its publisher: the title, the source's name, publisher and home, and the builder's name. Nor does a caveat of the pack, of a measure or of a cohort.
- **It names both of its inputs.** `source.files` has two entries: the archive, and the survey pack that the claims name.

### The archive

- **From a folder,** the builder writes the archive first, and then the pack that names it. The archive holds the bundles that the builder took, and no other. It holds each bundle's `objects` and `files`, and nothing else.
- It is written by the conditions of F3, with entries that are stored and not compressed. The builder sets every field of an entry that differs between systems. So the same bundles give the same bytes.
- **From an archive,** the builder writes none. The pack names the SHA-256 of the file as it was given. The builder stops unless it takes every bundle in that archive.
- The builder says, when it writes one, that an archive shows every value and every identity id in it.
- The builder reads an archive as `unseal.py` reads a package: no path that leaves its folder, no link, and a limit of size.

### Two tests, and what each is called

**"The pack is what this builder gives from this archive and this survey pack."**

- Three things are committed as a test vector. One archive of forty made-up bundles. The survey pack that their claims name. And the SHA-256 of the pool's pack.
- `make pool` runs the builder on the two committed files, and compares.
- The survey pack is committed under a name that starts `SYNTHETIC-` and ends `.pack`, with a note beside it. It is the one artefact here that cannot say of itself that it is made up (F5).
- The pool pack names the two files by their URLs in the repository. Those are the repository's own, and no host that anyone is told to use.
- This is what "reproducible" means here, and no more. The committed files have that one use: they pin the builder.

**"The path runs, and these are left out."**

- `make pool` runs the script of contributors with new keys, then the builder, `tools/pack/check.py`, and `bean compare -pack … -unknown-pack` from one more made-up log.
- Beside the good packages it hands in bad ones. Each must get its line, and stay out of the pack.
  - A package that is damaged.
  - A bundle with a record whose signature is bad.
  - A claim with false numbers.
  - A claim that names another survey pack.
  - A claim that an agent signed, and a reading that an agent signed.
  - A reading whose value `pack/0` cannot hold.
  - A bundle with no note.
  - Two different shares of one identity.
- The damaged one is a sealed package. The others are made as opened folders, since nothing seals a folder that was changed by hand.
- The same bundle handed in twice must be taken once.
- The record that was not shared must be in no bundle of the archive.
- A second build, with the bundles in another order, must give the same bytes.
- The output of `bean compare` must say `synthetic`.
- It pins no digest: new keys give a new pack each time (F6).

**Wiring.** `make pool` joins `make check`, and the list of targets in `.github/workflows/ci.yml`. It runs the tally of Part 3 on its fixture as well. New documents join `DOCS` in the Makefile.

## Part 2: a trial run of the network leg, on one machine

It is `examples/pool/`: a short script and a README. It is not in CI. A person runs it, and nothing in bean depends on it.

**One machine, and no real host: that is the author's choice.** The owner wrote "hosts it on Blossom". The reason is caution, and no more: a pack that is published cannot be taken back. He can order either of two things. One is a host that he names. The other is a relay and a Blossom server that others run, with a throwaway key and the same `synthetic pool:` words.

### The steps

1. **A relay and a file host.** `nak serve --blossom` on localhost, on a stated port. The script starts it, refuses any host but localhost, and stops it at its end.
2. **Keys.** The script clears `NOSTR_SECRET_KEY` first, and sets it for one command at a time. `nak key generate` makes a new key for each upload, and one for the pointer. Every command that signs gets such a key. So no key of a log is used, and `nak` never signs with the machine's key (F9).
3. **One log, for step 9.** The script makes it in a scratch folder of its own, with a made-up reading. It is the other user's log, and no comparison is saved in it.
4. **Upload.** The script uploads the archive and the survey pack with `nak blossom upload`. They are the two committed files. Then it runs the builder on them, with the URLs that the host gave the two, and uploads the pack.
5. **A pointer.** The script publishes one event of kind 1063 for the pack. Its content starts `synthetic pool:`. A key that was made for the relay signs it, and the script checks that the event carries that key.
6. **Find and fetch.** Another user finds the event with `nak req -k 1063`, and fetches the pack with `curl`. They compute its SHA-256 and compare it with the one in the event. That shows that the host served what the pointer named, and no more.
7. **The other two files.** They fetch the archive and the survey pack from the URLs that the pack names. They hold each to the SHA-256 that the pack states.
8. **Build again.** They build the pack from the archive and the survey pack, with the builder, and with the two URLs as the fetched pack names them. They compare the SHA-256 with the file that they fetched. This is the step that checks something.
9. **Compare.** They run `bean compare -pack … -unknown-pack`.

### What the script prints, and when

One script plays the operator and the other user. Only when the comparisons of steps 6 to 8 matched does it print what it checked:

- the host served the bytes that the pointer named;
- the pack is what this builder gives from the archive and the survey pack that the host served;
- whether those two files are the same bytes as the two files of the test vector in the repository. When they are not, it says that the pack was built again from other files than the vector's.

Then it prints what it cannot show:

- one operator ran the host and the relay, so the two agree by one hand;
- nothing shows who made the keys in the archive.

On any mismatch it prints `NOT reproduced`, and stops.

### What the README says

- **How to get `nak`,** and the version that ran.
- **What ties the files to the test vector.** A host that serves three files that agree with each other passes steps 6 to 8. Only the third check above ties the archive and the survey pack to the vector.
- **bean's fixed words are wrong here.** bean prints `N 40`, "fewer than 100 people" and "in this sample" under the result (F4). Those are forty keys of one script.
- **What the trial run is not.** It is not the document of a first setup that DECISIONS 325 asks for. That one is still owed before a first pack is announced. `nak`'s local server is a test server.
- **What else serves.** Finding the event needs a Nostr client, and `curl` is none. Fetching and building again need only the bytes and a SHA-256: any file host or a disk serves them. A file and its SHA-256 on a page is a full publication.
- **Without `nak`.** A person starts at step 8, with the files in the repository, and compares with the SHA-256 that the repository states.

## Part 3: receipts for a source file

It is `tools/receipts/`, and documents.

### A receipt is the record that exists

A person fetches a survey file with their own tool. They run `bean import nhanes <file>`, and share the record by its id.

**What the record states.** That this identity imported a file with this SHA-256 and size, under this dataset's name, at this time. All four are the signer's word: a record can be written with no file at all (F11).

**A receipt exists only for a file that `bean import nhanes` takes.** Those are the files of one cycle of one survey. The owner asked for public datasets, "e.g., from the CDC". For any other file there is no command. What would give it: a new importer, or a new record type. Each is a change to bean's core that he can order.

**The URL and the time of a download** are the signer's word, as the time of the import is. bean fetched nothing, so it cannot check either. A person who wants to state them writes `-note "fetched <URL> on <day>"` when they share. They share one dataset then, since a share has one note. This is the author's choice. It is less than the owner asked for: bean records neither. What would give it: a fetch in bean, or a member for a URL. Each changes the core or the record format.

### The tally

It is one small Python program. It reads bundles that are open: whoever was sent a sealed one opens it first.

- **What it takes.** It checks each bundle with `checkbundle.py`. A bundle that fails is refused whole, and its line names the folder and not an identity. It takes only `reference.file` records that the identity's own key signed.
- **Strict forms.** It reads the stated SHA-256 from the record. It takes a digest only as 64 lower-case hex digits, and a dataset's name only by the pattern that bean's importer has. Anything else it calls malformed, and writes out safely.
- **One line for each identity and dataset:** the identity's eight words, and the SHA-256 that it states. Then one of the two lines below.
- **"States only."**
- **Or:** "the share record lists a file for this record whose SHA-256, computed here, equals the stated one. Nothing shows that it is the file that the signer kept, or a file of the survey." That line is for a record with exactly one stored file, which it names. The tally computes the SHA-256 itself.
- **The note of the share,** in quotation marks, as the signer's word, with the number of datasets in that share. The tally cuts it at a stated length. It writes out control characters and line ends, as `bean check-bundle` does. It writes out quotation marks too, which `bean check-bundle` does not, and every character that shows as blank.
- **How it counts.** For each dataset and digest, the identities that state it. That count is printed with the lines of its digest. An identity counts once for a digest, in however many bundles or records. The same share handed in twice is one share.
- **Two marks.** An identity that states two digests for one dataset is listed under each, and marked, also across bundles. A digest that is in the project's own table is marked with the dataset that the table gives for it.
- **What the tally says of that second mark.** The table is the one in bean's code, from the project's own fetches of November 2026. The mark is no sign of the right digest. The tally loads the table from `reproduce.py` by its path, and keeps no copy.
- **Order.** Lines are in the order of the digests, not of the counts.
- **What it was given.** Each run lists the bundles that it was given: taken, or refused with the check's reason. It writes out safely every name of a folder and every reason.
- **What it never prints of its own:** a total over a dataset, or a percentage. Nor any word that says more than that a key signed a statement.

**What it prints of itself, each time.**

- It counts keys, and a key is free.
- A statement can be copied with no fetch: bean's own code holds the digests.
- It shows what these keys state, in the bundles that it was given. It cannot show that those are all the bundles.
- It does not show that the source served these bytes, and nothing about whether the data is true.

### The fixture

One fixture of made-up identities shows each line and each mark. It is chosen so that nobody learns that the larger count is right. Every note in it says `synthetic`.

- Most state a testdata file with a changed byte, and carry it.
- One states the unchanged testdata file, and carries it.
- One states a digest of the project's own table and carries no file. `bean append` wrote that record.
- One states two digests for one dataset, in two bundles.
- One states a digest that is malformed.
- One share has a note with a quotation mark, a backslash, and characters that show as blank. A check of a bundle lets some of those through, so the tally has its own rule for them.
- One bundle fails its check, and is listed as refused.
- One identity states one digest in two bundles, and one share is handed in twice. Each counts once.

### What the documents tell a person who is asked for a receipt

- Which commands to run, at their own terminal. `bean share -for` is in bean 0.8.0 (tag `v0.8.0`). To carry a file as well, they add `-files`.
- The bundle is sealed to the asker's address for its way there. The asker opens it, may show it or pass it on, and cannot give it back.
- What it shows of them (F14). That travels with the bundle.
- A note is shown by every tally of the bundle. A URL or a day in it travels, and is quoted.
- The import keeps each file in their log.
- They may use an identity that they made for it.

### The first real use is a paragraph, and no build

A second person fetches the survey's files from their own connection, and sends receipts. What that gives, in these words:

```
A second person, whose identity words the owner compared, signed that they imported files with these
digests under these names. That they fetched them from CDC, from their own connection, is what they
told the owner. It is outside the bundle, or in its note as their word.
```

It is never called a confirmation by receipts. Whether he asks that person is the owner's choice, and that person's.

## What the owner asked for that this plan does not give

Each line says why, and what would give it.

- **"Independent users" who share "selectively".** One script makes every contributor, and `-yes` skips the review where a person chooses. Each share does hold one comparison and its reading. It holds no other reading of the log, and the identity records that every share carries. What would give more: real people.
- **"Hosts it on Blossom".** One machine, by the author's choice (Part 2).
- **"BEAN records the source URL [and] timestamp" of a download.** bean fetches nothing. A person can state both in the signed note, as their word.
- **Receipts for any public dataset.** Only for the files that `bean import nhanes` takes.
- **"100 independent downloads … provide corroborating evidence."** That is earned only by a reader's own knowledge that the keys are distinct people who fetched. The tally cannot know it, and says so.
- **"A decentralized, Sybil-resistant web of trust."** Nothing here resists many keys. The first step would be a tally that sets apart the identities that a reader lists as known. Leaving it out is the author's choice: nobody would use it before a second real person sends a receipt. It is a few lines, and he can order it.
- **A pool, and a host.** This is a test of the path, on one machine.

## The order of work

Part 1 first. Part 2 second, since the network leg is what the owner named first. Part 3 third. Each part is a change of its own, with its own audit. Each has a page that says what it needs. Parts 1 and 3 need Go, Python 3 and the packages of `tools/requirements.txt`. Part 2 needs `nak` as well.

## Left out, on purpose

Any change to the bean binary. A flag that holds a pack to a SHA-256. A new record type or member. A real pool, real contributors, a real host. The document of a first setup. Homomorphic encryption. Scores of reputation. A fetch in bean.

## Known weak points

- **It comes before the order to 0.9,** by the author's reading of the owner's request (DECISIONS 332). He can say otherwise. That order slips by about four days.
- **Nothing shows that three parts and their audits fit four days.**
- **A demonstration can be taken for a pool.** The pool's pack, the pointer and the tally say what they are. The committed survey pack cannot. A recording or a post is outside what bean can mark.
- **"Reproducible" is one program run twice.** No second builder exists, so a fault of the builder gives the same wrong bytes both times.
- **The pinned SHA-256 holds two URLs of the repository.** It changes if the repository moves, and whenever the builder's rule changes.
- **The note `synthetic contributor` and the flag `-synthetic` are words.** A careless or dishonest person can put a real share into a public archive with them.
- **A tally is read as evidence,** whatever it prints of itself.
- **A note is free text.** A signer can write "verified by 100 people" in it. The tally quotes it, and cannot stop a reader from believing it.
- **Part 2 rests on one program at one version,** outside CI. A later `nak` can break it, and nothing says so until a person runs it.
- **The receipts add nothing to bean.** That may be less than the owner pictured.
