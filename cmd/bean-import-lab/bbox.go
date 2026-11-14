package main

// Reading a report by word position. `pdftotext -bbox-layout` prints every
// word of a PDF with its box in points, from the top left of its page. Where a
// word is printed is exact, so a layout whose columns drift in the -layout
// text can still be read by where each word starts (DECISIONS 235). This file
// only reads the words and groups them into lines; what a layout's lines
// mean is in that layout's file (quest.go).

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
)

// word is one word and its box: x0, y0 top left, x1, y1 bottom right.
type word struct {
	x0, y0, x1, y1 float64
	text           string
}

// bline is the words of one page whose tops lie within lineTolerance of
// each other, left to right.
type bline struct {
	page  int     // from 1
	y     float64 // the top of its first word
	words []word
}

type bpage struct {
	width, height float64
	lines         []bline
}

const (
	maxBBoxPages = 1000
	maxBBoxWords = 1000000
	// lineTolerance is how far apart, in points, the tops of two words on
	// one line may be.
	lineTolerance = 1.0
)

// bboxElements are the elements pdftotext -bbox-layout writes. Any other
// element is not its output, and is refused.
var bboxElements = map[string]bool{
	"html": true, "head": true, "title": true, "meta": true, "body": true,
	"doc": true, "page": true, "flow": true, "block": true, "line": true, "word": true,
}

// isBBoxDoc reports whether text is pdftotext -bbox-layout output rather
// than -layout text.
func isBBoxDoc(text string) bool {
	t := strings.TrimLeft(text, " \t\r\n")
	return (strings.HasPrefix(t, "<!DOCTYPE html") || strings.HasPrefix(t, "<html")) && strings.Contains(t, "<doc>")
}

// parseBBox reads pdftotext -bbox-layout output into pages of lines.
// Entities are unescaped. A word is text with no space or control
// character, in a box inside its page.
func parseBBox(doc string) ([]bpage, error) {
	d := xml.NewDecoder(strings.NewReader(doc))
	d.Strict = true
	var (
		pages        []bpage
		words        [][]word
		in           []string // open elements
		cur          *word
		text         strings.Builder
		count, total int
	)
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("the word positions are not pdftotext -bbox-layout output: not well-formed XHTML")
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			if !bboxElements[name] {
				return nil, fmt.Errorf("the word positions hold a %q element, which pdftotext -bbox-layout does not write", name)
			}
			if cur != nil {
				return nil, errors.New("the word positions hold an element inside a word")
			}
			switch name {
			case "page":
				if slices.Contains(in, "page") {
					return nil, errors.New("the word positions hold a page inside a page")
				}
				if len(pages) >= maxBBoxPages {
					return nil, fmt.Errorf("report exceeds %d pages", maxBBoxPages)
				}
				w, h := attrFloat(t, "width"), attrFloat(t, "height")
				if !(w > 0 && w <= 100000 && h > 0 && h <= 100000) {
					return nil, fmt.Errorf("page %d has no size", len(pages)+1)
				}
				pages = append(pages, bpage{width: w, height: h})
				words = append(words, nil)
			case "word":
				if !slices.Contains(in, "page") {
					return nil, errors.New("the word positions hold a word outside a page")
				}
				if count++; count > maxBBoxWords {
					return nil, fmt.Errorf("report exceeds %d words", maxBBoxWords)
				}
				p := pages[len(pages)-1]
				w := word{attrFloat(t, "xMin"), attrFloat(t, "yMin"), attrFloat(t, "xMax"), attrFloat(t, "yMax"), ""}
				if !(w.x0 >= 0 && w.x0 <= w.x1 && w.x1 <= p.width+1 && w.y0 >= 0 && w.y0 <= w.y1 && w.y1 <= p.height+1) {
					return nil, fmt.Errorf("page %d holds a word whose box is not inside the page", len(pages))
				}
				cur = &w
				text.Reset()
			}
			in = append(in, name)
		case xml.EndElement:
			in = in[:len(in)-1]
			if t.Name.Local == "word" && cur != nil {
				cur.text = text.String()
				if !safeText(cur.text) || strings.Contains(cur.text, " ") {
					return nil, fmt.Errorf("page %d holds a word with a space, control or invisible character, so where it starts cannot be read for sure", len(pages))
				}
				words[len(words)-1] = append(words[len(words)-1], *cur)
				cur = nil
			}
		case xml.CharData:
			switch {
			case cur != nil:
				if text.Len()+len(t) > maxLineBytes {
					return nil, errors.New("a word is longer than a line may be")
				}
				text.Write(t)
			case len(in) > 0 && in[len(in)-1] == "title":
			case strings.TrimSpace(string(t)) != "":
				return nil, errors.New("the word positions hold text outside a word")
			}
		}
	}
	if len(pages) == 0 {
		return nil, errors.New("the word positions hold no page")
	}
	for i := range pages {
		lines, err := groupLines(i+1, words[i])
		if err != nil {
			return nil, err
		}
		if total += len(lines); total > maxReportLines {
			return nil, errors.New("report exceeds line limit")
		}
		pages[i].lines = lines
	}
	return pages, nil
}

