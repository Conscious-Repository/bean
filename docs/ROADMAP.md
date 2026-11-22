# Roadmap

This file is the one place for open paths: what bean should do next, in what order, and what is not decided. Work that is done is in the git history, and the reasons are in [DECISIONS.md](DECISIONS.md). Nothing here is final.

## Where bean stands (2026-11-18)

A person can keep a log, and import lab reports, Apple Health and CSV files. They can compare a reading with public reference data: with a survey's own files, or with a reference pack. They can share chosen records as one sealed file that opens with six words, and a reader checks it in a browser. Two people who both run bean can seal a package to the reader's address, with no words read out. A reader who runs bean can answer with signed notes, and the person takes them into their log. Independent second implementations and a real-browser test guard those paths.

**bean is in use.** A second person keeps a log with it, and the owner keeps his own. So `main` is a release, whether or not it has a tag (DECISIONS 310). A log that an earlier build wrote must read the same in a later build, or [UPGRADING.md](../UPGRADING.md) says what changes.

Two things are still missing. A device cannot sign what it captures. Only one public source has been built into a pack. They are in the four paths below.

## Next: a first run with a peer, a seal to a reader's key, and a first pool (2026-11-18)

The owner set the next aim on 2026-11-18 (DECISIONS 325). Two people share. A person compares with a group, from a pack that a publisher hosts. On 2026-11-21 the owner dropped the view of two people side by side (DECISIONS 342). Three reviews, one for each paragon, judged the plan in laps. They changed its order and added rules.

**The design for a seal to a reader's key started at once, by the owner's word.** He chose that seal as the way a package travels. Its first step is built (DECISIONS 327), and its second (DECISIONS 330). The owner decided on 2026-11-18 that a reply to an address comes before the first run with a peer (DECISIONS 328). The order below says what happens if it is late.

**What does not change.** The core, the record format and `pack/0`. bean fetches nothing. No health record goes to a relay, and no service computes a comparison.

**The steps in bean.** Their order is below, under "The order to 0.9". This list keeps the numbers by which other sections name the steps. A step that touches a key or a share has its own audit and its own release.

1. **A first run with a real peer.** The rules are below. By the owner's word, a reply to an address is built before it.
2. **Dropped: two people side by side** (DECISIONS 342). Once a package checks, `bean check-bundle` shows the peer's records. bean shows no merged view. The number stays, so that the other steps keep theirs.
3. **A design for sealing to a reader's key, with its own audit** (path 1, step 2). **Done on paper:** [SHARE-PLAN.md](SHARE-PLAN.md) (DECISIONS 326). It answers these questions.
   - Where is a reader's key kept, and which copies of a log hold it?
   - How does a sender know that an address is the reader's? A second channel must confirm it before a package is sealed.
   - How long does a reader's key live? Can a reader make an address for one sender, and drop its key after reading?
   - Does a share record name the address that it was sealed to?
   - How is a reply sealed back?
   - What does an earlier bean say about such a package? The answer of today is below.
   - Does the check page open with a key, and does it make one?
4. **Seal to a key**, in the two steps of SHARE-PLAN.md, each with its own audit. The address is a recipient of the `age` format, so bean invents nothing.
   - **Step 1 is built,** and released as 0.8.0 (DECISIONS 327): `bean address`, `bean share -for`, and `-key` on `check-bundle` and on `reply -to`. [SHARE.md](SHARE.md) says how to use it and what it costs.
   - **Step 2 is built,** to be released as 0.8.1 (DECISIONS 330): `bean reply -for <address>`, and `bean accept -key`. Without `-for` a reply makes six words.
   - **Decided by the owner (DECISIONS 328):** a key file with a passphrase comes before any package of his goes on a public host.
5. **A builder of a pool, with synthetic contributors**, under `tools/` (path 4, "A pool"). It is a test of the path, and not a pool. The owner asked for a demonstration of it on 2026-11-18 (DECISIONS 332). [POOL-PLAN.md](POOL-PLAN.md) says what is built, and what it may claim. Its first part is built: `make pool` (DECISIONS 334). Its second part is a trial run on one machine: `examples/pool` (DECISIONS 335). Its third part is the tally of receipts: `tools/receipts` (DECISIONS 336). It is no step of the loop of 0.9. By the author's reading of that request, it comes before the order below.

**The order to 0.9, and what 1.0 is: a proposal (DECISIONS 328).** The owner asked for a path in stages that leads to his whole loop. The loop is: import, share 1:1, compare with pools or with packs that others host. Device signing comes later. This adds an order to the steps above. It adds two small things: true words on leaving, and a mark of format for a bundle and a reply. A third waits: a flag that takes a SHA-256.

A pack is one file of reference data, and a cell is its table for one measure and one cohort ([REFERENCE.md](REFERENCE.md)). A mark of format is a member that says which version of a format a file has.

**What the owner decided, when he was asked (2026-11-18).** Each line gives the question in short, and the answer that he chose in quotation marks. DECISIONS 328 (b) has what each answer said beside it.

