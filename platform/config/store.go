package config

import (
	"maps"
	"sync/atomic"
)

// Current is what a Store holds at one moment: the document that the configuration was made from,
// the snapshot of it, and how many documents the store has taken.
type Current struct {
	// Document is the text of the JSON value by variable name that the snapshot was built from. It
	// is a copy: the caller may keep it.
	Document map[string]string
	// Snapshot is the configuration that the document makes.
	Snapshot *Snapshot
	// Generation is how many documents were applied; 0 when none was.
	Generation uint64
}

// Store holds the configuration of the world outside the game: the document that a file, a service
// or a person gave, which a cluster brings into its configuration engine (see engine.Cluster). It
// is not what the engines read: they read the state of the configuration engine, through their
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
	s.cur.Store(&Current{
		Document: maps.Clone(document), Snapshot: snap, Generation: s.cur.Load().Generation + 1,
	})
	select {
	case s.changed <- struct{}{}:
	default: // one signal is waiting already, and it reads the latest
	}
	return nil
}

// Current returns what the store holds now. The document is a copy. A nil store holds the defaults.
func (s *Store) Current() Current {
	if s == nil {
		return Current{Snapshot: Defaults()}
	}
	c := *s.cur.Load()
	c.Document = maps.Clone(c.Document)
	return c
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
