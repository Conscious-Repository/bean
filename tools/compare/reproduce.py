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
 3. the claim is one the method could have made: the cohort starts with
    sex and age, its text says what its rules say, the weight is the one
    the method gives, and the status and reasons follow from the inputs;
    and each variable has, in its file, the label the claim gives it;
 4. the method of docs/COMPARE.md, computed here from the claim's own
    cohort and the reading's value, gives every number of the result;
 5. numpy's own weighted quantile and sums agree with that.

It prints one line per comparison, with the reading and the variable it
was set against, and exits 1 if any check fails. Whether the two measure
the same thing is for you to judge: the line shows both.

It shares no code with bean and has no table of tests: which variable a
reading is set against is read from the claim and shown to you. It knows
the method: the weights, the data sets a rule may name, the reasons, and
the SHA-256 of each file as CDC served it, so that it does not take the
claim's word for which file is CDC's. The record checks are those of the second
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
SMALL_N = 100
MAX_AGE = 80
DEMO, EXAM_WEIGHT = "DEMO_J", "WTMEC2YR"
# The data sets a rule may name, with the weight an analysis with each must
# use when it is not the examination weight (docs/COMPARE.md, step 4).
COHORT_FILES = {"DEMO_J": None, "BMX_J": None, "DR1TOT_J": "WTDRD1"}
# The SHA-256 of each file as wwwn.cdc.gov served it on 2026-11-09: the same
# table as bean's (internal/nhanes), kept here so that this tool does not
# take the claim's word for which file is CDC's.
KNOWN = {
    "DEMO_J": "c0b46e0345ea19404928656277c8b0d10b0cca348a9b2fe4fc3c67e8b7ee73ec",
    "TCHOL_J": "0291a6a4f6d82dac8392c8c992b519946824dfeda01647935520cf506ed9f4ab",
    "HDL_J": "9f4eb41b89f0c9f3d6262f873e671eeb2f2eb40581a8426b2a3c8fd4fc19439e",
    "GHB_J": "35f07094573a0061a03ed609a5a363b34eb1b1c7065d1623b43d72e132a8a654",
    "CBC_J": "00f964098dc91272e415344554cbf1b627ecaeb2eda201518527446a7d81e742",
    "BMX_J": "8d675e42d8826ac98714b2c3dd4c5138a5e353fb4424f7eff5e6db4a01ce838a",
    "DR1TOT_J": "390ae5205acfbcb87e0ca3d9aabd1c801b60657d3f2802ac08d5b3ed93f0af6d",
}
# The reasons of a qualified comparison, in the order a claim lists them.
REASON_METHOD = "the lab that measured the reading and NHANES used their own instruments and methods"
REASON_POPULATION = "NHANES is a sample of people living in the United States in the survey years, not a reference range, and like you only in what the cohort states"
REASON_COHORT = "you chose the cohort: bean does not know whether you belong to it"
REASON_TOP_AGE = "NHANES records every age of 80 and over as 80"
REASON_DIET = "the cohort is chosen by what people said they ate on one day, which is not what a person usually eats"
REASON_SMALL = "the cohort is small: fewer than 100 people"
REASON_UNKNOWN = "this copy of %s is not the file on record: its SHA-256 differs"
REASON_AUTHOR = "the reading was signed by an agent or another key, not by you"
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


def num(x):
    """A number the short way: 20, not 20.0."""
    return str(int(x)) if float(x).is_integer() else repr(float(x))


def is_number(x):
    return isinstance(x, (int, float)) and not isinstance(x, bool) and math.isfinite(x)


