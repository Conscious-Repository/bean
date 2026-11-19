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
import hashlib
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
MAX_NOTE = 200      # characters of a note that are shown
MAX_FILE = 1 << 30  # a carried file larger than this is not read
OF_ITSELF = [
    "This tally counts keys, and a key is free.",
    "A statement can be copied with no fetch: bean's own code holds the digests.",
    "It shows what these keys state, in the bundles that it was given. It cannot show that those are all the bundles.",
    "It does not show that the source served these bytes, and nothing about whether the data is true.",
]
CARRIES = ("the share record lists a file for this record whose SHA-256, computed here, equals the stated one. "
           "Nothing shows that it is the file that the signer kept, or a file of the survey.")


def plain(text, limit=120):
    """Any text of a bundle or of a folder's name, safe on one line: every
    character but printable ASCII is written out, and so are a quotation
    mark and a backslash. So nothing shows as blank, ends a line, or closes
    a quotation."""
    text = str(text)
    out = "".join(c if " " <= c <= "~" and c not in '"\\' else "\\u{%04x}" % ord(c) for c in text[:limit])
    return out + (" [cut: %d characters in all]" % len(text) if len(text) > limit else "")


def carried(folder, share, record, digest):
    """True when the share record lists exactly one file for the record, the
    record has exactly that one stored file, and the file in the bundle has
    the stated SHA-256, computed here."""
    blobs = record.get("blobs")
    if not isinstance(blobs, list) or len(blobs) != 1 or not isinstance(blobs[0], dict):
        return False
    if (record.get("value") or {}).get("blob") != blobs[0].get("hash"):
        return False
    listed = [f for f in (share.get("context") or {}).get("files") or [] if isinstance(f, dict) and f.get("record") == record["id"]]
    if len(listed) != 1 or listed[0].get("blob") != blobs[0].get("hash") or listed[0].get("sha256") != digest:
        return False
    path = os.path.join(folder, "files", digest)
    if os.path.islink(path) or not os.path.isfile(path) or os.path.getsize(path) > MAX_FILE:
        return False
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for part in iter(lambda: f.read(1 << 20), b""):
            h.update(part)
    return h.hexdigest() == digest


def tally(source):
    print("What the tally was given:")
    shares = {}                 # share id: the folder that had it first
    stated = {}                 # (dataset, digest): {person: [line, ...]}
    by_person = {}              # (person, dataset): {digest}
    malformed = []
    for name in sorted(os.listdir(source)):
        folder = os.path.join(source, name)
        if not os.path.isdir(folder) or os.path.islink(folder):
            print("  not read  %s: it is no folder" % plain(name))
            continue
        try:
            problems, facts = checkbundle.check(folder)
        except (OSError, KeyError, TypeError, ValueError, AttributeError, IndexError) as err:
            problems, facts = ["the bundle cannot be read as one: %s: %s" % (type(err).__name__, err)], None
        if problems or not facts:
            print("  refused   folder %s: %s" % (plain(name), plain(sorted(problems)[0].replace(folder, "the folder"), 300)))
            continue
        share, person, holds = facts[0], facts[1], facts[7]
        if share["id"] in shares:
            print("  the same  folder %s holds the share of folder %s again: it is one share" % (plain(name), plain(shares[share["id"]])))
            continue
        shares[share["id"]] = name
        words = checkbundle.id_words(person)
        receipts = [r for r in holds if r.get("type") == "reference.file"]
        own = [r for r in receipts if r.get("author") == person and r.get("subject") == person]
        print("  taken     folder %s: %d records of a source file by %s%s" % (
            plain(name), len(own), words,
            "" if len(own) == len(receipts) else "; %d more that the identity's own key did not sign are not counted" % (len(receipts) - len(own))))
        good = []
        for r in sorted(own, key=lambda r: r["id"]):
            ctx = r.get("context") if isinstance(r.get("context"), dict) else {}
            dataset, digest = ctx.get("dataset"), ctx.get("sha256")
            if not isinstance(dataset, str) or not isinstance(digest, str) or not DATASET_RE.fullmatch(dataset) or not DIGEST_RE.fullmatch(digest):
                malformed.append("%s, record %s: dataset \"%s\", SHA-256 \"%s\"" % (words, r["id"][7:19], plain(dataset, 40), plain(digest, 80)))
                continue
            good.append((dataset, digest, carried(folder, share, r, digest)))
        note = (share.get("context") or {}).get("note")
        datasets = len({d for d, _, _ in good})
        for dataset, digest, has_file in good:
            line = "States only." if not has_file else "Carries a file: " + CARRIES
            if isinstance(note, str) and note:
                line += "\n        The signer's note on this share, which has %d dataset%s: \"%s\"" % (datasets, "" if datasets == 1 else "s", plain(note, MAX_NOTE))
            stated.setdefault((dataset, digest), {}).setdefault(person, []).append(line)
            by_person.setdefault((person, dataset), set()).add(digest)

    table = {digest: dataset for dataset, digest in reproduce.KNOWN.items()}
    print()
    print("What these keys state, in the order of the digests:")
    if not stated:
        print("  nothing: no record of a source file was taken")
    for dataset, digest in sorted(stated):
        people = stated[(dataset, digest)]
        print("  dataset %s, SHA-256 %s: %d identit%s" % (dataset, digest, len(people), "y states it" if len(people) == 1 else "ies state it"))
        if digest in table:
            print("    This digest is in the table of bean's code, for %s. The table is from the project's own fetches of November 2026. That is no sign of the right digest." % table[digest])
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
    for stream in (sys.stdout, sys.stderr):
        stream.reconfigure(encoding="utf-8", errors="backslashreplace")
    if len(argv) != 2 or argv[1].startswith("-"):
        sys.stderr.write(__doc__)
        return 2
    if not os.path.isdir(argv[1]):
        print("STOPPED: %s is no folder" % plain(argv[1], 300))
        return 1
    try:
        return tally(argv[1])
    except OSError as err:
        print("STOPPED: %s" % plain(err, 300))
        return 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
