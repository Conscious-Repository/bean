package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/Conscious-Repository/bean/core"
	"github.com/Conscious-Repository/bean/internal/nhanes"
	"github.com/Conscious-Repository/bean/internal/pack"
)

// The test binary runs main() when BEAN_TEST_MAIN is set, so these tests
// drive the real CLI: arguments, exit codes, stdout and stderr.
func TestMain(m *testing.M) {
	if os.Getenv("BEAN_TEST_MAIN") == "1" {
		// A test can put the digest of a synthetic file on record, as
		// "TCHOL_J=<SHA-256>,DEMO_J=<SHA-256>": the real files are too large
		// to keep in the repository. This is test code: the program that
		// ships reads no such variable.
		for _, pair := range strings.Split(os.Getenv("BEAN_TEST_ON_RECORD"), ",") {
			if name, digest, ok := strings.Cut(pair, "="); ok {
				nhanes.Known[name] = digest
			}
		}
		// A test can put the synthetic pack on record in the same way, by
		// its SHA-256: it has the menu of the survey's packs.
		if digest := os.Getenv("BEAN_TEST_PACK_ON_RECORD"); digest != "" {
			pack.Known[digest] = pack.Known["10555d6769421d4305cbcd4ff14960d8b966ddd829a50581a9584a60a84d29e2"]
		}
		os.Args = append([]string{"bean"}, os.Args[1:]...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func run(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "BEAN_TEST_MAIN=1", "BEAN_DIR="+dir, "BEAN_NOW=")
	// A program built with the race detector sleeps for a second when it
	// exits. Hundreds of runs made the tests pass their time limit in CI.
	// A race is still reported, and still fails the run.
	if os.Getenv("GORACE") == "" {
		cmd.Env = append(cmd.Env, "GORACE=atexit_sleep_ms=0")
	}
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), errb.String(), code
}

func TestCLIHelpAndErrors(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		out, _, code := run(t, dir, "", args...)
		if code != 0 || !strings.Contains(out, "usage: bean [-dir <folder>] <command>") || !strings.Contains(out, "docs/COMMANDS.md") {
			t.Errorf("%v: exit %d\n%s", args, code, out)
		}
	}
	if out, _, code := run(t, dir, "", "version"); code != 0 || !strings.HasPrefix(out, "bean "+core.Version+" (records v 0)\n") {
		t.Errorf("version: %d %q", code, out)
	}
	if _, errs, code := run(t, dir, ""); code != 2 || !strings.Contains(errs, "usage: bean") {
		t.Errorf("no command: %d %q", code, errs)
	}
	if _, errs, code := run(t, dir, "", "lst"); code != 2 || !strings.Contains(errs, `unknown command "lst"`) || strings.Contains(errs, "Start") {
		t.Errorf("unknown command: %d %q", code, errs)
	}
	if _, errs, code := run(t, dir, "", "ls"); code != 1 || !strings.Contains(errs, "bean: not initialized") || !strings.Contains(errs, "no log in") {
		t.Errorf("ls in an empty folder: %d %q", code, errs)
	}
	run(t, dir, "", "init") // usage errors are checked inside a log
	if _, errs, code := run(t, dir, "", "purge"); code != 2 || !strings.Contains(errs, "usage: bean [-dir <folder>] purge [-reason <text>] <id>") {
		t.Errorf("purge usage: %d %q", code, errs)
	}
}

func TestCLIFirstRun(t *testing.T) {
	dir := t.TempDir()
	out, errs, code := run(t, dir, "", "init")
	if code != 0 || !strings.HasPrefix(out, "identity sha256:") || strings.Count(out, "\n") != 25 || !strings.Contains(errs, "24 words") {
		t.Fatalf("init: %d\n%s\n%s", code, out, errs)
	}
	if _, errs, code := run(t, dir, "", "init"); code != 1 || !strings.Contains(errs, "already initialized") {
		t.Errorf("second init: %d %q", code, errs)
	}
	if _, errs, code := run(t, dir, "", "add", "symptom.headache", "8", "-unit", "{score}", "-note", "salty lunch"); code != 0 {
		t.Fatalf("add: %d %s", code, errs)
	}
	out, _, code = run(t, dir, "", "ls")
	if code != 0 || !strings.Contains(out, "symptom.headache") || !strings.Contains(out, "8 {score}  # salty lunch") {
		t.Errorf("ls: %d\n%s", code, out)
	}
	if out, _, code := run(t, dir, "", "verify"); code != 0 || !strings.Contains(out, "summary: 2 valid, 0 invalid") {
		t.Errorf("verify: %d\n%s", code, out)
	}
}

