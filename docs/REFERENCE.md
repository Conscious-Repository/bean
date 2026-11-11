# Reference packs: public data to compare a reading with

This is for anyone who publishes reference data, and for anyone who writes a tool that reads it. It says what a pack must hold so that bean, and anyone else, can compare a reading with it and check the result.

It is a draft at version 0. One source has been built into a pack: NHANES 2017-2018. The format changes when a second source shows that it must.

## The idea

- A pack is one file of public reference data. Its name is the SHA-256 of its bytes.
- The pack holds a table for each measure and each cohort on its menu. The table is all that the method needs.
- The table comes to the value. The person's reading never leaves their machine, and nobody learns which cohort they chose.
- Anyone can hold a pack and hand it on. A host is not trusted: the SHA-256 says whether the bytes are right.
- The pack names the files of its source by SHA-256. Anyone can build the pack again from them and compare.
- A comparison is the person's signed claim. It names the reading, the pack, the cell and the method. Anyone with the pack computes it again.

NHANES and cholesterol are one case. Nothing in the format knows a survey, a blood test or a unit.

## Who does what

| Who | What they do | What they learn about a person |
|---|---|---|
| The source | Publishes data about many people, such as a survey. | Nothing. |
| The builder | Builds a pack from the source's files, with a program that anyone can run. | Nothing. |
| A host | Serves the pack's bytes: a web server, a relay, a friend's disk. | That someone fetched the pack. |
| The person | Fetches the pack whole and compares on their own machine. | |
| A reader | Gets a shared comparison and computes it again from the pack. | What the person shared. |

No service computes a comparison for a person. A request for one cell says "a man of 30 to 39 with this test", and a request with the value says more. The request is itself a fact about someone's health. So a pack is fetched whole, and the arithmetic is done at home.

## What a comparison supports

A comparison says where a value lies among the values of a cohort in the source's sample. It does not say whether a value is healthy. It does not diagnose, predict or advise. Each result has its n and its caveats, and bean gives no number for a cohort of fewer than 30 people.

## Try it

```
bean-pack-nhanes -out nhanes.pack DEMO_J.xpt TCHOL_J.xpt BMX_J.xpt    # build a pack from files you downloaded
bean compare -pack nhanes.pack                                         # what the pack holds
bean compare -pack nhanes.pack -cohort male-30-39 -type 'lab.*'        # compare
python3 tools/pack/check.py nhanes.pack                                # the check, without bean
make pack                                                              # both, on packs good and wrong
```

[COMPARE.md](COMPARE.md) says where the NHANES files are. `bean-pack-nhanes` reads the files that you give it and goes nowhere.

## The file

A pack is one JSON object, in UTF-8. A builder writes it in bean's canonical form (SPEC.md 4.1) with one line feed after it, so the same pack is the same bytes. A reader does not need that form: the name is the SHA-256 of the bytes as they are.

| Member | Value |
|---|---|
| `bean` | `pack/0`. |
| `title` | What the pack is, in a few words. |
| `source.name`, `source.publisher`, `source.home` | The source, who publishes it, and its address. |
| `source.files` | One entry or more. Each has `name`, `sha256` (64 lower-case hex digits), `bytes` and `url`. These are the files that the pack was built from. |
| `builder.name`, `builder.version` | The program that built the pack. |
| `caveats` | Texts that qualify every comparison with this pack. It can be empty. |
| `measures` | An object. Each key is a measure's id. |
| `cohorts` | An object. Each key is a cohort's id. |
| `cells` | A list of one cell or more. |

A measure has these members:

| Member | Value |
|---|---|
| `label` | The source's own label of the quantity, such as `Total Cholesterol (mg/dL)`. |
| `unit` | The unit of every value in the measure's cells. |
| `caveats` | Texts that qualify every comparison with this measure. |
| `same` | A list. Each entry has `type` and `unit`. |

A `same` entry is a claim by the builder. It says that a reading of that type, in that unit, is this measure's quantity in this measure's unit. A reader compares a reading only when its type and its unit equal an entry exactly. No reader converts a unit. No type and unit may be in the `same` lists of two measures.

