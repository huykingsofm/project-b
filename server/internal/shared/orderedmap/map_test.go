package orderedmap_test

import (
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/shared/orderedmap"
)

func TestMapKeepsAFixedOrderAndSwapsOnDelete(t *testing.T) {
	var m orderedmap.Map[int, string]
	for i, s := range []string{"a", "b", "c", "d"} {
		m.Set(i+1, s)
	}
	m.Set(2, "B") // replacing keeps the place
	if !slices.Equal(m.Keys(), []int{1, 2, 3, 4}) {
		t.Fatalf("keys = %v", m.Keys())
	}
	m.Delete(1) // the last key takes its place
	if !slices.Equal(m.Keys(), []int{4, 2, 3}) {
		t.Fatalf("keys after delete = %v", m.Keys())
	}
	if v, ok := m.Get(4); !ok || v != "d" {
		t.Fatalf("Get(4) = %q, %v", v, ok)
	}
	if m.Has(1) || m.Len() != 3 {
		t.Fatal("deleted key is still there")
	}
}

func TestMapCloneSharesNothing(t *testing.T) {
	var m orderedmap.Map[int, int]
	m.Set(1, 10)
	c := m.Clone()
	m.Set(1, 11)
	m.Set(2, 20)
	if v, _ := c.Get(1); v != 10 || c.Len() != 1 {
		t.Fatalf("the clone changed with the original: %v, len %d", v, c.Len())
	}
}
