#!/usr/bin/env python3
"""reproduce.py: check a saved bean comparison and compute it again,
without bean.

    reproduce.py <path>...

A path is a folder, a .jsonl file of records, or a data file. A folder is
read whole: every .json file in it is a record, every other file a data
file. So a `bean export` folder is enough by itself, and so is the output of
`bean ls -json` next to the NHANES files you downloaded from CDC.

For each comparison.percentile record it checks, in this order:

 1. the record is what its id says and its signature is good, and so is
    the reading it is about (the id is a hash of the record, so the claim
    pins the reading's value without holding it);
 2. each data file has the SHA-256 the claim names;
 3. each variable the claim names has, in its file, the label the claim
    gives it;
 4. the method of docs/COMPARE.md, computed here from the claim's own
    cohort and the reading's value, gives every number of the result;
 5. numpy's own weighted quantile and sums agree with that.

It prints one line per comparison, with the reading and the variable it
was set against, and exits 1 if any check fails. Whether the two measure
the same thing is for you to judge: the line shows both.

It shares no code with bean and has no table of tests or variables: it
reads only what the claim says. The record checks are those of the second
verifier (tools/second/beanverify.py), which must sit where this file
expects it. A good signature shows which key signed. Whether that key
speaks for the person is a question for the whole log: bean verify, or
beanverify.py, answers it.
"""
import hashlib
import json
import math
import os
import sys

import numpy as np
import pandas as pd

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "second"))
import beanverify  # noqa: E402

METHOD = ("nhanes-weighted-percentile", "1")
MIN_N = 30
QUANTILES = (("p5", 0.05), ("p25", 0.25), ("p50", 0.5), ("p75", 0.75), ("p95", 0.95))


def read_xpt(path):
    """A transport file as a table, indexed by SEQN, with each variable's
    label in table.attrs."""
    with pd.read_sas(path, format="xport", iterator=True) as reader:
        labels = {f["name"].decode("latin-1"): f["label"].decode("latin-1") for f in reader.fields}
        table = reader.read()
    table.attrs["labels"] = labels
    # pandas reads a transport-file zero as 5.4e-79, the smallest IBM
    # number. No NHANES value is that small: it is zero.
    numbers = table.select_dtypes("number")
    table[numbers.columns] = numbers.mask(numbers.abs() < 1e-70, 0.0)
    if table["SEQN"].isna().any() or table["SEQN"].duplicated().any():
        raise ValueError(path + " has a missing or repeated SEQN")
    return table.set_index("SEQN", drop=False)


def round1(y):
    return math.floor(10 * y + 0.5) / 10


def cohort_rows(claim, tables):
    """Steps 1 to 4: the value and the weight of each row of the cohort."""
    ref, cohort = claim["context"]["reference"], claim["context"]["cohort"]
    lab, demo = tables[ref["dataset"]], tables[ref["demographics"]]
    weights = tables[ref["weight"]["dataset"]][ref["weight"]["variable"]]
    rules = [(tables[r["dataset"]][r["variable"]], r["min"], r["max"]) for r in cohort["rules"]]
    out = []
    for seqn, x in zip(lab["SEQN"], lab[ref["variable"]]):  # file order
        if math.isnan(x):
            continue
        if seqn not in demo.index:
            raise ValueError("the files are not of one release")
        if not all(lo <= column.get(seqn, math.nan) <= hi for column, lo, hi in rules):
            continue
        w = weights.get(seqn, math.nan)
        if not (w > 0) or math.isinf(w):
            continue
        out.append((x, w))
    return out


def compute(rows, value):
    """Steps 5 to 9, as written."""
    n, total, below, equal = len(rows), 0.0, 0.0, 0.0
    by_value = {}
    for x, w in rows:
        total += w
        if x < value:
            below += w
        elif x == value:
            equal += w
        by_value[x] = by_value.get(x, 0.0) + w
    if n < MIN_N:
        return {"n": n}
    out = {
        "n": n,
        "percentile": round1(100 * (below + equal / 2) / total),
        "below": round1(100 * below / total),
        "equal": round1(100 * equal / total),
    }
    for name, q in QUANTILES:
        running = 0.0
        for x in sorted(by_value):
            running += by_value[x]
            if running >= q * total:
                break
        out[name] = x
    return out


def numpy_differs(rows, value, got):
    """The same numbers from numpy's own sums and weighted quantile. Its
    sums add in another order, so a share may differ in its last digit."""
    x = np.array([r[0] for r in rows])
    w = np.array([r[1] for r in rows])
    total = w.sum()
    shares = {
        "below": 100 * w[x < value].sum() / total,
        "equal": 100 * w[x == value].sum() / total,
    }
    shares["percentile"] = shares["below"] + shares["equal"] / 2
    for name, share in shares.items():
        if abs(share - got[name]) > 0.05 + 1e-6:
            return "%s is %s, numpy gives %.4f" % (name, got[name], share)
    for name, q in QUANTILES:
        theirs = float(np.quantile(x, q, weights=w, method="inverted_cdf"))
        if theirs != got[name]:
            return "%s is %s, numpy gives %s" % (name, got[name], theirs)
    return None


