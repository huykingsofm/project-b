// Package engine runs the engines of the game: the world, the zones and the zonelayers. Each one
// is a goroutine that, at every tick, takes what was sent to it, runs it as a plan of actions
// over its state, tells its clients what changed, publishes a copy of its state for the other
// engines to read, and sends the messages that its actions asked for.
//
// Everything about the actions and the state is in package sim; this package is what makes
// them run, and what connects the engines: the router that carries the messages, the
// registry of published states, the graph of the world, the bouncer that runs what a message says
// a failed delivery means, and the replay log that a replay is made from. Nothing here knows what
// the actions do.
package engine

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"maps"
	"sync"
	"sync/atomic"

	"github.com/huykingsofm/project-b/server/internal/shared/logging"
)

type routerData struct {
	chans   map[message.Address]OutgoingChannel
	retired map[message.Address]bool
}

// Router carries the messages between the engines. It holds the outgoing channel of every engine,
// finds the one of an address and puts the message in it without waiting. Reading it takes no lock:
// the table of channels is replaced as a whole when an engine is added or retired, which is rare.
type Router struct {
	mu       sync.Mutex
	data     atomic.Pointer[routerData]
	stopping atomic.Bool
	dropped  atomic.Uint64
}

// NewRouter makes an empty router.
func NewRouter() *Router {
	rt := &Router{}
	rt.data.Store(
		&routerData{
			chans: map[message.Address]OutgoingChannel{}, retired: map[message.Address]bool{},
		},
	)
	return rt
}

func (rt *Router) update(f func(*routerData)) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	old := rt.data.Load()
	next := &routerData{
		chans:   make(map[message.Address]OutgoingChannel, len(old.chans)+1),
		retired: make(map[message.Address]bool, len(old.retired)+1),
	}
	maps.Copy(next.chans, old.chans)
	maps.Copy(next.retired, old.retired)
	f(next)
	rt.data.Store(next)
}

// Register makes the channel of an engine reachable.
func (rt *Router) Register(o OutgoingChannel) {
	rt.update(func(n *routerData) {
		n.chans[o.Address()] = o
		delete(n.retired, o.Address())
	})
}

// Retire makes an address unreachable for good: a message to it bounces as Retired.
func (rt *Router) Retire(a message.Address) {
	rt.update(func(n *routerData) {
		delete(n.chans, a)
		n.retired[a] = true
	})
}

// Stop makes every later delivery bounce as Stopping.
func (rt *Router) Stop() { rt.stopping.Store(true) }

// Deliver puts a message in the channel of its address. When it cannot, it says why.
func (rt *Router) Deliver(l message.Message) (message.Reason, bool) {
	if rt.stopping.Load() {
		return message.Stopping, false
	}
	data := rt.data.Load()
	ch, ok := data.chans[l.Dest()]
	if !ok {
		if data.retired[l.Dest()] {
			return message.Retired, false
		}
		return message.NotBuilt, false
	}
	if !ch.Send(l) {
		return message.Full, false
	}
	return 0, true
}

// Return gives a message that its receiver could not take back to its sender. A sender that
// is gone or full loses it, and it is counted.
func (rt *Router) Return(l message.Message, r message.Reason) {
	from := l.Head().From
	ch, ok := rt.data.Load().chans[from]
	if !ok {
		rt.dropped.Add(1)
		logging.Warn(
			"returned message lost: its sender is gone",
			"from",
			from,
			"dest",
			l.Dest(),
			"reason",
			r,
		)
		return
	}
	if !ch.SendBack(ReturnedMessage{Message: l, Reason: r}) {
		rt.dropped.Add(1)
		logging.Warn(
			"returned message lost: the back channel of its sender is full",
			"from",
			from,
			"dest",
			l.Dest(),
			"reason",
			r,
		)
	}
}

// Dropped returns how many messages were lost because their sender could not take them back.
func (rt *Router) Dropped() uint64 { return rt.dropped.Load() }
