// Command genvectors computes vectors V1 to V12 (SPEC.md section 14) with the
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
	ps, err := newP256(p256D)
	if err != nil {
		panic(err)
	}
	pkid := p256Public(ps)
	low := p256Sign(ps, claimID)
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
	if core.Verify(pkid, claimID, low) != nil || core.Verify(pkid, claimID, high) == nil {
		panic("p256 vector self-check failed")
	}
	write("v6-p256.json", map[string]any{"spec": "SPEC.md 4.5", "private_scalar": hex.EncodeToString(p256D), "kid": pkid, "id": claimID, "ecdsa_hash_input": "sha256 of the 32 id bytes", "low_s_sig": low, "low_s_result": "accept", "high_s_sig": high, "high_s_result": "bad signature"})

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

func init() { extra = append(extra, genV19V20) }

func genV19V20(signer core.Signer, _ map[string]any) {
	kid := signer.Public()
	rec0 := core.RecoverySigner(seed, 0).Public()
	commit0 := core.Commitment(rec0)
	dedupeKey := core.DedupeKey(core.IdentitySecret(seed))

	// V19 and V20 (2026-10-30): verifier rules C26 and C27. The files and
	// the expected results come from core.VerifyFiles; tools/checkvectors.py
	// computes the results again on its own.
	nonce := 0x19000
	mk := func(sg core.Signer, m map[string]any) (string, map[string]any) {
		nonce++
		m["v"] = 0.0
		m["nonce"] = fmt.Sprintf("%032x", nonce)
		if _, ok := m["source"]; !ok {
			m["source"] = map[string]any{"system": "self", "adapter": "genvectors@1", "ref": nil}
		}
		if _, ok := m["times"]; !ok {
			m["times"] = map[string]any{"observed": nil, "imported": "2026-10-30T12:00:00Z"}
		}
		id, b, err := core.SignRecord(m, sg)
		if err != nil {
			panic(err)
		}
		return id, map[string]any{"path": core.PathOf(id), "bytes": string(b)}
	}
	claimBy := func(identity string, sg core.Signer, n float64, ref any) map[string]any {
		return map[string]any{"type": "hrv.rmssd", "subject": identity, "author": identity, "kid": sg.Public(),
			"value":  map[string]any{"n": n, "unit": "ms"},
			"times":  map[string]any{"observed": nil, "imported": "2026-10-30T12:00:00Z"},
			"source": map[string]any{"system": "csv", "adapter": "genvectors@1", "ref": ref}}
	}
	verifyFiles := func(files []any) *core.Report {
		m := map[string][]byte{}
		for _, f := range files {
			ff := f.(map[string]any)
			m[ff["path"].(string)] = []byte(ff["bytes"].(string))
		}
		return core.VerifyFiles(m)
	}

	// V19: genesis G with key K1. Claims A and B by K1. Succession S
	// (recovery generation 0) to K2 seals A only. Claim C by K2. Claim D by
	// K1 written after the rotation.
	k2 := core.NewEd25519Signer(hexb("3333333333333333333333333333333333333333333333333333333333333333"))
	var files []any
	g := core.NewGenesis(kid, commit0, "genvectors@1", "2026-10-30T12:00:00Z")
	gid, gf := mk(signer, g)
	files = append(files, gf)
	aid, af := mk(signer, claimBy(gid, signer, 1, nil))
	bid, bf := mk(signer, claimBy(gid, signer, 2, nil))
	files = append(files, af, bf)
	succ := core.NewSuccession(gid, rec0, gid, k2.Public(), core.Commitment(core.RecoverySigner(seed, 1).Public()), []string{aid}, "genvectors@1", "2026-10-30T12:00:00Z")
	sid, sf := mk(core.RecoverySigner(seed, 0), succ)
	cid, cf := mk(k2, claimBy(gid, k2, 3, nil))
	did, df := mk(signer, claimBy(gid, signer, 4, nil))
	files = append(files, sf, cf, df)
	rp := verifyFiles(files)
	result := func(id string) string {
		if rp.Valid[id] {
			return "valid"
		}
		return rp.Errors[id]
	}
	var expect []any
	for _, x := range []struct{ name, id string }{{"genesis G", gid}, {"claim A by K1, sealed", aid}, {"claim B by K1, not sealed", bid}, {"succession S", sid}, {"claim C by K2", cid}, {"claim D by K1 after rotation", did}} {
		expect = append(expect, map[string]any{"name": x.name, "id": x.id, "result": result(x.id)})
	}
	if result(aid) != "valid" || result(bid) != "not sealed" || result(cid) != "valid" || result(did) != "not sealed" {
		panic("V19 does not match C26")
	}
	write("v19-retired-key-seals.json", map[string]any{"spec": "SPEC.md 5.4 and 6.3 step 7, change note C26", "paper_seed": "paper_seed in v9-hkdf.json", "files": files, "expected": expect})

	// V20: the person P writes claims X and Y with one ref R. A bare agent
	// key writes claim Z about P with ref R. Only Y or X is a duplicate.
	agent := core.NewEd25519Signer(hexb("4444444444444444444444444444444444444444444444444444444444444444"))
	ref := core.DedupeRefNative(dedupeKey, "csv", "row-20", 0)
	files = []any{gf}
	xid, xf := mk(signer, claimBy(gid, signer, 5, ref))
	yid, yf := mk(signer, claimBy(gid, signer, 5, ref))
	z := claimBy(gid, agent, 6, ref)
	z["author"] = agent.Public()
	zid, zf := mk(agent, z)
	files = append(files, xf, yf, zf)
	rp = verifyFiles(files)
	var dups []any
	for _, id := range []string{xid, yid, zid} {
		if p, ok := rp.Duplicates[id]; ok {
			dups = append(dups, map[string]any{"id": id, "primary": p})
		}
	}
	if len(dups) != 1 || rp.Duplicates[zid] != "" {
		panic("V20 does not match C27")
	}
	write("v20-duplicates-per-author.json", map[string]any{"spec": "SPEC.md 6.3 step 12, change note C27", "files": files,
		"records": map[string]any{"person_x": xid, "person_y": yid, "agent_z": zid}, "expected_duplicates": dups})
}

