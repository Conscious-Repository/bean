package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The scan finds each narrowed case of the conformance folders, finds
// nothing in a folder that the old rules already judged the same, and
// changes no file.
func TestScanConformance(t *testing.T) {
	want := map[string]string{
		"06-time-offset-24":             "N1",
		"07-time-comma":                 "N1",
		"09-time-year0":                 "N1",
		"09c-time-10digits":             "N1",
		"17-key-is-recovery":            "N4",
		"17b-key-is-next-recovery":      "N4",
		"18-ed-small-order-bare":        "N2",
		"18d-ed-small-order-r":          "N2",
		"19-ed-notpoint-succession-key": "N2",
		"21-unknown-v-noncanonical":     "N3",
		"25-v-float":                    "N3",
		"05-time-lowercase-t":           "",
		"09e-time-good-forms":           "",
		"17c-bare-recovery-key":         "",
	}
	for name, rule := range want {
		dir := filepath.Join("..", "..", "vectors", "conformance", name)
		before := stamps(t, dir)
		hits, _, err := scan(dir)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.Join(hits, "\n")
		if (rule == "") != (len(hits) == 0) || !strings.Contains(got, rule) {
			t.Errorf("%s: %q, want %s", name, got, rule)
		}
		if after := stamps(t, dir); after != before {
			t.Errorf("%s: files changed", name)
		}
	}
}

func stamps(t *testing.T, dir string) string {
	var b strings.Builder
	filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err == nil {
			b.WriteString(p + fi.ModTime().String() + "\n")
		}
		return nil
	})
	return b.String()
}
