// Command narrowscan lists the records of a bean folder that the
// narrowed rules N1 to N4 (SPEC.md Appendix) make invalid. It only reads:
// it opens the files in objects/ and local/hidden/ and does not take the
// folder lock, write a cache or move a file. It prints the path, the id
// and the rule of each hit, never the content. Exit 0: nothing changes.
// Exit 1: hits. Exit 2: the folder cannot be read.
//
//	go run ./tools/narrowscan <folder>
package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Conscious-Repository/bean/core"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: narrowscan <folder>")
		os.Exit(2)
	}
	hits, n, err := scan(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "narrowscan:", err)
		os.Exit(2)
	}
	for _, h := range hits {
		fmt.Println(h)
	}
	fmt.Printf("scanned %d files, %d hits\n", n, len(hits))
	if len(hits) > 0 {
		os.Exit(1)
	}
}

// scan returns one line per hit and the number of files read.
func scan(dir string) ([]string, int, error) {
	var paths []string
	for _, sub := range []string{"objects", "local/hidden"} {
		root := filepath.Join(dir, sub)
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) && p == root {
					return nil
				}
				return err
			}
			if d.Type().IsRegular() && strings.HasSuffix(p, ".json") {
				paths = append(paths, p)
			}
			return nil
		})
		if err != nil {
			return nil, 0, err
		}
	}
	sort.Strings(paths)
	var hits []string
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, 0, err
		}
		rel, _ := filepath.Rel(dir, p)
		for _, rule := range check(b) {
			hits = append(hits, rel+"\t"+rule)
		}
	}
	return hits, len(paths), nil
}

// check returns the narrowed rules that the file breaks and the old rules
// allowed. A file that the old rules already reject is not listed.
func check(b []byte) []string {
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	str := func(v any) string { s, _ := v.(string); return s }
	kid, typ := str(m["kid"]), str(m["type"])
	var out []string
	// N3: the canonical check now comes before the version check.
	if v, ok := m["v"].(float64); ok && v != 0 && string(core.Canonical(m)) != string(b) {
		out = append(out, "N3 unknown v and not canonical")
	}
	// N2: Ed25519 keys and signatures.
	for _, k := range []struct{ name, key string }{{"kid", kid}, {"author", str(m["author"])}, {"value.key", valueKey(m)}} {
		if oldEdKey(k.key) {
			if _, err := core.ParseKey(k.key); err != nil {
				out = append(out, "N2 "+k.name+" is not a strict Ed25519 key")
			}
		}
	}
	if oldEdKey(kid) && oldEdSig(kid, str(m["id"]), str(m["sig"])) {
		if core.Verify(kid, str(m["id"]), str(m["sig"])) != nil {
			if _, err := core.ParseKey(kid); err == nil {
				out = append(out, "N2 sig has a small-order R")
			}
		}
	}
	// N1: times.
	if t, ok := m["times"].(map[string]any); ok {
		if s := str(t["imported"]); oldTime(s) && len(s) == 20 && !newTime(s) {
			out = append(out, "N1 times.imported")
		}
		if o, ok := t["observed"].(map[string]any); ok {
			st, et := str(o["start"]), str(o["end"])
			if oldTime(st) && !newTime(st) {
				out = append(out, "N1 times.observed.start")
			}
			if oldTime(et) && !newTime(et) {
				out = append(out, "N1 times.observed.end")
			}
		}
	}
	// N4: a signing key that is also a recovery key.
	val, _ := m["value"].(map[string]any)
	if vk, rec := valueKey(m), str(val["recovery"]); vk != "" && rec != "" {
		switch typ {
		case "bean.genesis":
			if core.Commitment(vk) == rec {
				out = append(out, "N4 genesis key is its recovery key")
			}
		case "bean.succession":
			if vk == kid || core.Commitment(vk) == rec {
				out = append(out, "N4 succession key is a recovery key")
			}
		}
	}
	return out
}

func valueKey(m map[string]any) string {
	v, _ := m["value"].(map[string]any)
	s, _ := v["key"].(string)
	return s
}

// oldEdKey is the old key rule: 64 lowercase hex digits.
func oldEdKey(k string) bool {
	hx, ok := strings.CutPrefix(k, "ed25519:")
	if !ok || len(hx) != 64 || strings.ToLower(hx) != hx {
		return false
	}
	_, err := hex.DecodeString(hx)
	return err == nil
}

// oldEdSig is the old signature rule: crypto/ed25519.Verify.
func oldEdSig(kid, id, sig string) bool {
	k, _ := hex.DecodeString(kid[8:])
	hx, ok := strings.CutPrefix(sig, "ed25519:")
	s, err := hex.DecodeString(hx)
	if !ok || err != nil || len(s) != 64 || !strings.HasPrefix(id, "sha256:") {
		return false
	}
	msg, err := hex.DecodeString(id[7:])
	return err == nil && ed25519.Verify(k, msg, s)
}

// oldTime is the old time rule: Go's time.Parse with RFC3339Nano.
func oldTime(s string) bool {
	_, err := time.Parse(time.RFC3339Nano, s)
	return s != "" && err == nil
}

func newTime(s string) bool {
	_, ok := core.ParseTime(s)
	return ok
}
