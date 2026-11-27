package engine

import (
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
)

// Options is what a cluster is made with, and what a replay of it needs to make it again: the seed
// of the random numbers of its engines, the size of their queues, the codecs that read back what
// they wrote and the registry that makes their engines. The rest is only for a cluster that runs.
type Options struct {
	Seed      uint64
	QueueSize int
	Codecs    *sim.Codecs
	Engines   *EngineRegistry
	// EventLog is where the engines write what a replay needs; nil writes nothing.
	EventLog eventlog.Log
	// Emitter is where the engines give what changed in their states; nil discards it.
	Emitter Emitter
	// CheckpointWriter writes the checkpoints that the engines take; nil: they take none.
	CheckpointWriter CheckpointWriter
}

// NewEnvironment makes the environment of a cluster that runs: a router, the published states, the
// acks, and what the options say.
func NewEnvironment(o Options) *Environment {
	var log eventlog.Log = eventlog.NopLog{}
	if o.EventLog != nil {
		log = o.EventLog
	}
	var emitter Emitter = NopEmitter{}
	if o.Emitter != nil {
		emitter = o.Emitter
	}
	return &Environment{
		Router: NewRouter(), EventLog: log, StableStore: NewStableStateStore(), Codecs: o.Codecs,
		EngineRegistry: o.Engines, Seed: o.Seed, QueueSize: o.QueueSize, Emitter: emitter,
		AckBoard: NewAckBoard(), CheckpointWriter: o.CheckpointWriter,
	}
}
