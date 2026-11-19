# Share records with someone

`bean share` writes records you choose into a folder: a bundle. You send the folder any way you like. The person who gets it can check that every record is yours and unchanged: with `bean check-bundle`, with a Python script, or in a browser. They need no account, no key of yours and no copy of your log.

[ARCHITECTURE.md](ARCHITECTURE.md) shows the path of a share in one picture.

```
bean share -out for-ana 3fa9c1 7b8b81              # these records
bean share -out lipids -type 'comparison.*'        # every saved comparison, each with its reading
bean share -out with-files -files 7b8b81           # also the files those records keep
bean share -seal -out for-ana.bean 3fa9c1          # one file that only its reader can open, with six words
bean share -for age1... -out for-ana.bean 3fa9c1   # one file that is sealed to the reader's address
```

## Seal it for one reader

A folder is plain: whoever carries it can read it. With `-seal`, bean writes one file that is encrypted, and prints six words that open it.

```
$ bean share -seal -out for-ana.bean -note "my lipids" 3fa9c1
shared 1 records and 0 files in for-ana.bean
share record sha256:41ae...
The package is sealed. It opens with these six words:
  moral arrive identify have dress oblige
They are a secret between you and the reader. ...

Your identity words, which are not a secret:
  injury glow drink patch dynamic submit plate robust
The check also shows this share's time and the start of its id, 41ae07c2d9e1: ...
```

- **Send the file one way and the words another.** Send the file by e-mail or any other way. Give the words by a phone call, in person, or in a message on another service. Whoever has only the file can read nothing in it: no record, no type, no note.
- **Read your identity words to the reader, in the same call.** They are eight words made from your identity id, and `bean whoami` shows them. The reader's check shows the identity words of whoever signed. If the two are the same, the records are from your identity.
- **What eight words are worth.** They are the first 88 bits of the id. An id is a hash, and an attacker can search for one without making a key. A second identity with your eight words costs about 2^88 hashes: out of reach today. Against many people at once it costs less, and it is still far out of reach.
- **The opening words do not show who sealed it.** They keep the package closed on its way. Anyone who knows them can seal another bundle of yours under them, such as an older share. The signatures and your identity words show that the records are yours.
- **Tell the reader which share it is.** The check shows the time of the share and the start of its id, and `bean share` prints both. A person can make many shares.
- **The reader needs no key and no account.** They open the file in a browser with `bean-check.html`, which asks for the words, or with `bean check-bundle for-ana.bean`. Both open it on the reader's device and then check it as they check a folder.
- **bean makes the words and does not keep them.** Six words of a list of 2048 are 66 bits that no person chose. If the words are lost, share again: a new package gets new words.
- **A changed package does not open.** Nor does a package that was cut short. The three openers read the format strictly, and `make bundles` holds them to one verdict.
- **Nothing of it is written in the clear.** bean builds, checks and encrypts the package in memory, then writes the one file. No folder of plain records is made beside it, at any moment.
- **The reader can get a carried file out.** The page lists each file that the package carries, with a button to save it. `bean check-bundle -unpack <new folder> for-ana.bean` writes the bundle's folder, with its files. What is saved is what was checked.
- **What its size shows.** A package grows by about 1 kB a record, more for a comparison, and by the size of each file. Whoever sees the file can estimate how much it holds, and can tell that bean sealed it.
- **The format is not bean's own.** A package is the bundle's folder as a zip file, encrypted in the age format (age-encryption.org/v1) with the words as its passphrase. `age -d` opens one. So does `tools/second/unseal.py`, which reads the format from its specification and shares no code with bean. bean seals at the scrypt work factor 16. It opens a package of that factor or a lower one, and refuses a higher one before it does any work.
- **What it does not do.** A reader who has opened a package holds a copy for good. Whoever learns both the file and the words can open it. The words protect the package on its way, not after.

Your log's share record says `"seal": "passphrase"`. It does not hold the words.

## Seal it to a reader's address

Two people who both run bean can share with no words read out. The reader makes a key file once, and gives its address to the sender. The sender seals each package to that address, and the reader's key file opens it.

```
# the reader, once
$ bean address -out ~/keys/bean-reader.key
wrote the key file /Users/ana/keys/bean-reader.key
address age1c66cfc86n0mk5kf3lsq8twsqlfyvahd3wvf50wwdzulm66y6pvqsu7l2az
address words table nothing proud slender scare away clay cherry
...

# the sender, for each package
$ bean share -for age1c66cfc86n0mk5kf3lsq8twsqlfyvahd3wvf50wwdzulm66y6pvqsu7l2az \
    -address-words "table nothing proud slender scare away clay cherry" -out for-ana.bean 3fa9c1

# the reader
$ bean check-bundle -key ~/keys/bean-reader.key for-ana.bean
```

**The key file and the address.**

