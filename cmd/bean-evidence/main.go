// bean-evidence is a disposable, host-software-only JWS evidence experiment.
// It deliberately does not import Bean core or implement device attestation.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

const profile = "bean-evidence-experiment"

var limits = map[string]int64{"receipt.jws": 16384, "raw.bin": 1048576, "settings.json": 4096}
var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)
var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
var integer = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
var b64chars = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type result struct {
	Signature bool     `json:"signature_valid"`
	Evidence  bool     `json:"evidence_complete"`
	Trusted   bool     `json:"source_trusted"`
	Replay    bool     `json:"replay_available"`
	Accuracy  bool     `json:"accuracy_validated"`
	Assurance string   `json:"assurance"`
	Freshness string   `json:"freshness"`
	Status    string   `json:"status"`
	Mean      *float64 `json:"mean_square_iq,omitempty"`
}

func encode(v any) []byte {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(v)
	return bytes.TrimSuffix(b.Bytes(), []byte{'\n'})
}
func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
func decode(s string) ([]byte, error) {
	if !b64chars.MatchString(s) {
		return nil, fmt.Errorf("base64url")
	}
	b, e := base64.RawURLEncoding.Strict().DecodeString(s)
	return b, e
}

// Reject noncanonical field encodings and the five y coordinates of the eight
// small-order points, independently of permissive library verification behavior.
// The signature library still performs curve decoding and the signature equation.
func safePoint(b []byte) bool {
	if len(b) != 32 {
		return false
	}
	y := append([]byte(nil), b...)
	y[31] &= 127
	p, _ := hex.DecodeString("edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f")
	less := false
	for i := 31; i >= 0; i-- {
		if y[i] < p[i] {
			less = true
			break
		}
		if y[i] > p[i] {
			return false
		}
	}
	if !less {
		return false
	}
	switch hex.EncodeToString(y) {
	case "0000000000000000000000000000000000000000000000000000000000000000",
		"0100000000000000000000000000000000000000000000000000000000000000",
		"ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f",
		"26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05",
		"c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac037a":
		return false
	}
	return true
}

func numeric(v any) bool {
	switch x := v.(type) {
	case json.Number:
		return integer.MatchString(string(x))
	case map[string]any:
		for _, v := range x {
			if !numeric(v) {
				return false
			}
		}
	case []any:
		for _, v := range x {
			if !numeric(v) {
				return false
			}
		}
	}
	return true
}
func parse(b []byte) (map[string]any, error) {
	depth := 0
	quoted := false
	for _, c := range b {
		if c > 127 || c == '\\' {
			return nil, fmt.Errorf("JSON subset")
		}
		if c == '"' {
			quoted = !quoted
		}
		if !quoted {
			if c == '[' || c == '{' {
				depth++
			}
			if c == ']' || c == '}' {
				depth--
			}
			if depth > 8 {
				return nil, fmt.Errorf("depth")
			}
		}
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var v map[string]any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	if v == nil || !numeric(v) || !bytes.Equal(encode(v), b) {
		return nil, fmt.Errorf("noncanonical JSON")
	}
	return v, nil
}
func fields(m map[string]any, keys ...string) bool {
	if len(m) != len(keys) {
		return false
	}
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}
func str(v any) string { s, _ := v.(string); return s }
func number(v any) int64 {
	n, ok := v.(json.Number)
	if !ok {
		return -1
	}
	i, e := n.Int64()
	if e != nil {
		return -1
	}
	return i
}

// Open through a confined root and refuse final symlinks and special files.
// Hash in chunks; retain at most the experiment's bounded file size for replay.
func read(root *os.Root, name string) ([]byte, string, error) {
	f, e := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return nil, "", e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, "", e
	}
	if !st.Mode().IsRegular() || st.Size() > limits[name] {
		return nil, "", fmt.Errorf("unsafe/large file")
	}
	h := sha256.New()
	var b bytes.Buffer
	n, e := io.Copy(io.MultiWriter(h, &b), io.LimitReader(f, limits[name]+1))
	if e != nil || n > limits[name] {
		return nil, "", fmt.Errorf("read/size")
	}
	return b.Bytes(), hex.EncodeToString(h.Sum(nil)), nil
}
func verify(dir string) (result, int) {
	r := result{Assurance: "host-software", Freshness: "unknown", Status: "invalid"}
	st, e := os.Lstat(dir)
	if e != nil || !st.IsDir() {
		return r, 2
	}
	root, e := os.OpenRoot(dir)
	if e != nil {
		return r, 2
	}
	defer root.Close()
	d, e := root.Open(".")
	if e != nil {
		return r, 2
	}
	defer d.Close()
	actual, e := d.Stat()
	if e != nil || !os.SameFile(st, actual) {
		return r, 2
	}
	receipt, _, e := read(root, "receipt.jws")
	if e != nil {
		return r, 2
	}
	parts := strings.Split(string(receipt), ".")
	if len(parts) != 3 {
		return r, 2
	}
	hb, e := decode(parts[0])
	if e != nil {
		return r, 2
	}
	h, e := parse(hb)
	if e != nil {
		return r, 2
	}
	pb, e := decode(parts[1])
	if e != nil {
		return r, 2
	}
	p, e := parse(pb)
	if e != nil {
		return r, 2
	}
	if h["alg"] != "Ed25519" || h["typ"] != profile || p["profile"] != profile {
		r.Status = "unsupported"
		return r, 3
	}
	k, ok := h["jwk"].(map[string]any)
	if !fields(h, "alg", "typ", "jwk") || !ok || !fields(k, "kty", "crv", "x") || k["kty"] != "OKP" || k["crv"] != "Ed25519" {
		return r, 2
	}
	r.Status = "signature_invalid"
	key, e := decode(str(k["x"]))
	if e != nil || !safePoint(key) {
		return r, 4
	}
	sig, e := decode(parts[2])
	if e != nil || len(sig) != 64 || !safePoint(sig[:32]) || !ed25519.Verify(key, []byte(parts[0]+"."+parts[1]), sig) {
		return r, 4
	}
	r.Signature = true
	r.Status = "invalid"
	aa, ok := p["artifacts"].([]any)
	if !fields(p, "profile", "capture", "complete", "assurance", "artifacts") || p["complete"] != true || p["assurance"] != "host-software" || !hex32.MatchString(str(p["capture"])) || !ok || len(aa) != 2 {
		return r, 2
	}
	names := []string{"raw.bin", "settings.json"}
	roles := []string{"raw", "settings"}
	for i, v := range aa {
		a, ok := v.(map[string]any)
		if !ok || !fields(a, "name", "role", "length", "sha256") || a["name"] != names[i] || a["role"] != roles[i] || number(a["length"]) <= 0 || number(a["length"]) > limits[names[i]] || !hex64.MatchString(str(a["sha256"])) {
			return r, 2
		}
	}
	entries, e := d.ReadDir(4)
	if e != nil && e != io.EOF {
		return r, 2
	}
	if len(entries) > 3 {
		return r, 2
	}
	for _, entry := range entries {
		if _, ok := limits[entry.Name()]; !ok {
			return r, 2
		}
	}
	r.Status = "evidence_mismatch"
	data := map[string][]byte{}
	for i, v := range aa {
		a := v.(map[string]any)
		b, digest, e := read(root, names[i])
		if e != nil || int64(len(b)) != number(a["length"]) || digest != a["sha256"] {
			return r, 5
		}
		data[names[i]] = b
	}
	r.Evidence = true
	r.Status = "complete"
	s, e := parse(data["settings.json"])
	if e == nil && fields(s, "format", "frames", "sample_rate_hz", "method") && s["format"] == "complex-i16-le" && s["method"] == "mean-square-iq-counts@1" && number(s["frames"]) > 0 && number(s["frames"]) <= 262144 && number(s["sample_rate_hz"]) > 0 && number(s["sample_rate_hz"]) <= 2147483647 && int64(len(data["raw.bin"])) == 4*number(s["frames"]) {
		var sum int64
		b := data["raw.bin"]
		for i := 0; i < len(b); i += 2 {
			n := int64(int16(binary.LittleEndian.Uint16(b[i:])))
			sum += n * n
		}
		mean := float64(sum) / float64(number(s["frames"]))
		r.Mean = &mean
		r.Replay = true
	}
	return r, 0
}

