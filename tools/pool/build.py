#!/usr/bin/env python3
"""build.py: build the pack of a synthetic pool from opened bundles.

    build.py -synthetic -survey <survey pack> -survey-url <URL>
             -archive-url <URL> -out <new pack> -archive <new archive> <folder>
    build.py -synthetic -survey <survey pack> -survey-url <URL>
             -archive-url <URL> -out <new pack> <archive>

This is a test of the path, and no pool of people (docs/POOL-PLAN.md). Do
not run it on shares of real people. The operator says with -synthetic that
every contributor is made up. The builder cannot check that, and the pack
says so.

It reads opened bundles only: a folder with one bundle in each folder
below it, or an archive that an earlier run wrote. Of a bundle that it takes
it reads the record files under objects/, and the archive holds those and
nothing else of the folder: not the README.txt that bean writes into a
bundle, and no file whose name starts with a dot. From a folder it writes
the archive first, and then the pack that names it. From an archive it
writes no archive: the pack names the SHA-256 of the file as it was given.
It takes an archive only when it takes every bundle in it, and when the
file is, byte for byte, the archive that the builder writes of them.

For each bundle it runs the check of a bundle (tools/second/checkbundle.py)
and computes each claim again (tools/compare/reproduce.py) with the survey
pack. It checks its own pack with tools/pack/check.py. It adds no check of
a bundle or of a claim. Its own rules are these:

  - a bundle is left out unless its share's note is exactly
    "synthetic contributor". That is a tripwire against an accident, and no
    proof: anyone can write the note;
  - a bundle is left out when it holds more than comparisons and the
    readings that they are about: another record, or any file that a
    record carries. An archive shows all the records of a bundle;
  - a claim counts when it computes again with the survey pack that the
    builder was given, and the identity's own key signed its reading. The
    check of a claim holds that the person signed the claim. A bundle with
    a claim that does not count is left out;
  - a bundle whose records are larger together than 1 MiB is left out;
  - a reading whose value pack/0 cannot hold is left out;
  - one reading for each identity and measure. An identity with two
    different bundles that pass their check is left out, whatever their
    notes. So is an identity with two readings of one measure that count,
    and one whose reading has claims in two cohorts;
  - two folders with one share id are one bundle, and it is taken once.

A cell has no least number of identities (DECISIONS 333): a cell of one
identity is that identity's value, and the pack shows it.

It prints one line for each identity or folder. A line on a claim that does
not compute again holds the value of its reading. It exits 0 when it wrote
a pack.
"""
import hashlib
import io
import os
import re
import shutil
import sys
import tempfile
import zipfile
import zlib

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
# The names of the two source files in the pack. They are labels, and the
# same whatever the files are called on a disk.
ARCHIVE_NAME, SURVEY_NAME = "pool-bundles.zip", "survey.pack"
COHORT_ID, COHORT_TEXT = "pool-", "synthetic pool: "
PACK_CAVEATS = [
    "synthetic pool: a test of the path, and no pool of people",
    "n counts identity ids, not people",
    "whoever ran the builder said that every contributor is made up; the builder cannot check that",
]
MEASURE_CAVEATS = ["each value is the reading of one identity id, as that identity signed it"]
COHORT_CAVEATS = ["each identity chose this cohort; the builder cannot check that it belongs to it"]
ARCHIVE_SHOWS = "An archive shows every record of every bundle in it, whole: each value, each identity id and key, and each time."

MAX_ARCHIVE = 64 << 20   # an archive is not larger than this, and its entries together are not
MAX_BUNDLE = 1 << 20     # the records of one bundle together are not larger than this
ENTRY_RE = re.compile(r"[0-9a-f]{64}/objects/[0-9a-f]{2}/[0-9a-f]{62}\.json")
ZIP_TIME = (1980, 1, 1, 0, 0, 0)


class Stop(Exception):
    pass


