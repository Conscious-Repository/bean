# A trial run of the network leg, on one machine

```
examples/pool/run.sh
```

One script plays two people, on this machine only. It is a test of the path, and no pool of people. Every contributor is made up: the archive is the test vector of [tools/pool](../../tools/pool/README.md).

[POOL-PLAN.md](../../docs/POOL-PLAN.md) says what this is for, and what it may claim. It is not in CI, and nothing in bean depends on it.

bean does not diagnose. Nothing here says whether a value is healthy.

## What it needs

- Go, `curl`, and Python 3 with the packages of `tools/requirements.txt`.
- `nak`, a command-line tool for Nostr. Its source is at `github.com/fiatjaf/nak`. You install it yourself. The author ran version 0.22.3, and the script prints the version that it runs.

The port is 10547. Set `POOL_PORT` for another.

## What it runs on your machine, and what stays

- It builds bean with Go into a scratch folder. The build fetches nothing: when a module is not on the machine, the script stops and says so. Go's own build cache stays, as after any build.
- It starts one server of `nak` on 127.0.0.1, at the port. It stops it at its end, also when you stop the script with Ctrl-C.
- It removes its scratch folder at its end. Nothing else stays.
- It uses no proxy, whatever your environment says. It sends no file and no event to another machine.

## What it does

| Step | Who | What happens |
|---|---|---|
| 1 | the script | It starts `nak serve --blossom` on 127.0.0.1: a test relay and a test file host. It stops them at its end. |
| 4 | the operator | Uploads the archive and the survey pack of the test vector. Builds the pool's pack with the URLs that the host gave. Uploads the pack. |
| 5 | the operator | Publishes one event of kind 1063 that names the pack: its URL, its SHA-256 and its size. |
| 6 | the other user | Finds the event with `nak req -k 1063`, and checks its signature with `nak verify`. Fetches the pack with `curl`. Holds it to the SHA-256 and the size of the event, and checks it with `tools/pack/check.py`. |
| 7 | the other user | Fetches the archive and the survey pack from the URLs that the pack names. Holds each to the SHA-256 and the size that the pack states. |
| 8 | the other user | Builds the pack again from those two files. The bytes must be the bytes that they fetched. |
| 9 | the other user | Compares a made-up reading with the pack: `bean compare -pack … -unknown-pack`. |

The numbers are the steps of the plan. Steps 2 and 3 are the keys and the other user's made-up log.

The other user's part reads the relay, the host and this repository. It reads nothing that the operator's part left on the disk.

## Keys

Every command that signs gets a key that `nak key generate` made for that one command. When `nak` prints no key, the script stops and signs nothing. One key signs the pointer, and the script checks that the event carries it.

No key of a log signs for the relay or the host. With no key, `nak` signs with a default key of your machine. So the script clears `NOSTR_SECRET_KEY` first, and sets it for one command at a time.

## What it prints

It prints bean's own output for step 9. Then, only when steps 6 to 8 matched, what it checked:

- The host served the bytes that the pointer named: its SHA-256 and its size.
- The pack is what this builder gives from the archive and the survey pack that the host served.
- Whether those two files are the same bytes as the two files of the test vector in this repository.

Then what it cannot show:

- One operator ran the host and the relay, so the two agree by one hand.
- Nothing shows who made the keys in the archive.

Then which of bean's own words above are wrong for this pack.

When a check of steps 6 to 8 does not match, it prints `NOT reproduced`, and stops. A pointer that has a tag twice names nothing, and is such a mismatch.

It prints `STOPPED` when a step could not run, and says which. One case is a URL that is not on its own server: it fetches from no other host. In both cases nothing was published. Read the line, and run the script again.

## What ties the files to the test vector

A host that serves three files that agree with each other passes steps 6 to 8. Only the third check ties the archive and the survey pack to the vector. Without it, the pack was built again from files that nobody else holds.

## bean's fixed words are wrong here

Under the result bean prints `N 40`, "fewer than 100 people" and "in this sample". Those are forty keys of one script.

bean also says to compare the SHA-256 "with the one that its publisher states". This pack has no publisher. The measure keeps the survey pack's own label.

## What this trial run is not

- It is not the document of a first setup that DECISIONS 325 asks for. That one is still owed before a first pack is announced.
- `nak serve` is a test server. It keeps everything in memory, and all of it is gone when the script ends.
- It publishes nothing. The script sends no file and no event to another machine.

## What else serves

Finding the event needs a Nostr client, and `curl` is none. Fetching and building again need only the bytes and a SHA-256: any file host or a disk serves them. A file and its SHA-256 on a page is a full publication.

## Without `nak`

Start at step 8, with the files in this repository:

    make pool

Its first test builds the pack from the two committed files. It compares the SHA-256 with the one that the repository states.
