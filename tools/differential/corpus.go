// Command corpus writes random bean logs for tools/differential/run.sh.
// Each log mixes identities, rotations, forks, seal parts, claims by
// head, retired, recovery and stranger keys, supersedes, tombstones,
// duplicates, unknown versions, blobs, lost files and broken files. It
// is a test tool: it never runs on a real log.
//
//	corpus <out dir> <count> <seed>   random logs
//	corpus -fuzz <dir> <out dir>       one log per FuzzVerifyFiles input in dir
package main

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Conscious-Repository/bean/core"
)

const imported = "2026-10-30T12:00:00Z"

type gen struct {
	r     *rand.Rand
	dir   string
	n     int
	files map[string][]byte // path -> bytes
	ids   []string          // every record id written, in order
	byKid map[string][]string
	subj  map[string]string // id -> subject
	// secret is each signer's key line; local is the first identity's
	// local/ folder and words, when the log has one.
	secret map[string]string
	local  map[string]string
	words  []string
}

func main() {
	if len(os.Args) == 4 && os.Args[1] == "-fuzz" {
		fuzzLogs(os.Args[2], os.Args[3])
		return
	}
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: corpus <out dir> <count> <seed>")
		os.Exit(2)
	}
	count, _ := strconv.Atoi(os.Args[2])
	seed, _ := strconv.ParseInt(os.Args[3], 10, 64)
	for i := 0; i < count; i++ {
		g := &gen{r: rand.New(rand.NewSource(seed + int64(i))), dir: filepath.Join(os.Args[1], fmt.Sprintf("rand-%04d", i)),
			files: map[string][]byte{}, byKid: map[string][]string{}, subj: map[string]string{}, secret: map[string]string{}}
		g.log()
		g.write()
	}
}

func (g *gen) chance(p float64) bool { return g.r.Float64() < p }

func (g *gen) signer() core.Signer {
	g.n++
	h := sha256.Sum256([]byte(fmt.Sprintf("%s/%d/%d", g.dir, g.n, g.r.Int63())))
	s := core.NewEd25519Signer(h[:])
	g.secret[s.Public()] = fmt.Sprintf("ed25519:%x\n", h)
	return s
}

func (g *gen) put(obj map[string]any, s core.Signer) string {
	id, b, err := core.SignRecord(obj, s)
	if err != nil {
		panic(err)
	}
	g.files[core.PathOf(id)] = b
	g.ids = append(g.ids, id)
	g.byKid[s.Public()] = append(g.byKid[s.Public()], id)
	if sub, ok := obj["subject"].(string); ok {
		g.subj[id] = sub
	}
	return id
}

func (g *gen) pick() string {
	if len(g.ids) == 0 {
		return "sha256:" + fmt.Sprintf("%064x", g.r.Int63())
	}
	return g.ids[g.r.Intn(len(g.ids))]
}

func (g *gen) claim(subject, author string, s core.Signer) string {
	obj := map[string]any{"v": 0.0, "nonce": core.NewNonce(), "type": []string{"weight", "hr", "note"}[g.r.Intn(3)],
		"subject": subject, "author": author, "kid": s.Public(),
		"value":  map[string]any{"n": float64(g.r.Intn(200)), "unit": "kg"},
		"times":  map[string]any{"observed": nil, "imported": imported},
		"source": map[string]any{"system": "self", "adapter": "corpus@1", "ref": nil}}
	if g.chance(0.3) {
		obj["source"] = map[string]any{"system": "dev", "adapter": "corpus@1", "ref": fmt.Sprintf("hmac-sha256:%064x", g.r.Intn(3))}
	}
	if g.chance(0.25) {
		obj["refs"] = []any{map[string]any{"rel": "supersedes", "id": g.pick()}}
	}
	if g.chance(0.15) {
		b := []byte(fmt.Sprintf("blob %d", g.r.Intn(4)))
		h := core.HashID(b)
		obj["blobs"] = []any{map[string]any{"hash": h, "role": "raw-export", "media_type": "text/plain"}}
		if g.chance(0.7) {
			g.files[core.BlobPathOf(h)] = b
		}
	}
	return g.put(obj, s)
}

func (g *gen) unknown(author string, s core.Signer) {
	obj := core.NewTombstone(author, s.Public(), g.pick(), "", "corpus@1", imported)
	if g.chance(0.3) {
		obj["type"] = "weight"
	}
	obj["v"] = 99.0
	g.put(obj, s)
}

