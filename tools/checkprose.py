#!/usr/bin/env python3
"""checkprose.py: no sentence of a document is longer than 25 words.

    checkprose.py [-max N] <file.md>...

The documents use Simplified Technical English (DECISIONS 99 and 101). One
of its rules can be counted, so this counts it: a sentence has at most 25
words. It prints each longer sentence with its line, and exits 1 if there
is one.

How it reads Markdown: a fenced block, an indented block, a heading and a
table's rule line are not prose. A list item and a table cell each start a
new sentence. A code span is one word, and so is a link's text plus its
target. A sentence ends at ". ", "? " or "! " before a capital letter, a
digit, a quote, a bracket, a code span or the word bean.

docs/DECISIONS.md is not checked: it is a dated log, and an entry is not
rewritten after the fact.
"""
import re
import sys

END = re.compile(r"(?<=[.!?])[\"')\]*]*\s+(?=[\"'(\[`*A-Z0-9]|bean\b)")
ITEM = re.compile(r"(?:[-*+]|\d+\.|>)\s+(.*)")
RULE = re.compile(r"\|?[-: |]+\|?")


def sentences(path):
    """Yields (line number, sentence) for the prose of a Markdown file."""
    out, para, start, fence = [], [], 0, False

    def flush():
        if para:
            for s in END.split(" ".join(para)):
                out.append((start, s.strip()))
            para.clear()

    with open(path, encoding="utf-8") as f:
        for n, line in enumerate(f, 1):
            raw = line.rstrip("\n")
            s = raw.strip()
            if s.startswith("```"):
                flush()
                fence = not fence
                continue
            if fence or (raw.startswith("    ") and not para):
                continue
            if not s or s.startswith("#") or RULE.fullmatch(s):
                flush()
                continue
            row = s.startswith("|")
            for cell in [c.strip() for c in s.strip("|").split("|")] if row else [s]:
                m = ITEM.match(cell)
                if m or row:
                    flush()
                if m:
                    cell = m.group(1)
                if not para:
                    start = n
                para.append(cell)
                if row:
                    flush()
    flush()
    return out


def words(sentence):
    s = re.sub(r"`[^`]*`", "CODE", sentence)
    s = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", s)
    s = re.sub(r"[*_>#]", "", s)
    return [w for w in s.split() if re.search(r"\w", w)]


def main(argv):
    limit = 25
    if len(argv) > 2 and argv[1] == "-max":
        limit, argv = int(argv[2]), argv[:1] + argv[3:]
    if len(argv) < 2:
        sys.stderr.write(__doc__)
        return 2
    over = 0
    for path in argv[1:]:
        for n, s in sentences(path):
            k = len(words(s))
            if k > limit:
                over += 1
                print("%s:%d: %d words: %s" % (path, n, k, s))
    if over:
        print("prose: %d sentences are longer than %d words" % (over, limit))
        return 1
    print("prose: no sentence of %d documents is longer than %d words" % (len(argv) - 1, limit))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
