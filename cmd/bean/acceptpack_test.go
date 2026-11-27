package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestComparePackAcceptPack: -accept-pack names the one pack that the
// person checked. bean uses it when the file has that SHA-256, and stops
// when it has another. Every result says what -unknown-pack makes it say.
func TestComparePackAcceptPack(t *testing.T) {
	dir, _, ids := compareLog(t, "TCHOL_J", "HDL_J", "DEMO_J")
	path, digest := testPack(t)
	other := strings.Repeat("a", 64)
	args := []string{"compare", "-pack", path, "-cohort", "male-30-39", ids[0]}
	// The same value: the same output as with -unknown-pack.
	unknown, _, code := run(t, dir, "", append(args, "-unknown-pack")...)
	if code != 0 {
		t.Fatalf("-unknown-pack: %d\n%s", code, unknown)
	}
	out, errs, code := run(t, dir, "", append(args, "-accept-pack", digest)...)
	if code != 0 || out != unknown || !strings.Contains(out, "pack NOT on record") || !strings.Contains(out, "WARNING: bean has NO record of this pack") {
		t.Errorf("-accept-pack with the pack's SHA-256: %d\n%s\n%s\nwant\n%s", code, out, errs, unknown)
	}
	// Another value: bean stops and shows both, with or without readings.
	for _, a := range [][]string{append(args, "-accept-pack", other), {"compare", "-pack", path, "-accept-pack", other}} {
		out, errs, code := run(t, dir, "", a...)
		if code != 1 || out != "" || !strings.Contains(errs, "the file has SHA-256 "+digest+", and -accept-pack names "+other+".") || !strings.Contains(errs, "the one that the pack's publisher states") || strings.Contains(errs, `\x`) {
			t.Errorf("%v: %d\n%s\n%s", a, code, out, errs)
		}
	}
	// An error of use: a value that is not a SHA-256, both flags, and
	// -accept-pack with no pack. bean opens no pack for any of them.
	for name, a := range map[string][]string{
		"upper case":      append(args, "-accept-pack", strings.ToUpper(digest)),
		"63 digits":       append(args, "-accept-pack", digest[:63]),
		"a prefix":        append(args, "-accept-pack", "sha256:"+digest),
		"both flags":      append(args, "-accept-pack", digest, "-unknown-pack"),
		"no pack":         {"compare", "-accept-pack", digest, "-sex", "male", "-age", "30-39", ids[0]},
		"a typo, menu":    {"compare", "-pack", path, "-accept-pack", "x"},
		"empty":           append(args, "-accept-pack", ""),
		"empty, both":     append(args, "-accept-pack=", "-unknown-pack"),
		"empty, no pack":  {"compare", "-accept-pack=", "-sex", "male", "-age", "30-39", ids[0]},
		"twice, the same": append(args, "-accept-pack", digest, "-accept-pack", digest),
		"twice, two":      append(args, "-accept-pack", other, "-accept-pack", digest),
		"twice, menu":     {"compare", "-pack", path, "-accept-pack", digest, "-accept-pack", digest},
	} {
		out, errs, code := run(t, dir, "", a...)
		if code != 2 || out != "" || !strings.Contains(errs, "usage") {
			t.Errorf("%s: %d\n%s\n%s", name, code, out, errs)
		}
	}
	if _, errs, _ := run(t, dir, "", append(args, "-accept-pack", "x")...); !strings.Contains(errs, "-accept-pack takes the SHA-256 of a pack: 64 hex digits in lower case, with no prefix") {
		t.Errorf("a malformed value: %s", errs)
	}
	// One run uses one pack, so a second value is an error of use, not a
	// value that bean picks (DECISIONS 354).
	if _, errs, _ := run(t, dir, "", append(args, "-accept-pack", other, "-accept-pack", digest)...); !strings.Contains(errs, "-accept-pack names the one pack of -pack: pass it once") {
		t.Errorf("twice: %s", errs)
	}
	// Without -pack the flag names no pack: the error says what is missing.
	if _, errs, code := run(t, dir, "", "compare", "-accept-pack", digest, "-type", "lab.*", "-sex", "male", "-age", "30-39"); code != 2 || !strings.Contains(errs, "-cohort, -accept-pack and -unknown-pack go with -pack: give the file of the pack with -pack") {
		t.Errorf("no -pack: %d, %s", code, errs)
	}
	if _, errs, _ := run(t, dir, "", append(args, "-accept-pack", digest, "-unknown-pack")...); !strings.Contains(errs, "pass -accept-pack or -unknown-pack, not both") {
		t.Errorf("both flags: %s", errs)
	}
	// The refusal names -accept-pack first, with the file's SHA-256, and
	// gives no command ready to run.
	_, errs, code = run(t, dir, "", args...)
	if code != 1 || !strings.Contains(errs, "(SHA-256 "+digest+")") || strings.Contains(errs, "-accept-pack "+digest) ||
		!strings.Contains(errs, "-accept-pack <sha256>") || strings.Index(errs, "-accept-pack <sha256>") > strings.Index(errs, "-unknown-pack") {
		t.Errorf("the refusal: %d\n%s", code, errs)
	}
}