def check_form(claim, reading):
    """The claim must be one that the method could have produced: the
    cohort starts with sex and age, the rules name data sets the method
    allows, the weight is the one step 4 gives, the text says what the rules
    say, and the status and reasons follow from the inputs. Returns the
    reasons it must carry but for those that need the result, or raises
    ValueError."""
    ctx = claim["context"]
    ref, cohort = ctx["reference"], ctx["cohort"]
    rules = cohort["rules"]
    if claim.get("author") != claim.get("subject"):
        raise ValueError("the claim is not signed by the person it is about")
    if ref.get("system") != "nhanes" or ref.get("cycle") != "2017-2018" or ref.get("demographics") != DEMO:
        raise ValueError("not NHANES 2017-2018 with DEMO_J")
    if not str(ref.get("dataset", "")).endswith("_J") or ref["dataset"] in COHORT_FILES:
        raise ValueError("the test's data set is not a test file of the cycle")
    if len(rules) < 2:
        raise ValueError("the cohort has no sex and age")
    seen = set()
    for r in rules:
        name = (r.get("dataset"), r.get("variable"))
        if name[0] not in COHORT_FILES or not isinstance(name[1], str):
            raise ValueError("a rule names a data set the method does not allow: %s" % (name[0],))
        if name[1] == "SEQN" or name[1].startswith("WT") or name[1].startswith("SDMV") or name in seen:
            raise ValueError("a rule on a design variable, or two rules on one variable: %s" % name[1])
        seen.add(name)
        if not is_number(r.get("min")) or not is_number(r.get("max")) or r["min"] > r["max"] or r["min"] < 0:
            raise ValueError("a rule without a range of two numbers: %s" % name[1])
    sex, age = rules[0], rules[1]
    if (sex["dataset"], sex["variable"]) != (DEMO, "RIAGENDR") or sex["min"] != sex["max"] or sex["min"] not in (1, 2):
        raise ValueError("the first rule is not one sex")
    if (age["dataset"], age["variable"]) != (DEMO, "RIDAGEYR") or age["max"] > MAX_AGE or not float(age["min"]).is_integer() or not float(age["max"]).is_integer():
        raise ValueError("the second rule is not an age range in years up to 80")
    # Step 4: the weight.
    weight = (DEMO, EXAM_WEIGHT)
    for r in rules:
        if COHORT_FILES[r["dataset"]]:
            weight = (r["dataset"], COHORT_FILES[r["dataset"]])
            break
    if (ref["weight"].get("dataset"), ref["weight"].get("variable")) != weight:
        raise ValueError("the weight is not the one the method gives: %s.%s" % weight)
    # The cohort in words, from the rules.
    text = "%s, age %s-%s" % ("male" if sex["min"] == 1 else "female", num(age["min"]), num(age["max"]))
    if age["max"] == MAX_AGE:
        text = "%s, age %s and over" % ("male" if sex["min"] == 1 else "female", num(age["min"]))
    for r in rules[2:]:
        span = num(r["min"]) if r["min"] == r["max"] else "%s-%s" % (num(r["min"]), num(r["max"]))
        text += ", %s.%s=%s" % (r["dataset"], r["variable"], span)
    if cohort.get("text") != text:
        raise ValueError("the cohort's text is not what its rules say: %r" % text)
    # The files: the test's, the demographics, then each rule's, once each.
    needed = []
    for d in [ref["dataset"], DEMO] + [r["dataset"] for r in rules]:
        if d not in needed:
            needed.append(d)
    if [f.get("dataset") for f in ref["files"]] != needed:
        raise ValueError("the claim does not name exactly the files the method reads")
    unknown = []
    for f in ref["files"]:
        on_record = KNOWN.get(f["dataset"]) == f.get("sha256")
        if f.get("known") is not on_record:
            raise ValueError("the claim is wrong about whether %s is the file on record" % f["dataset"])
        if not on_record:
            unknown.append(REASON_UNKNOWN % f["dataset"])
    if ctx.get("status") != "qualified":
        raise ValueError("the status is not qualified")
    reasons = [REASON_METHOD, REASON_POPULATION, REASON_COHORT]
    if age["max"] == MAX_AGE:
        reasons.append(REASON_TOP_AGE)
    if any(r["dataset"] == "DR1TOT_J" for r in rules):
        reasons.append(REASON_DIET)
    return reasons, unknown, ([REASON_AUTHOR] if reading.get("author") != claim["author"] else [])


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
    if not is_number(value) or not value > 0:
        return False, "the reading is not a number above zero"
    try:
        reasons, unknown, by = check_form(claim, reading)
    except (KeyError, TypeError, ValueError, AttributeError) as err:
        return False, "not a claim the method makes: %s" % err
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
    if got["n"] < SMALL_N:
        reasons.append(REASON_SMALL)
    if ctx.get("reasons") != reasons + unknown + by:
        return False, "%s: the reasons are not the ones these inputs give" % what
    note = "; NOT the files on record: " + ", ".join(f["dataset"] for f in ref["files"] if not f["known"]) if unknown else ""
    return True, "%s: percentile %s, n %s%s" % (what, got["percentile"], got["n"], note)


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
