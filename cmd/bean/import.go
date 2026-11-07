package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Conscious-Repository/bean/core"
)

// bean import <source> [importer flags] [-corrections] <file> keeps the raw
// file first, then its readings. It runs bean-import-<source>, which never
// signs: this command stores the file, checks it reads back, appends the
// importer's archive record, checks that the log keeps it, and only then
// appends the readings. Each step that can fail says what was written.

// maxImportFile bounds the private copy of a source file.
const maxImportFile = 256 << 20

var sourceName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func cmdImport(s *core.Store, args []string) {
	if len(args) < 2 || !sourceName.MatchString(args[0]) {
		usage()
	}
	source, file := args[0], args[len(args)-1]
	corrections := false
	var pass []string
	for _, a := range args[1 : len(args)-1] {
		if a == "-corrections" || a == "--corrections" {
			corrections = true
			continue
		}
		pass = append(pass, a)
	}
	bin, err := importerPath(source)
	if err != nil {
		fail(err)
	}
	out, err := runImport(s, bin, file, pass, corrections)
	for _, l := range out.lines {
		fmt.Println(l)
	}
	if err != nil {
		fail(err)
	}
	if len(out.changed) > 0 {
		for _, c := range out.changed {
			fmt.Fprintf(os.Stderr, "changed: %s %s: the log has %s, this file says %s\n", c.ID[7:19], safe(c.Type), safe(c.Old), safe(c.New))
		}
		fail(fmt.Errorf("%d readings have a new value, and nothing was written for them. The file is kept. Check that it is newer and right, then run bean import %s -corrections", len(out.changed), source))
	}
}

// importerPath finds bean-import-<source> next to this binary, else on PATH.
func importerPath(source string) (string, error) {
	name := "bean-import-" + source
	if self, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(self), name)
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			return p, nil
		}
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("no importer %s: build it with go install ./cmd/%s", name, name)
	}
	return p, nil
}

type importResult struct {
	lines   []string
	changed []core.Changed
}

// phase errors say what is on disk after a failure.
func phaseError(phase string, err error) error {
	msg := map[string]string{
		"validation": "the file was not accepted, and nothing was written",
		"storage":    "the file could not be stored and read back; an encrypted copy may remain, but no record was written",
		"archive":    "the record that keeps the file was not added; no reading was written",
		"retention":  "the log does not keep the file (it may have been deleted with bean purge); no reading was written",
		"readings":   "some readings may be written; run the same command again, it adds only what is missing",
	}[phase]
	return fmt.Errorf("%s: %w", msg, err)
}

func runImport(s *core.Store, bin, file string, args []string, corrections bool) (importResult, error) {
	var res importResult
	dir, err := os.MkdirTemp("", "bean-import-*")
	if err != nil {
		return res, err
	}
	defer os.RemoveAll(dir)
	// One private snapshot: the importer, the hash and the stored blob all see
	// the same bytes, whatever happens to the file meanwhile.
	data, err := snapshot(file, filepath.Join(dir, "source"))
	if err != nil {
		return res, phaseError("validation", err)
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	snap := filepath.Join(dir, "source")
	// Validate before any write.
	if _, _, err := runImporter(bin, "sha256:"+strings.Repeat("0", 64), args, snap); err != nil {
		return res, phaseError("validation", err)
	}
	bid, err := s.PutBlob(data)
	if err != nil {
		return res, phaseError("storage", err)
	}
	back, err := s.GetBlob(bid)
	if err != nil || !bytes.Equal(back, data) {
		return res, phaseError("storage", errors.New("the stored copy does not read back the same"))
	}
	out, note, err := runImporter(bin, bid, args, snap)
	if err != nil {
		return res, phaseError("validation", err)
	}
	if note != "" {
		res.lines = append(res.lines, note)
	}
	first, rest, _ := bytes.Cut(out, []byte("\n"))
	var archive struct {
		Type  string         `json:"type"`
		Value map[string]any `json:"value"`
	}
	if json.Unmarshal(first, &archive) != nil || archive.Value["blob"] != bid {
		return res, phaseError("archive", errors.New("the importer did not start with a record of the file"))
	}
	a, err := s.Append(bytes.NewReader(append(first, '\n')))
	if err != nil {
		return res, phaseError("archive", err)
	}
	if err := kept(s, archive.Type, bid, digest); err != nil {
		return res, phaseError("retention", err)
	}
	if a.Added == 1 {
		res.lines = append(res.lines, "kept "+archive.Type+" "+bid)
	} else {
		res.lines = append(res.lines, "already kept "+archive.Type+" "+bid)
	}
	appendFn := s.Append
	if corrections {
		appendFn = s.AppendCorrections
	}
	r, err := appendFn(bytes.NewReader(rest))
	line := fmt.Sprintf("added %d, skipped %d", r.Added, r.Skipped)
	if r.Corrected > 0 {
		line += fmt.Sprintf(", %d of them new values for readings you already had", r.Corrected)
	}
	res.lines = append(res.lines, line)
	res.changed = r.Changed
	if err != nil {
		return res, phaseError("readings", err)
	}
	if err := kept(s, archive.Type, bid, digest); err != nil {
		return res, phaseError("retention", err)
	}
	return res, nil
}

// snapshot copies a regular file, at most maxImportFile bytes, and returns it.
func snapshot(src, dst string) ([]byte, error) {
	info, err := os.Lstat(src)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file: " + src)
	}
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if opened, err := f.Stat(); err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("the file changed while it was opened")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxImportFile+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImportFile {
		return nil, fmt.Errorf("the file is larger than %d MiB", maxImportFile>>20)
	}
	return data, os.WriteFile(dst, data, 0o600)
}

// runImporter runs the importer and returns its output, all of it or
// nothing, and the last line it wrote to stderr.
func runImporter(bin, blob string, args []string, file string) ([]byte, string, error) {
	cmd := exec.Command(bin, append(append([]string{"-blob", blob}, args...), file)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	msg := lastLine(errb.String())
	if err != nil {
		if msg == "" {
			msg = err.Error()
		}
		return nil, "", errors.New(msg)
	}
	return out.Bytes(), msg, nil
}

func lastLine(s string) string {
	var last string
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		if t := strings.TrimSpace(sc.Text()); t != "" && !strings.Contains(t, "warning:") {
			last = t
		}
	}
	return last
}

// kept reports whether the log keeps the person's record of this exact
// file, and the file itself. It proves every record file, because the object
// cache vouches for a file by its size and time, and it reads the blob back.
func kept(s *core.Store, typ, bid, digest string) error {
	l, err := s.LoadFull()
	if err != nil {
		return err
	}
	if l.IsTooNew() {
		return l.TooNewError()
	}
	found := false
	for id, r := range l.Report.Records {
		if found || r.Type != typ || !l.Current(id) || r.Author != l.ID || r.Subject != l.ID || r.Value["blob"] != bid || r.Context["sha256"] != digest {
			continue
		}
		for _, b := range r.Blobs {
			found = found || b.Hash == bid
		}
	}
	if !found {
		return errors.New("no current record of yours keeps this file")
	}
	data, err := s.GetBlob(bid)
	if err != nil {
		return err
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != digest {
		return errors.New("the stored copy no longer matches the file")
	}
	return nil
}
