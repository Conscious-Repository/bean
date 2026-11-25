# How bean works, in three pictures

This is for anyone who wants the whole of bean in one view. It shows where your data lives, how you share it with one person, and how you compare it with public data. Each picture has the parts that work today as solid lines. A dashed line is a part that nobody runs yet.

bean is not a diagnosing tool. Nothing in these pictures says whether a value is healthy.

## 1. On your machine

Your log is one folder. bean signs each record with a key that stays on your machine, and names the record by the SHA-256 of its content. No server and no account are part of it.

```mermaid
flowchart LR
    paper["24 words on paper"]
    subgraph machine["Your machine"]
        you["You, at a terminal"]
        ai["Your AI assistant"]
        files["Your files: a lab PDF, an Apple Health export, a CSV"]
        importers["Importers"]
        bean["bean"]
        subgraph log["Your log: one folder"]
            objects["objects: signed records, each named by its SHA-256"]
            blobs["blobs: the files that you kept, encrypted"]
            local["local: your signing key"]
        end
    end
    copy["Another copy of the folder, or a plain export"]
    you -->|"add, import, ls, verify"| bean
    files --> importers
    importers -->|"readings, which you review"| bean
    ai -->|"reads, and writes with a key of its own"| bean
    bean -->|"signs and stores"| objects
    bean --> blobs
    local -->|"signs"| bean
    paper -->|"only to recover or to retire a lost key"| bean
    objects -->|"sync or export"| copy
    blobs -->|"sync or export"| copy
```

- A record is a claim with an author, a time and a source. It is never edited: a correction is a new record.
- The signing key in `local` is not copied by a sync. The 24 words are on paper, and never on a device.
- A record that your assistant writes is signed by the assistant's key, and shows as the assistant's.
- `bean verify` checks every record again. `bean export` writes your records and your stored files as plain files, so that you can leave. It holds no key file, no package and no reference pack ([SECURITY.md](../SECURITY.md)).

## 2. Share with one person

You choose records, and bean packs them into one sealed file. The file goes by any channel. Six words open it, and they go by another channel, such as a phone call. Two people who both run bean can seal to the reader's address instead. Then the reader's key file opens the package, and no words are read out ([SHARE.md](SHARE.md)).

```mermaid
flowchart LR
    subgraph yours["Your machine"]
        log["Your log"]
        share["bean share"]
        accept["bean accept"]
    end
    file["One sealed file: the records that you chose"]
    words["Six words, said in a call"]
    subgraph theirs["The reader: a peer or a doctor"]
        page["The check page, in a browser"]
        check["Or: bean check-bundle"]
        reply["bean reply: signed notes about what they received"]
    end
    log --> share
    share --> file
    share --> words
    file -->|"e-mail, a drive, any channel"| page
    words --> page
    file --> check
    words --> check
    check --> reply
    reply -->|"a sealed reply"| accept
    accept -->|"the notes, under the name that you give the peer"| log
```

- With six words, the reader needs no key and no account. The check runs on their device and sends nothing.
- Sealed to an address, the reader's key file takes the place of the six words. The check page cannot open such a package: the reader needs bean.
- The check shows who signed, as eight identity words that you read out in the same call.
- A check proves who signed a record and that nobody changed it. It does not prove that a record is true.
- A reader who has opened the file holds a copy for good.
- A peer's records are never mixed into your log. Only their signed notes come in, under a name that you choose.
- To see a peer's values next to yours, read their package with `bean check-bundle` and your own with `bean ls`. bean has no view of two people, and none is planned (DECISIONS 342).

## 3. Compare with public data

A reference pack is one file of public data: a table for each measure and each group of people. The pack comes to your machine whole, and the arithmetic is done there. Your reading never leaves, and nobody learns which group you chose.

```mermaid
flowchart LR
    subgraph made["How a pack is made"]
        source["A source: public data about many people, such as a survey of CDC"]
        builder["A builder: a program that anyone can run"]
        pack["A reference pack: one file, named by its SHA-256"]
    end
    subgraph public["Where a pack can live"]
        host["Any host that serves a file by its SHA-256: a Blossom server, a web server, a friend's disk"]
        relay["A Nostr relay: holds a signed pointer, and no health data"]
    end
    subgraph machine["Your machine"]
        reading["Your reading"]
        compare["bean compare"]
        claim["A saved comparison: your signed claim"]
    end
    peer["A peer, with the same pack"]
    source --> builder
    builder --> pack
    pack -->|"today: you build it, or someone hands you the file"| compare
    pack -->|"today: a host serves it, read only"| host
    pack -.->|"later: a publisher signs a pointer: title, SHA-256, address"| relay
    relay -.->|"where to find it"| compare
    host -->|"the whole pack, checked by its SHA-256"| compare
    reading --> compare
    compare -->|"where the value lies in the group, with caveats"| claim
    claim -->|"shared as in picture 2"| peer
    peer -->|"computes it again from the pack, with no bean"| claim
```

