package main

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/bundle"
	"github.com/Conscious-Repository/bean/internal/reply"
)

// firstWords is the first line of an output that holds words: the six
// that open a package.
func firstWords(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "  ") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// seedOf holds the 24 words of each log that replyLogs made, by folder.
var seedOf = map[string]string{}

// replyLogs makes a person's log and a peer's log, two readings of the
// person, and a sealed share of them for the peer. It returns the two
// folders, the package with its words, and the ids of the readings and of
// the share record.
func replyLogs(t *testing.T) (ben, ana, pkg, words string, ids [3]string) {
	t.Helper()
	ben, ana = t.TempDir(), t.TempDir()
	for _, dir := range []string{ben, ana} {
		out, errs, code := run(t, dir, "", "init")
		if code != 0 {
			t.Fatalf("init: %s", errs)
		}
		seedOf[dir] = wordsFrom(out)
	}
	for i, a := range [][]string{
		{"add", "lab.cholesterol-total", "187", "-unit", "mg/dL", "-at", "2026-11-01"},
		{"add", "weight", "81.2", "-unit", "kg", "-at", "2026-11-01"},
	} {
		out, errs, code := run(t, ben, "", a...)
		if code != 0 {
			t.Fatalf("%v: %s", a, errs)
		}
		ids[i] = strings.TrimSpace(out)
	}
	pkg = filepath.Join(t.TempDir(), "for-ana.bean")
	out, errs, code := run(t, ben, "", "share", "-yes", "-seal", "-note", "what do you think?", "-out", pkg, ids[0], ids[1])
	if code != 0 {
		t.Fatalf("share: %d\n%s\n%s", code, out, errs)
	}
	words = firstWords(out)
	for _, line := range strings.Split(out, "\n") {
		if id, ok := strings.CutPrefix(line, "share record "); ok {
			ids[2] = id
		}
	}
	if len(strings.Fields(words)) != 6 || !core.IsID(ids[2]) {
		t.Fatalf("no words or no share record in:\n%s", out)
	}
	return ben, ana, pkg, words, ids
}

// idWords are the identity words of a log's person, as bean whoami shows
// them: what the peer reads out, and what a script gives to bean accept.
func idWords(t *testing.T, dir string) string {
	t.Helper()
	out, _, _ := run(t, dir, "", "whoami")
	for _, line := range strings.Split(out, "\n") {
		if w, ok := strings.CutPrefix(line, "identity words "); ok {
			return w
		}
	}
	t.Fatalf("no identity words in:\n%s", out)
	return ""
}

// writeReply runs bean reply as the peer and returns the reply's file and
// its words.
func writeReply(t *testing.T, ana, pkg, words string, pairs ...string) (string, string) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "reply.bean")
	stdout, errs, code := run(t, ana, words+"\n", append([]string{"reply", "-to", pkg, "-out", out}, pairs...)...)
	if code != 0 {
		t.Fatalf("reply: %d\n%s\n%s", code, stdout, errs)
	}
	return out, firstWords(stdout)
}

