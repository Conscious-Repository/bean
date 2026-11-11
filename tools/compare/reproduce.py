#!/usr/bin/env python3
"""reproduce.py: check a saved bean comparison and compute it again,
without bean.

    reproduce.py <path>...

A path is a folder, a .jsonl file of records, a reference pack (a file whose
name ends in .pack), or a data file. A folder is read whole: every .json file
in it is a record, every .pack file at its top level a pack, every other file
a data file. So a `bean export` folder or a share bundle with its files is
enough by itself, and so is the output of `bean ls -json` next to the NHANES
files you downloaded from CDC, or next to a pack.

For each comparison.percentile record it checks, in this order:

 1. the record is what its id says and its signature is good, and so is
    the reading it is about (the id is a hash of the record, so the claim
    pins the reading's value without holding it);
 2. each data file has the SHA-256 the claim names;
 3. the claim is one the method could have made: the cohort starts with
    sex and age, its text says what its rules say, the weight is the one
    the method gives, and the caveats follow from the inputs; and each
    variable has, in its file, the label the claim gives it;
 4. the method of docs/COMPARE.md, computed here from the claim's own
    cohort and the reading's value, gives every number of the result.

A claim whose method is `weighted-percentile` version 1 is checked against a
pack (docs/REFERENCE.md, "The claim"), found by the SHA-256 that the claim
names: the pack has that SHA-256 and is a good pack (tools/pack/check.py);
the cell is there; the reading's type and unit are in the measure's `same`
list; the texts and the caveats are the pack's; and the method gives every
number of the result. A run given only packs and records needs no pandas.

It prints one line per comparison, with the reading and the variable it
was set against, and exits 1 if any check fails or cannot be made.

It shares no code with bean. It knows the method: which variable each kind
of reading may be set against and in which unit, the weights, the data sets
a rule may name, the caveats, and the SHA-256 of each file as CDC served
it, so that it can say for itself which files are the ones on record. A
test keeps those tables equal to bean's. The record checks are those of the
second verifier (tools/second/beanverify.py), which must sit where this
file expects it. A good signature shows which key signed. Whether that key
speaks for the person is a question for the whole log or bundle: bean
verify, bean check-bundle or their Python twins answer it.
"""
import hashlib
import importlib.util
import json
import math
import os
import sys
from decimal import Decimal

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(HERE, "..", "second"))
import beanverify  # noqa: E402


def _load_packcheck():
    """tools/pack/check.py, by its path: the name `check` is too common to
    take from sys.path."""
    spec = importlib.util.spec_from_file_location("bean_pack_check", os.path.join(HERE, "..", "pack", "check.py"))
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


packcheck = _load_packcheck()

