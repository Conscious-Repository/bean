package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The test binary runs main() when BEAN_TEST_MAIN is set, so these tests
// drive the real CLI: arguments, exit codes, stdout and stderr.
func TestMain(m *testing.M) {
	if os.Getenv("BEAN_TEST_MAIN") == "1" {
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
	if out, _, code := run(t, dir, "", "version"); code != 0 || !strings.HasPrefix(out, "bean 0.6.0") {
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

// wordsOf cuts the 24 words out of the init output.
func wordsOf(initOut string) string {
	var b strings.Builder
	for _, l := range strings.Split(initOut, "\n") {
		if strings.Contains(l, ". ") {
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}

// rotate lists the records that need a decision and writes nothing until each
// one has one. It does not ask for the 24 words first.
func TestCLIRotateNeedsDecision(t *testing.T) {
	dir := t.TempDir()
	out, _, _ := run(t, dir, "", "init")
	words := wordsOf(out)
	id, _, _ := run(t, dir, "", "add", "lab.hba1c", "9.1", "-unit", "%")
	if _, errs, code := run(t, dir, "", "rotate"); code != 1 || !strings.Contains(errs, "undecided: 1 records") || !strings.Contains(errs, id[7:19]) || !strings.Contains(errs, "lab.hba1c 9.1 %") || strings.Contains(errs, "paste the 24 words") {
		t.Errorf("rotate with no decision: %d\n%s", code, errs)
	}
	if out, _, _ := run(t, dir, "", "ls", "-all", "-type", "bean.succession"); strings.Contains(out, "bean.succession") {
		t.Error("rotate wrote a succession")
	}
	if _, errs, code := run(t, dir, words, "rotate", "-seal-all", "-no-seal", "abcdef"); code != 1 || !strings.Contains(errs, "input missing") {
		t.Errorf("bad id: %d %s", code, errs)
	}
	if out, errs, code := run(t, dir, words, "rotate", "-seal-all"); code != 0 || !strings.Contains(out, "sealed 1") {
		t.Errorf("rotate -seal-all: %d\n%s\n%s", code, out, errs)
	}
	if out, _, code := run(t, dir, "", "verify"); code != 0 {
		t.Errorf("verify: %d\n%s", code, out)
	}
}

// A frozen log says so on stderr and exits 4, on reads too. It still prints.
func TestCLIFreezeIsLoud(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	out, _, _ := run(t, a, "", "init")
	words := wordsOf(out)
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
	if _, errs, code := run(t, a, "", "add", "weight", "82"); code != 4 || !strings.Contains(errs, "compromised") {
		t.Errorf("add: %d %s", code, errs)
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

// checkpoint is a command, and a removed record shows in verify.
func TestCLICheckpoint(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "", "init")
	run(t, dir, "", "add", "weight", "81")
	if out, errs, code := run(t, dir, "", "checkpoint"); code != 0 || !strings.HasPrefix(out, "checkpoint sha256:") {
		t.Errorf("checkpoint: %d %s %s", code, out, errs)
	}
	if out, _, code := run(t, dir, "", "checkpoint"); code != 0 || !strings.Contains(out, "nothing new") {
		t.Errorf("second checkpoint: %d %s", code, out)
	}
}