- What "a user uploads data" means: "Import into own log".
- Who walks the loop at 1.0: "People with a terminal".
- What "compare against pools of user data" is at 1.0: "Side by side, two people". **Changed since.** The 30 that this answer rested on is gone (DECISIONS 333). On 2026-11-21 the owner dropped the view (DECISIONS 342).
- When a reply to an address is built: "Before the first run".
- What 1.0 promises. His own words: "maybe we should plan to leave this at .9 for a while too until we're positive formats are locked".
- Whether a key file with a passphrase is built before any package of his goes on a public host: "Yes, before a public host".

**What the owner asked for after that, on the same day.** In his words: "Over the next four days, prioritize a working end-to-end BEAN demonstration". [POOL-PLAN.md](POOL-PLAN.md) quotes the whole request. The author reads it so: the builder of a pool does not wait, and the order below follows the demonstration.

**The owner decided the points of the list above. The rest is the author's proposal:** what 0.9 holds, the order, and the rules for 1.0. Reviewers who had not seen the work judged it, one for each paragon, and DECISIONS 328 has the account. Their reviews are not kept. He can change any of it.

**0.9 is the loop for people with a terminal: import, share 1:1 and get an answer, and compare with a pack** (DECISIONS 342).

1. Import into your own log.
2. Share 1:1 and get an answer: with six words, or with identity words and address words.
3. Compare with a pack of the survey, which bean has on record. A person builds it with `bean-pack-nhanes` from the survey's files, which [COMPARE.md](COMPARE.md) lists. Or they fetch it, when the packs are on a host. A pack of a small pool of peers is read the same way, since a cell has no least number (DECISIONS 333).

**0.9 is not the owner's whole loop, and its status says so first.** No pool exists, and no other entity hosts a pack.

**The proposed order.** A step that touches keys or sharing has its own audit. A step with code is a release of its own, with a number below 0.9.

1. **First: publish 0.8.0, with true words on leaving.** One small change comes before the tag. **The change is made (DECISIONS 329), and release 0.8.0 is out.**
   - The README says today that `bean export` writes everything, and SECURITY.md and ARCHITECTURE.md say the like. A reader can take an export for a whole backup.
   - The three documents say: an export holds every record and file that bean could read. It leaves out what you deleted. `manifest.json` lists what it could not write. Those sentences are held to the code before the tag. It holds no key file, no package and no pack. A key file is kept apart, and the 24 words do not bring it back. Nor does an export hold `local/purged-refs.jsonl`, which the README already says to back up, or the other files of `local/` that COMMANDS.md lists.
   - A saved comparison names its pack, and an export does not hold the pack: keep it. An address that your peers hold opens nothing for you once its key file is lost.
   - SHARE.md and SECURITY.md say already what a copy of a key file opens, and that a lost one opens nothing again. The README points to them, and adds one thing: bean does not encrypt a key file, so keep it as any secret key is kept.
   - One case in `tools/smoke.sh` restores from an export, and opens an old package with a key file that was kept apart.
   - Then a tag and a release with the check page. **Done:** release 0.8.0, on 2026-11-18.
2. **Second: a reply to an address** ("Seal to a key" in the list above, its second half). Three points are for its design and its audit. **It is built (DECISIONS 330). The merge and the tag wait for the owner's word.** What was decided on each point is after the point, and the audit judged each by name.
   - **A bundle and a reply get a way to say their format.** Today only a record has a version. The mark is a design of its own, on paper. It is ready before the audit of the reply ends, so that the audit can say whether the reply must carry it. It does not hold the reply up. A file with no mark is format 0. A build from before the mark may refuse a file that carries it: then UPGRADING.md says so, and both people update first. If the mark is built, `tools/second` and the vectors change with it.
     - **Decided in the build: the reply does not carry a mark.** It is format 0 of a reply, and release 0.8.0 refuses its seal by name. The mark is a step of its own before the clock of 1.0 starts. The design on paper proposes a member of the signed share record and reply record. Whether it comes before 0.9 is the owner's to decide.
   - **The gate of `bean accept -key`.** With `-yes` and `-words`, a program that runs as the person and holds the key file takes a peer's notes. It can make the eight words from the reply itself. The reviews of this order recommend that `-yes` does not work with `-key`. The audit decides.
     - **Decided in the build: `-yes` does not go with `-key`.** It is a usage error. `bean accept -key` asks on a terminal, and a program that makes a terminal of its own still answers it.
   - **Where a reply goes.** Only to an address that the sender gave by a second channel, never to one that a package carries.
     - **Built so:** bean takes an address from `-for` only, behind the gate of the address words, and reads none from a package.
3. **Third: the first real run with a peer** (the first entry of the list above). [FIRST-RUN.md](FIRST-RUN.md) is its sheet, one page for the two people. Its round trip needs no six words. It needs identity words once, and address words.
   - Both people update first.
   - The two compare identity words by voice once. Each gives the other an address, and its address words by a second channel.
   - The packages go by a channel that the two already use. No package of the owner's goes on a public host until a key file can be protected.
   - Each person runs at their own terminal every command that prints a health value. An assistant helps as ONBOARDING.md says: it sees exit codes and counts, and no value.
   - The run is timed from the install on.
   - If the reply to an address is not merged when the peer is ready, the owner says whether the run goes on six words.
4. **Fourth: 0.9.** Its status is written on that day, from what is true then. From here the version stays at 0.9 until the owner says that the formats are locked.

