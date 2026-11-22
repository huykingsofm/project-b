package stablemap_test

import (
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/shared/stablemap"
)

func TestMapKeepsAFixedOrderAndSwapsOnDelete(t *testing.T) {
	var m stablemap.Map[int, string]
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
	var m stablemap.Map[int, int]
	m.Set(1, 10)
	c := m.Clone()
	m.Set(1, 11)
	m.Set(2, 20)
	if v, _ := c.Get(1); v != 10 || c.Len() != 1 {
		t.Fatalf("the clone changed with the original: %v, len %d", v, c.Len())
	}
}

// slots is a value that holds a slice, and so has a Clone method.
type slots struct{ Items []int }

func (s slots) Clone() slots { return slots{Items: slices.Clone(s.Items)} }

func TestMapCloneCopiesValuesThatHaveACloneWithTheirClone(t *testing.T) {
	var m stablemap.Map[string, slots]
	m.Set("a", slots{Items: []int{1, 2}})
	c := m.Clone()
	v, _ := c.Get("a")
	v.Items[0] = 99
	if got, _ := m.Get("a"); got.Items[0] != 1 {
		t.Fatalf("a change of the copy changed the map: %v", got.Items)
	}
	p := c.Ptr("a")
	p.Items = append(p.Items, 3)
	if got, _ := m.Get("a"); len(got.Items) != 2 {
		t.Fatalf("a change of the copy changed the map: %v", got.Items)
	}
}

func TestClearEmptiesTheMapAndItGoesOnFromNothing(t *testing.T) {
	var m stablemap.Map[int, string]
	m.Set(1, "a")
	m.Set(2, "b")
	m.Clear()
	if m.Len() != 0 || m.Has(1) || len(m.Keys()) != 0 {
		t.Fatal("the map still holds something after Clear")
	}
	m.Set(3, "c")
	if v, _ := m.Get(3); v != "c" || m.Len() != 1 {
		t.Fatal("the map does not go on after Clear")
	}
}
