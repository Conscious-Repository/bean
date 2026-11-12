# Share records with someone

`bean share` writes records you choose into a folder: a bundle. You send the folder any way you like. The person who gets it can check that every record is yours and unchanged: with `bean check-bundle`, with a Python script, or in a browser. They need no account, no key of yours and no copy of your log.

```
bean share -out for-ana 3fa9c1 7b8b81              # these records
bean share -out lipids -type 'comparison.*'        # every saved comparison, each with its reading
bean share -out with-files -files 7b8b81           # also the files those records keep
bean share -seal -out for-ana.bean 3fa9c1          # one file that only its reader can open
```

## Seal it for one reader

A folder is plain: whoever carries it can read it. With `-seal`, bean writes one file that is encrypted, and prints six words that open it.

```
$ bean share -seal -out for-ana.bean -note "my lipids" 3fa9c1
shared 1 records and 0 files in for-ana.bean
share record sha256:41ae...
The package is sealed. It opens with these six words:
  moral arrive identify have dress oblige
Your identity words, which are not a secret:
  injury glow drink patch dynamic submit
```

- **Send the file one way and the words another.** Send the file by e-mail or any other way. Give the words by a phone call, in person, or in a message on another service. Whoever has only the file can read nothing in it: no record, no type, no note.
- **Read your identity words to the reader, in the same call.** They are six words made from your identity id, and `bean whoami` shows them. The reader's check shows the identity words of whoever signed. If the two are the same, the records are from your identity. A changed package cannot pass this: nobody else can sign as your identity.
- **The words also show who sealed it.** A package opens only with its own six words. So whoever sealed a package that opens knew the words: you, or someone you told.
- **The reader needs no key and no account.** They open the file in a browser with `bean-check.html`, which asks for the words, or with `bean check-bundle for-ana.bean`. Both open it on the reader's device and then check it as they check a folder.
- **bean makes the words and does not keep them.** Six words of a list of 2048 are 66 bits that no person chose. If the words are lost, share again: a new package gets new words.
- **A changed package does not open.** Nor does a package that was cut short. The three openers read the format strictly, and `make bundles` holds them to one verdict.
- **Nothing of it is written in the clear.** bean builds, checks and encrypts the package in memory, then writes the one file. No folder of plain records is made beside it, at any moment.
- **The reader can get a carried file out.** The page lists each file that the package carries, with a button to save it. `bean check-bundle -unpack <new folder> for-ana.bean` writes the bundle's folder, with its files. What is saved is what was checked.
- **What its size shows.** A package grows by about 700 bytes a record, and by the size of each file. Whoever sees the file can estimate how much it holds, and can tell that bean sealed it.
- **The format is not bean's own.** A package is the bundle's folder as a zip file, encrypted in the age format (age-encryption.org/v1) with the words as its passphrase. `age -d` opens one. So does `tools/second/unseal.py`, which reads the format from its specification and shares no code with bean.
- **What it does not do.** A reader who has opened a package holds a copy for good. Whoever learns both the file and the words can open it. The words protect the package on its way, not after.

Your log's share record says `"seal": "passphrase"`. It does not hold the words.

## What goes in

- **The records you name**, by id or by `-type` (with `-since` and `-until`). Only current records about you: not a corrected one and not a deleted one. A record that another identity signed does not go either, since a bundle carries only your identity records.
- **What they are about, one step.** A record is read with the record it is about, so that one comes along. Examples are the reading of a comparison, the meal of an estimate, and the entry a note comments on. A comparison also brings the record of each reference file it used, without the file. What that one is about in turn stays home unless you name it. The review marks each record that comes along, and says which it could not bring and why.
- **Nothing else of yours.** Not the rest of the log, not a lab report unless you name it. A reading names its report by a hash, and the report stays home.
- **A file only when you name its record.** A record that keeps a file (a lab report, a photo) is shared without the file. With `-files`, the file of a record you named by its id goes in whole, as plaintext. For a lab report that is the report as you imported it, with your name and birth date. A file never goes because its record came along with another, or was picked by type. The one exception is a public reference file you kept, such as an NHANES file: with `-files` it goes with the comparison that used it.
- **A type never picks a record that keeps a file.** `-type 'lab.*'` shares your lab readings, not the lab report. Name such a record by its id when you mean to share it.
- **A read-me.** `README.txt` tells someone who has only the folder what it is and how to check it. It holds no value, and a check does not read it.
- **Your identity records**: the ones that show which keys speak for you, and the pairing of an agent whose record you share.
- **One share record**, signed by you: the list of exactly what the bundle holds, your `-note`, and the time.

bean shows the list and asks before it writes: each record, the cohort of a comparison, each file, and how many identity records go along. `-yes` skips that, for scripts. Without a terminal and without `-yes` it writes nothing. That stops an assistant from sharing through a tool that has no terminal. It does not stop a program that has your shell and passes `-yes`: what runs as you can do what you can.

