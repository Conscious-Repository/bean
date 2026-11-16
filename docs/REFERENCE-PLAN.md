# Reference data: what is decided before more is built

This is for anyone who builds on bean's reference data: a publisher of a pack, or the writer of a tool. [REFERENCE.md](REFERENCE.md) is the format that exists. This document says what changes next, what does not, and why. Nothing in it is built yet.

It is phase 1 of the order in [ROADMAP.md](ROADMAP.md). That phase decides on paper what is in logs and shared files for good once it is used. Four drafts were written, and reviewers who had not seen the work judged each. They sat in five seats: Linus, Satoshi, buildtall, a publisher from outside, and the writer of a second reader. DECISIONS 321 has the account.

**The main result is a refusal.** A format, a signed claim and a record type are costs that every reader carries for ever. So nothing new is frozen without a real case. Each thing that waits is written down with the case that would start it.

## What was measured

The public files of the survey are NHANES 2017-2018. Each fact here was measured by the author and again by one reviewer or more.

**F1. Zero is a real value, in tests that are coarse.** In the file `CBC_J`, 3802 of 7523 people have a basophil count of 0, and 427 have an eosinophil count of 0. `pack/0` cannot hold a value of 0. The survey records these counts in steps of 0.1, and a cell has about six distinct values. A reading of 0 lies at percentile 23 and a reading of 0.1 at 73. A reading of 0.12, which is finer than the step, lies at 99: that number means nothing. As a share of all white cells, basophils have no zero, and eosinophils have 10 in 7523.

**F2. A detection limit is real, and it touches few cohorts.** The survey marks a value below its limit of detection, and writes a fill value in its place. Among the adults of the pack's menu:

| Test | Adults marked | Cohorts of 14 with a marked person | Largest share of a cohort's weight |
|---|---|---|---|
| ALT | 0 of 4915 | 0 | 0 |
| GGT | 1 of 4915 | 1 | 0.37% |
| Total bilirubin | 1 of 4916 | 1 | 0.58% |
| C-reactive protein | 10 of 4909 | 6 | 0.82% |

These four tests are on almost every real report, and have no reference data today (DECISIONS 240, 309).

**F3. A pack that is not in canonical form passes both checks.** Its SHA-256 differs from that of the same content in canonical form. Ten such files were tried, and bean and the Python check gave the same verdict on each. The worst case is another pack, which bean has no record of. Nobody is misled.

**F4. A recipe works today, with tools that exist.** In an empty folder: a `go.mod` that requires `github.com/Conscious-Repository/bean v0.7.1`, and a `go.sum` with the lines of the build. Then, with `GOFLAGS=-mod=readonly` and `GOTOOLCHAIN=local`: `go build github.com/Conscious-Repository/bean/cmd/bean-pack-nhanes`. The program that came out wrote both packs on record from the survey's files, byte for byte. With one stated line changed, the tool stopped with `SECURITY ERROR`. With one line missing, it stopped too. Go 1.24.7 built it, not the 1.27.3 that the repository names, and the bytes were the same.

**F5. What that build checks, and what it does not.** With stated lines the `go` tool does not ask the public log of Go modules. So the lines are the publisher's word, until a second build without them asks the log. The plain `go install ...@v0.7.1` asks the log, and never reads lines that a recipe states. Two builds of one source gave two binaries of different size: a recipe pins the source, not a binary.

**F6. A second program builds the same bytes.** `tools/pack/rebuild_nhanes.py` is in Python and shares no code with bean. To write the same pack it writes the same `builder.name` and `builder.version`. So those members name a rule, not the program that ran. Both programs have one author.

**F7. The names are a rule, not a list.** `bean-import-lab` makes a type from the test name that the lab prints, and `cmd/bean/vocabulary.md` gives the same rule to a person's assistant. The unit is the unit as printed. A type holds ASCII only: the core refuses `lab.гемоглобин`. A reading of `lab.platelets` in `K/uL` gets this today: `not comparable: the reading is in "K/uL", and bean compares this type only in x10E3/uL`.

**F8. What a kept file gives, and what it does not.** `bean import nhanes <file>` writes a `reference.file` record, and the file is then in the log. Tested on 0.7.1:

