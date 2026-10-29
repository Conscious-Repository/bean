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
	if out, _, code := run(t, dir, "", "version"); code != 0 || !strings.HasPrefix(out, "bean 0.5.0") {
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
