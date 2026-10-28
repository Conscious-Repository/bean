//go:build !darwin

package core

import "os"

// syncFile flushes a file to disk.
func syncFile(f *os.File) error { return f.Sync() }
