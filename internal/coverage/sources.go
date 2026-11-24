// Package coverage replays current lab readings against explicitly pinned local sources.
package coverage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/pack"
)

type Pin struct {
	SHA256 string `json:"sha256"`
	Review string `json:"review,omitempty"`
}

type Selection struct {
	Format string `json:"format"`
	Packs  []Pin  `json:"packs"`
	Guides []Pin  `json:"guides"`
}

type survey struct {
	Pin
	pack *pack.Pack
}

type entry struct {
	Provider string   `json:"provider"`
	Tests    []string `json:"test_labels"`
	Panels   []string `json:"panels"`
	Title    string   `json:"title"`
	URL      string   `json:"url"`
	Limit    string   `json:"interpretation_limit"`
}

type guide struct {
	Pin
	Format    string  `json:"format"`
	Publisher string  `json:"publisher"`
	Entries   []entry `json:"entries"`
}

type Sources struct {
	Selection Selection
	packs     []survey
	guides    []guide
}

func readLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, limit)
	}
	return b, err
}

func webURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}

func strict(data []byte, dst any, closed bool) error {
	if _, err := core.Parse(data); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if closed {
		dec.DisallowUnknownFields()
	}
	return dec.Decode(dst)
}

// Load verifies every selected digest before a caller can use any result.
// Each file in the asset folder is named by its lowercase SHA-256.
func Load(path, assets string) (*Sources, error) {
	raw, err := readLimited(path, 1<<20)
	if err != nil {
		return nil, err
	}
	s := new(Sources)
	if err := strict(raw, &s.Selection, true); err != nil {
		return nil, fmt.Errorf("source selection: %w", err)
	}
	sel := s.Selection
	if sel.Format != "bean-coverage-sources/1" {
		return nil, fmt.Errorf("unsupported source selection %q", sel.Format)
	}
	if sel.Packs == nil || sel.Guides == nil {
		return nil, fmt.Errorf("source selection requires packs and guides arrays")
	}
	seen := map[string]bool{}
	load := func(pin Pin, max int64) ([]byte, error) {
		if !pack.IsDigest(pin.SHA256) || seen[pin.SHA256] {
			return nil, fmt.Errorf("invalid or duplicate source digest %q", pin.SHA256)
		}
		if pin.Review != "" && !webURL(pin.Review) {
			return nil, fmt.Errorf("invalid review URL for %s", pin.SHA256)
		}
		seen[pin.SHA256] = true
		data, err := readLimited(filepath.Join(assets, pin.SHA256), max)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != pin.SHA256 {
			return nil, fmt.Errorf("checksum mismatch for %s", pin.SHA256)
		}
		return data, nil
	}
	for _, pin := range sel.Packs {
		data, err := load(pin, pack.MaxBytes)
		if err != nil {
			return nil, err
		}
		p, err := pack.Read(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", pin.SHA256, err)
		}
		s.packs = append(s.packs, survey{pin, p})
	}
	for _, pin := range sel.Guides {
		data, err := load(pin, 8<<20)
		if err != nil {
			return nil, err
		}
		var g guide
		if err := strict(data, &g, false); err != nil {
			return nil, fmt.Errorf("%s: %w", pin.SHA256, err)
		}
		if g.Format != "stele-assay-sources/1" || g.Publisher == "" || len(g.Entries) == 0 {
			return nil, fmt.Errorf("unsupported or empty assay guide %s", pin.SHA256)
		}
		for _, e := range g.Entries {
			if e.Provider == "" || len(e.Tests) == 0 || e.Panels == nil || e.Limit == "" || e.Title == "" || !webURL(e.URL) {
				return nil, fmt.Errorf("invalid assay entry in %s", pin.SHA256)
			}
			for _, test := range e.Tests {
				if test == "" {
					return nil, fmt.Errorf("empty assay test in %s", pin.SHA256)
				}
			}
		}
		g.Pin = pin
		s.guides = append(s.guides, g)
	}
	return s, nil
}

func (s *Sources) CheckCohort(name string) error {
	if len(s.packs) == 0 {
		return nil
	}
	if name == "" {
		return fmt.Errorf("choose a cohort with -cohort")
	}
	for _, source := range s.packs {
		if _, ok := source.pack.Cohorts[name]; ok {
			return nil
		}
	}
	return fmt.Errorf("no selected pack defines cohort %q", name)
}
