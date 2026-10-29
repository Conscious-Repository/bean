package core

import (
	"bytes"
	"math"
	"testing"
)

// RFC 8785 Appendix B number samples (ES Number.prototype.toString), plus the
// bean-specific range rule from SPEC.md 4.1.
func TestFormatNumberRFC8785(t *testing.T) {
	cases := []struct {
		bits uint64
		want string
	}{
		{0x0000000000000000, "0"},
		{0x8000000000000000, "0"}, // -0 writes as 0
		{0x0000000000000001, "5e-324"},
		{0x7fefffffffffffff, "1.7976931348623157e+308"},
		{0x4340000000000000, "9007199254740992"},
		{0xc340000000000000, "-9007199254740992"},
		{0x4430000000000000, "295147905179352830000"},
		{0x44b52d02c7e14af5, "9.999999999999997e+22"},
		{0x44b52d02c7e14af6, "1e+23"},
		{0x44b52d02c7e14af7, "1.0000000000000001e+23"},
		{0x444b1ae4d6e2ef4e, "999999999999999700000"},
		{0x444b1ae4d6e2ef4f, "999999999999999900000"},
		{0x444b1ae4d6e2ef50, "1e+21"},
		{0x3eb0c6f7a0b5ed8c, "9.999999999999997e-7"},
		{0x3eb0c6f7a0b5ed8d, "0.000001"},
		{0x41b3de4355555553, "333333333.3333332"},
		{0x41b3de4355555554, "333333333.33333325"},
		{0x41b3de4355555555, "333333333.3333333"},
		{0x41b3de4355555556, "333333333.3333334"},
		{0x41b3de4355555557, "333333333.33333343"},
	}
	for _, c := range cases {
		f := math.Float64frombits(c.bits)
		got := formatNumber(f)
		if got != c.want {
			t.Errorf("bits %016x: got %q want %q", c.bits, got, c.want)
		}
	}
}

func TestNumberRange(t *testing.T) {
	// 2^53-1 is the largest allowed integer; 2^53 is rejected (SPEC 4.1).
	if _, err := Parse([]byte("9007199254740991")); err != nil {
		t.Errorf("2^53-1 should parse: %v", err)
	}
	if _, err := Parse([]byte("9007199254740992")); err == nil || err.Error() != "bad number" {
		t.Errorf("2^53 should be bad number, got %v", err)
	}
	if _, err := Parse([]byte("-9007199254740992")); err == nil || err.Error() != "bad number" {
		t.Errorf("-2^53 should be bad number, got %v", err)
	}
	// 1e21 canonicalizes with an exponent, so it is a double, not an integer.
	if _, err := Parse([]byte("1e21")); err != nil {
		t.Errorf("1e21 should parse: %v", err)
	}
	if _, err := Parse([]byte("1e400")); err == nil {
		t.Errorf("1e400 should be bad number")
	}
}

func TestCanonicalKeyOrderUTF16(t *testing.T) {
	// RFC 8785 Appendix B key ordering sample.
	in := []byte(`{"€":"Euro Sign","\r":"Carriage Return","דּ":"Hebrew Letter Dalet With Dagesh","1":"One","😀":"Emoji: Grinning Face","\u0080":"Control","ö":"Latin Small Letter O With Diaeresis"}`)
	want := []byte(`{"\r":"Carriage Return","1":"One","` + "\u0080" + `":"Control","ö":"Latin Small Letter O With Diaeresis","€":"Euro Sign","😀":"Emoji: Grinning Face","דּ":"Hebrew Letter Dalet With Dagesh"}`)
	// Replace the דּ escape in want with the literal character.
	want = bytes.ReplaceAll(want, []byte(`דּ`), []byte("דּ"))
	v, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	got := Canonical(v)
	if !bytes.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCanonicalStrings(t *testing.T) {
	cases := []struct{ in, want string }{
		{`"\u0008\u0009\u000a\u000c\u000d"`, `"\b\t\n\f\r"`},
		{`"\u0001\u001f"`, `"\u0001\u001f"`},
		{`"\/"`, `"/"`},
		{`"\u007f"`, "\"\u007f\""},
		{`"é"`, `"é"`},
		{`"😀"`, `"😀"`},
		{`"a\"b\\c"`, `"a\"b\\c"`},
	}
	for _, c := range cases {
		v, err := Parse([]byte(c.in))
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		if got := string(Canonical(v)); got != c.want {
			t.Errorf("%s: got %q want %q", c.in, got, c.want)
		}
	}
}

func TestParseRejects(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"a":1,"a":2}`, "duplicate key"},
		{`{"a":{"b":1,"b":2}}`, "duplicate key"},
		{`"\ud800"`, "bad string"},
		{`"\udc00"`, "bad string"},
		{`"\ud800x"`, "bad string"},
		{"\"\xff\"", "bad json"},
		{`{"a":1}x`, "bad json"},
		{`[1,]`, "bad json"},
		{`01`, "bad json"},
		{`1.`, "bad json"},
		{`NaN`, "bad json"},
		{`"a` + "\x01" + `"`, "bad json"},
		{``, "bad json"},
	}
	for _, c := range cases {
		_, err := Parse([]byte(c.in))
		if err == nil || err.Error() != c.want {
			t.Errorf("%q: got %v want %q", c.in, err, c.want)
		}
	}
}

func TestCanonicalRoundTrip(t *testing.T) {
	in := []byte(`{"b":[1,2.5,true,null,"x"],"a":{"z":-0,"y":1e-7}}`)
	v, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	c := Canonical(v)
	want := `{"a":{"y":1e-7,"z":0},"b":[1,2.5,true,null,"x"]}`
	if string(c) != want {
		t.Errorf("got %s want %s", c, want)
	}
	v2, err := Parse(c)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(Canonical(v2), c) {
		t.Error("canonical bytes are not stable")
	}
}
