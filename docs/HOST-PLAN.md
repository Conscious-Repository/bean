# A first host for the survey's packs: a plan on paper

This is for anyone who sets up, or reviews, a publisher's first host. Nothing of it is set up. Reviews judged it on paper, one for each paragon, and DECISIONS 337 has the account.

**It changes nothing in bean.** bean fetches nothing, and signs nothing. A host and a relay are programs that exist. A publisher's network address, terms and keys are its own, and none is in this repository.

**A host is a convenience, and never a gate.** A file and its SHA-256 on a page is a full publication ([REFERENCE.md](REFERENCE.md)). A person can also build each pack from the survey's files, with no host at all. That way stays.

**bean does not diagnose.** A pack says where a value lies in a sample, and not whether it is healthy.

## The need

The owner asked for this plan on 2026-11-19, and said where it can run:

```
we have a server in the lab we can host a relay + blossom on
```

[ROADMAP.md](ROADMAP.md) has the rules, under "A first publisher hosts" and "Phase 2". This plan puts them in an order.

## What is true today

**H1. The steps ran on one machine.** `examples/pool/run.sh` uploads a file, publishes an event of kind 1063, finds it, and fetches the file. It uses the test server of `nak` 0.22.3. That server keeps everything in memory, holds none of the rules below, and is no host.

**H2. No file host and no relay has run for this project.** No program is chosen. So no rule below is tried against a real program.

**H3. The two packs are in no repository as files.** A person builds each with `bean-pack-nhanes` from the survey's files. REFERENCE.md states the SHA-256 of each, and bean has both on record.

**H4. No second person has stated the digests of the survey's files.** `tools/receipts` tallies such statements, and has had no real one.

**H5. Nothing ties a pack to a publisher.** A record of a publisher's bean identity is not built. A pointer shows a key, and no more.

## What the owner decided before

- **A relay is in the first setup** (DECISIONS 318). The reviews said that a file host is enough to begin. He wants the relay and its pointers tried from the start. Nothing depends on the relay.
- **The word to host is his, and he has not given it** (DECISIONS 313). The terms of the survey limit how its data is used, and say nothing on handing it on. No pack goes on a public address before that word.
- **A publisher's own key waits** (DECISIONS 313, 318). First a design says how a reader knows the publisher when a key is lost. A second person holds the recovery words, and the steps are written down.
- **No package of his goes on a public host** before a key file can have a passphrase (DECISIONS 328).
- **No publisher is special, and the documents name none** (DECISIONS 318).

## What goes on the host

The two packs of the survey, each under its SHA-256.

**What never goes there.**

- A pack of made-up people, also in a trial. On a host it can mislead.
- A sealed package of a person, or a record of a person.
- A key.

The host can hold the first line by a rule: it takes only the two files, by their SHA-256. The others are the publisher's care.

## The rules of the host

The first three are ROADMAP's. The author proposes the others.

- The file host serves a file under its SHA-256.
- The logs hold no network address of a person, and are kept for a week at most. That is the publisher's word: nobody can check it.
- A pointer is a hint for finding a pack. It shows nothing of the publisher, and puts no pack on record.
- **Proposed:** the host takes an upload only from the publisher's upload key, and only a file with one of the two SHA-256. Then uploads are closed. A new pack opens them again, by the owner's word.
- **Proposed:** the relay takes only events of kind 1063, and only from the pointer key. So no other person's event is kept there.
- **Proposed:** the address is served with TLS.
- **Proposed:** a program that cannot hold these rules is not used. No rule is worked around.

A second publisher copies what it wants of this, and asks nobody.

## The keys

No key is on the server. A person signs on another machine.

- **The upload key** is made for the two uploads, and is deleted after them.
- **The pointer key** is made for the relay. It is no key of a log, and it is not the publisher's identity. It says one thing: the same key signed the earlier pointers.
- **A pointer key that leaks cannot be called back.** Whoever has it signs pointers that look the same. The publisher drops the key, and says so where the packs are announced.
- **Where the pointer key is kept, and by whom,** is the owner's to decide.

## The stages

