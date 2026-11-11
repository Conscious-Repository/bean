// Package capture checks what a device hands over: a folder of capture
// statements that the device signed, with the raw data they name.
// docs/DEVICE.md has the format. bean does not read a device yet; this is
// the check that a device's output must pass, and tools/device/check.py is
// the same check in Python, with no code of bean.
package capture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Conscious-Repository/bean/core"
)

// The tags of the three objects of the format.
const (
	StatementTag = "capture/0"
	DeviceTag    = "device/0"
	CertTag      = "device-cert/0"
)

var (
	hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
	kind  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*(\.[a-z0-9_-]+)*$`)
)

// ID is the id of a statement or a certificate: the SHA-256 of its
// canonical bytes without the sig member. The signature is over this id,
// as a record's signature is over the record's id (SPEC.md 4.5).
func ID(obj map[string]any) string {
	rest := map[string]any{}
	for k, v := range obj {
		if k != "sig" {
			rest[k] = v
		}
	}
	sum := sha256.Sum256(core.Canonical(rest))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// plain reports a value the format allows: text of printable ASCII with no
// quote and no backslash, or a whole number that is not below zero. A
// device writes such values the same way on every processor, with no rule
// for escapes or fractions to get wrong.
func plain(v any) bool {
	switch x := v.(type) {
	case string:
		for _, c := range x {
			if c < 0x20 || c > 0x7e || c == '"' || c == '\\' {
				return false
			}
		}
		return true
	case float64:
		return x >= 0 && x <= 1<<53-1 && x == float64(int64(x))
	}
	return false
}

// deviceKey reports a key that a device or a factory may sign with: P-256,
// which secure elements hold, or Ed25519.
func deviceKey(s string) bool {
	return core.IsTaggedKey(s) && (strings.HasPrefix(s, "p256:") || strings.HasPrefix(s, "ed25519:"))
}

func text(m map[string]any, key string) (string, bool) {
	s, ok := m[key].(string)
	return s, ok && plain(s)
}

func whole(m map[string]any, key string) (int64, bool) {
	n, ok := m[key].(float64)
	return int64(n), ok && plain(n)
}

// keys reports whether m has exactly the keys of must, and of may if any.
func keys(m map[string]any, must []string, may ...string) bool {
	n := 0
	for _, k := range must {
		if _, ok := m[k]; !ok {
			return false
		}
		n++
	}
	for _, k := range may {
		if _, ok := m[k]; ok {
			n++
		}
	}
	return n == len(m)
}

// Statement is one capture, as the device signed it.
type Statement struct {
	ID      string
	Key     string
	Capture int64
	Boot    int64
	Prev    string
	Raw     []Raw
	Line    []byte
}

// Raw is one raw file that a statement names.
type Raw struct {
	Name, SHA256 string
	Bytes        int64
}

// ParseStatement reads one line of captures.jsonl and checks everything
// that the line alone can show. It returns why a line is not a statement.
func ParseStatement(line []byte) (*Statement, error) {
	v, err := core.Parse(line)
	obj, ok := v.(map[string]any)
	if err != nil || !ok {
		return nil, fmt.Errorf("not one JSON object")
	}
	if !bytes.Equal(core.Canonical(obj), line) {
		return nil, fmt.Errorf("not in canonical form: keys in order, no space, no escape")
	}
	if !keys(obj, []string{"bean", "device", "kind", "prev", "raw", "selftest", "seq", "settings", "sig", "time"}, "calibration") {
		return nil, fmt.Errorf("a member is missing, or one is there that the format does not have")
	}
	if tag, _ := text(obj, "bean"); tag != StatementTag {
		return nil, fmt.Errorf("bean is not %q", StatementTag)
	}
	dev, _ := obj["device"].(map[string]any)
	if !keys(dev, []string{"fw", "hw", "key", "model"}) {
		return nil, fmt.Errorf("device needs fw, hw, key and model, and nothing else")
	}
	for _, k := range []string{"fw", "hw", "model"} {
		if s, ok := text(dev, k); !ok || s == "" {
			return nil, fmt.Errorf("device.%s is not plain text", k)
		}
	}
	st := &Statement{Line: line, ID: ID(obj)}
	if st.Key, ok = text(dev, "key"); !ok || !deviceKey(st.Key) {
		return nil, fmt.Errorf("device.key is not a P-256 or Ed25519 key")
	}
	if k, ok := text(obj, "kind"); !ok || !kind.MatchString(k) {
		return nil, fmt.Errorf("kind is not a name such as ultrasound.bmode")
	}
	if c, has := obj["calibration"]; has {
		if s, ok := c.(string); !ok || !plain(s) || s == "" {
			return nil, fmt.Errorf("calibration is not plain text")
		}
	}
	if s, ok := text(obj, "selftest"); !ok || (s != "pass" && s != "fail" && s != "none") {
		return nil, fmt.Errorf("selftest is not pass, fail or none")
	}
	if t, ok := text(obj, "time"); !ok {
		return nil, fmt.Errorf("time is not text")
	} else if t != "" {
		if _, err := time.Parse("2006-01-02T15:04:05Z", t); err != nil {
			return nil, fmt.Errorf("time is not empty and not like 2026-11-11T17:00:00Z")
		}
	}
	seq, _ := obj["seq"].(map[string]any)
	if !keys(seq, []string{"boot", "capture"}) {
		return nil, fmt.Errorf("seq needs boot and capture, and nothing else")
	}
	var ok1, ok2 bool
	st.Boot, ok1 = whole(seq, "boot")
	st.Capture, ok2 = whole(seq, "capture")
	if !ok1 || !ok2 || st.Capture < 1 {
		return nil, fmt.Errorf("seq.boot and seq.capture are whole numbers, and capture starts at 1")
	}
	if st.Prev, ok = text(obj, "prev"); !ok || (st.Prev != "" && !core.IsID(st.Prev)) {
		return nil, fmt.Errorf("prev is not empty and not an id")
	}
	if (st.Prev == "") != (st.Capture == 1) {
		return nil, fmt.Errorf("prev is empty for capture 1 and for no other")
	}
	settings, ok := obj["settings"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("settings is not an object")
	}
	for k, v := range settings {
		if !plain(k) || !plain(v) {
			return nil, fmt.Errorf("settings.%s is not plain text or a whole number", k)
		}
	}
	list, ok := obj["raw"].([]any)
	if !ok || len(list) == 0 {
		return nil, fmt.Errorf("raw names no file")
	}
	for _, x := range list {
		m, _ := x.(map[string]any)
		r := Raw{}
		var ok1, ok2, ok3 bool
		r.Name, ok1 = text(m, "name")
		r.SHA256, ok2 = text(m, "sha256")
		r.Bytes, ok3 = whole(m, "bytes")
		if !keys(m, []string{"bytes", "name", "sha256"}) || !ok1 || !ok2 || !ok3 || r.Name == "" || !hex64.MatchString(r.SHA256) {
			return nil, fmt.Errorf("a raw entry needs a name, a sha256 of 64 hex digits and bytes")
		}
		st.Raw = append(st.Raw, r)
	}
	sig, ok := text(obj, "sig")
	if !ok || core.Verify(st.Key, st.ID, sig) != nil {
		return nil, fmt.Errorf("the signature is not the device key's signature of this statement")
	}
	return st, nil
}

// checkCert checks a factory certificate for key, and returns its issuer.
func checkCert(v any, key string) (string, error) {
	cert, ok := v.(map[string]any)
	if !ok || !keys(cert, []string{"bean", "hw", "issuer", "key", "model", "sig"}) {
		return "", fmt.Errorf("the certificate needs bean, hw, issuer, key, model and sig, and nothing else")
	}
	for _, k := range []string{"bean", "hw", "issuer", "key", "model", "sig"} {
		if _, ok := text(cert, k); !ok {
			return "", fmt.Errorf("the certificate's %s is not plain text", k)
		}
	}
	issuer := cert["issuer"].(string)
	if cert["bean"] != CertTag || cert["key"] != key || !deviceKey(issuer) {
		return "", fmt.Errorf("the certificate is not one for this device key")
	}
	if core.Verify(issuer, ID(cert), cert["sig"].(string)) != nil {
		return "", fmt.Errorf("the certificate is not signed by its issuer")
	}
	return issuer, nil
}

// CheckFolder checks a folder that a device handed over. It returns the
// lines to print and whether the folder passed. It changes nothing.
func CheckFolder(dir string) ([]string, bool) {
	var lines, problems []string
	bad := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }
	done := func() ([]string, bool) {
		sort.Strings(problems)
		for _, p := range problems {
			lines = append(lines, "problem: "+p)
		}
		if len(problems) > 0 {
			return append(lines, fmt.Sprintf("device folder FAILED: %d problems", len(problems))), false
		}
		return append(lines, "device folder ok"), true
	}
	// device.json: whose folder this is.
	data, err := os.ReadFile(filepath.Join(dir, "device.json"))
	if err != nil {
		bad("device.json cannot be read")
		return done()
	}
	v, err := core.Parse(bytes.TrimRight(data, "\n"))
	dev, ok := v.(map[string]any)
	if err != nil || !ok || !keys(dev, []string{"bean", "key", "time"}, "cert") || dev["bean"] != DeviceTag {
		bad("device.json is not a device object: bean, key, time, and cert if there is one")
		return done()
	}
	key, ok := text(dev, "key")
	if !ok || !deviceKey(key) {
		bad("device.json: key is not a P-256 or Ed25519 key")
		return done()
	}
	lines = append(lines, "device "+key)
	if t, ok := text(dev, "time"); !ok {
		bad("device.json: time is not text")
	} else if t != "" {
		if _, err := time.Parse("2006-01-02T15:04:05Z", t); err != nil {
			bad("device.json: time is not empty and not like 2026-11-11T17:00:00Z")
		}
	}
	if c, has := dev["cert"]; has {
		if issuer, err := checkCert(c, key); err != nil {
			bad("device.json: %s", err)
		} else {
			lines = append(lines, "certificate: issued by "+issuer)
		}
	} else {
		lines = append(lines, "certificate: none")
	}
	// captures.jsonl: one statement a line, a run with no gap.
	data, err = os.ReadFile(filepath.Join(dir, "captures.jsonl"))
	if err != nil {
		bad("captures.jsonl cannot be read")
		return done()
	}
	var prev *Statement
	named := map[string]int64{}
	n := 0
	for i, line := range bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n")) {
		if len(data) == 0 {
			break
		}
		st, err := ParseStatement(line)
		if err != nil {
			bad("captures.jsonl line %d: %s", i+1, err)
			prev = nil
			continue
		}
		n++
		if st.Key != key {
			bad("captures.jsonl line %d: signed by another key than the device's", i+1)
		}
		if prev != nil {
			if st.Capture != prev.Capture+1 || st.Prev != prev.ID {
				bad("captures.jsonl line %d: it does not follow the line before it: capture %d after %d", i+1, st.Capture, prev.Capture)
			}
			if st.Boot < prev.Boot {
				bad("captures.jsonl line %d: seq.boot went back", i+1)
			}
		}
		for _, r := range st.Raw {
			if size, twice := named[r.SHA256]; twice && size != r.Bytes {
				bad("captures.jsonl line %d: raw file %s is named with two sizes", i+1, r.SHA256)
			}
			named[r.SHA256] = r.Bytes
		}
		if prev == nil {
			lines = append(lines, fmt.Sprintf("first capture: %d", st.Capture))
		}
		prev = st
	}
	lines = append(lines, fmt.Sprintf("captures: %d", n))
	if n == 0 {
		bad("captures.jsonl holds no statement")
	}
	// raw/: exactly the files that the statements name.
	entries, err := os.ReadDir(filepath.Join(dir, "raw"))
	if err != nil {
		bad("the raw folder cannot be read")
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		size, ok := named[e.Name()]
		if !ok {
			bad("raw/%s is named by no statement", e.Name())
			continue
		}
		delete(named, e.Name())
		info, err := e.Info()
		if err != nil || !e.Type().IsRegular() || info.Size() != size {
			bad("raw/%s is not a plain file of the size its statement gives", e.Name())
			continue
		}
		file, err := os.ReadFile(filepath.Join(dir, "raw", e.Name()))
		if sum := sha256.Sum256(file); err != nil || hex.EncodeToString(sum[:]) != e.Name() {
			bad("raw/%s does not have the SHA-256 of its name", e.Name())
		}
	}
	for name := range named {
		bad("raw/%s is named by a statement, and is not there", name)
	}
	lines = append(lines, fmt.Sprintf("raw files: %d", len(entries)))
	return done()
}
