package core

import (
	"strconv"
	"testing"
)

func TestSameDecimal(t *testing.T) {
	for raw, want := range map[string]bool{
		"0.1": true, "70": true, "-2.0": true, "1.190": true, "5e2": true,
		"0.30000000000000001": false, "1e-400": false, "123456789.123456789123": false,
		"1e1001": false, string(make([]byte, 257)): false,
	} {
		n, _ := strconv.ParseFloat(raw, 64)
		if got := SameDecimal(raw, n); got != want {
			t.Errorf("SameDecimal(%.30q) = %v, want %v", raw, got, want)
		}
	}
}
