package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// WorldZoneLayerAddAction asks the cluster for a zonelayer of a zone of the world, with room for
// Capacity characters, that heals them every RegenEvery ticks (0: never). The world lists the
// zonelayer when the cluster answers (worldstate.State.HandleDirective), and the zonelayer tells
// its zone that it is there when it starts (ZoneLayerAnnounceToZoneAction): the request only makes
// the engine (R12).
type WorldZoneLayerAddAction struct {
	ZoneID     sim.StateID
	Capacity   int32
	RegenEvery uint64
}

// decodeWorldZoneLayerAddAction reads what Encode wrote.
func decodeWorldZoneLayerAddAction(
	r *binary.Reader, version codec.Version,
) (WorldZoneLayerAddAction, error) {
	a := WorldZoneLayerAddAction{
		ZoneID: r.Uint[sim.StateID](), Capacity: r.Int[int32](), RegenEvery: r.Uvarint(),
	}
	return a, r.Err()
}

func (a WorldZoneLayerAddAction) ID() sim.ActionID { return WorldZoneLayerAddActionID }

func (a WorldZoneLayerAddAction) Priority() int16 { return PriorityResolve }

func (a WorldZoneLayerAddAction) Actor() any { return nil }

// Clone returns the action itself: it holds only values.
func (a WorldZoneLayerAddAction) Clone() sim.Action[*worldstate.State] { return a }

// Apply asks for the zonelayer, which starts by telling its zone, and heals at its first tick and
// every RegenEvery ticks after (the action sets itself again).
func (a WorldZoneLayerAddAction) Apply(
	c *sim.Context, _ *worldstate.State, r *sim.ActionRegistry[*worldstate.State],
	_ *sim.Plan[*worldstate.State],
) {
	start := []sim.Action[*zonelayerstate.State]{
		ZoneLayerAnnounceToZoneAction{Capacity: a.Capacity},
	}
	if a.RegenEvery > 0 {
		start = append(start, ZoneLayerCharacterRegenAction{Every: a.RegenEvery})
	}
	r.RequestCreateEngine(
		[]byte{worldstate.TagZoneLayer}, zonelayerstate.NewState(a.ZoneID, c.Self), start...,
	)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a WorldZoneLayerAddAction) Encode(w *binary.Writer) {
	w.Uvarint(uint64(a.ZoneID))
	w.Varint(int64(a.Capacity))
	w.Uvarint(a.RegenEvery)
}
