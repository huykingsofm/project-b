package state_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/testing/must"
)

func TestAnEntityIDAllocatorGivesIDsOfItsOwnerAndRefusesOnesOfAnother(t *testing.T) {
	allocator := state.NewEntityIDAllocator(5, 2)
	one := must.Ok(allocator.Give())
	if one != state.NewEntityID(5, 1) || !allocator.IsLive(one) {
		t.Fatalf("Give = %d", one)
	}
	other := state.NewEntityID(6, one.Local())
	if allocator.IsLive(other) || allocator.Release(other) {
		t.Fatal("an ID of another state was taken for one of the owner")
	}
	if !allocator.Release(one) || allocator.IsLive(one) {
		t.Fatal("an ID of the owner was not given back")
	}
}

func TestEntityIDsComeBackLiveOnlyWhenTheStateMarksThem(t *testing.T) {
	allocator := state.NewEntityIDAllocator(5, 2)
	one, two := must.Ok(allocator.Give()), must.Ok(allocator.Give())
	allocator.Release(one)
	var w binary.Writer
	allocator.Encode(&w)
	got, err := state.DecodeEntityIDAllocator(5, binary.NewReader(w.B))
	if err != nil {
		t.Fatal(err)
	}
	if got.IsLive(two) {
		t.Fatal("an ID is live before the state marks it")
	}
	if err := got.MarkLive(two); err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]state.EntityID{
		"of another state": state.NewEntityID(6, 2),
		"in the grave":     one,
		"marked already":   two,
		"never given":      state.NewEntityID(5, 9),
	} {
		if err := got.MarkLive(id); err == nil {
			t.Errorf("an ID %s was marked live", name)
		}
	}
}

func TestTheHashOfAnEntityIDAllocatorSeesTheGrave(t *testing.T) {
	hash := func(a *state.EntityIDAllocator) uint64 {
		h := sim.NewHasher()
		a.Hash(&h)
		return h.Sum()
	}
	a, b := state.NewEntityIDAllocator(5, 2), state.NewEntityIDAllocator(5, 2)
	for range 3 {
		a.Give()
		b.Give()
	}
	if hash(&a) != hash(&b) {
		t.Fatal("the same IDs hash differently")
	}
	a.Release(state.NewEntityID(5, 2))
	if hash(&a) == hash(&b) {
		t.Fatal("an ID in the grave does not change the hash")
	}
	b.Release(state.NewEntityID(5, 2))
	if hash(&a) != hash(&b) {
		t.Fatal("the same grave hashes differently")
	}
}
