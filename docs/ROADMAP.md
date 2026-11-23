# Roadmap

This file is the one place for open paths: what bean does now, what comes next, and what is not decided. Work that is done is in the git history. The reasons are in [DECISIONS.md](DECISIONS.md), and [KNOWN-GAPS.md](KNOWN-GAPS.md) lists known limits. Nothing here is final.

**Where bean stands (2026-11-22).** A person keeps a log, and imports lab reports, Apple Health and CSV files. They compare a reading with a survey, or with a reference pack. They share chosen records as one sealed file, with six words or sealed to the reader's address. A reader who runs bean answers with signed notes, and the person takes them into their log. Release 0.8.2 is out, as a pre-release. Two people keep a log, so `main` is a release, with or without a tag (DECISIONS 310).

## Now: the proof of concept

**Run the existing one-to-one exchange.** Two people each import a reading, exchange a package sealed to a verified reader address and a signed reply, then compare locally with a reference pack. Use [FIRST-RUN.md](FIRST-RUN.md) and a channel the two already use. A peer whose report bean does not read can enter a reading with the type and unit the pack lists. Each person checks the pack against the SHA-256 in [REFERENCE.md](REFERENCE.md). Record where each person needed help and how long the exchange took. This exercise can run today with a local copy of the pack.

**Prototype scoped updates over the next few days.** This is a closed trial with synthetic readings. Use the existing offline selection, seal and verification steps from [SHARE.md](SHARE.md). A separate delivery client owns contacts, invitations, delivery and retries. A local test inbox is enough to begin. Proposed order:

1. **Day 1 — one reader.** Let a sender choose a verified contact, one reading type, and a date window. Show exactly which records and attachments would leave, including the underlying reading that can accompany a comparison. Hold delivery if the complete package exceeds the approved scope. Seal one package to the reader. The reader accepts one invitation, checks the sender and opens the package locally. A third key cannot open it.
2. **Day 2 — ongoing access.** Offer “share once” and “include future results.” Save the approved rule on the sender’s device. Treat the date of a reading separately from the date permission expires. Show which new results reached the inbox. Exercise a late import, a correction, an unrelated reading, an offline reader catching up and a retry after interruption. Stop or expiry must cancel still-undelivered updates; the reader keeps copies already delivered.
3. **Day 3 — two readers.** Let the sender explicitly select two verified contacts. Seal separately for each using the existing one-reader format. Give the readers different scopes, remove one while an update is pending, and check that only the remaining authorized reader gets the next matching result, once. A recipient adding a contact grants that contact nothing. Repeat delivery using a second inbox or host and verify the packages offline.

Success means the sender can review and stop each permission and the readers can receive updates without manually moving files. An app handles delivery keys and addresses behind those controls. Keep contact and delivery policy in the client while testing. Bring a change to bean only for a demonstrated gap in offline selection, key handling or verification. Public hosting of personal packages still requires the protected reader key described in path 1 (DECISIONS 328). The synthetic trial can proceed independently.

## Next: 0.9, then the lock

**0.9 is the loop for people with a terminal:** import, share 1:1 and get an answer, and compare with a pack (DECISIONS 342). Report the results of the two-person proof of concept and its limitations. The scoped-updates trial informs paths 1 and 2; its client and the rollout of any publisher’s server have independent schedules.

**1.0 is the owner's word that the formats lock.** [CORE.md](../CORE.md) section 6 says what locks, and how the word is given (DECISIONS 343). There is no clock.

**Settle before the formats lock.** Each is an open question, from the audits of 2026-11-21.

- **English in signed comparisons.** A saved comparison holds caveat sentences, and a check compares them letter for letter. Should a claim carry short codes instead?
- **The built-in list of known packs.** It is in bean's code, which makes the project the judge of every pack. Should it only be a hint, outside the locked formats?
- **`pack/0` changed its meaning without a new label.** Release 0.8.2 reads packs that 0.8.1 refuses, and a reply to an address came the same way. Should a loosened format get a new label, and should bundles and replies carry a mark?
- **Nostr kind 29944 is not registered,** but the validity rules use it. Register it, or accept the risk in writing?
- **P-256 lets a hardware signer use random nonces,** and no device exists yet. A weak random source can leak the key. Should P-256 stay out of the lock until a real device signs?
- **A stolen Nostr key could swap the address of every peer** who finds addresses on Nostr. Does an address from Nostr still need a check by voice once, and again when it changes?
- **A lab server that carries packages would see who shares with whom.** What delivery metadata would the participants accept before a personal package goes on such a host?
- **A deletion stays only on the copy where it was made** (`local/purged-refs.jsonl`). Should a deletion travel with the log?

**Other open decisions of the owner.**

- Whether the survey method drops its 30 too, as a pack cell did (DECISIONS 333). The packs on record would need a new builder version.
- Whether a result from a very small cell gets words of its own.
- Whether 1.0 can come while `pack/0` is a draft, and whether a review from outside the project is a condition of 1.0.
- Whether the caveat of a small cohort stays at 100, and why 100. The number is set twice in the code, with no reason given.
- Whether a record gets a mark for a value that was typed by hand.
- Whether bean ever keeps a package or a pack with the log, so that an export holds it.
- Where code lives. Proposed: what runs offline is here, and what listens on a network is not.

