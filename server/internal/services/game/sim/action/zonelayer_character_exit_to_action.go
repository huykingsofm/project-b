package action

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

// ZoneLayerCharacterExitToActionID is the ID of ZoneLayerCharacterExitToAction.
type ZoneLayerCharacterExitToActionID struct{}

// ZoneLayerCharacterExitToAction is a character taking the exit it stands in, to a zonelayer of the zone
// the exit leads to. The exit is optimistic: the character moves at once, when the zonelayer it is going
// to has room, and does not wait for that zonelayer to answer.
//
// The exit is an entity of the zone, chosen by its entity ID, and the zone is read from what it published. It looks at what is published, never
// at the other zonelayer itself: the state of the zone the exit leads to for its zonelayers and their capacity, and the state
// of each of those zonelayers for how many characters it holds. What it reads may be a few ticks old,
// so two characters that go at the same tick may both find room for one, and a zonelayer may hold a
// character more than its capacity for a moment. The first zonelayer, in the order the zone has them,
// that holds fewer characters than its capacity is the one that is chosen. When there is none, or
// anything it needs is not published, the character stays where it is and nothing happens.
type ZoneLayerCharacterExitToAction struct {
	CharacterID  state.CharacterID
	ExitEntityID state.EntityID
}

func (a *ZoneLayerCharacterExitToAction) ID() ID          { return ZoneLayerCharacterExitToActionID{} }
func (a *ZoneLayerCharacterExitToAction) Priority() int16 { return PriorityIntent }
func (a *ZoneLayerCharacterExitToAction) Actor() any      { return a.CharacterID }

// ApplyZoneLayer takes the character out and sends it to the zonelayer that has room.
func (a *ZoneLayerCharacterExitToAction) ApplyZoneLayer(_ *Context, s *zonelayerstate.State, r *ZoneLayerRegistry, _ *Plan[ZoneLayerAction]) {
	ch, ok := s.CharacterByID(a.CharacterID)
	if !ok || ch.Health <= 0 {
		return
	}
	here := r.Stable.Zone(s.ZoneID)
	if here == nil {
		return
	}
	body, _ := s.CharacterEntityByCharacterID(a.CharacterID)
	exit, found := here.ExitEntityByID(a.ExitEntityID)
	if !found || !exit.Contains(body.Pos) {
		return
	}
	there := r.Stable.Zone(exit.ToZoneID)
	if there == nil {
		return
	}
	for zoneLayerID, zoneLayerSlot := range there.ZoneLayers.All() {
		held := 0
		if layer := r.Stable.ZoneLayer(zoneLayerID); layer != nil {
			held = layer.CharacterCount()
		}
		if held >= int(zoneLayerSlot.Capacity) {
			continue
		}
		character, _ := s.RemoveCharacterEntity(a.CharacterID)
		// If the message cannot be delivered the character is put back where it was, by the same
		// action that puts it anywhere.
		selfID, back := s.ID, body.Pos
		r.ToZoneLayer(zoneLayerID, OnFailCompensate(func(Reason) Message {
			return MessageToZoneLayer(selfID, OnFailDrop, &ZoneLayerCharacterEnterAction{Character: character, At: back})
		}), &ZoneLayerCharacterEnterAction{Character: character, At: exit.Arrival})
		return
	}
}
