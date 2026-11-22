# A first host for the survey's packs: a plan on paper

This is for anyone who sets up, or reviews, a publisher's first host. Nothing of it is set up. DECISIONS 337 has the account of its reviews.

**It changes nothing in bean.** bean fetches nothing. It signs no upload and no pointer. A host and a relay are programs that exist. A publisher's network address, terms and keys are its own, and none is in this repository.

**A host is a convenience, and never a gate.** A file and its SHA-256 on a page is a full publication ([REFERENCE.md](REFERENCE.md)). Any web server does that, and it is the cheapest way. A file host of the Blossom kind and a relay are additions. The owner wants the relay (DECISIONS 318), and said that the lab's server can host both. A person can also build each pack from the survey's files, with no host at all. That way stays.

**bean does not diagnose.** A pack says where a value lies in a sample, and not whether it is healthy.

## The need

The owner asked for this plan on 2026-11-19, and said where it can run:

```
we have a server in the lab we can host a relay + blossom on
```

[ROADMAP.md](ROADMAP.md) has the rules, under "A first publisher hosts" and "Phase 2". This plan puts them in an order.

## What is true today

**H1. The steps ran on one machine.** `examples/pool/run.sh` uploads a file, publishes an event of kind 1063, finds it, and fetches the file. It uses the test server of `nak` 0.22.3. That server keeps everything in memory, and is no host. Of the rules below, the trial tried only the first.

**H2. No real file host and no real relay has run for this project.** No program is chosen. So no rule below is tried against a real program.

**H3. The two packs are in no repository as files.** A person builds each with `bean-pack-nhanes` from the survey's files. REFERENCE.md states the SHA-256 of each, and bean has both on record.

**H4. No second person has stated the digests of the survey's files.** `tools/receipts` tallies such statements, and has had no real one.

**H5. Nothing ties a pack to a publisher.** A record of a publisher's bean identity is not built. A pointer shows a key, and no more.

## What was decided before, and by whom

**By the owner.**

- **A relay is in the first setup** (DECISIONS 314, 318). The reviews said that a file host is enough to begin. He wants the relay and its pointers tried from the start. In this plan the relay is in the trial of stage 2, and opens at stage 4. Nothing depends on the relay.
- **He gave the word to host the two packs of the survey** (DECISIONS 314). He had read the survey's terms. They limit how the data is used, and say nothing on handing it on (DECISIONS 313).
- **The first data on a publisher's host is those two packs** (DECISIONS 325).
- **No package of his goes on a public host** before a key file can have a passphrase (DECISIONS 328).
- **No publisher is special, and the documents name none** (DECISIONS 318).
- **The address is a name under the publisher's own domain** (DECISIONS 338). Which name is open.

**By the reviews of DECISIONS 313 and 317, and by ROADMAP.**

- **A publisher's own key waits.** First a design says how a reader knows the publisher when a key is lost. The policy for that key is the owner's to decide, and it is open. This plan makes no such key.
- **A rehearsal with a throwaway identity comes before any real key.** Whether a real key is used after it is the owner's to decide.

**The author proposes this plan as that rehearsal:** each key of it is thrown away. It is a publication all the same: the packs are the real ones, and what it makes public cannot be taken back. It rehearses the host, the relay and the pointers. It rehearses no bean identity of a publisher, no recovery words and no successor.

## What goes on the host

The two packs of the survey, each under its SHA-256.

**What never goes there.** The author proposes this list. The owner's rule of DECISIONS 328 is narrower: it covers his own packages.

- A pack of made-up people, also in a trial. On a host it can mislead.
- A sealed package of a person, or a record of a person.
- A key of bean, or a Nostr key.

The rules below keep other files off the file host. The relay is different: the content of a pointer is free text. The owner reads its words before it is signed.

## The rules of the host

The first three are ROADMAP's. The author proposes the others.

- The file host serves a file under its SHA-256.
- The logs hold no network address of a person, and are kept for a week at most. That is the publisher's word: nobody can check it.
- A pointer is a hint for finding a pack. It does not show who the publisher is, and puts no pack on record.
- **Proposed:** the file host and the relay answer anyone. A reader needs no account and no key, and for a pack only a web client.
- **Proposed:** the file host takes the two packs, and no other file. Then uploads are closed. A new pack opens them again, by the owner's word.
- **Proposed:** the relay takes two events of kind 1063, from the pointer key. Then it takes no event. So no other person's event is kept there.
- **Proposed:** the address is served with TLS.
- **Proposed:** when no program can hold a rule, the owner decides. He takes another program, or the rule changes in this document first. No rule is worked around.

