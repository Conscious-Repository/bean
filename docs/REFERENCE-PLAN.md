# Reference data: what is decided before more is built

This is for anyone who builds on bean's reference data: a publisher of a pack, or the writer of a tool. [REFERENCE.md](REFERENCE.md) is the format that exists. This document says what changes next, what does not, and why. Nothing in it is built yet.

It is phase 1 of the order in [ROADMAP.md](ROADMAP.md). That phase decides on paper what is in logs and shared files for good once it is used. Six drafts were written, and reviewers who had not seen the work judged each, in six laps. They sat in five seats: Linus, Satoshi, buildtall, a publisher from outside, and the writer of a second reader. DECISIONS 321 has the account.

**The main result is a refusal.** A format, a signed claim and a record type are costs that every reader carries for ever. So nothing new is frozen without a real case. Each thing that waits is written down with the case that would start it.

**What this plan does not give.** The first user from outside asked to choose the sources that his results are compared with (DECISIONS 308). This plan gives him more tests, and a way to hold a pack. It does not give him that choice. The section on the list of packs says why, and what would start it.

**One fault in what is shipped was found on the way.** With `-files`, a share can carry a kept file as a public file when it is not one (F8). It is mended in 0.7.2 (DECISIONS 322): a file is public only when bean has its SHA-256 on record.

## What was measured

The public files of the survey are NHANES 2017-2018. Each fact here was measured by the author and again by one reviewer or more.

**F1. Zero is a real value, in tests that are coarse.** In the file `CBC_J`, 3802 of 7523 people have a basophil count of 0, and 427 have an eosinophil count of 0. `pack/0` cannot hold a value of 0. The survey records these counts in steps of 0.1, and a cell has about six distinct values. Among all adults, a reading of 0 lies at percentile 23 and a reading of 0.1 at 73. A reading of 0.12, which is finer than the step, lies at 99: that number means nothing. As a share of all white cells, basophils have no zero, and eosinophils have 10 in 7523.

**F2. A detection limit is real, and it touches few people.** The survey marks a value below its limit of detection, and writes a fill value in its place. Among the adults of the pack's menu who have a value, with the last column from the whole file:

| Test | Adults marked | Cohorts of 14 with a marked person | Largest share of a cohort's weight | Fill value, and lowest value in the file with no mark |
|---|---|---|---|---|
| ALT | 0 of 4915 | 0 | 0 | 2, and 3 |
| GGT | 1 of 4915 | 1 | 0.37% | 2, and 3 |
| Total bilirubin | 1 of 4916 | 1 | 0.58% | 0.1, and 0.1 |
| C-reactive protein | 10 of 4909 | 6 | 0.82% | 0.11, and 0.15 |

These four tests are on almost every real report, and have no reference data today (DECISIONS 240, 309). About one adult in nine has no value for them at all, and is left out, as for every test. A trap for a second builder: some tools read the zero of these files as 5.4e-79. A test for a mark of 0 then finds nothing.

**F3. A pack that is not in canonical form passes both checks.** Its SHA-256 differs from that of the same content in canonical form. Twenty-five such files were tried, and bean and the Python check gave the same verdict on each. The worst case is another pack, which bean has no record of. Nobody is misled.

**F4. A recipe works today, with tools that exist.** In an empty folder: a `go.mod` that requires `github.com/Conscious-Repository/bean v0.7.1`, and a `go.sum` with the lines of the build. Then, with `GOFLAGS=-mod=readonly` and `GOTOOLCHAIN=local`: `go build github.com/Conscious-Repository/bean/cmd/bean-pack-nhanes`. The program that came out wrote both packs on record from the survey's files, byte for byte. With one stated line changed, the tool stopped with `SECURITY ERROR`. With one line missing, it stopped too. Go 1.24.7 built it, not the 1.27.3 that the repository names, and the bytes were the same.

