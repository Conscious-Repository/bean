# Roadmap

This file is the one place for open paths: what bean does now, what comes next, and what is not decided. Work that is done is in the git history. The reasons are in [DECISIONS.md](DECISIONS.md), and [KNOWN-GAPS.md](KNOWN-GAPS.md) lists known limits. Nothing here is final.

**Where bean stands (2026-11-27).** A person keeps a log, and imports lab reports, Apple Health and CSV files. They compare a reading with a survey, or with a reference pack. They share chosen records as one sealed file, with six words or sealed to the reader's address. A reader who runs bean answers with signed notes, and the person takes them into their log. A public host serves reference packs ([EXTERNAL-PACKS.md](EXTERNAL-PACKS.md)). Release 0.8.5 adds the cohort from two records (DECISIONS 348). Release 0.8.6 keeps those two records out of a share unless the person chooses them (DECISIONS 350). Both are out, as pre-releases. `main` is 0.8.7, with no tag. A person accepts one pack by its SHA-256 (DECISIONS 351). A reader computes a shared comparison again with bean (DECISIONS 353). A walk of FIRST-RUN.md by two made-up people found no fault in the program (DECISIONS 354). The tag and release are the owner's word. Two people keep a log, so `main` is a release, with or without a tag (DECISIONS 310).

## Now: the proof of concept

**Run the existing one-to-one exchange.** Two people each import a reading. They exchange a package sealed to a verified reader address and a signed reply, then compare locally with a reference pack. Use [FIRST-RUN.md](FIRST-RUN.md) and a channel the two already use. A peer whose report bean does not read can enter a reading with the type and unit the pack lists. Each person checks the pack against the SHA-256 in [REFERENCE.md](REFERENCE.md). Record where each person needed help and how long the exchange took. This exercise can run today with a local copy of the pack.

**Try ongoing sharing in a separate delivery client.** Use synthetic readings and a local inbox. These are proposed trials for the next few days. Each builds on the last. Use the existing [selection and seal for one reader](SHARE.md).

1. **One invitation.** The sender chooses a contact, one reading type and a date window. Verify the reader’s address and the sender’s signing identity by another channel. Preview the full selection, including records brought along by a comparison and any attachments. If it exceeds the permission, hold the package for review. The reader accepts one invitation and checks the expected signed share ID. A third key cannot open the file. A changed contact key needs fresh verification.
2. **One ongoing permission.** The sender chooses between sharing once and including future results, with a separate expiry for that permission. If the window refers to when a reading was taken, use its observation date. Hold an undated reading for review. Test a late import, a correction and an unrelated reading. Recheck the current permission before every handoff, including retries. Stop or expiry cancels queued work still under sender control. Once a recipient can retrieve the sealed bytes, count them as released even if unread. A timeout leaves delivery uncertain. Retry the same sealed package only while authorized. The reader catches up after an outage and recognizes a repeated signed share ID. Replaying an older share must never replace a correction.
3. **Two readers.** Pick two verified contacts and show what each will get. Seal separately to each using the same one-reader format. Give them different scopes. Remove one while an update is queued and check that only the remaining reader gets the next matching result once. A reader adding a contact grants that person nothing. Block the first host and start a fresh reader from a second inbox holding the authorized package. Verify the same signed share ID and sender offline with the reader’s own key.

The sender should be able to review and stop each permission, and each reader should be able to receive updates without moving files by hand. The client owns contacts, permissions, delivery and retries. If the trial exposes a missing offline operation, bring that specific case to bean. A public host for personal packages still needs the protected reader key described in path 1 (DECISIONS 328).

## Next: 0.9, then the lock

**0.9 is the loop for people with a terminal:** import, share 1:1 and get an answer, and compare with a pack (DECISIONS 342). Report the results of the two-person proof of concept and its limitations. The scoped-updates trial informs paths 1 and 2; its client and the rollout of any publisher’s server have independent schedules.

**1.0 is the owner's word that the formats lock.** [CORE.md](../CORE.md) section 6 says what locks, and how the word is given (DECISIONS 343). There is no clock.

**Settle before the formats lock.** Each is an open question, from the audits of 2026-11-21.

- **English in signed comparisons.** A saved comparison holds caveat sentences, and a check compares them letter for letter. Should a claim carry short codes instead?
- **The built-in list of known packs.** It is in bean's code, which makes the project the judge of every pack. Should it only be a hint, outside the locked formats?
- **`pack/0` changed its meaning without a new label.** Release 0.8.2 reads packs that 0.8.1 refuses, and a reply to an address came the same way. Should a loosened format get a new label, and should bundles and replies carry a mark?
- **A text of a pack is judged by the reader's Unicode version.** Two readers can differ on a character that a later version names. Should the format name one version, or refuse every character with no category (DECISIONS 347)?
- **Four pack labels came in one day** (`pack/1` to `pack/4`, DECISIONS 346). Each names one added rule, and a later label takes the rules before it. Is that one format with optional parts, or a label for each rule? Each rule is written three times: in bean, in `tools/pack/check.py` and in `tools/compare/reproduce.py`.
- **Nostr kind 29944 is not registered,** but the validity rules use it. Register it, or accept the risk in writing?
- **P-256 lets a hardware signer use random nonces,** and no device exists yet. A weak random source can leak the key. Should P-256 stay out of the lock until a real device signs?
- **A stolen Nostr key could swap the address of every peer** who finds addresses on Nostr. Does an address from Nostr still need a check by voice once, and again when it changes?
- **A lab server that carries packages would see who shares with whom.** What delivery metadata would the participants accept before a personal package goes on such a host?
- **A deletion stays only on the copy where it was made** (`local/purged-refs.jsonl`). Should a deletion travel with the log?
- **A saved comparison can carry another time than its reading,** and no check refuses it. The made-up pool on record holds such a claim. Must a claim's time be its reading's (DECISIONS 347)?
- **bean keeps one of two comparisons that state the same, under the lowest id.** The record around a claim is not in that test. So a copy with another source could be the one that stays. Should the test read the whole record?

