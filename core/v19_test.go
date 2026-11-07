package core

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
)

// V19 (SPEC.md 14): the authority rules as files and expected reports. A second
// implementation can run the same file.
func TestVectorV19(t *testing.T) {
	data, err := os.ReadFile("../vectors/v19-authority.json")
	if err != nil {
		t.Fatal(err)
	}
	var scenarios []struct {
		Name   string
		Files  map[string]string
		Expect struct {
			TooNew      []string          `json:"too_new"`
			Compromised []string          `json:"compromised"`
			Valid       []string          `json:"valid"`
			Errors      map[string]string `json:"errors"`
			Unattested  []string          `json:"unattested"`
			Missing     []string          `json:"missing"`
			Duplicates  map[string]string `json:"duplicates"`
		}
	}
	if err := json.Unmarshal(data, &scenarios); err != nil {
		t.Fatal(err)
	}
	if len(scenarios) == 0 {
		t.Fatal("no scenarios")
	}
	sameSet := func(got map[string]bool, want []string) bool {
		var g []string
		for k, v := range got {
			if v {
				g = append(g, k)
			}
		}
		sort.Strings(g)
		w := append([]string{}, want...)
		sort.Strings(w)
		if len(g) != len(w) {
			return false
		}
		for i := range g {
			if g[i] != w[i] {
				return false
			}
		}
		return true
	}
	sameMap := func(a, b map[string]string) bool {
		if len(a) != len(b) {
			return false
		}
		for k, v := range a {
			if b[k] != v {
				return false
			}
		}
		return true
	}
	for _, sc := range scenarios {
		t.Run(sc.Name, func(t *testing.T) {
			files := map[string][]byte{}
			for p, b := range sc.Files {
				files[p] = []byte(b)
			}
			rp := VerifyFiles(files)
			if len(rp.TooNew) != len(sc.Expect.TooNew) || len(rp.Compromised) != len(sc.Expect.Compromised) {
				t.Errorf("too new %v compromised %v", rp.TooNew, rp.Compromised)
			}
			if !sameSet(rp.Valid, sc.Expect.Valid) {
				t.Errorf("valid %v want %v", rp.Valid, sc.Expect.Valid)
			}
			if !sameMap(rp.Errors, sc.Expect.Errors) {
				t.Errorf("errors %v want %v", rp.Errors, sc.Expect.Errors)
			}
			if !sameSet(rp.Unattested, sc.Expect.Unattested) {
				t.Errorf("unattested %v want %v", rp.Unattested, sc.Expect.Unattested)
			}
			got := map[string]bool{}
			for _, m := range rp.MissingList() {
				got[m] = true
			}
			if !sameSet(got, sc.Expect.Missing) {
				t.Errorf("missing %v want %v", rp.MissingList(), sc.Expect.Missing)
			}
			if !sameMap(rp.Duplicates, sc.Expect.Duplicates) {
				t.Errorf("duplicates %v want %v", rp.Duplicates, sc.Expect.Duplicates)
			}
		})
	}
}