**F5. What that build checks, and what it does not.** With stated lines the `go` tool does not ask the public log of Go modules. So the lines are the publisher's word, until a build without them asks the log. Such a build prints no word about the log when the log agrees. It writes the same lines when it is told not to ask. So a recipe must set the tool to ask, and must show one run that fails. Two builds of one source gave two binaries that differ: a recipe pins the source, not a binary.

**F6. A second program builds the same bytes.** `tools/pack/rebuild_nhanes.py` is in Python and shares no code with bean. To write the same pack it writes the same `builder.name` and `builder.version`. So those members name a rule, not the program that ran. Both programs have one author.

**F7. The names are a rule, not a list.** `bean-import-lab` makes a type from the test name that the lab prints, and `cmd/bean/vocabulary.md` gives the same rule to a person's assistant. The unit is the unit as printed. A type holds ASCII only: the core refuses `lab.гемоглобин`. A reading of `lab.platelets` in `K/uL` gets this today: `not comparable: the reading is in "K/uL", and bean compares this type only in x10E3/uL`.

**F8. What a kept file gives, and what it does not.** `bean import nhanes <file>` writes a `reference.file` record, and the file is then in the log. Tested on 0.7.1:

- `bean purge` of that record erases the file, also when a saved comparison is about the record. `bean import` then refuses the same file for good on that copy.
- `bean append` writes a `reference.file` record with any SHA-256 and any title, and `bean import -yes` asks nothing. An assistant with a shell holds the person's key. So such a record does not prove that a person chose anything.
- A saved comparison with a pack does not bring the pack along in a share: the claim names no record that keeps it.
- With `-files`, a share carries every `reference.file` of the person that a shared record is about, as a public file. It decides that by the record's type alone, and a record of that type can name any file. A reviewer made a private note travel that way, and nobody had named its record. This was a fault in what was shipped, up to 0.7.1.

**F9. A pack that a person changes computes as well as any other.** A reviewer added `lab.cholesterol-total` in `mmol/L` to the `same` list of a measure in `mg/dL`, in a copy of the pack on record. bean then signed a reading of 5.2 mmol/L as percentile 0. The only mark was that bean has no record of the pack.

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
| A value may be zero | The tests with many zeros are coarse counts, and a reading can be finer than their step (F1). | Those counts wait. Each white cell share is judged by the rule on steps of decision 3. |
| A member for a detection limit | One member was wrong on bilirubin, where the fill value is also a measured value, and on a source with two limits. A right rule needs two members and a way to say "no number". | Decision 2 serves the four tests in `pack/0`, with the source's own values and a caveat. |
| The bytes must be canonical | It was forced only by zero. Without zero, no reader disagrees with another (F3). | A rule for the builder, as today. The rule for a reader was written and tested, and is kept below. |
| The source's terms and the builder's address, as members | An address in the bytes goes stale and cannot be mended. | A publisher states both beside the pack (decision 3). |
| A claim without the caveat that bean has no record of the pack | Right, by DECISIONS 319. But it is a new method version for a change of words alone. | Kept for the next method version, below. |

**What a next format could hold.** This is a list of what was worked out, so that the work is not done twice. None of it is decided.

- Canonical bytes, by the rule that was tested. Parse by RFC 8259 only. Write in the form of RFC 8785 with one line feed, and compare bytes, not decoded text. `cells`, `source.files` and each `same` list are in one stated order: by UTF-8 bytes, field by field, each element above the one before. Three writers, in Go, Python and JavaScript, gave the same bytes for about 150,000 numbers.
- Zero as a value. A reading of 0 gets a number only where the cell holds a 0.
- A detection limit as two members of a cell: the limit, and the weight of the people below it.
- The step in which the source records a value.
- Before a freeze: vectors for every rule, and two readers that share no code and agree on every vector. Also one real pack, built by two programs that share no code.

**What the next version of the method and its claim could hold.**

