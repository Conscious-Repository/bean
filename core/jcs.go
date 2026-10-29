// Package core implements the bean core specification: canonical JSON,
// record ids, keys and signatures, key derivation, blob encryption, paper
// seeds and dedupe refs. It uses no network and no clock.
package core

import (
	"bytes"
	"errors"
	"math"
	"sort"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// Error strings from SPEC.md section 12.2.
var (
	ErrBadJSON      = errors.New("bad json")
	ErrDuplicateKey = errors.New("duplicate key")
	ErrBadNumber    = errors.New("bad number")
	ErrBadString    = errors.New("bad string")
)

const maxSafeInt = 1<<53 - 1

// Parse reads one JSON value with the strict rules of SPEC.md 3.8 steps 3 to
// 6. Objects become map[string]any, arrays []any, numbers float64, strings
// string, booleans bool and null nil. When the input has more than one kind
// of error, Parse reports the one that comes first in the validation order:
// bad json, duplicate key, bad number, bad string.
func Parse(data []byte) (any, error) {
	if !utf8.Valid(data) {
		return nil, ErrBadJSON
	}
	p := &parser{data: data}
	p.skipWS()
	v, ok := p.value(0)
	if !ok {
		return nil, ErrBadJSON
	}
	p.skipWS()
	if p.pos != len(p.data) {
		return nil, ErrBadJSON
	}
	switch {
	case p.dupKey:
		return nil, ErrDuplicateKey
	case p.badNumber:
		return nil, ErrBadNumber
	case p.badString:
		return nil, ErrBadString
	}
	return v, nil
}

type parser struct {
	data      []byte
	pos       int
	dupKey    bool
	badNumber bool
	badString bool
}

const maxDepth = 256

func (p *parser) skipWS() {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *parser) value(depth int) (any, bool) {
	if depth > maxDepth || p.pos >= len(p.data) {
		return nil, false
	}
	switch c := p.data[p.pos]; {
	case c == '{':
		return p.object(depth)
	case c == '[':
		return p.array(depth)
	case c == '"':
		s, ok := p.str()
		return s, ok
	case c == 't':
		return true, p.lit("true")
	case c == 'f':
		return false, p.lit("false")
	case c == 'n':
		return nil, p.lit("null")
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	}
	return nil, false
}

func (p *parser) lit(s string) bool {
	if bytes.HasPrefix(p.data[p.pos:], []byte(s)) {
		p.pos += len(s)
		return true
	}
	return false
}

func (p *parser) object(depth int) (any, bool) {
	p.pos++ // {
	m := map[string]any{}
	p.skipWS()
	if p.pos < len(p.data) && p.data[p.pos] == '}' {
		p.pos++
		return m, true
	}
	for {
		p.skipWS()
		if p.pos >= len(p.data) || p.data[p.pos] != '"' {
			return nil, false
		}
		k, ok := p.str()
		if !ok {
			return nil, false
		}
		p.skipWS()
		if p.pos >= len(p.data) || p.data[p.pos] != ':' {
			return nil, false
		}
		p.pos++
		p.skipWS()
		v, ok := p.value(depth + 1)
		if !ok {
			return nil, false
		}
		if _, dup := m[k]; dup {
			p.dupKey = true
		}
		m[k] = v
		p.skipWS()
		if p.pos >= len(p.data) {
			return nil, false
		}
		switch p.data[p.pos] {
		case ',':
			p.pos++
		case '}':
			p.pos++
			return m, true
		default:
			return nil, false
		}
	}
}

func (p *parser) array(depth int) (any, bool) {
	p.pos++ // [
	a := []any{}
	p.skipWS()
	if p.pos < len(p.data) && p.data[p.pos] == ']' {
		p.pos++
		return a, true
	}
	for {
		p.skipWS()
		v, ok := p.value(depth + 1)
		if !ok {
			return nil, false
		}
		a = append(a, v)
		p.skipWS()
		if p.pos >= len(p.data) {
			return nil, false
		}
		switch p.data[p.pos] {
		case ',':
			p.pos++
		case ']':
			p.pos++
			return a, true
		default:
			return nil, false
		}
	}
}

func hexVal(b []byte) (rune, bool) {
	var r rune
	for _, c := range b {
		r <<= 4
		switch {
		case c >= '0' && c <= '9':
			r |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			r |= rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			r |= rune(c-'A') + 10
		default:
			return 0, false
		}
	}
	return r, true
}

// str parses a JSON string starting at the opening quote. A lone escaped
// surrogate is recorded as a bad string and decoded as U+FFFD so parsing can
// continue and find a higher-priority error.
func (p *parser) str() (string, bool) {
	p.pos++ // "
	var out []byte
	for {
		if p.pos >= len(p.data) {
			return "", false
		}
		c := p.data[p.pos]
		switch {
		case c == '"':
			p.pos++
			return string(out), true
		case c < 0x20:
			return "", false
		case c == '\\':
			p.pos++
			if p.pos >= len(p.data) {
				return "", false
			}
			e := p.data[p.pos]
			p.pos++
			switch e {
			case '"', '\\', '/':
				out = append(out, e)
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'u':
				r, ok := p.uEscape()
				if !ok {
					return "", false
				}
				if utf16.IsSurrogate(r) {
					if r < 0xdc00 && p.pos+1 < len(p.data) && p.data[p.pos] == '\\' && p.data[p.pos+1] == 'u' {
						save := p.pos
						p.pos += 2
						r2, ok := p.uEscape()
						if !ok {
							return "", false
						}
						if r2 >= 0xdc00 && r2 <= 0xdfff {
							r = utf16.DecodeRune(r, r2)
						} else {
							p.pos = save
							p.badString = true
							r = utf8.RuneError
						}
					} else {
						p.badString = true
						r = utf8.RuneError
					}
				}
				out = utf8.AppendRune(out, r)
			default:
				return "", false
			}
		default:
			out = append(out, c)
			p.pos++
		}
	}
}

func (p *parser) uEscape() (rune, bool) {
	if p.pos+4 > len(p.data) {
		return 0, false
	}
	r, ok := hexVal(p.data[p.pos : p.pos+4])
	p.pos += 4
	return r, ok
}

func (p *parser) number() (any, bool) {
	start := p.pos
	if p.data[p.pos] == '-' {
		p.pos++
	}
	digits := func() int {
		n := 0
		for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			p.pos++
			n++
		}
		return n
	}
	if p.pos >= len(p.data) {
		return nil, false
	}
	if p.data[p.pos] == '0' {
		p.pos++
	} else if digits() == 0 {
		return nil, false
	}
	if p.pos < len(p.data) && p.data[p.pos] == '.' {
		p.pos++
		if digits() == 0 {
			return nil, false
		}
	}
	if p.pos < len(p.data) && (p.data[p.pos] == 'e' || p.data[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.data) && (p.data[p.pos] == '+' || p.data[p.pos] == '-') {
			p.pos++
		}
		if digits() == 0 {
			return nil, false
		}
	}
	f, err := strconv.ParseFloat(string(p.data[start:p.pos]), 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) || !numberInRange(f) {
		p.badNumber = true
		return float64(0), true
	}
	return f, true
}

