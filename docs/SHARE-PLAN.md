# A seal to a reader's key: what was decided before it was built

This is for anyone who builds or reviews the next way to seal a package. [SHARE.md](SHARE.md) is what exists. This document says what is added, in which steps, and what it costs. It was written before any of it was built.

**Step 1 is built** (release 0.8.0, DECISIONS 327). SHARE.md now describes it, and the section "Step 1 as built" below says where the build adds to this plan. Step 2, and what is under "Later", are not built. The rest of this document stands as it was decided: where it says "will", read it as the plan.

It is step 3 of the section "Next" in [ROADMAP.md](ROADMAP.md). Six drafts were written. Reviewers who had not seen the work judged each, one for each paragon. DECISIONS 326 has the account.

**bean is not a diagnosing tool.** Nothing here says whether a value is healthy.

## The need

Two people who both run bean share more than once. Today each package needs a call: the sender reads six words to the reader. The owner asked for a package that needs no words read out. He also wants a package that can lie on a file host, with a link sent.

Six words are 66 bits, and a host keeps a file for good. So a package on a public host waits for a seal to a key (DECISIONS 313).

## What is measured

Each fact was checked in the code, and by one reviewer or more who built bean and ran it with made-up data.

**F1. What release 0.7.2 does with a package that is sealed to a key.** It asks for six words. When six words of the list are typed, it refuses: `it is not sealed with words: this is another kind of age file`. The check page shows the box for words, and then says `Check the six words and type them again`. Nothing is read wrongly.

**F2. No check reads the `seal` member of a share record.** `bean share` and `bean reply` write `"seal": "passphrase"`. The checks in Go and in Python read `records`, `proof`, `files` and `note`, and refuse no other member.

**F3. The `age` format does not show who sealed.** An address is public, and anyone who has it can seal to it.

**F4. A sealed file that was cut short still opens up to the cut.** A reviewer cut one, and its key gave 65,536 bytes of plaintext before the error.

**F5. bean can open what it has just sealed for another person.** The library hands the file's own key to the code that wraps it, through its public interface. A reviewer built that and opened the file with it.

**F6. An address that no key opens can be written by hand.** The library takes an address whose 32 bytes have the top bit set. It seals to it without an error, and the reader's true key does not open the file. A low-order point fails when the library seals.

**F7. `age` 1.2.1 writes one stanza for one reader.** It adds none of its own. bean pins that version.

**F8. `bean share` looks for the output file before the review.** The review can wait for minutes. Then bean writes the file, and then the share record. When the record fails, it removes the file: nobody had the words.

**F9. The six words are also a gate.** A program that runs as the reader cannot open another person's package without them. The person's own records are plain files on the disk.

**F10. The check page decides in its own script whether to ask for words.** It looks at the first line of the file, before it asks the Go code anything.

## Rules that stay

- bean invents no cipher and no encoding. A package is an `age` file, and the `age` tool opens it.
- bean fetches nothing and uploads nothing.
- `bean share -seal` and its six words do not change. No flag that exists changes its meaning.
- A log that an earlier bean wrote reads the same. No record changes its meaning.
- A line that bean prints claims only what bean checked.

## Step 1: seal to an address, and open with a key file

This is one release. A step that seals with no step that opens would leave a reader with a file and no way in.

### The key and the address

- **An address** is a recipient of the `age` format for X25519: the text `age1…`, 62 characters.
- **Its key** is a standard `age` identity file. The person places it. bean keeps no such key and no list of them.
- **`bean address -out <new file>`** makes a key. It writes a new file with mode 0600, and never over a file that exists. It writes the comment line that `age-keygen` writes. It needs no log. When a log is found, it refuses a place inside the log's folder.
- **`bean address -key <file>`** shows the address of a key file that exists, also one that `age-keygen` made. It works the address out from the secret. It prints nothing secret.
- **Both print eight address words.** They are made from the SHA-256 of the address text, as identity words are made from an id. SHARE.md will say how, so that a tool other than bean can make them.
- **`-out` prints the address only after the file is written, synced and read back.**

**What both print, before a person gives the address away.**

- The key is this one file. `bean restore`, `bean export` and a copy of the log do not hold it.
- Without the file you cannot open what was sealed to it. Make a new address, and ask for a new share.
- Whoever has a copy of the file opens every package that was sealed to it. That holds for as long as a copy of the package exists.
- A folder that syncs, and a backup, each hold a copy of the file.
- An address cannot be taken back. If the file is stolen, tell each sender to stop, by another channel.
- A program that runs as you, such as an assistant with a shell, opens such a package with no word typed.
- A sender needs the eight address words from you, and gives them to bean with every share.
- No call tells you which share a package is. The check shows its time and id. Ask the sender, by another way than the package came.
- To open on a second machine you copy the file, and every copy opens everything.
- If you delete the file, tell each sender. Their next package is one that you cannot open.
- For a package on a public host: make an address for that one sender. Delete the key file after you have read and answered.

