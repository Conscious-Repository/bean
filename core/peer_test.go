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
	// A name that differs only in its capital letters is the same name:
	// no second person takes it, and no agent.
	if _, err := s.PairPeer(key(), "ana", bob, head1); err == nil || !strings.Contains(err.Error(), "is taken") {
		t.Errorf("a second person as ana: %v", err)
	}
	if _, err := s.Pair(key(), "ANA"); err == nil || !strings.Contains(err.Error(), "is the name of a peer") {
		t.Errorf("an agent as ANA: %v", err)
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
	if labels := l.PairedLabels(); labels[k1] != "peer Ana" || labels[k3] != "helper" {
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
	if labels := l.PairedLabels(); labels[k1] != "" || labels[k3] != "helper" {
		t.Errorf("labels after the name was taken back: %v", labels)
	}
	// A key that the person unpaired is not paired again, under any name.
	if _, err := s.PairPeer(k1, "Ana", ana, head1); err == nil || !strings.Contains(err.Error(), "you unpaired this key") {
		t.Errorf("an unpaired key paired again: %v", err)
	}
	// The one writer of a pairing refuses a name that bean cannot show,
	// for a peer and for an agent.
	if _, err := s.PairPeer(key(), "x], [peer Bob", bob, head1); err == nil || !strings.Contains(err.Error(), "is not a name that bean can show") {
		t.Errorf("a peer under a name of two: %v", err)
	}
	if _, err := s.Pair(key(), "peer Ana"); err == nil || !strings.Contains(err.Error(), "is not a name that bean can show") {
		t.Errorf("an agent that reads as a peer: %v", err)
	}
}

// TestKeyPairedBothWays: two of the person's machines can pair one key as
// an agent and as a peer. It is then the person's agent for a reply, which
// takes nothing, and a peer for files, which it does not open.
func TestKeyPairedBothWays(t *testing.T) {
	s, _ := initStore(t)
	k := reKey(t).Public()
	l, err := s.load(false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pairWith(l, k, "phone", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pairWith(l, k, "Ana", "sha256:"+strings.Repeat("a", 64), "sha256:"+strings.Repeat("1", 64)); err != nil {
		t.Fatal(err)
	}
	if l, err = s.Load(); err != nil {
		t.Fatal(err)
	}
	if got := l.PairedAsAgent(k); got != "phone" {
		t.Errorf("PairedAsAgent: %q, want phone", got)
	}
	if got := l.FileLabels()[k]; got != "" {
		t.Errorf("the key opens files as %q", got)
	}
	if got := l.PairedAsAgent(reKey(t).Public()); got != "" {
		t.Errorf("a key that is not paired is an agent %q", got)
	}
}

// TestPeerUnlocksNoFile: a paired agent's record may name a file of the
// person. A peer's may not: a peer who knows the id of a deleted photo
// must not bring the photo back into an export. A key that is paired as
// both counts as a peer. The same holds in a log with records of a newer
// bean, where the pairings are read from records that this build cannot
// judge.
func TestPeerUnlocksNoFile(t *testing.T) {
	pair := func(s *Store, key, label, peerOf, head string) {
		t.Helper()
		l, err := s.load(false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.pairWith(l, key, label, peerOf, head); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name        string
		agent, peer bool
		tooNew      bool
		unlocks     bool
	}{
		{"an agent", true, false, false, true},
		{"a peer", false, true, false, false},
		{"an agent and a peer", true, true, false, false},
		{"an agent, newer bean", true, false, true, true},
		{"a peer, newer bean", false, true, true, false},
		{"an agent and a peer, newer bean", true, true, true, false},
	} {
		s, id, words := initWithWords(t)
		photo, err := s.Add("photo", "rash", AddOptions{Photo: []byte("DELETED PHOTO"), PhotoType: "image/jpeg"})
		if err != nil {
			t.Fatal(err)
		}
		l, _ := s.Load()
		hash := l.Report.Records[photo].Blobs[0].Hash
		k := reKey(t)
		if tc.agent {
			// Written as a second machine of the person would: Pair and
			// PairPeer each refuse a key that the other paired.
			pair(s, k.Public(), "phone", "", "")
		}
		if tc.peer {
			pair(s, k.Public(), "Ana", "sha256:"+strings.Repeat("a", 64), "sha256:"+strings.Repeat("1", 64))
		}
		o := reClaim(k, k.Public(), id, "note", map[string]any{"s": "x"})
		o["blobs"] = []any{map[string]any{"hash": hash, "role": "x", "media_type": "image/jpeg"}}
		rePlant(t, s, o, k)
		if _, err := s.Purge(photo, ""); err != nil {
			t.Fatal(err)
		}
		if tc.tooNew {
			seed, err := WordsToSeed(words)
			if err != nil {
				t.Fatal(err)
			}
			futureRecord(t, s, id, RecoverySigner(seed, 0))
			mustTooNew(t, s, id, true)
		}
		dir := filepath.Join(t.TempDir(), "out")
		// An export of a log with a newer bean's records writes all, and
		// then says that an update is needed.
		if res, err := s.ExportAll(dir); !res.Written || (err != nil) != tc.tooNew {
			t.Fatalf("%s: export: %v", tc.name, err)
		}
		if _, rerr := os.ReadFile(filepath.Join(dir, "blobs", hash[7:])); (rerr == nil) != tc.unlocks {
			t.Errorf("%s: the record unlocks the file: %v, want %v", tc.name, rerr == nil, tc.unlocks)
		}
	}
}

// TestCheckLabel: a name of a paired key cannot fake a second name, hide a
// character, or read as the mark of another person. A label that breaks
// the rule and is in a record all the same shows quoted.
func TestCheckLabel(t *testing.T) {
	for _, good := range []string{"Ana", "Dr. O'Neil", "claude-code", "phone_2", "agent@bens-laptop.local", "a", strings.Repeat("a", MaxLabel)} {
		if err := CheckLabel(good); err != nil {
			t.Errorf("%q: %v", good, err)
		}
		if ShownLabel(good) != good {
			t.Errorf("%q shows as %s", good, ShownLabel(good))
		}
	}
	for _, bad := range []string{"", " ", " Ana", "Ana ", "A  na", "x], [peer Bob", "a,b", "peer", "Peer Ana", "peer Ana", "A\u200bna", "\u0410na", "Jos\u00e9", "a\nb", "a\x1b[2J", strings.Repeat("a", MaxLabel+1)} {
		if err := CheckLabel(bad); err == nil {
			t.Errorf("%q passed", bad)
		}
		if bad != "" && (ShownLabel(bad) == bad || !strings.HasPrefix(ShownLabel(bad), `"`)) {
			t.Errorf("%q shows as %s", bad, ShownLabel(bad))
		}
	}
	if ShownLabel("") != "agent" {
		t.Errorf("no label shows as %q", ShownLabel(""))
	}
	// A record of an older bean, or one written by hand, can hold such a
	// label. It shows quoted wherever a name shows.
	s, _ := initStore(t)
	k := reKey(t).Public()
	if _, err := s.Append(strings.NewReader(`{"type":"agent.pair","value":{"key":"` + k + `","label":"peer Ana"},"times":{"observed":null},"source":{"system":"self","adapter":"t@1","ref":null}}` + "\n")); err != nil {
		t.Fatal(err)
	}
	l, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := l.PairedLabels()[k]; got != `"peer Ana"` {
		t.Errorf("an agent labelled peer Ana shows as %s", got)
	}
}

// TestPairingsOfTwoCopies: two copies of a log can each write a pairing
// that the other would have refused. When the copies come together, an
// unpairing holds over a peer's pairing of the same key, and two people
// under one name show apart.
func TestPairingsOfTwoCopies(t *testing.T) {
	a, _ := initStore(t)
	ana, bob := "sha256:"+strings.Repeat("a", 64), "sha256:"+strings.Repeat("b", 64)
	head := "sha256:" + strings.Repeat("1", 64)
	k1, k2 := reKey(t).Public(), reKey(t).Public()
	// A copy of the log, made before anything was paired.
	stale := Open(t.TempDir())
	if err := os.CopyFS(stale.Dir, os.DirFS(a.Dir)); err != nil {
		t.Fatal(err)
	}
	// On the first copy: Ana is paired, and then unpaired.
	if _, err := a.PairPeer(k1, "Ana", ana, head); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Unpair("Ana"); err != nil {
		t.Fatal(err)
	}
	// On the stale copy: the same key is paired, and another person takes
	// the name Bo. The first copy gives that name to someone else.
	if _, err := stale.PairPeer(k1, "Ana", ana, head); err != nil {
		t.Fatal(err)
	}
	if _, err := stale.PairPeer(k2, "Bo", bob, head); err != nil {
		t.Fatal(err)
	}
	k3 := reKey(t).Public()
	carol := "sha256:" + strings.Repeat("c", 64)
	if _, err := a.PairPeer(k3, "bo", carol, head); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Sync(stale); err != nil {
		t.Fatal(err)
	}
	l, err := a.Load()
	if err != nil {
		t.Fatal(err)
	}
	labels := l.PairedLabels()
	if labels[k1] != "" || !l.Unpaired(k1) {
		t.Errorf("a peer that was unpaired is paired again by the other copy: %q", labels[k1])
	}
	if labels[k2] != "peer Bo #bbbbbb" || labels[k3] != "peer bo #cccccc" {
		t.Errorf("two people under one name: %q and %q", labels[k2], labels[k3])
	}
	if names := l.PeerNames(); names[k2] != "Bo" || names[k3] != "bo" || names[k1] != "" {
		t.Errorf("PeerNames: %v", names)
	}
	if shown := l.PeerShown(); shown[k2] != "Bo #bbbbbb" || shown[k3] != "bo #cccccc" || shown[k1] != "" {
		t.Errorf("PeerShown: %v", shown)
	}
}

// TestOnlyThePersonPairs: a pairing that an agent's key signs has no
// effect, so the agent's key does not write one.
func TestOnlyThePersonPairs(t *testing.T) {
	s, _ := initStore(t)
	if _, _, err := s.AgentInit("", "bot"); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"agent.pair","value":{"key":"` + reKey(t).Public() + `","label":"Doctor"},"times":{"observed":null},"source":{"system":"self","adapter":"t@1","ref":null}}` + "\n"
	if _, err := s.AppendAsAgent(strings.NewReader(line)); err == nil || !strings.Contains(err.Error(), "only the person pairs a key") {
		t.Errorf("an agent's key wrote a pairing: %v", err)
	}
	if _, err := s.Append(strings.NewReader(line)); err != nil {
		t.Errorf("the person's own pairing: %v", err)
	}
}
