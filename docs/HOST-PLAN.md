# A first host for the survey's packs: a draft plan

This is for anyone who sets up, or reviews, a publisher's first host. It is a draft. No review has judged it, and nothing of it is set up. DECISIONS 337 has the account.

**It changes nothing in bean.** bean fetches nothing, and signs nothing for a relay. A host and a relay are programs that exist. A publisher's network address, terms and keys are its own, and none is in this repository.

**bean does not diagnose.** A pack says where a value lies in a sample, and not whether it is healthy.

## The need

The owner asked for this plan on 2026-11-19, and said where it can run:

```
we have a server in the lab we can host a relay + blossom on
```

[ROADMAP.md](ROADMAP.md) has the rules already, under "A first publisher hosts". This plan puts them in an order.

## What is true today

**H1. The steps ran on one machine.** `examples/pool/run.sh` uploads a file, publishes an event of kind 1063, finds it, and fetches the file. It uses the test server of `nak` 0.22.3. That server keeps everything in memory, and is no host.

**H2. No file host and no relay has run for this project.** No program is chosen for either.

**H3. The two packs are in no repository as files.** A person builds each with `bean-pack-nhanes` from the survey's files. [REFERENCE.md](REFERENCE.md) states the SHA-256 of each, and bean has both on record.

**H4. No second person has stated the digests of the survey's files.** `tools/receipts` tallies such statements, and has had no real one.

**H5. Nothing ties a pack to a publisher.** A record of a publisher's bean identity is not built. A pointer shows a key, and no more.

## What goes on the host

- The two packs of the survey, each under its SHA-256.

**What never goes there.**

- A pack of made-up people. On a real host it can mislead.
- A sealed package of a person. The owner's rule is that a key file with a passphrase comes first (DECISIONS 328).
- A record of a person, or a key.

## The rules of the host

They are ROADMAP's, and three are added. The author proposes the three that are marked.

- The file host serves a file under its SHA-256.
- **Proposed:** it takes an upload only from the publisher's upload key. When the two packs are there, uploads are closed.
- **Proposed:** the relay takes only events of kind 1063, and only from the publisher's pointer key. So no other person's event is kept there.
- **Proposed:** the address is served with TLS, so that a reader fetches by `https` and `wss`.
- The logs hold no network address of a person, and are kept for a week at most. That is the publisher's word: nobody can check it.
- Each key is made for the host or for the relay. No key of a log signs there.
- A pointer is a hint for finding a pack. It shows nothing of the publisher, and puts no pack on record.

## The stages

1. **Choose the programs.** One file host, one relay, and what serves TLS before them. Each is a program that exists, at a version that is written down. Each must be able to hold the rules above. None is chosen (H2).
2. **A rehearsal inside the lab.** The server cannot be reached from outside. The keys are throwaway keys. A second machine in the lab plays the other user: it finds the pointer, fetches, and holds the file to its SHA-256. Nothing leaves the lab's network, so the test vector's files may be used here. Then everything is wiped.
3. **Build the two packs.** On a machine of the publisher, from the survey's files. `bean compare -pack <file>` must say that the pack is on record. Its SHA-256 must be the one in REFERENCE.md.
4. **A rehearsal in public, with a throwaway key.** The real address, with TLS. The two real packs go up: they are public data. A throwaway key signs the pointers. A person outside the lab fetches each pack, and checks it. Then the pointers and the key are deleted.
5. **The setup document.** The section below is filled in: each program with its version, the settings for the rules, and the tags of a pointer. It holds no key, and names no publisher. It is in this repository before the next stage, so that a second publisher can copy it.
6. **The real key, and the announcement.** The owner says whether a real key is used after the rehearsal: that decision is his (ROADMAP). One pointer for each pack.
7. **A second place.** A second person fetches the survey's files from their own connection, and sends receipts ([tools/receipts](../tools/receipts/README.md)). They build each pack, and state its SHA-256. Until then one hand holds the host, the relay and this repository.

No stage needs a change to bean. No other step of ROADMAP waits for this plan.

## The setup, as far as it is known

**Known, from the trial run on one machine.**

- **An upload:** `nak blossom --server <host> upload <file>`, with a key in `NOSTR_SECRET_KEY` for that one command. The host must state the SHA-256 that the uploader computes.
- **A pointer:** one event of kind 1063. Its tags are `url`, `m` with `application/json`, `x` with the SHA-256, and `size`. Each is there once.
- **A reader:** fetches with a tool of their own, and computes the SHA-256. Then `bean compare -pack <file>` says whether the pack is on record.
- **With no key, `nak` signs with a default key of the machine.** Each command that signs gets a key by name.

**Not known.**

- The programs, their versions, and their settings for the rules above.
- How the logs are held to a week, and to no network address.

## What a reader may conclude

- The host served bytes with the SHA-256 that the pointer named.
- bean says that the pack is on record. That is the list in bean's code, and no word of the host.
- Not who the publisher is, and not that a second person checked the digests (H4, H5).

## The owner's to decide

- The network address.
- Which programs run, and who holds the server and its keys.
- Whether a relay runs from the first stage on. Nothing depends on it.
- Whether a real key is used after the rehearsal.
- Whether the packs are on the host before the first run with a peer.
- Where the packs are announced.

## Known weak points

- **No review has judged this plan.** It is a draft.
- **One hand holds everything** until stage 7.
- **A server in a lab has a network address that shows where the lab is.** It has no promise of being up.
- **A host on a network is attacked.** It serves two files. Closed uploads and a relay that takes one key keep its surface small.
- **The programs change.** The setup document names versions, and a later version can differ.
- **The rules of the logs are a word.** Nobody outside can check them.