// TestReplyAndAccept: a peer signs notes about records of a share, and the
// person takes them into their log under a name. Only the notes go in.
func TestReplyAndAccept(t *testing.T) {
	ben, ana, pkg, words, ids := replyLogs(t)
	rep, rwords := writeReply(t, ana, pkg, words, ids[0][7:15], "Looks fine to me. Ask about ApoB.", ids[2], "Thanks for sending this.")
	anaWords := idWords(t, ana)
	// The peer's log keeps a record that they replied, and no note.
	if ls, _, _ := run(t, ana, "", "ls"); !strings.Contains(ls, "reply") || strings.Contains(ls, "ApoB") {
		t.Errorf("the peer's log:\n%s", ls)
	}
	// With no terminal and no -yes, nothing is taken: the list is shown.
	out, errs, code := run(t, ben, rwords+"\n", "accept", "-label", "Ana", rep)
	if code == 0 || !strings.Contains(errs, "there is no terminal to ask on") || !strings.Contains(out, anaWords) {
		t.Errorf("accept with no terminal: %d\n%s\n%s", code, out, errs)
	}
	if ls, _, _ := run(t, ben, "", "ls", "-type", "note"); ls != "" {
		t.Errorf("a note was taken without a yes:\n%s", ls)
	}
	// With no name, nothing is taken either. Nor with -yes and no words, or
	// with other words than the signer's.
	if _, errs, code := run(t, ben, rwords+"\n", "accept", "-yes", "-words", anaWords, rep); code == 0 || !strings.Contains(errs, "give -label <name>") {
		t.Errorf("accept with no label: %d\n%s", code, errs)
	}
	if _, errs, code := run(t, ben, rwords+"\n", "accept", "-yes", "-label", "Ana", rep); code == 0 || !strings.Contains(errs, "it needs -words") {
		t.Errorf("accept -yes with no words: %d\n%s", code, errs)
	}
	if _, errs, code := run(t, ben, rwords+"\n", "accept", "-yes", "-label", "Ana", "-words", idWords(t, ben), rep); code == 0 || !strings.Contains(errs, "are not the words that you gave") {
		t.Errorf("accept with other words: %d\n%s", code, errs)
	}
	if ls, _, _ := run(t, ben, "", "ls", "-all", "-json"); strings.Contains(ls, "agent.pair") {
		t.Errorf("a refused accept paired a key:\n%s", ls)
	}
	out, errs, code = run(t, ben, rwords+"\n", "accept", "-yes", "-label", "Ana", "-words", strings.ToUpper(anaWords), rep)
	if code != 0 {
		t.Fatalf("accept: %d\n%s\n%s", code, out, errs)
	}
	for _, want := range []string{
		"A reply to your share " + ids[2][7:19], `Your note on that share: "what do you think?"`, anaWords,
		"about lab.cholesterol-total 187 mg/dL (" + ids[0][7:19] + ")", "Looks fine to me. Ask about ApoB.", "about your share itself",
		"it is not medical advice", `took 2 notes of "Ana", already there 0`, "You have not seen this identity before",
		"bean cannot tell whether this is still that person's current key", "The reply's id starts ", "Ask the person whether they sent a reply whose id starts so",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("accept: no %q in\n%s", want, out)
		}
	}
	// The notes show under the name, the log is whole, and the peer's
	// identity records and reply record are not in it.
	ls, _, _ := run(t, ben, "", "ls")
	if strings.Count(ls, "[peer Ana]") != 2 || !strings.Contains(ls, "Looks fine to me. Ask about ApoB.") {
		t.Errorf("ls:\n%s", ls)
	}
	if out, _, code := run(t, ben, "", "verify"); code != 0 || !strings.Contains(out, " 0 invalid, 0 skipped, 0 held") {
		t.Errorf("verify: %d\n%s", code, out)
	}
	if all, _, _ := run(t, ben, "", "ls", "-all", "-json"); strings.Contains(all, `"type":"reply"`) || strings.Count(all, `"type":"bean.genesis"`) > 1 {
		t.Errorf("more than the notes went into the log:\n%s", all)
	}
	// A second time takes nothing, and needs no name.
	if out, _, code := run(t, ben, rwords+"\n", "accept", "-yes", "-words", anaWords, rep); code != 0 || !strings.Contains(out, "holds every note of this reply already") {
		t.Errorf("accept again: %d\n%s", code, out)
	}
	// A second reply of the same key needs no name, and takes no other.
	rep2, rwords2 := writeReply(t, ana, pkg, words, ids[1], "Same as last year?")
	if _, errs, code := run(t, ben, rwords2+"\n", "accept", "-yes", "-words", anaWords, "-label", "Bob", rep2); code == 0 || !strings.Contains(errs, `already paired as "Ana"`) {
		t.Errorf("another name for the same key: %d\n%s", code, errs)
	}
	if out, errs, code := run(t, ben, rwords2+"\n", "accept", "-yes", "-words", anaWords, rep2); code != 0 || !strings.Contains(out, `You paired this key before, as "Ana"`) || !strings.Contains(out, `took 1 notes of "Ana"`) {
		t.Errorf("a second reply: %d\n%s\n%s", code, out, errs)
	}
	// A note that the person deleted is not taken again from its reply.
	deleted := ""
	all, _, _ := run(t, ben, "", "ls", "-json", "-type", "note")
	for _, line := range strings.Split(all, "\n") {
		if strings.Contains(line, "Same as last year?") {
			deleted = line[strings.Index(line, `"id":"`)+6:][:71]
		}
	}
	if _, errs, code := run(t, ben, "", "purge", deleted); code != 0 {
		t.Fatalf("purge: %s", errs)
	}
	if out, _, code := run(t, ben, rwords2+"\n", "accept", "-yes", "-words", anaWords, rep2); code != 0 || !strings.Contains(out, "you deleted this note") || strings.Contains(out, "took sha256:") {
		t.Errorf("a deleted note was taken again: %d\n%s", code, out)
	}
	// bean show lists the notes about a record.
	if out, _, _ := run(t, ben, "", "show", ids[0]); !strings.Contains(out, "Looks fine to me. Ask about ApoB.") {
		t.Errorf("show does not list the note about the record:\n%s", out)
	}
}