How a program holds the fifth rule is not known: by a list of SHA-256, or by one upload key. The trial of stage 2 finds it.

A reader can check the bytes that they fetch, and that TLS answers. What the host takes and what it keeps are settings of the publisher. A reader cannot check those.

A second publisher copies what it wants of this, and asks nobody. It reads the terms of its source itself: the word to host its copies is its own. The two packs of the survey are on record for anyone who serves them. A pack of another source is not, and compares only with `-unknown-pack`. The list is in bean's code, so a new pack gets on it only by a change to bean. REFERENCE.md says that this falls short of the rule that no publisher is special.

## The keys

The author proposes this part, and the owner decides.

No secret key of bean and no secret Nostr key of a person is on the server. A person signs on another machine. The server has secrets of its own: the key of the TLS certificate, and what asks for that certificate. Whether a program makes a key of its own is not known.

- **The upload key** is made at stage 3 for the two uploads, and is not kept after the last try with it. A program that takes uploads without a key needs none.
- **The pointer key** is made at stage 4 for the two pointers, and is not kept after the last try with it. It is the throwaway identity of ROADMAP's rule.
- **The trial of stage 2 has keys of its own, and so has each try that must be refused.** They are made the same way, and are not kept.
- **Each key is made as `examples/pool/run.sh` makes one:** by a command, into a variable of a shell, and into no file. Other users of a machine can see a key that a command gets. So the signer uses a machine that is theirs alone.
- **Nobody can show that a key is gone.** A copy from before can sign again, on any relay. So a key of this plan means nothing to a reader.
- **The cost: with the key gone, nobody can ask a relay to delete a pointer.** Whoever holds the relay removes the event there, and a copy elsewhere stays. A host that moves gets new pointers, under a new key.
- **DECISIONS 313 calls a note under a throwaway key a test, and no more.** A pointer on an open relay is more: it is public, and it stays.

## The stages

The author proposes this gate. Each stage starts on the owner's word at that time. Stages 3 and 4 need his word a second time, before the first act that cannot be taken back. The owner, or a person whom he names, does each act on the server. The stages are written with the keys as the author proposes them.

Without that word, an assistant does nothing on the server or on the lab's network. It does none of these for this plan:

- install a program, or run a command that listens on a network;
- open a port, change a firewall, or register or change a name;
- put the setup document into a repository;
- ask for a certificate;
- upload a file, sign, or make a key;
- announce the packs.

