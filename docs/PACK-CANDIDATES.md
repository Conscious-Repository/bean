# Pack candidates: public data that is worth a pack

This is for the owner, and for anyone who builds a reference pack. It lists public data that a pack could be built from, and what each source allows. [REFERENCE.md](REFERENCE.md) is the format, and [REFERENCE-PLAN.md](REFERENCE-PLAN.md) says what the format cannot hold.

Nothing here is built, and nothing is hosted. No pack was made, and no code changed. The owner asked for this search on 2026-11-19, in these words: "need to find more packs that would be valuable to host". DECISIONS 338 records them, in the pull request of the host plan.

A candidate is a distribution in a sample, with its n and its caveats. No candidate says whether a value is healthy, and no pack will.

## How to read this

- **Read on 2026-11-19.** Each terms line and each page was read on that day, from the publisher's own site. The address is beside each quote.
- **Measured.** A number with the word "measured" comes from the publisher's own files, fetched on that day into a scratch folder. No file of them is in this repository.
- **The cohorts** are the 14 of the packs on record: each sex, by ten years of age from 20, and 80 and over.
- **Marked** means that the file marks a value as below the limit of detection, and holds a fill value in its place.
- **The cap** is rule 4 of decision 2 in REFERENCE-PLAN.md. A builder takes the source's fill value only where marked people are under 1% of each cohort's weight.
- **Not confirmed** means that the author could not read the fact from the publisher's own site. Such a line is a lead and no more.
- **The reports** are the two layouts that `bean-import-lab` reads. Their fixtures show which tests a real report prints, and in which unit.

## The ranked list

The rank is by what a person with a routine lab report can compare today. It is the author's proposal, and the owner decides.

| Rank | Candidate | What a person gains | Fit to `pack/0` | What it waits for |
|---|---|---|---|---|
| 1 | The fasting tests of the US survey: glucose, triglycerides, LDL, insulin | Four tests that are on almost every report | Holds it | The decision on a weight for each test |
| 2 | Iron, iron binding capacity and transferrin saturation, same survey and years | Three tests of the iron panel, which the reports print | Holds it | A row in the builder, by version 2 of its rule |
| 3 | The cycle of August 2021 to August 2023, same survey | Testosterone, SHBG, magnesium, and newer people for every test | Holds most of it | A weight in each file, and a second set of digests |
| 4 | Blood pressure and pulse by an electronic cuff, same survey | A home cuff's reading | Holds a single reading | A real reading to take the type and unit from |
| 5 | Percentile tables of the Canadian survey, from another publisher | Most routine tests, in SI units, and a second publisher | Does not hold it | A new kind of cell, which is a new format |
| 6 | Thyroid tests, vitamin B12 and PSA, from earlier cycles of the US survey | Tests that later cycles lack | Holds it | The owner's word on data that is 12 to 17 years old |
| 7 | Vitamin D of the US survey | One test of many reports | Holds it in nmol/L only | A rule for a second unit, with a factor from a stated source |
| 8 | Body measures and hours of sleep, same survey | A scale, a tape, a watch | Holds the body measures | A real export that shows the unit |

The four tests with a mark are not in this list. ALT, GGT, total bilirubin and C-reactive protein are decided already (REFERENCE-PLAN.md, decision 2), and they are the next pack.

The files of 2017 to March 2020 have a section and no rank: they add people and no test. Three sources are no candidate today: the survey of England, the survey of Korea, and tables in journals. The section near the end says why.

## What the owner must decide

