package stablebimap_test

import (
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/shared/stablebimap"
)

func TestALinkIsReadFromBothSides(t *testing.T) {
	var b stablebimap.BiMap[string, int]
	b.Link("a", 1)
	b.Link("b", 2)
	if v, ok := b.GetByV1("a"); !ok || v != 1 {
		t.Fatalf(`Get("a") = %d, %v`, v, ok)
	}
	if k, ok := b.GetByV2(2); !ok || k != "b" {
		t.Fatalf("GetKey(2) = %q, %v", k, ok)
	}
	if _, ok := b.GetByV1("zzz"); ok {
		t.Fatal("an unknown key was found")
	}
	if _, ok := b.GetByV2(99); ok {
		t.Fatal("an unknown value was found")
	}
	if b.Len() != 2 {
		t.Fatalf("Len = %d", b.Len())
	}
}

func TestLinkingAKeyAgainReplacesItsValueOnBothSides(t *testing.T) {
	var b stablebimap.BiMap[string, int]
	b.Link("a", 1)
	b.Link("a", 5)
	if v, _ := b.GetByV1("a"); v != 5 {
		t.Fatalf(`Get("a") = %d, want 5`, v)
	}
	if _, ok := b.GetByV2(1); ok {
		t.Fatal("the old value still points to the key")
	}
	if b.Len() != 1 {
		t.Fatalf("Len = %d, want 1", b.Len())
	}
}

func TestLinkingAValueAgainTakesItFromItsOldKey(t *testing.T) {
	var b stablebimap.BiMap[string, int]
	b.Link("a", 1)
	b.Link("b", 1) // the value 1 now belongs to "b"
	if _, ok := b.GetByV1("a"); ok {
		t.Fatal(`"a" is still linked`)
	}
	if k, _ := b.GetByV2(1); k != "b" {
		t.Fatalf("GetKey(1) = %q, want b", k)
	}
	if b.Len() != 1 {
		t.Fatalf("Len = %d, want 1", b.Len())
	}
}

func TestUnlinkRemovesBothSides(t *testing.T) {
	var b stablebimap.BiMap[string, int]
	b.Link("a", 1)
	if !b.UnlinkByV1("a") {
		t.Fatal("Unlink reported no link")
	}
	if _, ok := b.GetByV1("a"); ok {
		t.Fatal("the key is still linked")
	}
	if _, ok := b.GetByV2(1); ok {
		t.Fatal("the value is still linked")
	}
	if b.UnlinkByV1("a") {
		t.Fatal("a second Unlink reported a link")
	}
	if b.Len() != 0 {
		t.Fatalf("Len = %d", b.Len())
	}
}

func TestLinksAreVisitedInTheOrderTheyWereMade(t *testing.T) {
	var b stablebimap.BiMap[int, int]
	for _, k := range []int{5, 3, 9, 1} {
		b.Link(k, k*10)
	}
	var keys []int
	for k := range b.AllV1() {
		keys = append(keys, k)
	}
	if !slices.Equal(keys, []int{5, 3, 9, 1}) {
		t.Fatalf("keys = %v", keys)
	}
}

func TestRemovingALinkFromTheMiddleKeepsTheOthersReadableFromBothSides(t *testing.T) {
	var b stablebimap.BiMap[int, string]
	for i, s := range []string{"a", "b", "c", "d"} {
		b.Link(i+1, s)
	}
	b.UnlinkByV1(2) // the last link ("d") moves into its place
	for _, want := range []struct {
		v1 int
		v2 string
	}{{1, "a"}, {3, "c"}, {4, "d"}} {
		if v2, ok := b.GetByV1(want.v1); !ok || v2 != want.v2 {
			t.Fatalf("GetByV1(%d) = %q, %v, want %q", want.v1, v2, ok, want.v2)
		}
		if v1, ok := b.GetByV2(want.v2); !ok || v1 != want.v1 {
			t.Fatalf("GetByV2(%q) = %d, %v, want %d", want.v2, v1, ok, want.v1)
		}
	}
	if _, ok := b.GetByV2("b"); ok {
		t.Fatal(`"b" is still linked`)
	}
	var order []int
	for v1 := range b.AllV1() {
		order = append(order, v1)
	}
	if !slices.Equal(order, []int{1, 4, 3}) {
		t.Fatalf("order = %v, want [1 4 3]", order)
	}
}

func TestACloneSharesNothingWithTheMap(t *testing.T) {
	var b stablebimap.BiMap[string, int]
	b.Link("a", 1)
	c := b.Clone()
	b.Link("b", 2)
	b.UnlinkByV1("a")
	if v, ok := c.GetByV1("a"); !ok || v != 1 {
		t.Fatal("the clone lost a link of the original")
	}
	if _, ok := c.GetByV1("b"); ok {
		t.Fatal("the clone took a link made after it")
	}
	if k, ok := c.GetByV2(1); !ok || k != "a" {
		t.Fatal("the clone lost the other direction")
	}
}

func TestEveryOperationWorksFromEitherSide(t *testing.T) {
	var b stablebimap.BiMap[int, string]
	b.Link(1, "a")
	b.Link(2, "b")
	b.Link(3, "c")

	if !b.UnlinkByV2("b") {
		t.Fatal(`UnlinkByV2("b") reported no link`)
	}
	if b.UnlinkByV2("b") {
		t.Fatal("a second UnlinkByV2 reported a link")
	}
	if _, ok := b.GetByV1(2); ok {
		t.Fatal("2 is still linked after its V2 was unlinked")
	}
	if v2, _ := b.GetByV1(3); v2 != "c" {
		t.Fatalf("GetByV1(3) = %q, want c", v2)
	}

	var firsts []int
	for v1 := range b.AllV1() {
		firsts = append(firsts, v1)
	}
	var seconds []string
	var backs []int
	for v2, v1 := range b.AllV2() {
		seconds = append(seconds, v2)
		backs = append(backs, v1)
	}
	if !slices.Equal(firsts, backs) {
		t.Fatalf(
			"AllV1 gave %v and AllV2 gave the V1s %v: not the same links in the same order",
			firsts,
			backs,
		)
	}
	if !slices.Equal(seconds, []string{"a", "c"}) {
		t.Fatalf("AllV2 gave %v", seconds)
	}
}
