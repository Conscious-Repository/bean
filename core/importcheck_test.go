package core

import (
	"os"
	"strings"
	"testing"
)

// BlobIDOf gives the id PutBlob gives, and writes no file.
func TestBlobIDOfMatchesPutBlob(t *testing.T) {
	s, _ := initStore(t)
	data := []byte("synthetic report\n")
	id, err := s.BlobIDOf(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.path(BlobPathOf(id))); !os.IsNotExist(err) {
		t.Fatalf("BlobIDOf wrote the blob: %v", err)
	}
	put, err := s.PutBlob(data)
	if err != nil || put != id {
		t.Fatalf("PutBlob %s %v, BlobIDOf %s", put, err, id)
	}
}

// InputPurged finds the ref of a line the person purged, as Append computes it.
func TestInputPurgedFollowsAppend(t *testing.T) {
	s, _ := initStore(t)
	line := `{"type":"lab.report","value":{"blob":"sha256:` + strings.Repeat("a", 64) + `"},"blobs":[{"hash":"sha256:` + strings.Repeat("a", 64) + `","role":"raw-export","media_type":"text/plain"}],"times":{"observed":null},"source":{"system":"labcorp","adapter":"t@1","ref_input":["bean/v0/ref","tuple","labcorp","lab.report",null,{"blob":"x"},0]}}`
	other := strings.Replace(line, `"blob":"x"`, `"blob":"y"`, 1)
	if p, err := s.InputPurged([]byte(line)); err != nil || p {
		t.Fatalf("before: %v %v", p, err)
	}
	r, err := s.Append(strings.NewReader(line + "\n"))
	if err != nil || r.Added != 1 {
		t.Fatalf("append: %+v %v", r, err)
	}
	if _, err := s.Purge(r.IDs[0], ""); err != nil {
		t.Fatal(err)
	}
	if p, err := s.InputPurged([]byte(line)); err != nil || !p {
		t.Errorf("after purge: %v %v", p, err)
	}
	if p, err := s.InputPurged([]byte(other)); err != nil || p {
		t.Errorf("other line: %v %v", p, err)
	}
	// Another subject has another ref.
	stranger := strings.Replace(line, `{"type"`, `{"subject":"sha256:`+strings.Repeat("7", 64)+`","type"`, 1)
	if p, err := s.InputPurged([]byte(stranger)); err != nil || p {
		t.Errorf("other subject: %v %v", p, err)
	}
	if _, err := s.InputPurged([]byte("not json")); err == nil {
		t.Error("bad json accepted")
	}
}
