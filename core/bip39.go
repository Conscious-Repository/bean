package core

import (
	"crypto/sha256"
	_ "embed"
	"errors"
	"strings"
)

// bip39-english.txt is the BIP-39 English word list from
// https://github.com/bitcoin/bips/blob/master/bip-0039/english.txt
// (BIP-39 is licensed under the MIT license; see NOTICE). Its SHA-256 is
// 2f5eed53a4727b4bf8880d8f3f199efc90e58503646d9ff8eff3a2ed3b24dbda.
//
//go:embed bip39-english.txt
var wordListText string

const wordListSHA256 = "2f5eed53a4727b4bf8880d8f3f199efc90e58503646d9ff8eff3a2ed3b24dbda"

var (
	wordList  []string
	wordIndex map[string]int
)

func init() {
	sum := sha256.Sum256([]byte(wordListText))
	if got := hexString(sum[:]); got != wordListSHA256 {
		panic("bip39 word list checksum mismatch: " + got)
	}
	wordList = strings.Split(strings.TrimRight(wordListText, "\n"), "\n")
	if len(wordList) != 2048 {
		panic("bip39 word list must have 2048 words")
	}
	wordIndex = make(map[string]int, 2048)
	for i, w := range wordList {
		wordIndex[w] = i
	}
}

func hexString(b []byte) string {
	const hx = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[i*2], out[i*2+1] = hx[c>>4], hx[c&0xf]
	}
	return string(out)
}

// ErrSeedMismatch: the words are not a valid seed, or not this identity's.
var ErrSeedMismatch = errors.New("seed does not match")

// SeedToWords encodes 32 entropy bytes as 24 BIP-39 English words
// (SPEC.md 4.10). No PBKDF2 and no passphrase.
func SeedToWords(entropy []byte) ([]string, error) {
	if len(entropy) != 32 {
		return nil, errors.New("seed must be 32 bytes")
	}
	sum := sha256.Sum256(entropy)
	bits := append(append([]byte{}, entropy...), sum[0]) // 256 + 8 bits
	words := make([]string, 24)
	for i := 0; i < 24; i++ {
		var idx int
		for j := 0; j < 11; j++ {
			bit := i*11 + j
			idx = idx<<1 | int(bits[bit/8]>>(7-bit%8)&1)
		}
		words[i] = wordList[idx]
	}
	return words, nil
}

// WordsToSeed decodes 24 words to 32 entropy bytes and checks the checksum.
func WordsToSeed(words []string) ([]byte, error) {
	if len(words) != 24 {
		return nil, ErrSeedMismatch
	}
	bits := make([]byte, 33)
	for i, w := range words {
		idx, ok := wordIndex[strings.ToLower(strings.TrimSpace(w))]
		if !ok {
			return nil, ErrSeedMismatch
		}
		for j := 0; j < 11; j++ {
			bit := i*11 + j
			if idx>>(10-j)&1 == 1 {
				bits[bit/8] |= 1 << (7 - bit%8)
			}
		}
	}
	entropy := bits[:32]
	sum := sha256.Sum256(entropy)
	if sum[0] != bits[32] {
		return nil, ErrSeedMismatch
	}
	return entropy, nil
}
