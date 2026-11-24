package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ZoneLayerCharacterMoveCommand asks to hold a direction for a number of ticks.
type ZoneLayerCharacterMoveCommand struct {
	CharacterID state.CharacterID
	Dir         state.Dir
	Ticks       uint32
}

func (ZoneLayerCharacterMoveCommand) ID() sim.CommandID { return ZoneLayerCharacterMoveCommandID }

// Clone returns the command itself: it holds only values.
func (c ZoneLayerCharacterMoveCommand) Clone() sim.Command[*zonelayerstate.State] { return c }

func (c ZoneLayerCharacterMoveCommand) Translate() []sim.Action[*zonelayerstate.State] {
	return []sim.Action[*zonelayerstate.State]{
		action.ZoneLayerCharacterMoveAction{
			CharacterID: c.CharacterID, Dir: c.Dir, Ticks: c.Ticks,
		},
	}
}

// Encode writes the fields of the command, not its ID: sim.EncodeCommand writes the ID first.
func (c ZoneLayerCharacterMoveCommand) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(c.CharacterID))
	w.Uint8(uint8(c.Dir))
	w.Uvarint(uint64(c.Ticks))
	return nil
}

// decodeZoneLayerCharacterMoveCommand reads what Encode wrote.
func decodeZoneLayerCharacterMoveCommand(
	r *binary.Reader, version codec.Version,
) (ZoneLayerCharacterMoveCommand, error) {
	c := ZoneLayerCharacterMoveCommand{
		CharacterID: r.Uint[state.CharacterID](),
		Dir:         state.Dir(r.Uint8()),
		Ticks:       r.Uint[uint32](),
	}
	return c, r.Err()
}
