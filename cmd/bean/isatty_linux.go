package main

import (
	"os"
	"syscall"
	"unsafe"
)

// isTerminal reports whether f is a terminal: a person who can read what is
// shown and answer, not a script or an AI assistant's tool. It asks the
// kernel. Being a character device is not enough: /dev/null is one too, and
// a tool that runs bean with its input closed must not count as a person.
func isTerminal(f *os.File) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