func wordsFrom(initOut string) string {
	var b strings.Builder
	for _, l := range strings.Split(initOut, "\n") {
		if strings.Contains(l, ". ") {
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}

// rotate shows what the old key signed and writes nothing until the person
// decides. It does not ask for the 24 words first.
func TestCLIRotateNeedsDecision(t *testing.T) {
	dir := t.TempDir()
	out, _, _ := run(t, dir, "", "init")
	words := wordsFrom(out)
	id, _, _ := run(t, dir, "", "add", "lab.hba1c", "9.1", "-unit", "%")
	if _, errs, code := run(t, dir, "", "rotate"); code != 1 || !strings.Contains(errs, "undecided: 1 records") || !strings.Contains(errs, id[7:19]) || !strings.Contains(errs, "lab.hba1c 9.1 %") || strings.Contains(errs, "paste the 24 words") {
		t.Errorf("rotate with no decision: %d\n%s", code, errs)
	}
	if _, errs, code := run(t, dir, "", "rotate", "-list"); code != 0 || !strings.Contains(errs, "lab.hba1c 9.1 %") {
		t.Errorf("rotate -list: %d\n%s", code, errs)
	}
	if out, _, code := run(t, dir, "", "verify"); code != 0 || !strings.Contains(out, "2 valid") {
		t.Errorf("a refused rotation wrote something: %d %s", code, out)
	}
	if _, errs, code := run(t, dir, words, "rotate", "-seal-all", "-no-seal", "abcdef"); code != 1 || !strings.Contains(errs, "input missing") {
		t.Errorf("a bad id: %d %s", code, errs)
	}
	if out, errs, code := run(t, dir, words, "rotate", "-seal-all"); code != 0 || !strings.Contains(out, "sealed 1") {
		t.Errorf("rotate -seal-all: %d\n%s\n%s", code, out, errs)
	}
}

// A long list shows the deletions in full, and counts the claims.
func TestCLIRotateListCapsClaims(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "", "init")
	var lines strings.Builder
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&lines, `{"type":"hr","value":{"n":%d},"times":{"observed":null},"source":{"system":"x","adapter":"x@1","ref":null}}`+"\n", i)
	}
	run(t, dir, lines.String(), "append")
	x, _, _ := run(t, dir, "", "add", "lab.x", "1")
	run(t, dir, "", "purge", strings.TrimSpace(x))
	_, errs, _ := run(t, dir, "", "rotate")
	if !strings.Contains(errs, "DELETES") || !strings.Contains(errs, "30 claims of these types: hr (30)") || strings.Count(errs, "  hr ") != 20 {
		t.Errorf("summary:\n%s", errs)
	}
	_, errs, _ = run(t, dir, "", "rotate", "-list")
	if strings.Count(errs, "  hr ") != 30 {
		t.Errorf("-list shows %d", strings.Count(errs, "  hr "))
	}
}