- No caveat that says what the signer's bean knew of the pack.
- No "you": a claim is read by others. "The signer chose the cohort."
- Every number that a caveat depends on, such as the 100 of a small cohort, is fixed by the method version.

A new method version costs each reader a second grammar of claims. So it comes with a new format, or with the person's own list, and not for words alone. Until then each saved claim holds the words of version 1.

**What would start a new format.** Any one of these.

- A second source, from another publisher, that `pack/0` cannot hold. A reviewer in that seat named two such cases: a test where 15% of people are below a limit, and a count in steps.
- A table that is not weights. In `pack/0` a cell of another kind is a member that the format does not have, so it is a new format.
- A test that needs zero and that people have, which the share of eosinophils may be.
- A reading that is finer than the source's step, in a test that people have.

**One thing must not wait: the words for a format that a reader does not know.** Today bean says `it is not a pack of version 0`. A reader that ships before a newer format exists must already say the right thing. So bean, the Python check and the check page say this: the pack is in a format that this bean does not read. A newer bean may read it.

## Decision 2: the four tests with a detection limit

ALT, GGT, total bilirubin and C-reactive protein wait today because the survey's file has a mark for a value below the limit (DECISIONS 240). Three ways to serve them were weighed, each with the numbers of F2. No way needs a new format. The reviews did not agree. The owner was asked, and left it to common sense and the project's values. **Way A is decided.**

| Way | What a person gets | What it costs |
|---|---|---|
| **A. The source's own value.** The table holds what the survey's file holds: the fill value for a marked person. The measure's caveat says so, and that caveat is in every signed claim. | All 56 cells. For ALT, GGT and C-reactive protein the fill value lies below every measured value. So each reading at or above the limit gets its exact place. | A number that nobody measured is in the pack, as the source wrote it. A reading below the limit gets a place that can be wrong by up to 0.82 of a point. For bilirubin the fill value, 0.1, is also a measured value. So a reading of exactly 0.1 can be wrong by up to 0.29 of a point, and one below 0.1 by up to 0.58. |
| **B. No table for a cohort with a marked person.** | 48 of the 56 cells, each exact: all for ALT, 13 of 14 for GGT and for bilirubin, 8 of 14 for C-reactive protein. | Eight cohorts get no number for one test, for one or two people each. A pack cannot say why a cell is missing. A cohort that a person narrows can have a table while the whole cohort has none. |
| **C. The tests wait** for a format that holds a limit. | Nothing, as today. | Four tests that are on almost every report stay without reference data. |

A fourth way was tried and cut: leave the marked people out of the cell. Every reading above the limit then lies too low, and the median of one cohort moved from 2.01 to 2.17.

**Why A.** Three reviews measured it and said so. The table is then what the source's file holds, with nothing left out by the builder's choice. So anyone can check each cell against the public file. Its error is confined to a reading below the limit and, for bilirubin, a reading of exactly 0.1. The error is bounded, and the signed claim itself states it. Way B gives a person in eight cohorts nothing, for one or two people each. The caveat of such a measure names the test:

```
C-reactive protein: NHANES could not measure a value below 0.15 mg/L, and writes 0.11 for such a person. Nobody measured that value. Such people are under 1% of a cohort's weight. A result for a reading below 0.15 is not exact.
```

**What way A is not.** For a reading below the limit it puts a bounded estimate in the place of a refusal. A person cannot switch that off. They see the caveat with the result, and it is in the claim that they sign.

**The rules of way A.**

