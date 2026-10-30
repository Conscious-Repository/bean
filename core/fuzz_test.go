package core

import (
	"strings"
	"testing"
)

// FuzzCanonical: Parse never panics; Canonical of a parsed value parses
// again to the same canonical bytes.
func FuzzCanonical(f *testing.F) {
	f.Add([]byte(`{"a":[1,2.5e10,"\ud83d\ude00",null,true]}`))
	f.Add([]byte(`"\udc00"`))
	f.Add([]byte(`{"b":1,"a":{"c":-0}}`))
	f.Add([]byte(`1e400`))
	f.Fuzz(func(t *testing.T, b []byte) {
		v, err := Parse(b)
		if err != nil {
			return
		}
		c := Canonical(v)
		v2, err := Parse(c)
		if err != nil {
			t.Fatalf("canonical not parseable: %q -> %q: %v", b, c, err)
		}
		if string(Canonical(v2)) != string(c) {
			t.Fatalf("not idempotent: %q", b)
		}
	})
}

// FuzzCheckFile: CheckFile never panics, and a file it accepts has the
// id of its path and is canonical.
func FuzzCheckFile(f *testing.F) {
	sg := NewEd25519Signer([]byte(strings.Repeat("f", 32)))
	g := NewGenesis(sg.Public(), Commitment(sg.Public()), "fuzz@1", "2026-10-30T00:00:00Z")
	gid, gb, _ := SignRecord(g, sg)
	f.Add(PathOf(gid), gb)
	f.Add("objects/00/"+strings.Repeat("0", 62)+".json", []byte("{"))
	f.Add(PathOf(gid), []byte(strings.Replace(string(gb), "fuzz@1", "fuzz@2", 1)))
	f.Fuzz(func(t *testing.T, path string, data []byte) {
		fr := CheckFile(path, data)
		if fr.Err != nil {
			return
		}
		if PathOf(fr.Record.ID) != path {
			t.Fatalf("accepted %q for id %s", path, fr.Record.ID)
		}
		v, err := Parse(data)
		if err != nil || string(Canonical(v)) != string(data) {
			t.Fatalf("accepted a non-canonical file")
		}
	})
}

// FuzzVerifyFiles: the verifier never panics on any set of files, and its
// result does not depend on map order.
func FuzzVerifyFiles(f *testing.F) {
	l := newTestLog()
	i := l.newIdentity(1)
	c := l.claim(i, "hrv.rmssd", 40, "")
	l.tombstone(i, c)
	var seed []byte
	for p, b := range l.files {
		seed = append(seed, []byte(p+"\n")...)
		seed = append(seed, b...)
		seed = append(seed, '\n')
	}
	f.Add(seed)
	f.Fuzz(func(t *testing.T, blob []byte) {
		lines := strings.Split(string(blob), "\n")
		files := map[string][]byte{}
		for k := 0; k+1 < len(lines); k += 2 {
			files[lines[k]] = []byte(lines[k+1])
		}
		a := strings.Join(VerifyFiles(files).Lines(), "\n")
		b := strings.Join(VerifyFiles(files).Lines(), "\n")
		if a != b {
			t.Fatal("verify output not stable")
		}
	})
}