// TestAcceptPackSavesWhatUnknownPackSaves: a claim saved with -accept-pack
// holds the same texts as one saved with -unknown-pack, for the same pack,
// reading and cohort. Only the ids, the keys and the times differ.
func TestAcceptPackSavesWhatUnknownPackSaves(t *testing.T) {
	path, digest := testPack(t)
	claim := func(flag ...string) string {
		dir, _, ids := compareLog(t, "TCHOL_J", "HDL_J", "DEMO_J")
		args := append([]string{"compare", "-pack", path, "-cohort", "male-30-39", "-save", ids[0]}, flag...)
		if out, errs, code := run(t, dir, "", args...); code != 0 || !strings.Contains(out, "added 1,") {
			t.Fatalf("%v: %d\n%s\n%s", flag, code, out, errs)
		}
		ls, _, _ := run(t, dir, "", "ls", "-json", "-type", "comparison.percentile")
		var rec map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(ls)), &rec); err != nil {
			t.Fatalf("%v: %v\n%s", flag, err, ls)
		}
		for _, key := range []string{"author", "id", "kid", "nonce", "sig", "subject"} {
			delete(rec, key)
		}
		delete(rec["times"].(map[string]any), "imported")
		data, _ := json.Marshal(rec)
		return strings.ReplaceAll(string(data), ids[0], "READING")
	}
	unknown, accepted := claim("-unknown-pack"), claim("-accept-pack", digest)
	if unknown != accepted || !strings.Contains(accepted, "READING") {
		t.Errorf("-accept-pack saves\n%s\n-unknown-pack saves\n%s", accepted, unknown)
	}
}

// TestAcceptPackWithAPackOnRecord: the file must still have the SHA-256
// that the person named. With it, nothing changes, also with no -cohort.
func TestAcceptPackWithAPackOnRecord(t *testing.T) {
	dir, path, _ := personLog(t)
	_, digest := testPack(t)
	add(t, dir, "person.sex", "male")
	add(t, dir, "person.birth", "1986-05")
	plain, _, code := run(t, dir, "", "compare", "-pack", path, "-type", "lab.*")
	if code != 0 || strings.Contains(plain, "NOT on record") {
		t.Fatalf("%d\n%s", code, plain)
	}
	// The list of a pack on record names no flag.
	if menu, _, code := run(t, dir, "", "compare", "-pack", path); code != 0 || !strings.HasSuffix(menu, "that bean has on record.\n") || strings.Contains(menu, "-accept-pack") {
		t.Errorf("the list of a pack on record: %d\n%s", code, menu)
	}
	if same, errs, code := run(t, dir, "", "compare", "-pack", path, "-accept-pack", digest, "-type", "lab.*"); code != 0 || same != plain {
		t.Errorf("the same value: %d\n%s\n%s", code, same, errs)
	}
	if out, errs, code := run(t, dir, "", "compare", "-pack", path, "-accept-pack", strings.Repeat("0", 64), "-type", "lab.*"); code != 1 || out != "" || !strings.Contains(errs, "this is not the pack that -accept-pack names") {
		t.Errorf("another value: %d\n%s\n%s", code, out, errs)
	}
}
