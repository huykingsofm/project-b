// Package replay is what an engine writes so that a replay can run it again: the Log interface the engines
// record to, the records, and three logs: NopLog records nothing, MemLog keeps the records in memory (for tests), and
// FileLog writes them to files, one log for each engine, on top of shared/segmentlog. Read gives the records of a
// FileLog back. It imports the actions and the commands, but no engine: the engines import it.
package replay

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
)

// Log is where an engine writes what a replay needs. A replay runs an engine again from the
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
type Log interface {
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
	// EndTick says that the engine has recorded all it will record for the tick. A log that writes to a file
	// hands what it has kept for the tick to the goroutine that writes it.
	EndTick(engine action.Address, tick uint64)
}

// NopLog records nothing.
type NopLog struct{}

func (NopLog) Inbound(action.Address, uint64, action.Header, []action.ID)        {}
func (NopLog) WorldCommand(action.Address, uint64, command.WorldCommand)         {}
func (NopLog) ZoneCommand(action.Address, uint64, command.ZoneCommand)           {}
func (NopLog) ZoneLayerCommand(action.Address, uint64, command.ZoneLayerCommand) {}
func (NopLog) ConfigRead(action.Address, uint64, uint64)                         {}
func (NopLog) Hash(action.Address, uint64, func() uint64)                        {}

func (NopLog) StableRead(action.Address, uint64, action.Address, uint64) {}
func (NopLog) EndTick(action.Address, uint64)                            {}
