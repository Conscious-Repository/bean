#!/usr/bin/env python3
"""checkreply.py: check a bean reply without bean.

    checkreply.py <reply folder>

A reply is what `bean reply` writes: a sealed package of record files
under objects/. unseal.py opens it into a folder, and this checks the
folder by the six rules of docs/SHARE.md, "The reply, exactly". It is
written from that text and shares no code with bean. It prints `reply ok`
and exits 0, or the problems and exits 1.

 1. It holds records and nothing else. Every record file passes the checks
    of a log. One warning is expected: a note is about a person whose
    identity records are not in the reply.
 2. Exactly one record of type `reply`, with source.system `bean-reply`. A
    person signed it about themselves, with the last key of their chain.
 3. Its context has `to`, `share` and `notes`; value.n is the count.
 4. Every other record is an identity record on that chain, or a listed
    note. Every listed note is there.
 5. A note is text about one record, by the reply's key, about `to`.
 6. A note's source is the one bean reply writes, and it states one time.

What this cannot tell is what only the receiver's log can: that `to` is
them, that `share` is a share of theirs, that each note is about a record
of that share, and whether they saw a later key of this identity.
"""
import datetime
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import beanverify  # noqa: E402
import checkbundle  # noqa: E402

IDENTITY = ("bean.genesis", "bean.succession", "bean.seal")
SYSTEM = "bean-reply"
MAX_NOTE, MAX_NOTES = 4000, 200


def minute(text):
    """An RFC 3339 time, cut to its minute in UTC, or None."""
    if not isinstance(text, str):
        return None
    try:
        t = datetime.datetime.fromisoformat(text.replace("Z", "+00:00"))
    except ValueError:
        return None
    if t.tzinfo is None:
        return None
    return t.astimezone(datetime.timezone.utc).replace(second=0, microsecond=0)


def chain(records, valid, who):
    """The identity's records from its genesis to its last key change, or
    None when they are not one line: a fork, or a change that hangs on
    nothing."""
    if who not in records or who not in valid or records[who]["type"] != "bean.genesis":
        return None
    line, changes = [who], [r for i, r in records.items() if r["type"] == "bean.succession" and r.get("subject") == who and i in valid]
    while True:
        nxt = [r for r in changes if r["refs"][0]["id"] == line[-1]]
        if len(nxt) > 1:
            return None
        if not nxt:
            break
        line.append(nxt[0]["id"])
    return line if len(line) == 1 + len(changes) else None


def note_fault(r, key, to):
    """Why a listed record is not a note of this reply, or ""."""
    value, refs = r.get("value"), r.get("refs") or []
    text = value.get("s") if isinstance(value, dict) else None
    source, times = r.get("source") or {}, r.get("times") or {}
    observed = times.get("observed")
    if r["type"] != "note":
        return "its type is not note"
    if r["author"] != key or r.get("kid") != key:
        return "the reply's key did not sign it by itself"
    if r.get("subject") != to:
        return "it is not about the person that the reply is for"
    if len(refs) != 1 or refs[0].get("rel") != "about":
        return "it is not about exactly one record"
    if not isinstance(text, str) or set(value) != {"s"} or not 1 <= len(text.encode("utf-8")) <= MAX_NOTE or not text.strip():
        return "its value is not a text of 1 to %d bytes" % MAX_NOTE
    if r.get("blobs") or r.get("context"):
        return "it holds more than a text"
    if source.get("system") != SYSTEM or source.get("ref") is not None:
        return "its source is not the one that bean reply writes"
    if observed is not None:
        at, written = minute(observed.get("start") if isinstance(observed, dict) else None), minute(times.get("imported"))
        if at is None or at != written or "end" in observed:
            return "it states another time than when it was written"
    return ""