// A changed reading is loud, and written only with -corrections.
func TestCLIChangedReading(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "", "init")
	line := func(v string) string {
		return `{"type":"lab.k","value":` + v + `,"times":{"observed":null},"source":{"system":"labcorp","adapter":"x@1","ref_input":["bean/v0/ref","id","labcorp","S/k",0]}}` + "\n"
	}
	run(t, dir, line(`{"s":"Pending"}`), "append")
	if out, errs, code := run(t, dir, line(`{"n":5.9}`), "append"); code != 1 || !strings.Contains(out, "added 0, skipped 1") || !strings.Contains(errs, "changed:") || !strings.Contains(errs, "-corrections") {
		t.Errorf("append: %d %s %s", code, out, errs)
	}
	if out, _, _ := run(t, dir, "", "ls"); !strings.Contains(out, "Pending") {
		t.Errorf("ls: %s", out)
	}
	if out, _, code := run(t, dir, line(`{"n":5.9}`), "append", "-corrections"); code != 0 || !strings.Contains(out, "added 1, skipped 0, 1 of them new values") {
		t.Errorf("append -corrections: %d %s", code, out)
	}
	if out, _, _ := run(t, dir, "", "ls"); !strings.Contains(out, "5.9") || strings.Contains(out, "Pending") {
		t.Errorf("ls: %s", out)
	}
}

// The list names every target and every pairing, and -no-seal-file works.
func TestCLIRotateListsEverything(t *testing.T) {
	dir := t.TempDir()
	out, _, _ := run(t, dir, "", "init")
	words := wordsFrom(out)
	s := core.Open(dir)
	id, _ := s.Identity()
	l, err := s.ReadLocal()
	if err != nil {
		t.Fatal(err)
	}
	x, _ := run2(t, dir, "add", "note", "coffee")
	absent := "sha256:" + strings.Repeat("b", 64)
	o := map[string]any{"v": 0.0, "nonce": core.NewNonce(), "type": "note", "subject": id, "author": id, "kid": l.Signer.Public(), "value": map[string]any{"s": "coffee"},
		"refs":   []any{map[string]any{"rel": "supersedes", "id": strings.TrimSpace(x)}, map[string]any{"rel": "supersedes", "id": absent}},
		"times":  map[string]any{"observed": nil, "imported": "2020-01-01T00:00:00Z"},
		"source": map[string]any{"system": "self", "adapter": "x@1", "ref": nil}}
	fid, data, err := core.SignRecord(o, l.Signer)
	if err != nil {
		t.Fatal(err)
	}
	p := dir + "/" + core.PathOf(fid)
	os.MkdirAll(p[:strings.LastIndex(p, "/")], 0o700)
	os.WriteFile(p, data, 0o600)
	run(t, dir, "", "agent", "init", "-label", "assistant")
	_, errs, _ := run(t, dir, "", "rotate")
	if !strings.Contains(errs, "A RECORD NOT IN THIS FOLDER, bbbbbbbbbbbb") || !strings.Contains(errs, "TRUSTS a key: assistant") {
		t.Errorf("list:\n%s", errs)
	}
	f := t.TempDir() + "/no-seal"
	os.WriteFile(f, []byte(fid+"\n"), 0o600)
	if out, errs, code := run(t, dir, words, "rotate", "-seal-all", "-no-seal-file", f); code != 0 || !strings.Contains(out, "not sealed "+fid) {
		t.Errorf("-no-seal-file: %d\n%s\n%s", code, out, errs)
	}
}

func run2(t *testing.T, dir string, args ...string) (string, string) {
	t.Helper()
	out, errs, _ := run(t, dir, "", args...)
	return out, errs
}

// A frozen log says so on reads, exits 4, and has a way forward.
func TestCLIFreezeIsLoud(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	out, _, _ := run(t, a, "", "init")
	words := wordsFrom(out)
	run(t, a, "", "add", "weight", "81", "-unit", "kg")
	if _, errs, code := run(t, b, "", "sync", a); code != 0 {
		t.Fatalf("sync: %d %s", code, errs)
	}
	if _, errs, code := run(t, a, words, "rotate", "-seal-all"); code != 0 {
		t.Fatalf("rotate: %d %s", code, errs)
	}
	if _, errs, code := run(t, b, words, "restore", "-seal-all"); code != 0 {
		t.Fatalf("restore: %d %s", code, errs)
	}
	run(t, a, "", "sync", b)
	out, errs, code := run(t, a, "", "ls")
	if code != 4 || !strings.Contains(out, "weight") || !strings.Contains(errs, "bean: compromised:") || !strings.Contains(errs, "bean migrate") {
		t.Errorf("ls: %d\n%s\n%s", code, out, errs)
	}
	dst := t.TempDir()
	out, errs, code = run(t, a, "", "migrate", dst)
	if code != 0 || !strings.Contains(out, "24. ") || !strings.Contains(out, "migrated 1 records") {
		t.Errorf("migrate: %d\n%s\n%s", code, out, errs)
	}
	if out, _, code := run(t, dst, "", "ls"); code != 0 || !strings.Contains(out, "81 kg") {
		t.Errorf("ls after migrate: %d\n%s", code, out)
	}
}

