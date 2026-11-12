package core

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// peerNote signs a record with a key of another person, as a bare author,
// about subject, and changes it before it is signed.
func peerNote(t *testing.T, signer Signer, subject string, change func(obj map[string]any)) (string, []byte) {
	t.Helper()
	obj := map[string]any{
		"v": 0.0, "nonce": NewNonce(), "type": "note", "subject": subject, "author": signer.Public(), "kid": signer.Public(),
		"value": map[string]any{"s": "a note"}, "times": map[string]any{"observed": nil, "imported": "2026-11-12T10:00:00Z"},
		"source": map[string]any{"system": "bean-reply", "adapter": Adapter, "ref": nil},
	}
	if change != nil {
		change(obj)
	}
	id, data, err := SignRecord(obj, signer)
	if err != nil {
		t.Fatal(err)
	}
	return id, data
}

// TestAddFromKey: a record that another person's key signed goes into the
// log only when it is about the person and has no power over any record.
func TestAddFromKey(t *testing.T) {
	s := Open(t.TempDir())
	me, _, err := s.Init(NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	mine, err := s.Add("weight", "81.2", AddOptions{Unit: "kg"})
	if err != nil {
		t.Fatal(err)
	}
	_, seed, _ := ed25519.GenerateKey(nil)
	peer := NewEd25519Signer(seed.Seed())
	local, _ := s.ReadLocal()
	other := "sha256:" + strings.Repeat("a", 64)
	count := func() int {
		l, err := s.Load()
		if err != nil {
			t.Fatal(err)
		}
		return len(l.Report.Records)
	}
	before := count()
	for name, tc := range map[string]struct {
		signer Signer
		change func(obj map[string]any)
		want   string
	}{
		"about another person":        {peer, func(o map[string]any) { o["subject"] = other }, "not a record about this log's person"},
		"signed as an identity":       {peer, func(o map[string]any) { o["author"] = other }, "not a record that a key signed by itself"},
		"signed by the log's own key": {local.Signer, nil, "is not another person's"},
		"a correction":                {peer, func(o map[string]any) { o["refs"] = []any{map[string]any{"rel": "supersedes", "id": mine}} }, "corrects and deletes nothing"},
		"a deletion": {peer, func(o map[string]any) {
			o["type"], o["refs"] = "note", []any{map[string]any{"rel": "purges", "id": mine}}
		}, ""}, // the checks of a file refuse it first: only a tombstone purges
		"a pairing":           {peer, func(o map[string]any) { o["type"], o["value"] = "agent.pair", map[string]any{"key": peer.Public()} }, "bad type"},
		"one of bean's types": {peer, func(o map[string]any) { o["type"] = "bean.note" }, ""},
		"a file": {peer, func(o map[string]any) {
			o["blobs"] = []any{map[string]any{"hash": "sha256:" + strings.Repeat("b", 64), "role": "photo", "media_type": "image/png"}}
		}, "keeps no file"},
	} {
		id, data := peerNote(t, tc.signer, me, tc.change)
		err := s.AddFromKey(id, data)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error with %q", name, err, tc.want)
		}
	}
	if after := count(); after != before {
		t.Errorf("a refused record changed the log: %d records, then %d", before, after)
	}
	// The plain form goes in, once, and not again after the person deleted it.
	id, data := peerNote(t, peer, me, func(o map[string]any) { o["refs"] = []any{map[string]any{"rel": "about", "id": mine}} })
	for range 2 {
		if err := s.AddFromKey(id, data); err != nil {
			t.Fatalf("a peer's note: %v", err)
		}
	}
	if count() != before+1 {
		t.Errorf("a peer's note is not in the log once")
	}
	if _, err := s.Purge(id, "not wanted"); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if err := s.AddFromKey(id, data); err == nil || !strings.Contains(err.Error(), "the person deleted this record") {
		t.Errorf("a deleted note was taken again: %v", err)
	}
}

