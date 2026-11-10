//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package main

import "os"

// isTerminal reports whether f is a terminal: see isatty_linux.go. Where
// the kernel cannot be asked, nothing is taken for one: pass -yes.
func isTerminal(f *os.File) bool { return false }
