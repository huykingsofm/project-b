package command

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// ZoneCharacterEnterCommand asks to put a character in the world, in this zone.
type ZoneCharacterEnterCommand struct {
	Character zonelayerstate.Character
}

func (ZoneCharacterEnterCommand) ID() ID { return ZoneCharacterEnterCommandID }

func (c ZoneCharacterEnterCommand) TranslateZone() []action.ZoneAction {
	return []action.ZoneAction{&action.ZoneCharacterEnterAction{Character: c.Character}}
}

// Encode writes the fields of the command, not its ID: package command writes the ID first.
func (c ZoneCharacterEnterCommand) Encode(w *binarycodec.Writer) error {
	c.Character.Encode(w)
	return nil
}

// decodeZoneCharacterEnterCommand reads what Encode wrote.
func decodeZoneCharacterEnterCommand(r *binarycodec.Reader, version binarycodec.Version) (Command, error) {
	ch, err := zonelayerstate.DecodeCharacter(r, version)
	return ZoneCharacterEnterCommand{Character: ch}, err
}
