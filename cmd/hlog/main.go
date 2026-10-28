// Command hlog is the reference CLI (SPEC.md section 12).
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"bean/core"
)

func usage() {
	fmt.Fprintln(os.Stderr, `usage: hlog [-dir <path>] <command> [args]

commands:
  init                      create an identity; prints the 24 words once
  append                    read JSONL partial records on stdin, sign, store
  blob put <file>           encrypt and store a file; prints the blob id
  blob get <id>             decrypt a blob to stdout
  verify                    check the folder; prints verify lines
  reindex                   rebuild local/cache
  purge [-reason <t>] <id>  tombstone and erase a record
  gc                        erase hidden targets older than the hold
  rotate [-no-seal <id>]... switch to a new signing key; reads the 24 words on stdin
  sync <source dir>         copy missing files from another replica
  export <dir>              write a plaintext copy (says so first)
  import-export <dir>       reverse an export

-dir defaults to $HLOG_DIR, then the current directory.
$HLOG_NOW (RFC 3339) overrides the clock.`)
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func fail(err error) {
	fmt.Fprintf(os.Stderr, "hlog: %s\n", err)
	code := 1
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "too new"):
		code = 3
	case strings.HasPrefix(msg, "compromised"):
		code = 4
	}
	os.Exit(code)
}

func main() {
	fs := flag.NewFlagSet("hlog", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", "", "log folder")
	if err := fs.Parse(os.Args[1:]); err != nil {
		usage()
		os.Exit(2)
	}
	if *dir == "" {
		*dir = os.Getenv("HLOG_DIR")
	}
	if *dir == "" {
		*dir = "."
	}
	args := fs.Args()
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}
	s := core.Open(*dir)
	if now := os.Getenv("HLOG_NOW"); now != "" {
		t, err := time.Parse(time.RFC3339, now)
		if err != nil {
			fmt.Fprintln(os.Stderr, "hlog: bad HLOG_NOW")
			os.Exit(2)
		}
		s.Now = func() time.Time { return t }
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "init":
		if len(rest) != 0 {
			usage()
			os.Exit(2)
		}
		id, words, err := s.Init()
		if err != nil {
			fail(err)
		}
		fmt.Println("identity", id)
		for i, w := range words {
			fmt.Printf("%2d. %s\n", i+1, w)
		}
		fmt.Fprintln(os.Stderr, "write these 24 words on paper. use them for hlog only. they are shown once.")
	case "append":
		res, err := s.Append(os.Stdin)
		fmt.Printf("added %d, skipped %d\n", res.Added, res.Skipped)
		if err != nil {
			fail(err)
		}
	case "blob":
		if len(rest) != 2 {
			usage()
			os.Exit(2)
		}
		switch rest[0] {
		case "put":
			data, err := os.ReadFile(rest[1])
			if err != nil {
				fail(err)
			}
			id, err := s.PutBlob(data)
			if err != nil {
				fail(err)
			}
			fmt.Println(id)
		case "get":
			pt, err := s.GetBlob(rest[1])
			if err != nil {
				fail(err)
			}
			os.Stdout.Write(pt)
		default:
			usage()
			os.Exit(2)
		}
	case "verify":
		lines, code, err := s.VerifyLines()
		if err != nil {
			fail(err)
		}
		for _, l := range lines {
			fmt.Println(l)
		}
		os.Exit(code)
	case "reindex":
		n, err := s.Reindex()
		if err != nil {
			fail(err)
		}
		fmt.Printf("indexed %d records\n", n)
	case "purge":
		pf := flag.NewFlagSet("purge", flag.ContinueOnError)
		pf.SetOutput(io.Discard)
		reason := pf.String("reason", "", "reason")
		if err := pf.Parse(rest); err != nil || pf.NArg() != 1 {
			usage()
			os.Exit(2)
		}
		tid, err := s.Purge(pf.Arg(0), *reason)
		if err != nil {
			fail(err)
		}
		fmt.Println("purged, tombstone", tid)
	case "gc":
		r, b, err := s.GC()
		if err != nil {
			fail(err)
		}
		fmt.Printf("erased %d records, %d blobs\n", r, b)
	case "rotate":
		rf := flag.NewFlagSet("rotate", flag.ContinueOnError)
		rf.SetOutput(io.Discard)
		var noSeal multiFlag
		rf.Var(&noSeal, "no-seal", "leave this destructive record unsealed (repeatable)")
		if err := rf.Parse(rest); err != nil || rf.NArg() != 0 {
			usage()
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "sync all devices before you rotate. paste the 24 words, then end input:")
		text, err := io.ReadAll(os.Stdin)
		if err != nil {
			fail(err)
		}
		res, err := s.Rotate(core.ParseWords(string(text)), noSeal)
		if err != nil {
			fail(err)
		}
		for _, id := range res.Sealed {
			fmt.Println("sealed", id)
		}
		for _, id := range res.NotSealed {
			fmt.Println("not sealed", id)
		}
		fmt.Println("rotated to", res.NewKey)
		fmt.Printf("sealed %d\n", len(res.Sealed))
	case "sync":
		if len(rest) != 1 {
			usage()
			os.Exit(2)
		}
		src := core.Open(rest[0])
		src.Now = s.Now
		res, err := s.Sync(src)
		if err != nil {
			fail(err)
		}
		fmt.Printf("copied %d records, %d blobs, refused %d purged\n", res.Records, res.Blobs, res.Refused)
		if res.Replaced > 0 {
			fmt.Printf("replaced %d bad copies\n", res.Replaced)
		}
	case "export":
		if len(rest) != 1 {
			usage()
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "hlog: the export is not encrypted")
		r, b, err := s.Export(rest[0])
		if err != nil {
			fail(err)
		}
		fmt.Printf("exported %d records, %d blobs (plaintext)\n", r, b)
	case "import-export":
		if len(rest) != 1 {
			usage()
			os.Exit(2)
		}
		r, b, err := s.ImportExport(rest[0])
		if err != nil {
			fail(err)
		}
		fmt.Printf("added %d records, %d blobs\n", r, b)
	default:
		usage()
		os.Exit(2)
	}
	_ = errors.New
}