func attrFloat(e xml.StartElement, name string) float64 {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			f, err := strconv.ParseFloat(a.Value, 64)
			if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
				return math.NaN()
			}
			return f
		}
	}
	return math.NaN()
}

// groupLines puts the words of a page into lines: top to bottom, a word
// whose top is within lineTolerance of the first word of the line is on
// it. Words of a line do not overlap: two words printed over each other
// cannot be read for sure.
func groupLines(page int, ws []word) ([]bline, error) {
	ws = slices.Clone(ws)
	slices.SortStableFunc(ws, func(a, b word) int {
		if c := cmpFloat(a.y0, b.y0); c != 0 {
			return c
		}
		return cmpFloat(a.x0, b.x0)
	})
	var lines []bline
	for _, w := range ws {
		if n := len(lines); n > 0 && w.y0-lines[n-1].y <= lineTolerance {
			lines[n-1].words = append(lines[n-1].words, w)
			continue
		}
		lines = append(lines, bline{page: page, y: w.y0, words: []word{w}})
	}
	for _, l := range lines {
		slices.SortStableFunc(l.words, func(a, b word) int { return cmpFloat(a.x0, b.x0) })
		for i := 1; i < len(l.words); i++ {
			if l.words[i].x0 < l.words[i-1].x1-0.5 {
				return nil, fmt.Errorf("page %d, %s: two words are printed over each other, so the line cannot be read for sure", page, atY(l.y))
			}
		}
	}
	return lines, nil
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// at names a place on a page, for an error.
func atY(y float64) string { return fmt.Sprintf("%.0f pt from the top", y) }

// text joins the words of a line with one space.
func (l bline) text() string { return joinWords(l.words) }

func joinWords(ws []word) string {
	s := make([]string, len(ws))
	for i, w := range ws {
		s[i] = w.text
	}
	return strings.Join(s, " ")
}

// wordsAre reports whether the words of ws are exactly want.
func wordsAre(ws []word, want ...string) bool {
	if len(ws) != len(want) {
		return false
	}
	for i, w := range ws {
		if w.text != want[i] {
			return false
		}
	}
	return true
}

// spaced reports whether the gap after a is wider than a word space: more
// than one and a half of a's characters. Words of a phrase are one space
// apart; columns are further.
func spaced(a, b word) bool {
	n := len([]rune(a.text))
	return b.x0-a.x1 > 1.5*(a.x1-a.x0)/float64(n)
}

// phrase is the number of words from the start of ws that are one space
// apart.
func phrase(ws []word) int {
	i := 1
	for i < len(ws) && !spaced(ws[i-1], ws[i]) {
		i++
	}
	return i
}
