package core

import (
	"strings"
	"testing"
)

// SignPartial signs and writes nothing; AddSigned stores exactly those
// bytes, and only a record that this log's key signed.
func TestSignPartialThenAddSigned(t *testing.T) {
	s, _ := initStore(t)
	line := []byte(`{"type":"note","value":{"s":"later"},"times":{"observed":null},"source":{"system":"self","adapter":"t@1","ref":null}}`)
	id, data, err := s.SignPartial(line)
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := s.Load(); l.Report.Records[id] != nil {
		t.Fatal("SignPartial wrote the record")
	}
	if err := s.AddSigned(id, data); err != nil {
		t.Fatal(err)
	}
	l, _ := s.Load()
	if r := l.Report.Records[id]; r == nil || !l.Report.Valid[id] || string(r.Bytes) != string(data) {
		t.Fatal("AddSigned did not store the signed bytes as a valid record")
	}
	// Again is not an error, and changed bytes are refused.
	if err := s.AddSigned(id, data); err != nil {
		t.Errorf("a second AddSigned: %v", err)
	}
	if err := s.AddSigned(id, []byte(strings.Replace(string(data), "later", "sooner", 1))); err == nil {
		t.Error("changed bytes were stored")
	}
	// A record that another log's key signed is refused.
	other, _ := initStore(t)
	oid, odata, err := other.SignPartial(line)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddSigned(oid, odata); err == nil {
		t.Error("a record of another identity was stored")
	}
}

// PurgeMany purges each record as Purge does, with one load.
func TestPurgeMany(t *testing.T) {
	s, _ := initStore(t)
	var ids []string
	for _, v := range []string{"1", "2", "3"} {
		id, err := s.Add("note", v, AddOptions{})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if n, err := s.PurgeMany(ids[:2], "test"); n != 2 || err != nil {
		t.Fatalf("%d %v", n, err)
	}
	recs, _, err := s.List(Filter{Type: "note"})
	if err != nil || len(recs) != 1 || recs[0].ID != ids[2] {
		t.Fatalf("after PurgeMany: %d notes, %v", len(recs), err)
	}
	lines, code, err := s.VerifyLines()
	if err != nil || code != 0 {
		t.Errorf("verify after PurgeMany: %d %v\n%s", code, err, strings.Join(lines, "\n"))
	}
	// A bad id stops it, and reports how many went before.
	if n, err := s.PurgeMany([]string{ids[2], "sha256:" + strings.Repeat("0", 64)}, "test"); n != 1 || err == nil {
		t.Errorf("a bad id: %d %v", n, err)
	}
}
