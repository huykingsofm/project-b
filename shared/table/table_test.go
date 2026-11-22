package table_test

import (
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/shared/table"
)

type thing struct {
	owner int
	kind  string
}

func keysOf[V any](seq func(func(int, V) bool)) []int {
	var out []int
	for k := range seq {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func newThings() (*table.Table[int, thing], table.Handle[string], table.Handle2[int, string]) {
	var t table.Table[int, thing]
	byKind := t.Index(func(v thing) (string, bool) { return v.kind, v.kind != "" })
	byOwnerKind := t.Index2(func(v thing) (int, string, bool) { return v.owner, v.kind, true })
	return &t, byKind, byOwnerKind
}

func TestAColumnFindsTheValuesByItsKeyAndFollowsSetAndDelete(t *testing.T) {
	tb, byKind, _ := newThings()
	tb.Set(1, thing{1, "item"})
	tb.Set(2, thing{1, "body"})
	tb.Set(3, thing{2, "item"})
	tb.Set(4, thing{2, ""}) // not in the column: it has no kind
	if got := keysOf[thing](tb.Lookup(byKind, "item")); !slices.Equal(got, []int{1, 3}) {
		t.Fatalf("items %v, want [1 3]", got)
	}
	tb.Set(1, thing{1, "body"}) // the value changes: the old key is let go
	tb.Delete(3)
	if got := keysOf[thing](tb.Lookup(byKind, "item")); len(got) != 0 {
		t.Fatalf("items %v, want none", got)
	}
	if got := keysOf[thing](tb.Lookup(byKind, "body")); !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("bodies %v, want [1 2]", got)
	}
	if tb.Delete(99) || tb.Len() != 3 {
		t.Fatal("Delete of a key that is not there, or Len, is wrong")
	}
}

func TestACompositeColumnFindsByTheFirstKeyAloneAndByBoth(t *testing.T) {
	tb, _, byOwnerKind := newThings()
	tb.Set(1, thing{1, "item"})
	tb.Set(2, thing{1, "body"})
	tb.Set(3, thing{2, "item"})
	if got := keysOf[thing](tb.LookupFirst(byOwnerKind, 1)); !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("owner 1 has %v, want [1 2]", got)
	}
	if got := keysOf[thing](tb.Lookup2(byOwnerKind, 1, "item")); !slices.Equal(got, []int{1}) {
		t.Fatalf("owner 1 items %v, want [1]", got)
	}
	tb.Delete(1)
	tb.Delete(2)
	if got := keysOf[thing](tb.LookupFirst(byOwnerKind, 1)); len(got) != 0 {
		t.Fatalf("owner 1 has %v after its values went, want none", got)
	}
	if got := keysOf[thing](tb.LookupFirst(byOwnerKind, 2)); !slices.Equal(got, []int{3}) {
		t.Fatalf("owner 2 has %v, want [3]", got)
	}
}

func TestAColumnDeclaredLateTakesWhatIsThereAndLookupAsLeavesOutOtherTypes(t *testing.T) {
	var tb table.Table[int, any]
	tb.Set(1, 10)
	tb.Set(2, "ten")
	tb.Set(3, 30)
	all := tb.Index(func(any) (bool, bool) { return true, true })
	var ints []int
	for _, n := range tb.LookupAs[bool, int](all, true) {
		ints = append(ints, n)
	}
	slices.Sort(ints)
	if !slices.Equal(ints, []int{10, 30}) {
		t.Fatalf("ints %v, want [10 30]", ints)
	}
}

func TestACloneSharesNothingAndItsColumnsAreTheSame(t *testing.T) {
	tb, byKind, byOwnerKind := newThings()
	tb.Set(1, thing{1, "item"})
	c := tb.Clone()
	tb.Set(2, thing{1, "item"})
	c.Set(3, thing{1, "item"})
	if got := keysOf[thing](tb.Lookup(byKind, "item")); !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("table %v, want [1 2]", got)
	}
	if got := keysOf[thing](c.Lookup(byKind, "item")); !slices.Equal(got, []int{1, 3}) {
		t.Fatalf("clone %v, want [1 3]", got)
	}
	if got := keysOf[thing](c.Lookup2(byOwnerKind, 1, "item")); !slices.Equal(got, []int{1, 3}) {
		t.Fatalf("clone, two keys %v, want [1 3]", got)
	}
}

func TestAHandleOfAnotherTableOrOfAnotherKindOfColumnFindsNothing(t *testing.T) {
	tb, byKind, _ := newThings()
	tb.Set(1, thing{1, "item"})
	var other table.Table[int, thing]
	if n := len(keysOf[thing](other.Lookup(byKind, "item"))); n != 0 {
		t.Fatalf("a handle that is not of the table found %d values", n)
	}
	wrong := table.Handle2[int, string]{} // position 0 is a column of one key
	if n := len(keysOf[thing](tb.LookupFirst(wrong, 1))); n != 0 {
		t.Fatalf("a handle of another kind of column found %d values", n)
	}
}

func TestTheOrderOfWhatIsFoundIsTheSameForTheSameCalls(t *testing.T) {
	run := func() []int {
		tb, byKind, _ := newThings()
		for i := range 20 {
			tb.Set(i, thing{i % 3, "item"})
		}
		for i := 0; i < 20; i += 3 {
			tb.Delete(i)
		}
		var out []int
		for k := range tb.Lookup(byKind, "item") {
			out = append(out, k)
		}
		return out
	}
	if a, b := run(), run(); !slices.Equal(a, b) || len(a) == 0 {
		t.Fatalf("two runs found %v and %v", a, b)
	}
}

func BenchmarkCloneWithColumns(b *testing.B) {
	tb, _, _ := newThings()
	for i := range 1000 {
		tb.Set(i, thing{i % 10, "item"})
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = tb.Clone()
	}
}
