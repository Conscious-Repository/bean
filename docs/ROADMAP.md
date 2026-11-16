# Roadmap

This file is the one place for open paths: what bean should do next, in what order, and what is not decided. Work that is done is in the git history, and the reasons are in [DECISIONS.md](DECISIONS.md). Nothing here is final.

## Where bean stands (2026-11-16)

A person can keep a log, and import lab reports, Apple Health and CSV files. They can compare a reading with public reference data: with a survey's own files, or with a reference pack. They can share chosen records as one sealed file that opens with six words, and a reader checks it in a browser. A reader who runs bean can answer with signed notes, and the person takes them into their log. Independent second implementations and a real-browser test guard those paths.

**bean is in use.** A second person keeps a log with it, and the owner keeps his own. So `main` is a release, whether or not it has a tag (DECISIONS 310). A log that an earlier build wrote must read the same in a later build, or [UPGRADING.md](../UPGRADING.md) says what changes.

Three things are still missing. Two people cannot see their records side by side. A device cannot sign what it captures. Only one public source has been built into a pack. They are in the four paths below.

## Path 1: a sealed package for one reader

**The need.** A person sends records to a doctor or a peer. Only that reader can open the package. The reader may have no key and no account.

**The plan.**

1. **Seal with a passphrase first. Done:** `bean share -seal` ([SHARE.md](SHARE.md), DECISIONS 275). bean packs the bundle into one file, encrypts it, and makes six random words. The person gives the words to the reader by another channel, such as a phone call. The check page asks for the words, opens the file on the reader's device, and runs the check. This needs no key from the reader.
2. **Seal to a reader's key next.** The check page makes a key for the reader, in their browser. The secret part stays on their device. They send the public part, a short reader address, to the sender. `bean share -to <address>` seals the package to it. A reader who runs bean gets an address from bean.
3. **Use a standard format. Done.** The file is an `age` file, so a tool other than bean can open it. bean invents no cipher.
4. **Say what leaves.** The review before a share also says that the person's identity id travels. It flags a file that holds a name or a birth date.
5. **Record who it was for.** The share record says that a passphrase was used. With a reader's key it will name the reader's address.

6. **The reader knows who signed. Done:** identity words (DECISIONS 280). The sender reads eight words in the call that carries the opening words, and the check shows the same eight (DECISIONS 287).
7. **One public address for the check page. Done (DECISIONS 311):** the releases of the repository, which is public. A tag on `main` builds the page on the code host's machine and publishes it with its SHA-256. CI records the SHA-256 of every push as well. Not done: a second, independent place that states the SHA-256, and a short address of the project's own.

**What it cannot do.** A reader who has opened the package holds a copy for good. Withdrawing permission stops the next share, not the last one.

**Open.**

- Should `-seal` become what `bean share` does unless told otherwise? Today a plain folder is the default, because the checks and the examples are built on it.
- Does the page need a project domain? Today it lives in the releases of the code host. A second, independent place for its SHA-256 is what catches a host that serves a changed page.
- Does a package need an end date after which the page refuses to open it? An end date stops an honest reader's page. It does not stop a copy.

## Path 2: people compare with each other

**The need.** A person with a condition wants to learn from others in the same situation: what they measured, what they tried, what happened. bean is the way they exchange evidence with people they trust. bean does not diagnose, rank treatments or advise.

**The plan, in steps that each stand alone.**

1. **One person to one person.** Path 1 is this step. A person sends a sealed package, such as a biopsy report with their notes, to a peer they already know. The peer checks it and reads it.
2. **A reply that the owner can keep. Done:** `bean reply` and `bean accept` ([SHARE.md](SHARE.md), DECISIONS 290). A peer who runs bean signs notes about the records that they received, and sends them back sealed. The owner compares the peer's identity words and the reply's id, and takes the notes into their log under a name. A note is text about one shared record, and a reply that holds anything else is refused. The log remembers how far a peer's chain of keys had got, so a retired key takes nothing later. `bean unpair` takes a name back.
3. **Side by side.** bean shows the records of two people next to each other, each with its signer and its caveats. It computes nothing across them.
4. **Finding each other.** A person publishes a short card: a condition, what they look for, and a reader address. It holds no measurement. Others answer with a sealed package. Where cards live is not decided.
5. **A statistic over a group.** A group wants its own distribution, and no member wants to show their values. This is the one step that needs computing on hidden values. It waits for a real group, and for steps 1 to 4.

**On homomorphic encryption.** Steps 1 to 4 need none: each person shows chosen records to chosen people. Step 5 is where it applies. For a small group, a simpler method than full homomorphic encryption may do. Each member splits their value into shares, and only the sum is opened. The choice waits for a real group and a real question.

**Risks to design for.**