def members(folder):
    """The record files of a bundle that passed its check: what lies under
    objects/, but for names that start with a dot. The check has refused a
    link there, and anything that is no plain file. Returns {path in the
    bundle: bytes}, or None when they are larger together than MAX_BUNDLE."""
    out, size = {}, 0
    for here, dirs, names in os.walk(os.path.join(folder, "objects")):
        dirs[:] = sorted(d for d in dirs if not d.startswith("."))
        for name in sorted(n for n in names if not n.startswith(".")):
            full = os.path.join(here, name)
            size += os.path.getsize(full)
            if size > MAX_BUNDLE:
                return None
            with open(full, "rb") as f:
                out[os.path.relpath(full, folder).replace(os.sep, "/")] = f.read()
    return out


def carries_files(folder):
    """True when the bundle has a file under files/. Nothing of it is read."""
    base = os.path.join(folder, "files")
    return os.path.isdir(base) and any(not n.startswith(".") for n in os.listdir(base))


def archive_bytes(bundles):
    """One zip file of the bundles, the same bytes for the same bundles:
    entries in one order, stored and not compressed, with fixed times,
    modes and system. bundles is {share id in hex: {path: bytes}}."""
    out = io.BytesIO()
    with zipfile.ZipFile(out, "w", zipfile.ZIP_STORED) as z:
        for sid in sorted(bundles):
            for rel in sorted(bundles[sid]):
                info = zipfile.ZipInfo("%s/%s" % (sid, rel), ZIP_TIME)
                info.compress_type = zipfile.ZIP_STORED
                info.create_system = 3
                info.external_attr = 0o100644 << 16
                z.writestr(info, bundles[sid][rel])
    return out.getvalue()


def open_archive(data, into):
    """Writes the bundles of an archive into a new folder. It takes only
    stored entries that are named as files of a bundle: no path that leaves
    the folder, nothing packed, and no more than MAX_ARCHIVE. So no entry
    grows when it is read. What else an archive must be, the comparison
    with the builder's own bytes holds, in assemble."""
    try:
        z = zipfile.ZipFile(io.BytesIO(data))
    except (zipfile.BadZipFile, NotImplementedError, OSError, ValueError) as err:
        raise Stop("the archive cannot be read: %s" % err)
    with z:
        infos = z.infolist()
        seen = set()
        for info in infos:
            name = info.filename
            if name in seen or not ENTRY_RE.fullmatch(name) or info.compress_type != zipfile.ZIP_STORED:
                raise Stop("the archive has an entry that is not a stored file of a bundle: %s" % safe(name)[:120])
            seen.add(name)
        if sum(i.file_size for i in infos) > MAX_ARCHIVE:
            raise Stop("the archive holds more than an archive can")
        for info in infos:
            try:
                content = z.read(info)
            except (zipfile.BadZipFile, RuntimeError, NotImplementedError, EOFError, zlib.error, OSError, ValueError) as err:
                raise Stop("the archive is damaged at %s: %s" % (safe(info.filename)[:120], err))
            full = os.path.join(into, *info.filename.split("/"))
            os.makedirs(os.path.dirname(full), exist_ok=True)
            with open(full, "wb") as f:
                f.write(content)