// TestReplyRefuses: what a peer cannot reply to, or with.
func TestReplyRefuses(t *testing.T) {
	ben, ana, pkg, words, ids := replyLogs(t)
	other, _, _ := run(t, ana, "", "add", "weight", "70", "-unit", "kg")
	for name, tc := range map[string]struct {
		dir, stdin string
		args       []string
		want       string
	}{
		"one's own share":              {ben, words, []string{"reply", "-to", pkg, "-out", "x.bean", ids[0], "a note"}, "this share is your own"},
		"a record that was not shared": {ana, words, []string{"reply", "-to", pkg, "-out", "x.bean", strings.TrimSpace(other), "a note"}, "is not a record of this share"},
		"an id that is too short":      {ana, words, []string{"reply", "-to", pkg, "-out", "x.bean", ids[0][7:11], "a note"}, "is not an id"},
		"a note with no text":          {ana, words, []string{"reply", "-to", pkg, "-out", "x.bean", ids[0], " "}, "a note is a text"},
		"a note too long":              {ana, words, []string{"reply", "-to", pkg, "-out", "x.bean", ids[0], strings.Repeat("a", reply.MaxNote+1)}, "a note is a text"},
		"an id with no note":           {ana, words, []string{"reply", "-to", pkg, "-out", "x.bean", ids[0]}, "usage"},
		"no place to write":            {ana, words, []string{"reply", "-to", pkg, ids[0], "a note"}, "usage"},
		"other words":                  {ana, "abandon ability able about above absent", []string{"reply", "-to", pkg, "-out", "x.bean", ids[0], "a note"}, "does not pass bean check-bundle"},
		"a file that is there":         {ana, words, []string{"reply", "-to", pkg, "-out", pkg, ids[0], "a note"}, "is there already"},
	} {
		wd := t.TempDir()
		for i, a := range tc.args {
			if a == "x.bean" {
				tc.args[i] = filepath.Join(wd, a)
			}
		}
		out, errs, code := run(t, tc.dir, tc.stdin+"\n", tc.args...)
		if code == 0 || !strings.Contains(out+errs, tc.want) {
			t.Errorf("%s: exit %d, no %q in\n%s%s", name, code, tc.want, out, errs)
		}
		if _, err := os.Stat(filepath.Join(wd, "x.bean")); err == nil {
			t.Errorf("%s: a reply was written", name)
		}
	}
	if ls, _, _ := run(t, ana, "", "ls", "-type", "reply"); ls != "" {
		t.Errorf("a refused reply left a record:\n%s", ls)
	}
	// A log with no personal key cannot reply.
	if _, errs, code := run(t, t.TempDir(), words+"\n", "reply", "-to", pkg, "-out", filepath.Join(t.TempDir(), "x.bean"), ids[0], "a note"); code == 0 || !strings.Contains(errs, "bean init") {
		t.Errorf("reply with no log: %d\n%s", code, errs)
	}
}

