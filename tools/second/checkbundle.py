#!/usr/bin/env python3
"""checkbundle.py: check a bean share bundle without bean.

    checkbundle.py <bundle folder>

A bundle is what `bean share` writes: objects/ holds record files, files/
holds shared files named by their SHA-256. This runs the second verifier
(beanverify.py) on the records, then the checks that make a folder of
records a bundle: the five rules of docs/SHARE.md. It shares no code with
bean. It prints `bundle ok` and exits 0, or the problems and exits 1.

 1. Every record file passes the checks of a log, with nothing left over:
    its id is its hash, its signature is good, its key spoke for its author.
 2. There is exactly one share record, signed by a person about themselves.
 3. The records are exactly the ones the share record lists, each valid,
    about that person and of a kind that is shared, plus the proof it
    names: that person's identity records and pairings.
 4. Each file the share record lists belongs to a listed record, and is
    the file that record states, where it states one.
 5. files/ holds exactly the files the share record lists, each with its
    SHA-256 and size.

A bundle shows that what it holds is authentic. It cannot show that it is
complete, that a value is true, who the identity is, or that the key that
signed is still the person's key.
"""
import hashlib
import os
import re
import stat
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import beanverify  # noqa: E402

IDENTITY = ("bean.genesis", "bean.succession", "bean.seal")
NOT_SHARED = IDENTITY + ("agent.pair", "bean.tombstone", "share")
MAX_RECORD = 16 << 20
HEX64 = re.compile(r"^[0-9a-f]{64}$")
CAVEATS = (
    "These are selected records, not a complete history: a correction or a deletion that the share leaves out does not show.",
    "A signature shows which identity signed, not that a value is true, nor who that identity is.",
    "The key above is the one that signed. A bundle cannot show that it is still the person's key: a key change made since is not in it. When it matters, ask the person for their current key (bean whoami).",
)


def real_dir(path):
    return os.path.isdir(path) and not os.path.islink(path)


WORDS_SHA256 = "2f5eed53a4727b4bf8880d8f3f199efc90e58503646d9ff8eff3a2ed3b24dbda"


def id_words(ident):
    """The first 66 bits of an identity id as six words of the BIP-39
    English list: the words that the sender reads out. The list is the
    public file core/bip39-english.txt, checked by its SHA-256."""
    path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "core", "bip39-english.txt")
    try:
        with open(path, "rb") as f:
            data = f.read()
    except OSError:
        return "not shown: the word list core/bip39-english.txt is not beside this tool"
    if hashlib.sha256(data).hexdigest() != WORDS_SHA256:
        return "not shown: core/bip39-english.txt is not the BIP-39 English list"
    words = data.decode("ascii").split()
    if not re.fullmatch(r"sha256:[0-9a-f]{64}", ident):
        return ""
    bits = bin(int(ident[7:], 16))[2:].zfill(256)
    return " ".join(words[int(bits[11 * i:11 * i + 11], 2)] for i in range(6))


def safe(s):
    """Text from a bundle, with nothing that can move the cursor or fake a
    line."""
    return "".join(c if c.isprintable() else "\\x%02x" % ord(c) for c in s)


def is_id(x):
    return isinstance(x, str) and beanverify.ID_RE.fullmatch(x) is not None


def whole(x):
    """A whole number that is not a boolean, and not below zero."""
    return isinstance(x, (int, float)) and not isinstance(x, bool) and x >= 0 and x == int(x)


def read_records(folder, problems):
    """The record files: plain files two levels below objects/, as in a
    log. Nothing else of the folder is read, nothing that is not a plain
    file is opened, and no file larger than a record can be is read."""
    files = {}
    base = os.path.join(folder, "objects")
    for shard in sorted(os.listdir(base)):
        if shard.startswith("."):
            continue
        sd = os.path.join(base, shard)
        if not real_dir(sd):
            problems.append("objects/%s is not a record file: not a plain file, or too large" % shard)
            continue
        for name in sorted(os.listdir(sd)):
            if name.startswith("."):
                continue
            rel = "objects/%s/%s" % (shard, name)
            st = os.lstat(os.path.join(sd, name))
            if not stat.S_ISREG(st.st_mode) or st.st_size > MAX_RECORD:
                problems.append("%s is not a record file: not a plain file, or too large" % rel)
                continue
            with open(os.path.join(sd, name), "rb") as f:
                files[rel] = f.read()
    return files


def id_list(ctx, key):
    """A list of distinct ids, or None when it is not one."""
    v = ctx.get(key)
    if not isinstance(v, list) or not all(is_id(i) for i in v) or len(set(v)) != len(v):
        return None
    return set(v)