def read_bundle(name, folder, survey_path):
    """One folder. Returns ("refused", why) for a folder that fails the
    check of a bundle, or ("bundle", facts). facts["why"] says why a bundle
    that passed its check is not taken, when it is not."""
    try:
        problems, facts = checkbundle.check(folder)
    except (OSError, KeyError, TypeError, ValueError, AttributeError, IndexError) as err:
        problems, facts = ["the bundle cannot be read as one: %s: %s" % (type(err).__name__, err)], None
    if problems or not facts:
        # The check names the folder by its path, which says nothing here.
        return "refused", "the bundle fails its check: %s" % sorted(problems)[0].replace(folder, "the folder")
    share, person, holds = facts[0], facts[1], facts[7]
    got = {"share": share["id"][7:], "person": person, "files": None, "counted": [], "notes": [], "name": name, "why": None}
    if (share.get("context") or {}).get("note") != NOTE:
        got["why"] = "the note of its share is not exactly %r: this builder takes made-up contributors only" % NOTE
        return "bundle", got
    claims = [r for r in holds if r.get("type") == "comparison.percentile"]
    about = {c["context"]["reading"] for c in claims if isinstance(c.get("context"), dict) and isinstance(c["context"].get("reading"), str)}
    if any(r.get("type") != "comparison.percentile" and r["id"] not in about for r in holds) or carries_files(folder):
        got["why"] = "it holds more than comparisons and the readings that they are about"
        return "bundle", got
    files = got["files"] = members(folder)
    if files is None:
        got["why"] = "its records are larger together than a bundle of this pool can be"
        return "bundle", got
    inputs = reproduce.Inputs()
    inputs.add_pack(survey_path)
    for rel in sorted(files):
        inputs.add_record(rel, files[rel])
    for claim in claims:
        short = claim["id"][7:19]
        try:
            # The survey pack is the only pack among the inputs: a claim
            # that names another pack does not compute again.
            reproduce.check(claim, inputs)
        except (reproduce.Missing, KeyError, TypeError, ValueError, AttributeError, OSError, OverflowError, packcheck.NotPlaced) as err:
            got["notes"].append("claim %s does not compute again: %s" % (short, err))
            continue
        ref, reading = claim["context"]["reference"], inputs.records[claim["context"]["reading"]]
        if reading.get("author") != person:  # the check of a claim holds that the reading is about the person
            got["notes"].append("claim %s: the identity's own key did not sign its reading" % short)
            continue
        value = reading["value"]["n"]
        if not packcheck.MIN_NUMBER <= value <= packcheck.MAX_NUMBER:  # the check of a claim holds that it is a number
            got["notes"].append("claim %s: pack/0 cannot hold the value of its reading" % short)
            continue
        got["counted"].append((ref["measure"], ref["cohort"], reading["id"], value))
    return "bundle", got


def build(args):
    survey_path = args["survey"]
    try:
        with open(survey_path, "rb") as f:
            survey_data = f.read(packcheck.MAX_BYTES + 1)
        survey = packcheck.parse_pack(survey_data)
    except OSError as err:
        raise Stop("-survey: %s" % err)
    except packcheck.Refused as err:
        raise Stop("-survey: the file is not a pack: %s" % err)
    survey_sha = hashlib.sha256(survey_data).hexdigest()
    print("survey pack sha256 %s" % survey_sha)

    source = args["input"]
    from_archive = os.path.isfile(source)
    if from_archive and args["archive"]:
        raise Stop("an archive is given: -archive names a new one, and the builder writes none from an archive")
    if not from_archive and not args["archive"]:
        raise Stop("a folder is given: -archive names the new archive that the pack will name")
    for flag, new in (("-out", args["out"]), ("-archive", args["archive"])):
        if new and os.path.lexists(new):
            raise Stop("%s: %s is there already: the builder writes over nothing" % (flag, new))
        if new and not os.path.isdir(os.path.dirname(os.path.abspath(new))):
            raise Stop("%s: the folder of %s is not there" % (flag, new))

    tmp, given = None, None
    try:
        if from_archive:
            if os.path.getsize(source) > MAX_ARCHIVE:
                raise Stop("the archive is larger than an archive can be")
            with open(source, "rb") as f:
                given = f.read()
            tmp = tempfile.mkdtemp(prefix="bean-pool-")
            open_archive(given, tmp)
            source = tmp
        return assemble(args, source, given, survey, survey_path, survey_sha, len(survey_data))
    finally:
        if tmp:
            shutil.rmtree(tmp, ignore_errors=True)


