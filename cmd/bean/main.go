// Command bean is the reference CLI (SPEC.md section 12).
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
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
  append [-agent | -corrections]
                                Add the records that an importer writes to stdin.
  import <source> [-yes] [-corrections] <file>
                                Keep a provider file, then add its readings. It
                                shows them and asks first; -yes skips that.
  blob put <file>               Store a file, encrypted. Prints its id.

Read
  ls [flags]                    List current records in time order. Flags: -type,
                                -since, -until, -author, -all, -json.
  show <id>                     Show one record, its status, files and replies.
  blob get <id>                 Write a stored file to stdout.
  verify                        Check every file and signature.

Compare
  compare -sex <s> -age <from>-<to> [-where <rule>]... [-unknown-files] [-save] <id>... | -type <type>
                                Show where a reading lies among people like the
                                ones you describe, in public survey data (NHANES).
  compare -pack <file> [-cohort <cohort> [-unknown-pack] [-save] <id>... | -type <type>]
                                The same with a reference pack: one file of
                                public reference data. Without -cohort, show
                                what the pack holds.

Share
  share -out <folder or file> [-seal | -for <address> [-address-words "<eight words>"]] [-files] [-note <text>] [-yes] <id>... | -type <type> [-since <time>] [-until <time>]
                                Write chosen records into a folder that anyone
                                can check without your log or keys. With -seal:
                                one file that only its reader can open, with
                                six words that you give them. With -for: one
                                file that is sealed to the reader's address.
  address -out <new file> | -key <file>
                                Make a key file and its address, for packages
                                that are sealed to you. Or show the address
                                of a key file. It needs no log.
  check-bundle [-key <file>] [-unpack <new folder>] <folder or file>
                                Check a bundle or a sealed package someone
                                sent you. -key opens a package that is sealed
                                to your address. -unpack writes a package
                                that passes into a folder, with its files.
  reply -to <what you received> [-key <file>] [-for <address>] -out <new file> <id> <note> [<id> <note>]...
                                Sign notes about records that someone shared
                                with you, and seal them for that person:
                                with six words, or with -for to an address
                                that the person gave you.
  accept [-label <name>] [-words "<eight words>"] [-yes | -key <file>] <reply>
                                Check a reply to a share of yours, see who
                                signed it, and take its notes into your log.
                                -key opens a reply that is sealed to your
                                address, and asks on a terminal.
  unpair <name>                 Take your word back from the keys that you
                                paired under a name: a peer, or an agent.

AI and agents
  mcp [-write] [-blobs] [-label <name>]
                                Run the MCP server for an AI client. Read-only
                                unless -write; stored files only with -blobs.
  agent init [-label <name>] [-identity <id>]
                                Make the agent key of this machine.
  agent pair <npub> [-label <name>]
                                Trust an agent key. Run it on your own machine.

Copies and keys
  sync <folder>                 Copy the records and files that another copy has.
  export <folder>               Write a plaintext copy of the log.
  import-export <folder>        Read an export into a log of the same identity.
  rotate [-seal-all] [-no-seal <id>]... [-no-seal-file <file>] [-list] [-nostr new|<file>]
                                Change your signing key. Reads the 24 words.
                                Shows what the old key signed: you choose.
  restore [flags]               Rebuild your log on a new machine from the
                                24 words and a backup. Same flags as rotate.
                                -export <folder> rebuilds it from an export.
  migrate <empty folder>        Start a new identity from a frozen log, with your
                                own records. Shows 24 new words once.

Delete
  purge [-reason <text>] <id>   Delete a record from every copy.
  gc                            Erase deleted records after the 7-day hold.

