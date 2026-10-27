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
	"strings"

	"bean/core"
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
	// V1: numbers. RFC 8785 Appendix B samples by literal, plus hlog range cases.
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
		"v": 0.0, "nonce": "00000000000000000000000000000001", "type": "hlog.genesis",
		"author": kid, "kid": kid,
		"value":  map[string]any{"key": kid, "recovery": commit0},
		"times":  map[string]any{"observed": nil, "imported": "2026-10-27T12:00:00Z"},
		"source": map[string]any{"system": "self", "adapter": "hlog@0.3.0", "ref": nil},
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
		"source":  map[string]any{"system": "csv", "adapter": "hlog-import-csv@0.3.0", "ref": claimRef},
		"context": map[string]any{"coding": map[string]any{"loinc": "80404-7"}},
	}
	claimID := core.RecordID(claim)
	sig, _ = signer.Sign(claimID)
	claim["id"], claim["sig"] = claimID, sig

	tomb := map[string]any{
		"v": 0.0, "nonce": "00000000000000000000000000000003", "type": "hlog.tombstone",
		"subject": genesisID, "author": genesisID, "kid": kid,
		"refs":    []any{map[string]any{"rel": "purges", "id": claimID}},
		"times":   map[string]any{"observed": nil, "imported": "2026-10-27T12:00:02Z"},
		"source":  map[string]any{"system": "self", "adapter": "hlog@0.3.0", "ref": nil},
		"context": map[string]any{"reason": "test"},
	}
	tombID := core.RecordID(tomb)
	sig, _ = signer.Sign(tombID)
	tomb["id"], tomb["sig"] = tombID, sig

	recVec := func(name string, r map[string]any) map[string]any {
		return map[string]any{"name": name, "record": r, "id_bytes": string(core.IDBytes(r)), "id": r["id"], "file_bytes": string(core.Canonical(r))}
	}
	write("v4-record-id.json", map[string]any{"spec": "SPEC.md 4.2", "records": []any{recVec("genesis", genesis), recVec("claim", claim), recVec("tombstone", tomb)}})

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