- `bean purge` of that record erases the file, also when a saved comparison is about the record. `bean import` then refuses the same file for good on that copy.
- `bean append` writes a `reference.file` record with any SHA-256 and any title, and `bean import -yes` asks nothing. An assistant with a shell holds the person's key. So such a record does not prove that a person chose anything.
- `bean share -files` carries a kept file only when the person names its record. A saved comparison with a pack does not bring the pack along.
- With `-files`, a share treats every `reference.file` of the person as public, by its type alone.

## Rules that stay

- A signed record means the same for ever. A check judges a record by what the record names, never by a list or a text that can change (DECISIONS 319).
- `pack/0`, method version 1 and its claims stay as they are.
- A reader refuses what it does not know. It never reads a table by a guess.
- bean fetches nothing, and never runs a program that a pack or a recipe names.
- The table comes to the value. Nothing here sends a reading anywhere.
- No output says whether a value is healthy.
- What a pack states of itself is shown as the pack's word.

## Decision 1: no new format now

`pack/0` stays the one format, and the next packs are `pack/0` packs. The gate of ROADMAP, path 4, stands: the format does not change before a real case shows that it must.

Two drafts of a `pack/1` were written and attacked. This is what became of each change.

| Change that was drafted | What the attack showed | Now |
|---|---|---|
| A value may be zero | The tests with many zeros are coarse counts, and a reading can be finer than their step (F1). | Those counts wait. The white cell shares can be built in `pack/0`, but for eosinophils. |
| A member for a detection limit | One member was wrong on bilirubin, where the fill value is also a measured value, and on a source with two limits. A right rule needs two members and a way to say "no number". | Decision 2 builds 49 of the 56 cells in `pack/0`, with no change of format. |
| The bytes must be canonical | It was forced only by zero. Without zero, no reader disagrees with another (F3). | A rule for the builder, as today. The rule for a reader was written and tested, and is kept below. |
| The source's terms and the builder's address, as members | An address in the bytes goes stale and cannot be mended. | A publisher states both beside the pack (decision 3). |
| A claim without the caveat that bean has no record of the pack | Right, by DECISIONS 319. But it is a new method version for a change of words alone. | Kept for the next method version, below. |

**What a next format will hold, when a real case starts it.** This is written down so that the work is not done twice.

- Canonical bytes, by the rule that was tested. Parse by RFC 8259 only. Write in the form of RFC 8785 with one line feed, and compare bytes, not decoded text. `cells`, `source.files` and each `same` list are in one stated order: by UTF-8 bytes, field by field, each element above the one before. Three writers, in Go, Python and JavaScript, gave the same bytes for about 150,000 numbers.
- Zero as a value. A reading of 0 gets a number only where the cell holds a 0.
- A detection limit as two members of a cell: the limit, and the weight of the people below it.
- The step in which the source records a value.
- Before a freeze: vectors for every rule, and two readers that share no code and agree on every vector. Also one real pack, built by two programs that share no code.

**What the next version of the method and its claim will hold.**

- No caveat that says what the signer's bean knew of the pack.
- No "you": a claim is read by others. "The signer chose the cohort."
- Every number that a caveat depends on, such as the 100 of a small cohort, is fixed by the method version.
- A check that holds the pack refuses a claim whose method version does not go with the pack's format.

**What would start a new format.** Any one of these.

- A second source, from another publisher, that `pack/0` cannot hold. A reviewer in that seat named two such cases: a test where 15% of people are below a limit, and a count in steps.
- A table that is not weights. In `pack/0` a cell of another kind is a member that the format does not have, so it is a new format.
- A test that needs zero and that people have, which eosinophils as a share may be.
- A reading that is finer than the source's step, in a test that people have.

**One thing must not wait: the words for a format that a reader does not know.** Today bean says `it is not a pack of version 0`. A reader that ships before a newer format exists must already say the right thing. So bean, the Python check and the check page say this: the pack is in a format that this bean does not read. A newer bean may read it.

## Decision 2: a cell is built only when the source measured every person in it

ALT, GGT, total bilirubin and C-reactive protein wait today because the survey's file has a mark for a value below the limit of detection. The old rule looked at the file. The new rule looks at the cell.

1. A cohort that holds a marked person gets no table for that test. In a pack that cell is left out, as a cell of fewer than 30 people is. In a comparison with the survey's files, bean gives no number for it.
2. Nobody is left out of a cell by their value, and no fill value is written as a value.
3. So every number that bean gives is exact, as it is today. By F2, this builds 49 of the 56 cells of the four tests. That is every cohort for ALT, 13 of 14 for GGT and for bilirubin, and 8 for C-reactive protein.
4. For a cell that is not built, bean says why, in words that are about the source: `no table for this test in this cohort: the source could not measure every person in it`.

