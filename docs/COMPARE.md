# Compare a reading with people like you

`bean compare` shows where one of your readings lies among people you describe, in public survey data. You state the cohort: sex and age, and any of ethnicity, body measures and a day's food. bean computes on your machine, from files you downloaded and keep, and shows how many people the cohort has. With `-save` the result is a signed record that anyone can compute again without bean.

It describes a position in a sample. It does not say whether a value is healthy, and it does not diagnose, predict or advise.

## The data: NHANES 2017-2018

[NHANES](https://wwwn.cdc.gov/nchs/nhanes/) is a survey by the US National Center for Health Statistics (CDC). About 9,000 people of all ages are interviewed, examined and have blood drawn. The files are public, one row per person, joined by the number `SEQN`. bean reads the 2017-2018 cycle: file names ending in `_J`.

Download the files you need from `https://wwwn.cdc.gov/Nchs/Data/Nhanes/Public/2017/DataFiles/<NAME>.xpt` and keep each one:

```
bean import nhanes DEMO_J.XPT
```

| File | Holds | Needed for |
|---|---|---|
| `DEMO_J` | Sex, age, ethnicity, the examination weight | Every comparison |
| `TCHOL_J` | Total cholesterol | `lab.cholesterol-total` |
| `HDL_J` | HDL cholesterol | `lab.hdl-cholesterol` |
| `GHB_J` | Glycohemoglobin | `lab.hemoglobin-a1c` |
| `CBC_J` | Complete blood count | `lab.wbc`, `lab.rbc`, `lab.hemoglobin`, `lab.hematocrit`, `lab.mcv`, `lab.mch`, `lab.mchc`, `lab.rdw`, `lab.platelets` |
| `BMX_J` | Body measures | A rule on BMI or waist |
| `DR1TOT_J` | What each person ate on one day, as nutrient totals | A rule on food |

Each file is kept encrypted, as a `reference.file` record with its SHA-256. You sign for having kept it. That does not make it CDC's claim: anyone can check the SHA-256 against the copy CDC serves. The importer says whether the file is the one CDC served on 2026-11-09 ([the digests](../internal/nhanes/nhanes.go)). A file it does not know still works, and every comparison with it says so.

## Compare

```
bean compare -sex male -age 30-39 -type 'lab.*'
bean compare -sex female -age 40-49 3fa9c1
```

`-sex` and `-age` are required. bean does not know yours, and never guesses a cohort. `-age` is a range of whole years, both ends included; 80 means 80 and over, because NHANES records it so. Give record ids, or `-type` for every current reading of a type.

```
Compared with NHANES 2017-2018: male, age 30-39.
READING                VALUE      PERCENTILE  LOWER  SAME  MIDDLE HALF  MEDIAN  N    STATUS     ID
lab.cholesterol-total  187 mg/dL  46          44.2%  3.5%  164 to 213   188     339  qualified  fca23ea91778
```

- **PERCENTILE**: the weighted share of the cohort with a lower value, plus half of those with the same value.
- **LOWER**, **SAME**: those two shares.
- **MIDDLE HALF**, **MEDIAN**: the 25th, 75th and 50th percentile of the cohort.
- **N**: the people in the cohort who have a value. Read it before the percentile.

The numbers in this example are statistics of the public files, not anyone's reading.

### People like you: more rules

Each `-where` narrows the cohort by one NHANES variable: `<FILE>.<VARIABLE>=<value>` or `=<from>-<to>`, both ends included. A person with no value for a rule's variable is left out.

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

Every rule shrinks the cohort. Sex and a ten-year age range leave about 340 people. Add ethnicity and BMI and about 40 are left. Below 30 bean gives no number, and below 100 the result says that the cohort is small. Widen a range before you trust a narrow cohort.

## What a result means, and its status

Each comparison has one of three statuses.

- **not comparable**: no number is given, and the row says why. bean has no reference data for the type; the reading is text, or in another unit; a file is not in the log; the cohort has fewer than 30 people.
- **qualified**: a number, with the reasons it is not a like-for-like measurement. Every comparison of a lab reading with NHANES is qualified:
  - the lab and NHANES used their own instruments and methods;
  - NHANES is a sample of people living in the United States in 2017-2018, not a reference range, and like you only in what the cohort states;
  - you chose the cohort: bean does not know whether you belong to it.

  And where it applies: ages of 80 and over are recorded as 80; a cohort chosen by one day's food says little about what people usually eat; the cohort is small; bean does not know a file.
- **comparable**: the same quantity, measured the same way. bean does not give this status to anything yet.

Limits beyond those:

- No interval of uncertainty is computed. The survey's design variables are in `DEMO_J` for it.
- NHANES fills in a number for a value below a detection limit, and marks it in a comment-code variable. The files of the twelve tests here have no such variable; a test whose file has one needs a rule for it first.
- The twelve tests are measured in everyone examined. Tests that need fasting (glucose, triglycerides, LDL) or that NHANES measured in part of the sample are not here yet: they need other weights.
- A lab's name for a test maps to an NHANES variable by a table in the code ([`internal/nhanes`](../internal/nhanes/nhanes.go)). Each row is a claim that the two measure the same thing in the same unit. Today the table has the names the Labcorp report prints.

## Save, share, compute again

```
bean compare -sex male -age 30-39 -type 'lab.*' -save
```

`-save` writes one `comparison.percentile` record per result, signed by you. Without it nothing is written: try cohorts freely, and save what you will act on or share. The same inputs twice add nothing.

The record holds the percentile, and in `context`:

- `reading`: the id of your reading. The value is not copied, so deleting the reading leaves no copy of it here. Purge the comparison too if its percentile should go.
- `reference`: the variable, its label and unit, the weight, and each file by data set, SHA-256, size and the id of your record that keeps it.
- `cohort`: every rule, with the label the file gives its variable.
- `method`: `nhanes-weighted-percentile`, version 1.
- `result`: n, the three shares and five percentiles.
- `status` and `reasons`.

Anyone with the record, the reading and the files from CDC can compute it again without bean:

```
bean ls -json > records.jsonl
python3 tools/compare/reproduce.py records.jsonl DEMO_J.XPT TCHOL_J.XPT
```

[`tools/compare/reproduce.py`](../tools/compare/reproduce.py) is a second implementation in Python with pandas. It shares no code with bean and reads only what the record says. It checks each file's SHA-256 and every number of the result, and exits 1 if one differs. `make compare` runs it against bean on synthetic files in CI, and on CDC's files when `BEAN_NHANES_DIR` names a folder of them.

An assistant can read saved comparisons through `bean mcp` like any record. It cannot write one.

## The method, version 1

Inputs: the value v of the reading; the test's file and variable; `DEMO_J`; the rules; the file of each rule.

1. Take the rows of the test's file in file order. Skip a row with no value.
2. `DEMO_J` must have a row with the same `SEQN`. If not, stop: the files are not of one release.
3. For each rule, find the row of its file with the same `SEQN`. Skip the row if there is none, if it has no value of the variable, or if the value is below the rule's minimum or above its maximum. Sex is the rule `RIAGENDR` = 1 (male) or 2 (female); age is a rule on `RIDAGEYR`.
4. The weight is `WTDRD1` of `DR1TOT_J` if a rule names that file, else `WTMEC2YR` of `DEMO_J`. CDC's notes on the dietary file say to use the day one weight whenever day one nutrient data are used, alone or with examination data. Skip the row if the weight is missing or not above zero.
5. W is the sum of the weights of the rows left; B of those with a value below v; E of those with a value equal to v. Add in file order, in IEEE 754 doubles.
6. lower = 100 B / W, same = 100 E / W, percentile = 100 (B + E/2) / W.
7. The q-th percentile of the cohort is the smallest value x such that the weights of the rows with a value of at most x reach q W / 100. Add the weights of one value in file order, then the values in rising order.
8. Round each share to one decimal: floor(10 y + 0.5) / 10.
9. With fewer than 30 rows left, give n and no other number.

A file is SAS transport, version 5. A number that an IEEE double cannot hold exactly is refused. bean's reader gives the same bits as pandas for every value of the ten CDC files it was tried on, but for zero, which pandas reads as 5.4e-79.

A change to any step is a new method version.
