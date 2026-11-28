package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ZoneLayerCharacterShieldGrantAction is a standing action: it stands in the plan at every tick, by
// setting itself for the next tick, until its time is up or it has blocked a damage. In each tick
// it runs in the modifier band, before the damage lands, finds the damage in the plan through the
// plan's index by ID, and cancels the first damage that is aimed at the entity of its character. It
// is the pattern that every effect that changes an action it was not written for follows.
type ZoneLayerCharacterShieldGrantAction struct {
	CharacterID state.CharacterID
	Duration    uint64 // for how many ticks it stands, counted from the first tick it runs

	started bool
	until   uint64
}

// decodeZoneLayerCharacterShieldGrantAction reads what Encode wrote.
func decodeZoneLayerCharacterShieldGrantAction(
	r *binary.Reader, version codec.Version,
) (ZoneLayerCharacterShieldGrantAction, error) {
	a := ZoneLayerCharacterShieldGrantAction{
		CharacterID: r.Uint[state.CharacterID](),
		Duration:    r.Uvarint(),
		started:     r.Bool(),
		until:       r.Uvarint(),
	}
	return a, r.Err()
}

func (a ZoneLayerCharacterShieldGrantAction) ID() sim.ActionID {
	return ZoneLayerCharacterShieldGrantActionID
}

func (a ZoneLayerCharacterShieldGrantAction) Priority() int16 { return PriorityModifier }

func (a ZoneLayerCharacterShieldGrantAction) Actor() any { return a.CharacterID }

// Clone returns the action itself: it holds only values.
func (a ZoneLayerCharacterShieldGrantAction) Clone() sim.Action[*zonelayerstate.State] {
	return a
}

// Apply blocks the first damage that is aimed at the character, or stands again next tick.
func (a ZoneLayerCharacterShieldGrantAction) Apply(
	c *sim.Context, s *zonelayerstate.State, r *sim.ActionRegistry[*zonelayerstate.State],
	p *sim.Plan[*zonelayerstate.State],
) {
	if !a.started {
		a.started, a.until = true, c.Tick+a.Duration
	}
	if c.Tick >= a.until {
		return
	}
	entity, ok := s.CharacterEntityByCharacterID(a.CharacterID)
	if !ok {
		return
	}
	damages := p.FindByID[ZoneLayerCharacterDamageAction](ZoneLayerCharacterDamageActionID)
	for e, damage := range damages {
		if e.Status() == sim.EntryPending && damage.VictimID == entity.ID() {
			p.Cancel(e)
			return
		}
	}
	r.ToSelf(a)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a ZoneLayerCharacterShieldGrantAction) Encode(w *binary.Writer) {
	w.Uvarint(uint64(a.CharacterID))
	w.Uvarint(a.Duration)
	w.Bool(a.started)
	w.Uvarint(a.until)
}
