# External reference packs

Release 0.8.4 and later read `pack/2`, `pack/3` and `pack/4`. Release 0.8.3 reads `pack/0` and `pack/1` ([UPGRADING.md](../UPGRADING.md)).

## Routine ratios, iron capacity and assay sources

NHANES 2017–2018 supplies the paired inputs for three dimensionless ratios and measured serum UIBC. The [source review](https://stele.aion.bio/ratios-iron/review.html) documents the rounded ratio definitions, signed panel matches and UIBC detection flags. This pack needs the `pack/4` reader of release 0.8.4 or later.

Pack SHA-256: `67a39c4ed9cd00f0b84dd78c154edd104e5d4d9b0f140dfa96eb5d91ca93771f`

The [assay source guide](https://stele.aion.bio/assays/review.html) is a separately selectable JSON directory of laboratory documentation. Match the printed test and provider to discover sources. Apply reference text from the original signed report with its original unit, bound and collection conditions. A source-guide match cannot produce a population percentile.

Guide SHA-256: `e1805bc10b8dbd8483ff3f0163ace4c4b0bdc842708810ecaa3d9f38b053a15b`

## Cardiometabolic measures

This NHANES 2015–2016 pack covers fasting serum apolipoprotein B, calculated non-HDL cholesterol and two unit displays of the same HbA1c-derived estimated average glucose. ApoB uses positive fasting-subsample weights and excludes values below the codebook detection limit. The derived measures use exam weights. [Codebooks, equations, eligibility and rebuild](https://stele.aion.bio/cardiometabolic/review.html).

SHA-256

```text
80a43c7bec2250c934ace36dc7f1de5a938d4a27f3f1bfbf946c186396f57bb4
```

## Evidence-qualified core packs

Stele’s current 13-, 20- and 23-measure releases use `pack/2`. Clinical rows require the signed source, printed test and order panel. Imported body mass requires the Apple Health XML source and HealthKit body-mass identifier. A white cell count coded “400 and over” has been excluded from the exact-count distribution.

| Measures | SHA-256 |
|---:|---|
| 13 | `a646d6ec1b954fae02b12f1895a5e67b47aea43d03a734eb2ae939e5751d13a1` |
| 20 | `7460c20ceab711ded88e9ad7f0dc676f5708d6f7ccf258f9d8c02fd15bf52692` |
| 23 | `05dc27e6e78429e80b373cf1f04e9d4ee2ca4925f42391c43f6fea3d1bd5f93e` |

Use the [source and matching review](https://stele.aion.bio/core/review.html) and verify the chosen download against [these checksums](STELE-SHA256SUMS). The ordinary Bean reader treats these as externally curated packs. After verifying the digest, pass `-unknown-pack` when comparing or `-accept-pack DIGEST` to the independent verifier.

The original Bean builder and the [archive](https://stele.aion.bio/archive.html) reproduce historical claims. A reader replaying one must fetch the digest in that claim.

## Aion's archived serum iron pack

This historical 23-measure pack was built by Aion from nine CDC NHANES 2017–2018 files. Its SHA-256 is:

```
a72b2a0933f7feba57cb24ce1461c278168a7dd619b31ba7566ab87bc50c714e
```

The file uses `pack/1` to check the source and printed panel for ambiguous test names. Bean 0.8.3 and later read this format.
Bean treats this as an external pack. Verify the digest above before choosing `-unknown-pack`. The independent verifier takes `-accept-pack` with that digest.
The [release review](https://stele.aion.bio/iron/review.html) gives the source files, both builders, and the rule for each iron measurement.

## Aion's fasting glucose and lipid pack

This separate pack uses NHANES fasting subsample weights. It holds plasma glucose, triglycerides, and three equation-specific calculated LDL distributions.

```
0e778021466c602ab0e7c7c27f63bdfbf7e2cc867ae608db4a43023e96e19e7f
```

Use a Bean build that supports `pack/2`. Each match requires signed fasting evidence. Glucose also requires an identified plasma result.
The [source review](https://stele.aion.bio/fasting/review.html) lists the LDL variants, source files, checks, and download instructions.

## Aion's routine blood chemistry pack

This pack adds eleven NHANES 2017–2018 serum chemistry measures across fourteen adult groups. It uses MEC exam weights. ALT, GGT and total bilirubin include only CDC results flagged at or above the detection limit.

```
588c19e75c80b4ef900b53d5b649d4c4bb2470e2a95fce2931a4e914b13e7de3
```

Use a reader with `pack/2` support. It checks the source, test, unit and panel or specimen recorded with each reading. The [release review](https://stele.aion.bio/chemistry/review.html) lists eligible tests, source file digests and instructions for rebuilding every table.

## Aion's CBC differential and mean platelet volume pack

Five differential percentages, five absolute counts and mean platelet volume come from NHANES 2017–2018. Each absolute count has tables in both printed unit scales.

```
c73be016d0cf67870a7fbc2c694de6459ac7438ed3e62c18c103e01ccd65a3c0
```

Use a reader with `pack/3` support to retain reported zeros. The [source review](https://stele.aion.bio/cbc/review.html) covers segmented neutrophil labels, unit scaling, rounding and the top-coded white cell count.

## Aion's thyroid function pack

Four serum measures come from the 2011–2012 NHANES thyroid subsample. Adult tables use WTSA2YR from THYROD_G and age and sex from DEMO_G. The CDC codebook and assay manual give total T4 in micrograms per deciliter.

```
ad6ff2869ef265301c1c92feebf5f3bfea107b6786e2be637017f00154651233
```

A reader with `pack/2` support checks the signed lab, test, panel and printed unit. The [release review](https://stele.aion.bio/thyroid/review.html) documents eligibility, assay differences and the total T4 transport-label correction. Individuals can select this digest independently of the other packs in [Stele’s discovery catalog](https://stele.aion.bio/catalog.json).

## Adult exam-weighted packs

These pack/2 distributions use positive NHANES MEC exam weights. Match the printed lab, analyte and unit, and the panel where required.

[NHANES 2013-2014: Total testosterone and SHBG](https://stele.aion.bio/androgens/review.html)

```
256d6644c1aebc7c62815ba93724813569e464bff9a20eea6a259ea2add3b457
```

[NHANES 2013-2014: Vitamin B12 and red cell folate](https://stele.aion.bio/b-vitamins/review.html)

```
5906be12d6ed39b36bb813ed90c3d8c1f81d171c433e384127585d7e43bd1167
```

[NHANES 2017-2018: High-sensitivity C-reactive protein](https://stele.aion.bio/hs-crp/review.html)

```
0996de8c76b333373c09acf5e2788ca1584c6b150783ee9309dd1d3be5f37164
```

[NHANES 2013-2014: Total 25-hydroxyvitamin D](https://stele.aion.bio/vitamin-d/review.html)

```
fee20744c19ce3c92192144e6173aa035e52c96f7284f68e048d34393fc4db51
```

## Verify rebuild tools before execution

The [release checksums](STELE-SHA256SUMS) cover every catalog download and the assay source guide and the nine sets of hosted builders, verifiers and shell recipes. Choose a trusted revision of this public Bean repository independently. From a new directory, verify a recipe before running it. For example, with the chosen Bean checkout at `$BEAN`, run

```sh
mkdir -p exam
curl -fsS https://stele.aion.bio/exam/rebuild.txt -o exam/rebuild.txt
awk '$2 == "exam/rebuild.txt"' "$BEAN/docs/STELE-SHA256SUMS" > recipe.sha256
shasum -a 256 -c recipe.sha256 && bash exam/rebuild.txt
```

The recipes also check their downloaded builder and verifier before execution. SHA-256 identifies the exact bytes. Source selection and scientific interpretation still belong to the reader.