1. The limit is the source's word, from its own document. The builder's table states it and where it is from. A test holds that no value without a mark lies below it. The source's page is not pinned by a digest, and the documents say so.
2. The caveat says "below" the limit when the fill value lies below every value without a mark. When the fill value is also a measured value, as for bilirubin, it says "at or below" that value.
3. The caveat says "under 1% of a cohort's weight", in a pack and in a comparison with the files. The cap makes that true, and no reader computes a share to check the words. A measure with no marked person in the pack has no such caveat: that is ALT.
4. A builder takes way A only where the marked people are under 1% of the weight in every cohort of the pack. Above that, the test waits for a format that holds a limit. A pack has no member for that share, so a second builder computes it again from the files. The line of 1% was drawn after the figures were seen: the pack's cohorts reach 0.82%, and children reach 4.5%.
5. The same rule holds in a comparison with the survey's files, where a person makes the cohort. bean computes the share for that cohort. Under 1% it gives the number with the same caveat, and at 1% or more it gives no number and says why. Among children up to 5 years the share for C-reactive protein is 4.5%.
6. bean shows a measure's caveats with that measure, not in one list under the table.
7. Each test must also pass the rules of DECISIONS 309. The source's table of the difference between its two analyzers has no row yet for three of the four. So this decision serves up to four tests. A stranger checks rules 1 and 7 against the source's own pages, and rule 6 on a running bean.

This is a new version of the builder's rule, with new packs. The packs on record stay on record.

## Decision 3: how anyone checks where a pack came from

**What a pack states of itself** is the pack's own word. That is its source, the source's files with their SHA-256, and the name and version of the builder's rule. A check of the format cannot tell a true digest from 64 zeros. bean shows all of it as "the pack says".

**A recipe** says: these files, with this program, give this pack. Anyone runs it and compares one SHA-256. It is a document, not a format, and nothing in bean reads one. It holds:

- the SHA-256 of the pack;
- each source file: its name, SHA-256, size and address;
- for a Go program: the `go.mod` and the `go.sum` as files, the version of Go, and the command of F4;
- then a second build that asks the public log: an empty module cache, no `go.sum`, and the tool set to ask. That is `GOSUMDB=sum.golang.org`, no `GONOSUMDB`, no `GOPRIVATE`, and `GOFLAGS=-mod=mod`. The tool stops when the log holds another hash. The recipe also has runs that must fail, so that a runner sees each check work (F5). One serves a copy of the module with a changed byte from a folder, and the tool must refuse it with the log's answer. One changes a source file, and the pack must change;
- for a script: the SHA-256 and address of each of its files, and the version and hash of the interpreter and of each library. It stops there: the operating system is not pinned, and the recipe says so. A publisher runs it on a second machine before they state it;
- the exact command, the version of the builder's rule, and where the source states its terms of use.

A recipe is found by the SHA-256 of its pack, wherever the pack is stated. The recipes of the packs on record go into REFERENCE.md, with a form that a publisher can copy.

A recipe is not in the pack. One pack can have several. A recipe can be written after the pack, by someone who did not build it. And a pack that named its own program and commit would change its bytes with every commit.

**What a run proves.** That this source, built with a tool that the runner trusts, makes this pack of those files. It proves the same bits, not the right rule. A program that ignores its input and prints stored bytes passes too. So the program's source must be open to read, in the copy that the build fetched and not on a page. Two recipes by two authors who share no code say more than one. The two that exist share an author (F6). The public log has one operator, and a first run cannot tell whether that log shows everyone the same.

**Running a recipe is running someone's program.** A person does it in a machine that holds no log and no key. bean never runs one.

**The builders keep every version of the rule.** A row of the builder's table says from which version of the rule it exists, and a flag names the version to build. So the newest code builds every pack on record, in Go and in Python. The builder builds the newest version when no flag is given, from the release that puts the newest pack on record. It prints the version that it built and the flag for it, and each recipe names the flag. REFERENCE.md changes its first run and its digests in that release. The tests on the real files, `make pack` and the job are changed to build each version. A recipe names a release as well, for a stranger who wants a fixed program.

**Rules for a builder, added to those of REFERENCE.md.** They hold from version 2 of the survey's rule.

