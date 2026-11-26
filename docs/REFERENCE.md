# Reference packs: public data to compare a reading with

This is for anyone who publishes reference data, and for anyone who writes a tool that reads it. It says what a pack must hold so that bean, and anyone else, can compare a reading with it and check the result. [ARCHITECTURE.md](ARCHITECTURE.md) shows the whole path in one picture.

Readers support five draft file formats, `pack/0` to `pack/4`. A saved comparison records the method and the digest of the exact pack it used. [REFERENCE-PLAN.md](REFERENCE-PLAN.md) says what changes next and what does not, and where bean falls short of its own rules.

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
| Whoever pools | Builds a pack from what people contribute to it. | Each contributor's values. And whoever holds that pack reads each value in it. |
| A host | Serves the pack's bytes: a web server, a relay, a friend's disk. | That someone fetched the pack. |
| The person | Fetches the pack whole and compares on their own machine. | |
| A reader | Gets a shared comparison and computes it again from the pack. | What the person shared. |

No service computes a comparison for a person. A request for one cell says "a man of 30 to 39 with this test", and a request with the value says more. The request is itself a fact about someone's health. So a pack is fetched whole, and the arithmetic is done at home.

## What a comparison supports

A comparison says where a value lies among the values of a cohort in the source's sample. It does not say whether a value is healthy. It does not diagnose, predict or advise. Each result has its n and its caveats. A pack's cell has no least size. N is the number that the pack states. The n and the values of a pack are its builder's word: the format holds n only to the number of distinct values. The 30 of [COMPARE.md](COMPARE.md) is the rule of the survey's method, and of the survey's builder. A survey's sample stands for a population, and a percentage that rests on a few of the sample is rough. A pool stands for no one but its contributors.

## Try it

```
cd ~/nhanes                                                            # the folder of the downloads, not bean's
bean-pack-nhanes -out nhanes.pack *.xpt                                # build the pack
bean compare -pack nhanes.pack                                         # what the pack holds
bean compare -pack nhanes.pack -cohort male-30-39 -type 'lab.*'        # compare
bean compare -pack nhanes.pack -cohort male-30-39 -type weight -save   # compare, and keep the result
bean compare -pack nhanes.pack -type 'lab.*'                           # each reading with the cohort of its own date: see below
```

Then, in the folder of bean's code:

```
python3 tools/pack/check.py ~/nhanes/nhanes.pack                              # the check, without bean
python3 tools/pack/rebuild_nhanes.py ~/nhanes/nhanes.pack ~/nhanes/*.xpt      # build the same bytes again, without bean
make pack                                                                     # both, on packs good and wrong
```

[COMPARE.md](COMPARE.md) says where the NHANES files are. Its loop downloads the eight files of the larger pack, and one more that a pack does not use. `bean-pack-nhanes` reads the files that you give it and goes nowhere.

- **`-out` names a new file.** The builder writes over nothing.
- **A pack holds the tests whose files you give.** `DEMO_J` is always needed. A file that a pack holds nothing of is named and left out, so `*.xpt` works. Two sets of files give a pack on record, and bean says so. The six files `DEMO_J`, `TCHOL_J`, `HDL_J`, `GHB_J`, `CBC_J` and `BMX_J` give the pack of 13 measures. With `BIOPRO_J` and `FERTIN_J` as well, the eight give the pack of 20 measures.
- **A file that is not the one on record is refused.** It may be a download that was cut short. `-unknown-files` builds from it, and the pack then says so in its caveats. Such a pack is not the pack on record: `bean compare` takes it only with `-unknown-pack`.
- **Keep the pack file.** Your log names a pack by its SHA-256 and does not hold it. A reader of a comparison that you share needs the same pack to compute it again.
- **Use the type names and units that the pack lists.** `bean compare -pack nhanes.pack` shows them: a reading in another unit gets no number.

## The cohort from two records

One run with `-cohort` sets every reading against one cohort, whatever its date. A reading of ten years ago then gets the cohort of today. With readings and no `-cohort`, bean gives each reading the cohort of the age that you had on its date. It needs two records for that. Both are optional, and you state each once:

```
bean add person.sex male          # exactly male or female
bean add person.birth 1986-05     # a year, or a year and a month
bean compare -pack nhanes.pack -type 'lab.*'
```

