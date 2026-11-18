package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/internal/bundle"
)

// replies counts the reply records of a log, and those that say that the
// reply was sealed to an address.
func replies(t *testing.T, log string) (int, int) {
	t.Helper()
	out, _, _ := run(t, log, "", "ls", "-all", "-json", "-type", "reply")
	return strings.Count(out, `"type":"reply"`), strings.Count(out, `"seal":"recipient"`)
}

// notes counts the notes of a log.
func notes(t *testing.T, log string) int {
	t.Helper()
	out, _, _ := run(t, log, "", "ls", "-all", "-json", "-type", "note")
	return strings.Count(out, `"type":"note"`)
}

// TestReplyForGate: bean reply -for seals to an address only when the
// replier gives the eight address words of that address, and asks before
// it writes. Until then nothing is written: no file and no record.
func TestReplyForGate(t *testing.T) {
	_, ana, pkg, words, ids := replyLogs(t)
	_, address, aw := newAddress(t, "ben.key")
	_, _, aw2 := newAddress(t, "other.key")
	where := t.TempDir()
	out := filepath.Join(where, "r.bean")
	for name, tc := range map[string]struct {
		args []string
		code int
		want string
	}{
		"no address words and no terminal": {[]string{"-for", address}, 1, "bean reply -for needs the eight address words of that person's address, and there is no terminal to ask on"},
		"no address words, with -yes":      {[]string{"-yes", "-for", address}, 1, "needs the eight address words"},
		"empty address words":              {[]string{"-yes", "-for", address, "-address-words", ""}, 1, "no address words were given"},
		"the words of another address":     {[]string{"-yes", "-for", address, "-address-words", aw2}, 1, "these are not the address words of this address"},
		"an empty address":                 {[]string{"-yes", "-for", "", "-address-words", aw}, 1, "nothing was written"},
		"an address in capitals":           {[]string{"-yes", "-for", strings.ToUpper(address), "-address-words", aw}, 1, "nothing was written"},
		"address words without -for":       {[]string{"-address-words", aw}, 2, "-address-words goes with -for"},
		"-yes without -for":                {[]string{"-yes"}, 2, "-yes goes with -for"},
		"no question asked, no -yes":       {[]string{"-for", address, "-address-words", aw}, 1, "bean reply -for asks before it writes, and there is no terminal to ask on"},
	} {
		args := append(append([]string{"reply", "-to", pkg, "-out", out}, tc.args...), ids[0], "a note")
		stdout, errs, code := run(t, ana, words+"\ny\n"+aw+"\n", args...)
		if code != tc.code || !strings.Contains(errs, tc.want) {
			t.Errorf("%s: exit %d, no %q in\n%s%s", name, code, tc.want, stdout, errs)
		}
		if entries, _ := os.ReadDir(where); len(entries) != 0 {
			t.Errorf("%s: a file was written", name)
		}
		if n, _ := replies(t, ana); n != 0 {
			t.Errorf("%s: a reply record was written", name)
		}
		// The gate stops before the package is opened, and bean shows
		// nothing that is made from the address.
		if strings.Contains(errs, aw) || (tc.code == 1 && !strings.Contains(tc.want, "asks before") && strings.Contains(errs, "Type the six words")) {
			t.Errorf("%s: bean showed the address words, or opened the package:\n%s", name, errs)
		}
	}
	// Without a question, bean says first what a reply to an address means.
	_, errs, _ := run(t, ana, words+"\n", "reply", "-to", pkg, "-for", address, "-address-words", aw, "-out", out, ids[0], "a note")
	for _, line := range replyForKeyLines {
		if !strings.Contains(errs, line) {
			t.Errorf("bean reply -for does not say, before it writes: %s", line)
		}
	}
}