- **An address** is a recipient of the age format for X25519: the text `age1…`, 62 characters. It is not a secret.
- **Its key** is a standard age identity file. You place it. bean keeps no such key and no list of them, and no copy of a log holds one.
- **`bean address -out <new file>`** makes a key. bean asks for mode 0600, and writes over no file. It makes no folder: the folder must be there. It holds the line `# public key: <address>`, as `age-keygen` writes it, and the secret.
- **It needs no log.** When it finds a log, it refuses a place inside the log's folder, also through a link. A copy of the log would carry the key.
- **It prints the address only after the file is written, synced and read back.**
- **`bean address -key <file>`** shows the address of a key file. It works the address out from the secret, and prints nothing secret.
- **A key from `age-keygen` works.** Run `bean address -key` on it before you give the address away. It prints the address words, and what you must know.
- **The eight address words** are made from the address as identity words are made from an id. Take the SHA-256 of the address text, in ASCII, with no line end. Its first 88 bits are eight numbers of 11 bits, and each is a word of the BIP-39 English list. So a tool other than bean can make them. The address in the example has the words in the example.

**What `bean address` says each time, before you give the address away.** First: give the address by one channel, and the address words by another, such as a call. Then:

- The key is this one file. `bean restore`, `bean export` and a copy of your log do not hold it.
- Without the file you cannot open what was sealed to it. Then make a new address, and ask for a new share.
- Whoever has a copy of the file opens every package that was sealed to it. That holds for as long as a copy of the package exists.
- A folder that syncs, and a backup, each hold a copy of the file.
- An address cannot be taken back. If the file is stolen, tell each sender to stop, by another channel.
- A program that runs as you, such as an assistant with a shell, opens such a package with no word typed.
- A sender needs the eight address words from you, and gives them to bean with every share.
- No call tells you which share a package is. The check shows its time and id. Ask the sender, by another way than the package came.
- To open on a second machine you copy the file, and every copy opens everything.
- If you delete the file, tell each sender. Their next package is one that you cannot open.
- For a package on a public host: make an address for that one sender. Delete the key file after you have read and answered.

**The sender's side.**

- **`-for <address>` seals to the address.** It implies `-seal`, and bean writes one recipient. The file is an age file, and the `age` tool opens it with the key file.
- **The address has one spelling.** bean reads it with the age library, and takes only the lower-case text that the library writes back. For any other text it stops. An empty address is not "no address": bean stops.
- **bean asks for the eight address words that the reader gave you.** It does not print them here. `-address-words` gives them for a script, and `-yes` never stands in for them. Without a terminal and without `-address-words`, bean stops.
- **What the words catch.** A wrong or swapped address, when the words came by another channel than the address. Words that came with the address prove nothing.
- **Keep the address and its words where nobody else can change them.** Neither is a secret, but a changed pair passes.
- **When the words are not those of the address,** nothing is written, and bean shows nothing that is made from the address. Run the command again after a mistyped word. If not, ask the reader again.
- **The review says what this seal means, before it asks:**
  - The package is sealed to an address. Whoever holds the key file of that address can open it.
  - bean checked only that the address words belong to the address.
  - A reader with only a browser, or with an earlier bean, cannot open it. Such a reader needs a package that is sealed with words.
  - A program that runs as the reader opens it with no word typed.
  - It stays closed only while the key file stays with the reader and the cipher holds. That is so for as long as any copy of the file exists.
  - You cannot open this file again, unless the address is your own. To keep a copy, do not delete those records, or keep a `bean export`.
  - Your share record names no address. A `-note` can say who the package is for. The note is signed, and travels to the reader and to whoever gets the bundle later.
- **What bean prints when the share is written.** Your identity words and the start of the share's id, as for any share. It prints no six words. It says again that you cannot open this file, and that an earlier bean does not open it.
- **bean opens what it sealed once, with the file's own key,** and checks what came out. It does not open the part that is for the reader: only the reader's key does. `make bundles` opens bean's packages with a reader's key, with bean and with Python.

**The reader's side.**

