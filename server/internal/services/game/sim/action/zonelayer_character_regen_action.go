package action

import (
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// regenAmount is the health that every living entity gets each time. It is 1 for now.
const regenAmount = 1

// ZoneLayerCharacterRegenAction gives a little health to every living entity, and sets itself again
// Every ticks after. It is how a zonelayer heals as time goes: one such action waits in the
// scheduler, and the zonelayer is made to heal by setting the first. Every 0 heals once.
type ZoneLayerCharacterRegenAction struct {
	Every uint64
}

func (a *ZoneLayerCharacterRegenAction) ID() ID          { return ZoneLayerCharacterRegenActionID }
func (a *ZoneLayerCharacterRegenAction) Priority() int16 { return PrioritySystem }

// ApplyZoneLayer heals everyone and sets the action again.
func (a *ZoneLayerCharacterRegenAction) ApplyZoneLayer(
	c *Context, s *zonelayerstate.State, r *ZoneLayerRegistry, _ *Plan[ZoneLayerAction],
) {
	s.HealAllEntities(regenAmount)
	if a.Every > 0 {
		r.ToSelfAt(c.Tick+a.Every, a)
	}
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a *ZoneLayerCharacterRegenAction) Encode(w *binarycodec.Writer) {
	w.Uvarint(a.Every)
}

// decodeZoneLayerCharacterRegenAction reads what Encode wrote.
func decodeZoneLayerCharacterRegenAction(
	r *binarycodec.Reader, version binarycodec.Version,
) (Action, error) {
	a := &ZoneLayerCharacterRegenAction{Every: r.Uvarint()}
	return a, r.Err()
}
