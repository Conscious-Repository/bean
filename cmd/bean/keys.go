package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Conscious-Repository/bean/core"
)

// nostrKey reads the -nostr flag of init, rotate and restore:
//
//	""      a new random Ed25519 key (the default)
//	"new"   a new random secp256k1 key, so the log gets its own npub
//	"-"     an existing nsec typed on standard input (init only)
//	<path>  an existing nsec in a file
func nostrKey(spec string, stdinFree bool) (core.NewKey, error) {
	switch spec {
	case "":
		return core.NewKey{}, nil
	case "new":
		return core.NewKey{Alg: "secp256k1"}, nil
	case "-":
		if !stdinFree {
			return core.NewKey{}, errors.New("-nostr -: standard input carries the 24 words here; put the nsec in a file")
		}
		line, err := readSecretLine("paste the nsec (nsec1... or 64 hex), then press Enter: ")
		if err != nil {
			return core.NewKey{}, err
		}
		sec, err := core.ParseNsec(line)
		return core.NewKey{Alg: "secp256k1", Secret: sec}, err
	}
	data, err := os.ReadFile(spec)
	if err != nil {
		return core.NewKey{}, err
	}
	sec, err := core.ParseNsec(string(data))
	return core.NewKey{Alg: "secp256k1", Secret: sec}, err
}

// readSecretLine prompts on standard error and reads one line from
// standard input, with echo off when standard input is a terminal.
func readSecretLine(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		off := exec.Command("stty", "-echo")
		off.Stdin = os.Stdin
		if off.Run() == nil {
			defer func() {
				on := exec.Command("stty", "echo")
				on.Stdin = os.Stdin
				_ = on.Run()
				fmt.Fprintln(os.Stderr)
			}()
		}
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return line, nil
}

func cmdWhoami(s *core.Store, args []string) {
	if len(args) != 0 {
		usage()
	}
	id, err := s.Identity()
	if err != nil {
		fail(err)
	}
	fmt.Println("identity", id)
	if local, err := s.ReadLocal(); err == nil {
		fmt.Println("key", local.Signer.Public())
		if npub := core.Npub(local.Signer.Public()); npub != "" {
			fmt.Println("npub", npub)
		}
	} else {
		fmt.Println("key none (agent-only copy)")
	}
	if a, err := s.ReadAgent(); err == nil {
		fmt.Println("agent", a.Signer.Public())
		fmt.Println("agent npub", core.Npub(a.Signer.Public()))
	}
}

