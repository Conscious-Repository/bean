// Package xpt reads SAS transport files, version 5 (XPORT), the format in
// which CDC publishes NHANES. It reads one data set of numeric and
// character variables and refuses anything it cannot read for sure.
//
// The layout is from SAS technical note TS-140, "The Record Layout of a
// Data Set in SAS Transport (XPORT) Format": 80-byte records, a library
// header, a member header, one 140-byte NAMESTR per variable, then the
// observations in fixed-width rows, padded with blanks to 80 bytes.
package xpt

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"strconv"
	"strings"
)

const (
	recordLen  = 80
	namestrLen = 140
	// maxVariables and maxRows bound what Parse accepts.
	maxVariables = 2000
	maxRows      = 5_000_000
)

const (
	libraryHeader = "HEADER RECORD*******LIBRARY HEADER RECORD!!!!!!!000000000000000000000000000000  "
	memberPrefix  = "HEADER RECORD*******MEMBER  HEADER RECORD!!!!!!!"
	dscrptrHeader = "HEADER RECORD*******DSCRPTR HEADER RECORD!!!!!!!000000000000000000000000000000  "
	namestrPrefix = "HEADER RECORD*******NAMESTR HEADER RECORD!!!!!!!000000"
	obsHeader     = "HEADER RECORD*******OBS     HEADER RECORD!!!!!!!000000000000000000000000000000  "
)

// Variable is one column of the data set.
type Variable struct {
	Name, Label string
	Numeric     bool
	pos, length int
}

// File is one parsed data set.
type File struct {
	// Dataset is the member name, for example TCHOL_J.
	Dataset   string
	Variables []Variable
	Rows      int
	rowLen    int
	obs       []byte
}

// Parse reads a transport file with one data set.
func Parse(data []byte) (*File, error) {
	rec := func(i int) ([]byte, error) {
		if (i+1)*recordLen > len(data) {
			return nil, errors.New("not a SAS transport file: it ends inside its header")
		}
		return data[i*recordLen : (i+1)*recordLen], nil
	}
	// Record 0: the library header. Records 1 and 2 hold the writer's name
	// and times, which nothing here depends on.
	if r, err := rec(0); err != nil || string(r) != libraryHeader {
		return nil, errors.New("not a SAS transport (XPORT version 5) file")
	}
	// Record 3: the member header. It gives the NAMESTR size.
	r, err := rec(3)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(string(r), memberPrefix) || string(r[74:78]) != "0140" {
		return nil, errors.New("not a transport file this reader knows: bad member header")
	}
	if r, err = rec(4); err != nil || string(r) != dscrptrHeader {
		return nil, errors.New("not a transport file this reader knows: bad descriptor header")
	}
	// Records 5 and 6: the member's name and times.
	if r, err = rec(5); err != nil {
		return nil, err
	}
	f := &File{Dataset: text(r[8:16])}
	if f.Dataset == "" {
		return nil, errors.New("the data set has no name")
	}
	if _, err = rec(6); err != nil {
		return nil, err
	}
	// Record 7: the NAMESTR header, with the number of variables.
	if r, err = rec(7); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(string(r), namestrPrefix) {
		return nil, errors.New("not a transport file this reader knows: bad NAMESTR header")
	}
	nvar, err := strconv.Atoi(string(r[54:58]))
	if err != nil || nvar < 1 || nvar > maxVariables || !isDigits(r[54:58]) {
		return nil, errors.New("bad number of variables")
	}
	off := 8 * recordLen
	end := off + nvar*namestrLen
	if end > len(data) {
		return nil, errors.New("the file ends inside its variable list")
	}
	names := map[string]bool{}
	for i := 0; i < nvar; i++ {
		n := data[off+i*namestrLen : off+(i+1)*namestrLen]
		typ := int(n[0])<<8 | int(n[1])
		v := Variable{
			Name:    text(n[8:16]),
			Label:   text(n[16:56]),
			Numeric: typ == 1,
			length:  int(n[4])<<8 | int(n[5]),
			pos:     int(n[84])<<24 | int(n[85])<<16 | int(n[86])<<8 | int(n[87]),
		}
		switch {
		case typ != 1 && typ != 2:
			return nil, fmt.Errorf("variable %d has an unknown kind", i+1)
		case v.Name == "" || names[v.Name]:
			return nil, fmt.Errorf("variable %d has no name, or a name used twice", i+1)
		case v.Numeric && v.length != 8:
			return nil, fmt.Errorf("variable %s is a number of %d bytes; only 8 is read", v.Name, v.length)
		case v.length < 1:
			return nil, fmt.Errorf("variable %s has no width", v.Name)
		case v.pos != f.rowLen:
			return nil, fmt.Errorf("variable %s does not follow the one before it", v.Name)
		}
		names[v.Name] = true
		f.rowLen += v.length
		f.Variables = append(f.Variables, v)
	}
	// The variable list is padded to a full record.
	off = (end + recordLen - 1) / recordLen * recordLen
	if off+recordLen > len(data) || !blank(data[end:off]) {
		return nil, errors.New("bad padding after the variable list")
	}
	if string(data[off:off+recordLen]) != obsHeader {
		return nil, errors.New("not a transport file this reader knows: bad observation header")
	}
	obs := data[off+recordLen:]
	if len(obs)%recordLen != 0 {
		return nil, errors.New("the file is not a whole number of 80-byte records")
	}
	// The last record is padded with blanks. A row of blanks at the end is
	// padding, not an observation: a transport file cannot tell the two
	// apart, and SAS reads it the same way.
	rows := len(obs) / f.rowLen
	if !blank(obs[rows*f.rowLen:]) {
		return nil, errors.New("bad padding after the observations")
	}
	for rows > 0 && blank(obs[(rows-1)*f.rowLen:rows*f.rowLen]) {
		rows--
	}
	if len(obs)-rows*f.rowLen >= recordLen {
		return nil, errors.New("more blanks at the end than padding needs")
	}
	if rows > maxRows {
		return nil, errors.New("too many rows")
	}
	if bytes.Contains(obs[:rows*f.rowLen], []byte("HEADER RECORD*******")) {
		return nil, errors.New("the file holds more than one data set")
	}
	f.Rows, f.obs = rows, obs[:rows*f.rowLen]
	return f, nil
}

