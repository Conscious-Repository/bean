package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Regression tests from attack 10 on the importer's process group.
// Synthetic data only.

// startImport runs bean import with a shell script for an importer. The
// script writes a process id to $CHILD_PID; startImport returns it.
func startImport(t *testing.T, script string, setpgid bool) (*exec.Cmd, int) {
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "bean-import-grp"), []byte(script), 0o755)
	dir := t.TempDir()
	run(t, dir, "", "init")
	pidFile := filepath.Join(t.TempDir(), "child")
	f := filepath.Join(t.TempDir(), "report.txt")
	os.WriteFile(f, []byte("SYNTHETIC glucose 101\n"), 0o600)
	cmd := exec.Command(os.Args[0], "import", "-yes", "grp", f)
	cmd.Env = append(os.Environ(), "BEAN_TEST_MAIN=1", "BEAN_DIR="+dir, "BEAN_NOW=", "TMPDIR="+t.TempDir(), "CHILD_PID="+pidFile,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if setpgid {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := 0
	for deadline := time.Now().Add(5 * time.Second); pid == 0 && time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if b, _ := os.ReadFile(pidFile); strings.HasSuffix(string(b), "\n") {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
	}
	if pid <= 0 {
		cmd.Process.Kill()
		t.Fatal("the importer did not start")
	}
	time.Sleep(300 * time.Millisecond)
	return cmd, pid
}

func stillAlive(pid int) bool {
	time.Sleep(500 * time.Millisecond)
	return syscall.Kill(pid, 0) != syscall.ESRCH
}

// The importer exits, but a child it started keeps its stdout open, so
// bean waits on the pipe. A signal then must still kill the group: the
// leader was reaped, but its child is a member.
func TestSignalKillsChildOfExitedImporter(t *testing.T) {
	cmd, pid := startImport(t, "#!/bin/sh\nsleep 30 &\necho $! > \"$CHILD_PID\"\nexit 0\n", false)
	defer syscall.Kill(pid, syscall.SIGKILL)
	cmd.Process.Signal(syscall.SIGTERM)
	cmd.Wait()
	t.Logf("bean exit %d", cmd.ProcessState.ExitCode())
	if stillAlive(pid) {
		t.Errorf("the importer's child %d runs on after bean was stopped", pid)
	}
}

// Without a signal, a child that keeps the importer's stdout open cannot
// hold bean: Wait gives up, the group is killed and the import fails.
func TestChildHoldingOutputCannotHangImport(t *testing.T) {
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "bean-import-grp"), []byte("#!/bin/sh\nsleep 30 &\necho $! > \"$CHILD_PID\"\nexit 0\n"), 0o755)
	dir := t.TempDir()
	run(t, dir, "", "init")
	pidFile := filepath.Join(t.TempDir(), "child")
	f := filepath.Join(t.TempDir(), "report.txt")
	os.WriteFile(f, []byte("SYNTHETIC\n"), 0o600)
	cmd := exec.Command(os.Args[0], "import", "-yes", "grp", f)
	cmd.Env = append(os.Environ(), "BEAN_TEST_MAIN=1", "BEAN_DIR="+dir, "BEAN_NOW=", "TMPDIR="+t.TempDir(), "CHILD_PID="+pidFile,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	start := time.Now()
	out, _ := cmd.CombinedOutput()
	if b, _ := os.ReadFile(pidFile); len(b) > 0 {
		pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		defer syscall.Kill(pid, syscall.SIGKILL)
		if pid > 0 && stillAlive(pid) {
			t.Errorf("the importer's child %d runs on after the import failed", pid)
		}
	}
	if time.Since(start) > 10*time.Second || cmd.ProcessState.ExitCode() == 0 {
		t.Fatalf("import held or succeeded: %v, exit %d: %s", time.Since(start), cmd.ProcessState.ExitCode(), out)
	}
	if !strings.Contains(string(out), "kept its output open") {
		t.Errorf("output: %s", out)
	}
}

// A SIGKILL to bean's process group (timeout -s KILL, a job runner
// cancelling a step) does not reach the importer, which leads a group of its
// own. On Linux the kernel kills it when bean dies. Processes the importer
// started are not covered (COMMANDS.md); the script execs, so the importer
// is the process that holds the file.
func TestGroupKillReachesImporter(t *testing.T) {
	if !importerDiesWithBean {
		t.Skip("no parent-death signal on this platform: an importer outlives a SIGKILL to bean's group (COMMANDS.md)")
	}
	cmd, pid := startImport(t, "#!/bin/sh\necho $$ > \"$CHILD_PID\"\nexec sleep 30\n", true)
	defer syscall.Kill(pid, syscall.SIGKILL)
	syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	cmd.Wait()
	if stillAlive(pid) {
		t.Errorf("the importer %d runs on after bean's group was killed", pid)
	}
}
