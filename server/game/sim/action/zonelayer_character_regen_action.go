package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// regenAmount is the health that every living entity gets each time. It is 1 for now.
const regenAmount = 1

// ZoneLayerCharacterRegenAction gives a little health to every living entity, and sets itself again
// Every ticks after. It is how a zonelayer heals as time goes: one such action waits in the
// scheduler, and the zonelayer is made to heal by setting the first. Every 0 heals once.
type ZoneLayerCharacterRegenAction struct {
	Every uint64
}

func (a *ZoneLayerCharacterRegenAction) ID() sim.ActionID { return ZoneLayerCharacterRegenActionID }
func (a *ZoneLayerCharacterRegenAction) Priority() int16  { return PrioritySystem }
func (a *ZoneLayerCharacterRegenAction) Actor() any       { return nil }

// Clone returns a copy that shares nothing with the action.
func (a *ZoneLayerCharacterRegenAction) Clone() sim.Action[*zonelayerstate.State] {
	c := *a
	return &c
}

// Apply heals everyone and sets the action again.
func (a *ZoneLayerCharacterRegenAction) Apply(
	c *sim.Context, s *zonelayerstate.State, r *sim.Registry[*zonelayerstate.State],
	_ *sim.Plan[*zonelayerstate.State],
) {
	s.HealAllEntities(regenAmount)
	if a.Every > 0 {
		r.ToSelfAt(c.Tick+a.Every, a)
	}
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a *ZoneLayerCharacterRegenAction) Encode(w *binary.Writer) {
	w.Uvarint(a.Every)
}

// decodeZoneLayerCharacterRegenAction reads what Encode wrote.
func decodeZoneLayerCharacterRegenAction(
	r *binary.Reader, version codec.Version,
) (*ZoneLayerCharacterRegenAction, error) {
	a := &ZoneLayerCharacterRegenAction{Every: r.Uvarint()}
	return a, r.Err()
}