METHOD = ("nhanes-weighted-percentile", "1")
PACK_METHOD = ("weighted-percentile", "1")
MIN_N = 30
SMALL_N = 100
MAX_AGE = 80
DEMO, EXAM_WEIGHT = "DEMO_J", "WTMEC2YR"
QUARTILES = (("p25", 0.25), ("p50", 0.5), ("p75", 0.75))
# The data sets a rule may name, with the weight an analysis with each must
# use when it is not the examination weight (docs/COMPARE.md, step 4).
COHORT_FILES = {"DEMO_J": None, "BMX_J": None, "DR1TOT_J": "WTDRD1"}
# The SHA-256 of each file as wwwn.cdc.gov served it on 2026-11-09: the same
# table as bean's (internal/nhanes), kept here so that this tool can say for
# itself which files are the ones on record.
KNOWN = {
    "DEMO_J": "c0b46e0345ea19404928656277c8b0d10b0cca348a9b2fe4fc3c67e8b7ee73ec",
    "TCHOL_J": "0291a6a4f6d82dac8392c8c992b519946824dfeda01647935520cf506ed9f4ab",
    "HDL_J": "9f4eb41b89f0c9f3d6262f873e671eeb2f2eb40581a8426b2a3c8fd4fc19439e",
    "GHB_J": "35f07094573a0061a03ed609a5a363b34eb1b1c7065d1623b43d72e132a8a654",
    "CBC_J": "00f964098dc91272e415344554cbf1b627ecaeb2eda201518527446a7d81e742",
    "BMX_J": "8d675e42d8826ac98714b2c3dd4c5138a5e353fb4424f7eff5e6db4a01ce838a",
    "DR1TOT_J": "390ae5205acfbcb87e0ca3d9aabd1c801b60657d3f2802ac08d5b3ed93f0af6d",
}
# The tests the method compares: a reading of this type, in this unit, with
# this variable of this data set (docs/COMPARE.md; internal/nhanes).
ANALYTES = {
    ("lab.cholesterol-total", "mg/dL"): ("TCHOL_J", "LBXTC"),
    ("lab.hdl-cholesterol", "mg/dL"): ("HDL_J", "LBDHDD"),
    ("lab.hemoglobin-a1c", "%"): ("GHB_J", "LBXGH"),
    ("lab.hemoglobin-a1c", "% of total Hgb"): ("GHB_J", "LBXGH"),
    ("lab.wbc", "x10E3/uL"): ("CBC_J", "LBXWBCSI"),
    ("lab.white-blood-cell-count", "Thousand/uL"): ("CBC_J", "LBXWBCSI"),
    ("lab.rbc", "x10E6/uL"): ("CBC_J", "LBXRBCSI"),
    ("lab.red-blood-cell-count", "Million/uL"): ("CBC_J", "LBXRBCSI"),
    ("lab.hemoglobin", "g/dL"): ("CBC_J", "LBXHGB"),
    ("lab.hematocrit", "%"): ("CBC_J", "LBXHCT"),
    ("lab.mcv", "fL"): ("CBC_J", "LBXMCVSI"),
    ("lab.mch", "pg"): ("CBC_J", "LBXMCHSI"),
    ("lab.mchc", "g/dL"): ("CBC_J", "LBXMC"),
    ("lab.rdw", "%"): ("CBC_J", "LBXRDW"),
    ("lab.platelets", "x10E3/uL"): ("CBC_J", "LBXPLTSI"),
    ("lab.platelet-count", "Thousand/uL"): ("CBC_J", "LBXPLTSI"),
}
# The caveats of a comparison, in the order a claim lists them.
CAVEAT_METHOD = "the lab that measured the reading and NHANES used their own instruments and methods"
CAVEAT_POPULATION = "NHANES is a sample of people living in the United States in the survey years, not a reference range"
CAVEAT_COHORT = "you chose the cohort: bean does not know whether you belong to it"
CAVEAT_TOP_AGE = "NHANES records every age of 80 and over as 80"
CAVEAT_DIET = "the cohort is chosen by what people said they ate on one day, which is not what a person usually eats"
CAVEAT_SMALL = "the cohort is small: fewer than 100 people"
CAVEAT_UNKNOWN = "this copy of %s is not the file on record: its SHA-256 differs"
CAVEAT_AUTHOR = "the reading was signed by an agent or another key, not by you"
# The members of a pack claim's context (docs/REFERENCE.md, "The claim").
PACK_CONTEXT = {"reading", "reference", "cohort", "method", "result", "caveats"}
PACK_REFERENCE = {"pack", "title", "source", "measure", "label", "unit", "cohort"}
PACK_RESULT = {"n", "percentile", "below", "equal", "p25", "p50", "p75"}


class Missing(Exception):
    """An input the check needs is not among the paths given."""


def safe(s):
    """Text from a record, with nothing that can end a line or move the
    cursor: a unit or a note must not be able to fake a line of this tool."""
    return "".join(c if c.isprintable() else "\\x%02x" % ord(c) for c in str(s))


def read_xpt(path):
    """A transport file as a table, indexed by SEQN, with each variable's
    label in table.attrs."""
    import pandas as pd  # only a run that reads data files needs it
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
    """A number the short way, in digits and never with an exponent."""
    f = float(x)
    return "0" if f == 0 else format(Decimal(repr(f)).normalize(), "f")


def is_number(x):
    return isinstance(x, (int, float)) and not isinstance(x, bool) and math.isfinite(x)


def check_form(claim, reading):
    """The claim must be one that the method could have produced: the
    cohort starts with sex and age, the rules name data sets the method
    allows, the weight is the one step 4 gives, and the text says what the
    rules say. Returns the caveats it must carry but for the ones that need
    the result, or raises ValueError."""
    ctx = claim["context"]
    ref, cohort = ctx["reference"], ctx["cohort"]
    rules = cohort["rules"]
    if claim.get("author") != claim.get("subject"):
        raise ValueError("the claim is not signed by the person it is about")
    if ref.get("system") != "nhanes" or ref.get("cycle") != "2017-2018":
        raise ValueError("not NHANES 2017-2018")
    # The reading must be of a kind, and in a unit, that the method sets
    # against this variable: bean itself refuses any other pair.
    kind = (reading.get("type"), (reading.get("value") or {}).get("unit"))
    if ANALYTES.get(kind) != (ref.get("dataset"), ref.get("variable")):
        raise ValueError("the method does not set %s in %s against %s.%s" % (kind[0], kind[1], ref.get("dataset"), ref.get("variable")))
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
    caveats = [CAVEAT_METHOD, CAVEAT_POPULATION, CAVEAT_COHORT]
    if age["max"] == MAX_AGE:
        caveats.append(CAVEAT_TOP_AGE)
    if any(r["dataset"] == "DR1TOT_J" for r in rules):
        caveats.append(CAVEAT_DIET)
    return caveats, ([CAVEAT_AUTHOR] if reading.get("author") != claim["author"] else [])