- **They are ordinary records.** You sign them, and `bean ls -type 'person.*'` shows them. Correct one with `bean add <type> <value> -corrects <id>`, and delete one with `bean purge <id>`. A deleted correction brings back the record that it replaced. To state nothing again, delete each record of the type. They sync, export and share as any record.
- **Which records count.** Current records that your identity signed about you, with the source `self` and no ref. `bean add` writes such a record. So does `bean append`, for a line that names that source. bean does not read a record of an agent or of a peer, or one with another source or with a ref. All records of one type must state one value. The number 1986 and the text `1986` are one value.
- **The birth** is a year of four digits with no unit, or a year and a month as `1986-05`. It is not after now. bean takes no day of birth.
- **The sex** is exactly `male` or `female`, the two groups of the survey. It picks a group to compare with, and says nothing more about you. If neither word fits, or you do not want to state one, use `-cohort`. One value stands for every date: if that is not so for you, use `-cohort` too.
- **The age on a date.** bean counts in months, from year 0. The months of a reading are those of its observed start, as written. A year alone is its twelve months, for a birth and for a reading whose precision is a year. The lowest age is floor((first month of the reading - last month of the birth - 1) / 12). The highest is floor((last month of the reading - first month of the birth) / 12). A row has a cohort when both ages are in one cohort.
- **A row with no cohort has no number, and says why.** The reading has no date, a date after now, or a date before the birth. Or the date lies on the edge of two cohorts, or the pack has no cohort for that age. No row names an age. With a year of birth alone, one calendar year in ten lies on an edge. With the month of birth, it is one month in ten years. To state the month, correct the record: a second record with another value is a stop.
- **The pack must be one on record.** A pack gives a cohort in words, and no format has a field for its sex and ages. bean's code holds both for each cohort of the two packs on record. For any other pack, name the cohort.
- **It shows, and does not save.** `-save` needs `-cohort`: run once for each cohort, with the ids of its readings. A saved claim says that you chose the cohort, and it would not be true here. The caveat on the screen is another: `the cohort follows from those records as they are now: bean does not know whether you belong to it`.
- **When a record breaks a rule, bean stops.** It names the type, the ids and the rule, and never the value. `-cohort` always works: bean then reads neither record, and nothing new is in your log.
- **A caveat of one cohort names it.** The rows of one run can differ in cohort. So a caveat that the pack states for one cohort shows as `the pack says of male-80-over: "..."`.

**Who can read the two records.** Whoever reads your log reads them: a copy, an export, and a share of `*`. A share of `lab.*`, or of a comparison, does not carry them. An assistant that reads the log reads them, through `bean mcp` or with `bean ls` in your shell. The table shows a cohort for each row, to you and to anyone that you paste it to. Rows on both sides of a birthday narrow the birth down, and so does a row on an edge. So does a row that says that its date is before the birth, or that the pack has no cohort. `bean compare -pack` with `-cohort` needs no record.

**What bean does not know.** The line above the table names the two records by id, and says that your identity signed them. It does not say that you wrote them: a program with your key can write such a record. So can an importer that names its source `self`. A second record with another value makes this way stop, until you correct or delete one. A reading with a wrong date gets the cohort of that date: the date and the cohort are in its row. So does a reading that you typed late with no `-at`. A reading keeps the month that its date was written with, in the offset of that place. The DATE column shows the time in the zone of your machine. So near the end of a month, the cohort can be the one of the month before or after.

## The file

A pack is one JSON object, in UTF-8. A builder writes it in bean's canonical form (SPEC.md 4.1) with one line feed after it, so the same pack is the same bytes. A reader does not need that form: the name is the SHA-256 of the bytes as they are.

| Member | Value |
|---|---|
| `bean` | `pack/0`, `pack/1`, `pack/2`, `pack/3`, or `pack/4`. |
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

A pack with a panel-qualified match uses `pack/1`. A `pack/2` file has at least one `same` entry with `any_of`. Other entries can use the earlier forms.
An entry uses at most one of `when` and `any_of`.
The latter lists 1 through 16 alternatives. Each alternative maps 1 through 8 field names to nonempty text values.
A field is `source.system` or `context.NAME`. NAME follows the measure id rules and contains no dot.
An alternative passes only when every named field of the signed reading is the exact stated string. One passing alternative gives a match.
A `pack/4` file includes at least one dimensionless measure (UCUM unit `1`) whose matching row declares `"unit":null` and a complete `when` panel. This matches only a signed record with the same type, provider, printed test and panel, and an absent `value.unit` member. An explicit empty string, null or a different unit fails. The publisher must establish what the named ratio divides, its scale and any rounding. An unreported numerator, denominator or result cannot be inferred from this rule. Other measures retain exact unit matching. Earlier formats reject a null matching unit.

