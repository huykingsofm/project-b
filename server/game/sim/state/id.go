// Package state holds what the states of the world, the zone and the zonelayer of this game have in
// common: the IDs of what lives in them and the plain values that more than one of them uses. The
// state of each is in a package of its own below this one, and its events in a package of its own
// below that.
package state

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// The types of the states of the game, which the codecs register. A number is never changed or
// reused, because checkpoints and the event log write it.
const (
	WorldType sim.StateType = iota + 1
	ZoneType
	ZoneLayerType
)

func init() {
	sim.MapStateTypeToName(WorldType, "world")
	sim.MapStateTypeToName(ZoneType, "zone")
	sim.MapStateTypeToName(ZoneLayerType, "zonelayer")
}

// CharacterID names a character: a figure in the game. A character is not always played by a
// person. PlayerID names a player: the person who plays. A character may be linked to a player
// or not.
type (
	CharacterID uint64
	PlayerID    uint64
)

// EntityID names an entity that stands in a state: the body a character has in a zonelayer, an item
// on its ground, an exit of a zone. It is the number that the state counts for itself and never
// gives again, from 1, so an ID names one entity among those of its own state for good; it names
// none among those of another state. A command or an action that names an entity of another state
// holds the ID of that state beside it (0: the state it is run on). 0 is no entity.
type EntityID uint64

// DecodeEntityID reads what Encode wrote.
func DecodeEntityID(r *binary.Reader) (EntityID, error) {
	id := EntityID(r.Uvarint())
	return id, r.Err()
}

// String is the number, for a log.
func (id EntityID) String() string { return fmt.Sprintf("%d", uint64(id)) }

// Hash works the ID into a hash.
func (id EntityID) Hash(h *sim.Hasher) { h.U64(uint64(id)) }

// Encode writes the ID as a uvarint.
func (id EntityID) Encode(w *binary.Writer) { w.Uvarint(uint64(id)) }

// ItemID names a kind of item: what a dropped item is, and what an inventory holds.
type ItemID uint32