// unpacked opens a sealed reply and returns its files by path.
func unpacked(t *testing.T, path, words string) map[string][]byte {
	t.Helper()
	sealed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	packed, err := bundle.Unseal(sealed, words)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(packed), int64(len(packed)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		files[f.Name], _ = io.ReadAll(r)
		r.Close()
	}
	return files
}

// sealed packs files as a reply and returns the file and its words.
func sealed(t *testing.T, files map[string][]byte) (string, string) {
	t.Helper()
	packed, err := bundle.PackFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	words, _ := bundle.NewWords()
	data, err := bundle.Seal(packed, words)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "forged.bean")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, words
}

// TestAcceptRefuses: a reply that holds more than notes about the share,
// or that someone else made, takes nothing into the log. Each forged reply
// is signed with the peer's real key, so every signature in it is good.
func TestAcceptRefuses(t *testing.T) {
	ben, ana, pkg, words, ids := replyLogs(t)
	rep, rwords := writeReply(t, ana, pkg, words, ids[0], "a first note", ids[1], "a second note")
	local, err := core.Open(ana).ReadLocal()
	if err != nil {
		t.Fatal(err)
	}
	key, anaID := local.Signer.Public(), local.Identity
	hidden, _, _ := run(t, ben, "", "add", "lab.hdl-cholesterol", "52", "-unit", "mg/dL")
	hidden = strings.TrimSpace(hidden)
	const stamp = "2026-11-12T10:00:00Z"
	// note signs a note of the peer's key about the person, then changes it.
	note := func(signer core.Signer, about string, change func(obj map[string]any)) (string, []byte) {
		obj := map[string]any{
			"v": 0.0, "nonce": core.NewNonce(), "type": "note", "subject": mustIdentity(t, ben), "author": signer.Public(), "kid": signer.Public(),
			"value": map[string]any{"s": "a forged note"}, "times": map[string]any{"observed": nil, "imported": stamp},
			"source": map[string]any{"system": reply.System, "adapter": core.Adapter, "ref": nil},
			"refs":   []any{map[string]any{"rel": "about", "id": about}},
		}
		if change != nil {
			change(obj)
		}
		id, data, err := core.SignRecord(obj, signer)
		if err != nil {
			t.Fatal(err)
		}
		return id, data
	}
	// forged builds a reply of the peer's identity records, the notes given
	// and a reply record that lists them.
	forged := func(to, share string, notes map[string][]byte, listed []string, extra map[string][]byte) (string, string) {
		files := map[string][]byte{}
		for path, data := range unpacked(t, rep, rwords) {
			if v, err := core.Parse(data); err == nil && strings.HasPrefix(v.(map[string]any)["type"].(string), "bean.") {
				files[path] = data
			}
		}
		var list []any
		for _, id := range listed {
			list = append(list, id)
		}
		for id, data := range notes {
			files[core.PathOf(id)] = data
		}
		id, data, err := core.SignRecord(map[string]any{
			"v": 0.0, "nonce": core.NewNonce(), "type": reply.Type, "subject": anaID, "author": anaID, "kid": key,
			"value": map[string]any{"n": float64(len(list)), "unit": "{note}"}, "times": map[string]any{"observed": nil, "imported": stamp},
			"source":  map[string]any{"system": reply.System, "adapter": core.Adapter, "ref": nil},
			"context": map[string]any{"to": to, "share": share, "notes": list},
		}, local.Signer)
		if err != nil {
			t.Fatal(err)
		}
		files[core.PathOf(id)] = data
		for path, data := range extra {
			files[path] = data
		}
		return sealed(t, files)
	}
	one := func(about string, change func(obj map[string]any)) (string, string) {
		id, data := note(local.Signer, about, change)
		return forged(mustIdentity(t, ben), ids[2], map[string][]byte{id: data}, []string{id}, nil)
	}
	_, seed, _ := ed25519.GenerateKey(nil)
	stranger := core.NewEd25519Signer(seed.Seed())
	benID := mustIdentity(t, ben)
	type forgery struct {
		make func() (string, string)
		want string
	}
	for name, f := range map[string]forgery{
		"the honest form passes": {func() (string, string) { return one(ids[0], nil) }, ""},
		"a lab value in place of a note": {func() (string, string) {
			return one(ids[0], func(o map[string]any) {
				o["type"], o["value"] = "lab.cholesterol-total", map[string]any{"n": 120.0, "unit": "mg/dL"}
			})
		}, "its type is not note"},
		"a note that corrects a record": {func() (string, string) {
			return one(ids[0], func(o map[string]any) { o["refs"] = []any{map[string]any{"rel": "supersedes", "id": ids[0]}} })
		}, ""},
		"a note about two records": {func() (string, string) {
			return one(ids[0], func(o map[string]any) {
				o["refs"] = []any{map[string]any{"rel": "about", "id": ids[0]}, map[string]any{"rel": "about", "id": ids[1]}}
			})
		}, "it is not about exactly one record"},
		"a note with more than a text": {func() (string, string) {
			return one(ids[0], func(o map[string]any) { o["context"] = map[string]any{"verified": "by a hospital"} })
		}, "it holds more than a text"},
		"a note with a number beside its text": {func() (string, string) {
			return one(ids[0], func(o map[string]any) { o["value"] = map[string]any{"s": "x", "n": 1.0} })
		}, "its value is not a text"},
		"a note with no text": {func() (string, string) {
			return one(ids[0], func(o map[string]any) { o["value"] = map[string]any{"s": ""} })
		}, "its value is not a text"},
		"a note about another person": {func() (string, string) {
			return one(ids[0], func(o map[string]any) { o["subject"] = anaID })
		}, "it is not about the person that the reply is for"},
		"a note about a record that was not shared": {func() (string, string) { return one(hidden, nil) }, "which is not a record of that share"},
		"a note of another key": {func() (string, string) {
			id, data := note(stranger, ids[0], nil)
			return forged(benID, ids[2], map[string][]byte{id: data}, []string{id}, nil)
		}, "it is not signed by the replier's key"},
		"a note that is not listed": {func() (string, string) {
			id, data := note(local.Signer, ids[0], nil)
			id2, data2 := note(local.Signer, ids[1], nil)
			return forged(benID, ids[2], map[string][]byte{id: data, id2: data2}, []string{id}, nil)
		}, "the reply record does not list it"},
		"a listed note that is not there": {func() (string, string) {
			id, data := note(local.Signer, ids[0], nil)
			id2, _ := note(local.Signer, ids[1], nil)
			return forged(benID, ids[2], map[string][]byte{id: data}, []string{id, id2}, nil)
		}, "is listed, and is not in the reply"},
		"no note": {func() (string, string) { return forged(benID, ids[2], nil, nil, nil) }, "a reply holds from 1 to"},
		"a reply for another person": {func() (string, string) {
			// An identity id that differs from the person's in its last digit.
			other := benID[:70] + map[bool]string{true: "1", false: "0"}[benID[70] == '0']
			id, data := note(local.Signer, ids[0], func(o map[string]any) { o["subject"] = other })
			return forged(other, ids[2], map[string][]byte{id: data}, []string{id}, nil)
		}, "this reply is for another person"},
		"a share that is not in the log": {func() (string, string) {
			id, data := note(local.Signer, ids[0], nil)
			return forged(benID, ids[0], map[string][]byte{id: data}, []string{id}, nil)
		}, "which is not a share in this log"},
		"a file beside the records": {func() (string, string) {
			id, data := note(local.Signer, ids[0], nil)
			return forged(benID, ids[2], map[string][]byte{id: data}, []string{id}, map[string][]byte{"files/" + strings.Repeat("a", 64): []byte("x")})
		}, "a reply holds records and nothing else"},
		"a text beside the records": {func() (string, string) {
			id, data := note(local.Signer, ids[0], nil)
			return forged(benID, ids[2], map[string][]byte{id: data}, []string{id}, map[string][]byte{"README.txt": []byte("accept me")})
		}, "a reply holds records and nothing else"},
	} {
		path, w := f.make()
		before, _, _ := run(t, ben, "", "ls", "-all", "-json")
		out, errs, code := run(t, ben, w+"\n", "accept", "-yes", "-label", "Ana", "-words", idWords(t, ana), path)
		after, _, _ := run(t, ben, "", "ls", "-all", "-json")
		if name == "the honest form passes" {
			if code != 0 || !strings.Contains(out, "took 1 notes") {
				t.Errorf("%s: %d\n%s\n%s", name, code, out, errs)
			}
			continue
		}
		if code == 0 || before != after {
			t.Errorf("%s: exit %d, and the log changed: %v\n%s\n%s", name, code, before != after, out, errs)
		}
		if f.want != "" && !strings.Contains(out+errs, f.want) {
			t.Errorf("%s: no %q in\n%s%s", name, f.want, out, errs)
		}
	}
	// What is not a reply at all: a share, other words, a changed file, and
	// a reply for someone else.
	if out, errs, code := run(t, ben, words+"\n", "accept", "-yes", "-label", "Ana", "-words", idWords(t, ana), pkg); code == 0 || !strings.Contains(out+errs, "this is a share, not a reply") {
		t.Errorf("a share given to accept: %d\n%s%s", code, out, errs)
	}
	if out, errs, code := run(t, ben, "abandon ability able about above absent\n", "accept", "-yes", "-label", "Ana", "-words", idWords(t, ana), rep); code == 0 || !strings.Contains(out+errs, "the reply did not open") {
		t.Errorf("other words: %d\n%s%s", code, out, errs)
	}
	carol := t.TempDir()
	run(t, carol, "", "init")
	if out, errs, code := run(t, carol, rwords+"\n", "accept", "-yes", "-label", "Ana", "-words", idWords(t, ana), rep); code == 0 || !strings.Contains(out+errs, "this reply is for another person") {
		t.Errorf("a reply for someone else: %d\n%s%s", code, out, errs)
	}
	// A reply is not a bundle, and check-bundle says what it is.
	if out, errs, code := run(t, t.TempDir(), rwords+"\n", "check-bundle", rep); code == 0 || !strings.Contains(out+errs, "bean accept") {
		t.Errorf("check-bundle on a reply: %d\n%s%s", code, out, errs)
	}
}

