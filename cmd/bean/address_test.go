package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"filippo.io/age"

	"github.com/Conscious-Repository/bean/internal/bundle"
)

// newAddress makes a key file with bean address -out, and returns its path,
// its address and its address words.
func newAddress(t *testing.T, name string) (string, string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	out, errs, code := run(t, t.TempDir(), "", "address", "-out", path)
	if code != 0 {
		t.Fatalf("address -out: %d\n%s\n%s", code, out, errs)
	}
	return path, lineAfter(out, "address "), lineAfter(out, "address words ")
}

// lineAfter is the rest of the first line of text that starts with prefix.
func lineAfter(text, prefix string) string {
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(line, prefix); ok && !strings.HasPrefix(rest, "words ") {
			return rest
		}
	}
	return ""
}

// TestAddress: bean address makes a key file and never writes over one,
// shows the address of a key file, and prints nothing secret. It needs no
// log, and it writes no key into a log's folder.
func TestAddress(t *testing.T) {
	empty := t.TempDir() // BEAN_DIR, with no log in it
	where := t.TempDir()
	path := filepath.Join(where, "reader.key")
	out, errs, code := run(t, empty, "", "address", "-out", path)
	if code != 0 {
		t.Fatalf("address -out with no log: %d\n%s\n%s", code, out, errs)
	}
	address, words := lineAfter(out, "address "), lineAfter(out, "address words ")
	if _, err := bundle.ParseAddress(address); err != nil || len(strings.Fields(words)) != 8 || words != bundle.AddressWords(address) {
		t.Fatalf("address %q, words %q", address, words)
	}
	// The file: mode 0600, the comment line of age-keygen, then the secret.
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the key file: %v, mode %v", err, info.Mode())
	}
	data, _ := os.ReadFile(path)
	lines := strings.Split(string(data), "\n")
	if len(lines) != 3 || lines[0] != "# public key: "+address || !strings.HasPrefix(lines[1], "AGE-SECRET-KEY-1") || lines[2] != "" {
		t.Fatalf("the key file is not as age-keygen writes one: %d lines", len(lines))
	}
	// The age library reads it, as one identity with that address.
	ids, err := age.ParseIdentities(bytes.NewReader(data))
	if err != nil || len(ids) != 1 || ids[0].(*age.X25519Identity).Recipient().String() != address {
		t.Fatalf("the age library does not read the key file: %v", err)
	}
	secret := lines[1]
	// Every line that a person must know is printed, and nothing secret.
	// The two channels come first, before the list.
	if two := strings.Index(out, "Give the address by one channel, and the address words by another"); two < 0 || two > strings.Index(out, addressLines[0]) {
		t.Errorf("address -out does not say first that the address and its words go by two channels:\n%s", out)
	}
	for _, line := range addressLines {
		if !strings.Contains(out, line) {
			t.Errorf("address -out does not say: %s", line)
		}
	}
	if strings.Contains(out+errs, "AGE-SECRET-KEY") {
		t.Error("address -out prints the secret")
	}
	// -key shows the same address, from the secret, and prints no secret.
	out2, errs2, code := run(t, empty, "", "address", "-key", path)
	if code != 0 || lineAfter(out2, "address ") != address || lineAfter(out2, "address words ") != words || strings.Contains(out2+errs2, "AGE-SECRET-KEY") {
		t.Errorf("address -key: %d\n%s\n%s", code, out2, errs2)
	}
	for _, line := range addressLines {
		if !strings.Contains(out2, line) {
			t.Errorf("address -key does not say: %s", line)
		}
	}
	// A key file that other users can read: bean reads it, and warns. A file
	// with mode 0600 gets no warning, from -out and from -key.
	if strings.Contains(errs+errs2, "warning") {
		t.Errorf("a key file with mode 0600 gets a warning:\n%s\n%s", errs, errs2)
	}
	if runtime.GOOS != "windows" {
		loose := filepath.Join(where, "loose.key")
		for mode, warned := range map[os.FileMode]bool{0o644: true, 0o640: true, 0o604: true, 0o600: false, 0o700: false} {
			os.Remove(loose)
			os.WriteFile(loose, data, mode)
			os.Chmod(loose, mode) // not cut by the umask
			out, errs, code := run(t, empty, "", "address", "-key", loose)
			says := strings.Contains(errs, fmt.Sprintf("warning: the mode of the key file %s is %04o, which lets other users of this machine read it", loose, mode)) && strings.Contains(errs, "chmod 600")
			if code != 0 || lineAfter(out, "address ") != address || says != warned || (!warned && errs != "") || strings.Contains(out+errs, "AGE-SECRET-KEY") {
				t.Errorf("address -key on a key file with mode %04o: %d, warned %v\n%s", mode, code, says, errs)
			}
		}
	}
	// A key file of age-keygen, whose comment line names another address:
	// the address comes from the secret.
	other, otherAddress, _ := newAddress(t, "other.key")
	keygen := filepath.Join(where, "keygen.key")
	os.WriteFile(keygen, []byte("# created: 2026-11-18T12:00:00Z\n# public key: "+otherAddress+"\n"+secret+"\n"), 0o600)
	if out, _, code := run(t, empty, "", "address", "-key", keygen); code != 0 || lineAfter(out, "address ") != address {
		t.Errorf("address -key on a file of age-keygen: %d\n%s", code, out)
	}
	// It never writes over a file: not a key file, and not any other.
	for _, there := range []string{path, other} {
		before, _ := os.ReadFile(there)
		_, errs, code := run(t, empty, "", "address", "-out", there)
		after, _ := os.ReadFile(there)
		if code != 1 || !strings.Contains(errs, "is there already") || !bytes.Equal(before, after) {
			t.Errorf("address -out onto a file: %d %s", code, errs)
		}
	}
	// What is not a key file gets one fixed line, with nothing of the file.
	two := filepath.Join(where, "two.key")
	otherData, _ := os.ReadFile(other)
	os.WriteFile(two, append(data, otherData...), 0o600)
	notes := filepath.Join(where, "notes.txt")
	os.WriteFile(notes, []byte("AGE-SECRET-KEY-1NOTAKEY my cholesterol was 193\n"), 0o600)
	for _, file := range []string{two, notes} {
		out, errs, code := run(t, empty, "", "address", "-key", file)
		if code != 1 || out != "" || !strings.Contains(errs, bundle.ErrKeyFile.Error()) || strings.Contains(errs, "AGE-SECRET") || strings.Contains(errs, "cholesterol") {
			t.Errorf("address -key %s: %d %q %q", filepath.Base(file), code, out, errs)
		}
	}
	// A file that is larger than a key file can be is not read to its end,
	// and is not taken for the one key at its start: it may hold a second.
	large := filepath.Join(where, "large.key")
	os.WriteFile(large, append(append(bytes.Clone(data), bytes.Repeat([]byte("# a comment line\n"), 5000)...), otherData...), 0o600)
	if _, errs, code := run(t, empty, "", "address", "-key", large); code != 1 || !strings.Contains(errs, bundle.ErrKeyFile.Error()) {
		t.Errorf("address -key on a large file: %d %s", code, errs)
	}
	// A folder that is not there: bean makes none, and says so.
	if _, errs, code := run(t, empty, "", "address", "-out", filepath.Join(where, "no-such-folder", "reader.key")); code != 1 || !strings.Contains(errs, "is not there. Make it first") {
		t.Errorf("address -out into a folder that is not there: %d %s", code, errs)
	}
	// One of -out and -key, and nothing else.
	for _, args := range [][]string{{"address"}, {"address", "-out", filepath.Join(where, "a"), "-key", path}, {"address", "-out", filepath.Join(where, "a"), "more"}, {"address", "-key", ""}, {"address", "-out", "", "-key", path}, {"address", "-key", "", "-out", filepath.Join(where, "a")}} {
		if _, _, code := run(t, empty, "", args...); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
	if _, err := os.Stat(filepath.Join(where, "a")); err == nil {
		t.Error("a usage error wrote a key file")
	}
	// Inside a log's folder: refused when a log is found there, and only
	// then. A copy of the log would carry the key.
	log := filepath.Join(t.TempDir(), "mylog") // a name with letters, for the case below
	os.Mkdir(log, 0o700)
	run(t, log, "", "init")
	for _, inside := range []string{filepath.Join(log, "reader.key"), filepath.Join(log, "local", "reader.key")} {
		_, errs, code := run(t, log, "", "address", "-out", inside)
		if _, err := os.Stat(inside); code != 1 || !strings.Contains(errs, "is inside your log") || err == nil {
			t.Errorf("a key file inside a log: %d %s", code, errs)
		}
	}
	if _, errs, code := run(t, empty, "", "address", "-out", filepath.Join(empty, "reader.key")); code != 0 {
		t.Errorf("a key file in a folder that holds no log: %d %s", code, errs)
	}
	// A link into the log is a place inside the log.
	link := filepath.Join(where, "keys")
	if err := os.Symlink(filepath.Join(log, "local"), link); err != nil {
		t.Skip("no links here")
	}
	_, errs, code = run(t, log, "", "address", "-out", filepath.Join(link, "linked.key"))
	if _, err := os.Stat(filepath.Join(log, "local", "linked.key")); code != 1 || !strings.Contains(errs, "is inside your log") || err == nil {
		t.Errorf("a key file inside a log, through a link: %d %s", code, errs)
	}
	// On a disk that does not tell capitals from small letters, the log's
	// folder under another spelling is the log's folder.
	spelled := filepath.Join(filepath.Dir(log), strings.ToUpper(filepath.Base(log)))
	if a, err := os.Stat(spelled); err == nil && spelled != log {
		if b, _ := os.Stat(log); os.SameFile(a, b) {
			_, errs, code = run(t, log, "", "address", "-out", filepath.Join(spelled, "cased.key"))
			if _, err := os.Stat(filepath.Join(log, "cased.key")); code != 1 || !strings.Contains(errs, "is inside your log") || err == nil {
				t.Errorf("a key file inside a log, in other capitals: %d %s", code, errs)
			}
		}
	}
	// A folder beside the log, and a folder above it, are not inside it.
	beside := filepath.Join(filepath.Dir(log), "beside.key")
	if _, errs, code := run(t, log, "", "address", "-out", beside); code != 0 {
		t.Errorf("a key file beside the log: %d %s", code, errs)
	}
	os.Remove(beside)
	// The same holds for a bundle: bean share judges a place the same way.
	// Release 0.7.2 wrote a bundle into the log through such a link.
	id, _, _ := run(t, log, "", "add", "weight", "81.2", "-unit", "kg")
	_, errs, code = run(t, log, "", "share", "-yes", "-out", filepath.Join(link, "bundle"), strings.TrimSpace(id))
	if _, err := os.Stat(filepath.Join(log, "local", "bundle")); code != 1 || !strings.Contains(errs, "is inside your log") || err == nil || shares(t, log) != 0 {
		t.Errorf("a bundle inside a log, through a link: %d %s", code, errs)
	}
}

