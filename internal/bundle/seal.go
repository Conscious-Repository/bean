package bundle

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/binary"
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
	packed, err := io.ReadAll(io.LimitReader(r, maxPackage+1))
	if err != nil {
		return nil, fmt.Errorf("it is damaged: %w", err)
	}
	if len(packed) > maxPackage {
		return nil, errors.New("it holds more than a package can")
	}
	return packed, nil
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
// folder, or a sealed package with the words that open it.
func FromPackage(name string, data []byte, words string) Input {
	if !IsSealed(data) {
		return FromZip(name, data)
	}
	if strings.TrimSpace(words) == "" {
		return Input{Fatal: []string{fmt.Sprintf("%s is sealed: it opens with the six words that its sender gave you", name)}, Locked: true}
	}
	packed, err := Unseal(data, words)
	if err != nil {
		return Input{Fatal: []string{fmt.Sprintf("%s was not opened: %s", name, err)}, Locked: true}
	}
	in := FromZip(name, packed)
	in.Opened = true
	return in
}