**Other open decisions of the owner.**

- Whether the survey method drops its 30 too, as a pack cell did (DECISIONS 333). The packs on record would need a new builder version.
- Whether a result from a very small cell gets words of its own.
- Whether `bean coverage` takes the cohort from the two records as well. It needs `-cohort` today. The three reviews of 2026-11-26 said that it waits (DECISIONS 348).
- Whether 1.0 can come while `pack/0` is a draft, and whether a review from outside the project is a condition of 1.0.
- Whether the caveat of a small cohort stays at 100, and why 100. The number is set twice in the code, with no reason given.
- Whether a record gets a mark for a value that was typed by hand.
- Whether bean ever keeps a package or a pack with the log, so that an export holds it.
- Where code lives. Proposed: what runs offline is here, and what listens on a network is not.
- Whether the documents name a publisher. The rule is that they name none (DECISIONS 318). Since 2026-11-23 some name one: EXTERNAL-PACKS.md, `STELE-SHA256SUMS`, FIRST-RUN.md with a second address of the same hand, and one line of UPGRADING.md. So do `examples/coverage`, in its README, a selection file and the default address of `fetch.py`, and a format name in `bean coverage`. A review raised this, and not the owner (DECISIONS 346).
- The three picks for `-accept-pack`, each of which the owner can reverse (DECISIONS 354). The list of a pack with no record names the flag. Upper-case digits stay refused. `compare` refuses a second value.
- Whether the relay of DECISIONS 318 comes before a tool reads a pointer. Three reviews of 2026-11-25 said that it waits (DECISIONS 346).

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

Done: reference packs, two NHANES packs on record, and `bean compare -pack` (DECISIONS 281, 309). A pool path with made-up contributors is built as a test (DECISIONS 334 to 336). `bean compare -pack` takes each row's cohort from two records of the person, `person.sex` and `person.birth` (DECISIONS 345, 348). `bean check-bundle -pack` computes a shared comparison by a pack again (DECISIONS 353). Open:

- **A second source, from another publisher.** The pack format does not lock before a second real source is built into a pack. Leads, none confirmed: other parts of NHANES, other open US survey files, and counts of genetic variants. Tables in journals may not be free to hand on.
- **The cohort from two records, for a pack that is not on record.** bean's code holds the sex and the ages of each cohort, for the two packs on record only. A pack states a cohort in words. A field for the sex and the ages would be a change to the pack format (DECISIONS 348).
- **More tests** by [REFERENCE-PLAN.md](REFERENCE-PLAN.md): values below a detection limit, more of the blood count, and a caveat with a size. The fasting tests need a weight of their own, which is a decision of their own.
- **A publisher signs a pack,** and **a person keeps their own list of packs** (DECISIONS 308). Each is built when a second publisher exists.
- **The check page computes a shared comparison again,** when the reader gives it the pack. `bean check-bundle -pack` does since 0.8.7 (DECISIONS 353); the page in a browser does not yet. First the check moves into the code that the page runs, with its tests. Then the page takes a pack file from the reader, and fetches none. A pack with no record counts only with a SHA-256 that the reader types.
- **A pool of real people.** It needs a share that names its reader, written consent and a review of the law (DECISIONS 325, 328). It also needs a pack format for data that is not public ([POOL-PLAN.md](POOL-PLAN.md)). A pack lists every value of a cell, so a small pool shows each member's value.

### Portability check

A second publisher can serve the same public pack. With the first host unreachable, a reader checks the digest and repeats the offline comparison. The [read-only example](../examples/pack-host/README.md) gives the reader check. [HOST-PLAN.md](HOST-PLAN.md) records the operator trial. Each operator owns deployment, domain, access policy and operations outside this roadmap.

### Smaller paths

- Reading a lab report in a layout that bean does not know (DECISIONS 306).
- More importers, each from a real file, and a time zone flag for CSV times.
- A check of a whole export in Python.
- Whether a file that only another key's record names keeps its file at all.
- A neutral home for the project ([GOVERNANCE-PROPOSAL.md](GOVERNANCE-PROPOSAL.md)).
- Require the CI check on `main`. Today GitHub reports that `main` is not protected. On 2026-11-23, 17 commits reached `main` with no pull request and no review (DECISIONS 346).
- `internal/coverage` repeats the match rules of `internal/pack` to show them. Its tests are those of the command, with two of its own.
- A table of names for one quantity. It waits for a real case (DECISIONS 323).
- On the edge of a pack's cohorts, a row can advise a month of birth where no month gives a number (DECISIONS 348).
- A test that holds the sex and the ages of each cohort in bean's code to the menu that the pack's builder uses.
- Newer versions of `golang.org/x/crypto` and `golang.org/x/sys`. `make vuln` names 22 listed faults in the two, and bean's code calls none (DECISIONS 349).

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