The type and unit must still match exactly in all other cases. All five formats use the same weighted percentile method. The cell kind determines whether a table may include a reported zero.

A `same` entry may also hold `when`, an object with three nonempty text fields.
Compare `when.system`, `when.test` and `when.panel` with the signed reading’s `source.system`, `context.test` and `context.panel`.
All three strings must match exactly, as must the type and unit. A missing field gives no match. Each type and unit still occurs only once.
A `pack/1` file must have at least one qualified entry. Readers that support only `pack/0` refuse this file. Both formats use the same percentile method.

A `same` entry is a claim by the builder. It says that a reading of that type, in that unit, is this measure's quantity in this measure's unit. A reader compares a reading only when its type and its unit equal an entry exactly. No reader converts a unit. bean shows which measure each reading was set against, so that the person can see the claim. No type and unit may be in the `same` lists of two measures. A type is never one of bean's own records: `share`, or a type that starts with `comparison.`, `reference.`, `bean.` or `agent.`.

A cohort has `text`, such as `male, age 30-39`, and `caveats`. Its id is the name that a person gives to choose it.

Each object has the members that this document names and no other, each once. A pack is at most 64 MiB, and does not start with a byte order mark.

A number is a JSON number that a bean record may hold: a double, and for a whole number at most 9007199254740991. How it is written does not matter: 94, 94.0 and 9.4e1 are one number. `bytes` is a whole number of 1 or more.

A text holds no character that ends a line, or that hides, reorders or fakes text. Those are the Unicode categories Cc, Cf, Cs, Co, Zl and Zp, and every space but U+0020. A reader takes the categories from the Unicode version that it is built with. So two readers can judge one character differently, when a later version gives it a category ([KNOWN-GAPS.md](KNOWN-GAPS.md)). They are also the blank fillers U+115F, U+1160, U+3164 and U+FFA0, and the noncharacters.

A measure or a cohort may have no cell. The cells may be in any order.

An id is 1 to 64 letters, digits, dots, `-` and `_`, and starts with a letter or a digit. A text is 1 to 400 bytes, with no space at either end.

## A cell

```
{"cohort":"male-30-39","kind":"weights","measure":"LBXTC","n":339,"values":[107,116,118.5],"weights":[51210.3,18830.9,40112.6]}
```

| Member | Value |
|---|---|
| `measure`, `cohort` | The ids of the measure and the cohort. A pair is there once. |
| `kind` | `weights` or, in `pack/3` or `pack/4`, `weights-nonnegative`. |
| `n` | The number of people: a whole number from 1 to 2147483647, and not below the number of distinct values. |
| `values` | The distinct values of the cohort, in rising order: one or more. Positive values are from 1e-12 to 1e15. Nonnegative cells also admit zero. |
| `weights` | One number from 1e-12 to 1e15 for each value: the weight that the value carries. |

A `pack/3` file uses the same match rules and has at least one cell of kind `weights-nonnegative`.
That kind admits zero among its values and uses method version 2. Every weight remains strictly positive.
Other cells keep kind `weights` and method version 1.

A weight is the sum of the sample weights of the people with that value. For a source with no sample weights, it is the count of those people.

A reader refuses a pack that has a cell of a kind it does not know. It never reads a table by a guess at what the table means.

## The method

The method's id is `weighted-percentile`, and this is version 1. Inputs: the value v of the reading, and one cell.

1. v must be a number above zero. Every measure of version 0 is a quantity above zero.
2. W is the sum of the weights, added in the cell's order. B is the sum of the weights of the values below v, in that order. E is the weight of the value v, or zero when the cell does not have it.
3. lower = 100 B / W, same = 100 E / W, percentile = 100 (B + E/2) / W. Round each to one decimal: floor(10 y + 0.5) / 10, with the multiplication and the addition as two steps.
4. The 25th, 50th and 75th percentile, for q = 0.25, 0.5 and 0.75. Add the weights in the cell's order, and call the sum so far S. Each percentile is the first value at which S >= q × W. Compute q × W as one multiplication, and compare S with it as doubles. If no value reaches it, take the last value.
5. n is the cell's n.

