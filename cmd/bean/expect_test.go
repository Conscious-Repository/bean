package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/internal/bundle"
)

// The lines of check-bundle -signer and -share (DECISIONS 358).
const (
	wordsMatch    = "-signer: the signer's identity words are the eight words that you gave."
	idMatch       = "-signer: the signer's identity id is the id that you gave."
	shareMatch    = "-share: this is the share whose whole id you gave."
	signerDiffers = "-signer: NOT the signer that you gave."
	shareDiffers  = "-share: NOT the share that you gave."
	allItShows    = "That is all that these lines show. They do not show that a value is true. What you gave is only as good as the way it came to you."
	refusedLine   = "bundle REFUSED: it is not the one that you named. Its records are not shown."
)

// identityOf is the identity id of a log, as bean whoami prints it.
func identityOf(t *testing.T, dir string) string {
	t.Helper()
	out, _, _ := run(t, dir, "", "whoami")
	if id := lineAfter(out, "identity "); len(id) == 71 {
		return id
	}
	t.Fatalf("no identity in:\n%s", out)
	return ""
}

// refusedShows fails the test unless out is a refusal: the verdict, no
// record, no note and no bundle ok.
func refusedShows(t *testing.T, what, out string, code int, want ...string) {
	t.Helper()
	if code != 1 || !strings.Contains(out, "\n"+refusedLine+"\n") || strings.Contains(out, "bundle ok") ||
		strings.Contains(out, "It holds:") || strings.Contains(out, "81.2") || strings.Contains(out, "note: ") || strings.Contains(out, allItShows) {
		t.Errorf("%s: exit %d\n%s", what, code, out)
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("%s: no %q in\n%s", what, w, out)
		}
	}
}