- A card for a rare condition can identify a person. A card is coarse, and it carries a reader address made for it, not the person's identity id.
- A claim is a claim. A peer's account of a treatment is not evidence that it works, and bean says so wherever it shows one.
- Any output that ranks, scores or recommends is a different product, with its own review. The steps above produce none.

**Open.**

- The first peers are people who already know and trust each other (the owner, 2026-11-11). So steps 1 to 3 come first, and step 4 waits.
- A reply from a reader who has only a browser. The page would make a key for that reader and sign with it, which is step 2 of path 1.
- A peer's notes in the peer's own log. Today the peer keeps the reply file and its words. Their log holds only a record that they replied.
- A reply to a reply. Today a note is about a record of a share, and the owner answers with a new share.
- Who runs the place where cards live: nobody (people pass addresses by hand), an open network, or AION?

## Path 3: a device signs what it captures

**The need.** An AION instrument signs each acquisition when it captures it. The person brings the signed records into their log over a cable or a radio link. A reader can tell which device captured, with which settings, and that nothing changed since.

No device exists yet, and the hardware is being decided now. bean does not wait for it: the protocol is written first, and the hardware is built to meet it. So this path starts with what the firmware and the hardware must be able to do. [DEVICE.md](DEVICE.md) is the format. [EXTENSIONS.md](EXTENSIONS.md) section 11 is an earlier draft that it replaces.

**Decide in the hardware now. These are hard to change later.**

1. **A key that is made on the device and never leaves it.** Use a secure element or the secure key store of the processor. Choose a part that signs with ECDSA on the P-256 curve: most secure elements do, and bean accepts it. The public key can be read out, and is also printed on the device as a code to scan.
2. **A certificate from the factory.** At manufacture, AION signs each device's public key with its model and hardware revision. Without it, a reader sees that some key signed. With it, a reader sees that an AION device of a named model signed. This needs a step on the production line and a key that AION guards.
3. **Signed firmware, and its version in every record.** A device key signs whatever the firmware hands it. If the firmware can be replaced, the signature means less. The record names the firmware version and build.
4. **A counter that never goes back.** Each record carries the count of acquisitions and the count of starts since manufacture. A device clock can be wrong or reset. A counter gives the order and shows a gap.
5. **Enough storage and time to hash the raw data.** The device computes the SHA-256 of each raw acquisition as it writes it.

**What the device signs, per acquisition. One record, not one per sample.**

- The SHA-256 of the raw data, and its size.
- Every setting of the acquisition, the calibration in force, and the result of the self-test.
- The model, hardware revision, firmware version and the two counters.
- The device's time, as a claim.
- Whole numbers and text only. The statement is JSON in one fixed order, and fractions are hard to write the same way on every processor.
- The id of the statement before it, so that none can be taken out of the middle.

**What the device does not do.**

- It does not know whose body it measured. The person says so when they bring the record into their log.
- It does not hold the person's keys or their log.
- It does not judge the measurement.

**The transfer.**

- The device hands over a folder: the signed records, the raw files, and its current time.
- The folder is the same over a cable, Bluetooth or Wi-Fi. The link is not trusted for integrity, because the records are signed.
- The link must still be private. A radio link is encrypted, and its pairing key is not the signing key.
- The device keeps an acquisition until the host confirms it holds it.

**What bean builds.**

1. **A written format and a checker, first. Done:** [DEVICE.md](DEVICE.md) and `tools/device/check.py`, one Python file that says whether a device's output is right (DECISIONS 276). The firmware team can build against it before bean reads a single device.
2. **A software device. Done:** `tools/device/sim` writes the same folder as a real device. `make device` holds the Go check and the Python check to the same verdict.
3. **`bean pair` and `bean import-device`.** The person trusts a device key. bean checks each signature, keeps the records unchanged, keeps the raw files encrypted, and records the device's clock against the host's.
4. **Who signed, shown everywhere.** A device record shows the device's label in a listing, in a bundle and on the check page.

**What a device signature supports.** This key signed these bytes, with these settings, in this order. With the factory certificate: the key belongs to an AION device of this model. It does not show that the reading is accurate, or whose body was measured.

**Open, for the hardware team.**

- Which processor, and does it have a secure element or a secure key store? Can it sign with P-256?
- Is there a production step where a certificate can be issued?
- How large is one raw acquisition, and does the device process it or send it raw?
- Which links will the device have: cable, Bluetooth, Wi-Fi? Is a phone always part of the transfer?
- Can the firmware be updated in the field, and is the update signed?

## Path 4: compare with public data, from any source

**The need.** A person sets a reading against data that someone else publishes. The value must not leave their machine. The result must be one that anyone can compute again. And the way to do it must not know one survey, one test or one unit.

