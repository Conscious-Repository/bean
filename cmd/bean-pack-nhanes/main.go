// Command bean-pack-nhanes builds a reference pack from NHANES files: one
// file that holds, for each test and each cohort on a short menu, the table
// that a comparison needs (docs/REFERENCE.md).
//
//	bean-pack-nhanes -out nhanes-2017-2018.pack DEMO_J.xpt TCHOL_J.xpt ...
//
// It reads the files that you downloaded from CDC and goes nowhere. The
// same files give the same pack, byte for byte, so anyone can build a pack
// again and compare its SHA-256 with the one they were given.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/Conscious-Repository/bean/internal/nhanes"
	"github.com/Conscious-Repository/bean/internal/pack"
	"github.com/Conscious-Repository/bean/internal/view"
	"github.com/Conscious-Repository/bean/internal/xpt"
)

func main() {
	fs := flag.NewFlagSet("bean-pack-nhanes", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	out := fs.String("out", "", "the pack to write: a new file")
	unknown := fs.Bool("unknown-files", false, "also build from a file whose SHA-256 is not the one on record; the pack then says so")
	if err := fs.Parse(os.Args[1:]); err != nil || fs.NArg() == 0 || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: bean-pack-nhanes [-unknown-files] -out <new file> <DEMO_J.xpt> <a test's .xpt>...")
		os.Exit(2)
	}
	if err := build(*out, *unknown, fs.Args()); err != nil {
		fmt.Fprintf(os.Stderr, "bean-pack-nhanes: %s\n", view.Safe(err.Error()))
		os.Exit(1)
	}
}

// tests lists the data sets that a pack takes a measure from.
func tests() []string {
	var out []string
	for _, a := range nhanes.Analytes {
		if !slices.Contains(out, a.Dataset) {
			out = append(out, a.Dataset)
		}
	}
	for _, m := range nhanes.BodyMeasures {
		if !slices.Contains(out, m.Dataset) {
			out = append(out, m.Dataset)
		}
	}
	return out
}

func usable(dataset string) bool { return dataset == nhanes.Demo || slices.Contains(tests(), dataset) }

func build(out string, unknown bool, paths []string) error {
	files := map[string]nhanes.SourceFile{}
	var skipped []string
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		x, err := xpt.Parse(raw)
		if err != nil {
			return fmt.Errorf("%s is not a file that NHANES publishes: %w", path, err)
		}
		// A file that a pack holds nothing of is left out, and named: so
		// a folder of downloads can be given whole.
		if !usable(x.Dataset) {
			skipped = append(skipped, x.Dataset)
			continue
		}
		if _, twice := files[x.Dataset]; twice {
			return fmt.Errorf("%s is given twice", x.Dataset)
		}
		sum := sha256.Sum256(raw)
		digest := hex.EncodeToString(sum[:])
		if digest != nhanes.Known[x.Dataset] && !unknown {
			return fmt.Errorf("%s is not the file of %s on record: it may be cut short, changed, or a later release. Fetch it again from %s, or pass -unknown-files and the pack will say so", path, x.Dataset, nhanes.URL)
		}
		files[x.Dataset] = nhanes.SourceFile{File: x, SHA256: digest, Bytes: int64(len(raw))}
	}
	p, err := nhanes.BuildPack(files)
	if err != nil {
		return err
	}
	data := p.Bytes()
	// What is written must read back as the same pack.
	back, err := pack.Read(data)
	if err != nil {
		return fmt.Errorf("the pack that was built does not pass its own check: %w", err)
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("%s: a pack is written to a new file, and never over one. Name another file, or remove this one first: %w", out, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(out)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(out)
		return err
	}
	fmt.Printf("%s: %d measures, %d cohorts, %d cells, %d bytes\n", out, len(back.Measures), len(back.Cohorts), len(back.Cells), len(data))
	fmt.Printf("sha256 %s\n", back.SHA256)
	if _, ok := pack.Known[back.SHA256]; ok {
		fmt.Println("This is the pack that bean has on record.")
	}
	if len(skipped) > 0 {
		fmt.Printf("not used: %s. A pack is built from %s and the files of these tests: %s\n", strings.Join(skipped, ", "), nhanes.Demo, strings.Join(tests(), ", "))
	}
	for _, c := range back.Caveats[1:] {
		fmt.Println("note: " + c)
	}
	return nil
}