def round1(y):
    return math.floor(10 * y + 0.5) / 10


def cohort_weights(claim, tables):
    """Steps 1 to 5: the number of rows in the cohort, and the weight of
    each distinct value, added in file order."""
    ref, cohort = claim["context"]["reference"], claim["context"]["cohort"]
    lab, demo = tables[ref["dataset"]], tables[DEMO]
    weights = tables[ref["weight"]["dataset"]][ref["weight"]["variable"]]
    rules = [(tables[r["dataset"]][r["variable"]], r["min"], r["max"]) for r in cohort["rules"]]
    n, by_value = 0, {}
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
        n += 1
        by_value[x] = by_value.get(x, 0.0) + w
    return n, by_value


def place(n, by_value, value):
    """Steps 6 to 8, with the sums over the values in rising order."""
    if n < MIN_N:
        return {"n": n}
    values = sorted(by_value)
    total = below = equal = 0.0
    for x in values:
        total += by_value[x]
        if x < value:
            below += by_value[x]
        elif x == value:
            equal = by_value[x]
    out = {
        "n": n,
        "percentile": round1(100 * (below + equal / 2) / total),
        "below": round1(100 * below / total),
        "equal": round1(100 * equal / total),
    }
    for name, q in QUARTILES:
        running = 0.0
        for x in values:
            running += by_value[x]
            if running >= q * total:
                break
        out[name] = x
    return out


class Inputs:
    """The records and data files found under the paths given."""

    def __init__(self):
        self.records = {}   # id -> parsed record, checked
        self.bad = {}       # id or name -> why a record was not accepted
        self.files = {}     # SHA-256 -> path
        self.tables = {}    # SHA-256 -> table
        self.packs = {}     # SHA-256 -> pack, checked
        self.packbad = {}   # SHA-256 -> (path, why a pack was not accepted)

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

    def add_pack(self, path):
        h = hashlib.sha256()
        with open(path, "rb") as f:
            data = f.read(packcheck.MAX_BYTES + 1)
            h.update(data)
            for chunk in iter(lambda: f.read(1 << 20), b""):
                h.update(chunk)
        digest = h.hexdigest()
        try:
            pack = packcheck.parse_pack(data)
        except packcheck.Refused as err:
            self.packbad[digest] = (path, str(err))
            return
        pack.sha256 = digest
        self.packs[digest] = pack

    def add(self, path, nested=False):
        if os.path.isdir(path):
            for base, _, names in sorted(os.walk(path)):
                for n in sorted(names):
                    # A pack is looked for at the top of a folder only.
                    self.add(os.path.join(base, n), nested=nested or base != path)
        elif path.endswith(".pack") and not nested:
            self.add_pack(path)
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


def find_reading(claim, inputs):
    """The reading a claim is about, and its value, or an error."""
    ctx = claim["context"]
    rid = ctx["reading"]
    reading = inputs.records.get(rid)
    if reading is None and rid in inputs.bad:
        raise ValueError("the reading %s fails its checks: %s" % (str(rid)[7:19], inputs.bad[rid]))
    if reading is None:
        raise Missing("the reading %s is not among the records given" % str(rid)[7:19])
    if {"rel": "about", "id": rid} not in claim.get("refs", []) or reading.get("subject") != claim.get("subject"):
        raise ValueError("the claim is not about the reading it names, or they are about two people")
    value = reading.get("value", {}).get("n")
    if not is_number(value) or not value > 0:
        raise ValueError("the reading is not a number above zero")
    return reading, value


