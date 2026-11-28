package config

import "sync/atomic"

// Current is what a Store holds at one moment: the snapshot of the last document it took, which
// holds the text of every variable that the document set, and how many documents the store has
// taken.
type Current struct {
	Snapshot *Snapshot
	// Generation is how many documents were applied; 0 when none was.
	Generation uint64
}

// Store holds the configuration of the world outside the game: the document that a file, a service
// or a person gave, which a cluster brings into its coordinator engine (see engine.Cluster). It
// is not what the engines read: they read the state of the coordinator engine, through their
// stable view, which the event log records. It is safe for any goroutine.
type Store struct {
	cur     atomic.Pointer[Current]
	changed chan struct{}
}

// NewStore returns a store that holds no document: its snapshot is the defaults and its generation
// is 0.
func NewStore() *Store {
	s := &Store{changed: make(chan struct{}, 1)}
	s.cur.Store(&Current{Snapshot: Defaults()})
	return s
}

// Apply takes the document as the whole configuration: a variable that is not named goes back to
// its default. When a value is wrong it returns the error and changes nothing. One goroutine calls
// Apply at a time.
func (s *Store) Apply(document map[string]string) error {
	snap, err := Build(document)
	if err != nil {
		return err
	}
	s.cur.Store(&Current{Snapshot: snap, Generation: s.cur.Load().Generation + 1})
	select {
	case s.changed <- struct{}{}:
	default: // one signal is waiting already, and it reads the latest
	}
	return nil
}

// Current returns what the store holds now. A nil store holds the defaults.
func (s *Store) Current() Current {
	if s == nil {
		return Current{Snapshot: Defaults()}
	}
	return *s.cur.Load()
}

// Snapshot returns the configuration that the store holds now.
func (s *Store) Snapshot() ReadOnlySnapshot { return s.Current().Snapshot }

// Changed is signalled after an Apply: a receiver that wakes reads Current, which is at least as
// new as the signal. A nil store never signals.
func (s *Store) Changed() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.changed
}