**Done.** A reference pack ([REFERENCE.md](REFERENCE.md), DECISIONS 281): one file, named by its SHA-256, with a table for each measure and each cohort on its menu. `bean-pack-nhanes` builds one from the NHANES files, with weight beside the blood tests. `bean compare -pack` compares with it and saves a claim that names the pack. A Python check reads the format from its document, builds every table again from the files, and computes every claim again.

**Done: more tests in the NHANES pack (DECISIONS 309).** A second pack is on record: 20 measures, from eight files. It adds six tests of the standard biochemistry profile, and ferritin. The tests are ones that the two real report layouts print. The first pack stays on record.

**Next for the breadth of tests.** `bean compare -type 'lab.*'` names the types with no reference data in one line, with no value. Each of these needs its own step:

- **A rule for a value below a detection limit.** ALT, GGT, total bilirubin and C-reactive protein wait for it. The rule needs a new member of a pack, so it is a change of the format.
- **The fasting part of the sample:** glucose, triglycerides, LDL and insulin. They need another weight.
- **More of the blood count:** the share and the count of each kind of white cell. `CBC_J` is in both packs on record, so these rows need a new version of the builder.
- **Tests where two analyzers differ too much for a percentile:** sodium, chloride, calcium, AST, alkaline phosphatase and iron. A percentile is the wrong result for them. Another kind of result may fit, such as a range that the source states.
- **A caveat with a size.** Each result says that the instruments differ, and not by how much. For the tests of the biochemistry profile CDC gives a figure, and a measure's caveat could state it.
- **Albumin, globulin and carbon dioxide**, when a source confirms that the lab and NHANES measure one thing ([COMPARE.md](COMPARE.md)).
- **Hormones, vitamins and thyroid tests** are in other files, some of other survey years. A second cycle is a second set of files and weights.

**The plan.**

1. **A second source, from another publisher.** The format is not frozen before a second real source is built into a pack. Each candidate is confirmed first from its publisher's own site: the terms, a stable address, and whether a file changes without notice.
2. **Publish the NHANES pack** beside the check page, with its SHA-256, and mirror it (the section on where the files live). Then a person needs one download of under 1 MB, not eight files. It waits for the owner to confirm the terms of the source (DECISIONS 311).
3. **The check page computes a shared comparison again**, when the reader gives it the pack.
4. **A publisher signs.** A record by the publisher's bean identity names a pack's SHA-256, and travels as a bundle. This is built when a second publisher exists.
5. **A pointer on relays.** A publisher announces a pack on Nostr relays: title, SHA-256, where it is. The relays hold a pointer and no health record. bean keys can already be Nostr keys.
6. **More kinds of cell**, each when a real source needs it: percentile points, a fitted curve, counts of a category. REFERENCE.md says what each may state.

**The person's own list of packs: scoped, and it waits (DECISIONS 308).** The first user outside the owner asked to choose his sources, and in time to use the sources of his peers. Today any pack works with `-unknown-pack`, so nobody is locked in. A signed record, "I use this pack", is the trace that a peer's vouch will be. It is built when a second pack exists that someone other than the project built, or when a peer asks to send their list. Entry 308 holds the rules for it.

**Who is paid for what.** A host may charge for bytes. A publisher may sell a pack that took work to build. Nobody is paid to compute a comparison: the request would carry the cohort or the value. bean holds no wallet.

**Open.**

- Which second source. Leads, none confirmed: other parts of NHANES (blood pressure, sleep), other open survey files of the US government, and population counts of genetic variants. Fitness and heart-rate tables in journals may not be free to hand on.
- A pack from data that is not public cannot be built again by others. It needs the publisher's signature, and a way to say so in the pack.
- A cohort by ethnicity, body or food needs the files today. Should a pack carry such cohorts, or should a person build a pack of their own?
- The digests of the NHANES files, and so of the pack, come from one download. A second person must confirm them from their own connection.

## Where the files live: the project's domain, and Blossom mirrors

**The need.** bean runs no server. Three files must still reach people: the check page, a reference pack, and a sealed package that is too large for e-mail.

**The fit.** A Blossom server keeps a file under its SHA-256 and serves it at that address. bean names a pack and the check page by SHA-256 already. So a server needs no trust: the reader checks the bytes (DECISIONS 286).

**The plan.**

1. **At the first release, mirror.** Put the check page and the NHANES pack on the project's own domain. Put the same bytes on one or two Blossom servers, with an existing Blossom client. State each SHA-256 in the release notes.
2. **One line in the documents.** A `curl` command with the server and the SHA-256, then the check. bean fetches nothing itself.
3. **A signed note with each SHA-256.** A project key signs a note on Nostr relays: the title, the SHA-256 and the servers. This is the second place for a digest (path 1, step 7, and path 4, step 5).
4. **A sealed package by link.** A person uploads the encrypted file with a Blossom client and sends the link. The six words go by phone, as now. SHARE.md then says what the server sees.
5. **A fetch command in bean, only if people ask.** Until then bean stays offline.