// TestPeerNames: a name belongs to one identity, the log keeps each head
// of a peer's chain that the person saw, and a name that was taken back
// answers for no identity.
func TestPeerNames(t *testing.T) {
	s := Open(t.TempDir())
	if _, _, err := s.Init(NewKey{}); err != nil {
		t.Fatal(err)
	}
	key := func() string {
		_, seed, _ := ed25519.GenerateKey(nil)
		return NewEd25519Signer(seed.Seed()).Public()
	}
	ana, bob := "sha256:"+strings.Repeat("a", 64), "sha256:"+strings.Repeat("b", 64)
	head1, head2 := "sha256:"+strings.Repeat("1", 64), "sha256:"+strings.Repeat("2", 64)
	k1, k2, k3 := key(), key(), key()
	if _, err := s.PairPeer(k1, "Ana", ana, head1); err != nil {
		t.Fatal(err)
	}
	// The same key keeps its name, and a second key of the identity takes it.
	if _, err := s.PairPeer(k1, "Anna", ana, head1); err == nil || !strings.Contains(err.Error(), "already paired") {
		t.Errorf("another name for a key: %v", err)
	}
	if _, err := s.PairPeer(k2, "Ana", ana, head2); err != nil {
		t.Errorf("a second key of the identity: %v", err)
	}
	// Another identity cannot have the name, nor a name without one.
	if _, err := s.PairPeer(k3, "Ana", bob, head1); err == nil || !strings.Contains(err.Error(), "is taken") {
		t.Errorf("one name for two identities: %v", err)
	}
	if _, err := s.PairPeer(k3, "", bob, head1); err == nil {
		t.Error("a peer with no name was paired")
	}
	if _, err := s.Pair(k3, "helper"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PairPeer(key(), "helper", bob, head1); err == nil || !strings.Contains(err.Error(), "is taken") {
		t.Errorf("an agent's name for a peer: %v", err)
	}
	l, _ := s.Load()
	if got := l.PeerLabel(ana); got != "Ana" {
		t.Errorf("PeerLabel(ana) = %q", got)
	}
	if got := l.PeerLabel(bob); got != "" {
		t.Errorf("PeerLabel(bob) = %q: no key of that identity is paired", got)
	}
	if heads := l.PeerHeads(ana); len(heads) != 2 || heads[0] == heads[1] {
		t.Errorf("PeerHeads(ana) = %v", heads)
	}
	if labels := l.AgentLabels(); labels[k1] != "peer Ana" || labels[k3] != "helper" {
		t.Errorf("labels: %v", labels)
	}
	// The name taken back: both keys, no label for the identity, and the
	// heads that the person saw stay on record.
	keys, err := s.Unpair("Ana")
	if err != nil || len(keys) != 2 {
		t.Fatalf("Unpair: %v, %v", keys, err)
	}
	l, _ = s.Load()
	if got := l.PeerLabel(ana); got != "" {
		t.Errorf("PeerLabel(ana) after the name was taken back = %q", got)
	}
	if !l.Unpaired(k1) || !l.Unpaired(k2) || l.Unpaired(k3) || l.Unpaired(key()) {
		t.Error("Unpaired is wrong for a key")
	}
	if heads := l.PeerHeads(ana); len(heads) != 2 {
		t.Errorf("PeerHeads(ana) after the name was taken back = %v", heads)
	}
	if labels := l.AgentLabels(); labels[k1] != "" || labels[k3] != "helper" {
		t.Errorf("labels after the name was taken back: %v", labels)
	}
}

// TestPeerUnlocksNoFile: a paired agent's record may name a file of the
// person. A peer's may not: a peer who knows the id of a deleted photo
// must not bring the photo back into an export.
func TestPeerUnlocksNoFile(t *testing.T) {
	for _, peer := range []bool{true, false} {
		s, id := initStore(t)
		photo, err := s.Add("photo", "rash", AddOptions{Photo: []byte("DELETED PHOTO"), PhotoType: "image/jpeg"})
		if err != nil {
			t.Fatal(err)
		}
		l, _ := s.Load()
		hash := l.Report.Records[photo].Blobs[0].Hash
		k := reKey(t)
		if peer {
			_, err = s.PairPeer(k.Public(), "Ana", "sha256:"+strings.Repeat("a", 64), "sha256:"+strings.Repeat("1", 64))
		} else {
			_, err = s.Pair(k.Public(), "phone")
		}
		if err != nil {
			t.Fatal(err)
		}
		o := reClaim(k, k.Public(), id, "note", map[string]any{"s": "x"})
		o["blobs"] = []any{map[string]any{"hash": hash, "role": "x", "media_type": "image/jpeg"}}
		rePlant(t, s, o, k)
		if _, err := s.Purge(photo, ""); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(t.TempDir(), "out")
		if _, err := s.ExportAll(dir); err != nil {
			t.Fatal(err)
		}
		_, rerr := os.ReadFile(filepath.Join(dir, "blobs", hash[7:]))
		if peer && rerr == nil {
			t.Errorf("a peer's record unlocked a photo that the person deleted")
		}
		if !peer && rerr != nil {
			t.Errorf("a paired agent's record no longer unlocks a file: %v", rerr)
		}
	}
}
