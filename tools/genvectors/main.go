// Command genvectors computes the vectors (SPEC.md section 14) with the
// core package and writes them to vectors/. A second implementation
// (tools/checkvectors.py) must agree before a vector is final.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Conscious-Repository/bean/core"
)

var seed = hexb("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
var signSeed = hexb("1111111111111111111111111111111111111111111111111111111111111111")
var p256D = hexb("2222222222222222222222222222222222222222222222222222222222222222")

func hexb(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func write(name string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join("vectors", name), append(b, '\n'), 0o644); err != nil {
		panic(err)
	}
	fmt.Println("wrote", name)
}

type jcsCase struct {
	Input     string `json:"input"`
	Canonical string `json:"canonical,omitempty"`
	Error     string `json:"error,omitempty"`
}

func jcs(inputs []string) []jcsCase {
	var out []jcsCase
	for _, in := range inputs {
		c := jcsCase{Input: in}
		v, err := core.Parse([]byte(in))
		if err != nil {
			c.Error = err.Error()
		} else {
			c.Canonical = string(core.Canonical(v))
		}
		out = append(out, c)
	}
	return out
}

func main() {
	v19()
	// V1: numbers. RFC 8785 Appendix B samples by literal, plus bean range cases.
	write("v1-jcs-numbers.json", map[string]any{
		"spec":  "SPEC.md 4.1",
		"cases": jcs([]string{"0", "-0", "0.0", "1", "1.0", "1e0", "5e-324", "1.7976931348623157e308", "1e400", "9007199254740991", "-9007199254740991", "9007199254740992", "-9007199254740992", "9.007199254740992e15", "1e21", "1e+21", "999999999999999900000", "1e-7", "0.000001", "1E-7", "333333333.3333332", "333333333.33333325", "2.5", "100", "1e2", "-1.5e-10", "123456789012345680000"}),
	})

	// V2: key order by UTF-16 code units (RFC 8785 Appendix B).
	keyOrder := `{"€":"Euro Sign","\r":"Carriage Return","דּ":"Hebrew Letter Dalet With Dagesh","1":"One","😀":"Emoji: Grinning Face","\u0080":"Control","ö":"Latin Small Letter O With Diaeresis","￿":"Last BMP","a":"ascii","A":"upper","":"empty","aa":"double"}`
	write("v2-jcs-key-order.json", map[string]any{"spec": "SPEC.md 4.1 rule 1", "cases": jcs([]string{keyOrder})})

	// V3: strings.
	write("v3-jcs-strings.json", map[string]any{
		"spec":  "SPEC.md 4.1 rules 3 to 6",
		"cases": jcs([]string{`"\u0000\u0001\u001f"`, `"\b\t\n\f\r"`, `"\u0008\u0009\u000a\u000c\u000d"`, `"\/"`, `"\u007f"`, `"é"`, `"é"`, `"😀"`, `"😀"`, `"a\"b\\c"`, `"\ud800"`, `"\udc00"`, `"\ud800A"`, `"😀"`, "\"  \""}),
	})

	// Keys for V4 to V7.
	signer := core.NewEd25519Signer(signSeed)
	kid := signer.Public()
	rec0 := core.RecoverySigner(seed, 0).Public()
	commit0 := core.Commitment(rec0)
	identitySecret := core.IdentitySecret(seed)
	dedupeKey := core.DedupeKey(identitySecret)
	blobKey := core.BlobKey(identitySecret)

	// V4: three records. Nonces are fixed so the vector is reproducible.
	genesis := map[string]any{
		"v": 0.0, "nonce": "00000000000000000000000000000001", "type": "bean.genesis",
		"author": kid, "kid": kid,
		"value":  map[string]any{"key": kid, "recovery": commit0},
		"times":  map[string]any{"observed": nil, "imported": "2026-10-27T12:00:00Z"},
		"source": map[string]any{"system": "self", "adapter": "bean@0.3.0", "ref": nil},
	}
	genesisID := core.RecordID(genesis)
	sig, _ := signer.Sign(genesisID)
	genesis["id"], genesis["sig"] = genesisID, sig

	claimRef := core.DedupeRefNative(dedupeKey, "csv", "row-17", 0)
	claim := map[string]any{
		"v": 0.0, "nonce": "00000000000000000000000000000002", "type": "hrv.rmssd",
		"subject": genesisID, "author": genesisID, "kid": kid,
		"value":   map[string]any{"n": 42.5, "unit": "ms"},
		"times":   map[string]any{"observed": map[string]any{"start": "2026-10-26T03:00:00Z", "precision": "minute"}, "imported": "2026-10-27T12:00:01Z"},
		"source":  map[string]any{"system": "csv", "adapter": "bean-import-csv@0.3.0", "ref": claimRef},
		"context": map[string]any{"coding": map[string]any{"loinc": "80404-7"}},
	}
	claimID := core.RecordID(claim)
	sig, _ = signer.Sign(claimID)
	claim["id"], claim["sig"] = claimID, sig

	tomb := map[string]any{
		"v": 0.0, "nonce": "00000000000000000000000000000003", "type": "bean.tombstone",
		"subject": genesisID, "author": genesisID, "kid": kid,
		"refs":    []any{map[string]any{"rel": "purges", "id": claimID}},
		"times":   map[string]any{"observed": nil, "imported": "2026-10-27T12:00:02Z"},
		"source":  map[string]any{"system": "self", "adapter": "bean@0.3.0", "ref": nil},
		"context": map[string]any{"reason": "test"},
	}
	tombID := core.RecordID(tomb)
	sig, _ = signer.Sign(tombID)
	tomb["id"], tomb["sig"] = tombID, sig

	recVec := func(name string, r map[string]any) map[string]any {
		return map[string]any{"name": name, "record": r, "id_bytes": string(core.IDBytes(r)), "id": r["id"], "file_bytes": string(core.Canonical(r))}
	}
	claimVec := recVec("claim", claim)
	claimVec["source_ref_input"] = core.RefInputNative("csv", "row-17", 0)
	claimVec["source_ref_key"] = "dedupe_key in v10-derived-keys.json"
	write("v4-record-id.json", map[string]any{"spec": "SPEC.md 4.2", "records": []any{recVec("genesis", genesis), claimVec, recVec("tombstone", tomb)}})

	// V5: Ed25519.
	write("v5-ed25519.json", map[string]any{"spec": "SPEC.md 4.5", "private_seed": hex.EncodeToString(signSeed), "kid": kid, "id": claimID, "sig": claim["sig"]})

	// V6: P-256 low-S accept, high-S reject.
	ps, err := core.NewP256Signer(p256D)
	if err != nil {
		panic(err)
	}
	low, _ := ps.Sign(claimID)
	raw := hexb(low[5:])
	// n - s
	n := hexb("ffffffff00000000ffffffffffffffffbce6faada7179e84f3b9cac2fc632551")
	var borrow int
	hs := make([]byte, 32)
	for i := 31; i >= 0; i-- {
		d := int(n[i]) - int(raw[32+i]) - borrow
		if d < 0 {
			d += 256
			borrow = 1
		} else {
			borrow = 0
		}
		hs[i] = byte(d)
	}
	high := "p256:" + hex.EncodeToString(append(append([]byte{}, raw[:32]...), hs...))
	if core.Verify(ps.Public(), claimID, low) != nil || core.Verify(ps.Public(), claimID, high) == nil {
		panic("p256 vector self-check failed")
	}
	write("v6-p256.json", map[string]any{"spec": "SPEC.md 4.5", "private_scalar": hex.EncodeToString(p256D), "kid": ps.Public(), "id": claimID, "ecdsa_hash_input": "sha256 of the 32 id bytes", "low_s_sig": low, "low_s_result": "accept", "high_s_sig": high, "high_s_result": "bad signature"})

	// V7: commitment.
	write("v7-commitment.json", map[string]any{"spec": "SPEC.md 4.6", "tagged_key": rec0, "commitment": commit0})

	// V8: paper seed words.
	words, _ := core.SeedToWords(seed)
	write("v8-paper-seed.json", map[string]any{"spec": "SPEC.md 4.10", "entropy": hex.EncodeToString(seed), "words": strings.Join(words, " ")})

	// V9: HKDF from seed.
	write("v9-hkdf.json", map[string]any{"spec": "SPEC.md 4.7", "paper_seed": hex.EncodeToString(seed), "identity_secret": hex.EncodeToString(identitySecret),
		"recovery_private_seeds": []string{hex.EncodeToString(core.RecoverySeed(seed, 0)), hex.EncodeToString(core.RecoverySeed(seed, 1)), hex.EncodeToString(core.RecoverySeed(seed, 2))},
		"recovery_keys":          []string{rec0, core.RecoverySigner(seed, 1).Public(), core.RecoverySigner(seed, 2).Public()}})

	// V10: derived keys.
	write("v10-derived-keys.json", map[string]any{"spec": "SPEC.md 4.7", "identity_secret": hex.EncodeToString(identitySecret), "blob_key": hex.EncodeToString(blobKey), "dedupe_key": hex.EncodeToString(dedupeKey)})

	// V11: AES-SIV blobs.
	pattern := bytes.Repeat([]byte{0xde, 0xad, 0xbe, 0xef}, 1<<18) // 1 MiB
	type blobVec struct {
		Name          string `json:"name"`
		Plaintext     string `json:"plaintext_hex,omitempty"`
		PlaintextDesc string `json:"plaintext_desc,omitempty"`
		Output        string `json:"output_hex,omitempty"`
		OutputSHA256  string `json:"output_sha256"`
		BlobID        string `json:"blob_id"`
	}
	var blobs []blobVec
	for _, p := range []struct {
		name string
		pt   []byte
		desc string
	}{{"empty", nil, ""}, {"one-byte", []byte{0x41}, ""}, {"1mib", pattern, "bytes de ad be ef repeated 262144 times"}} {
		out, err := core.EncryptBlob(blobKey, p.pt)
		if err != nil {
			panic(err)
		}
		back, err := core.DecryptBlob(blobKey, out)
		if err != nil || !bytes.Equal(back, p.pt) {
			panic("siv round trip")
		}
		sum := sha256.Sum256(out)
		bv := blobVec{Name: p.name, OutputSHA256: hex.EncodeToString(sum[:]), BlobID: core.HashID(out), PlaintextDesc: p.desc}
		if len(p.pt) <= 16 {
			bv.Plaintext = hex.EncodeToString(p.pt)
			bv.Output = hex.EncodeToString(out)
		}
		blobs = append(blobs, bv)
	}
	write("v11-aes-siv.json", map[string]any{"spec": "SPEC.md 4.9", "blob_key": hex.EncodeToString(blobKey), "associated_data": core.BlobAD, "blobs": blobs})

	for _, f := range extra {
		f(signer, claim)
	}

	// V12: dedupe refs.
	obs := map[string]any{"start": "2026-10-26T03:00:00Z", "precision": "minute"}
	val := map[string]any{"n": 42.5, "unit": "ms"}
	write("v12-dedupe-ref.json", map[string]any{"spec": "SPEC.md 4.8", "dedupe_key": hex.EncodeToString(dedupeKey), "cases": []any{
		map[string]any{"input": core.RefInputNative("oura", "12345", 0), "input_canonical": string(core.Canonical(core.RefInputNative("oura", "12345", 0))), "ref": core.DedupeRefNative(dedupeKey, "oura", "12345", 0)},
		map[string]any{"input": core.RefInputNative("oura", "12345", 1), "input_canonical": string(core.Canonical(core.RefInputNative("oura", "12345", 1))), "ref": core.DedupeRefNative(dedupeKey, "oura", "12345", 1)},
		map[string]any{"input": core.RefInputTuple("csv", "hrv.rmssd", obs, val, 0), "input_canonical": string(core.Canonical(core.RefInputTuple("csv", "hrv.rmssd", obs, val, 0))), "ref": core.DedupeRefTuple(dedupeKey, "csv", "hrv.rmssd", obs, val, 0)},
		map[string]any{"input": core.RefInputTuple("csv", "hrv.rmssd", obs, val, 1), "input_canonical": string(core.Canonical(core.RefInputTuple("csv", "hrv.rmssd", obs, val, 1))), "ref": core.DedupeRefTuple(dedupeKey, "csv", "hrv.rmssd", obs, val, 1)},
		map[string]any{"input": core.RefInputTuple("csv", "hrv.rmssd", nil, val, 0), "input_canonical": string(core.Canonical(core.RefInputTuple("csv", "hrv.rmssd", nil, val, 0))), "ref": core.DedupeRefTuple(dedupeKey, "csv", "hrv.rmssd", nil, val, 0)},
	}})
}

func init() { extra = append(extra, genV17) }

// genV17 writes one bad file per error string of SPEC.md 3.8 steps 1 to 14.
// Field-level errors (step 15) are covered by core/verify_test.go.
func genV17(signer core.Signer, claim map[string]any) {
	good := core.Canonical(claim)
	path := core.PathOf(claim["id"].(string))
	type bad struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		Bytes string `json:"bytes"`
		Error string `json:"error"`
	}
	mut := func(f func(m map[string]any), resign bool) (string, string) {
		v, _ := core.Parse(good)
		m := v.(map[string]any)
		f(m)
		if resign {
			id, b, _ := core.SignRecord(m, signer)
			return core.PathOf(id), string(b)
		}
		return core.PathOf(m["id"].(string)), string(core.Canonical(m))
	}
	var cases []bad
	add := func(name, p, b string) {
		fr := core.CheckFile(p, []byte(b))
		if fr.Err == nil {
			panic("V17 case passed: " + name)
		}
		if fr.Err.Error() != name {
			panic(fmt.Sprintf("V17 %s produced %s", name, fr.Err))
		}
		cases = append(cases, bad{name, p, b, fr.Err.Error()})
	}
	add("bad path", "objects/zz/"+strings.Repeat("0", 62)+".json", string(good))
	add("bad json", path, "{")
	add("duplicate key", path, `{"v":0,"v":0}`)
	add("bad number", path, `{"v":9007199254740992}`)
	add("bad string", path, `{"v":"\udc00"}`)
	add("bad v", path, `{"v":"0"}`)
	add("bad type", path, `{"v":0}`)
	add("id mismatch", path, strings.Replace(string(good), `"n":42.5`, `"n":42.6`, 1))
	p, b := mut(func(m map[string]any) { m["kid"] = "x25519:00"; m["author"] = m["kid"] }, true)
	add("unsupported key algorithm", p, b)
	p, b = mut(func(m map[string]any) { m["kid"] = "ed25519:00"; m["author"] = m["kid"] }, true)
	add("bad key", p, b)
	p, b = mut(func(m map[string]any) { m["sig"] = "ed25519:" + strings.Repeat("00", 64) }, false)
	add("bad signature", p, b)
	add("not canonical", path, strings.Replace(string(good), `"n":42.5`, `"n": 42.5`, 1))
	write("v17-bad-files.json", map[string]any{"spec": "SPEC.md 3.8 steps 1 to 14 (change set C2 order)", "note": "too large is omitted: a 1 MiB file does not belong in a vector", "cases": cases})
}

