// Command bean is the reference CLI (SPEC.md section 12).
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Conscious-Repository/bean/core"
)

const help = `bean keeps your health record as a folder of signed records.

usage: bean [-dir <folder>] <command> [arguments]

Start
  init [-nostr new|-|<file>]    Create your identity and log. Shows 24 words once.
  whoami                        Show your identity, signing key, npub and agent key.

Write
  add <type> <value> [flags]    Add one record. Flags: -unit, -at, -note, -about,
                                -corrects, -photo, -agent.
  append [-agent]               Add the records that an importer writes to stdin.
  blob put <file>               Store a file, encrypted. Prints its id.

Read
  ls [flags]                    List current records in time order. Flags: -type,
                                -since, -until, -author, -all, -json.
  show <id>                     Show one record, its status, files and replies.
  blob get <id>                 Write a stored file to stdout.
  verify                        Check every file and signature.

AI and agents
  mcp [-write] [-label <name>]  Run the MCP server for an AI client.
  agent init [-label <name>]    Make the agent key of this machine.
  agent pair <npub> [-label <name>]
                                Trust an agent key. Run it on your own machine.

Copies and keys
  sync <folder>                 Copy the records and files that another copy has.
  export <folder>               Write a plaintext copy of the log.
  import-export <folder>        Read an export into a log of the same identity.
  rotate [-nostr new|<file>]    Change your signing key. Reads the 24 words.
  restore [-nostr new|<file>]   Rebuild your log on a new machine from the
                                24 words and a backup.

Delete
  purge [-reason <text>] <id>   Delete a record from every copy.
  gc                            Erase deleted records after the 7-day hold.

Other
  reindex                       Rebuild local/cache/index.json.
  help                          Show this help.
  version                       Show the version.

-dir defaults to $BEAN_DIR, then the current folder.
An id can be shortened to its first 6 or more hex digits.
Full reference: docs/COMMANDS.md`

// syntax is the usage line of each command, shown on a usage error.
var syntax = map[string]string{
	"init":          "init [-nostr new|-|<nsec file>] [-force]",
	"whoami":        "whoami",
	"add":           "add <type> <value> [-unit <u>] [-at <time>] [-note <text>] [-about <id>] [-corrects <id>] [-photo <file>] [-agent]",
	"append":        "append [-agent] < records.jsonl",
	"blob":          "blob put <file> | blob get <id>",
	"ls":            "ls [-type <t>] [-since <time>] [-until <time>] [-author me|agent|<id>|<npub>] [-all] [-json]",
	"show":          "show <id>",
	"verify":        "verify",
	"mcp":           "mcp [-write] [-label <name>]",
	"agent":         "agent init [-label <name>] [-identity <id>] | agent pair <npub> [-label <name>]",
	"sync":          "sync <source folder>",
	"export":        "export <empty folder>",
	"import-export": "import-export <export folder>",
	"rotate":        "rotate [-no-seal <id>]... [-nostr new|<nsec file>] < words.txt",
	"restore":       "restore [-no-seal <id>]... [-nostr new|<nsec file>] [-identity <id>] < words.txt",
	"purge":         "purge [-reason <text>] <id>",
	"gc":            "gc",
	"reindex":       "reindex",
	"help":          "help",
	"version":       "version",
}

// current is the command being run, for usage errors.
var current string

// usage reports a usage error for the current command and exits 2.
func usage() {
	if line, ok := syntax[current]; ok {
		fmt.Fprintf(os.Stderr, "usage: bean [-dir <folder>] %s\nrun bean help for all commands\n", line)
	} else {
		fmt.Fprintln(os.Stderr, help)
	}
	os.Exit(2)
}

