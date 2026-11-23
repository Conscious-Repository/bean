# External reference packs

## Aion's serum iron pack

Stele publishes a 23-measure pack built by Aion from nine CDC NHANES 2017–2018 files. Its SHA-256 is:

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
