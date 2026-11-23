#!/usr/bin/env python3
"""check.py: check a reference pack and place a value in it, without bean.

    check.py <pack>
    check.py <pack> place <measure id> <cohort id> <value>

Written from docs/REFERENCE.md alone (formats pack/0 and pack/1, method
weighted-percentile version 1); it shares no code with bean.

The first form applies every rule of the document to the file. On success it
prints

    pack ok
    sha256 <64 hex digits of the file's bytes>
    <N> measures, <N> cohorts, <N> cells

and exits 0. Otherwise it prints one line, `not a pack: <why>`, and exits 1.

The second form checks the pack the same way and then sets the value in the
cell: `n=.. percentile=.. below=.. equal=.. p25=.. p50=.. p75=..`. It exits 1
with `not placed: <why>` when the cell is not there or the value is not a
number above zero.

As a module: read_pack(path) returns a Pack (a dict) or raises Refused, and
place(cell, value) returns the numbers of the result.

It shares no code with bean. It was written from the document, and where
the two checks first disagreed the document now states the rule.
"""
import hashlib
import json
import math
import re
import sys
import unicodedata
from decimal import Decimal

FORMAT = "pack/0"
MAX_BYTES = 64 * 1024 * 1024
MAX_N = 2147483647  # the largest n that a cell may state
MIN_NUMBER, MAX_NUMBER = 1e-12, 1e15  # the bounds of a value and of a weight
MAX_TEXT = 400
MAX_ID = 64
ID_RE = re.compile(r"[A-Za-z0-9][A-Za-z0-9._-]{0,63}")
SHA_RE = re.compile(r"[0-9a-f]{64}")
NUM_RE = re.compile(r"-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?")
TWO53 = 2 ** 53

TOP = ("bean", "title", "source", "builder", "caveats", "measures", "cohorts", "cells")
SOURCE = ("name", "publisher", "home", "files")
FILE = ("name", "sha256", "bytes", "url")
BUILDER = ("name", "version")
MEASURE = ("label", "unit", "caveats", "same")
SAME = ("type", "unit")
COHORT = ("text", "caveats")
CELL = ("measure", "cohort", "kind", "n", "values", "weights")


class Refused(Exception):
    """The file is not a pack; the message says why."""


class NotPlaced(Exception):
    """The value cannot be set in the cell."""


def safe(s, limit=120):
    """Text that came from a file, with nothing that can end a line or move
    the cursor, and cut short."""
    s = str(s)
    out = []
    for c in s[:limit]:
        o = ord(c)
        if c.isprintable() and c != "\\":
            out.append(c)
        elif c == "\\":
            out.append("\\\\")
        elif o < 0x100:
            out.append("\\x%02x" % o)
        elif o < 0x10000:
            out.append("\\u%04x" % o)
        else:
            out.append("\\U%08x" % o)
    if len(s) > limit:
        out.append("...")
    return "".join(out)


class Pack(dict):
    """The parsed pack. sha256 is that of the file's bytes; cells maps
    (measure, cohort) to the cell."""
    sha256 = ""
    cells = None


# ---------- reading the bytes ----------

def _int_hook(s):
    v = int(s)  # a ValueError for more digits than Python reads: refused
    # "a double can hold" is read with SPEC 3.8: a whole number must be
    # within +-(2^53 - 1), as bean's own records are.
    if abs(v) > TWO53 - 1:
        raise Refused("a whole number beyond 2^53 - 1: %s" % safe(s, 30))
    return v


def _float_hook(s):
    f = float(s)
    if math.isinf(f) or math.isnan(f):
        raise Refused("a number that overflows a double: %s" % safe(s, 30))
    if f == 0 and re.search(r"[1-9]", s.split("e")[0].split("E")[0]):
        # a literal that underflows to zero is a number a double cannot hold.
        raise Refused("a number that underflows a double: %s" % safe(s, 30))
    if abs(f) > TWO53 - 1 and f == math.floor(f):
        # the same limit for a whole number written with a point or an exponent.
        raise Refused("a whole number beyond 2^53 - 1: %s" % safe(s, 30))
    return f


def _const_hook(s):
    raise Refused("%s is not a JSON number" % safe(s, 30))


def _pairs_hook(items):
    out = {}
    for k, v in items:
        if k in out:
            raise Refused("a member is there twice: %s" % safe(k, 60))
        out[k] = v
    return out


def _no_surrogates(x):
    stack = [x]
    while stack:
        x = stack.pop()
        if isinstance(x, str):
            if any(0xD800 <= ord(c) <= 0xDFFF for c in x):
                raise Refused("a text with a lone surrogate")
        elif isinstance(x, dict):
            for k, v in x.items():
                stack.append(k)
                stack.append(v)
        elif isinstance(x, list):
            stack.extend(x)