### Sealing

`bean share -for <address> -out <file>` seals to the address. It implies `-seal`. bean writes one recipient.

- **The address.** bean reads it with the library, and takes only the lower-case text that the library writes back. For any other text it says that this is not an address in lower case, and stops. bean adds no decoder of its own.
- **The gate against a wrong address.** bean does not print the address words here. It asks the person to type the eight address words that the reader gave them. `-address-words` gives them for a script, and `-yes` never stands in for them. With no terminal and no `-address-words`, bean stops. It says that it needs the address words, and that the reader gets them from `bean address -key <file>`. The gate runs before bean writes anything to the log.
- **When the words are not those of the address,** bean prints nothing that is made from the address. It says that nothing was written. It says to run again after a mistyped word, and otherwise to ask the reader again.
- **What the gate is worth.** It catches a wrong or swapped address only when the words came by another channel than the address. An address of F6 has other words than the reader's true address, so the gate stops it.
- **Every line says "address words".** Identity words are another eight.

**The review before the share adds these lines, before it asks.**

- The package is sealed to an address. Whoever holds the key file of that address can open it.
- bean checked only that the address words belong to the address.
- A reader with only a browser, or with an earlier bean, cannot open it. Such a reader needs a package that is sealed with words.
- A program that runs as the reader opens it with no word typed.
- It stays closed only while the key file stays with the reader and the cipher holds. That is so for as long as any copy of the file exists.
- You cannot open this file again, unless the address is your own. To keep a copy, do not delete those records, or keep a `bean export`.
- The record names no address. A `-note` can say who the package is for. The note is signed, and travels to the reader and to whoever gets the bundle later.

**The order of work.**

1. bean seals in memory, and opens the result with the file's own key (F5). It checks what came out as its reader will check it.
2. It makes the output file, new and empty. If that fails, nothing was written.
3. It adds the share record. If that fails, it removes the empty file.
4. It writes the package into the file.

A package that is sealed to a key can be opened the moment it is written. So the record is in the log before the first byte is in the file. A log then never knows less than what left.

**When the file cannot be written whole,** bean says only what it did. It names the path and the id of the share record. It says that a part of such a file can open (F4). When it removed the file, it says so, and names `bean purge <id>` for a place that does not sync. When it could not remove the file, it says to delete it, and does not name `purge`.

**What step 1 of the order does not check.** bean does not open the one stanza that is for the reader. A test with test keys holds that code: the Python opener opens bean's packages with a reader's key. No line says "as its reader will" of this seal.

**The share record** says `"seal": "recipient"`. It names no address, and no hash of one.

**When the share is written,** bean prints the identity words and the start of the share's id, as today. It prints no line about six words or about a browser. It says again that the sender cannot open this file.

### Opening

- **One scan decides what a file is.** After the first line, split on LF. Find the first line that starts `---`, in the first 4 KiB. Refuse a CR before it. Exactly one line starts `->`. It starts `-> scrypt ` or `-> X25519 `. Anything else is refused before any work.
- **A file for two readers is refused,** though the `age` tool opens it. The documents say so.
- **`-key <file>`** on `check-bundle` and on `reply -to` names an identity file with exactly one identity. bean tries that key and no other. A flag comes before the file's name.
- **A key file that is protected with a passphrase is refused** in this step, with a line that says so.
- **A file that is not such a key** gets one fixed line, with nothing of the file in it.

**The lines.**

| Case | What bean says |
|---|---|
| Sealed to a key, and no `-key` | It is sealed to a key, not with words. Open it with `-key <your key file>`. If you no longer have the file, you cannot open it: ask the sender to share again, for a new address. |
| `-key` with a package that is sealed with words | It is sealed with words. `-key` is not used: give the six words. |
| `-key` with a folder or a plain zip file | It is not sealed. `-key` is not used. |
| The key does not open it | It was sealed to another address, or it was changed on the way. bean cannot tell which. `bean address -key <file>` shows the address of a key file. |
| `bean accept` on a file that is sealed to a key | `bean accept` takes a reply, and this bean seals a reply with words. |

**What the check says when it opened with a key.**

