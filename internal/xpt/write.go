package xpt

import (
	"bytes"
	"errors"
	"fmt"
	"math"
)

// Write makes a transport file of numeric variables. It exists to build
// synthetic test files; a missing value is NaN. created is the time in the
// form 01JAN26:00:00:00.
func Write(dataset, created string, names, labels []string, rows [][]float64) ([]byte, error) {
	if len(dataset) < 1 || len(dataset) > 8 || len(created) != 16 || len(names) < 1 || len(names) != len(labels) {
		return nil, errors.New("bad data set name, time or variable list")
	}
	var b bytes.Buffer
	pad := func() {
		for b.Len()%recordLen != 0 {
			b.WriteByte(' ')
		}
	}
	stamp := func(kind, name string) {
		fmt.Fprintf(&b, "%-8s%-8s%-8s%-8s%-8s%-24s%s", "SAS", name, kind, "9.4", "SYNTH", "", created)
		fmt.Fprintf(&b, "%-80s", created)
	}
	b.WriteString(libraryHeader)
	stamp("SASLIB", "SAS")
	b.WriteString(memberPrefix + "000000000000000001600000000140  ")
	b.WriteString(dscrptrHeader)
	stamp("SASDATA", dataset)
	fmt.Fprintf(&b, "%s%04d%s", namestrPrefix, len(names), "00000000000000000000  ")
	for i, name := range names {
		if len(name) < 1 || len(name) > 8 || len(labels[i]) > 40 {
			return nil, errors.New("bad variable name or label")
		}
		n := make([]byte, namestrLen)
		n[1], n[5], n[7] = 1, 8, byte(i+1) // numeric, 8 bytes, variable number
		copy(n[8:16], fmt.Sprintf("%-8s", name))
		copy(n[16:56], fmt.Sprintf("%-40s", labels[i]))
		copy(n[56:64], "        ")
		copy(n[72:80], "        ")
		pos := i * 8
		n[84], n[85], n[86], n[87] = byte(pos>>24), byte(pos>>16), byte(pos>>8), byte(pos)
		b.Write(n)
	}
	pad()
	b.WriteString(obsHeader)
	for _, row := range rows {
		if len(row) != len(names) {
			return nil, errors.New("a row with the wrong number of values")
		}
		for _, x := range row {
			v, err := toIBM(x)
			if err != nil {
				return nil, err
			}
			b.Write(v[:])
		}
	}
	pad()
	return b.Bytes(), nil
}

// toIBM writes a double as an IBM System/360 double, exactly or not at all.
func toIBM(x float64) ([8]byte, error) {
	var out [8]byte
	switch {
	case math.IsNaN(x):
		out[0] = '.'
		return out, nil
	case math.IsInf(x, 0):
		return out, errors.New("not a finite number")
	case x == 0:
		return out, nil
	}
	// |x| = fr * 2^e with 0.5 <= fr < 1, and = F * 16^exp16 with
	// 1/16 <= F < 1. So exp16 is e/4 rounded up, and F has 0 to 3 leading
	// zero bits. The 53 bits of fr always fit the 56 bits of F.
	fr, e := math.Frexp(math.Abs(x))
	m := uint64(math.Ldexp(fr, 53))
	exp16 := (e + 3) >> 2
	zeros := 4*exp16 - e
	frac := m << (3 - zeros)
	if exp16+64 < 0 || exp16+64 > 127 {
		return out, errors.New("the number does not fit an IBM double")
	}
	out[0] = byte(exp16 + 64)
	if x < 0 {
		out[0] |= 0x80
	}
	for i := 7; i >= 1; i-- {
		out[i] = byte(frac)
		frac >>= 8
	}
	return out, nil
}
