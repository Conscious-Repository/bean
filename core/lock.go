package core

import (
	"errors"
	"os"
	"syscall"
)

// The folder lock (DECISIONS 106). Every public operation that reads the
// log and then writes to it holds the lock from its first read to its last
// write, so two bean processes (two terminals, a CLI beside an MCP server)
// cannot both act on the same chain head. Two rotations from one head
// would fork the identity and freeze it forever (SPEC.md 5.7).
//
// The lock is an exclusive flock on local/lock plus the in-process mutex
// of the Store. The kernel releases a flock when its process dies, so a
// crash never leaves a stale lock.

const fileLock = "local/lock"

// lock takes the folder lock. The caller runs the returned function to
// release it. Locks do not nest: an operation that holds the lock calls
// only the unexported helpers.
func (s *Store) lock() (func(), error) {
	s.mu.Lock()
	if err := mkdirAllSync(s.path(dirLocal)); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	f, err := os.OpenFile(s.path(fileLock), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	if err := flockExclusive(f); err != nil {
		f.Close()
		s.mu.Unlock()
		return nil, err
	}
	return func() {
		flockRelease(f)
		f.Close()
		s.mu.Unlock()
	}, nil
}

// readOnlyFolder reports errors that mean the folder cannot be written,
// for example a log on read-only media. A read can go on without the lock
// there, because no bean process can write either.
func readOnlyFolder(err error) bool {
	return errors.Is(err, syscall.EROFS) || errors.Is(err, os.ErrPermission)
}
