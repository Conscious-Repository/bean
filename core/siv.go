package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/subtle"
	"errors"
)

// AES-SIV (RFC 5297) built from CMAC (RFC 4493), S2V and AES-CTR.

// BlobAD is the single associated data string for blobs (SPEC.md 4.9).
const BlobAD = "hlog/v0/blob"

var ErrBlobCorrupt = errors.New("blob corrupt")

type cmac struct {
	block  cipher.Block
	k1, k2 []byte
}

func newCMAC(key []byte) (*cmac, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	l := make([]byte, 16)
	b.Encrypt(l, l)
	k1 := dbl(l)
	k2 := dbl(k1)
	return &cmac{b, k1, k2}, nil
}

func dbl(in []byte) []byte {
	out := make([]byte, 16)
	var carry byte
	for i := 15; i >= 0; i-- {
		out[i] = in[i]<<1 | carry
		carry = in[i] >> 7
	}
	if carry != 0 {
		out[15] ^= 0x87
	}
	return out
}

func xor(dst, a, b []byte) {
	for i := range dst {
		dst[i] = a[i] ^ b[i]
	}
}

func (c *cmac) mac(m []byte) []byte {
	n := (len(m) + 15) / 16
	var last [16]byte
	if n == 0 {
		n = 1
		last[0] = 0x80
		xor(last[:], last[:], c.k2)
	} else if len(m)%16 == 0 {
		xor(last[:], m[(n-1)*16:], c.k1)
	} else {
		copy(last[:], m[(n-1)*16:])
		last[len(m)%16] = 0x80
		xor(last[:], last[:], c.k2)
	}
	x := make([]byte, 16)
	y := make([]byte, 16)
	for i := 0; i < n-1; i++ {
		xor(y, x, m[i*16:(i+1)*16])
		c.block.Encrypt(x, y)
	}
	xor(y, x, last[:])
	c.block.Encrypt(x, y)
	return x
}

func s2v(c *cmac, strings [][]byte) []byte {
	if len(strings) == 0 {
		one := make([]byte, 16)
		one[15] = 1
		return c.mac(one)
	}
	d := c.mac(make([]byte, 16))
	for _, s := range strings[:len(strings)-1] {
		d = dbl(d)
		xor(d, d, c.mac(s))
	}
	sn := strings[len(strings)-1]
	var t []byte
	if len(sn) >= 16 {
		t = append([]byte{}, sn...)
		xor(t[len(t)-16:], t[len(t)-16:], d)
	} else {
		d = dbl(d)
		pad := make([]byte, 16)
		copy(pad, sn)
		pad[len(sn)] = 0x80
		t = make([]byte, 16)
		xor(t, d, pad)
	}
	return c.mac(t)
}

func sivKeys(key []byte) (*cmac, cipher.Block, error) {
	if len(key) != 32 && len(key) != 48 && len(key) != 64 {
		return nil, nil, errors.New("siv: key must be 32, 48 or 64 bytes")
	}
	half := len(key) / 2
	c, err := newCMAC(key[:half])
	if err != nil {
		return nil, nil, err
	}
	b, err := aes.NewCipher(key[half:])
	if err != nil {
		return nil, nil, err
	}
	return c, b, nil
}

func ctrIV(v []byte) []byte {
	q := append([]byte{}, v...)
	q[8] &= 0x7f
	q[12] &= 0x7f
	return q
}

// sivEncrypt returns V || C. The associated data strings go first in the S2V
// vector and the plaintext last.
func sivEncrypt(key []byte, ad [][]byte, plaintext []byte) ([]byte, error) {
	c, b, err := sivKeys(key)
	if err != nil {
		return nil, err
	}
	v := s2v(c, append(append([][]byte{}, ad...), plaintext))
	out := make([]byte, 16+len(plaintext))
	copy(out, v)
	cipher.NewCTR(b, ctrIV(v)).XORKeyStream(out[16:], plaintext)
	return out, nil
}

func sivDecrypt(key []byte, ad [][]byte, data []byte) ([]byte, error) {
	c, b, err := sivKeys(key)
	if err != nil {
		return nil, err
	}
	if len(data) < 16 {
		return nil, ErrBlobCorrupt
	}
	v := data[:16]
	pt := make([]byte, len(data)-16)
	cipher.NewCTR(b, ctrIV(v)).XORKeyStream(pt, data[16:])
	t := s2v(c, append(append([][]byte{}, ad...), pt))
	if subtle.ConstantTimeCompare(t, v) != 1 {
		return nil, ErrBlobCorrupt
	}
	return pt, nil
}

// EncryptBlob encrypts a blob with the 64-byte blob key (SPEC.md 4.9).
func EncryptBlob(blobKey, plaintext []byte) ([]byte, error) {
	return sivEncrypt(blobKey, [][]byte{[]byte(BlobAD)}, plaintext)
}

// DecryptBlob reverses EncryptBlob. It returns blob corrupt when the
// synthetic IV does not match.
func DecryptBlob(blobKey, data []byte) ([]byte, error) {
	return sivDecrypt(blobKey, [][]byte{[]byte(BlobAD)}, data)
}
