# Roadmap

What comes next. Work that is done is in the git history, and the reasons are in [DECISIONS.md](DECISIONS.md). Nothing here is final.

## What bean is for

bean is an open format for personal health evidence that anyone can check without trusting the person who keeps it, or the program that wrote it. A record is a claim, not a fact. On top of the core sit three layers, kept apart:

1. **Evidence.** What was observed, who signed it, from which original file. This is the core, and it is built.
2. **Exchange.** A person hands chosen records, with their files, to someone else, who can check them without bean or an account.
3. **Comparison.** A reproducible computation that sets a personal reading against public reference data. It says what it compared, how, and what it could not compare.

Exchange and comparison are programs that write ordinary signed claims. They change no core rule. bean uses existing vocabularies (LOINC, units, public dataset variables) through adapters, and does not replace FHIR, OMOP or LOINC.

## Goal

1. The first user imports their own data and uses it every week. If that does not happen, stop and find out why.
2. By 2026-11-15, the first shareable loop (below): a technical person can run it, and a researcher can check it.
3. Then: technical users install bean, import their data, sign, share and compare, through a CLI and their own AI agent.

## Next: the first shareable loop (by 2026-11-15)

**Done means:** a person who has never used bean clones the repo and runs one script on a synthetic Labcorp report. The script does three things. It imports the report, keeping the PDF. It compares the total cholesterol reading with NHANES 2017–2018. It writes a share bundle. On a second machine, without a bean log, they check every signature in the bundle and get the same percentile from an independent implementation. The first user runs the same steps on their own report, privately. No real health data enters the repository.

In order. Each step is done only when its test passes.

1. **Real import, privately (day 1).**
   - Run `bean import labcorp` on a real report.
   - The strict grammar (DECISIONS 219 to 229) will likely refuse something. For each refusal, rebuild that line's shape in a synthetic fixture, then add or loosen a rule with a test.
   - The real text never enters the repository. The README "How a report is read" grows with each new shape.
   - Optional, small: `bean import labcorp` prints the table it read and waits for a yes. `-yes` skips the wait for scripts. The person holding the PDF is the last check on a parser of PDF text.
2. **Reference data: NHANES 2017–2018 total cholesterol.**
   - A reference adapter keeps the public files as blobs, each pinned by SHA-256: `TCHOL_J.XPT` (`LBXTC`, mg/dL) and `DEMO_J.XPT` (`RIAGENDR`, `RIDAGEYR`, `WTMEC2YR`, `SDMVPSU`, `SDMVSTRA`).
   - It writes one `reference.dataset` claim per release: source URL, cycle, license, variables, and file digests.
   - The person who imports it signs it as its curator. It is never written as CDC's claim.
   - The adapter reads SAS transport (XPT v5) itself, with a test against a small synthetic XPT.
3. **`bean compare`.**
   - It writes a `comparison.percentile` claim with `about` refs to the personal reading and the `reference.dataset` claim.
   - Its context records the cohort rule (age band, sex, exclusions), the method (weighted empirical percentile with exam weights), the program version, the inputs' digests and the result.
   - It returns one of three statuses, each with its reasons:
     - `comparable`;
     - `qualified`: Labcorp and NHANES ran different assays, and NHANES describes a population, not a clinic;
     - `not comparable`: another analyte, unit or specimen, or a missing age or sex. It writes no number.
   - It describes where the value lies in the selected sample. It does not diagnose, flag risk or advise. That keeps it a display of public statistics, separate from interpretation.
   - Same inputs, same bytes.
4. **Independent reproduction.**
   - A second implementation in `tools/second` (Python, reading the XPT with pandas) recomputes the percentile from the pinned files and the claim's context.
   - CI fails if the two disagree in the last printed digit.
5. **`bean share` and `bean check-bundle`.**
   - `bean share <ids> -out bundle/` writes the chosen record files and the plaintext blobs they name. It adds the identity records needed to verify them and a manifest signed by the person.
   - `bean check-bundle <dir>` verifies the bundle without a log, and so does the Python second verifier.
   - A bundle proves that what it holds is authentic. It does not prove completeness: it is the selected records, not "all records from a week". The tool says so.
   - A bundle is a disclosure: once sent, it cannot be taken back.
6. **Quickstart and release to peers.**
   - `examples/blood/` holds a synthetic report, a script that runs steps 1 to 5, and the expected output.
   - The README gets a five-minute path, and the repository opens to the people who will try it.

