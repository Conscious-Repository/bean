package bundle

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"filippo.io/age"
)

// TestUnsealRefusesACostlierHeader: bean seals with one work factor. A
// header that asks for more is refused for that, before any work is done:
// a changed header must not keep a reader's machine busy.
func TestUnsealRefusesACostlierHeader(t *testing.T) {
	words, err := NewWords()
	if err != nil {
		t.Fatal(err)
	}
	packed, err := PackFiles(map[string][]byte{"objects/aa/bb.json": []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := Seal(packed, words)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Unseal(sealed, words); err != nil || !bytes.Equal(got, packed) {
		t.Fatalf("a package does not open with its words: %v", err)
	}
	// A package of a lower cost opens: the sender chose it, and the words
	// are what protect the package. tools/second/unseal.py does the same.
	to, err := age.NewScryptRecipient(words)
	if err != nil {
		t.Fatal(err)
	}
	to.SetWorkFactor(10)
	var cheap bytes.Buffer
	w, _ := age.Encrypt(&cheap, to)
	w.Write(packed)
	w.Close()
	if got, err := Unseal(cheap.Bytes(), words); err != nil || !bytes.Equal(got, packed) {
		t.Errorf("a package of a lower cost: %v", err)
	}
	// The header's stanza line is "-> scrypt <salt> <work factor>".
	head, rest, _ := bytes.Cut(sealed[len(sealHeader):], []byte("\n"))
	if !bytes.HasSuffix(head, []byte(" 16")) {
		t.Fatalf("the header is not as expected: %q", head)
	}
	costly := append([]byte(sealHeader), bytes.TrimSuffix(head, []byte("16"))...)
	costly = append(append(append(costly, "18"...), '\n'), rest...)
	if _, err := Unseal(costly, words); err == nil || !strings.Contains(err.Error(), "work factor") {
		t.Errorf("a header that asks for more work: %v", err)
	}
}

// sealFor seals packed for each recipient with the age library, as another
// tool would.
func sealFor(t *testing.T, packed []byte, to ...age.Recipient) []byte {
	t.Helper()
	var out bytes.Buffer
	w, err := age.Encrypt(&out, to...)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(packed)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func newKey(t *testing.T) *age.X25519Identity {
	t.Helper()
	key, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// TestAsks: one scan of the header says what a file asks for, before any
// word or key is looked at. A file that names no reader, or two, or a
// reader of another kind, is refused there.
func TestAsks(t *testing.T) {
	packed := []byte("not a zip file, which the scan does not read")
	words, _ := NewWords()
	withWords, err := Seal(packed, words)
	if err != nil {
		t.Fatal(err)
	}
	a, b := newKey(t), newKey(t)
	forOne := sealFor(t, packed, a.Recipient())
	forTwo := sealFor(t, packed, a.Recipient(), b.Recipient())
	// The stanza line of forOne, to build headers that no library writes.
	stanza, _, _ := bytes.Cut(forOne[len(sealHeader):], []byte("\n"))
	edit := func(data []byte, old, new string) []byte {
		if !bytes.Contains(data, []byte(old)) {
			t.Fatalf("no %q in the header", old)
		}
		return bytes.Replace(bytes.Clone(data), []byte(old), []byte(new), 1)
	}
	// One reader, and a header that ends after the first 4 KiB: a stanza
	// with a body of 100 lines.
	long := append([]byte(sealHeader), stanza...)
	long = append(append(long, bytes.Repeat([]byte("\n"+strings.Repeat("A", 64)), 100)...), "\nAAAA\n--- AAAA\n"...)
	for name, c := range map[string]struct {
		data []byte
		want string
	}{
		"a zip file":                     {[]byte("PK\x03\x04"), ""},
		"in armor":                       {[]byte("-----BEGIN AGE ENCRYPTED FILE-----\nYWdlLWVuY3J5cHRpb24ub3JnL3YxCg==\n-----END AGE ENCRYPTED FILE-----\n"), AsksOther},
		"in armor, after an empty line":  {[]byte("\n-----BEGIN AGE ENCRYPTED FILE-----\n"), AsksOther},
		"nothing":                        {nil, ""},
		"sealed with words":              {withWords, AsksWords},
		"sealed to a key":                {forOne, AsksKey},
		"for two readers":                {forTwo, AsksOther},
		"a reader of another kind":       {edit(forOne, "-> X25519 ", "-> ssh-ed25519 "), AsksOther},
		"no reader":                      {edit(forOne, string(stanza)+"\n", ""), AsksOther},
		"words and a key":                {edit(withWords, "\n--- ", "\n"+string(stanza)+"\n--- "), AsksOther},
		"a key and another kind":         {edit(forOne, "\n--- ", "\n-> ssh-ed25519 AAAA AAAA\nAAAA\n--- "), AsksOther},
		"a carriage return":              {edit(forOne, string(stanza)+"\n", string(stanza)+"\r\n"), AsksOther},
		"a second reader after a return": {edit(forOne, string(stanza)+"\n", string(stanza)+"\n"+strings.Repeat("A", 43)+"\r-> X25519 x\n"), AsksOther},
		"only the first line":            {[]byte(sealHeader), AsksOther},
		"a header that ends after 4 KiB": {long, AsksOther},
		"a stanza with no space":         {edit(forOne, "-> X25519 ", "->X25519 "), AsksOther},
	} {
		asks, why := Asks(c.data)
		if asks != c.want || (asks == AsksOther) != (why != "") {
			t.Errorf("%s: asks %q (%s), want %q", name, asks, why, c.want)
		}
		// What is refused by the scan opens with nothing, and says what it
		// asks for either way.
		if c.want == AsksOther {
			for _, in := range []Input{FromPackage("p", c.data, words), FromPackageKey("p", c.data, a)} {
				if !in.Locked || in.Opened || in.Asks != AsksOther || len(in.Fatal) != 1 || in.Records != nil {
					t.Errorf("%s: not refused: %+v", name, in)
				}
			}
		}
	}
	// A file for two readers is refused, though the age tool opens it.
	if in := FromPackageKey("p", forTwo, a); !strings.Contains(in.Fatal[0], "has 2 stanzas") {
		t.Errorf("for two readers: %v", in.Fatal)
	}
	// Each reason says only what the scan found. A file with a reader for a
	// key and a reader of another kind is sealed to an address: bean does
	// not say otherwise.
	other := "not with words, and not to an address"
	for name, c := range map[string]struct {
		data []byte
		why  string
	}{
		"a key, then another kind": {edit(forOne, "\n--- ", "\n-> grease AAAA\nAAAA\n--- "), "has 2 stanzas"},
		"another kind, then a key": {edit(forOne, string(stanza), "-> grease AAAA\nAAAA\n"+string(stanza)), "has 2 stanzas"},
		"another kind alone":       {edit(forOne, "-> X25519 ", "-> ssh-ed25519 "), other},
		"no reader":                {edit(forOne, string(stanza)+"\n", ""), "has 0 stanzas"},
	} {
		if asks, why := Asks(c.data); asks != AsksOther || !strings.Contains(why, c.why) || (c.why != other && strings.Contains(why, other)) {
			t.Errorf("%s: %s", name, why)
		}
	}
}

// TestSealTo: a package that is sealed to an address opens with the key of
// that address and with no other, whole and not in part. bean opens what
// it sealed with the file's own key, and the reader's key opens the same.
func TestSealTo(t *testing.T) {
	// A file that does not pack small, so that the package has more than
	// one chunk of 64 KiB.
	noise := make([]byte, 200000)
	rand.Read(noise)
	packed, err := PackFiles(map[string][]byte{"objects/aa/bb.json": []byte("{}"), "files/x": noise})
	if err != nil {
		t.Fatal(err)
	}
	reader, other := newKey(t), newKey(t)
	sealed, back, err := SealTo("p", packed, reader.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	if !back.Opened || back.Locked || back.Asks != AsksKey || len(back.Records) != 1 || len(back.Files) != 1 {
		t.Fatalf("what bean opened with the file's own key: %+v", back)
	}
	// One reader, and the reader's own key opens it: this holds the stanza
	// that the round trip does not open.
	if n := bytes.Count(sealed[:bytes.Index(sealed, []byte("\n---"))], []byte("\n->")); n != 1 {
		t.Errorf("%d stanzas, want 1", n)
	}
	in := FromPackageKey("p", sealed, reader)
	if !in.Opened || in.Asks != AsksKey || len(in.Records) != 1 || !bytes.Equal(in.Records["objects/aa/bb.json"], []byte("{}")) {
		t.Fatalf("the reader's key does not open it: %+v", in.Fatal)
	}
	// Another key, and words, open nothing, and say what the file asks for.
	if in := FromPackageKey("p", sealed, other); !in.Locked || in.Opened || in.Records != nil || !strings.Contains(in.Fatal[0], "sealed to another address, or it was changed on the way. bean cannot tell which") {
		t.Errorf("another key: %+v", in)
	}
	words, _ := NewWords()
	if in := FromPackage("p", sealed, words); !in.Locked || in.Asks != AsksKey || !strings.Contains(in.Fatal[0], "is sealed to a key, not with words. Open it with -key <your key file>") {
		t.Errorf("words on a package for a key: %+v", in)
	}
	if in := FromPackage("p", sealed, ""); !in.Locked || in.Asks != AsksKey {
		t.Errorf("no words on a package for a key: %+v", in)
	}
	// A key on a package that is sealed with words, and on a plain zip file.
	withWords, _ := Seal(packed, words)
	if in := FromPackageKey("p", withWords, reader); !in.Locked || in.Asks != AsksWords || !strings.Contains(in.Fatal[0], "sealed with words") {
		t.Errorf("a key on a package with words: %+v", in)
	}
	if in := FromPackageKey("p", packed, reader); in.Opened || in.Locked || in.Asks != "" || len(in.Fatal) != 1 {
		t.Errorf("a key on a zip file: %+v", in)
	}
	// A package that was cut short gives a part of its plaintext to a tool
	// that streams. bean shows nothing of a part.
	if len(sealed) < 100000 {
		t.Fatalf("the package is too small to cut after its first chunk: %d", len(sealed))
	}
	for name, data := range map[string][]byte{
		"cut after the first chunk": sealed[:len(sealed)-1000],
		"cut by one byte":           sealed[:len(sealed)-1],
		"one byte more":             append(bytes.Clone(sealed), 'x'),
	} {
		if in := FromPackageKey("p", data, reader); !in.Locked || in.Opened || in.Records != nil || in.Files != nil {
			t.Errorf("%s: %+v", name, in.Fatal)
		}
	}
	for _, at := range []int{len(sealHeader) + 12, len(sealHeader) + 60, 230, len(sealed) - 20} {
		changed := bytes.Clone(sealed)
		changed[at] ^= 1
		if in := FromPackageKey("p", changed, reader); !in.Locked || in.Opened || in.Records != nil {
			t.Errorf("a byte changed at %d: opened", at)
		}
	}
	// The file's own key opens the one file that it sealed, and no other.
	own := &ownKey{to: reader.Recipient()}
	if in := FromPackageKey("p", sealed, own); !in.Locked {
		t.Error("a key that sealed nothing opened a package")
	}
	first := sealFor(t, packed, own)
	second := sealFor(t, packed, reader.Recipient())
	if in := FromPackageKey("p", first, own); !in.Opened {
		t.Errorf("the file's own key does not open its file: %v", in.Fatal)
	}
	if in := FromPackageKey("p", second, own); !in.Locked || in.Opened {
		t.Error("the file's own key opened another file")
	}
}

// TestReadOpened: what an opened package holds is read whole or not at
// all, and up to a limit. Both seals read through it.
func TestReadOpened(t *testing.T) {
	if got, err := readOpened(strings.NewReader("0123456789"), 10); err != nil || string(got) != "0123456789" {
		t.Errorf("as much as the limit: %q, %v", got, err)
	}
	if got, err := readOpened(strings.NewReader("0123456789x"), 10); err == nil || got != nil || !strings.Contains(err.Error(), "holds more than a package can") {
		t.Errorf("one byte more than the limit: %q, %v", got, err)
	}
	cut := io.MultiReader(strings.NewReader("a part"), iotest.ErrReader(errors.New("cut short")))
	if got, err := readOpened(cut, 10); err == nil || got != nil || !strings.Contains(err.Error(), "it is damaged") {
		t.Errorf("a part, then an error: %q, %v", got, err)
	}
}

// bech32 writes data under a prefix, as BIP 173 says. The test builds an
// address that no key opens with it.
func bech32(hrp string, data []byte) string {
	const charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"
	var five []byte
	acc, bits := 0, 0
	for _, b := range data {
		acc, bits = acc<<8|int(b), bits+8
		for bits >= 5 {
			bits -= 5
			five = append(five, byte(acc>>bits&31))
		}
	}
	if bits > 0 {
		five = append(five, byte(acc<<(5-bits)&31))
	}
	polymod := func(values []byte) uint32 {
		chk := uint32(1)
		for _, v := range values {
			top := chk >> 25
			chk = (chk&0x1ffffff)<<5 ^ uint32(v)
			for i, g := range []uint32{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3} {
				if top>>i&1 == 1 {
					chk ^= g
				}
			}
		}
		return chk
	}
	var values []byte
	for _, c := range []byte(hrp) {
		values = append(values, c>>5)
	}
	values = append(values, 0)
	for _, c := range []byte(hrp) {
		values = append(values, c&31)
	}
	values = append(values, five...)
	pm := polymod(append(values, 0, 0, 0, 0, 0, 0)) ^ 1
	out := hrp + "1"
	for _, v := range five {
		out += string(charset[v])
	}
	for i := range 6 {
		out += string(charset[pm>>(5*(5-i))&31])
	}
	return out
}

// TestAddress: an address has one spelling, and its eight address words
// are made from the SHA-256 of that text. An address that was written by
// hand so that no key opens it has other words than the reader's true
// address, so the words catch it.
func TestAddress(t *testing.T) {
	key := newKey(t)
	address := key.Recipient().String()
	to, err := ParseAddress(address)
	if err != nil || to.String() != address {
		t.Fatalf("an address: %v", err)
	}
	for name, text := range map[string]string{
		"in capitals":       strings.ToUpper(address),
		"with a space":      address + " ",
		"with a line break": address + "\n",
		"one letter short":  address[:len(address)-1],
		"empty":             "",
		"a secret key":      key.String(),
		"another prefix":    "npub1" + address[4:],
	} {
		if _, err := ParseAddress(text); err == nil || !strings.Contains(err.Error(), "not an address in lower case") {
			t.Errorf("%s: %v", name, err)
		}
		if err != nil && text != "" && strings.Contains(err.Error(), text) {
			t.Errorf("%s: the refusal shows what was given", name)
		}
	}
	// The words, by a fixed example that docs/SHARE.md states too.
	const example = "age1c66cfc86n0mk5kf3lsq8twsqlfyvahd3wvf50wwdzulm66y6pvqsu7l2az"
	if _, err := ParseAddress(example); err != nil {
		t.Errorf("the example is not an address: %v", err)
	}
	if got := AddressWords(example); got != "table nothing proud slender scare away clay cherry" {
		t.Errorf("the address words of the example: %s", got)
	}
	if words := AddressWords(address); len(strings.Fields(words)) != 8 || CleanAddressWords("  "+strings.ToUpper(strings.ReplaceAll(words, " ", ", "))+".\n") != words {
		t.Errorf("address words: %q", words)
	}
	// The same 32 bytes with the top bit set: the library reads it as an
	// address and seals to it, and the reader's key does not open the file.
	point := bech32Data(address)
	if bech32("age", point) != address {
		t.Fatalf("the test's own bech32 does not write the address back: %s", bech32("age", point))
	}
	point[31] |= 0x80
	forged := bech32("age", point)
	bad, err := ParseAddress(forged)
	if err != nil {
		t.Fatalf("the library does not read the address with the top bit set, so this case is gone: %v", err)
	}
	if AddressWords(forged) == AddressWords(address) {
		t.Error("an address that no key opens has the words of the true address")
	}
	sealed := sealFor(t, []byte("x"), bad)
	if in := FromPackageKey("p", sealed, key); in.Opened {
		t.Error("the reader's key opens a file that was sealed to the address with the top bit set: the case of SHARE-PLAN.md F6 is gone")
	}
}

// bech32Data reads the data of a bech32 text back, without its checksum.
func bech32Data(text string) []byte {
	const charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"
	_, rest, _ := strings.Cut(text, "1")
	rest = rest[:len(rest)-6]
	var out []byte
	acc, bits := 0, 0
	for _, c := range rest {
		acc, bits = acc<<5|strings.IndexRune(charset, c), bits+5
		if bits >= 8 {
			bits -= 8
			out = append(out, byte(acc>>bits))
		}
	}
	return out
}

// TestParseKey: a key file is a standard age identity file with exactly
// one identity. What bean says of any other file holds nothing of it.
func TestParseKey(t *testing.T) {
	key, second := newKey(t), newKey(t)
	// As age-keygen writes one, with both of its comment lines.
	file := "# created: 2026-11-18T12:00:00Z\n# public key: " + key.Recipient().String() + "\n" + key.String() + "\n"
	got, err := ParseKey([]byte(file))
	if err != nil || got.Recipient().String() != key.Recipient().String() {
		t.Fatalf("a key file of age-keygen: %v", err)
	}
	// The address comes from the secret, not from the comment line.
	lying := "# public key: " + second.Recipient().String() + "\n" + key.String() + "\n"
	if got, err := ParseKey([]byte(lying)); err != nil || got.Recipient().String() != key.Recipient().String() {
		t.Errorf("a comment line that names another address: %v", err)
	}
	words, _ := NewWords()
	protected, _ := Seal([]byte(file), words)
	for name, c := range map[string]struct {
		data []byte
		want error
	}{
		"two identities":            {[]byte(key.String() + "\n" + second.String() + "\n"), ErrKeyFile},
		"no identity":               {[]byte("# public key: " + key.Recipient().String() + "\n"), ErrKeyFile},
		"empty":                     {nil, ErrKeyFile},
		"an address":                {[]byte(key.Recipient().String() + "\n"), ErrKeyFile},
		"a damaged secret":          {[]byte(key.String()[:60] + "\n"), ErrKeyFile},
		"a secret and another line": {[]byte(key.String() + "\nmy health notes\n"), ErrKeyFile},
		"protected":                 {protected, ErrKeyProtected},
		"protected, in armor":       {[]byte("\n-----BEGIN AGE ENCRYPTED FILE-----\nYWdlLWVuY3J5cHRpb24ub3JnL3YxCg==\n-----END AGE ENCRYPTED FILE-----\n"), ErrKeyProtected},
	} {
		_, err := ParseKey(c.data)
		if err != c.want {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, e := range []error{ErrKeyFile, ErrKeyProtected} {
		if strings.Contains(e.Error(), "%") {
			t.Errorf("a line about a key file is not fixed: %s", e)
		}
	}
}