1. **Whether this rank is the order of work.** The author's reading of "valuable" is: tests that real reports print and that no pack holds.
2. **The fasting tests: which cycle, and which rule for the weight.** REFERENCE-PLAN.md keeps this as a decision of its own. The numbers for it are in the section on the fasting tests.
3. **Whether a second cycle counts as a second source.** ROADMAP.md says "A second source, from another publisher". By that line a second cycle does not start the person's own list of packs.
4. **Whether the Canadian tables are the case that starts the next format.** They need a cell of percentile points. REFERENCE-PLAN.md names such a table as one thing that starts a new format.
5. **What DECISIONS 317 (d) asks before a new `pack/0` pack is hosted.** It says that the next format is decided before any new pack is hosted. The author reads decision 1 of REFERENCE-PLAN.md as that decision: no new format now. So a candidate that `pack/0` holds can be hosted, and one that it cannot hold waits. This is the author's reading, and the owner confirms it or not.
6. **A second unit for a measure.** A reader converts no unit. Vitamin D and every Canadian table are in SI units, and the two reports print US units. The builder's rule must state each factor and its source.
7. **Which reading of blood pressure stands for a person:** the first of three, or their mean.
8. **The lines that the US publisher asks of anyone who hands its material on.** There are four, and the next section quotes them. A page beside a hosted pack could carry them.
9. **Who reads the sources that this search could not open.** The sites of Korea and England need a browser, and one may need an account.

## The US survey: terms and files

The candidates of ranks 1 to 4 and 6 to 8 are files of NHANES, the survey of the packs on record. Its terms and its file addresses are the same for each, so they are here once.

**Terms, read from two pages.** The agency's page on its materials, `https://www.cdc.gov/other/agencymaterials.html`:

```
Most of the information on the CDC and ATSDR websites is not subject to copyright, is in the public domain, and may be freely used or reproduced without obtaining copyright permission.
```

That page then lists four requirements for such use. In short: name the agency as the source, and say that the use does not imply its endorsement. Two more are quoted here.

```
3) You may not change the substantive content of the materials; and
4) You must state that the material is otherwise available on the agency website for no charge.
```

The data user agreement of the survey's center, `https://www.cdc.gov/nchs/policy/data-user-agreement.html`, dated September 17, 2024:

```
1.  Use the data in this dataset for statistical reporting and analysis only.
2.  Make no attempt to learn the identity of any person or establishment included in these data.
3.  Not link this dataset with individually identifiable data from other NCHS or non-NCHS datasets.
```

- **Handing on.** The first page allows it for material in the public domain. The second page limits the use and has no line on handing on. DECISIONS 313 found the same.
- **The owner's word.** He read the survey's terms and gave the word to host the two packs on record (DECISIONS 314). A new pack from the same source is under the same terms.
- **What is new here.** The four requirements of the first page were not in DECISIONS before. Decision 8 above asks about them.

**A stable address.** Each file is at `https://wwwn.cdc.gov/Nchs/Data/Nhanes/Public/<year>/DataFiles/<NAME>.xpt`. The year is the first year of the cycle: 2017, 2021, 2011. The documentation is beside it, as `<NAME>.htm`.

**Whether a file changes.** The publisher states no digest of a file. Each documentation page has a line "First Published" and a line "Last Revised". The page of `TRIGLY_J` says "First Published: December 2020" and "Last Revised: March 2021". So a file can change after it is out, and the page then says so. bean's own digests are what finds a change (COMPARE.md).

## The fasting tests of the US survey

**What it holds.** NHANES 2017-2018, three files. `GLU_J` holds fasting glucose in mg/dL. `TRIGLY_J` holds triglycerides in mg/dL and LDL cholesterol by three equations. `INS_J` holds insulin in uU/mL.

The three LDL values are by Friedewald, by Martin and Hopkins, and by "NIH equation 2". One report prints "LDL Chol Calc (NIH)". Which equation a report used is part of the `same` claim.

**Who was measured.** The page of `GLU_J` says: "Participants aged 12 years and older, who were examined in the morning session, were eligible." People who had not fasted for 8 to less than 24 hours have a weight of 0.

**n, measured.** Adults of 20 and over with a value and a weight above zero, in the 14 cohorts:

