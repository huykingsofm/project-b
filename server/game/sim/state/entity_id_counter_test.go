package state_test

import (
	"math"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func TestTheCounterGivesTheIDsOfItsOwnerFromOneAndNeverAgain(t *testing.T) {
	owner := sim.NewStateID(3, 9)
	c := state.NewEntityIDCounter(owner)
	for want := uint64(1); want <= 3; want++ {
		id, ok := c.Give()
		if !ok || id != state.NewEntityID(owner, want) || id.Owner != owner {
			t.Fatalf("the %dth ID is %v, %v", want, id, ok)
		}
	}
	if c.Owner() != owner {
		t.Fatalf("Owner = %v", c.Owner())
	}
}

func TestTheCounterStopsAtTheEndOfTheNumbers(t *testing.T) {
	owner := sim.NewStateID(3, 9)
	var w binary.Writer
	w.Uvarint(math.MaxUint64 - 1)
	c, err := state.DecodeEntityIDCounter(owner, binary.NewReader(w.B))
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := c.Give(); !ok || id.Local != math.MaxUint64 {
		t.Fatalf("the last number was refused: %v, %v", id, ok)
	}
	if id, ok := c.Give(); ok || id != (state.EntityID{}) {
		t.Fatalf("a number after the last was given: %v", id)
	}
}

func TestTheCounterWritesAndReadsBackAndACloneSharesNothing(t *testing.T) {
	owner := sim.NewStateID(3, 9)
	c := state.NewEntityIDCounter(owner)
	c.Give()
	c.Give()
	clone := c.Clone()
	c.Give()
	var w binary.Writer
	clone.Encode(&w)
	back, err := state.DecodeEntityIDCounter(owner, binary.NewReader(w.B))
	next, _ := back.Give()
	if err != nil || next.Local != 3 {
		t.Fatalf("what was read back gives the number %d, error %v, want 3", next.Local, err)
	}
	if again, _ := clone.Give(); again.Local != 3 {
		t.Fatalf("the clone gives the number %d, want 3", again.Local)
	}
	a, b := sim.NewHasher(), sim.NewHasher()
	clone.Hash(&a)
	back.Hash(&b)
	if a != b {
		t.Fatal("the hash of what was read back is not the hash of what was written")
	}
	if _, err := state.DecodeEntityIDCounter(owner, binary.NewReader(nil)); err == nil {
		t.Fatal("an empty read was accepted")
	}
}

func TestAnEntityIDWritesAndReadsBackAndRefusesWhatNoStateGives(t *testing.T) {
	id := state.NewEntityID(sim.NewStateID(2, 77), 1<<40)
	var w binary.Writer
	id.Encode(&w)
	back, err := state.DecodeEntityID(binary.NewReader(w.B))
	if err != nil || back != id {
		t.Fatalf("read back %v, error %v", back, err)
	}
	for name, e := range map[string]state.EntityID{
		"no owner":  {Local: 1},
		"no number": {Owner: sim.NewStateID(2, 1)},
	} {
		var bad binary.Writer
		e.Encode(&bad)
		if _, err := state.DecodeEntityID(binary.NewReader(bad.B)); err == nil {
			t.Errorf("an ID with %s was read", name)
		}
	}
	if _, err := state.DecodeEntityID(binary.NewReader([]byte{0x81})); err == nil {
		t.Error("a cut ID was read")
	}
}
