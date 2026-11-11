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
	words := strings.Fields(strings.ToLower(text))
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

// ErrWords is returned when the words do not open the package.
var ErrWords = errors.New("these words do not open it")

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
	// A package that asks for more work than bean ever sets is not bean's,
	// and could keep a reader's machine busy for hours.
	id.SetMaxWorkFactor(sealWork + 2)
	r, err := age.Decrypt(bytes.NewReader(sealed), id)
	if err != nil {
		var none *age.NoIdentityMatchError
		if errors.As(err, &none) {
			return nil, ErrWords
		}
		return nil, fmt.Errorf("it is damaged, or not a package of bean: %w", err)
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

// Pack makes one zip file of a bundle's folder. The same folder gives the
// same bytes: the names are in order, and no time is written.
func Pack(dir string) ([]byte, error) {
	var names []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			names = append(names, filepath.ToSlash(rel))
		} else if !d.IsDir() {
			return fmt.Errorf("%s is not a plain file", path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			return nil, err
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// FromPackage reads a bundle from one file: a zip file of the bundle's
// folder, or a sealed package with the words that open it.
func FromPackage(name string, data []byte, words string) Input {
	if !IsSealed(data) {
		return FromZip(name, data)
	}
	if strings.TrimSpace(words) == "" {
		return Input{Fatal: []string{fmt.Sprintf("%s is sealed: it opens with the six words that its sender gave you", name)}}
	}
	packed, err := Unseal(data, words)
	if err != nil {
		return Input{Fatal: []string{fmt.Sprintf("%s was not opened: %s", name, err)}}
	}
	return FromZip(name, packed)
}
