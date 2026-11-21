package action

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonestate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone"
)

// ZoneChatSendAction is a message said in a zone. The zone records it and, for now, gives it to each of
// its zonelayers one by one (a ZoneLayerChatSendAction for each), with no action of its own to fan it out.
type ZoneChatSendAction struct {
	FromID state.CharacterID
	Text   string
}

func (a *ZoneChatSendAction) ID() ID          { return ZoneChatSendActionID }
func (a *ZoneChatSendAction) Priority() int16 { return PriorityResolve }
func (a *ZoneChatSendAction) Actor() any      { return a.FromID }

// ApplyZone records the message and gives it to each zonelayer.
func (a *ZoneChatSendAction) ApplyZone(c *Context, s *zonestate.State, r *ZoneRegistry, _ *Plan[ZoneAction]) {
	s.SendMessage(zonestate.ChatMessage{FromID: a.FromID, Text: a.Text, Tick: c.Tick})
	for _, zoneLayerID := range s.ZoneLayers.Keys() {
		r.ToZoneLayer(zoneLayerID, OnFailDrop, &ZoneLayerChatSendAction{FromID: a.FromID, Text: a.Text})
	}
}
