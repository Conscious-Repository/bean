package main

import "syscall"

// importerAttr puts the importer in a process group of its own, so a signal
// to bean can stop the whole group. A signal to bean's own group, such as
// SIGKILL from timeout or a job runner, then misses the importer: on Linux
// the kernel kills it when bean dies. Processes the importer started are not
// covered; they die when the importer's group is killed, and only then.
func importerAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}

// importerDiesWithBean reports whether the kernel kills the importer when
// bean dies of a signal it cannot catch.
const importerDiesWithBean = true
