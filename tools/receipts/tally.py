#!/usr/bin/env python3
"""tally.py: what the keys in some opened bundles state about source files.

    tally.py <folder of opened bundles>

A receipt is a reference.file record, as `bean import nhanes <file>` writes
one: an identity states that it imported a file with this SHA-256, under
this dataset's name (docs/POOL-PLAN.md, part 3). The tally lists, for each
dataset and SHA-256, the identities that state it.

It counts keys, and a key is free. It does not show that anyone fetched a
file, that the source served these bytes, or that the data is true. It
prints no total over a dataset and no percentage.

It reads opened bundles only: one bundle in each folder below the folder
that it is given. It checks each with tools/second/checkbundle.py, and a
bundle that fails is refused whole. It takes only records that the
identity's own key signed. It exits 0 when it read the folder.
"""
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(HERE, "..", "second"))
sys.path.insert(0, os.path.join(HERE, "..", "compare"))
import checkbundle  # noqa: E402
import reproduce  # noqa: E402  (its table of digests, and no copy of it here)

DIGEST_RE = re.compile(r"[0-9a-f]{64}")
DATASET_RE = re.compile(r"[A-Z][A-Z0-9_]{0,7}")  # as cmd/bean-import-nhanes has it
MAX_NOTE = 200  # characters of a note that are shown
OF_ITSELF = [
    "This tally counts keys, and a key is free.",
    "A statement can be copied with no fetch: bean's own code holds the digests.",
    "It shows what these keys state, in the bundles that it was given. It cannot show that those are all the bundles.",
    "It does not show that the source served these bytes, and nothing about whether the data is true.",
]
CARRIES = ("the share record lists a file for this record with the stated SHA-256. The check of the bundle computed it from the file, in this run. "
           "Nothing shows that it is the file that the signer kept, or a file of the survey.")


def shown(text, limit):
    """The text up to its limit, safe on one line, and the tally's own mark
    for what was cut. Every character but printable ASCII is written out,
    and so are a quotation mark and a backslash: so nothing shows as blank,
    ends a line, or closes a quotation. The mark is apart, so that it never
    stands inside a quotation."""
    text = str(text)
    out = "".join(c if " " <= c <= "~" and c not in '"\\' else "\\u{%04x}" % ord(c) for c in text[:limit])
    return out, " [cut: %d characters in all]" % len(text) if len(text) > limit else ""


def quoted(text, limit):
    """A text that the tally did not write, in quotation marks: a text of a
    record, a folder's name, a reason of the check of a bundle. The mark
    for a cut is the tally's, and stands after the closing mark. What is no
    text gets no quotation marks: nobody wrote such words."""
    return '"%s"%s' % shown(text, limit) if isinstance(text, str) else "(no text)"


def carried(share, record, digest):
    """True when the share record lists a file for the record with the stated
    SHA-256. The check of the bundle, which ran before, holds that a listed
    file is in the bundle, and computed its SHA-256."""
    return any(f.get("record") == record["id"] and f.get("sha256") == digest for f in share["context"].get("files") or [])


def tally(source):
    print("What the tally was given:")
    shares = {}                 # share id: the folder that had it first
    stated = {}                 # (dataset, digest): {person: [line, ...]}
    by_person = {}              # (person, dataset): {digest}
    malformed = set()
    for name in sorted(os.listdir(source)):
        folder = os.path.join(source, name)
        if not os.path.isdir(folder):
            print("  not read  %s: it is no folder" % quoted(name, 120))
            continue
        try:
            problems, facts = checkbundle.check(folder)
        except Exception as err:  # whatever goes wrong in the check: the bundle is not one that passed
            problems, facts = ["the bundle cannot be read as one: %s: %s" % (type(err).__name__, err)], None
        if problems:
            # The reason is the check's, and can name a file as its sender named it: so it is quoted.
            print("  refused   folder %s: the check of a bundle says %s" % (quoted(name, 120), quoted(sorted(problems)[0].replace(folder, "the folder"), 300)))
            continue
        share, person, holds = facts[0], facts[1], facts[7]
        if share["id"] in shares:
            print("  the same  folder %s holds the share of folder %s again: it is one share" % (quoted(name, 120), quoted(shares[share["id"]], 120)))
            continue
        shares[share["id"]] = name
        words = checkbundle.id_words(person)
        receipts = [r for r in holds if r.get("type") == "reference.file"]
        own = [r for r in receipts if r.get("author") == person]  # the check holds that a listed record is about the person
        good, bad = [], 0
        for r in own:
            ctx = r.get("context") if isinstance(r.get("context"), dict) else {}  # a record of another tool can have any context
            dataset, digest = ctx.get("dataset"), ctx.get("sha256")
            if not isinstance(dataset, str) or not isinstance(digest, str) or not DATASET_RE.fullmatch(dataset) or not DIGEST_RE.fullmatch(digest):
                malformed.add("%s, record %s: dataset %s, SHA-256 %s" % (words, r["id"][7:19], quoted(dataset, 40), quoted(digest, 80)))
                bad += 1
                continue
            good.append((dataset, digest, carried(share, r, digest)))
        print("  taken     folder %s: %d record%s of a source file by %s%s%s" % (
            quoted(name, 120), len(own), "" if len(own) == 1 else "s", words,
            "" if not bad else "; %d of them malformed, and not counted" % bad,
            "" if len(own) == len(receipts) else "; %d more, not signed by the identity's own key and not counted" % (len(receipts) - len(own))))
        note = share["context"].get("note")  # the check holds that a share record has a context
        for dataset, digest, has_file in good:
            line = "States only." if not has_file else "Carries a file: " + CARRIES
            if note and isinstance(note, str):  # a note that is no text is not shown, as the check of a bundle shows none
                line += "\n        The signer's note on this share, of which the tally took %d record%s: %s" % (len(good), "" if len(good) == 1 else "s", quoted(note, MAX_NOTE))
            stated.setdefault((dataset, digest), {}).setdefault(person, []).append(line)
            by_person.setdefault((person, dataset), set()).add(digest)

    table = {digest: dataset for dataset, digest in reproduce.KNOWN.items()}
    print()
    print("What these keys state, in the order of the datasets and of the digests:")
    if not stated:
        print("  nothing: no record of a source file was taken")
    for dataset, digest in sorted(stated):
        people = stated[(dataset, digest)]
        print("  dataset %s, SHA-256 %s: %d identit%s" % (dataset, digest, len(people), "y states it" if len(people) == 1 else "ies state it"))
        if digest in table:
            print("    This digest is in the table of bean's code, for %s%s. The table is from the project's own fetches of November 2026." % (
                table[digest], "" if table[digest] == dataset else ", and not for %s" % dataset))
            print("    A match proves no fetch, and does not show that the digest is right: a statement that was copied from the table matches too.")
        for person in sorted(people, key=checkbundle.id_words):
            n = len(by_person[(person, dataset)])
            print("    %s%s" % (checkbundle.id_words(person), "" if n == 1 else "  [this identity states %d digests for %s]" % (n, dataset)))
            for line in sorted(set(people[person])):
                print("      %s" % line)
    if malformed:
        print()
        print("Malformed, and not counted:")
        for line in sorted(malformed):
            print("  %s" % line)
    print()
    for line in OF_ITSELF:
        print(line)
    return 0


def main(argv):
    # Every text that the tally did not write goes through quoted(), so each line is plain ASCII.
    if len(argv) != 2 or argv[1].startswith("-"):
        sys.stderr.write(__doc__)
        return 2
    try:
        return tally(argv[1])
    except OSError as err:
        print("STOPPED: %s" % quoted(str(err), 300))
        return 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