Each stage starts on the owner's word at that time. An assistant runs no command that listens on a network, signs, or makes a key for this plan without that word.

1. **Choose the programs, and set them up closed.** One file host, one relay, and what serves TLS before them. The server cannot be reached from outside. Each rule above is tried, with the two real packs and with keys that are deleted after. The setup is written down as it is made: each program, its version, and its settings.
2. **Build the two packs.** `bean-pack-nhanes -out <new file> <the survey's files>`: six files for the first pack, and those six with two more for the second. REFERENCE.md names them, and [COMPARE.md](COMPARE.md) says where they come from. `bean compare -pack <file>` must print that bean has the pack on record. Its SHA-256 must be the one in REFERENCE.md.
3. **Open the host.** This is a publication, and it cannot be taken back: a file that was fetched stays fetched. It needs the owner's word to host (above). The setup document is in this repository first, with no key and no publisher's name. A person outside the lab fetches each pack, and checks it.
4. **One pointer for each pack.** An event on a relay cannot be taken back either: to delete it is a request, and a copy can stay elsewhere. The content of a pointer says plainly what the pack is. The owner says where the packs are announced, since a reader has no other way to find the address.

**Not stages of this plan.**

- **A publisher's own key.** It waits for what the owner decided before (above).
- **A second place for the digests.** A second person fetches the survey's files from their own connection, and sends receipts ([tools/receipts](../tools/receipts/README.md)). They build each pack, and state its SHA-256. That needs no server. Until then one hand holds the host, the relay and this repository.
- **A second host,** and who keeps a copy of the survey's files for the day that their host is gone. ROADMAP has both under "Phase 2".

No stage needs a change to bean. No other step of ROADMAP waits for this plan. The first run with a peer needs no server: the author recommends that run first, or beside this. The owner decides.

## The setup, as far as it is known

**Known, from the trial run on one machine.**

- **An upload:** `nak blossom --server <host> upload <file>`, with a key in `NOSTR_SECRET_KEY` for that one command. The host answers with a SHA-256, and the uploader compares it with their own.
- **A pointer:** one event of kind 1063. Its tags are `url`, `m` with `application/json`, `x` with the SHA-256, and `size`. Each is there once.
- **With no key, `nak` signs with a default key of the machine.** Each command that signs gets a key by name.

**Not known.**

- The programs, their versions, and whether each can hold the rules above.
- How the logs are held to a week, and to no network address.
- Limits of size and of rate, and who patches the server.

## What each party learns

- **The file host, and what serves TLS before it:** the network address and the time of each fetch, and which pack. So does the network of the lab, and whoever answers for the name of the address.
- **The relay:** the network address of whoever asks, and what they ask for.
- **Anyone:** that the address is the publisher's, and where its server is.
- **A reader can show less.** They take the file from a person who has it, and check its SHA-256. Or they build the pack from the survey's files.

## What a reader may conclude

- The host served them, that one time, bytes with the SHA-256 that the pointer named. A host can serve other bytes to another reader.
- bean says that the pack is on record. That is the list in bean's code, and no word of the host.
- Not who the publisher is. Not that a second person checked the digests. One hand holds the host, the relay and this repository (H4, H5).
- A reader who does not compute the SHA-256 has only bean's check: a pack that is not on record compares only with `-unknown-pack`.

## The owner's to decide

- The word to host the survey's packs, under the survey's terms.
- The network address. It shows where the lab is.
- Which programs run. Who holds the server, who patches it, and who keeps the pointer key.
- Whether the packs are on the host before the first run with a peer.
- Where the packs are announced, and how long the host stays up.
- A second host.

## Known weak points

- **Nothing here is tried against a real program** (H2).
- **One hand holds everything** until a second person states the digests.
- **What is published stays published.** A pack and a pointer cannot be called back.
- **A server in a lab has no promise of being up.** When it is down, or its address changes, each pointer leads nowhere. The packs can still be built from the survey's files.
- **A host on a network is attacked.** It serves two files. Closed uploads and a relay that takes one key keep its surface small. How the server is kept apart from the lab's other machines is not said here.
- **The rules of the logs are a word.** Nobody outside can check them.
