package table

import (
	"slices"

	"github.com/huykingsofm/project-b/shared/stablemap"
)

// column is what the columns of a table or a queue ask of each column, whatever the type of its
// key.
type column[K comparable, V any] interface {
	add(k K, v V)
	remove(k K, v V)
	// change is remove of the old value and add of the new one, and nothing when the new value has
	// the same keys as the old one: the key stays where it is in its buckets.
	change(k K, old, v V)
	clone() column[K, V]
	reset()
}

// bucket is the keys that have one key of a column, in a stable order: the order they were added
// in, except that removing one moves the last into its place, so the order is a function of the
// calls and nothing else. Removing costs the size of the bucket (it looks for the key).
type bucket[K comparable] []K

func (b *bucket[K]) add(k K) { *b = append(*b, k) }

func (b *bucket[K]) remove(k K) {
	i := slices.Index(*b, k)
	if i < 0 {
		return
	}
	last := len(*b) - 1
	(*b)[i] = (*b)[last]
	var zero K
	(*b)[last] = zero
	*b = (*b)[:last]
}

// column1 is a column of one key: the keys of the data, by the key of their value.
type column1[K comparable, V any, IK comparable] struct {
	keyOf   func(V) (IK, bool)
	buckets stablemap.Map[IK, bucket[K]]
}

func (c *column1[K, V, IK]) add(k K, v V) {
	ik, ok := c.keyOf(v)
	if !ok {
		return
	}
	if b := c.buckets.Ptr(ik); b != nil {
		b.add(k)
		return
	}
	c.buckets.Set(ik, bucket[K]{k})
}

func (c *column1[K, V, IK]) remove(k K, v V) {
	ik, ok := c.keyOf(v)
	if !ok {
		return
	}
	if b := c.buckets.Ptr(ik); b != nil {
		b.remove(k)
		if len(*b) == 0 {
			c.buckets.Delete(ik)
		}
	}
}

func (c *column1[K, V, IK]) change(k K, old, v V) {
	oldKey, oldOK := c.keyOf(old)
	newKey, newOK := c.keyOf(v)
	if oldOK == newOK && (!oldOK || oldKey == newKey) {
		return
	}
	c.remove(k, old)
	c.add(k, v)
}

func (c *column1[K, V, IK]) reset() { c.buckets.Clear() }

func (c *column1[K, V, IK]) clone() column[K, V] {
	n := &column1[K, V, IK]{keyOf: c.keyOf}
	for ik, b := range c.buckets.All() {
		n.buckets.Set(ik, slices.Clone(b))
	}
	return n
}

// keys is a view of the keys that have the key. Do not change it, and do not keep it past the next
// change of the column.
func (c *column1[K, V, IK]) keys(ik IK) []K {
	b, _ := c.buckets.Get(ik)
	return b
}

// column2 is a column of two keys: for the first key, all the keys of the data, and for each
// second key, those that have it.
type column2[K comparable, V any, A, B comparable] struct {
	keyOf func(V) (A, B, bool)
	byA   stablemap.Map[A, *node[K, B]]
}

type node[K, B comparable] struct {
	all bucket[K]
	byB stablemap.Map[B, bucket[K]]
}

func (c *column2[K, V, A, B]) add(k K, v V) {
	a, b, ok := c.keyOf(v)
	if !ok {
		return
	}
	n, ok := c.byA.Get(a)
	if !ok {
		n = &node[K, B]{}
		c.byA.Set(a, n)
	}
	n.all.add(k)
	if s := n.byB.Ptr(b); s != nil {
		s.add(k)
		return
	}
	n.byB.Set(b, bucket[K]{k})
}

func (c *column2[K, V, A, B]) remove(k K, v V) {
	a, b, ok := c.keyOf(v)
	if !ok {
		return
	}
	n, ok := c.byA.Get(a)
	if !ok {
		return
	}
	n.all.remove(k)
	if s := n.byB.Ptr(b); s != nil {
		s.remove(k)
		if len(*s) == 0 {
			n.byB.Delete(b)
		}
	}
	if len(n.all) == 0 {
		c.byA.Delete(a)
	}
}

func (c *column2[K, V, A, B]) change(k K, old, v V) {
	oldA, oldB, oldOK := c.keyOf(old)
	newA, newB, newOK := c.keyOf(v)
	if oldOK == newOK && (!oldOK || (oldA == newA && oldB == newB)) {
		return
	}
	c.remove(k, old)
	c.add(k, v)
}

func (c *column2[K, V, A, B]) reset() { c.byA.Clear() }

func (c *column2[K, V, A, B]) clone() column[K, V] {
	out := &column2[K, V, A, B]{keyOf: c.keyOf}
	for a, n := range c.byA.All() {
		cn := &node[K, B]{all: slices.Clone(n.all)}
		for b, s := range n.byB.All() {
			cn.byB.Set(b, slices.Clone(s))
		}
		out.byA.Set(a, cn)
	}
	return out
}

func (c *column2[K, V, A, B]) keysOfFirst(a A) []K {
	n, ok := c.byA.Get(a)
	if !ok {
		return nil
	}
	return n.all
}

func (c *column2[K, V, A, B]) keysOf(a A, b B) []K {
	n, ok := c.byA.Get(a)
	if !ok {
		return nil
	}
	s, _ := n.byB.Get(b)
	return s
}