| Test | Variable | Adults | Smallest cohort | Cohorts under 100 | Marked |
|---|---|---|---|---|---|
| Fasting glucose | `LBXGLU` | 2295 | 78 | 2 | no such variable |
| Triglycerides | `LBXTR` | 2258 | 77 | 2 | no such variable |
| LDL, NIH equation 2 | `LBDLDLN` | 2251 | 77 | 2 | no such variable |
| LDL, Friedewald | `LBDLDL` | 2233 | 77 | 2 | no such variable |
| Insulin | `LBXIN` | 2251 | 76 | 2 | 5 adults, in 3 cohorts |

For scale: total cholesterol of the pack on record has 4937 adults, and its smallest cohort has 175. So a fasting cell rests on about half as many people.

**Insulin and the cap.** The limit is 1.0 uU/mL, by the page of `INS_J`. The fill value is 0.71, and the lowest value with no mark among these adults is 1.2. The marked people are at most 0.70% of a cohort's weight. That is under the cap.

**File stability.** `GLU_J` and `INS_J`: first published August 2020, not revised. `TRIGLY_J`: revised March 2021.

**Fit to `pack/0`.** The format holds these tables as it is. A cell is values and weights, and it does not know which weight was summed. What is missing is outside the format:

- **The weight.** The page says that `WTSAF2YR` of the test's own file "should be used when analyzing these data". The builder's rule has one weight today, `WTMEC2YR`.
- **The comparison with the files.** Step 4 of the method in COMPARE.md fixes the weight. A weight from the test's file is a new version of that method, or the tests are in packs only.
- **The words.** The cohort "male, age 30-39" is then other people for these tests: those who came in the morning and had fasted. The measure's caveat must say so.
- **The reading.** bean does not know whether the person fasted. A caveat says that the cohort did.

**What a builder needs.** Three digests on record, a row for each test with its weight, and the caveats. Then a `same` entry for each name that the two reports print. Then the owner's decision, which ROADMAP.md keeps open as "a decision of their own".

**A choice of cycle.** The same tests are in two more releases, and the sections below have their numbers. In the file of 2017 to March 2020 no cohort is under 100. In the cycle of 2021 to 2023 insulin is over the cap.

## Iron, binding capacity and saturation

**What it holds.** NHANES 2017-2018, file `FETIB_J`: serum iron in ug/dL, total iron binding capacity in ug/dL, transferrin saturation in percent, and UIBC in ug/dL. Both reports print an iron panel, and one fixture shows these units.

**Who was measured.** The page says: "Examined participants aged 12 years and older were eligible." So the weight is the examination weight, as in the packs on record.

**n, measured.** Iron: 4929 adults, smallest cohort 174. Binding capacity and saturation: 4908 each. No cohort is under 100. Saturation is recorded in whole percent.

**The cap.** Iron, binding capacity and saturation have no mark. UIBC has 5 marked adults, and they reach 2.45% of one cohort's weight. So UIBC is over the cap and waits.

**The rule on analyzers.** The biochemistry profile also holds an iron value, and it failed that rule at 0.21 (COMPARE.md). This file is another one, of frozen serum. Its page names one analyzer and has no table of a difference between two. So the rule of DECISIONS 309 has nothing to test here. A reviewer should weigh that before the row is added.

**File stability.** First published February 2020, not revised.

**Fit to `pack/0`.** Holds it as it is, for three of the four tests.

**What a builder needs.** One digest, three rows, and the `same` entries from the reports. It can go into the next pack, with the four decided tests.

**More files of these years that fit the same way.** Each was measured, and none was checked against a report's printed name.

- **Urine albumin to creatinine ratio**, `ALB_CR_J`, in mg/g: 5127 adults, smallest cohort 173. The ratio has no mark.
- **Serum folate**, `FOLFMS_J`, in ng/mL: 3080 adults, smallest cohort 94. It has a weight of its own, `WTFOL2YR`, so it shares the decision of the fasting tests.
- **Transferrin receptor**, `TFR_J`: 4978 adults. No report at hand prints it.

## The cycle of August 2021 to August 2023

