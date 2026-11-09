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

**Done means:** a person who has never used bean clones the repo and runs one script on a synthetic lab report. The script does three things. It imports the report, keeping the PDF. It compares the total cholesterol reading with NHANES 2017–2018. It writes a share bundle. On a second machine, without a bean log, they check every signature in the bundle and get the same percentile from an independent implementation. The first user runs the same steps on their own report, privately. No real health data enters the repository.

In order. Each step is done only when its test passes.

1. **Real import, privately (day 1). Done.**
   - A real report imports: 71 readings, each checked by hand against the PDF (DECISIONS 230, 231). Its layout, with synthetic content, is `cmd/bean-import-lab/testdata/real-layout.txt`. The real text never enters the repository.
   - `bean import` shows what it read and asks before it saves; `-yes` skips that for scripts (DECISIONS 234).
2. **Reference data: NHANES 2017–2018. Done.**
   - `bean import nhanes <file.XPT>` keeps a public file as a `reference.file` record, on the same rail as a lab report: encrypted blob, SHA-256, the data set, CDC's URL, each variable with its label (DECISIONS 236).
   - One record per file, not one per release: CDC publishes files, and each is pinned on its own.
   - The person who imports it signs for having kept it. It is never written as CDC's claim.
   - `internal/xpt` reads SAS transport itself. On ten CDC files it gives the same bits as pandas for every value.
3. **`bean compare`. Done.**
   - `bean compare -sex <s> -age <from>-<to> [-where <rule>]... <ids> | -type 'lab.*'` (DECISIONS 237 to 240 and 246 to 248, [COMPARE.md](COMPARE.md)).
   - The cohort is "people like me", stated as rules: sex and age, then any of ethnicity, body measures and a day's food. Every rule is in the claim, and n is on every row.
   - Twelve tests: total and HDL cholesterol, hemoglobin A1c and the blood count.
   - `-save` writes a `comparison.percentile` claim, `about` the reading and each file, with the cohort, the method, the files' digests and the result. It does not copy the reading's value.
   - Statuses: `qualified`, with reasons, or `not comparable`, with no number. Nothing is `comparable` yet. Fewer than 30 people: no number.
   - It says where a value lies in a sample. It does not diagnose, flag risk or advise.
   - Same inputs, same bytes.
4. **Independent reproduction. Done.**
   - `tools/compare/reproduce.py` (Python, pandas) computes every number of a saved claim again from the claim, the reading and the files. It shares no code with bean and has no table of its own.
   - `make compare` fails if one number differs, in CI on synthetic files; with `BEAN_NHANES_DIR` it runs on CDC's files.
5. **`bean share` and `bean check-bundle`. Done.**
   - `bean share -out <folder> <ids> | -type <type>` writes the chosen records, the records they are about, the identity records that verify them, and one `share` record signed by the person that lists exactly what the bundle holds. Files go only with `-files`, as plaintext named by SHA-256 (DECISIONS 251, [SHARE.md](SHARE.md)).
   - A bundle is a small log, so the checks of a log apply unchanged. `bean check-bundle <folder>` needs no log, and `tools/second/checkbundle.py`, which follows SHARE.md and shares no code with bean, gives the same verdict: `make bundles` checks both on bundles good and damaged.
   - The person sees what leaves and confirms. The share record stays in the log as the record of the disclosure.
   - A bundle proves that what it holds is authentic and exactly what the person listed. It does not prove completeness, and every check says so.
   - A shared comparison computes again from its bundle (`reproduce.py <bundle>`).
6. **Quickstart and release to peers.**
   - Done: `examples/blood/run.sh` runs steps 1 to 5 on a synthetic report with values a person could have: import, fetch the NHANES files, compare, share, check without bean. It fails if a number differs from `expected.txt`. `make example` runs it offline on synthetic reference files, in CI.
   - Done: the README has the five-minute path.
   - Open, and the owner's to do: confirm the NHANES digests from a second connection; open the repository to the people who will try it; then require the CI check on `main`. A private repository on the free plan cannot, so until then a PR merges only after CI passes on its exact head.

Not this week: Apple Health readings beyond what exists, `bean-import-csv -map`, `-to <npub>` encryption, `bean attest` and `bean trust`, NIP-46, nutrition.

## After the loop: people like me

The first question a person asks is not "where is my cholesterol" but "how do I compare with people like me, who eat what I eat". The rule-based cohort is the start. In order of value for the effort:

- **More tests.** Each is one row in `internal/nhanes`, from the file's own label and a real report's name for the test. Next: the biochemistry profile, hs-CRP, ferritin, vitamin D. Fasting tests (glucose, triglycerides, LDL, insulin) need the fasting weight first. Quest's names come with its importer.
- **More people.** One cycle has about 9,000 people, so three rules often leave fewer than 30. Pool cycles (2017 to March 2020 is published as one file set), with the weights CDC gives for pooling.
- **Your own food as the rule.** Work out the ranges from the person's own `food.estimate` records, and say how many days they cover. One recalled day in NHANES is not a usual diet; the two-day files and their weight come first.
- **Check a recommended range.** Given a range someone recommends, show the share of the cohort inside it. That is a fact about the sample, not advice.
- **Outcomes.** NHANES has linked mortality files. Associations only, and only after a regulatory read.
- **Genetics.** NHANES publishes no genotypes. This is another rail with the same shape: the person's genotype file, a public score file pinned by its digest, computed locally, saved as a signed claim.
- **An assistant asks for a comparison.** `bean mcp` can read saved comparisons today. A tool that runs one comes after the person has used the command for a while.

## Also after the loop

1. **Read PDFs by word position.** `pdftotext -bbox-layout` gives each word's box; columns come from where the header words sit, not from counted spaces. Most attacks on the layout grammar (DECISIONS 219 to 231) were artifacts of `-layout` spacing.
2. **Unknown layouts, with an assistant if the person wants.** The person's own assistant, or a local model, proposes each reading with the page and words it read it from. bean checks every value against those words in the PDF and refuses what is not printed there; the person confirms the table, and the readings are theirs, with the extractor in context. bean ships no model and no key. A scanned PDF has no words to check against: the person confirms each value. Design it on a second real report from another lab, never from memory.
3. **Freeze at `v` 0** (CORE.md section 6). After two weeks of real use (lab results, Apple Health, chat and agent logging) with no change to CORE.md, every DEFAULT in SPEC.md is final.
4. **Longitudinal comparison.** A reading over time against what published studies saw over the same interval: baseline, cadence, follow-up and attrition. This needs a time-course matching contract first.
5. **Summary-statistics references.** Many studies publish only aggregates. A reference may be a published table, with its citation, as well as participant-level files. Access-restricted datasets stay outside the repository.
6. **Comparison packages.** Standardize the interface of a comparison (inputs, method, cohort, status), and let others publish datasets and methods for it. Extension 1's replay rules wait until two kinds of comparison exist.
7. **Nutrition as exposure.** `food` and `food.estimate` exist. Next comes a source-backed estimate (USDA FoodData Central) with its uncertainty, kept apart from what the person said they ate. No causal claims.
8. **Share and trust, further.** `check-bundle -key <key>` to compare the signing key with one the person gave, `-to <npub>` encryption of a bundle, a share that does not carry the stable identity id, expiring links, `bean attest <id>`, and `bean trust <identity or npub> <1-100>`. Readers compute trust; the core does not.
9. **Release.** A `go install` path and a neutral home ([GOVERNANCE-PROPOSAL.md](GOVERNANCE-PROPOSAL.md)).
10. **NIP-46 remote signing,** so that the nsec never sits on the host.
11. **Device capture** (Extension 2). Wait for a real instrument, not a synthetic one.
12. **More importers,** from real export files, never from memory. `bean-import-csv -map "<column>=<type>[:<unit>]"`, so that one row can make several records. Apple Health heart rate and daily series wait for rules that reconcile exports.

## Limits to say out loud

- **The lab importer is proven on one real report.** Thirteen attack rounds found ways to make it store a wrong value, fewer each round, but not none. It refuses what it cannot read, so it refuses unfamiliar layouts, and it shows what it read before it saves.
- **A signature proves who signed, not that the number is right.** It does not prove that a lab or device produced the number, nor anyone's credentials.
- **A shared subset proves authenticity, not completeness.** Record files are readable JSON; only blobs are encrypted. Share by bundle, never by copying the log folder. The subject id is stable, so two bundles from one person can be linked.
- **Regulatory.** Storing, verifying and displaying are kept apart from interpreting. A percentile against public data is a function that needs its own review before any clinical use. bean does not interpret, predict or advise.
- **A comparison is as good as its cohort.** NHANES is one country, one two-year window, about 9,000 people. A lab and the survey measure with different instruments. A narrow cohort has few people, and no interval of uncertainty is computed yet. Every result says its n and its reasons.

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
