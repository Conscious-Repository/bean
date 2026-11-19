#!/usr/bin/env python3
"""build.py: build the pack of a synthetic pool from opened bundles.

    build.py -synthetic -survey <survey pack> -survey-url <URL>
             -archive-url <URL> -out <new pack> -archive <new archive> <folder>
    build.py -synthetic -survey <survey pack> -survey-url <URL>
             -archive-url <URL> -out <new pack> <archive>

This is a test of the path, and no pool of people (docs/POOL-PLAN.md). The
operator says with -synthetic that every contributor is made up. The
builder cannot check that, and the pack says so.

It reads opened bundles only: a folder with one bundle in each folder
below it, or an archive that an earlier run wrote. From a folder it writes
the archive first, and then the pack that names it. From an archive it
writes no archive: the pack names the SHA-256 of the file as it was given,
and the builder stops unless it takes every bundle in it.

For each bundle it runs the check of a bundle (tools/second/checkbundle.py)
and computes each claim again (tools/compare/reproduce.py) with the survey
pack. It checks its own pack with tools/pack/check.py. It adds no check of
a bundle or of a claim. Its own rules are these:

  - a bundle is left out unless its share's note is exactly
    "synthetic contributor". That is a tripwire against an accident, and no
    proof: anyone can write the note;
  - a claim counts when it names the survey pack that the builder was given,
    computes again, and the identity's own key signed it and its reading;
  - a reading whose value pack/0 cannot hold is left out;
  - one reading for each identity and measure. An identity with two
    different bundles is left out. So is an identity with two readings of
    one measure that count, and one whose reading has claims in two cohorts;
  - two folders with one share id, the same in objects and files, are one
    bundle, and it is taken once;
  - a cell of fewer than 30 identities is left out.

It prints one line for each identity or folder, and exits 0 when it wrote
a pack.
"""
import hashlib
import os
import re
import shutil
import stat
import sys
import tempfile
import zipfile

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(HERE, "..", "second"))
sys.path.insert(0, os.path.join(HERE, "..", "pack"))
sys.path.insert(0, os.path.join(HERE, "..", "compare"))
import checkbundle  # noqa: E402
import rebuild_nhanes  # noqa: E402  (its writer of canonical JSON)
import reproduce  # noqa: E402

packcheck = reproduce.packcheck
safe = reproduce.safe

NOTE = "synthetic contributor"
NONE = "none: synthetic"
TITLE = "synthetic pool: made-up contributors, a test of the path"
BUILDER = {"name": "bean-pool-synthetic", "version": "1"}
ARCHIVE_NAME, SURVEY_NAME = "pool-bundles.zip", "SYNTHETIC-survey.pack"
COHORT_ID, COHORT_TEXT = "pool-", "synthetic pool: "
PACK_CAVEATS = [
    "synthetic pool: a test of the path, and no pool of people",
    "n counts identity ids, not people",
    "whoever ran the builder said that every contributor is made up; the builder cannot check that",
]
MEASURE_CAVEATS = ["each value is the reading of one identity id, as that identity signed it"]
COHORT_CAVEATS = ["each identity chose this cohort; the builder cannot check that it belongs to it"]

MAX_ARCHIVE = 64 << 20   # an archive is not larger than this, packed or open
MAX_ENTRIES = 100000
ENTRY_RE = re.compile(r"([0-9a-f]{64})/(objects/[0-9a-f]{2}/[0-9a-f]{62}\.json|files/[0-9a-f]{64})")
ZIP_TIME = (1980, 1, 1, 0, 0, 0)


class Stop(Exception):
    pass


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def members(folder):
    """The files of a bundle: plain files under objects/ and files/, and
    nothing else of the folder. Returns {path in the bundle: bytes}."""
    out = {}
    for top in ("objects", "files"):
        base = os.path.join(folder, top)
        if not os.path.isdir(base) or os.path.islink(base):
            continue
        for here, dirs, names in os.walk(base):
            dirs[:] = sorted(d for d in dirs if not d.startswith(".") and not os.path.islink(os.path.join(here, d)))
            for name in sorted(names):
                full = os.path.join(here, name)
                if name.startswith(".") or not stat.S_ISREG(os.lstat(full).st_mode):
                    continue
                with open(full, "rb") as f:
                    out[os.path.relpath(full, folder).replace(os.sep, "/")] = f.read()
    return out


