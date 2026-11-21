package command

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// ZoneLayerCharacterShieldGrantCommand gives a character a shield. A skill or an item sends it in a game; here
// it is the command that stands for them.
type ZoneLayerCharacterShieldGrantCommand struct {
	CharacterID state.CharacterID
	Duration    uint64
}

func (ZoneLayerCharacterShieldGrantCommand) ID() ID { return ZoneLayerCharacterShieldGrantCommandID }

func (c ZoneLayerCharacterShieldGrantCommand) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{&action.ZoneLayerCharacterShieldGrantAction{CharacterID: c.CharacterID, Duration: c.Duration}}
}

// Encode writes the fields of the command, not its ID: package command writes the ID first.
func (c ZoneLayerCharacterShieldGrantCommand) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(c.CharacterID))
	w.Uvarint(c.Duration)
	return nil
}

// decodeZoneLayerCharacterShieldGrantCommand reads what Encode wrote.
func decodeZoneLayerCharacterShieldGrantCommand(r *binarycodec.Reader, version binarycodec.Version) (Command, error) {
	c := ZoneLayerCharacterShieldGrantCommand{CharacterID: state.CharacterID(r.Uvarint()), Duration: r.Uvarint()}
	return c, r.Err()
}