var extra []func(core.Signer, map[string]any)

func init() { extra = append(extra, genV18) }

// genV18 writes the secp256k1 (Nostr key) vector.
func genV18(_ core.Signer, claim map[string]any) {
	claimID := claim["id"].(string)
	// V18: secp256k1 (Nostr) keys. The signature is BIP-340 over the Nostr
	// event id of a wrapper whose content is the record id (change note C21).
	secp := make([]byte, 32)
	secp[31] = 3 // the BIP-340 test vector 0 key
	var zero [32]byte
	ss, err := core.NewSecp256k1Signer(secp)
	if err != nil {
		panic(err)
	}
	skid := ss.Public()
	pubHex := strings.TrimPrefix(skid, "secp256k1:")
	var sigCases []any
	var firstSig string
	for _, created := range []int64{0, 1761566400} {
		sg, err := core.SignSecp256k1With(secp, claimID, created, zero)
		if err != nil {
			panic(err)
		}
		if err := core.Verify(skid, claimID, sg); err != nil {
			panic("V18 does not verify: " + err.Error())
		}
		if firstSig == "" {
			firstSig = sg
		}
		wid := core.WrapperID(pubHex, created, claimID)
		sigCases = append(sigCases, map[string]any{"created_at": float64(created), "wrapper_bytes": string(core.WrapperBytes(pubHex, created, claimID)), "wrapper_id": hex.EncodeToString(wid[:]), "aux_rand": hex.EncodeToString(zero[:]), "sig": sg})
	}
	sigHex := firstSig[len("secp256k1:0:"):]
	var rejects []any
	for _, r := range []struct{ name, id, sig string }{
		{"other record id", core.HashID([]byte("another record")), firstSig},
		{"created_at changed", claimID, "secp256k1:1:" + sigHex},
		{"created_at with a leading zero", claimID, "secp256k1:00:" + sigHex},
		{"no created_at", claimID, "secp256k1:" + sigHex},
		{"uppercase hex", claimID, "secp256k1:0:" + strings.ToUpper(sigHex)},
	} {
		if core.Verify(skid, r.id, r.sig) == nil {
			panic("V18 reject accepted: " + r.name)
		}
		rejects = append(rejects, map[string]any{"name": r.name, "id": r.id, "sig": r.sig, "result": "bad signature"})
	}
	write("v18-secp256k1.json", map[string]any{
		"spec": "SPEC.md 4.4 and 4.5, change note C21", "secret_key": hex.EncodeToString(secp),
		"nsec": core.Bech32Encode("nsec", secp), "npub": core.Npub(skid), "kid": skid,
		"nostr_kind": float64(core.NostrKind), "alt": core.WrapperAlt, "id": claimID, "cases": sigCases, "rejects": rejects,
	})
}

