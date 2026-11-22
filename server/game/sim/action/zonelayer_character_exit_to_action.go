package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// ZoneLayerCharacterExitToAction is a character taking the exit it stands in, to a zonelayer of the
// zone the exit leads to. The exit is optimistic: the character moves at once, when the zonelayer
// it is going to has room, and does not wait for that zonelayer to answer.
//
// The exit is an entity of the zone, chosen by its entity ID, and the zone is read from what it
// published. It looks at what is published, never at the other zonelayer itself: the state of the
// zone the exit leads to for its zonelayers and their capacity, and the state of each of those
// zonelayers for how many characters it holds. What it reads may be a few ticks old, so two
// characters that go at the same tick may both find room for one, and a zonelayer may hold a
// character more than its capacity for a moment. The first zonelayer, in the order the zone has
// them, that holds fewer characters than its capacity is the one that is chosen. When there is
// none, or anything it needs is not published, the character stays where it is and nothing happens.
type ZoneLayerCharacterExitToAction struct {
	CharacterID  state.CharacterID
	ExitEntityID state.EntityID
}

func (a *ZoneLayerCharacterExitToAction) ID() sim.ActionID {
	return ZoneLayerCharacterExitToActionID
}
func (a *ZoneLayerCharacterExitToAction) Priority() int16 { return PriorityIntent }
func (a *ZoneLayerCharacterExitToAction) Actor() any      { return a.CharacterID }

// Apply takes the character out and sends it to the zonelayer that has room.
func (a *ZoneLayerCharacterExitToAction) Apply(
	_ *sim.Context, s *zonelayerstate.State, r *sim.Registry[*zonelayerstate.State],
	_ *sim.Plan[*zonelayerstate.State],
) {
	ch, ok := s.CharacterByID(a.CharacterID)
	if !ok || ch.Health <= 0 {
		return
	}
	here, err := r.Get[zonestate.ReadOnlyZoneState](s.ZoneID)
	if err != nil {
		return
	}
	body, _ := s.CharacterEntityByCharacterID(a.CharacterID)
	exit, found := here.ExitEntityByID(a.ExitEntityID)
	if !found || !exit.Contains(body.Pos) {
		return
	}
	there, err := r.Get[zonestate.ReadOnlyZoneState](exit.ToZoneID)
	if err != nil {
		return
	}
	for zoneLayerID, zoneLayerSlot := range there.ZoneLayerSlots() {
		held := 0
		if layer, err := r.Get[zonelayerstate.ReadOnlyZoneLayerState](zoneLayerID); err == nil {
			held = layer.CharacterCount()
		}
		if held >= int(zoneLayerSlot.Capacity) {
			continue
		}
		character, _ := s.RemoveCharacterEntity(a.CharacterID)
		// If the message cannot be delivered the character is put back where it was, by the same
		// action that puts it anywhere.
		selfID, back := s.ID, body.Pos
		undo := sim.NewActionMessage[*zonelayerstate.State](
			selfID, onfail.Drop{}, &ZoneLayerCharacterEnterAction{Character: character, At: back},
		)
		r.Send[*zonelayerstate.State](
			zoneLayerID, onfail.Compensate{With: undo},
			&ZoneLayerCharacterEnterAction{Character: character, At: exit.Arrival},
		)
		return
	}
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a *ZoneLayerCharacterExitToAction) Encode(w *binarycodec.Writer) {
	w.Uvarint(uint64(a.CharacterID))
	w.Uvarint(uint64(a.ExitEntityID))
}

// decodeZoneLayerCharacterExitToAction reads what Encode wrote.
func decodeZoneLayerCharacterExitToAction(
	r *binarycodec.Reader, version binarycodec.Version,
) (sim.Action[*zonelayerstate.State], error) {
	a := &ZoneLayerCharacterExitToAction{
		CharacterID:  state.CharacterID(r.Uvarint()),
		ExitEntityID: state.EntityID(r.Uvarint()),
	}
	return a, r.Err()
}
