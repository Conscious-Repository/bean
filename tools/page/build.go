// Command page builds bean-check.html: one file that checks a bundle in a
// browser, with no install, no account and no network.
//
//	go run ./tools/page -o bean-check.html
//
// It compiles tools/page/check (the check of bean check-bundle) to
// WebAssembly and puts it into page.html with the loader that Go ships
// (lib/wasm/wasm_exec.js). The page loads nothing else. The same commit and
// the same Go give the same bytes, so anyone can build the page and compare
// its SHA-256 with the one they were given. It prints that SHA-256.
package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed page.html
var page string

func main() {
	out := flag.String("o", "bean-check.html", "the page to write")
	flag.Parse()
	if err := build(*out); err != nil {
		fmt.Fprintln(os.Stderr, "page:", err)
		os.Exit(1)
	}
}

func build(out string) error {
	tmp, err := os.MkdirTemp("", "bean-page-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	wasmPath := filepath.Join(tmp, "check.wasm")
	// -trimpath, no version-control stamp and an empty build id: no path
	// of this machine, no state of this checkout and nothing of this run
	// ends up in the page.
	cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w -buildid=", "-o", wasmPath, "github.com/Conscious-Repository/bean/tools/page/check")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		return err
	}
	root, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return fmt.Errorf("go env GOROOT: %w", err)
	}
	loader, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(root)), "lib", "wasm", "wasm_exec.js"))
	if err != nil {
		return err
	}
	version, err := exec.Command("go", "env", "GOVERSION").Output()
	if err != nil {
		return fmt.Errorf("go env GOVERSION: %w", err)
	}
	// The program is packed, so that the page can go into an e-mail. The
	// header holds no time and no name: the bytes depend on the program only.
	var packed bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&packed, gzip.BestCompression)
	if _, err := zw.Write(wasm); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	html := page
	for mark, text := range map[string]string{
		"/*LOADER*/":  string(loader),
		"/*PROGRAM*/": base64.StdEncoding.EncodeToString(packed.Bytes()),
		"<!--GO-->":   strings.TrimSpace(string(version)),
	} {
		if strings.Count(html, mark) != 1 {
			return fmt.Errorf("page.html must hold %s once", mark)
		}
		// Nothing put into a script may end it.
		if strings.Contains(strings.ToLower(text), "</script") {
			return fmt.Errorf("the text for %s would end its script", mark)
		}
		html = strings.Replace(html, mark, text, 1)
	}
	if err := os.WriteFile(out, []byte(html), 0o644); err != nil {
		return err
	}
	fmt.Printf("%s  %d bytes  sha256 %x\n", out, len(html), sha256.Sum256([]byte(html)))
	return nil
}