def content_id(files):
    h = hashlib.sha256()
    for path in sorted(files):
        h.update(("%s\0%d\0" % (path, len(files[path]))).encode())
        h.update(files[path])
    return h.hexdigest()


def open_archive(path, into):
    """Writes the bundles of an archive into a new folder. It takes no path
    that leaves that folder, no link, and no more than MAX_ARCHIVE."""
    if os.path.getsize(path) > MAX_ARCHIVE:
        raise Stop("the archive is larger than an archive can be")
    try:
        z = zipfile.ZipFile(path)
    except (zipfile.BadZipFile, OSError) as err:
        raise Stop("the archive cannot be read: %s" % err)
    with z:
        infos = z.infolist()
        if len(infos) > MAX_ENTRIES or sum(i.file_size for i in infos) > MAX_ARCHIVE:
            raise Stop("the archive holds more than an archive can")
        seen = set()
        for info in infos:
            name = info.filename
            if name in seen or not ENTRY_RE.fullmatch(name) or info.is_dir() or (info.external_attr >> 16) & 0o170000 not in (0, 0o100000):
                raise Stop("the archive has an entry that is not a file of a bundle: %s" % safe(name)[:120])
            seen.add(name)
            data = z.read(info)
            if len(data) != info.file_size:
                raise Stop("the archive has an entry of another size than it states")
            full = os.path.join(into, *name.split("/"))
            os.makedirs(os.path.dirname(full), exist_ok=True)
            with open(full, "xb") as f:
                f.write(data)


def write_archive(path, bundles):
    """One zip file of the bundles, the same bytes for the same bundles:
    entries in one order, stored and not compressed, with fixed times,
    modes and system. bundles is {share id in hex: {path: bytes}}."""
    with open(path, "xb") as f, zipfile.ZipFile(f, "w", zipfile.ZIP_STORED) as z:
        for sid in sorted(bundles):
            for rel in sorted(bundles[sid]):
                info = zipfile.ZipInfo("%s/%s" % (sid, rel), ZIP_TIME)
                info.compress_type = zipfile.ZIP_STORED
                info.create_system = 3
                info.external_attr = 0o100644 << 16
                z.writestr(info, bundles[sid][rel])


def read_bundle(name, folder, survey_path, survey_sha):
    """One folder. Returns ("refused", why) for a folder that is no good
    bundle of a synthetic contributor, or ("bundle", facts)."""
    try:
        problems, facts = checkbundle.check(folder)
    except (OSError, KeyError, TypeError, ValueError, AttributeError, IndexError) as err:
        problems, facts = ["the bundle cannot be read as one: %s: %s" % (type(err).__name__, err)], None
    if problems or not facts:
        return "refused", "the bundle fails its check: %s" % sorted(problems)[0]
    share, person, holds = facts[0], facts[1], facts[7]
    if (share.get("context") or {}).get("note") != NOTE:
        return "refused", "the note of its share is not exactly %r" % NOTE
    files = members(folder)
    inputs = reproduce.Inputs()
    inputs.add_pack(survey_path)
    for rel in sorted(files):
        if rel.startswith("objects/"):
            inputs.add_record(rel, files[rel])
    counted, notes = [], []
    for claim in sorted((r for r in holds if r.get("type") == "comparison.percentile"), key=lambda r: r["id"]):
        short = claim["id"][7:19]
        ctx = claim.get("context") or {}
        ref = ctx.get("reference") if isinstance(ctx.get("reference"), dict) else {}
        if ref.get("pack") != survey_sha:
            notes.append("claim %s names another pack than the survey pack that the builder was given" % short)
            continue
        try:
            reproduce.check(claim, inputs)
        except reproduce.Missing as err:
            notes.append("claim %s cannot be computed again: %s" % (short, err))
            continue
        except (KeyError, TypeError, ValueError, AttributeError, OSError, OverflowError, packcheck.NotPlaced) as err:
            notes.append("claim %s does not compute again: %s" % (short, err))
            continue
        reading = inputs.records[ctx["reading"]]
        if claim.get("author") != person or reading.get("author") != person or reading.get("subject") != person:
            notes.append("claim %s: the identity's own key did not sign the claim and its reading" % short)
            continue
        value = reading["value"]["n"]
        if isinstance(value, bool) or not isinstance(value, (int, float)) or not packcheck.MIN_NUMBER <= value <= packcheck.MAX_NUMBER:
            notes.append("claim %s: pack/0 cannot hold the value of its reading" % short)
            continue
        counted.append((ref["measure"], ref["cohort"], reading["id"], value))
    return "bundle", {"share": share["id"][7:], "person": person, "files": files, "content": content_id(files),
                      "counted": counted, "notes": notes, "name": name}


