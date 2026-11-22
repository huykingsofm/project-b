package table_test

import (
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/shared/table"
)

type row struct {
	Owner string
	Kind  int
	N     int
}

func (r row) Index() map[string]any { return map[string]any{"owner": r.Owner, "kind": r.Kind} }

func ns(rows []row) []int {
	out := make([]int, len(rows))
	for i, r := range rows {
		out[i] = r.N
	}
	return out
}

func TestBuildIndexesEveryColumnOfEveryRow(t *testing.T) {
	tb := table.Build([]row{{"x", 1, 10}, {"y", 1, 20}, {"x", 2, 30}})
	if got := ns(tb.FindBy("owner", "x")); !slices.Equal(got, []int{10, 30}) {
		t.Fatalf("owner x = %v", got)
	}
	if got := ns(tb.FindBy("kind", 1)); !slices.Equal(got, []int{10, 20}) {
		t.Fatalf("kind 1 = %v", got)
	}
	if got := tb.FindBy("owner", "nobody"); len(got) != 0 {
		t.Fatalf("a value nobody has = %v", got)
	}
	if got := tb.FindBy("no such column", 1); len(got) != 0 {
		t.Fatalf("a column that does not exist = %v", got)
	}
	if len(tb.Dirty()) != 0 {
		t.Fatalf("a built table is not dirty: %v", tb.Dirty())
	}
}

func TestAddAndSetKeepTheIndexesAndTheDirtyRecordRight(t *testing.T) {
	var tb table.Table[row] // the zero table is ready to use
	a := tb.Add(row{"x", 1, 1})
	tb.Add(row{"y", 1, 2})
	tb.Add(row{"x", 1, 3})
	if !slices.Equal(tb.Dirty(), []int{0, 1, 2}) {
		t.Fatalf("dirty = %v", tb.Dirty())
	}
	tb.ClearDirty()

	tb.Set(a, row{"y", 1, 11}) // moves a row from one owner to another, keeping the order
	if got := ns(tb.FindBy("owner", "x")); !slices.Equal(got, []int{3}) {
		t.Fatalf("owner x after Set = %v", got)
	}
	if got := ns(tb.FindBy("owner", "y")); !slices.Equal(got, []int{11, 2}) {
		t.Fatalf("owner y after Set = %v", got)
	}
	if !slices.Equal(tb.Dirty(), []int{0}) {
		t.Fatalf("dirty after Set = %v", tb.Dirty())
	}
}

func TestDeleteAndTrimMovePositionsAndFixWhatHoldsThem(t *testing.T) {
	tb := table.Build([]row{{"x", 1, 0}, {"y", 1, 1}, {"x", 1, 2}, {"y", 1, 3}, {"x", 1, 4}})
	tb.Set(3, row{"y", 1, 33}) // row 3 is dirty
	tb.Delete(1)
	if got := ns(tb.Rows()); !slices.Equal(got, []int{0, 2, 33, 4}) {
		t.Fatalf("rows = %v", got)
	}
	if !slices.Equal(tb.Dirty(), []int{2}) { // the dirty row moved up with the rows
		t.Fatalf("dirty = %v", tb.Dirty())
	}
	if got := ns(tb.FindBy("owner", "x")); !slices.Equal(got, []int{0, 2, 4}) {
		t.Fatalf("owner x = %v", got)
	}

	tb.Trim(2) // the oldest go
	if got := ns(tb.Rows()); !slices.Equal(got, []int{33, 4}) {
		t.Fatalf("rows after Trim = %v", got)
	}
	if got := ns(tb.FindBy("owner", "y")); !slices.Equal(got, []int{33}) {
		t.Fatalf("owner y after Trim = %v", got)
	}
	if !slices.Equal(tb.Dirty(), []int{0}) {
		t.Fatalf("dirty after Trim = %v", tb.Dirty())
	}
}

func TestACloneSharesNothingAndCanBeSearchedAtOnce(t *testing.T) {
	tb := table.Build([]row{{"x", 1, 1}, {"y", 1, 2}})
	c := tb.Clone()
	tb.Add(row{"x", 1, 3})
	tb.Set(0, row{"z", 9, 100})

	if got := ns(c.FindBy("owner", "x")); !slices.Equal(got, []int{1}) {
		t.Fatalf("the clone changed with the original: %v", got)
	}
	if got := ns(tb.FindBy("owner", "x")); !slices.Equal(got, []int{3}) {
		t.Fatalf("the original = %v", got)
	}
	if c.Len() != 2 || len(c.Dirty()) != 0 {
		t.Fatalf("clone len %d, dirty %v", c.Len(), c.Dirty())
	}
}

// listRow is a row that holds a slice, and so has a Clone method.
type listRow struct {
	ID    int
	Items []int
}

func (r listRow) Index() map[string]any { return map[string]any{"id": r.ID} }
func (r listRow) Clone() listRow {
	return listRow{ID: r.ID, Items: append([]int(nil), r.Items...)}
}

func TestACloneCopiesRowsThatHaveACloneWithTheirClone(t *testing.T) {
	tb := table.Build([]listRow{{ID: 1, Items: []int{1, 2}}})
	c := tb.Clone()
	c.Rows()[0].Items[0] = 99
	if tb.Rows()[0].Items[0] != 1 {
		t.Fatal("a change of the copy changed the table")
	}
}