**Beside the order, and before the first run: the survey's packs on a host** (phase 2 of the section "Order"). It is the owner's own act, with no code in bean. The owner decided on 2026-11-19 that it comes before the first run with a peer (DECISIONS 339). No other step waits for it.

- It keeps the rules of "A first publisher hosts" below: a document of the setup, and a rehearsal with a throwaway identity.
- It gives a person a real file to fetch.
- The SHA-256 of each pack is also in the project's repository. The owner controls both places. A second person who states the digests from their own connection is a true second place.
- All reviews of this order recommended it before the run, and the owner decided so.
- The owner said on 2026-11-19: "we have a server in the lab we can host a relay + blossom on". [HOST-PLAN.md](HOST-PLAN.md) is a plan on paper in stages (DECISIONS 337). The author proposes it as that rehearsal, with keys that are thrown away. Nothing is set up. He gave the word to host the survey's packs on 2026-11-16 (DECISIONS 314).

**A key file with a passphrase is in no step of this order.** No step puts a package on a public host, so no step needs it. The owner's rule stands.

**What the status of 0.9 says, at least.** It is in the README.

- bean computes nothing over a group of its users, and there is no pool. A comparison with a survey is a percentile in that survey's sample.
- No publisher but the project has made a pack. As it is on that day: whether anyone hosts one, and whether a second person has confirmed the digests on record.
- bean does not diagnose.
- It points to SHARE.md and SECURITY.md for what a share, a key file and an export do not do. Each such line is written with the step that makes its act.

**Not in the order to 0.9.**

- **bean holds a pack to a SHA-256 that the person gives.** It is one new flag of `bean compare -pack`, as `reproduce.py` has with `-accept-pack`. A result says: held to the SHA-256 that you gave, and bean cannot tell where you got it. It is built before a pack of a second publisher is announced. `-unknown-pack` does not change with it. Until then, the compare of a SHA-256 for such a pack is the person's own act. 0.9 does not count it as part of the loop.
- **The builder of a pool** (the last entry of the list above). It is no step of the loop of 0.9. The owner asked for a demonstration of it, with made-up people (DECISIONS 332). By the author's reading it comes before this order.
- **A pack from a pool of real people, and device signing.** Each waits for its real case (paths 4 and 3).

**Proposed: 1.0 is the day the formats are locked.** It is no step of work. This plan proposes the rules of that promise, and not its words. Whoever makes the promise writes it on that day.

- **It lists what it covers and what stays a draft, in one place, by name.** The device format stays a draft. If the owner lets 1.0 come while `pack/0` is a draft, every saved comparison is a draft with it. The promise then says that the comparing part is not locked, and that a tool that writes packs builds on a draft.
- **The record format has frozen at `v` 0** (CORE.md section 6), or the promise names it as a draft.
- **It states its own clock.** CORE.md section 6 sets one for the record format: two weeks of real use, and only a change to a validity rule starts them again. The promise sets the same for a bundle and a reply, and states the day each clock started. The mark and its rules are written down before a clock starts, and the start is a commit that anyone can name. It states how many exchanges ran since that day, and between how many people. One exchange does not earn a promise.
- **It names its evidence:** how many logs, from which date, with which build, and which record types they hold. Today two people keep a log. The reports of the second held no reading that bean reads (DECISIONS 306).
- **It claims nothing about a member that a build does not know** (CORE.md rule 8). A share that names its reader can come later. A rule that a pool's builder sets on such a member is the builder's rule.
- **A bundle and a reply can say their format,** and that mark has run its clock. Until then no build can refuse a later format, and no test can hold that.
- **Each locked format names its vectors,** and the second implementation that reads them.
- **If nobody outside the project reviewed it, 1.0 says so.**

**"The person fetches, bean checks."** The owner asked what the three paragons would do about a pack on another entity's server. All three: no network code in bean. A SHA-256 shows that the bytes are the ones that were named. It does not show who made a pack, or that it is true.

**The first run with a peer.**

- Each of the two runs bean and keeps a log. Each runs the commands themselves, and no assistant does: a check prints the other person's values.
- A reader who has only a browser can check a package, and cannot reply.
- Each has readings. A report in a layout that bean reads gives them. So does a value that the person enters by hand, with the type name and unit that a pack lists. [COMPARE.md](COMPARE.md) lists them for the survey's packs.
- A value that was entered by hand reads like a lab's result. The person says so in a note.
- `bean share -for <address>` writes one file, sealed to the reader's address. It goes by a channel that the two already use. `bean share -seal` and its six words are the fallback, as the order above says.
- Each reads the other's package with `check-bundle`, and answers with `reply` and `accept`. For a package that is sealed to an address, `-key` names the key file. [SHARE.md](SHARE.md) says what the two compare by voice.
- A comparison with a pack is not needed for the run. A share does not carry the pack: the sender sends its file too, or the reader builds it.

**What an earlier bean says about a package that is sealed to a key.** Release 0.7.2 asks for six words, and such a package has none. When six words are typed, it refuses, and says that it is another kind of file. The check page says to check the six words. The request for words and the page's line are wrong for such a package. [UPGRADING.md](../UPGRADING.md) quotes both, and both people update first.