- The package was sealed to your address. Anyone who has your address can seal to it, also an older share of the signer. bean cannot tell who sealed it.
- Then the signer's identity words, and the share's time and id, as today.
- Ask the signer, by another way than this package came, whether they sent you a share whose id starts so.

**One check of what the signer signed.** A share that bean sealed to a key says `"seal": "recipient"`. A package can open with a key when its share record does not say so. Then the check says: the signer's record does not say that this share was sealed to a key. Someone sealed it again, or another tool sealed it. `check-bundle` marks that line. The page shows no such line in this step, since it opens no package with a key. A record that says `recipient` shows what the signer wrote, and not for which address.

**No rule asks for a terminal.** A program with a shell passes such a rule, and can run the `age` tool. The documents say what a key file on a disk means (F9).

### In the code

- `bundle.FromPackage` keeps its arguments. One sibling takes an `age` identity. A package that it opened counts as opened.
- The scan runs before the words are looked at. Its answer is one new member of the input and of the result: what the file asks for. It is set whenever the scan ran.
- `check-bundle` and the page print from that one place. So the page's script decides nothing.
- `tools/second/unseal.py` gets a key mode, from the `age` specification, with code of its own. `make bundles` holds the openers to one verdict, for both seals.

### The check page

It gets one change. It asks the Go code what the file asks for, before it shows anything. It shows the box for words only for a package that is sealed with words.

For a package that is sealed to a key it says three things. The package is sealed to a key. This page cannot open it. The reader opens it with bean, or with the `age` tool, or asks for a package that is sealed with words.

`check.mjs`, which CI runs, holds the page's Go code to that answer. The test in a real browser is not part of CI.

### The documents, in the same release

- **SHARE.md.** How to open a package with the `age` tool and no bean. How the address words are made. That a person with a key from `age-keygen` runs `bean address -key` before they give the address away. `"recipient"` in the table of the share record. The order of work for this seal, where it describes a share that fails. And when to choose which seal.
- **When to choose which.** Words: for a reader with no key, or with only a browser, and one call for each package. An address: for two people who run bean. After one exchange, no secret is passed for a package. The question of which share it is stays.
- **What is lost with an address.** The reader holds a key file that can be lost or stolen. The sender cannot open the file again. A program that runs as the reader opens what others sent, with no act of the person. The call is gone in which the sender said which share this is.
- **What that last loss means.** An older share that someone sealed again opens like a new one. Whoever holds any key of the sender, also a retired one, can deliver a share that they signed.
- **SECURITY.md.** A row in "Keys": a reader's key file has no paper copy, and its address cannot be taken back. And the sentence about a program that runs as you.
- **ONBOARDING.md.** The rules for an assistant name `bean address`, `-key`, a key file and address words. A person does those themselves.
- **UPGRADING.md.** What release 0.7.2 and its page print for such a file, word for word (F1). Both people update first. A bean that has `bean address` can open such a file.
- **COMMANDS.md and ROADMAP.md.**

### Step 1 as built

The build follows the plan above. It adds these points, which the plan left open.

- **`-key` with a package that needs no key.** bean says that `-key` is not used, and goes on. It asks for the six words, or checks the folder. It does not read the key file then.
- **An empty address.** `-for ""` is not "no address": bean stops. A script whose variable is empty must not write a plain folder.
- **The key file.** It holds the line `# public key: <address>` and the secret. `age-keygen` also writes a line with the time, and bean writes none.
- **`bean address` says one thing first.** Before its list it says to give the address by one channel, and the address words by another.
- **`bean share -for` says one thing more when the share is written:** a bean that has no `bean address` does not open the file. A script that passes `-yes` sees no review.
- **Three reasons of the scan are told apart.** A header with a carriage return, a header that names no reader or more than one, and a reader of another kind. A reader of another kind beside a reader for a key counts as two readers.
- **An `age` file that the scan refuses** is one more answer of "what the file asks for". The page shows no box for words for it, and says why.
- **An `age` file in armor is refused by name.** It is sealed, so bean does not say "it is not sealed" of it.
- **`bean reply -to` with `-key` says what a key shows,** before it signs a note. The plan gave those lines to the check only.
- **A place inside the log is judged by what is on the disk.** A place that reaches the log's folder through a link is inside it. So is one that is written in other capitals, on a disk that does not tell them apart. That holds for a key file, and now for a bundle and a reply too.
- **No address words at all get a line of their own.** A script whose variable is empty is not told to ask the reader again.
- **`bean address -out` does not print the mode of the file.** It asks for mode 0600, and does not check what the disk gave.
- **The check page hides one line.** For a package that is sealed to a key, the check's line names `-key`. The page says its three things, and not that line.
- **`check.mjs` takes `-asks`.** `make bundles` passes it for every sealed file, and it fails when the page's check answers otherwise.
- **`make pagetest` has the case too,** in a real browser. It is not part of CI.
- **The order of the gate.** It runs after the checks of the output place, and before anything is written to the log.

