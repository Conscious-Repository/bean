package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Conscious-Repository/bean/core"
)

// The conformance folders (V24) are logs that tools/second/cases.py
// writes. For each folder the generator writes expected.txt: the verify
// lines of SPEC 12.1 and the exit code of a replica with no local
// identity. tools/second/run.sh checks the second verifier against them.
const conformanceDir = "vectors/conformance"

func init() { extra = append(extra, genConformance) }

func genConformance(core.Signer, map[string]any) {
	dirs, err := filepath.Glob(filepath.Join(conformanceDir, "*", "objects"))
	if err != nil {
		panic(err)
	}
	for _, d := range dirs {
		dir := filepath.Dir(d)
		want := conformanceResult(readObjects(dir))
		if err := os.WriteFile(filepath.Join(dir, "expected.txt"), []byte(want), 0o644); err != nil {
			panic(err)
		}
	}
	fmt.Println("wrote", len(dirs), "conformance results")
}

// readObjects returns the plain files one directory below objects/, keyed
// by their path from the folder (SPEC 7.1).
func readObjects(dir string) map[string][]byte {
	files := map[string][]byte{}
	paths, err := filepath.Glob(filepath.Join(dir, "objects", "*", "*"))
	if err != nil {
		panic(err)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			panic(err)
		}
		rel, _ := filepath.Rel(dir, p)
		files[filepath.ToSlash(rel)] = b
	}
	return files
}

// conformanceResult is the verify output of the files and the exit code.
func conformanceResult(files map[string][]byte) string {
	rp := core.VerifyFiles(files)
	lines := rp.Lines()
	return strings.Join(lines, "\n") + fmt.Sprintf("\nexit %d\n", rp.ExitCode(nil))
}

func sortedDirs(glob string) []string {
	d, _ := filepath.Glob(glob)
	sort.Strings(d)
	return d
}