- **A pack is what its SHA-256 says.** A host cannot change it without the check failing. So no host needs to be trusted, and any person can host a pack for a friend.
- **A pointer is not the data.** A publisher signs a short note: the title of the pack, its SHA-256 and where it is. A relay holds that note. The note helps you find a pack. It never makes a pack right.
- **What a host learns:** that someone fetched a pack, from which network address and when. It learns no reading and no group, because the pack is fetched whole.
- **Who says that a pack is right.** bean holds the SHA-256 of the packs that it has on record. Two programs that share no code build those bytes from the source's files. For any other pack, bean says that nobody has checked it.
- **A saved comparison is your claim.** It names the reading, the pack, the group and the method. A peer with the pack can do the arithmetic again. That shows that the numbers hold, not that the reading is true.
- **Today** a person builds a pack from the source's files, gets the file from someone, or fetches it from a host. A public host serves packs, read only ([EXTERNAL-PACKS.md](EXTERNAL-PACKS.md)). Any person can run such a host ([examples/pack-host](../examples/pack-host/README.md)). Pointers on a relay can follow later ([ROADMAP.md](ROADMAP.md)).

### What you can compare with

| Compare with | Today |
|---|---|
| The survey's own files, for a group that you state | Works: `bean compare -sex ... -age ...` ([COMPARE.md](COMPARE.md)). |
| A reference pack that you built, or that someone gave you | Works: `bean compare -pack` ([REFERENCE.md](REFERENCE.md)). |
| A reference pack from a host | Works. You fetch the file with `curl`, and check its SHA-256 before you use it ([FIRST-RUN.md](FIRST-RUN.md#each-compares-with-a-pack)). A public host is up ([EXTERNAL-PACKS.md](EXTERNAL-PACKS.md)). |
| One peer | Works as a swap of packages ([FIRST-RUN.md](FIRST-RUN.md)). You read theirs beside your own. bean computes nothing across the records of two people. |
| A pool of real people | Not built. A path with made-up contributors is a test ([tools/pool](../tools/pool/README.md)). ROADMAP.md, path 4, says what a real pool waits for. |

### What bean checks in a comparison

bean's part is the same, whoever made the pack and whoever serves it. It keeps your reading on your machine, and it leaves a record that another person can check.

| Stage | What bean does | How a person checks it |
|---|---|---|
| Your reading | On an import, keeps the file first, then signs each reading that you confirmed. | `bean verify`, and `shasum -a 256` of your file. |
| The pack | Names it by the SHA-256 of its bytes. It stops at a pack that it does not have on record, unless you pass `-unknown-pack`. | `shasum -a 256` against [REFERENCE.md](REFERENCE.md#how-a-pack-travels), or build the pack again. |
| The match | Compares only a type and a unit that the pack lists, and converts no unit. | `bean compare -pack <file>` lists them. |
| The arithmetic | Runs on your machine. bean has no command that fetches from a network. | The method and its test values are in REFERENCE.md. |
| The result | Writes nothing, unless you pass `-save`. Then it signs the reading's id, the pack's SHA-256, the group, the method, the numbers and the caveats. | `bean show <id>`. |
| A correction | Deletes each saved comparison of a reading that you correct or delete. | `bean ls`. |
| A share | Shows what leaves and asks. Seals it with `-seal` or `-for`. | `bean check-bundle` fails if a record was changed, added or taken out. |
| The numbers of a share | Nothing: the check shows them as the sender's claim. | `tools/compare/reproduce.py` with the same pack computes each one again, with no bean. |

What no check shows:

- **That a reading is true.** A signature shows which key signed. A value that was typed by hand is signed like a lab's result.
- **That you belong to the group.** You chose it.
- **That the lab and the source measured alike.** The caveats of a result say so.
- **That a share is complete.** It is what its sender chose.

A saved comparison gives its reading away. Anyone who has the record and the pack can work the value out. Save and share one as you would the reading itself.

## The words

| Word | Meaning |
|---|---|
| Log | Your folder of signed records. |
| Record | One signed file: a reading, a note, a kept file, or one of bean's own. |
| Bundle | The records that you chose to share, as a folder or one sealed file. |
| Reference pack | One file of public data to compare with, named by its SHA-256. |
| Host | Any place that serves a file. A Blossom server serves a file by its SHA-256. |
| Pointer | A signed note that says where a file is, and what its SHA-256 is. |
| Relay | A server that holds signed notes, such as pointers. It holds no health data. |
| On record | A pack whose SHA-256 is in bean's code, and that two programs build again. |
| Claim | What a record is: a statement by its signer. A signature proves who, and never that it is true. |

[REFERENCE.md](REFERENCE.md) has the format of a pack. [SHARE.md](SHARE.md) has the format of a bundle. [SECURITY.md](../SECURITY.md) lists what is protected, and each key.
