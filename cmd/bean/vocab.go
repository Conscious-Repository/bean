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
var setup = []string{"agent.pair", "device.pair"}

// takenTerm reports whether a personal term would redefine a built-in
// term or open a core or setup type to agents.
func takenTerm(t string) bool {
	return builtin.allows(t) || core.IsCoreType(t) || slices.Contains(setup, t)
}

// takenPrefix is takenTerm for a personal prefix such as "cgm.".
func takenPrefix(p string) bool {
	if slices.Contains(builtin.prefixes, p) || strings.HasPrefix(p, "bean.") || strings.HasPrefix("bean.", p) {
		return true
	}
	for t := range builtin.exact {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return slices.ContainsFunc(setup, func(t string) bool { return strings.HasPrefix(t, p) })
}

// loadVocabulary returns the vocabulary of the folder dir and its text:
// the built-in one, plus dir/vocabulary.md when it exists.
func loadVocabulary(dir string) (vocab, string, error) {
	path := filepath.Join(dir, "vocabulary.md")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return builtin, vocabulary, nil
	}
	if err != nil {
		return vocab{}, "", fmt.Errorf("vocabulary.md: %w", err)
	}
	if len(b) > 64<<10 {
		return vocab{}, "", errors.New("vocabulary.md: larger than 64 KiB")
	}
	mine := parseVocabulary(string(b))
	for t := range mine.exact {
		if takenTerm(t) {
			return vocab{}, "", fmt.Errorf("vocabulary.md in %s: `%s` is a built-in or reserved term; a changed meaning is a new term", dir, t)
		}
	}
	for _, p := range mine.prefixes {
		if takenPrefix(p) {
			return vocab{}, "", fmt.Errorf("vocabulary.md in %s: `%s<...>` overlaps a built-in or reserved term; a changed meaning is a new term", dir, p)
		}
	}
	all := vocab{exact: map[string]bool{}, prefixes: append(slices.Clone(builtin.prefixes), mine.prefixes...)}
	for t := range builtin.exact {
		all.exact[t] = true
	}
	for t := range mine.exact {
		all.exact[t] = true
	}
	return all, vocabulary + "\n\n# Personal vocabulary (" + path + ")\n\n" + string(b), nil
}
