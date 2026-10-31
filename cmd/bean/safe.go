package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// safe returns s with every character that can end a line, move the
// cursor, start a terminal escape or reorder text written as \xNN or
// \uNNNN, so text from a record cannot fake output lines or control the
// terminal (audit 4, M3). Other characters are unchanged.
func safe(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r >= 0x80 && r <= 0x9f, r == 0x061c, r == 0x200e, r == 0x200f,
			r >= 0x2028 && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}
