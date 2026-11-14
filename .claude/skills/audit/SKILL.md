---
name: audit
description: Audit a scope, a design or a diff of bean against the project's three paragons, Linus, Satoshi and buildtall. Use when the owner asks for an audit, an audit loop or lap, or a review "as Linus / Satoshi". Use it for a scope "audited against our values", and for what is next "according to our values". Use it also before any change goes to main.
---

# Audit against the three paragons

bean is judged by three paragons. Every scope, design and diff must pass
each of them. This skill says who they are, how to run the audit, and what
to report.

## The paragons

**Linus: is it small, correct and needed?**

- What problem does this solve, for whom, today? Real data over imagined.
- The smallest change that solves it. A small core.
- Correct before clever. Simple before general.
- No framework before a second real case.
- Every line is a cost that someone carries forever. What can be cut?

**Satoshi: can anyone check it?**

- Verify, do not trust. No trusted third party where a check can do the work.
- Anyone can build the same bytes again from public inputs.
- A signature proves who said a thing. It never proves that the thing is true.
- Data is named by its hash.
- Think as the attacker: a hostile peer, a hostile file, a thief with an old
  key, a paired agent that turned. Think also as a reader who wants to be
  fooled.
- What does each party learn? What can never be undone?

**buildtall: does the person stay in charge?**

- The person owns their data and their choices. Nobody chooses for them.
- They can leave with everything: plain files, open formats, no account, no
  server.
- Refuse rather than guess.
- Low friction, also for a person who hands the work to their own assistant.
- Peers coordinate through signed records in their own logs, not through a
  central list.
- Others can build their own tools on the same files.

## Rules that no audit may trade away

- bean is not a diagnosing tool. It never says whether a value is healthy.
  It lets people exchange information about their health with peers they
  trust, and compare with public data. A source's own claim is shown as a
  quote, with the source's name.
- Never commit real health data. Fixtures keep a real layout with synthetic
  content. A real report lives only in a scratch folder, never in the repo.
- The person runs `bean init`, `rotate`, `restore` and `import`, not an
  assistant. The 24 words and health values never enter an assistant's
  context.
- A change reaches `main` only after CI passes on the exact head commit,
  and only when the owner asked for the merge.

## Two kinds of audit

**A scope or a design**, before any code. The question is whether to build
it, and in what form.

**A diff**, before it goes to `main`. The question is whether it is correct,
safe and as small as it can be.

## How to run it

1. **Write the thing down.** For a scope: one file in the scratch folder.
   It holds the need in the user's own words, what is true today, and the
   proposal. It also holds what is left out, and the weakness that you see
   yourself. For a diff:
   the range of commits.
2. **Start one fresh reviewer for each paragon, at the same time.** Each is
   a new subagent with no memory of the work. Give it the scope file or the
   diff range, the files to read, and its paragon's questions from above.
   Never give it your conclusion. Ask it to be adversarial, to check every
   claim against the code, and to name a concrete failure for each finding.
   For a diff, ask it to build bean and attack it with synthetic data in a
   scratch folder of its own. A smaller model is enough for a reviewer.
3. **Ask for one form of answer.** A verdict, then findings ranked
   must-fix, should-fix and note. For a scope the verdicts are: build as
   scoped, build with changes, do not build yet.
4. **Check each finding yourself before you act on it.** A reviewer can be
   wrong. Reproduce it, or read the code that it names.
5. **Fix every must-fix.** Each new guard gets a test. Then take the guard
   out and see the test fail: a test that passes without its guard holds
   nothing.
6. **Run it again with new reviewers**, never the same ones, until each
   says yes. A reviewer that saw the first version defends its own notes.
7. **Write it into the repo.** A numbered entry in `docs/DECISIONS.md` with
   the reason, and `docs/ROADMAP.md` for what stays open. Both in the same
   commit as the code. The documents are in Simplified Technical English:
   no sentence over 25 words.
8. **Before a push**, run `make check`. It includes `make prose` and
   `make differential`: this build must print what `main` prints. Run
   `tools/smoke.sh` too. In a cloud container that runs as root, four tests
   of process groups and file permissions fail. They pass in CI.

## When the paragons disagree

- A fault that can lose or leak a person's data is fixed, whoever found it.
- Linus decides the size and the order: what is built now and what waits.
- Satoshi decides what a record or a line of output may claim.
- buildtall decides who holds the choice, and the words that the person
  reads.
- A disagreement that changes what the person can do is the owner's
  decision. Give a recommendation and ask.

## How much audit

- A change to the docs, or a few lines: check it yourself against the three
  lists. No subagents.
- A feature, a new record type, or anything that touches keys, signatures,
  sharing or deletion: the full audit. It ends with one lap of new
  reviewers.

## What to tell the owner

Keep it short and plain. The owner reads the reply, not the documents.

- Each paragon's verdict, in one line.
- What changed because of the audit.
- What is known and left, with the reason.
- Any decision that is the owner's, with your recommendation.
