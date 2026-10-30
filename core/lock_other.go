//go:build !unix

package core

import "os"

// No flock on this system: only the in-process mutex protects the folder.
func flockExclusive(f *os.File) error { return nil }

func flockRelease(f *os.File) {}