// identity writes one identity with a chain of rotations, maybe a fork.
func (g *gen) identity(strangers []core.Signer) {
	seed := make([]byte, 32)
	g.r.Read(seed)
	key := g.signer()
	gid := g.put(core.NewGenesis(key.Public(), core.Commitment(core.RecoverySigner(seed, 0).Public()), "corpus@1", imported), key)
	keys := []core.Signer{key}
	parent := gid
	gens := g.r.Intn(4)
	for gn := 0; gn <= gens; gn++ {
		for k := g.r.Intn(4); k > 0; k-- {
			g.claim(gid, gid, keys[len(keys)-1])
		}
		if g.chance(0.4) {
			g.put(core.NewTombstone(gid, keys[len(keys)-1].Public(), g.pick(), "", "corpus@1", imported), keys[len(keys)-1])
		}
		if g.chance(0.12) {
			g.unknown(gid, keys[len(keys)-1])
		}
		if gn == gens {
			break
		}
		rec := core.RecoverySigner(seed, gn)
		var sealed []string
		for _, id := range g.byKid[keys[len(keys)-1].Public()] {
			if g.chance(0.8) {
				sealed = append(sealed, id)
			}
		}
		sortStrings(sealed)
		next := g.signer()
		var parts [][]string
		if len(sealed) > 1 && g.chance(0.25) {
			half := len(sealed) / 2
			parts = [][]string{sealed[half:]}
			sealed = sealed[:half]
		}
		succ := core.NewSuccession(gid, rec.Public(), parent, next.Public(), core.Commitment(core.RecoverySigner(seed, gn+1).Public()), sealed, "corpus@1", imported)
		if len(parts) > 0 {
			succ["value"].(map[string]any)["parts"] = float64(len(parts))
		}
		sid := g.put(succ, rec)
		for k, p := range parts {
			if g.chance(0.85) {
				g.put(core.NewSeal(gid, rec.Public(), sid, k+1, p, "corpus@1", imported), rec)
			}
		}
		if g.chance(0.1) {
			fork := g.signer()
			g.put(core.NewSuccession(gid, rec.Public(), parent, fork.Public(), core.Commitment(core.RecoverySigner(seed, gn+1).Public()), nil, "corpus@1", imported), rec)
		}
		if g.chance(0.1) {
			g.unknown(gid, core.RecoverySigner(seed, gn+g.r.Intn(2)))
		}
		if g.chance(0.15) {
			g.claim(gid, gid, keys[len(keys)-1]) // a retired key writes after its rotation
		}
		keys = append(keys, next)
		parent = sid
	}
	// The first identity of most logs is the local one, so the
	// differential can rotate it with its words.
	if g.local == nil && g.chance(0.7) {
		g.words, _ = core.SeedToWords(seed)
		g.local = map[string]string{"identity": gid + "\n", "identity-secret": fmt.Sprintf("%x\n", core.IdentitySecret(seed)),
			"signing-key": g.secret[keys[len(keys)-1].Public()]}
	}
	for _, st := range strangers {
		if g.chance(0.3) {
			g.claim(gid, st.Public(), st)
		}
		if g.chance(0.1) {
			g.unknown(gid, st)
		}
	}
}

func (g *gen) log() {
	strangers := []core.Signer{g.signer()}
	for k := 1 + g.r.Intn(3); k > 0; k-- {
		g.identity(strangers)
	}
	if g.chance(0.1) {
		s := g.signer()
		obj := core.NewGenesis(s.Public(), core.Commitment(s.Public()), "corpus@1", imported)
		obj["v"] = 99.0
		g.put(obj, s)
	}
	// Lose files, break files.
	for p := range g.files {
		switch {
		case g.chance(0.04):
			delete(g.files, p)
		case g.chance(0.02):
			b := append([]byte(nil), g.files[p]...)
			b[g.r.Intn(len(b))] ^= 1
			g.files[p] = b
		}
	}
}

func (g *gen) write() {
	for p, b := range g.files {
		full := filepath.Join(g.dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(full, b, 0o644); err != nil {
			panic(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(g.dir, "objects"), 0o755); err != nil {
		panic(err)
	}
	if g.local == nil {
		return
	}
	if err := os.MkdirAll(filepath.Join(g.dir, "local"), 0o700); err != nil {
		panic(err)
	}
	for name, text := range g.local {
		if err := os.WriteFile(filepath.Join(g.dir, "local", name), []byte(text), 0o600); err != nil {
			panic(err)
		}
	}
	if err := os.WriteFile(g.dir+".words", []byte(strings.Join(g.words, " ")+"\n"), 0o600); err != nil {
		panic(err)
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// fuzzLogs turns each go-fuzz input of FuzzVerifyFiles (path and bytes on
// alternate lines) into a folder. Only local paths under objects/ or
// blobs/ are written.
func fuzzLogs(in, out string) {
	entries, _ := os.ReadDir(in)
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(in, e.Name()))
		if err != nil {
			continue
		}
		lines := strings.SplitN(string(b), "\n", 2)
		if len(lines) != 2 || lines[0] != "go test fuzz v1" {
			continue
		}
		lit := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(lines[1]), "[]byte("), ")")
		blob, err := strconv.Unquote(lit)
		if err != nil {
			continue
		}
		dir := filepath.Join(out, "fuzz-"+e.Name()[:min(16, len(e.Name()))])
		parts := strings.Split(blob, "\n")
		for k := 0; k+1 < len(parts); k += 2 {
			p := parts[k]
			if !filepath.IsLocal(p) || !(strings.HasPrefix(p, "objects/") || strings.HasPrefix(p, "blobs/")) || strings.Count(p, "/") != 2 {
				continue
			}
			full := filepath.Join(dir, filepath.FromSlash(p))
			if os.MkdirAll(filepath.Dir(full), 0o755) == nil {
				os.WriteFile(full, []byte(parts[k+1]), 0o644)
			}
		}
		os.MkdirAll(filepath.Join(dir, "objects"), 0o755)
	}
}
