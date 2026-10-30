// Command adversary writes hostile files into a bean log for
// tools/smoke.sh. It is a test tool: it never runs on a real log.
//
//	adversary stranger-genesis <log>          a genesis with a copied recovery commitment
//	adversary old-key-claim <log> <old copy>  a claim signed by the key in <old copy>/local
//	adversary halt <log> <stolen copy>        a bean.tombstone of an unknown v, signed by the key in <stolen copy>/local
package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Conscious-Repository/bean/core"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: adversary stranger-genesis <log> | old-key-claim <log> <old copy> | halt <log> <stolen copy>")
		os.Exit(2)
	}
	dir := os.Args[2]
	s := core.Open(dir)
	switch os.Args[1] {
	case "stranger-genesis":
		l, err := s.Load()
		must(err)
		g := l.Report.Records[l.Local.Identity]
		seed := make([]byte, 32)
		rand.Read(seed)
		atk := core.NewEd25519Signer(seed)
		id, b, err := core.SignRecord(core.NewGenesis(atk.Public(), g.Recovery(), "adversary@1", "2026-10-30T12:00:00Z"), atk)
		must(err)
		put(dir, id, b)
	case "old-key-claim":
		if len(os.Args) != 4 {
			os.Exit(2)
		}
		old, err := core.Open(os.Args[3]).ReadLocal()
		must(err)
		obj := map[string]any{"v": 0.0, "nonce": core.NewNonce(), "type": "weight", "subject": old.Identity,
			"author": old.Identity, "kid": old.Signer.Public(), "value": map[string]any{"n": 1.0, "unit": "kg"},
			"times":  map[string]any{"observed": nil, "imported": "2026-10-30T12:00:00Z"},
			"source": map[string]any{"system": "self", "adapter": "adversary@1", "ref": nil}}
		id, b, err := core.SignRecord(obj, old.Signer)
		must(err)
		put(dir, id, b)
	case "halt":
		if len(os.Args) != 4 {
			os.Exit(2)
		}
		old, err := core.Open(os.Args[3]).ReadLocal()
		must(err)
		obj := core.NewTombstone(old.Identity, old.Signer.Public(), old.Identity, "", "adversary@1", "2026-10-30T12:00:00Z")
		obj["v"] = 99.0
		id, b, err := core.SignRecord(obj, old.Signer)
		must(err)
		put(dir, id, b)
	default:
		os.Exit(2)
	}
}

func put(dir, id string, b []byte) {
	p := filepath.Join(dir, filepath.FromSlash(core.PathOf(id)))
	must(os.MkdirAll(filepath.Dir(p), 0o700))
	must(os.WriteFile(p, b, 0o600))
	fmt.Println(id)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "adversary:", err)
		os.Exit(1)
	}
}