// v19 builds the authority scenarios (SPEC.md 14, V19). Each scenario is a set
// of files and what a verifier must report for them. The expected results are
// written by hand here, not read from the verifier, and the generator stops if
// the verifier disagrees.
type v19Expect struct {
	TooNew      []string          `json:"too_new"`
	Compromised []string          `json:"compromised"`
	Valid       []string          `json:"valid"`
	Errors      map[string]string `json:"errors"`
	Unattested  []string          `json:"unattested"`
	Missing     []string          `json:"missing"`
	Duplicates  map[string]string `json:"duplicates"`
}

type v19Scenario struct {
	Name   string            `json:"name"`
	Why    string            `json:"why"`
	Files  map[string]string `json:"files"`
	Expect v19Expect         `json:"expect"`
}

type v19Log struct {
	files map[string]string
	n     int
}

type v19Who struct {
	seed []byte
	key  core.Signer
	id   string
}

func (l *v19Log) put(o map[string]any, k core.Signer) string {
	if _, set := o["nonce"]; !set {
		l.n++
		o["nonce"] = fmt.Sprintf("%032x", l.n)
	}
	id, b, err := core.SignRecord(o, k)
	if err != nil {
		panic(err)
	}
	l.files[core.PathOf(id)] = string(b)
	return id
}

