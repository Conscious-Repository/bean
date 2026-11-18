package bundle

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"filippo.io/age"

	"github.com/Conscious-Repository/bean/core"
)

// A sealed package is a bundle that only its reader can open: the bundle's
// folder as one zip file, encrypted with a passphrase in the age format
// (age-encryption.org/v1). The passphrase is six words that bean makes and
// the sender gives to the reader by another way than the file. Any tool
// that reads age files opens a package: bean invents no cipher.

const (
	// sealWords words of a list of 2048 are 66 bits that no person chose.
	sealWords = 6
	// sealWork is the scrypt work factor: 2^16 steps and 64 MiB, which a
	// phone's browser can do. With 66 random bits behind it, a guesser
	// gains nothing from a lower cost.
	sealWork = 16
	// sealHeader starts every age file.
	sealHeader = "age-encryption.org/v1\n"
	// maxPackage is the most that a package may hold once it is open.
	maxPackage = 1 << 30
)

// IsSealed reports whether data is a sealed package.
func IsSealed(data []byte) bool { return bytes.HasPrefix(data, []byte(sealHeader)) }

// What a file asks of its reader, by its header alone. The header is not
// secret, and no work is done to read it.
const (
	AsksWords = "words" // the six words that its sender made
	AsksKey   = "key"   // the key file of the address that it was sealed to
	AsksOther = "other" // an age file that bean does not open
)

// isArmor reports the first line of an age file in armor (PEM).
func isArmor(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimLeft(data, " \t\r\n"), []byte("-----BEGIN AGE ENCRYPTED FILE-----"))
}

// maxHeader is how far into a file its header may reach. A header for one
// reader has about 200 bytes.
const maxHeader = 4 << 10

// Asks is the one scan that decides what a file is. It returns "" for a
// file that is not sealed, and otherwise what the file asks for. For
// AsksOther it also says why bean does not open the file.
//
// The lines after the first, split on LF, up to the first line that starts
// "---", hold the stanzas: one for each reader, and any that a tool adds.
// bean opens a file that has exactly one, for words or for an X25519 key.
// A file for two readers is refused, though the age tool opens it: a
// package is for one reader. A carriage return is
// refused, since some tools end a line at one and would count other lines.
func Asks(data []byte) (asks, why string) {
	// An age file in armor is sealed, and is not a package as bean writes
	// one. The age tool opens it, or takes the armor off.
	if isArmor(data) {
		return AsksOther, "it is an age file in armor, which bean does not open. The age tool opens such a file"
	}
	if !IsSealed(data) {
		return "", ""
	}
	head := data[len(sealHeader):min(len(data), maxHeader)]
	readers, kind, closed := 0, AsksOther, false
	for len(head) > 0 && !closed {
		line, rest, _ := bytes.Cut(head, []byte("\n"))
		head = rest
		switch {
		case bytes.HasPrefix(line, []byte("---")):
			closed = true
		case bytes.IndexByte(line, '\r') >= 0:
			return AsksOther, "its header holds a carriage return, which no package has"
		case bytes.HasPrefix(line, []byte("-> scrypt ")):
			readers, kind = readers+1, AsksWords
		case bytes.HasPrefix(line, []byte("-> X25519 ")):
			readers, kind = readers+1, AsksKey
		case bytes.HasPrefix(line, []byte("->")):
			// A reader of another kind is counted, and judged below: beside
			// a reader that bean knows, it makes two readers.
			readers, kind = readers+1, AsksOther
		}
	}
	switch {
	case !closed:
		return AsksOther, "its header is damaged, or longer than a package has"
	case readers != 1:
		return AsksOther, fmt.Sprintf("its header has %d stanzas. bean opens a package that has one, for one reader, though the age tool opens one with more", readers)
	case kind == AsksOther:
		return AsksOther, "it is sealed in a way that bean does not open: not with words, and not to an address"
	}
	return kind, ""
}

// NewWords makes the words for one package.
func NewWords() (string, error) {
	words := make([]string, sealWords)
	for i := range words {
		// Two bytes hold 16 bits and the list needs 11: the low 11 bits of
		// a uniform 16-bit number are uniform.
		var b [2]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		words[i] = core.WordAt(int(binary.BigEndian.Uint16(b[:])) % core.WordCount)
	}
	return strings.Join(words, " "), nil
}