A cohort has `text`, such as `male, age 30-39`, and `caveats`. Its id is the name that a person gives to choose it.

Each object has the members that this document names and no other, each once. A number is a JSON number that an IEEE 754 double can hold. A pack is at most 64 MiB.

Each object has the members that this document names and no other. An id is 1 to 64 letters, digits, dots, `-` and `_`, and starts with a letter or a digit. A text is 1 to 400 bytes, with no control character and no space at either end.

## A cell

```
{"cohort":"male-30-39","kind":"weights","measure":"LBXTC","n":339,"values":[107,116,118.5],"weights":[51210.3,18830.9,40112.6]}
```

| Member | Value |
|---|---|
| `measure`, `cohort` | The ids of the measure and the cohort. A pair is there once. |
| `kind` | `weights`. Version 0 has no other kind. |
| `n` | The number of people. It is 30 or more. |
| `values` | The distinct values of the cohort, in rising order. Each is a number above zero. |
| `weights` | One number above zero for each value: the weight that the value carries. |

A weight is the sum of the sample weights of the people with that value. For a source with no sample weights, it is the count of those people.

A reader refuses a pack that has a cell of a kind it does not know. It never reads a table by a guess at what the table means.

## The method

The method's id is `weighted-percentile`, and this is version 1. Inputs: the value v of the reading, and one cell.

1. v must be a number above zero. Every measure of version 0 is a quantity above zero.
2. W is the sum of the weights, added in the cell's order. B is the sum of the weights of the values below v, in that order. E is the weight of the value v, or zero when the cell does not have it.
3. lower = 100 B / W, same = 100 E / W, percentile = 100 (B + E/2) / W. Round each to one decimal: floor(10 y + 0.5) / 10, with the multiplication and the addition as two steps.
4. The 25th, 50th and 75th percentile, for q = 0.25, 0.5 and 0.75. Each is the first value at which the weights, added in the cell's order, reach q W.
5. n is the cell's n.

Every sum is in IEEE 754 doubles, in the order given. So every implementation gets the same bits. A change to any step is a new version.

These are steps 6 to 8 of the method in [COMPARE.md](COMPARE.md). So a comparison with a pack gives the same numbers as a comparison with the files that the pack was built from.

## The claim

`bean compare -pack ... -save` signs one `comparison.percentile` record for each result. Its `context` holds only what a reader needs to compute it again:

| Member | Value |
|---|---|
| `reading` | The id of the reading. The id is a hash of the reading, so the claim pins its value. |
| `reference` | `pack` (the SHA-256), `title`, `source`, `measure`, `label`, `unit` and `cohort` (the id). |
| `cohort.text` | The cohort in words, as the pack gives it. |
| `method` | `id` and `version`. |
| `result` | `n`, `percentile`, `below`, `equal`, `p25`, `p50`, `p75`. |
| `caveats` | In this order: the pack's, the measure's, the cohort's, then the three below where they apply. |

The three caveats that a claim adds:

- `you chose the cohort: bean does not know whether you belong to it`, always;
- `the cohort is small: fewer than 100 people`, when n is below 100;
- `the reading was signed by an agent or another key, not by you`, when that is so.

A reader with the pack checks a claim in four steps. The pack has the SHA-256 that the claim names. The reading's type and unit are in the measure's `same` list. The texts and the caveats are the ones that the pack gives. The method gives every number of the result. `tools/compare/reproduce.py` does this, and shares no code with bean.

A saved comparison gives the reading's value away to anyone with the pack ([COMPARE.md](COMPARE.md)). It does not outlive its reading.

## Rules for a builder

- **The same files give the same bytes.** Write the object in canonical form. Sort the cells by measure, then by cohort. Add no time and no name of a machine.
- **Name every source file by its SHA-256.** A reader who fetches the files from the source can then build the pack again.
- **Leave out a cell of fewer than 30 people.** A reader refuses one.
- **Take each `same` entry from a real reading.** Use the type and the unit that a real importer or a real report writes. A second name or unit for a quantity is a second entry.
- **Say what does not carry over.** A caveat is part of the pack. Each comparison shows it, and each saved claim holds it.
- **Use the weights that the source says to use.** A wrong weight gives a number that looks right.