1. **Build the two packs.** This needs no server. A person fetches the survey's files first: at most 21 MB, and [COMPARE.md](COMPARE.md) says from where. Then `bean-pack-nhanes -out <new file> <the survey's files>`: six files for the first pack, and those six with two more for the second. REFERENCE.md names them. `bean compare -pack <file>` must print that bean has the pack on record. `shasum -a 256 <file>` must give the SHA-256 in REFERENCE.md.
2. **Choose the programs, and try them closed.** First the owner says how the server is kept apart from the lab's other machines. One file host, one relay, and what serves TLS before them. The programs listen only inside the lab's network. The server must have no public name, and no certificate from a public authority. TLS is tried with a certificate that the lab makes itself. A person tries from outside the lab, and nothing must answer. Each rule above is tried with files of random bytes, and no pack goes on the server yet. Each way in which a program takes a write is tried, and not only an upload and an event. The person who does the trial writes the setup down as it is made, in a new document, apart from this plan. It starts from the part below on what is known. It names each program, its version, and its settings. It stays out of every repository until stage 3. Then whoever holds the server removes the trial's files and events, and takes the trial's keys out of the settings. The trial's keys are not kept. The stage is done when each rule held, or the owner decided on one that did not, and the document is written.
3. **Open the file host.** The owner gave the word to host (above). In this order:
   1. When the program takes uploads by key, a person makes the upload key. Its public half goes into the settings on the server, and into no repository.
   2. The two packs go up while the server is still closed.
   3. Uploads are closed, and the public half leaves the settings. A person inside the lab tries an upload of random bytes with another key, and one with the upload key when there is one. The host must refuse each. Then the upload key is not kept.
   4. **The owner's word, a second time.** What follows is a publication. He reads the setup document first.
   5. The setup document goes into this repository. It holds no key, no publisher's name, and no address inside the lab.
   6. The server gets its name, its public certificate, and an open port for the file host. To get a certificate, a port can have to be open first. The relay stays closed: it answers nothing from outside.
   7. A person outside the lab fetches each pack, and compares its SHA-256 with REFERENCE.md. They try an upload of random bytes under a key of their own, and the host must refuse it. They try the relay, and nothing must answer.

   The first three acts can be undone: whoever holds the server removes the files. From the fifth on they are a publication, and cannot be taken back. A check that fails stops the stage. A port that is open closes first, and the owner decides what is next.
4. **One pointer for each pack, and the relay opens.** In this order:
   1. The owner reads the content of each pointer before it is signed. It says plainly what the pack is. Its `url` is the address from which the person outside fetched the pack at stage 3.
   2. A person makes the pointer key. Its public half goes into the relay's settings.
   3. The person signs the two events, and sends them to the relay while it is still closed.
   4. The relay takes no more events, and the public half leaves its settings. A person inside the lab reads each event from the relay. They check its signature, its `url` and its `size`, and its `x` against REFERENCE.md.
   5. That person tries to publish an event under the pointer key, and one under another key. The relay must refuse both. Then the pointer key is not kept.
   6. **The owner's word, a second time.** Then the relay's port opens for reading. From here the pointers are public.
   7. A person outside the lab reads each pointer from the relay, checks its signature, and fetches the pack by its `url`. They try to publish an event under a key of their own, and the relay must refuse it.
   8. To announce the packs is an act of its own, and waits for its own word. First the setup document is made true to what was done. The owner says where the packs are announced, and it is no document of this repository. The announcement names the address of the file host and of the relay, each SHA-256, and no key.

   The first five acts can be undone: whoever holds the relay removes the events. After the port opens, a copy can be elsewhere. A check that fails stops the stage. A port that is open closes first, and the owner decides what is next.

The author proposes that the relay uses the name and the certificate of the file host. Then stage 4 makes no new name.

A try from outside is one try from one network. It is a sample, and no proof.

The setup document names each setting that changes after the trial: where each key that may write goes, and the files that the host takes. It holds no key.

**What cannot be taken back, by stage.**

- **Stage 3:** the setup document in this repository is public for good. A name is public when it is set. A certificate from a public authority puts the name in public logs for good. A file that was fetched stays fetched.
- **Stage 4:** an event, and an announcement. To delete an event is a request to each relay, and a copy can stay elsewhere. With the key gone, that request cannot be made.

**Not stages of this plan.**

- **A publisher's own key.** It waits for the design and the policy that are named above. So does the sealed copy of its recovery words, for which the owner named a second person (DECISIONS 318).
- **A second place for the digests.** A second person fetches the survey's files from their own connection, and sends receipts ([tools/receipts](../tools/receipts/README.md)). They build each pack, and state its SHA-256. That needs no server. Until then one hand holds the host, the relay and this repository.
- **A second host,** and who keeps a copy of the survey's files for the day that their host is gone. ROADMAP has both under "Phase 2".

No stage needs a change to bean. The first run with a peer needs no server. The owner decided all the same that the host comes first (DECISIONS 339). By the author's reading that run waits for stage 3, where a person can fetch a pack. No other step of ROADMAP waits for this plan.

## The setup, as far as it is known

**Known, from the trial run on one machine.**

- **An upload:** `nak blossom --server <host> upload <file>`, with a key in `NOSTR_SECRET_KEY` for that one command. The host answers with a SHA-256, and the uploader compares it with their own.
- **A pointer:** one event of kind 1063. Its tags are `url`, `m` with `application/json`, `x` with the SHA-256, and `size`. The trial's reader refuses a pointer that has `url`, `x` or `size` twice.
- **With no key, `nak` signs with a default key of the machine.** Each command that signs gets its key in `NOSTR_SECRET_KEY`.

**Read in the settings of two public programs, and not tried.**

- **`nostr-rs-relay`** has a list of the keys that may publish, and a list of the kinds that it takes. So it may hold the relay's rule alone.
- **`blossom-server`** removes a file that none of its rules keeps. A setup with it needs a rule that keeps the two packs for good.

**Not known.**

- Which programs run, their versions, and whether each can hold the rules above.
- How the logs are held to a week, and to no network address.
- Limits of size and of rate, and who patches the server.

## What each party learns

- **The file host, and what serves TLS before it:** the network address and the time of each fetch, and which of the two packs.
- **The network of the lab:** the address and the time of each fetch. The size of an answer can show which pack.
- **The relay:** the network address of whoever asks, and what they ask for. One hand holds the relay and the file host, and can tie the two together.
- **Whoever answers for the name, and the reader's own network:** that a reader asked for the name. The reader's network also sees the size of the answer.
- **The public logs of certificates:** the name, for good.
- **Anyone:** where the server is, and from that a guess at the publisher.
- **A reader can show less.** They take the file from a person who has it, and check its SHA-256. Or they build the pack from the survey's files.

## What a reader may conclude

- The host served them, that one time, the bytes that they got. A host can serve other bytes to another reader.
- **The SHA-256 to hold a pack to is the one in REFERENCE.md.** The one in a pointer is the pointer's word. bean computes the SHA-256 of a pack, and says whether the pack is on record.
- **The key of a pointer says nothing.** No key of this plan signs the announcement. Nothing ties a pack to a publisher (H5). A reader can guess the publisher from the address.
- **Anyone can publish a pointer** to any file, under any key, on any relay. Nothing in a pointer tells such a one apart. bean's list does: a pack that is not on record compares only with `-unknown-pack`.
- Not that a second person checked the digests. One hand holds the host, the relay and this repository (H4).

## The owner's to decide

DECISIONS 338 has his answers of 2026-11-19. Each line below says what is decided, what the author proposes, and what is open. A proposal is no decision.

- **The address.** Decided: a name under the publisher's own domain. Open: which name, how long it is held, and who renews it.
- **The relay.** Proposed: `nostr-rs-relay`, with no second program beside it.
- **The file host.** Proposed: a public program, so that a second publisher can get it. Open: which one.
- **Who acts.** Proposed: the owner, or a person whom he names, does each act on the server. An assistant writes the settings and the commands as files, and he reads them first. Open: who patches the server.
- **The person outside the lab,** who tries and fetches at stages 2, 3 and 4. Proposed: the peer of the first run, who states what they fetched as receipts.
- **The keys.** Proposed: each is thrown away for this publication. A kept key is the next step, after the design for a lost key.
- **A pack of made-up people on this host.** He said: maybe. Proposed: never on a public address. A test server that only the lab's network reaches does the same for a demonstration.
- **The order with the first run with a peer.** Decided: the host first (DECISIONS 339). Open: whether the run waits for the pointers of stage 4 too.
- **More packs.** He wants more packs that are worth a host. A search for sources is proposed as work of its own.
- Whether the relay uses the name of the file host. Proposed: yes.
- How the server is kept apart from the lab's other machines. The limits of size and of rate.
- Where the packs are announced, and how long the host stays up.
- A second host.
- **A package between two peers on this host.** Not in this plan. ROADMAP.md, "The week ahead", says what it needs first (DECISIONS 341).
- **What the setup document names.** Proposed: only what was run, as it was run. A public program that nobody tried is named as one that was read and not tried.

## Known weak points

- **Nothing here is tried against a real program** (H2).
- **One hand holds everything** until a second person states the digests.
- **What is published stays published.** A pack, a name and a pointer cannot be called back. With the pointer key gone, nobody can ask another relay to delete a copy.
- **A server in a lab has no promise of being up.** When it is down, each pointer leads nowhere. The packs can still be built from the survey's files.
- **A name can pass to a stranger.** It lapses, or another takes it, or whoever answers for it turns. Each pointer keeps its `url` for good. The stranger sees each fetch, can get a certificate, and can serve other bytes. A reader who compares the SHA-256 with REFERENCE.md can tell whether the bytes are the ones that this repository names.
- **The survey's files are on the survey's own host.** Nobody here keeps a copy for the day that it is gone (ROADMAP, "Phase 2").
- **A host on a network can be attacked.** It serves two files. Closed uploads and a closed relay keep its surface small. The setup document names its programs and versions, and an attacker reads that too.
- **Whoever holds the server can open uploads again.** Closed is a setting, and no proof.
- **The rules of the logs are a word.** Nobody outside can check them. They name the logs of the file host, of the relay, and of what serves TLS. What the lab's own network keeps is not said here.