// TestReadBack: bean address shows an address only when the key file reads
// back as the key that was made. A file that does not is removed.
func TestReadBack(t *testing.T) {
	made, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	other, _ := age.GenerateX25519Identity()
	dir := t.TempDir()
	good := filepath.Join(dir, "good.key")
	os.WriteFile(good, []byte("# public key: "+made.Recipient().String()+"\n"+made.String()+"\n"), 0o600)
	if err := readBack(good, made); err != nil {
		t.Errorf("a key file that reads back: %v", err)
	}
	if _, err := os.Stat(good); err != nil {
		t.Error("a key file that reads back was removed")
	}
	for name, content := range map[string]string{
		"another key": other.String() + "\n",
		"cut short":   made.String()[:40],
		"empty":       "",
	} {
		path := filepath.Join(dir, "bad.key")
		os.WriteFile(path, []byte(content), 0o600)
		err := readBack(path, made)
		if _, serr := os.Stat(path); err == nil || !strings.Contains(err.Error(), "no address was made") || !strings.Contains(err.Error(), "is removed") || serr == nil {
			t.Errorf("%s: %v, the file is there: %v", name, err, serr == nil)
		}
	}
}

// shareFor is a log with one record, a reader's key file, and a package
// of that record that is sealed to the reader's address.
type shareFor struct {
	log, record, key, address, words, where, pkg, stdout string
}