def check(claim, inputs):
    """Returns the text for one comparison that reproduces. Raises Missing
    when an input is not there, ValueError when a check fails."""
    ctx = claim["context"]
    method = ctx["method"]
    if (method.get("id"), method.get("version")) == PACK_METHOD:
        return check_pack_claim(claim, inputs)
    if (method.get("id"), method.get("version")) != METHOD:
        raise ValueError("a method this tool does not know: %s@%s" % (method.get("id"), method.get("version")))
    reading, value = find_reading(claim, inputs)
    caveats, by = check_form(claim, reading)
    ref = ctx["reference"]
    tables, off_record = {}, []
    for f in ref["files"]:
        if f["sha256"] not in inputs.files:
            raise Missing("the file %s (SHA-256 %s) is not among the paths given" % (f["dataset"], f["sha256"]))
        tables[f["dataset"]] = inputs.table(f["sha256"])
        if KNOWN.get(f["dataset"]) != f["sha256"]:
            off_record.append(f["dataset"])
    # The claim says in words what each variable is: the files must agree.
    named = [(ref["dataset"], ref["variable"], ref["label"])] + [(r["dataset"], r["variable"], r["label"]) for r in ctx["cohort"]["rules"]]
    for dataset, variable, label in named:
        if tables[dataset].attrs["labels"].get(variable) != label:
            raise ValueError("%s.%s is not labelled %r in the file" % (dataset, variable, label))
    n, by_value = cohort_weights(claim, tables)
    got = place(n, by_value, float(value))
    want = ctx["result"]
    what = "%s %s %s against %s %r, %s" % (reading["type"], value, reading["value"].get("unit", ""), ref["variable"], ref["label"], ctx["cohort"]["text"])
    if got != want or claim["value"] != {"n": want.get("percentile"), "unit": "{percentile}"}:
        raise ValueError("%s: the claim says %s, this tool computes %s" % (what, json.dumps(want, sort_keys=True), json.dumps(got, sort_keys=True)))
    if got["n"] < SMALL_N:
        caveats.append(CAVEAT_SMALL)
    # A claim made on a file that was not on record says so. Which files are
    # on record is this tool's own word, below, and may differ from bean's
    # on the day of the claim: that caveat is not compared.
    theirs = [c for c in ctx.get("caveats", []) if c not in {CAVEAT_UNKNOWN % f["dataset"] for f in ref["files"]}]
    if theirs != caveats + by:
        raise ValueError("%s: the caveats are not the ones these inputs give" % what)
    note = "; NOT the files on record: " + ", ".join(off_record) if off_record else ""
    return "%s: percentile %s, n %s%s" % (what, got["percentile"], got["n"], note)


def same_number(a, b):
    """Two JSON numbers that are equal, neither of them a boolean."""
    return is_number(a) and is_number(b) and a == b