Other
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
	"append":        "append [-agent | -corrections] < records.jsonl",
	"import":        "import <source> [importer flags] [-yes] [-corrections] <file>",
	"blob":          "blob put <file> | blob get <id>",
	"ls":            "ls [-type <t>] [-since <time>] [-until <time>] [-author me|agent|<id>|<npub>] [-all] [-json]",
	"show":          "show <id>",
	"verify":        "verify",
	"share":         "share -out <new folder or file> [-seal | -for <address> [-address-words \"<eight words>\"]] [-files] [-note <text>] [-yes] <id>... | -type <type> [-since <time>] [-until <time>]",
	"address":       "address -out <new key file> | address -key <key file>",
	"check-bundle":  "check-bundle [-key <key file>] [-unpack <new folder>] <folder, zip file or sealed package>",
	"reply":         "reply -to <what you received> [-key <key file>] [-for <address> [-address-words \"<eight words>\"] [-yes]] -out <new file> <id> <note> [<id> <note>]...",
	"accept":        "accept [-label <name>] [-words \"<eight words>\"] [-yes | -key <key file>] <reply>",
	"unpair":        "unpair <name>",
	"compare":       "compare -sex male|female -age <from>-<to> [-where <DATASET>.<VARIABLE>=<value>|<from>-<to>]... [-unknown-files] [-save] <id>... | -type <type>\n       bean [-dir <folder>] compare -pack <file> [-cohort <cohort> [-unknown-pack] [-save] <id>... | -type <type>]",
	"mcp":           "mcp [-write] [-blobs] [-label <name>]",
	"agent":         "agent init [-label <name>] [-identity <id>] | agent pair <npub> [-label <name>]",
	"sync":          "sync <source folder>",
	"export":        "export <empty folder>",
	"import-export": "import-export <export folder>",
	"rotate":        "rotate [-list] [-seal-all] [-no-seal <id>]... [-no-seal-file <file>] [-nostr new|<nsec file>] < words.txt",
	"restore":       "restore [-list] [-seal-all] [-no-seal <id>]... [-no-seal-file <file>] [-nostr new|<nsec file>] [-identity <id>] [-export <folder>] < words.txt",
	"migrate":       "migrate [-also <id>]... [-also-file <file>] [-nostr new|<nsec file>] <empty folder> | migrate -list",
	"purge":         "purge [-reason <text>] <id>",
	"gc":            "gc",
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

// sweepOnExit is the log of a command that can correct or delete a reading,
// or bring such a change from another copy. Such a command deletes the saved
// comparisons of that reading before it exits, also when it fails part way:
// what it wrote before the failure stays written (compare.go).
var sweepOnExit *core.Store

