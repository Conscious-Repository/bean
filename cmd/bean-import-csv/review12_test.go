package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func r12opts(blob bool) options {
	o := options{typ: "x.y", system: "csv", unit: "u", precision: "second"}
	if blob {
		o.blob = keepBlob
	}
	return o
}

func r12convert(t *testing.T, in string, blob bool) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := convert(strings.NewReader(in), &out, r12opts(blob)); err != nil {
		t.Fatalf("refused: %v", err)
	}
	var recs []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(err)
		}
		recs = append(recs, m)
	}
	return recs
}

// r12refused checks that convert refuses in, with and without -blob, and
// that the error says want.
func r12refused(t *testing.T, in, want string, blobOnly bool) {
	t.Helper()
	for _, blob := range []bool{true, false} {
		if blobOnly && !blob {
			continue
		}
		var out bytes.Buffer
		err := convert(strings.NewReader(in), &out, r12opts(blob))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("blob=%v: err %v, want %q; output %q", blob, err, want, out.String())
		}
	}
}

// R12-1: digits lost or a nonzero value stored as 0. Under -blob the file is
// refused; standalone, the value stays text.
func TestR12CSVInexactNumber(t *testing.T) {
	srcs := []string{"0.30000000000000001", "1e-400", "9007199254740993.0", "123456789.123456789123", "0.000e-5000"}
	recs := r12convert(t, "value\n"+strings.Join(srcs, "\n")+"\n", false)
	for i, src := range srcs {
		if v := recs[i]["value"].(map[string]any); v["s"] != src {
			t.Errorf("source %q stored as %v", src, v)
		}
	}
	for _, src := range srcs {
		r12refused(t, "value\n1\n"+src+"\n", "line 3: the value is a number that cannot be stored exactly", true)
	}
	// Ordinary decimals and trailing zeros stay numbers.
	recs = r12convert(t, "value\n0.1\n1.50\n+7\n.5\n-2.5e3\n0\n0.0\n", true)
	for i, want := range []float64{0.1, 1.5, 7, 0.5, -2500, 0, 0} {
		if got := recs[i]["value"].(map[string]any)["n"]; got != want {
			t.Errorf("row %d: n=%v, want %v", i+2, got, want)
		}
	}
}

// R12-0: a row with more fields than the header lost the extra fields, so an
// unquoted "1,234" or "72,5" was stored as 1 or 72. A short row is refused too.
func TestR12CSVExtraFields(t *testing.T) {
	r12refused(t, "time,value\n2026-10-20T08:00:00Z,1,234\n", "line 2: 3 fields, but the header has 2", false)
	r12refused(t, "time,value,note\n2026-10-20T08:00:00Z,1,a\n2026-10-21T08:00:00Z,72,5,b\n", "line 3: 4 fields, but the header has 3", false)
	r12refused(t, "time,value,note\n2026-10-20T08:00:00Z,1\n", "line 2: 2 fields, but the header has 3", false)
	// Quoted, the comma is part of the field, and the value is text.
	recs := r12convert(t, "time,value\n2026-10-20T08:00:00Z,\"1,234\"\n", true)
	if v := recs[0]["value"].(map[string]any); v["s"] != "1,234" {
		t.Errorf("quoted 1,234 stored as %v", v)
	}
}

// R12-2: two columns whose names fold to one: the first was dropped in silence.
func TestR12CSVDuplicateHeader(t *testing.T) {
	r12refused(t, "Value,value ,note\n10,20,first\n", `columns 1 and 2 have the same name "value"`, false)
	r12refused(t, "value,note, NOTE\n10,first,second\n", `columns 2 and 3 have the same name "note"`, false)
}

// R12-3: a column named raw_time overwrote the unread time.
func TestR12CSVRawTimeColumn(t *testing.T) {
	r12refused(t, "time,value,raw_time\n2026-10-20 08:00,5,from-device\n", "a column is named raw_time", false)
}

// R12-4: a native id repeated in one file got occurrence 0 twice (SPEC 4.8),
// so Append dropped the second reading as already there.
func TestR12CSVRepeatedID(t *testing.T) {
	r12refused(t, "time,value,id\n2026-10-20T08:00:00Z,5,7\n2026-10-21T08:00:00Z,6,7\n", "line 3: its id is the id of line 2", false)
	// A quoted field over two lines: the error names the line the row starts on.
	r12refused(t, "value,id,note\n5,7,\"a\nb\"\n6, 7 ,c\n", "line 4: its id is the id of line 2", false)
}

// R12-7: under -blob a time that does not parse is refused: the ref ignores
// context.raw_time, so a re-export with one more day stored the wrong day.
// Standalone it goes to context.raw_time, as before.
func TestR12CSVUnreadTime(t *testing.T) {
	r12refused(t, "time,value\n2026-10-20T08:00:00Z,70\n2026-10-01,70\n", "line 3: the time is not RFC 3339", true)
	recs := r12convert(t, "time,value\n2026-10-01,70\n,71\n", false)
	if recs[0]["context"].(map[string]any)["raw_time"] != "2026-10-01" {
		t.Errorf("standalone: %v", recs[0])
	}
	// An empty time is no time, not an unread one.
	recs = r12convert(t, "time,value\n,71\n", true)
	if recs[0]["times"].(map[string]any)["observed"] != nil {
		t.Errorf("empty time: %v", recs[0])
	}
}

// R12-5: the work per row was the whole header, even for a row of one byte,
// and bean import runs two passes before any output. Now a row has as many
// fields as the header (a short row is refused at once), and a row's work is
// its own fields, so the cost per input byte does not grow with the header.
func TestR12CSVWideHeaderWork(t *testing.T) {
	var short strings.Builder
	short.WriteString("value")
	for i := 0; i < 10000; i++ {
		fmt.Fprintf(&short, ",c%d", i)
	}
	short.WriteString("\n1\n")
	r12refused(t, short.String(), "line 2: 1 fields, but the header has 10001", false)

	perByte := map[int]float64{}
	for _, cols := range []int{10, 10000} {
		var b strings.Builder
		b.WriteString("value")
		for i := 0; i < cols; i++ {
			fmt.Fprintf(&b, ",c%d", i)
		}
		b.WriteString("\n")
		row := "1" + strings.Repeat(",", cols) + "\n"
		for b.Len() < 2<<20 {
			b.WriteString(row)
		}
		in := b.String()
		start := time.Now()
		if err := convert(strings.NewReader(in), io.Discard, options{typ: "x.y", system: "csv"}); err != nil {
			t.Fatal(err)
		}
		el := time.Since(start)
		perByte[cols] = float64(el.Nanoseconds()) / float64(len(in))
		t.Logf("cols=%d bytes=%d took %v (%.1f ns/byte)", cols, len(in), el, perByte[cols])
	}
	// Linear: a wide header costs no more per byte than a narrow one, within
	// a wide margin for noise. Before, 10000 columns cost thousands of times more.
	if perByte[10000] > 10*perByte[10] {
		t.Errorf("per-byte cost grows with the header: %.1f vs %.1f ns/byte", perByte[10000], perByte[10])
	}
}