def check_pack_claim(claim, inputs):
    """A claim of the method weighted-percentile, version 1, against a pack:
    the four steps of docs/REFERENCE.md, "The claim"."""
    ctx = claim["context"]
    reading, value = find_reading(claim, inputs)
    if claim.get("author") != claim.get("subject"):
        raise ValueError("the claim is not signed by the person it is about")
    # The context has the members the document names, and no others.
    for name, got, want in (("context", ctx, PACK_CONTEXT), ("reference", ctx["reference"], PACK_REFERENCE),
                            ("cohort", ctx["cohort"], {"text"}), ("method", ctx["method"], {"id", "version"}),
                            ("result", ctx["result"], PACK_RESULT)):
        if not isinstance(got, dict) or set(got) != want:
            raise ValueError("the %s of the claim does not have exactly the members the format names" % name)
    ref, want = ctx["reference"], ctx["result"]
    digest = ref["pack"]
    if not isinstance(digest, str) or not packcheck.SHA_RE.fullmatch(digest):
        raise ValueError("reference.pack is not a SHA-256")
    # Step 1: the pack has the SHA-256 the claim names.
    if digest in inputs.packbad:
        raise ValueError("the pack %s is not a pack: %s" % (digest[:12], inputs.packbad[digest][1]))
    # The claim is about its reading and nothing else.
    if claim.get("refs") != [{"rel": "about", "id": ctx["reading"]}]:
        raise ValueError("the claim's refs are not the one reading it is about")
    # A pack is what its SHA-256 says, whatever its file is called.
    if digest not in inputs.packs and digest not in inputs.packbad and digest in inputs.files:
        inputs.add_pack(inputs.files[digest])
        if digest in inputs.packbad:
            raise ValueError("the pack %s is not a pack: %s" % (digest[:12], inputs.packbad[digest][1]))
    pack = inputs.packs.get(digest)
    if pack is None:
        raise Missing("the pack with SHA-256 %s is not among the paths given: get that pack and give its file" % digest)
    if pack.sha256 != digest:
        raise ValueError("the pack does not have the SHA-256 the claim names")
    mid, cid = ref["measure"], ref["cohort"]
    if not isinstance(mid, str) or not isinstance(cid, str):
        raise ValueError("reference.measure and reference.cohort are not ids")
    measure, cohort = pack["measures"].get(mid), pack["cohorts"].get(cid)
    if measure is None or cohort is None:
        raise ValueError("the pack has no %s %s" % (("measure", mid) if measure is None else ("cohort", cid)))
    cell = pack.cells.get((mid, cid))
    if cell is None:
        raise ValueError("the pack has no cell for measure %s and cohort %s" % (mid, cid))
    # Step 2: the reading's type and unit are in the measure's same list.
    kind = (reading.get("type"), (reading.get("value") or {}).get("unit"))
    if {"type": kind[0], "unit": kind[1]} not in measure["same"]:
        raise ValueError("the pack does not set %s in %s against %s" % (kind[0], kind[1], mid))
    # Step 3: the texts and the caveats are the pack's.
    for what, got, expect in (("title", ref["title"], pack["title"]), ("source", ref["source"], pack["source"]["name"]),
                              ("label", ref["label"], measure["label"]), ("unit", ref["unit"], measure["unit"]),
                              ("cohort.text", ctx["cohort"]["text"], cohort["text"])):
        if got != expect:
            raise ValueError("%s is not the pack's: %r" % (what, expect))
    if (ctx["method"]["id"], ctx["method"]["version"]) != PACK_METHOD:
        raise ValueError("a method this tool does not know")
    caveats = list(pack["caveats"]) + list(measure["caveats"]) + list(cohort["caveats"]) + [CAVEAT_COHORT]
    if cell["n"] < SMALL_N:
        caveats.append(CAVEAT_SMALL)
    if reading.get("author") != claim["author"]:
        caveats.append(CAVEAT_AUTHOR)
    what = "%s %s %s against %s %r in pack %s" % (reading["type"], value, reading["value"].get("unit", ""), mid, measure["label"], digest[:12])
    # Step 4: the method gives every number of the result.
    got = packcheck.place(cell, float(value))
    if not all(same_number(got[k], want[k]) for k in PACK_RESULT):
        raise ValueError("%s: the claim says %s, this tool computes %s" % (what, json.dumps(want, sort_keys=True), json.dumps(got, sort_keys=True)))
    cv = claim["value"]
    if not isinstance(cv, dict) or set(cv) != {"n", "unit"} or not same_number(cv["n"], got["percentile"]) or cv["unit"] != "{percentile}":
        raise ValueError("%s: the claim's value is not the percentile in {percentile}" % what)
    if ctx["caveats"] != caveats:
        raise ValueError("%s: the caveats are not the ones these inputs give" % what)
    return "%s, %s: percentile %s, n %s" % (what, cohort["text"], packcheck.num(got["percentile"]), got["n"])


def main(argv):
    if len(argv) < 2:
        sys.stderr.write(__doc__)
        return 2
    inputs = Inputs()
    for path in argv[1:]:
        try:
            inputs.add(path)
        except OSError as err:
            print("cannot read %s: %s" % (safe(path), err))
            return 1
    claims = [r for r in inputs.records.values() if r["type"] == "comparison.percentile"]
    bad = 0
    for digest, (path, why) in sorted(inputs.packbad.items()):
        print("REFUSED pack %s %s" % (digest[:12], safe("%s: %s" % (path, why))))
        bad += 1
    for rid, why in sorted(inputs.bad.items()):
        if why != "not a record":
            print("REFUSED %s %s" % (safe(str(rid)[7:19] if str(rid).startswith("sha256:") else rid), safe(why)))
            bad += 1
    if not claims and not bad:
        print("no comparison among the records given")
        return 1
    signers = {}
    for claim in sorted(claims, key=lambda r: r["id"]):
        try:
            word, text = "reproduced", check(claim, inputs)
            signers[claim["kid"]] = signers.get(claim["kid"], 0) + 1
        except Missing as err:
            word, text = "CANNOT CHECK", str(err)
        except (KeyError, TypeError, ValueError, AttributeError, OSError, OverflowError, packcheck.NotPlaced) as err:
            word, text = "DIFFERS", "%s" % err
        bad += word != "reproduced"
        print("%s %s %s" % (word, claim["id"][7:19], safe(text)))
    for kid, n in sorted(signers.items()):
        print("signed by %s: %d" % (safe(kid), n))
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
