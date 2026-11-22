package action

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

// ZoneLayerCharacterEnterAction puts a character in the zonelayer: gives it a body at a position.
// It is the one way in: a character that enters the world, a character that comes through an exit
// (ZoneLayerCharacterExitToAction), and a character that is put back after it could not go, all
// arrive this way.
//
// It needs the map, to see whether the body fits at the position and where the spawn point is, and
// reads it from the state of the zone. The character is in the message and nowhere else, so when
// the zone has not published its state yet the action tries again in the next tick.
type ZoneLayerCharacterEnterAction struct {
	Character zonelayerstate.Character
	At        state.Vec
}

func (a *ZoneLayerCharacterEnterAction) ID() ID          { return ZoneLayerCharacterEnterActionID }
func (a *ZoneLayerCharacterEnterAction) Priority() int16 { return PriorityResolve }
func (a *ZoneLayerCharacterEnterAction) Actor() any      { return a.Character.CharacterID }

// ApplyZoneLayer gives the character a body.
func (a *ZoneLayerCharacterEnterAction) ApplyZoneLayer(
	_ *Context, s *zonelayerstate.State, r *ZoneLayerRegistry, _ *Plan[ZoneLayerAction],
) {
	if _, held := s.CharacterByID(a.Character.CharacterID); held {
		return
	}
	zone := r.Stable.Zone(s.ZoneID)
	if zone == nil {
		r.ToSelf(a)
		return
	}
	at := a.At
	if !zone.Tiles.FreeForBody(at, a.Character.Radius) {
		at = zone.Tiles.SpawnPosition()
	}
	s.AddCharacterEntity(a.Character, at)
}