func init() { extra = append(extra, genV21) }

// genV21 writes the seal continuation vector (C28, 2026-10-30). The files
// and the expected results come from core.VerifyFiles; tools/checkvectors.py
// computes the results again on its own.
func genV21(signer core.Signer, _ map[string]any) {
	kid := signer.Public()
	rec0 := core.RecoverySigner(seed, 0)
	nonce := 0x21000
	mk := func(sg core.Signer, m map[string]any) (string, map[string]any) {
		nonce++
		m["v"] = 0.0
		m["nonce"] = fmt.Sprintf("%032x", nonce)
		id, b, err := core.SignRecord(m, sg)
		if err != nil {
			panic(err)
		}
		return id, map[string]any{"path": core.PathOf(id), "bytes": string(b)}
	}
	const at = "2026-10-30T12:00:00Z"
	g := core.NewGenesis(kid, core.Commitment(rec0.Public()), "genvectors@1", at)
	gid, gf := mk(signer, g)
	claim := func(n float64) (string, map[string]any) {
		return mk(signer, map[string]any{"type": "hrv.rmssd", "subject": gid, "author": gid, "kid": kid,
			"value":  map[string]any{"n": n, "unit": "ms"},
			"times":  map[string]any{"observed": nil, "imported": at},
			"source": map[string]any{"system": "csv", "adapter": "genvectors@1", "ref": nil}})
	}
	// Claims A, B, C, D by K1. The rotation to K2 seals A, B and C over the
	// succession S and two parts; D is left out.
	var ids []string
	var claimFiles []any
	names := map[string]string{}
	for k, name := range []string{"A", "B", "C", "D"} {
		id, f := claim(float64(k + 1))
		ids = append(ids, id)
		claimFiles = append(claimFiles, f)
		names[id] = "claim " + name
	}
	sealed := append([]string(nil), ids[:3]...)
	sort.Strings(sealed)
	k2 := core.NewEd25519Signer(hexb("5555555555555555555555555555555555555555555555555555555555555555"))
	succ := core.NewSuccession(gid, rec0.Public(), gid, k2.Public(), core.Commitment(core.RecoverySigner(seed, 1).Public()), sealed[:1], "genvectors@1", at)
	succ["value"].(map[string]any)["parts"] = 2.0
	sid, sf := mk(rec0, succ)
	p1, p1f := mk(rec0, core.NewSeal(gid, rec0.Public(), sid, 1, sealed[1:2], "genvectors@1", at))
	p2, p2f := mk(rec0, core.NewSeal(gid, rec0.Public(), sid, 2, sealed[2:3], "genvectors@1", at))
	// Forged: part 2 naming D signed by the signing key K1, and part 3
	// (above value.parts) signed by the recovery key.
	f1, f1f := mk(signer, core.NewSeal(gid, kid, sid, 2, ids[3:4], "genvectors@1", at))
	f3, f3f := mk(rec0, core.NewSeal(gid, rec0.Public(), sid, 3, ids[3:4], "genvectors@1", at))
	// A second part 1, signed by the recovery key.
	d1, d1f := mk(rec0, core.NewSeal(gid, rec0.Public(), sid, 1, ids[3:4], "genvectors@1", at))
	names[gid], names[sid], names[p1], names[p2], names[f1], names[f3], names[d1] = "genesis G", "succession S", "part 1", "part 2", "forged part 2 by K1", "part 3 above parts", "second part 1"

	verify := func(files []any) *core.Report {
		m := map[string][]byte{}
		for _, f := range files {
			ff := f.(map[string]any)
			m[ff["path"].(string)] = []byte(ff["bytes"].(string))
		}
		return core.VerifyFiles(m)
	}
	base := append([]any{gf}, claimFiles...)
	type scen struct {
		name  string
		files []any
		ids   []string
	}
	all := append([]string{gid}, ids...)
	scens := []scen{
		{"complete", append(append([]any{}, base...), sf, p1f, p2f, f1f, f3f), append(append([]string{}, all...), sid, p1, p2, f1, f3)},
		{"missing part 2", append(append([]any{}, base...), sf, p1f), append(append([]string{}, all...), sid, p1)},
		{"duplicate part 1", append(append([]any{}, base...), sf, p1f, p2f, d1f), append(append([]string{}, all...), sid, p1, p2, d1)},
		{"parts before the succession", append(append([]any{}, base...), p1f, p2f), append(append([]string{}, all...), p1, p2)},
	}
	var out []any
	for _, sc := range scens {
		rp := verify(sc.files)
		var res []any
		for _, id := range sc.ids {
			r := "valid"
			if !rp.Valid[id] {
				r = rp.Errors[id]
			}
			if rp.Held[id] {
				r = "held"
			}
			res = append(res, map[string]any{"name": names[id], "id": id, "result": r})
		}
		inc := []any{}
		for _, x := range rp.Incomplete {
			inc = append(inc, x)
		}
		out = append(out, map[string]any{"name": sc.name, "files": sc.files, "expected": res, "incomplete": inc})
	}
	// The expected results must match C28 before the vector is written.
	want := map[string]string{
		"complete":                    "valid valid valid valid not sealed valid valid valid bad seal bad seal",
		"missing part 2":              "valid not sealed not sealed not sealed not sealed valid valid",
		"duplicate part 1":            "valid not sealed not sealed not sealed not sealed valid bad seal valid bad seal",
		"parts before the succession": "valid valid valid valid valid held held",
	}
	for _, o := range out {
		o := o.(map[string]any)
		var got []string
		for _, e := range o["expected"].([]any) {
			got = append(got, e.(map[string]any)["result"].(string))
		}
		// Claim order in "expected" is A, B, C, D, not sorted; A..C are sealed.
		if strings.Join(got, " ") != want[o["name"].(string)] {
			panic("V21 " + o["name"].(string) + ": " + strings.Join(got, " "))
		}
	}
	write("v21-seal-continuations.json", map[string]any{"spec": "SPEC.md 3.3, 5.4 and 6.3 steps 5b and 7, change note C28", "paper_seed": "paper_seed in v9-hkdf.json", "parts_limit": float64(core.MaxSealParts), "scenarios": out})
}
