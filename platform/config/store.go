package config

import "sync/atomic"

// Store holds the current snapshot, which readers take without a lock.
type Store struct{ cur atomic.Pointer[Snapshot] }

// NewStore returns a store that holds the defaults.
func NewStore() *Store {
	s := &Store{}
	s.cur.Store(Defaults())
	return s
}

// Snapshot returns the current snapshot.
func (s *Store) Snapshot() *Snapshot { return s.cur.Load() }

// Apply makes the next version out of the defaults and the overrides, which are the whole
// document: a variable that is not named goes back to its default. If any override is wrong
// nothing changes. It returns the names that are not declared. One goroutine calls Apply at a
// time.
func (s *Store) Apply(overrides map[string]string) (unknown []string, err error) {
	next, unknown, err := Build(s.cur.Load().Version()+1, overrides)
	if err != nil {
		return unknown, err
	}
	s.cur.Store(next)
	return unknown, nil
}
