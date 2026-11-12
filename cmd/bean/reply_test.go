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

	"filippo.io/age"

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
		"one's own share":                {ben, words, []string{"reply", "-to", pkg, "-out", "x.bean", ids[0], "a note"}, "this share is your own"},
		"a record that was not shared":   {ana, words, []string{"reply", "-to", pkg, "-out", "x.bean", strings.TrimSpace(other), "a note"}, "is not a record of this share"},
		"an id that is too short":        {ana, words, []string{"reply", "-to", pkg, "-out", "x.bean", ids[0][7:11], "a note"}, "is not an id"},
		"a note with no text":            {ana, words, []string{"reply", "-to", pkg, "-out", "x.bean", ids[0], " "}, "a note is a text"},
		"a note too long":                {ana, words, []string{"reply", "-to", pkg, "-out", "x.bean", ids[0], strings.Repeat("a", reply.MaxNote+1)}, "a note is a text"},
		"a note that starts with a dash": {ana, words, []string{"reply", "-to", pkg, "-out", "x.bean", ids[0], "-x"}, "Put -- before the first id"},
		"an id with no note":             {ana, words, []string{"reply", "-to", pkg, "-out", "x.bean", ids[0]}, "usage"},
		"no place to write":              {ana, words, []string{"reply", "-to", pkg, ids[0], "a note"}, "usage"},
		"other words":                    {ana, "abandon ability able about above absent", []string{"reply", "-to", pkg, "-out", "x.bean", ids[0], "a note"}, "what you reply to did not open"},
		"a place inside the log":         {ana, words, []string{"reply", "-to", pkg, "-out", filepath.Join(ana, "objects", "x.bean"), ids[0], "a note"}, "is inside your log"},
		"a file that is there":           {ana, words, []string{"reply", "-to", pkg, "-out", pkg, ids[0], "a note"}, "is there already"},
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
	// A forged package is sealed at a low cost: bean opens any cost up to
	// its own, and the many forged replies here would else take minutes
	// under the race detector. What bean reply writes keeps the full cost.
	to, err := age.NewScryptRecipient(words)
	if err != nil {
		t.Fatal(err)
	}
	to.SetWorkFactor(10)
	var data bytes.Buffer
	w, err := age.Encrypt(&data, to)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(packed); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "forged.bean")
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
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
	honest := unpacked(t, rep, rwords)
	forged := func(to, share string, notes map[string][]byte, listed []string, extra map[string][]byte, tweak ...func(obj map[string]any)) map[string][]byte {
		files := map[string][]byte{}
		for path, data := range honest {
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
		manifest := map[string]any{
			"v": 0.0, "nonce": core.NewNonce(), "type": reply.Type, "subject": anaID, "author": anaID, "kid": key,
			"value": map[string]any{"n": float64(len(list)), "unit": "{note}"}, "times": map[string]any{"observed": nil, "imported": stamp},
			"source":  map[string]any{"system": reply.System, "adapter": core.Adapter, "ref": nil},
			"context": map[string]any{"to": to, "share": share, "notes": list},
		}
		for _, change := range tweak {
			change(manifest)
		}
		id, data, err := core.SignRecord(manifest, local.Signer)
		if err != nil {
			t.Fatal(err)
		}
		files[core.PathOf(id)] = data
		for path, data := range extra {
			files[path] = data
		}
		return files
	}
	one := func(about string, change func(obj map[string]any)) map[string][]byte {
		id, data := note(local.Signer, about, change)
		return forged(mustIdentity(t, ben), ids[2], map[string][]byte{id: data}, []string{id}, nil)
	}
	_, seed, _ := ed25519.GenerateKey(nil)
	stranger := core.NewEd25519Signer(seed.Seed())
	benID := mustIdentity(t, ben)
	viaAccept := map[string]bool{"a lab value in place of a note": true, "a note that corrects a record": true, "a text beside the records": true}
	type forgery struct {
		make func() map[string][]byte
		want string
	}
	for name, f := range map[string]forgery{
		"the honest form passes": {func() map[string][]byte { return one(ids[0], nil) }, ""},
		"a lab value in place of a note": {func() map[string][]byte {
			return one(ids[0], func(o map[string]any) {
				o["type"], o["value"] = "lab.cholesterol-total", map[string]any{"n": 120.0, "unit": "mg/dL"}
			})
		}, "its type is not note"},
		"a note that corrects a record": {func() map[string][]byte {
			return one(ids[0], func(o map[string]any) { o["refs"] = []any{map[string]any{"rel": "supersedes", "id": ids[0]}} })
		}, ""},
		"a note about two records": {func() map[string][]byte {
			return one(ids[0], func(o map[string]any) {
				o["refs"] = []any{map[string]any{"rel": "about", "id": ids[0]}, map[string]any{"rel": "about", "id": ids[1]}}
			})
		}, "it is not about exactly one record"},
		"a note with a file": {func() map[string][]byte {
			return one(ids[0], func(o map[string]any) {
				o["blobs"] = []any{map[string]any{"hash": "sha256:" + strings.Repeat("b", 64), "role": "photo", "media_type": "image/png"}}
			})
		}, "it holds more than a text"},
		"a note with more than a text": {func() map[string][]byte {
			return one(ids[0], func(o map[string]any) { o["context"] = map[string]any{"verified": "by a hospital"} })
		}, "it holds more than a text"},
		"a note with a number beside its text": {func() map[string][]byte {
			return one(ids[0], func(o map[string]any) { o["value"] = map[string]any{"s": "x", "n": 1.0} })
		}, "its value is not a text"},
		"a note with no text": {func() map[string][]byte {
			return one(ids[0], func(o map[string]any) { o["value"] = map[string]any{"s": ""} })
		}, "its value is not a text"},
		"a note about another person": {func() map[string][]byte {
			return one(ids[0], func(o map[string]any) { o["subject"] = anaID })
		}, "it is not about the person that the reply is for"},
		"a note about a record that was not shared": {func() map[string][]byte { return one(hidden, nil) }, "which is not a record of that share"},
		"a note of another key": {func() map[string][]byte {
			id, data := note(stranger, ids[0], nil)
			return forged(benID, ids[2], map[string][]byte{id: data}, []string{id}, nil)
		}, "it is not signed by the replier's key"},
		"a note that is not listed": {func() map[string][]byte {
			id, data := note(local.Signer, ids[0], nil)
			id2, data2 := note(local.Signer, ids[1], nil)
			return forged(benID, ids[2], map[string][]byte{id: data, id2: data2}, []string{id}, nil)
		}, "the reply record does not list it"},
		"a listed note that is not there": {func() map[string][]byte {
			id, data := note(local.Signer, ids[0], nil)
			id2, _ := note(local.Signer, ids[1], nil)
			return forged(benID, ids[2], map[string][]byte{id: data}, []string{id, id2}, nil)
		}, "is listed, and is not in the reply"},
		"no note": {func() map[string][]byte { return forged(benID, ids[2], nil, nil, nil) }, "a reply holds from 1 to"},
		"a reply for another person": {func() map[string][]byte {
			// An identity id that differs from the person's in its last digit.
			other := benID[:70] + map[bool]string{true: "1", false: "0"}[benID[70] == '0']
			id, data := note(local.Signer, ids[0], func(o map[string]any) { o["subject"] = other })
			return forged(other, ids[2], map[string][]byte{id: data}, []string{id}, nil)
		}, "this reply is for another person"},
		"a share that is not in the log": {func() map[string][]byte {
			id, data := note(local.Signer, ids[0], nil)
			return forged(benID, ids[0], map[string][]byte{id: data}, []string{id}, nil)
		}, "which is not a share in this log"},
		"a count that is not the number of notes": {func() map[string][]byte {
			id, data := note(local.Signer, ids[0], nil)
			return forged(benID, ids[2], map[string][]byte{id: data}, []string{id}, nil, func(o map[string]any) { o["value"] = map[string]any{"n": 2.0, "unit": "{note}"} })
		}, "the reply record's lists are damaged"},
		"a reply record of another source": {func() map[string][]byte {
			id, data := note(local.Signer, ids[0], nil)
			return forged(benID, ids[2], map[string][]byte{id: data}, []string{id}, nil, func(o map[string]any) {
				o["source"] = map[string]any{"system": "a hospital", "adapter": core.Adapter, "ref": nil}
			})
		}, "as bean reply writes it"},
		"a reply for its own writer": {func() map[string][]byte {
			id, data := note(local.Signer, ids[0], func(o map[string]any) { o["subject"] = anaID })
			return forged(anaID, ids[2], map[string][]byte{id: data}, []string{id}, nil)
		}, "the reply is for the identity that wrote it"},
		"a note that is too long": {func() map[string][]byte {
			return one(ids[0], func(o map[string]any) { o["value"] = map[string]any{"s": strings.Repeat("a", reply.MaxNote+1)} })
		}, "its value is not a text"},
		"a note of spaces": {func() map[string][]byte {
			return one(ids[0], func(o map[string]any) { o["value"] = map[string]any{"s": "  \n "} })
		}, "its value is not a text"},
		"a note of another source": {func() map[string][]byte {
			return one(ids[0], func(o map[string]any) {
				o["source"] = map[string]any{"system": "lab \x1b[2J", "adapter": core.Adapter, "ref": nil}
			})
		}, "its source is not the one that bean reply writes"},
		"a note that places itself in the past": {func() map[string][]byte {
			return one(ids[0], func(o map[string]any) {
				o["times"] = map[string]any{"observed": map[string]any{"start": "2020-01-01T00:00:00Z", "precision": "day"}, "imported": stamp}
			})
		}, "it states another time than the time it was written"},
		"a note from the future": {func() map[string][]byte {
			return one(ids[0], func(o map[string]any) {
				o["times"] = map[string]any{"observed": nil, "imported": "2999-01-01T00:00:00Z"}
			})
		}, "which is later than now"},
		"more notes than a reply holds": {func() map[string][]byte {
			notes, listed := map[string][]byte{}, []string{}
			for range reply.MaxNotes + 1 {
				id, data := note(local.Signer, ids[0], nil)
				notes[id], listed = data, append(listed, id)
			}
			return forged(benID, ids[2], notes, listed, nil)
		}, "a reply holds from 1 to"},
		"an identity record of someone else": {func() map[string][]byte {
			id, data := note(local.Signer, ids[0], nil)
			extra := map[string][]byte{}
			for path, data := range unpacked(t, pkg, words) {
				if v, err := core.Parse(data); err == nil && v.(map[string]any)["type"] == "bean.genesis" {
					extra[path] = data
				}
			}
			return forged(benID, ids[2], map[string][]byte{id: data}, []string{id}, extra)
		}, "is an identity record that is not on the replier's chain"},
		"a file beside the records": {func() map[string][]byte {
			id, data := note(local.Signer, ids[0], nil)
			return forged(benID, ids[2], map[string][]byte{id: data}, []string{id}, map[string][]byte{"files/" + strings.Repeat("a", 64): []byte("x")})
		}, "a reply holds records and nothing else"},
		"a text beside the records": {func() map[string][]byte {
			id, data := note(local.Signer, ids[0], nil)
			return forged(benID, ids[2], map[string][]byte{id: data}, []string{id}, map[string][]byte{"README.txt": []byte("accept me")})
		}, "a reply holds records and nothing else"},
	} {
		files := f.make()
		// Most forged replies break a rule of the reply's own check. That
		// check runs here, without a run of the program for each: under
		// the race detector a run takes seconds. A few go through bean
		// accept all the same, and so does each reply that the check
		// passes: bean accept refuses those by what only the log can tell.
		if res := reply.Check(bundle.FromFiles("forged", files)); !res.OK && !viaAccept[name] {
			if all := strings.Join(res.Problems, "\n"); !strings.Contains(all, f.want) {
				t.Errorf("%s: no %q in\n%s", name, f.want, all)
			}
			continue
		}
		path, w := sealed(t, files)
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
	// A reply that is not sealed: a zip file of the same records.
	plain := filepath.Join(t.TempDir(), "plain.zip")
	packed, err := bundle.PackFiles(unpacked(t, rep, rwords))
	if err != nil || os.WriteFile(plain, packed, 0o600) != nil {
		t.Fatal(err)
	}
	if out, errs, code := run(t, ben, "", "accept", "-yes", "-label", "Ana", "-words", idWords(t, ana), plain); code == 0 || !strings.Contains(out+errs, "this is not a sealed file") {
		t.Errorf("a reply that is not sealed: %d\n%s%s", code, out, errs)
	}
	if out, errs, code := run(t, ben, "", "accept", "-yes", "-label", "Ana", "-words", idWords(t, ana), t.TempDir()); code == 0 || !strings.Contains(out+errs, "a reply is one sealed file") {
		t.Errorf("a folder given to accept: %d\n%s%s", code, out, errs)
	}
	// A reply record that an older key signed, with the chain as it is now:
	// the key that signed is not the identity's current key.
	if _, errs, code := run(t, ana, seedOf[ana], "rotate", "-seal-all"); code != 0 {
		t.Fatalf("rotate: %s", errs)
	}
	old := unpacked(t, rep, rwords)
	filepath.WalkDir(filepath.Join(ana, "objects"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, _ := os.ReadFile(path)
		if v, perr := core.Parse(data); perr == nil && strings.HasPrefix(v.(map[string]any)["type"].(string), "bean.") {
			rel, _ := filepath.Rel(ana, path)
			old[filepath.ToSlash(rel)] = data
		}
		return nil
	})
	stale, stalewords := sealed(t, old)
	if out, errs, code := run(t, ben, stalewords+"\n", "accept", "-yes", "-label", "Ana", "-words", idWords(t, ana), stale); code == 0 || !strings.Contains(out+errs, "not signed by the current key of its identity") {
		t.Errorf("a reply record of a retired key, with the whole chain: %d\n%s%s", code, out, errs)
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

// TestPeerRecordOpensNoFile: a record that a peer's key signed can name a
// file of the log, also one whose record the person deleted. No way out of
// the log sends that file: not a share with -files, and not an export.
func TestPeerRecordOpensNoFile(t *testing.T) {
	ben, ana, pkg, words, ids := replyLogs(t)
	out, errs, code := run(t, ben, "", "add", "symptom", "rash", "-photo", writePNG(t))
	if code != 0 {
		t.Fatalf("add: %s", errs)
	}
	photoID := strings.TrimSpace(out)
	rep, rwords := writeReply(t, ana, pkg, words, ids[0], "Looks fine to me.")
	if out, errs, code := run(t, ben, rwords+"\n", "accept", "-yes", "-label", "Ana", "-words", idWords(t, ana), rep); code != 0 {
		t.Fatalf("accept: %d\n%s\n%s", code, out, errs)
	}
	bs := core.Open(ben)
	l, err := bs.Load()
	if err != nil {
		t.Fatal(err)
	}
	hash := l.Report.Records[photoID].Blobs[0].Hash
	local, err := core.Open(ana).ReadLocal()
	if err != nil {
		t.Fatal(err)
	}
	// Ana knows the id of the file, and her record reaches Ben's folder.
	planted, data, err := core.SignRecord(map[string]any{
		"v": 0.0, "nonce": core.NewNonce(), "type": "meal", "subject": mustIdentity(t, ben), "author": local.Signer.Public(), "kid": local.Signer.Public(),
		"value": map[string]any{"s": "lunch"}, "times": map[string]any{"observed": nil, "imported": "2026-11-12T10:00:00Z"},
		"source": map[string]any{"system": "x", "adapter": core.Adapter, "ref": nil},
		"blobs":  []any{map[string]any{"hash": hash, "role": "photo", "media_type": "image/png"}},
	}, local.Signer)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ben, filepath.FromSlash(core.PathOf(planted)))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, errs, code := run(t, ben, "", "purge", photoID); code != 0 {
		t.Fatalf("purge: %d\n%s\n%s", code, out, errs)
	}
	for name, args := range map[string][]string{
		"a bundle":       {"share", "-yes", "-files", "-out", filepath.Join(t.TempDir(), "d"), planted},
		"a sealed share": {"share", "-yes", "-files", "-seal", "-out", filepath.Join(t.TempDir(), "d.bean"), planted},
	} {
		out, errs, code := run(t, ben, "", args...)
		if code == 0 || !strings.Contains(errs, "such a record opens no file") {
			t.Errorf("%s sent a file for a peer's record: %d\n%s\n%s", name, code, out, errs)
		}
		if _, err := os.Stat(args[len(args)-2]); err == nil {
			t.Errorf("%s: something was written", name)
		}
	}
	export := filepath.Join(t.TempDir(), "export")
	if out, errs, code := run(t, ben, "", "export", export); code != 0 {
		t.Fatalf("export: %d\n%s\n%s", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(export, "blobs", hash[7:])); err == nil {
		t.Errorf("the export holds the deleted photo")
	}
	// Without -files the record itself still travels: it is a claim.
	if out, errs, code := run(t, ben, "", "share", "-yes", "-out", filepath.Join(t.TempDir(), "plain"), planted); code != 0 {
		t.Errorf("share without -files: %d\n%s\n%s", code, out, errs)
	}
}

// TestAcceptRefusesTheLogsOwnAgent: a key that the person paired as their
// own agent is not another person's. A reply that it signed takes nothing.
func TestAcceptRefusesTheLogsOwnAgent(t *testing.T) {
	ben, ana, pkg, words, ids := replyLogs(t)
	local, err := core.Open(ana).ReadLocal()
	if err != nil {
		t.Fatal(err)
	}
	if out, errs, code := run(t, ben, "", "agent", "pair", local.Signer.Public(), "-label", "phone"); code != 0 {
		t.Fatalf("agent pair: %d\n%s\n%s", code, out, errs)
	}
	rep, rwords := writeReply(t, ana, pkg, words, ids[0], "Looks fine to me.")
	out, errs, code := run(t, ben, rwords+"\n", "accept", "-yes", "-words", idWords(t, ana), rep)
	if code == 0 || !strings.Contains(errs, `paired as your agent "phone"`) {
		t.Errorf("a reply of the log's own agent key: %d\n%s\n%s", code, out, errs)
	}
	if ls, _, _ := run(t, ben, "", "ls", "-type", "note"); strings.Contains(ls, "Looks fine") {
		t.Errorf("the note was taken:\n%s", ls)
	}
}