// cmdRotate is bean rotate and bean restore. Nothing is sealed without the
// person's decision, because a seal cannot be undone: it shows what the old
// key signed before it asks for the 24 words.
func cmdRotate(s *core.Store, args []string, restore bool) {
	fs := flag.NewFlagSet(current, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var noSeal multiFlag
	list := fs.Bool("list", false, "show every record of the old key, then stop")
	all := fs.Bool("seal-all", false, "keep every record of the old key, except those named with -no-seal")
	fs.Var(&noSeal, "no-seal", "do not keep this record (repeatable)")
	noSealFile := fs.String("no-seal-file", "", "a file with one id per line, not to keep")
	nostr := fs.String("nostr", "", "new, or a file holding an nsec")
	ident := fs.String("identity", "", "the identity id to restore when the words match more than one genesis")
	export := fs.String("export", "", "rebuild from this export folder: its records first, its files after the restore")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || (*ident != "" || *export != "") && !restore {
		usage()
	}
	if *export != "" {
		// An empty folder and an export: the records come first, so that the
		// restore can find the identity and show what the old key signed.
		who, n, err := s.SeedFromExport(*export)
		if err != nil {
			fail(err)
		}
		if *ident == "" {
			*ident = who
		}
		fmt.Fprintf(os.Stderr, "copied %d record files from the export\n", n)
	}
	key, err := nostrKey(*nostr, false)
	if err != nil {
		fail(err)
	}
	if *noSealFile != "" {
		b, err := os.ReadFile(*noSealFile)
		if err != nil {
			fail(err)
		}
		noSeal = append(noSeal, strings.Fields(string(b))...)
	}
	c := core.Seals{All: *all && !*list, NoSeal: noSeal}
	ue, l, err := s.Undecided(c, *ident)
	if err != nil && (*list || !errors.Is(err, core.ErrCannotList)) {
		fail(err)
	}
	if ue != nil {
		printUndecided(ue, l, *list)
		if *list {
			return
		}
		fail(ue)
	}
	if *list {
		fmt.Fprintln(os.Stderr, "bean: nothing to decide on")
		return
	}
	if restore {
		fmt.Fprintln(os.Stderr, "sync every copy of the log you still have into this folder first. Type the 24 words, then press Ctrl-D. Afterwards clear the terminal and its scrollback (Cmd-K in Terminal.app):")
	} else {
		fmt.Fprintln(os.Stderr, "sync all devices before you rotate. Type the 24 words, then press Ctrl-D. Afterwards clear the terminal and its scrollback (Cmd-K in Terminal.app):")
	}
	text, err := io.ReadAll(os.Stdin)
	if err != nil {
		fail(err)
	}
	var res core.RotateResult
	if restore {
		var id string
		if res, id, err = s.Restore(core.ParseWords(string(text)), c, key, *ident); err == nil {
			fmt.Println("identity", id)
		}
	} else {
		res, err = s.Rotate(core.ParseWords(string(text)), c, key)
	}
	if err != nil {
		var u *core.UndecidedError
		if errors.As(err, &u) {
			if rl, lerr := s.Load(); lerr == nil {
				printUndecided(u, rl, false)
			}
		}
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
	if *export != "" {
		// The keys are back, so the export's files can be encrypted again.
		ir, err := s.ImportExportAll(*export)
		if err != nil {
			fail(fmt.Errorf("restored, but the files did not come back: %w; run bean import-export %s", err, *export))
		}
		fmt.Printf("imported %d files from the export\n", ir.Blobs)
	}
}

// shownClaims is how many of the newest claims a rotation shows without -list.
const shownClaims = 20

// printUndecided shows what the old key signed, so that the person can tell
// their own records from a thief's. Deletions and corrections come first and
// always in full, because they change other records. -list shows every claim.
func printUndecided(ue *core.UndecidedError, l *core.Loaded, all bool) {
	w := os.Stderr
	fmt.Fprintln(w, "These records were signed by the key you are replacing. If that key was stolen, some may not be yours.")
	fmt.Fprintln(w, "A record you keep stays in effect for good, and you cannot undo a deletion you keep. A record you do not keep stops being valid.")
	targets := func(ids []string) string {
		var out []string
		for _, id := range ids {
			t := l.Report.Records[id]
			if t == nil {
				t = l.Report.Dropped[id]
			}
			if t != nil {
				out = append(out, t.Type+" "+valueText(t))
			} else {
				out = append(out, "A RECORD NOT IN THIS FOLDER, "+id[7:19]+", which counts if it arrives later")
			}
		}
		return strings.Join(out, ", and ")
	}
	// A claim with a lower id than a record that shares its import ref takes
	// that record's place. It looks like any other claim, so it is named.
	replaces := map[string][]string{}
	for dup, primary := range l.Report.Duplicates {
		replaces[primary] = append(replaces[primary], dup)
	}
	var claims []*core.Record
	byType := map[string]int{}
	for _, r := range ue.Records {
		switch {
		case len(replaces[r.ID]) > 0:
			fmt.Fprintf(w, "  %s  %s  REPLACES %s with %s\n", r.ID[7:19], r.Imported, safe(targets(replaces[r.ID])), safe(valueText(r)))
		case r.Type == "bean.tombstone":
			fmt.Fprintf(w, "  %s  %s  DELETES %s\n", r.ID[7:19], r.Imported, safe(targets(r.Targets("purges"))))
		case r.IsDestructive():
			fmt.Fprintf(w, "  %s  %s  CORRECTS %s to %s\n", r.ID[7:19], r.Imported, safe(targets(r.Targets("supersedes"))), safe(valueText(r)))
		case r.Type == "agent.pair":
			fmt.Fprintf(w, "  %s  %s  TRUSTS a key: %s\n", r.ID[7:19], r.Imported, safe(valueText(r)))
		default:
			claims = append(claims, r)
			byType[r.Type]++
		}
	}
	for _, id := range ue.Halt {
		fmt.Fprintf(w, "  %s  a record in a newer format that stops this log\n", id[7:19])
	}
	shown := claims
	if !all && len(shown) > shownClaims {
		shown = shown[len(shown)-shownClaims:]
		fmt.Fprintf(w, "  %d claims of these types:", len(claims))
		for _, t := range sortedTypes(byType) {
			fmt.Fprintf(w, " %s (%d)", safe(t), byType[t])
		}
		fmt.Fprintf(w, "\n  the %d imported last are below. A thief can fake that time, so run this command again with -list to see all of them.\n", shownClaims)
	}
	for _, r := range shown {
		fmt.Fprintf(w, "  %s  %s  %s %s\n", r.ID[7:19], r.Imported, safe(r.Type), safe(valueText(r)))
	}
	fmt.Fprintf(w, "If the key was not stolen, run this command again with -seal-all. If it may be stolen, add -no-seal <id> for each record that is not yours, or -no-seal-file <file> for many.\n")
}

func sortedTypes(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func cmdMigrate(s *core.Store, args []string) {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	nostr := fs.String("nostr", "", "new, or a file holding an nsec")
	list := fs.Bool("list", false, "show what a migration leaves behind, then stop")
	var also multiFlag
	fs.Var(&also, "also", "also move this claim in doubt, if it is yours (repeatable)")
	alsoFile := fs.String("also-file", "", "a file with one id per line, to also move")
	pos, err := parseInterspersed(fs, args)
	if err != nil || (*list && len(pos) != 0) || (!*list && len(pos) != 1) {
		usage()
	}
	if *list {
		recs, l, err := s.MigrateList()
		if err != nil {
			fail(err)
		}
		for _, r := range recs {
			fmt.Printf("%s  %s  %s %s  %s\n", r.ID[7:19], r.Imported, safe(r.Type), safe(valueText(r)), safe(l.Status(r.ID)))
		}
		fmt.Fprintf(os.Stderr, "%d records stay behind. If a claim in doubt is yours, name it with -also <id> or -also-file <file>. A key is not a machine: a thief may hold it, and moving a record signs it as yours.\n", len(recs))
		return
	}
	if *alsoFile != "" {
		b, err := os.ReadFile(*alsoFile)
		if err != nil {
			fail(err)
		}
		also = append(also, strings.Fields(string(b))...)
	}
	key, err := nostrKey(*nostr, false)
	if err != nil {
		fail(err)
	}
	if abs, err := filepath.Abs(pos[0]); err == nil {
		if top := gitTree(abs); top != "" {
			fail(fmt.Errorf("inside a git work tree: %s; a commit or push would publish the log and its keys. Use a folder outside it", top))
		}
	}
	res, err := s.Migrate(core.Open(pos[0]), key, also...)
	if res.Words != nil {
		fmt.Println("identity", res.Identity)
		for i, w := range res.Words {
			fmt.Printf("%2d. %s\n", i+1, w)
		}
		fmt.Fprintln(os.Stderr, "Write these 24 new words on paper now. bean shows them only once. The old words do not work for the new log.")
	}
	if err != nil {
		fail(err)
	}
	fmt.Printf("migrated %d records, %d blobs, %d blobs missing\n", res.Records, res.Blobs, res.MissingBlobs)
	if res.Disputed > 0 {
		fmt.Printf("%d of them moved with the old value, because only one side of the freeze corrected them: look for context.migrated_disputed_by\n", res.Disputed)
	}
	if res.LeftOut > 0 || res.TooLarge > 0 {
		fmt.Printf("left out %d records by others or in doubt, and %d too large to move. They stay in the old log: bean migrate -list shows them\n", res.LeftOut, res.TooLarge)
	}
}
