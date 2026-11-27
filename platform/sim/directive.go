package sim

// DirectiveKind is what a directive asks of the cluster, or answers.
type DirectiveKind uint8

const (
	// DirectiveCreateEngine asks the cluster to make an engine from a state and the actions that
	// it starts with.
	DirectiveCreateEngine DirectiveKind = iota + 1
	// DirectiveRemoveEngine asks the cluster to remove the engine with the ID in Target.
	DirectiveRemoveEngine
)

// DirectiveRequest is what an engine asks of its cluster, since an engine cannot make or remove
// another engine itself. Its actions put it in their registry; the engine numbers it, keeps it in
// its own data until the cluster has answered, and hands it to the cluster at the end of the tick
// (and again, as long as no answer came): the cluster serves a request once, whatever the number of
// times it is given.
type DirectiveRequest struct {
	// ID is the number that the engine gave the request: counted up from 1 in its own order, so
	// (the engine, ID) names the request.
	ID   uint64
	Kind DirectiveKind
	// Tag is what the requester wants back with the answer, which it writes and reads itself: what
	// the engine is for.
	Tag []byte
	// State and StartActions are the engine to make, for DirectiveCreateEngine: the state as the
	// first tick of its engine finds it, and the actions that run once, at that first tick: an
	// action that is to run again sets itself again. The cluster gives the engine its ID and keeps
	// a copy of the state, so the request is never changed.
	State        State
	StartActions []BaseAction
	// Target is the engine to remove, for DirectiveRemoveEngine.
	Target StateID
}

// DirectiveResponse is what the cluster answers to a request. It is written to the event log of the
// engine that asked, at the tick it was taken at, as a command is, since a replay cannot make it
// again.
type DirectiveResponse struct {
	// ID is the number of the request that this answers.
	ID   uint64
	Kind DirectiveKind
	// Tag is the tag of the request.
	Tag []byte
	// Engine is the ID of the engine that was made, or the one that was removed; 0 for a failure.
	Engine StateID
	// Failure says why the request was not done; empty for success.
	Failure string
}

// DirectiveHandler is a state that takes the answers of its cluster. It runs at the start of the
// tick, before the plan: it changes the state as an action does, and has the registry of the tick
// to ask or send what an action does, but it has no plan and is not given by anyone: the engine is
// the one that gives it the answer. A state that is not a handler gets none: the engine logs a line
// of data to fix and drops the answer.
type DirectiveHandler[S State] interface {
	HandleDirective(ctx *Context, reg *Registry[S], response DirectiveResponse)
}
