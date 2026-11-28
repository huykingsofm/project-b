package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ZoneLayerCharacterDamageAction takes health from an entity. It is in the plan after the action
// that caused it and after the effects that may change it, and an effect that runs before it may
// lower it or cancel it.
type ZoneLayerCharacterDamageAction struct {
	SourceID, VictimID state.EntityID
	Amount             int32
}

// decodeZoneLayerCharacterDamageAction reads what Encode wrote.
func decodeZoneLayerCharacterDamageAction(
	r *binary.Reader, version codec.Version,
) (ZoneLayerCharacterDamageAction, error) {
	sourceID, err := state.DecodeEntityID(r)
	if err != nil {
		return ZoneLayerCharacterDamageAction{}, err
	}
	victimID, err := state.DecodeEntityID(r)
	if err != nil {
		return ZoneLayerCharacterDamageAction{}, err
	}
	a := ZoneLayerCharacterDamageAction{
		SourceID: sourceID, VictimID: victimID, Amount: r.Int[int32](),
	}
	return a, r.Err()
}

func (a ZoneLayerCharacterDamageAction) ID() sim.ActionID {
	return ZoneLayerCharacterDamageActionID
}

func (a ZoneLayerCharacterDamageAction) Priority() int16 { return PriorityResolve }

func (a ZoneLayerCharacterDamageAction) Actor() any { return nil }

// Clone returns the action itself: it holds only values.
func (a ZoneLayerCharacterDamageAction) Clone() sim.Action[*zonelayerstate.State] {
	return a
}

// Apply hurts the victim.
func (a ZoneLayerCharacterDamageAction) Apply(
	_ *sim.Context, s *zonelayerstate.State, _ *sim.ActionRegistry[*zonelayerstate.State],
	_ *sim.Plan[*zonelayerstate.State],
) {
	if a.Amount <= 0 {
		return
	}
	s.DamageEntity(a.VictimID, a.Amount)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a ZoneLayerCharacterDamageAction) Encode(w *binary.Writer) {
	a.SourceID.Encode(w)
	a.VictimID.Encode(w)
	w.Varint(int64(a.Amount))
}
