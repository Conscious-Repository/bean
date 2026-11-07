package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mcpLog(label, value string) (string, []string) {
	in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"log_entry","arguments":{"type":"food","value":"` + value + `"}}}` + "\n"
	args := []string{"mcp", "-write"}
	if label != "" {
		args = append(args, "-label", label)
	}
	return in, args
}

// Two assistants on one machine share its one agent key. A second -label
// is refused: before, ChatGPT's records showed as [claude], and agent
// pair with a new label printed "paired" and changed nothing.
func TestCLIAgentSecondLabelRefused(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "", "init")
	in, args := mcpLog("claude", "ramen")
	if out, errs, code := run(t, dir, in, args...); code != 0 || !strings.Contains(out, `\"id\"`) {
		t.Fatalf("mcp claude: %d\n%s\n%s", code, out, errs)
	}
	in, args = mcpLog("chatgpt", "toast")
	out, errs, code := run(t, dir, in, args...)
	if code != 1 || !strings.Contains(errs, `paired as "claude"`) || !strings.Contains(errs, `"chatgpt"`) || out != "" {
		t.Errorf("mcp chatgpt: %d\n%s\n%s", code, out, errs)
	}
	// Without -label the key keeps its name.
	in, args = mcpLog("", "tea")
	if _, errs, code := run(t, dir, in, args...); code != 0 {
		t.Errorf("mcp without -label: %d %s", code, errs)
	}
	if _, errs, code := run(t, dir, "", "agent", "init", "-label", "other"); code != 1 || !strings.Contains(errs, `paired as "claude"`) {
		t.Errorf("agent init -label other: %d %s", code, errs)
	}
	who, _, _ := run(t, dir, "", "whoami")
	npub := ""
	for _, line := range strings.Split(who, "\n") {
		if rest, ok := strings.CutPrefix(line, "agent npub "); ok {
			npub = rest
		}
	}
	if npub == "" {
		t.Fatalf("no agent npub in\n%s", who)
	}
	if out, errs, code := run(t, dir, "", "agent", "pair", npub, "-label", "renamed"); code != 1 || !strings.Contains(errs, `already paired as "claude"`) || strings.Contains(out, "paired") {
		t.Errorf("agent pair -label renamed: %d\n%s\n%s", code, out, errs)
	}
	if out, errs, code := run(t, dir, "", "agent", "pair", npub, "-label", "claude"); code != 0 || !strings.HasPrefix(out, "paired sha256:") {
		t.Errorf("agent pair same label: %d\n%s\n%s", code, out, errs)
	}
	ls, _, _ := run(t, dir, "", "ls")
	if strings.Contains(ls, "toast") || strings.Contains(ls, "renamed") || strings.Count(ls, "[claude]") != 2 {
		t.Errorf("ls:\n%s", ls)
	}
}

// blob put refuses a file that import-export could not read back.
func TestCLIBlobPutLimit(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "", "init")
	big := filepath.Join(t.TempDir(), "big.bin")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(256<<20 + 1); err != nil { // sparse
		t.Fatal(err)
	}
	f.Close()
	out, errs, code := run(t, dir, "", "blob", "put", big)
	if code != 1 || out != "" || !strings.Contains(errs, "larger than 256 MiB") || !strings.Contains(errs, "import-export") {
		t.Errorf("blob put 256 MiB + 1: %d %q %q", code, out, errs)
	}
	small := filepath.Join(t.TempDir(), "small.txt")
	os.WriteFile(small, []byte("hello"), 0o600)
	if out, errs, code := run(t, dir, "", "blob", "put", small); code != 0 || !strings.HasPrefix(out, "sha256:") {
		t.Errorf("blob put small: %d %q %q", code, out, errs)
	}
}

// A time that cannot be read is a usage error: exit 2, as COMMANDS.md says.
func TestCLIBadTimeExitsTwo(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "", "init")
	for _, args := range [][]string{
		{"ls", "-since", "garbage"},
		{"ls", "-until", "garbage"},
		{"add", "weight", "80", "-unit", "kg", "-at", "garbage"},
	} {
		out, errs, code := run(t, dir, "", args...)
		if code != 2 || out != "" || !strings.Contains(errs, "bad time") || !strings.Contains(errs, "usage: bean") {
			t.Errorf("%v: %d %q %q", args, code, out, errs)
		}
	}
	if ls, _, _ := run(t, dir, "", "ls"); strings.Contains(ls, "weight") {
		t.Errorf("a record was written:\n%s", ls)
	}
}

// A kept report is a file, not a series. Before, ls showed every
// {"blob": id} value as "series".
func TestCLILsFileIsNotSeries(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "", "init")
	src := filepath.Join(t.TempDir(), "report.txt")
	os.WriteFile(src, []byte("synthetic report"), 0o600)
	out, _, _ := run(t, dir, "", "blob", "put", src)
	blob := strings.TrimSpace(out)
	hex := strings.TrimPrefix(blob, "sha256:")[:12]
	report := `{"type":"lab.report","value":{"blob":"` + blob + `"},"blobs":[{"hash":"` + blob + `","role":"raw-export","media_type":"text/plain"}],"times":{"observed":{"start":"2026-10-01T00:00:00Z","precision":"day"}},"source":{"system":"fake","adapter":"fake@1","ref":null}}`
	series := `{"type":"heart.rate","value":{"blob":"` + blob + `","unit":"/min"},"blobs":[{"hash":"` + blob + `","role":"raw-export","media_type":"text/csv"}],"times":{"observed":{"start":"2026-10-02T00:00:00Z","precision":"day"}},"source":{"system":"fake","adapter":"fake@1","ref":null}}`
	if out, errs, code := run(t, dir, report+"\n"+series+"\n", "append"); code != 0 {
		t.Fatalf("append: %d %s %s", code, out, errs)
	}
	ls, _, _ := run(t, dir, "", "ls")
	var rep, ser string
	for _, line := range strings.Split(ls, "\n") {
		switch {
		case strings.Contains(line, "lab.report"):
			rep = line
		case strings.Contains(line, "heart.rate"):
			ser = line
		}
	}
	if !strings.Contains(rep, "file text/plain "+hex) || strings.Contains(rep, "series") {
		t.Errorf("lab.report: %q", rep)
	}
	if !strings.Contains(ser, "series "+hex+" /min") {
		t.Errorf("series: %q", ser)
	}
}
