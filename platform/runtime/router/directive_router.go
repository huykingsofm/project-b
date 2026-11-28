package router

import (
	"sync"

	"github.com/huykingsofm/project-b/platform/sim"
)

// PublishedDirectiveRequest is a request of an engine, with the engine that asked.
type PublishedDirectiveRequest struct {
	From    sim.StateID
	Request sim.DirectiveRequest
}

// ResponseAcknowledgement says that the engine From took the response to its request number
// Request at its tick Tick: for a request to make an engine, it is what lets the engine that was
// made run.
type ResponseAcknowledgement struct {
	From    sim.StateID
	Request uint64
	Tick    uint64
}

// EngineDirectives is what an engine needs of the directive router: to publish its requests and
// the acknowledgements of the responses it took, to take its responses, and to leave.
type EngineDirectives interface {
	PublishRequest(r PublishedDirectiveRequest)
	PublishResponseAcknowledgement(a ResponseAcknowledgement)
	// TakeResponses returns the responses that wait for the engine, in the order they were
	// published, and forgets them.
	TakeResponses(engine sim.StateID) []sim.DirectiveResponse
	// Leave makes the engine unreachable: what waits for it is let go, and so are the requests it
	// published that no one took. The acknowledgements it published stay: they say what the engine
	// did, and the engine that it made runs though its creator is gone.
	Leave(engine sim.StateID)
}

var (
	_ EngineDirectives = (*DirectiveRouter)(nil)
)

// DirectiveRouter carries the requests of the engines to their cluster and the responses back,
// without either side waiting for the other: a list of requests, a list of acknowledgements and a
// list of responses for each engine, behind one mutex. Nothing is dropped because a list is full.
type DirectiveRouter struct {
	mu                       sync.Mutex
	requests                 []PublishedDirectiveRequest
	responseAcknowledgements []ResponseAcknowledgement
	responses                map[sim.StateID][]sim.DirectiveResponse
	left                     map[sim.StateID]bool
}

// NewDirectiveRouter makes an empty directive router.
func NewDirectiveRouter() *DirectiveRouter {
	return &DirectiveRouter{
		responses: map[sim.StateID][]sim.DirectiveResponse{}, left: map[sim.StateID]bool{},
	}
}

func (d *DirectiveRouter) PublishRequest(r PublishedDirectiveRequest) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.left[r.From] {
		d.requests = append(d.requests, r)
	}
}

func (d *DirectiveRouter) PublishResponseAcknowledgement(a ResponseAcknowledgement) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.left[a.From] {
		d.responseAcknowledgements = append(d.responseAcknowledgements, a)
	}
}

func (d *DirectiveRouter) TakeResponses(engine sim.StateID) []sim.DirectiveResponse {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.responses[engine]
	delete(d.responses, engine)
	return out
}

func (d *DirectiveRouter) Leave(engine sim.StateID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.left[engine] = true
	delete(d.responses, engine)
	keep := func(r PublishedDirectiveRequest) bool { return r.From != engine }
	d.requests = filterInPlace(d.requests, keep)
}

func (d *DirectiveRouter) TakeRequests() []PublishedDirectiveRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.requests
	d.requests = nil
	return out
}

func (d *DirectiveRouter) TakeResponseAcknowledgements() []ResponseAcknowledgement {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.responseAcknowledgements
	d.responseAcknowledgements = nil
	return out
}

func (d *DirectiveRouter) PublishResponse(to sim.StateID, r sim.DirectiveResponse) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.left[to] {
		return false
	}
	d.responses[to] = append(d.responses[to], r)
	return true
}

func filterInPlace[T any](list []T, keep func(T) bool) []T {
	out := list[:0]
	for _, v := range list {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}
