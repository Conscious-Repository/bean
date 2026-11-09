#!/usr/bin/env python3
"""reproduce.py: compute a saved bean comparison again, without bean.

    reproduce.py <records.jsonl> <file.XPT>...

records.jsonl holds comparison.percentile records and the readings they are
about, one stored record per line (bean ls -json writes them). The XPT files
are the NHANES files the comparisons name; get them from CDC yourself.

For each comparison it checks that each file has the SHA-256 the claim
names, reads the files with pandas, computes the method of docs/COMPARE.md
from the claim's own cohort and the reading's value, and compares every
number of the result. It prints one line per comparison and exits 1 if any
differs or cannot be computed.

It shares no code with bean, and it reads only what the claim says: it has
no table of tests or variables. It does not check signatures; bean verify
and the second verifier (tools/second) do.
"""
import hashlib
import json
import math
import sys

import pandas as pd

METHOD = ("nhanes-weighted-percentile", "1")
MIN_N = 30


def read_xpt(path):
    """Returns the SHA-256 of the file and its table, indexed by SEQN."""
    with open(path, "rb") as f:
        digest = hashlib.sha256(f.read()).hexdigest()
    table = pd.read_sas(path, format="xport")
    # pandas reads a transport-file zero as 5.4e-79, the smallest IBM
    # number. No NHANES value is that small: it is zero.
    table = table.mask(table.abs() < 1e-70, 0.0)
    if table["SEQN"].isna().any() or table["SEQN"].duplicated().any():
        raise ValueError(path + " has a missing or repeated SEQN")
    return digest, table.set_index("SEQN", drop=False)


def round1(y):
    return math.floor(10 * y + 0.5) / 10


def compute(claim, value, tables):
    """The method, from the claim's context alone."""
    ref, cohort = claim["context"]["reference"], claim["context"]["cohort"]
    lab, demo = tables[ref["dataset"]], tables[ref["demographics"]]
    weights = tables[ref["weight"]["dataset"]][ref["weight"]["variable"]]
    rules = [(tables[r["dataset"]][r["variable"]], r["min"], r["max"]) for r in cohort["rules"]]
    n, total, below, equal = 0, 0.0, 0.0, 0.0
    by_value = {}
    for seqn, x in zip(lab["SEQN"], lab[ref["variable"]]):  # file order
        if math.isnan(x):
            continue
        if seqn not in demo.index:
            raise ValueError("the files are not of one release")
        keep = True
        for column, lo, hi in rules:
            y = column.get(seqn, math.nan)
            if not (lo <= y <= hi):
                keep = False
                break
        if not keep:
            continue
        w = weights.get(seqn, math.nan)
        if not (w > 0) or math.isinf(w):
            continue
        n += 1
        total += w
        if x < value:
            below += w
        elif x == value:
            equal += w
        by_value[x] = by_value.get(x, 0.0) + w
    if n < MIN_N:
        return {"n": n}

    def quantile(q):
        running = 0.0
        for x in sorted(by_value):
            running += by_value[x]
            if running >= q * total:
                return x
        return max(by_value)

    return {
        "n": n,
        "percentile": round1(100 * (below + equal / 2) / total),
        "below": round1(100 * below / total),
        "equal": round1(100 * equal / total),
        "p5": quantile(0.05), "p25": quantile(0.25), "p50": quantile(0.5),
        "p75": quantile(0.75), "p95": quantile(0.95),
    }


def check(claim, records, files):
    """Returns (ok, text) for one comparison."""
    ctx = claim["context"]
    method = ctx["method"]
    if (method["id"], method["version"]) != METHOD or method["min_n"] != MIN_N:
        return False, "a method this tool does not know: %s@%s" % (method["id"], method["version"])
    reading = records.get(ctx["reading"])
    if reading is None:
        return False, "the reading %s is not in the records" % ctx["reading"][7:19]
    if {"rel": "about", "id": reading["id"]} not in claim.get("refs", []):
        return False, "the claim is not about the reading it names"
    value = reading["value"].get("n")
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        return False, "the reading is not a number"
    tables = {}
    for f in ctx["reference"]["files"]:
        if f["sha256"] not in files:
            return False, "no file given with the SHA-256 of %s, %s" % (f["dataset"], f["sha256"])
        tables[f["dataset"]] = files[f["sha256"]]
    try:
        got = compute(claim, float(value), tables)
    except (KeyError, ValueError) as err:
        return False, "cannot compute: %s" % err
    want = ctx["result"]
    what = "%s %s %s, %s" % (reading["type"], value, reading["value"].get("unit", ""), ctx["cohort"]["text"])
    if got != want or claim["value"] != {"n": want["percentile"], "unit": "{percentile}"}:
        return False, "%s: the claim says %s, this tool computes %s" % (what, json.dumps(want, sort_keys=True), json.dumps(got, sort_keys=True))
    return True, "%s: percentile %s, n %s" % (what, got["percentile"], got["n"])


def main(argv):
    if len(argv) < 3:
        sys.stderr.write(__doc__)
        return 2
    records = {}
    with open(argv[1], encoding="utf-8") as f:
        for line in f:
            if line.strip():
                r = json.loads(line)
                records[r["id"]] = r
    files = {}
    for path in argv[2:]:
        try:
            digest, table = read_xpt(path)
        except (OSError, ValueError, KeyError) as err:
            print("cannot read %s: %s" % (path, err))
            return 1
        files[digest] = table
    claims = [r for r in records.values() if r["type"] == "comparison.percentile"]
    if not claims:
        print("no comparison in " + argv[1])
        return 1
    bad = 0
    for claim in sorted(claims, key=lambda r: r["id"]):
        ok, text = check(claim, records, files)
        bad += not ok
        print(("reproduced " if ok else "DIFFERS ") + claim["id"][7:19] + " " + text)
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
