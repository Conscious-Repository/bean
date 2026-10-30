# Porting draft v0.7 into the repository spec

The repository SPEC.md is the canonical spec (Benjamin, 2026-10-30), and it can be changed when needed. Draft v0.7 (2026-10-27, written outside the repository) is a historical draft. This file lists each v0.7 item and what happened to it. Each port is one commit on branch `spec-port`. No port changes which records are valid, and none changes bytes, ids, signatures, key derivation or vectors.

## Ported

| v0.7 item | Repository | Commit subject |
|---|---|---|
| K7 AES-SIV test gate | SPEC 4.9, 14 | Port K7: state an AES-SIV test gate that a 64-byte-key library can run |
| Goals G6 to G8 (R10, R18) | SPEC 1.4 | Port goals G6 to G8 |
| 5.9 Custodians, agents and exit (R15, R16) | SPEC 5.11 | Port the custodian, agent and exit rules |
| 6.8 Trust is reader policy | SPEC 6.8 | Port 6.8 |
| 9.10 Ontology and `context.uncertainty` (R6 to R8) | SPEC 9.9, vocabulary.md, DECISIONS 140 | Port the vocabulary and uncertainty convention |
| 9.11 Credentials (R12) | SPEC 9.10 | Port 9.10 |
| 9.12 Signed source files, multi-file sources | SPEC 9.11 | Port 9.11 |
| 18 Out of scope additions | SPEC 18 | Port the out-of-scope list |
| 1.6, 9.9 First use case Whoop (R1 to R3) | SPEC 9.8 | Port the first use case |

## Already in the repository

| v0.7 item | Where |
|---|---|
| K1 too-new gate | PR #4 (accepted by Benjamin), with C29 in the thief-fix PR |
| K3 superseded records are not duplicates | C23, 6.3 step 12 |
| K4 replicas agree on ids, not `sig` bytes | 4.5 ("two valid files with the same id can have different `sig` bytes"), 7.4 |
| K5 rename to bean | C24 |
| R9 two times, R11 plain files, R13 `bean mcp` | 3.5, 7, 12 |

## Not ported, with the reason

| v0.7 item | Reason |
|---|---|
| K2 batch ref by (system, type, UTC day), later snapshot supersedes, warning `stale snapshot`; K6 series format (sorted CSV) | No batch adapter exists. The rule needs append code (a later snapshot of a day supersedes the earlier one) and a new ref form. Port it with the first batch adapter (Whoop), with tests, when it is proven needed. It adds a ref form and does not change existing refs. |
| Q5 / 5.6 `losing branch` error | The repository reports `frozen` for every key below a fork point (6.3 step 6.4). A new string changes the verify output and V17-style expectations without changing validity. Not worth the churn. |
| 3.8 `bad envelope: <name>` instead of `bad v` / `bad type` / `bad field: author` | Same: different strings for the same verdicts. The repository strings are in its vectors and in the second verifier. |
| 3.8 step 10 `unverifiable: <id>` (unknown `v` and unknown key tag: skip with a warning instead of an error) | Changes a verdict (error to skip, exit 1 to 0). A real forward-compatibility point for new key types. QUESTION for Benjamin. |
| 5.3 `bean rotate -since <time>` with `local/received.jsonl` | A new CLI feature and a new local file. The repository has `-no-seal` and prints every sealed id (C26). Build it when a real theft shows it is needed. QUESTION, low priority. |
| 5.5 `parent has a successor` guard | Redundant here: rotate always starts from the chain head, and a frozen identity refuses (`compromised`). |
| 15 succession tree limit of 10,000 nodes (SHOULD) | A verdict for huge trees that no real log has. The record size and nesting limits already bound the input. Revisit with the performance index. |
| 0, 20, 21, Appendices A to C (decision tables, milestones, M1 status, evidence) | Process history. The repository uses docs/DECISIONS.md and docs/ROADMAP.md. |
| Header "Editor: ... (AION Biosciences, steward)" | Benjamin: no ownership by AION or anyone. See docs/GOVERNANCE-PROPOSAL.md. |