**What it holds.** The newest cycle of NHANES, in files that end in `_L`. It repeats most tests of the packs on record. It adds a hormone file, `TST_L`, and magnesium in the biochemistry profile.

**n, measured.** `DEMO_L` has 11,933 people. Adults with a value and a weight above zero:

| Test | File and variable | Adults | Smallest cohort | Marked adults | Largest share of a cohort's weight |
|---|---|---|---|---|---|
| Total cholesterol | `TCHOL_L`, `LBXTC` | 5498 | 143 | no such variable | |
| Testosterone, total, ng/dL | `TST_L`, `LBXTST` | 5545 | 144 | 7, in 4 cohorts | 0.69% |
| SHBG, nmol/L | `TST_L`, `LBXSHBG` | 5569 | 145 | 0 | |
| DHEAS, ug/dL | `TST_L`, `LBXDHE` | 5472 | 145 | 2, in 2 cohorts | 1.77% |
| Estradiol, pg/mL | `TST_L`, `LBXEST` | 5532 | 145 | 286, in 13 cohorts | 17.93% |
| FSH, mIU/mL | `TST_L`, `LBXFSH` | 5569 | 145 | 62, in 9 cohorts | 7.35% |
| Vitamin D, nmol/L | `VID_L`, `LBXVIDMS` | 5596 | 147 | 0 | |
| Magnesium, mg/dL | `BIOPRO_L`, `LBXMAGN` | 5442 | 143 | no such variable | |
| ALT, IU/L | `BIOPRO_L`, `LBXSATSI` | 5438 | 143 | no such variable | |
| GGT, IU/L | `BIOPRO_L`, `LBXSGTSI` | 5444 | 143 | 1 | 0.29% |
| Total bilirubin, mg/dL | `BIOPRO_L`, `LBXSTB` | 5443 | 143 | 105, in 13 cohorts | 5.85% |
| C-reactive protein, mg/L | `HSCRP_L`, `LBXHSCRP` | 5594 | 147 | 21, in 8 cohorts | 1.93% |
| Fasting glucose | `GLU_L`, `LBXGLU` | 2958 | 76 | no such variable | |
| Triglycerides | `TRIGLY_L`, `LBXTLG` | 2838 | 72 | no such variable | |
| Insulin | `INS_L`, `LBXIN` | 2813 | 72 | 1 | 1.53% |

**What the table shows.**

- **Decision 2 does not carry over as it is.** In this cycle bilirubin, C-reactive protein and insulin are over the cap. In 2017-2018 each is under it.
- **Estradiol is the case that REFERENCE-PLAN.md names.** A reviewer there spoke of a test where 15% of people are below a limit. Here it is 17.93% of one cohort's weight.
- **One person can break the cap.** One adult has a marked insulin value, and carries 1.53% of the weight of that cohort.
- **Testosterone is under the cap,** and both reports print a testosterone.
- **Names change between cycles.** Triglycerides are `LBXTR` in 2017-2018 and `LBXTLG` here. A builder cannot reuse a row.

**The weight.** Each blood file of this cycle has a weight of its own, `WTPH2YR`. The page of `HSCRP_L` says:

```
The phlebotomy weight should be used for analyses that use variables derived from blood analytes, and is included in all relevant data files.
```

So a weight for each file is needed for every blood test of this cycle, and not only for the fasting tests.

**File stability.** `DEMO_L`, `HSCRP_L` and `BPXO_L`: first published September 2024, not revised. `TST_L`: October 2024. The list of files dates `TRIGLY_L` and `BIOPRO_L` September 2025.

**Fit to `pack/0`.** Holds every test that is under the cap or has no mark. Estradiol, FSH, DHEAS, bilirubin, C-reactive protein and insulin wait for a format that holds a limit.

**What a builder needs.** A second table of rows, with the cycle's own variable names, weights and digests. A pack of this cycle is a second pack with its own title. Its caveat names the years, as the first pack's does.

**What it does not give.** It is the same publisher. So it is no "second source, from another publisher" in the words of ROADMAP.md.

