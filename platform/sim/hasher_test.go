package sim_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
)

func sum(f func(h *sim.Hasher)) uint64 {
	h := sim.NewHasher()
	f(&h)
	return h.Sum()
}

func TestTheSameValuesGiveTheSameHashAndADifferenceChangesIt(t *testing.T) {
	a := sum(func(h *sim.Hasher) { h.U64(1); h.U64(2); h.String("an") })
	if a != sum(func(h *sim.Hasher) { h.U64(1); h.U64(2); h.String("an") }) {
		t.Fatal("the same values gave two hashes")
	}
	for name, other := range map[string]uint64{
		"another number": sum(func(h *sim.Hasher) { h.U64(1); h.U64(3); h.String("an") }),
		"another order":  sum(func(h *sim.Hasher) { h.U64(2); h.U64(1); h.String("an") }),
		"another text":   sum(func(h *sim.Hasher) { h.U64(1); h.U64(2); h.String("na") }),
		"nothing":        sum(func(h *sim.Hasher) {}),
	} {
		if other == a {
			t.Fatalf("%s gave the same hash", name)
		}
	}
}

func TestTwoTextsInARowDoNotRunTogether(t *testing.T) {
	ab := sum(func(h *sim.Hasher) { h.String("ab"); h.String("c") })
	a := sum(func(h *sim.Hasher) { h.String("a"); h.String("bc") })
	if ab == a {
		t.Fatal(`("ab","c") and ("a","bc") gave the same hash`)
	}
	long := sum(func(h *sim.Hasher) { h.String("0123456789abcdef!") })
	if long == sum(func(h *sim.Hasher) { h.String("0123456789abcdef?") }) {
		t.Fatal("a difference in the tail of a long text was lost")
	}
}

func TestBytesAreHashedAsTheTextOfTheSameBytes(t *testing.T) {
	for _, text := range []string{"", "a", "eight by", "more than eight bytes"} {
		asText := sum(func(h *sim.Hasher) { h.String(text) })
		if asText != sum(func(h *sim.Hasher) { h.Bytes([]byte(text)) }) {
			t.Errorf("%q: the bytes hash differently from the text", text)
		}
	}
}