**What a package that opens with a key shows.** Only that someone knew the address: anyone can seal to an address. The check says so. The signatures and the identity words show who signed, as today. Today the call for the six words also carries the identity words. With no such call, the two still compare them once.

**Beside these, with no code in bean.**

**A first publisher hosts.** This is phase 2 of the order below.

- A file host that serves a file under its SHA-256. A file and its SHA-256 on a page is a full publication.
- The two packs of the survey.
- A rehearsal with a throwaway identity, before any real key. It is a publication all the same: a fetched file and an event cannot be taken back.
- A relay with a pointer to each pack, because the owner asked for it (DECISIONS 318). Nothing depends on it.
- A pointer is a Nostr event of kind 1063, which exists. bean invents no kind.
- The logs hold no network address of a person, and are kept for a week at most. That is the publisher's word: nobody can check it.
- A document of the setup, in this repository, before the first pack is announced. [HOST-PLAN.md](HOST-PLAN.md) holds its start: what a trial run on one machine showed, and what is not known. It names each program with its version, the rule for uploads, and the tags of a pointer. It holds no key. A second publisher can copy it.
- For the packs, the host and the relay are programs that exist.

**What a pointer may claim.** It is a hint for finding a pack.

- A key that was made for the relay signs it. No key of a log does.
- It does not show who the publisher is, and it never puts a pack on record.
- It is never the second place for a SHA-256. A second place is one that the host's operator does not control, or a build from the source's files.
- A reader who follows a pointer fetches from a host that they do not know, with a tool of their own. The SHA-256 says whether the bytes are the ones that were named.
- A record of the publisher's bean identity is what ties a pack to a publisher (path 4, step 4). It is not built.

**A package by link, after step 4.** The sender puts the sealed file on a file host, with a tool that exists, and sends the link.

- A key that was made for that one upload signs it. No key of a log does: not the signing key, and not an agent's key.
- A link is a convenience, and no host is a gate. A sender with no host sends the file by any channel, as today.
- A host may ask for an account or for payment. bean holds no wallet.
- The link names the file by its SHA-256. The reader computes the SHA-256 of what they fetched, and compares.
- A host learns the size, the time, the key of the upload, and the network address of the upload and of each fetch. It holds the sealed bytes for as long as it likes.
- When a host is gone, the link is gone. The sender holds the file, and sends it again.
- A package goes on a public host only when it is sealed to a key (DECISIONS 313). The design of step 3 first says how long such a key lives. For his own packages the owner added a rule: a key file with a passphrase comes first (DECISIONS 328).
- A reader fetches with a tool of their own.

**A host that offers itself to people for their packages** takes only files that start as an `age` file. It takes them up to a stated size, for a stated time. A host can read that start, and cannot tell what is inside. A sender cannot hold another person's host to this rule. The rule is code in the host, and a second publisher needs the same code. So that code gets a repository beside this one, with no publisher's name, when it is written.

**The owner's example, said plainly.** He named a skin irritation that a person photographs over some days.

- After these steps the person keeps the photos, and a score in their own words.
- They send both to a peer, sealed. A photo goes only when they name its record, with `-files`.
- A photo's file can hold what the camera wrote: a place, a time, a device. bean does not take that out, and the review does not say so yet.
- The peer saves each photo from the package and opens it with any viewer. The peer answers with signed notes.
- bean shows no photo, and sets no two photos side by side.
- bean has no public data on skin, and does not say what an irritation is.

**Not in these steps.**

- A pool of real contributions (path 4, "A pool").
- Finding each other (path 2, step 4). People talk where they already do, and exchange an address there.
- A command that fetches, a table of names, and a unit that bean converts.
- A comparison of photos with a group.

**Known and left.**

- The review before a share does not warn of what a photo's file can hold. That is a small change, with its own audit.
- The second user's reports hold no reading (DECISIONS 306). As the peer of the run, he would enter values by hand.
- A reading that no pack takes waits for a pack. Only a publisher can add it.

**The owner's to decide.** The first two are the author's proposals, which the reviews let stand. The owner asked both questions, and has not answered them.

- **Where the code lives.** Proposed: what runs offline is in this repository, and what listens on a network is not. Formats, checks, builders and the document of a setup are here. A publisher's network address, terms and keys are its own. No key is in any repository.
- **How many spellings of one quantity are reconciled.** Proposed: the one who pools does it, in a pack's `same` list (path 4, "A pool"). bean keeps no table of names.
- Whether the first run goes on six words, if the reply to an address is not merged when the peer is ready.
- Which name the first publisher's server runs under. He said on 2026-11-19 that it is a name under the publisher's own domain (DECISIONS 338). Which programs run there, and who acts on it: [HOST-PLAN.md](HOST-PLAN.md) lists what is decided, proposed and open.
- Who the peer of the first run is, and whether values that were entered by hand are enough for it (DECISIONS 323).
- Which host takes a package by link, once a key file can be protected.
- Whether a publisher's real key is used after the rehearsal. DECISIONS 338 has the author's proposal: yes, as the next step, after the design for a lost key.
- Whether the network leg of the demonstration goes on a real host, his own or others' (POOL-PLAN.md). A pack of made-up people on a real host can mislead. He said "maybe" on 2026-11-19, and DECISIONS 338 has the author's answer.
- Whether bean gets a member for a URL, or an importer for other public files, for receipts (POOL-PLAN.md).
- Whether the tally of receipts sets apart the identities that he lists as known (POOL-PLAN.md).
- Whether the author's reading of his request is right: that the demonstration comes before the order to 0.9 (DECISIONS 332).
- Whether the survey method's own floor of 30 goes as well (DECISIONS 333). The author kept it: the packs on record are built by it.
- Whether a result from a very small cell gets words of its own (DECISIONS 333). Today a cell of one person prints a median, a middle half, and the caveat of a cohort under 100.
- Whether the survey's next pack comes right after the demonstration of the pool path, or after 0.9.
- Whether 1.0 can come while `pack/0` is a draft. The reviews recommend it: 1.0 then names the comparing part as not locked, and does not wait for a second source that nobody has.
- Whether a review by a person outside the project is a condition of 1.0. The reviews recommend that it is wanted, and is no condition.
- Whether bean ever keeps a package or a pack with the log, so that an export holds it. For a pack, that is item 7 of the work in the section "Order".
- Whether a record ever gets a mark for a value that was entered by hand. It would be a change to the format.