def check(folder):
    problems = []
    # The folder: objects/ and files/ are real folders, not links that lead
    # out of the bundle.
    if not real_dir(os.path.join(folder, "objects")):
        return ["%s is not a bundle: it has no objects folder" % folder], None
    # Anything else in the folder is not part of the bundle: it is never
    # read, so it cannot change the verdict.
    files_entry = os.path.join(folder, "files")
    if os.path.lexists(files_entry) and not real_dir(files_entry):
        return ["files is not a folder: a link or a file in its place is not followed"], None
    # Rule 1.
    files = read_records(folder, problems)
    # No encrypted file travels in a bundle, so a log's verifier says "blob
    # missing" for every valid record that names one. That, and only that,
    # is not a fault here: rule 4 accounts for each such file.
    lines, _, info = beanverify.verify(files, {})
    records, newer = info["records"], bool(info["unknown"])
    missing = {i for i, e in info["errors"].items() if e.startswith("blob missing: ")}
    if len(missing) != len(info["errors"]):
        problems.append("a record file fails a check of a log")
    if newer or any(l.startswith("too new: ") for l in lines) or not lines[-1].split(", ")[2].startswith("0 "):
        problems.append("a record is of a newer format than this tool knows, so it cannot be checked")
    if any(l.startswith("compromised: ") or "incomplete seals" in l for l in lines):
        problems.append("an identity in the bundle is frozen, or a key change is not whole")
    valid = (set(info["valid"]) | missing) - set(info["purged"])
    # Rule 2.
    shares = [r for _, r in sorted(records.items()) if r["type"] == "share"]
    if len(shares) != 1 or shares[0]["id"] not in valid:
        return problems + ["a bundle holds exactly one share record, and it is valid; this one holds %d" % len(shares)], None
    share = shares[0]
    person = share["author"]
    if not is_id(person) or share.get("subject") != person:
        return problems + ["the share record is not signed by a person about themselves"], None
    # Its lists, read strictly.
    ctx = share.get("context") if isinstance(share.get("context"), dict) else {}
    listed, proof = id_list(ctx, "records"), id_list(ctx, "proof")
    value = share.get("value") if isinstance(share.get("value"), dict) else {}
    if listed is None or proof is None or not whole(value.get("n")) or value["n"] != len(listed):
        return problems + ["the share record's lists are damaged"], None
    # The files it holds: each entry names a stored file of a listed record,
    # with the SHA-256 and the size of its plaintext.
    account, want = {}, {}
    entries = ctx.get("files", [])
    damaged = not isinstance(entries, list)
    for e in entries if not damaged else []:
        if not isinstance(e, dict):
            damaged = True
            continue
        blob, record, digest, size = e.get("blob"), e.get("record"), e.get("sha256"), e.get("bytes")
        if (not is_id(blob) or not is_id(record) or record not in listed or (blob, record) in account
                or not (isinstance(digest, str) and HEX64.fullmatch(digest)) or not whole(size)):
            damaged = True
            continue
        if want.get(digest, size) != size:
            damaged = True
        account[(blob, record)] = e
        want[digest] = size
    if damaged:
        return problems + ["the share record's list of files is damaged"], None
    # Rule 1, the warnings. One is expected: a listed record that corrects an
    # earlier record is "held" by a log's verifier when that earlier record
    # is not there, and a share leaves it out on purpose. Any other fails.
    warned = {}
    for l in lines:
        if l.startswith("warning: "):
            _, i, text = l.split(": ", 2)
            if not text.startswith("blob missing: "):
                warned.setdefault(i, []).append(text)
    held = {i for i, ws in warned.items() if any(w.startswith("held: ") for w in ws)}
    corrections = 0
    for i, ws in sorted(warned.items()):
        r = records.get(i)
        targets = [x["id"] for x in (r.get("refs") or []) if x.get("rel") == "supersedes"] if r else []
        if r and i in listed and targets and all(w.startswith("held: ") for w in ws) and not any(t in records for t in targets):
            corrections += 1
        else:
            problems.append("%s has a warning that a bundle may not have" % i)
    valid |= {i for i in held if i in listed}
    # Rule 3.

    def pairing(r):
        # A pairing in force: an agent.pair that does not say "unpaired".
        v = r.get("value") if isinstance(r.get("value"), dict) else {}
        return r["type"] == "agent.pair" and r["author"] == person and r.get("subject") == person and isinstance(v.get("key"), str) and v.get("key") and v.get("status") != "unpaired"

    # A key paired under two labels shows under both, in the order of the
    # pairings' ids, so that the same bundle reads the same on every check.
    paired = {}
    for i in sorted(proof):
        if i in records and pairing(records[i]) and i in valid:
            key, label = records[i]["value"]["key"], records[i]["value"].get("label") or "agent"
            if key not in paired:
                paired[key] = label
            elif label not in paired[key].split(", "):
                paired[key] += ", " + label
    signed = [0, 0, 0]  # by the person, by a paired agent, by another key
    for i, r in sorted(records.items()):
        if i == share["id"]:
            continue
        if i in listed and i in proof:
            problems.append("%s is listed as a record and as proof" % i)
        elif i in proof:
            on_chain = r["type"] in IDENTITY and (i == person or r.get("subject") == person) and i not in held
            if i not in valid or not (on_chain or pairing(r)):
                problems.append("%s is named as proof, and is not a valid identity record or pairing of the person" % i)
        elif i not in listed:
            problems.append("%s is in the bundle, and the share record does not list it" % i)
        elif i not in valid:
            problems.append("%s is listed, and is not valid" % i)
        elif r.get("subject") != person:
            problems.append("%s is listed, and is about someone else" % i)
        elif r["type"] in NOT_SHARED:
            problems.append("%s is listed, and is a %s record, which is not shared" % (i, r["type"]))
        else:
            signed[0 if r["author"] == person else 1 if r["author"] in paired else 2] += 1
    for i in sorted(listed - set(records)):
        problems.append("%s is listed, and is not in the bundle" % i)
    for i in sorted(proof - set(records)):
        problems.append("%s is named as proof, and is not in the bundle" % i)
    # Rule 4: each entry is for a stored file that its listed record names,
    # and is the file that record states, where it states one. A stored file
    # with no entry stays home: it is counted, not faulted.
    named = 0
    for i in sorted(listed & set(records)):
        r = records[i]
        blobs = r.get("blobs", []) or []
        once = set()
        for b in blobs:
            if b["hash"] in once:
                continue  # a record may name one file twice
            once.add(b["hash"])
            e = account.pop((b["hash"], i), None)
            if e is None:
                named += 1
                continue
            stated = (r.get("context") or {}).get("sha256")
            if isinstance(stated, str) and stated and len(blobs) == 1 and (r.get("value") or {}).get("blob") == b["hash"] and e["sha256"] != stated:
                problems.append("the file of %s is not the one its record states" % i)
    if account:
        problems.append("the share record lists a file that no listed record names")
    # Rule 5.
    included = len(want)
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
    holds = sorted((records[i] for i in listed if i in records), key=lambda r: (str(((r.get("times") or {}).get("observed") or {}).get("start", "")), r["id"]))
    return problems, (share, person, len(listed), included, named, signed, corrections, holds, paired)


