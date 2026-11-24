package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ZoneChatSendAction is a message said in a zone. The zone records it and, for now, gives it to
// each of its zonelayers one by one (a ZoneLayerChatSendAction for each), with no action of its own
// to fan it out.
type ZoneChatSendAction struct {
	FromID state.CharacterID
	Text   string
}

func (a ZoneChatSendAction) ID() sim.ActionID { return ZoneChatSendActionID }
func (a ZoneChatSendAction) Priority() int16  { return PriorityResolve }
func (a ZoneChatSendAction) Actor() any       { return a.FromID }

// Clone returns the action itself: it holds only values.
func (a ZoneChatSendAction) Clone() sim.Action[*zonestate.State] {
	return a
}

// Apply records the message and gives it to each zonelayer.
func (a ZoneChatSendAction) Apply(
	c *sim.Context, s *zonestate.State, r *sim.Registry[*zonestate.State],
	_ *sim.Plan[*zonestate.State],
) {
	s.SendMessage(zonestate.NewChatMessage(a.FromID, a.Text, c.Tick))
	for zoneLayerID := range s.ZoneLayerSlots() {
		r.Send(
			zoneLayerID, onfail.Drop{}, ZoneLayerChatSendAction{FromID: a.FromID, Text: a.Text},
		)
	}
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a ZoneChatSendAction) Encode(w *binary.Writer) {
	w.Uvarint(uint64(a.FromID))
	w.String(a.Text)
}

// decodeZoneChatSendAction reads what Encode wrote.
func decodeZoneChatSendAction(
	r *binary.Reader, version codec.Version,
) (ZoneChatSendAction, error) {
	a := ZoneChatSendAction{FromID: r.Uint[state.CharacterID](), Text: r.String()}
	return a, r.Err()
}