func (l *v19Log) who(b byte) *v19Who {
	seed := bytes.Repeat([]byte{b}, 32)
	key := core.NewEd25519Signer(bytes.Repeat([]byte{b + 0x40}, 32))
	g := core.NewGenesis(key.Public(), core.Commitment(core.RecoverySigner(seed, 0).Public()), "v19@0", "2026-11-06T00:00:00Z")
	delete(g, "nonce")
	return &v19Who{seed, key, l.put(g, key)}
}

func (l *v19Log) claim(w *v19Who, k core.Signer, ref string) string {
	var r any
	if ref != "" {
		r = ref
	}
	return l.put(map[string]any{"v": 0.0, "type": "lab.x", "subject": w.id, "author": w.id, "kid": k.Public(),
		"value": map[string]any{"n": float64(l.n)}, "times": map[string]any{"observed": nil, "imported": "2026-11-06T00:00:00Z"},
		"source": map[string]any{"system": "v19", "adapter": "v19@0", "ref": r}}, k)
}

func (l *v19Log) bare(k core.Signer, subject, ref string) string {
	var r any
	if ref != "" {
		r = ref
	}
	return l.put(map[string]any{"v": 0.0, "type": "lab.x", "subject": subject, "author": k.Public(), "kid": k.Public(),
		"value": map[string]any{"n": float64(l.n)}, "times": map[string]any{"observed": nil, "imported": "2026-11-06T00:00:00Z"},
		"source": map[string]any{"system": "v19", "adapter": "v19@0", "ref": r}}, k)
}

