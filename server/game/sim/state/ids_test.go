package state_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/testing/must"
)

func TestTheEntitiesOfAStateMustBeTheLiveIDsOfItsAllocator(t *testing.T) {
	allocator := state.NewEntityIDAllocator(2)
	one, two := must.Ok(allocator.Give()), must.Ok(allocator.Give())
	both := []state.EntityID{state.NewEntityID(5, one), state.NewEntityID(5, two)}
	if err := state.CheckEntityIDs(5, &allocator, both); err != nil {
		t.Fatalf("an allocator and the entities it gave: %v", err)
	}
	for name, entities := range map[string][]state.EntityID{
		"fewer":             both[:1],
		"an ID never given": {both[0], state.NewEntityID(5, 9)},
		"another owner":     {both[0], state.NewEntityID(6, two)},
	} {
		if err := state.CheckEntityIDs(5, &allocator, entities); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	allocator.Release(two)
	if err := state.CheckEntityIDs(5, &allocator, both); err == nil {
		t.Error("an entity whose ID is in the grave was accepted")
	}
}

func TestTheHashOfTheIDsSeesTheGrave(t *testing.T) {
	hash := func(a *state.EntityIDAllocator) uint64 {
		h := sim.NewHasher()
		state.HashIDAllocator(&h, a)
		return h.Sum()
	}
	a, b := state.NewEntityIDAllocator(2), state.NewEntityIDAllocator(2)
	for range 3 {
		a.Give()
		b.Give()
	}
	if hash(&a) != hash(&b) {
		t.Fatal("the same IDs hash differently")
	}
	a.Release(2)
	if hash(&a) == hash(&b) {
		t.Fatal("an ID in the grave does not change the hash")
	}
	b.Release(2)
	if hash(&a) != hash(&b) {
		t.Fatal("the same grave hashes differently")
	}
}
