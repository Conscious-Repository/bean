package main

import (
	"bufio"
	"errors"
	"fmt"
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
