package engine

import (
	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
)

// Options is what a cluster is made with, and what a replay of it needs to make it again: the seed
// of the random numbers of its engines, the size of their queues, the codecs that read back what
// they wrote and the registry that makes their engines. The rest is only for a cluster that runs.
type Options struct {
	// what the engines are made with
	Seed      uint64
	QueueSize int
	Codecs    *sim.Codecs
	Engines   *EngineRegistry

	// where the engines write what a replay needs, and what changed in their states, and their
	// checkpoints; nil is none: nothing is written, discarded, or taken
	EventLog         eventlog.Log
	Emitter          Emitter
	CheckpointWriter CheckpointWriter
}