def read_pack(path):
    """Read and check a pack. Returns a Pack, or raises Refused."""
    try:
        with open(path, "rb") as f:
            data = f.read(MAX_BYTES + 1)
    except OSError as err:
        raise Refused("cannot read the file: %s" % safe(err.strerror or err.__class__.__name__))
    return parse_pack(data)


def parse_pack(data):
    if len(data) > MAX_BYTES:
        raise Refused("more than 64 MiB")
    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError:
        raise Refused("not UTF-8")
    if text.startswith("﻿"):
        # the document says "in UTF-8" and nothing of a byte order mark.
        raise Refused("starts with a byte order mark")
    try:
        obj = json.loads(text, object_pairs_hook=_pairs_hook, parse_int=_int_hook,
                         parse_float=_float_hook, parse_constant=_const_hook)
    except Refused:
        raise
    except RecursionError:
        raise Refused("nested too deep")
    except (ValueError, OverflowError):
        raise Refused("not JSON")
    _no_surrogates(obj)
    pack = Pack(check_pack(obj))
    pack.sha256 = hashlib.sha256(data).hexdigest()
    pack.cells = {(c["measure"], c["cohort"]): c for c in obj["cells"]}
    return pack


# ---------- the rules ----------

def members(obj, names, where):
    if not isinstance(obj, dict):
        raise Refused("%s is not an object" % where)
    for k in obj:
        if k not in names:
            raise Refused("%s has a member the format does not have: %s" % (where, safe(k, 60)))
    for k in names:
        if k not in obj:
            raise Refused("%s lacks the member %s" % (where, k))


def text(x, where):
    if not isinstance(x, str):
        raise Refused("%s is not a text" % where)
    n = len(x.encode("utf-8"))
    if n < 1 or n > MAX_TEXT:
        raise Refused("%s is %d bytes, not 1 to %d" % (where, n, MAX_TEXT))
    for c in x:
        if c != " " and hidden(c):
            raise Refused("%s has a character that does not show, or a space other than the plain one: U+%04X" % (where, ord(c)))
    if x[0] == " " or x[-1] == " ":
        raise Refused("%s has a space at an end" % where)
    return x


def hidden(c):
    """A character that a text of a pack may not hold: a control, format,
    surrogate or private-use character, a separator, a space other than
    the plain one, a filler that shows as blank, or a noncharacter."""
    o = ord(c)
    return (unicodedata.category(c) in ("Cc", "Cf", "Cs", "Co", "Zl", "Zp", "Zs")
            or o in (0x115F, 0x1160, 0x3164, 0xFFA0)
            or 0xFDD0 <= o <= 0xFDEF or o & 0xFFFE == 0xFFFE)


def own_type(t):
    """A type of bean's own records: not a reading, so no measure claims it."""
    return t == "share" or t.startswith(("comparison.", "reference.", "bean.", "agent."))


def ident(x, where):
    if not isinstance(x, str) or not ID_RE.fullmatch(x):
        raise Refused("%s is not an id of 1 to 64 letters, digits, dots, - and _: %s" % (where, safe(x, 70)))
    return x


def is_num(x):
    return isinstance(x, (int, float)) and not isinstance(x, bool)


def whole(x, where, low):
    """A whole number, however it is written: 94, 94.0 and 9.4e1 are one
    number."""
    if not is_num(x) or x != int(x) or x < low:
        raise Refused("%s is not a whole number of %d or more" % (where, low))
    return int(x)


def text_list(x, where):
    if not isinstance(x, list):
        raise Refused("%s is not a list" % where)
    for i, t in enumerate(x):
        text(t, "%s[%d]" % (where, i))


