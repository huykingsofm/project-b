// Package replay is what an engine writes so that a replay can run it again: the Log interface the
// engines record to, the records, and three logs: NopLog records nothing, MemLog keeps the records
// in memory (for tests), and FileLog writes them to files, one log for each engine, on top of
// shared/segmentlog. ReadFileLog gives the records of a FileLog back. It imports the sim package,
// but no engine: the engines import it.
package eventlog

import "github.com/huykingsofm/project-b/platform/sim"

// Log is where an engine writes what a replay needs. A replay runs an engine again from the
// inputs that were recorded, and compares the hash of its state after each tick with the one taken
// the first time. What an engine cannot work out for itself is exactly what comes from outside it,
// and so that is what is recorded:
//
//   - the messages of other engines and the commands of clients, in the order the engine took them,
//     at the tick it took them;
//   - the messages the engine sent that came back undelivered, at the tick it settled them;
//   - which copy of another engine it first read at each tick, the configuration engine included.
//
// What the engines send themselves (the self list, the scheduler) is not recorded: it
// follows from the state and the inputs and is made again by the replay. Randomness comes from the
// seed of the engine and time from its tick count.
type Log interface {
	// Append records what the engine took, sent or read at the tick: a message in full (Inbound),
	// one of its own that came back undelivered (Returned), a command of a client (Command), the
	// hash of its state (Hash), or which copy of another engine it read first (StableRead).
	// Whether a queue was full or an engine gone depends on the time, so a replay cannot work out a
	// Returned; the message itself is made again by the replay of the sender.
	Append(engine sim.StateID, tick uint64, body Body)
	// EndTick says that the engine has recorded all it will record for the tick. A log that writes
	// to a file hands what it has kept for the tick to the goroutine that writes it.
	EndTick(engine sim.StateID, tick uint64)
	// Safe says up to which tick of the engine what it recorded and what it depends on is on the
	// disk, so that a crash does not take it back: all of what it did at a tick up to it, and at
	// the ticks of the engines it depends on that it took, is kept. A log that records nothing says
	// every tick is, since nothing it could do would make it more so.
	Safe(engine sim.StateID) uint64
}

// NopLog records nothing.
type NopLog struct{}

func (NopLog) Append(sim.StateID, uint64, Body) {}
func (NopLog) EndTick(sim.StateID, uint64)      {}

// Safe says every tick is: a log that records nothing cannot make a tick more safe.
func (NopLog) Safe(sim.StateID) uint64 { return ^uint64(0) }
