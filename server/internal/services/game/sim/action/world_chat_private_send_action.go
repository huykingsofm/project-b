package action

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// WorldChatPrivateSendAction is a private message from one player to another. It is between
// players, not between characters (a character is not always played by a player), and it is the
// world that manages it: it records it and tells the clients, and no zone or zonelayer takes part.
type WorldChatPrivateSendAction struct {
	FromID state.PlayerID
	ToID   state.PlayerID
	Text   string
}

func (a *WorldChatPrivateSendAction) ID() ID          { return WorldChatPrivateSendActionID }
func (a *WorldChatPrivateSendAction) Priority() int16 { return PriorityResolve }
func (a *WorldChatPrivateSendAction) Actor() any      { return a.FromID }

// ApplyWorld records the message.
func (a *WorldChatPrivateSendAction) ApplyWorld(
	c *Context, s *worldstate.State, _ *WorldRegistry, _ *Plan[WorldAction],
) {
	s.SendPrivateMessage(
		worldstate.PrivateChatMessage{FromID: a.FromID, ToID: a.ToID, Text: a.Text, Tick: c.Tick},
	)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a *WorldChatPrivateSendAction) Encode(w *binarycodec.Writer) {
	w.Uvarint(uint64(a.FromID))
	w.Uvarint(uint64(a.ToID))
	w.String(a.Text)
}

// decodeWorldChatPrivateSendAction reads what Encode wrote.
func decodeWorldChatPrivateSendAction(
	r *binarycodec.Reader, version binarycodec.Version,
) (Action, error) {
	a := &WorldChatPrivateSendAction{
		FromID: state.PlayerID(r.Uvarint()),
		ToID:   state.PlayerID(r.Uvarint()),
		Text:   r.String(),
	}
	return a, r.Err()
}