## Path 1: a sealed package for one reader

**The need.** A person sends records to a doctor or a peer. Only that reader can open the package. The reader may have no key and no account.

**The plan.**

1. **Seal with a passphrase first. Done:** `bean share -seal` ([SHARE.md](SHARE.md), DECISIONS 275). bean packs the bundle into one file, encrypts it, and makes six random words. The person gives the words to the reader by another channel, such as a phone call. The check page asks for the words, opens the file on the reader's device, and runs the check. This needs no key from the reader.
2. **Seal to a reader's key. Done for a reader who runs bean** (DECISIONS 327). `bean address` makes a key file and its address. `bean share -for <address>` seals the package to it, and `-key <file>` opens it. The design is [SHARE-PLAN.md](SHARE-PLAN.md) (DECISIONS 326). **Not done:** a key that the check page makes for a reader with only a browser. The plan lists it as later, with a design of its own: a forged page would take a key that opens every package.
3. **Use a standard format. Done.** The file is an `age` file, so a tool other than bean can open it. bean invents no cipher.
4. **Say what leaves. Done (DECISIONS 324).** The review before a share says that the person's identity id travels. It flags a kept lab report, which holds a name and a birth date. It lists every field that a record carries beside its row, with its values.
5. **Record who it was for.** The share record says that a passphrase was used. With a reader's address it says `"seal": "recipient"`, and no more. It names no address until something reads one (SHARE-PLAN.md).

6. **The reader knows who signed. Done:** identity words (DECISIONS 280). The sender reads eight words in the call that carries the opening words, and the check shows the same eight (DECISIONS 287).
7. **One public address for the check page. Done (DECISIONS 311):** the releases of the repository, which is public. A tag on `main` builds the page on the code host's machine and publishes it with its SHA-256. CI records the SHA-256 of every push as well. Not done: a second, independent place that states the SHA-256, and a short address of the project's own.

**What it cannot do.** A reader who has opened the package holds a copy for good. Withdrawing permission stops the next share, not the last one.

**Open.**

- Should `-seal` become what `bean share` does unless told otherwise? Today a plain folder is the default, because the checks and the examples are built on it.
- Does the page need a project domain? Today it lives in the releases of the code host. A second, independent place for its SHA-256 is what catches a host that serves a changed page.
- Should `bean share` take `-except <id>`? Today a person who sees one record that must not leave names all the others by id, or narrows by time. For a long share that is many ids (DECISIONS 324).
- Does a package need an end date after which the page refuses to open it? An end date stops an honest reader's page. It does not stop a copy.

## Path 2: people compare with each other

**The need.** A person with a condition wants to learn from others in the same situation: what they measured, what they tried, what happened. bean is the way they exchange evidence with people they trust. bean does not diagnose, rank treatments or advise.

**The plan, in steps that each stand alone.**

1. **One person to one person.** Path 1 is this step. A person sends a sealed package, such as a biopsy report with their notes, to a peer they already know. The peer checks it and reads it.
2. **A reply that the owner can keep. Done:** `bean reply` and `bean accept` ([SHARE.md](SHARE.md), DECISIONS 290). A peer who runs bean signs notes about the records that they received, and sends them back sealed. The owner compares the peer's identity words and the reply's id, and takes the notes into their log under a name. A note is text about one shared record, and a reply that holds anything else is refused. The log remembers how far a peer's chain of keys had got, so a retired key takes nothing later. `bean unpair` takes a name back.
3. **Side by side. Dropped by the owner on 2026-11-21 (DECISIONS 342).** Once a package checks, `bean check-bundle` shows the peer's records, with who signed each. bean shows no merged view. The first run with a peer stays: it is step 1 of the section "Next". The number stays, so that "path 2, step 4" still names finding each other.
4. **Finding each other.** A person publishes a short card: a condition, what they look for, and a reader address. It holds no measurement. Others answer with a sealed package. Where cards live is not decided.
5. **A statistic over a group.** A group wants its own distribution, and no member wants to show their values. This is the one step that needs computing on hidden values. It waits for a real group, and for steps 1, 2 and 4.