## The files of 2017 to March 2020

**What it holds.** The files that start with `P_`. The page of `P_DEMO` says why they exist:

```
Because the collected data from 18 locations were not nationally representative, these data were combined with data from the previous cycle (2017-2018) to create a 2017-March 2020 pre-pandemic data file.
```

**n, measured.** `P_DEMO` has 15,560 people. Adults: total cholesterol 7921, smallest cohort 261. Fasting glucose 3801, smallest cohort 110. Triglycerides 3737. Insulin 3718, with 5 marked at 0.66% at most.

**What it adds.** Cells that are about 1.6 times as large. For the fasting tests no cohort is under 100.

**What it costs.**

- **The people of 2017-2018 are in it.** A pack of these files and a pack on record are not two samples.
- **Its weights are its own:** `WTMECPRP`, and `WTSAFPRP` for the fasting tests. The page says that they give no true result for either part alone.
- **C-reactive protein is over the cap here:** 60 marked adults, and 2.42% of one cohort's weight.
- **The hormone file `P_TST` has no total testosterone.** Its estradiol has 302 marked adults, and 20.32% of one cohort's weight.

**File stability.** `P_DEMO`: first published May 2021, not revised. `P_TST`: December 2024.

**Fit to `pack/0`.** Holds it, with the same limits as the cycle above.

**What a builder needs.** The same as for a second cycle. The author ranks it below that cycle: it adds people and no new test.

## Blood pressure and pulse

**What it holds.** NHANES 2017-2018 measured blood pressure in two ways. `BPX_J` is by a person with a mercury device. `BPXO_J` is by an electronic cuff on the upper arm. Later cycles have the electronic cuff only.

**How it was measured.** The page of `BPXO_J` says:

```
After resting quietly in a seated position for 5 minutes; three consecutive BP measurements (systolic and diastolic) were taken 60 seconds apart using a digital upper-arm electronic blood pressure measurement device, Omron HEM–907XL.
```

People of 8 years and older were eligible.

**n, measured, for the first reading.**

| File | Adults | Smallest cohort | Step | Zeros |
|---|---|---|---|---|
| `BPXO_J`, electronic, 2017-2018 | 4573 | 146 | 1 mm Hg | none |
| `BPX_J`, mercury, 2017-2018 | 4659 | 163 | 2 mm Hg | 37 adults have a diastolic value of 0 |
| `BPXO_L`, electronic, 2021-2023 | 5863 | 152 | 1 mm Hg | none |
| `P_BPXO`, electronic, 2017 to March 2020 | 7648 | 221 | 1 mm Hg | none |

The pulse of the electronic cuff has 3905 adults in `BPXO_J` and 5863 in `BPXO_L`. The page of `BPX_J` says that its values "can be even numbers only", and "Diastolic BP can be zero."

**File stability.** `BPXO_J`: first published April 2021, not revised. `BPX_J`: February 2020, not revised.

**Fit to `pack/0`.**

- **The electronic cuff fits.** A first reading is a whole number above zero. A home cuff prints whole numbers too, so a reading is on the source's step.
- **The mercury file does not fit.** A value of 0 is real there, and `pack/0` holds no zero (REFERENCE-PLAN.md, F1).
- **A mean of three readings** has steps of a third. `pack/0` holds it. Which of the two stands for a person is the builder's rule, and decision 7 above.

**What a builder needs.**

- **A real reading.** REFERENCE.md says: "Take each `same` entry from a real reading." No importer of bean writes a blood pressure today. `bean-import-apple-health` reads sixteen types, and none is blood pressure or heart rate. So the first `same` entry would come from a reading that a person typed.
- **Caveats.** The cohort sat and rested for five minutes, and staff used one model of cuff. A wrist device, or a reading after a walk, is another thing.
- **Pulse is not a resting heart rate of a watch.** A watch gives a value from sleep or from a whole day. The survey gives a pulse on a chair at an exam.