- A rule states everything that the bytes depend on. That is the variable, the weight, who is left out, how a number is read and rounded, and the order of each sum. A reviewer summed one survey's weights in three orders and got three packs.
- A pack is written in the canonical form of RFC 8785, and REFERENCE.md will state that form in full. Two builders agree only when they write numbers the same way.
- A measure with no cell is left out of the pack.
- When the source records a value in steps, the measure's caveats say the step. A `same` entry is for a unit in which real reports print the value in that same step, neither finer nor coarser. A reading on the source's own steps gets an exact place, and bean shows the share with the same value. One real report prints a white cell share as a whole percent, and the survey records it in steps of 0.1. A basophil share of "1 %" then lies at percentile 78, and the truth is between 12 and 98. How coarse a report may be is a rule with a number, as the rule on analyzers is (DECISIONS 309). It is set when that pack is built.
- A reader converts no unit. A publisher who wants readings in a second unit builds a second measure in that unit. The rule states the factor, where it comes from, and how each value is rounded.
- A source may ask that no value stands for one person. A builder may then group values, and says so in a caveat.
- A pack that is made from another pack is a new pack. It has its own title and its own builder's name, and a caveat says what was changed. Then nobody takes it for the first publisher's.

## Decision 4: the names

**The `same` list stays.** The match is exact, byte for byte: `mg/dL` is not `mg/dl`. No reader converts a unit.

**The rule for a type's name is published as the importer has it, with vectors.** Map each character to lower case, by Unicode's simple mapping of one character to one. Keep `a` to `z` and `0` to `9`. Write every run of other characters as one `-`, and drop a `-` at either end. Put `lab.` in front. Two characters outside ASCII become ASCII letters by the first step: the Kelvin sign becomes `k`, and the dotted capital I becomes `i`. A language's own lower-case function can differ here, so the vectors hold both. The importer does not change: a type is part of what makes an import add nothing the second time. The rule is narrow: `Hämoglobin` gives `lab.h-moglobin`, and a name with no ASCII letter or digit gives no type. The published rule gives every limit of the importer.

**The published packs are the list of names.** The project keeps no file of the names that exist. Each pack lists its pairs in its `same` lists, in the open, and `bean compare -pack <file>` prints them. A new publisher takes over the pairs of a pack for the quantities that are the same. They add the ones that they know from real reports. No check prints anything about a pair that some list lacks. So nobody asks to join anything.

**A reading that no pack takes waits for a pack that takes it.** Only a pack can say that a type and unit is its measure. The project adds a spelling that a real report shows, in its next pack. That keeps the choice from the person, and this plan does not mend it.

- A record with which a person says that two names are one was drafted and cut. It adds a claim that points into the signer's own log. Its rule, "never convert a number", is one that no check can hold.
- A person can make a pack from another pack and add a pair (decision 3). bean then signs whatever the new pack says (F9). So the documents do not offer this as a way out. It is for someone who knows that two names are one unit on one scale, and it is never an assistant's choice.
- A claim made with such a pack says for good that bean has no record of the pack. A reader needs that pack to compute it again.

The line that refuses a reading says what is true, and what not to do:

```
not comparable: this pack takes lab.platelets only in x10E3/uL or Thousand/uL, and the reading is in K/uL.
bean converts nothing, and cannot tell whether the two are one unit. This is about the name of the unit, not about the value.
Do not type the reading again in another unit, and do not change a pack to make the two match: bean would then sign a result that nobody checked.
The pack's publisher can add the unit, in a new pack. For the survey's pack that is the bean project.
```

bean prints such words once, under the table, as it does today.

**Not decided.** Codes of a public system for lab tests and units. The terms of use are not confirmed, and no lab report that bean reads prints a code.

## Decision 5: keeping a pack is holding its bytes

**Why a person would do it:** so that the file that their saved comparisons depend on is not lost.

**The record is sketched, and it is built with what uses it.** It is final on the day it is built, and not before. A pack is a public data file that a person keeps to compare with, and bean has a record for that: `reference.file`. `bean import pack <file>` will write one:

