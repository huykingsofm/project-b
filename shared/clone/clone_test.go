package clone_test

import (
	"testing"

	"github.com/huykingsofm/project-b/shared/clone"
)

type withValueClone struct{ Items []int }

func (w withValueClone) Clone() withValueClone {
	return withValueClone{Items: append([]int(nil), w.Items...)}
}

type withPointerClone struct{ Items []int }

func (w *withPointerClone) Clone() withPointerClone {
	return withPointerClone{Items: append([]int(nil), w.Items...)}
}

type plain struct{ A, B int }

func TestHasSaysWhetherTheTypeOrItsPointerHasClone(t *testing.T) {
	if !clone.Has[withValueClone]() || !clone.Has[withPointerClone]() {
		t.Fatal("a type with Clone on the value or the pointer must have it")
	}
	if clone.Has[plain]() || clone.Has[int]() || clone.Has[string]() || clone.Has[any]() {
		t.Fatal("a value type has no Clone")
	}
}

func TestSliceOfTypesWithCloneCopiesEachElementWithItsClone(t *testing.T) {
	a := []withValueClone{{Items: []int{1, 2}}, {Items: []int{3}}}
	b := clone.Slice(a)
	b[0].Items[0] = 99
	b[1].Items = append(b[1].Items, 4)
	if a[0].Items[0] != 1 || len(a[1].Items) != 1 {
		t.Fatalf("a change of the copy changed the original: %+v", a)
	}
	c := []withPointerClone{{Items: []int{1}}}
	d := clone.Slice(c)
	d[0].Items[0] = 7
	if c[0].Items[0] != 1 {
		t.Fatal("a Clone on the pointer was not used")
	}
}

func TestSliceOfValuesIsACopyAndNilAndEmptyStayWhatTheyAre(t *testing.T) {
	a := []plain{{1, 2}, {3, 4}}
	b := clone.Slice(a)
	b[0].A = 9
	if a[0].A != 1 {
		t.Fatal("the copy shares its array")
	}
	if clone.Slice[plain](nil) != nil {
		t.Fatal("a nil slice became a non-nil one")
	}
	if got := clone.Slice([]plain{}); got == nil || len(got) != 0 {
		t.Fatalf("an empty slice became %v", got)
	}
}

func BenchmarkSliceOfValues(b *testing.B) {
	s := make([]plain, 64)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = clone.Slice(s)
	}
}

func BenchmarkSliceOfClonableValues(b *testing.B) {
	s := make([]withValueClone, 64)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = clone.Slice(s)
	}
}
