package table_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/shared/table"
)

// kind and owner are the key types of the columns of the tests: a column is found by the type of
// its key, so each has a type of its own.
type (
	kind  string
	owner int
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

func newThings(t testing.TB) *table.Table[int, thing] {
	t.Helper()
	var tb table.Table[int, thing]
	byKind := func(v thing) (kind, bool) { return kind(v.kind), v.kind != "" }
	if err := tb.Index(byKind); err != nil {
		t.Fatal(err)
	}
	if err := tb.Index2(func(v thing) (owner, kind, bool) {
		return owner(v.owner), kind(v.kind), true
	}); err != nil {
		t.Fatal(err)
	}
	return &tb
}

func TestAColumnFindsTheValuesByItsKeyAndFollowsSetAndDelete(t *testing.T) {
	tb := newThings(t)
	tb.Set(1, thing{1, "item"})
	tb.Set(2, thing{1, "body"})
	tb.Set(3, thing{2, "item"})
	tb.Set(4, thing{2, ""}) // not in the column: it has no kind
	if got := keysOf[thing](tb.Lookup(kind("item"))); !slices.Equal(got, []int{1, 3}) {
		t.Fatalf("items %v, want [1 3]", got)
	}
	tb.Set(1, thing{1, "body"}) // the value changes: the old key is let go
	tb.Delete(3)
	if got := keysOf[thing](tb.Lookup(kind("item"))); len(got) != 0 {
		t.Fatalf("items %v, want none", got)
	}
	if got := keysOf[thing](tb.Lookup(kind("body"))); !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("bodies %v, want [1 2]", got)
	}
	if tb.Delete(99) || tb.Len() != 3 {
		t.Fatal("Delete of a key that is not there, or Len, is wrong")
	}
}

func TestACompositeColumnFindsByTheFirstKeyAloneAndByBoth(t *testing.T) {
	tb := newThings(t)
	tb.Set(1, thing{1, "item"})
	tb.Set(2, thing{1, "body"})
	tb.Set(3, thing{2, "item"})
	if got := keysOf[thing](tb.Lookup2First(owner(1))); !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("owner 1 has %v, want [1 2]", got)
	}
	if got := keysOf[thing](tb.Lookup2(owner(1), kind("item"))); !slices.Equal(got, []int{1}) {
		t.Fatalf("owner 1 items %v, want [1]", got)
	}
	tb.Delete(1)
	tb.Delete(2)
	if got := keysOf[thing](tb.Lookup2First(owner(1))); len(got) != 0 {
		t.Fatalf("owner 1 has %v after its values went, want none", got)
	}
	if got := keysOf[thing](tb.Lookup2First(owner(2))); !slices.Equal(got, []int{3}) {
		t.Fatalf("owner 2 has %v, want [3]", got)
	}
}

func TestAKeyTypeIsOfOneColumnAndTheSecondDeclarationIsAnError(t *testing.T) {
	tb := newThings(t)
	err := tb.Index(func(v thing) (kind, bool) { return "x", true })
	if !errors.Is(err, table.ErrColumnDeclared) {
		t.Fatalf("a second column of the key type kind: %v", err)
	}
	err = tb.Index2(func(v thing) (owner, int, bool) { return 0, 0, true })
	if !errors.Is(err, table.ErrColumnDeclared) {
		t.Fatalf("a second column of two keys that starts with owner: %v", err)
	}
	tb.Set(1, thing{1, "item"})
	if got := keysOf[thing](tb.Lookup(kind("item"))); !slices.Equal(got, []int{1}) {
		t.Fatalf("the column that was refused changed the first: %v", got)
	}
}

func TestAColumnDeclaredLateTakesWhatIsThereAndLookupAsLeavesOutOtherTypes(t *testing.T) {
	type all bool
	var tb table.Table[int, any]
	tb.Set(1, 10)
	tb.Set(2, "ten")
	tb.Set(3, 30)
	if err := tb.Index(func(any) (all, bool) { return true, true }); err != nil {
		t.Fatal(err)
	}
	var ints []int
	for _, n := range tb.LookupAs[all, int](true) {
		ints = append(ints, n)
	}
	slices.Sort(ints)
	if !slices.Equal(ints, []int{10, 30}) {
		t.Fatalf("ints %v, want [10 30]", ints)
	}
}

func TestLookup2AsLeavesOutTheValuesThatAreNotOfTheType(t *testing.T) {
	type pair bool
	var tb table.Table[int, any]
	tb.Set(1, 10)
	tb.Set(2, "ten")
	if err := tb.Index2(func(any) (pair, bool, bool) { return true, true, true }); err != nil {
		t.Fatal(err)
	}
	n := 0
	for range tb.Lookup2As[pair, bool, int](true, true) {
		n++
	}
	if n != 1 {
		t.Fatalf("%d ints found, want 1", n)
	}
}

func TestACloneSharesNothingAndItsColumnsAreTheSame(t *testing.T) {
	tb := newThings(t)
	tb.Set(1, thing{1, "item"})
	c := tb.Clone()
	tb.Set(2, thing{1, "item"})
	c.Set(3, thing{1, "item"})
	if got := keysOf[thing](tb.Lookup(kind("item"))); !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("table %v, want [1 2]", got)
	}
	if got := keysOf[thing](c.Lookup(kind("item"))); !slices.Equal(got, []int{1, 3}) {
		t.Fatalf("clone %v, want [1 3]", got)
	}
	if got := keysOf[thing](c.Lookup2(owner(1), kind("item"))); !slices.Equal(got, []int{1, 3}) {
		t.Fatalf("clone, two keys %v, want [1 3]", got)
	}
}

func TestAKeyTypeThatIsOfNoColumnFindsNothing(t *testing.T) {
	tb := newThings(t)
	tb.Set(1, thing{1, "item"})
	if n := len(keysOf[thing](tb.Lookup("item"))); n != 0 { // a string, and no column has it
		t.Fatalf("a key type of no column found %d values", n)
	}
	if n := len(keysOf[thing](tb.Lookup2First(int64(1)))); n != 0 {
		t.Fatalf("a first key type of no column found %d values", n)
	}
}

func TestResetTakesTheValuesOutAndKeepsTheColumns(t *testing.T) {
	tb := newThings(t)
	tb.Set(1, thing{1, "item"})
	tb.Reset()
	if tb.Len() != 0 || len(keysOf[thing](tb.Lookup(kind("item")))) != 0 {
		t.Fatal("Reset left something")
	}
	tb.Set(2, thing{1, "item"})
	if got := keysOf[thing](tb.Lookup(kind("item"))); !slices.Equal(got, []int{2}) {
		t.Fatalf("the columns do not go on after Reset: %v", got)
	}
}

func TestTheOrderOfWhatIsFoundIsTheSameForTheSameCalls(t *testing.T) {
	run := func() []int {
		tb := newThings(t)
		for i := range 20 {
			tb.Set(i, thing{i % 3, "item"})
		}
		for i := 0; i < 20; i += 3 {
			tb.Delete(i)
		}
		var out []int
		for k := range tb.Lookup(kind("item")) {
			out = append(out, k)
		}
		return out
	}
	if a, b := run(), run(); !slices.Equal(a, b) || len(a) == 0 {
		t.Fatalf("two runs found %v and %v", a, b)
	}
}

func BenchmarkCloneWithColumns(b *testing.B) {
	tb := newThings(b)
	for i := range 1000 {
		tb.Set(i, thing{i % 10, "item"})
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = tb.Clone()
	}
}
