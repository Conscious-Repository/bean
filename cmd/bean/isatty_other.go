//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package main

import "os"

// isTerminal reports whether f is a terminal: see isatty_linux.go. Where
// the kernel cannot be asked, a character device that is not the null
// device is taken for one.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	null, err := os.Stat(os.DevNull)
	return err != nil || !os.SameFile(fi, null)
}
