package engine

import (
	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/router"
	"github.com/huykingsofm/project-b/platform/runtime/stable"
	"github.com/huykingsofm/project-b/platform/sim"
)

// Environment is what every engine of a cluster shares: the router that carries their messages, the
// event log they write, the store of the states they publish (the configuration is read from it,
// from the state of the coordinator engine), the emitter where they give what changed, the codecs
// that read back what they wrote, the registry that makes their engines, the seed of their random
// numbers, and the writer of their checkpoints.
type Environment struct {
	// how the engines talk: the message router, the directive router, and the board of acks
	Router          router.MessageRouter
	DirectiveRouter *router.DirectiveRouter
	AckBoard        *stable.AckBoard // an engine with none holds no message for an ack

	// where the engines write: the log of events, the copies that others read, the changes of
	// their states, and their checkpoints (with no writer they take none: a replay has none)
	EventLog         eventlog.Log
	StableStore      stable.Store
	Emitter          Emitter
	CheckpointWriter CheckpointWriter

	// what an engine is made with: the codecs, the registry of the states, the seed, and the size
	// of the queues
	Codecs         *sim.Codecs
	EngineRegistry *EngineRegistry
	Seed           uint64
	QueueSize      int
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
		Router: router.NewRouter(), EventLog: log, StableStore: stable.NewStateStore(),
		Codecs:         o.Codecs,
		EngineRegistry: o.Engines, Seed: o.Seed, QueueSize: o.QueueSize, Emitter: emitter,
		AckBoard: stable.NewAckBoard(), CheckpointWriter: o.CheckpointWriter,
		DirectiveRouter: router.NewDirectiveRouter(),
	}
}
