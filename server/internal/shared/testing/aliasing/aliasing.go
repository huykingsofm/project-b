// Package aliasing finds the places where two values of a program share memory that can be changed, which a
// copy that was meant to be separate must not do. It is for tests: it uses reflect, and walks everything a
// value can reach, unexported fields too. It imports nothing of the project.
//
// Disjoint says whether two values share a map, a pointer, a channel, or the array behind a slice.
// CheckCloneable says whether a type obeys the rule of data that is copied: it is a value, or it has a Clone
// method that copies everything it holds.
package aliasing

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
)

// ref is a piece of memory that a value refers to: the map, the pointee or the array behind a slice.
type ref struct {
	start, end uintptr
	path       string
}

// Disjoint returns nil when a and b share no memory that can be changed, and otherwise an error that says,
// by the path to each, what they share. Strings are not looked at: they cannot be changed. Functions are not
// looked at either: two closures of one function literal have one address.
func Disjoint(a, b any) error {
	ra, rb := collect(reflect.ValueOf(a)), collect(reflect.ValueOf(b))
	sort.Slice(ra, func(i, j int) bool { return ra[i].start < ra[j].start })
	sort.Slice(rb, func(i, j int) bool { return rb[i].start < rb[j].start })
	var shared []string
	for i, j := 0, 0; i < len(ra) && j < len(rb); {
		x, y := ra[i], rb[j]
		if x.start < y.end && y.start < x.end {
			shared = append(shared, fmt.Sprintf("%s and %s", x.path, y.path))
			if len(shared) == 5 {
				break
			}
		}
		if x.end <= y.end {
			i++
		} else {
			j++
		}
	}
	if len(shared) > 0 {
		return fmt.Errorf("the values share memory: %s", strings.Join(shared, "; "))
	}
	return nil
}

type visit struct {
	addr uintptr
	typ  reflect.Type
}

func collect(v reflect.Value) []ref {
	var out []ref
	walk(v, "value", map[visit]bool{}, &out)
	return out
}

func add(out *[]ref, start uintptr, size uintptr, path string) {
	if start == 0 {
		return
	}
	*out = append(*out, ref{start: start, end: start + max(size, 1), path: path})
}

func walk(v reflect.Value, path string, seen map[visit]bool, out *[]ref) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		add(out, v.Pointer(), v.Type().Elem().Size(), path)
		if key := (visit{v.Pointer(), v.Type()}); !seen[key] {
			seen[key] = true
			walk(v.Elem(), "(*"+path+")", seen, out)
		}
	case reflect.Map:
		if v.IsNil() {
			return
		}
		add(out, v.Pointer(), 1, path)
		if key := (visit{v.Pointer(), v.Type()}); !seen[key] {
			seen[key] = true
			it := v.MapRange()
			for it.Next() {
				walk(it.Key(), path+"[key]", seen, out)
				walk(it.Value(), path+"[value]", seen, out)
			}
		}
	case reflect.Slice:
		if v.IsNil() {
			return
		}
		add(out, v.Pointer(), v.Type().Elem().Size()*uintptr(v.Cap()), path)
		for i := range v.Len() {
			walk(v.Index(i), fmt.Sprintf("%s[%d]", path, i), seen, out)
		}
	case reflect.Array:
		for i := range v.Len() {
			walk(v.Index(i), fmt.Sprintf("%s[%d]", path, i), seen, out)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			walk(v.Field(i), path+"."+v.Type().Field(i).Name, seen, out)
		}
	case reflect.Interface:
		if !v.IsNil() {
			walk(v.Elem(), path, seen, out)
		}
	case reflect.Chan, reflect.UnsafePointer:
		if !v.IsNil() {
			add(out, v.Pointer(), 1, path)
		}
	}
}

// Rules are what CheckCloneable allows besides values and types with Clone.
type Rules struct {
	// Interfaces are interface types that may be held. An interface held in a state is implemented only by
	// value types, which the type itself cannot say: whoever allows one says it is so.
	Interfaces []reflect.Type
	// Shared are types that may be shared on purpose because they never change once made, like the map of a
	// zone. They are named by the type that is shared, for example *tilemap.TileMap.
	Shared []reflect.Type
	// Boundary is for the types of what crosses from one goroutine to another (a command, an action, an event): a
	// struct of such a type is accepted when each of its fields is a value or has Clone, without a Clone of its own,
	// because it is the one that stores a field that copies it. Without it a struct that holds anything but values
	// must have Clone itself.
	Boundary bool
	// Skip are fields that are not data and are not copied, named "type.field" as reflect writes the type
	// (for example "*zonelayer.State.events").
	Skip []string
}

func (r Rules) has(list []reflect.Type, t reflect.Type) bool {
	return slices.Contains(list, t)
}

