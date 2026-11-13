package main

// The type vocabulary (SPEC.md 9.9) is app level: the verifier never reads
// it. log_entry lets an agent write only its types. The built-in list is
// vocabulary.md next to this file; a person adds terms of their own in
// vocabulary.md in their data folder, in the same table form. A personal
// term never redefines a built-in one: a changed meaning is a new term.

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Conscious-Repository/bean/core"
)

//go:embed vocabulary.md
var vocabulary string

// vocab is the set of types in a vocabulary table: exact names, and
// prefixes for a name written as `exercise.<kind>`.
type vocab struct {
	exact    map[string]bool
	prefixes []string
}

var builtin = parseVocabulary(vocabulary)

// parseVocabulary reads the types in the first cell of each table row. A
// row starts the line; an indented one is an example.
func parseVocabulary(text string) vocab {
	v := vocab{exact: map[string]bool{}}
	for _, line := range strings.Split(text, "\n") {
		cells := strings.Split(line, "|")
		if !strings.HasPrefix(line, "|") || len(cells) < 3 || !strings.HasPrefix(strings.TrimSpace(cells[1]), "`") {
			continue
		}
		names, _, _ := strings.Cut(cells[1], " (") // drop examples in parentheses
		for _, part := range strings.Split(names, "`") {
			part = strings.TrimSpace(part)
			if part == "" || part == "," {
				continue
			}
			if p, _, ok := strings.Cut(part, "<"); ok {
				v.prefixes = append(v.prefixes, p)
			} else {
				v.exact[part] = true
			}
		}
	}
	return v
}

// allows reports whether typ is a term: an exact name, or one more part
// after a prefix.
func (v vocab) allows(typ string) bool {
	if v.exact[typ] {
		return true
	}
	for _, p := range v.prefixes {
		if rest, ok := strings.CutPrefix(typ, p); ok && rest != "" && !strings.Contains(rest, ".") {
			return true
		}
	}
	return false
}

// setup are the pairing types; a person writes them, never an agent.
// device.pair is only a name kept free: no code writes or reads one until
// device capture is built (docs/EXTENSIONS.md 11), and a personal term must
// not take it before then.
var setup = []string{"agent.pair", "device.pair"}

// takenTerm reports whether a personal term would redefine a built-in
// term or open a core or setup type to agents.
func takenTerm(t string) bool {
	return builtin.allows(t) || core.IsCoreType(t) || slices.Contains(setup, t)
}

// takenPrefix is takenTerm for a personal prefix such as "cgm.": true
// when the prefix can name a built-in, core or setup type. Two prefixes
// can name the same type when one is the other plus a part with no dot.
func takenPrefix(p string) bool {
	mine := vocab{prefixes: []string{p}}
	for t := range builtin.exact {
		if mine.allows(t) {
			return true
		}
	}
	return slices.ContainsFunc(builtin.prefixes, func(q string) bool { return extends(p, q) || extends(q, p) }) ||
		slices.ContainsFunc(setup, mine.allows) || strings.HasPrefix(p, "bean.") || strings.HasPrefix("bean.", p)
}

// extends reports whether a is b plus a part with no dot.
func extends(a, b string) bool {
	rest, ok := strings.CutPrefix(a, b)
	return ok && !strings.Contains(rest, ".")
}

// loadVocabulary returns the vocabulary of the folder dir and its text:
// the built-in one, plus dir/vocabulary.md when it exists.
func loadVocabulary(dir string) (vocab, string, error) {
	path := filepath.Join(dir, "vocabulary.md")
	// Its text goes to the assistant. A link there would send another
	// file's text, and a pipe would hold the read: only a plain file of
	// the folder is read.
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return vocab{}, "", errors.New("vocabulary.md in the bean folder is not a plain file: a link or a pipe is not read")
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return builtin, vocabulary, nil
	}
	if err != nil {
		return vocab{}, "", errors.New("vocabulary.md in the bean folder: cannot read it")
	}
	if len(b) > 64<<10 {
		return vocab{}, "", errors.New("vocabulary.md in the bean folder: larger than 64 KiB")
	}
	mine := parseVocabulary(string(b))
	for _, t := range sortedStrings(mine.exact) {
		if takenTerm(t) {
			return vocab{}, "", fmt.Errorf("vocabulary.md in the bean folder: `%s` is a built-in or reserved term; a changed meaning is a new term", t)
		}
	}
	for _, p := range mine.prefixes {
		if takenPrefix(p) {
			return vocab{}, "", fmt.Errorf("vocabulary.md in the bean folder: `%s<...>` overlaps a built-in or reserved term; a changed meaning is a new term", p)
		}
	}
	text := vocabulary + "\n\n# Personal vocabulary (vocabulary.md in the bean folder)\n\n" + string(b)
	return parseVocabulary(text), text, nil
}
