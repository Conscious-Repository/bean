package xpt

import (
	"bytes"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/internal/xpt/xpttest"
)

const created = "01JAN26:00:00:00"

func sample(t *testing.T) []byte {
	t.Helper()
	data, err := xpttest.Write("TEST_J", created, []string{"SEQN", "X"}, []string{"Sequence number", "A value (mg/dL)"},
		[][]float64{{1, 157}, {2, math.NaN()}, {3, 4.06}, {4, -0.1}, {5, 0}, {6, 13481.04209}, {7, 1e-60}, {8, 1e70}})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRoundTrip(t *testing.T) {
	f, err := Parse(sample(t))
	if err != nil {
		t.Fatal(err)
	}
	if f.Dataset != "TEST_J" || f.Created != created || f.Rows != 8 || len(f.Variables) != 2 || f.Variables[1].Label != "A value (mg/dL)" {
		t.Fatalf("%+v", f)
	}
	x, err := f.Column("X")
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{157, math.NaN(), 4.06, -0.1, 0, 13481.04209, 1e-60, 1e70}
	for i := range want {
		if math.Float64bits(x[i]) != math.Float64bits(want[i]) && !(math.IsNaN(x[i]) && math.IsNaN(want[i])) {
			t.Errorf("row %d: got %v, want %v", i+1, x[i], want[i])
		}
	}
	if _, err := f.Column("Y"); err == nil {
		t.Error("an unknown variable was read")
	}
}

// The bytes of 157, 148 and 4.06 are the first values of a published CDC
// file's LBXTC and LBDTCSI columns: public data, checked against pandas.
func TestKnownBytes(t *testing.T) {
	for _, c := range []struct {
		b    []byte
		want float64
	}{
		{[]byte{0x42, 0x9d, 0, 0, 0, 0, 0, 0}, 157},
		{[]byte{0x42, 0x94, 0, 0, 0, 0, 0, 0}, 148},
		{[]byte{0x41, 0x40, 0xf5, 0xc2, 0x8f, 0x5c, 0x28, 0xf4}, 4.06},
		{[]byte{0x45, 0x16, 0xe0, 0x90, 0, 0, 0, 0}, 93705},
		{[]byte{0xc1, 0x10, 0, 0, 0, 0, 0, 0}, -1},
	} {
		got, err := ibmFloat(c.b)
		if err != nil || got != c.want {
			t.Errorf("% x: got %v %v, want %v", c.b, got, err, c.want)
		}
		back, err := xpttest.ToIBM(c.want)
		if err != nil || !bytes.Equal(back[:], c.b) {
			t.Errorf("%v: wrote % x, want % x", c.want, back, c.b)
		}
	}
	for _, c := range []byte{'.', '_', 'A', 'Z'} {
		if x, err := ibmFloat([]byte{c, 0, 0, 0, 0, 0, 0, 0}); err != nil || !math.IsNaN(x) {
			t.Errorf("missing %q: %v %v", c, x, err)
		}
	}
}

func TestRefuses(t *testing.T) {
	good := sample(t)
	change := func(f func(b []byte) []byte) []byte { return f(bytes.Clone(good)) }
	obs := bytes.Index(good, []byte(xpttest.ObsHeader)) + 80
	cases := map[string][]byte{
		"empty":                {},
		"not a transport file": []byte(strings.Repeat("x", 4000)),
		"version 8":            change(func(b []byte) []byte { copy(b[20:], "LIBV8  "); return b }),
		"cut in the header":    good[:500],
		"cut inside a record":  good[:len(good)-3],
		"extra blank record":   append(bytes.Clone(good), bytes.Repeat([]byte(" "), 80)...),
		"no variables":         change(func(b []byte) []byte { copy(b[7*80+54:], "0000"); return b }),
		"more variables":       change(func(b []byte) []byte { copy(b[7*80+54:], "0003"); return b }),
		"a 4-byte number":      change(func(b []byte) []byte { b[8*80+5] = 4; return b }),
		"unknown kind":         change(func(b []byte) []byte { b[8*80+1] = 3; return b }),
		"same name twice":      change(func(b []byte) []byte { copy(b[8*80+140+8:], "SEQN    "); return b }),
		"position gap":         change(func(b []byte) []byte { b[8*80+140+87] = 16; return b }),
		"second data set":      change(func(b []byte) []byte { copy(b[obs:], "HEADER RECORD*******"); return b }),
	}
	for name, data := range cases {
		if _, err := Parse(data); err == nil {
			t.Errorf("%s: read", name)
		}
	}
	// Three variables make 24-byte rows, so the last record has bytes that
	// belong to no row: they must be blank. (A file cut at a record
	// boundary reads as a shorter file: the format has no row count. The
	// SHA-256 of the file is what catches that.)
	three, err := xpttest.Write("TEST_J", created, []string{"A", "B", "C"}, []string{"", "", ""}, [][]float64{{1, 2, 3}})
	if err != nil {
		t.Fatal(err)
	}
	if f, err := Parse(three); err != nil || f.Rows != 1 {
		t.Fatalf("%v %v", f, err)
	}
	three[len(three)-1] = 'x'
	if _, err := Parse(three); err == nil {
		t.Error("bad padding: read")
	}
	// A value that is not a number, a missing value or a zero.
	bad := change(func(b []byte) []byte { b[obs+8], b[obs+9] = 0x01, 0; copy(b[obs+10:obs+16], make([]byte, 6)); return b })
	if f, err := Parse(bad); err != nil {
		t.Fatal(err)
	} else if _, err := f.Column("X"); err == nil {
		t.Error("a zero with an exponent was read")
	}
	// What SAS does not write, and pandas reads differently: a fraction
	// that starts with a zero hex digit, and the smallest IBM number, which
	// pandas takes for zero.
	for name, bits := range map[string][]byte{
		"not normalized": {0x43, 0x0b, 0xb0, 0, 0, 0, 0, 0},
		"too small":      {0x00, 0x10, 0, 0, 0, 0, 0, 0},
	} {
		odd := change(func(b []byte) []byte { copy(b[obs+8:], bits); return b })
		if f, err := Parse(odd); err != nil {
			t.Fatal(err)
		} else if _, err := f.Column("X"); err == nil {
			t.Errorf("%s: read", name)
		}
	}
	// 56 significant bits do not fit a double.
	wide := change(func(b []byte) []byte {
		copy(b[obs+8:], []byte{0x41, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
		return b
	})
	if f, err := Parse(wide); err != nil {
		t.Fatal(err)
	} else if _, err := f.Column("X"); err == nil {
		t.Error("a number that a double cannot hold was read")
	}
}

// A character variable is kept in the row width but cannot be read as a
// number.
func TestCharacterVariable(t *testing.T) {
	data := sample(t)
	data[8*80+140+1] = 2 // X becomes a character variable of 8 bytes
	f, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Column("X"); err == nil {
		t.Error("a character variable was read as a number")
	}
	if x, err := f.Column("SEQN"); err != nil || x[7] != 8 {
		t.Errorf("%v %v", x, err)
	}
}

// With BEAN_NHANES_DIR set to a folder of CDC's files, every one parses.
func TestRealFiles(t *testing.T) {
	dir := os.Getenv("BEAN_NHANES_DIR")
	if dir == "" {
		t.Skip("BEAN_NHANES_DIR is not set")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(dir + "/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		f, err := Parse(data)
		if err != nil {
			t.Errorf("%s: %v", e.Name(), err)
			continue
		}
		for _, v := range f.Variables {
			if _, err := f.Column(v.Name); v.Numeric && err != nil {
				t.Errorf("%s: %v", e.Name(), err)
			}
		}
	}
}

func FuzzParse(f *testing.F) {
	data, _ := xpttest.Write("TEST_J", created, []string{"SEQN", "X"}, []string{"a", "b"}, [][]float64{{1, 2}, {3, math.NaN()}})
	f.Add(data)
	f.Fuzz(func(t *testing.T, data []byte) {
		x, err := Parse(data)
		if err != nil {
			return
		}
		for _, v := range x.Variables {
			if col, err := x.Column(v.Name); err == nil && len(col) != x.Rows {
				t.Fatal("a column with the wrong number of rows")
			}
		}
	})
}
