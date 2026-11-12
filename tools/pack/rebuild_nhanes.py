#!/usr/bin/env python3
"""rebuild_nhanes.py: build an NHANES pack again from the NHANES files,
without bean, and compare it with the pack byte for byte.

    rebuild_nhanes.py <pack> <xpt file>...

It checks that the pack is a good pack (tools/pack/check.py), that each file
given has the SHA-256 the pack names for it (a file is matched by its data
set name, such as DEMO_J, whatever the case of its name on disk), and that
every file the pack names is given. Then it builds the whole pack from the
files and from its own tables: the title, the source, the caveats, each
measure with its label, its unit and the types that it takes, each cohort,
and each cell by steps 1 to 5 of "The method, version 1" in docs/COMPARE.md.
It writes that pack in canonical JSON and compares the bytes.

So a pack passes only when every member is what the files and the builder
give: a table, and also a caveat, a title, or which reading a measure
takes. It shares no code with bean.

It prints `rebuilt <pack>: the pack is, byte for byte, the one that these
files give` and exits 0, or one line `DIFFERS: ...` that names the first
member that differs (or `REFUSED: ...` for a pack or a file that cannot be
used) and exits 1.
"""
import hashlib
import json
import math
import os
import struct
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
sys.path.insert(0, os.path.join(HERE, "..", "compare"))
import check as packcheck  # noqa: E402
import reproduce  # noqa: E402

MIN_N = 30
DEMO, SEX, AGE, WEIGHT = "DEMO_J", "RIAGENDR", "RIDAGEYR", "WTMEC2YR"


class Stop(Exception):
    pass


def bits(x):
    return struct.pack("<d", float(x))


def dataset(name):
    """The data set name of a file: DEMO_J of demo_j.XPT."""
    return os.path.splitext(os.path.basename(name))[0].upper()


def load(pack, paths):
    entries = {dataset(f["name"]): f for f in pack["source"]["files"]}
    given = {}
    usable = {DEMO} | {spec["dataset"] for spec in specs().values()}
    for path in paths:
        d = dataset(path)
        if d in given:
            raise Stop("REFUSED: the data set %s is given twice" % d)
        # A file that a pack holds nothing of is left out, as the builder
        # leaves it out: so a folder of downloads can be given whole.
        if d not in usable:
            print("not used: %s" % reproduce.safe(path))
            continue
        if d not in entries:
            raise Stop("REFUSED: %s is a file that the builder uses, and the pack does not name it: the pack was not built from these files" % reproduce.safe(path))
        with open(path, "rb") as f:
            digest = hashlib.sha256(f.read()).hexdigest()
        if digest != entries[d]["sha256"]:
            raise Stop("REFUSED: %s does not have the SHA-256 the pack names for %s (%s)" % (reproduce.safe(path), entries[d]["name"], entries[d]["sha256"]))
        given[d] = path
    for d, f in entries.items():
        if d not in given:
            raise Stop("REFUSED: the file %s that the pack names is not given" % reproduce.safe(f["name"]))
    if DEMO not in given:
        raise Stop("REFUSED: the pack does not name %s, which holds sex, age and the weight" % DEMO)
    return given


def demographics(table):
    out = {}
    for col in (SEX, AGE, WEIGHT):
        if col not in table.columns:
            raise Stop("DIFFERS: %s has no %s" % (DEMO, col))
    for seqn, sex, age, w in zip(table["SEQN"], table[SEX], table[AGE], table[WEIGHT]):
        out[seqn] = (sex, age, w)
    return out


def rows_of(table, variable, demo):
    """Steps 1 to 4 for the measure, apart from the cohort's own range:
    (value, sex, age, weight) of each row in file order that has a value, a
    demographic row and a weight above zero."""
    rows = []
    for seqn, x in zip(table["SEQN"], table[variable]):
        if math.isnan(x):
            continue
        if seqn not in demo:
            raise Stop("DIFFERS: the files are not of one release: SEQN %s of the file of %s is not in %s" % (seqn, variable, DEMO))
        sex, age, w = demo[seqn]
        if math.isnan(sex) or math.isnan(age):
            continue
        if not (w > 0) or math.isinf(w):
            continue
        rows.append((float(x), sex, age, float(w)))
    return rows