Two other ways were weighed, with numbers from the files.

- **Leave the marked people out and say so.** Every reading above the limit then lies too low, by up to the share that was left out. The median of one cohort for C-reactive protein moved from 2.01 to 2.17. A stranger cannot see from the pack who was left out.
- **Write the source's fill value.** It is exact for every reading at or above the limit, and wrong only below it. But the pack then holds a value that nobody measured, in a file that is named for ever.

The way that was chosen gives up 7 cells of 56 and nothing else. A later version of the rule can take the fill value. A pack that held a made-up value could not be taken back.

This is a new version of the builder's rule, with new packs. The packs on record stay on record.

## Decision 3: how anyone checks where a pack came from

**What a pack states of itself** is the pack's own word. That is its source, the source's files with their SHA-256, and the name and version of the builder's rule. A check of the format cannot tell a true digest from 64 zeros. bean shows all of it as "the pack says".

**A recipe** says: these files, with this program, give this pack. Anyone runs it and compares one SHA-256. It is a document, not a format, and nothing in bean reads one. It holds:

- the SHA-256 of the pack;
- each source file: its name, SHA-256, size and address;
- for a Go program: the `go.mod` and the `go.sum` as files, the version of Go, and the command of F4. Then a second build with no `go.sum`, which makes the tool ask the public log. The lines that it writes must be the stated ones. Without that second build, the lines are the publisher's word alone (F5);
- for a script: the SHA-256 and address of each of its files, and the version and hash of the interpreter and of each library. It stops there: the operating system is not pinned, and the recipe says so;
- the exact command, and where the source states its terms of use.

A recipe is found by the SHA-256 of its pack, wherever the pack is stated. The recipes of the packs on record go into REFERENCE.md, with a form that a publisher can copy.

A recipe is not in the pack. One pack can have several. A recipe can be written after the pack, by someone who did not build it. And a pack that named its own program and commit would change its bytes with every commit.

**What a run proves.** That this source, built with a tool that the runner trusts, makes this pack of those files. It proves the same bits, not the right rule. A program that ignores its input and prints stored bytes passes too. So the program's source must be open to read, in the copy that the build fetched and not on a page. Two recipes by two authors who share no code say more than one. The two that exist share an author (F6).

**Running a recipe is running someone's program.** A person does it in a machine that holds no log and no key. bean never runs one.

**A pack on record is built again by its recipe, and a recipe names a release.** The builder in the code builds only the newest version of its rule. So an old pack on record is built again with the release that built it. The job on the code host runs each recipe, and both builders stay small.

**Rules for a builder, added to those of REFERENCE.md.**

- A rule states everything that the bytes depend on. That is the variable, the weight, who is left out, how a number is read and rounded, and the order of each sum. A reviewer summed one survey's weights in three orders and got three packs.
- When the source records a value in steps, the measure's caveats say the step. A `same` entry is for a unit in which real reports print the value in that step, or in a coarser one. A reading on the source's own steps gets an exact place, and bean shows the share with the same value.
- A reader converts no unit. A publisher who wants readings in a second unit builds a second measure in that unit. The rule states the factor, where it comes from, and how each value is rounded.
- A source may ask that no value stands for one person. A builder may then group values, and says so in a caveat.

## Decision 4: the names

**The `same` list stays.** The match is exact, byte for byte: `mg/dL` is not `mg/dl`. No reader converts a unit.

**The rule for a type's name is published as the importer has it, with vectors.** Map each character to lower case. Keep `a` to `z` and `0` to `9`. Write every run of other characters as one `-`, and drop a `-` at either end. Put `lab.` in front. Two characters outside ASCII become ASCII letters by the first step: the Kelvin sign becomes `k`, and the dotted capital I becomes `i`. The published rule says so, and the importer does not change: a type is part of what makes an import add nothing the second time. The rule is narrow: `Hämoglobin` gives `lab.h-moglobin`, and a name with no ASCII letter gives no type.

**The published packs are the list of names.** The project keeps no file of the names that exist. Each pack lists its pairs in its `same` lists, in the open, and `bean compare -pack <file>` prints them. A new publisher takes over the pairs of a pack for the quantities that are the same. They add the ones that they know from real reports. No check prints anything about a pair that some list lacks. So nobody asks to join anything.

