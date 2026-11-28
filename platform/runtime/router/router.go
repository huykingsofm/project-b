package router

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"maps"
	"sync"
	"sync/atomic"

	"github.com/huykingsofm/project-b/shared/logging"
)

// MessageRouter is what an engine needs of the router that carries its messages: to make itself
// reachable and to retire, to deliver a message and to give back one it could not take, and to
// stop.
// The router of a cluster that runs is a Router; a replay gives the engines another one.
type MessageRouter interface {
	Register(o OutgoingChannel)
	MarkRetired(a sim.StateID)
	Stop()
	Deliver(l sim.Message) (sim.DeliveryFailure, bool)
	Return(l sim.Message, r sim.DeliveryFailure)
}

var _ MessageRouter = (*Router)(nil)

type routerData struct {
	chans   map[sim.StateID]OutgoingChannel
	retired map[sim.StateID]bool
}

// Router carries the messages between the engines. It holds the outgoing channel of every engine,
// finds the one of an ID and puts the message in it without waiting. Reading it takes no lock:
// the table of channels is replaced as a whole when an engine is added or retired, which is rare.
type Router struct {
	mu       sync.Mutex
	data     atomic.Pointer[routerData]
	stopping atomic.Bool
}

// NewRouter makes an empty router.
func NewRouter() *Router {
	rt := &Router{}
	rt.data.Store(
		&routerData{
			chans: map[sim.StateID]OutgoingChannel{}, retired: map[sim.StateID]bool{},
		},
	)
	return rt
}

// Register makes the channel of an engine reachable.
func (rt *Router) Register(o OutgoingChannel) {
	rt.update(func(n *routerData) {
		n.chans[o.StateID()] = o
		delete(n.retired, o.StateID())
	})
}

// MarkRetired makes an engine unreachable for good: a message to it bounces as
// DeliveryFailureRetired.
func (rt *Router) MarkRetired(a sim.StateID) {
	rt.update(func(n *routerData) {
		delete(n.chans, a)
		n.retired[a] = true
	})
}

// Stop makes every later delivery bounce as Stopping.
func (rt *Router) Stop() { rt.stopping.Store(true) }

// Deliver puts a message in the channel of its ID. When it cannot, it says why.
func (rt *Router) Deliver(l sim.Message) (sim.DeliveryFailure, bool) {
	if rt.stopping.Load() {
		return sim.DeliveryFailureStopping, false
	}
	data := rt.data.Load()
	ch, ok := data.chans[l.Dest()]
	if !ok {
		// an engine that was retired is gone; any other ID has no engine yet
		if data.retired[l.Dest()] {
			return sim.DeliveryFailureRetired, false
		}
		return sim.DeliveryFailureNotStarted, false
	}
	if !ch.Send(l) {
		logging.Warn(
			"message refused: the channel of its receiver is full",
			"from",
			l.Head().From,
			"dest",
			l.Dest(),
		)
		return sim.DeliveryFailureFull, false
	}
	return 0, true
}

// Return gives a message that its receiver could not take back to its sender. A sender that
// is gone or full loses it, and it is logged.
func (rt *Router) Return(l sim.Message, r sim.DeliveryFailure) {
	from := l.Head().From
	ch, ok := rt.data.Load().chans[from]
	if !ok {
		logging.Warn(
			"returned sim lost: its sender is gone",
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
		logging.Warn(
			"returned sim lost: the back channel of its sender is full",
			"from",
			from,
			"dest",
			l.Dest(),
			"reason",
			r,
		)
	}
}

func (rt *Router) update(f func(*routerData)) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	old := rt.data.Load()
	next := &routerData{
		chans:   make(map[sim.StateID]OutgoingChannel, len(old.chans)+1),
		retired: make(map[sim.StateID]bool, len(old.retired)+1),
	}
	maps.Copy(next.chans, old.chans)
	maps.Copy(next.retired, old.retired)
	f(next)
	rt.data.Store(next)
}