func newShareFor(t *testing.T) shareFor {
	t.Helper()
	f := shareFor{log: t.TempDir(), where: t.TempDir()}
	run(t, f.log, "", "init")
	id, _, _ := run(t, f.log, "", "add", "weight", "81.2", "-unit", "kg", "-at", "2026-11-02")
	f.record = strings.TrimSpace(id)
	f.key, f.address, f.words = newAddress(t, "reader.key")
	f.pkg = filepath.Join(f.where, "for-ana.bean")
	out, errs, code := run(t, f.log, "", "share", "-yes", "-for", f.address, "-address-words", f.words, "-note", "for Ana", "-out", f.pkg, f.record)
	if code != 0 {
		t.Fatalf("share -for: %d\n%s\n%s", code, out, errs)
	}
	f.stdout = out
	return f
}

// shares counts the share records of a log.
func shares(t *testing.T, log string) int {
	t.Helper()
	out, _, _ := run(t, log, "", "ls", "-json", "-type", "share")
	return strings.Count(out, `"type":"share"`)
}

// TestShareForGate: bean seals to an address only when the person gives
// the eight address words of that address. -yes never stands in for them.
// Until the gate has passed, nothing is written: no file and no record.
func TestShareForGate(t *testing.T) {
	log, where := t.TempDir(), t.TempDir()
	run(t, log, "", "init")
	id, _, _ := run(t, log, "", "add", "weight", "81.2", "-unit", "kg")
	id = strings.TrimSpace(id)
	_, address, words := newAddress(t, "reader.key")
	_, address2, words2 := newAddress(t, "second.key")
	out := filepath.Join(where, "p.bean")
	nothing := func(name string) {
		t.Helper()
		if entries, _ := os.ReadDir(where); len(entries) != 0 || shares(t, log) != 0 {
			t.Fatalf("%s: something was written: %d entries, %d share records", name, len(entries), shares(t, log))
		}
	}
	// No address words, and no terminal to ask on. With -yes too.
	for _, args := range [][]string{
		{"share", "-for", address, "-out", out, id},
		{"share", "-yes", "-for", address, "-out", out, id},
		{"share", "-yes", "-seal", "-for", address, "-out", out, id},
	} {
		stdout, errs, code := run(t, log, "y\n"+words+"\n", args...)
		if code != 1 || stdout != "" || !strings.Contains(errs, "needs the eight address words") || !strings.Contains(errs, "bean address -key <file>") {
			t.Errorf("%v: %d\n%s\n%s", args, code, stdout, errs)
		}
		nothing(strings.Join(args, " "))
	}
	// Words that are not those of the address: bean says nothing that is
	// made from the address.
	for name, given := range map[string]string{
		"the words of another address": words2,
		"seven of the eight":           strings.Join(strings.Fields(words)[:7], " "),
		"the eight and one more":       words + " able",
		"the address itself":           address,
	} {
		stdout, errs, code := run(t, log, "", "share", "-yes", "-for", address, "-address-words", given, "-out", out, id)
		if code != 1 || stdout != "" || !strings.Contains(errs, "nothing was written: these are not the address words of this address") {
			t.Errorf("%s: %d\n%s\n%s", name, code, stdout, errs)
		}
		if strings.Contains(errs, words) || strings.Contains(errs, address) {
			t.Errorf("%s: the refusal shows the address or its words: %s", name, errs)
		}
		nothing(name)
	}
	// No words at all, as from a script whose variable is empty: bean says
	// that none were given, and does not send the person to the reader.
	for _, none := range []string{"", "  ", ",.;"} {
		stdout, errs, code := run(t, log, "", "share", "-yes", "-for", address, "-address-words", none, "-out", out, id)
		if code != 1 || stdout != "" || !strings.Contains(errs, "nothing was written: no address words were given") || strings.Contains(errs, "ask the reader for the address") {
			t.Errorf("-address-words %q: %d\n%s\n%s", none, code, stdout, errs)
		}
		nothing("no address words")
	}
	// The address rule: only the lower-case text that the library writes.
	for name, given := range map[string]string{
		"in capitals":      strings.ToUpper(address),
		"with a space":     address + " ",
		"one letter short": address[:len(address)-1],
		"empty":            "",
		"an identity id":   "sha256:" + strings.Repeat("a", 64),
	} {
		stdout, errs, code := run(t, log, "", "share", "-yes", "-for", given, "-address-words", words, "-out", out, id)
		if code != 1 || stdout != "" || !strings.Contains(errs, "this is not an address in lower case") {
			t.Errorf("an address %s: %d\n%s\n%s", name, code, stdout, errs)
		}
		nothing("an address " + name)
	}
	// An empty address is not "no address": a script whose variable is
	// empty must not write a plain folder.
	if _, errs, code := run(t, log, "", "share", "-yes", "-for", "", "-out", filepath.Join(where, "folder"), id); code != 1 || !strings.Contains(errs, "not an address") {
		t.Errorf("-for with an empty address: %d %s", code, errs)
	}
	nothing("-for with an empty address")
	if _, _, code := run(t, log, "", "share", "-yes", "-address-words", words, "-out", filepath.Join(where, "folder"), id); code != 2 {
		t.Errorf("-address-words without -for: exit %d, want 2", code)
	}
	nothing("-address-words without -for")
	// The words of the address, as a person may write them.
	if stdout, errs, code := run(t, log, "", "share", "-yes", "-for", address, "-address-words", strings.ToUpper(strings.ReplaceAll(words, " ", ", "))+".", "-out", out, id); code != 0 {
		t.Errorf("the address words in capitals, with commas: %d\n%s\n%s", code, stdout, errs)
	}
	// Each address has its own words.
	if words == words2 || bundle.AddressWords(address2) != words2 {
		t.Error("two addresses with the same words")
	}
}

