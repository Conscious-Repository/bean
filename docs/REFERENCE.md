# Reference packs: public data to compare a reading with

This is for anyone who publishes reference data, and for anyone who writes a tool that reads it. It says what a pack must hold so that bean, and anyone else, can compare a reading with it and check the result.

It is a draft at version 0. One source has been built into a pack: NHANES 2017-2018. The format changes when a second source shows that it must.

## The idea

- A pack is one file of public reference data. Its name is the SHA-256 of its bytes.
- The pack holds a table for each measure and each cohort on its menu. The table is all that the method needs.
- The table comes to the value. The person's reading never leaves their machine, and nobody learns which cohort they chose.
- Anyone can hold a pack and hand it on. The SHA-256 says whether the bytes are the ones that were named. It does not say who built them: that is a separate question, answered below.
- The pack names the files of its source by SHA-256. For the NHANES pack, anyone can build the same bytes again from those files, with bean or with Python.
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
cd ~/nhanes                                                            # the folder of the downloads, not bean's
bean-pack-nhanes -out nhanes.pack *.xpt                                # build the pack
bean compare -pack nhanes.pack                                         # what the pack holds
bean compare -pack nhanes.pack -cohort male-30-39 -type 'lab.*'        # compare
bean compare -pack nhanes.pack -cohort male-30-39 -type weight -save   # compare, and keep the result
```

Then, in the folder of bean's code:

```
python3 tools/pack/check.py ~/nhanes/nhanes.pack                              # the check, without bean
python3 tools/pack/rebuild_nhanes.py ~/nhanes/nhanes.pack ~/nhanes/*.xpt      # build the same bytes again, without bean
make pack                                                                     # both, on packs good and wrong
```

[COMPARE.md](COMPARE.md) says where the NHANES files are. Its loop downloads the eight files of the larger pack, and one more that a pack does not use. `bean-pack-nhanes` reads the files that you give it and goes nowhere.

- **`-out` names a new file.** The builder writes over nothing.
- **A pack holds the tests whose files you give.** `DEMO_J` is always needed. A file that a pack holds nothing of is named and left out, so `*.xpt` works. Two sets of files give a pack on record, and bean says so. The six files `DEMO_J`, `TCHOL_J`, `HDL_J`, `GHB_J`, `CBC_J` and `BMX_J` give the pack of 13 measures. With `BIOPRO_J` and `FERTIN_J` as well, the eight give the pack of 26 measures.
- **A file that is not the one on record is refused.** It may be a download that was cut short. `-unknown-files` builds from it, and the pack then says so in its caveats. Such a pack is not the pack on record: `bean compare` takes it only with `-unknown-pack`.
- **Keep the pack file.** Your log names a pack by its SHA-256 and does not hold it. A reader of a comparison that you share needs the same pack to compute it again.
- **Use the type names and units that the pack lists.** `bean compare -pack nhanes.pack` shows them: a reading in another unit gets no number.

## The file

A pack is one JSON object, in UTF-8. A builder writes it in bean's canonical form (SPEC.md 4.1) with one line feed after it, so the same pack is the same bytes. A reader does not need that form: the name is the SHA-256 of the bytes as they are.

| Member | Value |
|---|---|
| `bean` | `pack/0`. |
| `title` | What the pack is, in a few words. |
| `source.name`, `source.publisher`, `source.home` | The source, who publishes it, and its address. |
| `source.files` | One entry or more. Each has `name`, `sha256` (64 lower-case hex digits), `bytes` and `url`. These are the files that the pack was built from. A name is there once. |
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
| `same` | A list of one entry or more. Each entry has `type` and `unit`. |

A `same` entry is a claim by the builder. It says that a reading of that type, in that unit, is this measure's quantity in this measure's unit. A reader compares a reading only when its type and its unit equal an entry exactly. No reader converts a unit. bean shows which measure each reading was set against, so that the person can see the claim. No type and unit may be in the `same` lists of two measures. A type is never one of bean's own records: `share`, or a type that starts with `comparison.`, `reference.`, `bean.` or `agent.`.

A cohort has `text`, such as `male, age 30-39`, and `caveats`. Its id is the name that a person gives to choose it.

Each object has the members that this document names and no other, each once. A pack is at most 64 MiB, and does not start with a byte order mark.

A number is a JSON number that a bean record may hold: a double, and for a whole number at most 9007199254740991. How it is written does not matter: 94, 94.0 and 9.4e1 are one number. `bytes` is a whole number of 1 or more.

A text holds no character that ends a line, or that hides, reorders or fakes text. Those are the Unicode categories Cc, Cf, Cs, Co, Zl and Zp, and every space but U+0020. They are also the blank fillers U+115F, U+1160, U+3164 and U+FFA0, and the noncharacters.

A measure or a cohort may have no cell. The cells may be in any order.

An id is 1 to 64 letters, digits, dots, `-` and `_`, and starts with a letter or a digit. A text is 1 to 400 bytes, with no space at either end.

## A cell

```
{"cohort":"male-30-39","kind":"weights","measure":"LBXTC","n":339,"values":[107,116,118.5],"weights":[51210.3,18830.9,40112.6]}
```

| Member | Value |
|---|---|
| `measure`, `cohort` | The ids of the measure and the cohort. A pair is there once. |
| `kind` | `weights`. Version 0 has no other kind. |
| `n` | The number of people: a whole number from 30 to 2147483647. |
| `values` | The distinct values of the cohort, in rising order: one or more. Each is a number from 1e-12 to 1e15. |
| `weights` | One number from 1e-12 to 1e15 for each value: the weight that the value carries. |

A weight is the sum of the sample weights of the people with that value. For a source with no sample weights, it is the count of those people.

A reader refuses a pack that has a cell of a kind it does not know. It never reads a table by a guess at what the table means.

## The method

The method's id is `weighted-percentile`, and this is version 1. Inputs: the value v of the reading, and one cell.

1. v must be a number above zero. Every measure of version 0 is a quantity above zero.
2. W is the sum of the weights, added in the cell's order. B is the sum of the weights of the values below v, in that order. E is the weight of the value v, or zero when the cell does not have it.
3. lower = 100 B / W, same = 100 E / W, percentile = 100 (B + E/2) / W. Round each to one decimal: floor(10 y + 0.5) / 10, with the multiplication and the addition as two steps.
4. The 25th, 50th and 75th percentile, for q = 0.25, 0.5 and 0.75. Add the weights in the cell's order, and call the sum so far S. Each percentile is the first value at which S >= q × W. Compute q × W as one multiplication, and compare S with it as doubles. If no value reaches it, take the last value.
5. n is the cell's n.

Every sum is in IEEE 754 doubles, in the order given. So every implementation gets the same bits. A change to any step is a new version.

Four vectors, each a cell and a value. An implementation that divides in step 4, where this one multiplies, gets the last one wrong.

| Values | Weights | v | percentile | lower | same | 25th | 50th | 75th |
|---|---|---|---|---|---|---|---|---|
| 1, 2, 3, 4 | 1, 1, 1, 1 | 2 | 37.5 | 25 | 25 | 1 | 2 | 3 |
| 1, 2, 3, 4 | 1, 1, 1, 1 | 2.5 | 50 | 50 | 0 | 1 | 2 | 3 |
| 10, 20, 30 | 1, 1, 8 | 30 | 60 | 20 | 80 | 30 | 30 | 30 |
| 10, 20 | 0.3, 0.1 | 15 | 75 | 75 | 0 | 10 | 10 | 20 |

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
| `caveats` | In this order: the pack's, the measure's, the cohort's, then the four below where they apply. |

The claim's `refs` has one entry: it is `about` the reading. Its value is the percentile, with the unit `{percentile}`.

The four caveats that a claim adds, in this order:

- `you chose the cohort: bean does not know whether you belong to it`, always;
- `the cohort is small: fewer than 100 people`, when n is below 100;
- `bean has no record of this pack: nobody has checked its tables, its caveats, or what it sets a reading against`, when the pack is not one on record;
- `the reading was signed by an agent or another key, not by you`, when that is so.

A reader with the pack checks a claim in four steps. The pack has the SHA-256 that the claim names. The reading's type and unit are in the measure's `same` list. The texts and the caveats are the ones that the pack gives. The method gives every number of the result. `tools/compare/reproduce.py` does this, and shares no code with bean. It fails for a pack that is not on record, unless the reader names that pack.

A saved comparison gives the reading's value away to anyone with the pack ([COMPARE.md](COMPARE.md)). It does not outlive its reading.

## Rules for a builder

- **The same files give the same bytes.** Write the object in canonical form. Sort the cells by measure, then by cohort. Add no time and no name of a machine.
- **Name every source file by its SHA-256.** A reader who fetches the files from the source can then build the pack again.
- **Leave out a cell of fewer than 30 people.** A reader refuses one.
- **Take each `same` entry from a real reading.** Use the type and the unit that a real importer or a real report writes. A second name or unit for a quantity is a second entry.
- **Say what does not carry over.** A caveat is part of the pack. Each comparison shows it, and each saved claim holds it.
- **Use the weights that the source says to use.** A wrong weight gives a number that looks right.

## A pack that is not on record

A pack is well formed or not, and a check can tell. Whether its tables are true is another matter. Whoever builds a pack chooses its tables, its caveats and which reading each measure takes. So a made-up pack computes as well as a real one.

bean has two packs on record, each by its SHA-256: the NHANES pack of 13 measures, and the NHANES pack of 26 measures. For any other pack:

- `bean compare -pack` shows a warning, and compares only with `-unknown-pack`. Each result then has the caveat above, in the output and in the saved claim.
- `bean check-bundle` and the check page mark the row: `by pack 7f3fda515b16 (NOT on record)`. They show what the reading was set against in quotation marks: it is the claim's word. They show the title only as what the sender calls it.
- `tools/compare/reproduce.py` computes the claim again, prints `NOT on record`, and fails. A reader who trusts the pack's publisher names the SHA-256: `-accept-pack <SHA-256>`.

For a pack on record, bean holds each measure's label, the types and units that it takes, each cohort in words, and the caveats. A row shows the label and the cohort, whatever the claim says. A claim that says otherwise is marked `(the claim MISSTATES this pack)`. So is one that sets a reading against a measure that does not take it, and one that drops a caveat or adds one.

Three more marks. `(names NO reference data)`: the comparison names no pack and no files. `(names BOTH a pack and files)`: a comparison is made with one. `(is NOT about a reading)`: it is about one of bean's own records, such as another comparison.

**What a check of a bundle does not do.** It does not compute a comparison again: it has the claim and not the pack. So a claim with false numbers that names the pack on record reads like a true one there. `reproduce.py` with the pack is what finds it.

One list in bean's code is not how many publishers will be known. A publisher's signature is the next step ([ROADMAP.md](ROADMAP.md)).

## What a check refuses

`make pack` tries each of these, and both checks refuse it:

- a member that the format does not have, a missing one, or one that is there twice;
- a cell of another kind;
- values that are not in rising order, or a value that is there twice;
- a value or a weight that is not a number from 1e-12 to 1e15;
- a list of weights that is shorter or longer than the list of values;
- an n below 30, above 2147483647, or below the number of distinct values;
- a cell that names a measure or a cohort that the pack does not have, or a cell that is there twice;
- a type and unit in the `same` lists of two measures, or twice in one, and a measure with an empty `same` list;
- a source file that is named twice;
- a text with a character that does not show, or with a space other than the plain one;
- a measure that claims one of bean's own record types;
- an id with a space.

## How a pack travels

A pack is public data, and a file. It needs no account and no server of its own.

- **Any host will do.** A web server, a file sent by e-mail, a disk. A host that serves a file under its SHA-256 as the path fits a pack with no change. The Blossom servers of Nostr do that.
- **A pointer is not the data.** A publisher can announce a pack: its title, its SHA-256 and where it is. A signed note on Nostr relays can carry that. The relays then hold a pointer and no health record.
- **A host may charge for bytes.** Storage and bandwidth cost money, and a host can ask for payment before it serves a file. bean holds no wallet. A person pays a host, if they must, with their own tools.
- **Nobody is paid to compute.** A paid comparison would send the cohort or the value to the one who computes. That is the disclosure that a pack exists to prevent.
- **Who says that a pack is the right one.** Today: this repository states the SHA-256 of the NHANES pack, below. Two builders that share no code give that SHA-256 from the files: `bean-pack-nhanes`, and `tools/pack/rebuild_nhanes.py` in Python.

The pack that `bean-pack-nhanes` version 1 builds from the six files on record has this SHA-256. The files are `DEMO_J`, `TCHOL_J`, `HDL_J`, `GHB_J`, `CBC_J` and `BMX_J`.

```
10555d6769421d4305cbcd4ff14960d8b966ddd829a50581a9584a60a84d29e2
```

The pack that it builds from those six and `BIOPRO_J` and `FERTIN_J` has this SHA-256.

```
d4b7205b503bc23d0842c176a47a5ce1d211189af4c107bec9dc48605812a4bf
```

The first pack stays on record. A comparison that a person saved names its pack, and must still check. A new row for a file that a pack on record holds would change that pack's bytes. Such a row needs a new version of the builder, and then a new pack on record.

`bean compare -pack` says whether a pack is one of these. The digests of the first six files come from one download ([COMPARE.md](COMPARE.md)). Those of the two new files come from two downloads through one network, five days apart. The `Reference data` workflow fetches the files from CDC again, from the code host's network, and checks each digest and the pack. That is a second network and not yet a second person.

## The NHANES pack

- **Measures.** The pack of 13 measures has the twelve blood tests of the first six files, and one body measure: weight in kg. The pack of 26 measures adds twelve tests of the biochemistry profile, and ferritin. [COMPARE.md](COMPARE.md) lists each test with its unit. Each is measured in everyone examined. Standing height is left out until a real export shows which unit a reading has.
- **Cohorts.** Each sex, by ten years of age from 20 to 79, and 80 and over: 14 cohorts, such as `female-40-49` and `male-80-over`.
- **Weights.** The examination weight, `WTMEC2YR`. Each cell is the table of steps 1 to 5 of the method in COMPARE.md.
- **Size.** About 500 kB for the first pack and 830 kB for the second.
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
- **A pack in a share.** `bean share` does not carry a pack. The reader's check names each pack by its SHA-256 and says that it is needed. The reader gets it from the sender or from any host. The check page does not compute a comparison again.
- **A measure that can be zero**, and an interval of uncertainty. Each is a new method version.
