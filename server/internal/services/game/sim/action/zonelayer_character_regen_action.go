package action

import (
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// regenAmount is the health that every living entity gets each time. It is 1 for now.
const regenAmount = 1

// ZoneLayerCharacterRegenAction gives a little health to every living entity. It has no state, so
// one value of it is the action of a cron and runs every time the cron is due.
type ZoneLayerCharacterRegenAction struct{}

func (a *ZoneLayerCharacterRegenAction) ID() ID          { return ZoneLayerCharacterRegenActionID }
func (a *ZoneLayerCharacterRegenAction) Priority() int16 { return PrioritySystem }

// ApplyZoneLayer heals everyone.
func (a *ZoneLayerCharacterRegenAction) ApplyZoneLayer(
	_ *Context, s *zonelayerstate.State, _ *ZoneLayerRegistry, _ *Plan[ZoneLayerAction],
) {
	s.HealAllEntities(regenAmount)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a *ZoneLayerCharacterRegenAction) Encode(w *binarycodec.Writer) {
	_ = w
}

// decodeZoneLayerCharacterRegenAction reads what Encode wrote.
func decodeZoneLayerCharacterRegenAction(
	r *binarycodec.Reader, version binarycodec.Version,
) (Action, error) {
	a := &ZoneLayerCharacterRegenAction{}
	return a, r.Err()
}
