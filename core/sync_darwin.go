package core

import (
	"os"
	"syscall"
)

// syncFile flushes a file to the drive. On macOS, Go's File.Sync issues
// F_FULLFSYNC, which also flushes the drive's own cache and costs about
// 4 ms per file; a 20,000-record import would take minutes. Plain fsync,
// SQLite's default on macOS, costs about 0.02 ms.
func syncFile(f *os.File) error { return syscall.Fsync(int(f.Fd())) }