Not this week: Apple Health readings beyond what exists, `bean-import-csv -map`, `-to <npub>` encryption, `bean attest` and `bean trust`, NIP-46, nutrition.

## After the loop

1. **Freeze at `v` 0** (CORE.md section 6). After two weeks of real use (lab results, Apple Health, chat and agent logging) with no change to CORE.md, every DEFAULT in SPEC.md is final.
2. **Longitudinal comparison.** A reading over time against what published studies saw over the same interval: baseline, cadence, follow-up and attrition. This needs a time-course matching contract first.
3. **Summary-statistics references.** Many studies publish only aggregates. A reference may be a published table, with its citation, as well as participant-level files. Access-restricted datasets stay outside the repository.
4. **Comparison packages.** Standardize the interface of a comparison (inputs, method, cohort, status), and let others publish datasets and methods for it. Extension 1's replay rules wait until two kinds of comparison exist.
5. **Nutrition as exposure.** `food` and `food.estimate` exist. Next comes a source-backed estimate (USDA FoodData Central) with its uncertainty, kept apart from what the person said they ate. No causal claims.
6. **Share and trust, further.** `-to <npub>` encryption of a bundle, expiring links, `bean attest <id>`, and `bean trust <identity or npub> <1-100>`. Readers compute trust; the core does not.
7. **Release.** A `go install` path and a neutral home ([GOVERNANCE-PROPOSAL.md](GOVERNANCE-PROPOSAL.md)).
8. **NIP-46 remote signing,** so that the nsec never sits on the host.
9. **Device capture** (Extension 2). Wait for a real instrument, not a synthetic one.
10. **More importers,** from real export files, never from memory. `bean-import-csv -map "<column>=<type>[:<unit>]"`, so that one row can make several records. Apple Health heart rate and daily series wait for rules that reconcile exports.

## Limits to say out loud

- **The Labcorp importer is not proven on real reports.** Seven attack rounds found ways to make it store a wrong value, fewer each round, but not none. It refuses what it cannot read, so it will refuse unfamiliar layouts.
- **A signature proves who signed, not that the number is right.** It does not prove that a lab or device produced the number, nor anyone's credentials.
- **A shared subset proves authenticity, not completeness.** Record files are readable JSON; only blobs are encrypted. Share by bundle, never by copying the log folder. The subject id is stable, so two bundles from one person can be linked.
- **Regulatory.** Storing, verifying and displaying are kept apart from interpreting. A percentile against public data is a function that needs its own review before any clinical use. bean does not interpret, predict or advise.

## Known gaps

- After `purge` and `gc`, the summary counts the tombstone as `1 held` for good. This is what the spec says, but it reads like a fault.
- From the re-audit of `ff4ef77` (DECISIONS 194):
  - A removed record or a removed newest succession goes unnoticed; the signed sealed lists could catch part of it.
  - A mode-000 file from rsync stops every command, and a 1 GiB sparse file costs 1 GiB per load.
  - A stranger's incomplete seal set blocks `gc`.
  - A retired key as a bare author stays current.
  - A correction undone at rotation keeps its purged ref. Two corrections of one target both show.
  - After a thief's link in a chain of `-corrections` is refused, the two records left can become duplicates, and the lower id wins.
  - Purging a correction brings the older value back. `-corrections` cannot tell an older report from a newer one.
  - Two current records with one ref migrate as one: the other is counted as left out, but `migrate -list` does not show it.
  - While a purge with an erased target exists, undoing another deletion keeps its purged ref, so a later import of that reading is skipped.
  - `stats` fails on values whose sum overflows.
  - Importers: a day-precision date lands in the wrong local day, and a note line can become a lab value (DECISIONS 201).
- From the seventh to thirteenth audits (DECISIONS 203 to 229):
  - `sync` proves every file of this copy on each run, which costs about as much as `verify`.
  - `sync` does not copy a blob that no record names; only `export` carries it.
  - `export` does not decrypt the files of an agent that the person unpaired.
  - A source with a stranger's broken file makes every `sync` from it exit 1.
  - The purge check in `bean import` reads only this copy's `local/purged-refs.jsonl`.
- No way to test the 24 words without rotating. `net/http` is in the binary only to detect image types. CI does not run `lint`, `vuln` or fuzzing.

## Not planned

Hosted or custodial accounts, a central data platform, a UI beyond a static report, relay publishing of records, encryption of record files at rest (use an encrypted volume), Whoop and Oura importers until there is a real export, zero-knowledge proofs, homomorphic encryption, and distributed trust scores.
