package table

import (
	"iter"

	"github.com/huykingsofm/project-b/shared/stablemap"
	"github.com/huykingsofm/project-b/shared/stableset"
)

// column1 is a column of one key: the keys of the table, by the key of their value.
type column1[K comparable, V any, IK comparable] struct {
	keyOf   func(V) (IK, bool)
	buckets stablemap.Map[IK, *stableset.Set[K]]
}

func (c *column1[K, V, IK]) add(k K, v V) {
	ik, ok := c.keyOf(v)
	if !ok {
		return
	}
	b, ok := c.buckets.Get(ik)
	if !ok {
		b = &stableset.Set[K]{}
		c.buckets.Set(ik, b)
	}
	b.Add(k)
}

func (c *column1[K, V, IK]) remove(k K, v V) {
	ik, ok := c.keyOf(v)
	if !ok {
		return
	}
	if b, ok := c.buckets.Get(ik); ok {
		b.Delete(k)
		if b.Len() == 0 {
			c.buckets.Delete(ik)
		}
	}
}

func (c *column1[K, V, IK]) clone() column[K, V] {
	n := &column1[K, V, IK]{keyOf: c.keyOf}
	for ik, b := range c.buckets.All() {
		cp := b.Clone()
		n.buckets.Set(ik, &cp)
	}
	return n
}

func (c *column1[K, V, IK]) keys(ik IK) iter.Seq[K] {
	b, ok := c.buckets.Get(ik)
	if !ok {
		return func(func(K) bool) {}
	}
	return b.All()
}

// column2 is a column of two keys: for the first key, all the keys of the table, and for each
// second key, those that have it.
type column2[K comparable, V any, A, B comparable] struct {
	keyOf func(V) (A, B, bool)
	byA   stablemap.Map[A, *node[K, B]]
}

type node[K, B comparable] struct {
	all stableset.Set[K]
	byB stablemap.Map[B, *stableset.Set[K]]
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
	n.all.Add(k)
	s, ok := n.byB.Get(b)
	if !ok {
		s = &stableset.Set[K]{}
		n.byB.Set(b, s)
	}
	s.Add(k)
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
	n.all.Delete(k)
	if s, ok := n.byB.Get(b); ok {
		s.Delete(k)
		if s.Len() == 0 {
			n.byB.Delete(b)
		}
	}
	if n.all.Len() == 0 {
		c.byA.Delete(a)
	}
}

func (c *column2[K, V, A, B]) clone() column[K, V] {
	out := &column2[K, V, A, B]{keyOf: c.keyOf}
	for a, n := range c.byA.All() {
		cn := &node[K, B]{all: n.all.Clone()}
		for b, s := range n.byB.All() {
			cs := s.Clone()
			cn.byB.Set(b, &cs)
		}
		out.byA.Set(a, cn)
	}
	return out
}

func (c *column2[K, V, A, B]) keysOfFirst(a A) iter.Seq[K] {
	n, ok := c.byA.Get(a)
	if !ok {
		return func(func(K) bool) {}
	}
	return n.all.All()
}

func (c *column2[K, V, A, B]) keysOf(a A, b B) iter.Seq[K] {
	n, ok := c.byA.Get(a)
	if !ok {
		return func(func(K) bool) {}
	}
	s, ok := n.byB.Get(b)
	if !ok {
		return func(func(K) bool) {}
	}
	return s.All()
}
