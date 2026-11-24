package sim

import (
	"testing"

	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func TestStateIDCountersGiveTheNumbersOfATypeFromOneAndNeverAgain(t *testing.T) {
	c := NewStateIDCounters()
	for want := uint64(1); want <= 5; want++ {
		id, ok := c.Give(3)
		if !ok || id != NewStateID(3, want) || id.Type() != 3 || id.Local() != want {
			t.Fatalf("the %dth ID of the type 3 is %v, %v", want, id, ok)
		}
	}
	if id, _ := c.Give(2); id != NewStateID(2, 1) {
		t.Fatalf("the first ID of the type 2 is %v: a type counts for itself", id)
	}
	if c.Last(3) != 5 || c.Last(2) != 1 || c.Last(9) != 0 {
		t.Fatalf("Last = %d, %d, %d", c.Last(3), c.Last(2), c.Last(9))
	}
}

func TestStateIDCountersRefuseAtTheEndOfTheNumbersOfATypeAndWrapNothing(t *testing.T) {
	c := NewStateIDCounters()
	c.last[4] = maxLocalStateID - 1
	c.ordered = append(c.ordered, 4)
	if id, ok := c.Give(4); !ok || id.Local() != maxLocalStateID {
		t.Fatalf("the last number was refused: %v, %v", id, ok)
	}
	if id, ok := c.Give(4); ok || id != 0 {
		t.Fatalf("a number after the last was given: %v", id)
	}
}

func TestStateIDCountersCloneShareNothingAndWriteAndReadBack(t *testing.T) {
	c := NewStateIDCounters()
	c.Give(7)
	c.Give(2)
	c.Give(7)
	clone := c.Clone()
	c.Give(7)
	if clone.Last(7) != 2 {
		t.Fatalf("a clone follows the original: %d", clone.Last(7))
	}
	var w binary.Writer
	clone.Encode(&w)
	back, err := DecodeStateIDCounters(binary.NewReader(w.B))
	if err != nil || back.Last(7) != 2 || back.Last(2) != 1 {
		t.Fatalf("read back: %v, %v", back.last, err)
	}
	a, b := NewHasher(), NewHasher()
	clone.Hash(&a)
	back.Hash(&b)
	if a != b {
		t.Fatal("the hash of what was read back is not the hash of what was written")
	}
}

func TestDecodeStateIDCountersRefusesWhatCannotBeWritten(t *testing.T) {
	typeAt := func(w *binary.Writer, t StateType, last uint64) {
		w.Uint8(uint8(t))
		w.Uvarint(last)
	}
	for name, build := range map[string]func(w *binary.Writer){
		"a type twice": func(w *binary.Writer) {
			w.Uvarint(2)
			typeAt(w, 3, 1)
			typeAt(w, 3, 1)
		},
		"types out of order": func(w *binary.Writer) {
			w.Uvarint(2)
			typeAt(w, 5, 1)
			typeAt(w, 3, 1)
		},
		"a number of 0": func(w *binary.Writer) {
			w.Uvarint(1)
			typeAt(w, 3, 0)
		},
		"a number too large": func(w *binary.Writer) {
			w.Uvarint(1)
			typeAt(w, 3, 1<<56)
		},
		"more types than bytes": func(w *binary.Writer) { w.Uvarint(1000) },
	} {
		var w binary.Writer
		build(&w)
		if _, err := DecodeStateIDCounters(binary.NewReader(w.B)); err == nil {
			t.Errorf("%s was read", name)
		}
	}
}
