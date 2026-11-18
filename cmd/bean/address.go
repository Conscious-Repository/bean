package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"filippo.io/age"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/bundle"
)

// bean address makes a reader's key, or shows the address of one
// (docs/SHARE.md, "Seal it to a reader's address").
//
// An address is a recipient of the age format for X25519, and its key is a
// standard age identity file. The person places the file. bean keeps no
// such key and no list of them, and no copy of a log holds one.

// maxKeyFile is the most that a key file is read. One identity with its
// comment lines has about 200 bytes.
const maxKeyFile = 64 << 10

// addressLines are what a person must know before they give an address
// away. bean address prints them every time.
var addressLines = []string{
	"The key is this one file. bean restore, bean export and a copy of your log do not hold it.",
	"Without the file you cannot open what was sealed to it. Then make a new address, and ask for a new share.",
	"Whoever has a copy of the file opens every package that was sealed to it. That holds for as long as a copy of the package exists.",
	"A folder that syncs, and a backup, each hold a copy of the file.",
	"An address cannot be taken back. If the file is stolen, tell each sender to stop, by another channel.",
	"A program that runs as you, such as an assistant with a shell, opens such a package with no word typed.",
	"A sender needs the eight address words from you, and gives them to bean with every share.",
	"No call tells you which share a package is. The check shows its time and id. Ask the sender, by another way than the package came.",
	"To open on a second machine you copy the file, and every copy opens everything.",
	"If you delete the file, tell each sender. Their next package is one that you cannot open.",
	"For a package on a public host: make an address for that one sender. Delete the key file after you have read and answered.",
}

func cmdAddress(s *core.Store, args []string) {
	fs := flag.NewFlagSet("address", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	out := fs.String("out", "", "make a key, and write it into this new file")
	keyFile := fs.String("key", "", "show the address of this key file")
	// Exactly one of the two, and with a value: an empty value is not "no
	// flag".
	err := fs.Parse(args)
	given := 0
	fs.Visit(func(*flag.Flag) { given++ })
	if err != nil || fs.NArg() != 0 || given != 1 || (*out == "") == (*keyFile == "") {
		usage()
	}
	var key *age.X25519Identity
	if *keyFile != "" {
		var err error
		if key, err = readKey(*keyFile); err != nil {
			fail(err)
		}
	} else {
		path, err := filepath.Abs(*out)
		if err != nil {
			fail(err)
		}
		// A copy of a log goes to other machines, and into backups. A key
		// file inside it would go along.
		if hasLog(s) && insideLog(path) {
			fail(fmt.Errorf("%s is inside your log: a key file goes somewhere else, since a copy of the log would carry it", path))
		}
		if key, err = age.GenerateX25519Identity(); err != nil {
			fail(err)
		}
		// The comment line that age-keygen writes, then the secret.
		text := "# public key: " + key.Recipient().String() + "\n" + key.String() + "\n"
		if err := writeNew(path, []byte(text)); err != nil {
			if errors.Is(err, os.ErrExist) {
				fail(fmt.Errorf("%s is there already: a key file is a new file, and bean writes over nothing", path))
			}
			if errors.Is(err, os.ErrNotExist) {
				fail(fmt.Errorf("no key was made: the folder %s is not there. Make it first: bean makes no folder", filepath.Dir(path)))
			}
			fail(fmt.Errorf("no key was made: %w", err))
		}
		if err := readBack(path, key); err != nil {
			fail(err)
		}
		fmt.Printf("wrote the key file %s\n", path)
	}
	address := key.Recipient().String()
	fmt.Println("address", address)
	fmt.Println("address words", bundle.AddressWords(address))
	// First, since the words are worth nothing when they travel with the
	// address: whoever changes one on the way changes both.
	fmt.Println("\nGive the address by one channel, and the address words by another, such as a call.")
	fmt.Println("Neither is a secret. The key file is. Before you give the address away:")
	for _, line := range addressLines {
		fmt.Println("  - " + line)
	}
}

// readBack holds a key file that was just written to the key that was
// made. The address is shown only for a file that reads back as that key:
// an address whose key file is not there opens nothing. A file that does
// not read back is removed.
func readBack(path string, key *age.X25519Identity) error {
	if back, err := readKey(path); err == nil && back.String() == key.String() {
		return nil
	}
	if os.Remove(path) != nil {
		return fmt.Errorf("no address was made: %s did not read back as it was written. Delete it", path)
	}
	return fmt.Errorf("no address was made: %s did not read back as it was written, and is removed", path)
}

// readKey reads the key file that -key names: exactly one age identity,
// and not one that is protected with a passphrase. What it says of a file
// that is not such a key holds nothing of the file. It warns, on standard
// error, when the mode of the file lets other users read it.
func readKey(path string) (*age.X25519Identity, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("the key file cannot be read: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxKeyFile+1))
	if err != nil {
		return nil, fmt.Errorf("the key file cannot be read: %w", err)
	}
	if len(data) > maxKeyFile {
		return nil, fmt.Errorf("the key file %s was not used: %w", path, bundle.ErrKeyFile)
	}
	key, err := bundle.ParseKey(data)
	if err != nil {
		return nil, fmt.Errorf("the key file %s was not used: %w", path, err)
	}
	// A warning, and no refusal: the key is read, and the person is told.
	// It says what the mode of the file says, and no more. Windows keeps no
	// such mode.
	if info, err := f.Stat(); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o044 != 0 && runtime.GOOS != "windows" {
		fmt.Fprintf(os.Stderr, "warning: the mode of the key file %s is %04o, which lets other users of this machine read it. Whoever has a copy of the file opens every package that was sealed to its address. To take that away: chmod 600 <the file>\n", safe(path), info.Mode().Perm())
	}
	return key, nil
}
