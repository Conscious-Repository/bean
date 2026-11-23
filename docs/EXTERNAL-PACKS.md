# External reference packs

## Aion's serum iron pack

Stele publishes a 23-measure pack built by Aion from nine CDC NHANES 2017–2018 files. Its SHA-256 is:

```
a72b2a0933f7feba57cb24ce1461c278168a7dd619b31ba7566ab87bc50c714e
```

The file uses `pack/1` to check the source and printed panel for ambiguous test names. Bean 0.8.3 and later read this format.
Bean treats this as an external pack. Verify the digest above before choosing `-unknown-pack`. The independent verifier takes `-accept-pack` with that digest.
The [release review](https://stele.aion.bio/iron/review.html) gives the source files, both builders, and the rule for each iron measurement.
