package config

import (
	"cmp"
	"fmt"
)

// spec is what the options of a declaration build: how the text of a value is read, and the
// checks that a value has to pass.
type spec[T any] struct {
	parse  func(text string) (T, error)
	checks []func(T) error
}

func (s *spec[T]) check(v T) error {
	for _, c := range s.checks {
		if err := c(v); err != nil {
			return err
		}
	}
	return nil
}

// Option changes how a variable is declared. A new option is a new type with an apply method;
// nothing in this package has to change for it.
type Option[T any] interface {
	apply(*spec[T])
}

// Limit is an option that restricts the values of a variable: a value that fails it is refused.
type Limit[T any] struct {
	check func(T) error
}

func (l Limit[T]) apply(s *spec[T]) { s.checks = append(s.checks, l.check) }

// Check is a limit made of a function that returns an error for a value that is not allowed.
func Check[T any](f func(T) error) Limit[T] { return Limit[T]{f} }

// Min requires a value of at least lo.
func Min[T cmp.Ordered](lo T) Limit[T] {
	return Limit[T]{func(v T) error {
		if v < lo {
			return fmt.Errorf("%v is below the minimum %v", v, lo)
		}
		return nil
	}}
}

// Max requires a value of at most hi.
func Max[T cmp.Ordered](hi T) Limit[T] {
	return Limit[T]{func(v T) error {
		if v > hi {
			return fmt.Errorf("%v is above the maximum %v", v, hi)
		}
		return nil
	}}
}

// Range requires a value from lo to hi, both included.
func Range[T cmp.Ordered](lo, hi T) Limit[T] {
	return Limit[T]{func(v T) error {
		if v < lo || v > hi {
			return fmt.Errorf("%v is outside %v to %v", v, lo, hi)
		}
		return nil
	}}
}

// Parser is an option that replaces json.Unmarshal as the way the text of a value is read into
// the type of the variable, for a kind of value that is not written as JSON.
type Parser[T any] struct {
	parse func(text string) (T, error)
}

func (p Parser[T]) apply(s *spec[T]) { s.parse = p.parse }

// Parse makes a parser option.
func Parse[T any](f func(text string) (T, error)) Parser[T] { return Parser[T]{f} }
