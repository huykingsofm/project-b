package action

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// ZoneLayerChatSendAction is a message said in a zonelayer. A character says it to the zonelayer it
// is in, and a zone gives it to each of its zonelayers (ZoneChatSendAction).
type ZoneLayerChatSendAction struct {
	FromID state.CharacterID
	Text   string
}

func (a *ZoneLayerChatSendAction) ID() ID          { return ZoneLayerChatSendActionID }
func (a *ZoneLayerChatSendAction) Priority() int16 { return PriorityResolve }
func (a *ZoneLayerChatSendAction) Actor() any      { return a.FromID }

// ApplyZoneLayer records the message.
func (a *ZoneLayerChatSendAction) ApplyZoneLayer(
	c *Context, s *zonelayerstate.State, _ *ZoneLayerRegistry, _ *Plan[ZoneLayerAction],
) {
	s.SendMessage(zonelayerstate.ChatMessage{FromID: a.FromID, Text: a.Text, Tick: c.Tick})
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a *ZoneLayerChatSendAction) Encode(w *binarycodec.Writer) {
	w.Uvarint(uint64(a.FromID))
	w.String(a.Text)
}

// decodeZoneLayerChatSendAction reads what Encode wrote.
func decodeZoneLayerChatSendAction(
	r *binarycodec.Reader, version binarycodec.Version,
) (Action, error) {
	a := &ZoneLayerChatSendAction{FromID: state.CharacterID(r.Uvarint()), Text: r.String()}
	return a, r.Err()
}
