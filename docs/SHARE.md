# Share records with someone

`bean share` writes records you choose into a folder: a bundle. You send the folder any way you like. The person who gets it can check that every record is yours and unchanged, with `bean check-bundle` or with a Python script, and needs no account, no key of yours and no copy of your log.

```
bean share -out for-ana 3fa9c1 7b8b81          # these records
bean share -out lipids -type 'lab.*' -since 2026-01-01
bean share -out with-files -files 7b8b81       # also the files they keep
```

## What goes in

- **The records you name**, by id or by `-type` (with `-since` and `-until`). Only current records about you: not a corrected one, not a deleted one.
- **What they are about.** A record is read with the record it is about, so that one comes along: the reading of a comparison, the meal of an estimate, the entry a note comments on.
- **Nothing else of yours.** Not the rest of the log, not a lab report unless you name it. A reading names its report by a hash, and the report stays home.
- **Files only with `-files`.** A record that keeps a file (a lab report, a photo, a reference file) is shared without the file, which the bundle then names and does not hold. With `-files` each file goes in as plaintext.
- **Your identity records**: the ones that show which keys speak for you, and the pairing of an agent whose record you share.
- **One share record**, signed by you: the list of exactly what the bundle holds, your `-note`, and the time.

bean shows the list and asks before it writes. `-yes` skips that, for scripts. Without a terminal and without `-yes` it writes nothing, so an assistant that runs it cannot disclose for you.

The share record also stays in your log, as type `share`: your own record of what you disclosed and when. `bean ls -type share` lists them.

## What the reader can and cannot tell

```
bean check-bundle for-ana
python3 tools/second/checkbundle.py for-ana
```

They can tell that:

- each record is byte for byte what your key signed, and that key spoke for your identity when it signed;
- the bundle holds exactly the records you listed, no more and no fewer;
- each file is the one you listed, and the one its own record states, where the record states its SHA-256;
- which records you signed, which an agent you paired signed, and which another key signed.

They cannot tell, and bean says so on every check:

- **That it is complete.** A bundle is the records you chose. A correction or a deletion you left out, or made later, does not show. "All my readings this year" is your word, not something the bundle proves.
- **That a value is true.** A signature shows who signed.
- **Who you are.** An identity is a key, not a name.

And you should know:

- **A bundle is a disclosure.** It is not encrypted, and once sent it cannot be taken back. Deleting a record later does not reach a bundle you sent.
- **Two bundles of yours can be linked.** Every record carries your identity id.
- **Your identity records say how many records you had at each key change.** A key change lists the ids of the records it seals. The ids are hashes and give no content away, but their number shows.
- **A shared comparison gives its reading away**, with the public files ([COMPARE.md](COMPARE.md)). Its reading is in the bundle anyway.

## Check a shared comparison

A comparison shared with `-files` can be computed again from the bundle alone, and without them from the bundle and the files from CDC:

```
python3 tools/compare/reproduce.py with-files
python3 tools/compare/reproduce.py for-ana DEMO_J.xpt TCHOL_J.xpt
```

## The bundle, exactly

A bundle is a folder with two entries.

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
| `context.files` | one entry per stored file that a shared record names: `blob` (the id the record names), `record`, `media_type`, `included`, and `sha256` and `bytes` of the plaintext when it is included. A record that states its file's `sha256` gives it here also when the file is not included. |
| `context.note` | the `-note`, if any |
| `context.statement` | `Selected records, not a complete history.` |

A bundle passes when all of this holds:

1. Every file under `objects/` passes the checks of a log: its id is the hash of its content, its signature is good, and by the rules of [SPEC.md](../SPEC.md) its key spoke for its author. A record that names a stored file is not at fault because the encrypted file is absent: a bundle never holds one.
2. Exactly one valid `share` record is there, with `author` equal to `subject`, an identity id, and `source.system` `bean-share`.
3. Every other record is either one that `context.records` lists, or an identity record (`bean.genesis`, `bean.succession`, `bean.seal`) of that person, or a pairing (`agent.pair`, `device.pair`) that the person signed about themselves. Every listed record is there, is valid, and has the person as its `subject`.
4. Every stored file that a listed record names has an entry in `context.files` with that `blob` and `record`. When the record has one stored file, names it in `value.blob` and states `context.sha256`, the entry's `sha256`, if it has one, is equal to it.
5. `files/` holds exactly the files whose entries say `included`, each a plain file with the entry's `bytes` and with the SHA-256 of its name.

A record that an agent or another key signed is valid by rule 1 and passes: the person chose to share it. The check counts such records apart.

`bean check-bundle` and `tools/second/checkbundle.py` both implement these five rules, and share no code. One author wrote both, so their agreement shows that the rules can be followed twice, not that a stranger has checked them. `make bundles` makes bundles with bean, good and damaged, and fails if the two disagree on any.
