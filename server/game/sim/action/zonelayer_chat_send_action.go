package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ZoneLayerChatSendAction is a message said in a zonelayer. A character says it to the zonelayer it
// is in, and a zone gives it to each of its zonelayers (ZoneChatSendAction).
type ZoneLayerChatSendAction struct {
	FromID state.CharacterID
	Text   string
}

// decodeZoneLayerChatSendAction reads what Encode wrote.
func decodeZoneLayerChatSendAction(
	r *binary.Reader, version codec.Version,
) (ZoneLayerChatSendAction, error) {
	a := ZoneLayerChatSendAction{FromID: r.Uint[state.CharacterID](), Text: r.String()}
	return a, r.Err()
}

func (a ZoneLayerChatSendAction) ID() sim.ActionID { return ZoneLayerChatSendActionID }

func (a ZoneLayerChatSendAction) Priority() int16 { return PriorityResolve }

func (a ZoneLayerChatSendAction) Actor() any { return a.FromID }

// Clone returns the action itself: it holds only values.
func (a ZoneLayerChatSendAction) Clone() sim.Action[*zonelayerstate.State] {
	return a
}

// Apply records the message.
func (a ZoneLayerChatSendAction) Apply(
	c *sim.Context, s *zonelayerstate.State, _ *sim.ActionRegistry[*zonelayerstate.State],
	_ *sim.Plan[*zonelayerstate.State],
) {
	s.SendMessage(zonelayerstate.NewChatMessage(a.FromID, a.Text, c.Tick))
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a ZoneLayerChatSendAction) Encode(w *binary.Writer) {
	w.Uvarint(uint64(a.FromID))
	w.String(a.Text)
}
