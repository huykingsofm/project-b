package sim_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/ids"
)

func giveState(t *testing.T, s *sim.StateIDAllocators, st sim.StateType) sim.StateID {
	t.Helper()
	id, ok := s.Give(st)
	if !ok {
		t.Fatalf("Give(%d): no ID", st)
	}
	return id
}

func TestEachTypeOfStateCountsItsOwnIDs(t *testing.T) {
	s := sim.NewStateIDAllocators(2)
	a1, b1, a2 := giveState(t, &s, 3), giveState(t, &s, 5), giveState(t, &s, 3)
	if a1 != sim.NewStateID(3, 1) || a2 != sim.NewStateID(3, 2) || b1 != sim.NewStateID(5, 1) {
		t.Fatalf("the IDs are %d, %d and %d", a1, a2, b1)
	}
	if s.LiveCount(3) != 2 || s.LiveCount(5) != 1 || s.LiveCount(9) != 0 {
		t.Fatalf("LiveCount = %d, %d and %d", s.LiveCount(3), s.LiveCount(5), s.LiveCount(9))
	}
}

func TestAStateIDThatIsGivenBackWaitsInTheGrave(t *testing.T) {
	s := sim.NewStateIDAllocators(2)
	id := giveState(t, &s, 3)
	if !s.Release(id) || s.Status(id) != ids.InGrave {
		t.Fatalf("Status after Release = %v", s.Status(id))
	}
	if s.Release(id) || s.Release(sim.NewStateID(9, 1)) {
		t.Fatal("an ID that is not live was given back")
	}
	s.Step()
	s.Step()
	if s.Status(id) != ids.NotGiven {
		t.Fatalf("Status after the steps = %v", s.Status(id))
	}
}

func TestStateIDsSurviveEncodeAndDecodeAndAreRefusedOutOfOrder(t *testing.T) {
	s := sim.NewStateIDAllocators(4)
	giveState(t, &s, 5)
	gone := giveState(t, &s, 3)
	giveState(t, &s, 3)
	s.Release(gone)
	var w binary.Writer
	s.Encode(&w)
	got, err := sim.DecodeStateIDAllocators(binary.NewReader(w.B))
	if err != nil {
		t.Fatal(err)
	}
	if got.IsLive(sim.NewStateID(5, 1)) {
		t.Fatal("an ID is live before its owner marks it")
	}
	for _, id := range []sim.StateID{sim.NewStateID(5, 1), sim.NewStateID(3, 2)} {
		if err := got.MarkLive(id); err != nil {
			t.Fatal(err)
		}
	}
	for name, id := range map[string]sim.StateID{
		"in the grave": gone, "marked already": sim.NewStateID(5, 1),
		"of a type never given": sim.NewStateID(9, 1),
	} {
		if err := got.MarkLive(id); err == nil {
			t.Errorf("an ID %s was marked live", name)
		}
	}
	var again binary.Writer
	got.Encode(&again)
	if string(again.B) != string(w.B) || got.Status(gone) != ids.InGrave {
		t.Fatal("what was read is not what was written")
	}
	hash := func(s *sim.StateIDAllocators) uint64 {
		h := sim.NewHasher()
		s.Hash(&h)
		return h.Sum()
	}
	if hash(&got) != hash(&s) {
		t.Fatal("the same IDs hash differently")
	}
	for n := range w.B {
		if _, err := sim.DecodeStateIDAllocators(binary.NewReader(w.B[:n])); err == nil {
			t.Fatalf("a cut at %d bytes of %d was read", n, len(w.B))
		}
	}
	empty := ids.New[uint32](4, 1<<24-1)
	var bad binary.Writer
	bad.Uvarint(4)
	bad.Uvarint(2)
	bad.Uint8(5)
	empty.Encode(&bad)
	bad.Uint8(3) // a smaller type after a larger one
	empty.Encode(&bad)
	if _, err := sim.DecodeStateIDAllocators(binary.NewReader(bad.B)); err == nil {
		t.Fatal("types out of order were read")
	}
}

func TestACloneOfStateIDsSharesNothing(t *testing.T) {
	s := sim.NewStateIDAllocators(2)
	id := giveState(t, &s, 3)
	c := s.Clone()
	s.Release(id)
	if c.Status(id) != ids.Live {
		t.Fatalf("giving an ID back changed the clone: %v", c.Status(id))
	}
	giveState(t, &c, 7)
	if s.LiveCount(7) != 0 {
		t.Fatal("giving an ID in the clone changed the original")
	}
}
