package config

import (
	"fmt"
	"iter"
	"maps"
	"reflect"
	"sync"

	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/stablemap"
)

// ReadOnlySnapshot is what a reader of the configuration is given: the value of every variable
// through Var.Get. Only a *Snapshot is one, so no one outside this package can make
// a configuration that a variable would read, and no one can change one.
type ReadOnlySnapshot interface {
	// lookup returns the value of the variable at the index, and false when there is none.
	lookup(idx int) (any, bool)
}

// Snapshot is the value of every variable at one moment, and the text of the JSON value of each
// variable that was set, which is what it was built from. It never changes after it is built, so
// any number of goroutines can read it, and a tick that reads one snapshot at its start sees one
// configuration.
type Snapshot struct {
	vals  []any
	texts stablemap.Map[string, string] // by name, in the order of the names
}

var _ ReadOnlySnapshot = (*Snapshot)(nil)

func (s *Snapshot) lookup(idx int) (any, bool) {
	if s == nil || idx >= len(s.vals) {
		return nil, false
	}
	return s.vals[idx], true
}

// Texts returns the name of each variable that was set, in the order of the names, with the text
// of its JSON value: what Build makes the same snapshot from.
func (s *Snapshot) Texts() iter.Seq2[string, string] { return s.texts.All() }

// Encode writes the texts of the variables that were set, which is all that Decode needs.
func (s *Snapshot) Encode(w *binary.Writer) {
	w.Uvarint(uint64(s.texts.Len()))
	for name, text := range s.texts.All() {
		w.String(name)
		w.String(text)
	}
}

// DecodeSnapshot reads what Encode wrote, and builds the snapshot of the texts again. It returns an
// error for bytes that are cut, or texts that no snapshot can be made of.
func DecodeSnapshot(r *binary.Reader) (*Snapshot, error) {
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	if n > uint64(r.Left()) { // a variable takes at least two bytes
		return nil, fmt.Errorf("%w: %d variables", binary.ErrShort, n)
	}
	texts := make(map[string]string, n)
	for range n {
		name, text := r.String(), r.String()
		if err := r.Err(); err != nil {
			return nil, err
		}
		texts[name] = text
	}
	return Build(texts)
}

// Apply returns the snapshot that has the variables of set at the text of their value, and the
// variables of unset at their default, and every other variable as s has it. It returns s itself
// when no variable ends with another value than it had, and an error, with no snapshot, when a
// value is wrong.
func (s *Snapshot) Apply(set map[string]string, unset []string) (*Snapshot, error) {
	texts := maps.Collect(s.texts.All())
	maps.Copy(texts, set)
	for _, name := range unset {
		delete(texts, name)
	}
	next, err := Build(texts)
	if err != nil {
		return nil, err
	}
	if len(next.Diff(s)) == 0 {
		return s, nil
	}
	return next, nil
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
	defaultsOnce.Do(func() { defaultsSnapshot, _ = std.build(nil) })
	return defaultsSnapshot
}

// Build returns the snapshot with the overrides applied to the defaults.
func Build(overrides map[string]string) (*Snapshot, error) {
	return std.build(overrides)
}