def main(argv):
    if len(argv) != 2:
        sys.stderr.write(__doc__)
        return 2
    try:
        problems, facts = check(argv[1])
    except (OSError, KeyError, TypeError, ValueError, AttributeError, IndexError) as err:
        problems, facts = ["the bundle cannot be read as one: %s: %s" % (type(err).__name__, err)], None
    if facts:
        share, person, n, included, named, signed, corrections, holds, paired = facts
        obs = (share.get("times") or {}).get("observed") or {}
        print("share %s" % share["id"])
        print("signed by identity %s, with key %s, at %s" % (person, share["kid"], safe(str(obs.get("start", "-")))))
        print("identity words: %s" % id_words(person))
        print("records: %d listed; files: %d included, %d named and not included" % (n, included, named))
        print("signed: %d by the person, %d by an agent the person paired, %d by another key" % tuple(signed))
        print("corrections: %d of the listed records replace an earlier record, which is not in the bundle" % corrections)
        # The note is shown only with a bundle that passed.
        note = (share.get("context") or {}).get("note")
        if isinstance(note, str) and note and not problems:
            print("note: " + safe(note))
        if not problems:
            # Shown only when every check passed.
            print("It holds:")
            # Each record as it is written: when, what, its value, and who
            # signed it when that is not the person.
            for r in holds:
                when = ((r.get("times") or {}).get("observed") or {}).get("start", "-")
                by = "" if r["author"] == person else "  [%s]" % paired[r["author"]] if r["author"] in paired else "  [signed by another key: %s]" % r["author"]
                print("  %s  %s  %s%s  %s" % (safe(when), r["type"], safe(beanverify.canonical(r.get("value", {}))), safe(by), r["id"][7:19]))
        for line in CAVEATS:
            print(line)
    for p in sorted(problems):
        print("problem: " + safe(p))
    if problems:
        print("bundle FAILED: %d problems" % len(problems))
        return 1
    print("bundle ok")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
