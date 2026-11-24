// Package state holds what the states of the world, the zone and the zonelayer of this game have in
// common: the IDs of what lives in them and the plain values that more than one of them uses. The
// state of each is in a package of its own below this one, and its events in a package of its own
// below that.
package state

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec/binary"
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
// on its ground, an exit of a zone. It is made of the ID of the state that holds the entity, its
// owner, and a number that the state counts for itself and never gives again, so it is unique among
// all the entities of the game for good, and an ID that names nothing any more names nothing else.
// The zero value is no entity: an owner is never 0 and a number is counted from 1.
type EntityID struct {
	Owner sim.StateID // the state that holds the entity
	Local uint64      // the number that the owner gave it
}

// NewEntityID makes the ID of the entity that the state owner gave the number local.
func NewEntityID(owner sim.StateID, local uint64) EntityID {
	return EntityID{Owner: owner, Local: local}
}

// String is the ID as the owner and the number, for a log.
func (id EntityID) String() string { return fmt.Sprintf("%d/%d", id.Owner, id.Local) }

// Hash works the ID into a hash: the owner and then the number.
func (id EntityID) Hash(h *sim.Hasher) {
	h.U64(uint64(id.Owner))
	h.U64(id.Local)
}

// Encode writes the ID: the owner and then the number, each as a uvarint.
func (id EntityID) Encode(w *binary.Writer) {
	w.Uvarint(uint64(id.Owner))
	w.Uvarint(id.Local)
}

// DecodeEntityID reads what Encode wrote. It reports an ID that has no owner or no number, which no
// state gives, so a reader finds what is not an ID that was written.
func DecodeEntityID(r *binary.Reader) (EntityID, error) {
	id := EntityID{Owner: sim.StateID(r.Uvarint()), Local: r.Uvarint()}
	if err := r.Err(); err != nil {
		return EntityID{}, err
	}
	if id.Owner == 0 || id.Local == 0 {
		return EntityID{}, fmt.Errorf("%w: %v", ErrNoEntity, id)
	}
	return id, nil
}

// ErrNoEntity is the error of an entity ID that has no owner or no number.
var ErrNoEntity = errors.New("state: an entity ID has no owner or no number")

// ItemID names a kind of item: what a dropped item is, and what an inventory holds.
type ItemID uint32