The bundle is written whole or not at all: it is built beside its place, checked as its reader will check it, and moved there. Only then does the share record go into your log, as type `share`: your own record of what you disclosed and when. `bean ls -type share` lists them. A share that fails leaves no bundle and no record. Ctrl-C before the bundle is in place removes what was built. From that moment the share runs to its end, so that no bundle exists without its record. Only a kill that cannot be caught (`kill -9`) can leave one without the other. For a plain bundle, such a kill can also leave the half-built folder, named `.bean-share-` and a number, beside the place you named: remove it. For a sealed package it can leave a package that nobody can open.

## Reading a bundle someone sent you

```
bean check-bundle for-ana          # a folder, or a zip file of it
bean check-bundle for-ana.bean     # a sealed package: it asks for the six words
```

One command checks the bundle and, when it passes, shows what it holds. Each record has its time as the sender recorded it, its type and its value. A record that the sender did not sign shows who signed it. Each comparison is in words, such as `percentile 46 of lab.cholesterol-total 187 mg/dL, among male, age 30-39 (n 339)`, with its caveats below. A bundle that fails lists no record and shows no note. The check still prints the share record's id, its signer and the counts. You need no log of your own for this, and the check changes nothing in the folder.

A comparison is shown as its claim states it. The check proves who signed the claim, not its arithmetic: `tools/compare/reproduce.py` computes it again ([COMPARE.md](COMPARE.md)).

`python3 tools/second/checkbundle.py for-ana` runs the same five rules without bean. Its listing is plainer: each record's time, type and raw value.

### In a browser, with no install

A reader with no terminal opens one page, `bean-check.html`, and chooses the file they received, or a folder. The page gives one of four verdicts:

- **The records passed.** The page says what the check shows and what it does not. It shows the signer's identity words, your note, each record, and what no bundle can show. A text file that the package carries can be read in the page, and any file can be saved.
- **The records failed.** The page shows what failed and nothing else: no record, no note, and not who signed.
- **The file did not open.** The words were wrong, or the file was changed. The form stays, to try again.
- **Nothing was checked.** The wrong folder, or a file that is not a bundle.

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

- **A reader with no terminal** gets the page from the bean project, not from the sender. The sender tells them where, in the same call that gives the words. Today the project has no public address for the page: that is the next step ([ROADMAP.md](ROADMAP.md)). Until then, such a reader trusts the sender for the page as well.
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
- **Who you are.** An identity is a key, not a name. The check shows the identity as six words. Your reader knows that it is you only when you read them the same words, by a way that an attacker does not control.
- **That the key is still yours.** A bundle holds the key changes made before it was signed, not later ones. Someone who stole a key you have since retired can sign a bundle with it, leave the retirement out, and it passes. The check prints the key that signed. When it matters, the reader asks you for your current key (`bean whoami`) by another way, and compares.

And you should know:

- **A bundle is a disclosure.** It is not encrypted, and once sent it cannot be taken back. Deleting or correcting a record later does not reach a bundle you sent: send a new one.
- **Keep the folder you sent** if you want to know later what was in it. The share record in your log lists ids; a record you deleted since is gone from your log, and the bundle is the only copy.
- **One share holds up to about 14,000 records.** Its list must fit one record. Share a long series in parts, by `-since` and `-until`.
- **Send it sealed.** `bean share -seal` writes one encrypted file that opens with six words ([above](#seal-it-for-one-reader)). A plain bundle is a folder that anyone who carries it can read. Zip it, and send it only over a channel that encrypts end to end. Without `-files` a bundle is a few kilobytes; with the NHANES files, 4 to 18 MB. A package sealed to a reader's own key is on the roadmap.
- **Two bundles of yours can be linked.** Every record carries your identity id.
- **Your identity records say how many records you had at each key change.** A key change lists the ids of the records it seals. The ids are hashes and give no content away, but their number shows.
- **A correction travels without what it corrects.** A record that replaces an earlier one is shared alone; the earlier one stays home. The check says how many such corrections a bundle holds.
- **A shared comparison gives its reading away**, with the public files ([COMPARE.md](COMPARE.md)). Its reading is in the bundle anyway.

## Check a shared comparison

A comparison shared with `-files` can be computed again from the bundle alone. Without `-files` it needs the bundle and the files: fetched from CDC, or your own kept copies in a `bean export`.

```
python3 tools/compare/reproduce.py with-files
python3 tools/compare/reproduce.py for-ana DEMO_J.xpt TCHOL_J.xpt
python3 tools/compare/reproduce.py for-ana ~/bean-export/blobs
```

A comparison that was made with a reference pack needs that pack ([REFERENCE.md](REFERENCE.md)). `bean check-bundle` and the page show in the row what the reading was set against, and `by pack` with the first digits of its SHA-256. A pack that bean has no record of is marked there: `(NOT on record)`. Below the records they give the full SHA-256. Send the pack file with the package, or tell your reader where it is: it is public data.

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
| `context.seal` | `"passphrase"` when the share left as a sealed package; absent for a folder |

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
