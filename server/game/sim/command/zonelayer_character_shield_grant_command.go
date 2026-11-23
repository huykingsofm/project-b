package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ZoneLayerCharacterShieldGrantCommand gives a character a shield. A skill or an item sends it in a
// game; here it is the command that stands for them.
type ZoneLayerCharacterShieldGrantCommand struct {
	CharacterID state.CharacterID
	Duration    uint64
}

func (ZoneLayerCharacterShieldGrantCommand) ID() sim.CommandID {
	return ZoneLayerCharacterShieldGrantCommandID
}

// Clone returns the command itself: it holds only values.
func (c ZoneLayerCharacterShieldGrantCommand) Clone() sim.Command[*zonelayerstate.State] {
	return c
}

func (c ZoneLayerCharacterShieldGrantCommand) Translate() []sim.Action[*zonelayerstate.State] {
	return []sim.Action[*zonelayerstate.State]{
		&action.ZoneLayerCharacterShieldGrantAction{
			CharacterID: c.CharacterID, Duration: c.Duration,
		},
	}
}

// Encode writes the fields of the command, not its ID: sim.EncodeCommand writes the ID first.
func (c ZoneLayerCharacterShieldGrantCommand) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(c.CharacterID))
	w.Uvarint(c.Duration)
	return nil
}

// decodeZoneLayerCharacterShieldGrantCommand reads what Encode wrote.
func decodeZoneLayerCharacterShieldGrantCommand(
	r *binary.Reader, version codec.Version,
) (sim.Command[*zonelayerstate.State], error) {
	c := ZoneLayerCharacterShieldGrantCommand{
		CharacterID: r.Uint[state.CharacterID](), Duration: r.Uvarint(),
	}
	return c, r.Err()
}
