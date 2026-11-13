#!/usr/bin/env python3
"""forgereply.py: writes replies with the independent Python writer, one
folder per case, each signed with keys that Python made. No reply here is
damaged: every signature is good. A case that is wrong breaks one rule of
docs/SHARE.md, "The reply, exactly", and nothing else.

    forgereply.py <new folder>

It prints one line per case: its folder's name, then `ok` or `bad`.
replies.sh asks bean and checkreply.py for a verdict on each.
"""
import os, shutil, sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import writer
from writer import Key, rec, Log, Ident

OUT = sys.argv[1]
shutil.rmtree(OUT, ignore_errors=True)
writer.OUT[0] = OUT

TO = "sha256:" + "b" * 64      # the person that the reply is for
SHARE = "sha256:" + "c" * 64   # their share record
ABOUT = "sha256:" + "d" * 64   # a record of that share
WRITTEN = "2026-11-12T14:33:04Z"
SOURCE = {"system": "bean-reply", "adapter": "craft@1", "ref": None}


def note(who, key=None, **change):
    """A note of the replier's key about the person, with one change."""
    key = key or who.key
    f = dict(type="note", subject=TO, author=key.pub, value={"s": "a note"}, refs=[{"rel": "about", "id": ABOUT}],
             times={"observed": None, "imported": WRITTEN}, source=dict(SOURCE))
    f.update(change)
    return rec(key, **f)


def case(name, want, notes=1, change=None, listed=None, reply=None, rotate=False, before=None, extra=None):
    """One reply. change(i, who) gives the fields that note i changes. listed
    gives the ids that the reply record lists, from the notes' ids. reply
    gives the fields that the reply record changes."""
    log = Log(name)
    who = Ident(log, 7)
    old = who.key
    if rotate:
        who.rotate([])
    if before:
        before(who)
    made = [note(who, **(change(i, who) if change else {})) for i in range(notes)]
    for r in made:
        log.put(r)
    ids = [r["id"] for r in made]
    ids = listed(ids) if listed else ids
    f = dict(type="reply", subject=who.id, author=who.id, value={"n": len(ids), "unit": "{note}"},
             times={"observed": None, "imported": WRITTEN}, source=dict(SOURCE), context={"to": TO, "share": SHARE, "notes": ids})
    signer = who.key
    for k, v in (reply(who, old) if reply else {}).items():
        if k == "signer":
            signer = v
        elif k in ("value", "context"):
            f[k].update(v)
        else:
            f[k] = v
    log.put(rec(signer, **f))
    if extra:
        extra(log, who)
    print(name, want)


stranger = Key(bytes([0x33]) * 32)

# What passes.
case("py-one-note", "ok")
case("py-two-notes", "ok", notes=2)
case("py-200-notes", "ok", notes=200)
case("py-text-4000-bytes", "ok", change=lambda i, w: {"value": {"s": "a" * 4000}})
case("py-text-4000-bytes-of-two", "ok", change=lambda i, w: {"value": {"s": "é" * 2000}})
case("py-text-a-control-character", "ok", change=lambda i, w: {"value": {"s": "\u001f"}})
case("py-observed-in-the-minute", "ok", change=lambda i, w: {"times": {"observed": {"start": "2026-11-12T14:33:00Z", "precision": "minute"}, "imported": WRITTEN}})
case("py-observed-in-another-zone", "ok", change=lambda i, w: {"times": {"observed": {"start": "2026-11-12T16:33:00+02:00", "precision": "minute"}, "imported": WRITTEN}})
case("py-after-a-key-change", "ok", rotate=True)

# Rule 2: the reply record.
case("py-reply-of-another-source", "bad", reply=lambda w, old: {"source": {"system": "a hospital", "adapter": "craft@1", "ref": None}})
case("py-reply-about-someone-else", "bad", reply=lambda w, old: {"subject": TO})
case("py-reply-by-the-older-key", "bad", rotate=True, change=lambda i, w: {}, reply=lambda w, old: {"signer": old})
# Rule 3: its lists.
case("py-for-its-own-writer", "bad", reply=lambda w, old: {"context": {"to": w.id}}, change=lambda i, w: {"subject": w.id})
case("py-count-is-wrong", "bad", reply=lambda w, old: {"value": {"n": 2}})
case("py-a-note-listed-twice", "bad", listed=lambda ids: ids + ids, reply=lambda w, old: {"value": {"n": 1}})
case("py-no-notes", "bad", notes=0)
case("py-201-notes", "bad", notes=201)
case("py-share-is-not-an-id", "bad", reply=lambda w, old: {"context": {"share": "the last one"}})
# Rule 4: what else is there.
case("py-a-note-not-listed", "bad", notes=2, listed=lambda ids: ids[:1])
case("py-a-listed-note-not-there", "bad", listed=lambda ids: ids + ["sha256:" + "e" * 64])
case("py-a-strangers-genesis", "bad", extra=lambda log, w: log.put(rec(stranger, type="bean.genesis", author=stranger.pub, value={"key": stranger.pub, "recovery": writer.commitment(Key(bytes([0x34]) * 32).pub)})))
# Rule 5: a note.
case("py-a-lab-value-as-a-note", "bad", change=lambda i, w: {"type": "lab.cholesterol-total", "value": {"n": 120, "unit": "mg/dL"}})
case("py-note-of-another-type", "bad", change=lambda i, w: {"type": "symptom"})
case("py-note-by-another-key", "bad", change=lambda i, w: {"key": stranger, "author": stranger.pub})
case("py-note-about-someone-else", "bad", change=lambda i, w: {"subject": "sha256:" + "f" * 64})
case("py-note-about-the-replier", "bad", change=lambda i, w: {"subject": w.id})
case("py-note-about-two-records", "bad", change=lambda i, w: {"refs": [{"rel": "about", "id": ABOUT}, {"rel": "about", "id": SHARE}]})
case("py-note-about-no-record", "bad", change=lambda i, w: {"refs": []})
case("py-text-4001-bytes", "bad", change=lambda i, w: {"value": {"s": "a" * 4001}})
case("py-text-4001-bytes-of-two", "bad", change=lambda i, w: {"value": {"s": "é" * 2000 + "a"}})
case("py-text-of-spaces", "bad", change=lambda i, w: {"value": {"s": " \t\n "}})
case("py-text-of-no-break-spaces", "bad", change=lambda i, w: {"value": {"s": "  "}})
case("py-text-empty", "bad", change=lambda i, w: {"value": {"s": ""}})
case("py-value-with-more", "bad", change=lambda i, w: {"value": {"s": "a note", "n": 1}})
case("py-note-with-context", "bad", change=lambda i, w: {"context": {"verified": "by a hospital"}})
# Rule 6: a note's source and time.
case("py-note-of-another-source", "bad", change=lambda i, w: {"source": {"system": "a hospital", "adapter": "craft@1", "ref": None}})
case("py-note-with-a-source-ref", "bad", change=lambda i, w: {"source": {"system": "bean-reply", "adapter": "craft@1", "ref": "hmac-sha256:" + "a" * 64}})
case("py-observed-in-another-minute", "bad", change=lambda i, w: {"times": {"observed": {"start": "2021-03-01T09:00:00Z", "precision": "minute"}, "imported": WRITTEN}})
case("py-observed-with-an-end", "bad", change=lambda i, w: {"times": {"observed": {"start": "2026-11-12T14:33:00Z", "end": "2026-11-12T14:33:30Z", "precision": "minute"}, "imported": WRITTEN}})
