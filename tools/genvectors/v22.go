package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Conscious-Repository/bean/core"
)

func init() { extra = append(extra, genV22) }

// genV22 writes the too-new gate vector (K1 and C29, 2026-10-30). Each
// scenario is one identity with one claim A and some records of an unknown
// v. The expected results come from core.VerifyFiles; tools/checkvectors.py
// computes them again on its own.
func genV22(signer core.Signer, _ map[string]any) {
	kid := signer.Public()
	rec0, rec1 := core.RecoverySigner(seed, 0), core.RecoverySigner(seed, 1)
	nonce := 0x22000
	mk := func(sg core.Signer, m map[string]any) (string, map[string]any) {
		nonce++
		if _, ok := m["v"]; !ok {
			m["v"] = 0.0
		}
		m["nonce"] = fmt.Sprintf("%032x", nonce)
		id, b, err := core.SignRecord(m, sg)
		if err != nil {
			panic(err)
		}
		return id, map[string]any{"path": core.PathOf(id), "bytes": string(b)}
	}
	const at = "2026-10-30T12:00:00Z"
	gid, gf := mk(signer, core.NewGenesis(kid, core.Commitment(rec0.Public()), "genvectors@1", at))
	aid, af := mk(signer, map[string]any{"type": "hrv.rmssd", "subject": gid, "author": gid, "kid": kid,
		"value":  map[string]any{"n": 1.0, "unit": "ms"},
		"times":  map[string]any{"observed": nil, "imported": at},
		"source": map[string]any{"system": "csv", "adapter": "genvectors@1", "ref": nil}})
	future := func(sg core.Signer, typ string) (string, map[string]any) {
		m := core.NewTombstone(gid, sg.Public(), gid, "", "genvectors@1", at)
		m["type"] = typ
		m["v"] = 99.0
		return mk(sg, m)
	}
	xid, xf := future(signer, "bean.tombstone") // by the head key K1
	k2 := core.NewEd25519Signer(hexb("6666666666666666666666666666666666666666666666666666666666666666"))
	stranger := core.NewEd25519Signer(hexb("7777777777777777777777777777777777777777777777777777777777777777"))
	past := []string{aid, xid}
	sort.Strings(past)
	sid, sf := mk(rec0, core.NewSuccession(gid, rec0.Public(), gid, k2.Public(), core.Commitment(rec1.Public()), past, "genvectors@1", at))
	plain := []string{aid}
	tid, tf := mk(rec0, core.NewSuccession(gid, rec0.Public(), gid, k2.Public(), core.Commitment(rec1.Public()), plain, "genvectors@1", at))
	zid, zf := future(rec0, "bean.succession")    // by recovery generation 0
	z1id, z1f := future(rec1, "bean.succession")  // by recovery generation 1
	wid, wf := future(stranger, "bean.tombstone") // by a stranger, author = the identity
	names := map[string]string{gid: "G: genesis", aid: "A: claim", xid: "X: v99 by K1", sid: "S: K1 to K2 sealing A and X",
		tid: "T: K1 to K2 sealing A", zid: "Z: v99 by recovery 0", z1id: "Z1: v99 by recovery 1", wid: "W: v99 by a stranger"}
	type scen struct {
		name  string
		files []any
	}
	scens := []scen{
		{"halt by the head key", []any{gf, af, xf}},
		{"rotation past the halt", []any{gf, af, xf, sf}},
		{"retired key", []any{gf, af, tf, xf}},
		{"next recovery key", []any{gf, af, zf}},
		{"used recovery key", []any{gf, af, tf, zf}},
		{"next recovery key after rotation", []any{gf, af, tf, z1f}},
		{"head key and recovery key", []any{gf, af, xf, zf}},
		{"stranger", []any{gf, af, wf}},
	}
	want := map[string]string{
		"halt by the head key":             "too_new=G ungated= valid= past=X",
		"rotation past the halt":           "too_new= ungated=X valid=A,G,S past=",
		"retired key":                      "too_new= ungated=X valid=A,G,T past=",
		"next recovery key":                "too_new=G ungated= valid= past=",
		"used recovery key":                "too_new=G ungated= valid= past=",
		"next recovery key after rotation": "too_new=G ungated= valid= past=",
		"head key and recovery key":        "too_new=G ungated= valid= past=",
		"stranger":                         "too_new= ungated=W valid=A,G past=",
	}
	short := func(ids []string) string {
		var out []string
		for _, id := range ids {
			out = append(out, strings.Fields(names[id])[0])
		}
		sort.Strings(out)
		return strings.TrimSuffix(strings.Join(out, ","), ":")
	}
	var out []any
	for _, sc := range scens {
		m := map[string][]byte{}
		for _, f := range sc.files {
			ff := f.(map[string]any)
			m[ff["path"].(string)] = []byte(ff["bytes"].(string))
		}
		rp := core.VerifyFiles(m)
		ungated := []string{}
		for id, ws := range rp.Warnings {
			for _, w := range ws {
				if strings.HasPrefix(w, "ungated: ") {
					ungated = append(ungated, id)
				}
			}
		}
		valid := []string{}
		for id := range rp.Valid {
			valid = append(valid, id)
		}
		pastIDs := append([]string{}, rp.HeadHalt[gid]...)
		tooNew := append([]string{}, rp.TooNew...)
		sort.Strings(ungated)
		sort.Strings(valid)
		got := fmt.Sprintf("too_new=%s ungated=%s valid=%s past=%s", short(tooNew), short(ungated), short(valid), short(pastIDs))
		got = strings.ReplaceAll(got, ":", "")
		if got != want[sc.name] {
			panic("V22 " + sc.name + ": " + got)
		}
		out = append(out, map[string]any{"name": sc.name, "files": sc.files,
			"too_new": tooNew, "ungated": ungated, "valid": valid, "rotate_past": pastIDs})
	}
	write("v22-too-new-gate.json", map[string]any{"spec": "SPEC.md 8.2 and 5.5 step 4a, change notes K1 and C29", "paper_seed": "paper_seed in v9-hkdf.json",
		"note": "rotate_past lists the halting records when 5.5 step 4a lets rotate and restore go past the halt; empty otherwise.", "scenarios": out})
}