**On homomorphic encryption.** Steps 1, 2 and 4 need none: each person shows chosen records to chosen people. Step 5 is where it applies. For a small group, a simpler method than full homomorphic encryption may do. Each member splits their value into shares, and only the sum is opened. The choice waits for a real group and a real question.

**Risks to design for.**

- A card for a rare condition can identify a person. A card is coarse, and it carries a reader address made for it, not the person's identity id.
- A claim is a claim. A peer's account of a treatment is not evidence that it works, and bean says so wherever it shows one.
- Any output that ranks, scores or recommends is a different product, with its own review. The steps above produce none.

**Open.**

- The first peers are people who already know and trust each other (the owner, 2026-11-11). So steps 1 and 2 come first, and step 4 waits.
- A reply from a reader who has only a browser. The page would make a key for that reader and sign with it. A key that the page makes is later (SHARE-PLAN.md).
- A peer's notes in the peer's own log. Today the peer keeps the reply file and its words. Their log holds only a record that they replied. A peer who replies to an address cannot open the reply again.
- A reply to a reply. Today a note is about a record of a share, and the owner answers with a new share.
- Who runs the place where cards live: nobody (people pass addresses by hand), an open network, or AION?
- A spelling that no pack has waits for a pack (REFERENCE-PLAN decision 4). A table of names waits for a real case (DECISIONS 323).

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

**Next for the breadth of tests.** `bean compare -type 'lab.*'` names the types with no reference data in one line, with no value. [REFERENCE-PLAN.md](REFERENCE-PLAN.md) decides how each of these is served, or what it waits for (DECISIONS 321).

- **A value below a detection limit:** ALT, GGT, total bilirubin and C-reactive protein. They are built in `pack/0` with the source's own value and a caveat, where few people are marked. Each must still pass the rule on analyzers.
- **The fasting part of the sample:** glucose, triglycerides, LDL and insulin. They need a weight for each test, which the method does not have. That is a decision of its own.
- **More of the blood count.** A white cell share is built when real reports print it in the survey's own step. The counts of basophils and eosinophils wait: they are coarse, and they need a value of zero.
- **Tests where two analyzers differ too much for a percentile:** sodium, chloride, calcium, AST, alkaline phosphatase and iron. A percentile is the wrong result for them. Another kind of result may fit, such as a range that the source states.
- **A caveat with a size.** Each result says that the instruments differ, and not by how much. For the tests of the biochemistry profile CDC gives a figure, and a measure's caveat could state it.
- **Albumin, globulin and carbon dioxide**, when a source confirms that the lab and NHANES measure one thing ([COMPARE.md](COMPARE.md)).
- **Hormones, vitamins and thyroid tests** are in other files, some of other survey years. A second cycle is a second set of files and weights.

**A pool: a pack from what people contribute (DECISIONS 325).** A group's own data is a source like any other, and its pack is a pack like any other.

**The first pool is a test of the path.** It holds nothing that the survey's pack lacks, and it tests no mapping. [POOL-PLAN.md](POOL-PLAN.md) says what is built, and DECISIONS 332 has the account. The builder and its test are built (DECISIONS 334).

- **A script makes every contributor.** Each seals a share to the operator's address, as a person would to a publisher's. The builder cannot check for whom a share was sealed.
- **It is built from saved comparisons.** A saved comparison names its reading, its measure and its cohort. `bean share -type 'comparison.*'` sends each with its reading. So the sharer's own bean did the mapping, with a survey pack.
- **The builder is one program in Python, under `tools/`.** It checks a bundle with `checkbundle.py`, a claim with `reproduce.py`, and its pack with `check.py`. Its rules are its own, and the program is the rule.
- **The pack's measures are the survey pack's:** ids, labels, units and `same` lists. A cell has no least number of identities (DECISIONS 333). One cell is enough for the test.
- **The test's rule is one reading for each identity and measure.** POOL-PLAN.md states it in full, with each case that is left out. That rule is for the test alone.
- **The pack says what it is, where a reader sees it.** A cohort's id starts with `pool-`, and its text with `synthetic pool:`. Its caveats are the builder's own, and never name the survey's publisher as the pool's.
- **The bundles are one archive in this repository, a test vector,** beside the survey pack that their claims name. The pack names both in `source.files`, and anyone builds the pack again. It is used with `-unknown-pack`.

**What a pool of real people needs first, on paper.** None of it is built.

- **A pack from data that is not public.** `pack/0` names in `source.files` a file that anyone can fetch. A real pool has none. So this is a decision on the format, and on a publisher's signature (the plan below, step 4, and "Open").
- **A contribution that the person meant.** A share is not bound to a reader today, so anyone who holds one can pass it on to a pool. A contribution counts only when its share record names the publisher's address. A share record names no address yet (SHARE-PLAN.md). A pool brings that member, with a design of its own.
- **A pack that is published cannot be taken back.** Whoever fetched it holds a copy.
- **One value for each identity and measure, and a person can correct it.** The rule of DECISIONS 325 takes the reading with the latest time, among the readings with a claim that counts. A real pool sets its own rule, and says it.
- **Mapping stays with the one who pools.** The publisher's rule decides which type and unit is which measure. The rule has a version, and its result is the pack's `same` list. bean keeps no table of names, and no reader converts a unit.
- **A reading that no `same` list has is not pooled.** The publisher counts such readings, and the counts are its list of work.
- **What a pool's pack may claim.** It says that it is a pool, and that each contributor chose a cohort. Its n counts identities, not people: one person can make many. bean's own line under a result says "in this sample" today, and a pack's n is "the number of people". A pool is not a sample, so both need other words for a pool.
- **Consent, and how long a contribution is kept.** A person cannot see what a publisher holds, and cannot take a reading back.
- **A review of the law, before a publisher holds a person's readings.**
- **What the publisher learns:** at least each contributor's reading, its times, the cohort and the identity id. SHARE.md lists what else a share carries. A person may contribute under an identity that they made for the pool.
- **A cell shows real values, with no name.** The rule says whether the builder groups or rounds them. Two packs that differ by one contributor show that person's value.
- **A pool cannot tell a real contribution from one that was made up.** One person with many keys can fill a cell. A device's signature narrows that: a signed reading was made by a device, and not typed (path 3). It does not show whose body was measured.