def check_pack(p):
    members(p, TOP, "the pack")
    if p["bean"] not in (FORMAT, "pack/1", "pack/2"):
        raise Refused("unsupported pack format: expected pack/0, pack/1 or pack/2")
    text(p["title"], "title")
    members(p["source"], SOURCE, "source")
    for k in ("name", "publisher", "home"):
        text(p["source"][k], "source." + k)
    files = p["source"]["files"]
    if not isinstance(files, list) or not files:
        raise Refused("source.files is not a list of one entry or more")
    names = set()
    for i, f in enumerate(files):
        w = "source.files[%d]" % i
        members(f, FILE, w)
        text(f["name"], w + ".name")
        if f["name"] in names:
            # two source files of one name cannot be told apart.
            raise Refused("%s: a file name is there twice" % w)
        names.add(f["name"])
        if not isinstance(f["sha256"], str) or not SHA_RE.fullmatch(f["sha256"]):
            raise Refused("%s.sha256 is not 64 lower-case hex digits" % w)
        whole(f["bytes"], w + ".bytes", 1)  # a source file is not empty
        text(f["url"], w + ".url")
    members(p["builder"], BUILDER, "builder")
    text(p["builder"]["name"], "builder.name")
    text(p["builder"]["version"], "builder.version")
    text_list(p["caveats"], "caveats")

    measures, cohorts, cells = p["measures"], p["cohorts"], p["cells"]
    if not isinstance(measures, dict):
        raise Refused("measures is not an object")
    if not isinstance(cohorts, dict):
        raise Refused("cohorts is not an object")
    if not isinstance(cells, list) or not cells:
        raise Refused("cells is not a list of one cell or more")
    has_panel = has_evidence = False
    owner = {}
    for mid, m in measures.items():
        ident(mid, "a measure id")
        w = "measure " + safe(mid, 64)
        members(m, MEASURE, w)
        text(m["label"], w + " label")
        text(m["unit"], w + " unit")
        text_list(m["caveats"], w + " caveats")
        same = m["same"]
        if not isinstance(same, list) or not same:
            # a measure with no entry can never be compared with.
            raise Refused("%s: same is not a list of one entry or more" % w)
        for i, s in enumerate(same):
            qualified = p["bean"] != FORMAT and isinstance(s, dict) and "when" in s
            guarded = p["bean"] == "pack/2" and isinstance(s, dict) and "any_of" in s and not qualified
            members(s, (*SAME, "any_of") if guarded else (*SAME, "when") if qualified else SAME, "%s same[%d]" % (w, i))
            if guarded:
                choices = s["any_of"]
                if not isinstance(choices, list) or not 1 <= len(choices) <= 16:
                    raise Refused("any_of needs 1 through 16 alternatives")
                for rule in choices:
                    if not isinstance(rule, dict) or not 1 <= len(rule) <= 8:
                        raise Refused("each alternative needs 1 through 8 fields")
                    for key, value in rule.items():
                        if key != "source.system" and not re.fullmatch(r"context\.[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}", key):
                            raise Refused("unsupported evidence field")
                        text(value, "evidence value")
                has_evidence = True
            if qualified:
                members(s["when"], ("system", "test", "panel"), "same.when")
                for field in ("system", "test", "panel"):
                    text(s["when"][field], "when." + field)
                has_panel = True
            text(s["type"], "%s same[%d].type" % (w, i))
            text(s["unit"], "%s same[%d].unit" % (w, i))
            if own_type(s["type"]):
                raise Refused("%s claims %s, which is a record of bean's own and not a reading" % (w, safe(s["type"], 60)))
            key = (s["type"], s["unit"])
            if key in owner:
                if owner[key] == mid:
                    raise Refused("%s: the type and unit %s %s are there twice" % (w, safe(key[0], 60), safe(key[1], 30)))
                raise Refused("the type and unit %s %s are in the same lists of two measures" % (safe(key[0], 60), safe(key[1], 30)))
            owner[key] = mid
    if p["bean"] == "pack/2" and not has_evidence:
        raise Refused("pack/2 requires evidence")
    if p["bean"] == "pack/1" and not has_panel:
        raise Refused("pack/1 requires a panel-qualified match")
    for cid, c in cohorts.items():
        ident(cid, "a cohort id")
        w = "cohort " + safe(cid, 64)
        members(c, COHORT, w)
        text(c["text"], w + " text")
        text_list(c["caveats"], w + " caveats")

    seen = set()
    for i, c in enumerate(cells):
        w = "cell %d" % i
        members(c, CELL, w)
        if not isinstance(c["measure"], str) or not isinstance(c["cohort"], str):
            raise Refused("%s: measure and cohort are not ids" % w)
        ident(c["measure"], w + " measure")
        ident(c["cohort"], w + " cohort")
        if c["kind"] != "weights":
            raise Refused("%s is of a kind this reader does not know: %s" % (w, safe(c["kind"], 40) if isinstance(c["kind"], str) else "not a text"))
        if c["measure"] not in measures:
            raise Refused("%s names a measure the pack does not have: %s" % (w, safe(c["measure"], 64)))
        if c["cohort"] not in cohorts:
            raise Refused("%s names a cohort the pack does not have: %s" % (w, safe(c["cohort"], 64)))
        pair = (c["measure"], c["cohort"])
        if pair in seen:
            raise Refused("%s: the pair %s %s is there twice" % (w, safe(pair[0], 64), safe(pair[1], 64)))
        seen.add(pair)
        values, weights = c["values"], c["weights"]
        if not isinstance(values, list) or not isinstance(weights, list):
            raise Refused("%s: values and weights are not lists" % w)
        if not values:
            # no value gives a total weight of zero.
            raise Refused("%s has no value" % w)
        if len(weights) != len(values):
            raise Refused("%s has %d weights for %d values" % (w, len(weights), len(values)))
        last = None
        for j, v in enumerate(values):
            if not is_num(v):
                raise Refused("%s: value %d is not a number" % (w, j))
            if not MIN_NUMBER <= v <= MAX_NUMBER:
                raise Refused("%s: value %d is not from 1e-12 to 1e15" % (w, j))
            if last is not None and not v > last:
                raise Refused("%s: value %d is not above the one before" % (w, j))
            last = v
        for j, x in enumerate(weights):
            if not is_num(x):
                raise Refused("%s: weight %d is not a number" % (w, j))
            if not MIN_NUMBER <= x <= MAX_NUMBER:
                raise Refused("%s: weight %d is not from 1e-12 to 1e15" % (w, j))
        n = c["n"]
        if not is_num(n) or n != int(n) or not 1 <= n <= MAX_N:
            raise Refused("%s has an n that is not a whole number from 1 to %d" % (w, MAX_N))
        n = int(n)
        if n < len(values):
            raise Refused("%s has an n below the number of distinct values" % w)
    return p