def check(folder):
    problems = []
    if not checkbundle.real_dir(os.path.join(folder, "objects")):
        return ["%s is not a reply: it has no objects folder" % folder], None
    # Rule 1: nothing beside the records. A name that starts with a dot is
    # not read, as in a bundle.
    for name in sorted(os.listdir(folder)):
        if name != "objects" and not name.startswith("."):
            problems.append("a reply holds records and nothing else: %s" % checkbundle.safe(name))
    files = checkbundle.read_records(folder, problems)
    lines, _, info = beanverify.verify(files, {})
    records = info["records"]
    if info["errors"]:
        problems.append("a record file fails a check of a log")
    if info["unknown"] or any(l.startswith("too new: ") for l in lines) or not lines[-1].split(", ")[2].startswith("0 "):
        problems.append("a record is of a newer format than this tool knows, so it cannot be checked")
    if any(l.startswith("compromised: ") or "incomplete seals" in l for l in lines):
        problems.append("an identity in the reply is frozen, or a key change is not whole")
    if problems:
        return problems, None
    valid = set(info["valid"]) - set(info["purged"])
    # Rule 2.
    replies = [r for _, r in sorted(records.items()) if r["type"] == "reply"]
    if len(replies) != 1 or replies[0]["id"] not in valid:
        return ["a reply holds exactly one reply record, and it is valid; this one holds %d" % len(replies)], None
    rep = replies[0]
    who = rep["author"]
    if not checkbundle.is_id(who) or rep.get("subject") != who or (rep.get("source") or {}).get("system") != SYSTEM:
        return ["the reply record is not signed by a person about themselves, as bean reply writes it"], None
    line = chain(records, valid, who)
    if line is None:
        return ["the identity's chain of keys is not whole"], None
    key = records[line[-1]]["value"]["key"]
    if rep.get("kid") != key:
        return ["the reply record is not signed by the current key of its identity"], None
    # Rule 3.
    ctx = rep.get("context") if isinstance(rep.get("context"), dict) else {}
    value = rep.get("value") if isinstance(rep.get("value"), dict) else {}
    to, share, listed = ctx.get("to"), ctx.get("share"), checkbundle.id_list(ctx, "notes")
    if not checkbundle.is_id(to) or not checkbundle.is_id(share) or listed is None or not checkbundle.whole(value.get("n")) or value["n"] != len(listed):
        return ["the reply record's lists are damaged"], None
    if to == who:
        return ["the reply is for the identity that wrote it"], None
    if not 1 <= len(listed) <= MAX_NOTES:
        return ["a reply holds from 1 to %d notes" % MAX_NOTES], None
    # Rule 1, the warnings: only the expected one, once, on a listed note.
    warned = {}
    for l in lines:
        if l.startswith("warning: "):
            _, i, text = l.split(": ", 2)
            warned.setdefault(i, []).append(text)
    for i, ws in sorted(warned.items()):
        if i not in listed or ws != ["unresolved subject: " + to]:
            problems.append("%s has a warning that a reply may not have" % i)
    # Rules 4 to 6.
    for i, r in sorted(records.items()):
        if i == rep["id"]:
            continue
        if i in listed:
            fault = note_fault(r, key, to) if i in valid else "it is not valid"
            if fault:
                problems.append("%s is listed as a note: %s" % (i, fault))
        elif not (r["type"] in IDENTITY and (i == who or r.get("subject") == who) and i in valid):
            problems.append("%s is in the reply, and is not a listed note or an identity record of the replier" % i)
    for i in sorted(listed - set(records)):
        problems.append("%s is listed, and is not in the reply" % i)
    return problems, (rep, who, key, to, share, len(listed))


def main(argv):
    if len(argv) != 2:
        print("usage: checkreply.py <reply folder>", file=sys.stderr)
        return 2
    problems, found = check(argv[1])
    if problems:
        for p in problems:
            print("problem: " + checkbundle.safe(p))
        print("the reply did not pass: nothing in it is checked")
        return 1
    rep, who, key, to, share, n = found
    print("reply %s" % rep["id"])
    print("from the identity %s" % who)
    print("identity words: %s" % checkbundle.id_words(who))
    print("signed by the key %s" % checkbundle.safe(key))
    print("for the identity %s, about the share %s" % (to, share))
    print("notes: %d" % n)
    print("This cannot show what only the receiver's log can: that the reply is for them, that the share is theirs, and whether they saw a later key of this identity.")
    print("reply ok")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