func fail(err error) {
	if s := sweepOnExit; s != nil {
		sweepOnExit = nil
		sweepComparisons(s)
	}
	fmt.Fprintf(os.Stderr, "bean: %s\n", safe(err.Error()))
	if errors.Is(err, core.ErrNotInitialized) {
		fmt.Fprintf(os.Stderr, "bean: no log in %s: point -dir or BEAN_DIR at your log, run bean init to start a new one, or run bean restore with your 24 words to rebuild yours\n", logDir)
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
	switch cmd {
	case "purge", "append", "import", "sync", "import-export", "verify":
		sweepOnExit = s
	}
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
		fmt.Printf("bean %s (records v 0)\n", core.Version)
		if from := builtFrom(); from != "" {
			fmt.Println(from)
		}
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
		fmt.Fprintln(os.Stderr, "Write these 24 words on paper now. bean shows them only once. They recover your identity on a new machine. Use them only for bean: never type them into a crypto wallet, a chat or an AI assistant.")
		if abs, err := filepath.Abs(s.Dir); err == nil {
			fmt.Fprintln(os.Stderr, "Your log is in "+abs+". Keep it on an encrypted disk.")
		}
	case "append":
		af := flag.NewFlagSet("append", flag.ContinueOnError)
		af.SetOutput(io.Discard)
		agent := af.Bool("agent", false, "sign with this machine's agent key")
		corrections := af.Bool("corrections", false, "write new values for readings you already have, as corrections")
		if err := af.Parse(rest); err != nil || af.NArg() != 0 || (*agent && *corrections) {
			usage()
		}
		appendFn := s.Append
		if *agent {
			appendFn = s.AppendAsAgent
		}
		if *corrections {
			appendFn = s.AppendCorrections
		}
		res, err := appendFn(os.Stdin)
		if res.Corrected > 0 {
			fmt.Printf("added %d, skipped %d, %d of them new values for readings you already had\n", res.Added, res.Skipped, res.Corrected)
		} else {
			fmt.Printf("added %d, skipped %d\n", res.Added, res.Skipped)
		}
		if err != nil {
			fail(err)
		}
		if err := reportChanged(res.Changed, "run append -corrections"); err != nil {
			fail(err)
		}
	case "blob":
		if len(rest) != 2 {
			usage()
		}
		switch rest[0] {
		case "put":
			data, err := readBlobFile(rest[1])
			if err != nil {
				fail(err)
			}
			id, err := s.PutBlob(data)
			if err != nil {
				fail(err)
			}
			fmt.Println(id)
		case "get":
			id, err := s.ResolveBlob(rest[1])
			if err != nil {
				fail(err)
			}
			pt, err := s.GetBlob(id)
			if err != nil {
				fail(err)
			}
			os.Stdout.Write(pt)
		default:
			usage()
		}
	case "ls":
		cmdList(s, rest)
	case "compare":
		cmdCompare(s, rest)
	case "share":
		cmdShare(s, rest)
	case "address":
		cmdAddress(s, rest)
	case "check-bundle":
		cmdCheckBundle(rest)
	case "reply":
		cmdReply(s, rest)
	case "accept":
		cmdAccept(s, rest)
	case "unpair":
		cmdUnpair(s, rest)
	case "show":
		cmdShow(s, rest)
	case "add":
		cmdAdd(s, rest)
	case "import":
		cmdImport(s, rest)
	case "mcp":
		cmdMCP(s, rest)
	case "whoami":
		cmdWhoami(s, rest)
	case "agent":
		cmdAgent(s, rest)
	case "verify":
		if len(rest) != 0 {
			usage() // a flag that is ignored reads as a flag that was obeyed
		}
		lines, code, err := s.VerifyLines()
		if err != nil {
			fail(err)
		}
		for _, l := range lines {
			fmt.Println(safe(l))
		}
		// verify is what the docs say to run after copying a log with
		// another tool: a deletion that came with the copy takes the saved
		// comparisons of its reading with it.
		sweepComparisons(s)
		os.Exit(code)
	case "purge":
		pf := flag.NewFlagSet("purge", flag.ContinueOnError)
		pf.SetOutput(io.Discard)
		reason := pf.String("reason", "", "reason")
		if err := pf.Parse(rest); err != nil || pf.NArg() != 1 {
			usage()
		}
		res, err := s.PurgeAll(pf.Arg(0), *reason)
		if err != nil {
			fail(err)
		}
		fmt.Println("purged, tombstone", res.Tombstone)
		// A file that another record names stays (C25), and is still readable.
		// Names counts only records that purge can reach; Others the deleted
		// records waiting for gc, records about someone else, and damaged files.
		for _, k := range res.Kept {
			if k.Names > 0 {
				rec := "records name"
				if k.Names == 1 {
					rec = "record names"
				}
				fmt.Printf("the file %s is kept: %d other %s it. Purge those too to erase it\n", k.ID, k.Names, rec)
			}
			if k.Others > 0 {
				rec := "files name"
				if k.Others == 1 {
					rec = "file names"
				}
				fmt.Printf("the file %s is kept: %d other %s it that purge cannot reach: deleted and waiting for gc, about someone else, or damaged\n", k.ID, k.Others, rec)
			}
		}
	case "gc":
		if len(rest) != 0 {
			usage() // gc erases for good: "gc -dry-run" must not run
		}
		r, b, err := s.GC()
		if err != nil {
			fail(err)
		}
		fmt.Printf("erased %d records, %d blobs\n", r, b)
	case "rotate":
		cmdRotate(s, rest, false)
	case "restore":
		cmdRotate(s, rest, true)
	case "migrate":
		cmdMigrate(s, rest)
	case "sync":
		if len(rest) != 1 {
			usage()
		}
		if err := ownCopy(s, rest[0]); err != nil {
			fail(err)
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
		if res.BadRecords > 0 || res.BadBlobs > 0 {
			fail(fmt.Errorf("skipped %d bad record files, %d bad blobs in %s; run bean -dir %s verify", res.BadRecords, res.BadBlobs, rest[0], rest[0]))
		}
	case "export":
		if len(rest) != 1 {
			usage()
		}
		fmt.Fprintln(os.Stderr, "note: the export is not encrypted: keep it somewhere private")
		res, err := s.ExportAll(rest[0])
		if res.Written {
			// The export ran to its end; an error after this is about what it
			// could not read, and everything else is in the folder.
			fmt.Printf("exported %d records, %d blobs (plaintext)\n", res.Records, res.Blobs)
			if n := len(res.Unreferenced); n > 0 {
				fmt.Printf("blobs that no record names, such as one from blob put: %d\n", n)
			}
			if n := len(res.Invalid); n > 0 {
				fmt.Fprintf(os.Stderr, "bean: %d record files fail their checks. manifest.json lists them (invalid_records), and %s holds a copy of each as it is. Run bean verify\n", n, filepath.Join(rest[0], "invalid"))
			}
			if n := len(res.NotExported); n > 0 {
				fmt.Fprintf(os.Stderr, "bean: %d stored files were not decrypted. manifest.json lists each one with the reason (not_exported_blobs)\n", n)
			}
			if n := len(res.Withheld); n > 0 {
				fmt.Fprintf(os.Stderr, "bean: %d damaged record files were left out because a deletion names them. manifest.json lists them (withheld_records)\n", n)
			}
			if n := res.MissingOthers; n > 0 {
				fmt.Fprintf(os.Stderr, "bean: %d files that only other people's records name are not in this log (missing_blobs_of_others)\n", n)
			}
		}
		if err != nil {
			fail(err)
		}
		if l, err := s.Load(); err == nil && warnFrozen(l) {
			os.Exit(4)
		}
	case "import-export":
		if len(rest) != 1 {
			usage()
		}
		if !hasLog(s) {
			// bean init would make a new identity, which cannot read the export.
			fmt.Fprintf(os.Stderr, "bean: no log in %s. To rebuild your log from this export, run bean restore -export %s -nostr new in an empty folder, with your 24 words. Do not run bean init: a new identity cannot read this export\n", logDir, rest[0])
			os.Exit(1)
		}
		res, err := s.ImportExportAll(rest[0])
		if err != nil {
			fail(err)
		}
		fmt.Printf("added %d records, %d blobs, refused %d purged\n", res.Records, res.Blobs, res.Refused)
	default:
		usage()
	}
	if sweepOnExit != nil {
		sweepComparisons(sweepOnExit)
	}
}

// ownCopy refuses a sync source that is not a copy of this log. sync copies
// every record of its source into the log, for good, which is right for the
// person's own copies and wrong for anyone else's folder, such as a bundle
// someone sent. A copy says whose it is in local/identity; one with no
// local folder, such as objects/ copied by rsync, must hold the person's
// own identity record, whole. This catches a mistake. It does not stop
// someone who builds a folder to pass it: sync only from your own copies.
func ownCopy(s *core.Store, src string) error {
	me, err := s.Identity()
	if err != nil {
		return nil // no identity here yet: an agent-only copy takes a first sync
	}
	if _, err := os.Stat(filepath.Join(src, "objects")); err != nil {
		return nil // sync itself says that the folder holds no log
	}
	no := fmt.Errorf("%s is not a copy of your log. bean sync copies every record of its source into your log, for good. To read a bundle someone sent you, run bean check-bundle %s", src, src)
	if theirs, err := os.ReadFile(filepath.Join(src, "local", "identity")); err == nil {
		if strings.TrimSpace(string(theirs)) != me {
			return no
		}
		return nil
	}
	data, err := os.ReadFile(filepath.Join(src, filepath.FromSlash(core.PathOf(me))))
	if err != nil || len(data) > core.MaxRecordSize {
		return no
	}
	if fr := core.CheckFile(core.PathOf(me), data); fr.Err != nil || fr.Record == nil || fr.Record.Type != "bean.genesis" {
		return no
	}
	return nil
}

// reportChanged prints one changed: line per reading that has a new value and
// was not written, and returns the error that the command fails with. A
// reading that -corrections cannot write either says why, and the error does
// not send the person to -corrections for it. fix names the step that writes
// the others.
func reportChanged(changed []core.Changed, fix string) error {
	if len(changed) == 0 {
		return nil
	}
	ask, refused := 0, 0
	for _, c := range changed {
		line := fmt.Sprintf("changed: %s %s: the log has %s, this input says %s", c.ID[7:19], safe(c.Type), safe(c.Old), safe(c.New))
		if c.Reason != "" {
			line += " (type " + safe(c.NewType) + "): not written, because " + safe(c.Reason)
			refused++
		} else {
			ask++
		}
		fmt.Fprintln(os.Stderr, line)
	}
	var parts []string
	if ask > 0 {
		parts = append(parts, fmt.Sprintf("%d readings have a new value, and nothing was written for them. Check that this input is newer and right, then %s", ask, fix))
	}
	if refused > 0 {
		parts = append(parts, fmt.Sprintf("%d readings were not written, and -corrections cannot write them either: each changed: line says why", refused))
	}
	return errors.New(strings.Join(parts, ". "))
}

// needsLog lists the commands that only make sense in a folder that holds
// a log; in an empty folder they say so instead of printing nothing.
var needsLog = map[string]bool{
	"whoami": true, "add": true, "append": true, "blob": true, "ls": true, "show": true,
	"verify": true, "mcp": true, "import": true, "export": true, "rotate": true,
	"purge": true, "gc": true, "migrate": true, "compare": true, "share": true,
	"reply": true, "accept": true, "unpair": true,
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

// builtFrom says which source this program was built from, when the Go
// tool recorded it: the commit of a checkout, or the version of a module
// that go install fetched. Two builds of one version are told apart by it.
func builtFrom() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	commit, changed := "", false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			commit = s.Value
		case "vcs.modified":
			changed = s.Value == "true"
		}
	}
	switch {
	case len(commit) >= 12 && changed:
		return "built from commit " + commit[:12] + ", with changes that are not committed"
	case len(commit) >= 12:
		return "built from commit " + commit[:12]
	case info.Main.Version != "" && info.Main.Version != "(devel)":
		return "built from module version " + info.Main.Version
	}
	return ""
}
