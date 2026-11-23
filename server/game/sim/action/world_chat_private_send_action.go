package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// WorldChatPrivateSendAction is a private message from one player to another. It is between
// players, not between characters (a character is not always played by a player), and it is the
// world that manages it: it records it and tells the clients, and no zone or zonelayer takes part.
type WorldChatPrivateSendAction struct {
	FromID state.PlayerID
	ToID   state.PlayerID
	Text   string
}

func (a *WorldChatPrivateSendAction) ID() sim.ActionID { return WorldChatPrivateSendActionID }
func (a *WorldChatPrivateSendAction) Priority() int16  { return PriorityResolve }
func (a *WorldChatPrivateSendAction) Actor() any       { return a.FromID }

// Apply records the message.
func (a *WorldChatPrivateSendAction) Apply(
	c *sim.Context, s *worldstate.State, _ *sim.Registry[*worldstate.State],
	_ *sim.Plan[*worldstate.State],
) {
	s.SendPrivateMessage(
		worldstate.NewPrivateChatMessage(a.FromID, a.ToID, a.Text, c.Tick),
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
) (sim.Action[*worldstate.State], error) {
	a := &WorldChatPrivateSendAction{
		FromID: r.Uint[state.PlayerID](),
		ToID:   r.Uint[state.PlayerID](),
		Text:   r.String(),
	}
	return a, r.Err()
}