## Step 2: a reply to an address

It has its own audit. It may be in the same release as step 1.

- `bean reply -to <package> -for <address> -address-words "…"` seals the reply to an address. The replier got it from the sender. The gate is the same.
- Without `-for`, a reply makes six words, as today. A share names no address for a reply, and bean picks none.
- `bean reply` has no review today. With `-for` it says, before it writes: you cannot open this reply again. Your log keeps a record that you replied, and not the notes.
- On a terminal it then asks. A script passes `-yes`, which `bean reply` gets for this. With no terminal and no `-yes` it writes nothing.
- The lines about six words, and about keeping the file and its words, are not printed for this seal.
- The order of work and the lines for a file that is not whole are those of step 1.
- The `reply` record says `"seal": "recipient"`.
- `bean accept -key <file>` opens it. Its checks do not change. With `-yes` and `-words`, a program that runs as the person then takes a peer's notes with no act of the person. The audit of this step judges that.
- A reader replies before they delete a key that was made for one sender. Or they unpack the package first, and reply to the folder.

## What a person does who wants no words read out

- **Once.** The reader runs `bean address -out`. They give the address by one channel, and the eight address words by another. The sender keeps both where nobody else can change them: neither is a secret, but a changed pair passes the gate.
- **Each package.** The sender runs `bean share -for <address> -address-words "…"`. The file goes by any channel, or by link.
- **What stands in the place of the six words.** Eight address words that are not secret, given to bean with each share. A key file that the reader guards. And one question from the reader, by another way than the package came.
- **Until step 2,** a reply still makes six words.

## Later, each with a real case and a design of its own

- **A key file that is protected with a passphrase.** It gives back the gate of F9: a person types before another person's package opens.
- **The check page opens with a key, and makes one.** A page cannot prove itself, and a key opens more than six words do. ROADMAP had this as the first step. It is not the first step now.
- **A member of the share record that names the reader.** A pool needs it, to tell a contribution that a person meant from a share that was passed on.
- **bean remembers an address that a person confirmed.** Then the address words are given once.

## The questions of ROADMAP, answered

| Question | Answer |
|---|---|
| Where is a reader's key kept, and which copies of a log hold it? | In one file that the person places. No copy of a log holds it. |
| How does a sender know that an address is the reader's? | By eight address words from the reader, by a second channel. bean asks for them with every share. |
| How long does a reader's key live? | As long as the person keeps the file. bean sets no time. |
| Does a share record name the address that it was sealed to? | No. It says `"seal": "recipient"`, and no more. |
| How is a reply sealed back? | To an address that the sender gave the replier, in step 2. Until then, with six words. |
| What does an earlier bean say about such a package? | F1. UPGRADING.md will quote it. |
| Does the check page open with a key, and does it make one? | No. It says that it cannot open such a package. |

## What this lifts, and at what cost

Step 1 lifts the hold of DECISIONS 313 on a package by link. The cost is said in the review, and here.

- Such a package is closed until the reader's key file leaks or the cipher breaks. It is not closed for good.
- It is closed to others only when the address reached the sender truly. The second channel for the address words is for that.

## Known weak points

- **The gate of the six words is gone for a reader who uses a key (F9).** The answer in step 1 is lines on a screen and in three documents. The real answer is a protected key file, which is later.
- **Eight address words with every share** are friction. They prove nothing when the address and the words came by one channel.
- **No forward secrecy.** A stolen key file opens everything that was sealed to it and still exists anywhere.
- **A share record can exist for a file that never left.** A failed write leaves one, and the person purges it by hand.
- **Nothing binds a share to its reader.** A package that someone sealed again for the reader reads like any other, but for the one check above. That check looks one way only: it does not mark a package that opened with words and whose record says `recipient`.
- **"Exactly one stanza" rests on the pin of F7.** A later `age` that adds a stanza of its own would make bean refuse its own packages.
- **X25519 on a host that keeps files for good** is open to whoever breaks that curve later.
- **Two kinds of seal** touch about twenty files of code, tests, scripts and documents.
- **The code that keeps the file's own key for the round trip** is new code that handles a key, small as it is.