**What a server learns.** That someone fetched a file, from which address, and when. For a package it also holds the size and the ciphertext, for as long as it likes. Nobody can make a server delete a file.

**What never goes there.** A record: record files are plain JSON. A request for one cohort, or one that carries a value.

**Open.**

- Which servers, and who pays for the bytes. How long a server keeps a file differs, so use more than one.
- The Blossom documents are drafts. Read them again before step 1: the upload, the list of servers, and the answer that asks for payment.
- Should the project run a Blossom server of its own? Not before someone needs it.
- Does a link to a package need to work once only? A server that keeps the ciphertext keeps it, whatever the link does.

## Order

The owner confirmed the long aim on 2026-11-16. A person keeps their own log. They share with people they trust. They compare with reference data from many sources, and they choose which sources count. Anyone can publish a source, and AION is the first of many.

Two audits on that day set this order (DECISIONS 313 and 316). Each step ships alone.

1. **The fasting tests, as a new pack:** glucose, triglycerides, LDL and insulin. It is what real logs lack. The builder needs a weight for each test, and the pack says that a person's own reading may not be fasting.
2. **The packs on a host of AION's own, with a pointer on a relay** (path 4, step 2). It is documents and the work of running a server, and no code in bean. It runs beside step 1.
3. **Side by side with a peer** (path 2, step 3). bean shows the readings of a bundle that a peer sent beside the person's own. It computes nothing across the two people.
4. **A person keeps a pack with their log.** A kept pack needs no flag, and each result says plainly why the person uses it. A saved comparison can then be computed again when no host has the pack.
5. **A check that computes a shared comparison again**, when the reader gives the pack (path 4, step 3). It comes before any word that sounds like trust.
6. **"This peer keeps this pack":** a signed record of a peer or a publisher, taken in under a name. It is built when a second party exists. That is a peer who runs bean and wants it, or a pack from a builder that is not the project's.

Then, each when its condition holds:

- **A second source, from another publisher.** First as research: one source confirmed from its publisher's own site. Until one exists, the project is still the one that says which source files are right.
- **The rule for a value below a detection limit:** ALT, GGT, bilirubin and C-reactive protein.
- **A package by link**, after a package can be sealed to a reader's key (path 1, step 2).
- Path 3, when a real device can produce the folder.

Not planned: a command in bean that fetches from a network, and a view that ranks or scores two people. bean never runs a program that a pack names. A public card of what a person holds is not planned either.

## Smaller open paths

- **Reading a lab report in a layout that bean does not know** (DECISIONS 306). The first real set of reports, 49 from two labs over 14 years, read none. The file is kept now. Next, in this order. Measure with `bean-import-lab -count` on real reports how far the rows read once the lines above the column header are passed over. Only if they read well: a second reader that proposes rows, each beside the cell it was read from. The person accepts a row, and `-yes` never takes one. Each such reading says which reader read it, and where in the file. Not planned: a reader for each layout that a person reports, and a reader that guesses with no review.
- **A file that only another key's record names.** `bean purge` keeps a file that another record names, and says so. When that record is a peer's or a stranger's, no export, share or assistant reads the file. Its encrypted copy stays all the same, and `bean sync` copies it. Decide whether such a record should keep a file at all.
- **More tests and cohort rules.** Each is one row in `internal/nhanes`, from the file's own label and a real report's name.
- **More importers,** each from a real file, never from memory. A time zone flag for CSV times.
- **A check of a whole export in Python.**
- **Release.** A `go install` path and a neutral home ([GOVERNANCE-PROPOSAL.md](GOVERNANCE-PROPOSAL.md)). A tag that starts with `v` publishes `bean-check.html` with its SHA-256 (`release.yml`). No tag exists yet.
- **The owner's to do.** Confirm the NHANES digests from a second connection. Open the repository to the people who will try it. Then require the CI check on `main`.
- **Freeze at `v` 0** (CORE.md section 6), after two weeks of real use with no change to CORE.md.

## Limits to say out loud

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
- No way to test the 24 words without rotating. `net/http` is in the binary only to detect image types. CI does not run `lint`, `vuln` or fuzzing.

## Not planned

A central data platform that a person cannot leave. A UI beyond a static report. Relay publishing of health records. A service that computes a comparison from a person's value or cohort, paid or free. A wallet in bean. Encryption of record files at rest (use an encrypted volume). Whoop and Oura importers until there is a real export. Zero-knowledge proofs, and distributed trust scores. Any output that diagnoses, predicts or advises.
