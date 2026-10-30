package core

import (
	"bytes"
	"sort"
	"strings"
	"testing"
)

// FuzzSealValue: any value in a recovery-signed bean.seal either fails
// CheckFile or has a part in 1..MaxSealParts and a non-empty, sorted,
// unique list of ids.
func FuzzSealValue(f *testing.F) {
	rec := NewEd25519Signer([]byte(strings.Repeat("r", 32)))
	id := "sha256:" + strings.Repeat("1", 64)
	sid := "sha256:" + strings.Repeat("2", 64)
	f.Add([]byte(`{"part":1,"sealed":["sha256:` + strings.Repeat("3", 64) + `"]}`))
	f.Add([]byte(`{"part":1000,"sealed":["sha256:` + strings.Repeat("3", 64) + `","sha256:` + strings.Repeat("4", 64) + `"]}`))
	f.Add([]byte(`{"part":0,"sealed":[]}`))
	f.Add([]byte(`{"part":1.0e0,"sealed":["x"],"parts":2}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		v, err := Parse(raw)
		if err != nil {
			return
		}
		m := NewSeal(id, rec.Public(), sid, 1, []string{sid}, "fuzz@1", "2026-10-30T00:00:00Z")
		m["value"] = v
		pid, data, err := SignRecord(m, rec)
		if err != nil {
			return
		}
		fr := CheckFile(PathOf(pid), data)
		if fr.Err != nil {
			return
		}
		r := fr.Record
		if r.Part() < 1 || r.Part() > MaxSealParts {
			t.Fatalf("accepted part %v", r.Value["part"])
		}
		ids := r.Sealed()
		if len(ids) == 0 || !sort.StringsAreSorted(ids) {
			t.Fatalf("accepted sealed %v", ids)
		}
		for k := 1; k < len(ids); k++ {
			if ids[k] == ids[k-1] {
				t.Fatal("accepted a repeated id")
			}
		}
	})
}

// FuzzSealSets: random parts (number, signer, ids) around one succession
// with value.parts = n. The verifier must agree with a direct model of
// 5.4: a claim of the old key is valid iff the set is complete and holds it.
func FuzzSealSets(f *testing.F) {
	f.Add(uint8(2), []byte{1, 0x0f, 0, 2, 0xf0, 0})
	f.Add(uint8(2), []byte{1, 0x0f, 0, 1, 0xf0, 0})
	f.Add(uint8(1), []byte{1, 0xff, 1})
	f.Add(uint8(0), []byte{})
	f.Add(uint8(3), []byte{3, 1, 0, 2, 2, 2, 1, 4, 0})
	f.Fuzz(func(t *testing.T, n uint8, spec []byte) {
		n %= 5
		if len(spec) > 3*12 {
			spec = spec[:3*12]
		}
		l := newTestLog()
		i := l.newIdentity(7)
		var claims []string
		for k := 0; k < 8; k++ {
			claims = append(claims, l.claim(i, "hrv.rmssd", float64(k), ""))
		}
		sorted := append([]string(nil), claims...)
		sort.Strings(sorted)
		rec := RecoverySigner(i.seed, 0)
		nk := NewEd25519Signer(bytes.Repeat([]byte{0x99}, 32))
		s := NewSuccession(i.id, rec.Public(), i.id, nk.Public(), Commitment(RecoverySigner(i.seed, 1).Public()), sorted[:1], "test@0", "2026-10-27T00:00:04Z")
		if n > 0 {
			s["value"].(map[string]any)["parts"] = float64(n)
		}
		s["nonce"] = l.nonce()
		sid := l.put(s, rec)
		other := RecoverySigner(bytes.Repeat([]byte{9}, 32), 0)
		signers := []Signer{rec, i.signer, other, RecoverySigner(i.seed, 1)}
		// Model.
		good := map[int][]map[string]bool{}
		for k := 0; k+2 < len(spec); k += 3 {
			part := int(spec[k]%6) + 1
			var ids []string
			set := map[string]bool{}
			for b := 0; b < 8; b++ {
				if spec[k+1]&(1<<b) != 0 {
					ids = append(ids, sorted[b])
					set[sorted[b]] = true
				}
			}
			if len(ids) == 0 {
				continue
			}
			sg := signers[int(spec[k+2])%len(signers)]
			m := NewSeal(i.id, sg.Public(), sid, part, ids, "test@0", "2026-10-27T00:00:05Z")
			m["nonce"] = l.nonce()
			l.put(m, sg)
			if sg.Public() == rec.Public() && part <= int(n) {
				good[part] = append(good[part], set)
			}
		}
		want := map[string]bool{sorted[0]: true}
		complete := true
		for k := 1; k <= int(n); k++ {
			if len(good[k]) != 1 {
				complete = false
				continue
			}
			for x := range good[k][0] {
				want[x] = true
			}
		}
		rp := l.verify()
		if !rp.Valid[sid] {
			t.Fatalf("succession: %q", rp.Errors[sid])
		}
		if complete != (len(rp.Incomplete) == 0) {
			t.Fatalf("complete %v, incomplete %v", complete, rp.Incomplete)
		}
		for _, c := range claims {
			if got := rp.Valid[c]; got != (complete && want[c]) {
				t.Fatalf("claim %s valid %v, model %v (%q)", c[:16], got, complete && want[c], rp.Errors[c])
			}
		}
		a := strings.Join(rp.Lines(), "\n")
		if b := strings.Join(VerifyFiles(l.files).Lines(), "\n"); a != b {
			t.Fatal("verify output not stable")
		}
	})
}
