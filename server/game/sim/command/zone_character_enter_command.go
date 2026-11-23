package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ZoneCharacterEnterCommand asks to put a character in the world, in this zone.
type ZoneCharacterEnterCommand struct {
	Character zonelayerstate.Character
}

func (ZoneCharacterEnterCommand) ID() sim.CommandID { return ZoneCharacterEnterCommandID }

// Clone returns a copy of the command that shares nothing with it: the character holds cooldowns.
func (c ZoneCharacterEnterCommand) Clone() sim.Command[*zonestate.State] {
	c.Character = c.Character.Clone()
	return c
}

func (c ZoneCharacterEnterCommand) Translate() []sim.Action[*zonestate.State] {
	return []sim.Action[*zonestate.State]{&action.ZoneCharacterEnterAction{Character: c.Character}}
}

// Encode writes the fields of the command, not its ID: sim.EncodeCommand writes the ID first.
func (c ZoneCharacterEnterCommand) Encode(w *binary.Writer) error {
	c.Character.Encode(w)
	return nil
}

// decodeZoneCharacterEnterCommand reads what Encode wrote.
func decodeZoneCharacterEnterCommand(
	r *binary.Reader, version codec.Version,
) (sim.Command[*zonestate.State], error) {
	ch, err := zonelayerstate.DecodeCharacter(r, version)
	return ZoneCharacterEnterCommand{Character: ch}, err
}
