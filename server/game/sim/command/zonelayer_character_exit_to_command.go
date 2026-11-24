package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ZoneLayerCharacterExitToCommand asks to take the exit, an entity of the zonelayer, that the
// character stands in.
type ZoneLayerCharacterExitToCommand struct {
	CharacterID  state.CharacterID
	ExitEntityID state.EntityID
}

func (ZoneLayerCharacterExitToCommand) ID() sim.CommandID {
	return ZoneLayerCharacterExitToCommandID
}

// Clone returns the command itself: it holds only values.
func (c ZoneLayerCharacterExitToCommand) Clone() sim.Command[*zonelayerstate.State] { return c }

func (c ZoneLayerCharacterExitToCommand) Translate() []sim.Action[*zonelayerstate.State] {
	return []sim.Action[*zonelayerstate.State]{
		action.ZoneLayerCharacterExitToAction{
			CharacterID: c.CharacterID, ExitEntityID: c.ExitEntityID,
		},
	}
}

// Encode writes the fields of the command, not its ID: sim.EncodeCommand writes the ID first.
func (c ZoneLayerCharacterExitToCommand) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(c.CharacterID))
	c.ExitEntityID.Encode(w)
	return nil
}

// decodeZoneLayerCharacterExitToCommand reads what Encode wrote.
func decodeZoneLayerCharacterExitToCommand(
	r *binary.Reader, version codec.Version,
) (ZoneLayerCharacterExitToCommand, error) {
	characterID := r.Uint[state.CharacterID]()
	exitEntityID, err := state.DecodeEntityID(r)
	if err != nil {
		return ZoneLayerCharacterExitToCommand{}, err
	}
	return ZoneLayerCharacterExitToCommand{
		CharacterID: characterID, ExitEntityID: exitEntityID,
	}, r.Err()
}
