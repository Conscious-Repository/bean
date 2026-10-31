# Governance proposal (draft, not decided)

Status: proposal. The open points at the end are not decided. SPEC.md section 19 is the current text.

## Principles

1. **No owner.** No company and no person owns bean. The name, the spec and the code belong to nobody.
2. **Neutral home.** The repositories live in a neutral organization whose name is not a company name.
3. **Licenses.** The specification (SPEC.md, CORE.md, the vectors) is CC0 1.0. The reference code is Apache-2.0. No CLA. Contributors keep their copyright; the license grants are enough.
4. **Change rule.** A change to the specification needs:
   - a written change note (as C1 to C29 are), with the reason;
   - new or changed test vectors when it changes a result;
   - after the freeze: two independent implementations that agree on the vectors and on the conformance logs before the change is final. The Go reference and the Python second verifier (`tools/second`) are the first two.
5. **Compatibility.** A change never makes an old valid record invalid without a change note that says so and a version step (SPEC 8.4). Old verifiers stop on newer records (`too new`) and erase nothing.
6. **Implementations are equal.** The reference implementation is a test of the spec, not its authority. Where it and the spec disagree, the disagreement is a bug in one of them, and the change rule decides which.
7. **Security reports.** SECURITY.md names a private contact for reports. A fix that changes validity follows the change rule, with a short embargo when a fix is needed before disclosure.

## Contribution rules (proposal)

- Open an issue first for a change to SPEC.md or CORE.md. Code-only fixes can go straight to a pull request.
- One change per pull request. Plain commit messages. Tests never weakened.
- A spec change lands with its change note, vectors and both implementations updated.

## Open points

1. The neutral organization name, and whether the current organization is renamed or the repositories move.
2. When the repositories go public.
3. Who merges until the freeze. Proposal: a small group of maintainers named in a MAINTAINERS file, with no company seat.
4. The private contact for security reports.