class Inputs:
    """The records and data files found under the paths given."""

    def __init__(self):
        self.records = {}   # id -> parsed record, checked
        self.bad = {}       # id or name -> why a record was not accepted
        self.files = {}     # SHA-256 -> path
        self.tables = {}    # SHA-256 -> table

    def add_record(self, name, data):
        try:
            claimed = json.loads(data).get("id", "")
        except (ValueError, AttributeError):
            self.bad[name] = "not a record"
            return
        if not beanverify.ID_RE.fullmatch(str(claimed)):
            self.bad[name] = "not a record"
            return
        try:
            # The id is the hash of the record, the signature is over the
            # id, and the bytes are canonical.
            record, unknown = beanverify.check_file("objects/%s/%s.json" % (claimed[7:9], claimed[9:]), data)
        except beanverify.Fail as err:
            self.bad[claimed] = str(err)
            return
        if unknown:
            self.bad[claimed] = unknown
            return
        self.records[claimed] = json.loads(data)

    def add(self, path):
        if os.path.isdir(path):
            for base, _, names in sorted(os.walk(path)):
                for n in sorted(names):
                    self.add(os.path.join(base, n))
        elif path.endswith(".jsonl"):
            with open(path, "rb") as f:
                for i, line in enumerate(f):
                    if line.strip():
                        self.add_record("%s line %d" % (path, i + 1), line.rstrip(b"\r\n"))
        elif path.endswith(".json"):
            with open(path, "rb") as f:
                self.add_record(path, f.read())
        else:
            h = hashlib.sha256()
            with open(path, "rb") as f:
                for chunk in iter(lambda: f.read(1 << 20), b""):
                    h.update(chunk)
            self.files[h.hexdigest()] = path

    def table(self, digest):
        if digest not in self.tables:
            self.tables[digest] = read_xpt(self.files[digest])
        return self.tables[digest]


def check(claim, inputs):
    """Returns (ok, text) for one comparison."""
    ctx = claim["context"]
    method = ctx["method"]
    if (method["id"], method["version"]) != METHOD or method["min_n"] != MIN_N:
        return False, "a method this tool does not know: %s@%s" % (method["id"], method["version"])
    rid = ctx["reading"]
    reading = inputs.records.get(rid)
    if reading is None:
        why = inputs.bad.get(rid, "it is not among the records given")
        return False, "the reading %s cannot be used: %s" % (str(rid)[7:19], why)
    if {"rel": "about", "id": rid} not in claim.get("refs", []) or reading.get("subject") != claim.get("subject"):
        return False, "the claim is not about the reading it names, or they are about two people"
    value = reading.get("value", {}).get("n")
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        return False, "the reading is not a number"
    tables = {}
    for f in ctx["reference"]["files"]:
        if f["sha256"] not in inputs.files:
            return False, "no file given with the SHA-256 of %s, %s" % (f["dataset"], f["sha256"])
        tables[f["dataset"]] = inputs.table(f["sha256"])
    # The claim says in words what each variable is: the files must agree.
    ref = ctx["reference"]
    named = [(ref["dataset"], ref["variable"], ref["label"])] + [(r["dataset"], r["variable"], r["label"]) for r in ctx["cohort"]["rules"]]
    for dataset, variable, label in named:
        if tables[dataset].attrs["labels"].get(variable) != label:
            return False, "%s.%s is not labelled %r in the file" % (dataset, variable, label)
    try:
        rows = cohort_rows(claim, tables)
        got = compute(rows, float(value))
    except (KeyError, ValueError, TypeError) as err:
        return False, "cannot compute: %s" % err
    want = ctx["result"]
    # Whether the reading and the variable measure the same thing is for the
    # reader to judge: show both.
    what = "%s %s %s against %s %r, %s" % (reading["type"], value, reading["value"].get("unit", ""), ref["variable"], ref["label"], ctx["cohort"]["text"])
    if got != want or claim["value"] != {"n": want.get("percentile"), "unit": "{percentile}"}:
        return False, "%s: the claim says %s, this tool computes %s" % (what, json.dumps(want, sort_keys=True), json.dumps(got, sort_keys=True))
    differs = numpy_differs(rows, float(value), got)
    if differs:
        return False, "%s: %s" % (what, differs)
    return True, "%s: percentile %s, n %s" % (what, got["percentile"], got["n"])


def main(argv):
    if len(argv) < 2:
        sys.stderr.write(__doc__)
        return 2
    inputs = Inputs()
    for path in argv[1:]:
        try:
            inputs.add(path)
        except OSError as err:
            print("cannot read %s: %s" % (path, err))
            return 1
    claims = [r for r in inputs.records.values() if r["type"] == "comparison.percentile"]
    bad_claims = 0
    for rid, why in sorted(inputs.bad.items()):
        if why != "not a record":
            print("REFUSED %s %s" % (str(rid)[7:19] if str(rid).startswith("sha256:") else rid, why))
            bad_claims += 1
    if not claims and not bad_claims:
        print("no comparison among the records given")
        return 1
    bad = bad_claims
    signers = {}
    for claim in sorted(claims, key=lambda r: r["id"]):
        try:
            ok, text = check(claim, inputs)
        except (KeyError, TypeError, ValueError, OSError) as err:
            ok, text = False, "cannot check: %s" % err
        bad += not ok
        if ok:
            signers[claim["kid"]] = signers.get(claim["kid"], 0) + 1
        print(("reproduced " if ok else "DIFFERS ") + claim["id"][7:19] + " " + text)
    for kid, n in sorted(signers.items()):
        print("signed by %s: %d" % (kid, n))
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
