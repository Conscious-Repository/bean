# A first host for the survey's packs: a plan on paper

This is for anyone who sets up, or reviews, a publisher's first host. Nothing of it is set up. DECISIONS 337 has the account of its reviews.

**It changes nothing in bean.** bean fetches nothing, and signs nothing. A host and a relay are programs that exist. A publisher's network address, terms and keys are its own, and none is in this repository.

**A host is a convenience, and never a gate.** A file and its SHA-256 on a page is a full publication ([REFERENCE.md](REFERENCE.md)). Any web server does that, and it is the cheapest way. A file host of the Blossom kind and a relay are additions. The owner asked for both. A person can also build each pack from the survey's files, with no host at all. That way stays.

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

## What was decided before

- **A relay is in the first setup.** The reviews said that a file host is enough to begin. The owner wants the relay and its pointers tried from the start (DECISIONS 318). Nothing depends on the relay.
- **The word to host is the owner's, and he has not given it** (DECISIONS 313). The terms of the survey limit how its data is used, and say nothing on handing it on. No pack goes on a public address before that word.
- **A publisher's own key waits.** First a design says how a reader knows the publisher when a key is lost. That is a rule from the reviews of DECISIONS 313.
- **The owner named a second person** who holds a sealed copy of the recovery words of the publisher's identity (DECISIONS 318). ROADMAP also asks for a written list of steps.
- **A rehearsal with a throwaway identity comes before any real key.** That is ROADMAP's rule. Whether a real key is used after it is the owner's to decide.
- **No package of the owner's goes on a public host** before a key file can have a passphrase (DECISIONS 328).
- **No publisher is special, and the documents name none** (DECISIONS 318).

**This plan is that rehearsal.** Each key of it is thrown away. It is a publication all the same: the packs are the real ones, and nothing of it can be taken back.

## What goes on the host

The two packs of the survey, each under its SHA-256.

**What never goes there.**

- A pack of made-up people, also in a trial. On a host it can mislead.
- A sealed package of a person, or a record of a person.
- A key that signs.

The rules below keep other files off the file host. The relay is different: the content of a pointer is free text. The person who signs a pointer answers for its words.

## The rules of the host

The first three are ROADMAP's. The author proposes the others.

- The file host serves a file under its SHA-256.
- The logs hold no network address of a person, and are kept for a week at most. That is the publisher's word: nobody can check it.
- A pointer is a hint for finding a pack. It shows nothing of the publisher, and puts no pack on record.
- **Proposed:** the file host takes the two packs, and no other file. Then uploads are closed. A new pack opens them again, by the owner's word.
- **Proposed:** the relay takes two events of kind 1063, from the pointer key. Then it takes no event. So no other person's event is kept there.
- **Proposed:** the address is served with TLS.
- **Proposed:** when no program can hold a rule, the owner decides. He takes another program, or the rule changes in this document first. No rule is worked around.

How a program holds the fourth rule is not known: by a list of SHA-256, or by one upload key. The trial of stage 2 finds it. Each of these rules is a setting of the publisher. A reader cannot check one of them.

A second publisher copies what it wants of this, and asks nobody.

## The keys

No key that signs is on the server. A person signs on another machine. The key of the TLS certificate is on the server, as it must be.

- **The upload key** is made at stage 3 for the two uploads, and is deleted after them.
- **The pointer key** is made at stage 4 for the two pointers, and is deleted after them. It is the throwaway identity of ROADMAP's rule.
- **The trial of stage 2 has keys of its own.** They are deleted after it.
- **A deleted key cannot leak, and cannot sign again.** A new pack gets new keys, by the owner's word.
- **A pointer under such a key is a test of the way, and no more** (DECISIONS 313).

## The stages

The author proposes this gate. Each stage starts on the owner's word at that time. The owner, or a person whom he names, does each act on the server.

Without that word, an assistant does none of these for this plan:

- install a program on the server, or run a command that listens on a network;
- open a port, or change a firewall or a name;
- ask for a certificate;
- upload a file, sign, or make a key.

