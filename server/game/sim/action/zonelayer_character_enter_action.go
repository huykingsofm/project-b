package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
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

func (a *ZoneLayerCharacterEnterAction) ID() sim.ActionID { return ZoneLayerCharacterEnterActionID }
func (a *ZoneLayerCharacterEnterAction) Priority() int16  { return PriorityResolve }
func (a *ZoneLayerCharacterEnterAction) Actor() any       { return a.Character.CharacterID() }

// Apply gives the character a body.
func (a *ZoneLayerCharacterEnterAction) Apply(
	_ *sim.Context, s *zonelayerstate.State, r *sim.Registry[*zonelayerstate.State],
	_ *sim.Plan[*zonelayerstate.State],
) {
	if _, held := s.CharacterByID(a.Character.CharacterID()); held {
		return
	}
	zone, err := r.Get[zonestate.ReadOnlyZoneState](s.ZoneID())
	if err != nil {
		r.ToSelf(a)
		return
	}
	at := a.At
	if !zone.TileMap().FreeForBody(at, a.Character.Radius()) {
		at = zone.TileMap().SpawnPosition()
	}
	s.AddCharacterEntity(a.Character, at)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a *ZoneLayerCharacterEnterAction) Encode(w *binary.Writer) {
	a.Character.Encode(w)
	a.At.Encode(w)
}

// decodeZoneLayerCharacterEnterAction reads what Encode wrote.
func decodeZoneLayerCharacterEnterAction(
	r *binary.Reader, version codec.Version,
) (sim.Action[*zonelayerstate.State], error) {
	c, err := zonelayerstate.DecodeCharacter(r, version)
	if err != nil {
		return nil, err
	}
	a := &ZoneLayerCharacterEnterAction{Character: c, At: state.DecodeVec(r, version)}
	return a, r.Err()
}