**Proposed additions to that list (DECISIONS 328). The owner decides, on the day a group exists.**

- **What a contributor is told in writing, and agrees to in writing.** The publisher collects the plain values of every contributor. The words say who can read them, and how long they are kept. A pack lists every value of a cell, so whoever holds the pack reads each value in it. No such words, no contribution.
- **A contribution asks the person at a terminal,** and shows every field that leaves, as a share does. bean cannot tell a person from a program that holds a terminal, and the words say so.
- **Leaving before the next pack does not take back a value that is in an earlier one.**
- **A reader of such a pack cannot build it again:** only the publisher holds the contributions. A result that was made with it says so, and never calls the pack's n "the number of people". Today's warning tells a person to build a pack again from its files: for a pool it gets other words.

**The plan.**

1. **A second source, from another publisher.** The format is not frozen before a second real source is built into a pack. Each candidate is confirmed first from its publisher's own site: the terms, a stable address, and whether a file changes without notice.
2. **Publish the NHANES pack** beside the check page, with its SHA-256, and mirror it (the section on where the files live). Then a person needs one download of under 1 MB, not eight files. The owner read the terms of the source and gave the word (DECISIONS 314).
3. **The check page computes a shared comparison again**, when the reader gives it the pack.
4. **A publisher signs.** A record by the publisher's bean identity names a pack's SHA-256, and travels as a bundle. This is built when a second publisher exists.
5. **A pointer on relays.** A publisher announces a pack on Nostr relays: title, SHA-256, where it is. The relays hold a pointer and no health record. A key that was made for the relay signs it, and no key of a log (the section "Next").
6. **More kinds of cell**, each when a real source needs it: percentile points, a fitted curve, counts of a category. REFERENCE.md says what each may state.

**The person's own list of packs: scoped, and it waits (DECISIONS 308).** The first user outside the owner asked to choose his sources, and in time to use the sources of his peers. Today any pack works with `-unknown-pack`, so nobody is locked in. A signed record, "I use this pack", is the trace that a peer's vouch will be. It is built when a pack exists from a second source, or when a peer asks to send their list. Entry 308 holds the rules for it. The owner was asked again on 2026-11-16, and said to wait (DECISIONS 321). He is asked again with each release that puts a pack on record.

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

1. **Mirror.** The check page is in the project's releases. A publisher puts a pack on a host of its own. It puts the same bytes on one or two Blossom servers, with an existing Blossom client. It states the SHA-256 in a second place.
2. **One line in the documents.** A `curl` command with the server and the SHA-256, then the check. bean fetches nothing itself.
3. **A signed note with each SHA-256.** A note on Nostr relays names the title, the SHA-256 and the servers. bean has no key of its own (DECISIONS 317). A note is a hint for finding a file. It is not a second place for a digest. The operator of a host can write a note that agrees with a changed file (DECISIONS 325).
4. **A sealed package by link.** A person uploads the encrypted file with a Blossom client and sends the link. A package on a public host is sealed to the reader's key, not with words (DECISIONS 313, 325). The section "Next" has the rules. SHARE.md then says what the server sees.
5. **A fetch command in bean, only if people ask.** Until then bean stays offline.

**What a server learns.** That someone fetched a file, from which address, and when. For a package it also holds the size and the ciphertext, for as long as it likes. Nobody can make a server delete a file.

**What never goes there.** A record of a person: record files are plain JSON. The trial run of POOL-PLAN.md puts made-up records on a test server on one machine. A request for one cohort, or one that carries a value.

**Open.**

- Which servers, and who pays for the bytes. How long a server keeps a file differs, so use more than one.
- The Blossom documents are drafts. Read them again before step 1: the upload, the list of servers, and the answer that asks for payment.
- Should the project run a Blossom server of its own? Not before someone needs it.
- Does a link to a package need to work once only? A server that keeps the ciphertext keeps it, whatever the link does.

## Order

The owner confirmed the long aim on 2026-11-16. A person keeps their own log. They share with people they trust. They compare with reference data from many sources, and they choose which sources count. Anyone can publish a source, and no publisher is special.

He then said that the infrastructure must be right before new capabilities are added. A design of the whole was written and judged by five reviews (DECISIONS 317). The design holds, with changes. This is the order that came out of it.

