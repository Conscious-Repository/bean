# Share records with someone

`bean share` writes records you choose into a folder: a bundle. You send the folder any way you like. The person who gets it can check that every record is yours and unchanged, with `bean check-bundle` or with a Python script, and needs no account, no key of yours and no copy of your log.

```
bean share -out for-ana 3fa9c1 7b8b81              # these records
bean share -out lipids -type 'comparison.*'        # every saved comparison, each with its reading
bean share -out with-files -files 7b8b81           # also the files those records keep
```

## What goes in

- **The records you name**, by id or by `-type` (with `-since` and `-until`). Only current records about you: not a corrected one, not a deleted one, and not one that another identity signed, since a bundle carries only your identity records.
- **What they are about, one step.** A record is read with the record it is about, so that one comes along: the reading of a comparison, the meal of an estimate, the entry a note comments on. What that one is about in turn stays home unless you name it. The review marks each record that comes along, and says which it could not bring and why.
- **Nothing else of yours.** Not the rest of the log, not a lab report unless you name it. A reading names its report by a hash, and the report stays home.
- **Files only with `-files`.** A record that keeps a file (a lab report, a photo, a reference file) is shared without the file. With `-files` each file goes in whole, as plaintext: for a lab report that is the report as you imported it, with your name and birth date.
- **A type never picks a record that keeps a file.** `-type 'lab.*'` shares your lab readings, not the lab report. Name such a record by its id when you mean to share it.
- **Your identity records**: the ones that show which keys speak for you, and the pairing of an agent whose record you share.
- **One share record**, signed by you: the list of exactly what the bundle holds, your `-note`, and the time.

bean shows the list and asks before it writes: each record, the cohort of a comparison, each file, and how many identity records go along. `-yes` skips that, for scripts. Without a terminal and without `-yes` it writes nothing. That stops an assistant from sharing through a tool that has no terminal. It does not stop a program that has your shell and passes `-yes`: what runs as you can do what you can.

The bundle is written whole or not at all: it is built beside its place, checked as its reader will check it, and moved there. Only then does the share record go into your log, as type `share`: your own record of what you disclosed and when. `bean ls -type share` lists them. A share that fails leaves no bundle and no record.

## Reading a bundle someone sent you

```
bean check-bundle for-ana
```

One command checks the bundle and, when it passes, shows what it holds: each record with its time, type and value, and each comparison in words, such as `percentile 46 of lab.cholesterol-total 187 mg/dL, among male, age 30-39 (n 339)`. Nothing is shown from a bundle that fails. You need no log of your own for this, and the check changes nothing in the folder.

`python3 tools/second/checkbundle.py for-ana` does the same check without bean.

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
- **Who you are.** An identity is a key, not a name.
- **That the key is still yours.** A bundle holds the key changes made before it was signed, not later ones. Someone who stole a key you have since retired can sign a bundle with it, leave the retirement out, and it passes. The check prints the key that signed. When it matters, the reader asks you for your current key (`bean whoami`) by another way, and compares.

And you should know:

- **A bundle is a disclosure.** It is not encrypted, and once sent it cannot be taken back. Deleting or correcting a record later does not reach a bundle you sent: send a new one.
- **Keep the folder you sent** if you want to know later what was in it. The share record in your log lists ids; a record you deleted since is gone from your log, and the bundle is the only copy.
- **Send it as one file.** A bundle is a folder: zip it. Without `-files` it is a few kilobytes; with the NHANES files about 18 MB.
- **Two bundles of yours can be linked.** Every record carries your identity id.
- **Your identity records say how many records you had at each key change.** A key change lists the ids of the records it seals. The ids are hashes and give no content away, but their number shows.
- **A correction travels without what it corrects.** A record that replaces an earlier one is shared alone; the earlier one stays home. The check says how many such corrections a bundle holds.
- **A shared comparison gives its reading away**, with the public files ([COMPARE.md](COMPARE.md)). Its reading is in the bundle anyway.

## Check a shared comparison

A comparison shared with `-files` can be computed again from the bundle alone, and without them from the bundle and the files from CDC:

```
python3 tools/compare/reproduce.py with-files
python3 tools/compare/reproduce.py for-ana DEMO_J.xpt TCHOL_J.xpt
```

## The bundle, exactly

A bundle is a folder with two entries, both real folders: `objects/`, and `files/` when it holds files. A link or a file in their place fails the check. Anything else in the folder is not part of the bundle: a checker never reads it, so it cannot change the verdict, and `bean check-bundle` names it. A name that starts with a dot is skipped in silence, there and in `files/`, as in a log: a file browser leaves such files behind.

`objects/` holds record files in the layout of a log ([SPEC.md](../SPEC.md) 7.1): `objects/<first two hex digits of the id>/<the other 62>.json`, each the stored bytes of one record. `files/` holds shared files, each named by the 64 hex digits of its SHA-256. There is no `blobs/` and no `local/`: no encrypted file and no key travels.

The share record is an ordinary record:

| Field | Value |
|---|---|
| `type` | `share` |
| `author`, `subject` | the person's identity id, both |
| `source.system` | `bean-share` |
| `value` | `{"n": <number of records listed>, "unit": "{record}"}` |
| `times.observed` | when it was written, to the minute |
| `context.records` | the ids of the shared records |
| `context.proof` | the ids of the identity records and pairings that travel with them |
| `context.files` | absent, or one entry per file in `files/`: `blob` (the id by which a listed record names it), `record` (that record), and `sha256` (64 hex digits) and `bytes` (a whole number) of the plaintext. A stored file that stays home has no entry: its own record names it. |
| `context.note` | the `-note`, if any |

A bundle passes when all of this holds. A list in the share record that is not as the table says fails the check; it is never read in a kinder way.

1. Every file under `objects/` is a plain file of at most 16 MiB, two levels down, and passes the checks of a log with nothing left over: no error, no warning, no record of a newer format, no frozen identity, no key change with a part missing. Two things a log's verifier reports are expected, and only these:
   - a record that names a stored file is not at fault because the encrypted file is absent: a bundle never holds one;
   - a listed record that replaces an earlier record (`supersedes`) is held when that earlier record is not in the bundle. The check counts these.
2. Exactly one `share` record is there. It is valid, its `author` equals its `subject` and is an identity id, its `source.system` is `bean-share`, and its `value.n` is the number of ids in `context.records`.
3. Every other record is one that `context.records` lists or one that `context.proof` names, never both, and every id in either list is there.
   - A listed record is valid, has the person as its `subject`, and is not an identity record, a tombstone, a pairing or a share record.
   - A proof record is valid, and is an identity record (`bean.genesis`, `bean.succession`, `bean.seal`) on the person's chain, or a pairing (`agent.pair`, `device.pair`) that the person signed about themselves.
4. Every entry of `context.files` is for a stored file that its listed record names, once. When that record has one stored file, names it in `value.blob` and states `context.sha256`, the entry's `sha256` is equal to it.
5. `files/` holds exactly the files that the entries list, each a plain file, not a link, with the entry's `bytes` and with the SHA-256 of its name.

A listed record that an agent or another key signed is valid by rule 1 and passes: the person chose to share it. The check counts such records apart: by the person, by an agent whose pairing is in the proof, by another key.

A checker reads `objects/` and `files/` and nothing else of the folder, opens nothing that is not a plain file, and writes nothing.

`bean check-bundle` and `tools/second/checkbundle.py` both implement these five rules, and share no code. One author wrote both, so their agreement shows that the rules can be followed twice, not that a stranger has checked them. `make bundles` makes bundles with bean, good and damaged, with records forged by a stranger and share records mis-signed by the person, and fails if the two disagree on any.