// TestReplyFor: a reply that is sealed to the sender's address. The
// replier cannot open it, the reply record says "recipient", and the
// sender opens it with bean accept -key, which asks on a terminal.
func TestReplyFor(t *testing.T) {
	ben, ana, pkg, words, ids := replyLogs(t)
	key, address, aw := newAddress(t, "ben.key")
	other, _, _ := newAddress(t, "other.key")
	out := filepath.Join(t.TempDir(), "r.bean")
	stdout, errs, code := run(t, ana, words+"\n", "reply", "-to", pkg, "-for", address, "-address-words", aw, "-yes", "-out", out, ids[0], "Looks fine to me.", ids[2], "Thanks.")
	if code != 0 {
		t.Fatalf("reply -for: %d\n%s\n%s", code, stdout, errs)
	}
	for _, want := range []string{"wrote 2 notes in " + out, "You cannot open this reply again", "bean accept -key <their key file> " + out, "A bean that has no bean accept -key does not open it", idWords(t, ana)} {
		if !strings.Contains(stdout, want) {
			t.Errorf("reply -for does not say %q:\n%s", want, stdout)
		}
	}
	// No six words: the replier has nothing that opens it.
	if strings.Contains(stdout+errs, "six words:") || strings.Contains(stdout, "keep this file and its words") {
		t.Errorf("reply -for prints lines of a reply with six words:\n%s%s", stdout, errs)
	}
	if n, to := replies(t, ana); n != 1 || to != 1 {
		t.Errorf("the reply record: %d, %d sealed to an address", n, to)
	}
	data, _ := os.ReadFile(out)
	if asks, _ := bundle.Asks(data); asks != bundle.AsksKey {
		t.Fatalf("the reply asks for %q", asks)
	}
	if info, err := os.Stat(out); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the reply's file: %v", err)
	}
	// A reply with six words still says "passphrase".
	writeReply(t, ana, pkg, words, ids[1], "And this one.")
	if n, to := replies(t, ana); n != 2 || to != 1 {
		t.Errorf("a reply with six words: %d records, %d sealed to an address", n, to)
	}

	// The sender. Without -key, bean says how to open it.
	if _, errs, code := run(t, ben, "", "accept", out); code != 1 || !strings.Contains(errs, "Open it with bean accept -key <your key file>") {
		t.Errorf("accept with no key: %d\n%s", code, errs)
	}
	// -yes does not go with -key, also with the right words: a program that
	// holds the key file makes those words from the reply itself.
	for _, args := range [][]string{
		{"accept", "-key", key, "-yes", "-words", idWords(t, ana), "-label", "Ana", out},
		{"accept", "-yes", "-key", key, "-label", "Ana", out},
	} {
		if _, errs, code := run(t, ben, "", args...); code != 2 || !strings.Contains(errs, "-yes does not go with -key") {
			t.Errorf("%v: %d\n%s", args, code, errs)
		}
	}
	if _, errs, code := run(t, ben, "", "accept", "-key", "", out); code != 2 || !strings.Contains(errs, "-key names a key file") {
		t.Errorf("accept -key \"\": %d\n%s", code, errs)
	}
	// Another key does not open it, and bean says why.
	if _, errs, code := run(t, ben, "", "accept", "-key", other, "-label", "Ana", out); code != 1 || !strings.Contains(errs, "sealed to another address") {
		t.Errorf("accept with another key: %d\n%s", code, errs)
	}
	// With its key and no terminal: the list, what a key shows, and nothing
	// taken.
	stdout, errs, code = run(t, ben, "", "accept", "-key", key, "-label", "Ana", out)
	if code != 1 || !strings.Contains(errs, "run bean accept -key yourself, at a terminal") || strings.Contains(errs, "pass -yes") {
		t.Errorf("accept -key with no terminal: %d\n%s", code, errs)
	}
	for _, want := range []string{"The reply was sealed to your address. Anyone who has your address can seal to it", "Looks fine to me.", idWords(t, ana), "Ask the person whether they sent a reply whose id starts so"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("accept -key does not say %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "does NOT say") {
		t.Errorf("a reply that bean sealed to an address is marked as sealed again:\n%s", stdout)
	}
	if notes(t, ben) != 0 {
		t.Error("a note was taken with no question")
	}
	// check-bundle names the command that reads it.
	if stdout, _, _ := run(t, ben, "", "check-bundle", "-key", key, out); !strings.Contains(stdout, "reads it with bean accept -key <their key file>") {
		t.Errorf("check-bundle -key on a reply:\n%s", stdout)
	}
	// A reply with six words opens with -key given too: bean says that the
	// key is not used.
	rep, rw := writeReply(t, ana, pkg, words, ids[0], "Six words.")
	if _, errs, _ := run(t, ben, rw+"\n", "accept", "-key", key, "-label", "Ana", rep); !strings.Contains(errs, withWordsLine) {
		t.Errorf("accept -key on a reply with six words:\n%s", errs)
	}
}