def cell_of(rows, sex, lo, hi):
    """Step 5: n and, for each distinct value in rising order, the sum of the
    weights of its rows, added in file order."""
    n, by_value = 0, {}
    for x, s, a, w in rows:
        if s != sex or not lo <= a <= hi:
            continue
        n += 1
        by_value[x] = by_value.get(x, 0.0) + w
    values = sorted(by_value)
    return n, values, [by_value[x] for x in values]


# What the builder writes beside the tables. bean holds the same texts and
# the same rows (internal/nhanes); this tool builds the whole pack from its
# own, so a pack that differs in any member, not only in a table, is found.
TITLE = "NHANES 2017-2018: adults by sex and ten years of age"
SOURCE = {"name": "NHANES 2017-2018", "publisher": "US National Center for Health Statistics (CDC)", "home": "https://wwwn.cdc.gov/nchs/nhanes/"}
URL = "https://wwwn.cdc.gov/Nchs/Data/Nhanes/Public/2017/DataFiles/"
BUILDER = {"name": "bean-pack-nhanes", "version": "1"}
CAVEAT_BODY = "the reading and NHANES were measured with their own instruments and methods"
# The body measures: the data set, the variable, and each type and unit
# that is the same quantity.
BODY = [
    ("BMX_J", "BMXWT", [("weight", "kg"), ("apple-health.body-mass", "kg")]),
]
AGES = [(20, 29), (30, 39), (40, 49), (50, 59), (60, 69), (70, 79), (80, 80)]


def number(x):
    """A number as bean's canonical JSON writes it (RFC 8785): the shortest
    digits that read back as the same double, with an exponent only below
    1e-6 and from 1e21."""
    if isinstance(x, int):
        return str(x)
    if x == int(x) and abs(x) < 1e21:
        return str(int(x))
    if abs(x) >= 1e-6:
        return packcheck.num(x)
    mantissa, _, exponent = repr(x).partition("e")
    return "%se-%d" % (mantissa, -int(exponent))


def canonical(x):
    if isinstance(x, dict):
        return "{" + ",".join(json.dumps(k, ensure_ascii=False) + ":" + canonical(x[k]) for k in sorted(x)) + "}"
    if isinstance(x, list):
        return "[" + ",".join(canonical(v) for v in x) + "]"
    if isinstance(x, str):
        return json.dumps(x, ensure_ascii=False)
    return number(x)


def specs():
    """Each measure that a pack can hold, in the builder's order: the lab
    tests, each once with every name and unit that a report prints, then
    the body measures."""
    out = {}
    for (kind, unit), (d, variable) in reproduce.ANALYTES.items():
        out.setdefault(variable, {"dataset": d, "caveat": reproduce.CAVEAT_METHOD, "same": []})["same"].append({"type": kind, "unit": unit})
    for d, variable, same in BODY:
        out[variable] = {"dataset": d, "caveat": CAVEAT_BODY, "same": [{"type": t, "unit": u} for t, u in same]}
    return out


def build(given, sizes, digests):
    """The whole pack that the files give, as the builder writes it."""
    tables = {}

    def table(d):
        if d not in tables:
            tables[d] = reproduce.read_xpt(given[d])
        return tables[d]

    demo = demographics(table(DEMO))
    caveats = [reproduce.CAVEAT_POPULATION]
    files = []
    for d in sorted(given):
        files.append({"name": d + ".xpt", "sha256": digests[d], "bytes": sizes[d], "url": URL + d + ".xpt"})
        if reproduce.KNOWN.get(d) != digests[d]:
            caveats.append(reproduce.CAVEAT_UNKNOWN % d)
    cohorts, menu = {}, []
    for sex, code in (("female", 2), ("male", 1)):
        for lo, hi in AGES:
            cid = "%s-%d-over" % (sex, lo) if hi == 80 else "%s-%d-%d" % (sex, lo, hi)
            text = "%s, age %d and over" % (sex, lo) if hi == 80 else "%s, age %d-%d" % (sex, lo, hi)
            cohorts[cid] = {"text": text, "caveats": [reproduce.CAVEAT_TOP_AGE] if hi == 80 else []}
            menu.append((cid, code, lo, hi))
    measures, cells = {}, []
    for variable, spec in specs().items():
        d = spec["dataset"]
        if d not in given or variable not in table(d).columns:
            continue
        label = table(d).attrs["labels"].get(variable) or ""
        if not label.endswith(")") or "(" not in label:
            raise Stop("DIFFERS: %s is labelled %r in %s, with no unit" % (variable, label, d))
        measures[variable] = {"label": label, "unit": label[label.rindex("(") + 1:-1], "caveats": [spec["caveat"]], "same": spec["same"]}
        rows = rows_of(table(d), variable, demo)
        for cid, code, lo, hi in menu:
            n, values, weights = cell_of(rows, code, lo, hi)
            if n >= MIN_N:
                cells.append({"measure": variable, "cohort": cid, "kind": "weights", "n": n, "values": values, "weights": weights})
    cells.sort(key=lambda c: (c["measure"], c["cohort"]))
    return {"bean": "pack/0", "title": TITLE, "source": dict(SOURCE, files=files), "builder": BUILDER,
            "caveats": caveats, "measures": measures, "cohorts": cohorts, "cells": cells}


