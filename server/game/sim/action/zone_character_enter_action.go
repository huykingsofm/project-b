package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// zoneCharacterEnterActionArrival is the arrival position of a character that enters the world: it
// is not a tile, so the zonelayer puts the character at the spawn point of its map.
var zoneCharacterEnterActionArrival = state.Vec{X: -1, Y: -1}

// ZoneCharacterEnterAction puts a character that entered the world in a zonelayer of the zone: it
// sends the character it carries, with its health and cooldowns as they were, to the first
// zonelayer that holds fewer characters than its capacity. It counts from what the zonelayers
// published, which may be a few ticks old, and does not wait for the zonelayer to answer. When
// there is no zonelayer with room it does nothing.
type ZoneCharacterEnterAction struct {
	Character zonelayerstate.Character
}

// decodeZoneCharacterEnterAction reads what Encode wrote.
func decodeZoneCharacterEnterAction(
	r *binary.Reader, version codec.Version,
) (ZoneCharacterEnterAction, error) {
	c, err := zonelayerstate.DecodeCharacter(r, version)
	if err != nil {
		return ZoneCharacterEnterAction{}, err
	}
	a := ZoneCharacterEnterAction{Character: c}
	return a, r.Err()
}

func (a ZoneCharacterEnterAction) ID() sim.ActionID { return ZoneCharacterEnterActionID }

func (a ZoneCharacterEnterAction) Priority() int16 { return PriorityIntent }

func (a ZoneCharacterEnterAction) Actor() any { return a.Character.CharacterID() }

// Clone returns the action with a clone of the character it holds.
func (a ZoneCharacterEnterAction) Clone() sim.Action[*zonestate.State] {
	a.Character = a.Character.Clone()
	return a
}

// Apply picks a zonelayer and sends it the character.
func (a ZoneCharacterEnterAction) Apply(
	_ *sim.Context, s *zonestate.State, r *sim.ActionRegistry[*zonestate.State],
	_ *sim.Plan[*zonestate.State],
) {
	for zoneLayerID, zoneLayerSlot := range s.ZoneLayerSlots() {
		held := 0
		if layer, err := r.Get[zonelayerstate.ReadOnlyZoneLayerState](zoneLayerID); err == nil {
			held = layer.CharacterCount()
		}
		if held >= int(zoneLayerSlot.Capacity()) {
			continue
		}
		r.Send(
			zoneLayerID,
			onfail.Drop{},
			ZoneLayerCharacterEnterAction{
				Character: a.Character, At: zoneCharacterEnterActionArrival,
			},
		)
		return
	}
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a ZoneCharacterEnterAction) Encode(w *binary.Writer) {
	a.Character.Encode(w)
}
