# Type vocabulary

Conventions for what goes in the log, so that a month of entries lines up for `stats`. The core checks none of this. New types are fine; reuse these first.

## Rules

1. **One thing per entry.** A meal is one `food` entry. Coffee with it is a second, `drink.coffee`.
2. **A quantity goes in the value with a unit**, so it can be averaged: `drink.water 0.5 L`, `weight 81.2 kg`, `symptom.headache 7 {score}`.
3. **A description goes in the value as text** when there is no single quantity: `food "2 eggs, sourdough, butter"`.
4. **Keep the person's words.** The value or the note says what they said. An inference (calories, sodium, a diagnosis guess) is a separate entry that points at the first one with `about`.
5. **`at` is when it happened**, in local time. Leave it out for "now". If the time is unknown and matters, ask; do not invent one.
6. **Nobody edits.** A correction is a new entry: a `note` with `about`, or, for the person's own entry, a supersede from their machine.

## Types

| Type | Value | Useful details |
|---|---|---|
| `food` | text: what was eaten | `meal` (breakfast, lunch, dinner, snack), `items` (list), `place` |
| `food.estimate` | number, `kcal` | `protein_g`, `carbs_g`, `fat_g`, `fiber_g`, `sugar_g`, `sodium_mg`, `method`, `confidence` (low, medium, high). Always `about` the food entry. |
| `drink.water` | number, `L` | |
| `drink.coffee`, `drink.tea` | number, `mL`, or text | `caffeine_mg` |
| `drink.alcohol` | number, `{drink}` (standard drinks) | `kind` |
| `supplement` | text: name | `dose` (`400 mg`), `brand` |
| `med` | text: name | `dose`, `reason` |
| `exercise.<kind>` (`run`, `walk`, `lift`, `bike`, `yoga`) | number with its natural unit (`km`, `min`) | `duration_min`, `intensity` (low, moderate, high), `felt` |
| `sleep` | number, `h` | `bed`, `wake`, `quality` (0 to 10) |
| `symptom.<name>` (`headache`, `nausea`, `soreness`) | number 0 to 10, `{score}` | `location`, `trigger` |
| `mood`, `energy`, `stress` | number 0 to 10, `{score}` | |
| `weight` | number, `kg` | |
| `heart.resting` | number, `/min`: resting heart rate, beats per minute | |
| `lab.<test>` (`glucose`, `tsh`, `wbc`) | number with the unit the lab printed (`mg/dL`, `uIU/mL`), or text (`Negative`, `<0.5`) | `test` (the name as printed), `panel`, `flag` (as printed, for example High), `reference_interval`, `performing_lab`, `fasting` |
| `bp` | text `120/80` | `systolic`, `diastolic` as numbers, `pulse` |
| `note` | text | Anything else. Use `about` to attach it to an entry. |

`<test>` in `lab.<test>` is the test name as the lab prints it, in lowercase with `-` for spaces and punctuation: "BUN/Creatinine Ratio" is `lab.bun-creatinine-ratio`. `bean-import-lab` writes these types.

`lab.report` is not a result. Its `value.blob` is the report as the lab gave it, PDF or text. `context` has the file's `sha256`, its `bytes`, and `assurance: unsigned-report`: you kept this file, which does not prove the lab wrote it. An amended report is a new `lab.report`; its changed values need `-corrections`. A `lab.<test>` reading names its report by `context.report_sha256`, not as a blob.

`apple-health.export` is not a measurement either. Its `value.blob` is an Apple Health `export.zip` or `export.xml` as Apple wrote it. `context` has the file's `sha256`, `bytes`, the count of each record type, and `assurance: unsigned-export`. With `-records`, `bean-import-apple-health` also writes `apple-health.<type>` readings (`apple-health.steps`, `apple-health.body-mass`) in Apple's own units and intervals, and with the export's SHA-256 in `context.export_sha256`. A percentage type such as `apple-health.walking-double-support` is a fraction with unit `1`: 0.272 is 27.2%. Blood pressure and food are not read. Nothing is summed per day or merged across sources. The export has no sample ids, so a changed export can add a reading again; see `cmd/bean-import-apple-health/README.md`. Importers may write other types of their own, for example `hrv.rmssd` for a CSV of HRV, kept by a `csv.file` record like `lab.report`.

`reference.file` is not about you. Its `value.blob` is a public data file that you kept to compare with, for example an NHANES file from `bean import nhanes`. `context` has the file's `sha256`, `bytes`, data set name and `assurance: unsigned-file`: you kept this file, which does not prove its publisher wrote it. `comparison.percentile` is a result of `bean compare`. Its value is a percentile with unit `{percentile}`, and it is `about` the reading and each file it read. `context` has the `reading`, the `reference` (variable, weight, files and their SHA-256), the `cohort` rules, the `method`, the `result` and the `caveats` (`docs/COMPARE.md`). A comparison with a reference pack has the same type. Its `reference` names the pack by its SHA-256, with the measure and the cohort, and its method is `weighted-percentile` (`docs/REFERENCE.md`). It says where a value lies in a sample, never whether it is healthy. `bean compare` and an importer write these two; the `bean mcp` tool does not. `share` is the record of a disclosure, written by `bean share`. Its value is the number of records shared, and `context` lists their ids and the files (`docs/SHARE.md`). The `bean mcp` tool does not write one either.

## Your own types

To let your assistant log a type that is not here, add a file `vocabulary.md` to your bean folder (the folder of `-dir` or `$BEAN_DIR`). Give it a table in the same form:

    | Type | Value | Useful details |
    |---|---|---|
    | `kombucha` | number, `mL` | `brand` |
    | `cgm.<sensor>` (`libre`) | number, `mg/dL` | |

Your terms are added to these. A term in your file cannot reuse a type from this list or `bean.*`, `agent.pair` or `device.pair`; a changed meaning is a new term. If it does, the assistant gets an error that names the term until you fix the file. Your assistant reads the file as instructions, so only you should write it; it can be up to 64 KiB. `bean sync` does not copy it; copy it yourself to each computer that runs `bean mcp -write`.

## Uncertainty

A measurement can say how sure it is in the detail `uncertainty`, stored as `context.uncertainty`:

    {"kind": "cv", "value": 0.05}
    {"kind": "sd", "value": 1.2, "unit": "ms", "source": "device manual, 2025"}

- `kind` is `cv` (coefficient of variation, as a fraction: 0.05 is 5 %) or `sd` (standard deviation).
- `value` is a number.
- `unit` (optional) is the unit of an `sd`. Without it, the `sd` is in the unit of the entry's value.
- `source` (optional) names the calibration or document the number comes from.

Leave it out when nobody published a number. Never guess one.

## Units

`kg`, `g`, `mg`, `L`, `mL`, `km`, `min`, `h`, `kcal`, `/min` (beats per minute), `{score}` (0 to 10), `{drink}`. Write the unit the person used; do not convert.
