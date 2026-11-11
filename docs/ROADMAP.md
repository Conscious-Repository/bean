# Roadmap

This file is the one place for open paths: what bean should do next, in what order, and what is not decided. Work that is done is in the git history, and the reasons are in [DECISIONS.md](DECISIONS.md). Nothing here is final.

## Where bean stands (2026-11-11)

A person can keep a log, import lab reports, Apple Health and CSV files, and compare a reading with public survey data. They can share chosen records as a bundle that anyone can check: with bean, with Python, or in a browser. Two independent checkers and a real-browser test guard those paths.

Three things that matter to other people are missing. A bundle is not encrypted. Two people cannot compare with each other. A device cannot sign what it captures. They are the three paths below, in the order to build them.

## Path 1: a sealed package for one reader

**The need.** A person sends records to a doctor or a peer. Only that reader can open the package. The reader may have no key and no account.

**The plan.**

1. **Seal with a passphrase first.** `bean share` packs the bundle into one file and encrypts it. bean makes the passphrase: six random words. The person gives the words to the reader by another channel, such as a phone call. The check page asks for the words, opens the file on the reader's device, and runs the check. This needs no key from the reader.
2. **Seal to a reader's key next.** The check page makes a key for the reader, in their browser. The secret part stays on their device. They send the public part, a short reader address, to the sender. `bean share -to <address>` seals the package to it. A reader who runs bean gets an address from bean.
3. **Use a standard format.** The file is an `age` file, so a tool other than bean can open it. bean invents no cipher.
4. **Say what leaves.** The review before a share also says that the person's identity id travels. It flags a file that holds a name or a birth date.
5. **Record who it was for.** The share record names the reader's address, or says that a passphrase was used.

**What it cannot do.** A reader who has opened the package holds a copy for good. Withdrawing permission stops the next share, not the last one.

**Open.**

- Is a spoken passphrase acceptable for a doctor, or must the first version use a reader's key?
- Does a package need an end date after which the page refuses to open it? An end date stops an honest reader's page. It does not stop a copy.

## Path 2: people compare with each other

**The need.** A person with a condition wants to learn from others in the same situation: what they measured, what they tried, what happened. bean is the way they exchange evidence with people they trust. bean does not diagnose, rank treatments or advise.

**The plan, in steps that each stand alone.**

1. **One person to one person.** Path 1 is this step. A person sends a sealed package, such as a biopsy report with their notes, to a peer they already know. The peer checks it and reads it.
2. **A reply that the owner can keep.** The peer signs notes about the records they received, and sends them back sealed. The owner adds those notes to their log, marked with who signed them. Today a log takes records from the person, their paired agents and nobody else.
3. **Side by side.** bean shows the records of two people next to each other, each with its signer and its caveats. It computes nothing across them.
4. **Finding each other.** A person publishes a short card: a condition, what they look for, and a reader address. It holds no measurement. Others answer with a sealed package. Where cards live is not decided.
5. **A statistic over a group.** A group wants its own distribution, and no member wants to show their values. This is the one step that needs computing on hidden values. It waits for a real group, and for steps 1 to 4.

**On homomorphic encryption.** Steps 1 to 4 need none: each person shows chosen records to chosen people. Step 5 is where it applies. For a small group, a simpler method than full homomorphic encryption may do. Each member splits their value into shares, and only the sum is opened. The choice waits for a real group and a real question.

**Risks to design for.**

- A card for a rare condition can identify a person. A card is coarse, and it carries a reader address made for it, not the person's identity id.
- A claim is a claim. A peer's account of a treatment is not evidence that it works, and bean says so wherever it shows one.
- Any output that ranks, scores or recommends is a different product, with its own review. The steps above produce none.

**Open.**

- Who are the first peers: people who already know each other, an existing patient group, or strangers?
- Who runs the place where cards live: nobody (people pass addresses by hand), an open network, or AION?

## Path 3: a device signs what it captures

**The need.** An AION instrument signs each acquisition when it captures it. The person brings the signed records into their log over a cable or a radio link. A reader can tell which device captured, with which settings, and that nothing changed since.

No device exists yet, and the hardware is being decided now. So this path starts with what the firmware and the hardware must be able to do. [EXTENSIONS.md](EXTENSIONS.md) section 11 is an earlier draft of the record shapes. No code implements it.

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
- Whole numbers and text only. The record is JSON in one fixed order, and fractions are hard to write the same way on every processor.

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

1. **A written format and a checker, first.** The record shape, the transfer folder and test vectors, with a script that says whether a device's output is right. The firmware team can build against it before bean reads a single device.
2. **A software device.** A small program that writes the same folder as a real device. bean and the firmware are then built side by side.
3. **`bean pair` and `bean import-device`.** The person trusts a device key. bean checks each signature, keeps the records unchanged, keeps the raw files encrypted, and records the device's clock against the host's.
4. **Who signed, shown everywhere.** A device record shows the device's label in a listing, in a bundle and on the check page.

**What a device signature supports.** This key signed these bytes, with these settings, in this order. With the factory certificate: the key belongs to an AION device of this model. It does not show that the reading is accurate, or whose body was measured.

**Open, for the hardware team.**

- Which processor, and does it have a secure element or a secure key store? Can it sign with P-256?
- Is there a production step where a certificate can be issued?
- How large is one raw acquisition, and does the device process it or send it raw?
- Which links will the device have: cable, Bluetooth, Wi-Fi? Is a phone always part of the transfer?
- Can the firmware be updated in the field, and is the update signed?

## Order

1. Path 1, the passphrase package. It is small, and Path 2 starts with it.
2. Path 3, the written format, the checker and the software device. The hardware decisions will not wait.
3. Path 2, steps 2 and 3.
4. Path 3, pairing and import, when a real device or its firmware can produce the folder.
5. Path 2, steps 4 and 5, when real peers ask for them.

## Smaller open paths

- **Compare without the download.** A reference pack: one small signed file per data set, with the table that the method needs for each cohort on a fixed menu. Anyone can build it again from the public files and compare the digest ([COMPARE.md](COMPARE.md)).
- **More tests and cohort rules.** Each is one row in `internal/nhanes`, from the file's own label and a real report's name.
- **More importers,** each from a real file, never from memory. A time zone flag for CSV times.
- **A check of a whole export in Python.**
- **Release.** A `go install` path and a neutral home ([GOVERNANCE-PROPOSAL.md](GOVERNANCE-PROPOSAL.md)). Each release publishes `bean-check.html` with its SHA-256 and the Go that built it.
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
  - A correction undone at rotation keeps its purged ref. Two corrections of one target both show.
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
- No way to test the 24 words without rotating. `net/http` is in the binary only to detect image types. CI does not run `lint`, `vuln` or fuzzing.

## Not planned

A central data platform that a person cannot leave. A UI beyond a static report. Relay publishing of health records. Encryption of record files at rest (use an encrypted volume). Whoop and Oura importers until there is a real export. Zero-knowledge proofs, and distributed trust scores. Any output that diagnoses, predicts or advises.