## Thyroid tests, vitamin B12 and PSA, from earlier cycles

TSH, vitamin B12 and PSA are in no file from 2017 on. Each is in an earlier cycle. Both fixtures print TSH and vitamin B12, and one prints PSA.

| Test | File | Years | Adults, measured | Smallest cohort | Note |
|---|---|---|---|---|---|
| TSH in uIU/mL, free T4 in ng/dL, free T3 in pg/mL | `THYROD_G` | 2011-2012 | 1610 | 39 | 4 cohorts are under 100 |
| Vitamin B12, pg/mL | `VITB12_H` | 2013-2014 | 5316 | 149 | no mark |
| PSA, total, ng/mL | `PSA_F` | 2009-2010 | 1746 men | | no man under 40 has a value |
| Apolipoprotein B, mg/dL | `APOB_I` | 2015-2016 | 2253 | 54 | the fasting weight |
| Testosterone, total, ng/dL | `TST_I` | 2015-2016 | 5134 | 150 | 3 marked, 1.16%: over the cap |

**The thyroid file.** Its page says: "Examined participants aged 12 years and older from a one-third sample." Its weight is its own, `WTSA2YR`. It was first published January 2014 and revised October 2014. Its label for total T4 gives the unit as "ug/mL", so a builder checks each label against the page.

**What was read.** Only the page of `THYROD_G` was read. The pages of the other four files were not, so who was eligible for each is not confirmed.

**Fit to `pack/0`.** Holds it. The thyroid tests and apolipoprotein B need a weight from the test's own file.

**What a builder needs.** A set of files and digests for each cycle, and a caveat that names the years. The value is real: both fixtures print TSH, and no later file of this survey that was listed has it.

**What weighs against it.** The people were measured 12 to 17 years ago, on the instruments of those years. Four thyroid cells are under 100. The owner decides whether such a pack is worth hosting.

## Vitamin D

**What it holds.** `VID_J` for 2017-2018 and `VID_L` for 2021-2023: total 25-hydroxyvitamin D, in nmol/L. By the page of `VID_J`, people of 1 year and older were eligible.

**n, measured.** 4983 adults in 2017-2018, smallest cohort 176. 5596 adults in 2021-2023. The total has no marked value in either file.

**A fill inside the total.** The total is the sum of two forms, D2 and D3. In 2017-2018, 3959 of the 4983 adults have a D2 below its limit. Their total then holds a fill value for that part, and the total has no mark for it.

**File stability.** `VID_J`: first published April 2022, not revised.

**Fit to `pack/0`.** Holds it, in nmol/L.

**What stands in the way.** The fixture of one report prints vitamin D in ng/mL. The page of `VID_J` gives nmol/L only, and states no factor. By REFERENCE-PLAN.md a publisher builds a second measure in the second unit. The rule then states the factor, its source and the rounding. That source would not be the file's own page.

**What a builder needs.** Decision 6 above, then two digests and a row. A caveat says what the total holds for a person with no measured D2.

## Percentile tables of the Canadian survey

**What it is.** Statistics Canada runs the Canadian Health Measures Survey. It publishes tables of percentiles from it, and no file with a row for each person was found. This is the one source here from another publisher that could be read in full.

**What it holds, measured from the table files.** Each table gives the 5th, 10th, 25th, 50th, 75th, 90th and 95th percentile, and a mean or other middle value. Each estimate has the two ends of a 95% interval. The groups are both sexes, females and males, by ages 20 to 39, 40 to 59 and 60 to 79. Children have groups of their own.

