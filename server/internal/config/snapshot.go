package config

import (
	"errors"
	"fmt"
	"slices"
)

// Snapshot is the value of every variable at one moment. It never changes after it is built, so
// any number of goroutines can read it, and a tick that reads one snapshot at its start sees one
// version of the configuration.
type Snapshot struct {
	version uint64
	vals    []any
}

// Version identifies the snapshot. Values derived from a snapshot are cached with the version
// they were computed from and recomputed when it changes.
func (s *Snapshot) Version() uint64 { return s.version }

// build makes a snapshot from the defaults and the overrides, which are the text of a JSON value
// by variable name. Every override is parsed into the type of its variable and checked against
// its options: if any of them is wrong, no snapshot is made and the error names each one, so a
// bad document never replaces a good configuration. A name that is not declared is returned in
// unknown and otherwise ignored.
func (r *registry) build(version uint64, overrides map[string]string) (snap *Snapshot, unknown []string, err error) {
	r.mu.Lock()
	r.frozen = true
	decls, byName := r.decls, r.byName
	r.mu.Unlock()

	vals := make([]any, len(decls))
	for i, d := range decls {
		vals[i] = d.def
	}

	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	slices.Sort(names)

	var errs []error
	for _, name := range names {
		i, ok := byName[name]
		if !ok {
			unknown = append(unknown, name)
			continue
		}
		v, perr := decls[i].parse(overrides[name])
		if perr != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, perr))
			continue
		}
		vals[i] = v
	}
	if len(errs) > 0 {
		return nil, unknown, errors.Join(errs...)
	}
	return &Snapshot{version: version, vals: vals}, unknown, nil
}

// Defaults returns the snapshot with every variable at its default. The first call, like Build,
// ends the declaration of variables.
func Defaults() *Snapshot {
	s, _, _ := std.build(0, nil)
	return s
}

// Build returns the snapshot of the given version with the overrides applied to the defaults.
func Build(version uint64, overrides map[string]string) (snap *Snapshot, unknown []string, err error) {
	return std.build(version, overrides)
}
