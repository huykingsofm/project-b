package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
)

// Var is a declared variable of type T. Get reads its value from a Snapshot.
type Var[T any] struct {
	name string
	idx  int
	def  T
}

// Name is the name the variable was declared with.
func (v Var[T]) Name() string { return v.name }

// Get returns the value of the variable in s, or its default when s is nil. A value that is a
// slice or a map is shared by every reader of the snapshot: do not change it.
func (v Var[T]) Get(s ReadOnlySnapshot) T {
	if s != nil {
		if val, ok := s.lookup(v.idx); ok {
			return val.(T)
		}
	}
	return v.def
}

// declaration is what the registry keeps of a variable. It carries everything that depends on
// the type of the variable, so the registry and the snapshot never look at a type.
type declaration struct {
	name string
	def  any
	// parse reads the text of a value into the type of the variable and checks its options.
	parse func(text string) (any, error)
}

// registry is the set of declared variables. Variables are declared once, while packages are
// initialized; the first snapshot freezes the registry, so every snapshot has a value for every
// variable.
type registry struct {
	mu     sync.Mutex
	decls  []declaration
	byName map[string]int
	frozen bool
}

func newRegistry() *registry { return &registry{byName: map[string]int{}} }

// std is the registry that NewVar uses.
var std = newRegistry()

// declarations returns the declared variables, in the order they were declared. It ends the
// declaration of variables, as a snapshot does.
func (r *registry) declarations() []declaration {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frozen = true
	return r.decls
}

func (r *registry) add(d declaration) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case d.name == "":
		panic("config: a variable needs a name")
	case r.frozen:
		panic(fmt.Sprintf("config: %q is declared after the first snapshot was built", d.name))
	}
	if _, dup := r.byName[d.name]; dup {
		panic(fmt.Sprintf("config: %q is declared twice", d.name))
	}
	r.byName[d.name] = len(r.decls)
	r.decls = append(r.decls, d)
	return len(r.decls) - 1
}

func declare[T any](r *registry, name string, def T, opts []Option[T]) Var[T] {
	s := spec[T]{parse: func(text string) (T, error) {
		var v T
		if err := json.Unmarshal([]byte(text), &v); err != nil {
			return v, fmt.Errorf("%q is not a valid %T: %w", text, v, err)
		}
		return v, nil
	}}
	for _, o := range opts {
		o.apply(&s)
	}
	if err := s.check(def); err != nil {
		panic(fmt.Sprintf("config: default of %q: %v", name, err))
	}
	idx := r.add(declaration{name: name, def: def, parse: func(text string) (any, error) {
		v, err := s.parse(text)
		if err != nil {
			return nil, err
		}
		if err := s.check(v); err != nil {
			return nil, err
		}
		return v, nil
	}})
	return Var[T]{name: name, idx: idx, def: def}
}

// NewVar declares a variable of type T with a default and options (limits, and a parser of its
// own when json.Unmarshal is not the way the value is written). A default that breaks a limit is
// a programming error and panics. Call it while the program starts.
func NewVar[T any](name string, def T, opts ...Option[T]) Var[T] {
	return declare(std, name, def, opts)
}

// build makes a snapshot from the defaults and the overrides, which are the text of a JSON value
// by variable name. Every override is parsed into the type of its variable and checked against
// its options: if any of them is wrong, no snapshot is made and the error names each one, so a
// bad document never replaces a good configuration. A name that is not declared is returned in
// unknown and otherwise ignored.
func (r *registry) build(
	overrides map[string]string,
) (snap *Snapshot, unknown []string, err error) {
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
	return &Snapshot{vals: vals}, unknown, nil
}
