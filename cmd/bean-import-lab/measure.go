package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// measure is bean-import-lab -count. It reads a report as far as each
// reader gets and says how far that was, in counts and line numbers. It
// writes no record, and prints no value, no name of a test and no word of
// the report: what a refusal quotes is masked, letters as x and digits as
// 9. A person can send its output to say which layouts bean cannot read,
// and how much of each it could.
func measure(text string, raw []byte) []string {
	kind := "text"
	switch {
	case bytes.HasPrefix(raw, []byte("%PDF")):
		kind = "pdf"
	case isBBoxDoc(text):
		kind = "word positions"
	}
	out := []string{fmt.Sprintf("input: %s, %d lines of text", kind, len(splitLines(text)))}
	// What marks a Quest report in its text.
	trademark, header := "no", "no"
	for _, l := range splitLines(text) {
		switch f := strings.Join(strings.Fields(l), " "); {
		case f == questTrademark:
			trademark = "yes"
		case f == strings.Join(questMain.header, " "):
			header = "main"
		case f == strings.Join(questAlt.header, " "):
			header = "alt"
		}
	}
	out = append(out, fmt.Sprintf("quest marks: trademark line %s, column header %s", trademark, header))
	// As an import reads it.
	if rep, err := readReport(text, raw); err != nil {
		out = append(out, "as an import reads it: no result, "+masked(err.Error()))
	} else {
		out = append(out, fmt.Sprintf("as an import reads it: %d results", len(rep.Results)))
	}
	// The text grammar, past the lines before the column header that it
	// does not know.
	if !isBBoxDoc(text) {
		p := &parser{rep: &Report{Labs: map[string]string{}}, past: true}
		stopped := ""
		for i, l := range splitLines(text) {
			if err := p.line(i+1, l); err != nil {
				stopped = masked(err.Error())
				break
			}
		}
		if stopped == "" {
			if _, err := p.finish(); err != nil {
				stopped = "at the end, " + masked(err.Error())
			}
		}
		at := "no column header found"
		if p.colsAt > 0 {
			at = "column header found"
		}
		end := "read to the end"
		if stopped != "" {
			end = "stopped: " + stopped
		}
		out = append(out, fmt.Sprintf("text grammar, past unknown lines before the header: %s, %d lines passed over, %d results, %s", at, p.passed, len(p.rep.Results), end))
	}
	// By word position, whatever the text says.
	if kind == "pdf" {
		line := "by word position: "
		if bbox, err := pdftotext(raw, "-bbox-layout"); err != nil {
			line += "no word positions"
		} else if pages, err := parseBBox(bbox); err != nil {
			line += "no result, " + masked(err.Error())
		} else if rep, err := parseQuest(pages); err != nil {
			line += fmt.Sprintf("%d pages, Quest pages %v, no result, %s", len(pages), questPages(pages), masked(err.Error()))
		} else {
			line += fmt.Sprintf("%d pages, Quest pages %v, %d results", len(pages), questPages(pages), len(rep.Results))
		}
		out = append(out, line)
	}
	return out
}

// maxMasked is the most characters of a quote that a masked refusal shows.
const maxMasked = 20

var (
	quotedText = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
	labType    = regexp.MustCompile(`lab\.[A-Za-z0-9._-]+`)
)

// masked is a refusal with the report's own text hidden: what it quotes,
// and a type that it made from a name. Every refusal of this importer puts
// what it takes from the report between quotation marks. A long quote is
// cut: its length could say something too.
func masked(msg string) string {
	msg = quotedText.ReplaceAllStringFunc(msg, func(q string) string {
		if r := []rune(q); len(r) > maxMasked+2 {
			q = string(r[:maxMasked+1]) + `..."`
		}
		return strings.Map(func(r rune) rune {
			switch {
			case r >= '0' && r <= '9':
				return '9'
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r > 127:
				return 'x'
			}
			return r
		}, q)
	})
	return labType.ReplaceAllString(msg, "lab.xxx")
}
