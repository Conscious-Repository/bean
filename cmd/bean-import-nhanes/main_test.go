package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/internal/xpt/xpttest"
)

const blob = "sha256:1111111111111111111111111111111111111111111111111111111111111111"

func TestRecord(t *testing.T) {
	raw, err := os.ReadFile("../../internal/nhanes/testdata/TCHOL_J.XPT")
	if err != nil {
		t.Fatal(err)
	}
	rec, note, err := record(raw, blob)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(rec)
	for _, want := range []string{
		`"type":"reference.file"`, `"value":{"blob":"` + blob + `"}`, `"observed":null`,
		`"system":"nhanes"`, `"adapter":"bean-import-nhanes@`, `"ref_input":["bean/v0/ref","tuple","nhanes","reference.file",null,{"blob":"` + blob + `"},0]`,
		`"dataset":"TCHOL_J"`, `"cycle":"2017-2018"`, `"bytes":33440`, `"assurance":"unsigned-file"`,
		`"url":"https://wwwn.cdc.gov/Nchs/Data/Nhanes/Public/2017/DataFiles/TCHOL_J.xpt"`,
		`{"label":"Total Cholesterol (mg/dL)","name":"LBXTC"}`,
		`"blobs":[{"hash":"` + blob + `","media_type":"application/x-sas-xport","role":"raw-export"}]`,
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("no %s in\n%s", want, data)
		}
	}
	// The synthetic file is not CDC's, and the importer says so.
	if !strings.Contains(note, "NOT the file on record") || !strings.Contains(note, "TCHOL_J, NHANES 2017-2018") {
		t.Errorf("note: %s", note)
	}
	// The same file gives the same record.
	again, _, _ := record(raw, blob)
	if b, _ := json.Marshal(again); !bytes.Equal(b, data) {
		t.Error("two runs differ")
	}
}

func TestRefuses(t *testing.T) {
	file := func(dataset string, names, labels []string, rows [][]float64) []byte {
		data, err := xpttest.Write(dataset, "09NOV26:00:00:00", names, labels, rows)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	two := []string{"SEQN", "X"}
	plain := []string{"a", "b"}
	rows := [][]float64{{1, 2}}
	for name, raw := range map[string][]byte{
		"not a transport file": []byte("%PDF-1.7 a lab report"),
		"empty":                {},
		"another cycle":        file("TCHOL_I", two, plain, rows),
		"no cycle":             file("TCHOL", two, plain, rows),
		"only the suffix":      file("_J", two, plain, rows),
		"lowercase name":       file("tchol_J", two, plain, rows),
		"no SEQN":              file("TCHOL_J", []string{"ID", "X"}, plain, rows),
		"no rows":              file("TCHOL_J", two, plain, nil),
		"a label with a bell":  file("TCHOL_J", two, []string{"a", "b\a"}, rows),
		"a label not in ASCII": file("TCHOL_J", two, []string{"a", "caf\xc3\xa9"}, rows),
		"a bad variable name":  file("TCHOL_J", []string{"SEQN", "x y"}, plain, rows),
	} {
		if rec, _, err := record(raw, blob); err == nil {
			t.Errorf("%s: read as %v", name, rec["context"])
		}
	}
}
