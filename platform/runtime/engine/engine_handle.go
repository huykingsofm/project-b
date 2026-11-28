package engine

import (
	"context"
	"time"

	"github.com/huykingsofm/project-b/platform/sim"
)

// EngineHandle is what the cluster needs of an engine, whatever the type of its state is: to run
// it on its own, to ask it to end, to give it a response of the cluster and to say which engine it
// is.
// What the code that holds an engine from the cluster (Cluster.Engines, Cluster.Find,
// Cluster.AddEngineByCheckpoint) needs besides, it asks of the handle itself: it defines its own
// interface and converts the handle to it.
type EngineHandle interface {
	StateID() sim.StateID
	// Run runs the engine on its own until the context ends or its tick fails.
	Run(ctx context.Context, ticks <-chan time.Time) error
	// RequestRetirement asks the engine to retire between two of its ticks: Run does it, or the
	// code that steps the engines itself (Engine.RetireIfRequested); it does not wait, and may be
	// called from any goroutine. by and byTick are the engine that asked and its tick.
	RequestRetirement(by sim.StateID, byTick uint64)
	// Retire ends the engine for good and writes the record of it to its log. It is for an engine
	// that no goroutine runs, which the cluster made and no one took: see Engine.Retire.
	Retire()
	// IsRetired says whether the engine retired, and the tick of the last frame of its log.
	IsRetired() (retiredTick uint64, retired bool)
	// Adopt tells an engine that a request made that its creator took the response: see
	// Engine.Adopt.
	Adopt(creator sim.StateID, responseTick uint64)
}
