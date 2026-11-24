package action

import (
	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

var (
	worldCharacterItemAddActionInventorySlotLimit = config.NewVar(
		"world.inventory.slots", 20, config.Range(1, 1000),
	)
	worldCharacterItemAddActionStackLimit = config.NewVar(
		"world.inventory.stack_limit", 99, config.Range(1, 100000),
	)
)

// WorldCharacterItemAddAction puts items in the inventory of a character. It is a message from a
// zonelayer: the items of a character belong to the world, and the zonelayer that gives them up has
// already taken them off its ground (R28).
//
// What does not fit in the inventory goes back to the zonelayer it came from, to the same place
// (ZoneLayerItemEntityAddAction), so nothing is lost by a full inventory.
type WorldCharacterItemAddAction struct {
	CharacterID state.CharacterID
	ItemID      state.ItemID
	Count       uint32

	// Where the items came from, so what does not fit can be put back.
	FromZoneLayerID sim.StateID
	At              state.Vec
	Expiry          uint64
}

func (a *WorldCharacterItemAddAction) ID() sim.ActionID { return WorldCharacterItemAddActionID }
func (a *WorldCharacterItemAddAction) Priority() int16  { return PriorityResolve }
func (a *WorldCharacterItemAddAction) Actor() any       { return a.CharacterID }

// Clone returns a copy that shares nothing with the action.
func (a *WorldCharacterItemAddAction) Clone() sim.Action[*worldstate.State] {
	c := *a
	return &c
}

// Apply adds the items and sends back the ones that did not fit.
func (a *WorldCharacterItemAddAction) Apply(
	c *sim.Context, s *worldstate.State, r *sim.Registry[*worldstate.State],
	_ *sim.Plan[*worldstate.State],
) {
	left := s.AddItemToCharacter(a.CharacterID, a.ItemID, a.Count,
		worldCharacterItemAddActionInventorySlotLimit.Get(c.Cfg), uint32(
			worldCharacterItemAddActionStackLimit.Get(c.Cfg),
		))
	if left == 0 {
		return
	}
	r.Send(
		a.FromZoneLayerID,
		onfail.Drop{},
		&ZoneLayerItemEntityAddAction{ItemID: a.ItemID, Count: left, At: a.At, Expiry: a.Expiry},
	)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a *WorldCharacterItemAddAction) Encode(w *binary.Writer) {
	w.Uvarint(uint64(a.CharacterID))
	w.Uvarint(uint64(a.ItemID))
	w.Uvarint(uint64(a.Count))
	w.Uvarint(uint64(a.FromZoneLayerID))
	a.At.Encode(w)
	w.Uvarint(a.Expiry)
}

// decodeWorldCharacterItemAddAction reads what Encode wrote.
func decodeWorldCharacterItemAddAction(
	r *binary.Reader, version codec.Version,
) (*WorldCharacterItemAddAction, error) {
	a := &WorldCharacterItemAddAction{
		CharacterID:     r.Uint[state.CharacterID](),
		ItemID:          r.Uint[state.ItemID](),
		Count:           r.Uint[uint32](),
		FromZoneLayerID: r.Uint[sim.StateID](),
		At:              state.DecodeVec(r, version),
		Expiry:          r.Uvarint(),
	}
	return a, r.Err()
}
