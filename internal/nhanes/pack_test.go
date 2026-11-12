package nhanes

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/internal/pack"
	"github.com/Conscious-Repository/bean/internal/xpt"
)

// sources reads files as BuildPack takes them.
func sources(t *testing.T, dir string, names ...string) (map[string]SourceFile, map[string]*xpt.File) {
	t.Helper()
	out, parsed := map[string]SourceFile{}, map[string]*xpt.File{}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name+".XPT"))
		if err != nil {
			t.Fatal(err)
		}
		f, err := xpt.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		out[name], parsed[name] = SourceFile{File: f, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(data))}, f
	}
	return out, parsed
}

// TestPackGivesWhatTheFilesGive: a comparison with a pack is the same
// numbers, to the last bit, as the comparison with the files that the pack
// was built from. And the same files give the same pack, byte for byte.
func TestPackGivesWhatTheFilesGive(t *testing.T) {
	files, parsed := sources(t, "testdata", "TCHOL_J", "HDL_J", "DEMO_J", "BMX_J")
	built, err := BuildPack(files)
	if err != nil {
		t.Fatal(err)
	}
	data := built.Bytes()
	if again, _ := BuildPack(files); string(again.Bytes()) != string(data) {
		t.Fatal("two builds from the same files differ")
	}
	p, err := pack.Read(data)
	if err != nil {
		t.Fatal(err)
	}
	// The synthetic files are not the files on record, and the pack says so.
	for _, name := range []string{"TCHOL_J", "HDL_J", "DEMO_J", "BMX_J"} {
		found := false
		for _, c := range p.Caveats {
			found = found || strings.Contains(c, name)
		}
		if !found {
			t.Errorf("the pack does not say that %s is not the file on record: %v", name, p.Caveats)
		}
	}
	if len(p.Cells) == 0 || len(p.Cohorts) != 14 {
		t.Fatalf("%d cells, %d cohorts", len(p.Cells), len(p.Cohorts))
	}
	checked := 0
	for _, sex := range []string{"female", "male"} {
		for _, ages := range PackAges {
			c := cohort(t, sex, ages[0], ages[1])
			id := PackCohortID(sex, ages)
			if p.Cohorts[id].Text != c.String() {
				t.Errorf("%s reads %q, and the cohort is %q", id, p.Cohorts[id].Text, c)
			}
			for _, a := range Analytes {
				if parsed[a.Dataset] == nil {
					continue
				}
				cell := p.Cell(a.Variable, id)
				_, n, err := Table(parsed, a.Dataset, a.Variable, a.Label, c)
				if (cell == nil) != (err == ErrSmall) {
					t.Errorf("%s in %s: cell %v, n %d, %v", a.Variable, id, cell != nil, n, err)
				}
				if cell == nil {
					continue
				}
				// Values in the cell, between two of them, and outside it.
				tries := []float64{0.001, cell.Values[0], cell.Values[len(cell.Values)-1], cell.Values[len(cell.Values)-1] + 1}
				for i := 1; i < len(cell.Values); i += 7 {
					tries = append(tries, cell.Values[i], (cell.Values[i]+cell.Values[i-1])/2)
				}
				for _, v := range tries {
					want, err1 := Compare(parsed, a, c, v)
					got, err2 := cell.Place(v)
					if err1 != nil || err2 != nil || got != want {
						t.Fatalf("%s in %s at %v: the pack gives %+v (%v), the files give %+v (%v)", a.Variable, id, v, got, err2, want, err1)
					}
					checked++
				}
			}
		}
	}
	if checked < 200 {
		t.Errorf("only %d comparisons were made", checked)
	}
	// Without the demographics file there is no pack.
	delete(files, "DEMO_J")
	if _, err := BuildPack(files); err == nil {
		t.Error("a pack was built without DEMO_J")
	}
}

// TestRealPack: with CDC's own files, the pack is the one on record.
func TestRealPack(t *testing.T) {
	dir := os.Getenv("BEAN_NHANES_DIR")
	if dir == "" {
		t.Skip("set BEAN_NHANES_DIR to a folder of CDC's files")
	}
	files, _ := sources(t, dir, "DEMO_J", "TCHOL_J", "HDL_J", "GHB_J", "CBC_J", "BMX_J")
	p, err := BuildPack(files)
	if err != nil {
		t.Fatal(err)
	}
	// A body measure is in the pack, under a type that bean writes.
	if id, _ := p.MeasureFor("weight", "kg"); id != "BMXWT" || p.Cell("BMXWT", "male-30-39") == nil {
		t.Errorf("weight in kg is the measure %q", id)
	}
	sum := sha256.Sum256(p.Bytes())
	if got := hex.EncodeToString(sum[:]); !pack.IsKnown(got) {
		t.Errorf("the pack of the files on record has SHA-256 %s, and bean has no record of it: %v", got, pack.Known)
	}
}
