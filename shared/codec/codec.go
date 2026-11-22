// Package codec holds the register of a kind of thing that is written as bytes and read back: the
// kinds are told apart by a number, and each has a function that reads it.
//
// A Registry is made for one family of things (the actions, the commands, the states of a game),
// with the function that says which number a thing has. Each kind is registered once with a value
// of it, a prototype, and the function that reads it; the registry refuses a number or a Go type
// that two kinds share, so a number that is written always means one kind.
package codec

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
)

// ErrDuplicate is the error of a kind that is registered with a number or a Go type that another
// kind already has.
var ErrDuplicate = errors.New("codec: the number or the type is already registered")

// ErrNoPrototype is the error of a kind that is registered with no value.
var ErrNoPrototype = errors.New(
	"codec: a kind needs a prototype value and a function that reads it",
)

// Number is what tells the kinds of a family apart.
type Number interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}

// Registry holds the kinds of one family: T is what the kinds have in common, N their number and D
// the function that reads one. It is filled when the program starts and only read after that, so
// it takes no lock.
type Registry[N Number, T any, D any] struct {
	numberOf func(T) N
	decoders map[N]D
	types    map[reflect.Type]N
}

// New makes an empty registry. numberOf says which number a value has.
func New[N Number, T any, D any](numberOf func(T) N) *Registry[N, T, D] {
	return &Registry[N, T, D]{
		numberOf: numberOf, decoders: map[N]D{}, types: map[reflect.Type]N{},
	}
}

// Register adds a kind: the prototype says its number and its Go type, and decode reads it. It is
// an error when the number, or the Go type, is already registered.
func (r *Registry[N, T, D]) Register(prototype T, decode D) error {
	value := reflect.ValueOf(prototype)
	if !value.IsValid() || reflect.ValueOf(decode).IsNil() {
		return ErrNoPrototype
	}
	n := r.numberOf(prototype)
	if _, ok := r.decoders[n]; ok {
		return fmt.Errorf("%w: the number %d, of %T", ErrDuplicate, n, prototype)
	}
	if other, ok := r.types[value.Type()]; ok {
		return fmt.Errorf("%w: %T has the number %d and now %d", ErrDuplicate, prototype, other, n)
	}
	r.decoders[n] = decode
	r.types[value.Type()] = n
	return nil
}

// Lookup returns the function that reads the kind with the number.
func (r *Registry[N, T, D]) Lookup(n N) (D, bool) {
	d, ok := r.decoders[n]
	return d, ok
}

// Has reports whether a kind with the number is registered.
func (r *Registry[N, T, D]) Has(n N) bool {
	_, ok := r.decoders[n]
	return ok
}

// Numbers returns the numbers of the kinds registered, in rising order.
func (r *Registry[N, T, D]) Numbers() []N {
	out := make([]N, 0, len(r.decoders))
	for n := range r.decoders {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}