**A reading that no pack takes.** A person is not stuck, and the documents will say so. A pack is a plain file. A person can copy one, add their lab's type and unit to a `same` list, and use the copy. The copy has another SHA-256. It is the person's own pack, by the person's own word, and bean has no record of it. A publisher can add the pair too, in a new pack. A record with which a person says that two names are one was drafted and cut. It adds a claim that points into the signer's own log. Its rule, "never convert a number", is one that no check can hold. The line that refuses a reading says what is true:

```
not comparable: this pack takes lab.platelets only in x10E3/uL, and the reading is in K/uL.
bean converts nothing, and cannot tell whether the two are one unit. The reading is not at fault.
```

**Not decided.** Codes of a public system for lab tests and units. The terms of use are not confirmed, and no lab report that bean reads prints a code.

## Decision 5: keeping a pack is holding its bytes

**The record is decided, and it is built with what uses it.** A pack is a public data file that a person keeps to compare with, and bean has a record for that: `reference.file`. `bean import pack <file>` will write one:

| Member | Value |
|---|---|
| `value.blob` | The pack's file, kept in the log like every kept file. |
| `context.sha256`, `context.bytes` | The SHA-256 and the size of the pack's bytes. |
| `context.assurance` | `unsigned-file`: this record does not prove who made the file. |
| `context.pack` | `format` and `title`, as the pack states them. |
| `source.system` | `pack`. Never `nhanes`: a 0.7.1 bean then passes the record by. |

A `reference.file` keeps a pack if and only if it has `context.pack`. The bytes are the authority. bean uses a kept pack only when all of this holds. The person's own key signed the record, and the record is current. The file has the SHA-256 and the size that the record states, and it reads as a pack. When two records keep one pack, either serves. A record that states what its file is not is false, and bean says so.

**What keeping gives.** The pack is in the person's log, export and backup. bean finds it by the SHA-256 that a saved comparison names. So the person can compute a comparison again from their own files when every host is gone. A share can carry the pack to a reader. Until this is built, a person keeps the pack's file beside the log, as REFERENCE.md says today.

**What keeping does not give.**

- It is not a choice that anyone can check (F8). So it gives no right. The flag, the words of a result and the caveat of a saved claim do not change when a pack is kept.
- A check that computes a comparison again never passes for a pack that bean has no record of, whatever is kept. It says that the arithmetic holds, and that the pack is not on record (DECISIONS 284).
- It is not "the sources that I use". Holding a file and choosing a source are two things. The second is the person's own list of packs, which DECISIONS 308 scoped and held back.
- A saved claim does not point to the record that keeps the pack. It names the pack by SHA-256, as today.

**Rules for the day it is built.**

- A file travels in a share as a public pack only when its bytes read as a pack and have the stated SHA-256. The type of its record is not enough.
- `bean purge` of a kept pack says what it did: the file is erased, and how many saved comparisons name the pack. It asks nothing, as today.
- A pack that was purged can be kept again. The refusal of a deleted file exists for a person's own data. A pack is public data.

## Decision 6: the list of packs on record stays code, for now

A draft moved the list to a data file with recipes. The attack showed that this is not smaller. It needs a tool that writes the file and a rule for a file that anyone can swap. A swapped file would change how saved comparisons read, which DECISIONS 319 forbids. The table in code is typed once and held by a test.

So the list stays as it is. What changes is what is said about it.

- **What "on record" means, in exact words.** The project built the pack from the source's own files, with two programs that share no code and have one author. A job on the code host builds it again from the source's address: a second network, and not a second person. Anyone can run the same recipe. It does not mean that the tables are right.
- **The cost, said plainly.** A pack gets on record only with a release of bean. A person with an earlier bean uses a newer pack with the flag, and their claims say that bean had no record of it. A later bean reads those claims rightly (DECISIONS 319). So the project's next pack is put on record in the release that follows its build.
- **The lines that bean prints for any pack are calm and of one kind.** "Whoever built it chose its tables" is as true of a pack on record as of any other. Drafts, for phase 3 to settle:

```
on record: The bean project built this pack from the source's files, and builds it again.
           That shows that its bytes are the ones named. It does not show that its tables are right.
any other: bean has no recipe for this pack. That is not a finding about it.
           Compare its SHA-256 with the one that its publisher states, or build it again from the files that it names.
```

