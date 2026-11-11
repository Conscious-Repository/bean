# Source-specific ingestion, without bridges

Scope: how a provider's data gets in. Built today: lab reports, Apple Health,
CSV and NHANES files. No bridge, iPhone application,
provider account creation, sharing protocol or new bean core rule is required.
Each additional provider gets its own small adapter and real input sample.

## Implemented Apple Health path

`cmd/bean-import-apple-health` preserves a validated raw export by default and
optionally emits supported individual readings. `bean import apple-health`
stores it, checks readback, and checks the log keeps it before any reading. The original file
is sufficient for replay even if the adapter changes. Tests use synthetic data.
See the command's README for explicit limits and approximate sample dedupe.

## Next direct providers

Access requirements checked against primary documentation on 2026-11-02. These
are conditional candidates, not promises of account access. Prefer a provider
already used by the owner with actual available data over this tentative order.
Do not buy a device or subscription just to satisfy the order.

| Candidate | Small first slice | Access gate |
| --- | --- | --- |
| Oura | One bounded range of daily/sleep JSON, `daily` scope | Registered OAuth app and authorized eligible account; personal access tokens are discontinued. |
| WHOOP | One bounded range of sleep or recovery, selected `read:*` scope plus `offline` for refresh | Developer registration requires WHOOP membership; broader app release requires approval. |
| Withings | Body measurements, `user.metrics` | Registered OAuth app; current Start for Free plan has account-date/Withings+ eligibility restrictions. Check before choosing it. |

Sources: [Oura authentication](https://cloud.ouraring.com/docs/authentication),
[Oura V2 reference](https://cloud.ouraring.com/v2/docs),
[WHOOP overview](https://developer.whoop.com/docs/developing/overview/),
[WHOOP OAuth](https://developer.whoop.com/docs/developing/oauth/),
[Withings reference](https://developer.withings.com/api-reference/),
[Withings published plans](https://static.withings.com/content/api/plans/plans.html).

An installable adapter is not an already-authorized connection. Application
registration, consent and provider eligibility still apply. The next necessary
input is which account the owner already uses, followed by an authorized sample
response kept outside the repository. Never request tokens/secrets in chat.

## Small implementation sequence

1. Inspect an actual export/API response. Preserve exact original response bytes,
   pagination boundaries, retrieval time and the requested resource/date range.
   Keep credentials and sensitive request headers out of the evidence.
2. Build the offline parser first, with synthetic fixtures matching that response.
   Use native reading identifiers only when provided; distinguish account/resource
   namespaces. Test repeat import, updated readings, missing fields and pagination.
3. Add an explicit local fetch command using provider OAuth consent and minimal
   scopes. Store credentials privately outside the log/repository. Serialize
   refresh and durably replace rotating tokens; do not keep using an invalidated
   token after a concurrent refresh. Respect Retry-After and bounded retries.
4. Save original pages before advancing a retrieval checkpoint. Replay locally
   into existing bean adoption. A partial fetch must not masquerade as a complete
   date range. Keep download success and adoption success separately observable.
5. Only then add incremental retrieval. Define corrections/deletions from actual
   provider behavior; absence in a page is not deletion. Webhooks can trigger
   reconciliation later but need not create a public receiver for the first slice.

A source-specific fetcher and a replayable file importer are enough. Extract a
shared helper only after two real adapters reveal the same need. No universal
connector framework, bridge dependency or stable new wire format is warranted.

## Trust and exit

Raw API bytes are what a provider exposes, often derived scores or summaries;
they are not necessarily raw sensor/ADC data. OAuth/TLS establish an authorized
retrieval channel. bean signatures establish owner adoption. Neither is a device
signature proving a physical measurement or the person measured. A future AION
acquisition receipt is a separate evidence class.

Keep original data exportable and replayable after a subscription expires or
permission is revoked. A provider can prevent future retrieval, not retroactively
change bytes already preserved. New APIs must not force users to re-sign old
history. Purging an individual claim does not erase its contents from a retained
raw archive; artifact retention needs its own explicit treatment.

These choices apply the three review lenses: minimal working code, explicit
trust/identity boundaries, and an independent user exit. No importer is written
before there is a real export from an account the owner uses (ROADMAP.md, "Not
planned"). Homomorphic encryption remains separate future work.
