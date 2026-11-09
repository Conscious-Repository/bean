package nhanes

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/Conscious-Repository/bean/internal/xpt/xpttest"
)

// The files in testdata are synthetic: made here, with the layout and the
// variable names of CDC's files and no person's data. Their header says
// SYNTH where CDC's says the system that wrote them, and their SHA-256 is
// not in Known, so a comparison with them says that bean does not know
// them. BEAN_UPDATE=1 go test ./internal/nhanes -run TestSyntheticFiles
// writes them again.

// splitmix is a small fixed generator, so the files never change with Go.
type splitmix uint64

func (s *splitmix) next() uint64 {
	*s += 0x9e3779b97f4a7c15
	z := uint64(*s)
	z = (z ^ z>>30) * 0xbf58476d1ce4e5b9
	z = (z ^ z>>27) * 0x94d049bb133111eb
	return z ^ z>>31
}

// below returns a whole number from 0 to n-1.
func (s *splitmix) below(n int) int { return int(s.next() % uint64(n)) }

const syntheticPeople = 1600

func syntheticFiles() (map[string][]byte, error) {
	rng := splitmix(20261109)
	nan := math.NaN()
	ethnicities := []float64{1, 2, 3, 3, 3, 4, 4, 6, 7}
	var demo, tchol, hdl, bmx, diet [][]float64
	for i := 0; i < syntheticPeople; i++ {
		seqn := float64(100001 + i)
		sex := float64(1 + rng.below(2))
		age := float64(rng.below(MaxAge + 1))
		// A weight with five decimals, as CDC's have. A few people were
		// interviewed and not examined: their weight is zero, and the
		// examination files have no row for them.
		weight := float64(500000000+rng.below(3500000000)) / 100000
		examined := rng.below(40) != 0
		if !examined {
			weight = 0
		}
		demo = append(demo, []float64{seqn, sex, age, ethnicities[rng.below(len(ethnicities))], weight, float64(1 + rng.below(2)), float64(134 + rng.below(15))})
		if !examined {
			continue
		}
		bmi := float64(160+rng.below(120)+rng.below(120)) / 10
		if rng.below(30) == 0 {
			bmi = nan
		}
		bmx = append(bmx, []float64{seqn, bmi})
		// The day one recall: done and reliable (1), not reliable (2) or
		// not done (5). Only a reliable one has nutrients, and a recall
		// that was not done has a weight of zero.
		status, dietWeight := 1.0, weight*float64(90+rng.below(40))/100
		kcal := float64(900 + rng.below(1300) + rng.below(1300))
		protein := float64(2000+rng.below(6000)+rng.below(6000)) / 100
		fiber := float64(20+rng.below(200)+rng.below(200)) / 10
		switch rng.below(12) {
		case 0:
			status, dietWeight, kcal, protein, fiber = 5, 0, nan, nan, nan
		case 1:
			status, kcal, protein, fiber = 2, nan, nan, nan
		}
		diet = append(diet, []float64{seqn, dietWeight, status, kcal, protein, fiber})
		// Lipids are measured from age 6, and not in everyone.
		if age < 6 || rng.below(12) == 0 {
			continue
		}
		// A sum of uniform draws gives a bell shape.
		tc, h := 110.0, 22.0
		for k := 0; k < 6; k++ {
			tc += float64(rng.below(28))
			h += float64(rng.below(11))
		}
		if rng.below(25) == 0 {
			tc, h = nan, nan
		}
		tchol = append(tchol, []float64{seqn, tc, math.Round(tc*2.586) / 100})
		hdl = append(hdl, []float64{seqn, h, math.Round(h*2.586) / 100})
	}
	out := map[string][]byte{}
	const created = "09NOV26:00:00:00"
	for _, f := range []struct {
		name          string
		names, labels []string
		rows          [][]float64
	}{
		{"DEMO_J", []string{"SEQN", "RIAGENDR", "RIDAGEYR", "RIDRETH3", "WTMEC2YR", "SDMVPSU", "SDMVSTRA"},
			[]string{"Respondent sequence number", "Gender", "Age in years at screening", "Race/Hispanic origin w/ NH Asian", "Full sample 2 year MEC exam weight", "Masked variance pseudo-PSU", "Masked variance pseudo-stratum"}, demo},
		{"TCHOL_J", []string{"SEQN", "LBXTC", "LBDTCSI"},
			[]string{"Respondent Sequence Number", "Total Cholesterol (mg/dL)", "Total Cholesterol (mmol/L)"}, tchol},
		{"HDL_J", []string{"SEQN", "LBDHDD", "LBDHDDSI"},
			[]string{"Respondent Sequence Number", "Direct HDL-Cholesterol (mg/dL)", "Direct HDL-Cholesterol (mmol/L)"}, hdl},
		{"BMX_J", []string{"SEQN", "BMXBMI"},
			[]string{"Respondent sequence number", "Body Mass Index (kg/m**2)"}, bmx},
		{"DR1TOT_J", []string{"SEQN", "WTDRD1", "DR1DRSTZ", "DR1TKCAL", "DR1TPROT", "DR1TFIBE"},
			[]string{"Respondent sequence number", "Dietary day one sample weight", "Dietary recall status", "Energy (kcal)", "Protein (gm)", "Dietary fiber (gm)"}, diet},
	} {
		data, err := xpttest.Write(f.name, created, f.names, f.labels, f.rows)
		if err != nil {
			return nil, err
		}
		out[f.name] = data
	}
	return out, nil
}

func TestSyntheticFiles(t *testing.T) {
	files, err := syntheticFiles()
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		path := filepath.Join("testdata", name+".XPT")
		if os.Getenv("BEAN_UPDATE") == "1" {
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		have, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(have, data) {
			t.Errorf("%s is not what this test makes (%v): run it with BEAN_UPDATE=1", path, err)
		}
		if _, known := Known[name]; !known {
			t.Errorf("%s: no digest of the real file on record", name)
		}
		if bytes.Contains(data, []byte("W32_8PRO")) || !bytes.Contains(data, []byte("SYNTH")) {
			t.Errorf("%s does not say that it is synthetic", name)
		}
	}
}