**Phase 0: mend what is shipped. Done in 0.7.1 (DECISIONS 319).** A saved comparison must mean the same for ever. Its standing changed when a pack was put on record. That is mended in bean and in the Python tool. The tests now hold a set of comparisons that a release saved, and what bean holds of each pack on record.

**Phase 1: decide on paper what is hard to change. Done (DECISIONS 321).** [REFERENCE-PLAN.md](REFERENCE-PLAN.md) is the result, after six laps of reviews by readers who had not seen the work. Its main result is a refusal: no new format, no new record type and no new method version now. Each of them waits for a real case, and the plan names that case. It decides how a pack's origin is checked, how the tests with a detection limit are served, and what a kept pack is. It says where bean falls short of the rule that no publisher is special. It does not give the first user his wish to choose his sources: that waits for a second source.

**The work that follows from it, in order.**

1. Mend what is shipped: a share with `-files` carried a kept file as public by its record's type alone. **Done in 0.7.2 (DECISIONS 322).**
2. The documents: recipes with a form to copy, the rules for a builder, the naming rule with vectors, and what "on record" means.
3. The words for a format that a reader does not know, and for a cell that a pack does not have.
4. The builders take a version of their rule, and version 1 still builds both packs on record.
5. The next pack, in `pack/0`, by version 2 of the builder's rule. It is on record in the release that first builds it.
6. The fasting tests, after a decision of their own.
7. Keeping a pack with the log: compare by SHA-256, a share that carries the pack, and a check that computes a shared comparison again.

**Phase 2: a first publisher hosts.** It needs nothing from bean, and can start at any time. A file host that serves a pack under its SHA-256, the SHA-256 stated in two places, and a second host. A relay with signed pointers runs from the start, as a way to find packs. Nothing depends on the relay. Before any real key: a rehearsal with a throwaway identity, a second person who holds the recovery words, and a written list of steps. Open for this phase: who keeps a copy of the survey's files and of each release, for the day that their host is gone.

**Phase 3: when a second party exists.** The person's own list of packs (DECISIONS 308). And a person takes in the record of a peer or a publisher who keeps a pack.

**The next steps (DECISIONS 325)** are in the section "Next", near the top. They take a first run with a peer, phase 2, and the seal to a reader's key. They name one builder of a pool. The owner asked for a demonstration of it (DECISIONS 332), and by the author's reading it comes first. The documents of item 2 above do not wait. The other work that follows from phase 1 keeps its order, after the first run with a peer.

Not planned: a command in bean that fetches from a network, and a view that ranks or scores two people. bean never runs a program that a pack names. A public card of what a person holds is not planned either.

## Smaller open paths

- **Reading a lab report in a layout that bean does not know** (DECISIONS 306). The first real set of reports, 49 from two labs over 14 years, read none. The file is kept now. Next, in this order. Measure with `bean-import-lab -count` on real reports how far the rows read once the lines above the column header are passed over. Only if they read well: a second reader that proposes rows, each beside the cell it was read from. The person accepts a row, and `-yes` never takes one. Each such reading says which reader read it, and where in the file. Not planned: a reader for each layout that a person reports, and a reader that guesses with no review.
- **A file that only another key's record names.** `bean purge` keeps a file that another record names, and says so. When that record is a peer's or a stranger's, no export, share or assistant reads the file. Its encrypted copy stays all the same, and `bean sync` copies it. Decide whether such a record should keep a file at all.
- **More tests and cohort rules.** Each is one row in `internal/nhanes`, from the file's own label and a real report's name.
- **More importers,** each from a real file, never from memory. A time zone flag for CSV times.
- **A check of a whole export in Python.**
- **Release.** Releases 0.7.0 and 0.7.1 are out, each with `bean-check.html` and its SHA-256 (DECISIONS 315, 319). Open: a neutral home ([GOVERNANCE-PROPOSAL.md](GOVERNANCE-PROPOSAL.md)).
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
- From the reviews of the share review (DECISIONS 324):
  - `view.Safe` shows a space other than U+0020, a combining mark and a few rare blank characters as they are. A value can look blank, struck through, or like another value. It cannot hide a name: the letters are shown.
  - A field of more than 50 different numbers or times shows its count. Many numbers could still spell a message.
- From the reviews of the plan for reference data (DECISIONS 321):
  - For a cell that a pack does not have, bean said that a cohort of fewer than 30 people is left out. A pack can leave a cell out for another reason. Done in 0.8.2: bean gives no reason.
  - For a pack in a format that it does not know, bean says that the file is not a pack of version 0.
  - `bean purge` of the record that keeps a file erases the file, also when a saved comparison is about that record. The same file cannot be kept again on that copy.
  - A pack that is not in canonical form is accepted, as another pack.
  - Nothing marks a pack on record that is later found wrong.
- No way to test the 24 words without rotating. `net/http` is in the binary only to detect image types. CI does not run `lint`, `vuln` or fuzzing.

## Not planned

A central data platform that a person cannot leave. A UI beyond a static report. Relay publishing of health records. A service that computes a comparison from a person's value or cohort, paid or free. A wallet in bean. Encryption of record files at rest (use an encrypted volume). Whoop and Oura importers until there is a real export. Zero-knowledge proofs, and distributed trust scores. Any output that diagnoses, predicts or advises.