| Table | Title | Measures | Years |
|---|---|---|---|
| 13-10-0326-01 | Cardiovascular health measures | Blood pressure, heart rate, fasting glucose, HbA1c, C-reactive protein, total, HDL and LDL cholesterol, triglycerides, fasting insulin, apolipoprotein A1 and B, homocysteine | 2009 to 2024 |
| 13-10-0332-01 | Chemistry panel | ALT, AST, alkaline phosphatase, GGT, albumin, total protein, creatinine, urea, uric acid, calcium, magnesium, phosphorus | 2009 to 2024 |
| 13-10-0333-01 | Complete blood count | Hemoglobin, hematocrit, mean corpuscular volume, platelets, white blood cells | 2009 to 2024 |
| 13-10-0334-01 | Hormone measures | TSH, free thyroxine, estradiol | 2013 to 2024 |
| 13-10-0336-01 | Nutritional status | Ferritin, vitamin B12, vitamin D, red cell folate, and more | 2009 to 2024 |
| 13-10-0324-01 | Physical fitness measures | Predicted maximal oxygen consumption, grip strength, and two more | 2009 to 2019 |

The page of table 13-10-0319-01, on body measures, also names percentiles. Its file was not read.

**n.** The tables give no number of people for a row. The survey's record says: "The target sample size for cycle 7 is 6,500 respondents". That is for all ages, so an adult group of one sex has far fewer.

**Terms.** The publisher's page of terms, `https://www.statcan.gc.ca/en/reference/terms-conditions`:

```
The Statistics Canada Open Licence governs the use of most data products and other materials that are published by Statistics Canada. This licence allows you to use Statistics Canada information without restrictions on sharing and redistribution, for commercial and non-commercial purposes.
```

The licence itself, `https://www.statcan.gc.ca/en/terms-conditions/open-licence`, dated 2025-07-10, grants a licence to:

```
use, reproduce, publish, freely distribute, or sell the Information;
```

It asks for a line of source. For a product that changes the data, the line is:

```
Adapted from Statistics Canada, name of product, reference date. This does not constitute an endorsement by Statistics Canada of this product.
```

So handing on is allowed in plain words. This is the clearest terms line of the search.

**A stable address.** Each table is one file: `https://www150.statcan.gc.ca/n1/tbl/csv/<number>-eng.zip`, such as `13100332-eng.zip`. Each table also has a DOI.

**Whether a file changes.** It does, and the page says so. One address holds every year, so a new cycle changes the bytes. The pages give "Release date: 2026-03-10". The page of the chemistry panel has a note of correction:

```
On November 13, 2019 the results for phosphorus and gamma-glutamyl transferase for 2017 have been corrected.
```

The publisher states no digest. A builder names the file by its own SHA-256, as for every source.

**Fit to `pack/0`.** It does not fit. `pack/0` has one kind of cell, a table of weights, and a reader refuses any other. These tables are the first shape of "Other kinds of reference" in REFERENCE.md: published percentile points, and no data of each person. A result may then say "between the 25th and the 50th percentile", and never one number.

**What a next format would need for it.**

- **A kind of cell for points:** each percentile with its value, and room for the two ends of the interval.
- **A cell with no n.** `pack/0` requires n, and these tables have none.
- **A method of its own,** with a version. It says which two points a value lies between, and what it says outside the outer points.
- **A cohort that the source defines.** The age groups are of twenty years, and not the ten of the packs on record.

**Units.** The tables are in SI units: mmol/L, g/L, umol/L. The two reports print mg/dL and g/dL. So no reading of those reports matches without a second measure and a factor. A report from a Canadian lab would match as it is. One trap for a builder: the file writes some units with a capital I for the letter l, as in "mmoI/L".

**The files with a row for each person.** The pages that were read name no public file of that kind for this survey. Whether one exists, and under which terms, is not confirmed.

**Why it ranks fifth and not first.** Its value is high: a second publisher, clear terms, and most routine tests. Its cost is the highest of the list: a new format, a new method, and units that the first users' reports do not print.

## Body measures and hours of sleep

**What it holds.** `BMX_J` of 2017-2018 holds body mass index, waist and standing height. Weight is in the pack on record. `SLQ_J` holds hours of sleep, as each person reported them.

**n, measured.** Body mass index: 5175 adults, smallest cohort 171. Waist in cm: 4937. Height in cm: 5185. Hours of sleep on workdays: 5522 adults have a value.