## What a check refuses

`make pack` tries each of these, and both checks refuse it:

- a member that the format does not have, a missing one, or one that is there twice;
- a cell of another kind;
- values that are not in rising order, or a value that is there twice;
- a value or a weight that is zero, negative or not a number;
- a list of weights that is shorter or longer than the list of values;
- an n below 30, or below the number of distinct values;
- a cell that names a measure or a cohort that the pack does not have, or a cell that is there twice;
- a type and unit in the `same` lists of two measures;
- a text with a control character, and an id with a space.

## How a pack travels

A pack is public data, and a file. It needs no account and no server of its own.

- **Any host will do.** A web server, a file sent by e-mail, a disk. A host that serves a file under its SHA-256 as the path fits a pack with no change. The Blossom servers of Nostr do that.
- **A pointer is not the data.** A publisher can announce a pack: its title, its SHA-256 and where it is. A signed note on Nostr relays can carry that. The relays then hold a pointer and no health record.
- **A host may charge for bytes.** Storage and bandwidth cost money, and a host can ask for payment before it serves a file. bean holds no wallet. A person pays a host, if they must, with their own tools.
- **Nobody is paid to compute.** A paid comparison would send the cohort or the value to the one who computes. That is the disclosure that a pack exists to prevent.
- **Who says that a pack is the right one.** Today: this repository states the SHA-256 of the NHANES pack, below. Anyone can build the pack and get the same SHA-256.

The pack that `bean-pack-nhanes` version 1 builds from the six files on record has this SHA-256. The files are `DEMO_J`, `TCHOL_J`, `HDL_J`, `GHB_J`, `CBC_J` and `BMX_J`.

```
efbb252a361b1994c67207618a088d492d1b0dafb8ea5ff88f857f97abc69277
```

`bean compare -pack` says whether a pack is this one. The digests of the six files come from one download ([COMPARE.md](COMPARE.md)). Until a second person has confirmed them, the pack is "the pack that one download gives".

## The NHANES pack

- **Measures.** The twelve blood tests of [COMPARE.md](COMPARE.md), and two body measures: weight in kg and standing height in cm. Each is measured in everyone examined.
- **Cohorts.** Each sex, by ten years of age from 20 to 79, and 80 and over: 14 cohorts, such as `female-40-49` and `male-80-over`.
- **Weights.** The examination weight, `WTMEC2YR`. Each cell is the table of steps 1 to 5 of the method in COMPARE.md.
- **Size.** About 570 kB, and 250 kB packed.
- **What it leaves out.** Children, and every cohort by ethnicity, body or food. For those, compare with the files: `bean compare -sex ... -where ...`.

## Other kinds of reference

Public reference data comes in more shapes than a table of weights. Version 0 defines none of these. Each needs a real source, a kind of cell and a method of its own before it is added.

| Shape of the source | What a cell would hold | What a result may say |
|---|---|---|
| Published percentile points and no data of each person | The points, such as the 5th to the 95th percentile | A range, such as "between the 25th and the 50th percentile". Never one number, and nothing beyond the outer points. |
| A fitted curve, such as the L, M and S of a growth chart | The parameters for each step of age | A score, only inside the ages and the population that the source states. |
| Counts of a category, such as how many people report a symptom | The count of each category, and the total | A share of the cohort. Never a risk for one person. |

## Not decided

- **A publisher's signature.** Today the SHA-256 is all. A publisher could sign a record that names a pack's SHA-256, with a bean identity, and send it as a bundle. That is built when a second publisher exists.
- **A pack from data that is not public.** Nobody can build such a pack again, so a reader has only the publisher's word. Version 0 requires an address for each source file.
- **Names for a measure that do not depend on bean's types.** A public code system for lab tests and for units could replace the `same` list. Its terms of use are not confirmed.
- **A pack in a share.** A reader gets the pack by its SHA-256 from any host. `bean share` does not carry one yet, and the check page does not compute a comparison again.
- **A measure that can be zero**, and an interval of uncertainty. Each is a new method version.
