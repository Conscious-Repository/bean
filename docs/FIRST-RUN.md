# A first exchange between two people

This is one page for two people who each run bean in a terminal. One shares records, the other reads them and answers, and then the two change places. Then each compares with a pack from a host. No six words are read out. [SHARE.md](SHARE.md) has the reasons for each step.

**bean is not a diagnosing tool.** A note from a peer is that person's word, and not medical advice.

**Each person runs every command here at their own terminal.** A check prints the other person's values. So no assistant runs these commands, and no assistant sees a key file ([ONBOARDING.md](ONBOARDING.md)).

In the examples, Ben shares and Ana answers.

## Before the call

1. **Both install release 0.8.4 or later.** `bean version` names the release. 0.8.1 is the oldest release that opens these files. 0.8.2 also reads a pack with a cell of fewer than 30 people ([UPGRADING.md](../UPGRADING.md)).

   ```
   go install github.com/Conscious-Repository/bean/cmd/...@latest
   bean version
   ```

2. **Each has a log,** made with `bean init` ([ONBOARDING.md](ONBOARDING.md)), and one reading or more. A reading comes from an import, or you enter it by hand:

   ```
   bean add lab.cholesterol-total 187 -unit mg/dL -at 2026-11-01 -note "typed from my report"
   ```

   A value that you entered by hand reads like a lab's result. Say so in the note.

3. **Each makes a key file and an address, once.** The folder must be there. bean writes over no file.

   ```
   mkdir -p ~/keys && chmod 700 ~/keys
   bean address -out ~/keys/bean.key
   ```

   It prints your address, which starts `age1`, and eight address words. Neither is a secret. The key file is a secret: bean does not encrypt it, and an export and the 24 words do not hold it. If you lose it, no package that was sealed to its address opens again.

4. **Each sends their address to the other,** in a message. Do not send the address words in that message.

## The call

One call, by voice. Each writes down what the other reads.

- **Your eight identity words.** `bean whoami` shows them. They say which identity signs your records.
- **Your eight address words.** `bean address -key ~/keys/bean.key` shows them again. They let the other person catch a wrong or swapped address.

## Ben shares

```
bean ls
bean share -for <Ana's address> -note "my lipids" -out for-ana.bean <id> <id>
```

- `bean ls` shows the id at the end of each row.
- bean asks for Ana's eight address words. Type the ones from the call.
- bean shows what leaves, and what a seal to an address means. Read it, then answer `y`.
- bean prints the start of the share's id. Tell Ana.
- Send `for-ana.bean` by a channel that you two already use. Do not put it on a public host.

## Ana reads, and answers

```
bean check-bundle -key ~/keys/bean.key for-ana.bean
bean reply -to for-ana.bean -key ~/keys/bean.key -for <Ben's address> -out reply-to-ben.bean <id> "Looks fine to me."
```

- The check shows the signer's identity words. They must be the words that Ben read in the call. If they are not, stop.
- The check shows the start of the share's id. It must be the one that Ben told you.
- The check lists each record with its id. A note is about one record: give its id, or the first 6 digits or more.
- A comparison is shown as Ben claims it. With the pack it names, `bean check-bundle -pack <file> -key ~/keys/bean.key for-ana.bean` computes it again. That shows the arithmetic, not that the value is true.
- `bean reply` asks for Ben's eight address words, says that you cannot open the reply again, and asks. Answer `y`.
- bean prints the start of the reply's id. Tell Ben.
- Send `reply-to-ben.bean` to Ben.

## Ben takes the notes

```
bean accept -key ~/keys/bean.key -label Ana reply-to-ben.bean
bean ls
```

- bean shows the identity words of whoever signed the reply. They must be the words that Ana read in the call.
- bean shows the start of the reply's id. It must be the one that Ana told you. If either is not the same, answer `n`.
- `-label` is the name under which Ana's notes show. `bean ls` then shows each note with `[peer Ana]`.

Then change places: Ana shares, and Ben answers.

## Each compares with a pack

Each does this alone, at their own terminal: `bean compare` prints your own values. bean fetches nothing, so you download the pack yourself ([REFERENCE.md](REFERENCE.md)).

```
digest=5cae07db64295d565ffae7c251c17fdb9f043dc0638868557275f8b0b0311efa
origin=https://stele.aion.bio
curl -fsS "$origin/$digest" -o nhanes.pack &&
  printf '%s  nhanes.pack\n' "$digest" | shasum -a 256 -c - &&
  bean compare -pack nhanes.pack
```

The digest above is the one Bean records for the 20-measure pack. On Linux, use `sha256sum -c -` in place of `shasum -a 256 -c -`. Continue once the file passes and Bean lists its cohorts.

```
bean compare -pack nhanes.pack -cohort male-30-39 -type 'lab.*'
```

- If Stele is down, set `origin=https://codex-reference-library-preview-aionbio.me-3ff.workers.dev` and repeat the download and check with the same digest. Both hosts use Cloudflare. Any lab can serve the same digest path. The [host check](../examples/pack-host/README.md) also works with either origin.
- The host sees your network address, the pack path and the time of the fetch. The reading and the choice of cohort stay on your machine.
- `bean compare -pack nhanes.pack` must say that bean has the pack on record. It lists the cohorts, and the type names and units that the pack takes.
- Choose your own cohort. A reading in another unit gets no number.
- One run sets every reading against the one cohort that you name, also an old reading. For a reading from when you were in another age group, run again. Name that cohort, and the reading by its id.
- From 0.8.5, bean can give each reading the cohort of its own date. It needs two records that you state once ([REFERENCE.md](REFERENCE.md#the-cohort-from-two-records)). You do not have to state them: `-cohort` needs neither.
- What bean checks here, and what no check shows: [ARCHITECTURE.md](ARCHITECTURE.md#what-bean-checks-in-a-comparison).
- With no host, build the same pack from the survey's files (REFERENCE.md). Its SHA-256 is the same.

## When a file does not open

| bean says | What to do |
|---|---|
| `is sealed to a key, not with words` | Add `-key ~/keys/bean.key` before the file's name. |
| `sealed to another address, or it was changed on the way` | `bean address -key ~/keys/bean.key` shows your address. Compare it with the one that the sender used. |
| `these are not the address words of this address` | You mistyped a word, or the address is not the one from the call. Ask again. |
| `-yes does not go with -key` | Run `bean accept -key` yourself, at a terminal. |
| bean asks for six words | You run an earlier bean, or the sender sealed with `-seal`. Update, or ask the sender for the six words. |

**The way back to six words.** `bean share -seal` and `bean reply` without `-for` work as before. bean prints six words, and you read them out in a call.

## Keep, and write down

- **Keep your key file, and each package that you want to read again.** A backup of the log and an export hold neither ([README](../README.md#backups-and-a-lost-laptop)).
- **Write down how long the run took, from the install on, and where you were stuck.** The status of 0.9 is written from that ([ROADMAP.md](ROADMAP.md)).