// logDir is the folder in use, named in errors.
var logDir string

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func fail(err error) {
	fmt.Fprintf(os.Stderr, "bean: %s\n", err)
	if errors.Is(err, core.ErrNotInitialized) {
		fmt.Fprintf(os.Stderr, "bean: no log in %s: run bean init, or point -dir or BEAN_DIR at your log\n", logDir)
	}
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
	fs := flag.NewFlagSet("bean", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", "", "log folder")
	if err := fs.Parse(os.Args[1:]); errors.Is(err, flag.ErrHelp) {
		fmt.Println(help)
		return
	} else if err != nil {
		usage()
	}
	if *dir == "" {
		*dir = os.Getenv("BEAN_DIR")
	}
	if *dir == "" {
		*dir = "."
	}
	args := fs.Args()
	if len(args) == 0 {
		usage()
	}
	if abs, err := filepath.Abs(*dir); err == nil {
		logDir = abs
	} else {
		logDir = *dir
	}
	s := core.Open(*dir)
	if now := os.Getenv("BEAN_NOW"); now != "" {
		t, err := time.Parse(time.RFC3339, now)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bean: bad BEAN_NOW")
			os.Exit(2)
		}
		s.Now = func() time.Time { return t }
	}
	cmd, rest := args[0], args[1:]
	current = cmd
	if _, known := syntax[cmd]; !known {
		fmt.Fprintf(os.Stderr, "bean: unknown command %q\nrun bean help for all commands\n", cmd)
		os.Exit(2)
	}
	if needsLog[cmd] && !hasLog(s) {
		fail(core.ErrNotInitialized)
	}
	switch cmd {
	case "help":
		fmt.Println(help)
	case "version":
		fmt.Printf("bean %s (spec v0.5)\n", core.Version)
	case "init":
		inf := flag.NewFlagSet("init", flag.ContinueOnError)
		inf.SetOutput(io.Discard)
		nostr := inf.String("nostr", "", "new, -, or a file holding an nsec")
		force := inf.Bool("force", false, "init inside a git work tree anyway")
		if err := inf.Parse(rest); err != nil || inf.NArg() != 0 {
			usage()
		}
		if top := gitTree(logDir); top != "" && !*force {
			fail(fmt.Errorf("inside a git work tree: %s; a commit or push would publish the log and its keys. Use a folder outside it, or pass -force", top))
		}
		key, err := nostrKey(*nostr, true)
		if err != nil {
			fail(err)
		}
		id, words, err := s.Init(key)
		if err != nil {
			fail(err)
		}
		fmt.Println("identity", id)
		if local, err := s.ReadLocal(); err == nil && core.Npub(local.Signer.Public()) != "" {
			fmt.Println("npub", core.Npub(local.Signer.Public()))
		}
		for i, w := range words {
			fmt.Printf("%2d. %s\n", i+1, w)
		}
		fmt.Fprintln(os.Stderr, "Write these 24 words on paper now. bean shows them only once. They recover your identity on a new machine. Use them only for bean: never type them into a crypto wallet.")
	case "append":
		af := flag.NewFlagSet("append", flag.ContinueOnError)
		af.SetOutput(io.Discard)
		agent := af.Bool("agent", false, "sign with this machine's agent key")
		if err := af.Parse(rest); err != nil || af.NArg() != 0 {
			usage()
		}
		appendFn := s.Append
		if *agent {
			appendFn = s.AppendAsAgent
		}
		res, err := appendFn(os.Stdin)
		fmt.Printf("added %d, skipped %d\n", res.Added, res.Skipped)
		if err != nil {
			fail(err)
		}
	case "blob":
		if len(rest) != 2 {
			usage()
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
		}
	case "ls":
		cmdList(s, rest)
	case "show":
		cmdShow(s, rest)
	case "add":
		cmdAdd(s, rest)
	case "mcp":
		cmdMCP(s, rest)
	case "whoami":
		cmdWhoami(s, rest)
	case "agent":
		cmdAgent(s, rest)
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
		nostr := rf.String("nostr", "", "new, or a file holding an nsec")
		if err := rf.Parse(rest); err != nil || rf.NArg() != 0 {
			usage()
		}
		key, err := nostrKey(*nostr, false)
		if err != nil {
			fail(err)
		}
		fmt.Fprintln(os.Stderr, "sync all devices before you rotate. paste the 24 words, then end input:")
		text, err := io.ReadAll(os.Stdin)
		if err != nil {
			fail(err)
		}
		res, err := s.Rotate(core.ParseWords(string(text)), noSeal, key)
		if err != nil {
			fail(err)
		}
		for _, id := range res.Sealed {
			fmt.Println("sealed", id)
		}
		for _, id := range res.NotSealed {
			fmt.Println("not sealed", id)
		}
		for _, id := range res.Parts {
			fmt.Println("seal part", id)
		}
		for _, id := range res.Past {
			fmt.Println("past halt", id)
		}
		fmt.Println("rotated to", res.NewKey)
		fmt.Printf("sealed %d\n", len(res.Sealed))
	case "restore":
		rf := flag.NewFlagSet("restore", flag.ContinueOnError)
		rf.SetOutput(io.Discard)
		var noSeal multiFlag
		rf.Var(&noSeal, "no-seal", "leave this destructive record unsealed (repeatable)")
		nostr := rf.String("nostr", "", "new, or a file holding an nsec")
		ident := rf.String("identity", "", "the identity id to restore when the words match more than one genesis")
		if err := rf.Parse(rest); err != nil || rf.NArg() != 0 {
			usage()
		}
		key, err := nostrKey(*nostr, false)
		if err != nil {
			fail(err)
		}
		fmt.Fprintln(os.Stderr, "sync every copy of the log you still have into this folder first. paste the 24 words, then end input:")
		text, err := io.ReadAll(os.Stdin)
		if err != nil {
			fail(err)
		}
		res, id, err := s.RestoreIdentity(core.ParseWords(string(text)), noSeal, key, *ident)
		if err != nil {
			fail(err)
		}
		fmt.Println("identity", id)
		for _, x := range res.Sealed {
			fmt.Println("sealed", x)
		}
		for _, x := range res.NotSealed {
			fmt.Println("not sealed", x)
		}
		for _, x := range res.Parts {
			fmt.Println("seal part", x)
		}
		for _, x := range res.Past {
			fmt.Println("past halt", x)
		}
		fmt.Println("rotated to", res.NewKey)
		fmt.Printf("sealed %d\n", len(res.Sealed))
	case "sync":
		if len(rest) != 1 {
			usage()
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
		}
		fmt.Fprintln(os.Stderr, "bean: the export is not encrypted")
		r, b, err := s.Export(rest[0])
		if err != nil {
			fail(err)
		}
		fmt.Printf("exported %d records, %d blobs (plaintext)\n", r, b)
	case "import-export":
		if len(rest) != 1 {
			usage()
		}
		r, b, err := s.ImportExport(rest[0])
		if err != nil {
			fail(err)
		}
		fmt.Printf("added %d records, %d blobs\n", r, b)
	default:
		usage()
	}
}

// needsLog lists the commands that only make sense in a folder that holds
// a log; in an empty folder they say so instead of printing nothing.
var needsLog = map[string]bool{
	"whoami": true, "add": true, "append": true, "blob": true, "ls": true, "show": true,
	"verify": true, "mcp": true, "export": true, "import-export": true, "rotate": true,
	"purge": true, "gc": true, "reindex": true,
}

// hasLog reports whether the folder holds a log: an identity, or synced
// records (an agent-only copy before agent init).
func hasLog(s *core.Store) bool {
	if _, err := s.Identity(); err == nil {
		return true
	}
	_, err := os.Stat(filepath.Join(s.Dir, "objects"))
	return err == nil
}

// gitTree returns the top of the git work tree that holds dir, or "".
// It looks for a .git entry in dir and each parent; it runs no git.
func gitTree(dir string) string {
	d := dir
	for {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}
