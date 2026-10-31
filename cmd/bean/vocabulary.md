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
| `heart.resting` | number, `/min`: resting heart rate, beats per minute | `device` |
| `lab.<test>` (`glucose`, `tsh`, `wbc`) | number with the unit the lab printed (`mg/dL`, `uIU/mL`), or text (`Negative`, `<0.5`) | `test` (the name as printed), `panel`, `flag` (High, Low), `reference_interval`, `performing_lab`, `fasting` |
| `bp` | text `120/80` | `systolic`, `diastolic` as numbers, `pulse` |
| `note` | text | Anything else. Use `about` to attach it to an entry. |

`<test>` in `lab.<test>` is the test name as the lab prints it, in lowercase with `-` for spaces and punctuation: "BUN/Creatinine Ratio" is `lab.bun-creatinine-ratio`. `bean-import-labcorp` writes these types. Importers may write other types of their own, for example `hrv.rmssd` for a CSV of HRV.

## Your own types

To let your assistant log a type that is not here, add a file `vocabulary.md` to your bean folder (the folder of `-dir` or `$BEAN_DIR`), with a table in the same form:

    | Type | Value | Useful details |
    |---|---|---|
    | `kombucha` | number, `mL` | `brand` |
    | `cgm.<sensor>` (`libre`) | number, `mg/dL` | |

Your terms are added to these. A term in your file cannot reuse a type from this list or `bean.*`, `agent.pair` or `device.pair`; a changed meaning is a new term. If it does, the assistant gets an error that names the term until you fix the file. Your assistant reads the file as instructions, so only you should write it. `bean sync` does not copy it; copy it yourself to each computer that runs `bean mcp -write`.

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
