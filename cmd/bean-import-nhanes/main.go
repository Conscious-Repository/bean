// Command bean-import-nhanes keeps one public NHANES data file (SAS
// transport, .XPT) as reference data: bean import nhanes TCHOL_J.XPT. It
// writes one reference.file record and no readings: the file describes
// other people, not the person. bean compare reads the kept files.
//
// The person who imports a file signs for having kept it. That does not
// make it CDC's claim: the record holds the file's SHA-256, so anyone can
// check it against the copy CDC serves.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/nhanes"
	"github.com/Conscious-Repository/bean/internal/xpt"
)

const version = "bean-import-nhanes@0.1.0"

// maxFile bounds the file, as bean import does.
const maxFile = 256 << 20

var (
	blobPattern    = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	datasetPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,7}$`)
	labelPattern   = regexp.MustCompile(`^[ -~]*$`) // printable ASCII
)

func main() {
	fs := flag.NewFlagSet("bean-import-nhanes", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	blob := fs.String("blob", "", "blob id of the file")
	if err := fs.Parse(os.Args[1:]); err != nil || fs.NArg() != 1 || !blobPattern.MatchString(*blob) {
		fmt.Fprintln(os.Stderr, "usage: bean-import-nhanes -blob <blob id> <file.XPT>\nrun it through bean: bean import nhanes <file.XPT>")
		os.Exit(2)
	}
	line, note, err := convert(fs.Arg(0), *blob)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bean-import-nhanes: %s\n", err)
		os.Exit(1)
	}
	os.Stdout.Write(append(line, '\n'))
	fmt.Fprintln(os.Stderr, "bean-import-nhanes: "+note)
}

// convert reads the file and returns the record that keeps it, and a line
// that says what it is.
func convert(path, blob string) ([]byte, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err != nil {
		return nil, "", err
	}
	if len(raw) > maxFile {
		return nil, "", fmt.Errorf("the file is larger than %d MiB", maxFile>>20)
	}
	rec, note, err := record(raw, blob)
	if err != nil {
		return nil, "", err
	}
	return core.Canonical(rec), note, nil
}

func record(raw []byte, blob string) (map[string]any, string, error) {
	x, err := xpt.Parse(raw)
	if err != nil {
		return nil, "", err
	}
	if !datasetPattern.MatchString(x.Dataset) {
		return nil, "", errors.New("the data set name is not one NHANES uses")
	}
	if !nhanes.InCycle(x.Dataset) {
		return nil, "", fmt.Errorf("%s is not a file of a survey cycle bean reads (2017-2018, names ending in _J)", x.Dataset)
	}
	if seqn, ok := x.Variable("SEQN"); !ok || !seqn.Numeric {
		return nil, "", fmt.Errorf("%s has no SEQN, the number that joins NHANES files", x.Dataset)
	}
	if x.Rows == 0 {
		return nil, "", fmt.Errorf("%s has no rows", x.Dataset)
	}
	var vars []any
	for _, v := range x.Variables {
		if !datasetPattern.MatchString(v.Name) || !labelPattern.MatchString(v.Label) {
			return nil, "", fmt.Errorf("variable %d of %s has a name or label that is not plain text", len(vars)+1, x.Dataset)
		}
		vars = append(vars, map[string]any{"name": v.Name, "label": v.Label})
	}
	if !labelPattern.MatchString(x.Created) {
		return nil, "", errors.New("the file's creation time is not plain text")
	}
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	value := map[string]any{"blob": blob}
	rec := map[string]any{
		"type":  nhanes.FileType,
		"value": value,
		"times": map[string]any{"observed": nil},
		"source": map[string]any{
			"system": nhanes.System, "adapter": version,
			"ref_input": core.RefInputTuple(nhanes.System, nhanes.FileType, nil, value, 0),
		},
		"context": map[string]any{
			"sha256": digest, "bytes": float64(len(raw)),
			"assurance": "unsigned-file",
			"dataset":   x.Dataset,
			"cycle":     nhanes.Years,
			"publisher": "US National Center for Health Statistics (CDC), NHANES",
			"url":       nhanes.URL + x.Dataset + ".xpt",
			"license":   "public domain (a work of the US Government); NCHS asks users not to try to identify participants",
			"created":   x.Created,
			"rows":      float64(x.Rows),
			"variables": vars,
		},
		"blobs": []any{map[string]any{"hash": blob, "role": "raw-export", "media_type": "application/x-sas-xport"}},
	}
	note := fmt.Sprintf("%s, NHANES %s: %d rows, %d variables. ", x.Dataset, nhanes.Years, x.Rows, len(x.Variables))
	switch known, listed := nhanes.Known[x.Dataset]; {
	case listed && known == digest:
		note += "It is the file on record"
	case listed:
		note += "It is NOT the file on record: it may be cut short or changed, and bean compare uses it only with -unknown-files. Its SHA-256 is " + digest
	default:
		note += "bean has no SHA-256 on record for this file. Its SHA-256 is " + digest
	}
	return rec, note, nil
}