// CleanWords reads the words as a person types them: in any letter case,
// with any space between. It refuses what cannot be a package's words, so
// that a typing mistake is named before any work is done.
func CleanWords(text string) (string, error) {
	// A person may write the words with commas, hyphens, full stops or
	// semicolons between them, or after the last one.
	words := strings.Fields(strings.NewReplacer(",", " ", "-", " ", ".", " ", ";", " ").Replace(strings.ToLower(text)))
	if len(words) != sealWords {
		return "", fmt.Errorf("a package opens with %d words, and these are %d", sealWords, len(words))
	}
	for _, w := range words {
		if !core.IsWord(w) {
			return "", fmt.Errorf("%q is not one of the words bean uses: check the spelling", w)
		}
	}
	return strings.Join(words, " "), nil
}

// Seal encrypts a packed bundle with the words.
func Seal(packed []byte, words string) ([]byte, error) {
	words, err := CleanWords(words)
	if err != nil {
		return nil, err
	}
	to, err := age.NewScryptRecipient(words)
	if err != nil {
		return nil, err
	}
	to.SetWorkFactor(sealWork)
	var out bytes.Buffer
	w, err := age.Encrypt(&out, to)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(packed); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ErrWords is returned when the words do not open the package. A package
// whose header was changed on the way fails the same way: until it opens,
// nothing in it can be told from a wrong word.
var ErrWords = errors.New("these words do not open it, or it was changed on the way")

// Unseal opens a sealed package with its words and returns the packed
// bundle. A package that was changed on the way does not open.
func Unseal(sealed []byte, words string) ([]byte, error) {
	words, err := CleanWords(words)
	if err != nil {
		return nil, err
	}
	id, err := age.NewScryptIdentity(words)
	if err != nil {
		return nil, err
	}
	// A package sealed for a key, or by another tool's rules, is not one
	// that words open. The header is not secret, so this is said plainly.
	if !IsSealed(sealed) || !bytes.HasPrefix(sealed[len(sealHeader):], []byte("-> scrypt ")) {
		return nil, errors.New("it is not sealed with words: this is another kind of age file")
	}
	// bean seals with one work factor. A header that asks for more is not
	// bean's, or was changed to keep a reader's machine busy.
	id.SetMaxWorkFactor(sealWork)
	r, err := age.Decrypt(bytes.NewReader(sealed), id)
	if err != nil {
		var none *age.NoIdentityMatchError
		if errors.As(err, &none) {
			return nil, ErrWords
		}
		return nil, fmt.Errorf("it is damaged, or not a package that bean sealed: %w", err)
	}
	return readOpened(r, maxPackage)
}

// readOpened reads what an opened package holds, whole or not at all, and
// no more than limit bytes. A package that was cut short gives a part of
// what it held before the error: nothing of that part is handed on.
func readOpened(r io.Reader, limit int64) ([]byte, error) {
	packed, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("it is damaged: %w", err)
	}
	if int64(len(packed)) > limit {
		return nil, errors.New("it holds more than a package can")
	}
	return packed, nil
}

// ParseAddress reads a reader's address: a recipient of the age format for
// X25519. bean adds no decoder of its own. It takes only the text that the
// library writes back, so one address has one spelling, and one set of
// address words.
func ParseAddress(text string) (*age.X25519Recipient, error) {
	to, err := age.ParseX25519Recipient(text)
	// Library 1.2.1 refuses capitals and spaces by itself. The comparison
	// holds the rule against a later library that reads more.
	if err != nil || to.String() != text {
		return nil, errors.New("this is not an address in lower case: an address starts with age1 and has 62 characters")
	}
	return to, nil
}

// AddressWords are eight words made from the SHA-256 of the address text,
// as identity words are made from an id. They are not a secret. A sender
// gives them to bean with each share, so that a wrong or swapped address
// is caught when the words came by another channel than the address.
func AddressWords(address string) string {
	sum := sha256.Sum256([]byte(address))
	return core.IDWords("sha256:" + hex.EncodeToString(sum[:]))
}

// CleanAddressWords reads address words as a person types them, in any
// letter case and with any space or mark between.
func CleanAddressWords(text string) string {
	return strings.Join(strings.Fields(strings.NewReplacer(",", " ", "-", " ", ".", " ", ";", " ").Replace(strings.ToLower(text))), " ")
}

// Errors of a key file. Each is one fixed line, with nothing of the file
// in it: a key file holds a secret.
var (
	ErrKeyProtected = errors.New("it is an encrypted age file. A key file that is protected with a passphrase is one, and this bean does not open such a key file")
	ErrKeyFile      = errors.New("it is not a key file that bean reads: a file with exactly one age identity, as bean address -out or age-keygen writes it")
)

// ParseKey reads a key file: a standard age identity file with exactly one
// identity. bean tries that key and no other.
func ParseKey(data []byte) (*age.X25519Identity, error) {
	// A key file that is protected with a passphrase is itself an age
	// file, plain or in armor.
	if IsSealed(data) || isArmor(data) {
		return nil, ErrKeyProtected
	}
	ids, err := age.ParseIdentities(bytes.NewReader(data))
	if err != nil || len(ids) != 1 {
		return nil, ErrKeyFile
	}
	key, ok := ids[0].(*age.X25519Identity)
	if !ok {
		return nil, ErrKeyFile
	}
	return key, nil
}