For `weights-nonnegative`, version 2 also accepts a reading of zero in step 1. Steps 2 through 5 are identical.

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

A claim means the same for ever. The third of these caveats says what the signer's bean knew on that day, and a pack can be put on record later. So a check allows that caveat, in its place, for a pack that the reader's bean has on record. A check judges a claim by what the claim names: the reading, the pack and the method.

A reader with the pack checks a claim in four steps. The pack has the SHA-256 that the claim names. The reading matches a `same` entry, including its `when` fields, one of its `any_of` alternatives, or its omitted unit, if present. The texts and the caveats are the ones that the pack gives. The method gives every number of the result. `tools/compare/reproduce.py` does this, and shares no code with bean. It fails for a pack that is not on record, unless the reader names that pack. It also checks the record around the claim: the record has the members of a record and no other, and its source is `bean-compare`. It does not check the claim's own time. bean sets that to the time of the reading, so take the time from the reading.

A saved comparison gives the reading's value away to anyone with the pack ([COMPARE.md](COMPARE.md)). It does not outlive its reading.

## Rules for a builder

- **The same files give the same bytes.** Write the object in canonical form. Sort the cells by measure, then by cohort. Add no time and no name of a machine.
- **Name every source file by its SHA-256.** A reader who fetches the files from the source can then build the pack again.
- **State your own rule for a small cell.** The format has no least size. The survey's builder leaves out a cell of fewer than 30 people: that is the method of [COMPARE.md](COMPARE.md), for a sample of a survey.
- **Tell each contributor what a cell shows, before they contribute.** A cell lists each distinct value with its weight, beside the cohort's text. In a pack that is built from contributions, each listed value is the value of one contributor or more. That holds in a cell of any size, and whoever has the pack reads it. The cells of one cohort are read together: in a cohort of one person, they are that person's values. `bean compare` prints a cell's median and its middle half: in a cell of one, that is the one value. Two packs that differ by one contributor show that person's value. bean cannot check that a builder said so. [ROADMAP.md](ROADMAP.md), path 4, and DECISIONS 325 and 328 list what a real pool needs first.
- **Take each `same` entry from a real reading.** Use the type and the unit that a real importer or a real report writes. A second name or unit for a quantity is a second entry.
- **Say what does not carry over.** A caveat is part of the pack. Each comparison shows it, and each saved claim holds it.
- **Use the weights that the source says to use.** A wrong weight gives a number that looks right.

## A pack that is not on record

A pack is well formed or not, and a check can tell. Whether its tables are true is another matter. Whoever builds a pack chooses its tables, its caveats and which reading each measure takes. So a made-up pack computes as well as a real one.

bean has two packs on record, each by its SHA-256: the NHANES pack of 13 measures, and the NHANES pack of 20 measures. For any other pack:

- `bean compare -pack` shows a warning, and compares only with `-unknown-pack`. Each result then has the caveat above, in the output and in the saved claim.
- `bean check-bundle` and the check page mark the row: `by pack 7f3fda515b16 (NOT on record)`. They show what the reading was set against in quotation marks: it is the claim's word. They show the title only as what the sender calls it.
- `tools/compare/reproduce.py` computes the claim again, prints `NOT on record`, and fails. A reader who trusts the pack's publisher names the SHA-256: `-accept-pack <SHA-256>`.

For a pack on record, bean holds each measure's label, the types and units that it takes, each cohort in words, and the caveats. A row shows the label and the cohort, whatever the claim says. A claim that says otherwise is marked `(the claim MISSTATES this pack)`. So is one that sets a reading against a measure that does not take it, and one that drops a caveat or adds one.

Three more marks. `(names NO reference data)`: the comparison names no pack and no files. `(names BOTH a pack and files)`: a comparison is made with one. `(is NOT about a reading)`: it is about one of bean's own records, such as another comparison.

**What a check of a bundle does not do.** It does not compute a comparison again: it has the claim and not the pack. So a claim with false numbers that names the pack on record reads like a true one there. `reproduce.py` with the pack is what finds it.

One list in bean's code is not how many publishers will be known. A publisher's signature is built when a second publisher exists ([ROADMAP.md](ROADMAP.md), path 4).