**Fit to `pack/0`.**

- **Body mass index, waist and height fit.** REFERENCE.md says that height waits "until a real export shows which unit a reading has". The same holds for the other two.
- **Hours of sleep do not fit well.** People answered in steps of half an hour, and the adults have 24 distinct values. A watch reports minutes. That is a reading finer than the source's step, which REFERENCE-PLAN.md names as a case for a new format.
- **Hours of sleep are a questionnaire.** Their weight is the interview weight, and a person's own report is not what a watch measures.

**What the Apple Health importer writes.** It reads height, body mass, steps, walking measures and sleep analysis. Steps are single samples, and bean adds up nothing. So no daily count of steps exists as a reading today.

**Steps, heart rate at rest, and fitness.** This search found no open file of people's daily steps or resting heart rate. The Canadian table of fitness has a predicted oxygen consumption, to 2019. Tables in journals are below.

## Sources that are no candidate today

**The Health Survey for England.** Not confirmed, in two parts.

- **Its files.** They are held by the UK Data Service. That site's page on access says that safeguarded data need a registration and its End User Licence. The licence, version 16.00 of 25 February 2026, at `https://ukdataservice.ac.uk/app/uploads/cd137-enduserlicence.pdf`, has this clause 4:

```
Not to give access to the Data Collection(s), in whole or in part, or to any Dataset(s) derived from the Data Collection(s) (including synthetic Dataset(s)), except to Registered Users
```

  A pack is a dataset that is derived from its source. So under this licence a pack could not go on a public host. Whether the survey's files are under this licence is not confirmed: the catalogue page did not load.
- **Its published tables.** The publisher's site refused each fetch. So their licence and what they hold are not confirmed.

**The survey of Korea, KNHANES.** Not confirmed. Its site builds each page in the browser, and a plain fetch gets no text. Its terms, its files and its tests were not read. A person with a browser can read them.

**The Canadian survey's files of each person.** Not confirmed, as the section on its tables says.

**Australia.** The statistics office's page, `https://www.abs.gov.au/website-privacy-copyright-and-disclaimer`, says: "All material presented on this website is provided under a Creative Commons Attribution 4.0 International licence". It lists exceptions. Its tables of blood tests were not read.

**Japan.** The terms of the government's statistics portal, `https://www.e-stat.go.jp/en/terms-of-use`, allow copying and changing with a line of source, and commercial use. The tables of the national health and nutrition survey were not read.

**A report of percentile points by the US survey's center.** "Anthropometric Reference Data for Children and Adults: United States, 2015–2018", at `https://www.cdc.gov/nchs/data/series/sr_03/sr03-046-508.pdf`. Its terms line:

```
All material appearing in this report is in the public domain and may be reproduced or copied without permission; citation as to source, however, is appreciated.
```

Its tables give the 5th to the 95th percentile of weight, height, body mass index and waist. It is free to hand on. It adds nothing for a person: the files behind it are public, and a table of weights says more. It could serve as a second test case for a cell of points.

**Leads that were not read.** Each fetch failed or gave no text. None is confirmed.

- The public data browser of the "All of Us" program of the US.
- The repository of the World Health Organization for its surveys of risk factors.
- The Demographic and Health Surveys program.
- The public use files of the German health institute.
- The Nordic project on reference intervals of 2000.
- Counts of genetic variants, which ROADMAP.md names as a lead. They are not a lab value or a device's reading.

**Tables in journals.** Norms of fitness, of heart rate variability and of daily steps are mostly in journal papers. ROADMAP.md says that they "may not be free to hand on". This search read no such terms, so that stands.

## What this search did not do

- It built no pack and wrote no row of a builder.
- It did not check a test's name on a report against a variable, except where a section says so.
- It read only the documentation pages that a section quotes. For the other files, who was eligible is taken from the file alone.
- It counted with one program, in Python. Nobody has counted a second time.
- The digests of the files that were fetched are not on record. A builder takes them again, from a second network.
