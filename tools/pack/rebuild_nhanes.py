#!/usr/bin/env python3
"""rebuild_nhanes.py: build an NHANES pack's tables again from the NHANES
files, and compare them with the pack, exactly.

    rebuild_nhanes.py <pack> <xpt file>...

It checks that the pack is a good pack (tools/pack/check.py), that each file
given has the SHA-256 the pack names for it (a file is matched by its data
set name, such as DEMO_J, whatever the case of its name on disk), and that
every file the pack names is given. Then, for each measure and each cohort of
the pack, it takes steps 1 to 5 of "The method, version 1" in docs/COMPARE.md
on the files:

  - a measure's id is the NHANES variable; the file that has it is the one
    that is given and holds that variable;
  - a cohort's id is `<sex>-<from>-<to>` or `<sex>-80-over` (male is
    RIAGENDR 1, female 2; age is RIDAGEYR from..to, both ends included;
    `80-over` is 80..80);
  - the weight is WTMEC2YR of DEMO_J.

A pair whose rebuilt cohort has fewer than 30 people must not be a cell of
the pack, and a pair with 30 or more must be one, with the same n, the same
values and the same weights, bit for bit.

It prints `rebuilt <N> cells of <pack>: every table is the one the files
give` and exits 0, or one line `DIFFERS: ...` (or `REFUSED: ...` for a pack
or a file that cannot be used) and exits 1.
"""
import hashlib
import math
import os
import re
import struct
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
sys.path.insert(0, os.path.join(HERE, "..", "compare"))
import check as packcheck  # noqa: E402
import reproduce  # noqa: E402

MIN_N = 30
DEMO, SEX, AGE, WEIGHT = "DEMO_J", "RIAGENDR", "RIDAGEYR", "WTMEC2YR"
COHORT_RE = re.compile(r"(male|female)-([0-9]{1,3})-(?:([0-9]{1,3})|over)")


class Stop(Exception):
    pass


def bits(x):
    return struct.pack("<d", float(x))


def dataset(name):
    """The data set name of a file: DEMO_J of demo_j.XPT."""
    return os.path.splitext(os.path.basename(name))[0].upper()


def parse_cohort(cid, text):
    """Sex code, youngest and oldest age of a cohort id, and the cohort's
    words, or None when the id is not one this tool knows."""
    m = COHORT_RE.fullmatch(cid)
    if not m:
        return None
    sex = 1 if m.group(1) == "male" else 2
    lo = int(m.group(2))
    if m.group(3) is None:
        if lo != 80:
            return None
        hi, words = 80, "%s, age 80 and over" % m.group(1)
    else:
        hi = int(m.group(3))
        words = "%s, age %d-%d" % (m.group(1), lo, hi)
    if lo > hi:
        return None
    return sex, lo, hi, words


def load(pack, paths):
    entries = {dataset(f["name"]): f for f in pack["source"]["files"]}
    given = {}
    for path in paths:
        d = dataset(path)
        if d in given:
            raise Stop("REFUSED: the data set %s is given twice" % d)
        if d not in entries:
            raise Stop("REFUSED: %s is not a file the pack names" % reproduce.safe(path))
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


def rebuild(pack, paths):
    given = load(pack, paths)
    tables = {}

    def table(d):
        if d not in tables:
            tables[d] = reproduce.read_xpt(given[d])
        return tables[d]

    demo = demographics(table(DEMO))
    checked = 0
    for mid, measure in pack["measures"].items():
        homes = [d for d in sorted(given) if d != DEMO and mid in table(d).columns]
        if len(homes) != 1:
            raise Stop("DIFFERS: measure %s: %s" % (reproduce.safe(mid), "no file given has this variable" if not homes else "more than one file has this variable: " + ", ".join(homes)))
        label = table(homes[0]).attrs["labels"].get(mid)
        if label != measure["label"]:
            raise Stop("DIFFERS: measure %s is labelled %r in %s, and %r in the pack" % (reproduce.safe(mid), label, homes[0], reproduce.safe(measure["label"])))
        rows = rows_of(table(homes[0]), mid, demo)
        for cid, cohort in pack["cohorts"].items():
            known = parse_cohort(cid, cohort["text"])
            if known is None:
                raise Stop("DIFFERS: cohort %s is not a cohort id this tool knows" % reproduce.safe(cid))
            sex, lo, hi, words = known
            if cohort["text"] != words:
                raise Stop("DIFFERS: cohort %s has the text %r, not %r" % (reproduce.safe(cid), reproduce.safe(cohort["text"]), words))
            n, values, weights = cell_of(rows, sex, lo, hi)
            cell = pack.cells.get((mid, cid))
            where = "cell %s %s" % (reproduce.safe(mid), reproduce.safe(cid))
            if n < MIN_N:
                if cell is not None:
                    raise Stop("DIFFERS: %s is in the pack, and the files give %d people" % (where, n))
                continue
            if cell is None:
                raise Stop("DIFFERS: %s is not in the pack, and the files give %d people" % (where, n))
            checked += 1
            if cell["n"] != n:
                raise Stop("DIFFERS: %s has n %s, and the files give %d" % (where, cell["n"], n))
            if len(cell["values"]) != len(values):
                raise Stop("DIFFERS: %s has %d values, and the files give %d" % (where, len(cell["values"]), len(values)))
            for i, (x, w) in enumerate(zip(values, weights)):
                if bits(cell["values"][i]) != bits(x):
                    raise Stop("DIFFERS: %s value %d is %s, and the files give %s" % (where, i, packcheck.num(cell["values"][i]), packcheck.num(x)))
                if bits(cell["weights"][i]) != bits(w):
                    raise Stop("DIFFERS: %s weight %d of the value %s is %s, and the files give %s" % (where, i, packcheck.num(x), packcheck.num(cell["weights"][i]), packcheck.num(w)))
    if checked != len(pack["cells"]):
        raise Stop("DIFFERS: the pack has %d cells, and %d pairs of its measures and cohorts are rebuilt" % (len(pack["cells"]), checked))
    return checked


def main(argv):
    try:
        sys.stdout.reconfigure(encoding="utf-8", errors="backslashreplace")
    except Exception:
        pass
    if len(argv) < 3:
        sys.stderr.write(__doc__)
        return 2
    try:
        pack = packcheck.read_pack(argv[1])
        n = rebuild(pack, argv[2:])
    except packcheck.Refused as err:
        print("REFUSED: not a pack: %s" % err)
        return 1
    except Stop as err:
        print(err)
        return 1
    except (OSError, ValueError, KeyError) as err:
        print("REFUSED: %s" % reproduce.safe(err))
        return 1
    print("rebuilt %d cells of %s: every table is the one the files give" % (n, reproduce.safe(argv[1])))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
