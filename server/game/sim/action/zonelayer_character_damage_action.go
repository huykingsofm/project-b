package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// ZoneLayerCharacterDamageAction takes health from an entity. It is in the plan after the action
// that caused it and after the effects that may change it, and an effect that runs before it may
// lower it or cancel it.
type ZoneLayerCharacterDamageAction struct {
	SourceID, VictimID state.EntityID
	Amount             int32
}

func (a *ZoneLayerCharacterDamageAction) ID() sim.ActionID {
	return ZoneLayerCharacterDamageActionID
}
func (a *ZoneLayerCharacterDamageAction) Priority() int16 { return PriorityResolve }

// Apply hurts the victim.
func (a *ZoneLayerCharacterDamageAction) Apply(
	_ *sim.Context, s *zonelayerstate.State, _ *sim.Registry[*zonelayerstate.State],
	_ *sim.Plan[*zonelayerstate.State],
) {
	if a.Amount <= 0 {
		return
	}
	s.DamageEntity(a.VictimID, a.Amount)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a *ZoneLayerCharacterDamageAction) Encode(w *binarycodec.Writer) {
	w.Uvarint(uint64(a.SourceID))
	w.Uvarint(uint64(a.VictimID))
	w.Varint(int64(a.Amount))
}

// decodeZoneLayerCharacterDamageAction reads what Encode wrote.
func decodeZoneLayerCharacterDamageAction(
	r *binarycodec.Reader, version binarycodec.Version,
) (sim.Action[*zonelayerstate.State], error) {
	a := &ZoneLayerCharacterDamageAction{
		SourceID: state.EntityID(r.Uvarint()),
		VictimID: state.EntityID(r.Uvarint()),
		Amount:   int32(r.Varint()),
	}
	return a, r.Err()
}
