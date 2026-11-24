# Read five lab results offline

This trial uses five invented readings, one survey pack and one assay directory guide.
It creates a temporary identity, signs the readings locally and discards the identity after the replay.
You need Go, Python 3.8 or later, and a POSIX shell. Start in the Bean checkout linked from Stele.

```sh
trial=$(mktemp -d)
go build -o "$trial/bean" ./cmd/bean
ex=examples/coverage
python3 -I "$ex/fetch.py" "$ex/trial.sources.json" "$trial/assets"
```

You can now disconnect. These commands replay the readings and save a JSON file you can give your own agent.

```sh
sh "$ex/trial.sh" "$trial/bean" "$trial/assets" > "$trial/replay.json"
python3 -m json.tool "$trial/replay.json"
```

1. Find the albumin/globulin ratio of 1.6 from the comprehensive metabolic panel. Which survey, cohort and signed fields support its percentile? Cite the pack digest and review.
2. Find the other numeric ratio of 1.6. Which field changes the decision? Check what Bean says about the bounded ratio, the blank result and the cortisol measurement.

Record the answers and the first step that took help. Check your answers locally.

```sh
python3 -I "$ex/verify-trial.py" < "$trial/replay.json"
```

## Use your own sources

The trial selection pins two files by SHA-256. The larger example, `stele.sources.json`, pins the active survey packs and assay guide in this revision.
Copy a selection and keep the sources you approve. Review links identify the publisher's rationale. Check the file digests against the [published receipts](../../docs/STELE-SHA256SUMS).
Only the download step contacts the host. The replay checks every selected file again.

To apply a selection to a local Bean log, use the following command. Supply a cohort available in your chosen packs.
Use `-report` with the original PDF's SHA-256 to restrict the replay to one report.

```sh
bean -dir "$log" coverage -sources "$selection" -assets "$assets" -cohort male-20-29 -json
```

Each current reading appears once. Overlapping packs can return several placements on the same row.
The source-covered count includes readings with a survey placement or a matching assay guide entry.
An assay entry identifies the lab's documentation. Its presence alone supplies no population rank.
A percentile describes the named survey group. The output includes the matching rule, sample size, method and caveats.
Personal replays contain values, identifiers and report context. The public exercise uses only the supplied synthetic fixture.
