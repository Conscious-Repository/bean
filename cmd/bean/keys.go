package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"

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

// cmdRotate is bean rotate and bean restore. It lists any record that needs a
// decision before it asks for the 24 words, and writes nothing until each one
// has a decision.
func cmdRotate(s *core.Store, args []string, restore bool) {
	fs := flag.NewFlagSet(current, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var seal, noSeal multiFlag
	all := fs.Bool("seal-all", false, "keep every record of the outgoing key")
	none := fs.Bool("seal-none", false, "keep only the records named with -seal")
	fs.Var(&seal, "seal", "keep this record (repeatable)")
	fs.Var(&noSeal, "no-seal", "do not keep this record (repeatable)")
	nostr := fs.String("nostr", "", "new, or a file holding an nsec")
	identity := fs.String("identity", "", "restore: the identity id, when the words fit more than one genesis")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || (*identity != "" && !restore) {
		usage()
	}
	key, err := nostrKey(*nostr, false)
	if err != nil {
		fail(err)
	}
	c := core.Seals{All: *all, None: *none, Seal: seal, NoSeal: noSeal}
	undecided, l, err := s.Undecided(c)
	if err != nil {
		fail(err)
	}
	if len(undecided) > 0 {
		printUndecided(undecided, l)
		fail(&core.UndecidedError{Records: undecided})
	}
	if restore {
		fmt.Fprintln(os.Stderr, "sync every copy of the log you still have into this folder first. paste the 24 words, then end input:")
	} else {
		fmt.Fprintln(os.Stderr, "sync all devices before you rotate. paste the 24 words, then end input:")
	}
	text, err := io.ReadAll(os.Stdin)
	if err != nil {
		fail(err)
	}
	var res core.RotateResult
	if restore {
		var id string
		if res, id, err = s.Restore(core.ParseWords(string(text)), c, key, *identity); err == nil {
			fmt.Println("identity", id)
		}
	} else {
		res, err = s.Rotate(core.ParseWords(string(text)), c, key)
	}
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
}

// printUndecided shows what each record of the outgoing key does, so the
// person can tell their own from a thief's.
func printUndecided(recs []*core.Record, l *core.Loaded) {
	fmt.Fprintln(os.Stderr, "These records were signed by the key you are replacing. If the key was stolen, some may not be yours.")
	fmt.Fprintln(os.Stderr, "A record you keep stays in effect for good. A correction or deletion you do not keep is undone.")
	for _, r := range recs {
		what := "claim " + r.Type + " " + valueText(r)
		target := func(id string) string {
			if t := l.Report.Records[id]; t != nil {
				return t.Type + " " + valueText(t)
			}
			return id
		}
		switch {
		case r.Type == "bean.tombstone":
			what = "deletes " + target(r.Targets("purges")[0])
		case r.IsCheckpoint():
			what = fmt.Sprintf("checkpoint over %d records", len(r.Covers()))
		case r.IsDestructive():
			what = "corrects " + target(r.Targets("supersedes")[0]) + " to " + valueText(r)
		}
		fmt.Fprintf(os.Stderr, "  %s  %s  %s\n", r.ID[7:19], r.Imported, what)
	}
	fmt.Fprintln(os.Stderr, "Run again with -seal-all if the key was not stolen. Otherwise use -seal-none, with -seal <id> for each record that is yours.")
}

func cmdCheckpoint(s *core.Store, args []string) {
	fs := flag.NewFlagSet("checkpoint", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var accept multiFlag
	fs.Var(&accept, "accept", "vouch for an unattested claim (repeatable)")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		usage()
	}
	ids, err := s.Checkpoint(accept)
	if err != nil {
		fail(err)
	}
	for _, id := range ids {
		fmt.Println("checkpoint", id)
	}
	if len(ids) == 0 {
		fmt.Println("nothing new to cover")
	}
}

func cmdMigrate(s *core.Store, args []string) {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	nostr := fs.String("nostr", "", "new, or a file holding an nsec")
	pos, err := parseInterspersed(fs, args)
	if err != nil || len(pos) != 1 {
		usage()
	}
	key, err := nostrKey(*nostr, false)
	if err != nil {
		fail(err)
	}
	res, err := s.Migrate(core.Open(pos[0]), key)
	if res.Words == nil && err != nil {
		fail(err)
	}
	fmt.Println("identity", res.Identity)
	for i, w := range res.Words {
		fmt.Printf("%2d. %s\n", i+1, w)
	}
	fmt.Fprintln(os.Stderr, "Write these 24 new words on paper now. bean shows them only once. The old words do not work for the new log.")
	if err != nil {
		fail(err)
	}
	fmt.Printf("migrated %d records, %d blobs, %d blobs missing\n", res.Records, res.Blobs, res.MissingBlobs)
	if res.Unattested > 0 || res.Skipped > 0 {
		fmt.Printf("left out %d unattested claims and %d records too large to move: they stay in the old log\n", res.Unattested, res.Skipped)
	}
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
