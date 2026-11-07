//go:build unix

package core

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// lock takes the log's write lock and returns the function that releases it.
// Commands that write hold it from their first read to their last write. Two
// rotations from one head would fork the identity and freeze it for good, so
// a filesystem without locks is an error, not a reason to go on.
//
// Each call opens the file, so a second lock in one process blocks: a locked
// method must not call another locked method.
func (s *Store) lock() (func(), error) {
	if err := os.MkdirAll(s.path(dirLocal), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(s.path(fileLock), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("cannot lock %s: %w", s.path(fileLock), err)
	}
	return func() { f.Close() }, nil // closing the file releases the lock
}