1. **Build the two packs.** This needs no server. A person fetches the survey's files first: about 21 MB, and [COMPARE.md](COMPARE.md) says from where. Then `bean-pack-nhanes -out <new file> <the survey's files>`: six files for the first pack, and those six with two more for the second. REFERENCE.md names them. `bean compare -pack <file>` must print that bean has the pack on record. Its SHA-256 must be the one in REFERENCE.md.
2. **Choose the programs, and try them closed.** One file host, one relay, and what serves TLS before them. The programs listen only inside the lab's network. The server has no public name, and no certificate from a public authority. A person tries from outside the lab, and nothing must answer. Each rule above is tried with files of random bytes, and no pack goes on the server yet. The setup is written down as it is made: each program, its version, and its settings. Then the trial's files, events and keys are deleted.
3. **Open the host.** This is a publication, and it cannot be taken back. It needs the owner's word to host (above). The setup document is in this repository first, with no key and no publisher's name. Then the server gets its name, its certificate and its open port. The public half of the new upload key goes into the setup. That is the one change after the trial, and the setup document says so. The two packs go up, uploads are closed, and the key is deleted. A person outside the lab fetches each pack, and checks its SHA-256.
4. **One pointer for each pack.** An event on a relay cannot be taken back either: to delete it is a request, and a copy can stay elsewhere. The content of a pointer says plainly what the pack is, and the owner reads it before it is signed. After the two events the relay takes no event, and the key is deleted. To announce the packs is an act of its own, and waits for its own word. The owner says where: a reader has no other way to find the address.

**What cannot be taken back, by stage.**

- **Stage 3:** a name is public when it is set. A certificate from a public authority puts the name in public logs for good. A file that was fetched stays fetched.
- **Stage 4:** an event, and an announcement.

**Not stages of this plan.**

- **A publisher's own key.** It waits for the design that is named above.
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

- **The file host, and what serves TLS before it:** the network address and the time of each fetch, and which of the two packs.
- **The network of the lab:** the address and the time of each fetch. The size of an answer can show which pack.
- **The relay:** the network address of whoever asks, and what they ask for.
- **Whoever answers for the name:** that a reader asked for it.
- **The public logs of certificates:** the name, for good.
- **Anyone:** that the address is the publisher's, and where its server is.
- **A reader can show less.** They take the file from a person who has it, and check its SHA-256. Or they build the pack from the survey's files.

## What a reader may conclude

- The host served them, that one time, bytes with the SHA-256 that the pointer named. A host can serve other bytes to another reader.
- bean computes the SHA-256 of a pack, and says whether the pack is on record. That is the list in bean's code, and no word of the host.
- **The key of a pointer says nothing.** Only the announcement ties a pointer to a publisher, and no key signs the announcement. A reader can guess the publisher from the address.
- **Anyone can publish a pointer** to any file, under any key, on any relay. Nothing in a pointer tells such a one apart. bean's list does: a pack that is not on record compares only with `-unknown-pack`.
- Not that a second person checked the digests. One hand holds the host, the relay and this repository (H4).

## The owner's to decide

- The word to host the survey's packs, under the survey's terms.
- The network address. It shows where the lab is.
- Which programs run. Who does each act on the server, who patches it, and who makes each key.
- Who the person outside the lab is, who fetches each pack at stage 3.
- How the server is kept apart from the lab's other machines. The limits of size and of rate.
- Whether the packs are on the host before the first run with a peer.
- Where the packs are announced, and how long the host stays up.
- A second host.

## Known weak points

- **Nothing here is tried against a real program** (H2).
- **One hand holds everything** until a second person states the digests.
- **What is published stays published.** A pack, a name and a pointer cannot be called back.
- **A server in a lab has no promise of being up.** When it is down, or its address changes, each pointer leads nowhere. The packs can still be built from the survey's files.
- **The survey's files are on the survey's own host.** Nobody here keeps a copy for the day that it is gone (ROADMAP, "Phase 2").
- **A host on a network is attacked.** It serves two files. Closed uploads and a closed relay keep its surface small. How the server is kept apart from the lab's other machines is not said here.
- **Whoever holds the server can open uploads again.** Closed is a setting, and no proof.
- **The rules of the logs are a word.** Nobody outside can check them.
