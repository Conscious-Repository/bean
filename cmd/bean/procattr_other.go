//go:build !linux

package main

import "syscall"

// importerAttr puts the importer in a process group of its own, so a signal
// to bean can stop the whole group. A signal to bean's own group, such as
// SIGKILL from timeout or a job runner, then misses the importer. macOS and
// the BSDs have no parent-death signal, so the importer runs on until it
// ends by itself (see COMMANDS.md).
func importerAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// importerDiesWithBean reports whether the kernel kills the importer when
// bean dies of a signal it cannot catch.
const importerDiesWithBean = false
