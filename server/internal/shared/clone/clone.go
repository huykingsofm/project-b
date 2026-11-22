// Package clone is the one contract for copying data that is kept in a state: a type is either a value, which
// assignment copies whole, or has a Clone method that copies everything it holds. The containers (orderedmap,
// table, bimap) use it, so their Clone is right for both kinds of element without the caller choosing. It imports
// nothing of the project.
package clone

import "slices"

// Cloner is implemented by a type that has a method Clone that returns a copy sharing nothing with it. The method
// may be on the value or on the pointer.
type Cloner[T any] interface {
	Clone() T
}

// Has reports whether T, or the pointer to T, has the method Clone() T. A type that has not is copied by
// assignment, which is right only for a value: no pointer, slice, map, channel or function in it.
func Has[T any]() bool {
	_, ok := any((*T)(nil)).(Cloner[T])
	return ok
}

// Slice returns a copy of s that shares nothing with it, if the element type is a value or has Clone: the
// elements that have it are copied with it, and the others by assignment. Whether the type has Clone is decided
// once, not for each element, so copying a slice of values costs what slices.Clone costs.
func Slice[T any](s []T) []T {
	out := slices.Clone(s)
	if len(s) == 0 || !Has[T]() {
		return out
	}
	for i := range s {
		out[i] = any(&s[i]).(Cloner[T]).Clone()
	}
	return out
}
