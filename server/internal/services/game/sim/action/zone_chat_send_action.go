package action

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/onfail"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonestate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// ZoneChatSendAction is a message said in a zone. The zone records it and, for now, gives it to
// each of its zonelayers one by one (a ZoneLayerChatSendAction for each), with no action of its own
// to fan it out.
type ZoneChatSendAction struct {
	FromID state.CharacterID
	Text   string
}

func (a *ZoneChatSendAction) ID() ID          { return ZoneChatSendActionID }
func (a *ZoneChatSendAction) Priority() int16 { return PriorityResolve }
func (a *ZoneChatSendAction) Actor() any      { return a.FromID }

// ApplyZone records the message and gives it to each zonelayer.
func (a *ZoneChatSendAction) ApplyZone(
	c *Context, s *zonestate.State, r *ZoneRegistry, _ *Plan[ZoneAction],
) {
	s.SendMessage(zonestate.ChatMessage{FromID: a.FromID, Text: a.Text, Tick: c.Tick})
	for _, zoneLayerID := range s.ZoneLayers.Keys() {
		r.ToZoneLayer(
			zoneLayerID, onfail.Drop{}, &ZoneLayerChatSendAction{FromID: a.FromID, Text: a.Text},
		)
	}
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a *ZoneChatSendAction) Encode(w *binarycodec.Writer) {
	w.Uvarint(uint64(a.FromID))
	w.String(a.Text)
}

// decodeZoneChatSendAction reads what Encode wrote.
func decodeZoneChatSendAction(r *binarycodec.Reader, version binarycodec.Version) (Action, error) {
	a := &ZoneChatSendAction{FromID: state.CharacterID(r.Uvarint()), Text: r.String()}
	return a, r.Err()
}