## Later

Each path waits for its real case. The steps that are done are in DECISIONS.md.

### Path 1: a sealed package for one reader

Done: a seal with six words, a seal to a reader's address, and the `age` format. The review before a share says what leaves. Identity words show who signed. Open:

- A key that the check page makes for a reader who has only a browser. A forged page would take a key that opens every package, so it needs a design of its own (SHARE-PLAN.md).
- A package by link on a file host, sealed to the reader's key. The owner's rule: a key file with a passphrase comes first (DECISIONS 328). A key made for that one upload signs it, never a key of a log. The host learns the size, the time and each network address.
- A second, independent place for the check page's SHA-256, and a short address of the project's own.
- Whether `-seal` becomes the default, `bean share -except <id>`, and an end date for a package.

A reader who opened a package holds a copy for good. Withdrawing permission stops the next share, not the last one.

### Path 2: people compare with each other

A person with a condition learns from peers they trust. bean does not diagnose, rank treatments or advise. Done: one person to one person (path 1), and a reply that the person keeps (DECISIONS 290). Open:

- **Finding each other.** A person publishes a short card with a reader address and no measurement. Where cards live is not decided. A card for a rare condition can identify a person. Finding a peer and their address on Nostr is proposed, as a separate tool (DECISIONS 340).
- **A statistic over a group** that hides each member's value. It waits for a real group and a real question.
- A reply from a reader who has only a browser, a peer's notes in the peer's own log, and a reply to a reply.

### Path 3: a device signs what it captures

An AION instrument signs each acquisition, and the person brings the signed records into their log. [DEVICE.md](DEVICE.md) is the format, with a checker and a software device. The hardware must have:

- a key that is made on the device and never leaves it;
- a certificate from the factory, if a reader is to see the maker and model;
- signed firmware, and its version in every record;
- a counter that never goes back;
- the time and storage to hash each raw acquisition.

Not built: `bean pair` and `bean import-device`, and the device's label wherever its records show. Open: whether any maker's certificate counts the same, with the reader choosing which makers to trust. A device signature shows which key signed which bytes. It does not show that the reading is accurate, or whose body was measured.

### Path 4: compare with public data, from any source

Done: reference packs, two NHANES packs on record, and `bean compare -pack` (DECISIONS 281, 309). A pool path with made-up contributors is built as a test (DECISIONS 334 to 336). Open:

- **A second source, from another publisher.** The pack format does not lock before a second real source is built into a pack. Leads, none confirmed: other parts of NHANES, other open US survey files, and counts of genetic variants. Tables in journals may not be free to hand on.
- **More tests** by [REFERENCE-PLAN.md](REFERENCE-PLAN.md): values below a detection limit, more of the blood count, and a caveat with a size. The fasting tests need a weight of their own, which is a decision of their own.
- **A publisher signs a pack,** and **a person keeps their own list of packs** (DECISIONS 308). Each is built when a second publisher exists.
- **The check page computes a shared comparison again,** when the reader gives it the pack.
- **A pool of real people.** It needs a share that names its reader, written consent and a review of the law (DECISIONS 325, 328). It also needs a pack format for data that is not public ([POOL-PLAN.md](POOL-PLAN.md)). A pack lists every value of a cell, so a small pool shows each member's value.

### Portability check

A second publisher can serve the same public pack, and a reader can check its digest and repeat the offline comparison with the first host unreachable. The [read-only example](../examples/pack-host/README.md) is one way to test this. Each operator owns deployment, domain, access policy and operations outside this roadmap.

### Smaller paths

- Reading a lab report in a layout that bean does not know (DECISIONS 306).
- More importers, each from a real file, and a time zone flag for CSV times.
- A check of a whole export in Python.
- Whether a file that only another key's record names keeps its file at all.
- A neutral home for the project ([GOVERNANCE-PROPOSAL.md](GOVERNANCE-PROPOSAL.md)).
- Require the CI check on `main`. Today GitHub reports that `main` is not protected.
- A table of names for one quantity. It waits for a real case (DECISIONS 323).

## Not planned

- A view of two people side by side (DECISIONS 342), or any view that ranks or scores people.
- A command in bean that fetches from a network (DECISIONS 328 (g), 340 (d)). A fetch is a plain download, or a separate tool.
- A central data platform that a person cannot leave, or a UI beyond a static report.
- Health records on relays, or a public card of what a person holds.
- A service that computes a comparison from a person's value or cohort, paid or free.
- A wallet in bean, or a program that a pack names and bean runs.
- A unit that bean converts.
- Encryption of record files at rest. Use an encrypted volume.
- Whoop and Oura importers until there is a real export.
- Zero-knowledge proofs, and distributed trust scores.
- Any output that diagnoses, predicts or advises.
