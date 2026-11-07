//go:build !unix

package core

// lock does nothing on systems without flock, such as Windows, which bean
// does not support yet. Do not run two bean commands at once there.
func (s *Store) lock() (func(), error) { return func() {}, nil }