## What a check refuses

`make pack` tries each of these, and both checks refuse it:

- a member that the format does not have, a missing one, or one that is there twice;
- a cell of another kind;
- values that are not in rising order, or a value that is there twice;
- a value or a weight that is not a number from 1e-12 to 1e15;
- a list of weights that is shorter or longer than the list of values;
- an n below 1, above 2147483647, or below the number of distinct values;
- a cell that names a measure or a cohort that the pack does not have, or a cell that is there twice;
- a type and unit in the `same` lists of two measures, or twice in one, and a measure with an empty `same` list;
- a source file that is named twice;
- a text with a character that does not show, or with a space other than the plain one;
- a measure that claims one of bean's own record types;
- an id with a space.

## How a pack travels

A pack is public data, and a file. It needs no account and no server of its own.

- **No publisher is special.** bean names no publisher of a pack, and no host of one. A file and its SHA-256 on a page is a full publication: a signature, a Blossom server and a relay are additions. One thing falls short of this today: the list of packs on record is in bean's code ([ROADMAP.md](ROADMAP.md)). A second thing: since 2026-11-23 some documents name one publisher and its host. Whether they may is an open decision of the owner (ROADMAP.md).
- **Any host will do.** A web server, a file sent by e-mail, a disk. [examples/pack-host](../examples/pack-host/README.md) is a read-only one, with a check that a reader runs. A host that serves a file under its SHA-256 as the path fits a pack with no change. The Blossom servers of Nostr do that.
- **A pointer is not the data.** A publisher can announce a pack: its title, its SHA-256 and where it is. A signed note on Nostr relays can carry that. The relays then hold a pointer and no health record.
- **A host may charge for bytes.** Storage and bandwidth cost money, and a host can ask for payment before it serves a file. bean holds no wallet. A person pays a host, if they must, with their own tools.
- **Nobody is paid to compute.** A paid comparison would send the cohort or the value to the one who computes. That is the disclosure that a pack exists to prevent.
- **Who says that a pack is the right one.** Today: this repository states the SHA-256 of each of the two NHANES packs, below. Two builders that share no code give each from the files: `bean-pack-nhanes`, and `tools/pack/rebuild_nhanes.py` in Python.

The pack that `bean-pack-nhanes` version 1 builds from the six files on record has this SHA-256. The files are `DEMO_J`, `TCHOL_J`, `HDL_J`, `GHB_J`, `CBC_J` and `BMX_J`.

```
10555d6769421d4305cbcd4ff14960d8b966ddd829a50581a9584a60a84d29e2
```

The pack that it builds from those six and `BIOPRO_J` and `FERTIN_J` has this SHA-256.

```
5cae07db64295d565ffae7c251c17fdb9f043dc0638868557275f8b0b0311efa
```

The first pack stays on record. A comparison that a person saved names its pack, and must still check. A new row for a file that a pack on record holds would change that pack's bytes. Such a row needs a new version of the builder, and then a new pack on record.

`bean compare -pack` says whether a pack is one of these. The digests of the first six files come from one download ([COMPARE.md](COMPARE.md)). Those of the two new files come from two downloads through one network, five days apart. The `Reference data` workflow fetches the files from CDC again, from the code host's network, and checks each digest and the pack. That is a second network and not yet a second person.

## The NHANES pack

- **Measures.** The pack of 13 measures has the twelve blood tests of the first six files, and one body measure: weight in kg. The pack of 20 measures adds six tests of the biochemistry profile, and ferritin. [COMPARE.md](COMPARE.md) lists each test with its unit. CDC planned each for everyone examined, and about nine in ten adults have a value. Standing height is left out until a real export shows which unit a reading has.
- **Cohorts.** Each sex, by ten years of age from 20 to 79, and 80 and over: 14 cohorts, such as `female-40-49` and `male-80-over`.
- **Weights.** The examination weight, `WTMEC2YR`. Each cell is the table of steps 1 to 5 of the method in COMPARE.md.
- **Size.** About 500 kB for the first pack and 720 kB for the second.
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
- **An interval of uncertainty.** It would need a new method version.

The [external pack directory](EXTERNAL-PACKS.md) records digests that a publisher supplied, and the reader version each pack requires. On 2026-11-25 every entry comes from one publisher.