# ---------- the method: weighted-percentile, version 1 ----------

def round1(y):
    return math.floor(10 * y + 0.5) / 10


def place(cell, value):
    """Steps 1 to 5 of the method for one cell. Returns a dict with n,
    percentile, below, equal, p25, p50, p75. Raises NotPlaced."""
    if not is_num(value):
        raise NotPlaced("the value is not a number above zero")
    try:
        v = float(value)
    except OverflowError:
        raise NotPlaced("the value is not a number above zero")
    if not (math.isfinite(v) and v > 0):
        raise NotPlaced("the value is not a number above zero")
    values = [float(x) for x in cell["values"]]
    weights = [float(x) for x in cell["weights"]]
    total = below = equal = 0.0
    for x, w in zip(values, weights):
        total += w
        if x < v:
            below += w
        elif x == v:
            equal = w
    out = {
        "n": cell["n"],
        "percentile": round1(100 * (below + equal / 2) / total),
        "below": round1(100 * below / total),
        "equal": round1(100 * equal / total),
    }
    for name, q in (("p25", 0.25), ("p50", 0.5), ("p75", 0.75)):
        running = 0.0
        pick = values[-1]
        for x, w in zip(values, weights):
            running += w
            if running >= q * total:
                pick = x
                break
        out[name] = pick
    return out


def num(x):
    """The shortest form that reads back as the same double, in digits and
    never with an exponent."""
    f = float(x)
    if f == 0:
        return "0"
    s = format(Decimal(repr(f)).normalize(), "f")
    return s


def parse_value(s):
    if not NUM_RE.fullmatch(s):
        raise NotPlaced("the value is not a number above zero")
    try:
        f = float(s)
    except (ValueError, OverflowError):
        raise NotPlaced("the value is not a number above zero")
    return f


def main(argv):
    try:
        sys.stdout.reconfigure(encoding="utf-8", errors="backslashreplace")
    except Exception:
        pass
    args = argv[1:]
    if len(args) not in (1, 5) or (len(args) == 5 and args[1] != "place"):
        sys.stderr.write("usage: check.py <pack>\n       check.py <pack> place <measure id> <cohort id> <value>\n")
        return 2
    try:
        pack = read_pack(args[0])
    except Refused as err:
        print("not a pack: %s" % err)
        return 1
    except Exception as err:  # never a traceback, whatever the input
        print("not a pack: could not be checked (%s)" % safe(err.__class__.__name__))
        return 1
    if len(args) == 1:
        print("pack ok")
        print("sha256 %s" % pack.sha256)
        print("%d measures, %d cohorts, %d cells" % (len(pack["measures"]), len(pack["cohorts"]), len(pack["cells"])))
        return 0
    try:
        cell = pack.cells.get((args[2], args[3]))
        if cell is None:
            raise NotPlaced("the pack has no cell for measure %s and cohort %s" % (safe(args[2], 64), safe(args[3], 64)))
        r = place(cell, parse_value(args[4]))
    except NotPlaced as err:
        print("not placed: %s" % err)
        return 1
    print("n=%d percentile=%s below=%s equal=%s p25=%s p50=%s p75=%s" % (
        r["n"], num(r["percentile"]), num(r["below"]), num(r["equal"]), num(r["p25"]), num(r["p50"]), num(r["p75"])))
    return 0


if __name__ == "__main__":
    try:
        code = main(sys.argv)
    except BaseException as err:  # never a traceback
        if isinstance(err, SystemExit):
            raise
        print("not a pack: could not be checked (%s)" % safe(err.__class__.__name__))
        code = 1
    sys.exit(code)
