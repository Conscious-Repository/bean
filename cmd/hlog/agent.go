package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"bean/core"
)

// cmdAgent is hlog agent init and hlog agent pair.
func cmdAgent(s *core.Store, args []string) {
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	label := fs.String("label", "", "name for the agent, for example hermes")
	identity := fs.String("identity", "", "identity id, when the folder holds more than one genesis")
	pos, err := parseInterspersed(fs, args[1:])
	if err != nil {
		usage()
		os.Exit(2)
	}
	switch args[0] {
	case "init":
		if len(pos) != 0 {
			usage()
			os.Exit(2)
		}
		pub, pairID, err := s.AgentInit(*identity, *label)
		if err != nil {
			fail(err)
		}
		fmt.Println("agent", pub)
		fmt.Println("npub", core.Npub(pub))
		if pairID != "" {
			fmt.Println("paired", pairID)
		} else {
			fmt.Fprintf(os.Stderr, "not paired: this replica has no personal key. on your own machine run:\n  hlog agent pair %s -label %q\n", core.Npub(pub), *label)
		}
	case "pair":
		if len(pos) != 1 {
			usage()
			os.Exit(2)
		}
		key, err := core.KeyFromNpub(pos[0])
		if err != nil {
			fail(err)
		}
		id, err := s.Pair(key, *label)
		if err != nil {
			fail(err)
		}
		fmt.Println("paired", id)
	default:
		usage()
		os.Exit(2)
	}
}
