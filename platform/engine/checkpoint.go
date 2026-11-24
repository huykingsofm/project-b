package engine

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/rand"
)

// A checkpoint is what an engine needs to go on from the tick it was taken at, as bytes: the start
// says which engine it is (the version of the data, its state ID, which has the type of the
// state in it, the tick), then its state, then what the engine waits for and what only the engine
// knows.
//
// It is taken between two ticks, by the goroutine of the engine or while the engine does not run.
// The queues of the engine are not in it: what was not yet taken from them is in the replay log.
// What the engine does again and again is an action that waits, so it is in the scheduler and
// there is nothing else to write for it.

// readCheckpointStart reads the start of a checkpoint and checks that it is the checkpoint of an
// engine whose state is of the type.
func readCheckpointStart(
	r *binary.Reader, want sim.StateType,
) (codec.Version, sim.StateID, uint64, error) {
	version := r.Uint[codec.Version]()
	if err := r.Err(); err != nil {
		return 0, 0, 0, err
	}
	if version != 1 {
		return 0, 0, 0, fmt.Errorf("the checkpoint is of the data version %d", version)
	}
	id := r.Uint[sim.StateID]()
	tick := r.Uvarint()
	if err := r.Err(); err != nil {
		return 0, 0, 0, err
	}
	if id.Type() != want {
		return 0, 0, 0, fmt.Errorf(
			"the checkpoint is of the engine %d, whose state is of the type %d, not %d",
			id, id.Type(), want,
		)
	}
	return version, id, tick, nil
}

// restored is what follows the state of an engine in a checkpoint, read and checked but not yet
// given to an engine: nothing of an engine is made, and so nothing is registered in a router, until
// all of the checkpoint has been read.
type restored[S sim.State] struct {
	sched sim.Scheduler[S]
	cell  restoredCell
}

// restoredCell is what a checkpoint says of what every engine knows besides its state.
type restoredCell struct {
	rng               *rand.Rand
	sentSeq           uint64
	cfgVersionSeen    uint64
	lastRecvSeq       map[sim.StateID]uint64
	unsettledMessages unsettledMessages
}

// PCG is where the random numbers of the engine were.
func (rc restoredCell) Rand() *rand.Rand { return rc.rng }

// SentSeq is the number of the last message the engine sent.
func (rc restoredCell) SentSeq() uint64 { return rc.sentSeq }

// CfgVersionSeen is the version of the configuration the engine read last.
func (rc restoredCell) CfgVersionSeen() uint64 { return rc.cfgVersionSeen }

// LastRecvSeq is the last number that the engine took from each sender.
func (rc restoredCell) LastRecvSeq() map[sim.StateID]uint64 { return rc.lastRecvSeq }

// UnsettledMessages is what the engine had sent or was to send.
func (rc restoredCell) UnsettledMessages() unsettledMessages { return rc.unsettledMessages }

// Scheduler is what the engine waited for.
func (r *restored[S]) Scheduler() sim.Scheduler[S] { return r.sched }

// Cell is what every engine knows besides its state.
func (r *restored[S]) Cell() restoredCell { return r.cell }

// decodeCell reads what encode wrote.
func decodeCell(
	c *sim.Codecs, r *binary.Reader, version codec.Version,
) (restoredCell, error) {
	rng, err := rand.Decode(r)
	if err != nil {
		return restoredCell{}, fmt.Errorf("its random numbers: %w", err)
	}
	rc := restoredCell{rng: rng}
	rc.sentSeq = r.Uvarint()
	rc.cfgVersionSeen = r.Uvarint()
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return restoredCell{}, err
	}
	if n > uint64(r.Left()) { // a sender takes at least one byte
		return restoredCell{}, fmt.Errorf("%w: %d senders", binary.ErrShort, n)
	}
	rc.lastRecvSeq = make(map[sim.StateID]uint64, n)
	for range n {
		sender := r.Uint[sim.StateID]()
		rc.lastRecvSeq[sender] = r.Uvarint()
	}
	if rc.unsettledMessages, err = decodeUnsettledMessages(c, r, version); err != nil {
		return restoredCell{}, err
	}
	return rc, r.Err()
}