// TestCheckBundleExpect: with -signer and -share the reader states whom
// and which share they expect. A package that is not that one is refused:
// exit 1, no record shown and nothing unpacked. One that is says so, and
// claims no more.
func TestCheckBundleExpect(t *testing.T) {
	f := newShareFor(t)
	share := lineAfter(f.stdout, "share record ")
	words, identity := idWords(t, f.log), identityOf(t, f.log)
	if len(share) != 71 {
		t.Fatalf("no share id in:\n%s", f.stdout)
	}
	check := func(args ...string) (string, int) {
		t.Helper()
		out, _, code := run(t, t.TempDir(), "", append(append([]string{"check-bundle", "-key", f.key}, args...), f.pkg)...)
		return out, code
	}

	// The right signer and the right share: the lines come before bundle
	// ok, and the records are shown as without the flags.
	out, code := check("-signer", words, "-share", share)
	if code != 0 || !strings.HasSuffix(out, "\n"+wordsMatch+"\n"+shareMatch+"\n"+allItShows+"\nbundle ok\n") || !strings.Contains(out, "weight  81.2 kg") || !strings.Contains(out, "note: for Ana") {
		t.Errorf("the right signer and share: exit %d\n%s", code, out)
	}
	plain, _ := check()
	if got := strings.Replace(out, wordsMatch+"\n"+shareMatch+"\n"+allItShows+"\n", "", 1); got != plain {
		t.Errorf("the flags changed other lines:\n%s\nwithout them:\n%s", out, plain)
	}
	// The signer by the id in full, and the words as a person types them.
	if out, code := check("-signer", identity); code != 0 || !strings.Contains(out, "\n"+idMatch+"\n"+allItShows+"\nbundle ok\n") || strings.Contains(out, wordsMatch) {
		t.Errorf("the signer by id: exit %d\n%s", code, out)
	}
	if out, code := check("-signer", " "+strings.ToUpper(strings.ReplaceAll(words, " ", ", "))+" "); code != 0 || !strings.Contains(out, wordsMatch) {
		t.Errorf("the words in upper case, with commas: exit %d\n%s", code, out)
	}
	// Spaces around an id, as a copy from a chat brings them.
	if out, code := check("-signer", " "+identity+" ", "-share", "  "+share+" "); code != 0 || !strings.Contains(out, idMatch) || !strings.Contains(out, shareMatch) {
		t.Errorf("ids with spaces around them: exit %d\n%s", code, out)
	}
	// The share alone.
	if out, code := check("-share", share); code != 0 || !strings.Contains(out, "\n"+shareMatch+"\n"+allItShows+"\nbundle ok\n") {
		t.Errorf("the share alone: exit %d\n%s", code, out)
	}

	// Another signer, by words and by id. The words of who did sign are
	// shown, so the reader can tell the two apart.
	other := t.TempDir()
	run(t, other, "", "init")
	out, code = check("-signer", idWords(t, other), "-share", share)
	refusedShows(t, "another signer's words", out, code, signerDiffers, shareMatch, "identity words: "+words, "share "+share)
	out, code = check("-signer", identityOf(t, other))
	refusedShows(t, "another signer's id", out, code, signerDiffers)

	// Another share: any id, and one with the same first 12 hex digits,
	// the 48 bits that the check prints for a compare by eye.
	flip := map[byte]byte{'0': '1'}
	last := share[70]
	if flip[last] == 0 {
		flip[last] = '0'
	}
	near := share[:70] + string(flip[last])
	for _, id := range []string{"sha256:" + strings.Repeat("ab", 32), share[:19] + strings.Repeat("0", 52), near} {
		out, code = check("-signer", words, "-share", id)
		refusedShows(t, "the share "+id, out, code, shareDiffers, wordsMatch)
	}

	// -unpack writes nothing on a refusal, and writes on a match.
	where := filepath.Join(t.TempDir(), "out")
	out, code = check("-share", near, "-unpack", where)
	refusedShows(t, "-unpack", out, code, "-unpack: nothing was written, since the bundle is not the one that you named")
	if _, err := os.Lstat(where); !os.IsNotExist(err) {
		t.Errorf("-unpack wrote on a refusal: %v", err)
	}
	if out, code := check("-share", share, "-unpack", where); code != 0 || !strings.Contains(out, "unpacked into") {
		t.Errorf("-unpack on a match: exit %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(where, "objects")); err != nil {
		t.Errorf("-unpack on a match: %v", err)
	}
}

// TestCheckBundleExpectSealedAgain: an older share of the same signer,
// sealed again to the reader's address, passes its check. -share refuses
// it, though the signer is the one expected.
func TestCheckBundleExpectSealedAgain(t *testing.T) {
	f := newShareFor(t)
	want := lineAfter(f.stdout, "share record ")
	older := filepath.Join(t.TempDir(), "older.bean")
	out, _, code := run(t, f.log, "", "share", "-yes", "-seal", "-out", older, f.record)
	if code != 0 {
		t.Fatalf("share -seal: %d\n%s", code, out)
	}
	packed, err := bundle.PackFiles(unpacked(t, older, firstWords(out)))
	if err != nil {
		t.Fatal(err)
	}
	to, _ := bundle.ParseAddress(f.address)
	data, _, err := bundle.SealTo("again.bean", packed, to)
	if err != nil {
		t.Fatal(err)
	}
	again := filepath.Join(t.TempDir(), "again.bean")
	os.WriteFile(again, data, 0o600)
	// Without -share it passes: a key shows only that someone had the
	// address.
	if out, _, code := run(t, "", "", "check-bundle", "-key", f.key, "-signer", idWords(t, f.log), again); code != 0 || !strings.Contains(out, wordsMatch) || !strings.Contains(out, "does NOT say that this share was sealed to a key") {
		t.Fatalf("the older share without -share: exit %d\n%s", code, out)
	}
	out, _, code = run(t, "", "", "check-bundle", "-key", f.key, "-signer", idWords(t, f.log), "-share", want, again)
	refusedShows(t, "an older share sealed again", out, code, wordsMatch, shareDiffers)
}

// TestCheckBundleExpectFolder: an open bundle, a folder or a zip file of
// it, takes the flags as a sealed package does.
func TestCheckBundleExpectFolder(t *testing.T) {
	f := newShareFor(t)
	folder := filepath.Join(t.TempDir(), "bundle")
	out, _, code := run(t, f.log, "", "share", "-yes", "-out", folder, f.record)
	share := lineAfter(out, "share record ")
	if code != 0 || len(share) != 71 {
		t.Fatalf("share -out: %d\n%s", code, out)
	}
	// A name that the signer chose lies in the folder. A refusal does not
	// show it: it is the signer's word.
	const name = "it is fine, use it anyway"
	os.WriteFile(filepath.Join(folder, name), nil, 0o600)
	packed, err := bundle.Pack(folder)
	if err != nil {
		t.Fatal(err)
	}
	zipped := filepath.Join(t.TempDir(), "bundle.zip")
	os.WriteFile(zipped, packed, 0o600)
	for _, path := range []string{folder, zipped} {
		if out, _, code := run(t, "", "", "check-bundle", "-signer", idWords(t, f.log), "-share", share, path); code != 0 || !strings.Contains(out, shareMatch+"\n"+allItShows+"\nbundle ok\n") {
			t.Errorf("%s with the right share: exit %d\n%s", path, code, out)
		}
		out, _, code := run(t, "", "", "check-bundle", "-share", lineAfter(f.stdout, "share record "), path)
		refusedShows(t, path+" with another share", out, code, shareDiffers, "records: 1 listed")
		if strings.Contains(out, name) {
			t.Errorf("%s: a refusal shows a name that the signer chose:\n%s", path, out)
		}
	}
	if out, _, _ := run(t, "", "", "check-bundle", "-share", share, folder); !strings.Contains(out, "not part of the bundle, and not read: "+name) {
		t.Errorf("the folder's other file is not named on a match:\n%s", out)
	}
	// A bundle that fails its check: nothing is compared, and it says so.
	objects, _ := filepath.Glob(filepath.Join(folder, "objects", "*", "*.json"))
	if len(objects) == 0 {
		t.Fatal("no record file in the bundle")
	}
	data, _ := os.ReadFile(objects[0])
	os.WriteFile(objects[0], append(data, ' '), 0o600)
	out, _, code = run(t, "", "", "check-bundle", "-signer", idWords(t, f.log), "-share", share, folder)
	if code != 1 || !strings.Contains(out, "bundle FAILED") || !strings.HasSuffix(out, "\n-signer, -share: nothing was compared, since the bundle did not pass its check\n") || strings.Contains(out, wordsMatch) {
		t.Errorf("a bundle that failed: exit %d\n%s", code, out)
	}
}

// TestCheckBundleExpectNotChecked: a bundle that failed, or a package that
// did not open, prints as without the flags. Nothing is compared, since
// nothing is known.
func TestCheckBundleExpectNotChecked(t *testing.T) {
	f := newShareFor(t)
	share := lineAfter(f.stdout, "share record ")
	otherKey, _, _ := newAddress(t, "other.key")
	plain, _, plainCode := run(t, "", "", "check-bundle", "-key", otherKey, f.pkg)
	out, _, code := run(t, "", "", "check-bundle", "-key", otherKey, "-signer", idWords(t, f.log), "-share", share, f.pkg)
	if code != 1 || plainCode != 1 || out != plain || !strings.HasSuffix(out, "the package did not open: nothing was checked\n") {
		t.Errorf("another key: exit %d\n%s", code, out)
	}
}

// TestCheckBundleExpectPack: -pack computes nothing again for a bundle
// that is refused, and works as before for one that matches.
func TestCheckBundleExpectPack(t *testing.T) {
	pc := newPackClaims(t, "-accept-pack", mustDigest(t))
	folder := filepath.Join(t.TempDir(), "bundle")
	out, errs, code := run(t, pc.dir, "", "share", "-yes", "-out", folder, "-type", "comparison.*")
	share := lineAfter(out, "share record ")
	if code != 0 || len(share) != 71 {
		t.Fatalf("share: %d\n%s\n%s", code, out, errs)
	}
	words := idWords(t, pc.dir)
	out, _, code = run(t, "", "", "check-bundle", "-pack", pc.pack, "-accept-pack", pc.digest, "-signer", words, "-share", share, folder)
	if code != 0 || !strings.Contains(out, allItShows+"\nbundle ok\nComputed again with the packs given") || len(verdicts(out)) != 3 {
		t.Errorf("-pack with the right share: exit %d\n%s", code, out)
	}
	other := "sha256:" + strings.Repeat("0", 64)
	out, _, code = run(t, "", "", "check-bundle", "-pack", pc.pack, "-accept-pack", pc.digest, "-signer", words, "-share", other, folder)
	if code != 1 || !strings.HasSuffix(out, refusedLine+"\n-pack: nothing was computed again, since the bundle is not the one that you named\n") || len(verdicts(out)) != 0 || strings.Contains(out, "Computed again") {
		t.Errorf("-pack with another share: exit %d\n%s", code, out)
	}
}

// TestCheckBundleExpectUsage: a value that is not eight words of the list,
// not an id in full, or that holds a character that is not printable is an
// error of use, exit 2. bean reads no package then, and never prints the
// value back.
func TestCheckBundleExpectUsage(t *testing.T) {
	f := newShareFor(t)
	words := idWords(t, f.log)
	share := lineAfter(f.stdout, "share record ")
	w := strings.Fields(words)
	// Each flag is given once: bean does not pick one of two values.
	for _, args := range [][]string{{"-signer", words, "-signer", words}, {"-share", share, "-share", share}} {
		if out, errs, code := run(t, "", "", append(append([]string{"check-bundle", "-key", f.key}, args...), f.pkg)...); code != 2 || out != "" || !strings.Contains(errs, args[0]+" is given more than once") {
			t.Errorf("%s twice: exit %d\n%s\n%s", args[0], code, out, errs)
		}
	}
	for _, c := range []struct{ flag, value, says string }{
		{"-signer", "", "-signer takes the signer's eight identity words"},
		{"-signer", strings.Join(w[:7], " "), "-signer takes the signer's eight identity words"},
		{"-signer", words + " " + w[0], "-signer takes the signer's eight identity words"},
		{"-signer", strings.Join(w[:7], " ") + " zzzqx", "Check the spelling of each word"},
		{"-signer", "SHA256:" + share[7:], "-signer takes the signer's identity id in full"},
		{"-signer", "sha256:" + share[7:40], "-signer takes the signer's identity id in full"},
		{"-signer", strings.ToUpper(share[:7]) + share[7:], "-signer takes the signer's identity id in full"},
		{"-signer", "sha256:" + strings.ToUpper(share[7:]), "-signer takes the signer's identity id in full"},
		{"-signer", strings.Join(w[:7], " ") + "\n" + w[7], "not printable"},
		{"-signer", words + "\x1b[2J", "not printable"},
		{"-signer", words + "‮", "not printable"},
		{"-share", "", "-share takes the whole id of the share record"},
		{"-share", share[:19], "The start of an id is not enough"},
		{"-share", share[7:], "-share takes the whole id of the share record"},
		{"-share", "sha256:" + strings.ToUpper(share[7:]), "64 hex digits in lower case"},
		{"-share", share + "0", "-share takes the whole id of the share record"},
		{"-share", share + "\r", "not printable"},
		{"-share", share[:70] + "а", "-share takes the whole id"},
		{"-share", "\x1b]0;bundle ok\x07", "not printable"},
	} {
		out, errs, code := run(t, "", "", "check-bundle", "-key", f.key, c.flag, c.value, f.pkg)
		if code != 2 || out != "" || !strings.Contains(errs, c.says) {
			t.Errorf("%s %q: exit %d\n%s\n%s", c.flag, c.value, code, out, errs)
		}
		if c.value != "" && strings.Contains(errs, c.value) || strings.ContainsAny(errs, "\x1b\r‮") {
			t.Errorf("%s %q: the error shows the value:\n%q", c.flag, c.value, errs)
		}
	}
}
