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

// A file deleted under one system is deleted under every one, and a line that
// carries a ready-made source.ref is checked too.
func TestInputPurgedAnySystemAndReadyRef(t *testing.T) {
	s, _ := initStore(t)
	value := map[string]any{"blob": "sha256:" + strings.Repeat("b", 64)}
	lineFor := func(system string) string {
		return string(Canonical(map[string]any{"type": "csv.file", "value": value, "times": map[string]any{"observed": nil},
			"blobs":  []any{map[string]any{"hash": value["blob"], "role": "raw-export", "media_type": "text/csv"}},
			"source": map[string]any{"system": system, "adapter": "t@1", "ref_input": RefInputTuple(system, "csv.file", nil, value, 0)}}))
	}
	r, err := s.Append(strings.NewReader(lineFor("csv") + "\n"))
	if err != nil || r.Added != 1 {
		t.Fatalf("append: %+v %v", r, err)
	}
	ref := DedupeRef(DedupeKey(mustSecret(t, s)), RefInputTuple("csv", "csv.file", nil, value, 0))
	if _, err := s.Purge(r.IDs[0], ""); err != nil {
		t.Fatal(err)
	}
	for _, sys := range []string{"csv", "csv-2"} {
		if p, err := s.InputPurged([]byte(lineFor(sys))); err != nil || !p {
			t.Errorf("system %s: %v %v", sys, p, err)
		}
	}
	ready := `{"type":"csv.file","value":{"blob":"x"},"source":{"system":"other","adapter":"t@1","ref":"` + ref + `"}}`
	if p, err := s.InputPurged([]byte(ready)); err != nil || !p {
		t.Errorf("ready-made ref: %v %v", p, err)
	}
	fresh := `{"type":"csv.file","value":{"blob":"x"},"source":{"system":"csv","adapter":"t@1","ref":"hmac-sha256:` + strings.Repeat("0", 64) + `"}}`
	if p, err := s.InputPurged([]byte(fresh)); err != nil || p {
		t.Errorf("unpurged ref: %v %v", p, err)
	}
}

func mustSecret(t *testing.T, s *Store) []byte {
	t.Helper()
	secret, err := s.identitySecret()
	if err != nil {
		t.Fatal(err)
	}
	return secret
}