func (l *v19Log) future(w *v19Who, k core.Signer) string {
	return l.put(map[string]any{"v": 1.0, "type": "bean.anything", "subject": w.id, "author": w.id, "kid": k.Public()}, k)
}

func (l *v19Log) checkpoint(w *v19Who, k core.Signer, covers ...string) string {
	sorted := append([]string{}, covers...)
	sort.Strings(sorted)
	c := make([]any, len(sorted))
	for i, x := range sorted {
		c[i] = x
	}
	return l.put(map[string]any{"v": 0.0, "type": "bean.checkpoint", "subject": w.id, "author": w.id, "kid": k.Public(),
		"value": map[string]any{"covers": c}, "times": map[string]any{"observed": nil, "imported": "2026-11-06T00:00:00Z"},
		"source": map[string]any{"system": "self", "adapter": "v19@0", "ref": nil}}, k)
}

func (l *v19Log) tombstone(w *v19Who, k core.Signer, target string) string {
	t := core.NewTombstone(w.id, k.Public(), target, "", "v19@0", "2026-11-06T00:00:00Z")
	delete(t, "nonce")
	return l.put(t, k)
}

func (l *v19Log) supersede(w *v19Who, k core.Signer, target string) string {
	return l.put(map[string]any{"v": 0.0, "type": "lab.x", "subject": w.id, "author": w.id, "kid": k.Public(),
		"value": map[string]any{"n": 1.5}, "refs": []any{map[string]any{"rel": "supersedes", "id": target}},
		"times":  map[string]any{"observed": nil, "imported": "2026-11-06T00:00:00Z"},
		"source": map[string]any{"system": "v19", "adapter": "v19@0", "ref": nil}}, k)
}

