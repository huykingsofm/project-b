package config

import (
	"reflect"
	"sync"
)

// ReadOnlySnapshot is what a reader of the configuration is given: the version, and the value of
// every variable through Var.Get. Only a *Snapshot is one, so no one outside this package can make
// a configuration that a variable would read, and no one can change one.
type ReadOnlySnapshot interface {
	// Version identifies the snapshot.
	Version() uint64
	// lookup returns the value of the variable at the index, and false when there is none.
	lookup(idx int) (any, bool)
}

// Snapshot is the value of every variable at one moment. It never changes after it is built, so
// any number of goroutines can read it, and a tick that reads one snapshot at its start sees one
// version of the configuration.
type Snapshot struct {
	version uint64
	vals    []any
}

var _ ReadOnlySnapshot = (*Snapshot)(nil)

// Version identifies the snapshot. Values derived from a snapshot are cached with the version
// they were computed from and recomputed when it changes.
func (s *Snapshot) Version() uint64 { return s.version }

func (s *Snapshot) lookup(idx int) (any, bool) {
	if s == nil || idx >= len(s.vals) {
		return nil, false
	}
	return s.vals[idx], true
}

// Diff returns the names of the variables whose value in s is not the value in other, in the
// order they were declared. A nil other, or one that is not a snapshot of this
// package, has every variable at its default.
func (s *Snapshot) Diff(other ReadOnlySnapshot) []string {
	var changed []string
	for i, d := range std.declarations() {
		mine, _ := s.lookup(i)
		if mine == nil {
			mine = d.def
		}
		theirs, ok := any(nil), false
		if other != nil {
			theirs, ok = other.lookup(i)
		}
		if !ok {
			theirs = d.def
		}
		if !reflect.DeepEqual(mine, theirs) {
			changed = append(changed, d.name)
		}
	}
	return changed
}

var (
	defaultsOnce     sync.Once
	defaultsSnapshot *Snapshot
)

// Defaults returns the snapshot with every variable at its default, the same one each time. The
// first call, like Build, ends the declaration of variables.
func Defaults() *Snapshot {
	defaultsOnce.Do(func() { defaultsSnapshot, _, _ = std.build(0, nil) })
	return defaultsSnapshot
}

// Build returns the snapshot of the given version with the overrides applied to the defaults.
func Build(
	version uint64, overrides map[string]string,
) (snap *Snapshot, unknown []string, err error) {
	return std.build(version, overrides)
}