// fixture is explicitly synthetic and uses the public test seed, never a device key.
func fixture(dir string) error {
	if e := os.Mkdir(dir, 0700); e != nil {
		return e
	}
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}
	key := ed25519.NewKeyFromSeed(seed)
	nonce := make([]byte, 16)
	if _, e := rand.Read(nonce); e != nil {
		return e
	}
	raw := make([]byte, 16)
	for i, n := range []int16{100, 0, 0, 50, -25, 0, 0, -12} {
		binary.LittleEndian.PutUint16(raw[i*2:], uint16(n))
	}
	settings := encode(map[string]any{"format": "complex-i16-le", "frames": 4, "sample_rate_hz": 1000, "method": "mean-square-iq-counts@1"})
	artifacts := []any{}
	for i, b := range [][]byte{raw, settings} {
		name := []string{"raw.bin", "settings.json"}[i]
		role := []string{"raw", "settings"}[i]
		if e := os.WriteFile(filepath.Join(dir, name), b, 0600); e != nil {
			return e
		}
		digest := sha256.Sum256(b)
		artifacts = append(artifacts, map[string]any{"name": name, "role": role, "length": len(b), "sha256": hex.EncodeToString(digest[:])})
	}
	h := map[string]any{"alg": "Ed25519", "typ": profile, "jwk": map[string]any{"kty": "OKP", "crv": "Ed25519", "x": b64(key.Public().(ed25519.PublicKey))}}
	p := map[string]any{"profile": profile, "capture": hex.EncodeToString(nonce), "complete": true, "assurance": "host-software", "artifacts": artifacts}
	message := b64(encode(h)) + "." + b64(encode(p))
	receipt := message + "." + b64(ed25519.Sign(key, []byte(message)))
	return os.WriteFile(filepath.Join(dir, "receipt.jws"), []byte(receipt), 0600)
}
func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: bean-evidence verify|fixture <capture-directory>")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "verify":
		r, code := verify(os.Args[2])
		fmt.Println(string(encode(r)))
		os.Exit(code)
	case "fixture":
		if e := fixture(os.Args[2]); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown command")
		os.Exit(1)
	}
}
