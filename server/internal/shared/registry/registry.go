// Package publish lets an engine make its latest state readable by the other engines,
// without a lock on the path of the reader and without the engines waiting for each other.
package registry

import (
	"sync"
	"sync/atomic"
)

// Registry holds, for each key, the latest value that was published under it.
//
// The publisher gives a pointer to a copy that it will never change again, and the readers
// take the pointer and may keep it as long as they like: the garbage collector frees the
// copy when nobody holds it any more, so there is no count of readers and nothing to give
// back. A reader sees the latest copy that exists when it asks, which may be a few ticks
// old; the engine that owns the data is the only one that knows it exactly.
type Registry[K comparable, T any] struct {
	slots sync.Map // K -> *atomic.Pointer[T]
}

// Publish makes v the latest value of the key.
func (r *Registry[K, T]) Publish(k K, v *T) {
	s, ok := r.slots.Load(k)
	if !ok {
		s, _ = r.slots.LoadOrStore(k, new(atomic.Pointer[T]))
	}
	s.(*atomic.Pointer[T]).Store(v)
}

// Get returns the latest value of the key, or nil if none was published.
func (r *Registry[K, T]) Get(k K) *T {
	s, ok := r.slots.Load(k)
	if !ok {
		return nil
	}
	return s.(*atomic.Pointer[T]).Load()
}

// Remove forgets the key.
func (r *Registry[K, T]) Remove(k K) { r.slots.Delete(k) }
