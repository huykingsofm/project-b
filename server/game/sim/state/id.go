// Package state holds what the states of the world, the zone and the zonelayer of this game have in
// common: the IDs of what lives in them and the plain values that more than one of them uses. The
// state of each is in a package of its own below this one, and its events in a package of its own
// below that.
package state

import (
	"github.com/huykingsofm/project-b/platform/sim"
)

// The types of the states of the game, which the codecs register. A number is never changed or
// reused, because checkpoints and the replay log write it.
const (
	WorldType sim.StateType = iota + 1
	ZoneType
	ZoneLayerType
)

func init() {
	for t, name := range map[sim.StateType]string{
		WorldType: "world", ZoneType: "zone", ZoneLayerType: "zonelayer",
	} {
		if err := sim.MapStateTypeToName(t, name); err != nil {
			panic(err) // a name that is wrong is found when the program starts
		}
	}
}

// CharacterID names a character: a figure in the game. A character is not always played by a
// person. PlayerID names a player: the person who plays. A character may be linked to a player
// or not.
type (
	CharacterID uint64
	PlayerID    uint64
)

// EntityID names an entity that stands in a state: the body a character has in a zonelayer, an item
// on its ground, an exit of a zone. It is made of the ID of the state that holds the entity and a
// number that the state counts for itself, so it is unique among all the entities of the game, and
// two states never give the same one.
type EntityID uint64

// NewEntityID makes the ID of the entity that a state gave the number local. The ID of a state is
// less than 2^32.
func NewEntityID(owner sim.StateID, local uint32) EntityID {
	return EntityID(uint64(owner)<<32 | uint64(local))
}

// Owner is the ID of the state that holds the entity.
func (id EntityID) Owner() sim.StateID { return sim.StateID(uint64(id) >> 32) }

// Local is the number that the state that holds the entity gave it.
func (id EntityID) Local() uint32 { return uint32(id) }

// ItemID names a kind of item: what a dropped item is, and what an inventory holds.
type ItemID uint32