// Variable returns the variable with this name.
func (f *File) Variable(name string) (Variable, bool) {
	for _, v := range f.Variables {
		if v.Name == name {
			return v, true
		}
	}
	return Variable{}, false
}

// Column returns a numeric variable, one value per row in file order. A
// missing value is NaN.
func (f *File) Column(name string) ([]float64, error) {
	v, ok := f.Variable(name)
	if !ok {
		return nil, fmt.Errorf("%s has no variable %s", f.Dataset, name)
	}
	if !v.Numeric {
		return nil, fmt.Errorf("%s in %s is not a number", name, f.Dataset)
	}
	out := make([]float64, f.Rows)
	for i := range out {
		x, err := ibmFloat(f.obs[i*f.rowLen+v.pos : i*f.rowLen+v.pos+8])
		if err != nil {
			return nil, fmt.Errorf("%s in %s, row %d: %w", name, f.Dataset, i+1, err)
		}
		out[i] = x
	}
	return out, nil
}

// ibmFloat reads an IBM System/360 double: a sign bit, a 7-bit exponent of
// 16 in excess 64, and a 56-bit fraction. A first byte of '.', '_' or 'A' to
// 'Z' with the rest zero is a SAS missing value. It refuses what SAS does
// not write and other readers read differently: a number that an IEEE
// double cannot hold exactly, one whose fraction does not start with a
// nonzero hex digit, and one below 1e-70, which some readers take for zero.
func ibmFloat(b []byte) (float64, error) {
	var frac uint64
	for _, c := range b[1:] {
		frac = frac<<8 | uint64(c)
	}
	if frac == 0 {
		switch c := b[0]; {
		case c == 0:
			return 0, nil
		case c == '.' || c == '_' || (c >= 'A' && c <= 'Z'):
			return math.NaN(), nil
		}
		return 0, errors.New("a zero that is not written as zero")
	}
	if bits.Len64(frac)-bits.TrailingZeros64(frac) > 53 {
		return 0, errors.New("a number with more digits than a double holds")
	}
	if frac>>52 == 0 {
		return 0, errors.New("a number that is not normalized")
	}
	exp := 4*(int(b[0]&0x7f)-64) - 56
	x := math.Ldexp(float64(frac), exp)
	if x < 1e-70 {
		return 0, errors.New("a number too small to be a measurement")
	}
	if b[0]&0x80 != 0 {
		x = -x
	}
	return x, nil
}

func text(b []byte) string { return strings.TrimRight(string(b), " \x00") }

func blank(b []byte) bool {
	for _, c := range b {
		if c != ' ' {
			return false
		}
	}
	return true
}

func isDigits(b []byte) bool {
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