// rotate writes a succession at generation gen to a key made from b.
func (l *v19Log) rotate(w *v19Who, parent string, gen int, b byte, sealed ...string) (string, core.Signer) {
	sort.Strings(sealed)
	nk := core.NewEd25519Signer(bytes.Repeat([]byte{b}, 32))
	rec := core.RecoverySigner(w.seed, gen)
	s := core.NewSuccession(w.id, rec.Public(), parent, nk.Public(), core.Commitment(core.RecoverySigner(w.seed, gen+1).Public()), sealed, "v19@0", "2026-11-06T00:00:00Z")
	delete(s, "nonce")
	return l.put(s, rec), nk
}

var v19Ref = "hmac-sha256:" + strings.Repeat("ab", 32)

func v19() {
	var out []v19Scenario
	add := func(name, why string, build func(l *v19Log) v19Expect) {
		l := &v19Log{files: map[string]string{}}
		e := build(l)
		files := map[string][]byte{}
		for p, b := range l.files {
			files[p] = []byte(b)
		}
		if diff := v19Diff(core.VerifyFiles(files), e); diff != "" {
			panic("V19 " + name + ": " + diff)
		}
		out = append(out, v19Scenario{name, why, l.files, e})
	}
	add("too-new-by-signing-key", "A signing key cannot make an identity too new, or a thief could hide it from every copy.", func(l *v19Log) v19Expect {
		w := l.who(1)
		c := l.claim(w, w.key, "")
		l.future(w, w.key)
		return v19Expect{Valid: []string{w.id, c}}
	})
	add("too-new-by-retired-key", "A retired signing key cannot do it either.", func(l *v19Log) v19Expect {
		w := l.who(1)
		s, _ := l.rotate(w, w.id, 0, 0x81)
		l.future(w, w.key)
		return v19Expect{Valid: []string{w.id, s}}
	})
	add("too-new-by-recovery-key", "A recovery key on the identity's tree can, including the next one, which no succession has used yet.", func(l *v19Log) v19Expect {
		w := l.who(1)
		l.claim(w, w.key, "")
		l.future(w, core.RecoverySigner(w.seed, 0))
		return v19Expect{TooNew: []string{w.id}}
	})
	add("duplicate-other-author", "A record with another author's dedupe ref does not make that author's record a duplicate, whatever the ids.", func(l *v19Log) v19Expect {
		w := l.who(1)
		c := l.claim(w, w.key, v19Ref)
		k := core.NewEd25519Signer(bytes.Repeat([]byte{0x77}, 32))
		f := l.bare(k, w.id, v19Ref)
		return v19Expect{Valid: []string{w.id, c, f}}
	})
	add("duplicate-same-author", "Two records of one author with one ref are duplicates. The lowest id is the primary.", func(l *v19Log) v19Expect {
		w := l.who(1)
		a, b := l.claim(w, w.key, v19Ref), l.claim(w, w.key, v19Ref)
		if a > b {
			a, b = b, a
		}
		return v19Expect{Valid: []string{w.id, a, b}, Duplicates: map[string]string{b: a}}
	})
	add("checkpoint-missing", "A record that a checkpoint covers must be in the set.", func(l *v19Log) v19Expect {
		w := l.who(1)
		c := l.claim(w, w.key, "")
		cp := l.checkpoint(w, w.key, w.id, c)
		delete(l.files, core.PathOf(c))
		return v19Expect{Valid: []string{w.id, cp}, Missing: []string{c + " of " + w.id}}
	})
	add("checkpoint-and-tombstone", "A valid tombstone of the identity explains a covered record that is gone.", func(l *v19Log) v19Expect {
		w := l.who(1)
		c := l.claim(w, w.key, "")
		cp := l.checkpoint(w, w.key, c)
		t := l.tombstone(w, w.key, c)
		delete(l.files, core.PathOf(c))
		return v19Expect{Valid: []string{w.id, cp, t}}
	})
	add("checkpoint-stranger-tombstone", "Another identity's tombstone does not explain it.", func(l *v19Log) v19Expect {
		w, x := l.who(1), l.who(2)
		c := l.claim(w, w.key, "")
		cp := l.checkpoint(w, w.key, c)
		t := l.tombstone(x, x.key, c)
		delete(l.files, core.PathOf(c))
		return v19Expect{Valid: []string{w.id, x.id, cp, t}, Missing: []string{c + " of " + w.id}}
	})
	add("retired-claim-unattested", "A claim of a retired key that nothing vouches for stays valid but is unattested.", func(l *v19Log) v19Expect {
		w := l.who(1)
		s, _ := l.rotate(w, w.id, 0, 0x81)
		c := l.claim(w, w.key, "")
		return v19Expect{Valid: []string{w.id, s, c}, Unattested: []string{c}}
	})
	add("retired-claim-sealed", "The succession that ended the key can vouch for the claim.", func(l *v19Log) v19Expect {
		w := l.who(1)
		c := l.claim(w, w.key, "")
		s, _ := l.rotate(w, w.id, 0, 0x81, c)
		return v19Expect{Valid: []string{w.id, s, c}}
	})
	add("retired-claim-covered-by-later-key", "A checkpoint of a later key can vouch for the claim.", func(l *v19Log) v19Expect {
		w := l.who(1)
		s, nk := l.rotate(w, w.id, 0, 0x81)
		c := l.claim(w, w.key, "")
		cp := l.checkpoint(w, nk, c)
		return v19Expect{Valid: []string{w.id, s, c, cp}}
	})
	add("retired-checkpoint-unsealed", "A checkpoint of a retired key needs a seal, like a deletion, so a thief's checkpoint does not vouch for a thief's claim.", func(l *v19Log) v19Expect {
		w := l.who(1)
		s, _ := l.rotate(w, w.id, 0, 0x81)
		c := l.claim(w, w.key, "")
		cp := l.checkpoint(w, w.key, c)
		return v19Expect{Valid: []string{w.id, s, c}, Errors: map[string]string{cp: "not sealed"}, Unattested: []string{c}}
	})
	add("stranger-checkpoint-does-not-attest", "A checkpoint of another identity vouches for nothing.", func(l *v19Log) v19Expect {
		w, x := l.who(1), l.who(2)
		s, _ := l.rotate(w, w.id, 0, 0x81)
		c := l.claim(w, w.key, "")
		cp := l.checkpoint(x, x.key, c)
		return v19Expect{Valid: []string{w.id, x.id, s, c, cp}, Unattested: []string{c}}
	})
	add("freeze-sealed-by-both", "A correction by the key at a fork stays valid when both branches sealed it.", func(l *v19Log) v19Expect {
		w := l.who(1)
		c := l.claim(w, w.key, "")
		f := l.supersede(w, w.key, c)
		s1, _ := l.rotate(w, w.id, 0, 0x81, c, f)
		s2, _ := l.rotate(w, w.id, 0, 0x82, c, f)
		return v19Expect{Compromised: []string{w.id}, Valid: []string{w.id, c, f}, Errors: map[string]string{s1: "frozen", s2: "frozen"}}
	})
	add("freeze-sealed-by-one", "It does not when only one branch sealed it, so neither party decides alone.", func(l *v19Log) v19Expect {
		w := l.who(1)
		c := l.claim(w, w.key, "")
		f := l.supersede(w, w.key, c)
		s1, _ := l.rotate(w, w.id, 0, 0x81, f)
		s2, _ := l.rotate(w, w.id, 0, 0x82)
		return v19Expect{Compromised: []string{w.id}, Valid: []string{w.id, c}, Errors: map[string]string{s1: "frozen", s2: "frozen", f: "frozen"}, Unattested: []string{c}}
	})
	add("freeze-claim-by-fork-key", "A claim by the key at a fork stays valid, but is unattested unless every branch sealed it. A thief can still write with that key.", func(l *v19Log) v19Expect {
		w := l.who(1)
		kept := l.claim(w, w.key, "")
		s1, _ := l.rotate(w, w.id, 0, 0x81, kept)
		s2, _ := l.rotate(w, w.id, 0, 0x82, kept)
		late := l.claim(w, w.key, "")
		return v19Expect{Compromised: []string{w.id}, Valid: []string{w.id, kept, late}, Errors: map[string]string{s1: "frozen", s2: "frozen"}, Unattested: []string{late}}
	})
	add("missing-named-by-two-identities", "Two identities that name one missing id each get a line. The result does not depend on map order.", func(l *v19Log) v19Expect {
		w, x := l.who(1), l.who(2)
		c := l.claim(w, w.key, "")
		cw := l.checkpoint(w, w.key, c)
		cx := l.checkpoint(x, x.key, c)
		delete(l.files, core.PathOf(c))
		return v19Expect{Valid: []string{w.id, x.id, cw, cx}, Missing: []string{c + " of " + w.id, c + " of " + x.id}}
	})
	add("held-succession-not-valid", "A succession not on the tree is held and not valid. Its signature proves nothing, because no key was checked.", func(l *v19Log) v19Expect {
		w := l.who(1)
		c := l.claim(w, w.key, "")
		k := core.NewEd25519Signer(bytes.Repeat([]byte{0x77}, 32))
		s := core.NewSuccession(w.id, k.Public(), c, k.Public(), core.Commitment(k.Public()), nil, "v19@0", "2026-11-06T00:00:00Z")
		delete(s, "nonce")
		l.put(s, k)
		return v19Expect{Valid: []string{w.id, c}}
	})
	add("unattested-copy-of-ref", "An unattested claim is not in a duplicate group, so a retired key cannot hide a record by copying its ref.", func(l *v19Log) v19Expect {
		w := l.who(1)
		c := l.claim(w, w.key, v19Ref)
		s, _ := l.rotate(w, w.id, 0, 0x81, c)
		f := l.claim(w, w.key, v19Ref)
		return v19Expect{Valid: []string{w.id, c, s, f}, Unattested: []string{f}}
	})
	add("tombstone-of-checkpoint", "A tombstone must not name a checkpoint.", func(l *v19Log) v19Expect {
		w := l.who(1)
		cp := l.checkpoint(w, w.key, w.id)
		t := l.tombstone(w, w.key, cp)
		return v19Expect{Valid: []string{w.id, cp}, Errors: map[string]string{t: "bad target"}}
	})
	write("v19-authority.json", out)
}