| Member | Value |
|---|---|
| `value.blob` | The pack's file, kept in the log like every kept file. |
| `context.sha256`, `context.bytes` | The SHA-256 and the size of the pack's bytes. |
| `context.assurance` | `unsigned-file`: this record does not prove who made the file. |
| `context.pack` | `format` and `title`, as the pack states them. |
| `source.system` | `pack`. Never `nhanes`: a 0.7.1 bean then passes the record by. |

A `reference.file` keeps a pack if and only if it has `context.pack`. The bytes are the authority. bean uses a kept pack only when all of this holds. The person's own key signed the record, and the record is current. The file has the SHA-256 and the size that the record states, and it reads as a pack. When two records keep one pack, either serves. A record that states what its file is not is false, and bean says so.

**What keeping gives.** The pack is in the person's log, export and backup. bean finds it by the SHA-256 that a saved comparison names. So the person can compute a comparison again from their own files when every host is gone. In the log the file is encrypted, like every kept file, and an export writes it out as a plain file. Until this is built, a person keeps the pack's file beside the log, as REFERENCE.md says today.

**What keeping does not give.**

- It is not a choice that anyone can check (F8). So it gives no right. The flag, the words of a result and the caveat of a saved claim do not change when a pack is kept.
- A check that computes a comparison again says two things apart: whether the arithmetic holds, and whether bean has the pack on record. For a pack that is not on record it passes only when the reader names the pack's SHA-256, whatever is kept (DECISIONS 284).
- It is not "the sources that I use". Holding a file and choosing a source are two things (decision 6).
- A saved claim does not point to the record that keeps the pack. It names the pack by SHA-256, as today.

**Rules for the day it is built.**

- A pack travels in a share only with `-files`, and only when a shared comparison names its SHA-256 or the person names its record. Its bytes must read as a pack and have the stated SHA-256. The review before the share shows its title and its SHA-256, and does not call it a public file. The type of a record is never enough: the set of packs that a person keeps says what they look at.
- `bean purge` of a kept pack says what it did. The file is erased, and it says how many saved comparisons name the pack. It says that nobody can compute them again from this log unless a copy of the file exists. It says that this copy cannot keep the file again. It asks nothing, as today.
- Whether a pack that was purged can be kept again is decided when keeping is built. Today it cannot: the rule that a deleted record stays deleted has no exception (SPEC.md 7.8). A pack is public data, so an exception may be right, and it needs its own review.

## Decision 6: the list of packs on record stays code, and the person's own list waits

A draft moved the list to a data file with recipes. The attack showed that this is not smaller. It needs a tool that writes the file and a rule for a file that anyone can swap. A swapped file would change how saved comparisons read, which DECISIONS 319 forbids. The table in code is typed once and held by a test.

So the list stays as it is. What changes is what is said about it.

- **What "on record" means, in exact words.** The SHA-256 of the pack is in a list that is compiled into this bean. With it bean holds each label, each `same` entry and each caveat of the pack, and a check judges a claim against those. The project built the pack from the source's own files, with two programs that share no code and have one author. A job on the code host builds it again from the source's address, with the code of the main branch. That is a second network, and not a second person. Anyone can run the recipe. It does not mean that the tables are right.
- **The cost, said plainly.** A pack gets on record only with a release of bean. A person with an earlier bean uses a newer pack with the flag, and their claims say that bean had no record of it. A later bean reads those claims rightly (DECISIONS 319). So the project's next pack is on record in the release that first builds it.
- **A person cannot switch the list off.** An earlier order of work said that a person can see and drop any list that came with bean (DECISIONS 317). For `pack/0` that is not so: the list is in the build. It takes nothing from the person: it saves a flag for two packs, and any other pack is used with the flag.
- **The lines that bean prints for any pack are calm and of one kind.** "Whoever built it chose its tables" is as true of a pack on record as of any other. Drafts, for the build to settle:

