package main

import (
	"bytes"
	"strings"
	"testing"
)

func a13(t *testing.T, in string) (string, error) {
	var out bytes.Buffer
	o := r12opts(true)
	o.digest = strings.Repeat("a", 64)
	err := convert(strings.NewReader(in), &out, o)
	return out.String(), err
}

func TestA13CSVProbe(t *testing.T) {
	for _, in := range []string{
		"\xef\xbb\xbfid,value,note\r\n1,5,\"a\r\nb\"\r\n 1 ,6,x\r\n",
		"id,value,note\n1,5,\"a\nb\"\n2,6\n",
		"id,value,time\n1,5,\"x\ny\"\n2,6,2026-13-01T00:00:00Z\n",
		"\xef\xbb\xbfValue, value\n5,6\n",
		"value\n.5\n5.\n-0\n+1e+2\n0.1\n",
		"id,value\n7,5\n 7 ,6\n",
	} {
		out, err := a13(t, in)
		t.Logf("%q\n  err=%v\n  out=%s", in, err, out)
	}
}