func mustIdentity(t *testing.T, dir string) string {
	t.Helper()
	id, err := core.Open(dir).Identity()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// TestAcceptAfterAKeyChange: the person's log keeps how far a peer's chain
// had got when they saw it. A peer who changed their key keeps their name.
// A key that the change retired, in a thief's hands, shows the peer's real
// identity words, and takes nothing once the person has seen the change.
func TestAcceptAfterAKeyChange(t *testing.T) {
	ben, _, pkg, words, ids := replyLogs(t)
	ana := t.TempDir()
	out, _, _ := run(t, ana, "", "init")
	seed, anaWords := wordsFrom(out), idWords(t, ana)
	accept := func(reply, rwords string, more ...string) (string, string, int) {
		return run(t, ben, rwords+"\n", append(append([]string{"accept", "-yes", "-words", anaWords}, more...), reply)...)
	}
	rep, rwords := writeReply(t, ana, pkg, words, ids[0], "before the change")
	if out, errs, code := accept(rep, rwords, "-label", "Ana"); code != 0 {
		t.Fatalf("accept: %d\n%s\n%s", code, out, errs)
	}
	// A thief's copy of the peer's log, with the key that the change retires.
	stolen := t.TempDir()
	if err := os.CopyFS(stolen, os.DirFS(ana)); err != nil {
		t.Fatal(err)
	}
	// Before the person knows of any change, the thief's reply passes: the
	// key is the one that the person paired. The review says so.
	early, earlyWords := writeReply(t, stolen, pkg, words, ids[0], "from the copy, before the change")
	if out, _, code := accept(early, earlyWords); code != 0 || !strings.Contains(out, "bean cannot tell whether this is still that person's current key") {
		t.Errorf("a reply of the paired key: %d\n%s", code, out)
	}
	if _, errs, code := run(t, ana, seed, "rotate", "-seal-all"); code != 0 {
		t.Fatalf("rotate: %s", errs)
	}
	// The peer's new key: the reply shows the change on the chain.
	rep2, rwords2 := writeReply(t, ana, pkg, words, ids[1], "after the change")
	out, errs, code := accept(rep2, rwords2)
	if code != 0 || !strings.Contains(out, `You paired this identity before, as "Ana". This is another key`) || !strings.Contains(out, `took 1 notes of "Ana"`) {
		t.Errorf("a reply after a key change: %d\n%s\n%s", code, out, errs)
	}
	// Another name for the same identity is refused.
	rep3, rwords3 := writeReply(t, ana, pkg, words, ids[1], "once more")
	if _, errs, code := accept(rep3, rwords3, "-label", "Anna"); code == 0 || !strings.Contains(errs, `already paired as "Ana"`) {
		t.Errorf("a second name: %d\n%s", code, errs)
	}
	// The thief's key signs a reply with the chain as it was before the
	// change, and with the peer's real identity words. The person has seen
	// the change, so it takes nothing: whether or not they paired that key.
	late, lateWords := writeReply(t, stolen, pkg, words, ids[0], "from a stolen key")
	out, errs, code = accept(late, lateWords)
	if code == 0 || !strings.Contains(errs, "as it was before a change of key that you have seen") {
		t.Errorf("a reply of a retired key: %d\n%s\n%s", code, out, errs)
	}
	if ls, _, _ := run(t, ben, "", "ls"); strings.Contains(ls, "from a stolen key") || strings.Count(ls, "[peer Ana]") != 3 {
		t.Errorf("ls:\n%s", ls)
	}
	if out, _, code := run(t, ben, "", "verify"); code != 0 || !strings.Contains(out, " 0 invalid, 0 skipped, 0 held") {
		t.Errorf("verify: %d\n%s", code, out)
	}
}

// TestNamesAndUnpair: a name belongs to one identity, a stranger's reply
// changes nothing of another peer's pairing, and the person can take a
// name back.
func TestNamesAndUnpair(t *testing.T) {
	ben, ana, pkg, words, ids := replyLogs(t)
	rep, rwords := writeReply(t, ana, pkg, words, ids[0], "from Ana")
	if out, errs, code := run(t, ben, rwords+"\n", "accept", "-yes", "-label", "Ana", "-words", idWords(t, ana), rep); code != 0 {
		t.Fatalf("accept: %d\n%s\n%s", code, out, errs)
	}
	// A second person cannot come in under the first one's name.
	mallory := t.TempDir()
	run(t, mallory, "", "init")
	rep2, rwords2 := writeReply(t, mallory, pkg, words, ids[0], "from someone else")
	if _, errs, code := run(t, ben, rwords2+"\n", "accept", "-yes", "-label", "Ana", "-words", idWords(t, mallory), rep2); code == 0 || !strings.Contains(errs, `the name "Ana" is taken`) {
		t.Errorf("one name for two people: %d\n%s", code, errs)
	}
	if out, errs, code := run(t, ben, rwords2+"\n", "accept", "-yes", "-label", "Mallory", "-words", idWords(t, mallory), rep2); code != 0 {
		t.Fatalf("accept under another name: %d\n%s\n%s", code, out, errs)
	}
	// It did nothing to the first person's pairing.
	rep3, rwords3 := writeReply(t, ana, pkg, words, ids[1], "from Ana again")
	if out, errs, code := run(t, ben, rwords3+"\n", "accept", "-yes", "-words", idWords(t, ana), rep3); code != 0 || !strings.Contains(out, `took 1 notes of "Ana"`) {
		t.Errorf("the first peer after a stranger's reply: %d\n%s\n%s", code, out, errs)
	}
	ls, _, _ := run(t, ben, "", "ls")
	if strings.Count(ls, "[peer Ana]") != 2 || strings.Count(ls, "[peer Mallory]") != 1 {
		t.Errorf("ls:\n%s", ls)
	}
	// The person takes their word back from a name. A later reply of that
	// key takes nothing, and the notes that it signed stay, with no name.
	if out, errs, code := run(t, ben, "", "unpair", "Ana"); code != 0 || strings.Count(out, "unpaired ed25519:") != 1 {
		t.Errorf("unpair: %d\n%s\n%s", code, out, errs)
	}
	if _, errs, code := run(t, ben, "", "unpair", "Ana"); code == 0 || !strings.Contains(errs, `no key is paired as "Ana"`) {
		t.Errorf("unpair again: %d\n%s", code, errs)
	}
	rep4, rwords4 := writeReply(t, ana, pkg, words, ids[0], "after the person took their word back")
	if _, errs, code := run(t, ben, rwords4+"\n", "accept", "-yes", "-label", "Ana", "-words", idWords(t, ana), rep4); code == 0 || !strings.Contains(errs, "you unpaired the key that signed this reply") {
		t.Errorf("a reply of an unpaired key: %d\n%s", code, errs)
	}
	if ls, _, _ := run(t, ben, "", "ls"); strings.Contains(ls, "[peer Ana]") || !strings.Contains(ls, "from Ana again") || !strings.Contains(ls, "[peer Mallory]") {
		t.Errorf("ls after unpair:\n%s", ls)
	}
}

// TestAcceptedNotesStay: a peer's notes are records of the person's log
// like any other. They stay through the person's own change of key, they
// are in an export, and they travel in a share with the name that the
// person gave their writer.
func TestAcceptedNotesStay(t *testing.T) {
	ben, ana, pkg, words, ids := replyLogs(t)
	rep, rwords := writeReply(t, ana, pkg, words, ids[0], "Looks fine to me.")
	if out, errs, code := run(t, ben, rwords+"\n", "accept", "-yes", "-label", "Ana", "-words", idWords(t, ana), rep); code != 0 {
		t.Fatalf("accept: %d\n%s\n%s", code, out, errs)
	}
	if out, errs, code := run(t, ben, seedOf[ben], "rotate", "-seal-all"); code != 0 {
		t.Fatalf("rotate: %d\n%s\n%s", code, out, errs)
	}
	if ls, _, _ := run(t, ben, "", "ls", "-type", "note"); !strings.Contains(ls, "Looks fine to me.") || !strings.Contains(ls, "[peer Ana]") {
		t.Errorf("after the person's change of key:\n%s", ls)
	}
	if out, _, code := run(t, ben, "", "verify"); code != 0 || !strings.Contains(out, " 0 invalid, 0 skipped, 0 held") {
		t.Errorf("verify: %d\n%s", code, out)
	}
	note, _, _ := run(t, ben, "", "ls", "-json", "-type", "note")
	noteID := note[strings.Index(note, `"id":"`)+6:][:71]
	export := filepath.Join(t.TempDir(), "export")
	if out, errs, code := run(t, ben, "", "export", export); code != 0 {
		t.Fatalf("export: %d\n%s\n%s", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(export, "records", noteID[7:]+".json")); err != nil {
		t.Errorf("the note is not in the export: %v", err)
	}
	// Shared on, the note shows under the name, and the key is named.
	dir := filepath.Join(t.TempDir(), "onward")
	if out, errs, code := run(t, ben, "", "share", "-yes", "-out", dir, noteID, ids[0]); code != 0 {
		t.Fatalf("share: %d\n%s\n%s", code, out, errs)
	}
	checked, _, code := run(t, t.TempDir(), "", "check-bundle", dir)
	if code != 0 || !strings.Contains(checked, "[peer Ana]") || !strings.Contains(checked, "paired by the person: peer Ana, key ed25519:") {
		t.Errorf("the note shared on: %d\n%s", code, checked)
	}
}