// v19Diff compares a report with what a scenario expects.
func v19Diff(rp *core.Report, e v19Expect) string {
	set := func(xs []string) string {
		s := append([]string{}, xs...)
		sort.Strings(s)
		return strings.Join(s, ",")
	}
	keys := func(m map[string]bool) string {
		var s []string
		for k, v := range m {
			if v {
				s = append(s, k)
			}
		}
		return set(s)
	}
	eq := func(a, b map[string]string) bool {
		if len(a) != len(b) {
			return false
		}
		for k, v := range a {
			if b[k] != v {
				return false
			}
		}
		return true
	}
	switch {
	case set(rp.TooNew) != set(e.TooNew):
		return fmt.Sprintf("too new %v", rp.TooNew)
	case set(rp.Compromised) != set(e.Compromised):
		return fmt.Sprintf("compromised %v", rp.Compromised)
	case keys(rp.Valid) != set(e.Valid) && len(e.TooNew) == 0:
		return fmt.Sprintf("valid %v want %v", keys(rp.Valid), set(e.Valid))
	case !eq(rp.Errors, e.Errors):
		return fmt.Sprintf("errors %v", rp.Errors)
	case keys(rp.Unattested) != set(e.Unattested):
		return fmt.Sprintf("unattested %v", rp.Unattested)
	case set(rp.MissingList()) != set(e.Missing):
		return fmt.Sprintf("missing %v", rp.MissingList())
	case !eq(rp.Duplicates, e.Duplicates):
		return fmt.Sprintf("duplicates %v", rp.Duplicates)
	}
	return ""
}