def build(args):
    survey_path = args["survey"]
    survey_sha = sha256_file(survey_path)
    try:
        survey = packcheck.read_pack(survey_path)
    except packcheck.Refused as err:
        raise Stop("the survey pack is not a pack: %s" % err)
    print("survey pack sha256 %s" % survey_sha)

    source, tmp = args["input"], None
    from_archive = os.path.isfile(source)
    if from_archive and args["archive"]:
        raise Stop("an archive is given: -archive names a new one, and the builder writes none from an archive")
    if not from_archive and not args["archive"]:
        raise Stop("a folder is given: -archive names the new archive that the pack will name")
    for new in (args["out"], args["archive"]):
        if new and os.path.lexists(new):
            raise Stop("%s is there already: the builder writes over nothing" % new)
    try:
        if from_archive:
            tmp = tempfile.mkdtemp(prefix="bean-pool-")
            open_archive(source, tmp)
            source = tmp
        elif not os.path.isdir(source):
            raise Stop("%s is no folder and no archive" % source)
        return assemble(args, source, from_archive, survey, survey_path, survey_sha)
    finally:
        if tmp:
            shutil.rmtree(tmp, ignore_errors=True)


def assemble(args, source, from_archive, survey, survey_path, survey_sha):
    left = 0
    by_share, order = {}, []
    for name in sorted(os.listdir(source)):
        folder = os.path.join(source, name)
        if name.startswith(".") or not os.path.isdir(folder) or os.path.islink(folder):
            continue
        kind, got = read_bundle(name, folder, survey_path, survey_sha)
        if kind == "refused":
            print("left out  folder %s: %s" % (safe(name), safe(got)))
            left += 1
            continue
        earlier = by_share.get(got["share"])
        if earlier is None:
            by_share[got["share"]] = got
            order.append(got["share"])
        elif earlier["content"] == got["content"]:
            print("the same  folder %s is the bundle of folder %s again: it is taken once" % (safe(name), safe(earlier["name"])))
        else:
            earlier["void"] = True
            print("left out  folder %s: another folder has its share id and other bytes" % safe(name))
            left += 1
    by_person = {}
    for sid in order:
        by_person.setdefault(by_share[sid]["person"], []).append(by_share[sid])

    cells, taken = {}, {}
    for person in sorted(by_person):
        words = checkbundle.id_words(person)
        bundles = by_person[person]
        for b in bundles:
            for note in b["notes"]:
                print("not counted  %s: %s" % (words, safe(note)))
        why = None
        if any(b.get("void") for b in bundles):
            why = "two folders have one share id and other bytes"
        elif len(bundles) > 1:
            why = "two different bundles of one identity"
        else:
            counted = bundles[0]["counted"]
            by_measure, by_reading = {}, {}
            for measure, cohort, rid, _ in counted:
                by_measure.setdefault(measure, set()).add(rid)
                by_reading.setdefault(rid, set()).add(cohort)
            if not counted:
                why = "no claim that counts"
            elif any(len(v) > 1 for v in by_measure.values()):
                why = "two readings of one measure that count"
            elif any(len(v) > 1 for v in by_reading.values()):
                why = "a reading with claims in two cohorts"
        if why:
            print("left out  %s: %s" % (words, why))
            left += 1
            continue
        b = bundles[0]
        taken[b["share"]] = b["files"]
        for measure, cohort, _, value in sorted(set((m, c, r, v) for m, c, r, v in b["counted"])):
            cells.setdefault((measure, cohort), []).append(value)
        print("taken     %s: %d of its readings" % (words, len(set(r for _, _, r, _ in b["counted"]))))

    if from_archive and left:
        raise Stop("the archive holds a bundle that the builder does not take: a pack is built from an archive only when all of it is taken")

    kept = {}
    for (measure, cohort), values in sorted(cells.items()):
        if len(values) < packcheck.MIN_N:
            print("dropped   cell %s %s%s: %d identities, fewer than %d" % (safe(measure), COHORT_ID, safe(cohort), len(values), packcheck.MIN_N))
        else:
            kept[(measure, cohort)] = values
    if not kept:
        raise Stop("no cell has %d identities or more: no pack is written" % packcheck.MIN_N)

    # The archive first, then the pack that names it.
    if from_archive:
        archive_path = args["input"]
    else:
        archive_path = args["archive"]
        write_archive(archive_path, taken)
        print("wrote the archive %s: %d bundles. An archive shows every value and every identity id in it." % (archive_path, len(taken)))
    archive_sha, archive_bytes = sha256_file(archive_path), os.path.getsize(archive_path)
    print("archive sha256 %s" % archive_sha)

    pack = {
        "bean": packcheck.FORMAT,
        "title": TITLE,
        "source": {"name": NONE, "publisher": NONE, "home": NONE, "files": [
            {"name": ARCHIVE_NAME, "sha256": archive_sha, "bytes": archive_bytes, "url": args["archive_url"]},
            {"name": SURVEY_NAME, "sha256": survey_sha, "bytes": os.path.getsize(survey_path), "url": args["survey_url"]},
        ]},
        "builder": dict(BUILDER),
        "caveats": list(PACK_CAVEATS),
        "measures": {}, "cohorts": {}, "cells": [],
    }
    for (measure, cohort), values in sorted(kept.items()):
        m = survey["measures"][measure]
        pack["measures"][measure] = {"label": m["label"], "unit": m["unit"], "caveats": list(MEASURE_CAVEATS),
                                     "same": [{"type": s["type"], "unit": s["unit"]} for s in m["same"]]}
        pack["cohorts"][COHORT_ID + cohort] = {"text": COHORT_TEXT + survey["cohorts"][cohort]["text"], "caveats": list(COHORT_CAVEATS)}
        distinct = sorted(set(values))
        pack["cells"].append({"measure": measure, "cohort": COHORT_ID + cohort, "kind": "weights", "n": len(values),
                              "values": distinct, "weights": [values.count(v) for v in distinct]})
        print("cell      %s %s%s: n %d" % (safe(measure), COHORT_ID, safe(cohort), len(values)))
    data = (rebuild_nhanes.canonical(pack) + "\n").encode("utf-8")
    try:
        packcheck.parse_pack(data)
    except packcheck.Refused as err:
        raise Stop("the builder made a pack that its own check refuses: %s" % err)
    with open(args["out"], "xb") as f:
        f.write(data)
    print("wrote the pack %s" % args["out"])
    print("pack sha256 %s" % hashlib.sha256(data).hexdigest())
    return 0


def main(argv):
    args = {"synthetic": False, "survey": None, "survey_url": None, "archive_url": None, "out": None, "archive": None, "input": None}
    rest, flags = argv[1:], {"-survey": "survey", "-survey-url": "survey_url", "-archive-url": "archive_url", "-out": "out", "-archive": "archive"}
    while rest:
        arg = rest.pop(0)
        if arg == "-synthetic":
            args["synthetic"] = True
        elif arg in flags and rest:
            args[flags[arg]] = rest.pop(0)
        elif not arg.startswith("-") and args["input"] is None:
            args["input"] = arg
        else:
            sys.stderr.write(__doc__)
            return 2
    if not args["synthetic"]:
        print("the builder is for made-up contributors only: say so with -synthetic, or do not run it")
        return 2
    if not all(args[k] for k in ("survey", "survey_url", "archive_url", "out", "input")):
        sys.stderr.write(__doc__)
        return 2
    try:
        return build(args)
    except Stop as err:
        print("STOPPED: %s" % safe(str(err)))
        return 1
    except OSError as err:
        print("STOPPED: %s" % safe(str(err)))
        return 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
