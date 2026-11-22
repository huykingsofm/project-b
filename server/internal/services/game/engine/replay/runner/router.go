package runner

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/engine"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
)

// msgKey names a message: who sent it and which message of the sender it was.
type msgKey struct {
	from message.Address
	seq  uint64
}

func keyOf(h message.Header) msgKey { return msgKey{h.From, h.Seq} }

// replayRouter is the router of a replay. It carries nothing to the engines by itself: the replay
// puts in the channel of an engine the messages that the log of that engine says it took. What an
// engine sends is compared with what the receivers recorded and kept when the log says it came
// back, so that it can be given back at the tick the log says.
type replayRouter struct {
	*engine.Router
	want     map[msgKey]message.Message // the messages as the receivers recorded them
	returned map[msgKey]bool            // the messages that the log says came back
	sent     map[msgKey]message.Message // the messages of those that were sent
	refuse   map[msgKey]message.Reason  // what the router answers at once, in the tick of the send
	div      *divergences
}

func (r *replayRouter) Deliver(m message.Message) (message.Reason, bool) {
	h := m.Head()
	key := keyOf(h)
	if want, ok := r.want[key]; ok && !replay.SameMessage(want, m) {
		r.div.add(h.From, "the message %d it sends is not the one that %v recorded as taken", h.Seq,
			m.Dest())
	}
	if r.returned[key] {
		r.sent[key] = m
	}
	if reason, ok := r.refuse[key]; ok {
		delete(r.refuse, key)
		return reason, false
	}
	return 0, true
}

// inject puts a message in the channel of the engine it is for.
func (r *replayRouter) inject(m message.Message) bool {
	_, ok := r.Router.Deliver(m)
	return ok
}