```
on record: The SHA-256 of this pack is in the list inside this bean. The project put it there: that is the project's word.
           It says that the pack was built from the source's files. It does not say that its tables are right.
any other: This bean does not know this pack. That is not a finding about it.
           Compare its SHA-256 with the one that its publisher states, or build it again from the files that it names.
```

These lines are for the screen. Two texts keep the words that they have today. The refusal without the flag is changed when these lines are built. The caveat in a saved claim is frozen in method version 1.

**The person's own list of packs waits, and this is what would start it.** It is what the first user from outside asked for (DECISIONS 308): a signed record of the sources that a person uses. A peer can read it. Three audits said to wait, for one reason: one source exists, so there is nothing to choose between. The owner was asked on 2026-11-16, and said to wait. It is built when a pack exists from a second source, or when a peer asks to send their list. A pack that a person made from the project's pack does not count. Until then a person uses any pack with the flag, and nobody is locked in. But each claim made with such a pack keeps the stern words of method version 1. The owner is asked again with each release that puts a pack on record. One limit is known now: such a record cannot prove that the person chose, while an assistant holds the person's key (F8).

The list in code is looked at again when a second publisher exists (ROADMAP, path 4).

## Decision 7: no publisher is special, and where bean falls short

The rule goes where a publisher reads: REFERENCE.md, "How a pack travels".

1. bean's code and its records name no publisher of a pack and no host of a pack. No record has a member for an organisation.
2. bean makes no call to a network.
3. A file and its SHA-256 on a page is a full publication. A recipe, a signature, a file host that serves by SHA-256 and a relay are additions.
4. A recipe is checked by a run, not by who wrote it.
5. There is no list of names that a publisher must be on.
6. Every pack is checked with the same tools, and every line that bean prints about a pack is of one kind.

Where bean falls short of the rule, said in the same place.

- The packs on record are used with no flag, and every other pack needs one. Each saved claim with another pack says that bean has no record of it and that nobody has checked its tables. Those words are frozen in method version 1, and a publisher from outside reads them as a mark against the pack. Nobody but the project can put a pack on record, and no step leads there. This stays until the person's own list is built.
- Only the project's packs are built again by a job. Anyone can run any recipe, but nobody else's pack is built again for them. No job runs a recipe as a stranger would, through the module proxy and the public log.
- Only the project can change the format and the method.
- The readers of lab reports, the rows of the survey's tests, the digests of the survey's files and the naming rule are the project's. A new file of the survey needs the project.
- What bean holds of each pack on record is in the Go code and again in the Python tool. Someone who forks bean changes both.
- The check page is built and published by the project's releases, and the recipe names the project's module.
- Only a pack can make a reading match (decision 4).
- The documents name the code host as the place of the check page, and the survey's agency as the source of its files. Neither is a publisher or a host of a pack. A source is always named: it is where the data comes from.

## What each party learns from the new parts

- **Keeping a pack:** nobody learns anything. The file is in the person's log.
- **A share that carries a pack:** the reader learns which pack, which the comparison names already. A record that keeps a pack, when it is shared by itself, tells the reader one source of the sender.
- **Running a recipe:** the source's host, the code host, the module proxy and the public log each see a fetch of public files. None sees a reading.

## The order of work that follows