// A halted log says so on reads, and the export still runs.
func TestCLIHaltIsLoud(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "", "init")
	run(t, dir, "", "add", "weight", "81", "-unit", "kg")
	s := core.Open(dir)
	id, _ := s.Identity()
	l, err := s.ReadLocal()
	if err != nil {
		t.Fatal(err)
	}
	rid, data, err := core.SignRecord(map[string]any{"v": 1.0, "nonce": core.NewNonce(), "type": "bean.anything", "author": id, "kid": l.Signer.Public(), "subject": id}, l.Signer)
	if err != nil {
		t.Fatal(err)
	}
	p := dir + "/" + core.PathOf(rid)
	os.MkdirAll(p[:strings.LastIndex(p, "/")], 0o700)
	os.WriteFile(p, data, 0o600)
	if _, errs, code := run(t, dir, "", "ls"); code != 3 || !strings.Contains(errs, "too new") || !strings.Contains(errs, "bean rotate") {
		t.Errorf("ls: %d %s", code, errs)
	}
	out, errs, code := run(t, dir, "", "export", t.TempDir()+"/x")
	if code != 3 || !strings.Contains(out, "exported 3 records") || !strings.Contains(errs, "too new") {
		t.Errorf("export: %d %s %s", code, out, errs)
	}
}

// A forged duplicate with a lower id takes a record's place, and the list says.
// restore -list does not claim that there is nothing to decide on.
func TestCLIRotateNamesReplacement(t *testing.T) {
	dir := t.TempDir()
	out, _, _ := run(t, dir, "", "init")
	words := wordsFrom(out)
	run(t, dir, `{"type":"lab.k","value":{"n":4},"times":{"observed":null},"source":{"system":"labcorp","adapter":"x@1","ref_input":["bean/v0/ref","id","labcorp","S/k",0]}}`+"\n", "append")
	s := core.Open(dir)
	id, _ := s.Identity()
	loc, _ := s.ReadLocal()
	l, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	var orig *core.Record
	for _, r := range l.Report.Records {
		if r.Type == "lab.k" {
			orig = r
		}
	}
	for {
		o := map[string]any{"v": 0.0, "nonce": core.NewNonce(), "type": "lab.k", "subject": id, "author": id, "kid": loc.Signer.Public(), "value": map[string]any{"n": 9.0},
			"times":  map[string]any{"observed": nil, "imported": "2001-01-01T00:00:00Z"},
			"source": map[string]any{"system": "labcorp", "adapter": "x@1", "ref": orig.Source.Ref}}
		if core.RecordID(o) > orig.ID {
			continue
		}
		fid, data, err := core.SignRecord(o, loc.Signer)
		if err != nil {
			t.Fatal(err)
		}
		p := dir + "/" + core.PathOf(fid)
		os.MkdirAll(p[:strings.LastIndex(p, "/")], 0o700)
		os.WriteFile(p, data, 0o600)
		break
	}
	if _, errs, _ := run(t, dir, "", "rotate"); !strings.Contains(errs, "REPLACES lab.k 4 with 9") {
		t.Errorf("list:\n%s", errs)
	}
	// Two identities and no local/: -list cannot tell, and says so.
	b := t.TempDir()
	run(t, b, "", "sync", dir)
	o2 := t.TempDir()
	run(t, o2, "", "init")
	run(t, b, "", "sync", o2)
	if _, errs, code := run(t, b, words, "restore", "-list"); code == 0 || !strings.Contains(errs, "-identity") {
		t.Errorf("restore -list with two identities: %d %s", code, errs)
	}
}
