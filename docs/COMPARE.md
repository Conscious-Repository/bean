# Compare a reading with people like you

`bean compare` shows where one of your readings lies among people you describe, in public survey data. You state the cohort: sex and age, and any of ethnicity, body measures and a day's food. bean computes on your machine, from files you downloaded and keep, and shows how many people the cohort has. With `-save` the result is a signed record that anyone can compute again without bean.

It describes a position in a sample. It does not say whether a value is healthy, and it does not diagnose, predict or advise.

There are two ways to compare, and both give the same numbers. This document is the first: with the survey's own files, for any cohort that you can state as rules. The second is with a reference pack, one small file with a fixed menu of cohorts: `bean compare -pack` ([REFERENCE.md](REFERENCE.md)). A pack is the general form. It holds any public source that someone has built into one, and NHANES is the first.

## The data: NHANES 2017-2018

[NHANES](https://wwwn.cdc.gov/nchs/nhanes/) is a survey by the US National Center for Health Statistics (CDC). About 9,000 people of all ages are interviewed, examined and have blood drawn. The files are public, one row per person, joined by the number `SEQN`. bean reads the 2017-2018 cycle: file names ending in `_J`.

Download the files you need from CDC and keep each one. bean never goes online: you fetch, bean keeps.

```
for f in DEMO_J TCHOL_J HDL_J GHB_J CBC_J BMX_J DR1TOT_J; do
  curl -fLO https://wwwn.cdc.gov/Nchs/Data/Nhanes/Public/2017/DataFiles/$f.xpt && bean import -yes nhanes $f.xpt
done
```

`-yes` skips the review: these files hold no reading of yours. Together they are about 18 MB.

bean uses a kept file only when its SHA-256 is the one on record. A download that was cut short can still read as a file, with fewer people in it, and give another number. `-unknown-files` uses a copy that is not on record, for example after CDC publishes a file again; each row then says `file not on record`. `bean purge` of a kept file is for good in this log, like every purge: the same file cannot be imported into it again.

| File | Holds | Needed for |
|---|---|---|
| `DEMO_J` | Sex, age, ethnicity, the examination weight | Every comparison |
| `TCHOL_J` | Total cholesterol | `lab.cholesterol-total` |
| `HDL_J` | HDL cholesterol | `lab.hdl-cholesterol` |
| `GHB_J` | Glycohemoglobin | `lab.hemoglobin-a1c` |
| `CBC_J` | Complete blood count | `lab.wbc` or `lab.white-blood-cell-count`, `lab.rbc` or `lab.red-blood-cell-count`, `lab.hemoglobin`, `lab.hematocrit`, `lab.mcv`, `lab.mch`, `lab.mchc`, `lab.rdw`, `lab.platelets` or `lab.platelet-count` |
| `BMX_J` | Body measures | A rule on BMI or waist |
| `DR1TOT_J` | What each person ate on one day, as nutrient totals | A rule on food |

A reading is compared only under one of these types, in the unit beside it. Type the name and the unit exactly: `mg/dl` and `mmol/L` give no number, and the output says which unit is needed.

| Type | Unit |
|---|---|
| `lab.cholesterol-total`, `lab.hdl-cholesterol` | `mg/dL` |
| `lab.hemoglobin-a1c` | `%` or `% of total Hgb` |
| `lab.hemoglobin`, `lab.mchc` | `g/dL` |
| `lab.hematocrit`, `lab.rdw` | `%` |
| `lab.mcv` | `fL` |
| `lab.mch` | `pg` |
| `lab.wbc`, `lab.platelets` | `x10E3/uL` |
| `lab.white-blood-cell-count`, `lab.platelet-count` | `Thousand/uL` |
| `lab.rbc` | `x10E6/uL` |
| `lab.red-blood-cell-count` | `Million/uL` |

A reading you typed and a reading you imported are two readings, even with the same value. Each gets its own row.

Each file is kept encrypted, as a `reference.file` record with its SHA-256. You sign for having kept it. That does not make it CDC's claim: anyone can check the SHA-256 against the copy CDC serves. The importer says whether the file is the one bean has on record ([the digests](../internal/nhanes/nhanes.go)).

The digests on record come from one download, on 2026-11-09, through a proxy that could read the traffic. Until a second person has confirmed them from their own connection, treat "the file on record" as "the file one download saw". Check for yourself: `shasum -a 256 DEMO_J.xpt` against the table in the code. bean knows of no digests that CDC publishes for these files.

## Compare

```
bean compare -sex male -age 30-39 -type 'lab.*'
bean compare -sex female -age 40-49 3fa9c1
```

`-sex` and `-age` are required. bean does not know yours, and never guesses a cohort. `-age` is a range of whole years, both ends included: for someone of 35, try `30-39`. A single year leaves too few people. 80 means 80 and over, because NHANES records it so. Give record ids, or `-type` for every current reading of a type.

```
Compared with NHANES 2017-2018: male, age 30-39.
READING                VALUE      PERCENTILE  LOWER  SAME  MIDDLE HALF  MEDIAN  N    NOTE  ID
lab.cholesterol-total  187 mg/dL  46          44.2%  3.5%  164 to 213   188     339        fca23ea91778
PERCENTILE is the weighted share of the cohort with a lower value, plus half of those with the same value. It says where a value lies in this sample, not whether it is healthy.
Read each number with these caveats:
  - the lab that measured the reading and NHANES used their own instruments and methods
  - NHANES is a sample of people living in the United States in the survey years, not a reference range
  - you chose the cohort: bean does not know whether you belong to it
Not saved. Pass -save to keep these comparisons as signed records.
```

- **PERCENTILE**: the weighted share of the cohort with a lower value, plus half of those with the same value.
- **LOWER**, **SAME**: those two shares.
- **MIDDLE HALF**, **MEDIAN**: the 25th, 75th and 50th percentile of the cohort.
- **N**: the people in the cohort who have a value. Read it before the percentile.
- **NOTE**: `small cohort` under 100 people; `file not on record`; or `not comparable:` and why, with no number.

The numbers in this example are statistics of the public files, not anyone's reading.

### People like you: more rules

Each `-where` narrows the cohort by one NHANES variable: `<DATASET>.<VARIABLE>=<value>` or `=<from>-<to>`, both ends included. A measurement such as BMI needs a range: `=24` matches almost nobody, `=22-26` does. A person with no value for a rule's variable is left out. The nutrient totals are sums, and some sit a hair off a whole number (14.999999999999998 g of fiber). So a bound exactly on a whole number can leave a few people out. Put bounds between the values you mean, such as 14.5-60.

```
bean compare -sex male -age 30-39 -where DEMO_J.RIDRETH3=3 -where BMX_J.BMXBMI=20-27 -type 'lab.*'
bean compare -sex male -age 30-39 -where DR1TOT_J.DR1TPROT=90-160 -where DR1TOT_J.DR1TFIBE=15-60 -type 'lab.*'
```

| Variable | Meaning | Values |
|---|---|---|
| `DEMO_J.RIDRETH3` | Race and Hispanic origin | 1 Mexican American, 2 Other Hispanic, 3 Non-Hispanic White, 4 Non-Hispanic Black, 6 Non-Hispanic Asian, 7 Other, including multi-racial |
| `BMX_J.BMXBMI` | Body mass index | kg/m² |
| `BMX_J.BMXWAIST` | Waist circumference | cm |
| `DR1TOT_J.DR1TKCAL` | Energy eaten on the recalled day | kcal |
| `DR1TOT_J.DR1TPROT`, `DR1TCARB`, `DR1TTFAT` | Protein, carbohydrate, fat | g |
| `DR1TOT_J.DR1TFIBE`, `DR1TSUGR` | Fiber, total sugars | g |
| `DR1TOT_J.DR1TSODI` | Sodium | mg |
| `DR1TOT_J.DR1TALCO`, `DR1TCAFF` | Alcohol (g), caffeine (mg) | |

The codes and labels are from CDC's documentation of each file (`<NAME>.htm` next to the `.xpt`), read on 2026-11-09. Any other numeric variable of these three files works the same way: read its codes there first. A code such as 7 for "refused" is a value like any other to a range. Rules on other files are refused until bean has their sample weight on record.

Every rule shrinks the cohort. Sex and a ten-year age range leave about 340 people. Add ethnicity and BMI and about 40 are left. Below 30 bean gives no number and N shows how many there were; below 100 the row says `small cohort`. Widen a range before you trust a narrow cohort.

## What a result means

A row is one of two things.

- **No number**, and a few words on why. bean has no reference data for the type. The reading is text, not above zero, or in another unit. A file is not in the log, or is not the file on record. The cohort has fewer than 30 people. What to do about it is said once, under the table.
- **A number, with caveats.** No comparison of a lab reading with NHANES is like for like, and bean never says one is. Every one comes with these:
  - the lab that measured the reading and NHANES used their own instruments and methods;
  - NHANES is a sample of people living in the United States in the survey years, not a reference range;
  - you chose the cohort: bean does not know whether you belong to it.

  And where it applies: ages of 80 and over are recorded as 80. A cohort chosen by one day's food says little about what people usually eat. The cohort is small. A file is not the one on record. An agent or another key signed the reading, not you.

Limits beyond those:

- No interval of uncertainty is computed. The survey's design variables are in `DEMO_J` for it.
- NHANES fills in a number for a value below a detection limit, and marks it in a comment-code variable. The files of the twelve tests here have no such variable; a test whose file has one needs a rule for it first.
- The twelve tests are measured in everyone examined. Tests that need fasting (glucose, triglycerides, LDL) or that NHANES measured in part of the sample are not here yet: they need other weights.
- A lab's name for a test maps to an NHANES variable by a table in the code ([`internal/nhanes`](../internal/nhanes/nhanes.go)). Each row is a claim that the two measure the same thing in the same unit. Today the table has the names and units that the Labcorp and the Quest report print for the twelve tests. Mean platelet volume is left out on purpose: it differs too much between makes of analyzer.

## Save, share, compute again

```
bean compare -sex male -age 30-39 -type 'lab.*' -save
```

A comparison is kept once. Two saves at one moment can each save the same one, and so can two copies that you then sync. bean then deletes all but one and says so.

`-save` writes one `comparison.percentile` record per result, signed by you. Without it nothing is written: try cohorts freely, and save what you will act on or share. The same inputs twice add nothing. A comparison you deleted is saved again when you ask again.

The record holds the percentile, and in `context` only what is needed to compute it again:

- `reading`: the id of your reading. The id is a hash of the reading, so the claim pins its value without holding it.
- `reference`: the data set and variable, the variable's label, the weight, and each file by data set and SHA-256.
- `cohort`: every rule, with the label the file gives its variable, and the cohort in words.
- `method`: `nhanes-weighted-percentile`, version 1.
- `result`: n, the three shares and the three quartiles.
- `caveats`: the ones above that apply.

**A saved comparison says who you compared yourself with, and gives the value away.** The cohort (sex, age range, and any rule on ethnicity, body or food) is in the record, which is plain JSON like every record. And the shares of the cohort below and at your value, with the public files, let anyone work the value out. Anyone who reads your log, or a share that holds the comparison, has both. bean says so when it saves.

**A comparison does not outlive its reading.** You can correct or delete a reading (`add -corrects`, `import -corrections`, `purge`), or such a change can arrive with `sync`. bean then deletes the saved comparisons of that reading and says how many. The deletion is a record, so every copy obeys it. Run `bean compare` again for the corrected reading. The cleanup also runs with `bean verify`, which is what to run after copying a log with another tool, and before a share. Until it has run, no command shows the old reading's value, and the comparison cannot be shared. To correct a lab value, import the corrected report with `-corrections`; `bean add -corrects` needs `-at` with the reading's own time, or the correction is dated now.

### Check one without bean

Anyone with the records and the files can check a comparison and compute it again, with no bean and no account:

```
pip install -r tools/requirements.txt
python3 tools/compare/reproduce.py <a bundle shared with -files, or a bean export folder>
python3 tools/compare/reproduce.py <a bundle> DEMO_J.xpt TCHOL_J.xpt
python3 tools/compare/reproduce.py records.jsonl DEMO_J.xpt TCHOL_J.xpt
```

A comparison that was made with a reference pack needs the pack and no other file: `python3 tools/compare/reproduce.py <a bundle> nhanes.pack`. The tool finds the pack by its SHA-256, whatever its file is called.

A folder is read whole, so a `bean export` or a bundle shared with `-files` is enough by itself. Without the files in the folder, give them: the ones named in each claim, fetched from CDC. `records.jsonl` is the output of `bean ls -json`. A file that is missing gives `CANNOT CHECK` and its name, not a result that differs.

The tool finds each file by its SHA-256, so the bytes are the same wherever you got them. What fetching from CDC yourself adds is the knowledge that CDC serves a file with that digest.

[`tools/compare/reproduce.py`](../tools/compare/reproduce.py) is a second implementation in Python. It shares no code with bean. It holds the same small tables as bean, and a test keeps them equal. The tables say which variable each kind of reading may be set against, and in which unit. They also give the weights, the caveats and the files' digests. For each comparison it checks that:

1. the claim and the reading are what their ids say, and their signatures are good (the checks of the second verifier, `tools/second/beanverify.py`);
2. each file has the SHA-256 the claim names;
3. the claim is one the method could have made. The reading is of a kind and in a unit that the method sets against that variable. The cohort starts with one sex and an age range, and its text says what its rules say. The weight is the one step 4 gives, and the caveats are the ones the inputs give. Each variable has, in its file, the label the claim gives it;
4. the method below gives every number of the result.

A signature alone is not enough for 3: anyone can sign a record of this type by hand. A claim that computes but says "female, age 20-29" over rules that say otherwise is refused. So is one that sets a weight in kilograms against cholesterol.

It prints the reading next to the variable it was set against, for example `lab.cholesterol-total 187 mg/dL against LBXTC 'Total Cholesterol (mg/dL)'`. It says for itself which files are not the ones on record. A good signature shows which key signed. `bean verify` on the log, or `bean check-bundle` on a bundle, shows whether that key speaks for the person. `make compare` runs all of this against bean on synthetic files in CI, and on CDC's files when `BEAN_NHANES_DIR` names a folder of them.

## The method, version 1

This is the one place the method is written. Inputs: the value v of the reading; the test's file and variable; `DEMO_J`; the rules; the file of each rule.

0. v must be above zero. None of these quantities can be zero or less.
1. Take the rows of the test's file in file order. Skip a row with no value.
2. `DEMO_J` must have a row with the same `SEQN`. If not, stop: the files are not of one release.
3. For each rule, find the row of its file with the same `SEQN`. Skip the row if there is none, or if it has no value of the variable. Also skip it if the value is below the rule's minimum or above its maximum. Sex is the rule `RIAGENDR` = 1 (male) or 2 (female); age is a rule on `RIDAGEYR`.
4. The weight is `WTDRD1` of `DR1TOT_J` if a rule names that file, else `WTMEC2YR` of `DEMO_J`. CDC's notes on the dietary file say to use the day one weight whenever day one nutrient data are used, alone or with examination data. Skip the row if the weight is missing or not above zero.
5. n is the number of rows left. With fewer than 30, give n and no other number. For each distinct value x among them, g(x) is the sum of the weights of the rows with that value. Add them in file order, in IEEE 754 doubles.
6. Go through the distinct values in rising order. W is the sum of every g(x), and B is the sum of g(x) for x below v. E is g(v), or zero when no row has the value v.
7. lower = 100 B / W, same = 100 E / W, percentile = 100 (B + E/2) / W. Round each to one decimal: floor(10 y + 0.5) / 10, with the multiplication and the addition as two steps.
8. The 25th, 50th and 75th percentile of the cohort, for q = 0.25, 0.5 and 0.75. Each is the smallest x such that the g of the values up to and including x, added in rising order, reach q W.

After step 5 only n and the table of g(x) are used. So a published table of g(x) for a cohort gives the same result, to the last bit, as the files it was made from. A reference pack is that table for each cohort on a menu ([REFERENCE.md](REFERENCE.md)).

The caveats of a saved claim are in a fixed order. First come the three that every comparison has. Then the top age, if the age range ends at 80. Then the one day of food, if a rule names `DR1TOT_J`. Then the small cohort, if n is below 100. Then one for each file that was not on record. Last, the reading's signer, if it is not the person. Their texts are in [`cmd/bean/compare.go`](../cmd/bean/compare.go) and in `reproduce.py`, and a test keeps them equal.

A file is SAS transport, version 5. A number that an IEEE double cannot hold exactly is refused, and so is one that SAS does not write: not normalized, or below 1e-70. bean's reader gives the same bits as pandas for every value of the ten CDC files it was tried on. The one exception is zero, which pandas reads as 5.4e-79.

A change to any step is a new method version.