def difference(pack, mine):
    """In words, the first member in which a pack differs from the one
    that this tool builds."""
    for name in ("title", "source", "builder", "caveats", "cohorts"):
        if pack[name] != mine[name]:
            return "the pack's %s is not what the files and the builder give: it has %s, and this tool builds %s" % (
                name, reproduce.safe(json.dumps(pack[name], sort_keys=True))[:300], json.dumps(mine[name], sort_keys=True)[:300])
    for mid in sorted(set(pack["measures"]) | set(mine["measures"])):
        got, want = pack["measures"].get(mid), mine["measures"].get(mid)
        if got != want:
            return "measure %s is not what the files and the builder give: it has %s, and this tool builds %s" % (
                reproduce.safe(mid), reproduce.safe(json.dumps(got, sort_keys=True))[:300], json.dumps(want, sort_keys=True)[:300])
    theirs = {(c["measure"], c["cohort"]): c for c in pack["cells"]}
    for c in mine["cells"]:
        where = "cell %s %s" % (c["measure"], c["cohort"])
        got = theirs.pop((c["measure"], c["cohort"]), None)
        if got is None:
            return "%s is not in the pack, and the files give %d people" % (where, c["n"])
        if got["n"] != c["n"]:
            return "%s has n %s, and the files give %d" % (where, got["n"], c["n"])
        if len(got["values"]) != len(c["values"]):
            return "%s has %d values, and the files give %d" % (where, len(got["values"]), len(c["values"]))
        for i, (x, w) in enumerate(zip(c["values"], c["weights"])):
            if bits(got["values"][i]) != bits(x):
                return "%s value %d is %s, and the files give %s" % (where, i, packcheck.num(got["values"][i]), packcheck.num(x))
            if bits(got["weights"][i]) != bits(w):
                return "%s weight %d of the value %s is %s, and the files give %s" % (where, i, packcheck.num(x), packcheck.num(got["weights"][i]), packcheck.num(w))
    for (mid, cid) in sorted(theirs):
        return "cell %s %s is in the pack, and the files give fewer than %d people for it" % (reproduce.safe(mid), reproduce.safe(cid), MIN_N)
    return "the pack holds what this tool builds, and is not written as the builder writes it: its bytes are not in canonical form"


def rebuild(path, paths):
    with open(path, "rb") as f:
        raw = f.read()
    pack = packcheck.parse_pack(raw)
    given = load(pack, paths)
    sizes, digests = {}, {}
    for d, p in given.items():
        with open(p, "rb") as f:
            data = f.read()
        sizes[d], digests[d] = len(data), hashlib.sha256(data).hexdigest()
    mine = build(given, sizes, digests)
    built = (canonical(mine) + "\n").encode("utf-8")
    if built != raw:
        raise Stop("DIFFERS: " + difference(pack, mine))
    return len(mine["cells"]), hashlib.sha256(raw).hexdigest()


def main(argv):
    try:
        sys.stdout.reconfigure(encoding="utf-8", errors="backslashreplace")
    except Exception:
        pass
    if len(argv) < 3:
        sys.stderr.write(__doc__)
        return 2
    try:
        n, digest = rebuild(argv[1], argv[2:])
    except packcheck.Refused as err:
        print("REFUSED: not a pack: %s" % err)
        return 1
    except Stop as err:
        print(err)
        return 1
    except (OSError, ValueError, KeyError) as err:
        print("REFUSED: %s" % reproduce.safe(err))
        return 1
    print("rebuilt %s: the pack is, byte for byte, the one that these files give (%d cells, SHA-256 %s)" % (reproduce.safe(argv[1]), n, digest))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