// CheckCloneable returns nil when t obeys the rule of data that is copied, and otherwise an error that says,
// by the path of its fields, where it does not.
//
//   - A value type (numbers, bool, string, and arrays and structs of them) is copied by assignment.
//   - Any other type must have a method Clone() T (or Clone() *T), which is trusted to copy what the type holds. What the type
//     holds is still looked at: a slice, a map or a pointer inside it is allowed, but the types they hold must
//     be values or have Clone too, because a container copies them with Clone when they have it and by
//     assignment when they have not.
//   - A channel and a function are never allowed. An interface is allowed when Rules says so, and as the key
//     of a map, because a key is an identifier.
func CheckCloneable(t reflect.Type, rules Rules) error {
	c := checker{rules: rules, active: map[reflect.Type]bool{}}
	return c.element(t, t.String(), false, false)
}

type checker struct {
	rules  Rules
	active map[reflect.Type]bool
}

// hasClone says whether the type, or its pointer, has a method Clone that returns the type or a pointer to it.
func hasClone(t reflect.Type) bool {
	m, ok := reflect.PointerTo(t).MethodByName("Clone")
	return ok && m.Type.NumIn() == 1 && m.Type.NumOut() == 1 && (m.Type.Out(0) == t || m.Type.Out(0) == reflect.PointerTo(t))
}

// pure says whether assignment copies everything the type holds.
func pure(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.String:
		return true
	case reflect.Array:
		return pure(t.Elem())
	case reflect.Struct:
		for field := range t.Fields() {
			if !pure(field.Type) {
				return false
			}
		}
		return true
	}
	return false
}

// element checks a type that is held as a value, as an element of a container or a field: it is a value, or a
// type with Clone whose contents are right too. Inside a type with Clone (trusted) a slice, a map or a pointer
// is a structure of that type, which its Clone copies, and only the types at its leaves are looked at. That is as
// far as a type can see: a container whose element is itself a slice would share it, and Disjoint finds that on a
// value that holds data.
func (c *checker) element(t reflect.Type, path string, isKey, trusted bool) error {
	switch {
	case pure(t) || c.rules.has(c.rules.Shared, t):
		return nil
	case trusted && (t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map || t.Kind() == reflect.Pointer):
		return c.held(t, path) // a structure inside a type with Clone: that Clone is trusted to copy it, and its leaves are looked at
	case t.Kind() == reflect.Interface:
		if c.rules.has(c.rules.Interfaces, t) || isKey {
			return nil
		}
		return fmt.Errorf("%s is the interface %s, which the rules do not allow", path, t)
	case t.Kind() == reflect.Chan || t.Kind() == reflect.Func || t.Kind() == reflect.UnsafePointer:
		return fmt.Errorf("%s is a %s, which cannot be copied", path, t.Kind())
	case !hasClone(t) && t.Kind() == reflect.Struct && c.rules.Boundary:
		for f := range t.Fields() {
			if slices.Contains(c.rules.Skip, t.String()+"."+f.Name) {
				continue
			}
			if err := c.element(f.Type, path+"."+f.Name, false, false); err != nil {
				return err
			}
		}
		return nil
	case !hasClone(t):
		return fmt.Errorf("%s is %s, which holds a reference and has no Clone method", path, t)
	}
	if c.active[t] {
		return nil // a type that holds itself: looked at where it began
	}
	c.active[t] = true
	defer delete(c.active, t)
	return c.contents(t, path)
}

// contents checks what a type with Clone holds: its Clone copies it, so references are allowed in it, but the
// types they lead to are checked as elements.
func (c *checker) contents(t reflect.Type, path string) error {
	switch t.Kind() {
	case reflect.Struct:
		for f := range t.Fields() {
			if slices.Contains(c.rules.Skip, t.String()+"."+f.Name) || slices.Contains(c.rules.Skip, "*"+t.String()+"."+f.Name) {
				continue
			}
			if err := c.held(f.Type, path+"."+f.Name); err != nil {
				return err
			}
		}
		return nil
	default:
		return c.held(t, path)
	}
}

// held checks a type that is a field of a type with Clone.
func (c *checker) held(t reflect.Type, path string) error {
	switch t.Kind() {
	case reflect.Slice, reflect.Array, reflect.Pointer:
		if c.rules.has(c.rules.Shared, t) {
			return nil
		}
		return c.element(t.Elem(), path+"[]", false, true)
	case reflect.Map:
		if err := c.element(t.Key(), path+"[key]", true, true); err != nil {
			return err
		}
		return c.element(t.Elem(), path+"[value]", false, true)
	case reflect.Struct:
		if pure(t) || hasClone(t) {
			return c.element(t, path, false, true)
		}
		return c.contents(t, path) // part of the same implementation
	default:
		return c.element(t, path, false, true)
	}
}
