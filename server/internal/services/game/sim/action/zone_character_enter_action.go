package action

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonestate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

// zoneCharacterEnterActionArrival is the arrival position of a character that enters the world: it is
// not a tile, so the zonelayer puts the character at the spawn point of its map.
var zoneCharacterEnterActionArrival = state.Vec{X: -1, Y: -1}

// ZoneCharacterEnterAction puts a character that entered the world in a zonelayer of the zone: it sends the
// character it carries, with its health and cooldowns as they were, to the first zonelayer that holds fewer characters than its capacity. It
// counts from what the zonelayers published, which may be a few ticks old, and does not wait for the
// zonelayer to answer. When there is no zonelayer with room it does nothing.
type ZoneCharacterEnterAction struct {
	Character zonelayerstate.Character
}

func (a *ZoneCharacterEnterAction) ID() ID          { return ZoneCharacterEnterActionID }
func (a *ZoneCharacterEnterAction) Priority() int16 { return PriorityIntent }
func (a *ZoneCharacterEnterAction) Actor() any      { return a.Character.CharacterID }

// ApplyZone picks a zonelayer and sends it the character.
func (a *ZoneCharacterEnterAction) ApplyZone(_ *Context, s *zonestate.State, r *ZoneRegistry, _ *Plan[ZoneAction]) {
	for zoneLayerID, zoneLayerSlot := range s.ZoneLayers.All() {
		held := 0
		if layer := r.Stable.ZoneLayer(zoneLayerID); layer != nil {
			held = layer.CharacterCount()
		}
		if held >= int(zoneLayerSlot.Capacity) {
			continue
		}
		r.ToZoneLayer(zoneLayerID, OnFailDrop, &ZoneLayerCharacterEnterAction{Character: a.Character, At: zoneCharacterEnterActionArrival})
		return
	}
}