The list is looked at again when a second publisher exists (ROADMAP, path 4).

## Decision 7: no publisher is special, and where bean falls short

The rule goes where a publisher reads: REFERENCE.md, "How a pack travels".

1. bean's code and its records name no publisher of a pack and no host of a pack. No record has a member for an organisation.
2. bean makes no call to a network.
3. A file and its SHA-256 on a page is a full publication. A recipe, a signature, a file host that serves by SHA-256 and a relay are additions.
4. A recipe is checked by a run, not by who wrote it.
5. There is no list of names that a publisher must be on.
6. Every pack is checked with the same tools, and every line that bean prints about a pack is of one kind.

Where bean falls short of the rule, said in the same place.

- The packs on record are used with no flag, and every other pack needs one. Each saved claim with another pack says that bean has no record of it and that nobody has checked its tables. Those words are frozen in method version 1, and a publisher from outside reads them as a mark against the pack. Nobody but the project can put a pack on record. This stays until the person's own list is built (DECISIONS 308).
- Only the project's recipes are run by a job. Anyone can run any recipe, but nobody else's is run for them.
- Only the project can change the format and the method. A cell of another kind, a new format and a new method version each need the project.
- The readers of lab reports, the rows of the survey's tests and the naming rule are the project's.
- The check page is built and published by the project's releases, and the recipe names the project's module.
- The documents name the code host as the place of the check page, and the survey's agency as the source of its files. Neither is a publisher or a host of a pack. A source is always named: it is where the data comes from.

## The order of work that follows

1. **The documents.** REFERENCE.md gets the recipes of the packs on record, with a form to copy, and the rules for a builder. It gets the naming rule with vectors, the exact meaning of "on record", and decision 7. It also says that `builder.name` names a rule and not a program.
2. **The words for a format that a reader does not know**, in bean, the Python check and the check page.
3. **`bean compare -accept-pack <SHA-256>`.** A person names the pack that they expect, and bean refuses other bytes. `tools/compare/reproduce.py` has this already. `-unknown-pack` stays, and takes any file. The words and the caveat of a claim are the same for both.
4. **The next pack**, in `pack/0`, by a new version of the builder's rule. It holds the four tests of decision 2, the fasting tests, the white cell shares, and each `same` entry that a real report shows. Each test is judged by DECISIONS 240 and 309 and by the rules above. It goes on record in the same release, with a recipe that names that release. The file of C-reactive protein, `HSCRP_J`, gets a digest on record first.
5. **Keeping a pack, with its three uses:** compare by SHA-256, a share that carries the pack, and a check that computes a shared comparison again.
6. **A first publisher hosts** (ROADMAP, phase 2). This needs nothing from bean and can start at any time. The owner gave the word for the survey's packs (DECISIONS 314).

## What breaks, and what happens then

| Event | Answer |
|---|---|
| A 0.7.1 bean meets a `reference.file` record that keeps a pack | It shows a kept file with its media type. Its comparison with the survey's files passes the record by. |
| A 0.7.1 bean is given the next `pack/0` pack | It reads it as a pack that it has no record of: the flag, and the caveat in each claim. A later bean with the pack on record reads those claims rightly. |
| The source takes a file away | The pack still computes. The recipe can no longer be run from the source. Whoever kept the files can hand them on, and the SHA-256 proves them. |
| A builder's rule has a fault | A new version of the rule, and a new pack. The old pack stays what it was. |
| The public log of Go modules is gone | The recipe holds the `go.sum`, and a copy of the modules that matches it is enough. The second build has then nothing to ask. |
| A person purges a kept pack | The file is erased, bean says so, and the person can keep the pack again. |

## Known weak points

- Seven cells of the four tests wait, six of them for C-reactive protein.
- Eosinophils as a share wait for zero, for 10 people in 7,523.
- A pack that is not canonical is still accepted (F3).
- Saved claims of version 1 say "you", and keep the caveat that says what the signer's bean knew.
- The person's own list waits, so the shortfalls of decision 7 stay.
- A person with a reading that no pack takes must edit a copy of a pack. That is a step for a patient person, or for their assistant.
- A pack gets on record only with a release.
- The list comes from one project, and both builders have one author.
- A share with `-files` treats a `reference.file` as public by its type alone. That holds today for the survey's files, and is mended with decision 5.