1. **Mend what is shipped: a share with `-files`. Done in 0.7.2 (DECISIONS 322).** A `reference.file` travels as a public file only when the bytes that it keeps have a SHA-256 that bean has on record. For a survey file that is the list of digests in bean's code. Any other kept file travels only when the person names its record (F8). A kept copy of a survey file that is not the one on record then needs its record named, and UPGRADING.md says so.
2. **The documents.** REFERENCE.md gets the recipes of the packs on record, with a form to copy, and the rules for a builder. It gets the canonical form in full, the naming rule with vectors, the exact meaning of "on record", and decision 7. It says that `builder.name` names a rule and not a program, and which version of Unicode its rule on texts means. It also gets the rules by which a test is judged (DECISIONS 240, 309), which today are only in the log of decisions.
3. **Words in bean and in the Python check.** A format that a reader does not know. And a cell that a pack does not have. Today bean says that a cohort of fewer than 30 people is left out, which is one reason of several. Release 0.8.2 gives no reason, and the format has no least n (DECISIONS 333).
4. **The builders take a version of the rule.** A flag in both builders, and a proof that version 1 still builds both packs on record, byte for byte. The tests on the real files, `make pack` and the job change with it.
5. **The next pack**, in `pack/0`, by version 2 of the rule. It holds the tests of decision 2 that pass, and each `same` entry that a real report shows. A white cell share is added only when a real report prints it in the survey's step, and none may pass. The file of C-reactive protein, `HSCRP_J`, gets a digest on record first. COMPARE.md and the tests no longer say that a test with a mark is left out. The pack is on record in the release that first builds it.
6. **The fasting tests**, after a decision of their own: they need a weight for each test, which the method does not have.
7. **Keeping a pack**, in three steps: compare by SHA-256, a share that carries the pack, and a check that computes a shared comparison again.
8. **A first publisher hosts** (ROADMAP, phase 2). This needs nothing from bean and can start at any time. The owner gave the word for the survey's packs (DECISIONS 314). bean prints the SHA-256 of each pack that it reads, and a person compares it with the one that the publisher states. A flag that refuses other bytes was drafted and cut: it does what `sha256sum` does. It is added when people ask.

## What breaks, and what happens then

| Event | Answer |
|---|---|
| A 0.7.1 bean meets a `reference.file` record that keeps a pack | It shows a kept file with its media type. Its comparison with the survey's files passes the record by. With `-files` it shares the pack as a public file, when the person names the record or a shared record is about it. |
| A 0.7.1 bean is given the next `pack/0` pack | It reads it as a pack that it has no record of: the flag, and the caveat in each claim. A later bean with the pack on record reads those claims rightly. |
| A 0.7.1 reader gets a claim that a newer bean made with the next pack | The newer bean has the pack on record, so the claim has no caveat about it. The 0.7.1 bean marks it `(NOT on record)`, which is true for that reader. The 0.7.1 Python tool refuses it: it expects that caveat. So both people update before they share such a claim, and UPGRADING.md will say so. |
| The source takes a file away | The pack still computes. The recipe can no longer be run from the source. Whoever kept the files can hand them on, and the SHA-256 proves them. |
| A builder's rule has a fault | A new version of the rule, and a new pack. The old pack stays what it was, and nothing marks it. A release can say in its lines that a pack on record was found wrong. Saved claims stay as they were signed. |
| The public log of Go modules is gone | The recipe holds the `go.sum`, and a copy of the modules that matches it is enough. The second build has then nothing to ask. |
| A person purges a kept pack | The file is erased, and bean says so. Today the pack cannot be kept again on that copy (decision 5). |

## Known weak points

- By decision 2 a pack holds, for 12 people, a value that the source wrote and nobody measured. A reading below the source's limit gets a place that is not exact, and a person cannot switch that off. The limit comes from a page of the source that no digest pins.
- The share of eosinophils waits for zero, for 10 people in 7,523. Shares that a report prints more coarsely than the survey may wait too.
- A pack that is not canonical is still accepted (F3).
- The rule on the texts of a pack names categories of Unicode. Two readers with tables of different years can disagree on a new character.
- Saved claims of version 1 say "you", and keep the caveat that says what the signer's bean knew.
- The person's own list waits, so the first user's wish and the shortfalls of decision 7 stay.
- A person with a reading that no pack takes waits for a pack.
- A pack gets on record only with a release, and nothing marks a pack on record that is found wrong.
- The list comes from one project, and both builders have one author. The digests of the survey's files come from one download and one job.
- Nothing keeps a copy of the survey's files or of a release for the day that their host is gone.