def assemble(args, source, given, survey, survey_path, survey_sha, survey_bytes):
    from_archive = given is not None
    left = 0
    by_share, order = {}, []
    for name in sorted(os.listdir(source)):
        folder = os.path.join(source, name)
        if not os.path.isdir(folder):
            print("not read  %s: it is no folder" % safe(name))
            left += 1
            continue
        kind, got = read_bundle(name, folder, survey_path)
        if kind == "refused":
            print("left out  folder %s: %s" % (safe(name), safe(got)))
            left += 1
            continue
        earlier = by_share.get(got["share"])
        if earlier is None:
            by_share[got["share"]] = got
            order.append(got["share"])
        else:
            # Two folders that pass the check with one share id hold the
            # same records and files: the share record lists each one.
            print("the same  folder %s is the bundle of folder %s again: it is taken once" % (safe(name), safe(earlier["name"])))
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
        if len(bundles) > 1:
            why = "two different bundles of one identity: folders %s" % ", ".join(safe(b["name"]) for b in bundles)
        else:
            b = bundles[0]
            by_measure, by_reading = {}, {}
            for measure, cohort, rid, _ in b["counted"]:
                by_measure.setdefault(measure, set()).add(rid)
                by_reading.setdefault(rid, set()).add(cohort)
            reason = b["why"]
            if reason:
                pass
            elif b["notes"]:
                reason = "a claim of its bundle does not count"
            elif not b["counted"]:
                reason = "its bundle has no claim"
            elif any(len(v) > 1 for v in by_measure.values()):
                reason = "two readings of one measure that count"
            elif any(len(v) > 1 for v in by_reading.values()):
                reason = "a reading with claims in two cohorts"
            if reason:
                why = "folder %s: %s" % (safe(b["name"]), reason)
        if why:
            print("left out  %s: %s" % (words, why))
            left += 1
            continue
        b = bundles[0]
        taken[b["share"]] = b["files"]
        for measure, cohort, _, value in sorted(set(b["counted"])):
            cells.setdefault((measure, cohort), []).append(value)
        print("taken     %s: %d of its readings" % (words, len(set(r for _, _, r, _ in b["counted"]))))

    if from_archive and left:
        raise Stop("the archive holds something that the builder does not take: a pack is built from an archive only when all of it is taken")

    if not cells:
        raise Stop("no identity is taken: no pack is written")

    archive = archive_bytes(taken)
    if len(archive) > MAX_ARCHIVE:
        raise Stop("the archive of these bundles is larger than an archive can be: no pack is written")
    if from_archive and archive != given:
        raise Stop("the archive is not, byte for byte, the one that the builder writes of its bundles: a pack names only such an archive")
    archive_sha = hashlib.sha256(archive).hexdigest()

    pack = {
        "bean": packcheck.FORMAT,
        "title": TITLE,
        "source": {"name": NONE, "publisher": NONE, "home": NONE, "files": [
            {"name": ARCHIVE_NAME, "sha256": archive_sha, "bytes": len(archive), "url": args["archive_url"]},
            {"name": SURVEY_NAME, "sha256": survey_sha, "bytes": survey_bytes, "url": args["survey_url"]},
        ]},
        "builder": dict(BUILDER),
        "caveats": list(PACK_CAVEATS),
        "measures": {}, "cohorts": {}, "cells": [],
    }
    for (measure, cohort), values in sorted(cells.items()):
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
        raise Stop("the builder made a pack that its own check refuses, and wrote nothing: %s" % err)
    # The archive first, then the pack that names it: both, or neither.
    if not from_archive:
        print(ARCHIVE_SHOWS)
    made = []
    try:
        for path, content in ([] if from_archive else [(args["archive"], archive)]) + [(args["out"], data)]:
            with open(path, "xb") as f:
                made.append(path)
                f.write(content)
    except OSError as err:
        for path in made:
            os.remove(path)
        raise Stop("%s: no file is left behind" % err)
    if not from_archive:
        print("wrote the archive %s: %d bundles" % (args["archive"], len(taken)))
    print("archive sha256 %s" % archive_sha)
    print("wrote the pack %s" % args["out"])
    print("pack sha256 %s" % hashlib.sha256(data).hexdigest())
    return 0


def main(argv):
    # Text of a bundle is printed. It must not stop the run on a terminal
    # that cannot show it.
    for stream in (sys.stdout, sys.stderr):
        stream.reconfigure(encoding="utf-8", errors="backslashreplace")
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
        print("STOPPED: this builder is for made-up contributors only. Do not run it on shares of real people.")
        print("The flag -synthetic is the operator's word that every contributor is made up. Nothing checks it.")
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
