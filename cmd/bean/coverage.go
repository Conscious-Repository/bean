package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/coverage"
	"github.com/Conscious-Repository/bean/internal/pack"
)

type coverageReport struct {
	Format        string             `json:"format"`
	Cohort        string             `json:"cohort"`
	Selection     coverage.Selection `json:"selection"`
	Total         int                `json:"total"`
	Counts        map[string]int     `json:"counts"`
	SourceCovered int                `json:"source_covered"`
	Rows          []coverage.Row     `json:"rows"`
}

func cmdCoverage(s *core.Store, args []string) {
	fs := flag.NewFlagSet("coverage", flag.ContinueOnError)
	path := fs.String("sources", "", "local source selection")
	assets := fs.String("assets", "", "folder of digest-named files")
	cohort := fs.String("cohort", "", "cohort for this replay")
	report := fs.String("report", "", "optional original lab report SHA-256")
	machine := fs.Bool("json", false, "write machine-readable replay")
	pos, err := parseInterspersed(fs, args)
	if err != nil || len(pos) > 0 || *path == "" {
		usage()
	}
	if *report != "" && !pack.IsDigest(*report) {
		badArg(errors.New("-report takes a lowercase original report SHA-256"))
	}
	if *assets == "" {
		*assets = filepath.Dir(*path)
	}
	sources, err := coverage.Load(*path, *assets)
	if err != nil {
		fail(err)
	}
	if err := sources.CheckCohort(*cohort); err != nil {
		badArg(err)
	}
	records, l, err := s.List(core.Filter{Type: "lab.*"})
	if err != nil {
		fail(err)
	}
	if l.ID == "" {
		fail(core.ErrNotInitialized)
	}
	if warnFrozen(l) {
		os.Exit(4)
	}
	result := coverageReport{Format: "bean-coverage/1", Cohort: *cohort, Selection: sources.Selection,
		Counts: map[string]int{"placed": 0, "unmatched_numeric": 0, "bounded": 0, "unreported": 0, "text": 0, "other": 0}, Rows: []coverage.Row{}}
	for _, r := range records {
		if r.Subject != l.ID || r.Type == "lab.report" || r.Value["blob"] != nil {
			continue
		}
		if *report != "" && r.Context["report_sha256"] != *report {
			continue
		}
		row, err := sources.Replay(r, *cohort)
		if err != nil {
			fail(err)
		}
		result.Rows = append(result.Rows, row)
		result.Counts[row.Status]++
		if len(row.Matches) > 0 || len(row.References) > 0 {
			result.SourceCovered++
		}
	}
	result.Total = len(result.Rows)
	if *report != "" && result.Total == 0 {
		fail(errors.New("no current lab readings for this report digest"))
	}
	if *machine {
		if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
			fail(err)
		}
		return
	}
	fmt.Printf("%d current lab readings. %d placed, %d other numeric, %d bounded, %d unreported, %d text, %d other. %d with a survey or assay source.\n",
		result.Total, result.Counts["placed"], result.Counts["unmatched_numeric"], result.Counts["bounded"], result.Counts["unreported"], result.Counts["text"], result.Counts["other"], result.SourceCovered)
	// A percentile is read with its caveats, as in bean compare. Each
	// placement names them by number, and the list follows the rows.
	var caveats []string
	for _, r := range result.Rows {
		test, _ := r.Facts["context.test"].(string)
		fmt.Printf("\n%s  %s  %s  (%s)\n", safe(r.ID), safe(test), safe(r.Type), r.Status)
		for _, m := range r.Matches {
			fmt.Printf("  %s in %s: percentile %g, n=%d (%s), SHA-256 %s\n", safe(m.Measure), safe(m.Cohort), m.Percentile, m.N, safe(m.Survey), m.SHA256)
			var evidence []string
			for _, k := range slices.Sorted(maps.Keys(m.Evidence.Required)) {
				v := m.Evidence.Required[k]
				if v == nil {
					evidence = append(evidence, k+" omitted")
				} else {
					evidence = append(evidence, fmt.Sprintf("%s=%q", k, v))
				}
			}
			fmt.Println("  Matched", safe(strings.Join(evidence, ", ")))
			if m.Review != "" {
				fmt.Println("  Review that your selection names:", safe(m.Review))
			}
			var marks []string
			for _, text := range m.Caveats {
				i := slices.Index(caveats, text)
				if i < 0 {
					i = len(caveats)
					caveats = append(caveats, text)
				}
				marks = append(marks, fmt.Sprintf("[%d]", i+1))
			}
			fmt.Println("  Caveats", strings.Join(marks, " "))
		}
		if len(r.Matches) == 0 {
			fmt.Println(" ", safe(r.Reason))
			if r.Status == "unmatched_numeric" {
				for _, d := range r.Declines {
					if len(d.Rules) == 0 {
						fmt.Println(" ", d.SHA256, safe(d.Reason))
						continue
					}
					for _, rule := range d.Rules {
						var fields []string
						for _, k := range rule.Different {
							want := rule.Required[k]
							if want == nil {
								fields = append(fields, k+" must be omitted")
								continue
							}
							got, ok := r.Facts[k]
							if !ok {
								fields = append(fields, fmt.Sprintf("%s needs %q (missing)", k, want))
							} else {
								fields = append(fields, fmt.Sprintf("%s needs %q (observed %v)", k, want, got))
							}
						}
						fmt.Printf("  %s %s: %s\n", d.SHA256, safe(rule.Measure), safe(strings.Join(fields, ", ")))
					}
				}
			}
		}
		// An assay guide is a publisher's list. Its words are shown as a
		// quote, with the publisher's name and the guide's SHA-256.
		for _, ref := range r.References {
			fmt.Printf("  Lab documentation that %s lists (guide SHA-256 %s): %s %s\n", safe(ref.Publisher), ref.SHA256, safe(ref.Title), safe(ref.URL))
			fmt.Printf("    %s says: %q\n", safe(ref.Publisher), safe(ref.Limit))
		}
	}
	if len(caveats) > 0 {
		fmt.Println()
		fmt.Println(percentileMeans)
		fmt.Println("Read each percentile with the caveats that its line names:")
		for i, text := range caveats {
			fmt.Printf("  [%d] %s\n", i+1, safe(text))
		}
	}
}