// TestShareFor: a package that is sealed to an address. The share record
// says "recipient" and names no address. The reader's key file opens the
// package, and nothing else does.
func TestShareFor(t *testing.T) {
	f := newShareFor(t)
	// What is printed: the identity words and the start of the share's id,
	// that the sender cannot open the file, and no line about six words or
	// a browser.
	for _, want := range []string{"is sealed to the address that you gave", "You cannot open this file again, unless the address is your own", "Your identity words, which are not a secret", "the start of its id", "The reader opens it with bean check-bundle -key <their key file>, or with the age tool", "A bean that has no bean address does not open it"} {
		if !strings.Contains(f.stdout, want) {
			t.Errorf("share -for does not say %q:\n%s", want, f.stdout)
		}
	}
	for _, not := range []string{"six words", "browser", f.address, f.words} {
		if strings.Contains(f.stdout, not) {
			t.Errorf("share -for says %q:\n%s", not, f.stdout)
		}
	}
	data, err := os.ReadFile(f.pkg)
	if err != nil {
		t.Fatal(err)
	}
	if asks, _ := bundle.Asks(data); asks != bundle.AsksKey {
		t.Fatalf("the package asks for %q", asks)
	}
	for _, clear := range []string{f.record[7:], "weight", "for Ana", "objects/"} {
		if bytes.Contains(data, []byte(clear)) {
			t.Errorf("the package shows %q in the clear", clear)
		}
	}
	if info, _ := os.Stat(f.pkg); info.Mode().Perm() != 0o600 {
		t.Errorf("the package has mode %v", info.Mode())
	}
	if entries, _ := os.ReadDir(f.where); len(entries) != 1 {
		t.Errorf("the share left %d entries beside the package", len(entries)-1)
	}
	// The record: "recipient", and no address, no hash of one, no words.
	ls, _, _ := run(t, f.log, "", "ls", "-json", "-type", "share")
	sum := sha256.Sum256([]byte(f.address))
	if !strings.Contains(ls, `"seal":"recipient"`) || shares(t, f.log) != 1 {
		t.Errorf("the share record: %s", ls)
	}
	for _, not := range []string{f.address, f.address[4:], hex.EncodeToString(sum[:]), hex.EncodeToString(sum[:8]), f.words, "age1"} {
		if strings.Contains(ls, not) {
			t.Errorf("the share record holds %q", not)
		}
	}
	// A package is a new file.
	if _, errs, code := run(t, f.log, "", "share", "-yes", "-for", f.address, "-address-words", f.words, "-out", f.pkg, f.record); code == 0 || !strings.Contains(errs, "is there already") || shares(t, f.log) != 1 {
		t.Errorf("a second share onto the package: %d\n%s", code, errs)
	}
	// -seal still makes six words, and its record says "passphrase".
	sealed := filepath.Join(f.where, "words.bean")
	stdout, _, code := run(t, f.log, "", "share", "-yes", "-seal", "-out", sealed, f.record)
	if code != 0 || !strings.Contains(stdout, "It opens with these six words") {
		t.Fatalf("share -seal: %d\n%s", code, stdout)
	}
	six := ""
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "  ") && six == "" {
			six = strings.TrimSpace(line)
		}
	}
	if ls, _, _ := run(t, f.log, "", "ls", "-json", "-type", "share"); strings.Count(ls, `"seal":"passphrase"`) != 1 || strings.Count(ls, `"seal":"recipient"`) != 1 {
		t.Errorf("the share records: %s", ls)
	}

	// Opening. A reader has no log: an empty folder.
	reader := t.TempDir()
	const prompt = "Type the six words"
	// No key: bean says what the file asks for, and asks for no words.
	stdout, errs, code := run(t, reader, six+"\n", "check-bundle", f.pkg)
	if code != 1 || strings.Contains(errs, prompt) || strings.Contains(stdout, "It holds") ||
		!strings.Contains(stdout, "is sealed to a key, not with words. Open it with -key <your key file>. If you no longer have the file, you cannot open it: ask the sender to share again, for a new address") ||
		!strings.Contains(stdout, "the package did not open: nothing was checked") {
		t.Errorf("check-bundle with no key: %d\n%s\n%s", code, stdout, errs)
	}
	// The key: it opens, and the check says what a key shows and what not.
	stdout, errs, code = run(t, reader, "", "check-bundle", "-key", f.key, f.pkg)
	if code != 0 || !strings.Contains(stdout, "bundle ok") || strings.Contains(errs, prompt) {
		t.Fatalf("check-bundle -key: %d\n%s\n%s", code, stdout, errs)
	}
	lines := strings.Split(stdout, "\n")
	shareID := lineAfter(stdout, "share ")
	if lines[0] != "The package was sealed to your address. Anyone who has your address can seal to it, also an older share of the signer. bean cannot tell who sealed it." ||
		!strings.Contains(stdout, "\nidentity words: ") || len(shareID) != 71 ||
		!strings.Contains(stdout, "\nAsk the signer, by another way than this package came, whether they sent you a share whose id starts "+shareID[7:19]+".\n") ||
		!strings.Contains(stdout, "note: for Ana") || !strings.Contains(stdout, "weight  81.2 kg") {
		t.Errorf("check-bundle -key says:\n%s", stdout)
	}
	if strings.Contains(stdout, "does NOT say") {
		t.Errorf("a package that bean sealed to a key is marked as sealed again:\n%s", stdout)
	}
	// A key file that other users can read opens the package too, and bean
	// warns on standard error.
	if runtime.GOOS != "windows" {
		loose := filepath.Join(f.where, "loose.key")
		os.WriteFile(loose, mustRead(t, f.key), 0o644)
		os.Chmod(loose, 0o644)
		stdout, errs, code := run(t, reader, "", "check-bundle", "-key", loose, f.pkg)
		if code != 0 || !strings.Contains(stdout, "bundle ok") || !strings.Contains(errs, "lets other users of this machine read it") || strings.Contains(stdout, "warning") {
			t.Errorf("check-bundle with a key file of mode 0644: %d\n%s\n%s", code, stdout, errs)
		}
	}
	if strings.Contains(errs, "warning") {
		t.Errorf("check-bundle with a key file of mode 0600 warns: %s", errs)
	}
	// Another key, and what is not a key file.
	other, _, _ := newAddress(t, "other.key")
	stdout, _, code = run(t, reader, "", "check-bundle", "-key", other, f.pkg)
	if code != 1 || strings.Contains(stdout, "It holds") || !strings.Contains(stdout, "it was sealed to another address, or it was changed on the way. bean cannot tell which. bean address -key <file> shows the address of a key file") {
		t.Errorf("check-bundle with another key: %d\n%s", code, stdout)
	}
	keyData, _ := os.ReadFile(f.key)
	otherData, _ := os.ReadFile(other)
	two := filepath.Join(f.where, "two.key")
	os.WriteFile(two, append(keyData, otherData...), 0o600)
	protected := filepath.Join(f.where, "protected.key")
	wordsOfKey, _ := bundle.NewWords()
	sealedKey, _ := bundle.Seal(keyData, wordsOfKey)
	os.WriteFile(protected, sealedKey, 0o600)
	for file, want := range map[string]string{two: bundle.ErrKeyFile.Error(), protected: "protected with a passphrase", f.pkg: "encrypted age file", filepath.Join(f.where, "none"): "cannot be read"} {
		stdout, errs, code := run(t, reader, "", "check-bundle", "-key", file, f.pkg)
		if code != 1 || strings.Contains(stdout, "It holds") || !strings.Contains(stdout, want) || strings.Contains(stdout+errs, "AGE-SECRET") {
			t.Errorf("check-bundle -key %s: %d\n%s", filepath.Base(file), code, stdout)
		}
	}
	// A flag comes before the file's name, and -key names a file.
	for _, args := range [][]string{{"check-bundle", f.pkg, "-key", f.key}, {"check-bundle", "-key", "", f.pkg}, {"check-bundle", "-key", f.key}} {
		if stdout, _, code := run(t, reader, "", args...); code != 2 || strings.Contains(stdout, "It holds") {
			t.Errorf("%v: exit %d, want 2", args[1:], code)
		}
	}
	// A changed package, and one that was cut short, show nothing.
	for name, change := range map[string]func([]byte) []byte{
		"one byte changed": func(d []byte) []byte { d[len(d)-20] ^= 1; return d },
		"cut short":        func(d []byte) []byte { return d[:len(d)-30] },
	} {
		bad := filepath.Join(f.where, "bad.bean")
		os.WriteFile(bad, change(bytes.Clone(data)), 0o600)
		if stdout, _, code := run(t, reader, "", "check-bundle", "-key", f.key, bad); code != 1 || strings.Contains(stdout, "It holds") || strings.Contains(stdout, "81.2") {
			t.Errorf("%s: %d\n%s", name, code, stdout)
		}
	}
	// -key with a package that is sealed with words: bean says that the
	// key is not used, and the six words open the package.
	stdout, errs, code = run(t, reader, six+"\n", "check-bundle", "-key", f.key, sealed)
	if code != 0 || !strings.Contains(errs, "It is sealed with words. -key is not used: give the six words.") || !strings.Contains(errs, prompt) || !strings.Contains(stdout, "bundle ok") || strings.Contains(stdout, "sealed to your address") {
		t.Errorf("-key with a package that is sealed with words: %d\n%s\n%s", code, stdout, errs)
	}
	// The key file is not read then: a file that is no key changes nothing.
	if stdout, _, code := run(t, reader, six+"\n", "check-bundle", "-key", two, sealed); code != 0 || !strings.Contains(stdout, "bundle ok") {
		t.Errorf("a key file that is not used was read: %d\n%s", code, stdout)
	}
	// -key with a folder, and with a plain zip file.
	folder := filepath.Join(f.where, "plain")
	run(t, f.log, "", "share", "-yes", "-out", folder, f.record)
	packed, err := bundle.Pack(folder)
	if err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(f.where, "plain.zip")
	os.WriteFile(zipPath, packed, 0o600)
	for _, plain := range []string{folder, zipPath} {
		stdout, errs, code := run(t, reader, "", "check-bundle", "-key", f.key, plain)
		if code != 0 || !strings.Contains(errs, "It is not sealed. -key is not used.") || !strings.Contains(stdout, "bundle ok") || strings.Contains(stdout, "sealed to your address") {
			t.Errorf("-key with %s: %d\n%s\n%s", filepath.Base(plain), code, stdout, errs)
		}
	}
	// -key with what is not there, or is no bundle and no package: bean
	// does not say that it is not sealed. An age file in armor is sealed.
	armor := filepath.Join(f.where, "armor.age")
	os.WriteFile(armor, []byte("-----BEGIN AGE ENCRYPTED FILE-----\nYWdlLWVuY3J5cHRpb24ub3JnL3YxCg==\n-----END AGE ENCRYPTED FILE-----\n"), 0o600)
	for _, what := range []string{filepath.Join(f.where, "not-there"), armor} {
		stdout, errs, code := run(t, reader, "", "check-bundle", "-key", f.key, what)
		if code != 1 || strings.Contains(errs, "It is not sealed") || strings.Contains(stdout, "bundle ok") {
			t.Errorf("-key with %s: %d\n%s\n%s", filepath.Base(what), code, stdout, errs)
		}
	}
	if stdout, _, _ := run(t, reader, "", "check-bundle", "-key", f.key, armor); !strings.Contains(stdout, "it is an age file in armor") {
		t.Errorf("an age file in armor: %s", stdout)
	}
	// -unpack writes what the key opened.
	unpacked := filepath.Join(f.where, "unpacked")
	if stdout, _, code := run(t, reader, "", "check-bundle", "-key", f.key, "-unpack", unpacked, f.pkg); code != 0 || !strings.Contains(stdout, "unpacked into") {
		t.Errorf("-unpack with -key: %d\n%s", code, stdout)
	}
	if lines, ok := checkBundle(unpacked); !ok {
		t.Errorf("the unpacked folder does not pass:\n%s", strings.Join(lines, "\n"))
	}
	// bean accept takes a reply, and this bean seals a reply with words.
	if stdout, errs, code := run(t, f.log, "", "accept", "-label", "Ana", f.pkg); code != 1 || strings.Contains(errs, prompt) || !strings.Contains(errs, "bean accept takes a reply, and this bean seals a reply with words") {
		t.Errorf("accept on a package that is sealed to a key: %d\n%s\n%s", code, stdout, errs)
	}

	// The one check of what the signer signed about the seal. Someone seals
	// the plain bundle, whose record says nothing of a seal, to the reader's
	// address. And the package of six words, opened and sealed again.
	key, err := bundle.ParseKey(keyData)
	if err != nil {
		t.Fatal(err)
	}
	fromWords, err := bundle.Unseal(mustRead(t, sealed), six)
	if err != nil {
		t.Fatal(err)
	}
	for name, zipped := range map[string][]byte{"a plain bundle": packed, "a package of six words": fromWords} {
		again, back, err := bundle.SealTo("p", zipped, key.Recipient())
		if err != nil || !back.Opened {
			t.Fatal(err)
		}
		resealed := filepath.Join(f.where, "again.bean")
		os.WriteFile(resealed, again, 0o600)
		stdout, _, code := run(t, reader, "", "check-bundle", "-key", f.key, resealed)
		if code != 0 || !strings.Contains(stdout, "bundle ok") || !strings.Contains(stdout, "The signer's record does NOT say that this share was sealed to a key. Someone sealed it again, or another tool sealed it.") {
			t.Errorf("%s, sealed again to the reader: %d\n%s", name, code, stdout)
		}
	}
	// The check looks one way only: a package that opens with words is not
	// marked, whatever its record says.
	if stdout, _, _ := run(t, reader, six+"\n", "check-bundle", sealed); strings.Contains(stdout, "does NOT say") || strings.Contains(stdout, "Ask the signer, by another way") {
		t.Errorf("a package that opened with words:\n%s", stdout)
	}

	// The reader replies to a package that their key opened. The reply is
	// sealed with six words, as today.
	readerLog := t.TempDir()
	run(t, readerLog, "", "init")
	reply := filepath.Join(f.where, "reply.bean")
	stdout, errs, code = run(t, readerLog, "", "reply", "-to", f.pkg, "-key", f.key, "-out", reply, f.record[7:19], "Looks fine to me.")
	if code != 0 || !strings.Contains(stdout, "It opens with these six words") || strings.Contains(errs, prompt) {
		t.Fatalf("reply -to with -key: %d\n%s\n%s", code, stdout, errs)
	}
	// Before it signs, it says what a key shows, and what to ask the signer.
	if !strings.Contains(errs, "The package was sealed to your address. Anyone who has your address can seal to it") || !strings.Contains(errs, "Ask the signer, by another way than this package came") || strings.Contains(errs, "does NOT say") {
		t.Errorf("reply -to with -key does not say what a key shows:\n%s", errs)
	}
	// A package that someone sealed again is marked for a reply too.
	againZip, _, err := bundle.SealTo("p", packed, key.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	againPath := filepath.Join(f.where, "again2.bean")
	os.WriteFile(againPath, againZip, 0o600)
	if _, errs, code := run(t, readerLog, "", "reply", "-to", againPath, "-key", f.key, "-out", filepath.Join(f.where, "reply3.bean"), f.record[7:19], "x"); code != 0 || !strings.Contains(errs, "The signer's record does NOT say that this share was sealed to a key") {
		t.Errorf("reply -to a package that was sealed again: %d\n%s", code, errs)
	}
	// -key names a file, and bean says when it is not used.
	if _, _, code := run(t, readerLog, "", "reply", "-to", f.pkg, "-key", "", "-out", filepath.Join(f.where, "reply5.bean"), f.record[7:19], "x"); code != 2 {
		t.Errorf("reply with an empty -key: exit %d, want 2", code)
	}
	if _, errs, code := run(t, readerLog, "", "reply", "-to", folder, "-key", f.key, "-out", filepath.Join(f.where, "reply6.bean"), f.record[7:19], "x"); code != 0 || !strings.Contains(errs, "It is not sealed. -key is not used.") {
		t.Errorf("reply -to a folder, with -key: %d\n%s", code, errs)
	}
	// A reply to a package of six words says none of it.
	if _, errs, _ := run(t, readerLog, six+"\n", "reply", "-to", sealed, "-out", filepath.Join(f.where, "reply4.bean"), f.record[7:19], "x"); strings.Contains(errs, "sealed to your address") || strings.Contains(errs, "Ask the signer") {
		t.Errorf("reply -to a package of six words: %s", errs)
	}
	if ls, _, _ := run(t, readerLog, "", "ls", "-json", "-type", "reply"); !strings.Contains(ls, `"seal":"passphrase"`) {
		t.Errorf("the reply record: %s", ls)
	}
	if _, errs, code := run(t, readerLog, "", "reply", "-to", f.pkg, "-out", filepath.Join(f.where, "reply2.bean"), f.record[7:19], "x"); code != 1 || !strings.Contains(errs, "is sealed to a key, not with words") || !strings.Contains(errs, "what you reply to did not open") {
		t.Errorf("reply -to with no key: %d\n%s", code, errs)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestWriteForKey: the order of work for a package that is sealed to a
// key. The file is made new and empty, then the share record is added, then
// the bytes are written. So a log never knows less than what left. What
// bean says of a file that was not written whole is only what it did.
func TestWriteForKey(t *testing.T) {
	const id = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	data := []byte("the sealed bytes")
	dir := t.TempDir()
	path := filepath.Join(dir, "p.bean")
	// The good case, and what each step sees.
	var order []string
	err := writeForKey(path, data, id, func() error {
		got, err := os.ReadFile(path)
		if err != nil || len(got) != 0 {
			t.Errorf("when the record is added, the file is not there and empty: %v, %d bytes", err, len(got))
		}
		order = append(order, "record")
		return nil
	}, func(f *os.File, d []byte) error {
		order = append(order, "fill")
		return fillFile(f, d)
	})
	if got, _ := os.ReadFile(path); err != nil || !bytes.Equal(got, data) || strings.Join(order, " ") != "record fill" {
		t.Fatalf("the good case: %v, %q, order %v", err, got, order)
	}
	// A file that is there already: nothing is written, and no record.
	recorded := false
	err = writeForKey(path, []byte("other"), id, func() error { recorded = true; return nil }, fillFile)
	if got, _ := os.ReadFile(path); err == nil || !strings.HasPrefix(err.Error(), "nothing was shared: ") || recorded || !bytes.Equal(got, data) {
		t.Errorf("onto a file that is there: %v, recorded %v", err, recorded)
	}
	// The record cannot be added: the empty file is removed, and no byte
	// of the package was written.
	path = filepath.Join(dir, "q.bean")
	filled := false
	err = writeForKey(path, data, id, func() error { return errors.New("disk full") }, func(*os.File, []byte) error { filled = true; return nil })
	if _, serr := os.Stat(path); err == nil || !strings.Contains(err.Error(), "nothing was shared: the share record could not be added to your log, and the empty file is removed: disk full") || filled || serr == nil {
		t.Errorf("the record fails: %v, filled %v, the file is there: %v", err, filled, serr == nil)
	}
	// The file cannot be written whole: a part of it is there. bean removes
	// it, says so, and names the record and how to delete it.
	part := func(f *os.File, d []byte) error {
		f.Write(d[:5])
		return errors.New("no space left")
	}
	err = writeForKey(path, data, id, func() error { return nil }, part)
	if err == nil {
		t.Fatal("a file that was not written whole is not an error")
	}
	msg := err.Error()
	if _, serr := os.Stat(path); serr == nil {
		t.Error("the part of the file is still there")
	}
	for _, want := range []string{path, id, "no space left", "A part of such a file can open", "bean removed the file", "bean purge " + id[7:19], "does not sync"} {
		if !strings.Contains(msg, want) {
			t.Errorf("after a file that was not written whole, bean does not say %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "nothing was") {
		t.Errorf("bean says that nothing was shared, and a record is in the log: %s", msg)
	}
	// The same, and the file cannot be removed: bean says to delete it, and
	// does not name purge.
	if os.Getuid() == 0 {
		t.Skip("root removes a file from a folder that is read-only")
	}
	locked := filepath.Join(dir, "locked")
	os.Mkdir(locked, 0o700)
	defer os.Chmod(locked, 0o700)
	path = filepath.Join(locked, "r.bean")
	err = writeForKey(path, data, id, func() error { return nil }, func(f *os.File, d []byte) error {
		f.Write(d[:5])
		os.Chmod(locked, 0o500)
		return errors.New("no space left")
	})
	if err == nil {
		t.Fatal("a file that was not written whole is not an error")
	}
	msg = err.Error()
	if _, serr := os.Stat(path); serr != nil {
		t.Fatalf("the test did not keep the file in place: %v", serr)
	}
	for _, want := range []string{path, id, "A part of such a file can open", "bean could not remove the file: delete it"} {
		if !strings.Contains(msg, want) {
			t.Errorf("when the file cannot be removed, bean does not say %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "purge") || strings.Contains(msg, "removed the file") {
		t.Errorf("bean names purge, or says that it removed a file that is there: %s", msg)
	}
	// The same for the empty file, when the record fails.
	os.Chmod(locked, 0o700)
	path = filepath.Join(locked, "s.bean")
	err = writeForKey(path, data, id, func() error { os.Chmod(locked, 0o500); return errors.New("disk full") }, fillFile)
	if err == nil || !strings.Contains(err.Error(), "could not be removed: delete it") || strings.Contains(err.Error(), "is removed") {
		t.Errorf("the record fails, and the empty file stays: %v", err)
	}
}

// TestSealPackageTo: bean opens what it sealed for another person with the
// file's own key, and checks what came out. A package that does not pass
// is not written.
func TestSealPackageTo(t *testing.T) {
	key, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"objects/aa/" + strings.Repeat("b", 62) + ".json": []byte("{}")}
	var seen bundle.Input
	data, err := sealPackageTo("p", files, key.Recipient(), func(in bundle.Input) bool { seen = in; return true })
	if err != nil || !seen.Opened || seen.Asks != bundle.AsksKey || len(seen.Records) != 1 {
		t.Fatalf("the round trip: %v, %+v", err, seen)
	}
	if in := bundle.FromPackageKey("p", data, key); !in.Opened || len(in.Records) != 1 {
		t.Errorf("the reader's key does not open what bean sealed: %v", in.Fatal)
	}
	if data, err := sealPackageTo("p", files, key.Recipient(), func(bundle.Input) bool { return false }); err == nil || data != nil {
		t.Error("a package that does not pass its check was handed on")
	}
}

// TestShareForOnATerminal: with a terminal and no -address-words, bean
// asks for the address words before it shows the review, also with -yes.
// The review says what a seal to an address means, before it asks. This
// runs bean on a terminal that Python makes.
func TestShareForOnATerminal(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is needed, to give bean a terminal")
	}
	log, where := t.TempDir(), t.TempDir()
	run(t, log, "", "init")
	id, _, _ := run(t, log, "", "add", "weight", "81.2", "-unit", "kg")
	id = strings.TrimSpace(id)
	_, address, words := newAddress(t, "reader.key")
	// onTerminal runs bean on a terminal, and answers each prompt in turn.
	onTerminal := func(answers [][2]string, args ...string) (string, int) {
		t.Helper()
		// A prompt that the test does not expect gets no answer: after 20
		// seconds bean is stopped, and the run counts as failed (exit 124).
		const script = `
import os, pty, select, signal, sys, time
n = int(sys.argv[1])
answers = [(sys.argv[2 + 2 * i].encode(), sys.argv[3 + 2 * i].encode()) for i in range(n)]
pid, fd = pty.fork()
if pid == 0:
    if os.environ.get("BEAN_TEST_STDERR"):
        os.dup2(os.open(os.environ["BEAN_TEST_STDERR"], os.O_WRONLY | os.O_CREAT, 0o600), 2)
    os.execv(sys.argv[2 + 2 * n], sys.argv[2 + 2 * n:])
seen = b""
mark = 0
late = False
end = time.time() + 20
while True:
    ready, _, _ = select.select([fd], [], [], max(0, end - time.time()))
    if not ready:
        late = True
        os.kill(pid, signal.SIGKILL)
        break
    try:
        got = os.read(fd, 4096)
    except OSError:
        break
    if not got:
        break
    seen += got
    if answers and answers[0][0] in seen[mark:]:
        mark = len(seen)
        os.write(fd, answers.pop(0)[1] + b"\n")
sys.stdout.buffer.write(seen)
_, status = os.waitpid(pid, 0)
sys.exit(124 if late else os.waitstatus_to_exitcode(status))
`
		argv := []string{"-I", "-c", script, fmt.Sprint(len(answers))}
		for _, a := range answers {
			argv = append(argv, a[0], a[1])
		}
		cmd := exec.Command(python, append(append(argv, os.Args[0]), args...)...)
		cmd.Env = append(os.Environ(), "BEAN_TEST_MAIN=1", "BEAN_DIR="+log, "BEAN_NOW=", "GORACE=atexit_sleep_ms=0")
		out, err := cmd.Output()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(out), "\r\n", "\n"), code
	}
	const gate, ask = "Type the eight address words that the reader gave you, then Enter:", "? [y/N] "
	// Wrong words at the prompt: nothing is shown of the review, and
	// nothing is written.
	out := filepath.Join(where, "p.bean")
	seen, code := onTerminal([][2]string{{gate, "abandon ability able about above absent absorb abstract"}}, "share", "-for", address, "-out", out, id)
	if code != 1 || !strings.Contains(seen, "these are not the address words of this address") || strings.Contains(seen, "To share, into") || shares(t, log) != 0 {
		t.Fatalf("wrong words on a terminal: %d\n%s", code, seen)
	}
	if strings.Contains(seen, words) {
		t.Errorf("bean shows the address words at the gate:\n%s", seen)
	}
	// The words, then the review with its lines, then the answer.
	seen, code = onTerminal([][2]string{{gate, words}, {ask, "y"}}, "share", "-for", address, "-out", out, id)
	if code != 0 || shares(t, log) != 1 {
		t.Fatalf("share -for on a terminal: %d\n%s", code, seen)
	}
	review, _, found := strings.Cut(seen, ask)
	if !found || strings.Index(seen, gate) > strings.Index(seen, "To share, into") {
		t.Fatalf("the gate does not come before the review:\n%s", seen)
	}
	for _, line := range forKeyLines {
		if !strings.Contains(review, line) {
			t.Errorf("the review does not say, before it asks: %s", line)
		}
	}
	// A share with six words has none of these lines.
	seen, _ = onTerminal([][2]string{{ask, "n"}}, "share", "-seal", "-out", filepath.Join(where, "w.bean"), id)
	for _, line := range forKeyLines {
		if strings.Contains(seen, line) {
			t.Errorf("the review of a share with six words says: %s", line)
		}
	}
	if !strings.Contains(seen, "To share, into") || strings.Contains(seen, gate) {
		t.Errorf("share -seal on a terminal:\n%s", seen)
	}
	// Standard input is a terminal, and standard error is not: the prompt
	// would go where the person does not see it. bean stops, as with no
	// terminal.
	errFile := filepath.Join(where, "stderr.txt")
	t.Setenv("BEAN_TEST_STDERR", errFile)
	out3 := filepath.Join(where, "r.bean")
	seen, code = onTerminal([][2]string{{gate, words}}, "share", "-yes", "-for", address, "-out", out3, id)
	t.Setenv("BEAN_TEST_STDERR", "")
	said, _ := os.ReadFile(errFile)
	if _, err := os.Stat(out3); code != 1 || err == nil || !strings.Contains(string(said), "needs the eight address words") || strings.Contains(string(said), gate) {
		t.Errorf("a terminal for standard input only: %d\n%s\n%s", code, seen, said)
	}
	// -yes skips the review, and never the address words.
	out2 := filepath.Join(where, "q.bean")
	seen, code = onTerminal([][2]string{{gate, "abandon ability able about above absent absorb abstract"}}, "share", "-yes", "-for", address, "-out", out2, id)
	if _, err := os.Stat(out2); code != 1 || err == nil || shares(t, log) != 1 {
		t.Errorf("-yes with wrong words on a terminal: %d\n%s", code, seen)
	}
	seen, code = onTerminal([][2]string{{gate, words}}, "share", "-yes", "-for", address, "-out", out2, id)
	if _, err := os.Stat(out2); code != 0 || err != nil || strings.Contains(seen, "To share, into") {
		t.Errorf("-yes with the words on a terminal: %d\n%s", code, seen)
	}
}
