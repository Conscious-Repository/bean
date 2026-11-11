// Command sim is a software device: it writes the folder that a real device
// hands over (docs/DEVICE.md), with a key it makes, statements it signs and
// raw data it invents. The firmware and bean are built against the same
// folder before a device exists. Nothing it writes is a measurement.
//
//	go run ./tools/device/sim -out <new folder> [-n 3] [-from 1] [-cert]
//	go run ./tools/device/sim -check <folder>
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/capture"
)

// signer is a P-256 key held in memory. A real device keeps its key in a
// secure element and never shows it.
type signer struct{ priv *ecdsa.PrivateKey }

func newSigner() signer {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	return signer{priv}
}

func (s signer) public() string {
	return "p256:" + hex.EncodeToString(elliptic.MarshalCompressed(elliptic.P256(), s.priv.X, s.priv.Y))
}

// sign signs an id as SPEC.md 4.5 says for P-256: ECDSA over the SHA-256 of
// the 32 bytes of the id, r and s as 32 bytes each, s in the lower half.
func (s signer) sign(id string) string {
	raw, _ := hex.DecodeString(id[len("sha256:"):])
	h := sha256.Sum256(raw)
	r, sv, err := ecdsa.Sign(rand.Reader, s.priv, h[:])
	if err != nil {
		panic(err)
	}
	n := elliptic.P256().Params().N
	if sv.Cmp(new(big.Int).Rsh(n, 1)) > 0 {
		sv.Sub(n, sv)
	}
	out := make([]byte, 64)
	r.FillBytes(out[:32])
	sv.FillBytes(out[32:])
	return "p256:" + hex.EncodeToString(out)
}

func signed(obj map[string]any, s signer) map[string]any {
	obj["sig"] = s.sign(capture.ID(obj))
	return obj
}

func main() {
	out := flag.String("out", "", "the folder to write, which must not exist")
	n := flag.Int("n", 3, "how many captures the device has made")
	from := flag.Int("from", 1, "the first capture to hand over: earlier ones were handed over before")
	cert := flag.Bool("cert", false, "add a factory certificate, signed by a key made for this run")
	check := flag.String("check", "", "write nothing: check this folder as bean will, and exit 0 or 1")
	flag.Parse()
	if *check != "" {
		report, ok := capture.CheckFolder(*check)
		for _, line := range report {
			fmt.Println(line)
		}
		if !ok {
			os.Exit(1)
		}
		return
	}
	if *out == "" || *n < 1 || *from < 1 || *from > *n {
		flag.Usage()
		os.Exit(2)
	}
	if err := os.MkdirAll(filepath.Join(*out, "raw"), 0o700); err != nil {
		fail(err)
	}
	dev := newSigner()
	var lines []byte
	prev := ""
	for i := 1; i <= *n; i++ {
		// Raw data that is no measurement: bytes that depend on the count.
		raw := make([]byte, 4096)
		for j := range raw {
			raw[j] = byte(i*31 + j*7)
		}
		sum := sha256.Sum256(raw)
		digest := hex.EncodeToString(sum[:])
		st := signed(map[string]any{
			"bean":        capture.StatementTag,
			"calibration": "phantom-7 2026-11-01",
			"device":      map[string]any{"fw": "0.1.0", "hw": "A", "key": dev.public(), "model": "sim-1"},
			"kind":        "sim.noise",
			"prev":        prev,
			"raw":         []any{map[string]any{"bytes": float64(len(raw)), "name": "samples.bin", "sha256": digest}},
			"selftest":    "pass",
			"seq":         map[string]any{"boot": float64(1 + i/3), "capture": float64(i)},
			"settings":    map[string]any{"depth_mm": float64(40), "gain_db": float64(12), "probe": "linear-5MHz"},
			"time":        fmt.Sprintf("2026-11-11T17:%02d:00Z", i%60),
		}, dev)
		prev = capture.ID(st)
		if i < *from {
			continue
		}
		lines = append(append(lines, core.Canonical(st)...), '\n')
		if err := os.WriteFile(filepath.Join(*out, "raw", digest), raw, 0o600); err != nil {
			fail(err)
		}
	}
	device := map[string]any{"bean": capture.DeviceTag, "key": dev.public(), "time": "2026-11-11T18:00:00Z"}
	if *cert {
		factory := newSigner()
		device["cert"] = signed(map[string]any{"bean": capture.CertTag, "hw": "A", "issuer": factory.public(), "key": dev.public(), "model": "sim-1"}, factory)
	}
	if err := os.WriteFile(filepath.Join(*out, "captures.jsonl"), lines, 0o600); err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join(*out, "device.json"), append(core.Canonical(device), '\n'), 0o600); err != nil {
		fail(err)
	}
	// The folder is checked as a reader will check it.
	report, ok := capture.CheckFolder(*out)
	for _, line := range report {
		fmt.Println(line)
	}
	if !ok {
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "sim:", err)
	os.Exit(1)
}