// TestAcceptKeyMarksASealAgain: a reply that opens with a key, and whose
// record does not say that it was sealed to a key, is marked: someone
// sealed it again, such as an older reply.
func TestAcceptKeyMarksASealAgain(t *testing.T) {
	ben, ana, pkg, words, ids := replyLogs(t)
	key, address, _ := newAddress(t, "ben.key")
	rep, rw := writeReply(t, ana, pkg, words, ids[0], "An older reply.")
	packed, err := bundle.PackFiles(unpacked(t, rep, rw))
	if err != nil {
		t.Fatal(err)
	}
	to, _ := bundle.ParseAddress(address)
	data, _, err := bundle.SealTo("again.bean", packed, to)
	if err != nil {
		t.Fatal(err)
	}
	again := filepath.Join(t.TempDir(), "again.bean")
	os.WriteFile(again, data, 0o600)
	stdout, _, _ := run(t, ben, "", "accept", "-key", key, "-label", "Ana", again)
	if !strings.Contains(stdout, "The signer's record does NOT say that this reply was sealed to a key") || !strings.Contains(stdout, "An older reply.") {
		t.Errorf("a reply that was sealed again:\n%s", stdout)
	}
}

// TestReplyForOnATerminal: on a terminal, bean reply -for asks for the
// address words, then for the six words of the package, then says what a
// reply to an address means and asks. bean accept -key asks before it
// takes the notes. This runs bean on a terminal that Python makes.
func TestReplyForOnATerminal(t *testing.T) {
	ben, ana, pkg, words, ids := replyLogs(t)
	key, address, aw := newAddress(t, "ben.key")
	const gate, six, ask = "Type the eight address words that the person you reply to gave you, then Enter:", "Type the six words of this package, then Enter:", "? [y/N] "
	out := filepath.Join(t.TempDir(), "r.bean")
	// No to the question: nothing is written.
	seen, code := onTerminal(t, ana, [][2]string{{gate, aw}, {six, words}, {ask, "n"}}, "reply", "-to", pkg, "-for", address, "-out", out, ids[0], "A note.")
	if _, err := os.Stat(out); code != 1 || err == nil || !strings.Contains(seen, "nothing was written") {
		t.Errorf("no to the question: %d\n%s", code, seen)
	}
	if n, _ := replies(t, ana); n != 0 {
		t.Error("a reply record was written after a no")
	}
	seen, code = onTerminal(t, ana, [][2]string{{gate, aw}, {six, words}, {ask, "y"}}, "reply", "-to", pkg, "-for", address, "-out", out, ids[0], "A note.")
	if code != 0 {
		t.Fatalf("reply -for on a terminal: %d\n%s", code, seen)
	}
	before, _, _ := strings.Cut(seen, ask)
	for _, line := range replyForKeyLines {
		if !strings.Contains(before, line) {
			t.Errorf("bean reply -for does not say before it asks: %s", line)
		}
	}
	if !(strings.Index(seen, gate) < strings.Index(seen, six)) {
		t.Errorf("the gate does not come first:\n%s", seen)
	}
	// The sender takes the notes on a terminal, after a yes.
	accept := []string{"accept", "-key", key, "-label", "Ana", out}
	if seen, code := onTerminal(t, ben, [][2]string{{ask, "n"}}, accept...); code != 1 || notes(t, ben) != 0 {
		t.Errorf("no to accept -key: %d\n%s", code, seen)
	}
	seen, code = onTerminal(t, ben, [][2]string{{ask, "y"}}, accept...)
	if code != 0 || notes(t, ben) != 1 || !strings.Contains(seen, `took 1 notes of "Ana"`) {
		t.Errorf("accept -key on a terminal: %d\n%s", code, seen)
	}
}

// TestWriteReplyForKey: the lines of writeForKey for a reply name a reply.
func TestWriteReplyForKey(t *testing.T) {
	const id = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	path := filepath.Join(t.TempDir(), "r.bean")
	err := writeForKey(path, []byte("x"), replyWritten, id, func() error { return errors.New("disk full") }, fillFile)
	if err == nil || !strings.Contains(err.Error(), "nothing was sent: the reply record could not be added to your log, and the empty file is removed: disk full") {
		t.Errorf("the record fails: %v", err)
	}
	err = writeForKey(path, []byte("the bytes"), replyWritten, id, func() error { return nil }, func(f *os.File, d []byte) error {
		f.Write(d[:3])
		return errors.New("no space left")
	})
	if err == nil || !strings.Contains(err.Error(), "the reply was not written whole into "+path) || !strings.Contains(err.Error(), "Your log holds its reply record, "+id) {
		t.Errorf("a reply not written whole: %v", err)
	}
	if got, _ := os.ReadFile(path); bytes.Contains(got, []byte("the")) {
		t.Error("a part of the reply is still there")
	}
}
