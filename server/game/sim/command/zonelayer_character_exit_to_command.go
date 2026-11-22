package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/binarycodec"
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
		&action.ZoneLayerCharacterExitToAction{
			CharacterID: c.CharacterID, ExitEntityID: c.ExitEntityID,
		},
	}
}

// Encode writes the fields of the command, not its ID: sim.EncodeCommand writes the ID first.
func (c ZoneLayerCharacterExitToCommand) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(c.CharacterID))
	w.Uvarint(uint64(c.ExitEntityID))
	return nil
}

// decodeZoneLayerCharacterExitToCommand reads what Encode wrote.
func decodeZoneLayerCharacterExitToCommand(
	r *binarycodec.Reader, version binarycodec.Version,
) (sim.Command[*zonelayerstate.State], error) {
	c := ZoneLayerCharacterExitToCommand{
		CharacterID: state.CharacterID(r.Uvarint()), ExitEntityID: state.EntityID(r.Uvarint()),
	}
	return c, r.Err()
}