- **`-key <file>`** on `check-bundle` and on `reply -to` names your key file. The file holds exactly one identity, and bean tries that key and no other. The flag comes before the file's name.
- **What a key shows.** The check says: the package was sealed to your address. Anyone who has your address can seal to it, also an older share of the signer. bean cannot tell who sealed it.
- **Then the signer's identity words, and the share's time and id,** as for any package. The identity words show who signed the records.
- **Ask the signer whether they sent you a share whose id starts so.** Ask by another way than the package came: whoever delivered a false package would answer on that way.
- **`bean reply -to` says the same,** before it signs a note: what a key shows, and what to ask the signer.
- **A share that someone sealed again.** A share that bean sealed to an address says `"seal": "recipient"` in its share record. A package can open with a key when its record does not say so. Then the check says that the signer's record does NOT say so: someone sealed it again, or another tool sealed it.
- **That check looks one way only.** It does not mark a package that opened with words and whose record says `recipient`. A record that says `recipient` shows what the signer wrote, and not for which address.
- **A file for two readers is refused,** though the `age` tool opens it. A package names one reader. An `age` file in armor is refused too: the `age` tool opens it.
- **A key file that is protected with a passphrase is refused.** This bean does not open one.
- **A key file that other users can read gets a warning.** bean reads the mode of the file, and says how to mend it: `chmod 600`. It still uses the key. That holds for `bean address -key` too.
- **A file that is not such a key gets one fixed line,** with nothing of the file in it.
- **Without bean.** `age -d -i <key file> for-ana.bean > bundle.zip` opens a package, and `bean check-bundle bundle.zip` or `checkbundle.py` then checks it. `tools/second/unseal.py -key <key file> for-ana.bean <new folder>` opens one too, and shares no code with bean. On these ways nothing says what a key shows: anyone who has your address can seal to it.
- **The check page does not open such a package.** It says that the file is sealed to a key, and that it cannot open it. A forged page that took a key would open every package of that key.
- **An earlier bean does not open it.** [UPGRADING.md](../UPGRADING.md) says what release 0.7.2 prints. Both people update first.
- **A reply can go to an address too.** `bean reply -for <address>` seals it to an address that the sender gave you ([below](#a-reply-to-an-address)). Without `-for`, a reply makes six words, also when the share came sealed to your address.

**What bean says when a package and a key do not go together.**

| Case | What bean says |
|---|---|
| Sealed to a key, and no `-key` | It is sealed to a key, not with words. Open it with `-key <your key file>`. If you no longer have the file, you cannot open it: ask the sender to share again, for a new address. |
| `-key` with a package that is sealed with words | It is sealed with words. `-key` is not used: give the six words. Then bean asks for them. |
| `-key` with a folder or a plain zip file | It is not sealed. `-key` is not used. Then bean checks it. |
| The key does not open it | It was sealed to another address, or it was changed on the way. bean cannot tell which. `bean address -key <file>` shows the address of a key file. |
| `bean accept` on a file that is sealed to a key, and no `-key` | It is sealed to a key. Open it with `bean accept -key <your key file>`. |

**When to choose which seal.**

- **Words:** for a reader with no key, or with only a browser. It takes one call for each package.
- **An address:** for two people who run bean. After one exchange, no secret is passed for a package. The question of which share it is stays.

**What is lost with an address.**

- **The reader holds a key file that can be lost or stolen.** A stolen key file opens everything that was sealed to it and still exists anywhere.
- **The sender cannot open the file again.**
- **A program that runs as the reader opens what others sent, with no act of the person.** The six words were also a gate: such a program does not have them. bean says so, and does not prevent it.
- **The call is gone in which the sender said which share this is.** An older share that someone sealed again opens like a new one. Whoever holds any key of the sender, also a retired one, can deliver a share that they signed.
- **It is not closed for good.** Such a package is closed until the reader's key file leaks or the cipher breaks. On a host that keeps files for good, that matters.

## What goes in

- **The records you name**, by id or by `-type` (with `-since` and `-until`). Only current records about you: not a corrected one and not a deleted one. A record that another identity signed does not go either, since a bundle carries only your identity records. A type picks your own records and your agents' only. A record that another key signed, such as a peer's note, goes when you name it by its id.
- **What they are about, one step.** A record is read with the record it is about, so that one comes along. Examples are the reading of a comparison, the meal of an estimate, and the entry a note comments on. A comparison also brings the record of each reference file it used, without the file. What that one is about in turn stays home unless you name it. The review marks each record that comes along, and says which it could not bring and why.
- **Nothing else of yours.** Not the rest of the log, not a lab report unless you name it. A reading names its report by a hash, and the report stays home.
- **A file only when you name its record.** A record that keeps a file (a lab report, a photo) is shared without the file. With `-files`, the file of a record you named by its id goes in whole, as plaintext. For a lab report that is the report as you imported it, with your name and birth date. A file never goes because its record came along with another, or was picked by type. The one exception is a public reference file that bean has on record, such as an NHANES file as CDC serves it. With `-files` it goes with the comparison that used it. bean knows such a file by the SHA-256 that its record states, and holds the bytes to that SHA-256 before the file leaves. The type of a record is never enough. A kept file that is not on record goes only when you name its record.
- **A type never picks a record that keeps a file.** `-type 'lab.*'` shares your lab readings, not the lab report. Name such a record by its id when you mean to share it.
- **A read-me.** `README.txt` tells someone who has only the folder what it is and how to check it. It holds no value, and a check does not read it.
- **Your identity records**: the ones that show which keys speak for you, and the pairing of an agent whose record you share.
- **One share record**, signed by you: the list of exactly what the bundle holds, your `-note`, and the time.

bean shows the list and asks before it writes: each record, the cohort of a comparison, each file, and how many identity records go along. `-yes` skips that, for scripts. Without a terminal and without `-yes` it writes nothing. That stops an assistant from sharing through a tool that has no terminal. It does not stop a program that has your shell and passes `-yes`: what runs as you can do what you can.

The bundle is written whole or not at all: it is built beside its place, checked as its reader will check it, and moved there. Only then does the share record go into your log, as type `share`: your own record of what you disclosed and when. `bean ls -type share` lists them. A share that fails leaves no bundle and no record. That holds for a folder and for a package with six words. Ctrl-C before the bundle is in place removes what was built. From that moment the share runs to its end, so that no bundle exists without its record. Only a kill that cannot be caught (`kill -9`) can leave one without the other. For a plain bundle, such a kill can also leave the half-built folder, named `.bean-share-` and a number, beside the place you named: remove it. For a sealed package it can leave a package that nobody can open.

**A package that is sealed to an address is written in another order.** Such a package can be opened the moment it is written: the reader's key exists already. So the share record is in your log before the first byte is in the file.

1. bean seals in memory, and opens the result with the file's own key. It checks what came out.
2. It makes the output file, new and empty. If that fails, nothing was written.
3. It adds the share record. If that fails, it removes the empty file.
4. It writes the package into the file.

A log then never knows less than what left. It can know more: a share record can exist for a file that never left.

- **When the file cannot be written whole,** bean says only what it did. It names the path and the id of the share record. A part of such a file can open with the reader's key.
- **When bean removed the file,** it says so. If the place does not sync, no part of the file left, and `bean purge <id>` deletes the record.
- **When bean could not remove the file,** it says to delete it.
- **A kill that cannot be caught** can leave an empty file, or a part of the package, with its share record.

## Reading a bundle someone sent you

```
bean check-bundle for-ana          # a folder, or a zip file of it
bean check-bundle for-ana.bean     # a sealed package: it asks for the six words
bean check-bundle -key ~/keys/bean-reader.key for-ana.bean   # a package that is sealed to your address
```

One command checks the bundle and, when it passes, shows what it holds. Each record has its time as the sender recorded it, its type and its value. A record that the sender did not sign shows who signed it. Each comparison is in words, such as `percentile 46 of lab.cholesterol-total 187 mg/dL, among male, age 30-39 (n 339)`, with its caveats below. A bundle that fails lists no record and shows no note. The check still prints the share record's id, its signer and the counts. You need no log of your own for this, and the check changes nothing in the folder.

A comparison is shown as its claim states it. The check proves who signed the claim, not its arithmetic: `tools/compare/reproduce.py` computes it again ([COMPARE.md](COMPARE.md)).

`python3 tools/second/checkbundle.py for-ana` runs the same five rules without bean. Its listing is plainer: each record's time, type and raw value.

### In a browser, with no install

A reader with no terminal opens one page, `bean-check.html`, and chooses the file they received, or a folder. The page gives one of five verdicts:

- **The records passed.** The page says what the check shows and what it does not. It shows the signer's identity words, the time and id of the share, your note, each record, and what no bundle can show. The box is green. It is amber, and counts the rows to read, when the check marks a comparison or when a record is not signed by the person. A comparison's numbers are labelled "as claimed, not computed here". A text file that the package carries can be read in the page, and any file can be saved.
- **The records failed.** The page shows what failed and nothing else: no record, no note, and not who signed.
- **The file did not open.** The words were wrong, or the file was changed. The form stays, to try again.
- **The file is sealed to a key.** The page cannot open it. The reader opens it with bean or with the `age` tool, or asks for a file that is sealed with six words.
- **Nothing was checked.** The wrong folder, or a file that is not a bundle.

The page asks its check what a file asks for, before it shows anything. Its script does not look into the file. It shows the box for words only for a package that is sealed with words. `tools/page/check.mjs` holds the page's check to that answer, and `make pagetest` tries the page's script in a real browser.

No earlier verdict stays on the page. The technical lines of `bean check-bundle` are under "Details for a technical reader". A printed page names the file, the time of the check and the page's commit.

- The page is one file. It holds the same check as `bean check-bundle`, built for a browser, and loads nothing else. It works from a disk, with no connection.
- It sends nothing: its script makes no request, and its content policy blocks every load. What a reader opens in it has nowhere to go. A policy cannot stop a page from leaving for another address; this page's script never does, and holds no link.
- `make page` builds it. The same commit and the same Go give the same bytes, and the build prints their SHA-256. The page names its commit and its Go, so anyone can build that page again. It is not in the repository: it is 3.7 MB of built program.
- Send the folder as a zip file; the page reads it packed. What the page is given must be the bundle's own folder, or one folder that is the bundle. Anything beside it is refused, and anything else inside it is named as not checked.
- A zip file can read differently in different tools. So a pass covers what the page read. To open a file that a bundle carries, the reader unpacks the zip file and checks the unpacked folder; the page says so.
- A page that came with the records is only as good as whoever sent both. A forged page can call anything good, and can show any identity words. The reader gets the page from another place than the records ([below](#which-page-to-trust)). The page says so itself.
- What only a disk can hold, a link or a pipe in the folder, a browser does not hand to a page. `check-bundle` judges those; the page judges the bytes it gets.
- A bundle whose records hold more than 32 MiB together is refused by the page: a browser has little memory. `check-bundle` has no such limit.
- It was tried in Chromium, opened from a disk, on a wide screen and a phone's (`make pagetest`). Firefox and Safari have what it needs and were not tried.

### Which page to trust

A page cannot prove itself: a forged page prints whatever its maker likes, and a page cannot compute the SHA-256 of its own file. So the trust comes from where the reader got the page.

- **A reader with no terminal** gets the page from the bean project, not from the sender. The sender tells them where, in the same call that gives the words. The place is the project's [releases](https://github.com/Conscious-Repository/bean/releases): each has the page and its SHA-256. A reader who takes the page from the sender trusts the sender for the page as well.
- **What to tell a first reader today.** "Get the page from me by another way than the file, or build it yourself. It came from me, so its verdict is not independent of me. The eight words that I read to you show that the records are mine, on a page that you trust." A page that travels with the file can be replaced with it, and can show any words.
- **A reader with a terminal** builds the page (`make page`) at the commit that a copy names, and compares the SHA-256 of the two files. Or they do not use the page: `bean check-bundle` and `tools/second/checkbundle.py` are the same check.
- **What a second source adds.** One address can serve one reader a changed page. A SHA-256 that is published in a second place, or a build of one's own, is what catches that.

Read a bundle with `check-bundle`, not by pointing `-dir` at it: a bundle is someone else's records, not your log. If you did, no harm is done: bean leaves a `local` folder there, which the check names and does not read.

A bundle does not go into your own log. `bean sync` refuses it: sync is for copies of your own log, and it would mix the sender's records into yours for good. To set someone's numbers next to yours, read their bundle with `check-bundle` and your own with `bean ls` or `bean compare`. There is no merged view yet.

## What the reader can and cannot tell

They can tell that:

- each record is byte for byte what a key signed, and that key spoke for your identity by the identity records in the bundle;
- the bundle holds exactly the records you listed, and exactly the identity and pairing records you named, no more and no fewer;
- each file is the one you listed, and the one its own record states, where the record states its SHA-256;
- which records you signed, which an agent you paired signed, and which another key signed.

They cannot tell, and bean says so on every check:

- **That it is complete.** A bundle is the records you chose. A correction or a deletion you left out, or made later, does not show. "All my readings this year" is your word, not something the bundle proves.
- **That a value is true.** A signature shows who signed.
- **Who you are.** An identity is a key, not a name. The check shows the identity as eight words. Your reader knows that it is you only when you read them the same words, by a way that an attacker does not control.
- **That the key is still yours.** A bundle holds the key changes made before it was signed, not later ones. Someone who stole a key you have since retired can sign a bundle with it, leave the retirement out, and it passes. The check prints the key that signed. When it matters, the reader asks you for your current key (`bean whoami`) by another way, and compares.

And you should know:

- **What travels beside the values.** Each record carries the time when it was entered, your key and the version of bean. Your identity records show when you made your log, when you changed a key, and how many records you had at each change. A comparison carries its cohort: a sex and an age range. A record also carries whatever its source wrote: in its `context`, its `value`, its `source`, its times and any field of its own. For a lab reading: the test's printed name, the panel, the lab, the report's date, its reference interval and the report's SHA-256. It also holds the lab's comments, word for word, and a comment can hold a name. The SHA-256 of a report links each share that carries a reading from it, and confirms the report to anyone who has a copy.
- **What the review lists.** Every field of every shared record and pairing, as its JSON holds it, with each different value in full.
  - Left out are the fields that bean makes and checks itself: `v`, `id`, `sig`, `kid`, `author`, `subject`, `nonce`, `type`, `times.imported` and `source.ref`. A `value.n` that is a number is in the row.
  - No record and no field is left out by its type or its name.
  - A long value names the records that carry it, when they are five or fewer.
  - A field of numbers or times with more than 50 different values shows their count, not the values. An export gives each reading its own index and time, and neither can hold a name. Any other value is always shown.
  - A character that a terminal draws as nothing is written out, such as `\u200b`.
  - To leave a record out, answer `n`. Then share again with only the records that you want: name them by their ids, or narrow `-type` with `-since` and `-until`.
- **A bundle is a disclosure.** It is not encrypted, and once sent it cannot be taken back. Deleting or correcting a record later does not reach a bundle you sent: send a new one.
- **Keep the folder you sent** if you want to know later what was in it. The share record in your log lists ids; a record you deleted since is gone from your log, and the bundle is the only copy.
- **One share holds up to about 14,000 records.** Its list must fit one record. Share a long series in parts, by `-since` and `-until`.
- **Send it sealed.** `bean share -seal` writes one encrypted file that opens with six words ([above](#seal-it-for-one-reader)). `bean share -for` seals it to a reader's address ([above](#seal-it-to-a-readers-address)). A plain bundle is a folder that anyone who carries it can read. Zip it, and send it only over a channel that encrypts end to end. Without `-files` a bundle is a few kilobytes; with the NHANES files, 4 to 18 MB.
- **Two bundles of yours can be linked.** Every record carries your identity id.
- **Your identity records say how many records you had at each key change.** A key change lists the ids of the records it seals. The ids are hashes and give no content away, but their number shows.
- **A correction travels without what it corrects.** A record that replaces an earlier one is shared alone; the earlier one stays home. The check says how many such corrections a bundle holds.
- **A shared comparison gives its reading away**, with the public files ([COMPARE.md](COMPARE.md)). Its reading is in the bundle anyway.
- **A reader can build a reference pack from what people share.** A pack lists each value of a cell, and yours may be among them. Whoever has the pack reads it ([REFERENCE.md](REFERENCE.md)). Ask what a reader will do with your share before you send it.

## A signed reply

A reader can answer. They sign notes about the records that you shared, and send them back sealed. You see who signed them, and take them into your log under a name that you give.

The reader needs bean and a log of their own: a note is signed with their key. A reader who has only a browser cannot reply yet ([ROADMAP.md](ROADMAP.md)).

```
# the reader, after: bean check-bundle for-ana.bean
bean reply -to for-ana.bean -out reply-to-ben.bean 3fa9c1 "Looks fine to me. Ask about ApoB."

# you
bean accept -label Ana reply-to-ben.bean
```

**The reader's side.**

- `-to` is what they received. It must pass its check, so a sealed package asks for its six words. A package that is sealed to their address opens with `-key <file>`.
- Then come pairs: a record of the share, and the text of a note about it. Name the record by its id from the check, or by the first 6 or more hex digits. The id of the share record names the share as a whole.
- bean writes one sealed file and prints six words that open it. The reader sends the file one way and gives you the words another, as you did. With `-for`, bean seals it to your address instead ([below](#a-reply-to-an-address)).
- bean also prints the reader's identity words and the start of the reply's id. They tell you both.
- The reader's log keeps a `reply` record: to whom, to which share, and the ids of the notes. It does not hold the notes. The reader keeps the file and its words to read them again.
- **What a reply shows of the reader.** It holds their identity records, as a share holds yours. They show the reader's identity id and key, when they made their log, and when they changed a key.

**Your side.** `bean accept` asks for the six words and checks the reply. A reply that is sealed to your address opens with `-key <your key file>`. Then it shows:

- the share that it answers, with your note on that share;
- the start of the reply's id, and the key that signed it;
- the eight identity words of the identity that the key speaks for;
- whether you have seen this identity or this key before;
- each note in quotation marks, with the record that it is about.

Ask the reader two things: whether they sent a reply whose id starts so, and to read their identity words. If either is not the same, do not accept. Then bean asks before it writes.

- **`-label` is the name under which this person's notes show.** The first reply of an identity needs it. A later reply needs none. A name belongs to one identity: bean does not rename one, and does not give one name to two. A name has 1 to 64 characters: the letters a to z, digits, single spaces, and `. ' _ - @`. It does not start with the word `peer`, and no agent has it. Capital letters make no second name: `ana` is `Ana`. So one name cannot read as two, or as another person's.
- **`-yes` is for scripts, and needs `-words`.** A script cannot compare words by eye, so it states the eight words that you heard. bean takes nothing when they are not the signer's. Without a terminal and without `-yes` it takes nothing. `-yes` does not go with `-key` ([below](#a-reply-to-an-address)).
- **What goes into your log.** The notes, and one record of your own that pairs the reader's key with the name. That record holds the reader's identity id and how far their chain of keys had got. The reader's identity records and their reply record stay out.
- **Where you see them.** `bean ls` shows a note in quotation marks, with `[peer Ana]`. `bean show <id>` lists the notes about a record.
- **A note is a record about you.** It stays through a change of your key, it is in an export, and you can share it on. A reader of that share sees `[peer Ana]`, the reader's key, and your record of the name with their identity id.

**What a note is.** It is the reader's key speaking by itself about you: text about one record of that share. It holds no value and no file. It cannot correct a record or delete one. A record of a reader's key opens no file of yours, as a paired agent's does: not in a share, and not in an export. A reply that one of your own agent keys signed takes nothing. bean refuses a reply that holds anything else, such as a lab value in the place of a note.

**What a reply shows, and what it does not.**

- **A note is the word of the person who wrote it.** bean does not check what it says, and it is not medical advice.
- **The name is your word.** The reply shows that the key is the last key of an identity, by the identity records that came with it. It does not show that those records are all there are. You compared the eight words and the reply's id by voice: that is the check that the reply is this person's.
- **bean cannot tell whether a key is still current.** A reply shows the changes of key that were made before it. Someone who holds an older key of the reader shows the same eight words. The reply's id is what such a person cannot match: the reader never sent it.
- **Your log remembers how far a reader's keys had got.** A later reply must show the same chain of keys, or a longer one. So once you accept a reply of a reader's new key, a reply of their old key takes nothing. Nor does one with another history.
- **A reader who changes their key keeps their name.** Their next reply shows the change, and bean says that it is another key of an identity that you know.
- **If a reader's 24 words are stolen, the thief can change the key as the reader can.** Then a reply of the thief shows a longer chain and the same words. Only the reader can tell you, by voice, that the reply is not theirs.
- **Take your word back with `bean unpair <name>`.** A later reply of that key takes nothing. That holds across your copies: a pairing of the key that another copy of your log wrote does not count once the copies sync.
- **One name on two copies.** bean gives one name to one person. Two copies of your log can each give it to another person before they sync. Then each shows with the start of its identity id, as `[peer Ana #3fa9c1]`. `bean unpair Ana` takes the name back from both. To unpair one, give its key: `bean ls -all -json -type agent.pair` shows the key of each pairing. The notes that it signed stay in your log, and show as an unpaired key's. `bean purge <id>` deletes one: every record in your log is about you, and yours to delete. A note that you deleted is not taken again.
- **What the reader's own log says.** Their `reply` record names your identity id, the id of your share and the ids of the notes. It travels if they share that record, as when they share all of their records.

### A reply to an address

The reader can seal the reply to an address of yours, so that no six words are read out. You make a key file once with `bean address`. You give its address by one channel, and its address words by another ([above](#seal-it-to-a-readers-address)).

```
# the reader
bean reply -to for-ana.bean -for age1... -address-words "table nothing ..." -out reply-to-ben.bean 3fa9c1 "Looks fine to me."

# you, at your own terminal
bean accept -key ~/keys/bean-ben.key -label Ana reply-to-ben.bean
```

**The reader's side.**

- **`-for <address>` seals the reply to that address.** bean takes an address from this flag only. It reads no address from a package, and a share carries none.
- **Take the address from the sender, by a second channel.** A note of a share is the signer's text. Whoever holds any key of the signer, also a retired one, can write a note that names an address of their own.
- **The gate is that of `bean share -for`.** bean asks for the eight address words that the sender gave you, or takes `-address-words`. `-yes` never stands in for them. The gate runs before bean opens what you received. When the words are wrong, nothing is written.
- **Before it writes, bean says what this seal means:**
  - The reply is sealed to an address. Whoever holds the key file of that address can open it.
  - bean checked only that the address words belong to the address.
  - The person you reply to needs a bean that has `bean accept -key`. An earlier bean does not open it.
  - A program that runs as that person opens it with no word typed.
  - It stays closed only while the key file stays with that person and the cipher holds. That is so for as long as any copy of the file exists.
  - You cannot open this reply again, unless the address is your own. Your log keeps a record that you replied, and not the notes.
- **Then it asks, on a terminal.** `-yes` writes without the question. Without a terminal and without `-yes`, nothing is written. `-yes` and `-address-words` go with `-for` only.
- **It prints no six words.** It prints your identity words and the start of the reply's id, as for any reply.
- **The order of work is that of a share to an address.** bean seals in memory, and opens the result with the file's own key. It checks what came out. It makes the file, new and empty. It adds the `reply` record. Then it writes the bytes.
- **When the file cannot be written whole,** bean says what it did, as for a share. It names the path and the id of the reply record, and says that a part of such a file can open.
- **Your `reply` record says `"seal": "recipient"`.** It names no address. A reply with six words says `"seal": "passphrase"`.
- **Reply before you delete a key that you made for one sender.** `-to` needs the key to open the package. Or unpack the package first, and reply to the folder.

**Your side.**

- **`bean accept -key <your key file>` opens the reply.** Its checks are those of any reply.
- **What a key shows.** bean says first: the reply was sealed to your address. Anyone who has your address can seal to it, also an older reply of the signer. bean cannot tell who sealed it. The reply's id, which you compare by voice, tells you which reply it is.
- **A reply that someone sealed again.** When the replier's record does not say `recipient`, bean says so. That check looks one way only, as for a share. A reply whose record says `recipient`, and that opened with six words, is not marked. A record that says `recipient` shows what the replier wrote, and not for which address.
- **`-yes` does not go with `-key`.** `bean accept -key` asks on a terminal, and takes nothing into your log without one. A program that runs as you and holds the key file opens such a reply with no word typed. It can make the eight identity words from the reply itself, so `-words` is no check for it.
- **What that rule does not do.** It guards your log, and not what such a program reads. Without a terminal, `bean accept -key` still lists the notes. The `age` tool opens the file with the key. A program that makes a terminal of its own can answer the question. The rule stops the flag that a script would use, and it is no wall.
- **What that rule costs.** A person who hands the work to an assistant runs `bean accept -key` themselves. A key file with a passphrase is the gate that is planned ([SHARE-PLAN.md](SHARE-PLAN.md)).
- **Without `-key`,** bean says that the file is sealed to a key, and how to open it. With another key, bean says that it was sealed to another address, or changed on the way.
- **An earlier bean does not take it.** [UPGRADING.md](../UPGRADING.md) says what release 0.8.0 prints. Both people update first.

### The reply, exactly

A reply is a sealed package like a share: record files under `objects/`, packed and encrypted with six words or to an address. A check passes when all of these hold.

1. It holds records and nothing else: no file under `files/`, and no other name beside `objects`. A name that starts with a dot is not read, as in a bundle. Every record file passes the checks of a log, of this format and no newer. One warning is expected: a note is about a person whose identity records are not in the reply.
2. It holds exactly one record of type `reply`, with `source.system` `bean-reply`. A person signed it about themselves, with the last key of their identity's chain. That key is the reply's key. The chain is whole and not frozen.
3. The `reply` record's `context` has `to`, the identity id that the reply is for: not the replier's own. It has `share`, the id of the share record. It has `notes`, the ids of the notes, each once: from 1 to 200. `value.n` is their count. `bean reply` also writes `seal` there: `"passphrase"` or `"recipient"`, as in a share record. No rule reads it, and `bean accept -key` reads it once.
4. Every other record is an identity record on that person's chain, or a note that the `reply` record lists. Every listed note is there.
5. A note has the type `note`. Its `author` and its `kid` are the reply's key. Its `subject` is `to`. Its `value` is `{"s": text}`: a text of 1 to 4000 bytes in UTF-8. At least one character of it is not white space, by Unicode's `White_Space` property. It has one ref, `about` one record, and no `context` and no file.
6. A note's `source.system` is `bean-reply`, and its `source.ref` is null. It states one time, the time it was written. Its `times.observed` is null, or has a `start` in the minute of `times.imported` and no `end`.

`bean accept` adds what only your log can tell. `to` is your identity. `share` is a share record of yours. Each note is about a record that the share lists, or about the share record. The reply is one sealed file of at most 64 MiB. No note says that it was written later than a day from now. The reply's key is not a key that you unpaired. A note about a record that you deleted is left out, and so is a note that you deleted. The chain holds every head of that identity that you recorded before. The name is not one that another identity or an agent has.

`bean reply` and `tools/second/checkreply.py` both implement these six rules, and share no code. The Python tool reads the folder that `tools/second/unseal.py` opens a reply into. `bean check-bundle` on a reply says that it is one, when it passes the six rules.

Your record of the name is an `agent.pair` record with `value.role` `peer`. Its `value` also has `key`, `label`, `identity`, and `head`: the id of the last record of the reader's chain as the reply showed it.

## Check a shared comparison

A comparison with the survey's files, shared with `-files`, can be computed again from the bundle alone. That holds for files that bean has on record. A kept copy that is not on record is in the bundle only when you named its record. Without `-files` it needs the bundle and the files: fetched from CDC, or your own kept copies in a `bean export`.

```
python3 tools/compare/reproduce.py with-files
python3 tools/compare/reproduce.py for-ana DEMO_J.xpt TCHOL_J.xpt
python3 tools/compare/reproduce.py for-ana ~/bean-export/blobs
```

A comparison that was made with a reference pack needs that pack ([REFERENCE.md](REFERENCE.md)). `bean check-bundle` and the page show in the row what the reading was set against, and `by pack` with the first digits of its SHA-256. For the pack that bean has on record, the label and the cohort are bean's own words for that pack. A claim that says otherwise is marked. A pack that bean has no record of is marked `(NOT on record)`, and its label is in quotation marks: it is the claim's word. The numbers of a comparison are shown as claimed. Below the records they give the full SHA-256. Send the pack file with the package, or tell your reader where it is: it is public data.

```
python3 tools/compare/reproduce.py for-ana nhanes.pack
```

A comparison computes again with any pack that is well formed, and a made-up pack is well formed. So `reproduce.py` fails for a pack that is not on record, and says so. A reader who trusts that pack's publisher passes `-accept-pack` and its SHA-256. A comparison that names no pack and no files is marked `(names NO reference data)`: nobody can compute it again.

## The bundle, exactly

A bundle is a folder with two entries, both real folders: `objects/`, and `files/` when it holds files. A link or a file in their place fails the check. Anything else in the folder is not part of the bundle. A checker never reads it, so it cannot change the verdict, and `bean check-bundle` names it. A name that starts with a dot is skipped in silence, there and in `files/`, as in a log. A file browser leaves such files behind.

`objects/` holds record files in the layout of a log ([SPEC.md](../SPEC.md) 7.1): `objects/<first two hex digits of the id>/<the other 62>.json`, each the stored bytes of one record. `files/` holds shared files, each named by the 64 hex digits of its SHA-256. There is no `blobs/` and no `local/`: no encrypted file and no key travels.

The share record is an ordinary record:

| Field | Value |
|---|---|
| `type` | `share` |
| `author`, `subject` | the person's identity id, both |
| `value` | `{"n": <number of records listed>, "unit": "{record}"}` |
| `times.observed` | when it was written, to the minute |
| `context.records` | the ids of the shared records |
| `context.proof` | the ids of the identity records and pairings that travel with them |
| `context.files` | absent, or one entry per file in `files/`. An entry has `blob` (the id by which a listed record names the file) and `record` (that record). It also has `sha256` (64 hex digits) and `bytes` (a whole number) of the plaintext. A stored file that stays home has no entry: its own record names it. |
| `context.note` | the `-note`, if any |
| `context.seal` | `"passphrase"` when the share left sealed with six words. `"recipient"` when it left sealed to an address: the record names no address, and no hash of one. Absent for a folder. No rule of a bundle reads it. `bean check-bundle` reads it once, for a package that a key opened ([above](#seal-it-to-a-readers-address)). |

A bundle passes when all of this holds. A list in the share record that is not as the table says fails the check; it is never read in a kinder way.

1. Every file under `objects/` is a plain file of at most 16 MiB, two levels down. It passes the checks of a log with nothing left over: no error, no warning and no record of a newer format. No identity is frozen, and no key change has a part missing. Two things a log's verifier reports are expected, and only these:
   - a record that names a stored file is not at fault because the encrypted file is absent: a bundle never holds one;
   - a listed record that replaces an earlier record (`supersedes`) is held when that earlier record is not in the bundle. The check counts these.
2. Exactly one `share` record is there. It is valid, its `author` equals its `subject` and is an identity id, and its `value.n` is the number of ids in `context.records`.
3. Every other record is one that `context.records` lists or one that `context.proof` names, never both, and every id in either list is there.
   - A listed record is valid, has the person as its `subject`, and is not an identity record, a tombstone, a pairing or a share record.
   - A proof record is valid, and is one of two kinds. The first is an identity record (`bean.genesis`, `bean.succession`, `bean.seal`) on the person's chain. The second is a pairing in force: an `agent.pair` that the person signed about themselves and that does not say `"status": "unpaired"`.
4. Every entry of `context.files` is for a stored file that its listed record names, once. When that record has one stored file, names it in `value.blob` and states `context.sha256`, the entry's `sha256` is equal to it.
5. `files/` holds exactly the files that the entries list. Each is a plain file, not a link, with the entry's `bytes` and with the SHA-256 of its name.

A listed record that an agent or another key signed is valid by rule 1 and passes: the person chose to share it. The check counts such records apart: by the person, by an agent whose pairing is in the proof, by another key. It names each paired key with the label the person gave it, and marks each such row.

A checker reads `objects/` and `files/` and nothing else of the folder, opens nothing that is not a plain file, and writes nothing.

`bean check-bundle` and `tools/second/checkbundle.py` both implement these five rules, and share no code. The page `bean-check.html` is not a third implementation: it is `check-bundle`'s own check (`internal/bundle`), built for a browser. One author wrote both, so their agreement shows that the rules can be followed twice, not that a stranger has checked them. `make bundles` makes bundles with bean, good and damaged. Some hold records forged by a stranger, and some hold share records mis-signed by the person. It fails if the two disagree on any.
