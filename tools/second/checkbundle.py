#!/usr/bin/env python3
"""checkbundle.py: check a bean share bundle without bean.

    checkbundle.py <bundle folder>

A bundle is what `bean share` writes: objects/ holds record files, files/
holds shared files named by their SHA-256. This runs the second verifier
(beanverify.py) on the records, then the checks that make a folder of
records a bundle: the five rules of docs/SHARE.md. It shares no code with bean.
It prints `bundle ok` and exits 0, or the problems and exits 1.

It checks that:
 1. every record file passes the checks of a log: its id is its hash, its
    signature is good, and its key spoke for its author;
 2. there is exactly one share record, signed by a person about themselves;
 3. the records are exactly the ones the share record lists, each valid and
    about that person, plus that person's identity and pairing records; it
    counts apart the records that an agent or another key signed;
 4. every stored file a listed record names is accounted for, and every
    file in files/ is one the share record includes, with its SHA-256 and
    size, and with the SHA-256 its own record states where it states one.

A bundle shows that what it holds is authentic, not that it is complete.
"""
import hashlib
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import beanverify  # noqa: E402

IDENTITY = ("bean.genesis", "bean.succession", "bean.seal")
PAIR = ("agent.pair", "device.pair")


def real_dir(path):
    return os.path.isdir(path) and not os.path.islink(path)


def safe(s):
    """Text from a bundle, with nothing that can move the cursor or fake a
    line."""
    return "".join(c if c.isprintable() else "\\x%02x" % ord(c) for c in s)


def check(folder):
    problems = []
    # objects/ and files/ are real folders, not links that lead out of the
    # bundle, and nothing else is there. Names that start with a dot are not
    # part of a bundle, as in a log.
    if not real_dir(os.path.join(folder, "objects")):
        return ["%s is not a bundle: it has no objects folder" % folder], None
    for name in sorted(os.listdir(folder)):
        if name.startswith("."):
            continue
        if name not in ("objects", "files") or not real_dir(os.path.join(folder, name)):
            problems.append("%s is in the bundle, and a bundle holds only objects and files, as folders" % name)
    if problems:
        return problems, None
    files, _ = beanverify.load_dir(folder)
    records = {}
    for path, data in files.items():
        try:
            r, unknown = beanverify.check_file(path, data)
        except beanverify.Fail:
            continue  # verify reports it below
        if not unknown:
            records[r["id"]] = r
    # No encrypted blobs travel in a bundle, so every record that names one
    # gets "blob missing" from a log verifier. That is expected here, and
    # only that: the share record must account for each such file (4).
    _, _, info = beanverify.verify(files, {})
    for where, err in sorted(info["errors"].items()):
        if not err.startswith("blob missing: "):
            problems.append("a record file fails its checks: %s: %s" % (where, err))
    valid = set(info["valid"]) | {i for i, e in info["errors"].items() if e.startswith("blob missing: ")}
    shares = [r for i, r in sorted(records.items()) if r["type"] == "share" and i in valid]
    if len(shares) != 1:
        return problems + ["a bundle holds exactly one valid share record; this one holds %d" % len(shares)], None
    share = shares[0]
    person = share["author"]
    if not beanverify.ID_RE.fullmatch(person) or share.get("subject") != person or share["source"].get("system") != "bean-share":
        return problems + ["the share record is not signed by a person about themselves"], None
    ctx = share.get("context", {})
    listed = ctx.get("records", [])
    if not isinstance(listed, list) or len(set(map(str, listed))) != len(listed) or not all(isinstance(i, str) and beanverify.ID_RE.fullmatch(i) for i in listed):
        return problems + ["the share record's list is damaged"], None
    listed = set(listed)
    paired = {r.get("value", {}).get("key") for i, r in records.items()
              if r["type"] in PAIR and r["author"] == person and r.get("subject") == person and i in valid}
    signed = [0, 0, 0]  # by the person, by a paired agent, by another key
    for i, r in sorted(records.items()):
        if i in listed:
            signed[0 if r["author"] == person else 1 if r["author"] in paired else 2] += 1
        if i == share["id"]:
            continue
        if r["type"] in IDENTITY and (i == person or r.get("subject") == person):
            continue
        if r["type"] in PAIR and r["author"] == person and r.get("subject") == person:
            continue
        if i not in listed:
            problems.append("%s is in the bundle, and the share record does not list it" % i)
        elif i not in valid:
            problems.append("%s is listed, and is not valid" % i)
        elif r.get("subject") != person:
            problems.append("%s is listed, and is about someone else" % i)
    for i in sorted(listed - set(records)):
        problems.append("%s is listed, and is not in the bundle" % i)
    account, want = {}, {}
    for e in ctx.get("files", []) or []:
        ok = isinstance(e, dict) and isinstance(e.get("blob"), str) and e.get("record") in listed and isinstance(e.get("included"), bool)
        if not ok or (e["included"] and not (isinstance(e.get("sha256"), str) and len(e["sha256"]) == 64 and isinstance(e.get("bytes"), (int, float)))):
            return problems + ["the share record's list of files is damaged"], None
        account[(e["blob"], e["record"])] = e
        if e["included"]:
            want[e["sha256"]] = e["bytes"]
    included = named = 0
    for i in sorted(listed & set(records)):
        r = records[i]
        blobs = r.get("blobs", []) or []
        for b in blobs:
            e = account.get((b["hash"], i))
            if e is None:
                problems.append("%s names a stored file that the share record does not account for" % i)
                continue
            stated = r.get("context", {}).get("sha256")
            if isinstance(stated, str) and stated and len(blobs) == 1 and r.get("value", {}).get("blob") == b["hash"] and e.get("sha256") and e["sha256"] != stated:
                problems.append("the file of %s is not the one its record states" % i)
            if e["included"]:
                included += 1
            else:
                named += 1
    fdir = os.path.join(folder, "files")
    for name in sorted(os.listdir(fdir)) if os.path.isdir(fdir) else []:
        if name.startswith("."):
            continue
        if name not in want:
            problems.append("files/%s is in the bundle, and the share record does not list it" % name)
            continue
        size = want.pop(name)
        full = os.path.join(fdir, name)
        if os.path.islink(full) or not os.path.isfile(full):
            problems.append("files/%s is not a plain file" % name)
            continue
        # The size first: a file of another size is not read at all.
        h = hashlib.sha256()
        if os.path.getsize(full) == size:
            with open(full, "rb") as f:
                for chunk in iter(lambda: f.read(1 << 20), b""):
                    h.update(chunk)
        if os.path.getsize(full) != size or h.hexdigest() != name:
            problems.append("files/%s is not the file the share record lists" % name)
    for name in sorted(want):
        problems.append("files/%s is listed, and is not in the bundle" % name)
    return problems, (share, person, len(listed), included, named, signed)


def main(argv):
    if len(argv) != 2:
        sys.stderr.write(__doc__)
        return 2
    problems, facts = check(argv[1])
    if facts:
        share, person, n, included, named, signed = facts
        print("share %s" % share["id"])
        print("signed by identity %s, with key %s" % (person, share["kid"]))
        print("records: %d listed; files: %d included, %d named and not included" % (n, included, named))
        print("signed: %d by the person, %d by an agent the person paired, %d by another key" % tuple(signed))
    for p in sorted(problems):
        print("problem: " + safe(p))
    if problems:
        print("bundle FAILED: %d problems" % len(problems))
        return 1
    print("bundle ok")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