// numberInRange applies the bean integer rule: a number that canonicalizes to
// an integer literal must be within -(2^53-1) to 2^53-1.
func numberInRange(f float64) bool {
	if f == math.Trunc(f) && math.Abs(f) < 1e21 {
		return math.Abs(f) <= maxSafeInt
	}
	return true
}

// Canonical returns the RFC 8785 canonical bytes of a value from Parse.
func Canonical(v any) []byte {
	var b []byte
	return appendCanonical(b, v)
}

func appendCanonical(b []byte, v any) []byte {
	switch x := v.(type) {
	case nil:
		return append(b, "null"...)
	case bool:
		if x {
			return append(b, "true"...)
		}
		return append(b, "false"...)
	case float64:
		return append(b, formatNumber(x)...)
	case int:
		return append(b, formatNumber(float64(x))...)
	case string:
		return appendString(b, x)
	case []any:
		b = append(b, '[')
		for i, e := range x {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendCanonical(b, e)
		}
		return append(b, ']')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return lessUTF16(keys[i], keys[j]) })
		b = append(b, '{')
		for i, k := range keys {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendString(b, k)
			b = append(b, ':')
			b = appendCanonical(b, x[k])
		}
		return append(b, '}')
	}
	panic("canonical: unsupported type")
}

func lessUTF16(a, b string) bool {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}

func appendString(b []byte, s string) []byte {
	b = append(b, '"')
	for _, r := range s {
		switch r {
		case '"':
			b = append(b, '\\', '"')
		case '\\':
			b = append(b, '\\', '\\')
		case '\b':
			b = append(b, '\\', 'b')
		case '\t':
			b = append(b, '\\', 't')
		case '\n':
			b = append(b, '\\', 'n')
		case '\f':
			b = append(b, '\\', 'f')
		case '\r':
			b = append(b, '\\', 'r')
		default:
			if r < 0x20 {
				const hx = "0123456789abcdef"
				b = append(b, '\\', 'u', '0', '0', hx[r>>4], hx[r&0xf])
			} else {
				b = utf8.AppendRune(b, r)
			}
		}
	}
	return append(b, '"')
}

// formatNumber implements ECMAScript Number::toString for a finite double
// (RFC 8785 section 3.2.2.3). -0 writes as 0.
func formatNumber(f float64) string {
	if f == 0 {
		return "0"
	}
	var sign string
	if f < 0 {
		sign = "-"
		f = -f
	}
	e := strconv.FormatFloat(f, 'e', -1, 64) // d.ddde±XX
	mant, expS, _ := bytes.Cut([]byte(e), []byte("e"))
	digits := bytes.ReplaceAll(mant, []byte("."), nil)
	exp, _ := strconv.Atoi(string(expS))
	k := len(digits)
	n := exp + 1
	var out []byte
	switch {
	case k <= n && n <= 21:
		out = append(digits, bytes.Repeat([]byte("0"), n-k)...)
	case 0 < n && n <= 21:
		out = append(append(append([]byte{}, digits[:n]...), '.'), digits[n:]...)
	case -6 < n && n <= 0:
		out = append(append([]byte("0."), bytes.Repeat([]byte("0"), -n)...), digits...)
	default:
		out = append(out, digits[0])
		if k > 1 {
			out = append(append(out, '.'), digits[1:]...)
		}
		out = append(out, 'e')
		if n-1 >= 0 {
			out = append(out, '+')
		}
		out = strconv.AppendInt(out, int64(n-1), 10)
	}
	return sign + string(out)
}
