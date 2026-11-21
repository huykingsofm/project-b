package engine

import (
	"sync"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
)

// ReplayLog is where an engine writes what a replay needs. A replay runs an engine again from the
// inputs that were recorded, and compares the hash of its state after each tick with the one taken
// the first time. What an engine cannot work out for itself is exactly what comes from outside it,
// and so that is what is recorded:
//
//   - the messages of other engines and the commands of clients, in the order the engine took them,
//     at the tick it took them;
//   - the version of the configuration, at the tick an engine first read a new one.
//
// What the engines send themselves (the self list, the scheduler, the crons) is not recorded: it
// follows from the state and the inputs and is made again by the replay. Randomness comes from the
// seed of the engine and time from its tick count.
type ReplayLog interface {
	Inbound(engine action.Address, tick uint64, h action.Header, ids []action.ID)
	// WorldCommand, ZoneCommand and ZoneLayerCommand record a command of a client, with the type of its scope.
	WorldCommand(engine action.Address, tick uint64, c command.WorldCommand)
	ZoneCommand(engine action.Address, tick uint64, c command.ZoneCommand)
	ZoneLayerCommand(engine action.Address, tick uint64, c command.ZoneLayerCommand)
	ConfigRead(engine action.Address, tick uint64, version uint64)
	// Hash is given a function that takes the hash of the state, and calls it only if it records the
	// hash: it costs as much as the state is big, so a replay log that records nothing must not make it.
	Hash(engine action.Address, tick uint64, hash func() uint64)
	// StableRead says which copy of the state of another engine the engine read first in the tick: the
	// tick the copy was taken at, or 0 when there was none.
	StableRead(engine action.Address, tick uint64, from action.Address, copyTick uint64)
}

// NopReplayLog records nothing.
type NopReplayLog struct{}

func (NopReplayLog) Inbound(action.Address, uint64, action.Header, []action.ID)        {}
func (NopReplayLog) WorldCommand(action.Address, uint64, command.WorldCommand)         {}
func (NopReplayLog) ZoneCommand(action.Address, uint64, command.ZoneCommand)           {}
func (NopReplayLog) ZoneLayerCommand(action.Address, uint64, command.ZoneLayerCommand) {}
func (NopReplayLog) ConfigRead(action.Address, uint64, uint64)                         {}
func (NopReplayLog) Hash(action.Address, uint64, func() uint64)                        {}

func (NopReplayLog) StableRead(action.Address, uint64, action.Address, uint64) {}

// RecordKind says what a Record is.
type RecordKind uint8

// The kinds of record.
const (
	RecordInbound RecordKind = iota + 1
	RecordCommand
	RecordConfig
	RecordHash
	RecordStableRead
)

// Record is one entry of a MemReplayLog.
type Record struct {
	Kind    RecordKind
	Engine  action.Address
	Tick    uint64
	Header  action.Header
	Actions []action.ID
	Command any
	Source  action.Address // for a RecordStableRead, the engine whose copy was read
	Value   uint64         // the version of a configuration, a hash, or the tick of a copy that was read
}

// MemReplayLog keeps the records in memory.
type MemReplayLog struct {
	mu      sync.Mutex
	Records []Record
}

func (j *MemReplayLog) add(r Record) {
	j.mu.Lock()
	j.Records = append(j.Records, r)
	j.mu.Unlock()
}

func (j *MemReplayLog) Inbound(a action.Address, t uint64, h action.Header, ids []action.ID) {
	j.add(Record{Kind: RecordInbound, Engine: a, Tick: t, Header: h, Actions: ids})
}

func (j *MemReplayLog) WorldCommand(a action.Address, t uint64, c command.WorldCommand) {
	j.add(Record{Kind: RecordCommand, Engine: a, Tick: t, Command: c})
}

func (j *MemReplayLog) ZoneCommand(a action.Address, t uint64, c command.ZoneCommand) {
	j.add(Record{Kind: RecordCommand, Engine: a, Tick: t, Command: c})
}

func (j *MemReplayLog) ZoneLayerCommand(a action.Address, t uint64, c command.ZoneLayerCommand) {
	j.add(Record{Kind: RecordCommand, Engine: a, Tick: t, Command: c})
}

func (j *MemReplayLog) ConfigRead(a action.Address, t uint64, v uint64) {
	j.add(Record{Kind: RecordConfig, Engine: a, Tick: t, Value: v})
}

func (j *MemReplayLog) Hash(a action.Address, t uint64, hash func() uint64) {
	j.add(Record{Kind: RecordHash, Engine: a, Tick: t, Value: hash()})
}

func (j *MemReplayLog) StableRead(a action.Address, t uint64, from action.Address, copyTick uint64) {
	j.add(Record{Kind: RecordStableRead, Engine: a, Tick: t, Source: from, Value: copyTick})
}
