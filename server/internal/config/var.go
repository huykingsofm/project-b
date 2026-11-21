package config

import (
	"encoding/json"
	"fmt"
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
func (v Var[T]) Get(s *Snapshot) T {
	if s == nil {
		return v.def
	}
	return s.vals[v.idx].(T)
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