// ownKey keeps the file's own key while a package is sealed, and opens that
// one package with it. The library hands the key to the code that wraps
// it, through its public interface. So bean can check what it has just
// sealed for another person, whose key it does not have.
type ownKey struct {
	to  age.Recipient
	key []byte
}

func (o *ownKey) Wrap(fileKey []byte) ([]*age.Stanza, error) {
	o.key = bytes.Clone(fileKey)
	return o.to.Wrap(fileKey)
}

// Unwrap gives the key that Wrap kept. Before Wrap ran there is none, and
// the library then says that no key opens the file.
func (o *ownKey) Unwrap([]*age.Stanza) ([]byte, error) { return o.key, nil }

// SealTo encrypts a packed bundle to a reader's address. It then opens the
// result once more with the file's own key, and returns the sealed bytes
// and what a reader gets out of them, for the caller to check. The file's
// own key does not leave this function.
//
// This does not open the one stanza that is for the reader: only the
// reader's key does that. A test with test keys holds that code.
func SealTo(name string, packed []byte, to *age.X25519Recipient) ([]byte, Input, error) {
	own := &ownKey{to: to}
	defer func() { clear(own.key) }()
	var out bytes.Buffer
	w, err := age.Encrypt(&out, own)
	if err != nil {
		return nil, Input{}, err
	}
	if _, err := w.Write(packed); err != nil {
		return nil, Input{}, err
	}
	if err := w.Close(); err != nil {
		return nil, Input{}, err
	}
	return out.Bytes(), FromPackageKey(name, out.Bytes(), own), nil
}

// PackFiles makes one zip file of a bundle's files, each under its path
// in the bundle's folder.
func PackFiles(files map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, name := range names {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(files[name]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Pack makes one zip file of a bundle's folder.
func Pack(dir string) ([]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a plain file", path)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)], err = os.ReadFile(path)
		return err
	})
	if err != nil {
		return nil, err
	}
	return PackFiles(files)
}

// FromPackage reads a bundle from one file: a zip file of the bundle's
// folder, or a sealed package with the words that open it. Asks of what it
// returns says what the file asks for, before the words are looked at.
func FromPackage(name string, data []byte, words string) Input {
	asks, why := Asks(data)
	locked := func(format string, a ...any) Input {
		return Input{Fatal: []string{fmt.Sprintf(format, a...)}, Locked: true, Asks: asks}
	}
	switch asks {
	case "":
		return FromZip(name, data)
	case AsksOther:
		return locked("%s was not opened: %s", name, why)
	case AsksKey:
		return locked("%s is sealed to a key, not with words. Open it with -key <your key file>. If you no longer have the file, you cannot open it: ask the sender to share again, for a new address", name)
	}
	if strings.TrimSpace(words) == "" {
		return locked("%s is sealed: it opens with the six words that its sender gave you", name)
	}
	packed, err := Unseal(data, words)
	if err != nil {
		return locked("%s was not opened: %s", name, err)
	}
	in := FromZip(name, packed)
	in.Opened, in.Asks = true, asks
	return in
}

// FromPackageKey reads a bundle from a package that is sealed to a key,
// with that key. It tries the one key and no other.
func FromPackageKey(name string, data []byte, key age.Identity) Input {
	asks, why := Asks(data)
	locked := func(format string, a ...any) Input {
		return Input{Fatal: []string{fmt.Sprintf(format, a...)}, Locked: true, Asks: asks}
	}
	switch asks {
	case "":
		return Input{Fatal: []string{fmt.Sprintf("%s is not sealed: a key opens only a package that is sealed to it", name)}}
	case AsksOther:
		return locked("%s was not opened: %s", name, why)
	case AsksWords:
		return locked("%s is sealed with words, and a key does not open it: give the six words", name)
	}
	r, err := age.Decrypt(bytes.NewReader(data), key)
	if err != nil {
		var none *age.NoIdentityMatchError
		if errors.As(err, &none) {
			return locked("%s was not opened: it was sealed to another address, or it was changed on the way. bean cannot tell which. bean address -key <file> shows the address of a key file", name)
		}
		return locked("%s was not opened: it is damaged, or not a package that bean sealed: %s", name, err)
	}
	packed, err := readOpened(r, maxPackage)
	if err != nil {
		return locked("%s was not opened: %s", name, err)
	}
	in := FromZip(name, packed)
	in.Opened, in.Asks = true, asks
	return in
}
